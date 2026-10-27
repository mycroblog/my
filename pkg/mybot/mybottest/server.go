// SPDX-License-Identifier: MIT

package mybottest

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mycroblog/my/pkg/mybot"
	"github.com/mycroblog/my/pkg/mykit"
)

// Domain is the domain of the server's accounts.
const Domain = "bot.test"

// Token is the access token of Server.Config; the server answers 401 to a request without an access token.
const Token = "mybottest-token"

// heartbeat is the interval of the comments that keep an event stream alive, as the server sends them.
const heartbeat = 30 * time.Second

// Server is a fake server of the client API for the tests of a bot: an httptest.Server that answers the operations
// that mybot calls (verifyAccountCredentials, updateAccountCredentials, listNotifications, streamEvents,
// createStatus, and those of the helpers of mybot.Context: reactToStatus, favouriteStatus, followAccount) with
// the types of api/openapi.yaml, and keeps in memory what the bot did. Inject sends a notification to the bot,
// Posted returns what it posted, and Clock moves the time of its scheduled jobs.
//
// Like the server, it posts once per Idempotency-Key: a reply that the bot sends again after a restart in the
// middle of its handler answers with the first post and adds none to Posted.
type Server struct {
	srv   *httptest.Server
	clock *Clock

	mu            sync.Mutex
	seq           int
	me            mykit.CredentialAccount
	notifications []mykit.Notification // newest first
	statuses      map[string]mykit.Status
	posts         []Post
	keys          map[string]int // Idempotency-Key -> index in posts
	reactions     []Reaction
	favourites    []string
	follows       []string
	streams       map[*stream]struct{}
	// held, while Disconnect holds the connections, is closed by Reconnect.
	held    chan struct{}
	changed chan struct{}
	// opened, when a test sets it, runs after an event stream has answered.
	opened func()
}

// Post is a post that the bot created with createStatus.
type Post struct {
	// IdempotencyKey is the Idempotency-Key of the request, such as mybot.ReplyIdempotencyKey of the event.
	IdempotencyKey string
	// Request is the body of the request.
	Request mykit.CreateStatusRequest
	// Status is the post as the server answered it.
	Status mykit.Status
}

// Reaction is an emoji reaction of the bot to a status (reactToStatus).
type Reaction struct {
	StatusID string
	Emoji    string
}

// stream is an open event stream.
type stream struct {
	events chan event
	drop   chan struct{}
}

type event struct {
	name string
	data []byte
}

// ServerOption is an option of NewServer.
type ServerOption func(*serverOptions)

type serverOptions struct {
	clockAt time.Time
}

// WithClockAt starts the server's Clock at the time at instead of the current time. A test that moves the Clock
// to fixed times (Clock.Set) starts it at or before the first of them, so that it runs the same on any day.
func WithClockAt(at time.Time) ServerOption {
	return func(o *serverOptions) { o.clockAt = at }
}

// NewServer starts a server whose own account, Me, is the bot account "bot" with the bot flag, and whose Clock
// is at the current time, or at the time of WithClockAt. Close it when the test ends.
func NewServer(opts ...ServerOption) *Server {
	o := serverOptions{clockAt: time.Now().UTC().Truncate(time.Second)}
	for _, opt := range opts {
		opt(&o)
	}
	s := &Server{
		clock:    NewClock(o.clockAt),
		statuses: map[string]mykit.Status{},
		keys:     map[string]int{},
		streams:  map[*stream]struct{}{},
		changed:  make(chan struct{}),
	}
	s.me = credentialAccount(s.newID(), "bot", s.clock.Now())
	s.srv = httptest.NewServer(s.handler())
	return s
}

// Close stops the server, ending the event streams.
func (s *Server) Close() {
	s.Reconnect()
	s.DropStreams()
	s.srv.Close()
}

// URL is the origin of the server, for mybot.Config.Server.
func (s *Server) URL() string { return s.srv.URL }

// Clock is the clock of the server: Config sets it as the bot's, and the notifications and posts take its time.
func (s *Server) Clock() *Clock { return s.clock }

// Config is a mybot.Config of a bot of the server: its URL, the access token Token, a mybot.MemoryStore, the
// server's Clock, and a logger that discards. Change the fields the test needs before mybot.New.
func (s *Server) Config() mybot.Config {
	return mybot.Config{
		Server:      s.URL(),
		TokenSource: mykit.StaticToken(Token),
		Store:       mybot.NewMemoryStore(),
		Clock:       s.clock,
		Logger:      slog.New(slog.DiscardHandler),
	}
}

// Me is the bot's own account, as verifyAccountCredentials answers it.
func (s *Server) Me() mykit.CredentialAccount {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.me
}

