package student_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	dto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	repo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/repository"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"image"
	"image/png"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryObjects struct {
	mu                  sync.Mutex
	data                map[string][]byte
	failPut, failDelete bool
}

func (o *memoryObjects) Put(_ context.Context, key, mime string, b []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.data[key] = append([]byte{}, b...)
	if o.failPut {
		return errors.New("ambiguous put failure")
	}
	return nil
}
func (o *memoryObjects) Get(_ context.Context, key string) ([]byte, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	data, ok := o.data[key]
	if !ok {
		return nil, errors.New("missing object")
	}
	return append([]byte{}, data...), nil
}
func (o *memoryObjects) Delete(_ context.Context, key string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.failDelete {
		return errors.New("delete failed")
	}
	delete(o.data, key)
	return nil
}
func TestStudentImagesPostgresLifecycle(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable database migrated to version 6")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := fmt.Sprintf("image30-%d", time.Now().UnixNano())
	uids := []string{}
	ids := []string{}
	for n, role := range []string{"admin", "warden", "student", "student"} {
		uid := fmt.Sprintf("%s-%d", prefix, n)
		var id string
		if err = pool.QueryRow(ctx, "INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text", uid, uid+"@example.invalid", role).Scan(&id); err != nil {
			t.Fatal(err)
		}
		uids = append(uids, uid)
		ids = append(ids, id)
	}
	defer func() {
		for _, q := range []string{"UPDATE hostelhive.students SET profile_image_key=NULL WHERE user_id=ANY($1::uuid[])", "DELETE FROM hostelhive.student_image_objects WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE user_id=ANY($1::uuid[]))", "DELETE FROM hostelhive.guardians WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE user_id=ANY($1::uuid[]))", "DELETE FROM hostelhive.students WHERE user_id=ANY($1::uuid[])", "DELETE FROM hostelhive.users WHERE user_id=ANY($1::uuid[])"} {
			if _, e := pool.Exec(context.Background(), q, ids); e != nil {
				t.Error("fixture cleanup", e)
			}
		}
	}()
	r := repo.NewStore(pool)
	profiles := svc.New(r)
	input := dto.CreateInput{UserID: ids[2], Details: validDetails()}
	input.IndexNo = prefix
	profile, err := profiles.Create(ctx, uids[1], input)
	if err != nil {
		t.Fatal("Warden cannot create", err)
	}
	details := input.Details
	details.FullName = "Warden Updated"
	if _, err = profiles.Update(ctx, uids[1], profile.StudentID, details); err != nil {
		t.Fatal("Warden cannot update", err)
	}
	objects := &memoryObjects{data: map[string][]byte{}}
	images := svc.NewImages(r, objects)
	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	raw := buf.Bytes()
	if _, err = images.Upload(ctx, uids[2], profile.StudentID, "image/png", raw); !errors.Is(err, d.ErrForbidden) {
		t.Fatal("student uploaded", err)
	}
	if _, _, err = images.Get(ctx, profile.StudentID); !errors.Is(err, d.ErrNotFound) {
		t.Fatal("missing image", err)
	}
	uploaded, err := images.Upload(ctx, uids[1], profile.StudentID, "image/png", raw)
	if err != nil || uploaded.ProfileImageURL == "" {
		t.Fatal("Warden upload", err)
	}
	first, err := r.GetImage(ctx, profile.StudentID)
	if err != nil {
		t.Fatal(err)
	}
	data, meta, err := images.Get(ctx, profile.StudentID)
	if err != nil || len(data) != meta.Size {
		t.Fatal("download", err)
	}
	// Ambiguous storage failure must leave the old reference and durable cleanup.
	objects.failPut = true
	if _, err = images.Upload(ctx, uids[0], profile.StudentID, "image/png", raw); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal("false upload success", err)
	}
	retained, _ := r.GetImage(ctx, profile.StudentID)
	if retained.Key != first.Key {
		t.Fatal("failed replacement lost old image")
	}
	objects.failPut = false
	// Force attachment failure after storage succeeded; the old image survives.
	function := "hostelhive." + strings.ReplaceAll(prefix, "-", "_") + "_fail"
	if _, err = pool.Exec(ctx, "CREATE FUNCTION "+function+"() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test fault'; END $$"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "CREATE TRIGGER image_fault BEFORE UPDATE OF profile_image_key ON hostelhive.students FOR EACH ROW EXECUTE FUNCTION "+function+"()"); err != nil {
		t.Fatal(err)
	}
	_, uploadErr := images.Upload(ctx, uids[0], profile.StudentID, "image/png", raw)
	pool.Exec(ctx, "DROP TRIGGER image_fault ON hostelhive.students")
	pool.Exec(ctx, "DROP FUNCTION "+function+"()")
	if !errors.Is(uploadErr, d.ErrUnavailable) {
		t.Fatal("false attach success", uploadErr)
	}
	retained, _ = r.GetImage(ctx, profile.StudentID)
	if retained.Key != first.Key {
		t.Fatal("failed commit lost image")
	}
	if _, err = images.Upload(ctx, uids[0], profile.StudentID, "image/png", raw); err != nil {
		t.Fatal("replacement", err)
	}
	current, _ := r.GetImage(ctx, profile.StudentID)
	if current.Key == first.Key {
		t.Fatal("replacement reused key")
	}
	// Make reserved orphan rows due, then retry cleanup after a provider outage.
	if _, err = pool.Exec(ctx, "UPDATE hostelhive.student_image_objects SET cleanup_at=clock_timestamp() WHERE student_id=$1 AND cleanup_at IS NOT NULL", profile.StudentID); err != nil {
		t.Fatal(err)
	}
	objects.failDelete = true
	if worked, e := r.CleanupOne(ctx, objects); !worked || !errors.Is(e, d.ErrUnavailable) {
		t.Fatal("cleanup falsely acknowledged", e)
	}
	objects.failDelete = false
	pool.Exec(ctx, "UPDATE hostelhive.student_image_objects SET cleanup_at=clock_timestamp() WHERE student_id=$1 AND cleanup_at IS NOT NULL", profile.StudentID)
	for range 10 {
		worked, e := r.CleanupOne(ctx, objects)
		if e != nil {
			t.Fatal(e)
		}
		if !worked {
			break
		}
	}
	if _, _, err = images.Get(ctx, profile.StudentID); err != nil {
		t.Fatal("cleanup removed current image", err)
	}
	var pending int
	if pool.QueryRow(ctx, "SELECT count(*) FROM hostelhive.student_image_objects WHERE student_id=$1 AND cleanup_at IS NOT NULL", profile.StudentID).Scan(&pending) != nil || pending != 0 {
		t.Fatal("orphan cleanup incomplete")
	}

	// Concurrent replacements produce one current reference; all superseded
	// objects remain discoverable by the durable cleanup worker.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := images.Upload(ctx, uids[1], profile.StudentID, "image/png", raw)
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatal("concurrent replacement", e)
		}
	}
	for range 10 {
		worked, e := r.CleanupOne(ctx, objects)
		if e != nil {
			t.Fatal(e)
		}
		if !worked {
			break
		}
	}
	current, _ = r.GetImage(ctx, profile.StudentID)
	// Even an incorrectly queued current object must not be deleted.
	pool.Exec(ctx, "UPDATE hostelhive.student_image_objects SET cleanup_at=clock_timestamp() WHERE object_key=$1", current.Key)
	if _, err = r.CleanupOne(ctx, objects); err != nil {
		t.Fatal(err)
	}
	if _, _, err = images.Get(ctx, profile.StudentID); err != nil {
		t.Fatal("referenced image cleaned", err)
	}
	// A cleanup attempt skips a reservation locked by an in-flight upload.
	reserved := d.Image{Key: "student-images/" + profile.StudentID + "/" + strings.Repeat("a", 32), ContentType: "image/png", Size: len(raw), Width: 2, Height: 2}
	if err = r.ReserveImage(ctx, uids[1], profile.StudentID, reserved); err != nil {
		t.Fatal(err)
	}
	pool.Exec(ctx, "UPDATE hostelhive.student_image_objects SET cleanup_at=clock_timestamp() WHERE object_key=$1", reserved.Key)
	work, e := r.BeginImage(ctx, uids[1], profile.StudentID, reserved.Key)
	if e != nil {
		t.Fatal(e)
	}
	worked, e := r.CleanupOne(ctx, objects)
	work.Close()
	if e != nil || worked {
		t.Fatal("cleanup did not skip upload lock", e)
	}
	if _, err = r.CleanupOne(ctx, objects); err != nil {
		t.Fatal(err)
	}
	if err = images.Remove(ctx, uids[1], profile.StudentID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = images.Get(ctx, profile.StudentID); !errors.Is(err, d.ErrNotFound) {
		t.Fatal("removed image still available")
	}
	if err = images.Remove(ctx, uids[0], profile.StudentID); err != nil {
		t.Fatal("remove not idempotent", err)
	}
	if _, err = r.CleanupOne(ctx, objects); err != nil {
		t.Fatal(err)
	}
	if _, err = images.Upload(ctx, uids[1], profile.StudentID, "image/png", raw); err != nil {
		t.Fatal(err)
	}
	if err = profiles.Delete(ctx, uids[1], profile.StudentID); err != nil {
		t.Fatal("Warden delete", err)
	}
	if _, _, err = images.Get(ctx, profile.StudentID); !errors.Is(err, d.ErrNotFound) {
		t.Fatal("deleted profile image readable")
	}
	if _, err = r.CleanupOne(ctx, objects); err != nil {
		t.Fatal(err)
	}
	objects.mu.Lock()
	remaining := len(objects.data)
	objects.mu.Unlock()
	if remaining != 0 {
		t.Fatal("objects leaked", remaining)
	}
}
