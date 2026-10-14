package complaint_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/service"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

type verifier struct{ uid string }

func (v verifier) Verify(context.Context, string) (string, error) { return v.uid, nil }
func TestComplaintWorkflow(t *testing.T) {
	db := os.Getenv("TEST_DATABASE_URL")
	if db == "" {
		t.Skip("requires disposable database migrated to 16")
	}
	ctx, c := context.WithTimeout(context.Background(), 90*time.Second)
	defer c()
	pool, e := pgxpool.New(ctx, db)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	prefix := fmt.Sprintf("complaint-%d", time.Now().UnixNano())
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
	rid := func() string { return insert(`SELECT gen_random_uuid()::text`) }
	user := func(suffix, role string) string {
		return insert(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text`, prefix+suffix, prefix+suffix+"@example.invalid", role)
	}
	admin := user("admin", "admin")
	user("warden", "warden")
	sub := user("sub", "sub_warden")
	user("security", "security_staff")
	studentUser := user("student", "student")
	otherUser := user("other", "student")
	user("missing", "student")
	student := func(u, suffix string) string {
		return insert(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Complaint Test','Engineering',1,'0771234567') RETURNING student_id::text`, u, prefix+suffix)
	}
	sid := student(studentUser, "student")
	otherSID := student(otherUser, "other")
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		for _, sql := range []string{
			`DELETE FROM hostelhive.complaint_updates WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1))`,
			`DELETE FROM hostelhive.complaints WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
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
	suid := prefix + "sub"
	expect := func(e, want error) {
		t.Helper()
		if !errors.Is(e, want) {
			t.Fatalf("want %v got %v", want, e)
		}
	}
	in := dto.Submit{RequestID: rid(), Category: "maintenance", Description: "Tap leaking"}
	v, e := s.Submit(ctx, uid, in)
	if e != nil || v.Version != 1 || v.Status != "pending" || v.StudentID != sid || v.SubmittedBy != studentUser {
		t.Fatal(v, e)
	}
	id := v.ComplaintID
	again, e := s.Submit(ctx, uid, in)
	if e != nil || again.ComplaintID != id {
		t.Fatal("duplicate retry", again, e)
	}
	changed := in
	changed.Description = "changed"
	_, e = s.Submit(ctx, uid, changed)
	expect(e, domain.ErrConflict)
	discipline, e := s.Submit(ctx, uid, dto.Submit{RequestID: rid(), Category: "disciplinary", Description: "Noise complaint"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Submit(ctx, auid, in)
	expect(e, domain.ErrForbidden)
	_, e = s.Submit(ctx, prefix+"missing", in)
	expect(e, domain.ErrIneligible)
	_, e = s.Get(ctx, prefix+"other", id)
	expect(e, domain.ErrNotFound)
	_, e = s.List(ctx, prefix+"other", domain.Filter{Limit: 20, StudentID: sid})
	expect(e, domain.ErrForbidden)
	p, e := s.List(ctx, prefix+"other", domain.Filter{Limit: 20})
	if e != nil || p.Total != 0 || p.Items == nil {
		t.Fatal(p, e)
	}
	p, e = s.List(ctx, suid, domain.Filter{Limit: 20})
	if e != nil || p.Total != 1 || p.Items[0].Category != "maintenance" {
		t.Fatal("Sub-Warden scope", p, e)
	}
	_, e = s.List(ctx, suid, domain.Filter{Limit: 20, Category: "disciplinary"})
	expect(e, domain.ErrForbidden)
	_, e = s.Get(ctx, suid, discipline.ComplaintID)
	expect(e, domain.ErrNotFound)
	_, e = s.Updates(ctx, suid, discipline.ComplaintID, 20, 0)
	expect(e, domain.ErrNotFound)
	_, e = s.Change(ctx, suid, discipline.ComplaintID, "note", dto.Change{RequestID: rid(), ExpectedVersion: 1, Text: "Unauthorized"})
	expect(e, domain.ErrNotFound)
	_, e = s.Change(ctx, suid, discipline.ComplaintID, "status", dto.Change{RequestID: rid(), ExpectedVersion: 1, Status: "resolved", Text: "Unauthorized"})
	expect(e, domain.ErrNotFound)
	if _, e = s.Get(ctx, wuid, discipline.ComplaintID); e != nil {
		t.Fatal(e)
	}
	_, e = s.Change(ctx, uid, id, "status", dto.Change{RequestID: rid(), ExpectedVersion: 1, Status: "in_progress"})
	expect(e, domain.ErrForbidden)
	note := dto.Change{RequestID: rid(), ExpectedVersion: 1, Text: "Water is leaking continuously"}
	n, e := s.Change(ctx, uid, id, "note", note)
	if e != nil || n.Version != 2 || n.ActorID != studentUser || n.Action != "note" {
		t.Fatal(n, e)
	}
	n2, e := s.Change(ctx, uid, id, "note", note)
	if e != nil || n.UpdateID != n2.UpdateID || !n.OccurredAt.Equal(n2.OccurredAt) {
		t.Fatal("retry changed audit", n2, e)
	}
	note.Text = "Changed"
	_, e = s.Change(ctx, uid, id, "note", note)
	expect(e, domain.ErrConflict)
	status := dto.Change{RequestID: rid(), ExpectedVersion: 2, Status: "in_progress", Text: "Plumber scheduled"}
	progress, e := s.Change(ctx, suid, id, "status", status)
	if e != nil || progress.Version != 3 || progress.ActorID != sub {
		t.Fatal(progress, e)
	}
	_, e = s.Change(ctx, auid, id, "note", dto.Change{RequestID: rid(), ExpectedVersion: 2, Text: "Stale"})
	expect(e, domain.ErrConflict)
	resolve := dto.Change{RequestID: rid(), ExpectedVersion: 3, Status: "resolved", Text: "Tap repaired"}
	resolved, e := s.Change(ctx, wuid, id, "status", resolve)
	if e != nil || resolved.Version != 4 || resolved.NewStatus != "resolved" {
		t.Fatal(resolved, e)
	}
	_, e = s.Change(ctx, uid, id, "note", dto.Change{RequestID: rid(), ExpectedVersion: 4, Text: "Still broken"})
	expect(e, domain.ErrConflict)
	// A retry from before resolution remains safe even after later updates.
	originalNote := dto.Change{RequestID: n.RequestID, ExpectedVersion: 1, Text: n.Text}
	if _, e = s.Change(ctx, uid, id, "note", originalNote); e != nil {
		t.Fatal(e)
	}
	_, e = s.Change(ctx, auid, id, "status", dto.Change{RequestID: rid(), ExpectedVersion: 4, Status: "in_progress"})
	expect(e, domain.ErrInvalid)
	reopen, e := s.Change(ctx, auid, id, "status", dto.Change{RequestID: rid(), ExpectedVersion: 4, Status: "in_progress", Text: "Leak returned"})
	if e != nil || reopen.Version != 5 || *reopen.PreviousStatus != "resolved" {
		t.Fatal(reopen, e)
	}
	history, e := s.Updates(ctx, uid, id, 100, 0)
	if e != nil || history.Total != 5 || history.Items[0].Action != "created" || history.Items[3].Text != "Tap repaired" {
		t.Fatal(history, e)
	}
	empty, e := s.Updates(ctx, uid, id, 1, 99)
	if e != nil || empty.Total != 5 || len(empty.Items) != 0 || empty.HasMore {
		t.Fatal(empty, e)
	}
	// Two workers editing the same revision cannot overwrite each other.
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, text := range []string{"Progress A", "Progress B"} {
		go func(text string) {
			<-start
			_, e := s.Change(ctx, wuid, id, "note", dto.Change{RequestID: rid(), ExpectedVersion: 5, Text: text})
			results <- e
		}(text)
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
		t.Fatal("concurrent revision winners", wins)
	}
	current, e := s.Get(ctx, uid, id)
	if e != nil || current.Version != 6 || current.Description != in.Description {
		t.Fatal(current, e)
	}
	// Concurrent creation retries must return one complaint and one creation event.
	submitIn := dto.Submit{RequestID: rid(), Category: "maintenance", Description: "Window broken"}
	started := make(chan struct{})
	ids := make(chan string, 2)
	errs := make(chan error, 2)
	for range 2 {
		go func() { <-started; c, e := s.Submit(ctx, uid, submitIn); ids <- c.ComplaintID; errs <- e }()
	}
	close(started)
	first, second := <-ids, <-ids
	for range 2 {
		if e := <-errs; e != nil {
			t.Fatal(e)
		}
	}
	if first != second {
		t.Fatal("duplicate concurrent submission")
	}
	creation, e := s.Updates(ctx, uid, first, 20, 0)
	if e != nil || creation.Total != 1 {
		t.Fatal(creation, e)
	}
	// Deactivated and archived accounts preserve history without accepting Student writes.
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, studentUser)
	_, e = s.Get(ctx, uid, id)
	expect(e, domain.ErrForbidden)
	if _, e = s.Get(ctx, auid, id); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Change(ctx, auid, id, "status", dto.Change{RequestID: rid(), ExpectedVersion: 6, Status: "resolved", Text: "Final repair"}); e != nil {
		t.Fatal(e)
	}
	exec(`UPDATE hostelhive.users SET is_active=true WHERE user_id=$1`, studentUser)
	exec(`UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, sid, admin)
	_, e = s.Submit(ctx, uid, dto.Submit{RequestID: rid(), Category: "maintenance", Description: "New"})
	expect(e, domain.ErrIneligible)
	_, e = s.Change(ctx, uid, first, "note", dto.Change{RequestID: rid(), ExpectedVersion: 1, Text: "New"})
	expect(e, domain.ErrIneligible)
	if _, e = s.Get(ctx, uid, id); e != nil {
		t.Fatal("archival erased history", e)
	}
	exec(`UPDATE hostelhive.students SET deleted_at=NULL,deleted_by=NULL WHERE student_id=$1`, sid)
	from := time.Now().Add(-time.Hour)
	to := time.Now().Add(time.Hour)
	p, e = s.List(ctx, uid, domain.Filter{Limit: 1, From: &from, To: &to})
	if e != nil || p.Total != 3 || len(p.Items) != 1 || !p.HasMore {
		t.Fatal(p, e)
	}
	p, e = s.List(ctx, uid, domain.Filter{Limit: 1, Offset: 99})
	if e != nil || p.Total != 3 || len(p.Items) != 0 || p.HasMore {
		t.Fatal(p, e)
	}
	// Exercise HTTP transport, middleware and repository together.
	call := func(actor, method, path, body, content string, want int) {
		t.Helper()
		mux := http.NewServeMux()
		complaint.Register(handler.New(s, 3*time.Second), mux, middleware.Middleware(verifier{actor}, identityrepo.NewPostgresAccounts(pool), 3*time.Second))
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if actor != "" {
			r.Header.Set("Authorization", "Bearer test")
		}
		r.Header.Set("Content-Type", content)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s expected %d got %d: %s", method, path, want, w.Code, w.Body.String())
		}
	}
	apiIn := dto.Submit{RequestID: rid(), Category: "maintenance", Description: "HTTP test"}
	b, _ := json.Marshal(apiIn)
	call(uid, "POST", "/api/v1/complaints", string(b), "application/json", 201)
	api, e := s.Submit(ctx, uid, apiIn)
	if e != nil {
		t.Fatal(e)
	}
	call(suid, "POST", "/api/v1/complaints/"+api.ComplaintID+"/status", fmt.Sprintf(`{"request_id":%q,"expected_version":1,"status":"in_progress","text":"Started"}`, rid()), "application/json", 200)
	call(uid, "POST", "/api/v1/complaints/"+api.ComplaintID+"/notes", fmt.Sprintf(`{"request_id":%q,"expected_version":2,"text":"More details"}`, rid()), "application/json", 200)
	call(uid, "GET", "/api/v1/complaints/"+api.ComplaintID, "", "application/json", 200)
	call(uid, "GET", "/api/v1/complaints/"+api.ComplaintID+"/updates?limit=1", "", "application/json", 200)
	call(wuid, "GET", "/api/v1/complaints?category=disciplinary&status=pending&student_id="+sid+"&created_from="+url.QueryEscape(from.Format(time.RFC3339))+"&created_to="+url.QueryEscape(to.Format(time.RFC3339)), "", "application/json", 200)
	call(uid, "POST", "/api/v1/complaints/"+id+"/status", `{}`, "application/json", 403)
	call(suid, "GET", "/api/v1/complaints/"+discipline.ComplaintID, "", "application/json", 404)
	call(suid, "GET", "/api/v1/complaints?category=disciplinary", "", "application/json", 403)
	call(prefix+"other", "GET", "/api/v1/complaints/"+id+"/updates", "", "application/json", 404)
	call(prefix+"other", "GET", "/api/v1/complaints?student_id="+sid, "", "application/json", 403)
	call(uid, "GET", "/api/v1/complaints?student_id="+otherSID, "", "application/json", 403)
	call("", "GET", "/api/v1/complaints", "", "application/json", 401)
	call(prefix+"security", "GET", "/api/v1/complaints", "", "application/json", 403)
	call(auid, "POST", "/api/v1/complaints", string(b), "application/json", 403)
	for _, q := range []string{"status=bad", "category=bad", "limit=0", "offset=-1", "limit=1&limit=2", "created_from=bad", "unknown=1"} {
		call(uid, "GET", "/api/v1/complaints?"+q, "", "application/json", 400)
	}
	call(uid, "GET", "/api/v1/complaints/"+id+"/updates?status=pending", "", "application/json", 400)
	call(uid, "GET", "/api/v1/complaints/bad", "", "application/json", 400)
	call(uid, "GET", "/api/v1/complaints/"+id, "{}", "application/json", 400)
	for _, body := range []string{`{}`, string(b) + ` {}`, `{"student_id":"forged"}`} {
		call(uid, "POST", "/api/v1/complaints", body, "application/json", 400)
	}
	call(uid, "POST", "/api/v1/complaints", "{}", "text/plain", 415)
	call(uid, "POST", "/api/v1/complaints", strings.Repeat(" ", 17000)+`{}`, "application/json", 413)
	call(uid, "POST", "/api/v1/complaints?uid=other", string(b), "application/json", 400)
	exec(`UPDATE hostelhive.users SET role='security_staff' WHERE user_id=$1`, sub)
	_, e = s.Get(ctx, suid, id)
	expect(e, domain.ErrForbidden)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, e = s.List(cancelled, auid, domain.Filter{Limit: 20})
	expect(e, domain.ErrUnavailable)
}
