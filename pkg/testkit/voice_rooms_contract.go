// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

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
// co-hosts, the invitations, and the bans and blocks (ADR-0100 §4, §5, §7, §11).
func VoiceRoomsContract(t *testing.T, newHarness VoiceFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *roomsEnv)
	}{
		{"OpensARoom", roomsOpen},
		{"JoinsMovesAndReplaces", roomsJoin},
		{"LeavesWithTheHostsChoice", roomsLeave},
		{"HandsOver", roomsHandOver},
		{"ExpiresHeartbeatsAndTheGrace", roomsGrace},
		{"SweepsOnce", roomsSweep},
		{"ListsVisibleRooms", roomsList},
		{"ReactsToSuspensions", roomsSuspension},
		{"SpeaksAndRaisesHands", roomsSpeaking},
		{"MutesAndRemoves", roomsRemove},
		{"AssignsCohosts", roomsCohosts},
		{"Invites", roomsInvitations},
		{"LimitsInvitations", roomsInvitationLimit},
		{"ReactsToBlocks", roomsBlocks},
		{"DecidesInTheTransaction", roomsRaces},
		{"SerializesConcurrentCommands", roomsConcurrent},
		{"FailsWithoutAGrant", roomsNoGrant},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t, newRoomsEnv(t, newHarness, appvoice.Config{Transport: voice.TransportP2P}))
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

func (m *roomsMembers) Following(_ context.Context, a string, ids []string) (map[string]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = m.follows[[2]string{a, id}]
	}
	return out, nil
}

func (m *roomsMembers) set(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn()
}

// roomsTransport records the calls of JobApply.
type roomsTransport struct {
	mu    sync.Mutex
	calls []string
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

func (*roomsTransport) Capabilities() appvoice.Capabilities { return appvoice.Capabilities{} }
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
	return false, nil
}
func (r *roomsTransport) Mute(_ context.Context, _ appvoice.RoomRef, s appvoice.Session) error {
	r.add("mute:" + s.ID)
	return nil
}
func (*roomsTransport) Unmute(context.Context, appvoice.RoomRef, appvoice.Session) error { return nil }
func (r *roomsTransport) Remove(_ context.Context, _ appvoice.RoomRef, s appvoice.Session) error {
	r.add("remove:" + s.ID)
	return nil
}
func (r *roomsTransport) Close(_ context.Context, room appvoice.RoomRef) error {
	r.add("close:" + room.ID)
	return nil
}
func (*roomsTransport) Sessions(context.Context, appvoice.RoomRef) ([]appvoice.SessionRef, error) {
	return nil, nil
}

type roomsEnv struct {
	h         VoiceHarness
	clock     *Clock
	ids       *IDGenerator
	members   *roomsMembers
	transport *roomsTransport
	jobs      *InlineJobs
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
		h: h, clock: clock, ids: NewIDGenerator(clock), transport: &roomsTransport{}, jobs: jobs,
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
