package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/config"
)

func TestRunReportsBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg := config.Config{HTTPAddr: listener.Addr().String(), DatabaseURL: "postgres://test:example@127.0.0.1:5432/hostelhive?sslmode=disable", DatabaseCheckTimeout: time.Second}
	err = Run(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "listen on HTTP_ADDR") {
		t.Fatalf("expected bind error, got %v", err)
	}
}

func TestShutdownDrainsActiveRequest(t *testing.T) {
	testShutdown(t, false)
}

func TestShutdownDeadlineClosesActiveRequest(t *testing.T) {
	testShutdown(t, true)
}

func testShutdown(t *testing.T, exceedDeadline bool) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	entered := make(chan struct{})
	release := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-release:
			_, _ = io.WriteString(w, "completed")
		case <-r.Context().Done():
		}
	})}
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timeout := 2 * time.Second
	if exceedDeadline {
		timeout = 50 * time.Millisecond
	}
	done := make(chan error, 1)
	go func() { done <- serve(ctx, srv, listener, config.Config{ShutdownTimeout: timeout}) }()
	result := make(chan error, 1)
	client := &http.Client{Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	go func() {
		resp, err := client.Get("http://" + listener.Addr().String())
		if err != nil {
			result <- err
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err == nil && string(body) != "completed" {
			err = errors.New("response truncated")
		}
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request never started")
	}
	cancel()
	if !exceedDeadline {
		// An active request must keep shutdown pending until it is released.
		select {
		case err := <-done:
			t.Fatalf("shutdown returned before request finished: %v", err)
		case <-time.After(30 * time.Millisecond):
		}
		close(release)
		released = true
	}
	select {
	case err := <-done:
		if exceedDeadline && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline, got %v", err)
		}
		if !exceedDeadline && err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not finish")
	}
	select {
	case err := <-result:
		if exceedDeadline && err == nil {
			t.Fatal("expected closed request")
		}
		if !exceedDeadline && err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("client did not finish")
	}
}
