// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/pkg/testkit"
)

// FederationRepositoryContract verifies the federation repository of a database adapter by its
// methods alone, the port contract of ADR-0153 §1: an outbound activity is found by its ID and
// listed with the others about its object, oldest first; its first document stays; an inbox is
// added to it once, and its deliveries are found and paged in ID order; the attempts of a
// delivery are listed oldest first; the health of a server is replaced, read in a transaction,
// and listed and counted by state, and a server without one is available; the availability
// events of a server are paged newest first; a received activity is answered once; the closure's
// Deletes are recorded once per account, and their fan-outs and deliveries counted by state; and
// the Moves still being sent are counted. The outbound use cases on top of it are
// FederationFlows, which internal/flowtest/federation runs once on SQLite.
func FederationRepositoryContract(t *testing.T, newHarness FederationFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *fedRepo)
	}{
		{"FindsAndListsOutboundActivities", fedRepoOutboundActivities},
		{"InsertsManyOutboundActivities", fedRepoManyOutboundActivities},
		{"KeepsTheFirstDocument", fedRepoDocuments},
		{"FindsTheContextOfADelivery", fedRepoDeliveryContext},
		{"AddsAnInboxOnceAndPagesDeliveries", fedRepoDeliveries},
		{"ListsTheAttemptsOfADelivery", fedRepoAttempts},
		{"ReplacesReadsAndListsTheHealthOfServers", fedRepoHealth},
		{"PagesAvailabilityEventsNewestFirst", fedRepoAvailabilityEvents},
		{"AnswersAReceivedActivityOnce", fedRepoAnswers},
		{"CountsTheClosureDeletes", fedRepoClosure},
		{"CountsPendingMoves", fedRepoPendingMoves},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{})
			clock := testkit.NewClock(time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC))
			tc.run(t, &fedRepo{h: h, clock: clock, ids: testkit.NewIDGenerator(clock)})
		})
	}
}

type fedRepo struct {
	h     FederationHarness
	clock *testkit.Clock
	ids   *testkit.IDGenerator
}

func (r *fedRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

// activity records an outbound activity of typ about the object of kind and id, created now, and
// advances the clock a second.
func (r *fedRepo) activity(t *testing.T, typ federation.ActivityType, kind federation.OutboundObjectKind, objectID string) federation.OutboundActivity {
	t.Helper()
	id := r.ids.NewID()
	a := federation.OutboundActivity{
		ID: id, ActivityURI: federation.IRI("https://" + testkit.ContractHost + "/ap/activities/" + id), Type: typ,
		ActorAccountID: testkit.NamedID("local-actor"), ObjectKind: kind, ObjectID: objectID, CreatedAt: r.clock.Now(),
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Federation.InsertOutboundActivity(ctx, tx, a) })
	r.clock.Advance(time.Second)
	return a
}

// delivery adds the inbox of host to the activity and returns it, or fails when it was added.
func (r *fedRepo) delivery(t *testing.T, activityID, host string) federation.Delivery {
	t.Helper()
	d := federation.Delivery{
		ID: r.ids.NewID(), OutboundActivityID: activityID, InboxURL: federation.IRI("https://" + host + "/inbox"),
		Domain: host, CreatedAt: r.clock.Now(),
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		ok, err := r.h.Federation.InsertDelivery(ctx, tx, d)
		if err == nil && !ok {
			t.Errorf("InsertDelivery(%s) = false, want true", d.InboxURL)
		}
		return err
	})
	return d
}

// attempt records an attempt of the delivery with outcome and advances the clock a second.
func (r *fedRepo) attempt(t *testing.T, deliveryID string, outcome federation.DeliveryOutcome) federation.DeliveryAttempt {
	t.Helper()
	a := federation.DeliveryAttempt{ID: r.ids.NewID(), DeliveryID: deliveryID, Outcome: outcome, AttemptedAt: r.clock.Now()}
	switch outcome {
	case federation.DeliverySucceeded:
		a.Scheme, a.HTTPStatus = "rfc9421", 202
	case federation.DeliveryRetryableFailure:
		a.Error = "connection refused"
	case federation.DeliveryPermanentFailure:
		a.Scheme, a.HTTPStatus = "cavage", 410
	case federation.DeliverySkipped:
		a.Error = "unavailable"
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Federation.InsertDeliveryAttempt(ctx, tx, a) })
	r.clock.Advance(time.Second)
	return a
}

