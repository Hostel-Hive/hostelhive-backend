package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/dto"
	"github.com/google/uuid"
)

type goingHomeFake struct {
	input    dto.GoingHomeInput
	calls    int
	replayed bool
	err      error
}

func (f *goingHomeFake) DeclareGoingHome(_ context.Context, actor string, in dto.GoingHomeInput) (domain.GoingHomeDeclaration, bool, error) {
	f.calls++
	f.input = in
	return domain.GoingHomeDeclaration{RequestID: in.RequestID}, f.replayed, f.err
}
func TestGoingHomeValidation(t *testing.T) {
	f := &goingHomeFake{}
	s := NewGoingHome(f)
	ctx := context.Background()
	for _, id := range []string{"", "invalid", "00000000-0000-0000-0000-000000000000"} {
		if _, _, e := s.DeclareGoingHome(ctx, "student", dto.GoingHomeInput{RequestID: id}); !errors.Is(e, domain.ErrInvalid) {
			t.Fatal(id, e)
		}
	}
	if _, _, e := s.DeclareGoingHome(ctx, " ", dto.GoingHomeInput{RequestID: uuid.NewString()}); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal(e)
	}
	if f.calls != 0 {
		t.Fatal("invalid input reached storage")
	}
	id := uuid.NewString()
	v, replay, e := s.DeclareGoingHome(ctx, "student", dto.GoingHomeInput{RequestID: strings.ToUpper(id)})
	if e != nil || replay || v.RequestID != id || f.input.RequestID != id {
		t.Fatal(v, replay, e)
	}
	f.replayed = true
	if _, replay, e = s.DeclareGoingHome(ctx, "student", dto.GoingHomeInput{RequestID: id}); e != nil || !replay {
		t.Fatal(replay, e)
	}
	f.err = domain.ErrAlreadyOutside
	if _, _, e = s.DeclareGoingHome(ctx, "student", dto.GoingHomeInput{RequestID: id}); !errors.Is(e, domain.ErrAlreadyOutside) {
		t.Fatal(e)
	}
}
