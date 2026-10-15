package report_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"

	"strings"
	"testing"
	"time"

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
)

type verifier struct{}

func (verifier) Verify(_ context.Context, token string) (string, error) { return token, nil }

func TestAttendanceHistoryReports(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL migrated to23")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := "r78-" + uuid.NewString()[:10] + "-"
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
		sid := scalar(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,$3,'Science',1,'0771234567') RETURNING student_id::text`, uid, prefix+name, "Report "+name)
		return sid, uid
	}
	s1, u1 := student("student1")
	s2, u2 := student("student2")
	dev := scalar(`INSERT INTO hostelhive.scanner_devices(device_id,name,credential_hash,created_by,updated_by) VALUES(gen_random_uuid(),$1,decode(repeat('ab',32),'hex'),$2,$2) RETURNING device_id::text`, prefix+"device", admin)
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		if _, e := pool.Exec(clean, `UPDATE hostelhive.late_access_requests SET consumed_at=NULL,consumed_device_id=NULL,consumed_event_id=NULL WHERE consumed_device_id=$1`, dev); e != nil {
			t.Error(e)
		}
		for _, sql := range []string{
			`DELETE FROM hostelhive.attendance_events WHERE device_id=$1::uuid`,
			`DELETE FROM hostelhive.scanner_devices WHERE device_id=$1::uuid`,
		} {
			if _, e := pool.Exec(clean, sql, dev); e != nil {
				t.Error(e)
			}
		}
		if _, e := pool.Exec(clean, `DELETE FROM hostelhive.late_access_requests WHERE student_id IN($1,$2)`, s1, s2); e != nil {
			t.Error(e)
		}
		if _, e := pool.Exec(clean, `DELETE FROM hostelhive.students WHERE student_id IN($1,$2)`, s1, s2); e != nil {
			t.Error(e)
		}
		if _, e := pool.Exec(clean, `DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`, prefix+"%"); e != nil {
			t.Error(e)
		}
	}()
	event := func(sid, at, dir, outcome string, result any) string {
		return scalar(`INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at,access_result)
 VALUES(gen_random_uuid(),$1,$2,decode(repeat('cd',32),'hex'),$3,$4,$5,$6) RETURNING event_id::text`, dev, sid, dir, outcome, at, result)
	}
	// Selected day is [18:30 UTC previous day,18:30 UTC current day) in Colombo.
	event(s1, "2040-03-09T18:29:59Z", "IN", "recorded", nil)
	legacy := event(s1, "2040-03-09T18:30:00Z", "IN", "recorded", nil)
	approvedExit := event(s1, "2040-03-10T08:00:00Z", "OUT", "recorded", "not_required")
	permission := scalar(`INSERT INTO hostelhive.late_access_requests(request_id,student_id,submitted_by,direction,expected_at,reason)
 VALUES(gen_random_uuid(),$1,$2,'OUT','2040-03-10T08:00:00Z','Report fixture') RETURNING late_access_id::text`, s1, u1)
	exec(`UPDATE hostelhive.late_access_requests SET status='approved',decided_at=clock_timestamp(),decided_by=$2 WHERE late_access_id=$1`, permission, admin)
	exec(`UPDATE hostelhive.attendance_events SET access_result='approved',late_access_id=$3 WHERE device_id=$1 AND event_id=$2`, dev, approvedExit, permission)
	exec(`UPDATE hostelhive.late_access_requests SET consumed_at='2040-03-10T08:00:00Z',consumed_device_id=$2,consumed_event_id=$3 WHERE late_access_id=$1`, permission, dev, approvedExit)
	late := event(s2, "2040-03-10T18:29:59.999999Z", "IN", "recorded", "unapproved")
	incident := scalar(`INSERT INTO hostelhive.attendance_incidents(device_id,event_id,student_id,direction,incident_type,occurred_at,hostel_timezone,exit_opening_minute,entry_deadline_minute,permission_window_seconds)
 VALUES($1,$2,$3,'IN','late_entry','2040-03-10T18:29:59.999999Z','Asia/Colombo',360,1200,1800) RETURNING incident_id::text`, dev, late, s2)
	// Remove only this fixture's queued incident alert before user cleanup.
	defer func() {
		if _, e := pool.Exec(context.Background(), `DELETE FROM hostelhive.notification_events WHERE attendance_incident_id=$1`, incident); e != nil {
			t.Error(e)
		}
	}()
	event(s1, "2040-03-10T18:30:00Z", "OUT", "recorded", "not_required")
	event(s1, "2040-03-10T09:00:00Z", "IN", "cooldown", "not_evaluated")
	event(s2, "2040-03-10T10:00:00Z", "IN", "already_in", "not_evaluated")
	event(s1, "2040-03-10T11:00:00Z", "OUT", "already_out", "not_evaluated")
	// Reports retain history despite current account/profile state.
	exec(`UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, s1, admin)
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id IN($1,$2)`, u1, u2)
	location, e := time.LoadLocation("Asia/Colombo")
	if e != nil {
		t.Fatal(e)
	}
	store := repo.NewAttendance(pool)
	service := svc.NewAttendance(store, location)
	mux := http.NewServeMux()
	report.RegisterAttendance(h.NewAttendance(service, 3*time.Second), mux, m.Middleware(verifier{}, identityrepo.NewPostgresAccounts(pool), time.Second))
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
		return w
	}
	// Scope every successful query to fixture students except an isolated far-future range.
	base := "/api/v1/reports/attendance"
	q := "?from=2040-03-10&to=2040-03-10"
	var all d.MovementPage
	w := call(base+q, "admin", "", 200)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("cache header")
	}
	if err := json.Unmarshal(w.Body.Bytes(), &all); err != nil {
		t.Fatal(err)
	}
	if all.Total != 3 || all.Summary != (d.Totals{Entries: 2, Exits: 1, DistinctStudents: 2}) || len(all.Items) != 3 || all.Timezone != "Asia/Colombo" {
		t.Fatal(all)
	}
	if all.Items[0].EventID != late || all.Items[2].EventID != legacy || all.Items[2].AccessResult != nil ||
		all.Items[0].IncidentID == nil || *all.Items[0].IncidentID != incident {
		t.Fatal(all.Items)
	}
	if all.Items[1].LateAccessID == nil || *all.Items[1].LateAccessID != permission || all.Items[1].AccessResult == nil || *all.Items[1].AccessResult != "approved" {
		t.Fatal("approved movement metadata missing", all.Items[1])
	}
	if strings.Contains(w.Body.String(), `"access_result":null`) || strings.Contains(w.Body.String(), "payload_hash") {
		t.Fatal("legacy or secret exposed")
	}
	for _, role := range []string{"admin", "warden"} {
		for _, route := range []string{base, base + "/daily"} {
			call(route+q, role, "", 200)
		}
	}
	var page d.MovementPage
	json.Unmarshal(call(base+q+"&limit=1&offset=1", "admin", "", 200).Body.Bytes(), &page)
	if page.Total != 3 || len(page.Items) != 1 || !page.HasMore || page.Items[0].Direction != "OUT" || page.Summary != all.Summary {
		t.Fatal(page)
	}
	json.Unmarshal(call(base+q+"&offset=999", "admin", "", 200).Body.Bytes(), &page)
	if page.Total != 3 || len(page.Items) != 0 || page.HasMore || page.Summary != all.Summary {
		t.Fatal(page)
	}
	json.Unmarshal(call(base+q+"&student_id="+s1, "admin", "", 200).Body.Bytes(), &page)
	if page.Total != 2 || page.Summary.DistinctStudents != 1 {
		t.Fatal(page)
	}
	json.Unmarshal(call(base+q+"&direction=IN", "admin", "", 200).Body.Bytes(), &page)
	if page.Total != 2 || page.Summary.Exits != 0 {
		t.Fatal(page)
	}
	var daily d.DailyPage
	dq := "?from=2040-03-10&to=2040-03-11&student_id=" + s1
	json.Unmarshal(call(base+"/daily"+dq, "warden", "", 200).Body.Bytes(), &daily)
	if daily.Total != 2 || len(daily.Items) != 2 || daily.Summary != (d.Totals{Entries: 1, Exits: 2, DistinctStudents: 1}) ||
		daily.Items[0].Date != "2040-03-11" || daily.Items[0].Exits != 1 || daily.Items[1].Entries != 1 {
		t.Fatal(daily)
	}
	json.Unmarshal(call(base+"/daily?from=2040-03-10&to=2040-03-12&student_id="+s2+"&limit=1", "admin", "", 200).Body.Bytes(), &daily)
	if daily.Total != 3 || !daily.HasMore || len(daily.Items) != 1 || daily.Items[0].Date != "2040-03-12" || daily.Items[0].Entries != 0 {
		t.Fatal(daily)
	}
	json.Unmarshal(call(base+"/daily"+q+"&direction=OUT&offset=999", "admin", "", 200).Body.Bytes(), &daily)
	if daily.Total != 1 || daily.HasMore || len(daily.Items) != 0 || daily.Summary.Exits != 1 || daily.Summary.Entries != 0 {
		t.Fatal(daily)
	}
	unknown := uuid.NewString()
	json.Unmarshal(call(base+q+"&student_id="+unknown, "admin", "", 200).Body.Bytes(), &page)
	if page.Total != 0 || len(page.Items) != 0 {
		t.Fatal(page)
	}
	json.Unmarshal(call(base+"/daily"+q+"&student_id="+unknown, "admin", "", 200).Body.Bytes(), &daily)
	if daily.Total != 1 || len(daily.Items) != 1 || daily.Items[0].DistinctStudents != 0 {
		t.Fatal(daily)
	}
	// Tie-breaking is stable when movements share the exact timestamp.
	tie1 := event(s1, "2042-05-01T10:00:00Z", "IN", "recorded", nil)
	tie2 := event(s1, "2042-05-01T10:00:00Z", "OUT", "recorded", nil)
	first, second := tie1, tie2
	if first > second {
		first, second = second, first
	}
	var tied d.MovementPage
	json.Unmarshal(call(base+"?from=2042-05-01&to=2042-05-01&student_id="+s1, "admin", "", 200).Body.Bytes(), &tied)
	if tied.Total != 2 || len(tied.Items) != 2 || tied.Items[0].EventID != first || tied.Items[1].EventID != second {
		t.Fatal("unstable tie order", tied)
	}
	// Exact repeats preserve counts and do not write to attendance or presence.
	var before, after string
	beforePresence := scalar(`SELECT count(*)::text FROM hostelhive.student_presence WHERE student_id IN($1,$2)`, s1, s2)
	beforeConsumed := scalar(`SELECT consumed_event_id::text FROM hostelhive.late_access_requests WHERE late_access_id=$1`, permission)
	before = scalar(`SELECT count(*)::text FROM hostelhive.attendance_events WHERE device_id=$1`, dev)
	for i := 0; i < 3; i++ {
		call(base+q, "admin", "", 200)
		call(base+"/daily"+q, "warden", "", 200)
	}
	after = scalar(`SELECT count(*)::text FROM hostelhive.attendance_events WHERE device_id=$1`, dev)
	afterPresence := scalar(`SELECT count(*)::text FROM hostelhive.student_presence WHERE student_id IN($1,$2)`, s1, s2)
	afterConsumed := scalar(`SELECT consumed_event_id::text FROM hostelhive.late_access_requests WHERE late_access_id=$1`, permission)
	if beforePresence != afterPresence || beforeConsumed != afterConsumed {
		t.Fatal("report mutated presence or permission use")
	}
	if before != after {
		t.Fatal("report mutated events")
	}
	for _, role := range []string{"sub", "security"} {
		for _, route := range []string{base, base + "/daily"} {
			call(route+q, role, "", 403)
		}
	}
	// Inactive Student authentication is 403; use an active Student for explicit route denial too.
	exec(`UPDATE hostelhive.users SET is_active=true WHERE user_id=$1`, u2)
	for _, route := range []string{base, base + "/daily"} {
		call(route+q, "student2", "", 403)
		call(route+q, "inactive", "", 403)
		call(route+q, "", "", 401)
		for _, bad := range []string{"", "?from=2040-03-10", "?from=bad&to=2040-03-10", "?from=2040-03-11&to=2040-03-10", "?from=2040-01-01&to=2041-01-02", q + "&direction=bad", q + "&student_id=bad", q + "&limit=0", q + "&limit=101", q + "&offset=-1", q + "&offset=1000001", q + "&limit=x", q + "&limit=", q + "&limit=1&limit=2", q + "&unknown=1", q + "&from=2040-03-10", q + "&bad=%ZZ"} {
			call(route+bad, "admin", "", 400)
		}
		call(route+q, "admin", "{}", 400)
	}
	// Repository rechecks current staff role, rather than trusting an earlier handler check.
	f := dto.AttendanceFilter{AttendanceQuery: dto.AttendanceQuery{From: "2040-03-10", To: "2040-03-10", Limit: 20}, Start: time.Date(2040, 3, 9, 18, 30, 0, 0, time.UTC), End: time.Date(2040, 3, 10, 18, 30, 0, 0, time.UTC), Timezone: "Asia/Colombo", Days: 1}
	exec(`UPDATE hostelhive.users SET role='student' WHERE user_id=$1`, admin)
	if _, e := store.Movements(ctx, prefix+"admin", f); !errors.Is(e, d.ErrForbidden) {
		t.Fatal(e)
	}
	if _, e := store.Daily(ctx, prefix+"admin", f); !errors.Is(e, d.ErrForbidden) {
		t.Fatal(e)
	}
	exec(`UPDATE hostelhive.users SET role='admin' WHERE user_id=$1`, admin)
	// A second reader remains identical after the role roundtrip.
	again, e := service.Movements(ctx, prefix+"admin", dto.AttendanceQuery{From: "2040-03-10", To: "2040-03-10", Limit: 20})
	againJSON, _ := json.Marshal(again)
	allJSON, _ := json.Marshal(all)
	if e != nil || string(againJSON) != string(allJSON) {
		t.Fatal(again, e)
	}
	closed, e := pgxpool.New(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	closed.Close()
	dead := repo.NewAttendance(closed)
	if _, e = dead.Movements(ctx, prefix+"admin", f); !errors.Is(e, d.ErrUnavailable) {
		t.Fatal(e)
	}
	if _, e = dead.Daily(ctx, prefix+"admin", f); !errors.Is(e, d.ErrUnavailable) {
		t.Fatal(e)
	}
}