func (r *fedRepo) fanOutCompleted(t *testing.T, activityID string) {
	t.Helper()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Federation.InsertFanOutCompletion(ctx, tx, activityID, r.clock.Now())
	})
}

func fedRepoOutboundActivities(t *testing.T, r *fedRepo) {
	status := testkit.NamedID("status")
	create := r.activity(t, federation.ActivityCreate, federation.OutboundStatus, status)
	r.activity(t, federation.ActivityCreate, federation.OutboundStatus, testkit.NamedID("another status"))
	r.activity(t, federation.ActivityAnnounce, federation.OutboundBoost, status)
	update := r.activity(t, federation.ActivityUpdate, federation.OutboundStatus, status)
	del := r.activity(t, federation.ActivityDelete, federation.OutboundStatus, status)

	if got, err := r.h.Federation.FindOutboundActivity(t.Context(), create.ID); err != nil || got != create {
		t.Errorf("FindOutboundActivity = %+v, %v; want %+v", got, err, create)
	}
	if _, err := r.h.Federation.FindOutboundActivity(t.Context(), testkit.NamedID("unknown")); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindOutboundActivity(unknown) = %v, want ErrNotFound", err)
	}
	got, err := r.h.Federation.ListOutboundActivitiesOf(t.Context(), federation.OutboundStatus, status)
	if err != nil || !slices.Equal(got, []federation.OutboundActivity{create, update, del}) {
		t.Errorf("ListOutboundActivitiesOf(status) = %+v, %v; want its Create, Update, and Delete", got, err)
	}
	if got, err := r.h.Federation.ListOutboundActivitiesOf(t.Context(), federation.OutboundFollow, status); err != nil || len(got) != 0 {
		t.Errorf("ListOutboundActivitiesOf(another kind) = %+v, %v; want none", got, err)
	}
}

// fedRepoManyOutboundActivities inserts many activities with one statement, each found as
// InsertOutboundActivity records one.
func fedRepoManyOutboundActivities(t *testing.T, r *fedRepo) {
	var as []federation.OutboundActivity
	for i, typ := range []federation.ActivityType{federation.ActivityUndo, federation.ActivityReject} {
		id := r.ids.NewID()
		as = append(as, federation.OutboundActivity{
			ID: id, ActivityURI: federation.IRI("https://" + testkit.ContractHost + "/ap/activities/" + id), Type: typ,
			ActorAccountID: testkit.NamedID(fmt.Sprintf("actor %d", i)), ObjectKind: federation.OutboundFollow, ObjectID: testkit.NamedID(fmt.Sprintf("follow %d", i)),
			CreatedAt: r.clock.Now(),
		})
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.h.Federation.InsertOutboundActivities(ctx, tx, as), r.h.Federation.InsertOutboundActivities(ctx, tx, nil))
	})
	for _, a := range as {
		if got, err := r.h.Federation.FindOutboundActivity(t.Context(), a.ID); err != nil || got != a {
			t.Errorf("FindOutboundActivity(%s) = %+v, %v; want %+v", a.ID, got, err, a)
		}
	}
}

