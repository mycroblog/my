// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/voice/sfu"
	"github.com/mycroblog/my/internal/adapter/voice/sfu/node"
	"github.com/mycroblog/my/internal/adapter/voice/sfu/node/nodetest"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/pkg/testkit"
)

// voiceMember registers a user without the permission to open rooms on the App of e.
func voiceMember(e *accountsEnv, name string) testUser {
	e.t.Helper()
	r, err := e.app.Identity.RegisterLocalUser(e.t.Context(), identity.RegisterInput{Username: name, Password: "correct horse battery staple"})
	if err != nil {
		e.t.Fatal(err)
	}
	return testUser{id: r.AccountID, token: issueTokens(e.t, e.app, r.User.ID, "").AccessToken}
}

// drainJobs runs the jobs that the App of e queued, voice.apply among them.
func drainJobs(e *accountsEnv) {
	e.t.Helper()
	if _, err := e.app.Runner.Drain(e.t.Context()); err != nil {
		e.t.Fatalf("Drain: %v", err)
	}
}

// openTalk opens a room as host and joins it as guest, through e.
func openTalk(e *accountsEnv, host, guest testUser) (opened, joined clientapi.VoiceRoomJoin) {
	e.t.Helper()
	e.want(e.do(http.MethodPost, "/voice/rooms", host.token, map[string]any{"name": "Talk"}, &opened), http.StatusCreated, "open")
	e.want(e.do(http.MethodPost, "/voice/rooms/"+opened.Room.Id+"/participation", guest.token, nil, &joined), http.StatusOK, "join")
	return opened, joined
}

// hears fails the test unless listener hears more than n further packets of account.
func hears(t *testing.T, listener *nodetest.Client, account string, n int64, what string) {
	t.Helper()
	from := listener.HeardFrom(account)
	if !testkit.Eventually(t, 10*time.Second, func() bool { return listener.HeardFrom(account) > from+n }) {
		t.Fatalf("%s: heard %d packets of %s, want more than %d", what, listener.HeardFrom(account)-from, account, n)
	}
}

// stopsHearing fails the test unless the node of p drops the audio of account as not_allowed and
// listener hears none of it while it does: once the drops began, a window of 10 more drops
// reaches listener with no packet of account.
func stopsHearing(t *testing.T, p *sfuProcess, listener *nodetest.Client, account, what string) {
	t.Helper()
	dropsMore := func(n float64) {
		t.Helper()
		from := p.packets(node.DropNotAllowed)
		if !testkit.Eventually(t, 10*time.Second, func() bool { return p.packets(node.DropNotAllowed) >= from+n }) {
			t.Fatalf("%s: the node dropped %v packets as not_allowed, want %v", what, p.packets(node.DropNotAllowed)-from, n)
		}
	}
	dropsMore(10)
	heard := listener.HeardFrom(account)
	dropsMore(10)
	if got := listener.HeardFrom(account); got != heard {
		t.Errorf("%s: heard %d packets of %s while the node dropped them", what, got-heard, account)
	}
}

// TestVoiceSFURoomWorksInOneProcess runs a room of voice = sfu in one process (role = all,
// SQLite; ADR-0101 §8, §10): the grants of the opening and the join name the process's own node,
// two Pion clients connect through their tickets, the listener hears the host, and the host hears
// nothing of the listener.
func TestVoiceSFURoomWorksInOneProcess(t *testing.T) {
	t.Parallel()
	p := newSFUProcess(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin}, "all")
	if p.app.VoiceNode == nil {
		t.Fatal("role = all with voice = sfu runs no media node")
	}
	e := &accountsEnv{t: t, app: p.app, h: p.app.Handler(), spec: apitest.Load(t)}
	alice := e.user("alice")
	bob := voiceMember(e, "bob")
	opened, joined := openTalk(e, alice, bob)
	for _, g := range []clientapi.VoiceGrant{opened.Grant, joined.Grant} {
		if g.Url == nil || *g.Url != p.url {
			t.Fatalf("grant = %+v, want the URL of the process's node %s", g, p.url)
		}
	}
	a := connectSFU(t, opened.Grant, opened.Session)
	b := connectSFU(t, joined.Grant, joined.Session)
	nodetest.WaitConnected(t, a, b)
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	go a.Speak(ctx, 30)
	go b.Speak(ctx, 30)
	hears(t, b, alice.id, 10, "the listener")
	stopsHearing(t, p, a, bob.id, "the listener's audio")
	if n := a.HeardFrom(bob.id); n != 0 {
		t.Errorf("the host heard %d packets of the listener, want none", n)
	}
}

