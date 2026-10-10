package student_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	dto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/handler"
	repo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/repository"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type qrVerifier struct{}

func (qrVerifier) Verify(_ context.Context, token string) (string, error) { return token, nil }

func TestStudentQRPostgresLifecycle(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable database migrated to version 11")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := fmt.Sprintf("qr50-%d", time.Now().UnixNano())
	roles := []string{"admin", "warden", "student", "student", "student", "security_staff", "student"}
	uids, ids := []string{}, []string{}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, sql := range []string{
			"DELETE FROM hostelhive.guardians WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE user_id=ANY($1::uuid[]))",
			"DELETE FROM hostelhive.students WHERE user_id=ANY($1::uuid[])",
			"DELETE FROM hostelhive.users WHERE user_id=ANY($1::uuid[])",
		} {
			if _, err := pool.Exec(clean, sql, ids); err != nil {
				t.Error("QR fixture cleanup failed")
			}
		}
	}()
	for i, role := range roles {
		uid := fmt.Sprintf("%s-%d", prefix, i)
		var id string
		if err := pool.QueryRow(ctx, "INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text", uid, uid+"@example.invalid", role).Scan(&id); err != nil {
			t.Fatal(err)
		}
		uids, ids = append(uids, uid), append(ids, id)
	}
	store := repo.NewStore(pool)
	profiles := []d.Profile{}
	for i := 2; i <= 4; i++ {
		input := validDetails()
		input.IndexNo = fmt.Sprintf("%s-%d", prefix, i)
		// Use the existing validated profile API service for fixture creation.
		v, err := svc.New(store).Create(ctx, uids[0], dto.CreateInput{UserID: ids[i], Details: input})
		if err != nil {
			t.Fatal(err)
		}
		profiles = append(profiles, v)
	}
	qrs := svc.NewQRs(store)
	mux := http.NewServeMux()
	student.RegisterQR(h.NewQRAPI(qrs, 5*time.Second), mux, m.Middleware(qrVerifier{}, identityrepo.NewPostgresAccounts(pool), time.Second))
	get := func(uid, path string, status int) []byte {
		t.Helper()
		r := httptest.NewRequest("GET", path, nil)
		if uid != "" {
			r.Header.Set("Authorization", "Bearer "+uid)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("QR HTTP %d expected %d: %s", w.Code, status, w.Body.String())
		}
		return w.Body.Bytes()
	}
	path := "/api/v1/students/" + profiles[0].StudentID + "/qr"
	first := get(uids[2], "/api/v1/me/qr", 200)
	if !bytes.Equal(first, get(uids[0], path, 200)) || !bytes.Equal(first, get(uids[1], path, 200)) {
		t.Fatal("self/staff QR differs")
	}
	second := get(uids[3], "/api/v1/me/qr", 200)
	if bytes.Equal(first, second) {
		t.Fatal("different students share QR")
	}
	// Database constraints defend uniqueness and payload format independently.
	_, constraintErr := pool.Exec(ctx, "UPDATE hostelhive.student_qr SET token=(SELECT token FROM hostelhive.student_qr WHERE student_id=$1) WHERE student_id=$2", profiles[0].StudentID, profiles[1].StudentID)
	var pgErr *pgconn.PgError
	if !errors.As(constraintErr, &pgErr) || pgErr.Code != "23505" {
		t.Fatal("duplicate token allowed")
	}
	_, constraintErr = pool.Exec(ctx, "UPDATE hostelhive.student_qr SET token='invalid' WHERE student_id=$1", profiles[1].StudentID)
	if !errors.As(constraintErr, &pgErr) || pgErr.Code != "23514" {
		t.Fatal("invalid token allowed")
	}
	if image, err := svc.NewQRs(repo.NewStore(pool)).PNG(ctx, uids[2], ""); err != nil || !bytes.Equal(first, image) {
		t.Fatal("QR not persisted across service recreation")
	}
	get(uids[6], "/api/v1/me/qr", 404)
	get(uids[2], "/api/v1/students/"+profiles[1].StudentID+"/qr", 403)
	get(uids[5], path, 403)
	get("", path, 401)
	get(uids[0], "/api/v1/students/00000000-0000-0000-0000-000000000000/qr", 404)
	// Concurrent first retrieval issues exactly one stable identifier.
	var wg sync.WaitGroup
	results := make(chan []byte, 4)
	errorsCh := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); image, err := qrs.PNG(ctx, uids[4], ""); results <- image; errorsCh <- err }()
	}
	wg.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	var stable []byte
	for image := range results {
		if stable == nil {
			stable = image
		} else if !bytes.Equal(stable, image) {
			t.Fatal("concurrent QRs differ")
		}
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM hostelhive.student_qr WHERE student_id=$1", profiles[2].StudentID).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate issuance")
	}
	// Fresh role and active checks happen inside persistence, not just middleware.
	if _, err := pool.Exec(ctx, "UPDATE hostelhive.users SET role='warden' WHERE user_id=$1", ids[2]); err != nil {
		t.Fatal(err)
	}
	if _, err := store.QR(ctx, uids[2], "", strings.Repeat("ab", 32)); !errors.Is(err, d.ErrForbidden) {
		t.Fatal("stale role accepted")
	}
	get(uids[0], path, 404)
	if _, err := pool.Exec(ctx, "UPDATE hostelhive.users SET role='student',is_active=false WHERE user_id=$1", ids[2]); err != nil {
		t.Fatal(err)
	}
	get(uids[2], "/api/v1/me/qr", 403)
	get(uids[0], path, 404)
	if _, err := pool.Exec(ctx, "UPDATE hostelhive.users SET is_active=true WHERE user_id=$1", ids[2]); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, get(uids[2], "/api/v1/me/qr", 200)) {
		t.Fatal("reactivation changed QR")
	}
	if err := svc.New(store).Delete(ctx, uids[0], profiles[0].StudentID); err != nil {
		t.Fatal(err)
	}
	get(uids[2], "/api/v1/me/qr", 404)
	get(uids[0], path, 404)
	if _, err := store.QR(ctx, uids[5], profiles[1].StudentID, strings.Repeat("ab", 32)); !errors.Is(err, d.ErrForbidden) {
		t.Fatal("unauthorized direct repository call")
	}
	if _, err := store.QR(ctx, "missing", "", strings.Repeat("ab", 32)); !errors.Is(err, d.ErrForbidden) {
		t.Fatal("missing actor accepted")
	}
	if _, err := pool.Exec(ctx, "UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := store.QR(ctx, uids[0], profiles[1].StudentID, strings.Repeat("ab", 32)); !errors.Is(err, d.ErrForbidden) {
		t.Fatal("inactive staff accepted")
	}
	if _, err := store.QR(ctx, uids[0], "bad", "bad"); !errors.Is(err, d.ErrInvalid) {
		t.Fatal("invalid input accepted")
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err := store.QR(canceled, uids[0], profiles[1].StudentID, strings.Repeat("ab", 32)); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal("canceled request accepted")
	}
}
