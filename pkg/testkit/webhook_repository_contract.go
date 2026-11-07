// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	appwebhook "github.com/mycroblog/my/internal/app/webhook"
	"github.com/mycroblog/my/internal/domain/webhook"
)

// WebhookRepositoryContract verifies the webhook repository of a database adapter by its methods
// alone, the port contract of ADR-0153 §1: the endpoints with their revisions, state changes,
// secrets, and heads; the enabled endpoints that subscribe to an event; the deliveries with their
// attempts, newest first and in pages; the runs of failures, which count each failure; the purge of
// old messages with their deliveries and attempts; and the projections derived again. The webhook
// use cases on top of it are WebhookFlows, which internal/flowtest/webhook runs once on SQLite.
func WebhookRepositoryContract(t *testing.T, newHarness WebhookFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *webhookRepo)
	}{
		{"FindsListsAndDeletesEndpoints", webhookRepoEndpoints},
		{"ListsTheEnabledSubscribers", webhookRepoSubscribers},
		{"ListsSecretsNewestFirst", webhookRepoSecrets},
		{"RecordsDeliveriesAndAttempts", webhookRepoDeliveries},
		{"CountsARunOfFailures", webhookRepoFailures},
		{"DeletesOldMessages", webhookRepoPurge},
		{"RebuildsTheProjections", webhookRepoRebuild},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{})
			clock := NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
			tc.run(t, &webhookRepo{h: h, clock: clock, ids: NewIDGenerator(clock)})
		})
	}
}

type webhookRepo struct {
	h     WebhookHarness
	clock *Clock
	ids   *IDGenerator
}

func (r *webhookRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

// endpoint inserts an endpoint with its first revision, secret, and state change, and its head, as
// the use case and its projector do.
func (r *webhookRepo) endpoint(t *testing.T, state webhook.State, events ...webhook.EventType) appwebhook.Head {
	t.Helper()
	slices.Sort(events)
	head := appwebhook.Head{EndpointID: r.ids.NewID(), RevisionID: r.ids.NewID(), State: state, SecretID: r.ids.NewID()}
	now := r.clock.Now()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.h.Webhooks.InsertEndpoint(ctx, tx, head.EndpointID, "", now),
			r.h.Webhooks.InsertRevision(ctx, tx, appwebhook.Revision{
				ID: head.RevisionID, EndpointID: head.EndpointID, RevisedAt: now,
				Settings: webhook.EndpointSettings{URL: "https://hooks.example/" + head.EndpointID, Description: "Chat", Events: events},
			}),
			r.h.Webhooks.InsertSecret(ctx, tx, webhook.Secret{ID: head.SecretID, EndpointID: head.EndpointID, Sealed: []byte("sealed"), CreatedAt: now}),
			r.h.Webhooks.InsertStateChange(ctx, tx, appwebhook.StateChange{
				ID: r.ids.NewID(), EndpointID: head.EndpointID, State: state, Reason: webhook.ReasonAdmin, ChangedAt: now,
			}),
			r.h.Webhooks.UpsertHead(ctx, tx, head),
		)
	})
	r.clock.Advance(time.Second)
	return head
}

// changeState records a state change of an endpoint and moves its head to it.
func (r *webhookRepo) changeState(t *testing.T, head appwebhook.Head, state webhook.State) appwebhook.Head {
	t.Helper()
	head.State = state
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.h.Webhooks.InsertStateChange(ctx, tx, appwebhook.StateChange{
				ID: r.ids.NewID(), EndpointID: head.EndpointID, State: state, Reason: webhook.ReasonAdmin, ChangedAt: r.clock.Now(),
			}),
			r.h.Webhooks.UpsertHead(ctx, tx, head),
		)
	})
	r.clock.Advance(time.Second)
	return head
}

// delivery inserts a message of event and a delivery of it to the endpoint.
func (r *webhookRepo) delivery(t *testing.T, endpointID string, event webhook.EventType) webhook.Delivery {
	t.Helper()
	m := webhook.Message{ID: r.ids.NewID(), EventType: event, Body: []byte(`{"type":"` + string(event) + `"}`), CreatedAt: r.clock.Now()}
	d := webhook.Delivery{ID: r.ids.NewID(), MessageID: m.ID, EndpointID: endpointID, CreatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.h.Webhooks.InsertMessage(ctx, tx, m), r.h.Webhooks.InsertDelivery(ctx, tx, d))
	})
	r.clock.Advance(time.Second)
	return d
}

