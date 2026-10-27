package leave_test

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

	adomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	adto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	arepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/repository"
	aservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/service"
	irepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	ldomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestGoingHomePostgresLifecycle(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable database migrated to27")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, e := pgxpool.New(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	prefix := fmt.Sprintf("going-home-%d-", time.Now().UnixNano())
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
	users := map[string]string{}
	for _, role := range []string{"admin", "warden", "sub_warden", "security_staff", "student", "other", "missing"} {
		actual := role
		if role == "other" || role == "missing" {
			actual = "student"
		}
		users[role] = scalar("INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text", prefix+role, prefix+role+"@example.invalid", actual)
	}
	students := map[string]string{}
	for _, role := range []string{"student", "other"} {
		students[role] = scalar("INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Going Home Student','Science',1,'0771234567') RETURNING student_id::text", users[role], prefix+role)
	}
	sid := students["student"]
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		for _, sql := range []string{
			"DELETE FROM hostelhive.going_home_declarations WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)",
			"DELETE FROM hostelhive.student_invitations WHERE user_id IN(SELECT user_id FROM hostelhive.users WHERE firebase_uid LIKE $1)",
			"DELETE FROM hostelhive.attendance_incidents WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)",
			"DELETE FROM hostelhive.student_presence WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)",
			"DELETE FROM hostelhive.attendance_events WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)",
			"DELETE FROM hostelhive.scanner_device_audit WHERE device_id IN(SELECT device_id FROM hostelhive.scanner_devices WHERE name LIKE $1)",
			"DELETE FROM hostelhive.scanner_devices WHERE name LIKE $1",
			"DELETE FROM hostelhive.students WHERE index_no LIKE $1",
			"DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1",
		} {
			if _, e := pool.Exec(clean, sql, prefix+"%"); e != nil {
				t.Error(e)
			}
		}
	}()
	store := repository.New(pool)
	s := service.NewGoingHome(store)
	uid := prefix + "student"
	input := func() dto.GoingHomeInput { return dto.GoingHomeInput{RequestID: uuid.NewString()} }
	if _, _, e = s.DeclareGoingHome(ctx, uid, input()); !errors.Is(e, domain.ErrPresenceUnknown) {
		t.Fatal("UNKNOWN accepted", e)
	}
	scansStore := arepo.NewDevicesWithPolicy(pool, adomainPolicy())
	deviceService := aservice.NewDevices(scansStore)
	issued, e := deviceService.Register(ctx, prefix+"admin", adto.RegisterDevice{Name: prefix + "gate"})
	if e != nil {
		t.Fatal(e)
	}
	token := strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "")
	exec("INSERT INTO hostelhive.student_qr(student_id,token) VALUES($1,$2)", sid, token)
	scan := func(direction, eventID string, cooldown time.Duration) (adomain.Event, error) {
		device, e := scansStore.SetDirection(ctx, prefix+"admin", issued.Device.DeviceID, direction)
		if e != nil {
			return adomain.Event{}, e
		}
		return aservice.NewScans(scansStore, cooldown).Record(ctx, device, adto.Scan{EventID: eventID, QR: "HH1:" + token})
	}
	move := func(direction string) adomain.Event {
		t.Helper()
		v, e := scan(direction, uuid.NewString(), time.Nanosecond)
		if e != nil || v.Outcome != "recorded" {
			t.Fatal(v, e)
		}
		return v
	}
	move("IN")
	snapshot := func() string {
		return scalar(`SELECT jsonb_build_object('presence',(SELECT to_jsonb(p) FROM hostelhive.student_presence p WHERE student_id=$1),'events',(SELECT count(*) FROM hostelhive.attendance_events),'leaves',(SELECT count(*) FROM hostelhive.leave_requests),'late_access',(SELECT count(*) FROM hostelhive.late_access_requests),'notifications',(SELECT count(*) FROM hostelhive.notification_events),'outbox',(SELECT count(*) FROM hostelhive.notification_outbox),'realtime',(SELECT to_jsonb(p) FROM hostelhive.attendance_publication p))::text`, sid)
	}
	before := snapshot()
	in := input()
	first, replay, e := s.DeclareGoingHome(ctx, uid, in)
	if e != nil || replay || first.StudentID != sid || first.SubmittedBy != users["student"] || first.Status != "declared" || first.DepartedAt != nil {
		t.Fatal(first, replay, e)
	}
	if snapshot() != before {
		t.Fatal("declaration changed attendance/other workflows")
	}
	again, replay, e := s.DeclareGoingHome(ctx, uid, in)
	if e != nil || !replay || again.GoingHomeID != first.GoingHomeID || !again.DeclaredAt.Equal(first.DeclaredAt) {
		t.Fatal(again, replay, e)
	}
	if _, _, e = s.DeclareGoingHome(ctx, uid, input()); !errors.Is(e, domain.ErrGoingHomePending) {
		t.Fatal("duplicate accepted", e)
	}
	exit := move("OUT")
	if exit.AccessResult != "unapproved" || exit.IncidentID == nil {
		t.Fatal("declaration bypassed early-exit permission", exit)
	}
	again, replay, e = s.DeclareGoingHome(ctx, uid, in)
	if e != nil || !replay || again.Status != "departed" || again.DepartedAt == nil || !again.DepartedAt.Equal(exit.OccurredAt) || again.DepartureEventID == nil || *again.DepartureEventID != exit.EventID {
		t.Fatal("departure not correlated", again, e)
	}
	if _, _, e = s.DeclareGoingHome(ctx, uid, input()); !errors.Is(e, domain.ErrAlreadyOutside) {
		t.Fatal(e)
	}
	move("IN")
	secondIn := input()
	second, _, e := s.DeclareGoingHome(ctx, uid, secondIn)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = scan("OUT", exit.EventID, time.Nanosecond); e != nil {
		t.Fatal(e)
	}
	suppressed, e := scan("OUT", uuid.NewString(), time.Hour)
	if e != nil || suppressed.Outcome != "cooldown" {
		t.Fatal(suppressed, e)
	}
	second, _, e = s.DeclareGoingHome(ctx, uid, secondIn)
	if e != nil || second.Status != "declared" || second.DepartedAt != nil {
		t.Fatal("suppressed/replayed scan consumed declaration", second, e)
	}
	// An older accepted event cannot consume a later declaration.
	oldEvent := uuid.NewString()
	hash := sha256.Sum256([]byte("old fixture"))
	exec("INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at) VALUES($1,$2,$3,$4,'OUT','recorded',$5)", oldEvent, issued.Device.DeviceID, sid, hash[:], second.DeclaredAt.Add(-time.Hour))
	if scalar("SELECT status FROM hostelhive.going_home_declarations WHERE going_home_id=$1", second.GoingHomeID) != "declared" {
		t.Fatal("old event consumed current intent")
	}
	move("OUT")
	move("IN")
	// Two identical concurrent requests create exactly one record.
	same := input()
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	errs := make(chan error, 2)
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, r, e := s.DeclareGoingHome(ctx, uid, same); results <- r; errs <- e }()
	}
	wg.Wait()
	close(results)
	close(errs)
	fresh := 0
	for r := range results {
		if !r {
			fresh++
		}
	}
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if fresh != 1 {
		t.Fatal("concurrent replay duplicated", fresh)
	}
	move("OUT")
	move("IN")
	// Different IDs cannot create two outstanding declarations.
	successes, conflicts := make(chan bool, 2), make(chan error, 2)
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, e := s.DeclareGoingHome(ctx, uid, input())
			successes <- e == nil
			conflicts <- e
		}()
	}
	wg.Wait()
	close(successes)
	close(conflicts)
	fresh = 0
	for ok := range successes {
		if ok {
			fresh++
		}
	}
	for e := range conflicts {
		if e != nil && !errors.Is(e, domain.ErrGoingHomePending) {
			t.Fatal(e)
		}
	}
	if fresh != 1 {
		t.Fatal("duplicate active intent", fresh)
	}
	move("OUT")
	move("IN")
	// Race with an actual exit: either intent is recorded then consumed, or it is rejected as OUT.
	raceIn := input()
	device, e := scansStore.SetDirection(ctx, prefix+"admin", issued.Device.DeviceID, "OUT")
	if e != nil {
		t.Fatal(e)
	}
	raceResult := make(chan error, 1)
	scanResult := make(chan error, 1)
	go func() { _, _, e := s.DeclareGoingHome(ctx, uid, raceIn); raceResult <- e }()
	go func() {
		_, e := aservice.NewScans(scansStore, time.Nanosecond).Record(ctx, device, adto.Scan{EventID: uuid.NewString(), QR: "HH1:" + token})
		scanResult <- e
	}()
	if e = <-scanResult; e != nil {
		t.Fatal(e)
	}
	if e = <-raceResult; e != nil && !errors.Is(e, domain.ErrAlreadyOutside) {
		t.Fatal(e)
	}
	if scalar("SELECT count(*)::text FROM hostelhive.going_home_declarations WHERE student_id=$1 AND status='declared'", sid) != "0" {
		t.Fatal("stranded intent after exit race")
	}
	// Exercise the authenticated HTTP contract against real storage.
	mux := http.NewServeMux()
	leave.RegisterGoingHome(handler.NewGoingHome(s, 2*time.Second), mux, middleware.Middleware(verifier{uid: uid}, irepo.NewPostgresAccounts(pool), time.Second))
	call := func(body string) int {
		r := httptest.NewRequest("POST", "/api/v1/going-home", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer test")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Code
	}
	if call(`{"request_id":"invalid"}`) != 400 || call(`{"request_id":"`+uuid.NewString()+`","student_id":"other"}`) != 400 {
		t.Fatal("invalid input accepted")
	}
	move("IN")
	httpIn := input()
	raw, _ := json.Marshal(httpIn)
	if call(string(raw)) != 201 || call(string(raw)) != 200 {
		t.Fatal("HTTP creation/replay")
	}
	for _, role := range []string{"admin", "warden", "sub_warden", "security_staff"} {
		if _, _, e = s.DeclareGoingHome(ctx, prefix+role, input()); !errors.Is(e, domain.ErrForbidden) {
			t.Fatal(role, e)
		}
	}
	if _, _, e = s.DeclareGoingHome(ctx, prefix+"missing", input()); !errors.Is(e, domain.ErrIneligible) {
		t.Fatal(e)
	}
	// The same request UUID belongs to each Student independently; UNKNOWN never leaks another Student's record.
	if _, _, e = s.DeclareGoingHome(ctx, prefix+"other", in); !errors.Is(e, domain.ErrPresenceUnknown) {
		t.Fatal("another Student received an existing declaration", e)
	}
	exec("UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1", sid, users["admin"])
	if _, _, e = s.DeclareGoingHome(ctx, uid, httpIn); !errors.Is(e, domain.ErrIneligible) {
		t.Fatal("archived replay bypass", e)
	}
	exec("UPDATE hostelhive.students SET deleted_at=NULL,deleted_by=NULL WHERE student_id=$1", sid)
	exec("UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", users["student"])
	if _, _, e = s.DeclareGoingHome(ctx, uid, httpIn); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal("inactive replay bypass", e)
	}
	exec("UPDATE hostelhive.users SET is_active=true,role='warden' WHERE user_id=$1", users["student"])
	if _, _, e = s.DeclareGoingHome(ctx, uid, httpIn); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal("changed role replay bypass", e)
	}
	exec("UPDATE hostelhive.users SET role='student' WHERE user_id=$1", users["student"])
	exec("INSERT INTO hostelhive.student_invitations(request_id,user_id,student_id,email,created_by,request_ip,payload_hash) SELECT $1,user_id,$2,email,$3,'127.0.0.1',$4 FROM hostelhive.users WHERE user_id=$5", uuid.NewString(), sid, users["admin"], hash[:], users["student"])
	if _, _, e = s.DeclareGoingHome(ctx, uid, httpIn); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal("setup pending replay bypass", e)
	}
	if call(string(raw)) != 403 {
		t.Fatal("middleware setup bypass")
	}
}
func adomainPolicy() ldomain.Policy {
	return ldomain.Policy{Location: time.FixedZone("TestEarly", -time.Now().UTC().Hour()*3600), ExitOpening: 360, EntryDeadline: 1200}
}
