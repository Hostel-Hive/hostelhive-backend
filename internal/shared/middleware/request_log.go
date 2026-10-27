package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type requestActorKey struct{}

type requestActor struct{ kind, id string }

// NoteActor records who made the request so RequestLog can include it.
// Only internal IDs are recorded, never emails, names or credentials.
func NoteActor(ctx context.Context, kind, id string) {
	if actor, ok := ctx.Value(requestActorKey{}).(*requestActor); ok && id != "" {
		actor.kind, actor.id = kind, id
	}
}

// RequestLog writes one structured line per finished request. It logs the
// matched route pattern instead of the raw path and never the query, headers or
// body, so tokens, QR payloads and personal data stay out of the logs.
// Refused requests (401/403) are warnings and server failures are errors.
func RequestLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			actor := &requestActor{}
			r = r.WithContext(context.WithValue(r.Context(), requestActorKey{}, actor))
			recorder := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(recorder, r)
			status := recorder.status
			if status == 0 {
				status = http.StatusOK
			}
			if status < 400 && (r.URL.Path == "/health" || r.URL.Path == "/ready") {
				return
			}
			_, route, found := strings.Cut(r.Pattern, " ")
			if !found {
				route = r.Pattern
			}
			if route == "" {
				route = "unmatched"
			}
			level := slog.LevelInfo
			switch {
			case status >= 500:
				level = slog.LevelError
			case status == http.StatusUnauthorized || status == http.StatusForbidden:
				level = slog.LevelWarn
			}
			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("route", route),
				slog.Int("status", status),
				slog.Int64("duration_ms", time.Since(started).Milliseconds()),
			}
			if actor.id != "" {
				attrs = append(attrs, slog.String("actor_type", actor.kind), slog.String("actor_id", actor.id))
			}
			logger.LogAttrs(r.Context(), level, "http request", attrs...)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	if s.status == 0 {
		s.status = status
	}
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the original writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
