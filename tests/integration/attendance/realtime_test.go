package attendance_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/handler"
	repo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/repository"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/service"
	ir "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type publicationFake struct {
	fail          bool
	projection    d.Projection
	grants        map[string]d.Grant
	duringPublish func()
}

func (p *publicationFake) Publish(_ context.Context, value d.Projection) error {
	if p.duringPublish != nil {
		fn := p.duringPublish
		p.duringPublish = nil
		fn()
	}
	if p.fail {
		return errors.New("provider unavailable")
	}
	if value.Version > p.projection.Version {
		p.projection = value
	}
	return nil
}
func (p *publicationFake) SetGrant(_ context.Context, uid string, value d.Grant) error {
	if p.fail {
		return errors.New("provider unavailable")
	}
	if p.grants == nil {
		p.grants = map[string]d.Grant{}
	}
	if value.Version > p.grants[uid].Version {
		p.grants[uid] = value
	}
	return nil
}

func TestAttendanceRealtimePostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable database migrated to version 14")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	prefix := "realtime60-" + uuid.NewString()
	var adminID, wardenID, userID, studentID string
	insert := func(uid, role string) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, "INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text", uid, uid+"@example.invalid", role).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	adminUID, wardenUID, studentUID := prefix+"-admin", prefix+"-warden", prefix+"-student"
	adminID = insert(adminUID, "admin")
	wardenID = insert(wardenUID, "warden")
	userID = insert(studentUID, "student")
	defer func() {
		clean, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		for _, sql := range []string{
			"DELETE FROM hostelhive.student_presence WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE user_id=ANY($1::uuid[]))",
			"DELETE FROM hostelhive.attendance_events WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE user_id=ANY($1::uuid[]))",
			"DELETE FROM hostelhive.student_qr WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE user_id=ANY($1::uuid[]))",
			"DELETE FROM hostelhive.students WHERE user_id=ANY($1::uuid[])",
			"DELETE FROM hostelhive.scanner_device_audit WHERE actor_id=ANY($1::uuid[])",
			"DELETE FROM hostelhive.scanner_devices WHERE created_by=ANY($1::uuid[])",
			"DELETE FROM hostelhive.users WHERE user_id=ANY($1::uuid[])",
		} {
			if _, err := pool.Exec(clean, sql, []string{adminID, wardenID, userID}); err != nil {
				t.Error(err)
			}
		}
		if _, err := pool.Exec(clean, "DELETE FROM hostelhive.realtime_staff_grants WHERE firebase_uid=ANY($1::text[])", []string{adminUID, wardenUID, studentUID}); err != nil {
			t.Error(err)
		}
	}()
	store := repo.NewDevices(pool)
	publisher := &publicationFake{}
	readStatus := func() d.PublicationStatus {
		t.Helper()
		v, err := store.PublicationStatus(ctx, adminUID)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	before := readStatus().Version
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", userID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if readStatus().Version != before {
		t.Fatal("rolled-back mutation queued publication")
	}
	// Student fixture schema matches required details, without QR or allocation.
	if err = pool.QueryRow(ctx, `INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone)
 VALUES($1,$2,'Realtime Test','Science',1,'0771234567') RETURNING student_id::text`, userID, prefix).Scan(&studentID); err != nil {
		t.Fatal(err)
	}
	if readStatus().Version <= before {
		t.Fatal("student creation not queued")
	}
	hash := sha256.Sum256([]byte(studentID))
	token := hex.EncodeToString(hash[:])
	exec("INSERT INTO hostelhive.student_qr(student_id,token) VALUES($1,$2)", studentID, token)
	devices := svc.NewDevices(store)
	issued, err := devices.Register(ctx, adminUID, dto.RegisterDevice{Name: prefix + "-entry"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SetDirection(ctx, adminUID, issued.Device.DeviceID, "IN"); err != nil {
		t.Fatal(err)
	}
	scans := svc.NewScans(store, 10*time.Second)
	scan := func() d.Event {
		t.Helper()
		event, err := scans.Record(ctx, issued.Device, dto.Scan{EventID: uuid.NewString(), QR: "HH1:" + token})
		if err != nil {
			t.Fatal(err)
		}
		return event
	}
	queuedBeforeScan := readStatus().Version
	if event := scan(); event.Outcome != "recorded" || readStatus().Version <= queuedBeforeScan {
		t.Fatal("accepted scan did not queue publication", event)
	}
	publisher.fail = true
	worked, err := store.PublishOne(ctx, publisher)
	if !worked || !errors.Is(err, d.ErrUnavailable) {
		t.Fatal(worked, err)
	}
	failed := readStatus()
	if !failed.Pending || failed.Attempts < 1 {
		t.Fatal("failure not durable", failed)
	}
	publisher.fail = false
	exec("UPDATE hostelhive.attendance_publication SET next_attempt_at=clock_timestamp()")
	if worked, err = store.PublishOne(ctx, publisher); !worked || err != nil {
		t.Fatal(worked, err)
	}
	published := readStatus()
	if published.Pending || published.PublishedVersion == nil || *published.PublishedVersion != publisher.projection.Version {
		t.Fatal(published)
	}
	if publisher.projection.Summary.In != 1 {
		t.Fatal("IN scan not projected", publisher.projection)
	}
	if _, err = store.SetDirection(ctx, adminUID, issued.Device.DeviceID, "OUT"); err != nil {
		t.Fatal(err)
	}
	if event := scan(); event.Outcome != "recorded" {
		t.Fatal(event)
	}
	if _, err = store.PublishOne(ctx, publisher); err != nil {
		t.Fatal(err)
	}
	if publisher.projection.Summary.Out != 1 || publisher.projection.Summary.In != 0 {
		t.Fatal("OUT scan not projected", publisher.projection)
	}
	beforeDuplicate := readStatus().Version
	if event := scan(); event.Outcome != "cooldown" || readStatus().Version != beforeDuplicate {
		t.Fatal("ignored scan changed projection", event)
	}
	baseline := publisher.projection.Summary.Total
	// A newer mutation commits while Firebase publishes an older snapshot.
	exec("UPDATE hostelhive.students SET full_name='Realtime Changed' WHERE student_id=$1", studentID)
	publisher.duringPublish = func() { exec("UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", userID) }
	if _, err = store.PublishOne(ctx, publisher); err != nil {
		t.Fatal(err)
	}
	if !readStatus().Pending {
		t.Fatal("older acknowledgement cleared newer revision")
	}
	if _, err = store.PublishOne(ctx, publisher); err != nil {
		t.Fatal(err)
	}
	if publisher.projection.Summary.Total != baseline-1 {
		t.Fatal("inactive student still counted")
	}
	exec("UPDATE hostelhive.users SET is_active=true WHERE user_id=$1", userID)
	exec("UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1", studentID, adminID)
	if _, err = store.PublishOne(ctx, publisher); err != nil {
		t.Fatal(err)
	}
	if publisher.projection.Summary.Total != baseline-1 {
		t.Fatal("archived student still counted")
	}
	// Restarted worker is a new repository instance reading the durable row.
	exec("UPDATE hostelhive.attendance_publication SET published_at=clock_timestamp()-interval '61 seconds'")
	restarted := repo.NewDevices(pool)
	if err = restarted.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if !readStatus().Pending {
		t.Fatal("reconciliation did not queue repair")
	}
	if _, err = restarted.PublishOne(ctx, publisher); err != nil {
		t.Fatal(err)
	}
	for _, uid := range []string{adminUID, wardenUID} {
		session, err := store.GrantSession(ctx, uid, publisher)
		if err != nil || time.Until(session.ExpiresAt) < 4*time.Minute || publisher.grants[uid].ExpiresAt == 0 {
			t.Fatal(session, err)
		}
	}
	if _, err = store.GrantSession(ctx, studentUID, publisher); !errors.Is(err, d.ErrForbidden) {
		t.Fatal("student received grant", err)
	}
	publisher.fail = true
	if _, err = store.GrantSession(ctx, wardenUID, publisher); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal(err)
	}
	exec("UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", wardenID)
	if _, err = store.SyncGrant(ctx, publisher, wardenUID); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal(err)
	}
	if readStatus().PendingGrants < 1 {
		t.Fatal("revocation not pending")
	}
	if worked, err := store.PublishOne(ctx, publisher); worked || err != nil {
		t.Fatal("publication continued before revocation", worked, err)
	}
	publisher.fail = false
	exec("UPDATE hostelhive.realtime_staff_grants SET next_attempt_at=clock_timestamp() WHERE firebase_uid=$1", wardenUID)
	if _, err = store.SyncGrant(ctx, publisher, wardenUID); err != nil {
		t.Fatal(err)
	}
	if publisher.grants[wardenUID].ExpiresAt != 0 {
		t.Fatal("deactivated grant retained")
	}
	if _, err = store.GrantSession(ctx, wardenUID, publisher); !errors.Is(err, d.ErrForbidden) {
		t.Fatal(err)
	}
	exec("UPDATE hostelhive.users SET is_active=true WHERE user_id=$1", wardenID)
	if publisher.grants[wardenUID].ExpiresAt != 0 {
		t.Fatal("reactivation automatically granted access")
	}
	if _, err = store.GrantSession(ctx, wardenUID, publisher); err != nil {
		t.Fatal(err)
	}
	exec("UPDATE hostelhive.users SET role='student' WHERE user_id=$1", wardenID)
	if _, err = store.SyncGrant(ctx, publisher, wardenUID); err != nil {
		t.Fatal(err)
	}
	if publisher.grants[wardenUID].ExpiresAt != 0 {
		t.Fatal("role downgrade retained grant")
	}
	exec("UPDATE hostelhive.realtime_staff_grants SET expires_at=clock_timestamp()-interval '1 second',pending=false WHERE firebase_uid=$1", adminUID)
	if _, err = store.SyncGrant(ctx, publisher, adminUID); err != nil {
		t.Fatal(err)
	}
	if publisher.grants[adminUID].ExpiresAt != 0 {
		t.Fatal("expiry not actively invalidated")
	}
	// Real middleware and routing: no Firebase calls, fake verified UID only.
	mux := http.NewServeMux()
	protect := m.Middleware(realtimeVerifier{}, ir.NewPostgresAccounts(pool), time.Second)
	attendance.RegisterRealtime(h.NewRealtimeAPI(svc.NewRealtime(store, publisher, "https://demo.firebaseio.com"), time.Second), mux, protect)
	request := func(method, path, uid, body string) int {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if uid != "" {
			r.Header.Set("Authorization", "Bearer "+uid)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Code
	}
	if got := request("POST", "/api/v1/attendance/realtime/session", adminUID, ""); got != 200 {
		t.Fatal(got)
	}
	if got := request("GET", "/api/v1/attendance/realtime/status", adminUID, ""); got != 200 {
		t.Fatal(got)
	}
	if got := request("POST", "/api/v1/attendance/realtime/session", studentUID, ""); got != 403 {
		t.Fatal(got)
	}
	if got := request("POST", "/api/v1/attendance/realtime/session", "", ""); got != 401 {
		t.Fatal(got)
	}
	if got := request("POST", "/api/v1/attendance/realtime/session?uid=other", adminUID, ""); got != 400 {
		t.Fatal(got)
	}
	if got := request("POST", "/api/v1/attendance/realtime/session", adminUID, "{}"); got != 400 {
		t.Fatal(got)
	}
	disabled := svc.NewRealtime(store, nil, "")
	if _, err = disabled.Session(ctx, adminUID); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal(err)
	}
	status, err := disabled.Status(ctx, adminUID)
	if err != nil || status.Enabled {
		t.Fatal(status, err)
	}
}

type realtimeVerifier struct{}

func (realtimeVerifier) Verify(_ context.Context, token string) (string, error) { return token, nil }
