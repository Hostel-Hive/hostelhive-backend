package firebase

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	sdk "firebase.google.com/go/v4"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
)

// Exercise the real SDK's conditional HTTP writes, including a concurrent
// winner delivered as a 412 response. The fake server is loopback only.
func TestRealtimeConditionalWrites(t *testing.T) {
	var mu sync.Mutex
	data := map[string]json.RawMessage{}
	versions := map[string]int{}
	conflict := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		path := r.URL.Path
		current := data[path]
		if current == nil {
			current = json.RawMessage("null")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", fmt.Sprintf("\"%d\"", versions[path]))
		if r.Method == "GET" {
			w.Write(current)
			return
		}
		if r.Method != "PUT" {
			w.WriteHeader(405)
			return
		}
		if conflict && strings.Contains(path, "attendance") {
			conflict = false
			data[path] = json.RawMessage(`{"version":200,"changed_at":1,"published_at":2,"summary":{"total":2,"in":1,"out":1,"unknown":0}}`)
			versions[path]++
			w.Header().Set("ETag", fmt.Sprintf("\"%d\"", versions[path]))
			w.WriteHeader(412)
			w.Write(data[path])
			return
		}
		if r.Header.Get("If-Match") != fmt.Sprintf("\"%d\"", versions[path]) {
			w.WriteHeader(412)
			w.Write(current)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		data[path] = body
		versions[path]++
		w.Write(body)
	}))
	defer server.Close()
	t.Setenv("FIREBASE_DATABASE_EMULATOR_HOST", strings.Replace(strings.TrimPrefix(server.URL, "http://"), "127.0.0.1", "localhost", 1)+"?ns=demo-hostelhive")
	app, err := sdk.NewApp(context.Background(), &sdk.Config{ProjectID: "demo-hostelhive", DatabaseURL: "https://demo-hostelhive.firebaseio.com"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := app.Database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	publisher := &RealtimeDatabase{client}
	ctx := context.Background()
	projection := domain.Projection{Version: 100, ChangedAt: 1, PublishedAt: 2, Summary: domain.PresenceSummary{Total: 1, In: 1}}
	if err = publisher.Publish(ctx, projection); err != nil {
		t.Fatal(err)
	}
	var received domain.Projection
	if err = client.NewRef("hostelhive/attendance").Get(ctx, &received); err != nil || received.Version != 200 || received.Summary.Total != 2 {
		t.Fatal("older write replaced concurrent winner", received, err)
	}
	projection.Version = 201
	if err = publisher.Publish(ctx, projection); err != nil {
		t.Fatal(err)
	}
	if err = publisher.Publish(ctx, projection); err != nil {
		t.Fatal("idempotent replay", err)
	}
	if err = client.NewRef("hostelhive/attendance").Get(ctx, &received); err != nil || received.Version != 201 {
		t.Fatal(received, err)
	}
	if err = publisher.SetGrant(ctx, "warden", domain.Grant{Version: 2, ExpiresAt: 0}); err != nil {
		t.Fatal(err)
	}
	if err = publisher.SetGrant(ctx, "warden", domain.Grant{Version: 1, ExpiresAt: 99999}); err != nil {
		t.Fatal(err)
	}
	var grant domain.Grant
	if err = client.NewRef("hostelhive/staffAccess/warden").Get(ctx, &grant); err != nil || grant.ExpiresAt != 0 || grant.Version != 2 {
		t.Fatal("revoked grant resurrected", grant, err)
	}
	projection.Summary.Total = 99
	if publisher.Publish(ctx, projection) == nil {
		t.Fatal("invalid counts accepted")
	}
	for _, uid := range []string{"", "bad/uid", "bad.uid"} {
		if publisher.SetGrant(ctx, uid, grant) == nil {
			t.Fatal("invalid UID accepted")
		}
	}
	if _, err = NewRealtimeDatabase(ctx, "demo-hostelhive", "https://demo-hostelhive.firebaseio.com"); err == nil {
		t.Fatal("application enabled emulator routing")
	}
}