// SetBot sets the bot flag of the bot's own account, such as to off to test mybot.Config.SetBotFlag.
func (s *Server) SetBot(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.me.Bot = on
}

// Account returns a new account of another user, with acct such as "alice" (a local account) or
// "alice@remote.example" (a remote one).
func (s *Server) Account(acct string) *mykit.Account {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := account(s.newID(), acct, s.clock.Now())
	return &a
}

// Status returns a new post of from with the plain text text, whose content mentions the accounts of mentions
// (none for a post that mentions nobody) at its start, as the server renders them.
func (s *Server) Status(from *mykit.Account, text string, v mykit.Visibility, mentions ...*mykit.Account) mykit.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.newStatus(*from, text, v)
	var prefix strings.Builder
	for _, m := range mentions {
		st.Mentions = append(st.Mentions, mykit.StatusMention{Id: m.Id, Acct: m.Acct, Username: m.Username, Url: m.Url})
		fmt.Fprintf(&prefix, `<span class="h-card"><a href="%s" class="u-url mention">@<span>%s</span></a></span> `,
			html.EscapeString(m.Url), html.EscapeString(m.Username))
	}
	st.Content = "<p>" + prefix.String() + html.EscapeString(text) + "</p>"
	s.statuses[st.Id] = st
	return st
}

// Mention returns a new mention notification: a post of from with visibility v that mentions the bot, then says
// text, such as Mention(alice, "ping", mykit.VisibilityPublic) for "@bot ping". Hand it to Inject.
func (s *Server) Mention(from *mykit.Account, text string, v mykit.Visibility) mykit.Notification {
	me := s.Me()
	bot := mykit.Account{Id: me.Id, Acct: me.Acct, Username: me.Username, Url: me.Url}
	st := s.Status(from, text, v, &bot)
	return mykit.Notification{Type: mykit.NotificationTypeMention, Account: from, Status: &st}
}

// Notification returns a new notification of type t from from about st (nil for one without a post, such as a
// follow). Hand it to Inject.
func (s *Server) Notification(t mykit.NotificationType, from *mykit.Account, st *mykit.Status) mykit.Notification {
	return mykit.Notification{Type: t, Account: from, Status: st}
}

// Inject adds the notification n and sends it on the open event streams, as the server does when it happens. An
// empty ID and creation time are filled in (a new ID, newer than those before, and the Clock's time); it returns
// n as it was added. A bot whose stream is not open yet finds it when it catches up.
func (s *Server) Inject(n mykit.Notification) mykit.Notification {
	n = s.add(n)
	s.broadcast("notification", n)
	return n
}

// InjectMissed adds the notification n without sending it on the event streams, as one that came while the bot
// was stopped: only the bot's catch-up (listNotifications) finds it, when the bot starts or connects again. The
// catch-up reads back to the newest notification that the bot processed, so add the missed ones before the bot
// runs, or between Disconnect and Reconnect. It fills n in as Inject does.
func (s *Server) InjectMissed(n mykit.Notification) mykit.Notification {
	return s.add(n)
}

// InjectStatus sends the post st on the open event streams as a new post of the home timeline (the event update,
// mybot.Bot.OnStatus).
func (s *Server) InjectStatus(st mykit.Status) {
	s.mu.Lock()
	st = normalizeStatus(st)
	s.statuses[st.Id] = st
	s.mu.Unlock()
	s.broadcast("update", st)
}

// DropStreams ends every open event stream, as the server does when it shuts down: the bot connects again and
// catches up.
func (s *Server) DropStreams() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropLocked()
}

// Disconnect ends every open event stream and holds the bot's next connections until Reconnect, as a network that
// is down: the notifications that Inject adds meanwhile reach the bot only by its catch-up after Reconnect.
func (s *Server) Disconnect() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held == nil {
		s.held = make(chan struct{})
	}
	s.dropLocked()
}

// Reconnect lets the connections that Disconnect holds through.
func (s *Server) Reconnect() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held != nil {
		close(s.held)
		s.held = nil
	}
}

func (s *Server) dropLocked() {
	for st := range s.streams {
		close(st.drop)
		delete(s.streams, st)
	}
	s.notifyLocked()
}

// Streams is how many event streams are open.
func (s *Server) Streams() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.streams)
}

// Notifications returns the notifications of the server, newest first, as listNotifications answers them.
func (s *Server) Notifications() []mykit.Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.notifications)
}

// Posted returns the posts that the bot created, oldest first: one per Idempotency-Key.
func (s *Server) Posted() []Post {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.posts)
}

// Reactions returns the reactions of the bot, oldest first; reacting again with the same emoji adds none.
func (s *Server) Reactions() []Reaction {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.reactions)
}

