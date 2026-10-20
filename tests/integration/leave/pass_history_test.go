package leave_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCombinedPassHistory(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL migrated to28")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := "passes103-" + uuid.NewString() + "-"
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
	users, students := map[string]string{}, map[string]string{}
	for _, name := range []string{"admin", "student", "other", "empty", "missing"} {
		role := "student"
		if name == "admin" {
			role = "admin"
		}
		users[name] = scalar(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text`, prefix+name, prefix+name+"@example.invalid", role)
		if name != "admin" && name != "missing" {
			students[name] = scalar(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'History fixture','Science',1,'0771234567') RETURNING student_id::text`, users[name], prefix+name)
		}
	}
	defer func() {
		clean, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		for _, sql := range []string{
			`DELETE FROM hostelhive.student_invitations WHERE email LIKE $1`,
			`DELETE FROM hostelhive.going_home_declarations WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.late_access_requests WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.leave_requests WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.student_presence WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.attendance_events WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.scanner_devices WHERE name LIKE $1`,
			`DELETE FROM hostelhive.students WHERE index_no LIKE $1`,
			`DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`,
		} {
			if _, err := pool.Exec(clean, sql, prefix+"%"); err != nil {
				t.Error(err)
			}
		}
	}()
	seed := func(kind, name, status, id string, created time.Time) {
		t.Helper()
		table, key := "leave_requests", "leave_id"
		if kind == "leave" {
			exec(`INSERT INTO hostelhive.leave_requests(leave_id,request_id,student_id,submitted_by,starts_at,ends_at,reason,created_at) VALUES($1,$2,$3,$4,$5,$6,'Family visit',$7)`, id, uuid.NewString(), students[name], users[name], created.Add(24*time.Hour), created.Add(48*time.Hour), created)
		} else {
			table, key = "late_access_requests", "late_access_id"
			exec(`INSERT INTO hostelhive.late_access_requests(late_access_id,request_id,student_id,submitted_by,direction,expected_at,reason,created_at) VALUES($1,$2,$3,$4,'IN',$5,'Late return',$6)`, id, uuid.NewString(), students[name], users[name], created.Add(48*time.Hour), created)
		}
		if status == "cancelled" {
			exec(`UPDATE hostelhive.`+table+` SET status='cancelled',cancelled_at=clock_timestamp(),cancelled_by=$2 WHERE `+key+`=$1`, id, users[name])
		} else if status != "pending" {
			exec(`UPDATE hostelhive.`+table+` SET status=$2,decided_at=clock_timestamp(),decided_by=$3,decision_note='Recorded decision' WHERE `+key+`=$1`, id, status, users["admin"])
		}
	}
	timestamp := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	// Equal timestamps AND identical source UUIDs across tables require both tie-breakers.
	for i, status := range []string{"pending", "approved", "rejected", "cancelled"} {
		id := fmt.Sprintf("10000000-0000-4000-8000-%012d", i+1)
		seed("leave", "student", status, id, timestamp)
		seed("late_access", "student", status, id, timestamp.Add(time.Duration(i)*time.Microsecond))
	}
	seed("leave", "other", "pending", uuid.NewString(), timestamp.Add(time.Hour))
	seed("late_access", "other", "cancelled", uuid.NewString(), timestamp.Add(time.Hour))
	exec(`INSERT INTO hostelhive.going_home_declarations(request_id,student_id,submitted_by) VALUES($1,$2,$3)`, uuid.NewString(), students["student"], users["student"])
	device := scalar(`INSERT INTO hostelhive.scanner_devices(device_id,name,credential_hash,direction,created_by,updated_by) VALUES(gen_random_uuid(),$1,decode(repeat('00',32),'hex'),'IN',$2,$2) RETURNING device_id::text`, prefix+"gate", users["admin"])
	event := scalar(`INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at,access_result) VALUES($1,$2,$3,decode(repeat('00',32),'hex'),'IN','recorded',clock_timestamp(),'not_required') RETURNING event_id::text`, uuid.NewString(), device, students["student"])
	exec(`INSERT INTO hostelhive.student_presence(student_id,status,updated_at,device_id,event_id) SELECT student_id,'IN',occurred_at,device_id,event_id FROM hostelhive.attendance_events WHERE device_id=$1 AND event_id=$2`, device, event)
	exec(`UPDATE hostelhive.late_access_requests SET consumed_at=clock_timestamp(),consumed_device_id=$1,consumed_event_id=$2 WHERE late_access_id='10000000-0000-4000-8000-000000000002'`, device, event)
	store := repository.New(pool)
	mux := http.NewServeMux()
	leave.RegisterPassHistory(handler.NewPassHistory(service.NewPassHistory(store), 5*time.Second), mux, middleware.Middleware(cancelVerifier{}, identityrepo.NewPostgresAccounts(pool), 5*time.Second))
	call := func(query, name string, want int) domain.PassPage {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/v1/me/passes"+query, nil)
		if name != "" {
			r.Header.Set("Authorization", "Bearer "+prefix+name)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: want%d got%d %s", name, query, want, w.Code, w.Body)
		}
		var page domain.PassPage
		if want == 200 && (json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Items == nil || w.Header().Get("Cache-Control") != "no-store") {
			t.Fatal(w.Body, w.Header())
		}
		return page
	}
	snapshot := func() string {
		return scalar(`SELECT jsonb_build_object(
 'leaves',(SELECT jsonb_agg(to_jsonb(v) ORDER BY leave_id) FROM hostelhive.leave_requests v),
 'late_access',(SELECT jsonb_agg(to_jsonb(v) ORDER BY late_access_id) FROM hostelhive.late_access_requests v),
 'going_home',(SELECT jsonb_agg(to_jsonb(v) ORDER BY going_home_id) FROM hostelhive.going_home_declarations v),
 'presence',(SELECT jsonb_agg(to_jsonb(v) ORDER BY student_id) FROM hostelhive.student_presence v),
 'attendance',(SELECT jsonb_agg(to_jsonb(v) ORDER BY device_id,event_id) FROM hostelhive.attendance_events v),
 'events',(SELECT jsonb_agg(to_jsonb(v) ORDER BY event_id) FROM hostelhive.notification_events v),
 'outbox',(SELECT jsonb_agg(to_jsonb(v) ORDER BY event_id,recipient_id) FROM hostelhive.notification_outbox v),
 'publication',(SELECT to_jsonb(v) FROM hostelhive.attendance_publication v))::text`)
	}
	before := snapshot()
	all := call("", "student", 200)
	if all.Total != 8 || len(all.Items) != 8 || all.HasMore || all.Limit != 20 || all.Offset != 0 {
		t.Fatal(all)
	}
	less := func(a, b domain.PassItem) bool {
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.After(b.CreatedAt)
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.SourceID > b.SourceID
	}
	if !sort.SliceIsSorted(all.Items, func(i, j int) bool { return less(all.Items[i], all.Items[j]) }) {
		t.Fatal("combined order is incorrect", all)
	}
	for _, item := range all.Items {
		if item.StudentID != students["student"] || item.RequestID == "" {
			t.Fatal("ownership or source details lost", item)
		}
		if item.Type == "leave" {
			if item.StartsAt == nil || item.EndsAt == nil || item.Direction != nil || item.ExpectedAt != nil || item.ConsumedAt != nil || item.Reason != "Family visit" {
				t.Fatal(item)
			}
		} else if item.Type == "late_access" {
			if item.StartsAt != nil || item.EndsAt != nil || item.Direction == nil || *item.Direction != "IN" || item.ExpectedAt == nil || item.Reason != "Late return" {
				t.Fatal(item)
			}
			if item.Status == "approved" && item.ConsumedAt == nil {
				t.Fatal("consumed approval disappeared", item)
			}
		} else {
			t.Fatal("unexpected request type", item)
		}
		if item.Status == "cancelled" && (item.CancelledAt == nil || item.CancelledBy == nil || *item.CancelledBy != users["student"] || item.DecidedAt != nil) {
			t.Fatal("cancellation audit missing", item)
		}
		if (item.Status == "approved" || item.Status == "rejected") && (item.DecidedAt == nil || item.DecidedBy == nil || *item.DecidedBy != users["admin"] || item.DecisionNote != "Recorded decision") {
			t.Fatal("decision audit missing", item)
		}
	}
	var gathered []domain.PassItem
	for offset := 0; offset < 8; offset += 3 {
		page := call(fmt.Sprintf("?limit=3&offset=%d", offset), "student", 200)
		if page.Total != 8 || page.Limit != 3 || page.Offset != offset || page.HasMore != (offset+len(page.Items) < 8) {
			t.Fatal(page)
		}
		gathered = append(gathered, page.Items...)
	}
	if !reflect.DeepEqual(gathered, all.Items) {
		t.Fatal("combined pagination skipped or duplicated requests")
	}
	for _, query := range []string{"?offset=8", "?offset=1000000"} {
		page := call(query, "student", 200)
		if page.Total != 8 || len(page.Items) != 0 || page.HasMore {
			t.Fatal(page)
		}
	}
	for _, kind := range []string{"leave", "late_access"} {
		page := call("?type="+kind, "student", 200)
		if page.Total != 4 {
			t.Fatal(page)
		}
		for _, item := range page.Items {
			if item.Type != kind {
				t.Fatal(item)
			}
		}
		for _, status := range []string{"pending", "approved", "rejected", "cancelled"} {
			page := call("?type="+kind+"&status="+status, "student", 200)
			if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Type != kind || page.Items[0].Status != status {
				t.Fatal(page)
			}
		}
	}
	if page := call("?status=cancelled", "student", 200); page.Total != 2 {
		t.Fatal(page)
	}
	if page := call("?status=rejected&type=leave", "other", 200); page.Total != 0 || len(page.Items) != 0 {
		t.Fatal(page)
	}
	other := call("", "other", 200)
	if other.Total != 2 {
		t.Fatal(other)
	}
	for _, item := range other.Items {
		if item.StudentID != students["other"] {
			t.Fatal("ownership leak", item)
		}
	}
	if page := call("", "empty", 200); page.Total != 0 || len(page.Items) != 0 || page.HasMore {
		t.Fatal(page)
	}
	call("?student_id="+students["student"], "other", 400)
	call("?user_id="+users["student"], "other", 400)
	call("", "admin", 403)
	call("", "missing", 409)
	call("", "", 401)
	if before != snapshot() {
		t.Fatal("history reads changed requests, attendance or notification work")
	}
	for _, actor := range []string{"admin", "unknown"} {
		if _, err := store.OwnPasses(ctx, prefix+actor, domain.PassFilter{Limit: 20}); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal(actor, err)
		}
	}
	exec(`UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, students["student"], users["admin"])
	call("", "student", 409)
	exec(`UPDATE hostelhive.students SET deleted_at=NULL,deleted_by=NULL WHERE student_id=$1`, students["student"])
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, users["student"])
	call("", "student", 403)
	if _, err := store.OwnPasses(ctx, prefix+"student", domain.PassFilter{Limit: 20}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal(err)
	}
	exec(`UPDATE hostelhive.users SET is_active=true WHERE user_id=$1`, users["student"])
	exec(`INSERT INTO hostelhive.student_invitations(request_id,user_id,student_id,email,created_by,request_ip,payload_hash) VALUES($1,$2,$3,$4,$5,'127.0.0.1',decode(repeat('00',32),'hex'))`, uuid.NewString(), users["student"], students["student"], prefix+"student@example.invalid", users["admin"])
	call("", "student", 403)
	if _, err := store.OwnPasses(ctx, prefix+"student", domain.PassFilter{Limit: 20}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal(err)
	}
	exec(`DELETE FROM hostelhive.student_invitations WHERE user_id=$1`, users["student"])
	// Concurrent inserts must never produce a total from a different snapshot.
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 30; i++ {
			_, err := pool.Exec(ctx, `INSERT INTO hostelhive.leave_requests(request_id,student_id,submitted_by,starts_at,ends_at,reason) VALUES($1,$2,$3,$4,$5,'Concurrent history fixture')`, uuid.NewString(), students["student"], users["student"], timestamp, timestamp.Add(time.Hour))
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for i := 0; i < 15; i++ {
		page := call("?limit=100", "student", 200)
		if page.Total != int64(len(page.Items)) || page.HasMore {
			t.Fatal("count/page snapshot mismatch", page)
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if page := call("?limit=100", "student", 200); page.Total != 38 || len(page.Items) != 38 {
		t.Fatal(page)
	}
	// History retains cancelled records without reinterpreting consumed as a status.
	if page := call("?status=cancelled", "student", 200); page.Total != 2 {
		t.Fatal(page)
	}
	if _, err := store.OwnPasses(ctx, prefix+"student", domain.PassFilter{Limit: 20, Status: "approved"}); err != nil {
		t.Fatal(err)
	}
}
