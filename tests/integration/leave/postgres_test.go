package leave_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type verifier struct{ uid string }

func (v verifier) Verify(context.Context, string) (string, error) { return v.uid, nil }
func TestLeaveWorkflow(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable database migrated to 15")
	}
	ctx, c := context.WithTimeout(context.Background(), 90*time.Second)
	defer c()
	pool, e := pgxpool.New(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	prefix := fmt.Sprintf("leave-%d", time.Now().UnixNano())
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	insert := func(sql string, args ...any) string {
		t.Helper()
		var id string
		if e := pool.QueryRow(ctx, sql, args...).Scan(&id); e != nil {
			t.Fatal(e)
		}
		return id
	}
	user := func(suffix, role string) string {
		return insert(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text`, prefix+suffix, prefix+suffix+"@example.invalid", role)
	}
	admin := user("admin", "admin")
	warden := user("warden", "warden")
	u := user("student", "student")
	other := user("other", "student")
	user("security", "security_staff")
	user("sub", "sub_warden")
	user("missing", "student")
	student := func(uid, index string) string {
		return insert(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Leave Test','Engineering',1,'0771234567') RETURNING student_id::text`, uid, prefix+index)
	}
	sid := student(u, "student")
	otherSID := student(other, "other")
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		for _, sql := range []string{
			`DELETE FROM hostelhive.student_presence WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.attendance_events WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.scanner_devices WHERE name LIKE $1`,
			`DELETE FROM hostelhive.leave_requests WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.students WHERE index_no LIKE $1`,
			`DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`,
		} {
			if _, e := pool.Exec(clean, sql, prefix+"%"); e != nil {
				t.Error(e)
			}
		}
	}()
	s := service.New(repository.New(pool))
	uid := prefix + "student"
	auid := prefix + "admin"
	wuid := prefix + "warden"
	expect := func(e, want error) {
		t.Helper()
		if !errors.Is(e, want) {
			t.Fatalf("want %v got %v", want, e)
		}
	}
	input := func(hours int) dto.Submit {
		return dto.Submit{RequestID: insert(`SELECT gen_random_uuid()::text`), StartsAt: time.Now().UTC().Add(time.Duration(hours) * time.Hour).Truncate(time.Microsecond), EndsAt: time.Now().UTC().Add(time.Duration(hours+1) * time.Hour).Truncate(time.Microsecond), Reason: "Family visit"}
	}
	in := input(24)
	v, e := s.Submit(ctx, uid, in)
	if e != nil || v.Status != "pending" || v.StudentID != sid || v.SubmittedBy != u || v.DecidedAt != nil {
		t.Fatal(v, e)
	}
	again, e := s.Submit(ctx, uid, in)
	if e != nil || again.LeaveID != v.LeaveID {
		t.Fatal("retry duplicated request", again, e)
	}
	changed := in
	changed.Reason = "changed"
	_, e = s.Submit(ctx, uid, changed)
	expect(e, domain.ErrConflict)
	overlap := in
	overlap.RequestID = input(0).RequestID
	_, e = s.Submit(ctx, uid, overlap)
	expect(e, domain.ErrConflict)
	adjacent := input(0)
	adjacent.StartsAt = in.EndsAt
	adjacent.EndsAt = in.EndsAt.Add(time.Hour)
	adj, e := s.Submit(ctx, uid, adjacent)
	if e != nil {
		t.Fatal(e)
	}
	past := input(-5)
	_, e = s.Submit(ctx, uid, past)
	expect(e, domain.ErrInvalid)
	_, e = s.Submit(ctx, auid, input(48))
	expect(e, domain.ErrForbidden)
	_, e = s.Submit(ctx, prefix+"missing", input(48))
	expect(e, domain.ErrIneligible)
	_, e = s.Get(ctx, prefix+"other", v.LeaveID)
	expect(e, domain.ErrNotFound)
	_, e = s.List(ctx, prefix+"other", domain.Filter{StudentID: sid, Limit: 20})
	expect(e, domain.ErrForbidden)
	p, e := s.List(ctx, prefix+"other", domain.Filter{Limit: 20})
	if e != nil || p.Total != 0 || p.Items == nil {
		t.Fatal(p, e)
	}
	p, e = s.List(ctx, uid, domain.Filter{Limit: 1})
	if e != nil || p.Total != 2 || len(p.Items) != 1 || !p.HasMore {
		t.Fatal(p, e)
	}
	p, e = s.List(ctx, uid, domain.Filter{Limit: 1, Offset: 99})
	if e != nil || p.Total != 2 || len(p.Items) != 0 || p.HasMore {
		t.Fatal(p, e)
	}
	_, e = s.Decide(ctx, uid, v.LeaveID, dto.Decision{Status: "approved"})
	expect(e, domain.ErrForbidden)
	approved, e := s.Decide(ctx, wuid, v.LeaveID, dto.Decision{Status: "approved", Note: "Approved"})
	if e != nil || approved.DecidedBy == nil || *approved.DecidedBy != warden || approved.DecidedAt == nil {
		t.Fatal(approved, e)
	}
	again, e = s.Decide(ctx, wuid, v.LeaveID, dto.Decision{Status: "approved", Note: "Approved"})
	if e != nil || !again.DecidedAt.Equal(*approved.DecidedAt) {
		t.Fatal("retry changed audit", again, e)
	}
	_, e = s.Decide(ctx, auid, v.LeaveID, dto.Decision{Status: "rejected", Note: "Changed mind"})
	expect(e, domain.ErrConflict)
	_, e = s.Submit(ctx, uid, overlap)
	expect(e, domain.ErrConflict)
	rejected, e := s.Decide(ctx, auid, adj.LeaveID, dto.Decision{Status: "rejected", Note: "Insufficient details"})
	if e != nil || *rejected.DecidedBy != admin {
		t.Fatal(rejected, e)
	}
	adjacent.RequestID = input(0).RequestID
	if _, e = s.Submit(ctx, uid, adjacent); e != nil {
		t.Fatal("rejected request blocked resubmission", e)
	}
	// Concurrent overlapping submissions must have exactly one winner.
	race := func(inputs []dto.Submit) {
		t.Helper()
		start := make(chan struct{})
		results := make(chan error, len(inputs))
		for _, in := range inputs {
			go func(v dto.Submit) { <-start; _, e := s.Submit(ctx, uid, v); results <- e }(in)
		}
		close(start)
		wins := 0
		for range inputs {
			e := <-results
			if e == nil {
				wins++
			} else {
				expect(e, domain.ErrConflict)
			}
		}
		if wins != 1 {
			t.Fatal("overlap race winners", wins)
		}
	}
	r1 := input(72)
	r2 := r1
	r2.RequestID = input(0).RequestID
	race([]dto.Submit{r1, r2})
	decisionIn := input(96)
	pending, e := s.Submit(ctx, uid, decisionIn)
	if e != nil {
		t.Fatal(e)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, decision := range []dto.Decision{{Status: "approved"}, {Status: "rejected", Note: "Not approved"}} {
		go func(d dto.Decision) { <-start; _, e := s.Decide(ctx, auid, pending.LeaveID, d); results <- e }(decision)
	}
	close(start)
	wins := 0
	for range 2 {
		e := <-results
		if e == nil {
			wins++
		} else {
			expect(e, domain.ErrConflict)
		}
	}
	if wins != 1 {
		t.Fatal("decision race winners", wins)
	}
	// Ineligible students cannot submit or receive new approval; history remains.
	inactiveIn := input(120)
	inactive, e := s.Submit(ctx, uid, inactiveIn)
	if e != nil {
		t.Fatal(e)
	}
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, u)
	_, e = s.Submit(ctx, uid, input(144))
	expect(e, domain.ErrForbidden)
	_, e = s.Decide(ctx, auid, inactive.LeaveID, dto.Decision{Status: "approved"})
	expect(e, domain.ErrIneligible)
	if _, e = s.Decide(ctx, auid, inactive.LeaveID, dto.Decision{Status: "rejected", Note: "Account inactive"}); e != nil {
		t.Fatal(e)
	}
	exec(`UPDATE hostelhive.users SET is_active=true WHERE user_id=$1`, u)
	archivedIn := input(144)
	archived, e := s.Submit(ctx, uid, archivedIn)
	if e != nil {
		t.Fatal(e)
	}
	exec(`UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, sid, admin)
	_, e = s.Submit(ctx, uid, input(168))
	expect(e, domain.ErrIneligible)
	_, e = s.Decide(ctx, wuid, archived.LeaveID, dto.Decision{Status: "approved"})
	expect(e, domain.ErrIneligible)
	if _, e = s.Get(ctx, uid, v.LeaveID); e != nil {
		t.Fatal("archive erased history", e)
	}
	exec(`UPDATE hostelhive.students SET deleted_at=NULL,deleted_by=NULL WHERE student_id=$1`, sid)
	lateIn := input(168)
	late, e := s.Submit(ctx, uid, lateIn)
	if e != nil {
		t.Fatal(e)
	}
	exec(`UPDATE hostelhive.leave_requests SET starts_at=clock_timestamp()-interval '1 hour' WHERE leave_id=$1`, late.LeaveID)
	_, e = s.Decide(ctx, wuid, late.LeaveID, dto.Decision{Status: "approved"})
	expect(e, domain.ErrConflict)
	if _, e = s.Decide(ctx, wuid, late.LeaveID, dto.Decision{Status: "rejected", Note: "Too late"}); e != nil {
		t.Fatal(e)
	}
	// Full HTTP -> middleware -> service -> PostgreSQL path.
	call := func(actor, method, path, body string, want int) {
		t.Helper()
		mux := http.NewServeMux()
		leave.Register(handler.New(s, 3*time.Second), mux, middleware.Middleware(verifier{actor}, identityrepo.NewPostgresAccounts(pool), 3*time.Second))
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if actor != "" {
			r.Header.Set("Authorization", "Bearer test")
		}
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s want %d got %d %s", method, path, want, w.Code, w.Body.String())
		}
	}
	apiIn := input(192)
	b, _ := json.Marshal(apiIn)
	call(uid, "POST", "/api/v1/leaves", string(b), 201)
	call(uid, "GET", "/api/v1/leaves?status=approved&limit=1", "", 200)
	call(wuid, "GET", "/api/v1/leaves?student_id="+sid, "", 200)
	call(auid, "GET", "/api/v1/leaves/"+v.LeaveID, "", 200)
	call(prefix+"other", "GET", "/api/v1/leaves/"+v.LeaveID, "", 404)
	call(uid, "GET", "/api/v1/leaves?student_id="+otherSID, "", 403)
	call(uid, "POST", "/api/v1/leaves/"+v.LeaveID+"/decision", `{"status":"approved"}`, 403)
	apiPage, e := s.List(ctx, uid, domain.Filter{Limit: 100})
	if e != nil {
		t.Fatal(e)
	}
	apiID := apiPage.Items[0].LeaveID
	call(wuid, "POST", "/api/v1/leaves/"+apiID+"/decision", `{"status":"approved"}`, 200)
	call("", "GET", "/api/v1/leaves", "", 401)
	for _, role := range []string{"security", "sub"} {
		call(prefix+role, "GET", "/api/v1/leaves", "", 403)
	}
	for _, q := range []string{"status=bad", "limit=0", "offset=-1", "limit=1&limit=2", "unknown=1", "status="} {
		call(uid, "GET", "/api/v1/leaves?"+q, "", 400)
	}
	for _, body := range []string{`{}`, string(b) + ` {}`, `{"student_id":"forged"}`, `{"reason":null}`} {
		call(uid, "POST", "/api/v1/leaves", body, 400)
	}
	call(uid, "POST", "/api/v1/leaves", strings.Repeat(" ", 17000)+`{}`, 413)
	call(uid, "GET", "/api/v1/leaves", `{}`, 400)
	call(uid, "GET", "/api/v1/leaves/bad", "", 400)
	call(uid, "GET", "/api/v1/leaves/00000000-0000-4000-8000-000000000001", "", 404)
	// Neither submission nor decisions manufacture attendance/presence records.
	var count int
	if e = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM hostelhive.student_presence WHERE student_id=$1)+(SELECT count(*) FROM hostelhive.attendance_events WHERE student_id=$1)`, sid).Scan(&count); e != nil || count != 0 {
		t.Fatal("leave changed presence", count, e)
	}
	// Existing scanned IN state must remain unchanged by a new approval.
	device := insert(`INSERT INTO hostelhive.scanner_devices(device_id,name,credential_hash,created_by,updated_by,direction) VALUES(gen_random_uuid(),$1,decode(repeat('01',32),'hex'),$2,$2,'IN') RETURNING device_id::text`, prefix+"device", admin)
	event := insert(`SELECT gen_random_uuid()::text`)
	exec(`INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at) VALUES($1,$2,$3,decode(repeat('02',32),'hex'),'IN','recorded',clock_timestamp())`, event, device, sid)
	exec(`INSERT INTO hostelhive.student_presence(student_id,status,updated_at,device_id,event_id) VALUES($1,'IN',clock_timestamp(),$2,$3)`, sid, device, event)
	var beforeVersion, afterVersion int64
	if e = pool.QueryRow(ctx, `SELECT version FROM hostelhive.attendance_publication WHERE singleton`).Scan(&beforeVersion); e != nil {
		t.Fatal(e)
	}
	preserve, e := s.Submit(ctx, uid, input(240))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Decide(ctx, auid, preserve.LeaveID, dto.Decision{Status: "approved"}); e != nil {
		t.Fatal(e)
	}
	var presence string
	if e = pool.QueryRow(ctx, `SELECT status FROM hostelhive.student_presence WHERE student_id=$1`, sid).Scan(&presence); e != nil || presence != "IN" {
		t.Fatal("approval changed scanned IN", presence, e)
	}
	if e = pool.QueryRow(ctx, `SELECT version FROM hostelhive.attendance_publication WHERE singleton`).Scan(&afterVersion); e != nil || afterVersion != beforeVersion {
		t.Fatal("leave queued a presence publication", e)
	}
	exec(`UPDATE hostelhive.users SET role='security_staff' WHERE user_id=$1`, warden)
	_, e = s.Get(ctx, wuid, v.LeaveID)
	expect(e, domain.ErrForbidden)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, e = s.List(cancelled, auid, domain.Filter{Limit: 20})
	expect(e, domain.ErrUnavailable)
}