// TestVoiceSFURoomWorksAcrossWebAndNode runs a room of voice = sfu across a web process and a
// media node process (role = sfu) on PostgreSQL, sharing its KeyValueStore and stream = postgres
// (ADR-0101 §8, §13, §14): the web process runs no node, its grants name the node, two Pion
// clients connect through their tickets, the listener hears the host, and the node follows a
// change of the room made through web.
func TestVoiceSFURoomWorksAcrossWebAndNode(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		"MY_DATABASE": "postgres", "MY_DATABASE_URL": newPostgresDatabase(t), "MY_WEB_ORIGIN": webOrigin, "MY_STREAM": "postgres",
	}
	web := newSFUProcess(t, env, "web")
	if web.app.VoiceNode != nil {
		t.Fatal("role = web runs a media node")
	}
	sfuNode := newSFUProcess(t, env, "sfu")
	if sfuNode.app.VoiceNode == nil {
		t.Fatal("role = sfu runs no media node")
	}
	e := &accountsEnv{t: t, app: web.app, h: web.app.Handler(), spec: apitest.Load(t)}
	alice := e.user("alice")
	bob := voiceMember(e, "bob")
	opened, joined := openTalk(e, alice, bob)
	for _, g := range []clientapi.VoiceGrant{opened.Grant, joined.Grant} {
		if g.Url == nil || *g.Url != sfuNode.url {
			t.Fatalf("grant from web = %+v, want the URL of the node %s", g, sfuNode.url)
		}
	}
	a := connectSFU(t, opened.Grant, opened.Session)
	b := connectSFU(t, joined.Grant, joined.Session)
	nodetest.WaitConnected(t, a, b)
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	go a.Speak(ctx, 30)
	go b.Speak(ctx, 30)
	hears(t, b, alice.id, 10, "the listener on the node")
	stopsHearing(t, sfuNode, a, bob.id, "the listener's audio on the node")

	// bob becomes a speaker through web; the node reads the room again and forwards him.
	room := "/voice/rooms/" + opened.Room.Id
	e.want(e.do(http.MethodPost, room+"/participants/"+bob.id+"/speaking_permission", alice.token, nil, nil), http.StatusOK, "grant")
	e.want(e.do(http.MethodPost, room+"/speaker", bob.token, nil, nil), http.StatusOK, "speak")
	drainJobs(e)
	hears(t, a, bob.id, 10, "the host, once bob speaks through web")
}

// TestVoiceSFUEnforcesAtTheForwarder drives the speaking, the muting, and the removal of a
// participant through the client API and checks them at the forwarder of the node (ADR-0101
// §12, §13): a listener's RTP is dropped as not_allowed; a speaker is heard; a moderator's mute
// drops the speaker's RTP until the participant turns the microphone on again; a speaker made a
// listener again is no longer heard, and the host gets a new offer; a removed participant's
// connection closes with room_unavailable.
func TestVoiceSFUEnforcesAtTheForwarder(t *testing.T) {
	t.Parallel()
	p := newSFUProcess(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin}, "all")
	e := &accountsEnv{t: t, app: p.app, h: p.app.Handler(), spec: apitest.Load(t)}
	alice := e.user("alice")
	bob := voiceMember(e, "bob")
	opened, joined := openTalk(e, alice, bob)
	room := "/voice/rooms/" + opened.Room.Id
	a := connectSFU(t, opened.Grant, opened.Session)
	b := connectSFU(t, joined.Grant, joined.Session)
	nodetest.WaitConnected(t, a, b)
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	go a.Speak(ctx, 30)
	go b.Speak(ctx, 30)

	// A listener: dropped at the node.
	hears(t, b, alice.id, 10, "the listener")
	stopsHearing(t, p, a, bob.id, "a listener")
	if n := a.HeardFrom(bob.id); n != 0 {
		t.Fatalf("the host heard %d packets of the listener", n)
	}

	// The host lets bob speak; he becomes a speaker and is heard.
	e.want(e.do(http.MethodPost, room+"/participants/"+bob.id+"/speaking_permission", alice.token, nil, nil), http.StatusOK, "grant")
	e.want(e.do(http.MethodPost, room+"/speaker", bob.token, nil, nil), http.StatusOK, "speak")
	drainJobs(e)
	hears(t, a, bob.id, 10, "the host, once bob speaks")

	// The host mutes bob: his audio is dropped until he turns the microphone on again.
	e.want(e.do(http.MethodPost, room+"/participants/"+bob.id+"/mute", alice.token, nil, nil), http.StatusNoContent, "mute")
	drainJobs(e)
	stopsHearing(t, p, a, bob.id, "a muted speaker")
	e.want(e.do(http.MethodPost, room+"/mic", bob.token, map[string]any{"session": joined.Session, "on": true}, nil), http.StatusNoContent, "mic on")
	drainJobs(e)
	hears(t, a, bob.id, 10, "the host, once bob turns his microphone on")

	// The host makes bob a listener again: the host gets a new offer, and bob is no longer heard.
	offers := a.Offers()
	e.want(e.do(http.MethodDelete, room+"/participants/"+bob.id+"/speaking_permission", alice.token, nil, nil), http.StatusOK, "revoke")
	drainJobs(e)
	if !testkit.Eventually(t, 10*time.Second, func() bool { return a.Offers() > offers }) {
		t.Errorf("the host got no new offer once bob no longer speaks")
	}
	stopsHearing(t, p, a, bob.id, "a speaker made a listener")

	// The host removes bob: his connection closes with room_unavailable.
	e.want(e.do(http.MethodDelete, room+"/participants/"+bob.id, alice.token, nil, nil), http.StatusOK, "remove")
	drainJobs(e)
	m, ok := b.WaitError(10 * time.Second)
	if !ok {
		t.Fatal("the removed participant got no error")
	}
	if m.Code != node.CodeRoomUnavailable {
		t.Errorf("error code = %q, want %q", m.Code, node.CodeRoomUnavailable)
	}
}

