package notification_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	complaintdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/dto"
	complaintrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/repository"
	complaintservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/service"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	leavedomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	leavedto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/dto"
	leaverepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/repository"
	leaveservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/service"
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

func TestLeaveNotificationInbox(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL migrated to 19")
	}
	ctx, c := context.WithTimeout(context.Background(), 60*time.Second)
	defer c()
	pool, e := pgxpool.New(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	prefix := fmt.Sprintf("leave-notification-%d-", time.Now().UnixNano())
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	scalar := func(sql string, args ...any) string {
		t.Helper()
		var s string
		if e := pool.QueryRow(ctx, sql, args...).Scan(&s); e != nil {
			t.Fatal(e)
		}
		return s
	}
	uuid := func() string { return scalar(`SELECT gen_random_uuid()::text`) }
	user := func(name, role string) string {
		return scalar(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text`, prefix+name, prefix+name+"@example.invalid", role)
	}
	admin := user("admin", "admin")
	warden := user("warden", "warden")
	sub := user("sub", "sub_warden")
	user("security", "security_staff")
	student := user("student", "student")
	user("other", "student")
	inactive := user("inactive", "admin")
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, inactive)
	sid := scalar(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Leave Alert Student','Engineering',1,'0771234567') RETURNING student_id::text`, student, prefix+"student")
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		for _, sql := range []string{
			`DELETE FROM hostelhive.student_presence WHERE student_id=$1`,
			`DELETE FROM hostelhive.attendance_events WHERE student_id=$1`,
			`DELETE FROM hostelhive.scanner_devices WHERE name IN(SELECT index_no FROM hostelhive.students WHERE student_id=$1)`,
			`DELETE FROM hostelhive.complaint_updates WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id=$1)`,
			`DELETE FROM hostelhive.complaints WHERE student_id=$1`,
			`DELETE FROM hostelhive.leave_requests WHERE student_id=$1`,
			`DELETE FROM hostelhive.students WHERE student_id=$1`,
		} {
			if _, e := pool.Exec(clean, sql, sid); e != nil {
				t.Error(e)
			}
		}
		if _, e := pool.Exec(clean, `DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`, prefix+"%"); e != nil {
			t.Error(e)
		}
	}()
	device, event := uuid(), uuid()
	exec(`INSERT INTO hostelhive.scanner_devices(device_id,name,credential_hash,created_by,updated_by,direction) VALUES($1,$2,decode(repeat('ab',32),'hex'),$3,$3,'IN')`, device, prefix+"student", admin)
	exec(`INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at) VALUES($1,$2,$3,decode(repeat('cd',32),'hex'),'IN','recorded',clock_timestamp())`, event, device, sid)
	exec(`INSERT INTO hostelhive.student_presence(student_id,status,updated_at,device_id,event_id) VALUES($1,'IN',clock_timestamp(),$2,$3)`, sid, device, event)
	presenceBefore := scalar(`SELECT to_jsonb(p)::text FROM hostelhive.student_presence p WHERE student_id=$1`, sid)
	ls := leaveservice.New(leaverepo.New(pool))
	store := repository.New(pool)
	ns := service.New(store)
	input := func(hours int) leavedto.Submit {
		start := time.Now().UTC().Add(time.Duration(hours) * time.Hour).Truncate(time.Microsecond)
		return leavedto.Submit{RequestID: uuid(), StartsAt: start, EndsAt: start.Add(time.Hour), Reason: "Private leave reason"}
	}
	list := func(name string) domain.Page {
		t.Helper()
		p, e := ns.List(ctx, prefix+name, dto.Filter{Limit: 100})
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	count := func(lid, recipient string) int {
		t.Helper()
		var n int
		if e := pool.QueryRow(ctx, `SELECT count(*) FROM hostelhive.notification_outbox o JOIN hostelhive.notification_events e ON e.event_id=o.event_id WHERE e.leave_id=$1 AND o.recipient_id=$2`, lid, recipient).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	in := input(24)
	leave, e := ls.Submit(ctx, prefix+"student", in)
	if e != nil {
		t.Fatal(e)
	}
	if count(leave.LeaveID, admin) != 1 || count(leave.LeaveID, warden) != 1 || count(leave.LeaveID, sub) != 0 || count(leave.LeaveID, student) != 0 || count(leave.LeaveID, inactive) != 0 {
		t.Fatal("wrong submission recipients")
	}
	if _, e = ls.Submit(ctx, prefix+"student", in); e != nil || count(leave.LeaveID, admin) != 1 {
		t.Fatal("duplicate submission intent", e)
	}
	changed := in
	changed.Reason = "Changed"
	if _, e = ls.Submit(ctx, prefix+"student", changed); !errors.Is(e, leavedomain.ErrConflict) {
		t.Fatal(e)
	}
	exec(`ALTER TABLE hostelhive.notifications ADD CONSTRAINT leave_notification_test_failure CHECK(recipient_id <> '` + warden + `'::uuid)`)
	_, failed := store.Deliver(ctx)
	exec(`ALTER TABLE hostelhive.notifications DROP CONSTRAINT leave_notification_test_failure`)
	if failed == nil || list("admin").Total != 0 {
		t.Fatal("failed delivery committed")
	}
	if _, e = ls.Get(ctx, prefix+"student", leave.LeaveID); e != nil {
		t.Fatal("delivery failure undid leave", e)
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if _, e = store.Deliver(cancelled); e == nil {
		t.Fatal("cancelled delivery succeeded")
	}
	if _, e = store.Deliver(ctx); e != nil {
		t.Fatal(e)
	}
	p := list("admin")
	if p.Total != 1 || p.Items[0].SourceType != "leave" || p.Items[0].LeaveID != leave.LeaveID || p.Items[0].ComplaintID != "" || p.Items[0].Category != "" || p.Items[0].Kind != "leave_submitted" || p.Items[0].Status != "pending" {
		t.Fatal(p)
	}
	adminAlert := p.Items[0]
	wardenAlert := list("warden").Items[0]
	read, e := ns.MarkRead(ctx, prefix+"admin", adminAlert.NotificationID)
	if e != nil || read.ReadAt == nil {
		t.Fatal(read, e)
	}
	retry, e := ns.MarkRead(ctx, prefix+"admin", adminAlert.NotificationID)
	if e != nil || !retry.ReadAt.Equal(*read.ReadAt) {
		t.Fatal("mark retry", e)
	}
	unread, e := ns.Unread(ctx, prefix+"admin")
	if e != nil || unread.Unread != 0 {
		t.Fatal(unread, e)
	}
	for _, name := range []string{"warden", "sub", "student", "other"} {
		if _, e = ns.MarkRead(ctx, prefix+name, adminAlert.NotificationID); !errors.Is(e, domain.ErrNotFound) {
			t.Fatal("cross-owner mark", name, e)
		}
	}
	if _, e = ns.List(ctx, prefix+"security", dto.Filter{Limit: 20}); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal(e)
	}
	decision := leavedto.Decision{Status: "approved", Note: "Private approval note"}
	approved, e := ls.Decide(ctx, prefix+"warden", leave.LeaveID, decision)
	if e != nil {
		t.Fatal(e)
	}
	again, e := ls.Decide(ctx, prefix+"warden", leave.LeaveID, decision)
	if e != nil || !again.DecidedAt.Equal(*approved.DecidedAt) || count(leave.LeaveID, student) != 1 {
		t.Fatal("duplicate decision intent", e)
	}
	if _, e = ls.Decide(ctx, prefix+"admin", leave.LeaveID, leavedto.Decision{Status: "rejected", Note: "Changed decision"}); !errors.Is(e, leavedomain.ErrConflict) {
		t.Fatal(e)
	}
	if _, e = store.Deliver(ctx); e != nil {
		t.Fatal(e)
	}
	own := list("student")
	if own.Total != 1 || own.Items[0].Kind != "leave_decided" || own.Items[0].Status != "approved" || own.Items[0].Title != "Your leave request is approved" {
		t.Fatal(own)
	}
	if list("other").Total != 0 || list("sub").Total != 0 {
		t.Fatal("leave alert leaked")
	}
	// Account and profile archival preserve history, while inactive accounts are denied.
	in2 := input(48)
	rejection, e := ls.Submit(ctx, prefix+"student", in2)
	if e != nil {
		t.Fatal(e)
	}
	exec(`UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, sid, admin)
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, student)
	if _, e = ls.Decide(ctx, prefix+"admin", rejection.LeaveID, decision); !errors.Is(e, leavedomain.ErrIneligible) {
		t.Fatal("inactive approval", e)
	}
	rejected, e := ls.Decide(ctx, prefix+"admin", rejection.LeaveID, leavedto.Decision{Status: "rejected", Note: "Private rejection note"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ls.Decide(ctx, prefix+"admin", rejection.LeaveID, leavedto.Decision{Status: "rejected", Note: "Private rejection note"}); e != nil || count(rejection.LeaveID, student) != 1 {
		t.Fatal("rejection retry", e)
	}
	if _, e = store.Deliver(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = ns.List(ctx, prefix+"student", dto.Filter{Limit: 20}); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal("inactive inbox", e)
	}
	exec(`UPDATE hostelhive.users SET is_active=true WHERE user_id=$1`, student)
	if list("student").Total != 2 {
		t.Fatal("archived Student cannot read history")
	}
	exec(`UPDATE hostelhive.students SET deleted_at=NULL,deleted_by=NULL WHERE student_id=$1`, sid)
	// Changed role hides an old leave submission alert on list/count/mark-read.
	exec(`UPDATE hostelhive.users SET role='sub_warden' WHERE user_id=$1`, warden)
	if list("warden").Total != 0 {
		t.Fatal("downgrade leaked leave alert")
	}
	if _, e = ns.MarkRead(ctx, prefix+"warden", wardenAlert.NotificationID); !errors.Is(e, domain.ErrNotFound) {
		t.Fatal(e)
	}
	unread, e = ns.Unread(ctx, prefix+"warden")
	if e != nil || unread.Unread != 0 {
		t.Fatal(unread, e)
	}
	exec(`UPDATE hostelhive.users SET role='warden' WHERE user_id=$1`, warden)
	// Competing decisions generate exactly one final event and one owner intent.
	raceLeave, e := ls.Submit(ctx, prefix+"student", input(72))
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, e := ls.Decide(ctx, prefix+"admin", raceLeave.LeaveID, decision)
		results <- e
	}()
	go func() {
		defer wg.Done()
		_, e := ls.Decide(ctx, prefix+"warden", raceLeave.LeaveID, leavedto.Decision{Status: "rejected", Note: "Race rejection"})
		results <- e
	}()
	wg.Wait()
	close(results)
	wins := 0
	for e := range results {
		if e == nil {
			wins++
		} else if !errors.Is(e, leavedomain.ErrConflict) {
			t.Fatal(e)
		}
	}
	if wins != 1 || count(raceLeave.LeaveID, student) != 1 {
		t.Fatal("competing decision duplicated event")
	}
	errorsOut := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := store.Deliver(ctx); errorsOut <- e }()
	}
	wg.Wait()
	close(errorsOut)
	for e := range errorsOut {
		if e != nil {
			t.Fatal(e)
		}
	}
	if list("student").Total != 3 {
		t.Fatal("duplicate/missing owner delivery")
	}
	// Mixed inbox retains complaint routing, IDs and read state beside leave alerts.
	cs := complaintservice.New(complaintrepo.New(pool))
	comp, e := cs.Submit(ctx, prefix+"student", complaintdto.Submit{RequestID: uuid(), Category: "maintenance", Description: "Private complaint description"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = store.Deliver(ctx); e != nil {
		t.Fatal(e)
	}
	if list("sub").Total != 1 || list("admin").Total != 3 || list("warden").Total != 4 {
		t.Fatal("mixed inbox routing changed")
	}
	var complaintAlert domain.Notification
	for _, v := range list("warden").Items {
		if v.ComplaintID == comp.ComplaintID {
			complaintAlert = v
		}
	}
	if complaintAlert.SourceType != "complaint" || complaintAlert.LeaveID != "" {
		t.Fatal(complaintAlert)
	}
	mixed := list("admin")
	for _, v := range mixed.Items {
		if v.NotificationID == adminAlert.NotificationID && (v.ReadAt == nil || !v.ReadAt.Equal(*read.ReadAt)) {
			t.Fatal("mixed inbox lost read state")
		}
	}
	page, e := ns.List(ctx, prefix+"warden", dto.Filter{Limit: 1})
	if e != nil || page.Total != 4 || len(page.Items) != 1 || !page.HasMore {
		t.Fatal(page, e)
	}
	if scalar(`SELECT to_jsonb(p)::text FROM hostelhive.student_presence p WHERE student_id=$1`, sid) != presenceBefore {
		t.Fatal("leave changed attendance presence")
	}
	if scalar(`SELECT count(*)::text FROM hostelhive.attendance_events WHERE student_id=$1`, sid) != "1" {
		t.Fatal("leave added attendance event")
	}
	// HTTP mixed inbox uses the same endpoints and never includes leave reason/decision notes.
	mux := http.NewServeMux()
	auth := middleware.Middleware(verifier{}, identityrepo.NewPostgresAccounts(pool), time.Second)
	notification.Register(handler.New(ns, time.Second), mux, auth)
	req := func(method, path, who string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "/api/v1/notifications"+path, nil)
		if who != "" {
			r.Header.Set("Authorization", "Bearer "+prefix+who)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: want %d got %d %s", path, want, w.Code, w.Body.String())
		}
		return w
	}
	raw := req("GET", "?limit=100", "student", 200)
	for _, secret := range []string{"Private leave reason", "Private approval note", "Private rejection note", "recipient_id", "decided_by", "complaint_id", "category"} {
		if strings.Contains(raw.Body.String(), secret) {
			t.Fatal("private/unrelated field exposed", secret)
		}
	}
	var httpPage domain.Page
	if json.Unmarshal(raw.Body.Bytes(), &httpPage) != nil || httpPage.Total != 3 {
		t.Fatal(raw.Body.String())
	}
	var rejectedAlert domain.Notification
	for _, v := range httpPage.Items {
		if v.LeaveID == rejected.LeaveID {
			rejectedAlert = v
		}
	}
	if rejectedAlert.Status != "rejected" || rejectedAlert.Title != "Your leave request is rejected" {
		t.Fatal(rejectedAlert)
	}
	req("POST", "/"+rejectedAlert.NotificationID+"/read", "student", 200)
	req("POST", "/"+rejectedAlert.NotificationID+"/read", "other", 404)
	req("POST", "/"+rejectedAlert.NotificationID+"/read", "admin", 404)
	req("GET", "/unread-count", "student", 200)
	req("GET", "?unread=true", "student", 200)
	req("GET", "", "", 401)
	req("GET", "", "security", 403)
	// A promoted Student must not gain staff access to their old owner alerts.
	exec(`UPDATE hostelhive.users SET role='warden' WHERE user_id=$1`, student)
	if list("student").Total != 0 {
		t.Fatal("owner alerts visible under staff role")
	}
	if _, e = ns.MarkRead(ctx, prefix+"student", rejectedAlert.NotificationID); !errors.Is(e, domain.ErrNotFound) {
		t.Fatal(e)
	}
	exec(`UPDATE hostelhive.users SET role='student' WHERE user_id=$1`, student)
}
