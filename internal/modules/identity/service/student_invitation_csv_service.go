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

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/dto"
)

const InvitationCSVHeader = "request_id,email,index_no,full_name,faculty,year,contact_phone,guardians"
const MaxInvitationCSVBytes = 2 * 1024 * 1024
const MaxInvitationCSVRows = 100

type invitationCSVRow struct {
	line  int
	input dto.StudentInvitationInput
	code  string
}
type InvitationImportRow struct {
	Record       int    `json:"record"`
	Line         int    `json:"line"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	InvitationID string `json:"invitation_id,omitempty"`
}
type InvitationImportReport struct {
	Total         int                   `json:"total"`
	Queued        int                   `json:"queued"`
	Replayed      int                   `json:"replayed"`
	Rejected      int                   `json:"rejected"`
	NotAttempted  int                   `json:"not_attempted"`
	RetryRequired int                   `json:"retry_required"`
	StoppedReason string                `json:"stopped_reason,omitempty"`
	Rows          []InvitationImportRow `json:"rows"`
}

func (s *Invitations) parseInvitationCSV(raw []byte) ([]invitationCSVRow, error) {
	if len(raw) > MaxInvitationCSVBytes || !utf8.Valid(raw) {
		return nil, d.ErrInvitationInvalid
	}
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})))
	headers, e := reader.Read()
	if e != nil || len(headers) != 8 {
		return nil, d.ErrInvitationInvalid
	}
	positions := map[string]int{}
	for n, h := range headers {
		if _, ok := positions[h]; ok {
			return nil, d.ErrInvitationInvalid
		}
		positions[h] = n
	}
	for _, h := range strings.Split(InvitationCSVHeader, ",") {
		if _, ok := positions[h]; !ok {
			return nil, d.ErrInvitationInvalid
		}
	}
	rows := []invitationCSVRow{}
	seenEmails, seenIndexes, seenRequests := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for {
		fields, e := reader.Read()
		if e == io.EOF {
			break
		}
		if e != nil || len(rows) >= MaxInvitationCSVRows {
			return nil, d.ErrInvitationInvalid
		}
		line, _ := reader.FieldPos(0)
		row := invitationCSVRow{line: line}
		value := func(k string) string { return fields[positions[k]] }
		v := dto.StudentInvitationInput{RequestID: value("request_id"), Email: value("email")}
		v.IndexNo, v.FullName, v.Faculty, v.ContactPhone = value("index_no"), value("full_name"), value("faculty"), value("contact_phone")
		var yearErr error
		v.Year, yearErr = strconv.Atoi(strings.TrimSpace(value("year")))
		dec := json.NewDecoder(strings.NewReader(value("guardians")))
		dec.DisallowUnknownFields()
		guardianErr := dec.Decode(&v.Guardians)
		if guardianErr == nil {
			var extra any
			if dec.Decode(&extra) != io.EOF {
				guardianErr = d.ErrInvitationInvalid
			}
		}
		normalized, e := s.normalize(v)
		if e != nil || yearErr != nil || guardianErr != nil {
			row.code = "invalid_input"
		} else {
			row.input = normalized
			index := strings.ToLower(normalized.IndexNo)
			if seenEmails[normalized.Email] || seenIndexes[index] || seenRequests[normalized.RequestID] {
				row.code = "duplicate_in_file"
			} else {
				seenEmails[normalized.Email], seenIndexes[index], seenRequests[normalized.RequestID] = true, true, true
			}
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, d.ErrInvitationInvalid
	}
	return rows, nil
}
func (s *Invitations) Import(ctx context.Context, actor, ip string, raw []byte) (InvitationImportReport, error) {
	report := InvitationImportReport{Rows: []InvitationImportRow{}}
	if !s.enabled {
		return report, d.ErrInvitationUnavailable
	}
	rows, e := s.parseInvitationCSV(raw)
	if e != nil {
		return report, e
	}
	report.Total = len(rows)
	for n, row := range rows {
		out := InvitationImportRow{Record: n + 1, Line: row.line, Status: "rejected", Error: row.code}
		if row.code != "" {
			report.Rejected++
			report.Rows = append(report.Rows, out)
			continue
		}
		if report.StoppedReason != "" || ctx.Err() != nil {
			if report.StoppedReason == "" {
				report.StoppedReason = "import_interrupted"
			}
			out.Status, out.Error = "not_attempted", report.StoppedReason
			report.NotAttempted++
			report.Rows = append(report.Rows, out)
			continue
		}
		v, replayed, e := s.Register(ctx, actor, ip, row.input)
		switch {
		case e == nil:
			out.Status, out.InvitationID = "queued", v.InvitationID
			if replayed {
				out.Status = "replayed"
				report.Replayed++
			} else {
				report.Queued++
			}
		case errors.Is(e, d.ErrInvitationConflict):
			out.Error = "invitation_conflict"
			report.Rejected++
		case errors.Is(e, d.ErrInvitationInvalid):
			out.Error = "invalid_input"
			report.Rejected++
		case errors.Is(e, d.ErrInvitationForbidden):
			out.Status, out.Error = "not_attempted", "forbidden"
			report.NotAttempted++
			report.StoppedReason = "forbidden"
		default:
			out.Status, out.Error = "retry_required", "registration_unavailable"
			report.RetryRequired++
			report.StoppedReason = "registration_unavailable"
		}
		report.Rows = append(report.Rows, out)
	}
	return report, nil
}
