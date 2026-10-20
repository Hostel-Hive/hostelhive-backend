package service

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/dto"
	"strings"
	"testing"
	"time"
)

type fake struct {
	calls int
	input dto.Submit
}

func (f *fake) Submit(_ context.Context, _ string, in dto.Submit) (domain.Request, error) {
	f.calls++
	f.input = in
	return domain.Request{}, nil
}
func (f *fake) Decide(context.Context, string, string, dto.Decision) (domain.Request, error) {
	f.calls++
	return domain.Request{}, nil
}
func (f *fake) Cancel(context.Context, string, string) (domain.Request, error) {
	f.calls++
	return domain.Request{}, nil
}
func (f *fake) Get(context.Context, string, string) (domain.Request, error) {
	f.calls++
	return domain.Request{}, nil
}
func (f *fake) List(context.Context, string, domain.Filter) (domain.Page, error) {
	f.calls++
	return domain.Page{}, nil
}
func TestValidation(t *testing.T) {
	f := &fake{}
	s := New(f)
	ctx := context.Background()
	id := "10000000-0000-4000-8000-000000000001"
	valid := dto.Submit{RequestID: id, StartsAt: time.Now().Add(time.Hour), EndsAt: time.Now().Add(2 * time.Hour), Reason: " Family visit "}
	bad := []dto.Submit{}
	for i := range 7 {
		v := valid
		switch i {
		case 0:
			v.RequestID = "bad"
		case 1:
			v.StartsAt = time.Time{}
		case 2:
			v.EndsAt = v.StartsAt
		case 3:
			v.Reason = " "
		case 4:
			v.Reason = strings.Repeat("x", 1001)
		case 5:
			v.Reason = "bad\x00"
		case 6:
			v.EndsAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		bad = append(bad, v)
	}
	for _, in := range bad {
		_, e := s.Submit(ctx, "actor", in)
		if !errors.Is(e, domain.ErrInvalid) {
			t.Fatal(e)
		}
	}
	if f.calls != 0 {
		t.Fatal("invalid input reached repository")
	}
	if _, e := s.Submit(ctx, "actor", valid); e != nil || f.input.Reason != "Family visit" || f.input.StartsAt.Nanosecond()%1000 != 0 {
		t.Fatal(f.input, e)
	}
	for _, in := range []dto.Decision{{Status: "pending"}, {Status: "rejected"}, {Status: "approved", Note: "\x00"}} {
		_, e := s.Decide(ctx, "actor", id, in)
		if !errors.Is(e, domain.ErrInvalid) {
			t.Fatal(e)
		}
	}
	if _, e := s.Decide(ctx, "actor", id, dto.Decision{Status: "approved"}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Decide(ctx, "actor", id, dto.Decision{Status: "rejected", Note: " reason "}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Get(ctx, "actor", "bad"); !errors.Is(e, domain.ErrInvalid) {
		t.Fatal(e)
	}
	if _, e := s.Get(ctx, "actor", id); e != nil {
		t.Fatal(e)
	}
	for _, v := range []domain.Filter{{Limit: 0}, {Limit: 101}, {Limit: 20, Offset: -1}, {Limit: 20, Status: "bad"}, {Limit: 20, StudentID: "bad"}} {
		_, e := s.List(ctx, "actor", v)
		if !errors.Is(e, domain.ErrInvalid) {
			t.Fatal(e)
		}
	}
	if _, e := s.List(ctx, "actor", domain.Filter{Limit: 20, Status: "pending"}); e != nil {
		t.Fatal(e)
	}
}
