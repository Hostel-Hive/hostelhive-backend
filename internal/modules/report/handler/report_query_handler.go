package handler

import (
	"io"
	"net/http"
	"net/url"
	"strconv"

	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
)

// Each route declares its extra filter; status is not accepted on attendance
// routes and direction is not accepted on leave reports.
func reportQuery(w http.ResponseWriter, r *http.Request, filter string) (string, url.Values, int, int, bool) {
	a, ok := middleware.AccountFromContext(r.Context())
	if !ok {
		response.Error(w, 401, "unauthorized")
		return "", nil, 0, 0, false
	}
	if !a.IsActive || (a.Role != identity.RoleAdmin && a.Role != identity.RoleWarden) {
		response.Error(w, 403, "forbidden")
		return "", nil, 0, 0, false
	}
	fail := func() (string, url.Values, int, int, bool) {
		response.Error(w, 400, "invalid_report_query")
		return "", nil, 0, 0, false
	}
	if r.Body != nil {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1))
		if err != nil || len(raw) > 0 {
			return fail()
		}
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return fail()
	}
	limit, offset := 20, 0
	for key, vs := range values {
		if len(vs) != 1 || vs[0] == "" {
			return fail()
		}
		switch key {
		case "from", "to", "student_id":
		case "limit", "offset":
			n, e := strconv.Atoi(vs[0])
			if e != nil {
				return fail()
			}
			if key == "limit" {
				limit = n
			} else {
				offset = n
			}
		default:
			if key != filter {
				return fail()
			}
		}
	}
	return a.FirebaseUID, values, limit, offset, true
}
