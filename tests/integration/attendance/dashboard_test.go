package attendance_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/handler"
	repo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/repository"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/service"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAttendanceDashboardPostgres(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("requires disposable PostgreSQL migrated to version 13")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "dashboard57-" + uuid.NewString()
	cfg.ConnConfig.RuntimeParams["application_name"] = prefix
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	ids, uids, students := []string{}, []string{}, []string{}
	defer func() {
		clean, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		for _, sql := range []string{
			"DELETE FROM hostelhive.student_presence WHERE student_id=ANY($1::uuid[])",
			"DELETE FROM hostelhive.attendance_events WHERE student_id=ANY($1::uuid[])",
			"DELETE FROM hostelhive.students WHERE student_id=ANY($1::uuid[])",
		} {
			if _, err := pool.Exec(clean, sql, students); err != nil {
				t.Error("student cleanup", err)
			}
		}
		for _, sql := range []string{
			"DELETE FROM hostelhive.scanner_device_audit WHERE actor_id=ANY($1::uuid[])",
			"DELETE FROM hostelhive.scanner_devices WHERE created_by=ANY($1::uuid[])",
			"DELETE FROM hostelhive.users WHERE user_id=ANY($1::uuid[])",
		} {
			if _, err := pool.Exec(clean, sql, ids); err != nil {
				t.Error("account cleanup", err)
			}
		}
	}()
	for i, role := range []string{"admin", "warden", "security_staff", "sub_warden", "student", "student", "student", "student", "student", "student", "student"} {
		uid := fmt.Sprintf("%s-%d", prefix, i)
		var id string
		err := pool.QueryRow(ctx, "INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text", uid, uid+"@example.invalid", role).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		uids = append(uids, uid)
	}
	store := repo.NewDevices(pool)
	dashboard := svc.NewDashboard(store)
	device, err := svc.NewDevices(store).Register(ctx, uids[0], dto.RegisterDevice{Name: prefix})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	attendance.RegisterDashboard(h.NewDashboardAPI(dashboard, 5*time.Second), mux, m.Middleware(verifier{}, identityrepo.NewPostgresAccounts(pool), time.Second))
	do := func(path, token string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", path, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s HTTP %d want %d: %s", path, w.Code, status, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cacheable attendance")
		}
		return w
	}
	summary := func(query, token string) d.PresenceSummary {
		t.Helper()
		w := do("/api/v1/attendance/summary?q="+url.QueryEscape(query), token, 200)
		var v d.PresenceSummary
		if json.Unmarshal(w.Body.Bytes(), &v) != nil || v.Total != v.In+v.Out+v.Unknown {
			t.Fatal("count invariant", w.Body.String())
		}
		return v
	}
	baseline := summary("", uids[0])
	names := []string{"Alpha", "Bravo", "Charlie", "Inactive", "Archived", "OtherRole", "Percent%_!\\"}
	for i, name := range names {
		id := uuid.NewString()
		students = append(students, id)
		exec("INSERT INTO hostelhive.students(student_id,user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,$3,$4,'Test',1,'0771234567')", id, ids[i+4], fmt.Sprintf("%s-ID-%d", prefix, i), prefix+" "+name)
		if i > 0 && i < 6 {
			status := "IN"
			if i == 2 || i == 4 {
				status = "OUT"
			}
			eventID := uuid.NewString()
			hash := sha256.Sum256([]byte(eventID))
			exec("INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at) VALUES($1,$2,$3,$4,$5,'recorded',clock_timestamp())", eventID, device.Device.DeviceID, id, hash[:], status)
			exec("INSERT INTO hostelhive.student_presence(student_id,status,updated_at,device_id,event_id) VALUES($1,$2,clock_timestamp(),$3,$4)", id, status, device.Device.DeviceID, eventID)
		}
	}
	exec("UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", ids[7])
	exec("UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1", students[4], ids[0])
	exec("UPDATE hostelhive.users SET role='warden' WHERE user_id=$1", ids[9])
	expected := d.PresenceSummary{Total: 4, In: 1, Out: 1, Unknown: 2}
	if got := summary(prefix, uids[0]); got != expected {
		t.Fatal("eligibility counts", got)
	}
	if got := summary(prefix, uids[1]); got != expected {
		t.Fatal("warden counts", got)
	}
	all := summary("", uids[0])
	if all.Total != baseline.Total+4 || all.In != baseline.In+1 || all.Out != baseline.Out+1 || all.Unknown != baseline.Unknown+2 {
		t.Fatal("global counts", all, baseline)
	}
	list := func(suffix string) d.PresencePage {
		t.Helper()
		w := do("/api/v1/attendance/students?q="+url.QueryEscape(prefix)+suffix, uids[0], 200)
		var v d.PresencePage
		if json.Unmarshal(w.Body.Bytes(), &v) != nil {
			t.Fatal("page decode")
		}
		return v
	}
	page := list("&limit=1")
	if page.Total != 4 || !page.HasMore || len(page.Items) != 1 || page.Items[0].StudentID != students[0] || page.Items[0].Status != "UNKNOWN" || page.Items[0].UpdatedAt != nil {
		t.Fatal("initial page", page)
	}
	second := list("&limit=1&offset=1")
	if second.Items[0].StudentID != students[1] || second.Items[0].Status != "IN" || second.Items[0].UpdatedAt == nil {
		t.Fatal("page ordering", second)
	}
	for status, total := range map[string]int64{"IN": 1, "OUT": 1, "UNKNOWN": 2} {
		p := list("&status=" + status)
		if p.Total != total || int64(len(p.Items)) != total {
			t.Fatal("status filter", status, p)
		}
		for _, item := range p.Items {
			if item.Status != status {
				t.Fatal("wrong status")
			}
		}
	}
	empty := list("&offset=100")
	if empty.Total != 4 || empty.HasMore || empty.Items == nil || len(empty.Items) != 0 {
		t.Fatal("empty offset lost total", empty)
	}
	if got := summary("no-match-"+prefix, uids[0]); got != (d.PresenceSummary{}) {
		t.Fatal("empty summary", got)
	}
	w := do("/api/v1/attendance/students?q="+url.QueryEscape("no-match-"+prefix), uids[0], 200)
	if !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatal("empty page not array")
	}
	for _, query := range []string{prefix + " bravo", strings.ToUpper(prefix) + "-id-1", "  " + prefix + " Bravo  "} {
		if got := summary(query, uids[0]); got.Total != 1 || got.In != 1 {
			t.Fatal("name/index/trim search", got)
		}
	}
	// Wildcards and escape characters are literal search text, not SQL patterns.
	if got := summary("%_!\\", uids[0]); got.Total != 1 || got.Unknown != 1 {
		t.Fatal("literal wildcard search", got)
	}
	// A later ignored OUT scan cannot overwrite the last accepted IN state.
	ignoredID := uuid.NewString()
	hash := sha256.Sum256([]byte(ignoredID))
	exec("INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at) VALUES($1,$2,$3,$4,'OUT','cooldown',clock_timestamp())", ignoredID, device.Device.DeviceID, students[1], hash[:])
	if summary(prefix, uids[0]) != expected {
		t.Fatal("ignored event changed dashboard")
	}
	// A presence change between count and row queries cannot split a page snapshot.
	tracedConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	tracedConfig.ConnConfig.Tracer = dashboardTrace{beforeRows: func() { exec("UPDATE hostelhive.student_presence SET status='OUT' WHERE student_id=$1", students[1]) }}
	tracedPool, err := pgxpool.NewWithConfig(ctx, tracedConfig)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := svc.NewDashboard(repo.NewDevices(tracedPool)).PresenceStudents(ctx, uids[0], dto.PresenceFilter{Query: prefix, Status: "IN", Limit: 20})
	tracedPool.Close()
	if err != nil || snapshot.Total != 1 || len(snapshot.Items) != 1 || snapshot.Items[0].Status != "IN" {
		t.Fatal("page count and rows used different snapshots", snapshot, err)
	}
	exec("UPDATE hostelhive.student_presence SET status='IN' WHERE student_id=$1", students[1])
	// Dashboard excludes stale presence immediately when eligibility changes.
	exec("UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", ids[5])
	if got := summary(prefix, uids[0]); got.Total != 3 || got.In != 0 {
		t.Fatal("deactivation still counted", got)
	}
	exec("UPDATE hostelhive.users SET is_active=true WHERE user_id=$1", ids[5])
	// Existing history is retained; eligibility filtering never deletes attendance.
	var history int
	if pool.QueryRow(ctx, "SELECT count(*) FROM hostelhive.attendance_events WHERE student_id=$1", students[1]).Scan(&history) != nil || history != 2 {
		t.Fatal("history lost")
	}
	for _, token := range []string{uids[2], uids[3], uids[4]} {
		do("/api/v1/attendance/students", token, 403)
		do("/api/v1/attendance/summary", token, 403)
	}
	do("/api/v1/attendance/summary", "", 401)
	do("/api/v1/attendance/students", device.Credential, 401)
	for _, suffix := range []string{"?status=SIDE", "?status=in", "?status=", "?limit=0", "?limit=101", "?offset=-1", "?offset=1000001", "?limit=", "?limit=x", "?limit=1&limit=2", "?q=a&q=b", "?unknown=1", "?q=%FF", "?q=%00", "?q=%ZZ", "?q=" + strings.Repeat("a", 201)} {
		do("/api/v1/attendance/students"+suffix, uids[0], 400)
	}
	for _, suffix := range []string{"?status=IN", "?limit=20", "?offset=0", "?q=a&q=b", "?q=%00"} {
		do("/api/v1/attendance/summary"+suffix, uids[0], 400)
	}
	r := httptest.NewRequest("GET", "/api/v1/attendance/summary", strings.NewReader("{}"))
	r.Header.Set("Authorization", "Bearer "+uids[0])
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	if rec.Code != 400 {
		t.Fatal("GET body accepted")
	}
	// Authority checked inside the read transaction, not only HTTP middleware.
	exec("UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", ids[1])
	if _, err := dashboard.PresenceSummary(ctx, uids[1], prefix); !errors.Is(err, d.ErrForbidden) {
		t.Fatal("inactive warden read", err)
	}
	exec("UPDATE hostelhive.users SET is_active=true WHERE user_id=$1", ids[1])
	// An actor changed while waiting for the account lock cannot use an old snapshot.
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err := blocker.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	if _, err := blocker.Exec(ctx, "UPDATE hostelhive.users SET role='student' WHERE user_id=$1", ids[1]); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := dashboard.PresenceSummary(ctx, uids[1], prefix); result <- err }()
	waiting := false
	for attempt := 0; attempt < 100; attempt++ {
		var found bool
		err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock' AND query='LOCK TABLE hostelhive.users IN SHARE MODE')", prefix).Scan(&found)
		if err != nil {
			t.Fatal(err)
		}
		if found {
			waiting = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("reader did not wait for authority lock")
	}
	if blocker.Commit(ctx) != nil {
		t.Fatal("role commit failed")
	}
	if err := <-result; !errors.Is(err, d.ErrForbidden) {
		t.Fatal("stale authority snapshot", err)
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err := dashboard.PresenceStudents(canceled, uids[0], dto.PresenceFilter{Limit: 20}); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal("database failure not closed", err)
	}
}

type dashboardTrace struct{ beforeRows func() }

func (t dashboardTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(data.SQL, "SELECT s.student_id::text,s.index_no,s.full_name,COALESCE") {
		t.beforeRows()
	}
	return ctx
}
func (dashboardTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
