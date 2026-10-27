package service

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/dto"
	student "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/google/uuid"
	"strings"
	"testing"
)

type invitationRepoFake struct {
	calls int
	err   error
	hash  []byte
}

func (f *invitationRepoFake) RegisterInvitation(_ context.Context, _, _ string, v dto.StudentInvitationInput, hash []byte) (d.StudentInvitation, bool, error) {
	f.calls++
	f.hash = append([]byte{}, hash...)
	return d.StudentInvitation{InvitationID: v.RequestID}, false, f.err
}
func (f *invitationRepoFake) GetInvitation(context.Context, string, string) (d.StudentInvitation, error) {
	return d.StudentInvitation{}, f.err
}
func (f *invitationRepoFake) ListInvitations(context.Context, string, int, int) (d.InvitationPage, error) {
	return d.InvitationPage{}, f.err
}
func (f *invitationRepoFake) ResendInvitation(context.Context, string, string, string, string) (d.StudentInvitation, error) {
	return d.StudentInvitation{}, f.err
}
func (f *invitationRepoFake) CompleteSetup(context.Context, string, func(context.Context, string, string, float64) error) (d.SetupResult, error) {
	return d.SetupResult{}, f.err
}
func (f *invitationRepoFake) DispatchInvitation(context.Context, InvitationProvider) (bool, error) {
	return false, f.err
}
func invitationInput() dto.StudentInvitationInput {
	v := dto.StudentInvitationInput{RequestID: uuid.NewString(), Email: " Student@STU.KLN.AC.LK "}
	v.IndexNo = "TEST-95"
	v.FullName = "Student"
	v.Faculty = "Science"
	v.Year = 1
	v.ContactPhone = "0771234567"
	v.Guardians = []student.GuardianInput{{Name: "Guardian", Relationship: "Parent", ContactPhone: "0771234568"}}
	return v
}
func TestInvitationInputValidation(t *testing.T) {
	repo := &invitationRepoFake{}
	s := NewInvitations(repo, nil, []string{"stu.kln.ac.lk"}, true)
	v := invitationInput()
	normalized, e := s.normalize(v)
	if e != nil || normalized.Email != "student@stu.kln.ac.lk" {
		t.Fatal(normalized, e)
	}
	for _, edit := range []func(*dto.StudentInvitationInput){
		func(v *dto.StudentInvitationInput) { v.Email = "student@gmail.com" }, func(v *dto.StudentInvitationInput) { v.Email = "student@evil.stu.kln.ac.lk" },
		func(v *dto.StudentInvitationInput) { v.Email = "Name <student@stu.kln.ac.lk>" }, func(v *dto.StudentInvitationInput) { v.Email = "student@stu.kln.ac.lk.evil" },
		func(v *dto.StudentInvitationInput) { v.RequestID = uuid.Nil.String() }, func(v *dto.StudentInvitationInput) { v.RequestID = "bad" },
		func(v *dto.StudentInvitationInput) { v.Guardians = nil }, func(v *dto.StudentInvitationInput) { v.Year = 0 },
	} {
		bad := invitationInput()
		edit(&bad)
		if _, _, e = s.Register(context.Background(), "admin", "127.0.0.1", bad); !errors.Is(e, d.ErrInvitationInvalid) {
			t.Fatal("invalid accepted", bad, e)
		}
	}
	if repo.calls != 0 {
		t.Fatal("invalid inputs reached SQL")
	}
	if _, _, e = s.Register(context.Background(), "admin", "127.0.0.1", v); e != nil || repo.calls != 1 || len(repo.hash) != 32 {
		t.Fatal("valid registration", e)
	}
	if _, _, e = s.Register(context.Background(), "", "127.0.0.1", v); !errors.Is(e, d.ErrInvitationForbidden) {
		t.Fatal(e)
	}
	if _, _, e = s.Register(context.Background(), "admin", "invalid", v); !errors.Is(e, d.ErrInvitationInvalid) {
		t.Fatal(e)
	}
	if _, _, e = NewInvitations(repo, nil, nil, false).Register(context.Background(), "admin", "127.0.0.1", v); !errors.Is(e, d.ErrInvitationUnavailable) {
		t.Fatal("disabled")
	}
	if _, e = s.Get(context.Background(), "admin", "bad"); !errors.Is(e, d.ErrInvitationInvalid) {
		t.Fatal("bad id")
	}
	if _, e = s.List(context.Background(), "admin", 101, 0); !errors.Is(e, d.ErrInvitationInvalid) {
		t.Fatal("bad limit")
	}
	if _, e = s.Resend(context.Background(), "admin", uuid.NewString(), uuid.NewString(), "bad"); !errors.Is(e, d.ErrInvitationInvalid) {
		t.Fatal("bad IP")
	}
}

