package attendance_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/handler"
	repo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/repository"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/service"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
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

func TestStudentAttendanceHistoryPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := "history93-" + uuid.NewString()
	uids, ids, students, devices := []string{}, []string{}, []string{}, []string{}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	defer func() {
		clean, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		for _, sql := range []string{
			"DELETE FROM hostelhive.student_presence WHERE student_id=ANY($1::uuid[])",
			"DELETE FROM hostelhive.attendance_events WHERE student_id=ANY($1::uuid[])",
			"DELETE FROM hostelhive.late_access_requests WHERE student_id=ANY($1::uuid[])",
			"DELETE FROM hostelhive.students WHERE student_id=ANY($1::uuid[])",
		} {
			if _, e := pool.Exec(clean, sql, students); e != nil {
				t.Error("student fixture cleanup", e)
			}
		}
		for _, sql := range []string{
			"DELETE FROM hostelhive.scanner_devices WHERE created_by=ANY($1::uuid[])",
			"DELETE FROM hostelhive.users WHERE user_id=ANY($1::uuid[])",
		} {
			if _, e := pool.Exec(clean, sql, ids); e != nil {
				t.Error("user fixture cleanup", e)
			}
		}
	}()
	for i, role := range []string{"admin", "warden", "sub_warden", "security_staff", "student", "student", "student", "student"} {
		uid := fmt.Sprintf("%s-%d", prefix, i)
		var id string
		if e := pool.QueryRow(ctx, "INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text", uid, uid+"@example.invalid", role).Scan(&id); e != nil {
			t.Fatal(e)
		}
		uids = append(uids, uid)
		ids = append(ids, id)
	}
	for i := 4; i <= 6; i++ {
		id := uuid.NewString()
		students = append(students, id)
		exec("INSERT INTO hostelhive.students(student_id,user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,$3,'History Student','Science',1,'0771234567')", id, ids[i], fmt.Sprintf("H93-%s-%d", uuid.NewString(), i))
	}
	for i := 0; i < 2; i++ {
		id := uuid.NewString()
		devices = append(devices, id)
		exec("INSERT INTO hostelhive.scanner_devices(device_id,name,credential_hash,created_by,updated_by,direction) VALUES($1,$2,$3,$4,$4,'IN')", id, fmt.Sprintf("%s-%d", prefix, i), make([]byte, 32), ids[0])
	}
	loc := time.FixedZone("Asia/Colombo", 19800)
	start := time.Date(2026, 10, 15, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)
	tie := start.Add(12 * time.Hour)
	insert := func(student, device, event, direction, outcome string, at time.Time) {
		t.Helper()
		hash := sha256.Sum256([]byte(device + event))
		exec("INSERT INTO hostelhive.attendance_events(device_id,event_id,student_id,payload_hash,direction,outcome,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7)", device, event, student, hash[:], direction, outcome, at)
	}
	same := uuid.NewString()
	insert(students[0], devices[0], uuid.NewString(), "IN", "recorded", start.Add(-time.Microsecond))
	insert(students[0], devices[0], uuid.NewString(), "OUT", "recorded", start)
	insert(students[0], devices[0], same, "IN", "recorded", tie)
	insert(students[0], devices[1], same, "OUT", "recorded", tie)
	insert(students[0], devices[0], uuid.NewString(), "IN", "recorded", end.Add(-time.Microsecond))
	insert(students[0], devices[0], uuid.NewString(), "OUT", "recorded", end)
	for _, outcome := range []string{"cooldown", "already_in", "already_out"} {
		insert(students[0], devices[0], uuid.NewString(), "IN", outcome, tie)
	}
	insert(students[1], devices[0], uuid.NewString(), "OUT", "recorded", tie)
	exec("INSERT INTO hostelhive.student_presence(student_id,status,updated_at,device_id,event_id) VALUES($1,'IN',$2,$3,$4)", students[0], tie, devices[0], same)
	permission := uuid.NewString()
	exec("INSERT INTO hostelhive.late_access_requests(late_access_id,request_id,student_id,submitted_by,direction,expected_at,reason) VALUES($1,$2,$3,$4,'IN',$5,'history read preservation')", permission, uuid.NewString(), students[0], ids[4], end.Add(21*time.Hour))
	exec("UPDATE hostelhive.late_access_requests SET status='approved',decided_at=clock_timestamp(),decided_by=$2 WHERE late_access_id=$1", permission, ids[0])
	store := repo.NewDevices(pool)
	service := svc.NewStudentHistory(store, loc)
	mux := http.NewServeMux()
	attendance.RegisterStudentHistory(h.NewStudentHistoryAPI(service, 5*time.Second), mux, m.Middleware(verifier{}, identityrepo.NewPostgresAccounts(pool), time.Second))
	do := func(query, token string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/v1/me/attendance/events"+query, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s HTTP %d: %s", query, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cacheable history")
		}
		return w
	}
	page := func(query, token string) d.StudentHistoryPage {
		t.Helper()
		w := do(query, token, 200)
		var p d.StudentHistoryPage
		if e := json.Unmarshal(w.Body.Bytes(), &p); e != nil {
			t.Fatal(e)
		}
		if p.Items == nil {
			t.Fatal("null items")
		}
		return p
	}
	snapshot := func() string {
		t.Helper()
		var s string
		e := pool.QueryRow(ctx, `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(e) ORDER BY device_id,event_id) FROM hostelhive.attendance_events e WHERE student_id=ANY($1::uuid[])),
 (SELECT jsonb_agg(to_jsonb(p) ORDER BY student_id) FROM hostelhive.student_presence p WHERE student_id=ANY($1::uuid[])),
 (SELECT jsonb_agg(to_jsonb(l) ORDER BY late_access_id) FROM hostelhive.late_access_requests l WHERE student_id=ANY($1::uuid[])))::text`, students).Scan(&s)
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	before := snapshot()
	all := page("", uids[4])
	if all.Total != 6 || len(all.Items) != 6 || all.Limit != 20 || all.Offset != 0 || all.HasMore || all.From != nil || all.To != nil || all.Timezone != "Asia/Colombo" {
		t.Fatalf("all history %+v", all)
	}
	q := "?from=2026-10-15&to=2026-10-15"
	window := page(q, uids[4])
	if window.Total != 4 || len(window.Items) != 4 || window.From == nil || *window.From != "2026-10-15" || *window.To != "2026-10-15" {
		t.Fatalf("local window %+v", window)
	}
	seen := map[string]bool{}
	sameIDs := map[string]bool{}
	for i := 0; i < 4; i++ {
		p := page(q+fmt.Sprintf("&limit=1&offset=%d", i), uids[4])
		if p.Total != 4 || len(p.Items) != 1 || p.HasMore != (i < 3) || p.Offset != i || p.Limit != 1 {
			t.Fatal("pagination", p)
		}
		item := p.Items[0]
		if seen[item.MovementID] || item.MovementID != window.Items[i].MovementID || len(item.MovementID) != 64 {
			t.Fatal("unstable or duplicate movement key")
		}
		seen[item.MovementID] = true
		if item.EventID == same {
			sameIDs[item.MovementID] = true
		}
		if i > 0 && item.OccurredAt.After(window.Items[i-1].OccurredAt) {
			t.Fatal("not newest first")
		}
	}
	if len(sameIDs) != 2 {
		t.Fatal("composite database event identity collapsed")
	}
	for _, direction := range []string{"IN", "OUT"} {
		p := page(q+"&direction="+direction, uids[4])
		if p.Total != 2 || len(p.Items) != 2 {
			t.Fatal("direction total", p)
		}
		for _, item := range p.Items {
			if item.Direction != direction {
				t.Fatal("direction leak")
			}
		}
	}
	if p := page(q+"&offset=99", uids[4]); len(p.Items) != 0 || p.Total != 4 || p.HasMore {
		t.Fatal("beyond last page")
	}
	if p := page("?from=2000-01-01&to=2000-01-01", uids[4]); p.Total != 0 || len(p.Items) != 0 {
		t.Fatal("empty date period")
	}
	if p := page("", uids[5]); p.Total != 1 || p.Items[0].Direction != "OUT" {
		t.Fatal("cross-student leak")
	}
	if p := page("", uids[6]); p.Total != 0 || len(p.Items) != 0 || p.HasMore {
		t.Fatal("no-event student")
	}
	raw := do(q, uids[4], 200).Body.String()
	for _, key := range []string{"device_id", "student_id", "payload_hash", "credential_hash", "access_result", "late_access_id", "incident_id", "outcome"} {
		if strings.Contains(raw, `"`+key+`"`) {
			t.Fatal("internal field leaked", key)
		}
	}
	if before != snapshot() {
		t.Fatal("history reads changed events, presence or permissions")
	}
	for _, uid := range uids[:4] {
		do("", uid, 403)
	}
	do("", "", 401)
	do("", uids[7], 404)
	for _, query := range []string{"?student_id=" + students[1], "?user_id=" + ids[5], "?limit=1&limit=2", "?direction=in", "?from=2026-10-01", "?from=bad&to=bad"} {
		do(query, uids[4], 400)
	}
	exec("UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1", students[0], ids[0])
	do("", uids[4], 404)
	if _, e := service.Events(ctx, uids[4], dto.StudentHistoryQuery{Limit: 20}); !errors.Is(e, d.ErrNotFound) {
		t.Fatal("repository archive eligibility", e)
	}
	exec("UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", ids[5])
	do("", uids[5], 403)
	if _, e := service.Events(ctx, uids[5], dto.StudentHistoryQuery{Limit: 20}); !errors.Is(e, d.ErrForbidden) {
		t.Fatal("repository account eligibility", e)
	}
	exec("UPDATE hostelhive.users SET is_active=true,role='warden' WHERE user_id=$1", ids[5])
	do("", uids[5], 403)
	if _, e := service.Events(ctx, uids[5], dto.StudentHistoryQuery{Limit: 20}); !errors.Is(e, d.ErrForbidden) {
		t.Fatal("repository role eligibility", e)
	}
}
