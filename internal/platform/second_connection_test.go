// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// secondConnections are the stacks of the reads on a second connection that this package's tests
// ran on SQLite, by where they come from.
var secondConnections struct {
	sync.Mutex
	stacks map[string][]byte
}

// goroutineHeader and stackAddress are the parts of a stack that differ between runs of a path.
var (
	goroutineHeader = regexp.MustCompile(`goroutine \d+`)
	stackAddress    = regexp.MustCompile(`\(0x[^)]*\)| \+0x[0-9a-f]+`)
)

// checkSecondConnections turns on sqlite.ReportSecondConnections for the tests of the package: a
// transaction must use its own connection, so that a full pool cannot stall the server. It returns
// the check to make after the tests, which reports the paths and returns whether there are none.
func checkSecondConnections() func() bool {
	sqlite.ReportSecondConnections(func(stack []byte) {
		key := goroutineHeader.ReplaceAll(stackAddress.ReplaceAll(stack, nil), nil)
		secondConnections.Lock()
		defer secondConnections.Unlock()
		if secondConnections.stacks == nil {
			secondConnections.stacks = map[string][]byte{}
		}
		secondConnections.stacks[string(key)] = stack
	})
	return func() bool {
		secondConnections.Lock()
		defer secondConnections.Unlock()
		for _, stack := range secondConnections.stacks {
			fmt.Fprintf(os.Stderr, "a read on a second connection while holding a transaction:\n%s\n", stack)
		}
		return len(secondConnections.stacks) == 0
	}
}

// TestOneConnectionIsEnough runs the paths that notify, restrict an account, and deliver on a
// PostgreSQL pool of one connection, where a read on a second connection while holding a
// transaction waits for the request's deadline and fails it.
func TestOneConnectionIsEnough(t *testing.T) {
	dbURL, err := url.Parse(newPostgresDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	q := dbURL.Query()
	q.Set("pool_max_conns", "1")
	dbURL.RawQuery = q.Encode()
	cfg := loadConfig(t, map[string]string{"MY_DB": "postgres", "MY_DATABASE_URL": dbURL.String(), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h, spec := app.Handler(), apitest.Load(t)

	admin, err := app.Identity.CreateAdmin(t.Context(), identity.CreateAdminInput{
		Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	member, err := app.Identity.RegisterLocalUser(t.Context(), identity.RegisterInput{
		Username: "bob", Password: "correct horse battery staple", DisplayName: "Bob", Locale: "en",
	})
	if err != nil {
		t.Fatalf("RegisterLocalUser: %v", err)
	}
	alice := testUser{id: admin.AccountID, token: issueTokens(t, app, admin.User.ID, "").AccessToken}
	bob := testUser{id: member.AccountID, token: issueTokens(t, app, member.User.ID, "").AccessToken}
	do := func(method, path, token string, body any, out any) {
		t.Helper()
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		req := httptest.NewRequestWithContext(ctx, method, "/api/v1"+path, bytes.NewReader(b))
		req.RemoteAddr = "192.0.2.1:1234"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		res := spec.Serve(t, h, req)
		if res.StatusCode/100 != 2 {
			t.Fatalf("%s %s = %d", method, path, res.StatusCode)
		}
		if out != nil {
			if err := json.NewDecoder(res.Body).Decode(out); err != nil {
				t.Fatal(err)
			}
		}
	}

	do(http.MethodPost, "/accounts/"+alice.id+"/follow", bob.token, map[string]any{}, nil)
	var post struct {
		ID string `json:"id"`
	}
	do(http.MethodPost, "/statuses", alice.token, map[string]any{"text": "a post", "visibility": "public"}, &post)
	do(http.MethodPost, "/statuses/"+post.ID+"/favourite", bob.token, map[string]any{}, nil)
	do(http.MethodPost, "/statuses/"+post.ID+"/boost", bob.token, map[string]any{}, nil)
	do(http.MethodPost, "/statuses", bob.token, map[string]any{"text": "@alice hello", "visibility": "public"}, nil)
	do(http.MethodPost, "/admin/accounts/"+bob.id+"/actions", alice.token, map[string]any{"kind": "limit"}, nil)
	do(http.MethodPost, "/statuses/"+post.ID+"/favourite", bob.token, map[string]any{}, nil)

	var page clientapi.NotificationPage
	do(http.MethodGet, "/notifications", alice.token, nil, &page)
	if len(page.Items) == 0 {
		t.Error("no notification came of the follow, the favourite, the boost, or the mention")
	}
	// A deactivation touches many modules in its transaction, and so does the way back.
	do(http.MethodPost, "/accounts/deactivation", bob.token, map[string]any{"password": "correct horse battery staple"}, nil)
	if err := app.Identity.ReactivateOwnAccount(t.Context(), member.User.ID); err != nil {
		t.Fatalf("ReactivateOwnAccount: %v", err)
	}
	if _, err := app.Accounts.ExecuteDueDeletions(t.Context()); err != nil {
		t.Fatalf("ExecuteDueDeletions: %v", err)
	}
	do(http.MethodPost, "/admin/accounts/"+bob.id+"/actions", alice.token, map[string]any{"kind": "suspend"}, nil)
}

// TestDatabaseAcquireTimeoutAnswers503 fills a pool of one connection: requests that wait out
// database_acquire_timeout answer 503 with Retry-After on the client API and on the surfaces that
// answer a bare status, and work again once the connection is free.
func TestDatabaseAcquireTimeoutAnswers503(t *testing.T) {
	dbURL, err := url.Parse(newPostgresDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	q := dbURL.Query()
	q.Set("pool_max_conns", "1")
	dbURL.RawQuery = q.Encode()
	cfg := loadConfig(t, map[string]string{
		"MY_DB": "postgres", "MY_DATABASE_URL": dbURL.String(), "MY_WEB_ORIGIN": webOrigin,
		"MY_DATABASE_ACQUIRE_TIMEOUT": "200ms",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	admin, err := app.Identity.CreateAdmin(t.Context(), identity.CreateAdminInput{
		Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	token := issueTokens(t, app, admin.User.ID, "").AccessToken
	h := app.Handler()
	get := func(path string, header map[string]string) *http.Response {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		req.RemoteAddr = "192.0.2.1:1234"
		for k, v := range header {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Result()
	}
	bearer := map[string]string{"Authorization": "Bearer " + token}
	webfinger := "/.well-known/webfinger?resource=acct:alice@" + cfg.AccountDomain

	held, done, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		finished <- app.UnitOfWork.Tx(context.Background(), func(uow.Tx) error {
			close(held)
			<-done
			return nil
		})
	}()
	<-held
	for name, res := range map[string]*http.Response{
		"client API": get("/api/v1/notifications", bearer),
		"webfinger":  get(webfinger, nil),
	} {
		if res.StatusCode != http.StatusServiceUnavailable || res.Header.Get("Retry-After") != "5" {
			t.Errorf("%s while the pool is full = %d, Retry-After %q; want 503 and 5", name, res.StatusCode, res.Header.Get("Retry-After"))
		}
	}
	close(done)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if res := get("/api/v1/notifications", bearer); res.StatusCode != http.StatusOK {
		t.Errorf("client API once free = %d", res.StatusCode)
	}
}
