// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/app/uow"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
)

// VoiceRoomsContract verifies the room use cases of the voice module (ADR-0100 §2, §5, §6, §8,
// ADR-0101 §2, §6) on a database adapter: opening a room, joining, moving, replacing a session,
// leaving with the choice of the host, handing over, the expiry of the heartbeats and the grace
// of the host, voice.sweep (time_limit, disabled, and concurrent sweeps that apply once), the
// visibility and the order of the list, the reaction to suspensions and lost eligibility, the
// operations of the participants, the host, and the co-hosts with their permissions, the
// co-hosts, the invitations, and the bans and blocks (ADR-0100 §4, §5, §7, §11), and, on a
// media server, the reconciliation of voice.sweep and the events of voice.media_event
// (ADR-0102 §4).
func VoiceRoomsContract(t *testing.T, newHarness VoiceFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *roomsEnv)
		// transport is the method of the case, TransportP2P when empty.
		transport voice.Transport
	}{
		{"OpensARoom", roomsOpen, ""},
		{"JoinsMovesAndReplaces", roomsJoin, ""},
		{"LeavesWithTheHostsChoice", roomsLeave, ""},
		{"HandsOver", roomsHandOver, ""},
		{"ExpiresHeartbeatsAndTheGrace", roomsGrace, ""},
		{"SweepsOnce", roomsSweep, ""},
		{"ListsVisibleRooms", roomsList, ""},
		{"ReactsToSuspensions", roomsSuspension, ""},
		{"ReconcilesTheMediaPath", roomsMedia, voice.TransportLiveKit},
		{"AuthorizesConnections", roomsConnect, voice.TransportSora},
		{"SpeaksAndRaisesHands", roomsSpeaking, ""},
		{"MutesAndRemoves", roomsRemove, ""},
		{"AssignsCohosts", roomsCohosts, ""},
		{"Invites", roomsInvitations, ""},
		{"LimitsInvitations", roomsInvitationLimit, ""},
		{"ReactsToBlocks", roomsBlocks, ""},
		{"DecidesInTheTransaction", roomsRaces, ""},
		{"SerializesConcurrentCommands", roomsConcurrent, ""},
		{"FailsWithoutAGrant", roomsNoGrant, ""},
		{"StreamsTheRoom", roomsStream, ""},
		{"RelaysSignals", roomsSignals, ""},
		{"SendsMicReactionsAndGrants", roomsMicReactions, ""},
		{"NotifiesFollowersWhoSee", roomsNotifications, ""},
		{"WritesTheStartPost", roomsStartPost, ""},
		{"SummarizesForVisitors", roomsSummary, ""},
		{"ShowsTheProfileEntryAndHistory", roomsProfileAndHistory, ""},
		{"ReportsAndEndsAsAModerator", roomsModeration, ""},
		{"PurgesTheRecordsAfterTheRetention", roomsPurge, ""},
		{"ErasesTheRevisionsOfAnAccount", roomsErasure, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := tc.transport
			if tr == "" {
				tr = voice.TransportP2P
			}
			tc.run(t, newRoomsEnv(t, newHarness, appvoice.Config{Transport: tr}))
		})
	}
}

// roomsMembers are the accounts of the contract: every account is a local, active member
// unless changed; hosts lists who may start rooms.
type roomsMembers struct {
	mu       sync.Mutex
	inactive map[string]bool
	limited  map[string]bool
	starters map[string]bool
	follows  map[[2]string]bool
	blocks   map[[2]string]bool
	everyone bool
	// public is the instance setting voice_public_summary.
	public bool
	// calls counts the calls of Follows and Blocked by name; hooks run once, outside the lock,
	// when a call reaches their count, to commit a change between the reads of a command and
	// its transaction.
	calls map[string]int
	hooks map[string]roomsHook
}

type roomsHook struct {
	at int
	fn func()
}

// after runs fn once, when the call of name (follows, blocked) n calls from now starts.
func (m *roomsMembers) after(name string, n int, fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls == nil {
		m.calls, m.hooks = map[string]int{}, map[string]roomsHook{}
	}
	m.hooks[name] = roomsHook{at: m.calls[name] + n, fn: fn}
}

// fire counts a call of name and runs its hook when it is due.
func (m *roomsMembers) fire(name string) {
	m.mu.Lock()
	if m.calls == nil {
		m.mu.Unlock()
		return
	}
	m.calls[name]++
	h, ok := m.hooks[name]
	due := ok && m.calls[name] == h.at
	if due {
		delete(m.hooks, name)
	}
	m.mu.Unlock()
	if due {
		h.fn()
	}
}

func (m *roomsMembers) Member(_ context.Context, id string) (appvoice.Member, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return appvoice.Member{Local: true, Active: !m.inactive[id], Limited: m.limited[id], MayStart: m.starters[id]}, nil
}

func (m *roomsMembers) EveryoneStarts(context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.everyone, nil
}

func (m *roomsMembers) Follows(_ context.Context, a, b string) (bool, error) {
	m.fire("follows")
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.follows[[2]string{a, b}], nil
}

func (m *roomsMembers) Blocked(_ context.Context, a, b string) (bool, error) {
	m.fire("blocked")
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.blocks[[2]string{a, b}] || m.blocks[[2]string{b, a}], nil
}

func (m *roomsMembers) BlockedAmong(_ context.Context, a string, ids []string) (map[string]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]bool{}
	for _, id := range ids {
		if m.blocks[[2]string{a, id}] || m.blocks[[2]string{id, a}] {
			out[id] = true
		}
	}
	return out, nil
}

func (m *roomsMembers) Following(_ context.Context, a string, ids []string) (map[string]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = m.follows[[2]string{a, id}]
	}
	return out, nil
}

func (m *roomsMembers) PublicSummary(context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.public, nil
}

// LocalFollowers returns the followers of a in ID order, one to a batch, so that the fan-out
// walks the batches.
func (m *roomsMembers) LocalFollowers(_ context.Context, a, before string, _ int) ([]string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var ids []string
	for pair, ok := range m.follows {
		if ok && pair[1] == a && pair[0] > before {
			ids = append(ids, pair[0])
		}
	}
	slices.Sort(ids)
	if len(ids) == 0 {
		return nil, "", nil
	}
	return ids[:1], ids[0], nil
}

func (m *roomsMembers) set(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn()
}

// roomsTransport records the calls of JobApply; reconnect makes SetSpeaking ask the client to
// connect again.
type roomsTransport struct {
	mu        sync.Mutex
	calls     []string
	reconnect bool
	// enforced makes it a media server that holds the sessions of media, by room.
	enforced bool
	media    map[string][]string
	// conns is the connection of each session on the media path, by session ID.
	conns map[string]string
	// grantErr fails the grants when set, as a TURN source that cannot be reached does.
	grantErr error
}

