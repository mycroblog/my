// SPDX-License-Identifier: MIT

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/platform/platformtest"

	"github.com/mycroblog/my/pkg/mybot"
	"github.com/mycroblog/my/pkg/mykit"
)

// tokenSource signs u in as the web client would: the code of the authorization page exchanged at the token
// endpoint with the PKCE verifier of mykit. Its access token outlives the test.
func tokenSource(t *testing.T, srv *platformtest.Server, u platformtest.User) mykit.TokenSource {
	t.Helper()
	verifier := mykit.NewCodeVerifier()
	code := srv.AuthorizationCode(t, u, "", mykit.CodeChallenge(verifier))
	oauth := mykit.OAuthConfig{Server: srv.URL, ClientID: platformtest.WebClientID, RedirectURI: platformtest.WebClientRedirectURI}
	tok, err := oauth.Exchange(t.Context(), code, verifier)
	if err != nil {
		t.Fatalf("exchange the authorization code of %s: %v", u.Username, err)
	}
	return mykit.NewRefreshingTokenSource(oauth, tok, func(context.Context, *mykit.Token) error { return nil })
}

// client is a mykit client of u.
func client(t *testing.T, srv *platformtest.Server, u platformtest.User) *mykit.API {
	t.Helper()
	api, err := mykit.New(mykit.Config{Server: srv.URL, TokenSource: tokenSource(t, srv, u), AcceptLanguage: "en"})
	if err != nil {
		t.Fatal(err)
	}
	return api
}

// cutter is the transport of the bot, which can cut its event stream and hold the next connection, as a network
// that goes down and comes back.
type cutter struct {
	mu     sync.Mutex
	bodies []*cutBody
	held   chan struct{}
	opened chan struct{} // receives a value for every stream that opens
}

// cutBody is the body of a stream, which reads nothing once it is cut: the data that the server sent after the
// cut is lost, as on a network that went down.
type cutBody struct {
	io.ReadCloser
	cut chan struct{}
}

var errCut = errors.New("the network is down")

func (b *cutBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	select {
	case <-b.cut:
		return 0, errCut
	default:
		return n, err
	}
}

func newCutter() *cutter { return &cutter{opened: make(chan struct{}, 16)} }

