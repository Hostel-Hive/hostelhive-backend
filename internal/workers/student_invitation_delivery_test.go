package workers

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

type invitationLoopFake struct {
	calls  int
	cancel context.CancelFunc
}

func (f *invitationLoopFake) Dispatch(ctx context.Context) (bool, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("missing deadline")
	}
	f.calls++
	if f.calls == 2 {
		f.cancel()
		return false, context.Canceled
	}
	return true, nil
}
func TestInvitationWorkerDrainsAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &invitationLoopFake{cancel: cancel}
	RunStudentInvitations(ctx, f, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if f.calls != 2 {
		t.Fatal(f.calls)
	}
}
