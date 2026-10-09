package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	domain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	dto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
)

const importID = "11111111-1111-1111-1111-111111111111"
const importID2 = "22222222-2222-2222-2222-222222222222"

func csvRow(id, index string) []string {
	return []string{id, index, "Student, Example", "Science", "1", "0771234567", `[{"name":"Parent","relationship":"Guardian","contact_phone":"0777654321"}]`}
}
func document(rows ...[]string) []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write(strings.Split(CSVHeader, ","))
	_ = w.WriteAll(rows)
	w.Flush()
	return b.Bytes()
}

type importStore struct {
	inputs []dto.CreateInput
	actors []string
	errors []error
	cancel context.CancelFunc
}

func (s *importStore) CreateImport(ctx context.Context, actor string, in dto.CreateInput) (domain.Profile, error) {
	s.inputs = append(s.inputs, in)
	s.actors = append(s.actors, actor)
	n := len(s.inputs) - 1
	if s.cancel != nil {
		s.cancel()
	}
	if n < len(s.errors) && s.errors[n] != nil {
		return domain.Profile{}, s.errors[n]
	}
	return domain.Profile{StudentID: in.UserID}, nil
}
func TestCSVStructuralRejectionBeforeAnyWrite(t *testing.T) {
	good := document(csvRow(importID, "A"))
	cases := map[string][]byte{
		"empty": nil, "header only": []byte(CSVHeader + "\n"), "unknown header": []byte(strings.Replace(string(good), "faculty", "department", 1)),
		"duplicate header":          []byte(strings.Replace(string(good), "faculty", "full_name", 1)),
		"truncated after valid row": append(append([]byte{}, good...), []byte(`"unterminated`)...),
		"wrong columns":             append(append([]byte{}, good...), []byte("a,b\n")...),
		"utf8":                      append(append([]byte{}, good...), 0xff), "oversize": bytes.Repeat([]byte("a"), MaxImportBytes+1),
	}
	many := [][]string{}
	for i := 0; i < MaxImportRecords+1; i++ {
		many = append(many, csvRow(importID, "A"))
	}
	cases["too many"] = document(many...)
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			store := &importStore{}
			_, err := NewImporter(store).Import(context.Background(), "admin", raw)
			if !errors.Is(err, domain.ErrInvalid) || len(store.inputs) != 0 {
				t.Fatalf("err=%v writes=%d", err, len(store.inputs))
			}
		})
	}
}
func TestCSVValidationAndDuplicateReport(t *testing.T) {
	bad := csvRow(importID2, "B")
	bad[4] = "11"
	badGuardian := csvRow(importID2, "B")
	badGuardian[6] = `[{"name":"Parent","relationship":"Parent","contact_phone":"0771234567","role":"admin"}]`
	rows := [][]string{csvRow(importID, " a "), csvRow(importID2, "A"), bad, badGuardian, csvRow(importID2, "B"), csvRow(importID2, "C")}
	store := &importStore{}
	report, err := NewImporter(store).Import(context.Background(), "verified-admin", document(rows...))
	if err != nil || report.Created != 2 || report.Rejected != 4 || report.Total != 6 || report.NotAttempted != 0 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	expected := []string{"", "duplicate_in_file", "invalid_input", "invalid_input", "", "duplicate_in_file"}
	for i, r := range report.Rows {
		if r.Record != i+1 || r.Line != i+2 || r.Error != expected[i] {
			t.Fatalf("row %+v", r)
		}
	}
	if store.inputs[0].IndexNo != "a" || store.inputs[0].FullName != "Student, Example" || store.actors[0] != "verified-admin" {
		t.Fatal("normalized CSV or actor lost")
	}
	b, _ := json.Marshal(report)
	if strings.Contains(string(b), "Parent") || strings.Contains(string(b), "077") || strings.Contains(string(b), "Student,") {
		t.Fatal("report echoed personal data")
	}
}
func TestCSVGuardiansAndHeaderVariants(t *testing.T) {
	row := csvRow(importID, "A")
	row[2] = "学生 Example"
	row[6] = `[{"name":"Guardian One","relationship":"Parent","contact_phone":"0771234567"},{"name":"Guardian Two","relationship":"Parent","contact_phone":"0771234568"}]`
	raw := append([]byte{0xef, 0xbb, 0xbf}, document(row)...)
	rows, err := parseCSV(raw)
	if err != nil || len(rows[0].input.Guardians) != 2 || rows[0].error != "" {
		t.Fatalf("BOM/guardians %v", err)
	}
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.UseCRLF = true
	header := strings.Split(CSVHeader, ",")
	for l, r := 0, len(header)-1; l < r; l, r = l+1, r-1 {
		header[l], header[r] = header[r], header[l]
		row[l], row[r] = row[r], row[l]
	}
	_ = w.Write(header)
	_ = w.Write(row)
	w.Flush()
	parsed, err := parseCSV(b.Bytes())
	if err != nil || parsed[0].input.UserID != importID || parsed[0].error != "" {
		t.Fatalf("reordered CRLF header %v", err)
	}
}
func TestCSVExpectedRejectionsAndStop(t *testing.T) {
	rows := [][]string{csvRow(importID, "A"), csvRow(importID2, "B"), csvRow("33333333-3333-3333-3333-333333333333", "C")}
	for _, fatal := range []error{domain.ErrForbidden, domain.ErrUnavailable, errors.New("private db details")} {
		store := &importStore{errors: []error{nil, fatal}}
		report, err := NewImporter(store).Import(context.Background(), "admin", document(rows...))
		if err != nil || report.Created != 1 || report.Rejected != 1 || report.NotAttempted != 1 || len(store.inputs) != 2 || report.StoppedReason == "" {
			t.Fatalf("stop %+v %v", report, err)
		}
		if report.Rows[2].Status != "not_attempted" || strings.Contains(report.Rows[1].Error, "private") {
			t.Fatal("unsafe/misleading failure")
		}
	}
	store := &importStore{errors: []error{domain.ErrConflict, domain.ErrAccount}}
	report, err := NewImporter(store).Import(context.Background(), "admin", document(rows...))
	if err != nil || report.Created != 1 || report.Rejected != 2 || report.StoppedReason != "" {
		t.Fatalf("row errors %+v", report)
	}
	ctx, cancel := context.WithCancel(context.Background())
	store = &importStore{cancel: cancel}
	report, err = NewImporter(store).Import(ctx, "admin", document(rows...))
	if err != nil || report.Created != 1 || report.NotAttempted != 2 || len(store.inputs) != 1 {
		t.Fatalf("cancel %+v", report)
	}
}
