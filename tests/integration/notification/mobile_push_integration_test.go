package notification_test

import (
	"context"
	"encoding/json"
	"errors"

	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type pushProviderFake struct {
	mu       sync.Mutex
	messages []domain.PushMessage
	results  map[string]domain.PushResult
}

func (f *pushProviderFake) Send(_ context.Context, m domain.PushMessage) domain.PushResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, m)
	if r, ok := f.results[m.Token]; ok {
		return r
	}
	return domain.PushResult{State: "accepted", ProviderID: "projects/test/messages/" + uuid.NewString()}
}
func (f *pushProviderFake) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = nil
	f.results = map[string]domain.PushResult{}
}
func (f *pushProviderFake) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.messages) }

func TestMobilePushLifecycle(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL migrated to 25")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := "push89-" + uuid.NewString()[:8] + "-"
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	scalar := func(sql string, args ...any) string {
		t.Helper()
		var v string
		if err := pool.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	user := func(name, role string) string {
		return scalar(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text`, prefix+name, prefix+name+"@example.invalid", role)
	}
	admin, warden, owner, other := user("admin", "admin"), user("warden", "warden"), user("student", "student"), user("other", "student")
	sid := scalar(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Push Student','Science',1,'0771234567') RETURNING student_id::text`, owner, prefix)
	scanner := scalar(`INSERT INTO hostelhive.scanner_devices(device_id,name,credential_hash,created_by,updated_by,direction) VALUES(gen_random_uuid(),$1,decode(repeat('ab',32),'hex'),$2,$2,'IN') RETURNING device_id::text`, prefix, admin)
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		for _, sql := range []string{`DELETE FROM hostelhive.attendance_incidents WHERE student_id=$1`, `DELETE FROM hostelhive.attendance_events WHERE student_id=$1`, `DELETE FROM hostelhive.scanner_devices WHERE name=$2`, `DELETE FROM hostelhive.late_access_requests WHERE student_id=$1`, `DELETE FROM hostelhive.leave_requests WHERE student_id=$1`, `DELETE FROM hostelhive.students WHERE student_id=$1`} {
			// Bind only the parameters that each cleanup statement actually uses.
			var err error
			if strings.Contains(sql, "$2") {
				_, err = pool.Exec(clean, strings.ReplaceAll(sql, "$2", "$1"), prefix)
			} else {
				_, err = pool.Exec(clean, sql, sid)
			}
			if err != nil {
				t.Error(err)
			}
		}
		if _, err := pool.Exec(clean, `DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`, prefix+"%"); err != nil {
			t.Error(err)
		}
	}()
	store := repository.New(pool)
	devices := service.NewPush(store)
	provider := &pushProviderFake{}
	provider.reset()
	dispatcher := service.NewPushDispatcher(store, provider)
	token := func() string { return "FCM_TEST_" + strings.ReplaceAll(uuid.NewString(), "-", "") }
	d1, d2, wd := uuid.NewString(), uuid.NewString(), uuid.NewString()
	t1, t2 := token(), token()
	register := func(name, id, value, platform string) {
		t.Helper()
		if _, err := devices.RegisterPushDevice(ctx, prefix+name, id, dto.PushRegistration{Platform: platform, Token: value}); err != nil {
			t.Fatal(err)
		}
	}
	register("student", d1, t1, "android")
	register("student", d2, t2, "ios")
	register("warden", wd, token(), "android")
	generation := scalar(`SELECT generation::text FROM hostelhive.push_devices WHERE user_id=$1 AND device_id=$2`, owner, d1)
	register("student", d1, t1, "android")
	if scalar(`SELECT generation::text FROM hostelhive.push_devices WHERE user_id=$1 AND device_id=$2`, owner, d1) != generation {
		t.Fatal("idempotent refresh rotated binding")
	}
	if _, err := devices.RegisterPushDevice(ctx, prefix+"other", uuid.NewString(), dto.PushRegistration{Platform: "android", Token: t1}); !errors.Is(err, domain.ErrPushConflict) {
		t.Fatal("another owner bound active token", err)
	}
	if err := devices.RemovePushDevice(ctx, prefix+"other", d1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("cross-owner delete", err)
	}
	page, err := devices.ListPushDevices(ctx, prefix+"student")
	if err != nil || page.Total != 2 {
		t.Fatal(page, err)
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), t1) || strings.Contains(string(raw), "token_hash") {
		t.Fatal("token leaked")
	}
	mux := http.NewServeMux()
	auth := middleware.Middleware(verifier{}, identityrepo.NewPostgresAccounts(pool), time.Second)
	notification.RegisterPush(handler.NewPush(devices, time.Second), mux, auth)
	r := httptest.NewRequest("GET", "/api/v1/me/push-devices", nil)
	r.Header.Set("Authorization", "Bearer "+prefix+"other")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"total":0`) {
		t.Fatal("device list crossed owners", w.Code, w.Body.String())
	}
	sequence := 0
	makeLeave := func() string {
		t.Helper()
		sequence++
		id := scalar(`INSERT INTO hostelhive.leave_requests(request_id,student_id,submitted_by,starts_at,ends_at,reason) VALUES(gen_random_uuid(),$1,$2,clock_timestamp()+$3*interval '3 days',clock_timestamp()+$3*interval '3 days'+interval '1 day','PRIVATE LEAVE REASON') RETURNING leave_id::text`, sid, owner, sequence+10)
		exec(`UPDATE hostelhive.leave_requests SET status='approved',decided_at=clock_timestamp(),decided_by=$2 WHERE leave_id=$1`, id, admin)
		return id
	}
	inbox := func() {
		t.Helper()
		for i := 0; i < 100; i++ {
			n, err := store.Deliver(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if n < 100 {
				return
			}
		}
		t.Fatal("inbox did not drain")
	}
	leaveNID := func(id string) string {
		return scalar(`SELECT n.notification_id::text FROM hostelhive.notifications n JOIN hostelhive.notification_events e ON e.event_id=n.event_id WHERE e.leave_id=$1 AND e.kind='leave_decided'`, id)
	}
	countState := func(nid, state string) string {
		return scalar(`SELECT count(*)::text FROM hostelhive.push_deliveries WHERE notification_id=$1 AND state=$2`, nid, state)
	}
	drain := func() {
		t.Helper()
		for i := 0; i < 100; i++ {
			n, err := dispatcher.Dispatch(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if n == 0 {
				return
			}
		}
		t.Fatal("push queue did not settle")
	}
	sourceState := func() string {
		return scalar(`SELECT jsonb_build_object('leaves',(SELECT jsonb_agg(to_jsonb(l) ORDER BY leave_id) FROM hostelhive.leave_requests l WHERE student_id=$1),'inbox',(SELECT jsonb_agg(to_jsonb(n) ORDER BY notification_id) FROM hostelhive.notifications n WHERE recipient_id=$2),'outbox',(SELECT jsonb_agg(to_jsonb(o) ORDER BY event_id) FROM hostelhive.notification_outbox o WHERE recipient_id=$2))::text`, sid, owner)
	}
	lid := makeLeave()
	inbox()
	nid := leaveNID(lid)
	if countState(nid, "pending") != "2" {
		t.Fatal("missing per-device intent")
	}
	before := sourceState()
	drain()
	if countState(nid, "accepted") != "2" || provider.count() != 2 || sourceState() != before {
		t.Fatal("push changed inbox or source")
	}
	for _, m := range provider.messages {
		if m.NotificationID != nid || m.SourceType != "leave" || m.Kind != "leave_decided" {
			t.Fatal("unstable message identity")
		}
	}
	inbox()
	drain()
	if provider.count() != 2 {
		t.Fatal("duplicate intents or accepted sends")
	}
	// Transient failures remain durable, honor provider backoff, and terminate.
	provider.reset()
	provider.results[t1] = domain.PushResult{State: "retry", RetryAfter: 3 * time.Minute}
	provider.results[t2] = domain.PushResult{State: "retry"}
	lid = makeLeave()
	inbox()
	nid = leaveNID(lid)
	before = sourceState()
	drain()
	if countState(nid, "pending") != "2" || provider.count() != 2 || sourceState() != before {
		t.Fatal("provider failure damaged source/inbox")
	}
	var delay float64
	if err := pool.QueryRow(ctx, `SELECT EXTRACT(epoch FROM (p.next_attempt_at-clock_timestamp()))::float8 FROM hostelhive.push_deliveries p JOIN hostelhive.push_devices d USING(registration_id) WHERE p.notification_id=$1 AND d.device_id=$2`, nid, d1).Scan(&delay); err != nil || delay < 170 {
		t.Fatal("provider backoff ignored", delay, err)
	}
	exec(`UPDATE hostelhive.push_deliveries SET attempts=7,next_attempt_at=clock_timestamp() WHERE notification_id=$1`, nid)
	drain()
	if countState(nid, "failed") != "2" {
		t.Fatal("unbounded retry")
	}
	// Old generations must not be sent after rotation or logout.
	provider.reset()
	lid = makeLeave()
	inbox()
	nid = leaveNID(lid)
	t1, t2 = token(), token()
	register("student", d1, t1, "android")
	register("student", d2, t2, "ios")
	drain()
	if countState(nid, "skipped") != "2" || provider.count() != 0 {
		t.Fatal("rotation retained old targets")
	}
	lid = makeLeave()
	inbox()
	nid = leaveNID(lid)
	if err := devices.RemovePushDevice(ctx, prefix+"student", d1); err != nil {
		t.Fatal(err)
	}
	drain()
	if countState(nid, "skipped") != "1" || countState(nid, "accepted") != "1" {
		t.Fatal("logout did not suppress old target")
	}
	if scalar(`SELECT token FROM hostelhive.push_devices WHERE user_id=$1 AND device_id=$2`, owner, d1) != "" {
		t.Fatal("logout retained raw token")
	}
	// No retroactive sends to newly bound devices.
	if err := devices.RemovePushDevice(ctx, prefix+"student", d2); err != nil {
		t.Fatal(err)
	}
	lid = makeLeave()
	register("student", d1, t1, "android")
	register("student", d2, t2, "ios")
	inbox()
	nid = leaveNID(lid)
	if scalar(`SELECT count(*)::text FROM hostelhive.push_deliveries WHERE notification_id=$1`, nid) != "0" {
		t.Fatal("historical push backfill")
	}
	// Immediate current-account eligibility; tokens alone never authorize a send.
	provider.reset()
	lid = makeLeave()
	inbox()
	nid = leaveNID(lid)
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, owner)
	drain()
	exec(`UPDATE hostelhive.users SET is_active=true WHERE user_id=$1`, owner)
	if countState(nid, "skipped") != "2" || provider.count() != 0 {
		t.Fatal("inactive account received push")
	}
	// Expiry also suppresses all external calls.
	lid = makeLeave()
	inbox()
	nid = leaveNID(lid)
	exec(`UPDATE hostelhive.push_deliveries SET expires_at=clock_timestamp()-interval '1 second' WHERE notification_id=$1`, nid)
	drain()
	if countState(nid, "expired") != "2" || provider.count() != 0 {
		t.Fatal("expired push sent")
	}
	// Definitively unregistered tokens are cleared; other devices still succeed.
	provider.reset()
	provider.results[t1] = domain.PushResult{State: "invalid_token"}
	lid = makeLeave()
	inbox()
	nid = leaveNID(lid)
	drain()
	if countState(nid, "failed") != "1" || countState(nid, "accepted") != "1" {
		t.Fatal("invalid-token classification")
	}
	page, err = devices.ListPushDevices(ctx, prefix+"student")
	if err != nil || page.Total != 1 {
		t.Fatal("invalid token not disabled", page, err)
	}
	t1 = token()
	register("student", d1, t1, "android")
	// Parallel workers claim each per-device intent once.
	provider.reset()
	lid = makeLeave()
	inbox()
	nid = leaveNID(lid)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := dispatcher.Dispatch(ctx); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if provider.count() != 2 || countState(nid, "accepted") != "2" {
		t.Fatal("concurrent duplicate send")
	}
	// A crash/ambiguous acknowledgment preserves the stable ID for a retry.
	provider.reset()
	lid = makeLeave()
	inbox()
	nid = leaveNID(lid)
	abort, stop := context.WithCancel(ctx)
	seen := ""
	_, err = store.DispatchPush(abort, func(_ context.Context, m domain.PushMessage) domain.PushResult {
		seen = m.NotificationID
		stop()
		return domain.PushResult{State: "accepted", ProviderID: "ambiguous"}
	})
	if err == nil || seen != nid || countState(nid, "pending") != "2" {
		t.Fatal("ambiguous acknowledgment lost durable intent", err)
	}
	drain()
	for _, m := range provider.messages {
		if m.NotificationID != seen {
			t.Fatal("retry changed notification identity")
		}
	}
	// Late access decisions qualify; submissions and ordinary announcements do not.
	provider.reset()
	late := scalar(`INSERT INTO hostelhive.late_access_requests(request_id,student_id,submitted_by,direction,expected_at,reason) VALUES(gen_random_uuid(),$1,$2,'IN',clock_timestamp()+interval '20 days','PRIVATE LATE REASON') RETURNING late_access_id::text`, sid, owner)
	exec(`UPDATE hostelhive.late_access_requests SET status='approved',decided_at=clock_timestamp(),decided_by=$2 WHERE late_access_id=$1`, late, admin)
	inbox()
	drain()
	if provider.count() != 2 {
		t.Fatal("late-access decision did not reach devices")
	}
	for _, m := range provider.messages {
		if m.Kind != "late_access_decided" {
			t.Fatal("unexpected pushed event", m.Kind)
		}
	}
	announcement := scalar(`INSERT INTO hostelhive.announcements(request_id,published_by,title,content) VALUES(gen_random_uuid(),$1,'PRIVATE TITLE','PRIVATE CONTENT') RETURNING announcement_id::text`, admin)
	inbox()
	if scalar(`SELECT count(*)::text FROM hostelhive.push_deliveries p JOIN hostelhive.notifications n USING(notification_id) JOIN hostelhive.notification_events e USING(event_id) WHERE e.announcement_id=$1`, announcement) != "0" {
		t.Fatal("ordinary announcement pushed")
	}
	// Current staff role governs incident pushes even after recipient snapshot.
	incident := func() string {
		event := uuid.NewString()
		exec(`INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at,access_result) VALUES($1,$2,$3,decode(repeat('cd',32),'hex'),'IN','recorded',clock_timestamp(),'unapproved')`, event, scanner, sid)
		return scalar(`INSERT INTO hostelhive.attendance_incidents(device_id,event_id,student_id,direction,incident_type,occurred_at,hostel_timezone,exit_opening_minute,entry_deadline_minute,permission_window_seconds) VALUES($1,$2,$3,'IN','late_entry',clock_timestamp(),'Asia/Colombo',360,1200,1800) RETURNING incident_id::text`, scanner, event, sid)
	}
	provider.reset()
	incident()
	inbox()
	exec(`UPDATE hostelhive.users SET role='sub_warden' WHERE user_id=$1`, warden)
	drain()
	exec(`UPDATE hostelhive.users SET role='warden' WHERE user_id=$1`, warden)
	if provider.count() != 0 {
		t.Fatal("downgraded staff received incident")
	}
	provider.reset()
	incident()
	inbox()
	drain()
	if provider.count() != 1 || provider.messages[0].Kind != "attendance_incident" {
		t.Fatal("incident scope")
	}
	exec(`DELETE FROM hostelhive.announcements WHERE announcement_id=$1`, announcement)
	// Enrollment cap is serialized even with competing registrations.
	success := 0
	var mu sync.Mutex
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := devices.RegisterPushDevice(ctx, prefix+"other", uuid.NewString(), dto.PushRegistration{Platform: "android", Token: token()})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				success++
			} else if !errors.Is(err, domain.ErrPushConflict) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if success != domain.PushDeviceLimit {
		t.Fatal("device limit race", success)
	}
	// Enrollment is denied immediately to inactive accounts, even using a stale UID.
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, other)
	if _, err := devices.ListPushDevices(ctx, prefix+"other"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("stale device authority", err)
	}

}
