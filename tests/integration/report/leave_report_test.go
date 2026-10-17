package report_test

import (
	"context"
	"encoding/json"
	"errors"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/handler"
	repo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/repository"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/service"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLeaveRequestReports(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL migrated to24")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := "r81-" + uuid.NewString()[:10] + "-"
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	scalar := func(sql string, args ...any) string {
		t.Helper()
		var v string
		if err := pool.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	user := func(name, role string) string {
		return scalar(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text`, prefix+name, prefix+name+"@example.invalid", role)
	}
	admin := user("admin", "admin")
	user("warden", "warden")
	user("sub", "sub_warden")
	user("security", "security_staff")
	inactive := user("inactive", "admin")
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, inactive)
	student := func(name string) (string, string) {
		uid := user(name, "student")
		sid := scalar(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,$3,'Science',1,'0771234567') RETURNING student_id::text`, uid, prefix+name, "Leave report "+name)
		return sid, uid
	}
	s1, u1 := student("student1")
	s2, u2 := student("student2")
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		for _, sql := range []string{`DELETE FROM hostelhive.leave_requests WHERE student_id IN($1,$2)`, `DELETE FROM hostelhive.students WHERE student_id IN($1,$2)`} {
			if _, e := pool.Exec(clean, sql, s1, s2); e != nil {
				t.Error(e)
			}
		}
		if _, e := pool.Exec(clean, `DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`, prefix+"%"); e != nil {
			t.Error(e)
		}
	}()
	leave := func(sid, uid, created, status string) string {
		// Requested leave and decision dates deliberately fall OUTSIDE submission dates.
		id := scalar(`INSERT INTO hostelhive.leave_requests(request_id,student_id,submitted_by,starts_at,ends_at,reason,created_at) VALUES(gen_random_uuid(),$1,$2,'2041-12-20T00:00:00Z','2041-12-21T00:00:00Z','Report fixture family visit',$3) RETURNING leave_id::text`, sid, uid, created)
		if status != "pending" {
			exec(`UPDATE hostelhive.leave_requests SET status=$2,decided_at='2041-11-01T12:00:00Z',decided_by=$3,decision_note=$4 WHERE leave_id=$1`, id, status, admin, "Report fixture "+status)
		}
		return id
	}
	leave(s1, u1, "2041-03-09T18:29:59Z", "pending")
	pending := leave(s1, u1, "2041-03-09T18:30:00Z", "pending")
	approved := leave(s1, u1, "2041-03-10T08:00:00Z", "approved")
	rejected := leave(s2, u2, "2041-03-10T18:29:59.999999Z", "rejected")
	leave(s1, u1, "2041-03-10T18:30:00Z", "pending")
	// A leave happening on the selected day, but submitted earlier, must be excluded.
	exec(`UPDATE hostelhive.leave_requests SET starts_at='2041-03-10T01:00:00Z',ends_at='2041-03-10T02:00:00Z' WHERE student_id=$1 AND created_at='2041-03-09T18:29:59Z'`, s1)
	exec(`UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, s1, admin)
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id IN($1,$2)`, u1, u2)
	location, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		t.Fatal(err)
	}
	store := repo.NewLeaves(pool)
	service := svc.NewLeaves(store, location)
	mux := http.NewServeMux()
	auth := m.Middleware(verifier{}, identityrepo.NewPostgresAccounts(pool), time.Second)
	report.RegisterLeaves(h.NewLeaves(service, 3*time.Second), mux, auth)
	report.RegisterAttendance(h.NewAttendance(svc.NewAttendance(repo.NewAttendance(pool), location), 3*time.Second), mux, auth)
	call := func(path, token, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+prefix+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s as %s: %d %s", path, token, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("missing no-store")
		}
		return w
	}
	base := "/api/v1/reports/leaves"
	q := "?from=2041-03-10&to=2041-03-10"
	page := func(query, role string) d.LeavePage {
		t.Helper()
		var p d.LeavePage
		if err := json.Unmarshal(call(base+query, role, "", 200).Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	all := page(q, "admin")
	if all.Total != 3 || all.Summary != (d.LeaveTotals{Pending: 1, Approved: 1, Rejected: 1}) || len(all.Items) != 3 || all.Timezone != "Asia/Colombo" || all.From != "2041-03-10" || all.To != "2041-03-10" || all.Limit != 20 || all.Offset != 0 || all.HasMore {
		t.Fatal(all)
	}
	if all.Items[0].LeaveID != rejected || all.Items[1].LeaveID != approved || all.Items[2].LeaveID != pending {
		t.Fatal("order or midnight boundary", all.Items)
	}
	for _, item := range all.Items {
		if item.Reason != "Report fixture family visit" || item.RequestID == "" || item.SubmittedBy == "" {
			t.Fatal(item)
		}
		if item.Status == "pending" {
			if item.DecidedAt != nil || item.DecidedBy != nil || item.DecisionNote != "" {
				t.Fatal("pending decision", item)
			}
		} else {
			if item.DecidedAt == nil || item.DecidedBy == nil || *item.DecidedBy != admin || item.DecisionNote != "Report fixture "+item.Status || item.DecidedAt.Month() != time.November {
				t.Fatal("decision details", item)
			}
		}
	}
	raw := call(base+q, "admin", "", 200).Body.String()
	if !strings.Contains(raw, `"decided_at":null`) || !strings.Contains(raw, `"decided_by":null`) || strings.Contains(raw, "firebase_uid") || strings.Contains(raw, "@example.invalid") {
		t.Fatal(raw)
	}
	ward := page(q, "warden")
	if ward.Total != all.Total || ward.Summary != all.Summary {
		t.Fatal(ward)
	}
	p := page(q+"&limit=1&offset=1", "admin")
	if p.Total != 3 || p.Summary != all.Summary || len(p.Items) != 1 || p.Items[0].LeaveID != approved || !p.HasMore {
		t.Fatal(p)
	}
	p = page(q+"&offset=999", "admin")
	if p.Total != 3 || p.Summary != all.Summary || len(p.Items) != 0 || p.Items == nil || p.HasMore {
		t.Fatal(p)
	}
	for _, status := range []string{"pending", "approved", "rejected"} {
		p = page(q+"&status="+status, "admin")
		if p.Total != 1 || len(p.Items) != 1 || p.Items[0].Status != status || p.Status != status || p.Total != p.Summary.Pending+p.Summary.Approved+p.Summary.Rejected {
			t.Fatal(p)
		}
	}
	p = page(q+"&student_id="+s1, "admin")
	if p.Total != 2 || p.Summary != (d.LeaveTotals{Pending: 1, Approved: 1}) || p.StudentID != s1 {
		t.Fatal(p)
	}
	p = page(q+"&student_id="+s1+"&status=rejected", "admin")
	if p.Total != 0 || p.Summary != (d.LeaveTotals{}) || len(p.Items) != 0 || p.Items == nil {
		t.Fatal(p)
	}
	p = page(q+"&student_id="+uuid.NewString(), "admin")
	if p.Total != 0 || p.Summary != (d.LeaveTotals{}) || p.Items == nil {
		t.Fatal(p)
	}
	// Stable ordering also holds across separately requested pages at identical timestamps.
	tie1 := leave(s1, u1, "2043-05-01T10:00:00Z", "pending")
	tie2 := leave(s1, u1, "2043-05-01T10:00:00Z", "pending")
	first, second := tie1, tie2
	if first < second {
		first, second = second, first
	}
	tq := "?from=2043-05-01&to=2043-05-01&student_id=" + s1 + "&limit=1"
	p = page(tq, "admin")
	next := page(tq+"&offset=1", "admin")
	if p.Total != 2 || !p.HasMore || p.Items[0].LeaveID != first || next.Items[0].LeaveID != second || next.HasMore {
		t.Fatal(p, next)
	}
	// Snapshot source state and queued delivery intent after setup: reads must not mutate it.
	state := func() string {
		return scalar(`SELECT jsonb_build_object(
 'leaves',(SELECT jsonb_agg(to_jsonb(l) ORDER BY leave_id) FROM hostelhive.leave_requests l WHERE student_id IN($1,$2)),
 'presence',(SELECT jsonb_agg(to_jsonb(p) ORDER BY student_id) FROM hostelhive.student_presence p WHERE student_id IN($1,$2)),
 'attendance',(SELECT count(*) FROM hostelhive.attendance_events WHERE student_id IN($1,$2)),
 'incidents',(SELECT count(*) FROM hostelhive.attendance_incidents WHERE student_id IN($1,$2)),
 'alerts',(SELECT jsonb_agg(to_jsonb(e) ORDER BY event_id) FROM hostelhive.notification_events e WHERE leave_id IN(SELECT leave_id FROM hostelhive.leave_requests WHERE student_id IN($1,$2))),
 'outbox',(SELECT jsonb_agg(to_jsonb(o) ORDER BY event_id,recipient_id) FROM hostelhive.notification_outbox o WHERE event_id IN(SELECT event_id FROM hostelhive.notification_events WHERE leave_id IN(SELECT leave_id FROM hostelhive.leave_requests WHERE student_id IN($1,$2))))
 )::text`, s1, s2)
	}
	before := state()
	for i := 0; i < 3; i++ {
		page(q, "admin")
		page(q+"&status=approved", "warden")
	}
	if state() != before {
		t.Fatal("report mutated source state or notification intent")
	}
	exec(`UPDATE hostelhive.users SET is_active=true WHERE user_id=$1`, u2)
	for _, role := range []string{"student2", "sub", "security", "inactive"} {
		call(base+q, role, "", 403)
	}
	call(base+q, "", "", 401)
	for _, bad := range []string{"", "?from=2041-03-10", "?from=bad&to=2041-03-10", "?from=2041-03-11&to=2041-03-10", "?from=2041-01-01&to=2042-01-02", q + "&status=Approved", q + "&direction=IN", q + "&student_id=bad", q + "&limit=0", q + "&limit=101", q + "&offset=-1", q + "&offset=1000001", q + "&limit=x", q + "&offset=x", q + "&status=", q + "&status=pending&status=approved", q + "&unknown=1", q + "&bad=%ZZ"} {
		call(base+bad, "admin", "", 400)
	}
	call(base+q, "admin", "{}", 400)
	call("/api/v1/reports/attendance"+q+"&status=pending", "admin", "", 400)
	f := dto.LeaveFilter{LeaveQuery: dto.LeaveQuery{From: "2041-03-10", To: "2041-03-10", Limit: 20}, Start: time.Date(2041, 3, 9, 18, 30, 0, 0, time.UTC), End: time.Date(2041, 3, 10, 18, 30, 0, 0, time.UTC), Timezone: "Asia/Colombo"}
	exec(`UPDATE hostelhive.users SET role='student' WHERE user_id=$1`, admin)
	if _, err := store.Requests(ctx, prefix+"admin", f); !errors.Is(err, d.ErrForbidden) {
		t.Fatal(err)
	}
	exec(`UPDATE hostelhive.users SET role='admin' WHERE user_id=$1`, admin)
	closed, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	if _, err := repo.NewLeaves(closed).Requests(ctx, prefix+"admin", f); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal(err)
	}
}
