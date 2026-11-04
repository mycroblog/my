// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"slices"
	"testing"
	"time"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// RelayHarness is one database under test for the relays: an empty database with the product
// schema, its UnitOfWork, and the relay repository on it.
type RelayHarness struct {
	UnitOfWork uow.UnitOfWork
	Relays     appfederation.RelayRepository
}

// RelayFactory returns a harness on a new, empty database whose UnitOfWork uses opts. The
// factory registers the cleanup of everything it creates with t.Cleanup.
type RelayFactory func(t *testing.T, opts uow.Options) RelayHarness

// RelayContract verifies the relay repository of a database adapter (ADR-0006, data-model
// §4.8): a registration once per URL, a relay read with its state, one event of each kind per
// Follow, the state replaced, the lookups by the current Follow and by the accepted actor, the
// inboxes of the accepted relays, the deletion with the events and state, and relay_states
// rebuilt from the newest event of each relay.
func RelayContract(t *testing.T, newHarness RelayFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, h RelayHarness)
	}{
		{"RegistersOncePerURL", relayRegistersOnce},
		{"RecordsEventsAndState", relayRecordsEvents},
		{"FindsRelaysByFollowAndActor", relayFindsRelays},
		{"DeletesRelays", relayDeletes},
		{"RebuildsStates", relayRebuildsStates},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newHarness(t, uow.Options{Projectors: &uow.Projectors{}})) })
	}
}

var relayEpoch = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

// relayIDs are UUIDv7 IDs in time order, as the server issues them.
var relayIDs = func() func(n int) string {
	g := NewIDGenerator(NewClock(relayEpoch.AddDate(1, 0, 0)))
	return func(n int) string { return g.NewIDAt(relayEpoch.Add(time.Duration(n) * time.Minute)) }
}()

func relayOf(id, url, inbox string, p federation.RelayProtocol, at time.Time) federation.Relay {
	return federation.Relay{
		ID: id, Protocol: p, URL: federation.IRI(url), InboxURL: federation.IRI(inbox), Receive: true, CreatedAt: at,
	}
}

func insertRelay(t *testing.T, h RelayHarness, r federation.Relay) {
	t.Helper()
	if err := h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return h.Relays.InsertRelay(t.Context(), tx, r) }); err != nil {
		t.Fatalf("InsertRelay(%s) = %v", r.URL, err)
	}
}

// relayEvent records e and the status it leads to, as the use cases do.
func relayEvent(t *testing.T, h RelayHarness, e federation.RelayEvent) bool {
	t.Helper()
	var inserted bool
	err := h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		rec, err := h.Relays.LockRelay(t.Context(), tx, e.RelayID)
		if err != nil {
			return err
		}
		next, err := rec.Status.Next(e)
		if err != nil {
			return err
		}
		if inserted, err = h.Relays.InsertRelayEvent(t.Context(), tx, e); err != nil || !inserted {
			return err
		}
		return h.Relays.UpsertRelayState(t.Context(), tx, next)
	})
	if err != nil {
		t.Fatalf("record %s of %s: %v", e.Kind, e.RelayID, err)
	}
	return inserted
}

func relayRegistersOnce(t *testing.T, h RelayHarness) {
	ctx := t.Context()
	r := relayOf(relayIDs(1), "https://relay.example/inbox", "https://relay.example/inbox", federation.RelayMastodon, relayEpoch)
	r.CreatedByUserID = relayIDs(0)
	insertRelay(t, h, r)
	dup := relayOf(relayIDs(2), "https://relay.example/inbox", "https://relay.example/inbox", federation.RelayLitePub, relayEpoch)
	err := h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return h.Relays.InsertRelay(ctx, tx, dup) })
	if !errors.Is(err, appfederation.ErrRelayExists) {
		t.Fatalf("InsertRelay of a registered URL = %v, want ErrRelayExists", err)
	}
	got, err := h.Relays.FindRelay(ctx, r.ID)
	if err != nil || got.Relay != r || got.Status != (federation.RelayStatus{}) {
		t.Errorf("FindRelay = %+v, %v; want %+v without a status", got, err, r)
	}
	if _, err := h.Relays.FindRelay(ctx, relayIDs(3)); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindRelay(unknown) = %v, want ErrNotFound", err)
	}
	lite := relayOf(relayIDs(4), "https://lite.example/relay", "https://lite.example/inbox", federation.RelayLitePub, relayEpoch.Add(time.Hour))
	lite.Receive = false
	insertRelay(t, h, lite)
	list, err := h.Relays.ListRelays(ctx)
	if err != nil || len(list) != 2 || list[0].Relay != r || list[1].Relay != lite {
		t.Errorf("ListRelays = %+v, %v; want both in registration order", list, err)
	}
}

