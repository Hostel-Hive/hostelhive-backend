package attendance_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/handler"
	repo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/repository"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/service"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type verifier struct{}

func (verifier) Verify(_ context.Context, token string) (string, error) {
	if strings.HasPrefix(token, "hhd1.") {
		return "", errors.New("not a Firebase token")
	}
	return token, nil
}

func TestScannerDevicePostgresLifecycle(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL with current migrations")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	prefix := fmt.Sprintf("device53-%d", time.Now().UnixNano())
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
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	ids, uids := []string{}, []string{}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, sql := range []string{
			"DELETE FROM hostelhive.student_presence WHERE device_id IN(SELECT device_id FROM hostelhive.scanner_devices WHERE created_by=ANY($1::uuid[]))",
			"DELETE FROM hostelhive.attendance_events WHERE device_id IN(SELECT device_id FROM hostelhive.scanner_devices WHERE created_by=ANY($1::uuid[]))",
			"DELETE FROM hostelhive.students WHERE user_id=ANY($1::uuid[])",
			"DELETE FROM hostelhive.scanner_device_audit WHERE device_id IN(SELECT device_id FROM hostelhive.scanner_devices WHERE created_by=ANY($1::uuid[]))",
			"DELETE FROM hostelhive.scanner_devices WHERE created_by=ANY($1::uuid[])",
			"DELETE FROM hostelhive.users WHERE user_id=ANY($1::uuid[])",
		} {
			if _, err := pool.Exec(clean, sql, ids); err != nil {
				t.Error("fixture cleanup failed")
			}
		}
	}()
	for i, role := range []string{"admin", "warden", "student", "security_staff", "sub_warden"} {
		uid := fmt.Sprintf("%s-%d", prefix, i)
		var id string
		if err := pool.QueryRow(ctx, "INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text", uid, uid+"@example.invalid", role).Scan(&id); err != nil {
			t.Fatal(err)
		}
		uids, ids = append(uids, uid), append(ids, id)
	}
	store := repo.NewDevices(pool)
	service := svc.NewDevices(store)
	mux := http.NewServeMux()
	attendance.Register(h.New(service, 5*time.Second, time.Second), mux, m.Middleware(verifier{}, identityrepo.NewPostgresAccounts(pool), time.Second))
	do := func(method, path, body, token string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("HTTP %d expected %d at %s", w.Code, status, path)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cacheable device response")
		}
		return w
	}
	var issued dto.IssuedCredential
	created := do("POST", "/api/v1/scanner-devices", `{"name":"`+prefix+`"}`, uids[0], 201)
	if json.Unmarshal(created.Body.Bytes(), &issued) != nil || issued.Credential == "" || !issued.Device.IsActive || issued.Device.CreatedBy != ids[0] {
		t.Fatal("registration response invalid")
	}
	id := issued.Device.DeviceID
	var persisted []byte
	if err := pool.QueryRow(ctx, "SELECT credential_hash FROM hostelhive.scanner_devices WHERE device_id=$1", id).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(issued.Credential))
	if string(persisted) != string(hash[:]) {
		t.Fatal("hash-only persistence failed")
	}
	do("GET", "/api/v1/device/check", "", issued.Credential, 200)
	listed := do("GET", "/api/v1/scanner-devices?limit=1", "", uids[0], 200)
	if strings.Contains(listed.Body.String(), issued.Credential) || strings.Contains(listed.Body.String(), "credential_hash") || strings.Contains(listed.Body.String(), `"credential":`) {
		t.Fatal("listing exposed credentials")
	}
	do("POST", "/api/v1/scanner-devices", `{"name":"`+strings.ToUpper(prefix)+`"}`, uids[0], 409)
	do("GET", "/api/v1/device/check", "", uids[0], 401)
	do("GET", "/api/v1/scanner-devices", "", issued.Credential, 401)
	do("GET", "/api/v1/device/check", "", "", 401)
	for _, uid := range uids[1:] {
		do("GET", "/api/v1/scanner-devices", "", uid, 403)
		do("POST", "/api/v1/scanner-devices", `{"name":"another"}`, uid, 403)
	}
	do("POST", "/api/v1/scanner-devices", `{"name":" "}`, uids[0], 400)
	do("POST", "/api/v1/scanner-devices/bad/deactivate", "", uids[0], 400)
	do("POST", "/api/v1/scanner-devices/00000000-0000-0000-0000-000000000000/deactivate", "", uids[0], 404)
	do("GET", "/api/v1/scanner-devices?limit=0", "", uids[0], 400)
	do("GET", "/api/v1/scanner-devices?offset=-1", "", uids[0], 400)
	parts := strings.Split(issued.Credential, ".")
	parts[2] = strings.Repeat("A", 43)
	do("GET", "/api/v1/device/check", "", strings.Join(parts, "."), 401)
	// Committed rotation invalidates the old credential without caching.
	var rotated dto.IssuedCredential
	w := do("POST", "/api/v1/scanner-devices/"+id+"/rotate-credential", "", uids[0], 200)
	if json.Unmarshal(w.Body.Bytes(), &rotated) != nil || rotated.Device.CredentialVersion != 2 || rotated.Credential == issued.Credential {
		t.Fatal("rotation response invalid")
	}
	do("GET", "/api/v1/device/check", "", issued.Credential, 401)
	do("GET", "/api/v1/device/check", "", rotated.Credential, 200)
	// A newly constructed service authenticates the same database verifier.
	if _, err := svc.NewDevices(repo.NewDevices(pool)).Authenticate(ctx, rotated.Credential); err != nil {
		t.Fatal("authentication not persisted")
	}
	// Concurrent rotations serialize versions; exactly the last credential works.
	var wg sync.WaitGroup
	rotations := make(chan dto.IssuedCredential, 2)
	failures := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); v, err := service.Rotate(ctx, uids[0], id); rotations <- v; failures <- err }()
	}
	wg.Wait()
	close(rotations)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	for v := range rotations {
		if v.Device.CredentialVersion == 4 {
			rotated = v
		}
	}
	if rotated.Device.CredentialVersion != 4 {
		t.Fatal("concurrent version increments lost")
	}
	do("GET", "/api/v1/device/check", "", rotated.Credential, 200)
	// Deactivation is idempotent, including its audit event and updated timestamp.
	var inactive, again d.Device
	w = do("POST", "/api/v1/scanner-devices/"+id+"/deactivate", "", uids[0], 200)
	if json.Unmarshal(w.Body.Bytes(), &inactive) != nil || inactive.IsActive {
		t.Fatal("deactivation failed")
	}
	do("GET", "/api/v1/device/check", "", rotated.Credential, 401)
	w = do("POST", "/api/v1/scanner-devices/"+id+"/deactivate", "", uids[0], 200)
	if json.Unmarshal(w.Body.Bytes(), &again) != nil || !again.UpdatedAt.Equal(inactive.UpdatedAt) {
		t.Fatal("repeated deactivation mutated device")
	}
	var auditCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM hostelhive.scanner_device_audit WHERE device_id=$1 AND actor_id=$2", id, ids[0]).Scan(&auditCount); err != nil || auditCount != 5 {
		t.Fatal("audit history missing or duplicated")
	}
	w = do("POST", "/api/v1/scanner-devices/"+id+"/rotate-credential", "", uids[0], 200)
	if json.Unmarshal(w.Body.Bytes(), &rotated) != nil || rotated.Device.IsActive {
		t.Fatal("rotation reactivated device")
	}
	do("GET", "/api/v1/device/check", "", rotated.Credential, 401)
	// Account mutation while a management request is blocked must be rechecked.
	// Reactivation preserves identity, direction and the previously recorded scan.
	if _, err := store.SetDirection(ctx, uids[0], id, "IN"); err != nil {
		t.Fatal(err)
	}
	var historyStudent, historyEvent string
	if err := pool.QueryRow(ctx, `INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Scanner Test Student','Test Faculty',1,'0771234567') RETURNING student_id::text`, ids[2], prefix).Scan(&historyStudent); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at,access_result) VALUES(gen_random_uuid(),$1,$2,$3,'IN','recorded',clock_timestamp(),'not_required') RETURNING event_id::text`, id, historyStudent, make([]byte, 32)).Scan(&historyEvent); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO hostelhive.student_presence(student_id,status,updated_at,device_id,event_id) SELECT student_id,'IN',occurred_at,device_id,event_id FROM hostelhive.attendance_events WHERE device_id=$1 AND event_id=$2`, id, historyEvent)
	snapshot := func(query string) string {
		t.Helper()
		var value string
		if err := pool.QueryRow(ctx, query, id).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	eventBefore := snapshot("SELECT row_to_json(e)::text FROM hostelhive.attendance_events e WHERE device_id=$1")
	presenceBefore := snapshot("SELECT row_to_json(p)::text FROM hostelhive.student_presence p WHERE device_id=$1")
	for _, uid := range uids[1:] {
		do("POST", "/api/v1/scanner-devices/"+id+"/reactivate", `{"confirmed":true}`, uid, 403)
	}
	do("POST", "/api/v1/scanner-devices/bad/reactivate", `{"confirmed":true}`, uids[0], 400)
	do("POST", "/api/v1/scanner-devices/00000000-0000-0000-0000-000000000000/reactivate", `{"confirmed":true}`, uids[0], 404)
	do("POST", "/api/v1/scanner-devices/"+id+"/reactivate", `{"confirmed":false}`, uids[0], 400)
	var reactivated dto.IssuedCredential
	w = do("POST", "/api/v1/scanner-devices/"+id+"/reactivate", `{"confirmed":true}`, uids[0], 200)
	if json.Unmarshal(w.Body.Bytes(), &reactivated) != nil || !reactivated.Device.IsActive || reactivated.Device.DeviceID != id || reactivated.Device.Name != issued.Device.Name || !reactivated.Device.CreatedAt.Equal(issued.Device.CreatedAt) || reactivated.Device.CreatedBy != issued.Device.CreatedBy || reactivated.Device.Direction == nil || *reactivated.Device.Direction != "IN" || reactivated.Device.CredentialVersion != rotated.Device.CredentialVersion+1 || reactivated.Credential == rotated.Credential {
		t.Fatal("reactivation changed identity/direction or did not replace credential")
	}
	do("GET", "/api/v1/device/check", "", issued.Credential, 401)
	do("GET", "/api/v1/device/check", "", rotated.Credential, 401)
	do("GET", "/api/v1/device/check", "", reactivated.Credential, 200)
	if eventBefore != snapshot("SELECT row_to_json(e)::text FROM hostelhive.attendance_events e WHERE device_id=$1") || presenceBefore != snapshot("SELECT row_to_json(p)::text FROM hostelhive.student_presence p WHERE device_id=$1") {
		t.Fatal("reactivation changed attendance history or presence")
	}
	if count := snapshot("SELECT count(*)::text FROM hostelhive.scanner_device_audit WHERE device_id=$1 AND action='reactivated'"); count != "1" {
		t.Fatal("reactivation audit missing")
	}
	beforeRepeat := snapshot("SELECT row_to_json(d)::text FROM hostelhive.scanner_devices d WHERE device_id=$1")
	w = do("POST", "/api/v1/scanner-devices/"+id+"/reactivate", `{"confirmed":true}`, uids[0], 409)
	if !strings.Contains(w.Body.String(), "scanner_already_active") || strings.Contains(w.Body.String(), "credential") || beforeRepeat != snapshot("SELECT row_to_json(d)::text FROM hostelhive.scanner_devices d WHERE device_id=$1") {
		t.Fatal("active retry changed credential or leaked secret")
	}
	do("GET", "/api/v1/device/check", "", reactivated.Credential, 200)
	// A failed audit insert rolls back activation, verifier and version together.
	do("POST", "/api/v1/scanner-devices/"+id+"/deactivate", "", uids[0], 200)
	beforeFailure := snapshot("SELECT row_to_json(d)::text FROM hostelhive.scanner_devices d WHERE device_id=$1")
	func() {
		exec(`ALTER TABLE hostelhive.scanner_device_audit ADD CONSTRAINT test_reactivation_audit_rejected CHECK(action<>'reactivated') NOT VALID`)
		defer exec(`ALTER TABLE hostelhive.scanner_device_audit DROP CONSTRAINT test_reactivation_audit_rejected`)
		v, e := service.Reactivate(ctx, uids[0], id, dto.ReactivateDevice{Confirmed: true})
		if !errors.Is(e, d.ErrUnavailable) || v.Credential != "" {
			t.Fatal("failed audit reported successful credential issuance")
		}
		if beforeFailure != snapshot("SELECT row_to_json(d)::text FROM hostelhive.scanner_devices d WHERE device_id=$1") {
			t.Fatal("audit failure did not roll back scanner state")
		}
	}()
	// Concurrent submissions must issue one usable credential, not rotate twice.
	responses := make(chan dto.IssuedCredential, 2)
	errorsFound := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := service.Reactivate(ctx, uids[0], id, dto.ReactivateDevice{Confirmed: true})
			responses <- v
			errorsFound <- e
		}()
	}
	wg.Wait()
	close(responses)
	close(errorsFound)
	successes, conflicts := 0, 0
	for e := range errorsFound {
		if e == nil {
			successes++
		} else if errors.Is(e, d.ErrAlreadyActive) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent reactivation did not serialize")
	}
	for v := range responses {
		if v.Credential != "" {
			do("GET", "/api/v1/device/check", "", v.Credential, 200)
		}
	}
	do("GET", "/api/v1/device/check", "", reactivated.Credential, 401)
	// Rollback may not discard the new security audit history.
	downSQL, e := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "000033_scanner_reactivation.down.sql"))
	if e != nil {
		t.Fatal(e)
	}
	conn, e := pool.Acquire(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, downErr := conn.Exec(ctx, string(downSQL))
	_, rollbackErr := conn.Exec(ctx, "ROLLBACK")
	conn.Release()
	if downErr == nil || !strings.Contains(downErr.Error(), "Cannot roll back scanner reactivation") || rollbackErr != nil {
		t.Fatal("rollback guard did not preserve audit history")
	}
	// Account mutation while a management request is blocked must be rechecked.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "UPDATE hostelhive.users SET role='warden' WHERE user_id=$1", ids[0]); err != nil {
		t.Fatal(err)
	}
	blocked := make(chan error, 1)
	go func() {
		_, err := service.Reactivate(ctx, uids[0], id, dto.ReactivateDevice{Confirmed: true})
		blocked <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock' AND query='LOCK TABLE hostelhive.users IN SHARE MODE')", prefix).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("management did not wait for account lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if tx.Commit(ctx) != nil {
		t.Fatal("account change commit failed")
	}
	if err := <-blocked; !errors.Is(err, d.ErrForbidden) {
		t.Fatal("stale Admin authority accepted")
	}
	exec("UPDATE hostelhive.users SET role='admin',is_active=false WHERE user_id=$1", ids[0])
	if _, err := service.Register(ctx, uids[0], dto.RegisterDevice{Name: prefix + "-inactive"}); !errors.Is(err, d.ErrForbidden) {
		t.Fatal("inactive Admin accepted")
	}
	do("GET", "/api/v1/scanner-devices", "", uids[0], 403)
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err := store.Authentication(canceled, id); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal("canceled auth failed open")
	}
}
