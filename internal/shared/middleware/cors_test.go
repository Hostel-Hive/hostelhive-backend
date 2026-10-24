package middleware_test

import (
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const corsWebOrigin = "https://web-kohl-delta-90.vercel.app"

func TestCORSPreflight(t *testing.T) {
	for _, tc := range []struct {
		name, origin, method, headers string
		status                        int
	}{
		{"login", corsWebOrigin, "GET", "authorization", 204},
		{"json create", corsWebOrigin, "POST", "authorization,content-type", 204},
		{"photo", corsWebOrigin, "PUT", "Authorization, Content-Type", 204},
		{"profile edit", corsWebOrigin, "PATCH", "authorization, content-type", 204},
		{"photo removal", corsWebOrigin, "DELETE", "authorization", 204},
		{"idempotency", corsWebOrigin, "POST", "Authorization,Content-Type,Idempotency-Key", 204},
		{"cache headers", corsWebOrigin, "GET", "authorization,cache-control,pragma", 204},
		{"head", corsWebOrigin, "HEAD", "", 204},
		{"other origin", "https://evil.example", "GET", "authorization", 403},
		{"suffix spoof", corsWebOrigin + ".evil.example", "GET", "authorization", 403},
		{"preview not implicit", "https://other-preview.vercel.app", "GET", "authorization", 403},
		{"null", "null", "GET", "authorization", 403},
		{"scheme mismatch", "http://web-kohl-delta-90.vercel.app", "GET", "authorization", 403},
		{"port mismatch", corsWebOrigin + ":8443", "GET", "authorization", 403},
		{"origin path", corsWebOrigin + "/", "GET", "authorization", 403},
		{"trace", corsWebOrigin, "TRACE", "", 403},
		{"method case", corsWebOrigin, "get", "authorization", 403},
		{"arbitrary header", corsWebOrigin, "GET", "authorization,x-unknown", 403},
		{"empty header item", corsWebOrigin, "GET", "authorization,", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			handler := authentication.CORS([]string{corsWebOrigin})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(401) }))
			req := httptest.NewRequest("OPTIONS", "/api/v1/me", nil)
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Access-Control-Request-Method", tc.method)
			if tc.headers != "" {
				req.Header.Set("Access-Control-Request-Headers", tc.headers)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tc.status || calls != 0 {
				t.Fatal(w.Code, calls, w.Body)
			}
			if w.Header().Get("Access-Control-Allow-Credentials") != "" {
				t.Fatal("cookie credentials enabled")
			}
			if tc.status == 204 {
				if w.Body.Len() != 0 || w.Header().Get("Access-Control-Allow-Origin") != corsWebOrigin || !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Authorization") || !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Content-Type") || !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), tc.method) || w.Header().Get("Access-Control-Max-Age") != "600" {
					t.Fatal(w.Header(), w.Body)
				}
			} else if w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("denied preflight granted CORS")
			}
			vary := strings.Join(w.Header().Values("Vary"), ",")
			for _, name := range []string{"Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers"} {
				if !strings.Contains(vary, name) {
					t.Fatal("missing Vary", vary)
				}
			}
		})
	}
}

func TestCORSActualResponses(t *testing.T) {
	for _, status := range []int{200, 204, 400, 401, 403, 404, 405, 409, 413, 415, 429, 503} {
		calls := 0
		handler := authentication.CORS([]string{corsWebOrigin})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Retry-After", "300")
			w.WriteHeader(status)
		}))
		req := httptest.NewRequest("GET", "/api/v1/me", nil)
		req.Header.Set("Origin", corsWebOrigin)
		w := httptest.NewRecorder()
		w.Header().Set("Vary", "Accept-Encoding")
		handler.ServeHTTP(w, req)
		if calls != 1 || w.Code != status || w.Header().Get("Access-Control-Allow-Origin") != corsWebOrigin || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Retry-After") != "300" {
			t.Fatal(w.Code, w.Header())
		}
		for _, name := range []string{"Content-Disposition", "Retry-After", "WWW-Authenticate"} {
			if !strings.Contains(w.Header().Get("Access-Control-Expose-Headers"), name) {
				t.Fatal("missing exposure", w.Header())
			}
		}
		if strings.Join(w.Header().Values("Vary"), ",") != "Accept-Encoding,Origin" {
			t.Fatal("Vary overwritten", w.Header())
		}
	}
}

func TestCORSDoesNotReplaceRoutingOrAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name, origin, method string
		origins              []string
		status               int
	}{
		{"no origin", "", "GET", []string{corsWebOrigin}, 401},
		{"same-origin Swagger", "https://hostelhive-backend-dev.onrender.com", "POST", []string{corsWebOrigin}, 401},
		{"unlisted actual origin", "https://evil.example", "GET", []string{corsWebOrigin}, 401},
		{"disabled", corsWebOrigin, "GET", nil, 401},
		{"ordinary options", corsWebOrigin, "OPTIONS", []string{corsWebOrigin}, 405},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			handler := authentication.CORS(tc.origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(tc.status) }))
			req := httptest.NewRequest(tc.method, "/api/v1/me", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if calls != 1 || w.Code != tc.status {
				t.Fatal("routing bypassed")
			}
			if tc.method != "OPTIONS" && w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("unexpected CORS permission")
			}
		})
	}
	// Reject ambiguous origins and methods without reflecting an Origin value.
	for _, name := range []string{"Origin", "Access-Control-Request-Method"} {
		req := httptest.NewRequest("OPTIONS", "/api/v1/me", nil)
		req.Header.Set("Origin", corsWebOrigin)
		req.Header.Set("Access-Control-Request-Method", "GET")
		req.Header.Add(name, req.Header.Get(name))
		w := httptest.NewRecorder()
		authentication.CORS([]string{corsWebOrigin})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("preflight reached authentication") })).ServeHTTP(w, req)
		if w.Code != 403 || w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("ambiguous preflight accepted")
		}
	}
}
