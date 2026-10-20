package announcement_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

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
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFacultyAnnouncementIsolation(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable database migrated to 29")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := fmt.Sprintf("faculty-%d-", time.Now().UnixNano())
	scalar := func(sql string, args ...any) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	uuid := func() string { return scalar("SELECT gen_random_uuid()::text") }
	users := map[string]string{}
	for _, name := range []string{"admin", "warden", "one", "two", "arts", "inactive", "archived"} {
		role := "student"
		if name == "admin" || name == "warden" {
			role = name
		}
		users[name] = scalar(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text`, prefix+name, prefix+name+"@example.invalid", role)
	}
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		for _, sql := range []string{`DELETE FROM hostelhive.announcements WHERE published_by IN(SELECT user_id FROM hostelhive.users WHERE firebase_uid LIKE $1)`, `DELETE FROM hostelhive.students WHERE index_no LIKE $1`, `DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`} {
			if _, err := pool.Exec(clean, sql, prefix+"%"); err != nil {
				t.Error(err)
			}
		}
	}()
	science, arts := prefix+"science", prefix+"arts"
	for _, name := range []string{"one", "two", "arts", "inactive", "archived"} {
		faculty := science
		if name == "arts" {
			faculty = arts
		}
		if name == "two" {
			faculty = strings.ToUpper(science)
		}
		exec(`INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Faculty Student',$3,1,'0771234567')`, users[name], prefix+name, faculty)
	}
	exec(`UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, users["inactive"])
	exec(`UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE user_id=$1`, users["archived"], users["admin"])
	s := service.New(repository.New(pool))
	inbox := nrepo.New(pool)
	mux := http.NewServeMux()
	announcement.Register(handler.New(s, 5*time.Second), mux, middleware.Middleware(verifier{}, identityrepo.NewPostgresAccounts(pool), 5*time.Second))
	request := func(method, path, name, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if name != "" {
			r.Header.Set("Authorization", "Bearer "+prefix+name)
		}
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	input := dto.Publish{RequestID: uuid(), Title: "Science notice", Content: "For this faculty only", Audience: "faculty", Faculty: " " + strings.ToUpper(science) + " "}
	raw, _ := json.Marshal(input)
	response := request("POST", "/api/v1/announcements", "admin", string(raw))
	if response.Code != 201 {
		t.Fatal(response.Code, response.Body)
	}
	var published domain.Publication
	if err = json.Unmarshal(response.Body.Bytes(), &published); err != nil {
		t.Fatal(err)
	}
	aid := published.Announcement.AnnouncementID
	if published.Announcement.Faculty == nil || *published.Announcement.Faculty != science {
		t.Fatal("faculty missing or not canonical", published)
	}
	global, err := s.Publish(ctx, prefix+"warden", dto.Publish{RequestID: uuid(), Title: "Global", Content: "For everybody"})
	if err != nil || global.Announcement.Faculty != nil {
		t.Fatal(global, err)
	}
	replay, err := s.Publish(ctx, prefix+"admin", input)
	if err != nil || !replay.Replayed || replay.Announcement.AnnouncementID != aid {
		t.Fatal("unsafe retry", replay, err)
	}
	changed := input
	changed.Faculty = arts
	if _, err = s.Publish(ctx, prefix+"admin", changed); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("changed target accepted on retry", err)
	}
	unknown := input
	unknown.RequestID = uuid()
	unknown.Faculty = prefix + "unknown"
	if _, err = s.Publish(ctx, prefix+"admin", unknown); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("unknown faculty accepted", err)
	}
	for name, want := range map[string]int64{"one": 2, "two": 2, "arts": 1, "admin": 2, "warden": 2} {
		page, e := s.List(ctx, prefix+name, dto.Filter{Limit: 1})
		if e != nil || page.Total != want || len(page.Items) != 1 || page.HasMore != (want > 1) {
			t.Fatal(name, page, e)
		}
	}
	for name, want := range map[string]int{"one": 200, "two": 200, "arts": 404, "admin": 200, "warden": 200, "inactive": 403, "archived": 403, "": 401} {
		w := request("GET", "/api/v1/announcements/"+aid, name, "")
		if w.Code != want {
			t.Fatal(name, w.Code, w.Body)
		}
	}
	if w := request("POST", "/api/v1/announcements", "one", string(raw)); w.Code != 403 {
		t.Fatal(w.Code, w.Body)
	}
	if w := request("GET", "/api/v1/announcements/"+aid+"/delivery-summary", "one", ""); w.Code != 403 {
		t.Fatal(w.Code, w.Body)
	}
	summary, err := s.Summary(ctx, prefix+"admin", aid)
	if err != nil || summary.RecipientsTotal != 2 || summary.PendingDelivery != 2 {
		t.Fatal("recipient snapshot", summary, err)
	}
	if _, err = inbox.Deliver(ctx); err != nil {
		t.Fatal(err)
	}
	list := func(name string) ndomain.Page {
		t.Helper()
		p, e := inbox.List(ctx, prefix+name, ndto.Filter{Limit: 100})
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	var alert string
	for _, item := range list("one").Items {
		if item.AnnouncementID == aid {
			alert = item.NotificationID
		}
	}
	if alert == "" || list("arts").Total != 1 {
		t.Fatal("faculty alert missing or leaked")
	}
	read, err := inbox.MarkRead(ctx, prefix+"one", alert)
	if err != nil || read.ReadAt == nil {
		t.Fatal(read, err)
	}
	// A committed faculty transfer removes access while preserving historical receipts.
	transfer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer transfer.Rollback(context.Background())
	if _, err = transfer.Exec(ctx, `UPDATE hostelhive.students SET faculty=$2 WHERE user_id=$1`, users["one"], arts); err != nil {
		t.Fatal(err)
	}
	blocked := make(chan error, 1)
	go func() { _, e := s.Get(ctx, prefix+"one", aid); blocked <- e }()
	select {
	case e := <-blocked:
		t.Fatal("read bypassed uncommitted faculty transfer", e)
	case <-time.After(20 * time.Millisecond):
	}
	if err = transfer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if e := <-blocked; !errors.Is(e, domain.ErrNotFound) {
		t.Fatal("stale faculty read", e)
	}
	if list("one").Total != 1 {
		t.Fatal("transferred Student kept old faculty inbox")
	}
	unread, err := inbox.Unread(ctx, prefix+"one")
	if err != nil || unread.Unread != 1 {
		t.Fatal("ineligible alert counted", unread, err)
	}
	if _, err = inbox.MarkRead(ctx, prefix+"one", alert); !errors.Is(err, ndomain.ErrNotFound) {
		t.Fatal("ineligible alert marked read", err)
	}
	exec(`UPDATE hostelhive.students SET faculty=$2 WHERE user_id=$1`, users["arts"], science)
	if _, err = s.Get(ctx, prefix+"arts", aid); err != nil {
		t.Fatal("new faculty feed denied", err)
	}
	if list("arts").Total != 1 {
		t.Fatal("faculty change created retroactive alert")
	}
	if _, err = s.Publish(ctx, prefix+"admin", input); err != nil {
		t.Fatal(err)
	}
	summary, err = s.Summary(ctx, prefix+"admin", aid)
	if err != nil || summary.RecipientsTotal != 2 || summary.Delivered != 2 || summary.Read != 1 || summary.Unread != 1 || summary.PendingDelivery != 0 {
		t.Fatal("historical summary or retry changed audience", summary, err)
	}
	exec(`UPDATE hostelhive.students SET faculty=$2 WHERE user_id=$1`, users["one"], science)
	restored, err := inbox.MarkRead(ctx, prefix+"one", alert)
	if err != nil || restored.ReadAt == nil || !restored.ReadAt.Equal(*read.ReadAt) {
		t.Fatal("original read timestamp lost", restored, err)
	}
}