func fedRepoDocuments(t *testing.T, r *fedRepo) {
	a := r.activity(t, federation.ActivityCreate, federation.OutboundStatus, testkit.NamedID("status"))
	if _, err := r.h.Federation.FindOutboundDocument(t.Context(), a.ID); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindOutboundDocument before the document = %v, want ErrNotFound", err)
	}
	first, second := []byte(`{"id":"first"}`), []byte(`{"id":"second"}`)
	for _, c := range []struct {
		doc  []byte
		want bool
	}{{first, true}, {second, false}} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ok, err := r.h.Federation.InsertOutboundDocument(ctx, tx, a.ID, c.doc, r.clock.Now())
			if err == nil && ok != c.want {
				t.Errorf("InsertOutboundDocument(%s) = %v, want %v", c.doc, ok, c.want)
			}
			return err
		})
	}
	if got, err := r.h.Federation.FindOutboundDocument(t.Context(), a.ID); err != nil || string(got) != string(first) {
		t.Errorf("FindOutboundDocument = %s, %v; want the first document", got, err)
	}
	// The documents of many activities together, the first of each kept, bytes as they are.
	b, c := r.activity(t, federation.ActivityDelete, federation.OutboundStatus, testkit.NamedID("b")), r.activity(t, federation.ActivityDelete, federation.OutboundStatus, testkit.NamedID("c"))
	docs := map[string][]byte{a.ID: []byte(`{"id":"not kept"}`), b.ID: []byte("{\"id\":\"b\",\"x\":\"\u00e9\"}"), c.ID: {0x7b, 0x7d}}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.h.Federation.InsertOutboundDocuments(ctx, tx, docs, r.clock.Now()), r.h.Federation.InsertOutboundDocuments(ctx, tx, nil, r.clock.Now()))
	})
	for id, want := range map[string][]byte{a.ID: first, b.ID: docs[b.ID], c.ID: docs[c.ID]} {
		if got, err := r.h.Federation.FindOutboundDocument(t.Context(), id); err != nil || string(got) != string(want) {
			t.Errorf("FindOutboundDocument(%s) after InsertOutboundDocuments = %q, %v; want %q", id, got, err, want)
		}
	}
}

// fedRepoDeliveryContext reads a delivery with its activity, document, and server's health, as
// the reads of each find them, without a document or a health row as with them.
func fedRepoDeliveryContext(t *testing.T, r *fedRepo) {
	ctx := t.Context()
	a := r.activity(t, federation.ActivityCreate, federation.OutboundStatus, testkit.NamedID("status"))
	d := r.delivery(t, a.ID, "a.example")
	want := func(name string, doc []byte, health federation.InstanceState, row bool) {
		t.Helper()
		got, err := r.h.Federation.FindDeliveryContext(ctx, d.ID)
		del, derr := r.h.Federation.FindDelivery(ctx, d.ID)
		if err != nil || derr != nil || got.Delivery != del || got.Activity != a || string(got.Document) != string(doc) ||
			(doc == nil) != (got.Document == nil) || got.Health != health || got.HealthRow != row {
			t.Errorf("FindDeliveryContext %s = %+v, %v; want %+v of %+v, document %s, health %q (row %v)", name, got, errors.Join(err, derr), del, a, doc, health, row)
		}
	}
	want("without a document or health", nil, federation.InstanceAvailable, false)
	doc := []byte(`{"id":"doc"}`)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		_, err := r.h.Federation.InsertOutboundDocument(ctx, tx, a.ID, doc, r.clock.Now())
		return err
	})
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Federation.UpsertInstanceHealth(ctx, tx, federation.InstanceHealth{Domain: "a.example", State: federation.InstanceUnavailable, Since: r.clock.Now(), FailureDays: 7})
	})
	want("with both", doc, federation.InstanceUnavailable, true)
	if _, err := r.h.Federation.FindDeliveryContext(ctx, testkit.NamedID("unknown")); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindDeliveryContext(unknown) = %v, want ErrNotFound", err)
	}
}

