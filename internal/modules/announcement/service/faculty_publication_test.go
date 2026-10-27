package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/dto"
)

type facultyStore struct {
	calls int
	input dto.Publish
}

func (f *facultyStore) Publish(_ context.Context, _ string, in dto.Publish) (domain.Publication, error) {
	f.calls++
	f.input = in
	return domain.Publication{}, nil
}
func (*facultyStore) List(context.Context, string, dto.Filter) (domain.Page, error) {
	return domain.Page{}, nil
}
func (*facultyStore) Get(context.Context, string, string) (domain.Announcement, error) {
	return domain.Announcement{}, nil
}
func (*facultyStore) Summary(context.Context, string, string) (domain.DeliverySummary, error) {
	return domain.DeliverySummary{}, nil
}

func TestFacultyPublicationValidation(t *testing.T) {
	store := &facultyStore{}
	s := New(store)
	base := dto.Publish{RequestID: "10000000-0000-4000-8000-000000000001", Title: "Notice", Content: "Notice content", Audience: "faculty", Faculty: " Science "}
	if _, err := s.Publish(context.Background(), "admin", base); err != nil || store.input.Faculty != "science" {
		t.Fatal("faculty normalization", err, store.input)
	}
	for _, faculty := range []string{"", "   ", "Sci\nence", "Sci\x00ence", strings.Repeat("x", 121), string([]byte{0xff})} {
		in := base
		in.Faculty = faculty
		if _, err := s.Publish(context.Background(), "admin", in); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("invalid faculty accepted", err)
		}
	}
	for _, audience := range []string{"all_students", "", "staff", "Faculty"} {
		in := base
		in.Audience = audience
		if _, err := s.Publish(context.Background(), "admin", in); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("invalid audience/faculty combination accepted", err)
		}
	}
	if store.calls != 1 {
		t.Fatal("invalid targeting reached persistence")
	}
	base.Audience = ""
	base.Faculty = ""
	if _, err := s.Publish(context.Background(), "admin", base); err != nil || store.input.Audience != "all_students" {
		t.Fatal("legacy broadcast no longer works", err)
	}
}
