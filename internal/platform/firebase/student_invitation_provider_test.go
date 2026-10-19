package firebase

import (
	"context"
	"encoding/json"
	"errors"
	firebaseapp "firebase.google.com/go/v4"
	auth "firebase.google.com/go/v4/auth"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type invitationTokenFake struct {
	token *auth.Token
	err   error
}

func (f *invitationTokenFake) GetUser(context.Context, string) (*auth.UserRecord, error) {
	return &auth.UserRecord{UserInfo: &auth.UserInfo{UID: "reserved", Email: "student@stu.kln.ac.lk"}}, nil
}
func (f *invitationTokenFake) CreateUser(context.Context, *auth.UserToCreate) (*auth.UserRecord, error) {
	panic("unexpected create")
}
func (f *invitationTokenFake) VerifyIDTokenAndCheckRevoked(context.Context, string) (*auth.Token, error) {
	return f.token, f.err
}
func TestInvitationProviderPasswordSetupProof(t *testing.T) {
	now := time.Now().Unix()
	updated := float64(now * 1000)
	fake := &invitationTokenFake{}
	calls := 0
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if !strings.HasSuffix(r.URL.Path, "/projects/project/accounts:lookup") {
			t.Fatal("wrong lookup")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"users":[{"localId":"reserved","email":"student@stu.kln.ac.lk","passwordUpdatedAt":` + jsonNumber(updated) + `}]}`)), Header: http.Header{}}, nil
	})
	p := &StudentInvitationProvider{identities: fake, http: &http.Client{Transport: transport}, project: "project", base: "https://identitytoolkit.googleapis.com/v1"}
	valid := func() *auth.Token {
		return &auth.Token{UID: "reserved", AuthTime: now, Firebase: auth.FirebaseInfo{SignInProvider: "password"}, Claims: map[string]any{"email": "student@stu.kln.ac.lk"}}
	}
	for _, edit := range []func(*auth.Token){
		func(v *auth.Token) { v.UID = "other" }, func(v *auth.Token) { v.AuthTime = now - 301 }, func(v *auth.Token) { v.AuthTime = now + 100 },
		func(v *auth.Token) { v.Firebase.SignInProvider = "google.com" }, func(v *auth.Token) { v.Claims["email"] = "other@stu.kln.ac.lk" },
		func(v *auth.Token) { delete(v.Claims, "email") },
	} {
		fake.token = valid()
		edit(fake.token)
		if e := p.VerifySetup(context.Background(), "reserved", "student@stu.kln.ac.lk", updated-1000, "token"); !errors.Is(e, d.ErrSetupPending) {
			t.Fatal("unverified setup", e)
		}
	}
	if calls != 0 {
		t.Fatal("invalid proof reached metadata lookup")
	}
	fake.token = valid()
	if e := p.VerifySetup(context.Background(), "reserved", "student@stu.kln.ac.lk", updated, "token"); !errors.Is(e, d.ErrSetupPending) {
		t.Fatal("unchanged password")
	}
	if e := p.VerifySetup(context.Background(), "reserved", "student@stu.kln.ac.lk", updated-1000, "token"); e != nil {
		t.Fatal("valid proof", e)
	}
	fake.err = errors.New("private token")
	if e := p.VerifySetup(context.Background(), "reserved", "student@stu.kln.ac.lk", updated-1000, "token"); !errors.Is(e, d.ErrSetupPending) {
		t.Fatal("revoked token")
	}
}
func jsonNumber(v float64) string { b, _ := json.Marshal(v); return string(b) }
func TestInvitationProviderUsesReservedIdentityAndHostedEmail(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "")
	exists, creates, sends := false, 0, 0
	password := ""
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Fatal(e)
		}
		status := 200
		result := "{}"
		switch {
		case strings.HasSuffix(r.URL.Path, "/accounts:lookup"):
			if body["localId"].([]any)[0] != "reserved" {
				t.Fatal("unreserved identity")
			}
			if exists {
				result = `{"users":[{"localId":"reserved","email":"student@stu.kln.ac.lk","passwordUpdatedAt":1000}]}`
			} else {
				result = `{"users":[]}`
			}
		case strings.HasSuffix(r.URL.Path, "/accounts"):
			creates++
			exists = true
			password, _ = body["password"].(string)
			if len(password) < 40 || body["localId"] != "reserved" || body["email"] != "student@stu.kln.ac.lk" || body["disabled"] != false {
				t.Fatal("bad bootstrap identity")
			}
			result = `{"localId":"reserved","email":"student@stu.kln.ac.lk"}`
		case strings.HasSuffix(r.URL.Path, "/accounts:sendOobCode"):
			sends++
			if body["requestType"] != "PASSWORD_RESET" || body["targetProjectId"] != "project" || body["returnOobLink"] != false || body["userIp"] != "192.0.2.1" {
				t.Fatal("unsafe email request")
			}
		default:
			t.Fatal("unexpected endpoint", r.URL.Path)
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(result)), Request: r}, nil
	})
	clientHTTP := &http.Client{Transport: transport}
	app, e := firebaseapp.NewApp(context.Background(), &firebaseapp.Config{ProjectID: "project"}, option.WithTokenSource(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "offline"})), option.WithHTTPClient(clientHTTP))
	if e != nil {
		t.Fatal(e)
	}
	client, e := app.Auth(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	p := &StudentInvitationProvider{identities: client, http: clientHTTP, project: "project", base: "https://identitytoolkit.googleapis.com/v1"}
	for n := 0; n < 2; n++ {
		if value, e := p.Prepare(context.Background(), "reserved", "student@stu.kln.ac.lk"); e != nil || value != 1000 {
			t.Fatal("prepare", value, e)
		}
	}
	if creates != 1 || password == "" {
		t.Fatal("retry created or reset identity")
	}
	if e := p.Send(context.Background(), "reserved", "student@stu.kln.ac.lk", "192.0.2.1"); e != nil || sends != 1 {
		t.Fatal("send", e)
	}
}
func TestInvitationProviderRejectsBadMetadataAndHonorsRetryAfter(t *testing.T) {
	for _, response := range []string{`{"users":[]}`, `{"users":[{"localId":"other","email":"student@stu.kln.ac.lk","passwordUpdatedAt":1000}]}`, `{"users":[{"localId":"reserved","email":"other@stu.kln.ac.lk","passwordUpdatedAt":1000}]}`, `{"users":[{"localId":"reserved","email":"student@stu.kln.ac.lk","disabled":true,"passwordUpdatedAt":1000}]}`, `{"users":[{"localId":"reserved","email":"student@stu.kln.ac.lk","passwordUpdatedAt":0}]}`, `invalid`} {
		p := &StudentInvitationProvider{identities: &invitationTokenFake{}, project: "project", base: "https://example.invalid", http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
		})}}
		if _, e := p.Prepare(context.Background(), "reserved", "student@stu.kln.ac.lk"); e == nil {
			t.Fatal("bad metadata accepted", response)
		}
	}
	p := &StudentInvitationProvider{project: "project", base: "https://example.invalid", http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"600"}}, Body: io.NopCloser(strings.NewReader("private provider action code"))}, nil
	})}}
	e := p.post(context.Background(), "/test", map[string]string{}, nil)
	var retry *d.InvitationRetry
	if !errors.As(e, &retry) || retry.After != 600*time.Second || strings.Contains(e.Error(), "private") {
		t.Fatal("unsafe retry metadata", e)
	}
}