func relayRecordsEvents(t *testing.T, h RelayHarness) {
	ctx := t.Context()
	r := relayOf(relayIDs(10), "https://relay.example/inbox", "https://relay.example/inbox", federation.RelayMastodon, relayEpoch)
	insertRelay(t, h, r)
	follow := federation.IRI("https://my.example/ap/activities/" + relayIDs(11))
	sent := federation.RelayEvent{ID: relayIDs(11), RelayID: r.ID, Kind: federation.RelayFollowSent, FollowActivityURI: follow, ActorUserID: relayIDs(9), OccurredAt: relayEpoch.Add(time.Minute)}
	relayEvent(t, h, sent)
	accepted := federation.RelayEvent{ID: relayIDs(12), RelayID: r.ID, Kind: federation.RelayAcceptReceived, FollowActivityURI: follow, RelayActorURI: "https://relay.example/actor", OccurredAt: relayEpoch.Add(2 * time.Minute)}
	relayEvent(t, h, accepted)
	// The same answer again is no new event.
	err := h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		again := accepted
		again.ID = relayIDs(13)
		ok, err := h.Relays.InsertRelayEvent(ctx, tx, again)
		if err == nil && ok {
			t.Errorf("InsertRelayEvent of a second accepted event of the Follow = true, want false")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.Relays.FindRelay(ctx, r.ID)
	want := federation.RelayStatus{RelayID: r.ID, State: federation.RelayAccepted, FollowActivityURI: follow, RelayActorURI: "https://relay.example/actor", Since: accepted.OccurredAt}
	if err != nil || got.Status != want {
		t.Errorf("status = %+v, %v; want %+v", got.Status, err, want)
	}
	err = h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		rec, err := h.Relays.LockRelay(ctx, tx, r.ID)
		if err == nil && rec.Status != want {
			t.Errorf("LockRelay status = %+v, want %+v", rec.Status, want)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	relayEvent(t, h, federation.RelayEvent{ID: relayIDs(14), RelayID: r.ID, Kind: federation.RelayLeftSent, FollowActivityURI: follow, OccurredAt: relayEpoch.Add(3 * time.Minute)})
	if got, err := h.Relays.FindRelay(ctx, r.ID); err != nil || got.Status.State != federation.RelayLeft || got.Status.RelayActorURI != "" {
		t.Errorf("status after leaving = %+v, %v; want left without the actor", got.Status, err)
	}
}

func relayFindsRelays(t *testing.T, h RelayHarness) {
	ctx := t.Context()
	a := relayOf(relayIDs(20), "https://a.example/inbox", "https://a.example/inbox", federation.RelayMastodon, relayEpoch)
	b := relayOf(relayIDs(21), "https://b.example/relay", "https://shared.example/inbox", federation.RelayLitePub, relayEpoch)
	c := relayOf(relayIDs(22), "https://c.example/relay", "https://shared.example/inbox", federation.RelayLitePub, relayEpoch)
	p := relayOf(relayIDs(23), "https://pending.example/inbox", "https://pending.example/inbox", federation.RelayMastodon, relayEpoch)
	for _, r := range []federation.Relay{a, b, c, p} {
		insertRelay(t, h, r)
	}
	follows := map[string]federation.IRI{}
	for i, r := range []federation.Relay{a, b, c, p} {
		follows[r.ID] = federation.IRI("https://my.example/ap/activities/f" + r.ID)
		relayEvent(t, h, federation.RelayEvent{ID: relayIDs(30 + i), RelayID: r.ID, Kind: federation.RelayFollowSent, FollowActivityURI: follows[r.ID], OccurredAt: relayEpoch})
	}
	for i, r := range []federation.Relay{a, b, c} {
		relayEvent(t, h, federation.RelayEvent{
			ID: relayIDs(40 + i), RelayID: r.ID, Kind: federation.RelayAcceptReceived, FollowActivityURI: follows[r.ID],
			RelayActorURI: federation.IRI(string(r.URL) + "#actor"), OccurredAt: relayEpoch.Add(time.Minute),
		})
	}
	if got, err := h.Relays.FindRelayByFollow(ctx, follows[p.ID]); err != nil || got.Relay.ID != p.ID || got.Status.State != federation.RelayPending {
		t.Errorf("FindRelayByFollow = %+v, %v; want the pending relay", got, err)
	}
	if _, err := h.Relays.FindRelayByFollow(ctx, "https://my.example/ap/activities/unknown"); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindRelayByFollow(unknown) = %v, want ErrNotFound", err)
	}
	if got, err := h.Relays.FindAcceptedRelayByActor(ctx, "https://b.example/relay#actor"); err != nil || got.Relay.ID != b.ID {
		t.Errorf("FindAcceptedRelayByActor = %+v, %v; want relay b", got, err)
	}
	if _, err := h.Relays.FindAcceptedRelayByActor(ctx, "https://pending.example/inbox#actor"); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindAcceptedRelayByActor of a pending relay = %v, want ErrNotFound", err)
	}
	inboxes, err := h.Relays.ListAcceptedRelayInboxes(ctx)
	if err != nil || !slices.Equal(inboxes, []federation.IRI{"https://a.example/inbox", "https://shared.example/inbox"}) {
		t.Errorf("ListAcceptedRelayInboxes = %v, %v; want the two inboxes of the accepted relays", inboxes, err)
	}
}