func TestInvitationGmailAndUniversityDomains(t *testing.T) {
	for _, tc := range []struct {
		email, normalized string
		valid             bool
	}{
		{"student@gmail.com", "student@gmail.com", true},
		{" Student@GMAIL.COM ", "student@gmail.com", true},
		{"student+hostel@gmail.com", "student+hostel@gmail.com", true},
		{"student@stu.kln.ac.lk", "student@stu.kln.ac.lk", true},
		{" Student@STU.KLN.AC.LK ", "student@stu.kln.ac.lk", true},
		{"student@other.example", "", false},
		{"student@gmail.com.evil.example", "", false},
		{"student@sub.gmail.com", "", false},
		{"student@evil.stu.kln.ac.lk", "", false},
		{"student@stu.kln.ac.lk.evil.example", "", false},
		{"not-an-email", "", false},
	} {
		t.Run(tc.email, func(t *testing.T) {
			repo := &invitationRepoFake{}
			s := NewInvitations(repo, nil, []string{"gmail.com", "stu.kln.ac.lk"}, true)
			input := invitationInput()
			input.Email = tc.email
			if tc.valid {
				normalized, err := s.normalize(input)
				if err != nil || normalized.Email != tc.normalized {
					t.Fatal("email normalization", normalized.Email, err)
				}
				if _, _, err = s.Register(context.Background(), "admin", "127.0.0.1", input); err != nil || repo.calls != 1 {
					t.Fatal("approved email rejected", err)
				}
			} else if _, _, err := s.Register(context.Background(), "admin", "127.0.0.1", input); !errors.Is(err, d.ErrInvitationInvalid) || repo.calls != 0 {
				t.Fatal("unapproved email reached repository", err)
			}
		})
	}
	// CSV registration uses the same policy as individual registration.
	repo := &invitationRepoFake{}
	s := NewInvitations(repo, nil, []string{"gmail.com", "stu.kln.ac.lk"}, true)
	gmail, university := invitationInput(), invitationInput()
	gmail.Email = "student@gmail.com"
	gmail.IndexNo = "GMAIL-STUDENT"
	university.Email = "student@stu.kln.ac.lk"
	university.IndexNo = "UNIVERSITY-STUDENT"
	report, err := s.Import(context.Background(), "admin", "127.0.0.1", invitationCSV(gmail, university))
	if err != nil || report.Queued != 2 || report.Rejected != 0 || repo.calls != 2 {
		t.Fatal("mixed-domain CSV registration", report, err)
	}
}
func invitationCSV(vs ...dto.StudentInvitationInput) []byte {
	var b strings.Builder
	w := csv.NewWriter(&b)
	_ = w.Write(strings.Split("request_id,"+InvitationCSVHeader, ","))
	for _, v := range vs {
		g, _ := json.Marshal(v.Guardians)
		_ = w.Write([]string{v.RequestID, v.Email, v.IndexNo, v.FullName, v.Faculty, "1", v.ContactPhone, string(g)})
	}
	w.Flush()
	return []byte(b.String())
}
func TestInvitationCSVContractAndPartialResults(t *testing.T) {
	repo := &invitationRepoFake{}
	s := NewInvitations(repo, nil, []string{"stu.kln.ac.lk"}, true)
	good := invitationInput()
	bad := invitationInput()
	bad.Email = "not-university@gmail.com"
	raw := invitationCSV(good, good, bad)
	report, e := s.Import(context.Background(), "admin", "127.0.0.1", raw)
	if e != nil || report.Queued != 1 || report.Rejected != 2 || repo.calls != 1 || report.Rows[1].Error != "duplicate_in_file" {
		t.Fatalf("%+v %v", report, e)
	}
	for _, raw := range [][]byte{[]byte("bad,header\n"), []byte(InvitationCSVHeader + "\n"), append([]byte{0xff}, invitationCSV(good)...), []byte(strings.Replace(string(invitationCSV(good)), "request_id", "password", 1)), []byte(InvitationCSVHeader + "\n\"unclosed")} {
		if _, e := s.Import(context.Background(), "admin", "127.0.0.1", raw); !errors.Is(e, d.ErrInvitationInvalid) {
			t.Fatal("malformed CSV accepted", e)
		}
	}
	second := invitationInput()
	second.Email = "second@stu.kln.ac.lk"
	second.IndexNo = "SECOND"
	for _, tc := range []struct {
		err                error
		retry, reject, not int
	}{
		{d.ErrInvitationConflict, 0, 2, 0}, {d.ErrInvitationForbidden, 0, 0, 2}, {d.ErrInvitationUnavailable, 1, 0, 1},
	} {
		repo.err = tc.err
		r, e := s.Import(context.Background(), "admin", "127.0.0.1", invitationCSV(good, second))
		if e != nil || r.RetryRequired != tc.retry || r.Rejected != tc.reject || r.NotAttempted != tc.not {
			t.Fatalf("%+v %v", r, e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, e := s.Import(ctx, "admin", "127.0.0.1", invitationCSV(good))
	if e != nil || r.NotAttempted != 1 {
		t.Fatal("interruption", r, e)
	}
}

func TestEmailCSVStableKeysAndRegistration(t *testing.T) {
	repo := &invitationRepoFake{}
	s := NewInvitations(repo, nil, []string{"stu.kln.ac.lk"}, true)
	input := invitationInput()
	g, _ := json.Marshal(input.Guardians)
	var b strings.Builder
	w := csv.NewWriter(&b)
	_ = w.Write(strings.Split(InvitationCSVHeader, ","))
	_ = w.Write([]string{input.Email, input.IndexNo, input.FullName, input.Faculty, "1", input.ContactPhone, string(g)})
	w.Flush()
	raw := []byte(b.String())
	rows, err := s.parseInvitationCSV("admin", raw)
	if err != nil || len(rows) != 1 || rows[0].code != "" || !invitationUUID(rows[0].input.RequestID) {
		t.Fatal("email-only CSV rejected", rows, err)
	}
	again, _ := s.parseInvitationCSV("admin", raw)
	other, _ := s.parseInvitationCSV("another-admin", raw)
	if rows[0].input.RequestID != again[0].input.RequestID || rows[0].input.RequestID == other[0].input.RequestID {
		t.Fatal("row key must be stable and actor scoped")
	}
	canonical, _ := s.parseInvitationCSV("admin", []byte(strings.Replace(string(raw), input.Email, "student@stu.kln.ac.lk", 1)))
	if rows[0].input.RequestID != canonical[0].input.RequestID {
		t.Fatal("normalization changed exact replay key")
	}
	changed, _ := s.parseInvitationCSV("admin", []byte(strings.Replace(string(raw), input.FullName, "Different Name", 1)))
	if rows[0].input.RequestID == changed[0].input.RequestID {
		t.Fatal("changed data must not replay another payload")
	}
	report, err := s.Import(context.Background(), "admin", "127.0.0.1", raw)
	if err != nil || report.Queued != 1 || repo.calls != 1 {
		t.Fatal("registration flow was not used", report, err)
	}
	if _, err := s.Import(context.Background(), "", "127.0.0.1", raw); !errors.Is(err, d.ErrInvitationForbidden) {
		t.Fatal("missing actor accepted", err)
	}
	parts := strings.SplitN(string(raw), "\n", 2)
	before := repo.calls
	for _, malformed := range []string{string(raw) + "\"unclosed", parts[0] + "\n" + strings.Repeat(parts[1], 101)} {
		if _, err := s.Import(context.Background(), "admin", "127.0.0.1", []byte(malformed)); !errors.Is(err, d.ErrInvitationInvalid) {
			t.Fatal("invalid whole file accepted", err)
		}
	}
	if repo.calls != before {
		t.Fatal("structural failure caused partial writes")
	}
}
