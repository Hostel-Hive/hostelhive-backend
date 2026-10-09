package inventory_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/service"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
	"time"
)

func TestInventoryPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable database migrated to version 7")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal("database connection failed")
	}
	defer pool.Close()
	prefix := fmt.Sprintf("inventory-%d", time.Now().UnixNano())
	var block, emptyBlock, room, emptyRoom, bed1, bed2, bed3, user1, user2, student1, student2, allocation string
	insert := func(sql string, args []any, id *string) {
		t.Helper()
		if e := pool.QueryRow(ctx, sql, args...).Scan(id); e != nil {
			t.Fatal(e)
		}
	}
	insert(`INSERT INTO hostelhive.blocks(name) VALUES($1) RETURNING block_id::text`, []any{prefix}, &block)
	defer func() {
		for _, sql := range []string{
			`DELETE FROM hostelhive.bed_allocations WHERE bed_id IN(SELECT b.bed_id FROM hostelhive.beds b JOIN hostelhive.rooms r USING(room_id) WHERE r.block_id IN(SELECT block_id FROM hostelhive.blocks WHERE name LIKE $1))`,
			`DELETE FROM hostelhive.beds WHERE room_id IN(SELECT room_id FROM hostelhive.rooms WHERE block_id IN(SELECT block_id FROM hostelhive.blocks WHERE name LIKE $1))`,
			`DELETE FROM hostelhive.rooms WHERE block_id IN(SELECT block_id FROM hostelhive.blocks WHERE name LIKE $1)`,
			`DELETE FROM hostelhive.blocks WHERE name LIKE $1`,
			`DELETE FROM hostelhive.students WHERE index_no LIKE $1`,
			`DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`,
		} {
			if _, e := pool.Exec(context.Background(), sql, prefix+"%"); e != nil {
				t.Error(e)
			}
		}
	}()
	insert(`INSERT INTO hostelhive.blocks(name) VALUES($1) RETURNING block_id::text`, []any{prefix + "-empty"}, &emptyBlock)
	insert(`INSERT INTO hostelhive.rooms(block_id,room_no) VALUES($1,'101') RETURNING room_id::text`, []any{block}, &room)
	insert(`INSERT INTO hostelhive.rooms(block_id,room_no) VALUES($1,'102') RETURNING room_id::text`, []any{block}, &emptyRoom)
	insert(`INSERT INTO hostelhive.beds(room_id,bed_no) VALUES($1,'1') RETURNING bed_id::text`, []any{room}, &bed1)
	insert(`INSERT INTO hostelhive.beds(room_id,bed_no) VALUES($1,'2') RETURNING bed_id::text`, []any{room}, &bed2)
	insert(`INSERT INTO hostelhive.beds(room_id,bed_no) VALUES($1,'1') RETURNING bed_id::text`, []any{emptyRoom}, &bed3)
	for i, pair := range []struct{ user, student *string }{{&user1, &student1}, {&user2, &student2}} {
		uid := fmt.Sprintf("%s-%d", prefix, i)
		insert(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,'student',true) RETURNING user_id::text`, []any{uid, uid + "@example.invalid"}, pair.user)
		insert(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Inventory Test','Engineering',1,'0771234567') RETURNING student_id::text`, []any{*pair.user, uid}, pair.student)
	}
	insert(`INSERT INTO hostelhive.bed_allocations(bed_id,student_id) VALUES($1,$2) RETURNING allocation_id::text`, []any{bed1, student1}, &allocation)
	expectConstraint := func(sql, code string, args ...any) {
		t.Helper()
		_, e := pool.Exec(ctx, sql, args...)
		var pg *pgconn.PgError
		if !errors.As(e, &pg) || pg.Code != code {
			t.Fatalf("expected %s, got %v", code, e)
		}
	}
	expectConstraint(`INSERT INTO hostelhive.bed_allocations(bed_id,student_id) VALUES($1,$2)`, "23505", bed1, student2)
	expectConstraint(`INSERT INTO hostelhive.bed_allocations(bed_id,student_id) VALUES($1,$2)`, "23505", bed2, student1)
	expectConstraint(`INSERT INTO hostelhive.beds(room_id,bed_no) VALUES($1,'1')`, "23505", room)
	expectConstraint(`INSERT INTO hostelhive.rooms(block_id,room_no) VALUES($1,'101')`, "23505", block)
	expectConstraint(`INSERT INTO hostelhive.blocks(name) VALUES(upper($1))`, "23505", prefix)
	expectConstraint(`UPDATE hostelhive.bed_allocations SET ended_at=started_at-interval '1 second' WHERE allocation_id=$1`, "23514", allocation)
	expectConstraint(`DELETE FROM hostelhive.beds WHERE bed_id=$1`, "23503", bed1)
	s := service.New(repository.New(pool))
	f := domain.Filter{Limit: 100}
	blocks, e := s.Blocks(ctx, f)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, b := range blocks.Items {
		if b.BlockID == block {
			found = true
			if b.Capacity != 3 || b.OccupiedBeds != 1 || b.AvailableBeds != 2 || b.RoomCount != 2 {
				t.Fatalf("block summary: %+v", b)
			}
		}
		if b.BlockID == emptyBlock && (b.Capacity != 0 || b.RoomCount != 0) {
			t.Fatal("empty block counted beds")
		}
	}
	if !found {
		t.Fatal("fixture block missing")
	}
	f.BlockID = block
	rooms, e := s.Rooms(ctx, f)
	if e != nil || rooms.Total != 2 {
		t.Fatalf("rooms: %+v %v", rooms, e)
	}
	for _, r := range rooms.Items {
		if r.RoomID == room && (r.Capacity != 2 || r.OccupiedBeds != 1 || r.AvailableBeds != 1) {
			t.Fatalf("room summary: %+v", r)
		}
	}
	f.RoomID = room
	yes := true
	f.Available = &yes
	beds, e := s.Beds(ctx, f)
	if e != nil || beds.Total != 1 || beds.Items[0].BedID != bed2 {
		t.Fatalf("available filter %+v %v", beds, e)
	}
	no := false
	f.Available = &no
	beds, e = s.Beds(ctx, f)
	if e != nil || beds.Total != 1 || beds.Items[0].BedID != bed1 {
		t.Fatalf("occupied filter %+v %v", beds, e)
	}
	f.Available = nil
	f.Limit = 1
	f.Offset = 99
	beds, e = s.Beds(ctx, f)
	if e != nil || beds.Total != 2 || len(beds.Items) != 0 || beds.Items == nil {
		t.Fatalf("empty page %+v %v", beds, e)
	}
	f.Offset = 0
	first, e := s.Beds(ctx, f)
	if e != nil {
		t.Fatal(e)
	}
	f.Offset = 1
	second, e := s.Beds(ctx, f)
	if e != nil || first.Items[0].BedID == second.Items[0].BedID {
		t.Fatal("unstable pagination")
	}
	f.BlockID = emptyBlock
	f.Offset = 0
	beds, e = s.Beds(ctx, f)
	if e != nil || beds.Total != 0 {
		t.Fatal("mismatched parent filters")
	}
	if _, e = pool.Exec(ctx, `UPDATE hostelhive.bed_allocations SET ended_at=clock_timestamp() WHERE allocation_id=$1`, allocation); e != nil {
		t.Fatal(e)
	}
	f = domain.Filter{Limit: 100, RoomID: room, Available: &yes}
	beds, e = s.Beds(ctx, f)
	if e != nil || beds.Total != 2 {
		t.Fatal("released bed still occupied")
	}
	insert(`INSERT INTO hostelhive.bed_allocations(bed_id,student_id) VALUES($1,$2) RETURNING allocation_id::text`, []any{bed1, student2}, &allocation)
	var history int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM hostelhive.bed_allocations WHERE bed_id=$1`, bed1).Scan(&history); e != nil || history != 2 {
		t.Fatal("allocation history lost")
	}
	// Competing inserts on a free bed cannot both succeed.
	var user3, student3 string
	insert(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,'student',true) RETURNING user_id::text`, []any{prefix + "-3", prefix + "-3@example.invalid"}, &user3)
	insert(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Test','Test',1,'0771234567') RETURNING student_id::text`, []any{user3, prefix + "-3"}, &student3)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, student := range []string{student1, student3} {
		go func(id string) {
			<-start
			_, e := pool.Exec(ctx, `INSERT INTO hostelhive.bed_allocations(bed_id,student_id) VALUES($1,$2)`, bed2, id)
			results <- e
		}(student)
	}
	close(start)
	success := 0
	for range 2 {
		e := <-results
		if e == nil {
			success++
		} else {
			var pg *pgconn.PgError
			if !errors.As(e, &pg) || pg.Code != "23505" {
				t.Fatal(e)
			}
		}
	}
	if success != 1 {
		t.Fatal("competing allocations accepted")
	}
	cancelled, c := context.WithCancel(ctx)
	c()
	if _, e = s.Rooms(cancelled, domain.Filter{Limit: 20}); e == nil {
		t.Fatal("cancelled query succeeded")
	}
	// Invalid filters fail before any SQL query, including injection strings.
	if _, e = s.Rooms(ctx, domain.Filter{Limit: 20, BlockID: "';DROP TABLE hostelhive.rooms;--"}); !errors.Is(e, domain.ErrInvalid) {
		t.Fatal("invalid UUID accepted")
	}

}
