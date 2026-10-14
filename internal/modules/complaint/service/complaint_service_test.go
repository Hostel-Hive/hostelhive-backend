package service

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/dto"
	"strings"
	"testing"
	"time"
)

type fake struct{ calls int }

func (f *fake) Submit(context.Context, string, dto.Submit) (domain.Complaint, error) {
	f.calls++
	return domain.Complaint{}, nil
}
func (f *fake) Get(context.Context, string, string) (domain.Complaint, error) {
	f.calls++
	return domain.Complaint{}, nil
}
func (f *fake) List(context.Context, string, domain.Filter) (domain.Page, error) {
	f.calls++
	return domain.Page{}, nil
}
func (f *fake) Change(context.Context, string, string, string, dto.Change) (domain.Update, error) {
	f.calls++
	return domain.Update{}, nil
}
func (f *fake) Updates(context.Context, string, string, int, int) (domain.UpdatePage, error) {
	f.calls++
	return domain.UpdatePage{}, nil
}
func TestValidation(t *testing.T) {
	f := &fake{}
	s := New(f)
	ctx := context.Background()
	id := "10000000-0000-4000-8000-000000000001"
	for _, in := range []dto.Submit{{RequestID: "bad", Category: "maintenance", Description: "Tap"}, {RequestID: id, Category: "invalid", Description: "Tap"}, {RequestID: id, Category: "maintenance", Description: " "}, {RequestID: id, Category: "maintenance", Description: strings.Repeat("x", 2001)}, {RequestID: id, Category: "maintenance", Description: "\x00"}} {
		_, e := s.Submit(ctx, "actor", in)
		if !errors.Is(e, domain.ErrInvalid) {
			t.Fatal(e)
		}
	}
	if f.calls != 0 {
		t.Fatal("invalid input reached store")
	}
	if _, e := s.Submit(ctx, "actor", dto.Submit{RequestID: id, Category: "maintenance", Description: " Tap "}); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		action string
		in     dto.Change
	}{
		{"note", dto.Change{RequestID: id, ExpectedVersion: 1, Text: " "}},
		{"note", dto.Change{RequestID: id, ExpectedVersion: 1, Text: "text", Status: "resolved"}},
		{"status", dto.Change{RequestID: id, ExpectedVersion: 1, Status: "resolved"}},
		{"status", dto.Change{RequestID: id, ExpectedVersion: 1, Status: "pending"}},
		{"bad", dto.Change{RequestID: id, ExpectedVersion: 1, Text: "x"}},
		{"note", dto.Change{RequestID: "bad", ExpectedVersion: 1, Text: "x"}},
		{"note", dto.Change{RequestID: id, ExpectedVersion: 0, Text: "x"}},
		{"note", dto.Change{RequestID: id, ExpectedVersion: 9007199254740991, Text: "x"}},
	} {
		_, e := s.Change(ctx, "actor", id, tc.action, tc.in)
		if !errors.Is(e, domain.ErrInvalid) {
			t.Fatal(tc, e)
		}
	}
	if _, e := s.Change(ctx, "actor", id, "note", dto.Change{RequestID: id, ExpectedVersion: 1, Text: "Progress"}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Change(ctx, "actor", id, "status", dto.Change{RequestID: id, ExpectedVersion: 1, Status: "resolved", Text: "Fixed"}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Get(ctx, "actor", "bad"); !errors.Is(e, domain.ErrInvalid) {
		t.Fatal(e)
	}
	if _, e := s.Get(ctx, "actor", id); e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	past := now.Add(-time.Hour)
	for _, in := range []domain.Filter{{Limit: 0}, {Limit: 101}, {Limit: 20, Offset: -1}, {Limit: 20, Category: "bad"}, {Limit: 20, Status: "bad"}, {Limit: 20, StudentID: "bad"}, {Limit: 20, From: &now, To: &past}} {
		_, e := s.List(ctx, "actor", in)
		if !errors.Is(e, domain.ErrInvalid) {
			t.Fatal(e)
		}
	}
	if _, e := s.List(ctx, "actor", domain.Filter{Limit: 20, From: &past, To: &now}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Updates(ctx, "actor", id, 0, 0); !errors.Is(e, domain.ErrInvalid) {
		t.Fatal(e)
	}
	if _, e := s.Updates(ctx, "actor", id, 20, 0); e != nil {
		t.Fatal(e)
	}
}
