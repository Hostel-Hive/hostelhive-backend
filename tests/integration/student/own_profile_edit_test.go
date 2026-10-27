package student_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/handler"
	repo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/repository"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOwnStudentProfileEditing(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := "edit-own-" + uuid.NewString()[:12]
	ids, uids := map[string]string{}, map[string]string{}
	var block, device string
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
	defer func() {
		clean, stop := context.WithTimeout(context.Background(), 8*time.Second)
		defer stop()
		for _, q := range []string{
			"ALTER TABLE hostelhive.guardians DROP CONSTRAINT IF EXISTS own_profile_edit_failure",
			"DELETE FROM hostelhive.student_invitations WHERE user_id IN(SELECT user_id FROM hostelhive.users WHERE firebase_uid LIKE $1)",
			"DELETE FROM hostelhive.bed_allocations WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)",
			"DELETE FROM hostelhive.student_presence WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)",
			"DELETE FROM hostelhive.attendance_events WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)",
			"DELETE FROM hostelhive.student_qr WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)",
			"DELETE FROM hostelhive.guardians WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)",
			"DELETE FROM hostelhive.students WHERE index_no LIKE $1",
			"DELETE FROM hostelhive.scanner_device_audit WHERE device_id IN(SELECT device_id FROM hostelhive.scanner_devices WHERE name LIKE $1)",
			"DELETE FROM hostelhive.scanner_devices WHERE name LIKE $1",
			"DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1",
		} {
			var e error
			if strings.Contains(q, "$1") {
				_, e = pool.Exec(clean, q, prefix+"%")
			} else {
				_, e = pool.Exec(clean, q)
			}
			if e != nil {
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
	for _, name := range []string{"admin", "warden", "sub_warden", "security_staff", "student", "other", "missing"} {
		role := name
		if name == "other" || name == "missing" {
			role = "student"
		}
		uids[name] = prefix + "-" + name
		ids[name] = scalar("INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text", uids[name], uids[name]+"@example.invalid", role)
	}
	store := repo.NewStore(pool)
	profiles := svc.New(store)
	owned := map[string]d.Profile{}
	for _, name := range []string{"student", "other"} {
		input := validDetails()
		input.IndexNo = prefix + "-" + name
		p, e := profiles.Create(ctx, uids["admin"], dto.CreateInput{UserID: ids[name], Details: input})
		if e != nil {
			t.Fatal(e)
		}
		owned[name] = p
	}
	sid := owned["student"].StudentID
	block = scalar("INSERT INTO hostelhive.blocks(name) VALUES($1) RETURNING block_id::text", prefix)
	bed := scalar(`WITH room AS (INSERT INTO hostelhive.rooms(block_id,room_no) VALUES($1,'EDIT') RETURNING room_id) INSERT INTO hostelhive.beds(room_id,bed_no) SELECT room_id,'1' FROM room RETURNING bed_id::text`, block)
	allocation := scalar("INSERT INTO hostelhive.bed_allocations(bed_id,student_id) VALUES($1,$2) RETURNING allocation_id::text", bed, sid)
	device = scalar("INSERT INTO hostelhive.scanner_devices(device_id,name,credential_hash,created_by,updated_by,direction) VALUES(gen_random_uuid(),$1,decode(repeat('01',32),'hex'),$2,$2,'IN') RETURNING device_id::text", prefix, ids["admin"])
	event := scalar("INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at) VALUES(gen_random_uuid(),$1,$2,decode(repeat('02',32),'hex'),'IN','recorded',clock_timestamp()) RETURNING event_id::text", device, sid)
	exec("INSERT INTO hostelhive.student_presence(student_id,status,updated_at,device_id,event_id) VALUES($1,'IN',clock_timestamp(),$2,$3)", sid, device, event)
	exec("INSERT INTO hostelhive.student_qr(student_id,token) VALUES($1,repeat('ab',32))", sid)
	immutable := func() string {
		return scalar(`SELECT jsonb_build_array(
 (SELECT to_jsonb(s)-'contact_phone'-'updated_at' FROM hostelhive.students s WHERE student_id=$1),
 (SELECT to_jsonb(s) FROM hostelhive.students s WHERE student_id=$2),
 (SELECT jsonb_agg(to_jsonb(g) ORDER BY guardian_id) FROM hostelhive.guardians g WHERE student_id=$2),
 (SELECT jsonb_agg(to_jsonb(u) ORDER BY user_id) FROM hostelhive.users u WHERE firebase_uid LIKE $3),
 (SELECT jsonb_agg(to_jsonb(a) ORDER BY allocation_id) FROM hostelhive.bed_allocations a WHERE student_id=$1),
 (SELECT to_jsonb(p) FROM hostelhive.student_presence p WHERE student_id=$1),
 (SELECT jsonb_agg(to_jsonb(e) ORDER BY event_id) FROM hostelhive.attendance_events e WHERE student_id=$1),
 (SELECT to_jsonb(q) FROM hostelhive.student_qr q WHERE student_id=$1),
 (SELECT jsonb_agg(to_jsonb(o) ORDER BY object_key) FROM hostelhive.student_image_objects o WHERE student_id=$1),
 (SELECT count(*) FROM hostelhive.notification_events),
 (SELECT count(*) FROM hostelhive.notification_outbox))::text`, sid, owned["other"].StudentID, prefix+"%")
	}
	before := immutable()
	mux := http.NewServeMux()
	auth := m.Middleware(qrVerifier{}, identityrepo.NewPostgresAccounts(pool), 5*time.Second)
	student.RegisterOwnProfileEdit(h.NewOwnProfileEditAPI(svc.NewOwnProfileEditor(store), 5*time.Second), mux, auth)
	student.RegisterOwnProfile(h.NewOwnProfileAPI(svc.NewOwnProfiles(store), nil, 5*time.Second), mux, auth)
	student.Register(h.NewAPI(profiles, 5*time.Second), mux, auth)
	call := func(method, path, body, name string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if name != "" {
			r.Header.Set("Authorization", "Bearer "+uids[name])
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	check := func(w *httptest.ResponseRecorder, want int) d.OwnProfile {
		t.Helper()
		if w.Code != want {
			t.Fatalf("want%d got%d %s", want, w.Code, w.Body)
		}
		var p d.OwnProfile
		if want == 200 {
			if e := json.Unmarshal(w.Body.Bytes(), &p); e != nil {
				t.Fatal(e)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(w.Header())
			}
			var fields map[string]any
			_ = json.Unmarshal(w.Body.Bytes(), &fields)
			for _, key := range []string{"firebase_uid", "user_id", "account_active", "profile_image_key"} {
				if _, ok := fields[key]; ok {
					t.Fatal("internal field leaked", key)
				}
			}
		}
		return p
	}
	path := "/api/v1/me/student-profile"
	phoneBody := `{"contact_phone":" 0777654321 "}`
	p := check(call("PATCH", path, phoneBody, "student"), 200)
	if p.StudentID != sid || p.ContactPhone != "0777654321" || len(p.Guardians) != 1 || p.Guardians[0].GuardianID != owned["student"].Guardians[0].GuardianID || p.Presence.Status != "IN" || p.Accommodation == nil || p.Accommodation.AllocationID != allocation {
		t.Fatal(p)
	}
	guardianBody := `{"guardians":[{"name":" New Guardian ","relationship":" Father ","contact_phone":" 0717654321 "}]}`
	p = check(call("PATCH", path, guardianBody, "student"), 200)
	if p.ContactPhone != "0777654321" || len(p.Guardians) != 1 || p.Guardians[0].Name != "New Guardian" || p.Guardians[0].Relationship != "Father" {
		t.Fatal(p)
	}
	if before != immutable() {
		t.Fatal("profile patch changed protected fields, another Student or unrelated source records")
	}
	other := check(call("GET", path, "", "other"), 200)
	if other.StudentID != owned["other"].StudentID || other.ContactPhone != owned["other"].ContactPhone {
		t.Fatal("another Student was changed", other)
	}
	for _, name := range []string{"admin", "warden", "sub_warden", "security_staff"} {
		check(call("PATCH", path, phoneBody, name), 403)
	}
	check(call("PATCH", path, phoneBody, ""), 401)
	check(call("PATCH", path, phoneBody, "missing"), 404)
	for _, query := range []string{"student_id=" + owned["other"].StudentID, "user_id=" + ids["other"], "unknown=1"} {
		check(call("PATCH", path+"?"+query, phoneBody, "student"), 400)
	}
	for _, body := range []string{`{}`, `{"user_id":"` + ids["other"] + `","contact_phone":"0771234567"}`, `{"email":"new@example.invalid"}`, `{"year":5}`, `{"guardians":[]}`} {
		check(call("PATCH", path, body, "student"), 400)
	}
	// A write failure after replacing phone/deleting guardians must restore all
	// source rows, timestamps, guardian IDs and queued publication state.
	snapshot := func() string {
		return scalar(`SELECT jsonb_build_array((SELECT to_jsonb(s) FROM hostelhive.students s WHERE student_id=$1),(SELECT jsonb_agg(to_jsonb(g) ORDER BY guardian_id) FROM hostelhive.guardians g WHERE student_id=$1),(SELECT to_jsonb(p) FROM hostelhive.attendance_publication p))::text`, sid)
	}
	previous := snapshot()
	exec("ALTER TABLE hostelhive.guardians ADD CONSTRAINT own_profile_edit_failure CHECK(name<>'Forced failure') NOT VALID")
	check(call("PATCH", path, `{"contact_phone":"0771111111","guardians":[{"name":"Forced failure","relationship":"Mother","contact_phone":"0711111111"}]}`, "student"), 503)
	exec("ALTER TABLE hostelhive.guardians DROP CONSTRAINT own_profile_edit_failure")
	if previous != snapshot() {
		t.Fatal("partial write escaped rollback")
	}
	// Concurrent disjoint patches must not restore stale values for omitted fields.
	var wg sync.WaitGroup
	results := make(chan *httptest.ResponseRecorder, 2)
	for _, body := range []string{`{"contact_phone":"0772222222"}`, `{"guardians":[{"name":"Concurrent Guardian","relationship":"Mother","contact_phone":"0712222222"}]}`} {
		wg.Add(1)
		go func(body string) { defer wg.Done(); results <- call("PATCH", path, body, "student") }(body)
	}
	wg.Wait()
	close(results)
	for w := range results {
		check(w, 200)
	}
	p = check(call("GET", path, "", "student"), 200)
	if p.ContactPhone != "0772222222" || p.Guardians[0].Name != "Concurrent Guardian" {
		t.Fatal("lost disjoint edit", p)
	}
	// Admin/Warden editing still works through its existing complete-profile PUT.
	staffInput := validDetails()
	staffInput.IndexNo = owned["student"].IndexNo
	staffInput.FullName = "Staff managed name"
	body, _ := json.Marshal(staffInput)
	if w := call("PUT", "/api/v1/students/"+sid, string(body), "warden"); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if check(call("GET", path, "", "student"), 200).FullName != "Staff managed name" {
		t.Fatal("staff editing regressed")
	}
	// Fresh repository checks reject stale middleware authority and pending setup.
	phone := "0773333333"
	exec("UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", ids["other"])
	if _, e := store.PatchOwnProfile(ctx, uids["other"], dto.OwnProfilePatch{ContactPhone: &phone}); !errors.Is(e, d.ErrForbidden) {
		t.Fatal(e)
	}
	exec("UPDATE hostelhive.users SET is_active=true,role='warden' WHERE user_id=$1", ids["other"])
	if _, e := store.PatchOwnProfile(ctx, uids["other"], dto.OwnProfilePatch{ContactPhone: &phone}); !errors.Is(e, d.ErrForbidden) {
		t.Fatal(e)
	}
	exec("UPDATE hostelhive.users SET role='student' WHERE user_id=$1", ids["other"])
	exec(`INSERT INTO hostelhive.student_invitations(request_id,user_id,student_id,email,created_by,request_ip,payload_hash) VALUES(gen_random_uuid(),$1,$2,$3,$4,'127.0.0.1',decode(repeat('01',32),'hex'))`, ids["other"], owned["other"].StudentID, uids["other"]+"@example.invalid", ids["admin"])
	check(call("PATCH", path, phoneBody, "other"), 403)
	if _, e := store.PatchOwnProfile(ctx, uids["other"], dto.OwnProfilePatch{ContactPhone: &phone}); !errors.Is(e, d.ErrForbidden) {
		t.Fatal(e)
	}
	exec("UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1", sid, ids["admin"])
	check(call("PATCH", path, phoneBody, "student"), 404)
}