func (r *roomsTransport) add(c string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, c)
}

func (r *roomsTransport) count(c string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, x := range r.calls {
		if x == c {
			n++
		}
	}
	return n
}

func (r *roomsTransport) Capabilities() appvoice.Capabilities {
	r.mu.Lock()
	defer r.mu.Unlock()
	return appvoice.Capabilities{ServerEnforced: r.enforced, Events: r.enforced}
}

// sfu makes the transport a media server that holds the sessions it was told of with connect
// and sends the events of deliver.
func (r *roomsTransport) sfu() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enforced, r.media, r.conns = true, map[string][]string{}, map[string]string{}
}

func (r *roomsTransport) connect(room string, sessions ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.media[room] = append(r.media[room], sessions...)
}

// connectAs puts a session on the media path of a room with the connection conn, replacing a
// connection of the same session as a full reconnect does.
func (r *roomsTransport) connectAs(room, session, conn string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !slices.Contains(r.media[room], session) {
		r.media[room] = append(r.media[room], session)
	}
	r.conns[session] = conn
}

// vanish drops the media room as a media server that closed it does.
func (r *roomsTransport) vanish(room string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.media, room)
}

// disconnect takes a session off the media path.
func (r *roomsTransport) disconnect(room, session string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.media[room] = slices.DeleteFunc(r.media[room], func(id string) bool { return id == session })
}

// AuthorizeConnect implements appvoice.ConnectAuthorizer: the body is the JSON of the attempt,
// the answer the JSON of the decision, and a header X-Bad refuses the request with 401.
func (*roomsTransport) AuthorizeConnect(_ context.Context, h http.Header, body []byte, decide func(appvoice.ConnectAttempt) appvoice.ConnectDecision) (int, []byte, error) {
	if h.Get("X-Bad") != "" {
		return http.StatusUnauthorized, nil, errors.New("bad credentials")
	}
	var a appvoice.ConnectAttempt
	if err := json.Unmarshal(body, &a); err != nil {
		return http.StatusOK, nil, nil
	}
	b, err := json.Marshal(decide(a))
	return http.StatusOK, b, err
}

// ParseEvents implements appvoice.EventSource: the body is the JSON of the events, and a
// header X-Bad refuses the delivery.
func (*roomsTransport) ParseEvents(_ context.Context, h http.Header, body []byte) ([]appvoice.Event, error) {
	if h.Get("X-Bad") != "" {
		return nil, errors.New("bad signature")
	}
	var evs []appvoice.Event
	return evs, json.Unmarshal(body, &evs)
}
func (r *roomsTransport) Open(_ context.Context, room appvoice.RoomRef) error {
	r.add("open:" + room.ID)
	return nil
}
func (r *roomsTransport) Grant(_ context.Context, _ appvoice.RoomRef, s appvoice.Session, _ bool) (appvoice.Grant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.grantErr != nil {
		return appvoice.Grant{}, r.grantErr
	}
	return appvoice.Grant{Kind: "test", Token: s.ID, ExpiresAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}, nil
}
func (r *roomsTransport) SetSpeaking(_ context.Context, _ appvoice.RoomRef, s appvoice.Session, canSpeak bool) (bool, error) {
	r.add("speak:" + s.ID + ":" + strconv.FormatBool(canSpeak))
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reconnect, nil
}
func (r *roomsTransport) Mute(_ context.Context, _ appvoice.RoomRef, s appvoice.Session) error {
	r.add("mute:" + s.ID)
	return nil
}
func (r *roomsTransport) Unmute(_ context.Context, _ appvoice.RoomRef, s appvoice.Session) error {
	r.add("unmute:" + s.ID)
	return nil
}
func (r *roomsTransport) Remove(_ context.Context, room appvoice.RoomRef, s appvoice.Session) error {
	r.add("remove:" + s.ID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.media != nil {
		r.media[room.ID] = slices.DeleteFunc(r.media[room.ID], func(id string) bool { return id == s.ID })
	}
	return nil
}
func (r *roomsTransport) Close(_ context.Context, room appvoice.RoomRef) error {
	r.add("close:" + room.ID)
	return nil
}
func (r *roomsTransport) Sessions(_ context.Context, room appvoice.RoomRef) ([]appvoice.SessionRef, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []appvoice.SessionRef
	for _, id := range r.media[room.ID] {
		out = append(out, appvoice.SessionRef{ID: id, Connection: r.conns[id]})
	}
	return out, nil
}

type roomsEnv struct {
	h         VoiceHarness
	clock     *Clock
	ids       *IDGenerator
	members   *roomsMembers
	transport *roomsTransport
	notices   *roomsNotifier
	posts     *roomsPosts
	jobs      *InlineJobs
	hub       *stream.Hub
	retention *roomsRetention
	audits    *roomsAudits
	svc       *appvoice.Service
	newSvc    func(cfg appvoice.Config) *appvoice.Service
}

func newRoomsEnv(t *testing.T, newHarness VoiceFactory, cfg appvoice.Config) *roomsEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 50}, Projectors: projectors})
	appvoice.RegisterProjectors(projectors, h.Voice)
	clock := NewClock(time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC))
	outbox, jobs := inlineOutbox(projectors)
	e := &roomsEnv{
		h: h, clock: clock, ids: NewIDGenerator(clock), transport: &roomsTransport{}, notices: &roomsNotifier{}, posts: &roomsPosts{},
		jobs: jobs, hub: stream.NewHub(stream.HubOptions{}), retention: newRoomsRetention(), audits: &roomsAudits{},
		members: &roomsMembers{
			inactive: map[string]bool{}, limited: map[string]bool{}, starters: map[string]bool{},
			follows: map[[2]string]bool{}, blocks: map[[2]string]bool{},
		},
	}
	kv := NewKeyValueStore(clock)
	e.newSvc = func(cfg appvoice.Config) *appvoice.Service {
		d := appvoice.Deps{
			Repository: h.Voice, UnitOfWork: h.UnitOfWork, Outbox: outbox, Clock: clock, IDs: e.ids,
			Heartbeats: appvoice.NewHeartbeats(kv, h.Voice), Members: e.members, Relations: e.members, Config: cfg,
			Streams: e.hub, Notifier: e.notices, Posts: e.posts, Host: contractHost, Retention: e.retention,
		}
		if cfg.Transport != "" {
			d.Transport = e.transport
		}
		return appvoice.NewService(d)
	}
	e.svc = e.newSvc(cfg)
	reg := jobs.Registry
	e.svc.RegisterJobs(reg, nil)
	e.svc.RegisterEligibilityProjectors(projectors)
	e.svc.RegisterCommitHooks(projectors)
	e.audits.register(projectors)
	t.Cleanup(e.hub.Close)
	return e
}

