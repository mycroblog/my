// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/voice/sfu/node"
	"github.com/mycroblog/my/internal/adapter/voice/sfu/node/nodetest"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestVoiceSFUFullRoom fills a room of voice = sfu to its maximum through the client API
// (ADR-0100 §3, ADR-0101 §12): three speakers, one of them silent, and the rest listeners. Every
// participant hears every other speaker who makes sound, nobody hears the silent one, whose
// packets the node drops as silent, and one more join is a 409 voice-room-full.
func TestVoiceSFUFullRoom(t *testing.T) {
	t.Parallel()
	const maximum = 8
	p := newSFUProcess(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin}, "all")
	e := &accountsEnv{t: t, app: p.app, h: p.app.Handler(), spec: apitest.Load(t)}
	host := e.user("alice")
	var opened clientapi.VoiceRoomJoin
	e.want(e.do(http.MethodPost, "/voice/rooms", host.token, map[string]any{"name": "Full", "max_participants": maximum}, &opened), http.StatusCreated, "open")
	room := "/voice/rooms/" + opened.Room.Id
	users := []testUser{host}
	joins := []clientapi.VoiceRoomJoin{opened}
	for i := 1; i < maximum; i++ {
		u := e.member(fmt.Sprintf("member%d", i))
		var j clientapi.VoiceRoomJoin
		e.want(e.do(http.MethodPost, room+"/participation", u.token, nil, &j), http.StatusOK, "join of "+u.id)
		users, joins = append(users, u), append(joins, j)
	}
	if got := e.want(e.do(http.MethodPost, room+"/participation", e.member("late").token, nil, nil), http.StatusConflict, "join of one more"); got != clientapi.ProblemTypeVoiceRoomFull {
		t.Errorf("join of one more = %s, want %s", got, clientapi.ProblemTypeVoiceRoomFull)
	}

	// The host and two members speak; the second of them makes no sound.
	for _, u := range users[1:3] {
		e.want(e.do(http.MethodPost, room+"/participants/"+u.id+"/speaking_permission", host.token, nil, nil), http.StatusOK, "grant "+u.id)
		e.want(e.do(http.MethodPost, room+"/speaker", u.token, nil, nil), http.StatusOK, "speak "+u.id)
	}
	drainJobs(e)
	loud, silent := []testUser{users[0], users[1]}, users[2]
	clients := make([]*nodetest.Client, len(joins))
	for i, j := range joins {
		clients[i] = connectSFU(t, j.Grant, j.Session)
	}
	nodetest.WaitConnected(t, clients...)
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	go clients[0].Speak(ctx, 30)
	go clients[1].Speak(ctx, 30)
	go clients[2].Speak(ctx, 127)

	for i, c := range clients {
		for _, s := range loud {
			if s.id != users[i].id {
				hears(t, c, s.id, 10, fmt.Sprintf("participant %d of %s", i, s.id))
			}
		}
	}
	from := p.packets(node.DropSilent)
	if !testkit.Eventually(t, 10*time.Second, func() bool { return p.packets(node.DropSilent) >= from+10 }) {
		t.Fatalf("the node dropped %v packets as silent, want 10 more", p.packets(node.DropSilent)-from)
	}
	for i, c := range clients {
		if n := c.HeardFrom(silent.id); n != 0 {
			t.Errorf("participant %d heard %d packets of the silent speaker", i, n)
		}
	}
	if got := p.observed(node.Sessions.Name, nil); got != maximum {
		t.Errorf("sessions = %v, want %d", got, maximum)
	}
}

// TestVoiceSFUClientChangesNetworks keeps a listener hearing across the reconnections of ADR-0101
// §11, on the same session: an ICE restart that the client asks for, and a WebSocket that drops
// while the PeerConnection lives, which the client reattaches with the ticket of a new grant.
func TestVoiceSFUClientChangesNetworks(t *testing.T) {
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
	hears(t, b, alice.id, 10, "the listener")
	restart := func(what string) {
		t.Helper()
		offers := b.Offers()
		b.Send(node.ClientMessage{V: node.Version, Type: node.TypeRestart})
		if !testkit.Eventually(t, 10*time.Second, func() bool { return b.Offers() > offers }) {
			t.Fatalf("%s: no offer after a restart", what)
		}
	}
	sameSession := func(what string) {
		t.Helper()
		if got := p.observed(node.Sessions.Name, nil); got != 2 {
			t.Errorf("%s: sessions = %v, want the same 2", what, got)
		}
	}

	// The network changes: the client asks for an ICE restart.
	restart("the ICE restart")
	hears(t, b, alice.id, 10, "the listener after an ICE restart")
	sameSession("after an ICE restart")

	// The WebSocket drops; the media goes on, and a new grant's ticket reattaches the session.
	b.DropSignal()
	hears(t, b, alice.id, 10, "the listener while its WebSocket is dropped")
	g := refreshGrant(e, bob.token, opened.Room.Id, joined.Session)
	if g.Url == nil || g.Token == nil {
		t.Fatalf("grant = %+v, want a URL and a ticket", g)
	}
	if err := b.Reattach(*g.Url, *g.Token); err != nil {
		t.Fatal(err)
	}
	restart("the reattached WebSocket")
	hears(t, b, alice.id, 10, "the listener after it reattached")
	sameSession("after the reattach")
	if got := p.meter.Sum(node.ICERestarts.Name, nil); got != 2 {
		t.Errorf("ICE restarts = %v, want 2", got)
	}
}
