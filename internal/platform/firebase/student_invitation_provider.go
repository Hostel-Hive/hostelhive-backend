package firebase

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	firebaseapp "firebase.google.com/go/v4"
	auth "firebase.google.com/go/v4/auth"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

type invitationIdentityClient interface {
	GetUser(context.Context, string) (*auth.UserRecord, error)
	CreateUser(context.Context, *auth.UserToCreate) (*auth.UserRecord, error)
	VerifyIDTokenAndCheckRevoked(context.Context, string) (*auth.Token, error)
}
type StudentInvitationProvider struct {
	identities    invitationIdentityClient
	http          *http.Client
	project, base string
}

func NewStudentInvitationProvider(ctx context.Context, project string) (*StudentInvitationProvider, error) {
	credentialCtx := context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: 5 * time.Second})
	client, e := google.DefaultClient(credentialCtx, "https://www.googleapis.com/auth/identitytoolkit")
	if e != nil {
		return nil, errors.New("initialize Student invitations: check Firebase credentials")
	}
	client.Timeout = 5 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	app, e := firebaseapp.NewApp(ctx, &firebaseapp.Config{ProjectID: project}, option.WithHTTPClient(client))
	if e != nil {
		return nil, d.ErrInvitationUnavailable
	}
	identities, e := app.Auth(ctx)
	if e != nil {
		return nil, d.ErrInvitationUnavailable
	}
	return &StudentInvitationProvider{identities: identities, http: client, project: project, base: "https://identitytoolkit.googleapis.com/v1"}, nil
}

type invitationProviderState struct {
	UID               string  `json:"localId"`
	Email             string  `json:"email"`
	Disabled          bool    `json:"disabled"`
	PasswordUpdatedAt float64 `json:"passwordUpdatedAt"`
}

func (p *StudentInvitationProvider) post(ctx context.Context, path string, input, output any) error {
	raw, e := json.Marshal(input)
	if e != nil {
		return d.ErrInvitationUnavailable
	}
	request, e := http.NewRequestWithContext(ctx, "POST", p.base+path, bytes.NewReader(raw))
	if e != nil {
		return d.ErrInvitationUnavailable
	}
	request.Header.Set("Content-Type", "application/json")
	result, e := p.http.Do(request)
	if e != nil {
		return d.ErrInvitationUnavailable
	}
	defer result.Body.Close()
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		after := time.Duration(0)
		value := result.Header.Get("Retry-After")
		if n, e := strconv.ParseInt(value, 10, 32); e == nil && n > 0 {
			after = time.Duration(n) * time.Second
		} else if at, e := http.ParseTime(value); e == nil && at.After(time.Now()) {
			after = time.Until(at)
		}
		return &d.InvitationRetry{After: after}
	}
	if output == nil {
		return nil
	}
	if json.NewDecoder(io.LimitReader(result.Body, 64*1024)).Decode(output) != nil {
		return d.ErrInvitationUnavailable
	}
	return nil
}
func (p *StudentInvitationProvider) state(ctx context.Context, uid, email string) (invitationProviderState, error) {
	var response struct {
		Users []invitationProviderState `json:"users"`
	}
	e := p.post(ctx, "/projects/"+p.project+"/accounts:lookup", map[string]any{"localId": []string{uid}}, &response)
	if e != nil {
		return invitationProviderState{}, e
	}
	if len(response.Users) != 1 {
		return invitationProviderState{}, d.ErrInvitationIdentity
	}
	v := response.Users[0]
	if v.UID != uid || !strings.EqualFold(v.Email, email) || v.Disabled {
		return v, d.ErrInvitationIdentity
	}
	if math.IsNaN(v.PasswordUpdatedAt) || math.IsInf(v.PasswordUpdatedAt, 0) || v.PasswordUpdatedAt <= 0 || v.PasswordUpdatedAt >= 1e16 {
		return v, d.ErrInvitationUnavailable
	}
	return v, nil
}
func (p *StudentInvitationProvider) Prepare(ctx context.Context, uid, email string) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	user, e := p.identities.GetUser(ctx, uid)
	if auth.IsUserNotFound(e) {
		var random [64]byte
		if _, e = rand.Read(random[:]); e != nil {
			return 0, d.ErrInvitationUnavailable
		}
		password := "Aa1!" + base64.RawURLEncoding.EncodeToString(random[:])
		user, e = p.identities.CreateUser(ctx, (&auth.UserToCreate{}).UID(uid).Email(email).Password(password).Disabled(false))
		if auth.IsEmailAlreadyExists(e) || auth.IsUIDAlreadyExists(e) {
			return 0, d.ErrInvitationIdentity
		}
	}
	if e != nil {
		return 0, d.ErrInvitationUnavailable
	}
	if !matches(user, uid, email) || user.Disabled {
		return 0, d.ErrInvitationIdentity
	}
	// Existing identities are never enabled or assigned another password on retry.
	state, e := p.state(ctx, uid, email)
	return state.PasswordUpdatedAt, e
}
func (p *StudentInvitationProvider) Send(ctx context.Context, uid, email, ip string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, e := p.state(ctx, uid, email); e != nil {
		return e
	}
	// OAuth project binding sends the provider-hosted email. Never request a URL.
	return p.post(ctx, "/accounts:sendOobCode", map[string]any{"requestType": "PASSWORD_RESET", "email": email, "targetProjectId": p.project, "userIp": ip, "returnOobLink": false, "canHandleCodeInApp": false}, nil)
}
func (p *StudentInvitationProvider) VerifySetup(ctx context.Context, uid, email string, baseline float64, raw string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	token, e := p.identities.VerifyIDTokenAndCheckRevoked(ctx, raw)
	if e != nil || token == nil || token.UID != uid || token.Firebase.SignInProvider != "password" {
		return d.ErrSetupPending
	}
	claimed, ok := token.Claims["email"].(string)
	if !ok || !strings.EqualFold(claimed, email) {
		return d.ErrSetupPending
	}
	now := time.Now().Unix()
	if token.AuthTime <= 0 || token.AuthTime > now+30 || now-token.AuthTime > 300 {
		return d.ErrSetupPending
	}
	state, e := p.state(ctx, uid, email)
	if e != nil {
		return e
	}
	if state.PasswordUpdatedAt <= baseline || token.AuthTime < int64(math.Floor(state.PasswordUpdatedAt/1000)) {
		return d.ErrSetupPending
	}
	return nil
}