// account returns a new account ID that may start rooms when starter is true.
func (e *roomsEnv) account(starter bool) string {
	id := e.ids.NewID()
	e.members.set(func() { e.members.starters[id] = starter })
	return id
}

func (e *roomsEnv) open(t *testing.T, host string, in appvoice.SettingsInput) appvoice.Joined {
	t.Helper()
	if in.Name == "" {
		in.Name = "Room"
	}
	j, err := e.svc.Open(t.Context(), host, in)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return j
}

func (e *roomsEnv) join(t *testing.T, account, room string) appvoice.Joined {
	t.Helper()
	j, err := e.svc.Join(t.Context(), account, room)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	return j
}

func (e *roomsEnv) room(t *testing.T, id string) appvoice.RoomRecord {
	t.Helper()
	r, err := e.h.Voice.FindRoom(t.Context(), nil, id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// participants returns the account IDs of the participants of a room.
func (e *roomsEnv) participants(t *testing.T, room string) map[string]appvoice.Participant {
	t.Helper()
	ps, err := e.h.Voice.Participants(t.Context(), nil, room)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]appvoice.Participant{}
	for _, p := range ps {
		out[p.AccountID] = p
	}
	return out
}

func (e *roomsEnv) wantEnded(t *testing.T, room string, reason voice.EndReason) {
	t.Helper()
	r := e.room(t, room)
	if r.Head.State != voice.StateEnded || r.Ending == nil || r.Ending.Reason != reason {
		t.Fatalf("room %s = %+v ending %+v, want ended with %s", room, r.Head, r.Ending, reason)
	}
}

func (e *roomsEnv) cohost(t *testing.T, room, account string) {
	t.Helper()
	c := appvoice.CohostChange{ID: e.ids.NewID(), RoomID: room, AccountID: account, Change: voice.CohostAssigned, ChangedAt: e.clock.Now()}
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		if err := e.h.Voice.InsertCohostChange(t.Context(), tx, c); err != nil {
			return err
		}
		return tx.Emit(t.Context(), appvoice.CohostChanged{Change: c})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (e *roomsEnv) beat(t *testing.T, account, room, session string) {
	t.Helper()
	if err := e.svc.Heartbeat(t.Context(), account, room, session); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
}

func wantErr(t *testing.T, what string, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("%s: err = %v, want %v", what, err, want)
	}
}

func roomsOpen(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host := e.account(true)
	j := e.open(t, host, appvoice.SettingsInput{Name: "Talk", Description: "a\r\nb\rc", Hashtags: []string{"#Go", "voice"}})
	if j.Session == "" || j.Grant.Kind != "test" {
		t.Errorf("Joined = %+v, want a session and a grant", j)
	}
	r := j.Room
	if r.Head.HostAccountID != host || r.Head.State != voice.StateOpen || r.Revision.Settings.Description != "a\nb\nc" ||
		r.Revision.Settings.Visibility != voice.VisibilityLocal || r.Revision.Settings.Speakers != voice.SpeakersManual {
		t.Errorf("room = %+v, want the host, open, the defaults, and the description on LF", r)
	}
	if tags := r.Revision.Settings.Hashtags; len(tags) != 2 || tags[0].Name != "Go" || tags[0].NameNormalized != "go" || tags[1].Name != "voice" {
		t.Errorf("hashtags = %+v", tags)
	}
	if ps := e.participants(t, r.ID); len(ps) != 1 || ps[host].Role != voice.RoleSpeaker {
		t.Errorf("participants = %+v, want the host as a speaker", ps)
	}
	if e.transport.count("open:"+r.ID) != 1 {
		t.Errorf("transport calls = %v, want one open", e.transport.calls)
	}

	_, err := e.svc.Open(ctx, e.account(false), appvoice.SettingsInput{Name: "No"})
	wantErr(t, "an account without start_voice_rooms", err, appvoice.ErrNotEligible)
	e.members.set(func() { e.members.everyone = true })
	if _, err := e.svc.Open(ctx, e.account(false), appvoice.SettingsInput{Name: "Everyone"}); err != nil {
		t.Errorf("Open with voice_room_starters = everyone: %v", err)
	}
	limited := e.account(true)
	e.members.set(func() { e.members.limited[limited] = true })
	_, err = e.svc.Open(ctx, limited, appvoice.SettingsInput{Name: "Limited"})
	wantErr(t, "a limited account", err, appvoice.ErrNotEligible)

	var se *voice.SettingsError
	if _, err := e.svc.Open(ctx, e.account(true), appvoice.SettingsInput{Name: ""}); !errors.As(err, &se) || se.Field != voice.FieldName {
		t.Errorf("Open without a name: %v, want the name invalid", err)
	}
	if _, err := e.svc.Open(ctx, e.account(true), appvoice.SettingsInput{Name: "x", Hashtags: []string{"no space"}}); !errors.As(err, &se) || se.Field != voice.FieldHashtags {
		t.Errorf("Open with a bad hashtag: %v, want the hashtags invalid", err)
	}
	_, err = e.svc.Open(ctx, host, appvoice.SettingsInput{Name: "Second"})
	wantErr(t, "a host of an open room", err, appvoice.ErrHostingElsewhere)

	capped := e.newSvc(appvoice.Config{Transport: voice.TransportP2P, MaxOpenRooms: 2})
	_, err = capped.Open(ctx, e.account(true), appvoice.SettingsInput{Name: "Third"})
	wantErr(t, "a third room of two", err, appvoice.ErrTooManyRooms)

	off := e.newSvc(appvoice.Config{})
	_, err = off.Open(ctx, e.account(true), appvoice.SettingsInput{Name: "Off"})
	wantErr(t, "voice = off", err, appvoice.ErrDisabled)
}

func roomsJoin(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	a, b, c := e.account(true), e.account(true), e.account(false)
	r1 := e.open(t, a, appvoice.SettingsInput{}).Room.ID
	r2 := e.open(t, b, appvoice.SettingsInput{}).Room.ID

	j := e.join(t, c, r1)
	if p := e.participants(t, r1)[c]; p.Role != voice.RoleListener || j.Session == "" {
		t.Errorf("participant = %+v, want a listener with a session", p)
	}
	// Another device replaces the session.
	j2 := e.join(t, c, r1)
	if j2.Session == j.Session || len(e.participants(t, r1)) != 2 {
		t.Errorf("joined again = %+v, want a new session and one participation", j2)
	}
	if e.transport.count("remove:"+j.Session) != 1 {
		t.Errorf("transport calls = %v, want the replaced session removed", e.transport.calls)
	}
	wantErr(t, "a heartbeat of the replaced session", e.svc.Heartbeat(ctx, c, r1, j.Session), appvoice.ErrNotFound)
	e.beat(t, c, r1, j2.Session)

	// Joining another room ends the first participation (moved).
	e.join(t, c, r2)
	if _, ok := e.participants(t, r1)[c]; ok {
		t.Error("the participant is still in the first room")
	}
	if _, ok := e.participants(t, r2)[c]; !ok {
		t.Error("the participant is not in the second room")
	}
	if e.transport.count("remove:"+j2.Session) != 1 {
		t.Errorf("transport calls = %v, want the moved session removed", e.transport.calls)
	}
	_, err := e.svc.Join(ctx, a, r2)
	wantErr(t, "a host who joins another room", err, appvoice.ErrHostingElsewhere)

	full := e.newSvc(appvoice.Config{Transport: voice.TransportP2P, Capacity: 2})
	_, err = full.Join(ctx, e.account(false), r2)
	wantErr(t, "a full room", err, appvoice.ErrRoomFull)

	_, err = e.svc.Join(ctx, c, e.ids.NewID())
	wantErr(t, "an unknown room", err, appvoice.ErrNotFound)
}

func roomsLeave(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host, b, c := e.account(true), e.account(true), e.account(false)
	r := e.open(t, host, appvoice.SettingsInput{}).Room.ID
	e.join(t, b, r)
	e.join(t, c, r)

	wantErr(t, "the host leaving without a choice", e.svc.Leave(ctx, host, r, appvoice.LeaveInput{}), appvoice.ErrHostMustChoose)
	wantErr(t, "an ineligible next host", e.svc.Leave(ctx, host, r, appvoice.LeaveInput{NextHostID: c}), appvoice.ErrInvalidNextHost)
	if err := e.svc.Leave(ctx, host, r, appvoice.LeaveInput{NextHostID: b}); err != nil {
		t.Fatalf("Leave with the next host: %v", err)
	}
	if rec := e.room(t, r); rec.Head.HostAccountID != b {
		t.Errorf("host = %s, want %s", rec.Head.HostAccountID, b)
	}
	if err := e.svc.Leave(ctx, c, r, appvoice.LeaveInput{}); err != nil {
		t.Fatalf("Leave: %v", err)
	}
	wantErr(t, "leaving again", e.svc.Leave(ctx, c, r, appvoice.LeaveInput{}), appvoice.ErrNotFound)
	// The last one out ends the room.
	if err := e.svc.Leave(ctx, b, r, appvoice.LeaveInput{}); err != nil {
		t.Fatalf("Leave of the last one: %v", err)
	}
	e.wantEnded(t, r, voice.EndEmpty)
	if e.transport.count("close:"+r) != 1 {
		t.Errorf("transport calls = %v, want one close", e.transport.calls)
	}

	// The host ends a room with participants.
	r2 := e.open(t, host, appvoice.SettingsInput{}).Room.ID
	e.join(t, c, r2)
	if err := e.svc.Leave(ctx, host, r2, appvoice.LeaveInput{End: true}); err != nil {
		t.Fatalf("Leave ending the room: %v", err)
	}
	e.wantEnded(t, r2, voice.EndHost)
	if len(e.participants(t, r2)) != 0 {
		t.Error("an ended room keeps participants")
	}

	r3 := e.open(t, host, appvoice.SettingsInput{}).Room.ID
	e.join(t, c, r3)
	wantErr(t, "End by a participant", e.svc.End(ctx, c, r3), appvoice.ErrNotHost)
	if err := e.svc.End(ctx, host, r3); err != nil {
		t.Fatalf("End: %v", err)
	}
	e.wantEnded(t, r3, voice.EndHost)
}

func roomsHandOver(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host, b, c := e.account(true), e.account(true), e.account(false)
	r := e.open(t, host, appvoice.SettingsInput{}).Room.ID
	e.join(t, b, r)
	e.join(t, c, r)
	e.cohost(t, r, b)
	if !e.participants(t, r)[b].Cohost() {
		t.Fatal("b is not a co-host")
	}
	_, err := e.svc.HandOver(ctx, b, r, c)
	wantErr(t, "a hand over by a co-host", err, appvoice.ErrNotHost)
	_, err = e.svc.HandOver(ctx, host, r, c)
	wantErr(t, "a hand over to an ineligible participant", err, appvoice.ErrInvalidNextHost)
	v, err := e.svc.HandOver(ctx, host, r, b)
	if err != nil {
		t.Fatalf("HandOver: %v", err)
	}
	ps := e.participants(t, r)
	if v.Head.HostAccountID != b || ps[b].Cohost() {
		t.Errorf("host = %s, b co-host = %t, want b the host and no longer a co-host", v.Head.HostAccountID, ps[b].Cohost())
	}
	if _, ok := ps[host]; !ok {
		t.Error("the former host left the room")
	}
	// A settings change is the host's.
	name := "Renamed"
	_, err = e.svc.Update(ctx, host, r, appvoice.SettingsPatch{Name: &name})
	wantErr(t, "Update by the former host", err, appvoice.ErrNotHost)
	upd, err := e.svc.Update(ctx, b, r, appvoice.SettingsPatch{Name: &name})
	if err != nil || upd.Revision.Settings.Name != name {
		t.Fatalf("Update = %+v, %v", upd.Revision, err)
	}
	// A host who lost the eligibility hands the room over at its next operation.
	e.members.set(func() { e.members.starters[b] = false })
	_, err = e.svc.Update(ctx, b, r, appvoice.SettingsPatch{Name: &name})
	wantErr(t, "Update by an ineligible host", err, appvoice.ErrNotHost)
	e.wantEnded(t, r, voice.EndHostIneligible)
}

func roomsGrace(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host, b, c := e.account(true), e.account(true), e.account(false)
	jh := e.open(t, host, appvoice.SettingsInput{})
	r := jh.Room.ID
	jb := e.join(t, b, r)
	jc := e.join(t, c, r)
	e.cohost(t, r, b)

	// c stops sending heartbeats; the host and b go on.
	e.clock.Advance(30 * time.Second)
	e.beat(t, host, r, jh.Session)
	e.beat(t, b, r, jb.Session)
	e.clock.Advance(30 * time.Second)
	e.beat(t, b, r, jb.Session)
	if _, ok := e.participants(t, r)[c]; ok {
		t.Fatal("a participant whose heartbeat expired is still in the room")
	}
	wantErr(t, "a heartbeat after the expiry", e.svc.Heartbeat(ctx, c, r, jc.Session), appvoice.ErrNotFound)

	// The host's heartbeat expired: the grace starts, and a heartbeat in it ends it.
	e.clock.Advance(20 * time.Second)
	e.beat(t, b, r, jb.Session)
	if _, err := e.h.Voice.FindHostGrace(ctx, nil, r); err != nil {
		t.Fatalf("the grace of the host: %v", err)
	}
	for range 2 {
		e.clock.Advance(30 * time.Second)
		e.beat(t, b, r, jb.Session)
	}
	e.beat(t, host, r, jh.Session)
	if _, err := e.h.Voice.FindHostGrace(ctx, nil, r); !errors.Is(err, appvoice.ErrNotFound) {
		t.Fatalf("the grace after the host came back: %v", err)
	}

	// The host goes for good: past the grace, the co-host takes over.
	for range 5 {
		e.clock.Advance(40 * time.Second)
		e.beat(t, b, r, jb.Session)
	}
	rec := e.room(t, r)
	if rec.Head.HostAccountID != b || rec.Head.State != voice.StateOpen {
		t.Fatalf("room = %+v, want b the host", rec.Head)
	}
	if _, ok := e.participants(t, r)[host]; ok {
		t.Error("the disconnected host is still in the room")
	}

	// Without a co-host, the room ends past the grace.
	r2 := e.open(t, host, appvoice.SettingsInput{}).Room.ID
	jd := e.join(t, c, r2)
	for range 4 {
		e.clock.Advance(40 * time.Second)
		e.beat(t, c, r2, jd.Session)
	}
	// The heartbeat that finds the grace over ends the room before it renews the session.
	e.clock.Advance(40 * time.Second)
	wantErr(t, "a heartbeat of an ended room", e.svc.Heartbeat(ctx, c, r2, jd.Session), appvoice.ErrNotFound)
	e.wantEnded(t, r2, voice.EndHostDisconnected)
}

func roomsSweep(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	limited := e.newSvc(appvoice.Config{Transport: voice.TransportP2P, MaxRoomDuration: time.Hour})
	r1 := e.open(t, e.account(true), appvoice.SettingsInput{}).Room.ID
	e.clock.Advance(30 * time.Second)
	if err := limited.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if e.room(t, r1).Head.State != voice.StateOpen {
		t.Fatal("a sweep ended a live room")
	}
	e.clock.Advance(time.Hour)
	if err := limited.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	e.wantEnded(t, r1, voice.EndTimeLimit)

	r2 := e.open(t, e.account(true), appvoice.SettingsInput{}).Room.ID
	if err := e.newSvc(appvoice.Config{}).Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	e.wantEnded(t, r2, voice.EndDisabled)

	// Rooms whose participants are all gone end once, however many sweeps race.
	host := e.account(true)
	r3 := e.open(t, host, appvoice.SettingsInput{}).Room.ID
	e.join(t, e.account(false), r3)
	e.clock.Advance(time.Minute)
	if err := e.svc.Sweep(ctx); err != nil { // the listener leaves, the host's grace starts
		t.Fatal(err)
	}
	e.clock.Advance(3 * time.Minute)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			if err := e.svc.Sweep(ctx); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("Sweep: %v", err)
	}
	e.wantEnded(t, r3, voice.EndHostDisconnected)
	if n := e.transport.count("close:" + r3); n != 1 {
		t.Errorf("close calls = %d, want 1", n)
	}
	applied := 0
	for _, res := range e.jobs.Results() {
		if res.Job.Type == appvoice.JobApply && res.Job.IdempotencyKey == "voice.apply:"+r3+":close" && res.Result.IsDone() {
			applied++
		}
	}
	if applied != 1 {
		t.Errorf("close jobs = %d, want 1", applied)
	}
}

