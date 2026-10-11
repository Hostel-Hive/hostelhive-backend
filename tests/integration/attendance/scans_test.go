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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAttendanceScansPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL migrated to version 13")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := "scan55-" + uuid.NewString()
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
			"DELETE FROM hostelhive.student_presence WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE user_id=ANY($1::uuid[]))",
			"DELETE FROM hostelhive.attendance_events WHERE device_id IN(SELECT device_id FROM hostelhive.scanner_devices WHERE created_by=ANY($1::uuid[]))",
			"DELETE FROM hostelhive.student_qr WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE user_id=ANY($1::uuid[]))",
			"DELETE FROM hostelhive.students WHERE user_id=ANY($1::uuid[])",
			"DELETE FROM hostelhive.scanner_device_audit WHERE actor_id=ANY($1::uuid[])",
			"DELETE FROM hostelhive.scanner_devices WHERE created_by=ANY($1::uuid[])",
			"DELETE FROM hostelhive.users WHERE user_id=ANY($1::uuid[])",
		} {
			if _, err := pool.Exec(clean, sql, ids); err != nil {
				t.Error("fixture cleanup", err)
			}
		}
	}()
	for i, role := range []string{"admin", "warden", "student", "security_staff", "sub_warden", "student"} {
		uid := fmt.Sprintf("%s-%d", prefix, i)
		var id string
		err := pool.QueryRow(ctx, "INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text", uid, uid+"@example.invalid", role).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		uids = append(uids, uid)
	}
	students, tokens := []string{}, []string{}
	for _, i := range []int{2, 5} {
		id := uuid.NewString()
		hash := sha256.Sum256([]byte(id))
		token := fmt.Sprintf("%x", hash)
		exec("INSERT INTO hostelhive.students(student_id,user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,$3,'Test Student','Test Faculty',1,'0771234567')", id, ids[i], id)
		exec("INSERT INTO hostelhive.student_qr(student_id,token) VALUES($1,$2)", id, token)
		students = append(students, id)
		tokens = append(tokens, token)
	}
	store := repo.NewDevices(pool)
	devices := svc.NewDevices(store)
	scans := svc.NewScans(store, 10*time.Second)
	deviceAPI := h.New(devices, 5*time.Second, time.Second)
	mux := http.NewServeMux()
	protected := m.Middleware(verifier{}, identityrepo.NewPostgresAccounts(pool), time.Second)
	attendance.Register(deviceAPI, mux, protected)
	attendance.RegisterScans(h.NewScanAPI(scans, 5*time.Second), deviceAPI, mux, protected)
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
			t.Fatalf("%s %s got %d want %d: %s", method, path, w.Code, status, w.Body.String())
		}
		return w
	}
	register := func(name string) dto.IssuedCredential {
		t.Helper()
		v, err := devices.Register(ctx, uids[0], dto.RegisterDevice{Name: prefix + name})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	entry, exit, other := register("entry"), register("exit"), register("other")
	set := func(device dto.IssuedCredential, direction string) {
		t.Helper()
		path := "/api/v1/scanner-devices/" + device.Device.DeviceID + "/direction"
		w := do("PUT", path, `{"direction":"`+direction+`"}`, uids[0], 200)
		var configured d.Device
		if json.Unmarshal(w.Body.Bytes(), &configured) != nil || configured.Direction == nil || *configured.Direction != direction {
			t.Fatal("direction response")
		}
	}
	body := func(event, token string) string { return `{"event_id":"` + event + `","qr":"HH1:` + token + `"}` }
	scan := func(device dto.IssuedCredential, event, token string) d.Event {
		t.Helper()
		w := do("POST", "/api/v1/device/scans", body(event, token), device.Credential, 200)
		var e d.Event
		if json.Unmarshal(w.Body.Bytes(), &e) != nil {
			t.Fatal("scan response")
		}
		return e
	}
	presence := func(status string) {
		t.Helper()
		w := do("GET", "/api/v1/attendance/presence?student_id="+students[0], "", uids[1], 200)
		var p d.Presence
		if json.Unmarshal(w.Body.Bytes(), &p) != nil || p.Status != status {
			t.Fatalf("presence %s", w.Body.String())
		}
	}
	presence("UNKNOWN")
	do("POST", "/api/v1/device/scans", body(uuid.NewString(), tokens[0]), entry.Credential, 409)
	set(entry, "IN")
	set(exit, "OUT")
	set(other, "IN")
	// Repeating direction configuration is idempotent and only Admin can change it.
	set(entry, "IN")
	var auditCount int
	if pool.QueryRow(ctx, "SELECT count(*) FROM hostelhive.scanner_device_audit WHERE device_id=$1 AND action='direction_set'", entry.Device.DeviceID).Scan(&auditCount) != nil || auditCount != 1 {
		t.Fatal("direction audit not idempotent")
	}
	for _, uid := range uids[1:5] {
		do("PUT", "/api/v1/scanner-devices/"+entry.Device.DeviceID+"/direction", `{"direction":"OUT"}`, uid, 403)
	}
	do("PUT", "/api/v1/scanner-devices/"+entry.Device.DeviceID+"/direction", `{"direction":"SIDE"}`, uids[0], 400)
	do("PUT", "/api/v1/scanner-devices/"+uuid.NewString()+"/direction", `{"direction":"IN"}`, uids[0], 404)
	start := time.Now().Add(-time.Second)
	eventID := uuid.NewString()
	first := scan(entry, eventID, tokens[0])
	if first.Outcome != "recorded" || first.Direction != "IN" || first.StudentID != students[0] || first.OccurredAt.Before(start) || first.OccurredAt.After(time.Now().Add(time.Second)) {
		t.Fatal("first event", first)
	}
	presence("IN")
	repeated := scan(entry, eventID, tokens[0])
	if repeated != first {
		t.Fatal("retry changed result")
	}
	do("POST", "/api/v1/device/scans", body(eventID, tokens[1]), entry.Credential, 409)
	cooled := scan(other, uuid.NewString(), tokens[0])
	if cooled.Outcome != "cooldown" {
		t.Fatal("cross-device cooldown", cooled)
	}
	presence("IN")
	left := scan(exit, uuid.NewString(), tokens[0])
	if left.Outcome != "recorded" || left.Direction != "OUT" {
		t.Fatal("legitimate opposite direction blocked")
	}
	presence("OUT")
	// After the cooldown, duplicate direction still never toggles presence.
	exec("UPDATE hostelhive.attendance_events SET occurred_at=occurred_at-interval '11 seconds' WHERE student_id=$1", students[0])
	already := scan(exit, uuid.NewString(), tokens[0])
	if already.Outcome != "already_out" {
		t.Fatal("already out", already)
	}
	presence("OUT")
	again := scan(entry, uuid.NewString(), tokens[0])
	if again.Outcome != "recorded" {
		t.Fatal("re-entry", again)
	}
	presence("IN")
	exec("UPDATE hostelhive.attendance_events SET occurred_at=occurred_at-interval '11 seconds' WHERE student_id=$1", students[0])
	if scan(entry, uuid.NewString(), tokens[0]).Outcome != "already_in" {
		t.Fatal("already in")
	}
	// Reconfiguration never reinterprets a previously committed event.
	set(entry, "OUT")
	retry := scan(entry, eventID, tokens[0])
	if retry.Direction != "IN" || retry.Outcome != "recorded" {
		t.Fatal("direction changed old result")
	}
	set(entry, "IN")
	// First scan can be an exit; UNKNOWN must not invent an earlier entry.
	if scan(exit, uuid.NewString(), tokens[1]).Outcome != "recorded" {
		t.Fatal("first exit rejected")
	}
	// Concurrent same-ID requests commit exactly one result for a second student.
	concurrentID := uuid.NewString()
	input := dto.Scan{EventID: concurrentID, QR: "HH1:" + tokens[1]}
	current, err := devices.Authenticate(ctx, entry.Credential)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan d.Event, 6)
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); e, err := scans.Record(ctx, current, input); results <- e; errs <- err }()
	}
	wg.Wait()
	close(results)
	close(errs)
	var saved d.Event
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for e := range results {
		if saved.EventID == "" {
			saved = e
		}
		if e != saved {
			t.Fatal("concurrent retry differs")
		}
	}
	var count int
	if pool.QueryRow(ctx, "SELECT count(*) FROM hostelhive.attendance_events WHERE device_id=$1 AND event_id=$2", entry.Device.DeviceID, concurrentID).Scan(&count) != nil || count != 1 {
		t.Fatal("duplicate events")
	}
	// Different events on different readers serialize on the student row.
	exec("UPDATE hostelhive.attendance_events SET occurred_at=occurred_at-interval '11 seconds' WHERE student_id=$1", students[1])
	if scan(exit, uuid.NewString(), tokens[1]).Outcome != "recorded" {
		t.Fatal("concurrency baseline OUT failed")
	}
	otherCurrent, err := devices.Authenticate(ctx, other.Credential)
	if err != nil {
		t.Fatal(err)
	}
	movements := make(chan d.Event, 2)
	movementErrors := make(chan error, 2)
	for _, reader := range []d.Device{current, otherCurrent} {
		wg.Add(1)
		go func(device d.Device) {
			defer wg.Done()
			e, err := scans.Record(ctx, device, dto.Scan{EventID: uuid.NewString(), QR: "HH1:" + tokens[1]})
			movements <- e
			movementErrors <- err
		}(reader)
	}
	wg.Wait()
	close(movements)
	close(movementErrors)
	for err := range movementErrors {
		if err != nil {
			t.Fatal(err)
		}
	}
	recorded, ignored := 0, 0
	for e := range movements {
		if e.Outcome == "recorded" {
			recorded++
		} else if e.Outcome == "cooldown" {
			ignored++
		}
	}
	if recorded != 1 || ignored != 1 {
		t.Fatalf("concurrent readers: recorded=%d ignored=%d", recorded, ignored)
	}
	// Current account and profile eligibility required for every NEW event.
	exec("UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", ids[2])
	do("POST", "/api/v1/device/scans", body(uuid.NewString(), tokens[0]), entry.Credential, 422)
	exec("UPDATE hostelhive.users SET is_active=true,role='warden' WHERE user_id=$1", ids[2])
	do("POST", "/api/v1/device/scans", body(uuid.NewString(), tokens[0]), entry.Credential, 422)
	exec("UPDATE hostelhive.users SET role='student' WHERE user_id=$1", ids[2])
	exec("UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1", students[0], ids[0])
	do("POST", "/api/v1/device/scans", body(uuid.NewString(), tokens[0]), entry.Credential, 422)
	// Archived attendance is retained for Admin/Warden; retries return history.
	presence("IN")
	scan(entry, eventID, tokens[0])
	do("POST", "/api/v1/device/scans", body(uuid.NewString(), strings.Repeat("0", 64)), entry.Credential, 422)
	do("POST", "/api/v1/device/scans", `{"event_id":"bad","qr":"HH1:bad"}`, entry.Credential, 400)
	do("POST", "/api/v1/device/scans", body(uuid.NewString(), tokens[1]), uids[0], 401)
	do("POST", "/api/v1/device/scans", body(uuid.NewString(), tokens[1]), "", 401)
	do("POST", "/api/v1/device/scans", `{"event_id":"`+uuid.NewString()+`","qr":"HH1:`+tokens[1]+`","occurred_at":"2020-01-01"}`, entry.Credential, 400)
	for _, uid := range []string{uids[2], uids[3], uids[4]} {
		do("GET", "/api/v1/attendance/events?student_id="+students[0], "", uid, 403)
	}
	do("GET", "/api/v1/attendance/presence?student_id="+students[0], "", entry.Credential, 401)
	do("GET", "/api/v1/attendance/presence", "", uids[0], 400)
	do("GET", "/api/v1/attendance/presence?student_id="+uuid.NewString(), "", uids[0], 404)
	do("GET", "/api/v1/attendance/events?student_id="+students[0]+"&limit=0", "", uids[0], 400)
	do("GET", "/api/v1/attendance/events?student_id="+students[0]+"&limit=1&limit=2", "", uids[0], 400)
	w := do("GET", "/api/v1/attendance/events?student_id="+students[0]+"&limit=1", "", uids[0], 200)
	var page d.EventPage
	if json.Unmarshal(w.Body.Bytes(), &page) != nil || !page.HasMore || len(page.Items) != 1 || strings.Contains(w.Body.String(), tokens[0]) {
		t.Fatal("history pagination or leaked QR")
	}
	// A service-authenticated device becomes stale when credential rotates.
	if _, err := devices.Rotate(ctx, uids[0], entry.Device.DeviceID); err != nil {
		t.Fatal(err)
	}
	if _, err := scans.Record(ctx, current, dto.Scan{EventID: uuid.NewString(), QR: "HH1:" + tokens[1]}); !errors.Is(err, d.ErrUnauthorized) {
		t.Fatal("stale authentication allowed", err)
	}
	do("POST", "/api/v1/device/scans", body(uuid.NewString(), tokens[1]), entry.Credential, 401)
	exitCurrent, err := devices.Authenticate(ctx, exit.Credential)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := devices.Deactivate(ctx, uids[0], exit.Device.DeviceID); err != nil {
		t.Fatal(err)
	}
	if _, err := scans.Record(ctx, exitCurrent, dto.Scan{EventID: uuid.NewString(), QR: "HH1:" + tokens[1]}); !errors.Is(err, d.ErrUnauthorized) {
		t.Fatal("deactivated scanner allowed", err)
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err := scans.Presence(canceled, uids[0], students[0]); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal("database failure did not fail closed")
	}
}
