package attendance_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/handler"
	repo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/repository"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/service"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	accessd "github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/domain"
	accessrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/repository"
	accesssvc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/service"
	nd "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	ndto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/dto"
	nrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/repository"
	nsvc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/service"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAttendancePermissionChecks(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL migrated to22")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := "p76-" + uuid.NewString()[:12] + "-"
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
	admin := user("admin", "admin")
	user("warden", "warden")
	user("sub", "sub_warden")
	user("security", "security_staff")
	inactiveAdmin := user("inactive-admin", "admin")
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, inactiveAdmin)
	type subject struct{ sid, uid, qr string }
	subjectIndex := 0
	newSubject := func() subject {
		subjectIndex++
		name := uuid.NewString()
		u := user(name, "student")
		sid := scalar(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Permission Test','Engineering',1,'0771234567') RETURNING student_id::text`, u, prefix+name)
		hash := sha256.Sum256([]byte(sid))
		token := strings.ToLower(strings.TrimSpace(scalar(`SELECT encode($1::bytea,'hex')`, hash[:])))
		exec(`INSERT INTO hostelhive.student_qr(student_id,token) VALUES($1,$2)`, sid, token)
		return subject{sid, prefix + name, "HH1:" + token}
	}
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		for _, sql := range []string{
			`UPDATE hostelhive.late_access_requests SET consumed_at=NULL,consumed_device_id=NULL,consumed_event_id=NULL WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.student_presence WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.attendance_events WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.late_access_requests WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.student_qr WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)`,
			`DELETE FROM hostelhive.students WHERE index_no LIKE $1`,
			`DELETE FROM hostelhive.scanner_device_audit WHERE device_id IN(SELECT device_id FROM hostelhive.scanner_devices WHERE name LIKE $1)`,
			`DELETE FROM hostelhive.scanner_devices WHERE name LIKE $1`,
			`DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`,
		} {
			if _, e := pool.Exec(clean, sql, prefix+"%"); e != nil {
				t.Error(e)
			}
		}
	}()
	// Controlled timezone sets local time to noon, independent of actual wall clock.
	// This is test-only configuration, not a client-provided scan timestamp.
	utc := time.Now().UTC()
	seconds := utc.Hour()*3600 + utc.Minute()*60 + utc.Second()
	zone := time.FixedZone("test-hostel", 12*3600-seconds)
	policy := accessd.Policy{Location: zone, ExitOpening: 13 * 60, EntryDeadline: 20 * 60}
	store := repo.NewDevicesWithPolicy(pool, policy)
	deviceService := svc.NewDevices(store)
	scans := svc.NewScans(store, time.Nanosecond)
	incidents := svc.NewIncidents(store)
	notifications := nrepo.New(pool)
	inbox := nsvc.New(notifications)
	register := func(direction string) (d.Device, string) {
		t.Helper()
		registered, e := deviceService.Register(ctx, prefix+"admin", dto.RegisterDevice{Name: prefix + uuid.NewString()})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = scans.SetDirection(ctx, prefix+"admin", registered.Device.DeviceID, dto.SetDirection{Direction: direction}); e != nil {
			t.Fatal(e)
		}
		authenticated, e := deviceService.Authenticate(ctx, registered.Credential)
		if e != nil {
			t.Fatal(e)
		}
		return authenticated, registered.Credential
	}
	entry, entryCredential := register("IN")
	entry2, _ := register("IN")
	exit, _ := register("OUT")
	request := func(s subject, direction, status string, delta time.Duration) string {
		t.Helper()
		id := scalar(`INSERT INTO hostelhive.late_access_requests(request_id,student_id,submitted_by,direction,expected_at,reason) SELECT gen_random_uuid(),s.student_id,s.user_id,$2,clock_timestamp()+($3::double precision * interval '1 second'),'Permission test' FROM hostelhive.students s WHERE s.student_id=$1 RETURNING late_access_id::text`, s.sid, direction, delta.Seconds())
		if status != "pending" {
			exec(`UPDATE hostelhive.late_access_requests SET status=$2,decided_at=clock_timestamp()-interval '1 second',decided_by=$3,decision_note='Fixture decision' WHERE late_access_id=$1`, id, status, admin)
		}
		return id
	}
	record := func(dev d.Device, s subject, id string) d.Event {
		t.Helper()
		e, err := scans.Record(ctx, dev, dto.Scan{EventID: id, QR: s.qr})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	expect := func(err, want error) {
		t.Helper()
		if !errors.Is(err, want) {
			t.Fatalf("want%v got%v", want, err)
		}
	}
	// No permission: movement/presence, incident and durable staff intent coexist.
	unapprovedSubject := newSubject()
	eventID := uuid.NewString()
	unapproved := record(entry, unapprovedSubject, eventID)
	if unapproved.Outcome != "recorded" || unapproved.AccessResult != "unapproved" || unapproved.IncidentID == nil || unapproved.LateAccessID != nil {
		t.Fatal(unapproved)
	}
	if scalar(`SELECT status FROM hostelhive.student_presence WHERE student_id=$1`, unapprovedSubject.sid) != "IN" {
		t.Fatal("incident prevented presence")
	}
	retry := record(entry, unapprovedSubject, eventID)
	if retry.AccessResult != unapproved.AccessResult || retry.IncidentID == nil || *retry.IncidentID != *unapproved.IncidentID || !retry.OccurredAt.Equal(unapproved.OccurredAt) {
		t.Fatal("retry reclassified movement", retry)
	}
	detail, e := incidents.Incident(ctx, prefix+"warden", *unapproved.IncidentID)
	if e != nil || detail.IncidentType != "late_entry" || detail.PermissionWindowSeconds != 1800 || detail.StudentID != unapprovedSubject.sid || detail.HostelTimezone != "test-hostel" {
		t.Fatal(detail, e)
	}
	// Database delivery failure cannot undo the committed movement or incident.
	exec(`ALTER TABLE hostelhive.notifications ADD CONSTRAINT incident_delivery_failure CHECK(recipient_id <> '` + admin + `'::uuid)`)
	_, deliveryError := notifications.Deliver(ctx)
	exec(`ALTER TABLE hostelhive.notifications DROP CONSTRAINT incident_delivery_failure`)
	if deliveryError == nil {
		t.Fatal("expected delivery failure")
	}
	if scalar(`SELECT count(*)::text FROM hostelhive.attendance_incidents WHERE incident_id=$1`, *unapproved.IncidentID) != "1" {
		t.Fatal("delivery failure lost incident")
	}
	if _, e = notifications.Deliver(ctx); e != nil {
		t.Fatal(e)
	}
	alert := func(uid string) nd.Notification {
		t.Helper()
		page, e := inbox.List(ctx, uid, ndto.Filter{Limit: 100})
		if e != nil {
			t.Fatal(e)
		}
		var found []nd.Notification
		for _, n := range page.Items {
			if n.AttendanceIncidentID == *unapproved.IncidentID {
				found = append(found, n)
			}
		}
		if len(found) != 1 {
			t.Fatal("incident alert count", len(found))
		}
		return found[0]
	}
	adminAlert := alert(prefix + "admin")
	wardenAlert := alert(prefix + "warden")
	if adminAlert.NotificationID == wardenAlert.NotificationID || adminAlert.SourceType != "attendance" || adminAlert.Status != "unapproved" || adminAlert.Kind != "attendance_incident" || adminAlert.Title != "Late entry without permission" {
		t.Fatal(adminAlert)
	}
	if scalar(`SELECT count(*)::text FROM hostelhive.notification_outbox o JOIN hostelhive.notification_events e USING(event_id) WHERE e.attendance_incident_id=$1`, *unapproved.IncidentID) != "2" {
		t.Fatal("wrong staff recipients")
	}
	_, e = inbox.MarkRead(ctx, unapprovedSubject.uid, adminAlert.NotificationID)
	expect(e, nd.ErrNotFound)
	read, e := inbox.MarkRead(ctx, prefix+"admin", adminAlert.NotificationID)
	if e != nil || read.ReadAt == nil {
		t.Fatal(read, e)
	}
	readRetry, e := inbox.MarkRead(ctx, prefix+"admin", adminAlert.NotificationID)
	if e != nil || !readRetry.ReadAt.Equal(*read.ReadAt) {
		t.Fatal(readRetry, e)
	}
	// Approval created after the original scan cannot change its replay result.
	laterApproval := request(unapprovedSubject, "IN", "approved", 10*time.Minute)
	if replay := record(entry, unapprovedSubject, eventID); replay.AccessResult != "unapproved" {
		t.Fatal("later approval changed history", replay)
	}
	if scalar(`SELECT (consumed_at IS NULL)::text FROM hostelhive.late_access_requests WHERE late_access_id=$1`, laterApproval) != "true" {
		t.Fatal("retry consumed new permission")
	}
	// Approval matches correct Student/direction/window; closest matching one wins.
	permittedSubject := newSubject()
	far := request(permittedSubject, "IN", "approved", 20*time.Minute)
	near := request(permittedSubject, "IN", "approved", 10*time.Minute)
	allowed := record(entry, permittedSubject, uuid.NewString())
	if allowed.AccessResult != "approved" || allowed.LateAccessID == nil || *allowed.LateAccessID != near || allowed.IncidentID != nil {
		t.Fatal("approval selection", allowed)
	}
	if scalar(`SELECT consumed_event_id::text FROM hostelhive.late_access_requests WHERE late_access_id=$1`, near) != allowed.EventID || scalar(`SELECT (consumed_at IS NULL)::text FROM hostelhive.late_access_requests WHERE late_access_id=$1`, far) != "true" {
		t.Fatal("consumption incorrect")
	}
	accessService := accesssvc.New(accessrepo.New(pool, policy))
	used, e := accessService.Get(ctx, permittedSubject.uid, near)
	if e != nil || used.ConsumedAt == nil || used.ConsumedEventID == nil || *used.ConsumedEventID != allowed.EventID || !used.ConsumedAt.Equal(allowed.OccurredAt) {
		t.Fatal("consumption not exposed", used, e)
	}
	if page, e := accessService.List(ctx, permittedSubject.uid, accessd.Filter{Limit: 20}); e != nil || len(page.Items) != 2 {
		t.Fatal(page, e)
	}
	if replay := record(entry, permittedSubject, allowed.EventID); replay.LateAccessID == nil || *replay.LateAccessID != near {
		t.Fatal(replay)
	}
	// Database backstops reject a nullable legacy result with a permission ID,
	// and reject a second event attempting to reference an already-used grant.
	for _, test := range []struct {
		result any
		code   string
	}{{nil, "23514"}, {"approved", "23505"}} {
		_, dbErr := pool.Exec(ctx, `INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at,access_result,late_access_id) VALUES($1,$2,$3,decode(repeat('03',32),'hex'),'IN','recorded',clock_timestamp(),$4,$5)`, uuid.NewString(), entry.DeviceID, permittedSubject.sid, test.result, near)
		var pgError *pgconn.PgError
		if !errors.As(dbErr, &pgError) || pgError.Code != test.code {
			t.Fatal("permission database invariant", dbErr)
		}
	}
	// Suppressed scans do not consume another available approval or raise incidents.
	suppressed := record(entry, permittedSubject, uuid.NewString())
	if suppressed.Outcome != "already_in" || suppressed.AccessResult != "not_evaluated" || suppressed.LateAccessID != nil || suppressed.IncidentID != nil {
		t.Fatal(suppressed)
	}
	if scalar(`SELECT (consumed_at IS NULL)::text FROM hostelhive.late_access_requests WHERE late_access_id=$1`, far) != "true" {
		t.Fatal("already-in consumed approval")
	}
	cool := svc.NewScans(store, 10*time.Second)
	coolEvent, e := cool.Record(ctx, entry, dto.Scan{EventID: uuid.NewString(), QR: permittedSubject.qr})
	if e != nil || coolEvent.Outcome != "cooldown" || coolEvent.AccessResult != "not_evaluated" {
		t.Fatal(coolEvent, e)
	}
	// One approval cannot authorize a later distinct movement.
	oneUseSubject := newSubject()
	single := request(oneUseSubject, "IN", "approved", 10*time.Minute)
	first := record(entry, oneUseSubject, uuid.NewString())
	record(exit, oneUseSubject, uuid.NewString())
	second := record(entry, oneUseSubject, uuid.NewString())
	if first.AccessResult != "approved" || second.AccessResult != "unapproved" || scalar(`SELECT consumed_event_id::text FROM hostelhive.late_access_requests WHERE late_access_id=$1`, single) != first.EventID {
		t.Fatal("permission reused", first, second)
	}
	// Unapproved early exit still records OUT and exposes its incident type.
	earlySubject := newSubject()
	early := record(exit, earlySubject, uuid.NewString())
	if early.AccessResult != "unapproved" || early.IncidentID == nil || scalar(`SELECT status FROM hostelhive.student_presence WHERE student_id=$1`, earlySubject.sid) != "OUT" {
		t.Fatal(early)
	}
	earlyDetail, e := incidents.Incident(ctx, prefix+"admin", *early.IncidentID)
	if e != nil || earlyDetail.IncidentType != "early_exit" {
		t.Fatal(earlyDetail, e)
	}
	// Approved early OUT is consumed; already-OUT leaves a second approval unused.
	approvedExitSubject := newSubject()
	exitApproval := request(approvedExitSubject, "OUT", "approved", 10*time.Minute)
	exitSpare := request(approvedExitSubject, "OUT", "approved", 20*time.Minute)
	approvedExit := record(exit, approvedExitSubject, uuid.NewString())
	if approvedExit.AccessResult != "approved" || approvedExit.LateAccessID == nil || *approvedExit.LateAccessID != exitApproval || approvedExit.IncidentID != nil {
		t.Fatal("early OUT approval", approvedExit)
	}
	alreadyOut := record(exit, approvedExitSubject, uuid.NewString())
	if alreadyOut.Outcome != "already_out" || alreadyOut.AccessResult != "not_evaluated" || scalar(`SELECT (consumed_at IS NULL)::text FROM hostelhive.late_access_requests WHERE late_access_id=$1`, exitSpare) != "true" {
		t.Fatal("already OUT consumed permission", alreadyOut)
	}
	// Pending/rejected/outside-window/wrong-direction/other-Student/future-decision do not match.
	for _, test := range []string{"pending", "rejected", "old", "future", "direction", "other", "future-decision"} {
		s := newSubject()
		owner := s
		direction, status, delta := "IN", "approved", 10*time.Minute
		switch test {
		case "pending":
			status = "pending"
		case "rejected":
			status = "rejected"
		case "old":
			delta = -31 * time.Minute
		case "future":
			delta = 31 * time.Minute
		case "direction":
			direction = "OUT"
		case "other":
			owner = newSubject()
		}
		id := request(owner, direction, status, delta)
		if test == "future-decision" {
			exec(`UPDATE hostelhive.late_access_requests SET decided_at=clock_timestamp()+interval '1 day' WHERE late_access_id=$1`, id)
		}
		if result := record(entry, s, uuid.NewString()); result.AccessResult != "unapproved" {
			t.Fatal(test, result)
		}
		if scalar(`SELECT (consumed_at IS NULL)::text FROM hostelhive.late_access_requests WHERE late_access_id=$1`, id) != "true" {
			t.Fatal(test, "ineligible approval consumed")
		}
	}
	// Both sides of the window: a past approved expected time is still eligible.
	pastSubject := newSubject()
	request(pastSubject, "IN", "approved", -29*time.Minute)
	if result := record(entry, pastSubject, uuid.NewString()); result.AccessResult != "approved" {
		t.Fatal("negative grace", result)
	}
	aheadSubject := newSubject()
	request(aheadSubject, "IN", "approved", 29*time.Minute)
	if result := record(entry, aheadSubject, uuid.NewString()); result.AccessResult != "approved" {
		t.Fatal("positive grace", result)
	}
	// Normal hours do not consume an out-of-window approval; replays retain classification.
	normalSubject := newSubject()
	normalID := request(normalSubject, "IN", "approved", 2*time.Hour)
	normalStore := repo.NewDevicesWithPolicy(pool, accessd.Policy{Location: zone, ExitOpening: 0, EntryDeadline: 23*60 + 59})
	normalScans := svc.NewScans(normalStore, time.Nanosecond)
	normal, e := normalScans.Record(ctx, entry, dto.Scan{EventID: uuid.NewString(), QR: normalSubject.qr})
	if e != nil || normal.AccessResult != "not_required" || normal.LateAccessID != nil || normal.IncidentID != nil {
		t.Fatal(normal, e)
	}
	if scalar(`SELECT (consumed_at IS NULL)::text FROM hostelhive.late_access_requests WHERE late_access_id=$1`, normalID) != "true" {
		t.Fatal("normal scan consumed permission")
	}
	if replay := record(entry, normalSubject, normal.EventID); replay.AccessResult != "not_required" {
		t.Fatal("configuration changed history")
	}
	// A matching recorded movement before curfew still consumes the one-use grant.
	earlyArrivalSubject := newSubject()
	earlyArrivalApproval := request(earlyArrivalSubject, "IN", "approved", 10*time.Minute)
	earlyArrival, e := normalScans.Record(ctx, entry, dto.Scan{EventID: uuid.NewString(), QR: earlyArrivalSubject.qr})
	if e != nil || earlyArrival.AccessResult != "approved" || earlyArrival.LateAccessID == nil || *earlyArrival.LateAccessID != earlyArrivalApproval {
		t.Fatal("normal-hours matching grant unused", earlyArrival, e)
	}
	if scalar(`SELECT consumed_event_id::text FROM hostelhive.late_access_requests WHERE late_access_id=$1`, earlyArrivalApproval) != earlyArrival.EventID {
		t.Fatal("early arrival did not consume grant")
	}
	// Concurrent readers serialize one real movement and one suppression.
	concurrentSubject := newSubject()
	concurrentApproval := request(concurrentSubject, "IN", "approved", 10*time.Minute)
	var wg sync.WaitGroup
	results := make(chan d.Event, 2)
	errs := make(chan error, 2)
	for _, device := range []d.Device{entry, entry2} {
		wg.Add(1)
		go func(dev d.Device) {
			defer wg.Done()
			v, e := scans.Record(ctx, dev, dto.Scan{EventID: uuid.NewString(), QR: concurrentSubject.qr})
			results <- v
			errs <- e
		}(device)
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	matched, ignored := 0, 0
	for v := range results {
		if v.AccessResult == "approved" {
			matched++
		}
		if v.AccessResult == "not_evaluated" {
			ignored++
		}
	}
	if matched != 1 || ignored != 1 || scalar(`SELECT (consumed_at IS NOT NULL)::text FROM hostelhive.late_access_requests WHERE late_access_id=$1`, concurrentApproval) != "true" {
		t.Fatal("concurrent permission", matched, ignored)
	}
	// Intent failure rolls back movement, incident, presence and RTDB publication.
	failedSubject := newSubject()
	failedEvent := uuid.NewString()
	version := scalar(`SELECT version::text FROM hostelhive.attendance_publication WHERE singleton`)
	exec(`CREATE FUNCTION hostelhive.test_incident_failure76() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.source_type='attendance' AND EXISTS(SELECT 1 FROM hostelhive.attendance_incidents WHERE incident_id=NEW.attendance_incident_id AND student_id='` + failedSubject.sid + `'::uuid) THEN RAISE EXCEPTION 'Injected incident intent failure'; END IF; RETURN NEW; END; $$`)
	exec(`CREATE TRIGGER test_incident_failure76 BEFORE INSERT ON hostelhive.notification_events FOR EACH ROW EXECUTE FUNCTION hostelhive.test_incident_failure76()`)
	_, failure := scans.Record(ctx, entry, dto.Scan{EventID: failedEvent, QR: failedSubject.qr})
	exec(`DROP TRIGGER test_incident_failure76 ON hostelhive.notification_events`)
	exec(`DROP FUNCTION hostelhive.test_incident_failure76()`)
	expect(failure, d.ErrUnavailable)
	if scalar(`SELECT count(*)::text FROM hostelhive.attendance_events WHERE device_id=$1 AND event_id=$2`, entry.DeviceID, failedEvent) != "0" || scalar(`SELECT count(*)::text FROM hostelhive.student_presence WHERE student_id=$1`, failedSubject.sid) != "0" || scalar(`SELECT version::text FROM hostelhive.attendance_publication WHERE singleton`) != version {
		t.Fatal("partial movement after intent failure")
	}
	if retried := record(entry, failedSubject, failedEvent); retried.AccessResult != "unapproved" {
		t.Fatal(retried)
	}
	// Failure after consumption must leave the approval available for retry.
	consumeSubject := newSubject()
	consumeApproval := request(consumeSubject, "IN", "approved", 10*time.Minute)
	consumeEvent := uuid.NewString()
	exec(`ALTER TABLE hostelhive.attendance_events ADD CONSTRAINT permission_record_failure CHECK(event_id <> '` + consumeEvent + `'::uuid OR access_result IS DISTINCT FROM 'approved')`)
	_, failure = scans.Record(ctx, entry, dto.Scan{EventID: consumeEvent, QR: consumeSubject.qr})
	exec(`ALTER TABLE hostelhive.attendance_events DROP CONSTRAINT permission_record_failure`)
	expect(failure, d.ErrUnavailable)
	if scalar(`SELECT (consumed_at IS NULL)::text FROM hostelhive.late_access_requests WHERE late_access_id=$1`, consumeApproval) != "true" {
		t.Fatal("failed transaction consumed permission")
	}
	if retried := record(entry, consumeSubject, consumeEvent); retried.AccessResult != "approved" {
		t.Fatal(retried)
	}
	// Incident list uses one snapshot and keeps totals for empty pages.
	list, e := incidents.Incidents(ctx, prefix+"admin", dto.IncidentFilter{StudentID: unapprovedSubject.sid, Direction: "IN", IncidentType: "late_entry", Limit: 1})
	if e != nil || list.Total != 1 || len(list.Items) != 1 {
		t.Fatal(list, e)
	}
	emptyPage, e := incidents.Incidents(ctx, prefix+"warden", dto.IncidentFilter{StudentID: unapprovedSubject.sid, Limit: 1, Offset: 1000})
	if e != nil || emptyPage.Total != 1 || len(emptyPage.Items) != 0 || emptyPage.HasMore {
		t.Fatal(emptyPage, e)
	}
	mux := http.NewServeMux()
	protected := m.Middleware(verifier{}, identityrepo.NewPostgresAccounts(pool), time.Second)
	attendance.RegisterIncidents(h.NewIncidentAPI(incidents, 3*time.Second), mux, protected)
	deviceAPI := h.New(deviceService, 3*time.Second, time.Second)
	attendance.RegisterScans(h.NewScanAPI(scans, 3*time.Second), deviceAPI, mux, protected)
	call := func(method, path, body, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s want%d got%d %s", method, path, want, w.Code, w.Body.String())
		}
		return w
	}
	call("GET", "/api/v1/attendance/incidents/"+*unapproved.IncidentID, "", prefix+"admin", 200)
	call("GET", "/api/v1/attendance/incidents?student_id="+unapprovedSubject.sid+"&direction=IN&incident_type=late_entry&limit=1", "", prefix+"warden", 200)
	for _, who := range []string{unapprovedSubject.uid, prefix + "sub", prefix + "security"} {
		call("GET", "/api/v1/attendance/incidents", "", who, 403)
	}
	call("GET", "/api/v1/attendance/incidents", "", "", 401)
	for _, q := range []string{"unknown=1", "limit=0", "limit=101", "offset=-1", "offset=1000001", "limit=1&limit=2", "direction=bad", "incident_type=bad", "student_id=bad", "direction="} {
		call("GET", "/api/v1/attendance/incidents?"+q, "", prefix+"admin", 400)
	}
	call("GET", "/api/v1/attendance/incidents", "{}", prefix+"admin", 400)
	call("GET", "/api/v1/attendance/incidents/bad", "", prefix+"admin", 400)
	call("GET", "/api/v1/attendance/incidents/"+uuid.NewString(), "", prefix+"admin", 404)
	call("GET", "/api/v1/attendance/incidents/"+*unapproved.IncidentID+"?unknown=1", "", prefix+"admin", 400)
	// Real device-authenticated retry returns the persisted classification.
	body, _ := json.Marshal(dto.Scan{EventID: eventID, QR: unapprovedSubject.qr})
	response := call("POST", "/api/v1/device/scans", string(body), entryCredential, 200)
	var httpEvent d.Event
	if e = json.Unmarshal(response.Body.Bytes(), &httpEvent); e != nil || httpEvent.AccessResult != "unapproved" || httpEvent.IncidentID == nil {
		t.Fatal(httpEvent, e)
	}
	history, e := scans.Events(ctx, prefix+"admin", permittedSubject.sid, 20, 0)
	if e != nil || len(history.Items) < 1 {
		t.Fatal(history, e)
	}
	// Archival/deactivation retains staff incident history; current role revokes access.
	exec(`UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, unapprovedSubject.sid, admin)
	if _, e = incidents.Incident(ctx, prefix+"admin", *unapproved.IncidentID); e != nil {
		t.Fatal(e)
	}
	exec(`UPDATE hostelhive.users SET role='security_staff' WHERE firebase_uid=$1`, prefix+"warden")
	_, e = incidents.Incident(ctx, prefix+"warden", *unapproved.IncidentID)
	expect(e, d.ErrForbidden)
	for _, who := range []string{prefix + "sub", unapprovedSubject.uid} {
		page, e := inbox.List(ctx, who, ndto.Filter{Limit: 100})
		if e != nil {
			t.Fatal(e)
		}
		for _, n := range page.Items {
			if n.SourceType == "attendance" {
				t.Fatal("incident leaked to nonstaff")
			}
		}
	}
}