func fedRepoDeliveries(t *testing.T, r *fedRepo) {
	a := r.activity(t, federation.ActivityCreate, federation.OutboundStatus, testkit.NamedID("status"))
	other := r.activity(t, federation.ActivityCreate, federation.OutboundStatus, testkit.NamedID("another status"))
	var want []federation.Delivery
	for _, host := range []string{"a.example", "b.example", "c.example"} {
		want = append(want, r.delivery(t, a.ID, host))
	}
	// The same inbox of another activity is a delivery of its own.
	r.delivery(t, other.ID, "a.example")
	again := want[0]
	again.ID = r.ids.NewID()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		ok, err := r.h.Federation.InsertDelivery(ctx, tx, again)
		if err == nil && ok {
			t.Error("InsertDelivery of an inbox the activity has = true, want false")
		}
		return err
	})

	// Many deliveries at once: those of the inboxes the activity has, a second time or twice in
	// the batch, are not inserted.
	batch := r.activity(t, federation.ActivityCreate, federation.OutboundStatus, testkit.NamedID("a third status"))
	had := r.delivery(t, batch.ID, "a.example")
	newOf := func(host string) federation.Delivery {
		return federation.Delivery{ID: r.ids.NewID(), OutboundActivityID: batch.ID, InboxURL: federation.IRI("https://" + host + "/inbox"), Domain: host, CreatedAt: r.clock.Now()}
	}
	d, e, hadAgain, dAgain := newOf("d.example"), newOf("e.example"), newOf("a.example"), newOf("d.example")
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := r.h.Federation.InsertDeliveries(ctx, tx, []federation.Delivery{d, hadAgain, e, dAgain})
		if err == nil && !maps.Equal(got, map[string]bool{d.ID: true, e.ID: true}) {
			t.Errorf("InsertDeliveries = %v; want the new inboxes d and e only", got)
		}
		return err
	})
	if got, err := r.h.Federation.ListDeliveries(t.Context(), batch.ID, "", 10); err != nil || len(got) != 3 ||
		!slices.Contains(got, had) || !slices.Contains(got, d) || !slices.Contains(got, e) {
		t.Errorf("ListDeliveries of the batch = %+v, %v; want the first and the two new ones", got, err)
	}

	first, err := r.h.Federation.ListDeliveries(t.Context(), a.ID, "", 2)
	if err != nil || !slices.Equal(first, want[:2]) {
		t.Errorf("ListDeliveries(first page) = %+v, %v; want %+v", first, err, want[:2])
	}
	if len(first) == 2 {
		rest, err := r.h.Federation.ListDeliveries(t.Context(), a.ID, first[1].ID, 2)
		if err != nil || !slices.Equal(rest, want[2:]) {
			t.Errorf("ListDeliveries(after %s) = %+v, %v; want %+v", first[1].ID, rest, err, want[2:])
		}
	}
	if got, err := r.h.Federation.FindDelivery(t.Context(), want[1].ID); err != nil || got != want[1] {
		t.Errorf("FindDelivery = %+v, %v; want %+v", got, err, want[1])
	}
	if _, err := r.h.Federation.FindDelivery(t.Context(), again.ID); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindDelivery of the delivery not added = %v, want ErrNotFound", err)
	}
}

func fedRepoAttempts(t *testing.T, r *fedRepo) {
	a := r.activity(t, federation.ActivityCreate, federation.OutboundStatus, testkit.NamedID("status"))
	d := r.delivery(t, a.ID, "remote.example")
	other := r.delivery(t, a.ID, "other.example")
	want := []federation.DeliveryAttempt{
		r.attempt(t, d.ID, federation.DeliveryRetryableFailure),
		r.attempt(t, d.ID, federation.DeliverySkipped),
		r.attempt(t, d.ID, federation.DeliverySucceeded),
	}
	r.attempt(t, other.ID, federation.DeliveryPermanentFailure)
	if got, err := r.h.Federation.ListDeliveryAttempts(t.Context(), d.ID); err != nil || !slices.Equal(got, want) {
		t.Errorf("ListDeliveryAttempts = %+v, %v; want %+v", got, err, want)
	}
	if got, err := r.h.Federation.ListDeliveryAttempts(t.Context(), testkit.NamedID("unknown")); err != nil || len(got) != 0 {
		t.Errorf("ListDeliveryAttempts(unknown) = %+v, %v; want none", got, err)
	}
}

