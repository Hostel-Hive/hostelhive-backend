package middleware

import (
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
	"net/http"
	"strings"
)

const corsMethods = "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS"
const corsHeaders = "Authorization, Content-Type, Idempotency-Key, Accept, Cache-Control, Pragma"

// CORS must wrap routing and authentication, so unauthenticated browser
// preflights can finish and actual error responses retain the CORS headers.
// It grants browser read access only; ordinary API authorization is unchanged.
func CORS(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		if origin != "" && origin != "*" && origin != "null" {
			allowed[origin] = true
		}
	}
	methods := map[string]bool{}
	for _, method := range strings.Split(corsMethods, ", ") {
		methods[method] = true
	}
	headers := map[string]bool{}
	for _, header := range strings.Split(corsHeaders, ", ") {
		headers[strings.ToLower(header)] = true
	}
	return func(next http.Handler) http.Handler {
		if len(allowed) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			addCORSVary(w.Header(), "Origin")
			if r.Method == http.MethodOptions {
				addCORSVary(w.Header(), "Access-Control-Request-Method")
				addCORSVary(w.Header(), "Access-Control-Request-Headers")
			}
			origin := r.Header.Get("Origin")
			preflight := r.Method == http.MethodOptions && len(r.Header.Values("Access-Control-Request-Method")) > 0 && origin != ""
			permitted := len(r.Header.Values("Origin")) == 1 && allowed[origin]
			if !permitted {
				if preflight {
					response.Error(w, http.StatusForbidden, "cors_origin_denied")
					return
				}
				// Preserve same-origin Swagger, devices, CLI clients and regular routing.
				// An unlisted browser origin receives no permission to read this response.
				next.ServeHTTP(w, r)
				return
			}
			if preflight {
				requested := r.Header.Values("Access-Control-Request-Method")
				if len(requested) != 1 || !methods[requested[0]] {
					response.Error(w, http.StatusForbidden, "cors_method_denied")
					return
				}
				for _, line := range r.Header.Values("Access-Control-Request-Headers") {
					for _, header := range strings.Split(line, ",") {
						if !headers[strings.ToLower(strings.TrimSpace(header))] {
							response.Error(w, http.StatusForbidden, "cors_headers_denied")
							return
						}
					}
				}
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", corsMethods)
				w.Header().Set("Access-Control-Allow-Headers", corsHeaders)
				w.Header().Set("Access-Control-Max-Age", "600")
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition, Retry-After, WWW-Authenticate")
			next.ServeHTTP(w, r)
		})
	}
}

func addCORSVary(header http.Header, value string) {
	for _, line := range header.Values("Vary") {
		for _, existing := range strings.Split(line, ",") {
			if existing = strings.TrimSpace(existing); existing == "*" || strings.EqualFold(existing, value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}