// attempt records an attempt of a delivery: a success with 200, or a failure with 500.
func (r *webhookRepo) attempt(t *testing.T, deliveryID string, ok bool) webhook.Attempt {
	t.Helper()
	a := webhook.Attempt{ID: r.ids.NewID(), DeliveryID: deliveryID, Outcome: webhook.OutcomeSucceeded, StatusCode: 200,
		Duration: 120 * time.Millisecond, AttemptedAt: r.clock.Now()}
	if !ok {
		a.Outcome, a.StatusCode = webhook.OutcomeHTTPError, 500
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Webhooks.InsertAttempt(ctx, tx, a) })
	r.clock.Advance(time.Second)
	return a
}

func webhookRepoEndpoints(t *testing.T, r *webhookRepo) {
	e1 := r.endpoint(t, webhook.StateEnabled, webhook.EventReportCreated)
	created := r.clock.Now()
	e2 := r.endpoint(t, webhook.StateDisabled, webhook.EventAccountCreated)

	got, err := r.h.Webhooks.FindEndpoint(t.Context(), e2.EndpointID)
	if err != nil {
		t.Fatalf("FindEndpoint: %v", err)
	}
	if got.RevisionID != e2.RevisionID || got.SecretID != e2.SecretID || got.State != webhook.StateDisabled ||
		got.StateReason != webhook.ReasonAdmin || !got.CreatedAt.Equal(created) || got.Failures != nil ||
		got.Settings.URL != "https://hooks.example/"+e2.EndpointID || got.Settings.Description != "Chat" ||
		!slices.Equal(got.Settings.Events, []webhook.EventType{webhook.EventAccountCreated}) {
		t.Errorf("FindEndpoint = %+v, want the endpoint as inserted", got)
	}
	ids := func(es []webhook.Endpoint) []string {
		var out []string
		for _, e := range es {
			out = append(out, e.ID)
		}
		return out
	}
	list, err := r.h.Webhooks.ListEndpoints(t.Context())
	if err != nil || !slices.Equal(ids(list), []string{e2.EndpointID, e1.EndpointID}) {
		t.Errorf("ListEndpoints = %v, %v; want newest first", ids(list), err)
	}

	var head appwebhook.Head
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		head, err = r.h.Webhooks.HeadForUpdate(ctx, tx, e1.EndpointID)
		return err
	})
	if head != e1 {
		t.Errorf("HeadForUpdate = %+v, want %+v", head, e1)
	}

	r.changeState(t, e2, webhook.StateDeleted)
	if _, err := r.h.Webhooks.FindEndpoint(t.Context(), e2.EndpointID); !errors.Is(err, appwebhook.ErrNotFound) {
		t.Errorf("FindEndpoint of a deleted endpoint: %v, want ErrNotFound", err)
	}
	list, err = r.h.Webhooks.ListEndpoints(t.Context())
	if err != nil || !slices.Equal(ids(list), []string{e1.EndpointID}) {
		t.Errorf("ListEndpoints after a delete = %v, %v", ids(list), err)
	}
	unknown := r.ids.NewID()
	existing, err := r.h.Webhooks.ExistingEndpointIDs(t.Context(), []string{e2.EndpointID, unknown, e1.EndpointID})
	if err != nil || !slices.Equal(existing, []string{e1.EndpointID}) {
		t.Errorf("ExistingEndpointIDs = %v, %v; want the endpoint that is not deleted", existing, err)
	}
	if _, err := r.h.Webhooks.FindEndpoint(t.Context(), unknown); !errors.Is(err, appwebhook.ErrNotFound) {
		t.Errorf("FindEndpoint of an unknown endpoint: %v, want ErrNotFound", err)
	}
	err = r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		_, err := r.h.Webhooks.HeadForUpdate(t.Context(), tx, unknown)
		return err
	})
	if !errors.Is(err, appwebhook.ErrNotFound) {
		t.Errorf("HeadForUpdate of an unknown endpoint: %v, want ErrNotFound", err)
	}
}

func webhookRepoSubscribers(t *testing.T, r *webhookRepo) {
	a := r.endpoint(t, webhook.StateEnabled, webhook.EventReportCreated)
	b := r.endpoint(t, webhook.StateEnabled, webhook.EventReportCreated, webhook.EventAccountCreated)
	r.endpoint(t, webhook.StateDisabled, webhook.EventReportCreated)
	c := r.endpoint(t, webhook.StateEnabled, webhook.EventReportCreated)
	r.changeState(t, c, webhook.StateDeleted)

	sorted := func(ids ...string) []string { slices.Sort(ids); return ids }
	for event, want := range map[webhook.EventType][]string{
		webhook.EventReportCreated:  sorted(a.EndpointID, b.EndpointID),
		webhook.EventAccountCreated: {b.EndpointID},
		webhook.EventAppealCreated:  nil,
	} {
		var got []string
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			got, err = r.h.Webhooks.SubscribedEndpoints(ctx, tx, event)
			return err
		})
		if !slices.Equal(got, want) {
			t.Errorf("SubscribedEndpoints(%s) = %v, want %v", event, got, want)
		}
	}
}

