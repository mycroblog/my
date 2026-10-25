// SPDX-License-Identifier: MIT

package mybot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/pkg/mykit"
)

// fakeServer is the part of the client API that a bot uses, in memory.
type fakeServer struct {
	t   *testing.T
	srv *httptest.Server

	mu            sync.Mutex
	me            mykit.CredentialAccount
	notifications []mykit.Notification // newest first
	posts         []posted
	reactions     []string
	listCalls     int
	streamCalls   int
	botUpdates    []bool
	refreshTokens []string
	// stream serves the n-th connection (from 1) of GET /streaming; nil keeps it open until the client leaves.
	stream func(w http.ResponseWriter, r *http.Request, n int)
	// tokens maps a valid refresh token to the one that replaces it.
	tokens map[string]string
	scope  string
}

// posted is a request of createStatus.
type posted struct {
	key string
	req mykit.CreateStatusRequest
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	f := &fakeServer{
		t:      t,
		me:     mykit.CredentialAccount{Id: id(0), Username: "bot", Acct: "bot", Domain: "example.com", Bot: true},
		tokens: map[string]string{},
		scope:  "read write",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/accounts/verify_credentials", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		writeJSON(w, f.me)
	})
	mux.HandleFunc("PATCH /api/v1/accounts/update_credentials", func(w http.ResponseWriter, r *http.Request) {
		var req mykit.UpdateCredentialsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if req.Bot != nil {
			f.botUpdates = append(f.botUpdates, *req.Bot)
			f.me.Bot = *req.Bot
		}
		writeJSON(w, f.me)
	})
	mux.HandleFunc("GET /api/v1/notifications", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.listCalls++
		items := f.notifications
		start := 0
		if c := r.URL.Query().Get("cursor"); c != "" {
			_, _ = fmt.Sscanf(c, "%d", &start)
		}
		limit := 2 // small pages, so that the catch-up follows the cursor
		end := min(start+limit, len(items))
		page := mykit.NotificationPage{Items: append([]mykit.Notification{}, items[start:end]...)}
		if end < len(items) {
			next := fmt.Sprint(end)
			page.NextCursor = &next
		}
		writeJSON(w, page)
	})
	mux.HandleFunc("POST /api/v1/statuses", func(w http.ResponseWriter, r *http.Request) {
		var req mykit.CreateStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.posts = append(f.posts, posted{key: r.Header.Get("Idempotency-Key"), req: req})
		writeJSON(w, mykit.Status{Id: id(9000 + len(f.posts)), Account: mykit.Account{Id: f.me.Id}, Content: req.Text, Visibility: req.Visibility})
	})
	mux.HandleFunc("PUT /api/v1/statuses/{id}/reaction", func(w http.ResponseWriter, r *http.Request) {
		var req mykit.ReactToStatusRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.reactions = append(f.reactions, r.PathValue("id")+" "+req.Emoji)
		writeJSON(w, map[string]any{"id": r.PathValue("id")})
	})
	mux.HandleFunc("GET /api/v1/streaming", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.streamCalls++
		n, serve := f.streamCalls, f.stream
		f.mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		if serve != nil {
			serve(w, r, n)
			return
		}
		<-r.Context().Done()
	})
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rt := r.PostForm.Get("refresh_token")
		f.mu.Lock()
		defer f.mu.Unlock()
		f.refreshTokens = append(f.refreshTokens, rt)
		next, ok := f.tokens[rt]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
			return
		}
		delete(f.tokens, rt)
		f.tokens[next] = next + "'"
		writeJSON(w, map[string]any{"access_token": "at-" + next, "token_type": "Bearer", "expires_in": 3600, "refresh_token": next, "scope": f.scope})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// add puts a notification at the top of the list.
func (f *fakeServer) add(n mykit.Notification) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notifications = append([]mykit.Notification{n}, f.notifications...)
}

func (f *fakeServer) postsSoFar() []posted {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]posted{}, f.posts...)
}

// config is a Config of a bot of f, in Mode Polling with a MemoryStore.
func (f *fakeServer) config() Config {
	return Config{
		Server:      f.srv.URL,
		TokenSource: mykit.StaticToken("token"),
		Store:       NewMemoryStore(),
		Logger:      slog.New(slog.DiscardHandler),
		Mode:        Polling,
	}
}

// writeEvent writes one event of the stream and flushes it.
func writeEvent(w http.ResponseWriter, name string, data any) {
	b, _ := json.Marshal(data)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
	w.(http.Flusher).Flush()
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// id is a UUIDv7-shaped ID whose order is the order of n.
func id(n int) string { return fmt.Sprintf("01900000-0000-7000-8000-%012d", n) }

// account is an account of another user.
func account(n int, acct string) *mykit.Account {
	return &mykit.Account{Id: id(n), Username: strings.Split(acct, "@")[0], Acct: acct}
}

// mention is a mention notification n from a, of a post with text and visibility v.
func mention(n int, a *mykit.Account, text string, v mykit.Visibility) mykit.Notification {
	return mykit.Notification{
		Id:        id(n),
		Type:      mykit.NotificationTypeMention,
		CreatedAt: time.Now(),
		Account:   a,
		Status: &mykit.Status{
			Id:         id(n + 500),
			Account:    *a,
			Content:    `<p><span class="h-card"><a href="https://example.com/@bot" class="u-url mention">@<span>bot</span></a></span> ` + text + `</p>`,
			Visibility: v,
			Mentions:   []mykit.StatusMention{{Id: id(0), Acct: "bot"}, {Id: id(77), Acct: "carol"}},
		},
	}
}

// runBot runs b until stop is closed or the test ends, and returns the error of Run.
func runBot(t *testing.T, b *Bot) (stop func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	var once sync.Once
	var err error
	stop = func() error {
		once.Do(func() {
			cancel()
			select {
			case err = <-done:
			case <-time.After(15 * time.Second):
				t.Fatal("Run did not return")
			}
		})
		return err
	}
	t.Cleanup(func() { _ = stop() })
	return stop
}

// eventually waits for cond, failing the test after a few seconds.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// processed reports whether the store has recorded every one of ids.
func processed(s Store, ids ...string) bool {
	for _, i := range ids {
		if ok, _ := s.IsProcessed(context.Background(), i); !ok {
			return false
		}
	}
	return true
}
