package workers

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

type pushLoopFake struct {
	cancel  context.CancelFunc
	calls   int
	entered chan struct{}
}

func (f *pushLoopFake) Dispatch(ctx context.Context) (int, error) {
	f.calls++
	if f.entered != nil {
		close(f.entered)
		return 0, nil
	}
	if f.calls == 2 {
		f.cancel()
		return 0, context.Canceled
	}
	return 1, nil
}
func TestPushWorkerDrainsAndCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake := &pushLoopFake{cancel: cancel}
	RunPushDelivery(ctx, fake, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if fake.calls != 2 {
		t.Fatal(fake.calls)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	fake = &pushLoopFake{entered: entered}
	done := make(chan struct{})
	go func() { defer close(done); RunPushDelivery(ctx, fake, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not attempt")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not cancel idle poll")
	}
}