// Favourites returns the IDs of the statuses that the bot favourited, oldest first.
func (s *Server) Favourites() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.favourites)
}

// Follows returns the IDs of the accounts that the bot followed, oldest first.
func (s *Server) Follows() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.follows)
}

// WaitPosted waits until the bot has created at least n posts and returns them, or returns the error of ctx when
// it ends first.
func (s *Server) WaitPosted(ctx context.Context, n int) ([]Post, error) {
	err := s.wait(ctx, func() bool { return len(s.posts) >= n })
	return s.Posted(), err
}

// WaitStreams waits until at least n event streams are open, or returns the error of ctx when it ends first.
func (s *Server) WaitStreams(ctx context.Context, n int) error {
	return s.wait(ctx, func() bool { return len(s.streams) >= n })
}

// wait waits until cond, called with s.mu held, holds.
func (s *Server) wait(ctx context.Context, cond func() bool) error {
	for {
		s.mu.Lock()
		ok, changed := cond(), s.changed
		s.mu.Unlock()
		if ok {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *Server) notifyLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

// newID is a new UUIDv7-shaped ID, after every ID before it in their order, as the server's IDs are.
func (s *Server) newID() string {
	s.seq++
	return fmt.Sprintf("01900000-0000-7000-8000-%012d", s.seq)
}

func (s *Server) add(n mykit.Notification) mykit.Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n.Id == "" {
		n.Id = s.newID()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = s.clock.Now()
	}
	n = normalizeNotification(n)
	if n.Status != nil {
		s.statuses[n.Status.Id] = *n.Status
	}
	s.notifications = slices.Insert(s.notifications, 0, n)
	s.notifyLocked()
	return n
}

func (s *Server) broadcast(name string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for st := range s.streams {
		select {
		case st.events <- event{name, b}:
		default:
			// A stream that does not keep up is ended, as the server ends it.
			close(st.drop)
			delete(s.streams, st)
		}
	}
}

// newStatus is a post of a by the server, at the Clock's time, with s.mu held.
func (s *Server) newStatus(a mykit.Account, text string, v mykit.Visibility) mykit.Status {
	id := s.newID()
	return normalizeStatus(mykit.Status{
		Id:         id,
		Account:    a,
		Content:    "<p>" + html.EscapeString(text) + "</p>",
		CreatedAt:  s.clock.Now(),
		Visibility: v,
		Uri:        "https://" + Domain + "/statuses/" + id,
	})
}

// handler is the HTTP handler of the operations.
func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/accounts/verify_credentials", s.verifyCredentials)
	mux.HandleFunc("PATCH /api/v1/accounts/update_credentials", s.updateCredentials)
	mux.HandleFunc("GET /api/v1/notifications", s.listNotifications)
	mux.HandleFunc("GET /api/v1/streaming", s.streamEvents)
	mux.HandleFunc("POST /api/v1/statuses", s.createStatus)
	mux.HandleFunc("PUT /api/v1/statuses/{id}/reaction", s.reactToStatus)
	mux.HandleFunc("POST /api/v1/statuses/{id}/favourite", s.favouriteStatus)
	mux.HandleFunc("POST /api/v1/accounts/{id}/follow", s.followAccount)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			writeProblem(w, http.StatusUnauthorized, "/api/v1/problems/unauthenticated", "The operation needs an access token.")
			return
		}
		if _, pattern := mux.Handler(r); pattern == "" {
			writeProblem(w, http.StatusNotFound, "about:blank", "mybottest does not serve "+r.Method+" "+r.URL.Path+".")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) verifyCredentials(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.Me())
}

func (s *Server) updateCredentials(w http.ResponseWriter, r *http.Request) {
	var req mykit.UpdateCredentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "about:blank", err.Error())
		return
	}
	s.mu.Lock()
	if req.Bot != nil {
		s.me.Bot = *req.Bot
	}
	if req.DisplayName != nil {
		s.me.DisplayName = *req.DisplayName
	}
	me := s.me
	s.mu.Unlock()
	writeJSON(w, me)
}

// listNotifications answers the notifications newest first, a page of limit (20 by default, 40 at most) from the
// opaque cursor.
func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 20
	if l := q.Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 || n > 40 {
			writeProblem(w, http.StatusBadRequest, "about:blank", "limit is from 1 to 40.")
			return
		}
		limit = n
	}
	types := q["types"]
	s.mu.Lock()
	items := slices.Clone(s.notifications)
	s.mu.Unlock()
	if len(types) > 0 {
		items = slices.DeleteFunc(items, func(n mykit.Notification) bool { return !slices.Contains(types, string(n.Type)) })
	}
	// The cursor is the ID of the last notification of the previous page.
	start := 0
	if c := q.Get("cursor"); c != "" {
		start = len(items)
		for i, n := range items {
			if n.Id < c {
				start = i
				break
			}
		}
	}
	end := min(start+limit, len(items))
	page := mykit.NotificationPage{Items: items[start:end]}
	if end < len(items) {
		next := items[end-1].Id
		page.NextCursor = &next
	}
	writeJSON(w, page)
}

