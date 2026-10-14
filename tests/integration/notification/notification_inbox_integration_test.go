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
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/workers"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log/slog"
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
func TestComplaintNotificationInbox(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL migrated to 18")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, e := pgxpool.New(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	prefix := fmt.Sprintf("notification-%d-", time.Now().UnixNano())
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	id := func(sql string, args ...any) string {
		t.Helper()
		var v string
		if e := pool.QueryRow(ctx, sql, args...).Scan(&v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	uuid := func() string { return id(`SELECT gen_random_uuid()::text`) }
	user := func(name, role string) string {
		return id(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text`, prefix+name, prefix+name+"@example.invalid", role)
	}
	admin := user("admin", "admin")
	warden := user("warden", "warden")
	sub := user("sub", "sub_warden")
	user("security", "security_staff")
	student := user("student", "student")
	other := user("other", "student")
	inactive := user("inactive", "warden")
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, inactive)
	sid := id(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Notification Student','Engineering',1,'0771234567') RETURNING student_id::text`, student, prefix+"student")
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		for _, sql := range []string{
			`DELETE FROM hostelhive.complaint_updates WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id=$1)`,
			`DELETE FROM hostelhive.complaints WHERE student_id=$1`,
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
	cs := complaintservice.New(complaintrepo.New(pool))
	store := repository.New(pool)
	s := service.New(store)
	filter := dto.Filter{Limit: 100}
	list := func(name string) domain.Page {
		t.Helper()
		p, e := s.List(ctx, prefix+name, filter)
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	count := func(complaint, recipient string) int {
		t.Helper()
		var n int
		if e := pool.QueryRow(ctx, `SELECT count(*) FROM hostelhive.notification_outbox o JOIN hostelhive.complaint_updates e ON e.update_id=o.event_id WHERE e.complaint_id=$1 AND o.recipient_id=$2`, complaint, recipient).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	submit := complaintdto.Submit{RequestID: uuid(), Category: "maintenance", Description: "Leaking tap"}
	m, e := cs.Submit(ctx, prefix+"student", submit)
	if e != nil {
		t.Fatal(e)
	}
	if count(m.ComplaintID, warden) != 1 || count(m.ComplaintID, sub) != 1 || count(m.ComplaintID, admin) != 0 || count(m.ComplaintID, student) != 0 || count(m.ComplaintID, inactive) != 0 {
		t.Fatal("wrong maintenance recipients")
	}
	if list("warden").Total != 0 {
		t.Fatal("notification delivered before worker")
	}
	if _, e = cs.Submit(ctx, prefix+"student", submit); e != nil || count(m.ComplaintID, warden) != 1 {
		t.Fatal("submission retry duplicated intent", e)
	}
	// A database inbox failure must leave the committed complaint and pending intent.
	exec(`ALTER TABLE hostelhive.notifications ADD CONSTRAINT notification_test_failure CHECK(recipient_id <> '` + warden + `'::uuid)`)
	_, deliveryError := store.Deliver(ctx)
	exec(`ALTER TABLE hostelhive.notifications DROP CONSTRAINT notification_test_failure`)
	if deliveryError == nil {
		t.Fatal("expected materialization failure")
	}
	if _, e = cs.Get(ctx, prefix+"student", m.ComplaintID); e != nil || list("warden").Total != 0 {
		t.Fatal("delivery failure affected complaint", e)
	}
	if _, e = store.Deliver(ctx); e != nil {
		t.Fatal(e)
	}
	p := list("warden")
	if p.Total != 1 || p.Items[0].Kind != "created" || p.Items[0].ComplaintID != m.ComplaintID {
		t.Fatal(p)
	}
	nid := p.Items[0].NotificationID
	unread, e := s.Unread(ctx, prefix+"warden")
	if e != nil || unread.Unread != 1 {
		t.Fatal(unread, e)
	}
	read, e := s.MarkRead(ctx, prefix+"warden", nid)
	if e != nil || read.ReadAt == nil {
		t.Fatal(read, e)
	}
	replay, e := s.MarkRead(ctx, prefix+"warden", nid)
	if e != nil || !replay.ReadAt.Equal(*read.ReadAt) {
		t.Fatal("read retry changed timestamp", e)
	}
	unread, e = s.Unread(ctx, prefix+"warden")
	if e != nil || unread.Unread != 0 {
		t.Fatal(unread, e)
	}
	only, e := s.List(ctx, prefix+"warden", dto.Filter{Limit: 20, UnreadOnly: true})
	if e != nil || only.Total != 0 || only.Items == nil {
		t.Fatal(only, e)
	}
	for _, name := range []string{"admin", "sub", "student", "other"} {
		if _, e = s.MarkRead(ctx, prefix+name, nid); !errors.Is(e, domain.ErrNotFound) {
			t.Fatal("cross-user marking", name, e)
		}
	}
	if _, e = s.MarkRead(ctx, prefix+"warden", "invalid"); !errors.Is(e, domain.ErrInvalid) {
		t.Fatal(e)
	}
	if _, e = s.List(ctx, prefix+"security", filter); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal(e)
	}
	// Notes generate no alert; status changes generate one owner alert, not staff alerts.
	note, e := cs.Change(ctx, prefix+"student", m.ComplaintID, "note", complaintdto.Change{RequestID: uuid(), ExpectedVersion: 1, Text: "Follow-up"})
	if e != nil {
		t.Fatal(e)
	}
	if count(m.ComplaintID, warden) != 1 {
		t.Fatal("note generated alert")
	}
	change := complaintdto.Change{RequestID: uuid(), ExpectedVersion: note.Version, Status: "in_progress", Text: "Handling"}
	status, e := cs.Change(ctx, prefix+"warden", m.ComplaintID, "status", change)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = cs.Change(ctx, prefix+"warden", m.ComplaintID, "status", change); e != nil || count(m.ComplaintID, student) != 1 {
		t.Fatal("status retry duplicated intent", e)
	}
	if _, e = store.Deliver(ctx); e != nil {
		t.Fatal(e)
	}
	own := list("student")
	if own.Total != 1 || own.Items[0].Status != "in_progress" || own.Items[0].Kind != "status" {
		t.Fatal(own)
	}
	if list("other").Total != 0 {
		t.Fatal("another Student sees owner inbox")
	}
	// Archived active Students retain read access to their historical alerts.
	exec(`UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, sid, admin)
	if list("student").Total != 1 {
		t.Fatal("archived history lost")
	}
	exec(`UPDATE hostelhive.students SET deleted_at=NULL,deleted_by=NULL WHERE student_id=$1`, sid)
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, student)
	if _, e = s.List(ctx, prefix+"student", filter); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal("inactive read", e)
	}
	// Owner intent persists even while inactive; inbox is available after activation.
	_, e = cs.Change(ctx, prefix+"warden", m.ComplaintID, "status", complaintdto.Change{RequestID: uuid(), ExpectedVersion: status.Version, Status: "resolved", Text: "Fixed"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = store.Deliver(ctx); e != nil {
		t.Fatal(e)
	}
	exec(`UPDATE hostelhive.users SET is_active=true WHERE user_id=$1`, student)
	if list("student").Total != 2 {
		t.Fatal("status alert lost")
	}
	discipline, e := cs.Submit(ctx, prefix+"student", complaintdto.Submit{RequestID: uuid(), Category: "disciplinary", Description: "Noise"})
	if e != nil {
		t.Fatal(e)
	}
	if count(discipline.ComplaintID, warden) != 1 || count(discipline.ComplaintID, sub) != 0 {
		t.Fatal("disciplinary recipient leak")
	}
	// Concurrent workers are safe and delivery retries never duplicate inbox entries.
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := store.Deliver(ctx); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if list("warden").Total != 2 || list("sub").Total != 1 {
		t.Fatal("duplicates or wrong recipient count")
	}
	page, e := s.List(ctx, prefix+"warden", dto.Filter{Limit: 1})
	if e != nil || page.Total != 2 || len(page.Items) != 1 || !page.HasMore {
		t.Fatal(page, e)
	}
	// Downgrading to Sub-Warden hides existing disciplinary alerts and their count.
	exec(`UPDATE hostelhive.users SET role='sub_warden' WHERE user_id=$1`, warden)
	if list("warden").Total != 1 {
		t.Fatal("downgrade leaked disciplinary alert")
	}
	wUnread, e := s.Unread(ctx, prefix+"warden")
	if e != nil || wUnread.Unread != 0 {
		t.Fatal(wUnread, e)
	}
	var disciplineNID string
	if pool.QueryRow(ctx, `SELECT n.notification_id::text FROM hostelhive.notifications n JOIN hostelhive.complaint_updates e ON e.update_id=n.event_id WHERE n.recipient_id=$1 AND e.complaint_id=$2`, warden, discipline.ComplaintID).Scan(&disciplineNID) != nil {
		t.Fatal("missing delivered disciplinary event")
	}
	if _, e = s.MarkRead(ctx, prefix+"warden", disciplineNID); !errors.Is(e, domain.ErrNotFound) {
		t.Fatal("hidden mark read", e)
	}
	exec(`UPDATE hostelhive.users SET role='student' WHERE user_id=$1`, warden)
	if list("warden").Total != 0 {
		t.Fatal("role change exposed staff history")
	}
	exec(`UPDATE hostelhive.users SET role='warden' WHERE user_id=$1`, warden)
	// HTTP ownership, strict parsing and empty-body mark-read through real middleware.
	mux := http.NewServeMux()
	auth := middleware.Middleware(verifier{}, identityrepo.NewPostgresAccounts(pool), time.Second)
	notification.Register(handler.New(s, time.Second), mux, auth)
	request := func(method, path, who, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "/api/v1/notifications"+path, strings.NewReader(body))
		if who != "" {
			r.Header.Set("Authorization", "Bearer "+prefix+who)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: want %d got %d %s", method, path, want, w.Code, w.Body.String())
		}
		return w
	}
	w := request("GET", "?limit=1", "warden", "", 200)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("cacheable inbox")
	}
	var public domain.Page
	if json.Unmarshal(w.Body.Bytes(), &public) != nil || public.Total != 2 {
		t.Fatal(w.Body.String())
	}
	request("GET", "/unread-count", "student", "", 200)
	request("POST", "/"+own.Items[0].NotificationID+"/read", "student", "", 200)
	request("POST", "/"+own.Items[0].NotificationID+"/read", "other", "", 404)
	request("POST", "/"+nid+"/read", "warden", "{}", 400)
	request("POST", "/invalid/read", "warden", "", 400)
	request("POST", "/"+nid+"/read?unknown=1", "warden", "", 400)
	request("GET", "", "", "", 401)
	request("GET", "", "security", "", 403)
	for _, q := range []string{"limit=0", "limit=101", "offset=-1", "unread=1", "limit=1&limit=2", "unknown=1", "recipient_id=" + other, "limit=bad", "offset=bad", "unread=true&unread=false", "x=%zz"} {
		request("GET", "?"+q, "student", "", 400)
	}
	request("GET", "/unread-count?unread=true", "student", "", 400)
	request("GET", "", "student", "x", 400)
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, student)
	request("GET", "", "student", "", 403)
	exec(`UPDATE hostelhive.users SET is_active=true WHERE user_id=$1`, student)
	// The background worker survives a restart and publishes durable pending events.
	_, e = cs.Submit(ctx, prefix+"student", complaintdto.Submit{RequestID: uuid(), Category: "maintenance", Description: "Worker recovery"})
	if e != nil {
		t.Fatal(e)
	}
	cancelled, c := context.WithCancel(ctx)
	c()
	if _, e = store.Deliver(cancelled); e == nil {
		t.Fatal("cancelled delivery succeeded")
	}
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		workers.RunNotificationDelivery(workerCtx, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	deadline := time.Now().Add(5 * time.Second)
	for list("warden").Total != 3 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	stop()
	<-done
	if list("warden").Total != 3 {
		t.Fatal("worker did not recover")
	}
}
