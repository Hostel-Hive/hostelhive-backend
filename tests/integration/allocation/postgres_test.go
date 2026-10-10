package allocation_test

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

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/service"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	inventorydomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	inventoryrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type verifier struct{ uid string }

func (v verifier) Verify(context.Context, string) (string, error) { return v.uid, nil }
func TestAllocationLifecycleAndConcurrency(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable database migrated to version 10")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := fmt.Sprintf("allocation-%d", time.Now().UnixNano())
	actor := prefix + "-admin"
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
	user := func(uid, role string) string {
		return insert(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text`, uid, uid+"@example.invalid", role)
	}
	admin := user(actor, "admin")
	wardenUID := prefix + "-warden"
	user(wardenUID, "warden")
	block := insert(`INSERT INTO hostelhive.blocks(name) VALUES($1) RETURNING block_id::text`, prefix)
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		for _, sql := range []string{
			`DELETE FROM hostelhive.bed_allocations WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.beds WHERE room_id IN(SELECT room_id FROM hostelhive.rooms WHERE block_id IN(SELECT block_id FROM hostelhive.blocks WHERE name LIKE $1))`,
			`DELETE FROM hostelhive.rooms WHERE block_id IN(SELECT block_id FROM hostelhive.blocks WHERE name LIKE $1)`,
			`DELETE FROM hostelhive.blocks WHERE name LIKE $1`,
			`DELETE FROM hostelhive.students WHERE index_no LIKE $1`,
			`DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`,
		} {
			if _, e := pool.Exec(clean, sql, prefix+"%"); e != nil {
				t.Error(e)
			}
		}
	}()
	room := insert(`INSERT INTO hostelhive.rooms(block_id,room_no) VALUES($1,'101') RETURNING room_id::text`, block)
	beds := []string{}
	for i := range 6 {
		beds = append(beds, insert(`INSERT INTO hostelhive.beds(room_id,bed_no) VALUES($1,$2) RETURNING bed_id::text`, room, fmt.Sprint(i)))
	}
	students := []string{}
	users := []string{}
	for i := range 5 {
		uid := fmt.Sprintf("%s-student-%d", prefix, i)
		u := user(uid, "student")
		users = append(users, u)
		students = append(students, insert(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Allocation Test','Engineering',1,'0771234567') RETURNING student_id::text`, u, uid))
	}
	s := service.New(repository.New(pool))
	expect := func(e, want error) {
		t.Helper()
		if !errors.Is(e, want) {
			t.Fatalf("want %v, got %v", want, e)
		}
	}
	a, e := s.Assign(ctx, actor, dto.Assign{StudentID: students[0], BedID: beds[0]})
	if e != nil || a.StartedBy == nil || *a.StartedBy != admin {
		t.Fatalf("assign %+v %v", a, e)
	}
	_, e = s.Assign(ctx, actor, dto.Assign{StudentID: students[1], BedID: beds[0]})
	expect(e, domain.ErrConflict)
	_, e = s.Assign(ctx, actor, dto.Assign{StudentID: students[0], BedID: beds[1]})
	expect(e, domain.ErrConflict)
	occupied, e := s.Assign(ctx, wardenUID, dto.Assign{StudentID: students[1], BedID: beds[1]})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Transfer(ctx, actor, a.AllocationID, dto.Transfer{BedID: beds[1]})
	expect(e, domain.ErrConflict)
	_, e = s.Transfer(ctx, actor, a.AllocationID, dto.Transfer{BedID: beds[0]})
	expect(e, domain.ErrConflict)
	missing := "00000000-0000-4000-8000-000000000000"
	_, e = s.Transfer(ctx, actor, a.AllocationID, dto.Transfer{BedID: missing})
	expect(e, domain.ErrNotFound)
	p, e := s.List(ctx, actor, domain.Filter{StudentID: students[0], Limit: 20})
	if e != nil || p.Total != 1 || p.Items[0].EndedAt != nil {
		t.Fatal("failed transfer modified the old allocation", p, e)
	}
	next, e := s.Transfer(ctx, wardenUID, a.AllocationID, dto.Transfer{BedID: beds[2]})
	if e != nil || next.AllocationID == a.AllocationID {
		t.Fatal(next, e)
	}
	p, e = s.List(ctx, actor, domain.Filter{StudentID: students[0], Limit: 20})
	if e != nil || p.Total != 2 {
		t.Fatal(p, e)
	}
	old := p.Items[1]
	if old.EndedAt == nil || old.EndReason == nil || *old.EndReason != "transferred" || old.EndedBy == nil {
		t.Fatal("transfer history missing", old)
	}
	_, e = s.Revoke(ctx, actor, a.AllocationID)
	expect(e, domain.ErrConflict) // stale ID cannot revoke new allocation
	yes := true
	no := false
	availability, e := inventoryrepo.New(pool).Beds(ctx, inventorydomain.Filter{RoomID: room, Available: &yes, Limit: 100})
	if e != nil || availability.Total != 4 {
		t.Fatal("inventory not updated", availability, e)
	}
	ended, e := s.Revoke(ctx, actor, next.AllocationID)
	if e != nil || ended.EndedAt == nil || *ended.EndReason != "revoked" {
		t.Fatal(ended, e)
	}
	again, e := s.Revoke(ctx, actor, next.AllocationID)
	if e != nil || !again.EndedAt.Equal(*ended.EndedAt) {
		t.Fatal("revocation retry changed history", again, e)
	}
	p, e = s.List(ctx, actor, domain.Filter{StudentID: students[0], Active: &yes, Limit: 20})
	if e != nil || p.Total != 0 || p.Items == nil || len(p.Items) != 0 {
		t.Fatal(p, e)
	}
	p, e = s.List(ctx, actor, domain.Filter{StudentID: students[0], Active: &no, Limit: 1, Offset: 99})
	if e != nil || p.Total != 2 || len(p.Items) != 0 {
		t.Fatal("empty pagination lost total", p, e)
	}
	first, e := s.List(ctx, actor, domain.Filter{StudentID: students[0], Limit: 1})
	if e != nil {
		t.Fatal(e)
	}
	second, e := s.List(ctx, actor, domain.Filter{StudentID: students[0], Limit: 1, Offset: 1})
	if e != nil || first.Items[0].AllocationID == second.Items[0].AllocationID {
		t.Fatal("pagination repeated row")
	}
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, users[2])
	_, e = s.Assign(ctx, actor, dto.Assign{StudentID: students[2], BedID: beds[0]})
	expect(e, domain.ErrIneligible)
	exec(`UPDATE hostelhive.users SET is_active=true,role='warden' WHERE user_id=$1`, users[2])
	_, e = s.Assign(ctx, actor, dto.Assign{StudentID: students[2], BedID: beds[0]})
	expect(e, domain.ErrIneligible)
	exec(`UPDATE hostelhive.users SET role='student' WHERE user_id=$1`, users[2])
	exec(`UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, students[2], admin)
	_, e = s.Assign(ctx, actor, dto.Assign{StudentID: students[2], BedID: beds[0]})
	expect(e, domain.ErrIneligible)
	exec(`UPDATE hostelhive.students SET deleted_at=NULL,deleted_by=NULL WHERE student_id=$1`, students[2])
	// Deactivation preserves occupancy/history; explicit revocation remains possible.
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, users[1])
	_, e = s.Transfer(ctx, actor, occupied.AllocationID, dto.Transfer{BedID: beds[0]})
	expect(e, domain.ErrIneligible)
	_, e = s.Revoke(ctx, actor, occupied.AllocationID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Assign(ctx, "not-an-actor", dto.Assign{StudentID: students[0], BedID: beds[0]})
	expect(e, domain.ErrForbidden)
	_, e = s.List(ctx, prefix+"-student-0", domain.Filter{Limit: 20})
	expect(e, domain.ErrForbidden)
	_, e = s.Assign(ctx, actor, dto.Assign{StudentID: missing, BedID: beds[0]})
	expect(e, domain.ErrNotFound)
	_, e = s.Assign(ctx, actor, dto.Assign{StudentID: students[0], BedID: missing})
	expect(e, domain.ErrNotFound)
	_, e = s.Revoke(ctx, actor, missing)
	expect(e, domain.ErrNotFound)
	// Concurrent service requests, not just direct inserts, contend on a single bed.
	race := func(inputs []dto.Assign) {
		t.Helper()
		start := make(chan struct{})
		results := make(chan error, len(inputs))
		for _, in := range inputs {
			go func(v dto.Assign) { <-start; _, e := s.Assign(ctx, actor, v); results <- e }(in)
		}
		close(start)
		success := 0
		for range inputs {
			e := <-results
			if e == nil {
				success++
			} else {
				expect(e, domain.ErrConflict)
			}
		}
		if success != 1 {
			t.Fatal("concurrent assignments both succeeded")
		}
	}
	race([]dto.Assign{{StudentID: students[3], BedID: beds[3]}, {StudentID: students[4], BedID: beds[3]}})
	exec(`UPDATE hostelhive.bed_allocations SET ended_at=clock_timestamp() WHERE bed_id=$1 AND ended_at IS NULL`, beds[3])
	race([]dto.Assign{{StudentID: students[3], BedID: beds[4]}, {StudentID: students[3], BedID: beds[5]}})
	// Simultaneous transfers of one current allocation have one winner.
	current, e := s.List(ctx, actor, domain.Filter{StudentID: students[3], Active: &yes, Limit: 20})
	if e != nil || len(current.Items) != 1 {
		t.Fatal(current, e)
	}
	_, e = s.Transfer(ctx, actor, current.Items[0].AllocationID, dto.Transfer{BedID: strings.ToUpper(current.Items[0].BedID)})
	expect(e, domain.ErrConflict)
	start := make(chan struct{})
	results := make(chan struct {
		a   domain.Allocation
		err error
	}, 2)
	for _, bed := range []string{beds[0], beds[2]} {
		go func(target string) {
			<-start
			a, e := s.Transfer(ctx, actor, current.Items[0].AllocationID, dto.Transfer{BedID: target})
			results <- struct {
				a   domain.Allocation
				err error
			}{a, e}
		}(bed)
	}
	close(start)
	wins := 0
	var winning domain.Allocation
	for range 2 {
		result := <-results
		if result.err == nil {
			wins++
			winning = result.a
		} else {
			expect(result.err, domain.ErrConflict)
		}
	}
	if wins != 1 {
		t.Fatal("competing transfers must have one winner")
	}
	if _, e = s.Revoke(ctx, actor, winning.AllocationID); e != nil {
		t.Fatal(e)
	}
	// Full route/service/database chain for Admin and Warden, without real Firebase.
	for _, uid := range []string{actor, wardenUID} {
		mux := http.NewServeMux()
		allocation.Register(handler.New(s, time.Second), mux, middleware.Middleware(verifier{uid}, identityrepo.NewPostgresAccounts(pool), time.Second))
		r := httptest.NewRequest("GET", "/api/v1/allocations?student_id="+students[0], nil)
		r.Header.Set("Authorization", "Bearer fake-test-token")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "allocation_id") {
			t.Fatal(w.Code, w.Body.String())
		}
		body := fmt.Sprintf(`{"student_id":%q,"bed_id":%q}`, students[0], beds[0])
		r = httptest.NewRequest("POST", "/api/v1/allocations", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer fake-test-token")
		r.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
		var created domain.Allocation
		if e = json.Unmarshal(w.Body.Bytes(), &created); e != nil {
			t.Fatal(e)
		}
		r = httptest.NewRequest("POST", "/api/v1/allocations/"+created.AllocationID+"/transfer", strings.NewReader(fmt.Sprintf(`{"bed_id":%q}`, beds[2])))
		r.Header.Set("Authorization", "Bearer fake-test-token")
		r.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if e = json.Unmarshal(w.Body.Bytes(), &created); e != nil {
			t.Fatal(e)
		}
		r = httptest.NewRequest("POST", "/api/v1/allocations/"+created.AllocationID+"/revoke", nil)
		r.Header.Set("Authorization", "Bearer fake-test-token")
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	cancelled, c := context.WithCancel(ctx)
	c()
	_, e = s.List(cancelled, actor, domain.Filter{Limit: 20})
	expect(e, domain.ErrUnavailable)
	exec(`UPDATE hostelhive.users SET role='student' WHERE user_id=$1`, admin)
	_, e = s.Revoke(ctx, actor, next.AllocationID)
	expect(e, domain.ErrForbidden)
}