func roomsList(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	viewer, a, b, c := e.account(false), e.account(true), e.account(true), e.account(true)
	big := e.open(t, a, appvoice.SettingsInput{Name: "Big", Hashtags: []string{"go"}}).Room.ID
	e.join(t, e.account(false), big)
	e.join(t, e.account(false), big)
	followed := e.open(t, b, appvoice.SettingsInput{Name: "Followed"}).Room.ID
	hidden := e.open(t, c, appvoice.SettingsInput{Name: "Followers", Visibility: voice.VisibilityFollowers}).Room.ID
	e.members.set(func() { e.members.follows[[2]string{viewer, b}] = true })

	rooms, more, err := e.svc.List(ctx, viewer, appvoice.ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range rooms {
		ids = append(ids, r.ID)
	}
	if more || len(ids) != 2 || ids[0] != followed || ids[1] != big {
		t.Errorf("List = %v, want the followed room, then the bigger one, without the followers' room", ids)
	}
	page, more, err := e.svc.List(ctx, viewer, appvoice.ListQuery{Limit: 1})
	if err != nil || !more || len(page) != 1 {
		t.Fatalf("List page = %d rooms, more %t, %v", len(page), more, err)
	}
	next, _, err := e.svc.List(ctx, viewer, appvoice.ListQuery{Limit: 1, After: &page[0].Key})
	if err != nil || len(next) != 1 || next[0].ID != big {
		t.Fatalf("List next page = %+v, %v", next, err)
	}
	tagged, _, err := e.svc.List(ctx, viewer, appvoice.ListQuery{Tag: "#GO"})
	if err != nil || len(tagged) != 1 || tagged[0].ID != big {
		t.Fatalf("List by tag = %+v, %v", tagged, err)
	}
	_, err = e.svc.Get(ctx, viewer, hidden)
	wantErr(t, "Get of a followers' room", err, appvoice.ErrNotFound)
	_, err = e.svc.Join(ctx, viewer, hidden)
	wantErr(t, "Join of a followers' room", err, appvoice.ErrNotFound)
	e.members.set(func() { e.members.follows[[2]string{viewer, c}] = true })
	if _, err := e.svc.Get(ctx, viewer, hidden); err != nil {
		t.Errorf("Get by a follower: %v", err)
	}
	e.members.set(func() { e.members.blocks[[2]string{a, viewer}] = true })
	_, err = e.svc.Get(ctx, viewer, big)
	wantErr(t, "Get with a block", err, appvoice.ErrNotFound)
}

func roomsSuspension(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host, b, c := e.account(true), e.account(true), e.account(false)
	r := e.open(t, host, appvoice.SettingsInput{}).Room.ID
	e.join(t, b, r)
	e.join(t, c, r)
	e.cohost(t, r, b)

	e.members.set(func() { e.members.inactive[c] = true })
	if err := e.svc.CheckAccount(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.participants(t, r)[c]; ok {
		t.Error("a suspended participant is still in the room")
	}
	e.members.set(func() { e.members.inactive[host] = true })
	if err := e.svc.CheckAccount(ctx, host); err != nil {
		t.Fatal(err)
	}
	ps := e.participants(t, r)
	if rec := e.room(t, r); rec.Head.HostAccountID != b {
		t.Errorf("host = %s, want the co-host", rec.Head.HostAccountID)
	}
	if _, ok := ps[host]; ok {
		t.Error("the suspended host is still in the room")
	}
	// The co-host lost start_voice_rooms; no one else may host.
	e.members.set(func() { e.members.starters[b] = false })
	if err := e.svc.CheckHosts(ctx); err != nil {
		t.Fatal(err)
	}
	e.wantEnded(t, r, voice.EndHostIneligible)
}

// roomsMedia checks a room on a media server (ADR-0101 §2, ADR-0102 §4): voice.sweep removes
// the sessions on the media path that are not in the room and disconnects the participants the
// media path has not seen for longer than the life of a grant, applies the room's state to a
// session it sees for the first time, and opens a media room again that vanished;
// voice.media_event removes a session that joined without being in the room, applies the room's
// state to one that is, disconnects one that left unless it connected again, and opens a media
// room again that finished while the room is open; a session taken away cannot be revived by
// heartbeats; and a delivery of another method or that does not verify is refused.
func roomsMedia(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	e.transport.sfu()
	host, listener := e.account(true), e.account(false)
	hj := e.open(t, host, appvoice.SettingsInput{})
	r := hj.Room.ID
	lj := e.join(t, listener, r)
	e.transport.connect(r, hj.Session, "ghost")
	lst := e.openStream(t, listener, r, lj.Session)

	e.clock.Advance(30 * time.Second)
	e.beat(t, host, r, hj.Session)
	e.beat(t, listener, r, lj.Session)
	if err := e.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if e.transport.count("remove:ghost") != 1 {
		t.Errorf("transport calls = %v, want the session outside the room removed", e.transport.calls)
	}
	if ps := e.participants(t, r); len(ps) != 2 {
		t.Fatalf("participants = %v, want the listener kept while it may still connect", ps)
	}
	for range 4 {
		e.clock.Advance(30 * time.Second)
		e.beat(t, host, r, hj.Session)
		e.beat(t, listener, r, lj.Session)
	}
	if err := e.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if ps := e.participants(t, r); len(ps) != 1 || ps[host].AccountID != host {
		t.Fatalf("participants = %v, want the listener that never connected disconnected", ps)
	}
	if st := e.streamState(t, lst); st.End != appvoice.StreamRemoved || st.Reason != string(voice.Disconnected) {
		t.Errorf("the stream of the disconnected listener = %s %s, want removed (disconnected)", st.End, st.Reason)
	}
	if err := e.svc.Heartbeat(ctx, listener, r, lj.Session); !errors.Is(err, appvoice.ErrNotFound) {
		t.Errorf("heartbeat of the disconnected listener: %v, want ErrNotFound", err)
	}
	if e.transport.count("speak:"+hj.Session+":true") != 2 {
		t.Errorf("transport calls = %v, want the host's state applied on each sweep that sees it", e.transport.calls)
	}

	deliver := func(transport string, h http.Header, evs ...appvoice.Event) appvoice.MediaWebhookOutcome {
		t.Helper()
		body, err := json.Marshal(evs)
		if err != nil {
			t.Fatal(err)
		}
		res, err := e.svc.ReceiveMediaEvents(ctx, appvoice.MediaWebhook{Transport: transport, Header: h, Body: body, ClientIP: "192.0.2.1"})
		if err != nil {
			t.Fatal(err)
		}
		return res.Outcome
	}
	ref := appvoice.RoomRef{ID: r}
	joined := appvoice.Event{ID: "EV_1", Kind: appvoice.MediaSessionJoined, Room: ref, Session: "intruder"}
	for range 2 {
		if got := deliver("livekit", nil, joined); got != appvoice.MediaWebhookAccepted {
			t.Fatalf("delivery = %s, want accepted", got)
		}
	}
	if got := deliver("livekit", nil, appvoice.Event{ID: "EV_2", Kind: appvoice.MediaSessionJoined, Room: ref, Session: hj.Session}); got != appvoice.MediaWebhookAccepted {
		t.Fatalf("delivery = %s, want accepted", got)
	}
	if e.transport.count("remove:intruder") != 2 || e.transport.count("remove:"+hj.Session) != 0 {
		t.Errorf("transport calls = %v, want the intruder removed and the host kept", e.transport.calls)
	}
	keys := map[string]int{}
	for _, res := range e.jobs.Results() {
		if res.Job.Type == appvoice.JobMediaEvent {
			keys[res.Job.IdempotencyKey]++
		}
	}
	if keys["voice.media_event:livekit:EV_1"] != 2 || keys["voice.media_event:livekit:EV_2"] != 1 {
		t.Errorf("media event jobs = %v, want one key per event", keys)
	}

	if got := deliver("sora", nil, joined); got != appvoice.MediaWebhookNotFound {
		t.Errorf("delivery of another method = %s, want not_found", got)
	}
	if got := deliver("livekit", http.Header{"X-Bad": {"1"}}, joined); got != appvoice.MediaWebhookRejected {
		t.Errorf("delivery that does not verify = %s, want rejected", got)
	}
	p2p := e.newSvc(appvoice.Config{Transport: voice.TransportP2P})
	if res, err := p2p.ReceiveMediaEvents(ctx, appvoice.MediaWebhook{Transport: "livekit", Body: []byte("[]")}); err != nil || res.Outcome != appvoice.MediaWebhookNotFound {
		t.Errorf("delivery to voice = p2p = %+v, %v; want not_found", res, err)
	}

	deliver("livekit", nil, appvoice.Event{ID: "EV_3", Kind: appvoice.MediaRoomFinished, Room: ref})
	if e.transport.count("open:"+r) != 2 {
		t.Errorf("transport calls = %v, want the media room opened again", e.transport.calls)
	}
	noGrace := func(what string) {
		t.Helper()
		if _, err := e.h.Voice.FindHostGrace(ctx, nil, r); !errors.Is(err, appvoice.ErrNotFound) {
			t.Errorf("%s: host grace %v, want none", what, err)
		}
	}

	// A session that joins gets the room's state, whatever its grant said: a listener may not
	// speak.
	l2 := e.join(t, e.account(false), r)
	deliver("livekit", nil, appvoice.Event{ID: "EV_5", Kind: appvoice.MediaSessionJoined, Room: ref, Session: l2.Session, Connection: "PA_l2"})
	if e.transport.count("speak:"+l2.Session+":false") != 1 || e.transport.count("speak:"+hj.Session+":true") != 3 {
		t.Errorf("transport calls = %v, want the roles applied on join", e.transport.calls)
	}

	// A full reconnect: the new connection joins before the old one's left arrives.
	e.transport.connectAs(r, hj.Session, "PA_new")
	deliver("livekit", nil, appvoice.Event{ID: "EV_6", Kind: appvoice.MediaSessionLeft, Room: ref, Session: hj.Session, Connection: "PA_old"})
	noGrace("left of an old connection")
	e.beat(t, host, r, hj.Session)

	// A media room that vanished is opened again, and its sessions get the time to connect.
	e.transport.vanish(r)
	for range 2 {
		if err := e.svc.Sweep(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if e.transport.count("open:"+r) != 3 {
		t.Errorf("transport calls = %v, want the vanished media room opened again once", e.transport.calls)
	}
	noGrace("a vanished media room")
	e.transport.connectAs(r, hj.Session, "PA_3")
	if err := e.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if e.transport.count("speak:"+hj.Session+":true") != 4 {
		t.Errorf("transport calls = %v, want the host's state applied after it connected again", e.transport.calls)
	}

	// The host leaves the media path: the grace starts, and its session cannot be revived by
	// heartbeats; the client joins again.
	e.transport.disconnect(r, hj.Session)
	deliver("livekit", nil, appvoice.Event{ID: "EV_7", Kind: appvoice.MediaSessionLeft, Room: ref, Session: hj.Session, Connection: "PA_3"})
	if _, err := e.h.Voice.FindHostGrace(ctx, nil, r); err != nil {
		t.Errorf("host grace after the host left the media path: %v, want one", err)
	}
	if e.room(t, r).Head.State != voice.StateOpen {
		t.Error("the room ended when its host left the media path, want the grace")
	}
	if err := e.svc.Heartbeat(ctx, host, r, hj.Session); !errors.Is(err, appvoice.ErrNotFound) {
		t.Errorf("heartbeat of the session that left: %v, want ErrNotFound", err)
	}
	back := e.join(t, host, r)
	if back.Session == hj.Session || e.room(t, r).Head.HostAccountID != host {
		t.Errorf("rejoin of the host = %+v, want a new session of the host", back)
	}
}

// parallel runs fn n times at once and returns the errors.
func parallel(n int, fn func(i int) error) []error {
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { errs[i] = fn(i) })
	}
	wg.Wait()
	return errs
}

func roomsConcurrent(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	// The cap of the open rooms holds against concurrent opens.
	capped := e.newSvc(appvoice.Config{Transport: voice.TransportP2P, MaxOpenRooms: 1, Capacity: 3})
	hosts := make([]string, 6)
	for i := range hosts {
		hosts[i] = e.account(true)
	}
	opened := ""
	for i, err := range parallel(len(hosts), func(i int) error {
		_, err := capped.Open(ctx, hosts[i], appvoice.SettingsInput{Name: "Race"})
		return err
	}) {
		switch {
		case err == nil:
			if opened != "" {
				t.Fatalf("two rooms opened past a cap of one")
			}
			opened = hosts[i]
		case !errors.Is(err, appvoice.ErrTooManyRooms):
			t.Fatalf("Open: %v", err)
		}
	}
	p, err := e.h.Voice.FindParticipant(ctx, nil, opened)
	if err != nil {
		t.Fatal(err)
	}
	room := p.RoomID

	// The participant cap holds against concurrent joins.
	joiners := make([]string, 6)
	for i := range joiners {
		joiners[i] = e.account(true)
	}
	for _, err := range parallel(len(joiners), func(i int) error {
		_, err := capped.Join(ctx, joiners[i], room)
		return err
	}) {
		if err != nil && !errors.Is(err, appvoice.ErrRoomFull) {
			t.Fatalf("Join: %v", err)
		}
	}
	ps := e.participants(t, room)
	if len(ps) != 3 {
		t.Fatalf("participants = %d, want the cap of 3", len(ps))
	}

	// Concurrent host changes apply one at a time: each sees the host that the other set.
	var others []string
	for id := range ps {
		if id != opened {
			others = append(others, id)
		}
	}
	errs := parallel(2, func(i int) error {
		if i == 0 {
			_, err := capped.HandOver(ctx, opened, room, others[0])
			return err
		}
		return capped.Leave(ctx, opened, room, appvoice.LeaveInput{NextHostID: others[1]})
	})
	won := 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case errors.Is(err, appvoice.ErrNotHost), errors.Is(err, appvoice.ErrConflict), errors.Is(err, appvoice.ErrNotFound):
		default:
			t.Fatalf("host change: %v", err)
		}
	}
	rec := e.room(t, room)
	if won == 0 || rec.Head.State != voice.StateOpen || rec.Head.HostAccountID == opened {
		t.Fatalf("after the host changes: %d won, room %+v", won, rec.Head)
	}
	if _, ok := e.participants(t, room)[rec.Head.HostAccountID]; !ok {
		t.Errorf("the host %s is not in the room", rec.Head.HostAccountID)
	}
}