func fedRepoHealth(t *testing.T, r *fedRepo) {
	ctx := t.Context()
	upsert := func(h federation.InstanceHealth) {
		t.Helper()
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Federation.UpsertInstanceHealth(ctx, tx, h) })
	}
	if got, err := r.h.Federation.InstanceState(ctx, "unknown.example"); err != nil || got != federation.InstanceAvailable {
		t.Errorf("InstanceState without a row = %q, %v; want available", got, err)
	}
	if _, err := r.h.Federation.FindInstanceHealth(ctx, "unknown.example"); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindInstanceHealth without a row = %v, want ErrNotFound", err)
	}
	now := r.clock.Now()
	day := federation.FailureDay(now)
	healthy := federation.NewInstanceHealth("a.example", now)
	failing := federation.InstanceHealth{Domain: "b.example", State: federation.InstanceAvailable, Since: now, FailureDays: 2, LastFailureDay: day}
	down := federation.InstanceHealth{Domain: "c.example", State: federation.InstanceUnavailable, Since: now, FailureDays: 7, LastFailureDay: day}
	downToo := federation.InstanceHealth{Domain: "d.example", State: federation.InstanceUnavailable, Since: now, FailureDays: 7, LastFailureDay: day}
	for _, h := range []federation.InstanceHealth{downToo, healthy, down, failing} {
		upsert(h)
	}
	// The upsert replaces the row: d.example was failing before it became unavailable.
	upsert(federation.InstanceHealth{Domain: "d.example", State: federation.InstanceAvailable, Since: now, FailureDays: 1, LastFailureDay: day})
	upsert(downToo)

	if got, err := r.h.Federation.FindInstanceHealth(ctx, "b.example"); err != nil || got != failing {
		t.Errorf("FindInstanceHealth = %+v, %v; want %+v", got, err, failing)
	}
	if got, err := r.h.Federation.InstanceState(ctx, "c.example"); err != nil || got != federation.InstanceUnavailable {
		t.Errorf("InstanceState(unavailable) = %q, %v", got, err)
	}
	states := map[string]federation.InstanceState{"a.example": federation.InstanceAvailable, "b.example": federation.InstanceAvailable,
		"c.example": federation.InstanceUnavailable, "unknown.example": federation.InstanceAvailable}
	if got, err := r.h.Federation.InstanceStatesOf(ctx, []string{"a.example", "b.example", "c.example", "unknown.example"}); err != nil || !maps.Equal(got, states) {
		t.Errorf("InstanceStatesOf = %v, %v; want %v", got, err, states)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Federation.LockInstanceHealth(ctx, tx, "c.example"); err != nil || got != down {
			t.Errorf("LockInstanceHealth = %+v, %v; want %+v", got, err, down)
		}
		if _, err := r.h.Federation.LockInstanceHealth(ctx, tx, "unknown.example"); !errors.Is(err, appfederation.ErrNotFound) {
			t.Errorf("LockInstanceHealth without a row = %v, want ErrNotFound", err)
		}
		return nil
	})

	if got, err := r.h.Federation.ListUnavailableInstances(ctx, "", 1); err != nil || !slices.Equal(got, []federation.InstanceHealth{down}) {
		t.Errorf("ListUnavailableInstances(first) = %+v, %v; want c.example", got, err)
	}
	if got, err := r.h.Federation.ListUnavailableInstances(ctx, "c.example", 10); err != nil || !slices.Equal(got, []federation.InstanceHealth{downToo}) {
		t.Errorf("ListUnavailableInstances(after c.example) = %+v, %v; want d.example", got, err)
	}
	all := []federation.InstanceHealth{healthy, failing, down, downToo}
	if got, err := r.h.Federation.ListInstanceHealth(ctx, "", "", 3); err != nil || !slices.Equal(got, all[:3]) {
		t.Errorf("ListInstanceHealth(every state) = %+v, %v; want %+v", got, err, all[:3])
	}
	if got, err := r.h.Federation.ListInstanceHealth(ctx, "", "c.example", 3); err != nil || !slices.Equal(got, all[3:]) {
		t.Errorf("ListInstanceHealth(after c.example) = %+v, %v; want %+v", got, err, all[3:])
	}
	if got, err := r.h.Federation.ListInstanceHealth(ctx, federation.InstanceAvailable, "", 10); err != nil || !slices.Equal(got, all[:2]) {
		t.Errorf("ListInstanceHealth(available) = %+v, %v; want %+v", got, err, all[:2])
	}
	// The healthy server is neither failing nor unavailable, so it is not counted.
	want := map[federation.InstanceState]int64{federation.InstanceAvailable: 1, federation.InstanceUnavailable: 2}
	if got, err := r.h.Federation.CountInstanceHealth(ctx); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("CountInstanceHealth = %v, %v; want %v", got, err, want)
	}
}

