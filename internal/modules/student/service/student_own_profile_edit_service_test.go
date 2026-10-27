package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	s "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
)

type editOwnRepo struct {
	calls int
	actor string
	input dto.OwnProfilePatch
	err   error
}

func (r *editOwnRepo) PatchOwnProfile(_ context.Context, actor string, input dto.OwnProfilePatch) (d.OwnProfile, error) {
	r.calls++
	r.actor, r.input = actor, input
	return d.OwnProfile{StudentID: "owned"}, r.err
}
func ownString(v string) *string                            { return &v }
func ownGuardians(gs ...d.GuardianInput) *[]d.GuardianInput { return &gs }

func TestOwnProfilePatchValidation(t *testing.T) {
	guardian := d.GuardianInput{Name: " Mock Guardian ", Relationship: " Mother ", ContactPhone: " 0712345678 "}
	for _, input := range []dto.OwnProfilePatch{
		{ContactPhone: ownString(" 0771234567 ")},
		{Guardians: ownGuardians(guardian)},
		{ContactPhone: ownString("0771234567"), Guardians: ownGuardians(guardian)},
	} {
		r := &editOwnRepo{}
		if p, err := s.NewOwnProfileEditor(r).Patch(context.Background(), "verified-student", input); err != nil || p.StudentID != "owned" || r.actor != "verified-student" {
			t.Fatal(p, err)
		}
		if r.input.ContactPhone != nil && *r.input.ContactPhone != "0771234567" {
			t.Fatal(r.input)
		}
		if r.input.Guardians != nil && (*r.input.Guardians)[0].Name != "Mock Guardian" {
			t.Fatal(r.input)
		}
		if input.Guardians != nil && (*input.Guardians)[0].Name != guardian.Name {
			t.Fatal("caller guardian slice was mutated")
		}
	}
	for _, input := range []dto.OwnProfilePatch{
		{}, {ContactPhone: ownString("")}, {ContactPhone: ownString("123456")}, {ContactPhone: ownString("bad-phone")},
		{ContactPhone: ownString(strings.Repeat("7", 33))}, {Guardians: ownGuardians()},
		{Guardians: ownGuardians(make([]d.GuardianInput, 11)...)},
		{Guardians: ownGuardians(d.GuardianInput{Name: "", Relationship: "Mother", ContactPhone: "0712345678"})},
		{Guardians: ownGuardians(d.GuardianInput{Name: strings.Repeat("a", 201), Relationship: "Mother", ContactPhone: "0712345678"})},
		{Guardians: ownGuardians(d.GuardianInput{Name: "Guardian", Relationship: strings.Repeat("a", 81), ContactPhone: "0712345678"})},
		{Guardians: ownGuardians(d.GuardianInput{Name: "Guardian\n", Relationship: "Mo\nther", ContactPhone: "0712345678"})},
		{Guardians: ownGuardians(d.GuardianInput{Name: string([]byte{0xff}), Relationship: "Mother", ContactPhone: "0712345678"})},
		{Guardians: ownGuardians(d.GuardianInput{Name: "Guardian", Relationship: "Mother", ContactPhone: "bad"})},
	} {
		r := &editOwnRepo{}
		if _, err := s.NewOwnProfileEditor(r).Patch(context.Background(), "student", input); !errors.Is(err, d.ErrInvalid) || r.calls != 0 {
			t.Fatal(input, err, r.calls)
		}
	}
	r := &editOwnRepo{err: d.ErrNotFound}
	if _, err := s.NewOwnProfileEditor(r).Patch(context.Background(), "", dto.OwnProfilePatch{ContactPhone: ownString("0771234567")}); !errors.Is(err, d.ErrForbidden) || r.calls != 0 {
		t.Fatal(err)
	}
	if _, err := s.NewOwnProfileEditor(r).Patch(context.Background(), "student", dto.OwnProfilePatch{ContactPhone: ownString("0771234567")}); !errors.Is(err, d.ErrNotFound) {
		t.Fatal(err)
	}
}
