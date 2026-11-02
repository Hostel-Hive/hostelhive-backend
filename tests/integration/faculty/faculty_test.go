package faculty_test

import (
	"bytes"
	"context"
	"encoding/csv"
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

	announcementdomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/domain"
	announcementdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/dto"
	announcementrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/repository"
	announcementservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/faculty"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/faculty/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/faculty/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/faculty/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/faculty/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/faculty/service"
	identitydomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	identitydto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/dto"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	notificationdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/dto"
	notificationrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/repository"
	studentdomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	studentdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	studentrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/repository"
	studentservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type verifier struct{}

func (verifier) Verify(_ context.Context, s string) (string, error) { return s, nil }

func TestFacultyLifecycleAndAudience(t *testing.T) {
	database := os.Getenv("TEST_DATABASE_URL")
	if database == "" {
		t.Skip("requires disposable database migrated to 34")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, e := pgxpool.New(ctx, database)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	prefix := "faculty34-" + uuid.NewString()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	scalar := func(sql string, args ...any) string {
		t.Helper()
		var s string
		if e := pool.QueryRow(ctx, sql, args...).Scan(&s); e != nil {
			t.Fatal(e)
		}
		return s
	}
	defer func() {
		cleanup, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		for _, sql := range []string{
			`DELETE FROM hostelhive.announcements WHERE published_by IN(SELECT user_id FROM hostelhive.users WHERE firebase_uid LIKE $1)`,
			`DELETE FROM hostelhive.guardians WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE user_id IN(SELECT user_id FROM hostelhive.users WHERE firebase_uid LIKE $1))`,
			`DELETE FROM hostelhive.students WHERE user_id IN(SELECT user_id FROM hostelhive.users WHERE firebase_uid LIKE $1)`,
			`DELETE FROM hostelhive.faculties WHERE name LIKE $1`,
			`DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1`,
		} {
			if _, e := pool.Exec(cleanup, sql, prefix+"%"); e != nil {
				t.Error(e)
			}
		}
	}()
	users := map[string]string{}
	for _, role := range []string{"admin", "warden", "sub_warden", "security_staff", "student", "other", "new", "inactive"} {
		actual := role
		if role == "other" || role == "new" {
			actual = "student"
		}
		if role == "inactive" {
			actual = "admin"
		}
		users[role] = scalar(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,$4) RETURNING user_id::text`, prefix+role, prefix+role+"@example.invalid", actual, role != "inactive")
	}
	store := repository.New(pool)
	s := service.New(store)
	mux := http.NewServeMux()
	faculty.Register(handler.New(s, 5*time.Second), mux, middleware.Middleware(verifier{}, identityrepo.NewPostgresAccounts(pool), 5*time.Second))
	request := func(method, path, role, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if role != "" {
			r.Header.Set("Authorization", "Bearer "+prefix+role)
		}
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	expect := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("want %d got %d %s", status, w.Code, w.Body)
		}
	}
	parse := func(w *httptest.ResponseRecorder) d.Faculty {
		t.Helper()
		var v d.Faculty
		if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	body := func(name string) string { b, _ := json.Marshal(dto.Details{Name: name}); return string(b) }
	scienceName, artsName := prefix+" Science", prefix+" Arts"
	created := request("POST", "/api/v1/faculties", "admin", body(" "+scienceName+" "))
	expect(created, 201)
	science := parse(created)
	arts, e := s.Create(ctx, prefix+"admin", dto.Details{Name: artsName})
	if e != nil {
		t.Fatal(e)
	}
	if !science.IsActive || science.Name != scienceName || science.FacultyID == "" {
		t.Fatal(science)
	}
	expect(request("POST", "/api/v1/faculties", "admin", body(strings.ToUpper(scienceName))), 409)
	t.Run("role permissions", func(t *testing.T) {
		for _, role := range []string{"admin", "warden", "sub_warden", "security_staff", "student", "inactive", ""} {
			for _, route := range []struct {
				method, path, body string
				write              bool
			}{
				{"GET", "/api/v1/faculties", "", false}, {"GET", "/api/v1/faculties/" + science.FacultyID, "", false},
				{"POST", "/api/v1/faculties", body(scienceName), true}, {"PATCH", "/api/v1/faculties/" + science.FacultyID, body(scienceName), true},
				{"POST", "/api/v1/faculties/" + science.FacultyID + "/activate", "", true}, {"POST", "/api/v1/faculties/" + science.FacultyID + "/deactivate", "", true},
			} {
				// Allowed mutations are tested in the lifecycle below, avoiding extra changes here.
				if route.write && role == "admin" {
					continue
				}
				want := 403
				if !route.write && (role == "admin" || role == "warden") {
					want = 200
				}
				if role == "" {
					want = 401
				}
				expect(request(route.method, route.path, role, route.body), want)
			}
		}
		if _, e := store.SetActive(ctx, prefix+"warden", science.FacultyID, false); !errors.Is(e, d.ErrForbidden) {
			t.Fatal("repository trusted stale/wrong role", e)
		}
	})
	t.Run("strict input", func(t *testing.T) {
		for _, raw := range []string{`{}`, `[]`, `null`, `{"name":""}`, `{"name":"a","extra":1}`, `{"name":"a","name":"b"}`, `{"name":"a"}{}`, `{"name":"Science\nFaculty"}`, body(strings.Repeat("x", 121))} {
			expect(request("POST", "/api/v1/faculties", "admin", raw), 400)
		}
		for _, q := range []string{"?active=no", "?active=true&active=false", "?limit=0", "?offset=-1", "?x=1", "?active=%xx"} {
			expect(request("GET", "/api/v1/faculties"+q, "admin", ""), 400)
		}
		expect(request("GET", "/api/v1/faculties/not-a-uuid", "admin", ""), 400)
		expect(request("GET", "/api/v1/faculties/"+uuid.NewString(), "admin", ""), 404)
		expect(request("POST", "/api/v1/faculties/"+science.FacultyID+"/deactivate", "admin", `{}`), 400)
		expect(request("DELETE", "/api/v1/faculties/"+science.FacultyID, "admin", ""), 405)
	})
	students := studentrepo.NewStore(pool)
	studentAPI := studentservice.New(students)
	details := func(name, index string) studentdto.Details {
		return studentdto.Details{IndexNo: prefix + index, FullName: "Faculty Test Student", Faculty: name, Year: 2, ContactPhone: "0770000000", Guardians: []studentdomain.GuardianInput{{Name: "Test Guardian", Relationship: "Mother", ContactPhone: "0770000001"}}}
	}
	input := studentdto.CreateInput{UserID: users["student"], Details: details(strings.ToUpper(scienceName), "student")}
	member, e := studentAPI.Create(ctx, prefix+"admin", input)
	if e != nil || member.Faculty != scienceName {
		t.Fatal(member, e)
	}
	other, e := studentAPI.Create(ctx, prefix+"admin", studentdto.CreateInput{UserID: users["other"], Details: details(artsName, "other")})
	if e != nil {
		t.Fatal(e)
	}
	notices := announcementservice.New(announcementrepo.New(pool))
	notice := announcementdto.Publish{RequestID: uuid.NewString(), Title: "Science notice", Content: "Science only", Audience: "faculty", Faculty: scienceName}
	published, e := notices.Publish(ctx, prefix+"warden", notice)
	if e != nil {
		t.Fatal(e)
	}
	aid := published.Announcement.AnnouncementID
	inbox := notificationrepo.New(pool)
	if _, e = inbox.Deliver(ctx); e != nil {
		t.Fatal(e)
	}
	t.Run("deactivate preserves records and blocks new work", func(t *testing.T) {
		path := "/api/v1/faculties/" + science.FacultyID + "/deactivate"
		w := request("POST", path, "admin", "")
		expect(w, 200)
		disabled := parse(w)
		if disabled.IsActive {
			t.Fatal(disabled)
		}
		w = request("POST", path, "admin", "")
		expect(w, 200)
		if !parse(w).UpdatedAt.Equal(disabled.UpdatedAt) {
			t.Fatal("repeat changed faculty")
		}
		if n := scalar(`SELECT count(*)::text FROM hostelhive.faculty_audit WHERE faculty_id=$1 AND action='deactivated'`, science.FacultyID); n != "1" {
			t.Fatal(n)
		}
		var active d.Page
		w = request("GET", "/api/v1/faculties?limit=100", "warden", "")
		expect(w, 200)
		_ = json.Unmarshal(w.Body.Bytes(), &active)
		for _, f := range active.Items {
			if f.FacultyID == science.FacultyID {
				t.Fatal("inactive choice leaked")
			}
		}
		var inactive d.Page
		w = request("GET", "/api/v1/faculties?active=false&limit=100", "admin", "")
		expect(w, 200)
		_ = json.Unmarshal(w.Body.Bytes(), &inactive)
		found := false
		for _, f := range inactive.Items {
			found = found || f.FacultyID == science.FacultyID
		}
		if !found {
			t.Fatal("inactive missing from admin filter")
		}
		newInput := studentdto.CreateInput{UserID: users["new"], Details: details(scienceName, "new")}
		if _, e := studentAPI.Create(ctx, prefix+"admin", newInput); !errors.Is(e, studentdomain.ErrInvalid) {
			t.Fatal("inactive registration accepted", e)
		}
		move := details(scienceName, "other")
		if _, e := studentAPI.Update(ctx, prefix+"admin", other.StudentID, move); !errors.Is(e, studentdomain.ErrInvalid) {
			t.Fatal("inactive transfer accepted", e)
		}
		edit := details(scienceName, "student")
		edit.FullName = "Updated existing Student"
		if _, e := studentAPI.Update(ctx, prefix+"admin", member.StudentID, edit); e != nil {
			t.Fatal("existing inactive assignment could not be edited", e)
		}
		iv := identitydto.StudentInvitationInput{RequestID: uuid.NewString(), Email: prefix + "invite@example.invalid", Details: details(scienceName, "invite")}
		if _, _, e := identityrepo.NewInvitationStore(pool).RegisterInvitation(ctx, prefix+"admin", "127.0.0.1", iv, []byte("test")); !errors.Is(e, identitydomain.ErrInvitationInvalid) {
			t.Fatal("inactive invitation accepted", e)
		}
		var b bytes.Buffer
		cw := csv.NewWriter(&b)
		_ = cw.Write(strings.Split(studentservice.CSVHeader, ","))
		g, _ := json.Marshal(newInput.Guardians)
		_ = cw.Write([]string{newInput.UserID, newInput.IndexNo, newInput.FullName, newInput.Faculty, "2", newInput.ContactPhone, string(g)})
		cw.Flush()
		report, e := studentservice.NewImporter(students).Import(ctx, prefix+"admin", b.Bytes())
		if e != nil || report.Rejected != 1 || report.Created != 0 {
			t.Fatal(report, e)
		}
		next := notice
		next.RequestID = uuid.NewString()
		if _, e := notices.Publish(ctx, prefix+"admin", next); !errors.Is(e, announcementdomain.ErrInvalid) {
			t.Fatal("inactive notice accepted", e)
		}
		replay, e := notices.Publish(ctx, prefix+"warden", notice)
		if e != nil || !replay.Replayed {
			t.Fatal("exact retry denied after deactivation", e)
		}
		if _, e := notices.Get(ctx, prefix+"student", aid); e != nil {
			t.Fatal("old notice denied", e)
		}
		if _, e := notices.Get(ctx, prefix+"other", aid); !errors.Is(e, announcementdomain.ErrNotFound) {
			t.Fatal("notice leaked", e)
		}
		p, e := inbox.List(ctx, prefix+"student", notificationdto.Filter{Limit: 100})
		if e != nil || p.Total != 1 {
			t.Fatal("old inbox lost", p, e)
		}
	})
	t.Run("activate same identity and rename safely", func(t *testing.T) {
		w := request("POST", "/api/v1/faculties/"+science.FacultyID+"/activate", "admin", "")
		expect(w, 200)
		enabled := parse(w)
		if !enabled.IsActive || enabled.FacultyID != science.FacultyID {
			t.Fatal(enabled)
		}
		expect(request("POST", "/api/v1/faculties/"+science.FacultyID+"/activate", "admin", ""), 200)
		if scalar(`SELECT count(*)::text FROM hostelhive.faculty_audit WHERE faculty_id=$1 AND action='activated'`, science.FacultyID) != "1" {
			t.Fatal("duplicate activation audit")
		}
		if _, e := studentAPI.Create(ctx, prefix+"admin", studentdto.CreateInput{UserID: users["new"], Details: details(scienceName, "new")}); e != nil {
			t.Fatal("registration still denied after activation", e)
		}
		renamed := prefix + " Natural Sciences"
		w = request("PATCH", "/api/v1/faculties/"+science.FacultyID, "admin", body(renamed))
		expect(w, 200)
		if parse(w).FacultyID != science.FacultyID {
			t.Fatal("rename replaced identity")
		}
		if got, e := students.Get(ctx, member.StudentID); e != nil || got.Faculty != renamed {
			t.Fatal(got, e)
		}
		history, e := notices.Get(ctx, prefix+"student", aid)
		if e != nil || history.Faculty == nil || *history.Faculty != strings.ToLower(scienceName) {
			t.Fatal("historical snapshot/access changed", history, e)
		}
		if _, e := notices.Get(ctx, prefix+"other", aid); !errors.Is(e, announcementdomain.ErrNotFound) {
			t.Fatal("rename leaked to other faculty", e)
		}
		p, e := inbox.List(ctx, prefix+"student", notificationdto.Filter{Limit: 100})
		if e != nil || p.Total != 1 {
			t.Fatal("rename lost inbox", p, e)
		}
		expect(request("POST", "/api/v1/faculties", "admin", body(scienceName)), 409)
		expect(request("PATCH", "/api/v1/faculties/"+arts.FacultyID, "admin", body(scienceName)), 409)
		retry, e := notices.Publish(ctx, prefix+"warden", notice)
		if e != nil || !retry.Replayed {
			t.Fatal("rename broke old exact retry", e)
		}
		next := notice
		next.RequestID = uuid.NewString()
		if _, e := notices.Publish(ctx, prefix+"admin", next); !errors.Is(e, announcementdomain.ErrInvalid) {
			t.Fatal("former name accepted for new publication", e)
		}
		next.Faculty = renamed
		if _, e := notices.Publish(ctx, prefix+"admin", next); e != nil {
			t.Fatal("new name could not publish", e)
		}
		if _, e := inbox.Deliver(ctx); e != nil {
			t.Fatal(e)
		}
		p, e = inbox.List(ctx, prefix+"new", notificationdto.Filter{Limit: 100})
		if e != nil || p.Total != 1 {
			t.Fatal("new audience delivery failed", p, e)
		}
	})
	t.Run("global notices still reach inactive faculty", func(t *testing.T) {
		if _, e := s.SetActive(ctx, prefix+"admin", science.FacultyID, false); e != nil {
			t.Fatal(e)
		}
		global, e := notices.Publish(ctx, prefix+"warden", announcementdto.Publish{RequestID: uuid.NewString(), Title: "All Students", Content: "All faculties"})
		if e != nil {
			t.Fatal(e)
		}
		if _, e := inbox.Deliver(ctx); e != nil {
			t.Fatal(e)
		}
		for _, role := range []string{"student", "other", "new"} {
			if _, e := notices.Get(ctx, prefix+role, global.Announcement.AnnouncementID); e != nil {
				t.Fatal(role, e)
			}
			page, e := inbox.List(ctx, prefix+role, notificationdto.Filter{Limit: 100})
			if e != nil {
				t.Fatal(e)
			}
			found := false
			for _, item := range page.Items {
				found = found || item.AnnouncementID == global.Announcement.AnnouncementID
			}
			if !found {
				t.Fatal("global notification missing", role)
			}
		}
		if _, e := s.SetActive(ctx, prefix+"admin", science.FacultyID, true); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("audit failure rolls back faculty aliases and students", func(t *testing.T) {
		before, e := s.Get(ctx, prefix+"admin", science.FacultyID)
		if e != nil {
			t.Fatal(e)
		}
		exec(fmt.Sprintf(`CREATE FUNCTION hostelhive.test_faculty_audit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.faculty_id='%s'::uuid THEN RAISE EXCEPTION 'forced audit failure'; END IF; RETURN NEW; END $$`, science.FacultyID))
		defer func() {
			if _, e := pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS test_faculty_audit_failure ON hostelhive.faculty_audit; DROP FUNCTION IF EXISTS hostelhive.test_faculty_audit_failure()`); e != nil {
				t.Error(e)
			}
		}()
		exec(`CREATE TRIGGER test_faculty_audit_failure BEFORE INSERT ON hostelhive.faculty_audit FOR EACH ROW EXECUTE FUNCTION hostelhive.test_faculty_audit_failure()`)
		if _, e := s.Rename(ctx, prefix+"admin", science.FacultyID, dto.Details{Name: prefix + " Failed rename"}); !errors.Is(e, d.ErrUnavailable) {
			t.Fatal("audit failure not surfaced", e)
		}
		if _, e := s.SetActive(ctx, prefix+"admin", science.FacultyID, false); !errors.Is(e, d.ErrUnavailable) {
			t.Fatal("deactivation audit failure not surfaced", e)
		}
		after, e := s.Get(ctx, prefix+"admin", science.FacultyID)
		if e != nil || after.Name != before.Name || !after.IsActive || !after.UpdatedAt.Equal(before.UpdatedAt) {
			t.Fatal("partial faculty write", after, e)
		}
		profile, e := students.Get(ctx, member.StudentID)
		if e != nil || profile.Faculty != before.Name {
			t.Fatal("partial profile rename", profile, e)
		}
		if scalar(`SELECT count(*)::text FROM hostelhive.faculty_names WHERE name_key=lower($1)`, prefix+" Failed rename") != "0" {
			t.Fatal("partial alias write")
		}
	})
	t.Run("current admin authority is rechecked", func(t *testing.T) {
		exec(`UPDATE hostelhive.users SET role='warden' WHERE user_id=$1`, users["admin"])
		defer exec(`UPDATE hostelhive.users SET role='admin' WHERE user_id=$1`, users["admin"])
		if _, e := s.SetActive(ctx, prefix+"admin", science.FacultyID, false); !errors.Is(e, d.ErrForbidden) {
			t.Fatal("role downgrade bypassed", e)
		}
	})
	t.Run("concurrent duplicate creates", func(t *testing.T) {
		var wg sync.WaitGroup
		out := make(chan int, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out <- request("POST", "/api/v1/faculties", "admin", body(prefix+" Concurrent")).Code
			}()
		}
		wg.Wait()
		close(out)
		counts := map[int]int{}
		for code := range out {
			counts[code]++
		}
		if counts[201] != 1 || counts[409] != 1 {
			t.Fatal(counts)
		}
	})
	t.Run("migration rollback fails closed", func(t *testing.T) {
		raw, e := os.ReadFile("../../../migrations/000034_faculty_management.down.sql")
		if e != nil {
			t.Fatal(e)
		}
		conn, e := pool.Acquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer conn.Release()
		_, e = conn.Exec(ctx, string(raw))
		_, rollbackErr := conn.Exec(ctx, "ROLLBACK")
		if e == nil || !strings.Contains(e.Error(), "Cannot roll back faculty management") || rollbackErr != nil {
			t.Fatal(e, rollbackErr)
		}
	})
}