func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	// Subscribe before answering: once the bot sees the stream open and catches up, every notification that
	// Inject adds reaches it on the stream or in the list, however late this goroutine runs.
	st := &stream{events: make(chan event, 64), drop: make(chan struct{})}
	for {
		s.mu.Lock()
		held := s.held
		if held == nil {
			s.streams[st] = struct{}{}
			s.notifyLocked()
			s.mu.Unlock()
			break
		}
		s.mu.Unlock()
		select {
		case <-held:
		case <-r.Context().Done():
			return
		}
	}
	defer func() {
		s.mu.Lock()
		if _, ok := s.streams[st]; ok {
			delete(s.streams, st)
			s.notifyLocked()
		}
		s.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}
	if s.opened != nil {
		s.opened()
	}
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	for {
		select {
		case e := <-st.events:
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.name, e.data)
		case <-ticker.C:
			_, _ = io.WriteString(w, ": heartbeat\n\n")
		case <-st.drop:
			return
		case <-r.Context().Done():
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func (s *Server) createStatus(w http.ResponseWriter, r *http.Request) {
	var req mykit.CreateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "about:blank", err.Error())
		return
	}
	if req.Visibility == "" {
		writeProblem(w, http.StatusUnprocessableEntity, "about:blank", "visibility is required.")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	s.mu.Lock()
	defer s.mu.Unlock()
	if i, ok := s.keys[key]; ok && key != "" {
		writeJSON(w, s.posts[i].Status)
		return
	}
	me := s.me
	st := s.newStatus(mykit.Account{
		Id: me.Id, Acct: me.Acct, Username: me.Username, Domain: me.Domain, Bot: me.Bot, CreatedAt: me.CreatedAt,
		Uri: me.Uri, Url: me.Url,
	}, req.Text, req.Visibility)
	st.Account = normalizeAccount(st.Account)
	if req.InReplyToId != nil {
		st.InReplyToId = req.InReplyToId
		if parent, ok := s.statuses[*req.InReplyToId]; ok {
			st.InReplyToAccountId = &parent.Account.Id
		}
	}
	if req.SpoilerText != nil {
		st.SpoilerText = *req.SpoilerText
	}
	if req.Language != nil {
		st.Language = *req.Language
	}
	s.statuses[st.Id] = st
	if key != "" {
		s.keys[key] = len(s.posts)
	}
	s.posts = append(s.posts, Post{IdempotencyKey: key, Request: req, Status: st})
	s.notifyLocked()
	writeJSON(w, st)
}

func (s *Server) reactToStatus(w http.ResponseWriter, r *http.Request) {
	var req mykit.ReactToStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Emoji == "" {
		writeProblem(w, http.StatusBadRequest, "about:blank", "emoji is required.")
		return
	}
	id := r.PathValue("id")
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.statusLocked(w, id)
	if !ok {
		return
	}
	if rc := (Reaction{StatusID: id, Emoji: req.Emoji}); !slices.Contains(s.reactions, rc) {
		s.reactions = append(s.reactions, rc)
		s.notifyLocked()
	}
	writeJSON(w, st)
}

func (s *Server) favouriteStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.statusLocked(w, id)
	if !ok {
		return
	}
	if !slices.Contains(s.favourites, id) {
		s.favourites = append(s.favourites, id)
		s.notifyLocked()
	}
	on := true
	st.Favourited = &on
	writeJSON(w, st)
}

func (s *Server) followAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.Contains(s.follows, id) {
		s.follows = append(s.follows, id)
		s.notifyLocked()
	}
	writeJSON(w, mykit.Relationship{Id: id, Following: true, ShowBoosts: true})
}

// statusLocked is the status id that the server knows, or writes a 404.
func (s *Server) statusLocked(w http.ResponseWriter, id string) (mykit.Status, bool) {
	st, ok := s.statuses[id]
	if !ok {
		writeProblem(w, http.StatusNotFound, "about:blank", "No status "+id+".")
	}
	return st, ok
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

// writeProblem writes an RFC 9457 problem document, as the server does.
func writeProblem(w http.ResponseWriter, status int, typ, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": typ, "title": http.StatusText(status), "status": status, "detail": detail,
	})
}