// TestVoiceSFUDrainMovesTheRoom drains the media node of a room with another node running, on
// PostgreSQL across processes (ADR-0101 §14): the clients get migrate, their new grants from web
// name the other node, they connect there with the same sessions and the listener hears the host,
// the drain returns once they left, and the drained node counts its migrations.
func TestVoiceSFUDrainMovesTheRoom(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		"MY_DATABASE": "postgres", "MY_DATABASE_URL": newPostgresDatabase(t), "MY_WEB_ORIGIN": webOrigin, "MY_STREAM": "postgres",
	}
	web := newSFUProcess(t, env, "web")
	first := newSFUProcess(t, env, "sfu")
	e := &accountsEnv{t: t, app: web.app, h: web.app.Handler(), spec: apitest.Load(t)}
	alice := e.user("alice")
	bob := voiceMember(e, "bob")
	opened, joined := openTalk(e, alice, bob)
	if opened.Grant.Url == nil || *opened.Grant.Url != first.url {
		t.Fatalf("grant = %+v, want the first node %s", opened.Grant, first.url)
	}
	a := connectSFU(t, opened.Grant, opened.Session)
	b := connectSFU(t, joined.Grant, joined.Session)
	nodetest.WaitConnected(t, a, b)
	second := newSFUProcess(t, env, "sfu")

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	drained := make(chan struct{})
	go func() {
		first.app.VoiceNode.Drain(ctx)
		close(drained)
	}()
	for _, c := range []*nodetest.Client{a, b} {
		select {
		case <-c.Migrated():
		case <-ctx.Done():
			t.Fatal("a client of the draining node got no migrate")
		}
		c.Close()
	}
	ga := refreshGrant(e, alice.token, opened.Room.Id, opened.Session)
	gb := refreshGrant(e, bob.token, opened.Room.Id, joined.Session)
	for _, g := range []clientapi.VoiceGrant{ga, gb} {
		if g.Url == nil || *g.Url != second.url {
			t.Fatalf("grant after the migrate = %+v, want the second node %s", g, second.url)
		}
	}
	a2 := connectSFU(t, ga, opened.Session)
	b2 := connectSFU(t, gb, joined.Session)
	nodetest.WaitConnected(t, a2, b2)
	speaking, stop := context.WithCancel(t.Context())
	defer stop()
	go a2.Speak(speaking, 30)
	hears(t, b2, alice.id, 10, "the listener on the second node")
	select {
	case <-drained:
	case <-ctx.Done():
		t.Fatal("the drain did not return once its clients left")
	}
	if got := first.meter.Sum(node.Migrations.Name, nil); got != 2 {
		t.Errorf("migrations of the drained node = %v, want 2", got)
	}
}