func relayDeletes(t *testing.T, h RelayHarness) {
	ctx := t.Context()
	r := relayOf(relayIDs(50), "https://relay.example/inbox", "https://relay.example/inbox", federation.RelayMastodon, relayEpoch)
	insertRelay(t, h, r)
	follow := federation.IRI("https://my.example/ap/activities/" + relayIDs(51))
	relayEvent(t, h, federation.RelayEvent{ID: relayIDs(51), RelayID: r.ID, Kind: federation.RelayFollowSent, FollowActivityURI: follow, OccurredAt: relayEpoch})
	relayEvent(t, h, federation.RelayEvent{ID: relayIDs(52), RelayID: r.ID, Kind: federation.RelayRejectReceived, FollowActivityURI: follow, OccurredAt: relayEpoch})
	if err := h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return h.Relays.DeleteRelay(ctx, tx, r.ID) }); err != nil {
		t.Fatalf("DeleteRelay = %v", err)
	}
	if _, err := h.Relays.FindRelay(ctx, r.ID); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindRelay after the deletion = %v, want ErrNotFound", err)
	}
	if _, err := h.Relays.FindRelayByFollow(ctx, follow); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindRelayByFollow after the deletion = %v, want ErrNotFound", err)
	}
	// The URL may be registered again, and its events start over.
	again := relayOf(relayIDs(53), "https://relay.example/inbox", "https://relay.example/inbox", federation.RelayMastodon, relayEpoch)
	insertRelay(t, h, again)
	if !relayEvent(t, h, federation.RelayEvent{ID: relayIDs(54), RelayID: again.ID, Kind: federation.RelayFollowSent, FollowActivityURI: follow, OccurredAt: relayEpoch}) {
		t.Errorf("the first event of the registered again relay was not recorded")
	}
}

func relayRebuildsStates(t *testing.T, h RelayHarness) {
	ctx := t.Context()
	a := relayOf(relayIDs(60), "https://a.example/inbox", "https://a.example/inbox", federation.RelayMastodon, relayEpoch)
	b := relayOf(relayIDs(61), "https://b.example/inbox", "https://b.example/inbox", federation.RelayMastodon, relayEpoch)
	insertRelay(t, h, a)
	insertRelay(t, h, b)
	fa := federation.IRI("https://my.example/ap/activities/" + relayIDs(62))
	relayEvent(t, h, federation.RelayEvent{ID: relayIDs(62), RelayID: a.ID, Kind: federation.RelayFollowSent, FollowActivityURI: fa, OccurredAt: relayEpoch.Add(time.Minute)})
	relayEvent(t, h, federation.RelayEvent{ID: relayIDs(63), RelayID: a.ID, Kind: federation.RelayAcceptReceived, FollowActivityURI: fa, RelayActorURI: "https://a.example/actor", OccurredAt: relayEpoch.Add(2 * time.Minute)})
	fb := federation.IRI("https://my.example/ap/activities/" + relayIDs(64))
	relayEvent(t, h, federation.RelayEvent{ID: relayIDs(64), RelayID: b.ID, Kind: federation.RelayFollowSent, FollowActivityURI: fb, OccurredAt: relayEpoch.Add(3 * time.Minute)})
	want := map[string]federation.RelayStatus{}
	for _, id := range []string{a.ID, b.ID} {
		got, err := h.Relays.FindRelay(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		want[id] = got.Status
	}
	err := h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		// A drifted state is derived again from the events.
		if err := h.Relays.UpsertRelayState(ctx, tx, federation.RelayStatus{RelayID: b.ID, State: federation.RelayLeft, FollowActivityURI: "x", Since: relayEpoch}); err != nil {
			return err
		}
		return h.Relays.RebuildRelayStates(ctx, tx)
	})
	if err != nil {
		t.Fatalf("RebuildRelayStates = %v", err)
	}
	for id, w := range want {
		if got, err := h.Relays.FindRelay(ctx, id); err != nil || got.Status != w {
			t.Errorf("status of %s after the rebuild = %+v, %v; want %+v", id, got.Status, err, w)
		}
	}
}
