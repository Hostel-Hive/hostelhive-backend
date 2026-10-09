package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	domain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	dto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
)

const MaxImportBytes = 2 * 1024 * 1024
const MaxImportRecords = 500
const CSVHeader = "user_id,index_no,full_name,faculty,year,contact_phone,guardians"

// The repository must recheck active Admin authority inside each transaction.
type ImportRepository interface {
	CreateImport(context.Context, string, dto.CreateInput) (domain.Profile, error)
}
type Importer struct{ repository ImportRepository }

func NewImporter(r ImportRepository) *Importer { return &Importer{repository: r} }

type importRow struct {
	line  int
	input dto.CreateInput
	error string
}

// Parse the complete document before writes. Structural corruption rejects it all.
func parseCSV(raw []byte) ([]importRow, error) {
	if len(raw) > MaxImportBytes || !utf8.Valid(raw) {
		return nil, domain.ErrInvalid
	}
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})))
	header, err := r.Read()
	if err != nil || len(header) != 7 {
		return nil, domain.ErrInvalid
	}
	positions := make(map[string]int)
	required := strings.Split(CSVHeader, ",")
	for i, h := range header {
		if _, ok := positions[h]; ok {
			return nil, domain.ErrInvalid
		}
		positions[h] = i
	}
	for _, h := range required {
		if _, ok := positions[h]; !ok {
			return nil, domain.ErrInvalid
		}
	}
	rows := []importRow{}
	seenUsers, seenIndexes := map[string]bool{}, map[string]bool{}
	for {
		fields, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(rows) >= MaxImportRecords {
			return nil, domain.ErrInvalid
		}
		line, _ := r.FieldPos(0)
		row := importRow{line: line}
		value := func(k string) string { return fields[positions[k]] }
		row.input.UserID = strings.ToLower(strings.TrimSpace(value("user_id")))
		row.input.IndexNo = value("index_no")
		row.input.FullName = value("full_name")
		row.input.Faculty = value("faculty")
		row.input.ContactPhone = value("contact_phone")
		year, yearErr := strconv.Atoi(strings.TrimSpace(value("year")))
		row.input.Year = year
		decoder := json.NewDecoder(strings.NewReader(value("guardians")))
		decoder.DisallowUnknownFields()
		guardianErr := decoder.Decode(&row.input.Guardians)
		if guardianErr == nil {
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				guardianErr = domain.ErrInvalid
			}
		}
		normalized, validationErr := Normalize(row.input.Details)
		if yearErr != nil || guardianErr != nil || validationErr != nil || !validation.UUID(row.input.UserID) {
			row.error = "invalid_input"
		} else {
			row.input.Details = normalized
			index := strings.ToLower(normalized.IndexNo)
			if seenUsers[row.input.UserID] || seenIndexes[index] {
				row.error = "duplicate_in_file"
			} else {
				seenUsers[row.input.UserID] = true
				seenIndexes[index] = true
			}
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, domain.ErrInvalid
	}
	return rows, nil
}

func (s *Importer) Import(ctx context.Context, actor string, raw []byte) (domain.ImportReport, error) {
	report := domain.ImportReport{Rows: []domain.ImportRowResult{}}
	if strings.TrimSpace(actor) == "" {
		return report, domain.ErrForbidden
	}
	rows, err := parseCSV(raw)
	if err != nil {
		return report, err
	}
	report.Total = len(rows)
	for i, row := range rows {
		result := domain.ImportRowResult{Record: i + 1, Line: row.line, Status: "rejected", Error: row.error}
		if row.error != "" {
			report.Rejected++
			report.Rows = append(report.Rows, result)
			continue
		}
		if report.StoppedReason != "" {
			result.Status = "not_attempted"
			result.Error = report.StoppedReason
			report.NotAttempted++
			report.Rows = append(report.Rows, result)
			continue
		}
		if ctx.Err() != nil {
			report.StoppedReason = "import_interrupted"
			result.Status = "not_attempted"
			result.Error = report.StoppedReason
			report.NotAttempted++
			report.Rows = append(report.Rows, result)
			continue
		}
		profile, err := s.repository.CreateImport(ctx, actor, row.input)
		switch {
		case err == nil:
			result.Status = "created"
			result.StudentID = profile.StudentID
			report.Created++
		case errors.Is(err, domain.ErrConflict):
			result.Error = "duplicate_student"
			report.Rejected++
		case errors.Is(err, domain.ErrAccount):
			result.Error = "active_student_account_required"
			report.Rejected++
		case errors.Is(err, domain.ErrInvalid):
			result.Error = "invalid_input"
			report.Rejected++
		default:
			report.StoppedReason = "student_profiles_unavailable"
			if errors.Is(err, domain.ErrForbidden) {
				report.StoppedReason = "forbidden"
			}
			if ctx.Err() != nil {
				report.StoppedReason = "import_interrupted"
			}
			result.Error = report.StoppedReason
			report.Rejected++
		}
		report.Rows = append(report.Rows, result)
	}
	return report, nil
}