func (c *cutter) RoundTrip(req *http.Request) (*http.Response, error) {
	stream := req.URL.Path == "/api/v1/streaming"
	if stream {
		c.mu.Lock()
		held := c.held
		c.mu.Unlock()
		if held != nil {
			select {
			case <-held:
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil || !stream || resp.StatusCode != http.StatusOK {
		return resp, err
	}
	body := &cutBody{ReadCloser: resp.Body, cut: make(chan struct{})}
	resp.Body = body
	c.mu.Lock()
	c.bodies = append(c.bodies, body)
	c.mu.Unlock()
	c.opened <- struct{}{}
	return resp, nil
}

// cut ends the open streams, whose data from now on is lost, and holds the next connection until restore.
func (c *cutter) cut() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.held = make(chan struct{})
	for _, b := range c.bodies {
		close(b.cut)
		_ = b.Close()
	}
	c.bodies = nil
}

func (c *cutter) restore() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.held != nil {
		close(c.held)
		c.held = nil
	}
}

// waitOpen waits for a stream of the bot to open.
func (c *cutter) waitOpen(t *testing.T) {
	t.Helper()
	select {
	case <-c.opened:
	case <-time.After(30 * time.Second):
		t.Fatal("the event stream of the bot did not open")
	}
}

// world is a server with the bot account "bot" and the user alice, who mentions it.
type world struct {
	srv      *platformtest.Server
	bot      platformtest.User
	botToken mykit.TokenSource
	alice    *mykit.API
}

func newWorld(t *testing.T) *world {
	t.Helper()
	srv := platformtest.Start(t, platformtest.Options{})
	bot, alice := srv.User(t, "bot"), srv.User(t, "alice")
	return &world{srv: srv, bot: bot, botToken: tokenSource(t, srv, bot), alice: client(t, srv, alice)}
}

// config is the Config of the echo bot: the bot flag set on start, the store at storePath (kept across the
// restarts of a test), and transport for its requests.
func (w *world) config(storePath string, transport http.RoundTripper) mybot.Config {
	store, err := mybot.NewFileStore(storePath)
	if err != nil {
		panic(err)
	}
	return mybot.Config{
		Server:        w.srv.URL,
		TokenSource:   w.botToken,
		HTTPClient:    &http.Client{Transport: transport},
		Store:         store,
		Logger:        slog.New(slog.DiscardHandler),
		SetBotFlag:    true,
		ShutdownGrace: 200 * time.Millisecond,
	}
}

// echo is the handler of the echo bot: it replies with the text of the mention after the mention of itself.
func echo(c *mybot.Context) error {
	_, err := c.Reply(strings.TrimSpace(strings.TrimPrefix(c.Text, "@"+c.Me().Username)))
	return err
}

// mention posts a public post of alice that mentions the bot.
func (w *world) mention(t *testing.T, text string) mykit.Status {
	t.Helper()
	resp, err := w.alice.CreateStatusWithResponse(t.Context(), &mykit.CreateStatusParams{},
		mykit.CreateStatusRequest{Text: "@bot " + text, Visibility: mykit.VisibilityPublic})
	if err != nil {
		t.Fatalf("alice mentions the bot: %v", err)
	}
	if resp.JSON200 == nil {
		t.Fatalf("alice mentions the bot = %d %s", resp.StatusCode(), resp.Body)
	}
	return *resp.JSON200
}

// replies are the replies to st, as alice sees its thread.
func (w *world) replies(t *testing.T, st mykit.Status) []mykit.Status {
	t.Helper()
	resp, err := w.alice.GetStatusContextWithResponse(t.Context(), st.Id)
	if err != nil {
		t.Fatalf("the thread of %s: %v", st.Id, err)
	}
	if resp.JSON200 == nil {
		t.Fatalf("the thread of %s = %d %s", st.Id, resp.StatusCode(), resp.Body)
	}
	return resp.JSON200.Descendants
}

// waitReply waits for the first reply of the bot to st and returns it.
func (w *world) waitReply(t *testing.T, st mykit.Status) mykit.Status {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if r := w.replies(t, st); len(r) > 0 {
			return r[0]
		}
		if time.Now().After(deadline) {
			t.Fatalf("the bot did not reply to %q", mybot.PlainText(st.Content))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// checkOneReply checks that the bot replied to st once, with want, and with an unlisted reply to the public post.
func (w *world) checkOneReply(t *testing.T, st mykit.Status, want string) {
	t.Helper()
	r := w.replies(t, st)
	if len(r) != 1 {
		texts := make([]string, len(r))
		for i, s := range r {
			texts[i] = mybot.PlainText(s.Content)
		}
		t.Fatalf("replies to %q = %d %q, want 1", mybot.PlainText(st.Content), len(r), texts)
	}
	got := r[0]
	if got.Account.Id != w.bot.AccountID || got.Visibility != mykit.VisibilityUnlisted {
		t.Errorf("reply by %s with visibility %s, want the bot's, unlisted", got.Account.Acct, got.Visibility)
	}
	if text := mybot.PlainText(got.Content); text != "@alice "+want {
		t.Errorf("reply = %q, want %q", text, "@alice "+want)
	}
}

// start runs bot until the returned function stops it, or the test ends.
func start(t *testing.T, bot *mybot.Bot) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- bot.Run(ctx) }()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("Run = %v", err)
				}
			case <-time.After(30 * time.Second):
				t.Error("Run did not return")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

// settle gives a bot that would answer twice the time to do it.
func settle() { time.Sleep(time.Second) }

// TestEchoOnceAcrossStreamCut runs the echo bot against the server: it answers a mention from the event stream
// once, then its stream is cut and a mention comes while it is down, which it answers once by its catch-up after
// it connects again, without answering the first mention, which the catch-up lists too, again.
func TestEchoOnceAcrossStreamCut(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	net := newCutter()
	bot, err := mybot.New(w.config(filepath.Join(t.TempDir(), "state.json"), net))
	if err != nil {
		t.Fatal(err)
	}
	bot.OnMention(echo)
	start(t, bot)
	net.waitOpen(t)

	first := w.mention(t, "hello")
	w.waitReply(t, first)

	net.cut()
	second := w.mention(t, "while you were away")
	// Long enough for the server to have made the notification, which no stream carries to the bot.
	time.Sleep(500 * time.Millisecond)
	if r := w.replies(t, second); len(r) != 0 {
		t.Fatalf("the bot replied %d times to %q with its stream cut", len(r), "while you were away")
	}
	net.restore()
	net.waitOpen(t)
	w.waitReply(t, second)
	settle()
	w.checkOneReply(t, first, "hello")
	w.checkOneReply(t, second, "while you were away")
}

// TestEchoOnceAcrossRestart stops the echo bot in the middle of its handler, after it posted the reply and
// before it returned, so that the mention is not recorded as processed; the bot that starts again on the same
// Store handles the mention again, and its reply, with the same Idempotency-Key, posts nothing new.
func TestEchoOnceAcrossRestart(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	state := filepath.Join(t.TempDir(), "state.json")

	net := newCutter()
	first, err := mybot.New(w.config(state, net))
	if err != nil {
		t.Fatal(err)
	}
	replied := make(chan string, 1)
	first.OnMention(func(c *mybot.Context) error {
		if err := echo(c); err != nil {
			return err
		}
		replied <- c.Notification.Id
		<-c.Done() // the process stops here, before the handler returns
		return c.Err()
	})
	stop := start(t, first)
	net.waitOpen(t)

	st := w.mention(t, "hello")
	var notification string
	select {
	case notification = <-replied:
	case <-time.After(30 * time.Second):
		t.Fatal("the bot did not reply")
	}
	stop()
	if ok, err := first.Store().IsProcessed(t.Context(), notification); err != nil || ok {
		t.Fatalf("IsProcessed(%s) after the stop in the handler = %v, %v; want false", notification, ok, err)
	}

	again, err := mybot.New(w.config(state, newCutter()))
	if err != nil {
		t.Fatal(err)
	}
	handled := make(chan error, 1)
	again.OnMention(func(c *mybot.Context) error {
		err := echo(c)
		handled <- err
		return err
	})
	start(t, again)
	select {
	case err := <-handled:
		if err != nil {
			t.Fatalf("the reply of the restarted bot: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the restarted bot did not handle the mention again")
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		ok, err := again.Store().IsProcessed(t.Context(), notification)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(errors.New("the restarted bot did not record the mention processed"))
		}
		time.Sleep(20 * time.Millisecond)
	}
	settle()
	w.checkOneReply(t, st, "hello")
}

// TestEchoManyMentions answers each of a burst of mentions once.
func TestEchoManyMentions(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	net := newCutter()
	bot, err := mybot.New(w.config(filepath.Join(t.TempDir(), "state.json"), net))
	if err != nil {
		t.Fatal(err)
	}
	bot.OnMention(echo)
	start(t, bot)
	net.waitOpen(t)

	var posts []mykit.Status
	for i := range 5 {
		posts = append(posts, w.mention(t, fmt.Sprintf("number %d", i)))
	}
	for _, st := range posts {
		w.waitReply(t, st)
	}
	settle()
	for i, st := range posts {
		w.checkOneReply(t, st, fmt.Sprintf("number %d", i))
	}
}