func webhookRepoSecrets(t *testing.T, r *webhookRepo) {
	e := r.endpoint(t, webhook.StateEnabled, webhook.EventReportCreated)
	var newer []string
	for range 2 {
		s := webhook.Secret{ID: r.ids.NewID(), EndpointID: e.EndpointID, Sealed: []byte("rotated"), CreatedAt: r.clock.Now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Webhooks.InsertSecret(ctx, tx, s) })
		newer = append([]string{s.ID}, newer...)
		r.clock.Advance(time.Second)
	}
	got, err := r.h.Webhooks.Secrets(t.Context(), e.EndpointID, 2)
	if err != nil || len(got) != 2 || got[0].ID != newer[0] || got[1].ID != newer[1] || string(got[0].Sealed) != "rotated" {
		t.Errorf("Secrets(2) = %+v, %v; want the two newest, newest first", got, err)
	}
	all, err := r.h.Webhooks.Secrets(t.Context(), e.EndpointID, 10)
	if err != nil || len(all) != 3 || all[2].ID != e.SecretID {
		t.Errorf("Secrets(10) = %d secrets, %v; want 3 ending with the first", len(all), err)
	}
}

func webhookRepoDeliveries(t *testing.T, r *webhookRepo) {
	e := r.endpoint(t, webhook.StateEnabled, webhook.EventReportCreated)
	other := r.endpoint(t, webhook.StateEnabled, webhook.EventReportCreated)
	d1 := r.delivery(t, e.EndpointID, webhook.EventReportCreated)
	d2 := r.delivery(t, e.EndpointID, webhook.EventReportResolved)

	send, err := r.h.Webhooks.FindSend(t.Context(), d1.ID)
	if err != nil || send.Delivery.ID != d1.ID || send.Message.ID != d1.MessageID || send.Message.EventType != webhook.EventReportCreated ||
		string(send.Message.Body) != `{"type":"report.created"}` || send.URL != "https://hooks.example/"+e.EndpointID ||
		send.Description != "Chat" || send.State != webhook.StateEnabled || send.Succeeded {
		t.Errorf("FindSend = %+v, %v", send, err)
	}

	failed := r.attempt(t, d1.ID, false)
	rec, err := r.h.Webhooks.FindDelivery(t.Context(), e.EndpointID, d1.ID)
	if err != nil || rec.Attempts != 1 || rec.Succeeded || rec.Last == nil || rec.Last.ID != failed.ID ||
		rec.Last.Outcome != webhook.OutcomeHTTPError || rec.Last.StatusCode != 500 || rec.Last.Duration != 120*time.Millisecond ||
		rec.EventType != webhook.EventReportCreated {
		t.Errorf("FindDelivery after a failure = %+v, %v", rec, err)
	}
	r.attempt(t, d1.ID, true)
	rec, err = r.h.Webhooks.FindDelivery(t.Context(), e.EndpointID, d1.ID)
	if err != nil || rec.Attempts != 2 || !rec.Succeeded || rec.Last == nil || rec.Last.Outcome != webhook.OutcomeSucceeded {
		t.Errorf("FindDelivery after a success = %+v, %v", rec, err)
	}
	if send, err := r.h.Webhooks.FindSend(t.Context(), d1.ID); err != nil || !send.Succeeded {
		t.Errorf("FindSend after a success: Succeeded %v, %v", send.Succeeded, err)
	}

	page, err := r.h.Webhooks.ListDeliveries(t.Context(), e.EndpointID, "", 10)
	if err != nil || len(page) != 2 || page[0].Delivery.ID != d2.ID || page[1].Delivery.ID != d1.ID || page[0].Attempts != 0 {
		t.Errorf("ListDeliveries = %+v, %v; want newest first", page, err)
	}
	page, err = r.h.Webhooks.ListDeliveries(t.Context(), e.EndpointID, d2.ID, 1)
	if err != nil || len(page) != 1 || page[0].Delivery.ID != d1.ID {
		t.Errorf("ListDeliveries before the newest = %+v, %v", page, err)
	}
	if page, err := r.h.Webhooks.ListDeliveries(t.Context(), other.EndpointID, "", 10); err != nil || len(page) != 0 {
		t.Errorf("ListDeliveries of another endpoint = %+v, %v", page, err)
	}
	if _, err := r.h.Webhooks.FindDelivery(t.Context(), other.EndpointID, d1.ID); !errors.Is(err, appwebhook.ErrNotFound) {
		t.Errorf("FindDelivery under another endpoint: %v, want ErrNotFound", err)
	}
	if _, err := r.h.Webhooks.FindSend(t.Context(), r.ids.NewID()); !errors.Is(err, appwebhook.ErrNotFound) {
		t.Errorf("FindSend of an unknown delivery: %v, want ErrNotFound", err)
	}
}

