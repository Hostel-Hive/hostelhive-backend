package students

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/authentication"
)

const testID = "11111111-1111-1111-1111-111111111111"

func validDetails() Details {
	return Details{IndexNo: "SC/2026/001", FullName: "Test Student", Faculty: "Science", Year: 1, ContactPhone: "+94 771234567", Guardians: []GuardianInput{{Name: "Test Guardian", Relationship: "Parent", ContactPhone: "0771234567"}}}
}

type verifier struct{}

func (verifier) Verify(context.Context, string) (string, error) { return "verified-admin", nil }

type accounts struct {
	role   string
	active bool
}

func (a accounts) FindByFirebaseUID(context.Context, string) (authentication.Account, error) {
	return authentication.Account{FirebaseUID: "verified-admin", Role: a.role, IsActive: a.active}, nil
}

type fakeStore struct {
	calls     int
	err       error
	actor, id string
	input     CreateInput
	filter    Filter
}

func (f *fakeStore) List(ctx context.Context, q Filter) (Page, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("missing deadline")
	}
	f.calls++
	f.filter = q
	return Page{Students: []Profile{}, Limit: q.Limit, Offset: q.Offset}, f.err
}
func (f *fakeStore) Get(context.Context, string) (Profile, error) {
	f.calls++
	return Profile{StudentID: testID}, f.err
}
func (f *fakeStore) Create(_ context.Context, actor string, v CreateInput) (Profile, error) {
	f.calls++
	f.actor = actor
	f.input = v
	return Profile{StudentID: testID, UserID: v.UserID, FullName: v.FullName}, f.err
}
func (f *fakeStore) Update(_ context.Context, actor, id string, v Details) (Profile, error) {
	f.calls++
	f.actor = actor
	f.id = id
	return Profile{StudentID: id, FullName: v.FullName}, f.err
}
func (f *fakeStore) Delete(_ context.Context, actor, id string) error {
	f.calls++
	f.actor = actor
	f.id = id
	return f.err
}
func handler(s *fakeStore, role string, active bool) http.Handler {
	m := http.NewServeMux()
	NewAPI(s, time.Second).Register(m, authentication.Middleware(verifier{}, accounts{role, active}, time.Second))
	return m
}
func request(h http.Handler, method, path, body string, token bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token {
		r.Header.Set("Authorization", "Bearer token")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func body(v any) string { b, _ := json.Marshal(v); return string(b) }
func TestAllStudentRoutesRequireAdmin(t *testing.T) {
	routes := []struct{ method, path, body string }{{"GET", "/api/v1/students", ""}, {"GET", "/api/v1/students/" + testID, ""}, {"POST", "/api/v1/students", body(CreateInput{UserID: testID, Details: validDetails()})}, {"PUT", "/api/v1/students/" + testID, body(validDetails())}, {"DELETE", "/api/v1/students/" + testID, ""}}
	for _, role := range []string{"admin", "warden", "sub_warden", "security_staff", "student"} {
		for _, r := range routes {
			s := &fakeStore{}
			w := request(handler(s, role, true), r.method, r.path, r.body, true)
			want := 403
			if role == "admin" {
				want = 200
				if r.method == "POST" {
					want = 201
				}
				if r.method == "DELETE" {
					want = 204
				}
			}
			if w.Code != want || (role != "admin" && s.calls != 0) {
				t.Fatalf("%s %s %d", role, r.path, w.Code)
			}
			w = request(handler(&fakeStore{}, role, true), r.method, r.path, r.body, false)
			if w.Code != 401 {
				t.Fatal("missing token accepted")
			}
			w = request(handler(&fakeStore{}, "admin", false), r.method, r.path, r.body, true)
			if w.Code != 403 {
				t.Fatal("inactive admin accepted")
			}
		}
	}
}
func TestStudentHTTPValidationAndResponses(t *testing.T) {
	for _, tc := range []struct{ method, path, body string }{{"GET", "/api/v1/students?limit=0", ""}, {"GET", "/api/v1/students?limit=101", ""}, {"GET", "/api/v1/students?offset=-1", ""}, {"GET", "/api/v1/students?offset=1000001", ""}, {"GET", "/api/v1/students?year=0", ""}, {"GET", "/api/v1/students?year=11", ""}, {"GET", "/api/v1/students?limit=1&limit=2", ""}, {"GET", "/api/v1/students?unknown=x", ""}, {"GET", "/api/v1/students?q=%0A", ""}, {"GET", "/api/v1/students?faculty=", ""}, {"GET", "/api/v1/students?year=no", ""}, {"GET", "/api/v1/students?x=%zz", ""}, {"GET", "/api/v1/students/not-uuid", ""}, {"POST", "/api/v1/students", `{}`}, {"POST", "/api/v1/students", `{"role":"admin"}`}, {"POST", "/api/v1/students", strings.Repeat("x", 17000)}, {"POST", "/api/v1/students", body(CreateInput{UserID: testID, Details: validDetails()}) + ` {}`}, {"PUT", "/api/v1/students/" + testID, body(CreateInput{UserID: testID, Details: validDetails()})}, {"DELETE", "/api/v1/students/" + testID, `{}`}} {
		s := &fakeStore{}
		w := request(handler(s, "admin", true), tc.method, tc.path, tc.body, true)
		if w.Code != 400 || s.calls != 0 {
			t.Fatalf("invalid %s %s got %d", tc.method, tc.path, w.Code)
		}
	}
	s := &fakeStore{}
	w := request(handler(s, "admin", true), "GET", "/api/v1/students?q=test&faculty=Science&year=2&limit=3&offset=4", "", true)
	if w.Code != 200 || s.filter.Q != "test" || s.filter.Year != 2 || s.filter.Faculty != "Science" || s.filter.Limit != 3 || s.filter.Offset != 4 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("filter not passed correctly")
	}
	s = &fakeStore{}
	w = request(handler(s, "admin", true), "POST", "/api/v1/students", body(CreateInput{UserID: testID, Details: validDetails()}), true)
	if w.Code != 201 || s.actor != "verified-admin" || w.Header().Get("Location") != "/api/v1/students/"+testID {
		t.Fatal("wrong creator or location")
	}
	for _, tc := range []struct {
		err  error
		code int
	}{{ErrNotFound, 404}, {ErrConflict, 409}, {ErrAccount, 409}, {ErrForbidden, 403}, {ErrInvalid, 400}, {errors.New("secret database detail"), 503}} {
		s = &fakeStore{err: tc.err}
		w = request(handler(s, "admin", true), "GET", "/api/v1/students/"+testID, "", true)
		if w.Code != tc.code || strings.Contains(w.Body.String(), "secret") {
			t.Fatal("unsafe error response")
		}
	}
	s = &fakeStore{}
	w = request(handler(s, "admin", true), "DELETE", "/api/v1/students/"+testID, "", true)
	if w.Code != 204 || w.Body.Len() != 0 || s.id != testID || s.actor != "verified-admin" {
		t.Fatal("delete response wrong")
	}
}
func TestProfileValidation(t *testing.T) {
	for _, change := range []func(*Details){func(v *Details) { v.IndexNo = "" }, func(v *Details) { v.IndexNo = strings.Repeat("x", 65) }, func(v *Details) { v.FullName = "\n" }, func(v *Details) { v.FullName = string([]byte{0xff}) }, func(v *Details) { v.Faculty = "" }, func(v *Details) { v.Year = 0 }, func(v *Details) { v.Year = 11 }, func(v *Details) { v.ContactPhone = "-------" }, func(v *Details) { v.ContactPhone = "abc1234567" }, func(v *Details) { v.Guardians = nil }, func(v *Details) { v.Guardians[0].Name = "" }, func(v *Details) { v.Guardians[0].Relationship = "" }, func(v *Details) { v.Guardians[0].ContactPhone = "123" }} {
		v := validDetails()
		change(&v)
		if _, err := Normalize(v); !errors.Is(err, ErrInvalid) {
			t.Fatal("accepted invalid profile")
		}
	}
	v := validDetails()
	v.FullName = "  \u0dc3\u0dc0\u0dd2 Student  "
	normalized, err := Normalize(v)
	if err != nil || strings.HasPrefix(normalized.FullName, " ") {
		t.Fatal("Unicode or trimming failed")
	}
	v = validDetails()
	v.IndexNo = strings.Repeat("x", 64)
	v.FullName = strings.Repeat("\u754c", 200)
	v.Faculty = strings.Repeat("x", 120)
	v.Year = 10
	if _, err = Normalize(v); err != nil {
		t.Fatal("valid boundaries rejected")
	}
}

func TestInvalidUTF8AndGuardianLimits(t *testing.T) {
	s := &fakeStore{}
	invalid := strings.Replace(body(CreateInput{UserID: testID, Details: validDetails()}), "Test Student", string([]byte{0xff}), 1)
	if w := request(handler(s, "admin", true), "POST", "/api/v1/students", invalid, true); w.Code != 400 || s.calls != 0 {
		t.Fatal("invalid UTF-8 accepted")
	}
	v := validDetails()
	guardian := v.Guardians[0]
	v.Guardians = nil
	for n := 0; n < 10; n++ {
		v.Guardians = append(v.Guardians, guardian)
	}
	if _, err := Normalize(v); err != nil {
		t.Fatal("ten guardians rejected")
	}
	v.Guardians = append(v.Guardians, guardian)
	if _, err := Normalize(v); !errors.Is(err, ErrInvalid) {
		t.Fatal("more than ten guardians accepted")
	}
}
