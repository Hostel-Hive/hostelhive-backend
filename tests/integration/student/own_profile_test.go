package student_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	dto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/handler"
	repo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/repository"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestOwnStudentProfileLifecycle(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := fmt.Sprintf("own91-%d", time.Now().UnixNano())
	ids, uids := []string{}, []string{}
	var block, device string
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, q := range []string{
			"DELETE FROM hostelhive.student_presence WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE user_id=ANY($1::uuid[]))",
			"DELETE FROM hostelhive.attendance_events WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE user_id=ANY($1::uuid[]))",
			"DELETE FROM hostelhive.bed_allocations WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE user_id=ANY($1::uuid[]))",
			"UPDATE hostelhive.students SET profile_image_key=NULL WHERE user_id=ANY($1::uuid[])",
			"DELETE FROM hostelhive.student_image_objects WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE user_id=ANY($1::uuid[]))",
			"DELETE FROM hostelhive.guardians WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE user_id=ANY($1::uuid[]))",
			"DELETE FROM hostelhive.students WHERE user_id=ANY($1::uuid[])",
			"DELETE FROM hostelhive.scanner_devices WHERE created_by=ANY($1::uuid[])",
			"DELETE FROM hostelhive.users WHERE user_id=ANY($1::uuid[])",
		} {
			if _, e := pool.Exec(clean, q, ids); e != nil {
				t.Error("cleanup", e)
			}
		}
		if block != "" {
			for _, q := range []string{"DELETE FROM hostelhive.beds WHERE room_id IN(SELECT room_id FROM hostelhive.rooms WHERE block_id=$1)", "DELETE FROM hostelhive.rooms WHERE block_id=$1", "DELETE FROM hostelhive.blocks WHERE block_id=$1"} {
				if _, e := pool.Exec(clean, q, block); e != nil {
					t.Error(e)
				}
			}
		}
	}()
	for n, role := range []string{"admin", "student", "student", "student"} {
		uid := fmt.Sprintf("%s-%d", prefix, n)
		var id string
		if err := pool.QueryRow(ctx, "INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text", uid, uid+"@example.invalid", role).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		uids = append(uids, uid)
	}
	store := repo.NewStore(pool)
	profiles := svc.New(store)
	own := svc.NewOwnProfiles(store)
	ps := []d.Profile{}
	for n := 1; n < 3; n++ {
		input := validDetails()
		input.IndexNo = fmt.Sprintf("%s-%d", prefix, n)
		p, e := profiles.Create(ctx, uids[0], dto.CreateInput{UserID: ids[n], Details: input})
		if e != nil {
			t.Fatal(e)
		}
		ps = append(ps, p)
	}
	get := func(n int) d.OwnProfile {
		t.Helper()
		p, e := own.Get(ctx, uids[n])
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	p := get(1)
	if p.StudentID != ps[0].StudentID || p.Accommodation != nil || p.ProfileImageURL != nil || p.Presence.Status != "UNKNOWN" || p.Presence.UpdatedAt != nil || len(p.Guardians) != 1 || p.Email != uids[1]+"@example.invalid" {
		t.Fatalf("unexpected empty state %+v", p)
	}
	if get(2).StudentID != ps[1].StudentID {
		t.Fatal("ownership leaked")
	}
	if _, e := own.Get(ctx, uids[3]); !errors.Is(e, d.ErrNotFound) {
		t.Fatalf("missing profile %v", e)
	}
	if _, e := own.Get(ctx, uids[0]); !errors.Is(e, d.ErrForbidden) {
		t.Fatalf("staff accepted %v", e)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	if e := pool.QueryRow(ctx, "INSERT INTO hostelhive.blocks(name) VALUES($1) RETURNING block_id::text", prefix).Scan(&block); e != nil {
		t.Fatal(e)
	}
	var bed, allocation string
	if e := pool.QueryRow(ctx, `WITH room AS (INSERT INTO hostelhive.rooms(block_id,room_no) VALUES($1,'R91') RETURNING room_id) INSERT INTO hostelhive.beds(room_id,bed_no) SELECT room_id,'B91' FROM room RETURNING bed_id::text`, block).Scan(&bed); e != nil {
		t.Fatal(e)
	}
	if e := pool.QueryRow(ctx, "INSERT INTO hostelhive.bed_allocations(bed_id,student_id) VALUES($1,$2) RETURNING allocation_id::text", bed, ps[0].StudentID).Scan(&allocation); e != nil {
		t.Fatal(e)
	}
	if e := pool.QueryRow(ctx, "INSERT INTO hostelhive.scanner_devices(device_id,name,credential_hash,created_by,updated_by,direction) VALUES(gen_random_uuid(),$1,decode(repeat('00',32),'hex'),$2,$2,'IN') RETURNING device_id::text", prefix, ids[0]).Scan(&device); e != nil {
		t.Fatal(e)
	}
	var event string
	if e := pool.QueryRow(ctx, "INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at) VALUES(gen_random_uuid(),$1,$2,decode(repeat('00',32),'hex'),'IN','recorded',clock_timestamp()) RETURNING event_id::text", device, ps[0].StudentID).Scan(&event); e != nil {
		t.Fatal(e)
	}
	exec("INSERT INTO hostelhive.student_presence(student_id,status,updated_at,device_id,event_id) VALUES($1,'IN',clock_timestamp(),$2,$3)", ps[0].StudentID, device, event)
	objects := &memoryObjects{data: map[string][]byte{}}
	images := svc.NewImages(store, objects)
	var raw bytes.Buffer
	if e := png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 2, 2))); e != nil {
		t.Fatal(e)
	}
	if _, e := images.Upload(ctx, uids[0], ps[0].StudentID, "image/png", raw.Bytes()); e != nil {
		t.Fatal(e)
	}
	p = get(1)
	if p.Accommodation == nil || p.Accommodation.AllocationID != allocation || p.Accommodation.BlockName != prefix || p.Accommodation.RoomNo != "R91" || p.Accommodation.BedNo != "B91" || p.Presence.Status != "IN" || p.Presence.UpdatedAt == nil || p.ProfileImageURL == nil {
		t.Fatalf("bad aggregate %+v", p)
	}
	// Compare complete source rows before/after repeated profile and binary reads.
	snapshot := func() string {
		t.Helper()
		var value string
		e := pool.QueryRow(ctx, `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(s)) FROM hostelhive.students s WHERE s.student_id=$1),
 (SELECT jsonb_agg(to_jsonb(g) ORDER BY guardian_id) FROM hostelhive.guardians g WHERE g.student_id=$1),
 (SELECT jsonb_agg(to_jsonb(a) ORDER BY allocation_id) FROM hostelhive.bed_allocations a WHERE a.student_id=$1),
 (SELECT jsonb_agg(to_jsonb(p)) FROM hostelhive.student_presence p WHERE p.student_id=$1),
 (SELECT jsonb_agg(to_jsonb(e) ORDER BY event_id) FROM hostelhive.attendance_events e WHERE e.student_id=$1),
 (SELECT jsonb_agg(to_jsonb(o) ORDER BY object_key) FROM hostelhive.student_image_objects o WHERE o.student_id=$1))::text`, ps[0].StudentID).Scan(&value)
		if e != nil {
			t.Fatal(e)
		}
		return value
	}
	before := snapshot()
	mux := http.NewServeMux()
	student.RegisterOwnProfile(h.NewOwnProfileAPI(own, images, time.Second), mux, m.Middleware(qrVerifier{}, identityrepo.NewPostgresAccounts(pool), time.Second))
	for i := 0; i < 3; i++ {
		get(1)
		r := httptest.NewRequest("GET", "/api/v1/me/student-profile/image", nil)
		r.Header.Set("Authorization", "Bearer "+uids[1])
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		data, meta, e := images.Get(ctx, ps[0].StudentID)
		if e != nil || w.Code != 200 || !bytes.Equal(w.Body.Bytes(), data) || w.Header().Get("Content-Type") != meta.ContentType {
			t.Fatalf("own image %d %v", w.Code, e)
		}
	}
	if before != snapshot() {
		t.Fatal("reads changed source records")
	}
	exec("UPDATE hostelhive.bed_allocations SET ended_at=clock_timestamp() WHERE allocation_id=$1", allocation)
	if get(1).Accommodation != nil {
		t.Fatal("ended allocation returned")
	}
	exec("UPDATE hostelhive.student_presence SET status='OUT',updated_at=clock_timestamp() WHERE student_id=$1", ps[0].StudentID)
	if get(1).Presence.Status != "OUT" {
		t.Fatal("out presence missing")
	}
	exec("DELETE FROM hostelhive.guardians WHERE student_id=$1", ps[1].StudentID)
	if len(get(2).Guardians) != 0 {
		t.Fatal("empty guardians")
	}
	exec("UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1", ps[0].StudentID, ids[0])
	if _, e := own.Get(ctx, uids[1]); !errors.Is(e, d.ErrNotFound) {
		t.Fatalf("archive %v", e)
	}
	exec("UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", ids[2])
	if _, e := own.Get(ctx, uids[2]); !errors.Is(e, d.ErrForbidden) {
		t.Fatalf("inactive %v", e)
	}
	exec("UPDATE hostelhive.users SET is_active=true,role='warden' WHERE user_id=$1", ids[2])
	if _, e := own.Get(ctx, uids[2]); !errors.Is(e, d.ErrForbidden) {
		t.Fatalf("changed role %v", e)
	}
}