func webhookRepoFailures(t *testing.T, r *webhookRepo) {
	e := r.endpoint(t, webhook.StateEnabled, webhook.EventReportCreated)
	d := r.delivery(t, e.EndpointID, webhook.EventReportCreated)
	failures := func() *webhook.Failures {
		t.Helper()
		var f *webhook.Failures
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			f, err = r.h.Webhooks.Failures(ctx, tx, e.EndpointID)
			return err
		})
		return f
	}
	if f := failures(); f != nil {
		t.Fatalf("Failures before any = %+v, want nil", f)
	}
	first := r.attempt(t, d.ID, false)
	second := r.attempt(t, d.ID, false)
	for _, a := range []webhook.Attempt{first, second} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return r.h.Webhooks.AddFailure(ctx, tx, e.EndpointID, a.ID, a.AttemptedAt)
		})
	}
	f := failures()
	if f == nil || f.Consecutive != 2 || f.FirstFailureAttemptID != first.ID || !f.FirstFailureAt.Equal(first.AttemptedAt) {
		t.Errorf("Failures = %+v, want 2 from the first attempt", f)
	}
	if got, err := r.h.Webhooks.FindEndpoint(t.Context(), e.EndpointID); err != nil || got.Failures == nil || got.Failures.Consecutive != 2 {
		t.Errorf("FindEndpoint's failures = %+v, %v", got.Failures, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Webhooks.DeleteFailures(ctx, tx, e.EndpointID) })
	if f := failures(); f != nil {
		t.Errorf("Failures after DeleteFailures = %+v, want nil", f)
	}
}

func webhookRepoPurge(t *testing.T, r *webhookRepo) {
	e := r.endpoint(t, webhook.StateEnabled, webhook.EventReportCreated)
	var old []webhook.Delivery
	for range 3 {
		d := r.delivery(t, e.EndpointID, webhook.EventReportCreated)
		r.attempt(t, d.ID, true)
		old = append(old, d)
	}
	cutoff := r.clock.Now()
	r.clock.Advance(time.Hour)
	kept := r.delivery(t, e.EndpointID, webhook.EventReportCreated)

	deleted := func(limit int) int64 {
		t.Helper()
		var n int64
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			n, err = r.h.Webhooks.DeleteMessagesBefore(ctx, tx, cutoff, limit)
			return err
		})
		return n
	}
	if n := deleted(2); n != 2 {
		t.Errorf("DeleteMessagesBefore(limit 2) = %d, want 2", n)
	}
	if n := deleted(10); n != 1 {
		t.Errorf("DeleteMessagesBefore again = %d, want the 1 left", n)
	}
	for _, d := range old {
		if _, err := r.h.Webhooks.FindSend(t.Context(), d.ID); !errors.Is(err, appwebhook.ErrNotFound) {
			t.Errorf("FindSend of a purged delivery: %v, want ErrNotFound", err)
		}
	}
	if _, err := r.h.Webhooks.FindSend(t.Context(), kept.ID); err != nil {
		t.Errorf("FindSend of a newer delivery: %v", err)
	}
	if page, err := r.h.Webhooks.ListDeliveries(t.Context(), e.EndpointID, "", 10); err != nil || len(page) != 1 || page[0].Delivery.ID != kept.ID {
		t.Errorf("ListDeliveries after the purge = %+v, %v", page, err)
	}
}

func webhookRepoRebuild(t *testing.T, r *webhookRepo) {
	e := r.endpoint(t, webhook.StateEnabled, webhook.EventReportCreated)
	e = r.changeState(t, e, webhook.StateDisabled)
	d := r.delivery(t, e.EndpointID, webhook.EventReportCreated)
	r.attempt(t, d.ID, true)
	first := r.attempt(t, d.ID, false)
	r.attempt(t, d.ID, false)

	if err := appwebhook.RebuildProjections(t.Context(), r.h.UnitOfWork, r.h.Webhooks); err != nil {
		t.Fatalf("RebuildProjections: %v", err)
	}
	var head appwebhook.Head
	var f *webhook.Failures
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		if head, err = r.h.Webhooks.HeadForUpdate(ctx, tx, e.EndpointID); err != nil {
			return err
		}
		f, err = r.h.Webhooks.Failures(ctx, tx, e.EndpointID)
		return err
	})
	if head != e {
		t.Errorf("the rebuilt head = %+v, want %+v", head, e)
	}
	if f == nil || f.Consecutive != 2 || f.FirstFailureAttemptID != first.ID {
		t.Errorf("the rebuilt failures = %+v, want the 2 after the newest success", f)
	}
}