// TestVoiceSFUMetrics records the metrics of the media node through the metrics port (ADR-0055,
// ADR-0101 §16) while a room runs: every instrument of the node is declared, none with a label
// of a room or a session; the packets carry only their outcome; the forwarded and dropped
// packets, the bytes, a renegotiation, an ICE restart, and a refused ticket are counted; and the
// gauges tell the rooms, the sessions, and the capacity.
func TestVoiceSFUMetrics(t *testing.T) {
	t.Parallel()
	p := newSFUProcess(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin}, "all")
	e := &accountsEnv{t: t, app: p.app, h: p.app.Handler(), spec: apitest.Load(t)}
	alice := e.user("alice")
	bob := voiceMember(e, "bob")
	opened, joined := openTalk(e, alice, bob)
	a := connectSFU(t, opened.Grant, opened.Session)
	b := connectSFU(t, joined.Grant, joined.Session)
	nodetest.WaitConnected(t, a, b)
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	go a.Speak(ctx, 30)
	go b.Speak(ctx, 30)
	hears(t, b, alice.id, 10, "the listener")
	if !testkit.Eventually(t, 10*time.Second, func() bool { return p.packets(node.DropNotAllowed) > 0 }) {
		t.Error("no packet counted as not_allowed")
	}

	// bob becomes a speaker: the host's connection is offered again.
	room := "/voice/rooms/" + opened.Room.Id
	e.want(e.do(http.MethodPost, room+"/participants/"+bob.id+"/speaking_permission", alice.token, nil, nil), http.StatusOK, "grant")
	e.want(e.do(http.MethodPost, room+"/speaker", bob.token, nil, nil), http.StatusOK, "speak")
	drainJobs(e)
	hears(t, a, bob.id, 5, "the host")
	if !testkit.Eventually(t, 10*time.Second, func() bool { return p.meter.Sum(node.Renegotiations.Name, nil) > 0 }) {
		t.Error("no renegotiation counted once the speakers changed")
	}

	// The host's client asks for an ICE restart: counted, and offered.
	offers := a.Offers()
	a.Send(node.ClientMessage{V: node.Version, Type: node.TypeRestart})
	if !testkit.Eventually(t, 10*time.Second, func() bool { return a.Offers() > offers }) {
		t.Error("no offer after a restart")
	}
	if got := p.meter.Sum(node.ICERestarts.Name, nil); got != 1 {
		t.Errorf("ICE restarts = %v, want 1", got)
	}

	// A ticket that does not exist is refused and counted as invalid.
	read, closeConn, err := nodetest.DialRaw(t.Context(), p.url, "unknown")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	_ = read()
	closeConn()
	if !testkit.Eventually(t, 5*time.Second, func() bool {
		return p.meter.Sum(node.TicketRefusals.Name, map[string]string{"my.voice.sfu.reason": node.RefusalInvalid}) == 1
	}) {
		t.Errorf("ticket refusals = %v, want 1 invalid", p.meter.Measurements(node.TicketRefusals.Name, nil))
	}

	if got := p.packets(node.OutcomeForwarded); got <= 0 {
		t.Errorf("forwarded packets = %v, want some", got)
	}
	if got := p.observed(node.Bytes.Name, nil); got <= 0 {
		t.Errorf("forwarded bytes = %v, want some", got)
	}
	outcomes := []string{node.OutcomeForwarded, node.DropSilent, node.DropNotAllowed, node.DropQueueFull}
	for _, m := range p.meter.Measurements(node.Packets.Name, nil) {
		if len(m.Labels) != 1 || !slices.Contains(outcomes, m.Labels["my.outcome"]) {
			t.Errorf("a packet measurement has the labels %v, want an outcome of %v only", m.Labels, outcomes)
		}
	}

	if err := p.meter.Collect(t.Context()); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got := p.observed(node.Rooms.Name, nil); got != 1 {
		t.Errorf("rooms = %v, want 1", got)
	}
	if got := p.observed(node.Sessions.Name, nil); got != 2 {
		t.Errorf("sessions = %v, want 2", got)
	}
	if got := p.observed(node.CapacityConnected.Name, nil); got != 2 {
		t.Errorf("connected = %v, want 2", got)
	}
	// The reservations reach the gauge when the node renews its lease (sfu.LeaseRenewal).
	if !testkit.Eventually(t, 2*sfu.LeaseRenewal, func() bool { return p.observed(node.CapacityReserved.Name, nil) == 10 }) {
		t.Errorf("reserved = %v, want the room's maximum, 10", p.observed(node.CapacityReserved.Name, nil))
	}

	for _, i := range []struct {
		name   string
		labels []string
	}{
		{node.Rooms.Name, nil}, {node.Sessions.Name, nil}, {node.CapacityReserved.Name, nil}, {node.CapacityConnected.Name, nil},
		{node.Packets.Name, []string{"my.outcome"}}, {node.Bytes.Name, nil}, {node.Renegotiations.Name, nil},
		{node.ICERestarts.Name, nil}, {node.Migrations.Name, nil}, {node.TicketRefusals.Name, []string{"my.voice.sfu.reason"}},
		{node.ClientRTT.Name, nil}, {node.ClientPacketLoss.Name, nil}, {node.OfferWait.Name, nil},
	} {
		got, ok := p.meter.Instrument(i.name)
		if !ok {
			t.Errorf("%s is not declared", i.name)
			continue
		}
		if !slices.Equal(got.LabelKeys, i.labels) {
			t.Errorf("%s has the label keys %v, want %v", i.name, got.LabelKeys, i.labels)
		}
		for _, k := range got.LabelKeys {
			if strings.Contains(k, "room") || strings.Contains(k, "session") {
				t.Errorf("%s has the label %s of a room or a session", i.name, k)
			}
		}
	}
	if err := p.meter.Err(); err != nil {
		t.Errorf("the meter saw a misuse: %v", err)
	}
}