// roomsConnect checks the decisions of the connections that a media server asks about
// (ADR-0102 §4): the current, live session of a participant of an open room connects, a
// connection that publishes audio only as a speaker; a session that is not live (unknown or
// replaced), of a banned participant, past the capacity in the order of joining, or of a room
// that is not open is refused; a request that does not authenticate is rejected; and the
// endpoint of another method is not found.
func roomsConnect(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	e.transport.sfu()
	host, listener, banned := e.account(true), e.account(false), e.account(false)
	hj := e.open(t, host, appvoice.SettingsInput{})
	r := hj.Room.ID
	old := e.join(t, listener, r).Session
	bj := e.join(t, banned, r)
	// The listener joins again from another device: the first session is replaced.
	lj := e.join(t, listener, r)
	ban := appvoice.Ban{ID: e.ids.NewID(), RoomID: r, AccountID: banned, BannedByAccountID: host, BannedAt: e.clock.Now()}
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := e.h.Voice.LockRoom(ctx, tx, r); err != nil {
			return err
		}
		_, err := e.h.Voice.InsertBan(ctx, tx, ban)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	authorize := func(svc *appvoice.Service, transport string, h http.Header, a appvoice.ConnectAttempt) appvoice.MediaConnectResult {
		t.Helper()
		body, err := json.Marshal(a)
		if err != nil {
			t.Fatal(err)
		}
		res, err := svc.AuthorizeMediaConnect(ctx, appvoice.MediaWebhook{Transport: transport, Header: h, Body: body, ClientIP: "192.0.2.1"})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	decision := func(svc *appvoice.Service, a appvoice.ConnectAttempt) appvoice.ConnectDecision {
		t.Helper()
		res := authorize(svc, "sora", nil, a)
		if res.Outcome != appvoice.MediaWebhookAccepted || res.Status != http.StatusOK {
			t.Fatalf("AuthorizeMediaConnect(%+v) = %+v, want an answer", a, res)
		}
		var d appvoice.ConnectDecision
		if err := json.Unmarshal(res.Body, &d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	room := appvoice.RoomRef{ID: r}
	limited := e.newSvc(appvoice.Config{Transport: voice.TransportSora, Capacity: 1})
	for name, c := range map[string]struct {
		svc     *appvoice.Service
		attempt appvoice.ConnectAttempt
		reason  string
	}{
		"host speaks":             {attempt: appvoice.ConnectAttempt{Room: room, Session: hj.Session, Speaker: true}},
		"listener listens":        {attempt: appvoice.ConnectAttempt{Room: room, Session: lj.Session}},
		"listener speaks":         {attempt: appvoice.ConnectAttempt{Room: room, Session: lj.Session, Speaker: true}, reason: "not a speaker"},
		"replaced session":        {attempt: appvoice.ConnectAttempt{Room: room, Session: old}, reason: "session not live"},
		"unknown session":         {attempt: appvoice.ConnectAttempt{Room: room, Session: "unknown"}, reason: "session not live"},
		"session of another room": {attempt: appvoice.ConnectAttempt{Room: appvoice.RoomRef{ID: e.ids.NewID()}, Session: lj.Session}, reason: "room not open"},
		"banned":                  {attempt: appvoice.ConnectAttempt{Room: room, Session: bj.Session}, reason: "banned from the room"},
		"host within capacity":    {svc: limited, attempt: appvoice.ConnectAttempt{Room: room, Session: hj.Session, Speaker: true}},
		"listener past capacity":  {svc: limited, attempt: appvoice.ConnectAttempt{Room: room, Session: lj.Session}, reason: "room full"},
	} {
		svc := c.svc
		if svc == nil {
			svc = e.svc
		}
		d := decision(svc, c.attempt)
		if d.Allowed != (c.reason == "") || d.Reason != c.reason {
			t.Errorf("%s: decision = %+v, want reason %q", name, d, c.reason)
		}
	}

	if res := authorize(e.svc, "sora", http.Header{"X-Bad": {"1"}}, appvoice.ConnectAttempt{Room: room, Session: hj.Session}); res.Outcome != appvoice.MediaWebhookRejected || res.Status != http.StatusUnauthorized {
		t.Errorf("request that does not authenticate = %+v, want rejected with 401", res)
	}
	if res := authorize(e.svc, "livekit", nil, appvoice.ConnectAttempt{Room: room, Session: hj.Session}); res.Outcome != appvoice.MediaWebhookNotFound {
		t.Errorf("request of another method = %+v, want not_found", res)
	}

	// Only the requests that do not verify count against the limits, each endpoint its own:
	// a busy media server is never refused.
	ip := "192.0.2.9"
	attempt, err := json.Marshal(appvoice.ConnectAttempt{Room: room, Session: lj.Session})
	if err != nil {
		t.Fatal(err)
	}
	for i := range appvoice.MediaConnectIPLimit.Max + 1 {
		res, err := e.svc.AuthorizeMediaConnect(ctx, appvoice.MediaWebhook{Transport: "sora", Body: attempt, ClientIP: ip})
		if err != nil || res.Outcome != appvoice.MediaWebhookAccepted {
			t.Fatalf("authentic request %d = %+v, %v; want an answer", i, res, err)
		}
		ev, err := e.svc.ReceiveMediaEvents(ctx, appvoice.MediaWebhook{Transport: "sora", Body: []byte("[]"), ClientIP: ip})
		if err != nil || ev.Outcome != appvoice.MediaWebhookAccepted {
			t.Fatalf("authentic delivery %d = %+v, %v; want accepted", i, ev, err)
		}
	}
	bad := http.Header{"X-Bad": {"1"}}
	var last appvoice.MediaConnectResult
	for range appvoice.MediaConnectIPLimit.Max + 1 {
		if last, err = e.svc.AuthorizeMediaConnect(ctx, appvoice.MediaWebhook{Transport: "sora", Header: bad, Body: attempt, ClientIP: ip}); err != nil {
			t.Fatal(err)
		}
	}
	if last.Outcome != appvoice.MediaWebhookRateLimited || last.RetryAfter <= 0 {
		t.Errorf("request past the limit of the ones that do not verify = %+v, want rate_limited", last)
	}
	if ev, err := e.svc.ReceiveMediaEvents(ctx, appvoice.MediaWebhook{Transport: "sora", Header: bad, Body: []byte("[]"), ClientIP: ip}); err != nil || ev.Outcome != appvoice.MediaWebhookRejected {
		t.Errorf("delivery that does not verify, under its own limit = %+v, %v; want rejected", ev, err)
	}
	if res, err := e.svc.AuthorizeMediaConnect(ctx, appvoice.MediaWebhook{Transport: "sora", Body: attempt, ClientIP: ip}); err != nil || res.Outcome != appvoice.MediaWebhookAccepted {
		t.Errorf("authentic request past the limit = %+v, %v; want an answer", res, err)
	}
	if err := e.svc.End(ctx, host, r); err != nil {
		t.Fatal(err)
	}
	if d := decision(e.svc, appvoice.ConnectAttempt{Room: room, Session: hj.Session, Speaker: true}); d.Allowed || d.Reason != "room not open" {
		t.Errorf("connection to an ended room = %+v, want refused", d)
	}
}

// roomsNoGrant checks that a join whose transport gives no grant (a TURN source that cannot be
// reached, ADR-0101 §5) fails with ErrTransportUnavailable, which the API answers with 503.
func roomsNoGrant(t *testing.T, e *roomsEnv) {
	room := e.open(t, e.account(true), appvoice.SettingsInput{}).Room.ID
	e.transport.mu.Lock()
	e.transport.grantErr = errors.New("the TURN service is down")
	e.transport.mu.Unlock()
	_, err := e.svc.Join(t.Context(), e.account(false), room)
	wantErr(t, "Join without a grant", err, appvoice.ErrTransportUnavailable)
}
