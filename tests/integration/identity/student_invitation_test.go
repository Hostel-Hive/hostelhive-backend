package identity_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/dto"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/handler"
	repo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/service"
	student "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
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

type invitationVerifier struct{}

func (invitationVerifier) Verify(_ context.Context, token string) (string, error) { return token, nil }

type invitationProviderFake struct {
	mu                    sync.Mutex
	prepared, sent, proof map[string]int
	verified              map[string]bool
	fail                  map[string]error
}

func (f *invitationProviderFake) Prepare(_ context.Context, uid, email string) (float64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prepared[uid]++
	return 1000, f.fail[uid]
}
func (f *invitationProviderFake) Send(_ context.Context, uid, email, ip string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent[uid]++
	return f.fail[uid]
}
func (f *invitationProviderFake) VerifySetup(_ context.Context, uid, email string, baseline float64, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.proof[uid]++
	if baseline != 1000 || token != uid || !f.verified[uid] {
		return d.ErrSetupPending
	}
	return nil
}
func TestStudentInvitationsPostgresLifecycle(t *testing.T) {
	database := os.Getenv("TEST_DATABASE_URL")
	if database == "" {
		t.Skip("requires migrated disposable PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, e := pgxpool.New(ctx, database)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	prefix := "i95-" + uuid.NewString()
	uids, ids := []string{}, []string{}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, sql := range []string{
			"DELETE FROM hostelhive.student_invitations WHERE email LIKE $1",
			"DELETE FROM hostelhive.guardians WHERE student_id IN(SELECT student_id FROM hostelhive.students s JOIN hostelhive.users u USING(user_id) WHERE u.email LIKE $1)",
			"DELETE FROM hostelhive.students WHERE user_id IN(SELECT user_id FROM hostelhive.users WHERE email LIKE $1)",
			"DELETE FROM hostelhive.user_revocations WHERE user_id IN(SELECT user_id FROM hostelhive.users WHERE email LIKE $1)",
			"DELETE FROM hostelhive.user_provisioning WHERE email LIKE $1",
			"DELETE FROM hostelhive.users WHERE email LIKE $1",
		} {
			if _, e := pool.Exec(clean, sql, prefix+"%"); e != nil {
				t.Error("cleanup", e)
			}
		}
	}()
	for n, role := range []string{"admin", "warden", "student", "security_staff", "sub_warden"} {
		uid := fmt.Sprintf("%s-%d", prefix, n)
		var id string
		if e := pool.QueryRow(ctx, "INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text", uid, uid+"@example.invalid", role).Scan(&id); e != nil {
			t.Fatal(e)
		}
		uids = append(uids, uid)
		ids = append(ids, id)
	}
	store := repo.NewInvitationStore(pool)
	provider := &invitationProviderFake{prepared: map[string]int{}, sent: map[string]int{}, proof: map[string]int{}, verified: map[string]bool{}, fail: map[string]error{}}
	service := svc.NewInvitations(store, provider, []string{"stu.kln.ac.lk"}, true)
	lookup := repo.NewPostgresAccounts(pool)
	protect := m.Middleware(invitationVerifier{}, lookup, time.Second)
	mux := http.NewServeMux()
	identity.RegisterInvitations(h.NewInvitationAPI(service, 8*time.Second), mux, protect)
	identity.RegisterMe(mux, protect)
	mux.Handle("GET /api/v1/me/student-profile", protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })))
	do := func(method, path, token, body, media string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", media)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s HTTP %d expected %d: %s", method, path, w.Code, status, w.Body.String())
		}
		return w
	}
	input := func(label string) dto.StudentInvitationInput {
		v := dto.StudentInvitationInput{RequestID: uuid.NewString(), Email: prefix + "-" + label + "@stu.kln.ac.lk"}
		v.IndexNo = "I95-" + uuid.NewString()
		v.FullName = "Invitation Student"
		v.Faculty = "Science"
		v.Year = 2
		v.ContactPhone = "0771234567"
		v.Guardians = []student.GuardianInput{{Name: "Guardian", Relationship: "Parent", ContactPhone: "0771234568"}}
		return v
	}
	register := func(v dto.StudentInvitationInput) d.StudentInvitation {
		t.Helper()
		i, replay, e := service.Register(ctx, uids[0], "192.0.2.1", v)
		if e != nil || replay {
			t.Fatal("registration", e, replay)
		}
		return i
	}
	uidOf := func(i d.StudentInvitation) string {
		t.Helper()
		var uid string
		if e := pool.QueryRow(ctx, "SELECT firebase_uid FROM hostelhive.users WHERE user_id=$1", i.UserID).Scan(&uid); e != nil {
			t.Fatal(e)
		}
		return uid
	}
	status := func(i d.StudentInvitation) d.StudentInvitation {
		t.Helper()
		v, e := service.Get(ctx, uids[0], i.InvitationID)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	dispatch := func() bool {
		t.Helper()
		worked, e := service.Dispatch(ctx)
		if e != nil {
			t.Fatal("dispatch", e)
		}
		return worked
	}
	drain := func() {
		t.Helper()
		for n := 0; n < 100; n++ {
			if !dispatch() {
				return
			}
		}
		t.Fatal("delivery did not drain")
	}
	firstInput := input("first")
	raw, _ := json.Marshal(firstInput)
	response := do("POST", "/api/v1/student-invitations", uids[0], string(raw), "application/json", 201)
	var firstResult struct {
		Invitation d.StudentInvitation `json:"invitation"`
		Replayed   bool                `json:"replayed"`
	}
	if json.Unmarshal(response.Body.Bytes(), &firstResult) != nil {
		t.Fatal("registration result")
	}
	first := firstResult.Invitation
	firstUID := uidOf(first)
	if first.DeliveryStatus != "pending" || first.Attempts != 0 || first.SetupCompletedAt != nil {
		t.Fatal("new invitation state", first)
	}
	if a, e := lookup.FindByFirebaseUID(ctx, firstUID); e != nil || !a.SetupRequired || !a.IsActive {
		t.Fatal("pending account flag", a, e)
	}
	do("POST", "/api/v1/student-invitations", uids[0], string(raw), "application/json", 200)
	changed := firstInput
	changed.FullName = "Changed"
	changedRaw, _ := json.Marshal(changed)
	do("POST", "/api/v1/student-invitations", uids[0], string(changedRaw), "application/json", 409)
	me := do("GET", "/api/v1/me", firstUID, "", "", 200)
	if !strings.Contains(me.Body.String(), `"setup_required":true`) {
		t.Fatal("setup flag absent")
	}
	do("GET", "/api/v1/me/student-profile", firstUID, "", "", 403)
	do("POST", "/api/v1/me/setup/complete", firstUID, "", "", 409)
	do("POST", "/api/v1/me/setup/complete", firstUID, `{"setup_complete":true}`, "application/json", 400)
	do("POST", "/api/v1/me/setup/complete?user_id="+ids[2], firstUID, "", "", 400)
	do("GET", "/api/v1/me", "unapproved-firebase-only", "", "", 403)
	if !dispatch() || provider.prepared[firstUID] != 1 || provider.sent[firstUID] != 0 {
		t.Fatal("baseline not committed separately")
	}
	do("POST", "/api/v1/me/setup/complete", firstUID, "", "", 409)
	resendKey := uuid.NewString()
	resendJSON := `{"request_id":"` + resendKey + `"}`
	do("POST", "/api/v1/student-invitations/"+first.InvitationID+"/resend", uids[0], resendJSON, "application/json", 200)
	if !dispatch() || status(first).DeliveryStatus != "sent" || provider.sent[firstUID] != 1 {
		t.Fatal("email not accepted")
	}
	do("POST", "/api/v1/student-invitations/"+first.InvitationID+"/resend", uids[0], resendJSON, "application/json", 200)
	if dispatch() {
		t.Fatal("resend replay queued duplicate")
	}
	do("POST", "/api/v1/student-invitations/"+first.InvitationID+"/resend", uids[0], `{"request_id":"`+uuid.NewString()+`"}`, "application/json", 429)
	exec("UPDATE hostelhive.student_invitations SET last_requested_at=clock_timestamp()-interval '6 minutes' WHERE invitation_id=$1", first.InvitationID)
	if _, e := service.Resend(ctx, uids[0], first.InvitationID, uuid.NewString(), "192.0.2.1"); e != nil {
		t.Fatal(e)
	}
	drain()
	if provider.prepared[firstUID] != 1 || provider.sent[firstUID] != 2 {
		t.Fatal("resend reset credentials or baseline")
	}
	provider.verified[firstUID] = true
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := service.Complete(ctx, firstUID, firstUID); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal("concurrent completion", e)
		}
	}
	if provider.proof[firstUID] != 2 {
		t.Fatalf("expected failed proof plus one completion proof, got %d", provider.proof[firstUID])
	}
	var completedAudits int
	if e := pool.QueryRow(ctx, "SELECT count(*) FROM hostelhive.student_invitation_audit WHERE invitation_id=$1 AND action='completed'", first.InvitationID).Scan(&completedAudits); e != nil || completedAudits != 1 {
		t.Fatal("duplicate completion")
	}
	if a, e := lookup.FindByFirebaseUID(ctx, firstUID); e != nil || a.SetupRequired {
		t.Fatal("completion flag", e)
	}
	do("GET", "/api/v1/me/student-profile", firstUID, "", "", 204)
	do("POST", "/api/v1/me/setup/complete", firstUID, "", "", 200)
	do("POST", "/api/v1/student-invitations", uids[0], string(raw), "application/json", 200)
	do("POST", "/api/v1/student-invitations/"+first.InvitationID+"/resend", uids[0], resendJSON, "application/json", 200)
	do("POST", "/api/v1/student-invitations/"+first.InvitationID+"/resend", uids[0], `{"request_id":"`+uuid.NewString()+`"}`, "application/json", 409)
	var guardians int
	if e := pool.QueryRow(ctx, "SELECT count(*) FROM hostelhive.guardians WHERE student_id=$1", first.StudentID).Scan(&guardians); e != nil || guardians != 1 {
		t.Fatal("profile/guardian duplicates")
	}
	duplicate := input("duplicate")
	duplicate.IndexNo = firstInput.IndexNo
	if _, _, e := service.Register(ctx, uids[0], "192.0.2.1", duplicate); !errors.Is(e, d.ErrInvitationConflict) {
		t.Fatal("duplicate index accepted", e)
	}
	var orphans int
	if e := pool.QueryRow(ctx, "SELECT count(*) FROM hostelhive.users WHERE email=$1", duplicate.Email).Scan(&orphans); e != nil || orphans != 0 {
		t.Fatal("partial row left orphan account")
	}
	duplicate = input("email-duplicate")
	duplicate.Email = firstInput.Email
	if _, _, e := service.Register(ctx, uids[0], "192.0.2.1", duplicate); !errors.Is(e, d.ErrInvitationConflict) {
		t.Fatal("duplicate email", e)
	}
	for _, actor := range uids[1:] {
		if _, _, e := service.Register(ctx, actor, "192.0.2.1", input("unauthorized")); !errors.Is(e, d.ErrInvitationForbidden) {
			t.Fatal("non Admin registration", e)
		}
		do("GET", "/api/v1/student-invitations", actor, "", "", 403)
	}
	do("GET", "/api/v1/student-invitations", "", "", "", 401)
	do("GET", "/api/v1/student-invitations/"+uuid.NewString(), uids[0], "", "", 404)
	do("GET", "/api/v1/student-invitations?limit=1&offset=0", uids[0], "", "", 200)
	metadata := do("GET", "/api/v1/student-invitations/"+first.InvitationID, uids[0], "", "", 200).Body.String()
	for _, field := range []string{"password", "baseline_password_updated_at", "request_ip", "payload_hash", "oobCode", "oobLink"} {
		if strings.Contains(metadata, `"`+field+`"`) {
			t.Fatal("private metadata exposed", field)
		}
	}
	// The legacy Student predates invitations and retains access.
	if a, e := lookup.FindByFirebaseUID(ctx, uids[2]); e != nil || a.SetupRequired {
		t.Fatal("legacy account affected", e)
	}
	do("GET", "/api/v1/me/student-profile", uids[2], "", "", 204)
	// Delivery skips inactive/archived/role-changed recipients without contacting Firebase.
	for _, mode := range []string{"inactive", "archived", "role"} {
		i := register(input(mode))
		uid := uidOf(i)
		switch mode {
		case "inactive":
			exec("UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", i.UserID)
		case "archived":
			exec("UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1", i.StudentID, ids[0])
		case "role":
			exec("UPDATE hostelhive.users SET role='warden' WHERE user_id=$1", i.UserID)
		}
		drain()
		if status(i).DeliveryStatus != "skipped" || provider.prepared[uid] != 0 || provider.sent[uid] != 0 {
			t.Fatal("ineligible delivery")
		}
		if _, e := service.Resend(ctx, uids[0], i.InvitationID, uuid.NewString(), "192.0.2.1"); !errors.Is(e, d.ErrInvitationConflict) {
			t.Fatal("ineligible resend", e)
		}
		if mode == "archived" {
			if _, e := service.Complete(ctx, uid, uid); !errors.Is(e, d.ErrInvitationNotFound) {
				t.Fatal("archived completion", e)
			}
		} else {
			if _, e := service.Complete(ctx, uid, uid); !errors.Is(e, d.ErrInvitationForbidden) {
				t.Fatal("ineligible completion", e)
			}
		}
	}
	conflict := register(input("provider-conflict"))
	conflictUID := uidOf(conflict)
	provider.fail[conflictUID] = d.ErrInvitationIdentity
	drain()
	if status(conflict).DeliveryStatus != "failed" || provider.sent[conflictUID] != 0 {
		t.Fatal("provider conflict adopted")
	}
	retry := register(input("retry"))
	retryUID := uidOf(retry)
	provider.fail[retryUID] = &d.InvitationRetry{After: 10 * time.Minute}
	drain()
	r := status(retry)
	if r.DeliveryStatus != "pending" || r.Attempts != 1 || r.NextAttemptAt == nil || time.Until(*r.NextAttemptAt) < 9*time.Minute {
		t.Fatal("retry/backoff", r)
	}
	exec("UPDATE hostelhive.student_invitations SET attempts=7,next_attempt_at=clock_timestamp() WHERE invitation_id=$1", retry.InvitationID)
	drain()
	if status(retry).DeliveryStatus != "failed" || status(retry).Attempts != 8 {
		t.Fatal("retry cap")
	}
	delete(provider.fail, retryUID)
	exec("UPDATE hostelhive.student_invitations SET last_requested_at=clock_timestamp()-interval '6 minutes' WHERE invitation_id=$1", retry.InvitationID)
	if _, e := service.Resend(ctx, uids[0], retry.InvitationID, uuid.NewString(), "192.0.2.1"); e != nil {
		t.Fatal(e)
	}
	drain()
	if status(retry).DeliveryStatus != "sent" {
		t.Fatal("retry recovery")
	}
	// Two concurrent request retries reserve only one account/profile.
	parallel := input("parallel")
	results := make(chan d.StudentInvitation, 2)
	parallelErrors := make(chan error, 2)
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			i, _, e := service.Register(ctx, uids[0], "192.0.2.1", parallel)
			results <- i
			parallelErrors <- e
		}()
	}
	wg.Wait()
	close(results)
	close(parallelErrors)
	for e := range parallelErrors {
		if e != nil {
			t.Fatal(e)
		}
	}
	unique := map[string]bool{}
	var parallelInvite d.StudentInvitation
	for i := range results {
		unique[i.InvitationID] = true
		parallelInvite = i
	}
	if len(unique) != 1 {
		t.Fatal("duplicate reservation")
	}
	parallelUID := uidOf(parallelInvite)
	for stage := 0; stage < 2; stage++ {
		worked := make(chan bool, 2)
		errs := make(chan error, 2)
		for n := 0; n < 2; n++ {
			wg.Add(1)
			go func() { defer wg.Done(); ok, e := service.Dispatch(ctx); worked <- ok; errs <- e }()
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	drain()
	if provider.prepared[parallelUID] != 1 || provider.sent[parallelUID] != 1 {
		t.Fatal("concurrent delivery duplicated stage")
	}
	// New bulk registration is separate from #38; valid rows survive invalid rows.
	bulk := input("bulk")
	bad := input("bad")
	bad.Email = "invalid@gmail.com"
	var b bytes.Buffer
	writer := csv.NewWriter(&b)
	_ = writer.Write(strings.Split("request_id,"+svc.InvitationCSVHeader, ","))
	for _, v := range []dto.StudentInvitationInput{firstInput, bulk, bad, bulk} {
		g, _ := json.Marshal(v.Guardians)
		_ = writer.Write([]string{v.RequestID, v.Email, v.IndexNo, v.FullName, v.Faculty, "2", v.ContactPhone, string(g)})
	}
	writer.Flush()
	bulkResponse := do("POST", "/api/v1/student-invitations/import", uids[0], b.String(), "text/csv", 200)
	var report svc.InvitationImportReport
	if json.Unmarshal(bulkResponse.Body.Bytes(), &report) != nil || report.Total != 4 || report.Queued != 1 || report.Replayed != 1 || report.Rejected != 2 {
		t.Fatalf("bulk report %+v", report)
	}
	do("POST", "/api/v1/student-invitations/import", uids[0], "password,email\nsecret,value\n", "text/csv", 400)
	do("GET", "/api/v1/student-invitations/import/template", uids[0], "", "", 200)
	// Provider creation is not performed inside the registration transaction.
	var bulkUID string
	if e := pool.QueryRow(ctx, "SELECT firebase_uid FROM hostelhive.users WHERE email=$1", bulk.Email).Scan(&bulkUID); e != nil {
		t.Fatal(e)
	}
	if provider.prepared[bulkUID] != 0 {
		t.Fatal("HTTP registration contacted provider")
	}

	// The browser's email-only CSV creates the account/profile without a
	// pre-existing account, and exact retries preserve the registration identity.
	emailOnly := input("email-csv")
	var emailCSV bytes.Buffer
	emailWriter := csv.NewWriter(&emailCSV)
	_ = emailWriter.Write(strings.Split(svc.InvitationCSVHeader, ","))
	guardianJSON, _ := json.Marshal(emailOnly.Guardians)
	_ = emailWriter.Write([]string{emailOnly.Email, emailOnly.IndexNo, emailOnly.FullName, emailOnly.Faculty, "2", emailOnly.ContactPhone, string(guardianJSON)})
	emailWriter.Flush()
	var firstRow svc.InvitationImportRow
	for attempt := 0; attempt < 2; attempt++ {
		res := do("POST", "/api/v1/student-invitations/import", uids[0], emailCSV.String(), "text/csv", 200)
		var result svc.InvitationImportReport
		if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil || len(result.Rows) != 1 {
			t.Fatal("email CSV report", err)
		}
		row := result.Rows[0]
		if row.StudentID == "" || row.InvitationID == "" {
			t.Fatal("missing profile/invitation identity", result)
		}
		if attempt == 0 {
			if result.Queued != 1 {
				t.Fatal("email registration was not queued", result)
			}
			firstRow = row
		} else if result.Replayed != 1 || firstRow.StudentID != row.StudentID || firstRow.InvitationID != row.InvitationID {
			t.Fatal("exact CSV retry created another registration", result)
		}
	}

	drain()
	if _, _, e := svc.NewInvitations(store, provider, []string{"stu.kln.ac.lk"}, false).Register(ctx, uids[0], "192.0.2.1", input("disabled")); !errors.Is(e, d.ErrInvitationUnavailable) {
		t.Fatal("disabled feature accepted registration")
	}
}
