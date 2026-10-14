package announcement_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/service"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	ndomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	ndto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/dto"
	nrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/repository"
	nservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type verifier struct{}

func (verifier) Verify(_ context.Context, token string) (string, error) { return token, nil }
func TestAnnouncementBroadcast(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable database migrated to 20")
	}
	ctx, c := context.WithTimeout(context.Background(), 60*time.Second)
	defer c()
	pool, e := pgxpool.New(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	prefix := fmt.Sprintf("announcement-%d-", time.Now().UnixNano())
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	scalar := func(sql string, args ...any) string {
		t.Helper()
		var id string
		if e := pool.QueryRow(ctx, sql, args...).Scan(&id); e != nil {
			t.Fatal(e)
		}
		return id
	}
	uuid := func() string { return scalar(`SELECT gen_random_uuid()::text`) }
	user := func(name, role string) string {
		return scalar(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text`, prefix+name, prefix+name+"@example.invalid", role)
	}
	admin := user("admin", "admin")
	warden := user("warden", "warden")
	user("sub", "sub_warden")
	user("security", "security_staff")
	student := user("student", "student")
	other := user("other", "student")
	archived := user("archived", "student")
	inactive := user("inactive", "student")
	user("missing", "student")
	profile := func(uid, name string) string {
		return scalar(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Announcement Student','Engineering',1,'0771234567') RETURNING student_id::text`, uid, prefix+name)
	}
	sid := profile(student, "student")
	profile(other, "other")
	archivedSID := profile(archived, "archived")
	profile(inactive, "inactive")
	exec(`UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, archivedSID, admin)
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, inactive)
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		for _, sql := range []string{
			`DELETE FROM hostelhive.announcements WHERE published_by IN(SELECT user_id FROM hostelhive.users WHERE firebase_uid LIKE $1)`,
			`DELETE FROM hostelhive.students WHERE index_no LIKE $1`,
			`DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`,
		} {
			if _, e := pool.Exec(clean, sql, prefix+"%"); e != nil {
				t.Error(e)
			}
		}
	}()
	store := repository.New(pool)
	s := service.New(store)
	notifications := nrepo.New(pool)
	inbox := nservice.New(notifications)
	input := dto.Publish{RequestID: uuid(), Title: " Water supply notice ", Content: " Water unavailable tomorrow.\nPlease prepare. "}
	pub, e := s.Publish(ctx, prefix+"warden", input)
	if e != nil || pub.Replayed || pub.Announcement.Audience != "all_students" || pub.Announcement.Priority != "normal" || pub.Announcement.PublishedBy != warden || pub.Announcement.Title != "Water supply notice" {
		t.Fatal(pub, e)
	}
	aid := pub.Announcement.AnnouncementID
	replay, e := s.Publish(ctx, prefix+"warden", input)
	if e != nil || !replay.Replayed || replay.Announcement.AnnouncementID != aid || !replay.Announcement.PublishedAt.Equal(pub.Announcement.PublishedAt) {
		t.Fatal("unsafe replay", replay, e)
	}
	changed := input
	changed.Content = "Different content"
	if _, e = s.Publish(ctx, prefix+"warden", changed); !errors.Is(e, domain.ErrConflict) {
		t.Fatal(e)
	}
	summary := func() domain.DeliverySummary {
		t.Helper()
		v, e := s.Summary(ctx, prefix+"admin", aid)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	before := summary()
	if before.RecipientsTotal != 2 || before.Delivered != 0 || before.PendingDelivery != 2 || before.Read != 0 {
		t.Fatal(before)
	}
	var eventCount int
	if pool.QueryRow(ctx, `SELECT count(*) FROM hostelhive.notification_events WHERE announcement_id=$1`, aid).Scan(&eventCount) != nil || eventCount != 1 {
		t.Fatal("duplicate publication events")
	}
	exec(`ALTER TABLE hostelhive.notifications ADD CONSTRAINT announcement_delivery_failure CHECK(recipient_id <> '` + student + `'::uuid)`)
	_, deliveryError := notifications.Deliver(ctx)
	exec(`ALTER TABLE hostelhive.notifications DROP CONSTRAINT announcement_delivery_failure`)
	if deliveryError == nil || summary() != before {
		t.Fatal("delivery failure changed summary")
	}
	if _, e = s.Get(ctx, prefix+"student", aid); e != nil {
		t.Fatal("delivery failure undid announcement", e)
	}
	if _, e = notifications.Deliver(ctx); e != nil {
		t.Fatal(e)
	}
	got := summary()
	if got.Delivered != 2 || got.PendingDelivery != 0 || got.Read != 0 || got.Unread != 2 {
		t.Fatal(got)
	}
	items := func(name string) ndomain.Page {
		t.Helper()
		p, e := inbox.List(ctx, prefix+name, ndto.Filter{Limit: 100})
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	studentPage := items("student")
	if studentPage.Total != 1 || studentPage.Items[0].SourceType != "announcement" || studentPage.Items[0].AnnouncementID != aid || studentPage.Items[0].Kind != "announcement_published" || studentPage.Items[0].Status != "published" || studentPage.Items[0].Title != "Water supply notice" {
		t.Fatal(studentPage)
	}
	alert := studentPage.Items[0]
	for _, name := range []string{"admin", "warden", "sub", "archived", "missing"} {
		if items(name).Total != 0 {
			t.Fatal("incorrect recipient", name)
		}
	}
	if _, e = inbox.List(ctx, prefix+"inactive", ndto.Filter{Limit: 20}); !errors.Is(e, ndomain.ErrForbidden) {
		t.Fatal(e)
	}
	for _, name := range []string{"admin", "other"} {
		if _, e = inbox.MarkRead(ctx, prefix+name, alert.NotificationID); !errors.Is(e, ndomain.ErrNotFound) {
			t.Fatal("cross-user marking", e)
		}
	}
	read, e := inbox.MarkRead(ctx, prefix+"student", alert.NotificationID)
	if e != nil || read.ReadAt == nil {
		t.Fatal(read, e)
	}
	again, e := inbox.MarkRead(ctx, prefix+"student", alert.NotificationID)
	if e != nil || !again.ReadAt.Equal(*read.ReadAt) {
		t.Fatal("read retry", e)
	}
	if summary().Read != 1 || summary().Unread != 1 {
		t.Fatal(summary())
	}
	unread, e := inbox.Unread(ctx, prefix+"student")
	if e != nil || unread.Unread != 0 {
		t.Fatal(unread, e)
	}
	// Feed access does not itself mark alerts read; current eligibility is rechecked.
	for _, name := range []string{"admin", "warden", "student", "other"} {
		v, e := s.Get(ctx, prefix+name, aid)
		if e != nil || v.AnnouncementID != aid {
			t.Fatal(name, v, e)
		}
	}
	for _, name := range []string{"sub", "security", "archived", "missing", "inactive"} {
		if _, e = s.Get(ctx, prefix+name, aid); !errors.Is(e, domain.ErrForbidden) {
			t.Fatal(name, e)
		}
	}
	if _, e = s.Summary(ctx, prefix+"student", aid); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal(e)
	}
	archiveTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer archiveTx.Rollback(context.Background())
	if _, err = archiveTx.Exec(ctx, `UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, sid, admin); err != nil {
		t.Fatal(err)
	}
	type inboxResult struct {
		page ndomain.Page
		err  error
	}
	waiting := make(chan inboxResult, 1)
	go func() {
		p, e := inbox.List(ctx, prefix+"student", ndto.Filter{Limit: 100})
		waiting <- inboxResult{p, e}
	}()
	select {
	case result := <-waiting:
		t.Fatalf("inbox bypassed uncommitted profile archival: %+v", result)
	case <-time.After(20 * time.Millisecond):
	}
	if err = archiveTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	result := <-waiting
	if result.err != nil || result.page.Total != 0 {
		t.Fatal("archival authority recheck", result)
	}
	if items("student").Total != 0 {
		t.Fatal("archived recipient still sees announcement alert")
	}
	if _, e = inbox.MarkRead(ctx, prefix+"student", alert.NotificationID); !errors.Is(e, ndomain.ErrNotFound) {
		t.Fatal(e)
	}
	if summary().Read != 1 {
		t.Fatal("historical read receipt lost")
	}
	exec(`UPDATE hostelhive.students SET deleted_at=NULL,deleted_by=NULL WHERE student_id=$1`, sid)
	exec(`UPDATE hostelhive.users SET role='warden' WHERE user_id=$1`, student)
	if items("student").Total != 0 {
		t.Fatal("role change exposed old Student announcement alert")
	}
	exec(`UPDATE hostelhive.users SET role='student' WHERE user_id=$1`, student)
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, student)
	if _, e = inbox.Unread(ctx, prefix+"student"); !errors.Is(e, ndomain.ErrForbidden) {
		t.Fatal(e)
	}
	exec(`UPDATE hostelhive.users SET is_active=true WHERE user_id=$1`, student)
	if items("student").Items[0].ReadAt == nil {
		t.Fatal("reactivation lost read timestamp")
	}
	// New eligible Students can read the global feed, without retroactive alerts.
	late := user("late", "student")
	profile(late, "late")
	if _, e = s.Get(ctx, prefix+"late", aid); e != nil {
		t.Fatal(e)
	}
	if items("late").Total != 0 {
		t.Fatal("retroactive alert")
	}
	if _, e = s.Publish(ctx, prefix+"warden", input); e != nil || summary().RecipientsTotal != 2 {
		t.Fatal("retry resnapshotted recipients", e)
	}
	// Concurrent identical publisher retries serialize into one publication.
	race := dto.Publish{RequestID: uuid(), Title: "Concurrent publication", Content: "One announcement only"}
	var wg sync.WaitGroup
	results := make(chan domain.Publication, 4)
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); v, e := s.Publish(ctx, prefix+"admin", race); results <- v; errs <- e }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	winners := 0
	var raceID string
	for v := range results {
		if !v.Replayed {
			winners++
		}
		if raceID == "" {
			raceID = v.Announcement.AnnouncementID
		}
		if v.Announcement.AnnouncementID != raceID {
			t.Fatal("duplicate publication")
		}
	}
	if winners != 1 {
		t.Fatal(winners)
	}
	// Same request UUID has a separate namespace for another authorised publisher.
	another, e := s.Publish(ctx, prefix+"warden", race)
	if e != nil || another.Replayed || another.Announcement.AnnouncementID == raceID {
		t.Fatal(another, e)
	}
	page, e := s.List(ctx, prefix+"student", dto.Filter{Limit: 1})
	if e != nil || page.Total != 3 || len(page.Items) != 1 || !page.HasMore {
		t.Fatal(page, e)
	}
	page, e = s.List(ctx, prefix+"student", dto.Filter{Limit: 1, Offset: 99})
	if e != nil || page.Total != 3 || len(page.Items) != 0 || page.HasMore || page.Items == nil {
		t.Fatal(page, e)
	}
	for _, name := range []string{"student", "sub", "security"} {
		if _, e = s.Publish(ctx, prefix+name, race); !errors.Is(e, domain.ErrForbidden) {
			t.Fatal(name, e)
		}
	}
	for _, bad := range []dto.Publish{
		{RequestID: "bad", Title: "Valid", Content: "Valid"},
		{RequestID: uuid(), Title: " ", Content: "Valid"},
		{RequestID: uuid(), Title: "Valid", Content: " "},
		{RequestID: uuid(), Title: strings.Repeat("x", 161), Content: "Valid"},
		{RequestID: uuid(), Title: "Valid", Content: strings.Repeat("x", 5001)},
		{RequestID: uuid(), Title: "Line\nBreak", Content: "Valid"},
		{RequestID: uuid(), Title: "Valid", Content: "Bad\x00content"},
		{RequestID: uuid(), Title: "Valid", Content: "Valid", Audience: "staff"},
	} {
		if _, e = s.Publish(ctx, prefix+"admin", bad); !errors.Is(e, domain.ErrInvalid) {
			t.Fatal(bad, e)
		}
	}
	if _, e = s.Get(ctx, prefix+"admin", uuid()); !errors.Is(e, domain.ErrNotFound) {
		t.Fatal(e)
	}
	if _, e = s.Summary(ctx, prefix+"admin", uuid()); !errors.Is(e, domain.ErrNotFound) {
		t.Fatal(e)
	}
	// HTTP validation, role policy and creation/replay status codes.
	mux := http.NewServeMux()
	auth := middleware.Middleware(verifier{}, identityrepo.NewPostgresAccounts(pool), time.Second)
	announcement.Register(handler.New(s, time.Second), mux, auth)
	request := func(method, path, who, body, mime string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "/api/v1/announcements"+path, strings.NewReader(body))
		if who != "" {
			r.Header.Set("Authorization", "Bearer "+prefix+who)
		}
		if mime != "" {
			r.Header.Set("Content-Type", mime)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: want %d got %d %s", method, path, want, w.Code, w.Body.String())
		}
		return w
	}
	body, _ := json.Marshal(dto.Publish{RequestID: uuid(), Title: "HTTP publication", Content: "Plain text"})
	first := request("POST", "", "admin", string(body), "application/json", 201)
	var httpPub domain.Publication
	if json.Unmarshal(first.Body.Bytes(), &httpPub) != nil || httpPub.Replayed {
		t.Fatal(first.Body.String())
	}
	request("POST", "", "admin", string(body), "application/json", 200)
	request("GET", "/"+aid, "student", "", "", 200)
	receipt := request("GET", "/"+aid+"/delivery-summary", "warden", "", "", 200)
	if receipt.Header().Get("Cache-Control") != "no-store" || strings.Contains(receipt.Body.String(), "recipient_id") {
		t.Fatal("summary privacy")
	}
	request("GET", "/"+aid+"/delivery-summary", "student", "", "", 403)
	request("GET", "", "", "", "", 401)
	request("GET", "", "security", "", "", 403)
	request("POST", "", "student", string(body), "application/json", 403)
	request("GET", "/invalid", "admin", "", "", 400)
	request("GET", "/"+aid+"?unknown=1", "student", "", "", 400)
	request("GET", "/"+aid+"/delivery-summary?unknown=1", "admin", "", "", 400)
	request("GET", "", "student", "x", "", 400)
	request("POST", "?unknown=1", "admin", string(body), "application/json", 400)
	request("POST", "", "admin", "{}", "text/plain", 415)
	request("POST", "", "admin", "{", "application/json", 400)
	request("POST", "", "admin", string(body)+"{}", "application/json", 400)
	request("POST", "", "admin", `{"request_id":"`+uuid()+`","title":"Valid","content":"Valid","published_by":"`+student+`"}`, "application/json", 400)
	request("POST", "", "admin", `{"request_id":"`+uuid()+`","title":"Valid","content":"`+strings.Repeat("x", 33000)+`"}`, "application/json", 413)
	for _, q := range []string{"limit=0", "limit=101", "offset=-1", "limit=1&limit=2", "unknown=1", "audience=staff", "limit=bad"} {
		request("GET", "?"+q, "student", "", "", 400)
	}
	// Further successful delivery/replay leaves the first announcement receipts unchanged.
	if _, e = notifications.Deliver(ctx); e != nil {
		t.Fatal(e)
	}
	if summary().Read != 1 || summary().Delivered != 2 {
		t.Fatal("summary changed for unrelated announcements")
	}
}