func fedRepoAvailabilityEvents(t *testing.T, r *fedRepo) {
	ctx := t.Context()
	var events []federation.AvailabilityEvent
	for i, kind := range []federation.AvailabilityKind{federation.MarkedUnavailable, federation.MarkedAvailable, federation.MarkedUnavailable} {
		reason := federation.ReasonFailureDays
		if i == 1 {
			reason = federation.ReasonProbeSucceeded
		}
		e := federation.AvailabilityEvent{ID: r.ids.NewID(), Domain: "remote.example", Kind: kind, Reason: reason, OccurredAt: r.clock.Now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Federation.InsertAvailabilityEvent(ctx, tx, e) })
		events = append(events, e)
		r.clock.Advance(time.Second)
	}
	other := federation.AvailabilityEvent{
		ID: r.ids.NewID(), Domain: "other.example", Kind: federation.MarkedUnavailable, Reason: federation.ReasonOperator, OccurredAt: r.clock.Now(),
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Federation.InsertAvailabilityEvent(ctx, tx, other)
	})

	newest, err := r.h.Federation.ListAvailabilityEvents(ctx, "remote.example", "", 2)
	if want := []federation.AvailabilityEvent{events[2], events[1]}; err != nil || !slices.Equal(newest, want) {
		t.Errorf("ListAvailabilityEvents(newest) = %+v, %v; want %+v", newest, err, want)
	}
	if got, err := r.h.Federation.ListAvailabilityEvents(ctx, "remote.example", events[1].ID, 2); err != nil || !slices.Equal(got, events[:1]) {
		t.Errorf("ListAvailabilityEvents(before the second) = %+v, %v; want the first", got, err)
	}
	if got, err := r.h.Federation.ListAvailabilityEvents(ctx, "unknown.example", "", 10); err != nil || len(got) != 0 {
		t.Errorf("ListAvailabilityEvents(unknown) = %+v, %v; want none", got, err)
	}
}

func fedRepoAnswers(t *testing.T, r *fedRepo) {
	uri := federation.IRI("https://remote.example/activities/follow-1")
	for _, want := range []bool{true, false} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ok, err := r.h.Federation.InsertReceivedActivityAnswer(ctx, tx, uri, r.clock.Now())
			if err == nil && ok != want {
				t.Errorf("InsertReceivedActivityAnswer = %v, want %v", ok, want)
			}
			return err
		})
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		ok, err := r.h.Federation.InsertReceivedActivityAnswer(ctx, tx, uri+"-2", r.clock.Now())
		if err == nil && !ok {
			t.Error("InsertReceivedActivityAnswer of another activity = false, want true")
		}
		return err
	})
}

