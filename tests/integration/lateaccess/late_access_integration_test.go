package lateaccess_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/service"
	ndomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	ndto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/dto"
	nrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/repository"
	nservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type verifier struct{}

func (verifier) Verify(_ context.Context, token string) (string, error) { return token, nil }
func TestLateAccessWorkflow(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL migrated to 21")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, e := pgxpool.New(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	prefix := fmt.Sprintf("lateaccess-%d-", time.Now().UnixNano())
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	scalar := func(sql string, args ...any) string {
		t.Helper()
		var v string
		if e := pool.QueryRow(ctx, sql, args...).Scan(&v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	uuid := func() string { return scalar(`SELECT gen_random_uuid()::text`) }
	user := func(name, role string) string {
		return scalar(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text`, prefix+name, prefix+name+"@example.invalid", role)
	}
	admin := user("admin", "admin")
	user("warden", "warden")
	student := user("student", "student")
	other := user("other", "student")
	user("sub", "sub_warden")
	user("security", "security_staff")
	user("missing", "student")
	profile := func(uid, name string) string {
		return scalar(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Late Access Test','Engineering',1,'0771234567') RETURNING student_id::text`, uid, prefix+name)
	}
	sid := profile(student, "student")
	otherSID := profile(other, "other")
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		for _, sql := range []string{
			`DELETE FROM hostelhive.student_presence WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.attendance_events WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.scanner_device_audit WHERE device_id IN(SELECT device_id FROM hostelhive.scanner_devices WHERE name LIKE $1)`,
			`DELETE FROM hostelhive.scanner_devices WHERE name LIKE $1`,
			`DELETE FROM hostelhive.late_access_requests WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.students WHERE index_no LIKE $1`,
			`DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`,
		} {
			if _, e := pool.Exec(clean, sql, prefix+"%"); e != nil {
				t.Error(e)
			}
		}
	}()
	zone := time.FixedZone("Colombo", 19800)
	policy := domain.Policy{Location: zone, ExitOpening: 360, EntryDeadline: 1200}
	store := repository.New(pool, policy)
	s := service.New(store)
	nstore := nrepo.New(pool)
	inbox := nservice.New(nstore)
	uid := prefix + "student"
	auid := prefix + "admin"
	wuid := prefix + "warden"
	makeInput := func(day, hour int, direction string) dto.Submit {
		date := time.Now().In(zone).AddDate(0, 0, day)
		return dto.Submit{RequestID: uuid(), Direction: direction, ExpectedAt: time.Date(date.Year(), date.Month(), date.Day(), hour, 0, 0, 0, zone), Reason: "Travel permission"}
	}
	expect := func(err, want error) {
		t.Helper()
		if !errors.Is(err, want) {
			t.Fatalf("want %v, got %v", want, err)
		}
	}
	in := makeInput(30, 21, "IN")
	v, e := s.Submit(ctx, uid, in)
	if e != nil || v.StudentID != sid || v.SubmittedBy != student || v.Status != "pending" || v.Direction != "IN" {
		t.Fatal(v, e)
	}
	retry, e := s.Submit(ctx, uid, in)
	if e != nil || retry.LateAccessID != v.LateAccessID {
		t.Fatal("retry", retry, e)
	}
	changed := in
	changed.Reason = "changed"
	_, e = s.Submit(ctx, uid, changed)
	expect(e, domain.ErrConflict)
	duplicate := in
	duplicate.RequestID = uuid()
	_, e = s.Submit(ctx, uid, duplicate)
	expect(e, domain.ErrConflict)
	// Submission and delivery are independent durable transactions.
	exec(`ALTER TABLE hostelhive.notifications ADD CONSTRAINT late_access_delivery_failure CHECK(recipient_id <> '` + admin + `'::uuid)`)
	_, deliveryError := nstore.Deliver(ctx)
	exec(`ALTER TABLE hostelhive.notifications DROP CONSTRAINT late_access_delivery_failure`)
	if deliveryError == nil {
		t.Fatal("expected provider delivery failure")
	}
	if _, e = s.Get(ctx, uid, v.LateAccessID); e != nil {
		t.Fatal("failed delivery lost request", e)
	}
	if _, e = nstore.Deliver(ctx); e != nil {
		t.Fatal(e)
	}
	alert := func(who, kind, id string) ndomain.Notification {
		t.Helper()
		page, e := inbox.List(ctx, who, ndto.Filter{Limit: 100})
		if e != nil {
			t.Fatal(e)
		}
		var found []ndomain.Notification
		for _, n := range page.Items {
			if n.LateAccessID == id && n.Kind == kind {
				found = append(found, n)
			}
		}
		if len(found) != 1 {
			t.Fatalf("%s %s: %v", who, kind, found)
		}
		return found[0]
	}
	adminAlert := alert(auid, "late_access_submitted", v.LateAccessID)
	alert(wuid, "late_access_submitted", v.LateAccessID)
	if adminAlert.SourceType != "late_access" || adminAlert.Status != "pending" || adminAlert.LeaveID != "" || adminAlert.ComplaintID != "" || adminAlert.AnnouncementID != "" {
		t.Fatal(adminAlert)
	}
	approved, e := s.Decide(ctx, wuid, v.LateAccessID, dto.Decision{Status: "approved"})
	if e != nil || approved.DecidedAt == nil {
		t.Fatal(approved, e)
	}
	approvedAgain, e := s.Decide(ctx, wuid, v.LateAccessID, dto.Decision{Status: "approved"})
	if e != nil || !approvedAgain.DecidedAt.Equal(*approved.DecidedAt) {
		t.Fatal(approvedAgain, e)
	}
	_, e = s.Decide(ctx, auid, v.LateAccessID, dto.Decision{Status: "approved"})
	expect(e, domain.ErrConflict)
	_, e = s.Decide(ctx, wuid, v.LateAccessID, dto.Decision{Status: "rejected", Note: "different"})
	expect(e, domain.ErrConflict)
	if _, e = nstore.Deliver(ctx); e != nil {
		t.Fatal(e)
	}
	studentAlert := alert(uid, "late_access_decided", v.LateAccessID)
	if studentAlert.Status != "approved" {
		t.Fatal(studentAlert)
	}
	read, e := inbox.MarkRead(ctx, uid, studentAlert.NotificationID)
	if e != nil || read.ReadAt == nil {
		t.Fatal(read, e)
	}
	readRetry, e := inbox.MarkRead(ctx, uid, studentAlert.NotificationID)
	if e != nil || !readRetry.ReadAt.Equal(*read.ReadAt) {
		t.Fatal(readRetry, e)
	}
	for _, who := range []string{prefix + "other", auid, wuid} {
		_, e = inbox.MarkRead(ctx, who, studentAlert.NotificationID)
		expect(e, ndomain.ErrNotFound)
	}
	_, e = s.Get(ctx, prefix+"other", v.LateAccessID)
	expect(e, domain.ErrNotFound)
	_, e = s.List(ctx, uid, domain.Filter{StudentID: otherSID, Limit: 20})
	expect(e, domain.ErrForbidden)
	if p, e := s.List(ctx, uid, domain.Filter{Status: "approved", Limit: 1}); e != nil || p.Total != 1 || len(p.Items) != 1 {
		t.Fatal(p, e)
	}
	if p, e := s.List(ctx, auid, domain.Filter{StudentID: sid, Limit: 1, Offset: 100}); e != nil || p.Total != 1 || len(p.Items) != 0 || p.HasMore {
		t.Fatal(p, e)
	}
	// Boundaries are evaluated in hostel time, not UTC or arbitrary client offset.
	for _, test := range []struct {
		hour      int
		direction string
	}{{19, "IN"}, {6, "OUT"}, {20, "OUT"}} {
		_, e = s.Submit(ctx, uid, makeInput(31, test.hour, test.direction))
		expect(e, domain.ErrInvalid)
	}
	for _, test := range []struct {
		hour      int
		direction string
	}{{20, "IN"}, {5, "OUT"}, {1, "IN"}} {
		req, e := s.Submit(ctx, uid, makeInput(32, test.hour, test.direction))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.Decide(ctx, auid, req.LateAccessID, dto.Decision{Status: "rejected", Note: "Not permitted"}); e != nil {
			t.Fatal(e)
		}
	}
	_, e = s.Submit(ctx, uid, makeInput(-2, 21, "IN"))
	expect(e, domain.ErrInvalid)
	// Rejected movements allow a replacement; exact retries after expiry still work.
	old, e := s.Submit(ctx, uid, makeInput(33, 5, "OUT"))
	if e != nil {
		t.Fatal(e)
	}
	exec(`UPDATE hostelhive.late_access_requests SET expected_at=clock_timestamp()-interval '1 day' WHERE late_access_id=$1`, old.LateAccessID)
	_, e = s.Decide(ctx, auid, old.LateAccessID, dto.Decision{Status: "approved"})
	expect(e, domain.ErrConflict)
	if _, e = s.Decide(ctx, auid, old.LateAccessID, dto.Decision{Status: "rejected", Note: "Expired"}); e != nil {
		t.Fatal(e)
	}
	inactiveIn := makeInput(34, 21, "IN")
	inactive, e := s.Submit(ctx, uid, inactiveIn)
	if e != nil {
		t.Fatal(e)
	}
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, student)
	_, e = s.Submit(ctx, uid, inactiveIn)
	expect(e, domain.ErrForbidden)
	_, e = s.Decide(ctx, auid, inactive.LateAccessID, dto.Decision{Status: "approved"})
	expect(e, domain.ErrIneligible)
	if _, e = s.Decide(ctx, auid, inactive.LateAccessID, dto.Decision{Status: "rejected", Note: "Inactive"}); e != nil {
		t.Fatal(e)
	}
	exec(`UPDATE hostelhive.users SET is_active=true WHERE user_id=$1`, student)
	replacement := inactiveIn
	replacement.RequestID = uuid()
	archived, e := s.Submit(ctx, uid, replacement)
	if e != nil {
		t.Fatal(e)
	}
	exec(`UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, sid, admin)
	_, e = s.Submit(ctx, uid, makeInput(35, 21, "IN"))
	expect(e, domain.ErrIneligible)
	_, e = s.Decide(ctx, auid, archived.LateAccessID, dto.Decision{Status: "approved"})
	expect(e, domain.ErrIneligible)
	if _, e = s.Get(ctx, uid, v.LateAccessID); e != nil {
		t.Fatal("archival removed history", e)
	}
	exec(`UPDATE hostelhive.students SET deleted_at=NULL,deleted_by=NULL WHERE student_id=$1`, sid)
	_, e = s.Submit(ctx, prefix+"missing", makeInput(35, 21, "IN"))
	expect(e, domain.ErrIneligible)
	// Concurrent retry transactions yield one permanent request/event.
	concurrent := makeInput(40, 21, "IN")
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	ids := make(chan string, 4)
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); r, e := s.Submit(ctx, uid, concurrent); errs <- e; ids <- r.LateAccessID }()
	}
	wg.Wait()
	close(errs)
	close(ids)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if first != id {
			t.Fatal("concurrent duplicate")
		}
	}
	if got := scalar(`SELECT count(*)::text FROM hostelhive.notification_events WHERE late_access_id=$1`, first); got != "1" {
		t.Fatal("duplicate event", got)
	}
	// Conflicting concurrent final decisions yield one success and one conflict.
	raceRequest, e := s.Submit(ctx, uid, makeInput(41, 21, "IN"))
	if e != nil {
		t.Fatal(e)
	}
	decisions := make(chan error, 2)
	for _, status := range []string{"approved", "rejected"} {
		wg.Add(1)
		go func(status string) {
			defer wg.Done()
			_, e := s.Decide(ctx, auid, raceRequest.LateAccessID, dto.Decision{Status: status, Note: "Staff decision"})
			decisions <- e
		}(status)
	}
	wg.Wait()
	close(decisions)
	success, conflict := 0, 0
	for e := range decisions {
		if e == nil {
			success++
		} else if errors.Is(e, domain.ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("competing decisions", success, conflict)
	}
	// Expired original payload retries return history rather than new requests.
	expiredIn := makeInput(42, 21, "IN")
	expired, e := s.Submit(ctx, uid, expiredIn)
	if e != nil {
		t.Fatal(e)
	}
	expiredIn.ExpectedAt = expiredIn.ExpectedAt.AddDate(0, 0, -100)
	exec(`UPDATE hostelhive.late_access_requests SET expected_at=$2 WHERE late_access_id=$1`, expired.LateAccessID, expiredIn.ExpectedAt)
	if retried, e := s.Submit(ctx, uid, expiredIn); e != nil || retried.LateAccessID != expired.LateAccessID {
		t.Fatal("expired retry", retried, e)
	}
	if scalar(`SELECT count(*)::text FROM hostelhive.notification_events WHERE late_access_id=$1`, raceRequest.LateAccessID) != "2" {
		t.Fatal("decision event duplication")
	}
	// No request/decision creates attendance when presence is initially unknown.
	if scalar(`SELECT ((SELECT count(*) FROM hostelhive.student_presence WHERE student_id=$1)+(SELECT count(*) FROM hostelhive.attendance_events WHERE student_id=$1))::text`, sid) != "0" {
		t.Fatal("permission manufactured attendance")
	}
	mux := http.NewServeMux()
	lateaccess.Register(handler.New(s, 3*time.Second), mux, middleware.Middleware(verifier{}, identityrepo.NewPostgresAccounts(pool), 3*time.Second))
	call := func(who, method, path, body, mime string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "/api/v1/late-access-requests"+path, strings.NewReader(body))
		if who != "" {
			r.Header.Set("Authorization", "Bearer "+who)
		}
		r.Header.Set("Content-Type", mime)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s wanted %d got %d %s", method, path, want, w.Code, w.Body.String())
		}
		return w
	}
	apiIn := makeInput(50, 5, "OUT")
	body, _ := json.Marshal(apiIn)
	response := call(uid, "POST", "", string(body), "application/json", 201)
	var api domain.Request
	if e = json.Unmarshal(response.Body.Bytes(), &api); e != nil {
		t.Fatal(e)
	}
	call(uid, "POST", "", string(body), "application/json", 201)
	call(auid, "POST", "/"+api.LateAccessID+"/decision", `{"status":"approved"}`, "application/json", 200)
	call(uid, "GET", "/"+api.LateAccessID, "", "", 200)
	call(wuid, "GET", "?student_id="+sid+"&limit=1", "", "", 200)
	call(prefix+"other", "GET", "/"+api.LateAccessID, "", "", 404)
	call(uid, "GET", "?student_id="+otherSID, "", "", 403)
	call(uid, "POST", "/"+api.LateAccessID+"/decision", `{"status":"approved"}`, "application/json", 403)
	call("", "GET", "", "", "", 401)
	for _, who := range []string{"sub", "security"} {
		call(prefix+who, "GET", "", "", "", 403)
	}
	call(wuid, "POST", "", string(body), "application/json", 403)
	for _, q := range []string{"status=bad", "limit=0", "limit=101", "offset=-1", "limit=1&limit=2", "unknown=1", "student_id=bad", "status="} {
		call(uid, "GET", "?"+q, "", "", 400)
	}
	for _, bad := range []string{`{}`, string(body) + ` {}`, `{"student_id":"forged"}`, `{"expected_at":"invalid"}`} {
		call(uid, "POST", "", bad, "application/json", 400)
	}
	for _, change := range []string{"direction", "request_id", "reason", "long_reason", "past", "zero"} {
		bad := apiIn
		bad.RequestID = uuid()
		switch change {
		case "direction":
			bad.Direction = "bad"
		case "request_id":
			bad.RequestID = "bad"
		case "reason":
			bad.Reason = " "
		case "long_reason":
			bad.Reason = strings.Repeat("x", 1001)
		case "past":
			bad.ExpectedAt = time.Now().Add(-time.Hour)
		case "zero":
			bad.ExpectedAt = time.Time{}
		}
		raw, _ := json.Marshal(bad)
		call(uid, "POST", "", string(raw), "application/json", 400)
	}
	call(uid, "POST", "", string(body), "text/plain", 415)
	call(uid, "POST", "", strings.Repeat(" ", 17000)+`{}`, "application/json", 413)
	call(uid, "GET", "", "{}", "", 400)
	call(uid, "GET", "/bad", "", "", 400)
	call(uid, "GET", "/"+uuid(), "", "", 404)
	call(uid, "GET", "/"+api.LateAccessID+"?unknown=1", "", "", 400)
	call(auid, "POST", "/"+api.LateAccessID+"/decision", `{"status":"rejected"}`, "application/json", 400)
	// A recorded IN state and RTDB outbox version survive approval unchanged.
	device := scalar(`INSERT INTO hostelhive.scanner_devices(device_id,name,credential_hash,created_by,updated_by,direction) VALUES(gen_random_uuid(),$1,decode(repeat('01',32),'hex'),$2,$2,'IN') RETURNING device_id::text`, prefix+"device", admin)
	event := uuid()
	exec(`INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at) VALUES($1,$2,$3,decode(repeat('02',32),'hex'),'IN','recorded',clock_timestamp())`, event, device, sid)
	exec(`INSERT INTO hostelhive.student_presence(student_id,status,updated_at,device_id,event_id) VALUES($1,'IN',clock_timestamp(),$2,$3)`, sid, device, event)
	before := scalar(`SELECT version::text FROM hostelhive.attendance_publication WHERE singleton`)
	preserve, e := s.Submit(ctx, uid, makeInput(60, 21, "IN"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Decide(ctx, auid, preserve.LateAccessID, dto.Decision{Status: "approved"}); e != nil {
		t.Fatal(e)
	}
	if scalar(`SELECT status FROM hostelhive.student_presence WHERE student_id=$1`, sid) != "IN" || scalar(`SELECT version::text FROM hostelhive.attendance_publication WHERE singleton`) != before {
		t.Fatal("approval changed attendance or queued RTDB update")
	}
	exec(`UPDATE hostelhive.users SET role='security_staff' WHERE firebase_uid=$1`, wuid)
	_, e = s.Get(ctx, wuid, v.LateAccessID)
	expect(e, domain.ErrForbidden)
	deniedPage, e := inbox.List(ctx, prefix+"sub", ndto.Filter{Limit: 100})
	if e != nil {
		t.Fatal(e)
	}
	for _, n := range deniedPage.Items {
		if n.SourceType == "late_access" {
			t.Fatal("sub-warden received late-access alert")
		}
	}
	cancelled, c := context.WithCancel(ctx)
	c()
	_, e = s.List(cancelled, auid, domain.Filter{Limit: 20})
	expect(e, domain.ErrUnavailable)
}
