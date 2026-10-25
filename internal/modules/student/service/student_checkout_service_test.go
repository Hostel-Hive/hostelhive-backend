package service

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
)

type checkoutRepoFake struct {
	calls     int
	actor, id string
	input     dto.CheckoutInput
	hash      []byte
	err       error
}

func (f *checkoutRepoFake) Checkout(_ context.Context, actor, id string, in dto.CheckoutInput, hash []byte) (d.CheckoutResult, error) {
	f.calls++
	f.actor, f.id, f.input = actor, id, in
	f.hash = append([]byte{}, hash...)
	return d.CheckoutResult{}, f.err
}
func checkoutInput() dto.CheckoutInput {
	deactivate := false
	return dto.CheckoutInput{RequestID: "11111111-1111-4111-8111-111111111111", Reason: "graduated", DeactivateAccount: &deactivate, Confirmed: true}
}
func TestCheckoutNormalizesAndHashesExplicitChoices(t *testing.T) {
	f := &checkoutRepoFake{}
	s := NewCheckouts(f)
	v := checkoutInput()
	v.Reason = " GRADUATED "
	v.Notes = " completed clearance "
	id := "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA"
	if _, err := s.Checkout(context.Background(), "admin", id, v); err != nil {
		t.Fatal(err)
	}
	first := append([]byte{}, f.hash...)
	if f.id != strings.ToLower(id) || f.input.Reason != "graduated" || f.input.Notes != "completed clearance" || len(first) != 32 || *f.input.DeactivateAccount {
		t.Fatal("normalization", f)
	}
	v.Reason = "graduated"
	v.Notes = "completed clearance"
	_, _ = s.Checkout(context.Background(), "admin", strings.ToLower(id), v)
	if !bytes.Equal(first, f.hash) {
		t.Fatal("normalized replay changed hash")
	}
	deactivate := true
	v.DeactivateAccount = &deactivate
	_, _ = s.Checkout(context.Background(), "admin", id, v)
	if bytes.Equal(first, f.hash) {
		t.Fatal("account choice not bound to replay hash")
	}
}
func TestCheckoutRejectsMissingConfirmationAndPolicyInputs(t *testing.T) {
	for _, change := range []func(*dto.CheckoutInput){
		func(v *dto.CheckoutInput) { v.RequestID = "" }, func(v *dto.CheckoutInput) { v.RequestID = "00000000-0000-0000-0000-000000000000" },
		func(v *dto.CheckoutInput) { v.DeactivateAccount = nil }, func(v *dto.CheckoutInput) { v.Confirmed = false },
		func(v *dto.CheckoutInput) { v.Reason = "going_home" }, func(v *dto.CheckoutInput) { v.Notes = strings.Repeat("x", 1001) },
		func(v *dto.CheckoutInput) { v.Notes = "bad\x00notes" },
	} {
		f := &checkoutRepoFake{}
		v := checkoutInput()
		change(&v)
		if _, err := NewCheckouts(f).Checkout(context.Background(), "admin", "22222222-2222-4222-8222-222222222222", v); !errors.Is(err, d.ErrInvalid) || f.calls != 0 {
			t.Fatal("invalid checkout reached storage", err)
		}
	}
}
