package inventory_test

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

	allocationdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/dto"
	allocationrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/repository"
	allocationservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/service"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type managementVerifier struct{}

func (managementVerifier) Verify(_ context.Context, token string) (string, error) { return token, nil }

func TestInventoryManagementLifecycle(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable database migrated to version 10")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	prefix := fmt.Sprintf("inventory-management-%d", time.Now().UnixNano())
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = prefix
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
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
	actor := prefix + "-admin"
	for _, role := range []string{"admin", "warden", "student", "sub_warden", "security_staff"} {
		uid := prefix + "-" + role
		insert(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text`, uid, uid+"@example.invalid", role)
	}
	s := service.New(repository.New(pool))
	mux := http.NewServeMux()
	inventory.Register(handler.New(s, 5*time.Second), mux, middleware.Middleware(managementVerifier{}, identityrepo.NewPostgresAccounts(pool), 5*time.Second))
	request := func(method, path string, body any, uid string, want int, out any) {
		t.Helper()
		raw, e := json.Marshal(body)
		if e != nil {
			t.Fatal(e)
		}
		r := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		if uid != "" {
			r.Header.Set("Authorization", "Bearer "+uid)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s got %d: %s", method, path, w.Code, w.Body.String())
		}
		if out != nil {
			if e = json.Unmarshal(w.Body.Bytes(), out); e != nil {
				t.Fatal(e)
			}
		}
	}
	var block, block2 domain.Block
	request("POST", "/blocks", dto.BlockDetails{Name: " " + prefix + " "}, actor, 201, &block)
	request("POST", "/blocks", dto.BlockDetails{Name: prefix + "-other"}, actor, 201, &block2)
	if block.Name != prefix || block.Capacity != 0 {
		t.Fatal("incorrect creation result")
	}
	request("POST", "/blocks", dto.BlockDetails{Name: strings.ToUpper(prefix)}, actor, 409, nil)
	request("PUT", "/blocks/"+block2.BlockID, dto.BlockDetails{Name: prefix}, actor, 409, nil)
	var room, room2 domain.Room
	request("POST", "/rooms", dto.CreateRoom{BlockID: strings.ToUpper(block.BlockID), RoomNo: " A101 "}, actor, 201, &room)
	request("POST", "/rooms", dto.CreateRoom{BlockID: block.BlockID, RoomNo: "A102"}, actor, 201, &room2)
	request("POST", "/rooms", dto.CreateRoom{BlockID: block.BlockID, RoomNo: "a101"}, actor, 409, nil)
	request("PUT", "/rooms/"+room2.RoomID, dto.RoomDetails{RoomNo: "a101"}, actor, 409, nil)
	// Identical numbers in another parent are valid.
	request("POST", "/rooms", dto.CreateRoom{BlockID: block2.BlockID, RoomNo: "A101"}, actor, 201, nil)
	var bed, bed2 domain.Bed
	request("POST", "/beds", dto.CreateBed{RoomID: room.RoomID, BedNo: " ONE "}, actor, 201, &bed)
	request("POST", "/beds", dto.CreateBed{RoomID: room.RoomID, BedNo: "TWO"}, actor, 201, &bed2)
	if !bed.Available || bed.BlockID != block.BlockID {
		t.Fatal("incorrect bed creation result")
	}
	request("POST", "/beds", dto.CreateBed{RoomID: room.RoomID, BedNo: "one"}, actor, 409, nil)
	request("PUT", "/beds/"+bed2.BedID, dto.BedDetails{BedNo: "one"}, actor, 409, nil)
	request("POST", "/beds", dto.CreateBed{RoomID: room2.RoomID, BedNo: "ONE"}, actor, 201, nil)
	const missing = "00000000-0000-0000-0000-000000000000"
	request("POST", "/rooms", dto.CreateRoom{BlockID: missing, RoomNo: "A"}, actor, 404, nil)
	request("POST", "/beds", dto.CreateBed{RoomID: missing, BedNo: "A"}, actor, 404, nil)
	request("PUT", "/blocks/"+missing, dto.BlockDetails{Name: "A"}, actor, 404, nil)
	request("PUT", "/rooms/"+missing, dto.RoomDetails{RoomNo: "A"}, actor, 404, nil)
	request("PUT", "/beds/"+missing, dto.BedDetails{BedNo: "A"}, actor, 404, nil)
	for _, role := range []string{"warden", "student", "sub_warden", "security_staff"} {
		request("POST", "/blocks", dto.BlockDetails{Name: prefix + "-denied"}, prefix+"-"+role, 403, nil)
	}
	request("POST", "/blocks", dto.BlockDetails{Name: prefix + "-anonymous"}, "", 401, nil)
	request("GET", "/blocks", nil, prefix+"-warden", 200, nil)
	// Both active and ended allocation records must be unchanged by renaming.
	studentUser := insert(`SELECT user_id::text FROM hostelhive.users WHERE firebase_uid=$1`, prefix+"-student")
	student := insert(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Inventory Test','Engineering',1,'0771234567') RETURNING student_id::text`, studentUser, prefix)
	as := allocationservice.New(allocationrepo.New(pool))
	a, e := as.Assign(ctx, actor, allocationdto.Assign{StudentID: student, BedID: bed.BedID})
	if e != nil {
		t.Fatal(e)
	}
	allocationJSON := func() string {
		t.Helper()
		var raw string
		if e := pool.QueryRow(ctx, `SELECT row_to_json(a)::text FROM hostelhive.bed_allocations a WHERE allocation_id=$1`, a.AllocationID).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		return raw
	}
	before := allocationJSON()
	request("PUT", "/blocks/"+block.BlockID, dto.BlockDetails{Name: prefix + "-renamed"}, actor, 200, &block)
	request("PUT", "/rooms/"+strings.ToUpper(room.RoomID), dto.RoomDetails{RoomNo: " B101 "}, actor, 200, &room)
	request("PUT", "/beds/"+strings.ToUpper(bed.BedID), dto.BedDetails{BedNo: " THREE "}, actor, 200, &bed)
	if bed.Available || bed.BedNo != "THREE" || room.BlockID != block.BlockID || bed.RoomID != room.RoomID || room.OccupiedBeds != 1 || block.OccupiedBeds != 1 || allocationJSON() != before {
		t.Fatal("rename changed allocation, hierarchy or occupancy")
	}
	request("PUT", "/rooms/"+room.RoomID, map[string]string{"room_no": "X", "block_id": block2.BlockID}, actor, 400, nil)
	request("PUT", "/beds/"+bed.BedID, map[string]string{"bed_no": "X", "room_id": room2.RoomID}, actor, 400, nil)
	if _, e = as.Revoke(ctx, actor, a.AllocationID); e != nil {
		t.Fatal(e)
	}
	before = allocationJSON()
	request("PUT", "/beds/"+bed.BedID, dto.BedDetails{BedNo: "FOUR"}, actor, 200, &bed)
	request("PUT", "/beds/"+bed.BedID, dto.BedDetails{BedNo: "FOUR"}, actor, 200, &bed)
	if !bed.Available || allocationJSON() != before {
		t.Fatal("rename changed ended history or same-value retry failed")
	}
	// Two competing creates for each resource must have exactly one winner.
	for _, kind := range []string{"block", "room", "bed"} {
		start := make(chan struct{})
		results := make(chan error, 2)
		for range 2 {
			go func() {
				<-start
				var e error
				switch kind {
				case "block":
					_, e = s.CreateBlock(ctx, actor, dto.BlockDetails{Name: prefix + "-race"})
				case "room":
					_, e = s.CreateRoom(ctx, actor, dto.CreateRoom{BlockID: block.BlockID, RoomNo: "RACE"})
				case "bed":
					_, e = s.CreateBed(ctx, actor, dto.CreateBed{RoomID: room.RoomID, BedNo: "RACE"})
				}
				results <- e
			}()
		}
		close(start)
		wins, conflicts := 0, 0
		for range 2 {
			e := <-results
			if e == nil {
				wins++
			} else if errors.Is(e, domain.ErrConflict) {
				conflicts++
			} else {
				t.Fatal(e)
			}
		}
		if wins != 1 || conflicts != 1 {
			t.Fatalf("%s race: %d winners, %d conflicts", kind, wins, conflicts)
		}
	}
	// A role change committed while a write is blocked must deny that write.
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(ctx, `LOCK TABLE hostelhive.users IN SHARE ROW EXCLUSIVE MODE`); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `UPDATE hostelhive.users SET role='warden' WHERE firebase_uid=$1`, actor); e != nil {
		t.Fatal(e)
	}
	result := make(chan error, 1)
	go func() { _, e := s.CreateBlock(ctx, actor, dto.BlockDetails{Name: prefix + "-role-race"}); result <- e }()
	waitUntil := time.Now().Add(5 * time.Second)
	waiting := false
	for time.Now().Before(waitUntil) {
		if e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock' AND query='LOCK TABLE hostelhive.users IN SHARE MODE')`, prefix).Scan(&waiting); e != nil {
			t.Fatal(e)
		}
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("inventory write did not wait for concurrent role change")
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = <-result; !errors.Is(e, domain.ErrForbidden) {
		t.Fatalf("stale Admin write accepted: %v", e)
	}
	exec(`UPDATE hostelhive.users SET role='admin',is_active=false WHERE firebase_uid=$1`, actor)
	_, e = s.CreateBlock(ctx, actor, dto.BlockDetails{Name: prefix + "-inactive"})
	if !errors.Is(e, domain.ErrForbidden) {
		t.Fatal("inactive Admin persistence write accepted")
	}
	request("POST", "/blocks", dto.BlockDetails{Name: prefix + "-inactive-http"}, actor, 403, nil)
	_, e = s.CreateBlock(ctx, prefix+"-missing", dto.BlockDetails{Name: prefix + "-missing-actor"})
	if !errors.Is(e, domain.ErrForbidden) {
		t.Fatal("missing actor persistence write accepted")
	}
	bad, c := context.WithCancel(ctx)
	c()
	_, e = s.CreateBlock(bad, actor, dto.BlockDetails{Name: prefix + "-canceled"})
	if !errors.Is(e, domain.ErrUnavailable) {
		t.Fatalf("canceled write: %v", e)
	}
}
