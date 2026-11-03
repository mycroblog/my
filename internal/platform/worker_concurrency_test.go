// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// postgresURLWithPool returns the URL of a new PostgreSQL database with a pool of size connections.
func postgresURLWithPool(t *testing.T, size int) string {
	t.Helper()
	u, err := url.Parse(newPostgresDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("pool_max_conns", strconv.Itoa(size))
	u.RawQuery = q.Encode()
	return u.String()
}

// TestWorkerRunsRealJobsAtOnce runs real handlers in several slots, on both dialects and with a
// selection of job types: posts become the status notifications of their subscribers in jobs that
// notify, alongside jobs that do nothing but overlap. Run it with -race.
func TestWorkerRunsRealJobsAtOnce(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  func(t *testing.T) map[string]string
	}{
		{"sqlite", func(t *testing.T) map[string]string {
			return map[string]string{"MY_DATABASE_URL": newDatabase(t, true)}
		}},
		{"postgres", func(t *testing.T) map[string]string {
			return map[string]string{"MY_DATABASE": "postgres", "MY_DATABASE_URL": postgresURLWithPool(t, 16)}
		}},
		{"postgres with a selection of job types", func(t *testing.T) map[string]string {
			return map[string]string{"MY_DATABASE": "postgres", "MY_DATABASE_URL": postgresURLWithPool(t, 16), "MY_WORKER_JOB_TYPES_EXCEPT": "media.*"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.env(t)
			env["MY_WEB_ORIGIN"] = webOrigin
			env["MY_WORKER_JOB_CONCURRENCY"] = "8"
			// A selection is resolved when the app starts, before a test can register a handler.
			var o *overlap
			if env["MY_WORKER_JOB_TYPES_EXCEPT"] == "" {
				o = newOverlap(4)
			}
			s := start(t, loadConfig(t, env), platform.ServeOptions{}, func(a *platform.App) {
				if o != nil {
					a.Handlers.Register("test.overlap", o.handler)
				}
			})
			exerciseSlots(t, s.app, o)
		})
	}
}

// overlap is a job that finishes when want of its runs are in progress at once, which they reach
// only in several slots.
type overlap struct {
	mu      sync.Mutex
	running int
	want    int
	reached chan struct{}
}

func newOverlap(want int) *overlap { return &overlap{want: want, reached: make(chan struct{})} }

func (o *overlap) handler(ctx context.Context, _ queue.Job) queue.Result {
	o.mu.Lock()
	o.running++
	if o.running == o.want {
		close(o.reached)
	}
	o.mu.Unlock()
	select {
	case <-o.reached:
		return queue.Done()
	case <-ctx.Done():
		return queue.Retry(0, ctx.Err())
	}
}

func exerciseSlots(t *testing.T, app *platform.App, o *overlap) {
	t.Helper()
	const subscribers, posts = 4, 8
	ctx := t.Context()
	h, spec := app.Handler(), apitest.Load(t)
	do := func(method, path, token string, body any, out any) {
		t.Helper()
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		req := httptest.NewRequestWithContext(reqCtx, method, "/api/v1"+path, bytes.NewReader(b))
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
	admin, err := app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{
		Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	author := testUser{id: admin.AccountID, token: issueTokens(t, app, admin.User.ID, "").AccessToken}
	readers := make([]testUser, subscribers)
	for i := range readers {
		member, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{
			Username: fmt.Sprintf("reader%d", i), Password: "correct horse battery staple", DisplayName: "Reader", Locale: "en",
		})
		if err != nil {
			t.Fatalf("RegisterLocalUser: %v", err)
		}
		readers[i] = testUser{id: member.AccountID, token: issueTokens(t, app, member.User.ID, "").AccessToken}
		do(http.MethodPost, "/accounts/"+author.id+"/subscribe", readers[i].token, map[string]any{}, nil)
	}

	if o != nil {
		jobs := make([]queue.NewJob, o.want)
		for i := range jobs {
			jobs[i] = queue.NewJob{Type: "test.overlap"}
		}
		if err := app.Jobs.Enqueue(ctx, jobs...); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	for i := range posts {
		do(http.MethodPost, "/statuses", author.token, map[string]any{"text": fmt.Sprintf("post %d", i), "visibility": "public"}, nil)
	}

	if o != nil {
		select {
		case <-o.reached:
		case <-time.After(30 * time.Second):
			t.Fatalf("%d jobs were never in progress at once", o.want)
		}
	}
	for deadline := time.Now().Add(60 * time.Second); ; {
		short := 0
		for _, r := range readers {
			var page clientapi.NotificationPage
			do(http.MethodGet, "/notifications?limit=40", r.token, nil, &page)
			if len(page.Items) < posts {
				short++
			}
		}
		if short == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d subscribers have not got all %d notifications", short, subscribers, posts)
		}
		time.Sleep(50 * time.Millisecond)
	}
	dead, err := app.DeadJobs.ListDeadJobs(ctx, "", queue.DeadJobPosition{}, 10)
	if err != nil {
		t.Fatalf("ListDeadJobs: %v", err)
	}
	if len(dead) != 0 {
		t.Errorf("dead jobs: %+v", dead)
	}
}

func TestWorkerJobConcurrencyAgainstThePool(t *testing.T) {
	const fails = "must be below"
	for _, tc := range []struct {
		name        string
		pool        int
		concurrency string
		wantErr     bool
	}{
		{"below the pool", 4, "3", false},
		{"at the pool", 4, "4", true},
		{"above the pool", 4, "5", true},
		{"not given on a pool of one", 1, "", false},
		{"given as one on a pool of one", 1, "1", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{"MY_DATABASE": "postgres", "MY_DATABASE_URL": postgresURLWithPool(t, tc.pool)}
			if tc.concurrency != "" {
				env["MY_WORKER_JOB_CONCURRENCY"] = tc.concurrency
			}
			a, err := platform.NewApp(t.Context(), loadConfig(t, env), platform.AppOptions{})
			if err == nil {
				t.Cleanup(func() { _ = a.Close() })
			}
			switch {
			case tc.wantErr && (err == nil || !strings.Contains(err.Error(), fails)):
				t.Errorf("NewApp = %v, want an error with %q", err, fails)
			case !tc.wantErr && err != nil:
				t.Errorf("NewApp = %v", err)
			}
		})
	}
	t.Run("a web process does not run jobs", func(t *testing.T) {
		env := map[string]string{"MY_DATABASE": "postgres", "MY_DATABASE_URL": postgresURLWithPool(t, 1), "MY_ROLE": "web", "MY_WORKER_JOB_CONCURRENCY": "1"}
		a, err := platform.NewApp(t.Context(), loadConfig(t, env), platform.AppOptions{})
		if err != nil {
			t.Fatalf("NewApp = %v", err)
		}
		_ = a.Close()
	})
}

func TestWorkerJobConcurrencyWarnsOnSQLite(t *testing.T) {
	t.Parallel()
	const warning = "many jobs at once"
	for _, tc := range []struct {
		concurrency string
		want        bool
	}{{"32", false}, {"33", true}} {
		t.Run(tc.concurrency, func(t *testing.T) {
			t.Parallel()
			cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_ROLE": "worker", "MY_WORKER_JOB_CONCURRENCY": tc.concurrency})
			s := start(t, cfg, platform.ServeOptions{}, nil)
			for deadline := time.Now().Add(5 * time.Second); !strings.Contains(s.logs.String(), `"started"`) && time.Now().Before(deadline); {
				time.Sleep(10 * time.Millisecond)
			}
			if got := strings.Contains(s.logs.String(), warning); got != tc.want {
				t.Errorf("warning logged = %v, want %v:\n%s", got, tc.want, s.logs.String())
			}
		})
	}
}
