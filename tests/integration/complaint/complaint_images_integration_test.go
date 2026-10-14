package complaint_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/service"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
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

type imageObjects struct {
	mu                                              sync.Mutex
	data                                            map[string][]byte
	puts                                            int
	failPut, failDelete, failGet, corrupt, afterPut bool
}

func (o *imageObjects) Put(_ context.Context, key, mime string, b []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.puts++
	if o.failPut {
		return errors.New("provider unavailable")
	}
	o.data[key] = append([]byte{}, b...)
	if o.afterPut {
		return errors.New("lost response")
	}
	return nil
}
func (o *imageObjects) Get(_ context.Context, key string) ([]byte, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.failGet {
		return nil, errors.New("provider unavailable")
	}
	b, ok := o.data[key]
	if !ok {
		return nil, errors.New("missing")
	}
	b = append([]byte{}, b...)
	if o.corrupt {
		b[0] ^= 1
	}
	return b, nil
}
func (o *imageObjects) Delete(_ context.Context, key string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.failDelete {
		return errors.New("provider unavailable")
	}
	delete(o.data, key)
	return nil
}
func (o *imageObjects) has(key string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, ok := o.data[key]
	return ok
}
func TestComplaintImages(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable database migrated to 17")
	}
	ctx, c := context.WithTimeout(context.Background(), 90*time.Second)
	defer c()
	pool, e := pgxpool.New(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	prefix := fmt.Sprintf("complaint-image-%d", time.Now().UnixNano())
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
	user("sub", "sub_warden")
	user("security", "security_staff")
	u := user("student", "student")
	other := user("other", "student")
	student := func(id, suffix string) string {
		return insert(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Image Test','Engineering',1,'0771234567') RETURNING student_id::text`, id, prefix+suffix)
	}
	sid := student(u, "student")
	student(other, "other")
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		for _, sql := range []string{
			`DELETE FROM hostelhive.complaint_updates WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1))`,
			`DELETE FROM hostelhive.complaint_images WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1))`,
			`DELETE FROM hostelhive.complaints WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.students WHERE index_no LIKE $1`,
			`DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`,
		} {
			if _, e := pool.Exec(clean, sql, prefix+"%"); e != nil {
				t.Error(e)
			}
		}
	}()
	repo := repository.New(pool)
	core := service.New(repo)
	objects := &imageObjects{data: map[string][]byte{}}
	images := service.NewImages(repo, objects)
	disabled := service.NewImages(repo, nil)
	uid := prefix + "student"
	auid := prefix + "admin"
	wuid := prefix + "warden"
	subuid := prefix + "sub"
	expect := func(e, want error) {
		t.Helper()
		if !errors.Is(e, want) {
			t.Fatalf("want %v got %v", want, e)
		}
	}
	create := func(category string) string {
		t.Helper()
		v, e := core.Submit(ctx, uid, dto.Submit{RequestID: rid(), Category: category, Description: "Image test"})
		if e != nil {
			t.Fatal(e)
		}
		return v.ComplaintID
	}
	var b bytes.Buffer
	if e = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4))); e != nil {
		t.Fatal(e)
	}
	raw := append(append([]byte{}, b.Bytes()...), []byte("private trailing metadata")...)
	id := create("maintenance")
	imageID := rid()
	first, e := images.Upload(ctx, uid, id, imageID, "image/png", raw)
	if e != nil || first.Version != 2 || first.Replayed || first.Image.UploadedAt == nil {
		t.Fatal(first, e)
	}
	before := objects.puts
	again, e := images.Upload(ctx, uid, id, imageID, "image/png", raw)
	if e != nil || !again.Replayed || again.Version != 2 || objects.puts != before {
		t.Fatal("upload retry changed history/storage", again, e)
	}
	var changed bytes.Buffer
	png.Encode(&changed, image.NewRGBA(image.Rect(0, 0, 3, 3)))
	_, e = images.Upload(ctx, uid, id, imageID, "image/png", changed.Bytes())
	expect(e, d.ErrConflict)
	data, m, e := images.Get(ctx, uid, id, imageID)
	if e != nil || bytes.Contains(data, []byte("private trailing metadata")) || len(data) != m.Size {
		t.Fatal(m, e)
	}
	for _, actor := range []string{auid, wuid, subuid} {
		if _, _, e = images.Get(ctx, actor, id, imageID); e != nil {
			t.Fatal(e)
		}
	}
	_, _, e = images.Get(ctx, prefix+"other", id, imageID)
	expect(e, d.ErrNotFound)
	_, e = images.Upload(ctx, auid, id, rid(), "image/png", raw)
	expect(e, d.ErrForbidden)
	_, e = images.Remove(ctx, subuid, id, imageID)
	expect(e, d.ErrForbidden)
	_, e = images.Remove(ctx, prefix+"other", id, imageID)
	expect(e, d.ErrNotFound)
	list, e := images.List(ctx, uid, id)
	if e != nil || len(list.Items) != 1 || list.Version != 2 {
		t.Fatal(list, e)
	}
	public, _ := json.Marshal(list)
	if bytes.Contains(public, []byte("object_key")) || bytes.Contains(public, []byte("complaint-images/")) || bytes.Contains(public, []byte("payload_hash")) {
		t.Fatal("private storage data exposed", string(public))
	}
	for range 2 {
		if _, e = images.Upload(ctx, uid, id, rid(), "image/png", raw); e != nil {
			t.Fatal(e)
		}
	}
	_, e = images.Upload(ctx, uid, id, rid(), "image/png", raw)
	expect(e, d.ErrConflict)
	removed, e := images.Remove(ctx, uid, id, imageID)
	if e != nil || removed.Version != 5 || !removed.Removed {
		t.Fatal(removed, e)
	}
	againRemoved, e := images.Remove(ctx, uid, id, imageID)
	if e != nil || !againRemoved.Replayed || againRemoved.Version != 5 {
		t.Fatal(againRemoved, e)
	}
	_, _, e = images.Get(ctx, uid, id, imageID)
	expect(e, d.ErrNotFound)
	if !objects.has(first.Image.Key) {
		t.Fatal("removal did not queue provider cleanup")
	}
	worked, e := repo.CleanupImageOne(ctx, objects)
	if e != nil || !worked || objects.has(first.Image.Key) {
		t.Fatal("removed object cleanup", worked, e)
	}
	_, e = images.Upload(ctx, uid, id, imageID, "image/png", raw)
	expect(e, d.ErrConflict)
	replacement, e := images.Upload(ctx, uid, id, rid(), "image/png", raw)
	if e != nil || replacement.Version != 6 {
		t.Fatal(replacement, e)
	}
	_, e = core.Change(ctx, wuid, id, "status", dto.Change{RequestID: rid(), ExpectedVersion: 1, Status: "in_progress"})
	expect(e, d.ErrConflict)
	if _, e = core.Change(ctx, wuid, id, "status", dto.Change{RequestID: rid(), ExpectedVersion: 6, Status: "in_progress"}); e != nil {
		t.Fatal(e)
	}
	_, e = images.Upload(ctx, uid, id, rid(), "image/png", raw)
	expect(e, d.ErrConflict)
	_, e = images.Remove(ctx, uid, id, replacement.Image.ImageID)
	expect(e, d.ErrConflict)
	again, e = images.Upload(ctx, uid, id, replacement.Image.ImageID, "image/png", raw)
	if e != nil || !again.Replayed {
		t.Fatal("frozen exact retry failed", again, e)
	}
	history, e := core.Updates(ctx, uid, id, 100, 0)
	if e != nil || history.Total != 7 || history.Items[1].ImageID == nil || *history.Items[1].ImageID != imageID || history.Items[4].Action != "image_removed" {
		t.Fatal(history, e)
	}
	discipline := create("disciplinary")
	di := rid()
	if _, e = images.Upload(ctx, uid, discipline, di, "image/png", raw); e != nil {
		t.Fatal(e)
	}
	_, _, e = images.Get(ctx, subuid, discipline, di)
	expect(e, d.ErrNotFound)
	_, e = images.List(ctx, subuid, discipline)
	expect(e, d.ErrNotFound)
	// Failure leaves a durable reservation; retry can attach without consuming another slot.
	failure := create("maintenance")
	fi := rid()
	objects.failPut = true
	_, e = images.Upload(ctx, uid, failure, fi, "image/png", raw)
	expect(e, d.ErrUnavailable)
	objects.failPut = false
	current, e := core.Get(ctx, uid, failure)
	if e != nil || current.Version != 1 {
		t.Fatal("failed upload altered complaint", current, e)
	}
	if _, e = images.Upload(ctx, uid, failure, fi, "image/png", raw); e != nil {
		t.Fatal(e)
	}
	// Simulate a lost provider response after writing bytes, then expiry and durable deletion retry.
	abandoned := create("maintenance")
	ai := rid()
	objects.afterPut = true
	_, e = images.Upload(ctx, uid, abandoned, ai, "image/png", raw)
	expect(e, d.ErrUnavailable)
	objects.afterPut = false
	key := insert(`SELECT object_key FROM hostelhive.complaint_images WHERE complaint_id=$1 AND image_id=$2`, abandoned, ai)
	if !objects.has(key) {
		t.Fatal("lost-response fixture missing")
	}
	exec(`UPDATE hostelhive.complaint_images SET cleanup_at=clock_timestamp()-interval '1 second' WHERE complaint_id=$1 AND image_id=$2`, abandoned, ai)
	objects.failDelete = true
	worked, e = repo.CleanupImageOne(ctx, objects)
	if !worked {
		t.Fatal("cleanup not attempted")
	}
	expect(e, d.ErrUnavailable)
	objects.failDelete = false
	exec(`UPDATE hostelhive.complaint_images SET cleanup_at=clock_timestamp()-interval '1 second' WHERE complaint_id=$1 AND image_id=$2`, abandoned, ai)
	worked, e = repo.CleanupImageOne(ctx, objects)
	if e != nil || !worked || objects.has(key) {
		t.Fatal("abandoned cleanup", worked, e)
	}
	_, e = images.Upload(ctx, uid, abandoned, ai, "image/png", raw)
	expect(e, d.ErrConflict)
	if _, e = images.Upload(ctx, uid, abandoned, rid(), "image/png", raw); e != nil {
		t.Fatal("expired slot not released", e)
	}
	// Four concurrent uploads contend on the three available slots.
	raceID := create("maintenance")
	ids := []string{rid(), rid(), rid(), rid()}
	start := make(chan struct{})
	results := make(chan error, 4)
	for _, i := range ids {
		go func(i string) { <-start; _, e := images.Upload(ctx, uid, raceID, i, "image/png", raw); results <- e }(i)
	}
	close(start)
	wins := 0
	for range 4 {
		e := <-results
		if e == nil {
			wins++
		} else {
			expect(e, d.ErrConflict)
		}
	}
	if wins != 3 {
		t.Fatal("image cap race", wins)
	}
	objects.corrupt = true
	_, _, e = images.Get(ctx, uid, id, replacement.Image.ImageID)
	expect(e, d.ErrUnavailable)
	objects.corrupt = false
	objects.failGet = true
	_, _, e = images.Get(ctx, uid, id, replacement.Image.ImageID)
	expect(e, d.ErrUnavailable)
	objects.failGet = false
	textOnly := create("maintenance")
	_, e = disabled.Upload(ctx, uid, textOnly, rid(), "image/png", raw)
	expect(e, d.ErrUnavailable)
	none, e := disabled.List(ctx, uid, textOnly)
	if e != nil || len(none.Items) != 0 {
		t.Fatal("disabled upload reserved slots", none, e)
	}
	_, _, e = disabled.Get(ctx, uid, id, replacement.Image.ImageID)
	expect(e, d.ErrUnavailable)
	_, _, e = disabled.Get(ctx, subuid, discipline, di)
	expect(e, d.ErrNotFound)
	// HTTP path checks response privacy, raw media, role/scope and body limits.
	call := func(actor, method, path, mime string, data []byte, want int) *httptest.ResponseRecorder {
		t.Helper()
		mux := http.NewServeMux()
		auth := middleware.Middleware(verifier{actor}, identityrepo.NewPostgresAccounts(pool), 3*time.Second)
		complaint.RegisterImages(handler.NewImageAPI(images, 3*time.Second), mux, auth)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		if actor != "" {
			r.Header.Set("Authorization", "Bearer test")
		}
		if mime != "" {
			r.Header.Set("Content-Type", mime)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s want %d got %d %s", method, path, want, w.Code, w.Body.String())
		}
		return w
	}
	httpID := create("maintenance")
	hi := rid()
	path := "/api/v1/complaints/" + httpID + "/images/" + hi
	response := call(uid, "PUT", path, "image/png", raw, 201)
	if strings.Contains(response.Body.String(), "complaint-images/") {
		t.Fatal("object key exposed")
	}
	call(uid, "PUT", path, "image/png", raw, 200)
	read := call(subuid, "GET", path, "", nil, 200)
	if read.Header().Get("Cache-Control") != "no-store" || read.Header().Get("X-Content-Type-Options") != "nosniff" || read.Header().Get("Content-Type") != "image/png" {
		t.Fatal(read.Header())
	}
	call(uid, "GET", "/api/v1/complaints/"+httpID+"/images", "", nil, 200)
	call(prefix+"other", "GET", path, "", nil, 404)
	call(prefix+"security", "GET", path, "", nil, 403)
	call("", "GET", path, "", nil, 401)
	call(auid, "PUT", path, "image/png", raw, 403)
	call(subuid, "GET", "/api/v1/complaints/"+discipline+"/images/"+di, "", nil, 404)
	call(uid, "PUT", "/api/v1/complaints/"+httpID+"/images/"+rid(), "image/jpeg", raw, 400)
	call(uid, "PUT", path, "text/plain", raw, 415)
	call(uid, "PUT", path, "image/png", make([]byte, d.MaxComplaintImageBytes+1), 413)
	call(uid, "PUT", "/api/v1/complaints/"+httpID+"/images/bad", "image/png", raw, 400)
	call(uid, "GET", path+"?uid=other", "", nil, 400)
	call(uid, "DELETE", path, "application/json", []byte("{}"), 400)
	call(uid, "DELETE", path, "", nil, 200)
	call(uid, "DELETE", path, "", nil, 200)
	call(uid, "GET", path, "", nil, 404)
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, u)
	_, _, e = images.Get(ctx, uid, id, replacement.Image.ImageID)
	expect(e, d.ErrForbidden)
	if _, _, e = images.Get(ctx, wuid, id, replacement.Image.ImageID); e != nil {
		t.Fatal(e)
	}
	exec(`UPDATE hostelhive.users SET is_active=true WHERE user_id=$1`, u)
	exec(`UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, sid, admin)
	_, e = images.Upload(ctx, uid, textOnly, rid(), "image/png", raw)
	expect(e, d.ErrIneligible)
	if _, _, e = images.Get(ctx, uid, id, replacement.Image.ImageID); e != nil {
		t.Fatal("archival erased evidence", e)
	}
}
