package leave_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	adomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	adto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	arepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/repository"
	aservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/service"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess"
	ldomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/domain"
	lhandler "github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/handler"
	lrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/repository"
	lservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/service"
	rdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	rrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/repository"
	rservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type cancelVerifier struct{}

func (cancelVerifier) Verify(_ context.Context, token string) (string, error) { return token, nil }

type cancelledRequest struct {
	LeaveID      string     `json:"leave_id"`
	LateAccessID string     `json:"late_access_id"`
	RequestID    string     `json:"request_id"`
	Status       string     `json:"status"`
	CancelledAt  *time.Time `json:"cancelled_at"`
	CancelledBy  *string    `json:"cancelled_by"`
	DecidedAt    *time.Time `json:"decided_at"`
	DecidedBy    *string    `json:"decided_by"`
	ConsumedAt   *time.Time `json:"consumed_at"`
	DecisionNote string     `json:"decision_note"`
}

func (v cancelledRequest) id() string {
	if v.LeaveID != "" {
		return v.LeaveID
	}
	return v.LateAccessID
}

func TestPendingPassCancellation(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL migrated to28")
	}
	ctx, c := context.WithTimeout(context.Background(), 90*time.Second)
	defer c()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := "cancel101-" + uuid.NewString() + "-"
	exec := func(t *testing.T, sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	scalar := func(t *testing.T, sql string, args ...any) string {
		t.Helper()
		var v string
		if err := pool.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	users := map[string]string{}
	for _, role := range []string{"admin", "warden", "student", "other", "sub_warden", "security_staff", "missing"} {
		actual := role
		if role == "other" || role == "missing" {
			actual = "student"
		}
		users[role] = scalar(t, `INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text`, prefix+role, prefix+role+"@example.invalid", actual)
	}
	profiles := map[string]string{}
	for _, name := range []string{"student", "other"} {
		profiles[name] = scalar(t, `INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Cancellation test','Science',1,'0771234567') RETURNING student_id::text`, users[name], prefix+name)
	}
	sid := profiles["student"]
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		for _, sql := range []string{
			`UPDATE hostelhive.late_access_requests SET consumed_at=NULL,consumed_device_id=NULL,consumed_event_id=NULL WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.student_presence WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.attendance_events WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.scanner_device_audit WHERE device_id IN(SELECT device_id FROM hostelhive.scanner_devices WHERE name LIKE $1)`,
			`DELETE FROM hostelhive.scanner_devices WHERE name LIKE $1`,
			`DELETE FROM hostelhive.leave_requests WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.late_access_requests WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.students WHERE index_no LIKE $1`,
			`DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`,
		} {
			if _, err := pool.Exec(cleanup, sql, prefix+"%"); err != nil {
				t.Error(err)
			}
		}
	}()
	mux := http.NewServeMux()
	auth := middleware.Middleware(cancelVerifier{}, identityrepo.NewPostgresAccounts(pool), time.Second*5)
	leave.Register(handler.New(service.New(repository.New(pool)), time.Second*5), mux, auth)
	// IN is always restricted in this isolated fixture, independent of wall time.
	policy := ldomain.Policy{Location: time.UTC, ExitOpening: 0, EntryDeadline: 0}
	lateaccess.Register(lhandler.New(lservice.New(lrepo.New(pool, policy)), time.Second*5), mux, auth)
	request := func(method, path, body, who string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if who != "" {
			r.Header.Set("Authorization", "Bearer "+prefix+who)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	check := func(t *testing.T, w *httptest.ResponseRecorder, want int) cancelledRequest {
		t.Helper()
		if w.Code != want {
			t.Fatalf("want HTTP%d got%d: %s", want, w.Code, w.Body.String())
		}
		var v cancelledRequest
		_ = json.Unmarshal(w.Body.Bytes(), &v)
		return v
	}
	scans := arepo.NewDevicesWithPolicy(pool, policy)
	device, err := aservice.NewDevices(scans).Register(ctx, prefix+"admin", adto.RegisterDevice{Name: prefix + "gate"})
	if err != nil {
		t.Fatal(err)
	}
	qr := strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "")
	exec(t, `INSERT INTO hostelhive.student_qr(student_id,token) VALUES($1,$2)`, sid, qr)
	scan := func(direction string) (adomain.Event, error) {
		d, err := scans.SetDirection(ctx, prefix+"admin", device.Device.DeviceID, direction)
		if err != nil {
			return adomain.Event{}, err
		}
		return aservice.NewScans(scans, time.Nanosecond).Record(ctx, d, adto.Scan{EventID: uuid.NewString(), QR: "HH1:" + qr})
	}
	if _, err := scan("IN"); err != nil {
		t.Fatal(err)
	}
	snapshot := func(t *testing.T) string {
		return scalar(t, `SELECT jsonb_build_object('presence',(SELECT to_jsonb(p) FROM hostelhive.student_presence p WHERE student_id=$1),'events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY event_id) FROM hostelhive.attendance_events e WHERE student_id=$1),'publication',(SELECT to_jsonb(p) FROM hostelhive.attendance_publication p),'alerts',(SELECT jsonb_agg(to_jsonb(e) ORDER BY event_id) FROM hostelhive.notification_events e),'outbox',(SELECT jsonb_agg(to_jsonb(o) ORDER BY event_id,recipient_id) FROM hostelhive.notification_outbox o))::text`, sid)
	}
	for _, base := range []string{"/api/v1/leaves", "/api/v1/late-access-requests"} {
		t.Run(base, func(t *testing.T) {
			n := 0
			makeBody := func(at time.Time) string {
				n++
				if at.IsZero() {
					at = time.Now().UTC().Add(time.Duration(n+48) * time.Hour)
				}
				v := map[string]any{"request_id": uuid.NewString(), "reason": "Cancellation fixture"}
				if base == "/api/v1/leaves" {
					v["starts_at"] = at
					v["ends_at"] = at.Add(time.Minute * 30)
				} else {
					v["direction"] = "IN"
					v["expected_at"] = at
				}
				b, _ := json.Marshal(v)
				return string(b)
			}
			submit := func() (cancelledRequest, string) {
				b := makeBody(time.Time{})
				return check(t, request("POST", base, b, "student"), 201), b
			}
			v, body := submit()
			path := base + "/" + v.id()
			cancelURL := path + "/cancel"
			check(t, request("POST", cancelURL, "", "other"), 404)
			for _, who := range []string{"admin", "warden", "sub_warden", "security_staff"} {
				check(t, request("POST", cancelURL, "", who), 403)
			}
			check(t, request("POST", cancelURL, "", ""), 401)
			check(t, request("POST", cancelURL, "", "missing"), 409)
			for _, suffix := range []string{"?student_id=" + sid, "?unknown=1", "?x=1&x=2"} {
				check(t, request("POST", cancelURL+suffix, "", "student"), 400)
			}
			for _, bad := range []string{"{}", "null", " ", strings.Repeat("x", 20000)} {
				check(t, request("POST", cancelURL, bad, "student"), 400)
			}
			for _, method := range []string{"GET", "PUT", "DELETE"} {
				check(t, request(method, cancelURL, "", "student"), 405)
			}
			check(t, request("POST", base+"/bad/cancel", "", "student"), 400)
			check(t, request("POST", base+"/"+uuid.NewString()+"/cancel", "", "student"), 404)
			exec(t, `UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, sid, users["admin"])
			check(t, request("POST", cancelURL, "", "student"), 409)
			exec(t, `UPDATE hostelhive.students SET deleted_at=NULL,deleted_by=NULL WHERE student_id=$1`, sid)
			exec(t, `UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, users["student"])
			check(t, request("POST", cancelURL, "", "student"), 403)
			exec(t, `UPDATE hostelhive.users SET is_active=true WHERE user_id=$1`, users["student"])
			before := snapshot(t)
			v = check(t, request("POST", cancelURL, "", "student"), 200)
			if v.Status != "cancelled" || v.CancelledAt == nil || v.CancelledBy == nil || *v.CancelledBy != users["student"] || v.DecidedAt != nil || v.DecidedBy != nil || v.DecisionNote != "" {
				t.Fatal(v)
			}
			if before != snapshot(t) {
				t.Fatal("cancellation changed attendance/publication/notification intent")
			}
			for range 2 {
				again := check(t, request("POST", cancelURL, "", "student"), 200)
				if again.CancelledAt == nil || !again.CancelledAt.Equal(*v.CancelledAt) || *again.CancelledBy != *v.CancelledBy {
					t.Fatal("retry changed audit", again)
				}
			}
			replay := check(t, request("POST", base, body, "student"), 201)
			if replay.id() != v.id() || replay.Status != "cancelled" {
				t.Fatal("submission retry reopened request", replay)
			}
			detail := check(t, request("GET", path, "", "student"), 200)
			if detail.CancelledAt == nil || !detail.CancelledAt.Equal(*v.CancelledAt) {
				t.Fatal(detail)
			}
			check(t, request("POST", cancelURL, "", "other"), 404)
			var page struct {
				Items []cancelledRequest `json:"items"`
				Total int64              `json:"total"`
			}
			json.Unmarshal(checkPage(t, request("GET", base+"?status=cancelled", "", "student")), &page)
			if page.Total != 1 || len(page.Items) != 1 || page.Items[0].CancelledAt == nil {
				t.Fatal(page)
			}
			for _, status := range []string{"approved", "rejected"} {
				check(t, request("POST", path+"/decision", fmt.Sprintf(`{"status":%q,"note":"test decision"}`, status), "admin"), 409)
			}
			// A cancelled slot is reusable with a NEW operation UUID; the old one stays terminal.
			var replacement map[string]any
			json.Unmarshal([]byte(body), &replacement)
			replacement["request_id"] = uuid.NewString()
			b, _ := json.Marshal(replacement)
			replacementRow := check(t, request("POST", base, string(b), "student"), 201)
			check(t, request("POST", base+"/"+replacementRow.id()+"/cancel", "", "student"), 200)
			for _, status := range []string{"approved", "rejected"} {
				terminal, _ := submit()
				p := base + "/" + terminal.id()
				check(t, request("POST", p+"/decision", fmt.Sprintf(`{"status":%q,"note":"test decision"}`, status), "warden"), 200)
				before := request("GET", p, "", "student").Body.String()
				check(t, request("POST", p+"/cancel", "", "student"), 409)
				if request("GET", p, "", "student").Body.String() != before {
					t.Fatal("terminal request mutated")
				}
			}
			// Student row locking gives exactly one winner; no cancellation/decision hybrid.
			for i := 0; i < 6; i++ {
				v, _ := submit()
				p := base + "/" + v.id()
				results := make(chan *httptest.ResponseRecorder, 2)
				status := "approved"
				if i%2 == 1 {
					status = "rejected"
				}
				start := make(chan struct{})
				go func() { <-start; results <- request("POST", p+"/cancel", "", "student") }()
				go func() {
					<-start
					results <- request("POST", p+"/decision", fmt.Sprintf(`{"status":%q,"note":"race"}`, status), "admin")
				}()
				close(start)
				a, b := <-results, <-results
				if !((a.Code == 200 && b.Code == 409) || (a.Code == 409 && b.Code == 200)) {
					t.Fatal("race", a.Code, a.Body, b.Code, b.Body)
				}
				final := check(t, request("GET", p, "", "student"), 200)
				if final.Status == "cancelled" {
					if final.DecidedAt != nil || final.CancelledAt == nil {
						t.Fatal(final)
					}
				} else if final.CancelledAt != nil || final.DecidedAt == nil {
					t.Fatal(final)
				}
			}
			v, _ = submit()
			p := base + "/" + v.id() + "/cancel"
			var wg sync.WaitGroup
			responses := make(chan *httptest.ResponseRecorder, 8)
			for range 8 {
				wg.Add(1)
				go func() { defer wg.Done(); responses <- request("POST", p, "", "student") }()
			}
			wg.Wait()
			close(responses)
			var at *time.Time
			for w := range responses {
				v := check(t, w, 200)
				if at != nil && !at.Equal(*v.CancelledAt) {
					t.Fatal("concurrent retries changed timestamp")
				}
				at = v.CancelledAt
			}
		})
	}
	// A pending request racing a scan is not permission; cancellation cannot change the scan.
	at := time.Now().UTC().Add(2 * time.Minute)
	body := fmt.Sprintf(`{"request_id":%q,"direction":"IN","expected_at":%q,"reason":"Cancelled near scan"}`, uuid.NewString(), at.Format(time.RFC3339Nano))
	v := check(t, request("POST", "/api/v1/late-access-requests", body, "student"), 201)
	if _, err := scan("OUT"); err != nil {
		t.Fatal(err)
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- request("POST", "/api/v1/late-access-requests/"+v.id()+"/cancel", "", "student") }()
	event, err := scan("IN")
	check(t, <-done, 200)
	if err != nil || event.AccessResult != "unapproved" || event.LateAccessID != nil {
		t.Fatal(event, err)
	}
	final := check(t, request("GET", "/api/v1/late-access-requests/"+v.id(), "", "student"), 200)
	if final.Status != "cancelled" || final.ConsumedAt != nil {
		t.Fatal(final)
	}
	// Approved and consumed requests remain uncancellable.
	body = fmt.Sprintf(`{"request_id":%q,"direction":"IN","expected_at":%q,"reason":"Approved near scan"}`, uuid.NewString(), at.Format(time.RFC3339Nano))
	v = check(t, request("POST", "/api/v1/late-access-requests", body, "student"), 201)
	p := "/api/v1/late-access-requests/" + v.id()
	check(t, request("POST", p+"/decision", `{"status":"approved"}`, "admin"), 200)
	if _, err := scan("OUT"); err != nil {
		t.Fatal(err)
	}
	go func() { done <- request("POST", p+"/cancel", "", "student") }()
	event, err = scan("IN")
	check(t, <-done, 409)
	if err != nil || event.AccessResult != "approved" || event.LateAccessID == nil || *event.LateAccessID != v.id() {
		t.Fatal(event, err)
	}
	final = check(t, request("GET", p, "", "student"), 200)
	if final.ConsumedAt == nil || final.CancelledAt != nil || final.Status != "approved" {
		t.Fatal(final)
	}
	// Report totals include the new state, including archived history and empty pages.
	exec(t, `UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, sid, users["admin"])
	reports := rservice.NewLeaves(rrepo.NewLeaves(pool), time.UTC)
	today := time.Now().UTC().Format("2006-01-02")
	q := rdto.LeaveQuery{From: today, To: today, StudentID: sid, Limit: 100}
	all, err := reports.Requests(ctx, prefix+"admin", q)
	if err != nil || all.Summary.Cancelled < 1 || all.Total != all.Summary.Pending+all.Summary.Approved+all.Summary.Rejected+all.Summary.Cancelled {
		t.Fatal(all, err)
	}
	q.Status = "cancelled"
	q.Offset = 100
	empty, err := reports.Requests(ctx, prefix+"admin", q)
	if err != nil || empty.Total != all.Summary.Cancelled || empty.Summary.Cancelled != empty.Total || len(empty.Items) != 0 {
		t.Fatal(empty, err)
	}
	q.Offset = 0
	page, err := reports.Requests(ctx, prefix+"warden", q)
	if err != nil || len(page.Items) == 0 || page.Items[0].CancelledAt == nil || page.Items[0].CancelledBy == nil {
		t.Fatal(page, err)
	}
	// Downgrading must refuse rather than reopen real cancelled requests. Execute
	// the SQL directly so this assertion does not dirty migration CLI metadata.
	down, err := os.ReadFile("../../../migrations/000028_pass_cancellation.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, rollbackErr := conn.Exec(ctx, string(down))
	_, resetErr := conn.Exec(ctx, "ROLLBACK")
	conn.Release()
	if rollbackErr == nil || !strings.Contains(rollbackErr.Error(), "Cannot roll back pass cancellation") || resetErr != nil {
		t.Fatal(rollbackErr, resetErr)
	}
	if scalar(t, `SELECT count(*)::text FROM hostelhive.leave_requests WHERE student_id=$1 AND status='cancelled' AND cancelled_at IS NOT NULL`, sid) == "0" {
		t.Fatal("rollback lost cancellations")
	}
}
func checkPage(t *testing.T, w *httptest.ResponseRecorder) []byte {
	t.Helper()
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	return w.Body.Bytes()
}