func fedRepoClosure(t *testing.T, r *fedRepo) {
	ctx := t.Context()
	const maxAttempts = 2
	empty := federation.ClosureProgress{Deliveries: map[federation.ClosureDeliveryState]int{}}
	if got, err := r.h.Federation.ClosureProgress(ctx, maxAttempts); err != nil || !reflect.DeepEqual(got, empty) {
		t.Errorf("ClosureProgress before any Delete = %+v, %v; want none", got, err)
	}
	record := func(accountID string, want bool) federation.OutboundActivity {
		t.Helper()
		a := r.activity(t, federation.ActivityDelete, federation.OutboundAccount, accountID)
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ok, err := r.h.Federation.InsertClosureDelete(ctx, tx, federation.ClosureDelete{AccountID: accountID, OutboundActivityID: a.ID, RecordedAt: r.clock.Now()})
			if err == nil && ok != want {
				t.Errorf("InsertClosureDelete(%s) = %v, want %v", accountID, ok, want)
			}
			return err
		})
		return a
	}
	alice, bob, carol := testkit.NamedID("alice"), testkit.NamedID("bob"), testkit.NamedID("carol")
	first := record(alice, true)
	record(alice, false)
	second := record(bob, true)
	// A Delete of an account that is not the closure's counts for nothing.
	plain := r.activity(t, federation.ActivityDelete, federation.OutboundAccount, carol)
	r.delivery(t, plain.ID, "plain.example")

	if got, err := r.h.Federation.ClosureDeleteAccounts(ctx, []string{carol, bob, alice}); err != nil || !slices.Equal(got, slices.Sorted(slices.Values([]string{alice, bob}))) {
		t.Errorf("ClosureDeleteAccounts = %v, %v; want alice and bob in ID order", got, err)
	}
	if got, err := r.h.Federation.ClosureDeleteAccounts(ctx, nil); err != nil || len(got) != 0 {
		t.Errorf("ClosureDeleteAccounts(none) = %v, %v; want none", got, err)
	}

	r.fanOutCompleted(t, first.ID)
	r.delivery(t, first.ID, "pending.example")
	retrying := r.delivery(t, first.ID, "retrying.example")
	r.attempt(t, retrying.ID, federation.DeliveryRetryableFailure)
	dead := r.delivery(t, first.ID, "dead.example")
	r.attempt(t, dead.ID, federation.DeliveryRetryableFailure)
	r.attempt(t, dead.ID, federation.DeliveryRetryableFailure)
	succeeded := r.delivery(t, second.ID, "succeeded.example")
	r.attempt(t, succeeded.ID, federation.DeliveryRetryableFailure)
	r.attempt(t, succeeded.ID, federation.DeliverySucceeded)
	refused := r.delivery(t, second.ID, "refused.example")
	r.attempt(t, refused.ID, federation.DeliveryPermanentFailure)
	skipped := r.delivery(t, second.ID, "skipped.example")
	r.attempt(t, skipped.ID, federation.DeliverySkipped)

	want := federation.ClosureProgress{Accounts: 2, FanOuts: 1, Deliveries: map[federation.ClosureDeliveryState]int{
		federation.ClosureDeliveryPending: 2, federation.ClosureDeliveryDead: 1, federation.ClosureDeliverySucceeded: 1,
		federation.ClosureDeliveryPermanentFailure: 1, federation.ClosureDeliverySkipped: 1,
	}}
	if got, err := r.h.Federation.ClosureProgress(ctx, maxAttempts); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ClosureProgress = %+v, %v; want %+v", got, err, want)
	}
	// A repeated completion keeps the first, and ends the second fan-out.
	r.fanOutCompleted(t, first.ID)
	r.fanOutCompleted(t, second.ID)
	if got, err := r.h.Federation.ClosureProgress(ctx, maxAttempts); err != nil || got.FanOuts != 0 {
		t.Errorf("ClosureProgress after the fan-outs = %+v, %v; want no fan-out left", got, err)
	}
}

func fedRepoPendingMoves(t *testing.T, r *fedRepo) {
	ctx := t.Context()
	const maxAttempts = 2
	move := func() federation.OutboundActivity {
		t.Helper()
		return r.activity(t, federation.ActivityMove, federation.OutboundMove, r.ids.NewID())
	}
	move() // recorded before since, not counted
	since := r.clock.Now()
	fanningOut := move()
	r.delivery(t, fanningOut.ID, "a.example")

	unattempted := move()
	r.fanOutCompleted(t, unattempted.ID)
	r.delivery(t, unattempted.ID, "a.example")

	retrying := move()
	r.fanOutCompleted(t, retrying.ID)
	d := r.delivery(t, retrying.ID, "a.example")
	r.attempt(t, d.ID, federation.DeliveryRetryableFailure)

	exhausted := move()
	r.fanOutCompleted(t, exhausted.ID)
	d = r.delivery(t, exhausted.ID, "a.example")
	r.attempt(t, d.ID, federation.DeliveryRetryableFailure)
	r.attempt(t, d.ID, federation.DeliveryRetryableFailure)

	ended := move()
	r.fanOutCompleted(t, ended.ID)
	for outcome, host := range map[federation.DeliveryOutcome]string{
		federation.DeliverySucceeded: "a.example", federation.DeliveryPermanentFailure: "b.example", federation.DeliverySkipped: "c.example",
	} {
		d := r.delivery(t, ended.ID, host)
		r.attempt(t, d.ID, outcome)
	}
	// An activity of another kind is not a Move, whatever its deliveries.
	r.activity(t, federation.ActivityCreate, federation.OutboundStatus, testkit.NamedID("status"))

	// The Moves still being sent: the one fanning out, the one not attempted, and the one retrying.
	if got, err := r.h.Federation.CountPendingMoves(ctx, since, maxAttempts); err != nil || got != 3 {
		t.Errorf("CountPendingMoves = %d, %v; want 3", got, err)
	}
	if got, err := r.h.Federation.CountPendingMoves(ctx, r.clock.Now(), maxAttempts); err != nil || got != 0 {
		t.Errorf("CountPendingMoves(since now) = %d, %v; want 0", got, err)
	}
}
