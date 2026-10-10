package workers

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	s "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
)

type cleanupRepository struct {
	s.ImageRepository
	cleanup func(context.Context) (bool, error)
}

func (r cleanupRepository) CleanupOne(ctx context.Context, _ s.Objects) (bool, error) {
	return r.cleanup(ctx)
}

func awaitCleanupStop(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("image cleanup worker did not stop")
	}
}

func TestImageCleanupRetriesAfterFailureOrEmptyQueue(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty_queue", true: "storage_failure"}[failure], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls atomic.Int32
			repo := cleanupRepository{cleanup: func(attempt context.Context) (bool, error) {
				if _, ok := attempt.Deadline(); !ok {
					t.Error("cleanup attempt has no deadline")
				}
				if calls.Add(1) == 1 {
					if failure {
						return false, errors.New("temporary storage failure")
					}
					return false, nil
				}
				cancel()
				return true, nil
			}}
			done := make(chan struct{})
			go func() { defer close(done); RunStudentImageCleanup(ctx, repo, nil, time.Second, 5*time.Millisecond) }()
			awaitCleanupStop(t, done)
			if calls.Load() != 2 {
				t.Fatal("failed/empty attempt prevented a later retry")
			}
		})
	}
}

func TestImageCleanupBoundsBatchAndStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	tenth := make(chan struct{})
	extra := make(chan struct{}, 1)
	repo := cleanupRepository{cleanup: func(context.Context) (bool, error) {
		n := calls.Add(1)
		if n == 10 {
			close(tenth)
		}
		if n > 10 {
			select {
			case extra <- struct{}{}:
			default:
			}
		}
		return true, nil
	}}
	done := make(chan struct{})
	go func() { defer close(done); RunStudentImageCleanup(ctx, repo, nil, time.Second, time.Hour) }()
	select {
	case <-tenth:
	case <-time.After(2 * time.Second):
		t.Fatal("first batch was not drained")
	}
	select {
	case <-extra:
		t.Fatal("worker busy-looped beyond batch limit")
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	awaitCleanupStop(t, done)
	if calls.Load() != 10 {
		t.Fatal("unexpected cleanup batch size")
	}
}

func TestImageCleanupCancelsInFlightStorageWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	repo := cleanupRepository{cleanup: func(attempt context.Context) (bool, error) {
		close(entered)
		<-attempt.Done()
		return false, attempt.Err()
	}}
	done := make(chan struct{})
	go func() { defer close(done); RunStudentImageCleanup(ctx, repo, nil, time.Minute, time.Hour) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup did not start")
	}
	cancel()
	awaitCleanupStop(t, done)
}
