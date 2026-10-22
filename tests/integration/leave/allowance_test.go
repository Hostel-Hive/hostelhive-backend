package leave_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestYearlyLeaveAllowance(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL migrated to28")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	// The database session timezone must not affect Colombo date charging.
	config.ConnConfig.RuntimeParams["timezone"] = "America/New_York"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := "allowance102-" + uuid.NewString() + "-"
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
	users := map[string]string{}
	for _, name := range []string{"admin", "student", "other", "missing"} {
		role := "student"
		if name == "admin" {
			role = "admin"
		}
		users[name] = scalar(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text`, prefix+name, prefix+name+"@example.invalid", role)
	}
	students := map[string]string{}
	for _, name := range []string{"student", "other"} {
		students[name] = scalar(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Allowance fixture','Science',1,'0771234567') RETURNING student_id::text`, users[name], prefix+name)
	}
	defer func() {
		clean, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		for _, sql := range []string{
			`DELETE FROM hostelhive.going_home_declarations WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.student_presence WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.attendance_events WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.scanner_devices WHERE name LIKE $1`,
			`DELETE FROM hostelhive.late_access_requests WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.leave_requests WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.students WHERE index_no LIKE $1`,
			`DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`,
		} {
			if _, err := pool.Exec(clean, sql, prefix+"%"); err != nil {
				t.Error(err)
			}
		}
	}()
	seed := func(name, start, end, status string) string {
		t.Helper()
		id := scalar(`INSERT INTO hostelhive.leave_requests(request_id,student_id,submitted_by,starts_at,ends_at,reason) VALUES($1,$2,$3,$4,$5,'Allowance historical fixture') RETURNING leave_id::text`, uuid.NewString(), students[name], users[name], start, end)
		switch status {
		case "approved", "rejected":
			exec(`UPDATE hostelhive.leave_requests SET status=$2,decided_at=clock_timestamp(),decided_by=$3,decision_note='Fixture decision' WHERE leave_id=$1`, id, status, users["admin"])
		case "cancelled":
			exec(`UPDATE hostelhive.leave_requests SET status='cancelled',cancelled_at=clock_timestamp(),cancelled_by=$2 WHERE leave_id=$1`, id, users[name])
		}
		return id
	}
	// Historical duplicate/overlapping intervals deliberately exercise date union.
	for _, interval := range [][2]string{
		{"2026-01-02T10:00:00+05:30", "2026-01-03T00:00:00+05:30"},
		{"2026-01-02T20:00:00+05:30", "2026-01-03T02:00:00+05:30"},
		{"2026-01-03T12:00:00+05:30", "2026-01-05T00:00:00+05:30"},
		{"2026-01-05T23:59:59.999999+05:30", "2026-01-06T00:00:00+05:30"},
		{"2026-01-05T18:30:00Z", "2026-01-05T19:00:00Z"},
		{"2025-12-31T23:30:00+05:30", "2026-01-01T00:00:00+05:30"},
		{"2026-12-31T23:00:00+05:30", "2027-01-01T01:00:00+05:30"},
		{"2024-02-28T12:00:00+05:30", "2024-03-01T00:00:00+05:30"},
		{"2024-02-29T23:00:00+05:30", "2024-03-01T00:00:00+05:30"},
		{"2028-01-01T00:00:00+05:30", "2028-01-21T00:00:00+05:30"},
		{"2032-01-01T00:00:00+05:30", "2033-01-01T00:00:00+05:30"},
		{"2035-12-31T00:00:00+05:30", "2038-01-01T00:00:00+05:30"},
	} {
		seed("student", interval[0], interval[1], "approved")
	}
	for _, status := range []string{"pending", "rejected", "cancelled"} {
		seed("student", "2026-02-01T00:00:00+05:30", "2026-03-01T00:00:00+05:30", status)
		seed("student", "2034-01-01T00:00:00+05:30", "2034-02-01T00:00:00+05:30", status)
	}
	seed("other", "2026-01-01T00:00:00+05:30", "2027-01-01T00:00:00+05:30", "approved")
	// Neither approved late access, Going Home nor scanned presence consumes leave.
	late := scalar(`INSERT INTO hostelhive.late_access_requests(request_id,student_id,submitted_by,direction,expected_at,reason) VALUES($1,$2,$3,'IN','2026-02-01T21:00:00+05:30','Ignored by allowance') RETURNING late_access_id::text`, uuid.NewString(), students["student"], users["student"])
	exec(`UPDATE hostelhive.late_access_requests SET status='approved',decided_at=clock_timestamp(),decided_by=$2 WHERE late_access_id=$1`, late, users["admin"])
	exec(`INSERT INTO hostelhive.going_home_declarations(request_id,student_id,submitted_by) VALUES($1,$2,$3)`, uuid.NewString(), students["student"], users["student"])
	device := scalar(`INSERT INTO hostelhive.scanner_devices(device_id,name,credential_hash,direction,created_by,updated_by) VALUES(gen_random_uuid(),$1,decode(repeat('00',32),'hex'),'IN',$2,$2) RETURNING device_id::text`, prefix+"gate", users["admin"])
	event := scalar(`INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at,access_result) VALUES($1,$2,$3,decode(repeat('00',32),'hex'),'IN','recorded',clock_timestamp(),'not_required') RETURNING event_id::text`, uuid.NewString(), device, students["student"])
	exec(`INSERT INTO hostelhive.student_presence(student_id,status,updated_at,device_id,event_id) SELECT student_id,'IN',occurred_at,device_id,event_id FROM hostelhive.attendance_events WHERE device_id=$1 AND event_id=$2`, device, event)
	store := repository.New(pool)
	mux := http.NewServeMux()
	// Exercise the retained historical calculation in isolation. The production
	// route now returns410 and is covered by TestRetiredLeaveRoutes.
	auth := middleware.Middleware(cancelVerifier{}, identityrepo.NewPostgresAccounts(pool), 5*time.Second)
	mux.Handle("GET /api/v1/me/leave-allowance", auth(http.HandlerFunc(handler.NewAllowance(service.NewAllowance(store), 5*time.Second).Get)))
	call := func(query, body, name string, want int) domain.AllowanceSummary {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/v1/me/leave-allowance"+query, strings.NewReader(body))
		if name != "" {
			r.Header.Set("Authorization", "Bearer "+prefix+name)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: want%d got%d %s", name, query, want, w.Code, w.Body)
		}
		var v domain.AllowanceSummary
		if want == 200 {
			if json.Unmarshal(w.Body.Bytes(), &v) != nil || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(w.Body, w.Header())
			}
		}
		return v
	}
	snapshot := func() string {
		return scalar(`SELECT jsonb_build_object(
 'leaves',(SELECT jsonb_agg(to_jsonb(v) ORDER BY leave_id) FROM hostelhive.leave_requests v),
 'late_access',(SELECT jsonb_agg(to_jsonb(v) ORDER BY late_access_id) FROM hostelhive.late_access_requests v),
 'going_home',(SELECT jsonb_agg(to_jsonb(v) ORDER BY going_home_id) FROM hostelhive.going_home_declarations v),
 'students',(SELECT jsonb_agg(to_jsonb(v) ORDER BY student_id) FROM hostelhive.students v),
 'presence',(SELECT jsonb_agg(to_jsonb(v) ORDER BY student_id) FROM hostelhive.student_presence v),
 'attendance',(SELECT jsonb_agg(to_jsonb(v) ORDER BY device_id,event_id) FROM hostelhive.attendance_events v),
 'events',(SELECT jsonb_agg(to_jsonb(v) ORDER BY event_id) FROM hostelhive.notification_events v),
 'outbox',(SELECT jsonb_agg(to_jsonb(v) ORDER BY event_id,recipient_id) FROM hostelhive.notification_outbox v),
 'publication',(SELECT to_jsonb(v) FROM hostelhive.attendance_publication v))::text`)
	}
	before := snapshot()
	for _, tc := range []struct{ year, days int }{
		{1, 0}, {2024, 2}, {2025, 1}, {2026, 6}, {2027, 1}, {2028, 20},
		{2032, 366}, {2033, 0}, {2034, 0}, {2035, 1}, {2036, 366}, {2037, 365}, {2038, 0}, {9998, 0},
	} {
		v := call(fmt.Sprintf("?year=%04d", tc.year), "", "student", 200)
		if v.StudentID != students["student"] || v.Year != tc.year || v.Timezone != "Asia/Colombo" || v.TotalDays != 18 || v.UsedDays != tc.days || v.RemainingDays != max(18-tc.days, 0) || v.ExceededDays != max(tc.days-18, 0) {
			t.Fatal(tc, v)
		}
	}
	location, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		t.Fatal(err)
	}
	current := call("", "", "student", 200)
	if current.Year != time.Now().In(location).Year() {
		t.Fatal("default year is not Colombo current year", current)
	}
	other := call("?year=2026", "", "other", 200)
	if other.StudentID != students["other"] || other.UsedDays != 365 {
		t.Fatal("another Student's summary leaked or mixed accounts", other)
	}
	call("?student_id="+students["student"], "", "other", 400)
	call("?user_id="+users["student"], "", "other", 400)
	call("?year=2026", "{}", "student", 400)
	call("?year=2026", "", "admin", 403)
	call("?year=2026", "", "missing", 409)
	call("?year=2026", "", "", 401)
	if before != snapshot() {
		t.Fatal("allowance reads changed source history, presence or notification work")
	}
	if scalar(`SELECT current_setting('TimeZone')`) != "America/New_York" {
		t.Fatal("fixture did not exercise a different database timezone")
	}
	period := domain.AllowancePeriod{Year: 2026, Start: time.Date(2026, 1, 1, 0, 0, 0, 0, location), End: time.Date(2027, 1, 1, 0, 0, 0, 0, location), Timezone: "Asia/Colombo"}
	if _, err := store.AllowanceUsage(ctx, prefix+"admin", period); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("repository did not recheck Student role", err)
	}
	exec(`UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, students["student"], users["admin"])
	call("?year=2026", "", "student", 409)
	exec(`UPDATE hostelhive.students SET deleted_at=NULL,deleted_by=NULL WHERE student_id=$1`, students["student"])
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, users["student"])
	call("?year=2026", "", "student", 403)
	exec(`UPDATE hostelhive.users SET is_active=true WHERE user_id=$1`, users["student"])
	// This summary is not an admission rule: submission AND approval still work
	// after 18 approved dates, and approved future dates count immediately.
	futureYear := time.Now().In(location).Year() + 100
	seed("student", fmt.Sprintf("%04d-01-01T00:00:00+05:30", futureYear), fmt.Sprintf("%04d-01-21T00:00:00+05:30", futureYear), "approved")
	start := time.Date(futureYear, 1, 25, 0, 0, 0, 0, location)
	requests := service.New(store)
	request, err := requests.Submit(ctx, prefix+"student", dto.Submit{RequestID: uuid.NewString(), StartsAt: start, EndsAt: start.AddDate(0, 0, 1), Reason: "Allowance is informational"})
	if err != nil {
		t.Fatal("allowance incorrectly blocked submission", err)
	}
	if _, err := requests.Decide(ctx, prefix+"admin", request.LeaveID, dto.Decision{Status: "approved"}); err != nil {
		t.Fatal("allowance incorrectly blocked approval", err)
	}
	v := call(fmt.Sprintf("?year=%04d", futureYear), "", "student", 200)
	if v.UsedDays != 21 || v.RemainingDays != 0 || v.ExceededDays != 3 {
		t.Fatal("future approved dates or excess usage missing", v)
	}
}
