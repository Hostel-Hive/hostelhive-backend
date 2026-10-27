package service

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/dto"
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
func TestValidationAndCanonicalRetries(t *testing.T) {
	f := &fake{}
	s := New(f)
	ctx := context.Background()
	id := "abcdef01-0000-4000-8000-000000000001"
	valid := dto.Submit{RequestID: strings.ToUpper(id), Direction: "IN", ExpectedAt: time.Date(2027, 1, 1, 21, 0, 0, 123456789, time.FixedZone("Colombo", 19800)), Reason: "  Travel  "}
	for i := range 9 {
		v := valid
		switch i {
		case 0:
			v.RequestID = "bad"
		case 1:
			v.Direction = "entry"
		case 2:
			v.ExpectedAt = time.Time{}
		case 3:
			v.Reason = " "
		case 4:
			v.Reason = strings.Repeat("x", 1001)
		case 5:
			v.Reason = "bad\x00"
		case 6:
			v.ExpectedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
		case 7:
			v.Reason = string([]byte{0xff})
		case 8:
			v.ExpectedAt = time.Date(1, 1, 1, 0, 0, 0, 0, time.FixedZone("offset", 3600))
		}
		if _, e := s.Submit(ctx, "actor", v); !errors.Is(e, domain.ErrInvalid) {
			t.Fatalf("case %d: %v", i, e)
		}
	}
	if f.calls != 0 {
		t.Fatal("invalid payload reached storage")
	}
	if _, e := s.Submit(ctx, "actor", valid); e != nil || f.input.RequestID != id || f.input.Reason != "Travel" || f.input.ExpectedAt.Location() != time.UTC || f.input.ExpectedAt.Nanosecond() != 123456000 {
		t.Fatal(f.input, e)
	}
	for _, v := range []dto.Decision{{Status: "pending"}, {Status: "rejected"}, {Status: "approved", Note: "\x00"}, {Status: "approved", Note: strings.Repeat("x", 1001)}} {
		if _, e := s.Decide(ctx, "actor", id, v); !errors.Is(e, domain.ErrInvalid) {
			t.Fatal(e)
		}
	}
	if _, e := s.Decide(ctx, "actor", id, dto.Decision{Status: "approved"}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Decide(ctx, "actor", id, dto.Decision{Status: "rejected", Note: "reason"}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Get(ctx, "actor", "bad"); !errors.Is(e, domain.ErrInvalid) {
		t.Fatal(e)
	}
	if _, e := s.Get(ctx, "actor", id); e != nil {
		t.Fatal(e)
	}
	for _, v := range []domain.Filter{{Limit: 0}, {Limit: 101}, {Limit: 20, Offset: -1}, {Limit: 20, Status: "bad"}, {Limit: 20, StudentID: "bad"}} {
		if _, e := s.List(ctx, "actor", v); !errors.Is(e, domain.ErrInvalid) {
			t.Fatal(e)
		}
	}
	if _, e := s.List(ctx, "actor", domain.Filter{Limit: 20, Status: "pending"}); e != nil {
		t.Fatal(e)
	}
}
