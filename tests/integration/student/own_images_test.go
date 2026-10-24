package student_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOwnStudentImages(t *testing.T) {
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
	prefix := "own-image-" + uuid.NewString()[:12]
	ids, uids := map[string]string{}, map[string]string{}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	defer func() {
		clean, stop := context.WithTimeout(context.Background(), 8*time.Second)
		defer stop()
		for _, q := range []string{
			"DROP TRIGGER IF EXISTS own_image_fault ON hostelhive.students",
			"DROP FUNCTION IF EXISTS hostelhive.own_image_test_failure()",
			"DELETE FROM hostelhive.student_invitations WHERE user_id IN(SELECT user_id FROM hostelhive.users WHERE firebase_uid LIKE $1)",
			"UPDATE hostelhive.students SET profile_image_key=NULL WHERE index_no LIKE $1",
			"DELETE FROM hostelhive.student_image_objects WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)",
			"DELETE FROM hostelhive.guardians WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)",
			"DELETE FROM hostelhive.students WHERE index_no LIKE $1",
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
	}()
	for _, role := range []string{"admin", "warden", "student", "other", "missing"} {
		uid := prefix + "-" + role
		dbRole := role
		if role == "other" || role == "missing" {
			dbRole = "student"
		}
		var id string
		if err = pool.QueryRow(ctx, "INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text", uid, uid+"@example.invalid", dbRole).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[role] = id
		uids[role] = uid
	}
	store := repo.NewStore(pool)
	ownStore := repo.NewOwnImages(store)
	profiles := svc.New(store)
	owned := map[string]d.Profile{}
	for _, role := range []string{"student", "other"} {
		input := dto.CreateInput{UserID: ids[role], Details: validDetails()}
		input.IndexNo = prefix + "-" + role
		p, e := profiles.Create(ctx, uids["admin"], input)
		if e != nil {
			t.Fatal(e)
		}
		owned[role] = p
	}
	objects := &memoryObjects{data: map[string][]byte{}}
	ownImages := svc.NewOwnImages(store, ownStore, objects)
	mux := http.NewServeMux()
	auth := m.Middleware(qrVerifier{}, identityrepo.NewPostgresAccounts(pool), 5*time.Second)
	student.RegisterOwnImages(h.NewOwnImageAPI(ownImages, 5*time.Second), mux, auth)
	student.RegisterOwnProfile(h.NewOwnProfileAPI(svc.NewOwnProfiles(store), svc.NewImages(store, objects), 5*time.Second), mux, auth)
	var buf bytes.Buffer
	if err = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	path := "/api/v1/me/student-profile/image"
	call := func(method, query, role string, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path+query, bytes.NewReader(body))
		req.Header.Set("Content-Type", "image/png")
		if role != "" {
			req.Header.Set("Authorization", "Bearer "+uids[role])
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("status %d want %d: %s", w.Code, status, w.Body)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cacheable response")
		}
	}
	sid := owned["student"].StudentID
	otherID := owned["other"].StudentID
	snapshot := func(id string) string {
		t.Helper()
		var result string
		err := pool.QueryRow(ctx, `SELECT ((to_jsonb(s)-'profile_image_key'-'updated_at')||jsonb_build_object('guardians',(SELECT jsonb_agg(to_jsonb(g) ORDER BY guardian_id) FROM hostelhive.guardians g WHERE g.student_id=s.student_id)))::text FROM hostelhive.students s WHERE student_id=$1`, id).Scan(&result)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	before, otherBefore := snapshot(sid), snapshot(otherID)
	check(call("GET", "", "student", nil), 404)
	check(call("PUT", "", "", raw), 401)
	check(call("PUT", "", "admin", raw), 403)
	check(call("DELETE", "", "warden", nil), 403)
	check(call("PUT", "?student_id="+otherID, "student", raw), 400)
	check(call("DELETE", "?user_id="+ids["other"], "student", nil), 400)
	check(call("PUT", "", "missing", raw), 404)
	check(call("PUT", "", "student", []byte(`{"student_id":"`+otherID+`"}`)), 400)
	w := call("PUT", "", "student", raw)
	check(w, 200)
	var result d.OwnImageResult
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.StudentID != sid || result.ProfileImageURL != path || strings.Contains(w.Body.String(), "student-images/") {
		t.Fatal("unsafe acknowledgement", w.Body)
	}
	check(call("GET", "", "student", nil), 200)
	check(call("GET", "", "other", nil), 404)
	first, err := store.GetImage(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	// The repository independently rejects another account, including direct
	// calls which bypass HTTP identity resolution.
	meta := d.Image{Key: "student-images/" + otherID + "/" + strings.ReplaceAll(uuid.NewString(), "-", ""), ContentType: "image/png", Size: len(raw), Width: 2, Height: 2}
	if e := ownStore.ReserveImage(ctx, uids["student"], otherID, meta); !errors.Is(e, d.ErrForbidden) {
		t.Fatal("cross-account reserve", e)
	}
	if work, e := ownStore.BeginImage(ctx, uids["student"], otherID, first.Key); !errors.Is(e, d.ErrForbidden) {
		if work != nil {
			work.Close()
		}
		t.Fatal("cross-account attach", e)
	}
	if e := ownStore.RemoveImage(ctx, uids["other"], sid); !errors.Is(e, d.ErrForbidden) {
		t.Fatal("cross-account remove", e)
	}
	objects.failPut = true
	check(call("PUT", "", "student", raw), 503)
	objects.failPut = false
	retained, _ := store.GetImage(ctx, sid)
	if retained.Key != first.Key {
		t.Fatal("storage failure lost current photo")
	}
	exec("CREATE FUNCTION hostelhive.own_image_test_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private test fault'; END $$")
	exec("CREATE TRIGGER own_image_fault BEFORE UPDATE OF profile_image_key ON hostelhive.students FOR EACH ROW EXECUTE FUNCTION hostelhive.own_image_test_failure()")
	failed := call("PUT", "", "student", raw)
	check(failed, 503)
	if strings.Contains(failed.Body.String(), "private test fault") {
		t.Fatal("database error exposed")
	}
	check(call("DELETE", "", "student", nil), 503)
	exec("DROP TRIGGER own_image_fault ON hostelhive.students")
	exec("DROP FUNCTION hostelhive.own_image_test_failure()")
	retained, _ = store.GetImage(ctx, sid)
	if retained.Key != first.Key {
		t.Fatal("database rollback lost current photo")
	}
	// Concurrent replacements retain one current object and durable orphan rows.
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); statuses <- call("PUT", "", "student", raw).Code }()
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != 200 {
			t.Fatal("concurrent upload", status)
		}
	}
	current, _ := store.GetImage(ctx, sid)
	if current.Key == first.Key {
		t.Fatal("replacement reused key")
	}
	var cleanup bool
	if err = pool.QueryRow(ctx, "SELECT cleanup_at IS NOT NULL FROM hostelhive.student_image_objects WHERE object_key=$1", first.Key).Scan(&cleanup); err != nil || !cleanup {
		t.Fatal("old photo not queued", err)
	}
	check(call("DELETE", "", "student", nil), 204)
	check(call("GET", "", "student", nil), 404)
	check(call("DELETE", "", "student", nil), 204)
	if err = pool.QueryRow(ctx, "SELECT cleanup_at IS NOT NULL FROM hostelhive.student_image_objects WHERE object_key=$1", current.Key).Scan(&cleanup); err != nil || !cleanup {
		t.Fatal("removed photo not queued", err)
	}
	if snapshot(sid) != before || snapshot(otherID) != otherBefore {
		t.Fatal("photo mutation changed unrelated fields")
	}
	// Account checks run again after reservation, before any object is written.
	meta.Key = "student-images/" + sid + "/" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = ownStore.ReserveImage(ctx, uids["student"], sid, meta); err != nil {
		t.Fatal(err)
	}
	exec("UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", ids["student"])
	if work, e := ownStore.BeginImage(ctx, uids["student"], sid, meta.Key); !errors.Is(e, d.ErrForbidden) {
		if work != nil {
			work.Close()
		}
		t.Fatal("stale active status", e)
	}
	check(call("PUT", "", "student", raw), 403)
	exec("UPDATE hostelhive.users SET is_active=true,role='warden' WHERE user_id=$1", ids["student"])
	if e := ownStore.RemoveImage(ctx, uids["student"], sid); !errors.Is(e, d.ErrForbidden) {
		t.Fatal("stale role", e)
	}
	exec("UPDATE hostelhive.users SET role='student' WHERE user_id=$1", ids["student"])
	exec(`INSERT INTO hostelhive.student_invitations(request_id,user_id,student_id,email,created_by,request_ip,payload_hash) VALUES(gen_random_uuid(),$1,$2,$3,$4,'127.0.0.1',decode(repeat('01',32),'hex'))`, ids["student"], sid, uids["student"]+"@example.invalid", ids["admin"])
	check(call("PUT", "", "student", raw), 403)
	check(call("DELETE", "", "student", nil), 403)
	if e := ownStore.RemoveImage(ctx, uids["student"], sid); !errors.Is(e, d.ErrForbidden) {
		t.Fatal("pending setup", e)
	}
	exec("UPDATE hostelhive.student_invitations SET setup_completed_at=clock_timestamp() WHERE user_id=$1", ids["student"])
	check(call("PUT", "", "student", raw), 200)
	exec("UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1", sid, ids["admin"])
	check(call("PUT", "", "student", raw), 404)
	check(call("DELETE", "", "student", nil), 404)
}
