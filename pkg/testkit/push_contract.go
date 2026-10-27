// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// PushHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, and the Web Push subscriptions of the notification repository on it.
type PushHarness struct {
	UnitOfWork uow.UnitOfWork
	Push       appnotification.PushRepository
}

// PushFactory returns a harness on a new, empty database. The factory registers the cleanup
// of everything it creates with t.Cleanup.
type PushFactory func(t *testing.T) PushHarness

// PushSubscriptionContract verifies the Web Push subscriptions of a database adapter together
// with the notification module's PushService (FR-206): one subscription per grant and per
// endpoint, the alerts in a stable order, changing the alerts of a grant's browser in place,
// the subscriptions that take a type, deleting the subscription of a grant, and the push job,
// which sends to a subscription that takes the type while its grant is active, sends nothing
// once the subscription is gone, and deletes a subscription that the push service reports gone
// or whose grant has ended.
func PushSubscriptionContract(t *testing.T, newHarness PushFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *pushEnv)
	}{
		{"KeepsOneSubscriptionPerGrantAndEndpoint", pushSubscriptions},
		{"ListsTheSubscriptionsOfAType", pushSubscriptionIDs},
		{"RejectsInvalidSubscriptions", pushRejects},
		{"SendsOnlyToLiveSubscriptions", pushJobs},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newPushEnv(t, newHarness)) })
	}
}

// Keys of the example of RFC 8291 Appendix A.
const (
	contractPushP256DH = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	contractPushAuth   = "BTBZMqHH6r4Tts7J_aSIgg"
	contractPushAcct   = "01890a5d-ac96-774b-bcce-b302099a8057"
)

type pushEnv struct {
	h        PushHarness
	clock    *Clock
	sender   *stubPushSender
	audience *stubPushAudience
	svc      *appnotification.PushService
	jobs     *queue.JobHandlerRegistry
}

func newPushEnv(t *testing.T, newHarness PushFactory) *pushEnv {
	t.Helper()
	h := newHarness(t)
	clock := NewClock(time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC))
	e := &pushEnv{
		h: h, clock: clock, sender: &stubPushSender{outcome: appnotification.PushDelivered},
		audience: &stubPushAudience{ended: map[string]bool{}}, jobs: &queue.JobHandlerRegistry{},
	}
	e.svc = appnotification.NewPushService(appnotification.PushDeps{
		UnitOfWork: h.UnitOfWork, Repository: h.Push, Clock: clock, IDs: NewIDGenerator(clock),
		Sender: e.sender, Audience: e.audience, Host: "social.example",
	})
	e.svc.RegisterPushJobs(e.jobs)
	return e
}

func (e *pushEnv) subscribe(t *testing.T, family, endpoint string, alerts ...notification.Type) notification.PushSubscription {
	t.Helper()
	s, err := e.svc.Subscribe(t.Context(), appnotification.SubscribeInput{
		AccountID: contractPushAcct, TokenFamilyID: family, Endpoint: endpoint,
		KeyP256DH: contractPushP256DH, KeyAuth: contractPushAuth + "==", Alerts: alerts,
	})
	if err != nil {
		t.Fatalf("Subscribe(%s, %s): %v", family, endpoint, err)
	}
	return s
}

func (e *pushEnv) ofFamily(t *testing.T, family string) (notification.PushSubscription, bool) {
	t.Helper()
	s, err := e.h.Push.FindPushSubscriptionOfFamily(t.Context(), family)
	if errors.Is(err, appnotification.ErrNotFound) {
		return s, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return s, true
}

func pushSubscriptions(t *testing.T, e *pushEnv) {
	first := e.subscribe(t, NamedID("family-1"), "https://push.example.net/1", notification.TypeMention, notification.TypeFollow, notification.TypeMention)
	if want := []notification.Type{notification.TypeFollow, notification.TypeMention}; !reflect.DeepEqual(first.Alerts, want) {
		t.Errorf("alerts = %v, want %v", first.Alerts, want)
	}
	if first.KeyAuth != contractPushAuth {
		t.Errorf("auth = %q, want it without padding", first.KeyAuth)
	}
	got, ok := e.ofFamily(t, NamedID("family-1"))
	if !ok || !reflect.DeepEqual(got, first) {
		t.Errorf("FindPushSubscriptionOfFamily = %+v, %v; want %+v", got, ok, first)
	}
	if byID, err := e.h.Push.FindPushSubscription(t.Context(), first.ID); err != nil || !reflect.DeepEqual(byID, first) {
		t.Errorf("FindPushSubscription = %+v, %v", byID, err)
	}

	// The same browser changes its alerts: the subscription stays.
	e.clock.Advance(time.Minute)
	changed := e.subscribe(t, NamedID("family-1"), "https://push.example.net/1", notification.TypeBoost)
	if changed.ID != first.ID || !changed.CreatedAt.Equal(first.CreatedAt) || !changed.UpdatedAt.After(first.UpdatedAt) {
		t.Errorf("changed = %+v, want the ID and creation of %+v", changed, first)
	}
	if got, _ := e.ofFamily(t, NamedID("family-1")); !reflect.DeepEqual(got.Alerts, []notification.Type{notification.TypeBoost}) {
		t.Errorf("alerts after the change = %v", got.Alerts)
	}

	// Another grant registers the same endpoint: it takes the endpoint over.
	other := e.subscribe(t, NamedID("family-2"), "https://push.example.net/1", notification.TypeMention)
	if _, ok := e.ofFamily(t, NamedID("family-1")); ok {
		t.Error("the first grant keeps a subscription with the endpoint of the second")
	}
	if _, err := e.h.Push.FindPushSubscription(t.Context(), first.ID); !errors.Is(err, appnotification.ErrNotFound) {
		t.Errorf("FindPushSubscription of the replaced = %v, want ErrNotFound", err)
	}

	// The grant subscribes another browser: it replaces its subscription.
	moved := e.subscribe(t, NamedID("family-2"), "https://push.example.net/2", notification.TypeMention)
	if moved.ID == other.ID {
		t.Error("a new endpoint keeps the ID of the old subscription")
	}
	if got, _ := e.ofFamily(t, NamedID("family-2")); got.Endpoint != "https://push.example.net/2" {
		t.Errorf("endpoint = %s", got.Endpoint)
	}

	if err := e.svc.Unsubscribe(t.Context(), NamedID("family-2")); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.ofFamily(t, NamedID("family-2")); ok {
		t.Error("Unsubscribe left the subscription")
	}
	if err := e.svc.Unsubscribe(t.Context(), NamedID("family-2")); err != nil {
		t.Errorf("Unsubscribe again: %v", err)
	}
}

func pushSubscriptionIDs(t *testing.T, e *pushEnv) {
	a := e.subscribe(t, NamedID("family-a"), "https://push.example.net/a", notification.TypeMention, notification.TypeBoost)
	b := e.subscribe(t, NamedID("family-b"), "https://push.example.net/b", notification.TypeMention)
	ids := func(acct string, typ notification.Type) []string {
		t.Helper()
		var out []string
		err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
			var err error
			out, err = e.h.Push.PushSubscriptionIDs(t.Context(), tx, acct, typ)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := ids(contractPushAcct, notification.TypeMention); !reflect.DeepEqual(got, []string{a.ID, b.ID}) {
		t.Errorf("mention = %v, want %v", got, []string{a.ID, b.ID})
	}
	if got := ids(contractPushAcct, notification.TypeBoost); !reflect.DeepEqual(got, []string{a.ID}) {
		t.Errorf("boost = %v", got)
	}
	if got := ids(contractPushAcct, notification.TypeFollow); len(got) != 0 {
		t.Errorf("follow = %v", got)
	}
	if got := ids("01890a5d-ac96-774b-bcce-b302099a8058", notification.TypeMention); len(got) != 0 {
		t.Errorf("another account = %v", got)
	}
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		deleted, err := e.h.Push.DeletePushSubscription(t.Context(), tx, a.ID)
		if err == nil && !deleted {
			t.Error("DeletePushSubscription reports nothing deleted")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(contractPushAcct, notification.TypeBoost); len(got) != 0 {
		t.Errorf("boost after the deletion = %v", got)
	}
}

func pushRejects(t *testing.T, e *pushEnv) {
	cases := []struct {
		name string
		in   appnotification.SubscribeInput
		want error
	}{
		{"http endpoint", appnotification.SubscribeInput{Endpoint: "http://push.example.net/1", KeyP256DH: contractPushP256DH, KeyAuth: contractPushAuth}, notification.ErrInvalidPushEndpoint},
		{"relative endpoint", appnotification.SubscribeInput{Endpoint: "/push", KeyP256DH: contractPushP256DH, KeyAuth: contractPushAuth}, notification.ErrInvalidPushEndpoint},
		{"short key", appnotification.SubscribeInput{Endpoint: "https://push.example.net/1", KeyP256DH: "BCVx", KeyAuth: contractPushAuth}, notification.ErrInvalidPushKey},
		{"short auth", appnotification.SubscribeInput{Endpoint: "https://push.example.net/1", KeyP256DH: contractPushP256DH, KeyAuth: "BTBZ"}, notification.ErrInvalidPushKey},
		{"unknown type", appnotification.SubscribeInput{Endpoint: "https://push.example.net/1", KeyP256DH: contractPushP256DH, KeyAuth: contractPushAuth, Alerts: []notification.Type{"poke"}}, notification.ErrInvalidType},
	}
	for _, tc := range cases {
		tc.in.AccountID, tc.in.TokenFamilyID = contractPushAcct, NamedID("family")
		if _, err := e.svc.Subscribe(t.Context(), tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: Subscribe = %v, want %v", tc.name, err, tc.want)
		}
	}
	if _, ok := e.ofFamily(t, NamedID("family")); ok {
		t.Error("an invalid subscription was stored")
	}
}

func pushJobs(t *testing.T, e *pushEnv) {
	handler, _, ok := e.jobs.Lookup(appnotification.JobPush)
	if !ok {
		t.Fatal("no handler of JobPush")
	}
	sub := e.subscribe(t, NamedID("family-j"), "https://push.example.net/j", notification.TypeMention)
	run := func(subID string, typ notification.Type) queue.Result {
		t.Helper()
		payload, _ := json.Marshal(map[string]string{
			"subscription_id": subID, "notification_id": "01890a5d-ac96-774b-bcce-b302099a8059", "account_id": contractPushAcct,
			"type": string(typ), "source_account_id": "01890a5d-ac96-774b-bcce-b302099a805a", "status_id": "01890a5d-ac96-774b-bcce-b302099a805b",
		})
		return handler(t.Context(), queue.Job{ID: NamedID("job"), Type: appnotification.JobPush, Payload: payload, Attempt: 1})
	}

	if r := run(sub.ID, notification.TypeMention); !r.IsDone() {
		t.Fatalf("push = %+v", r)
	}
	if r := run(sub.ID, notification.TypeMention); !r.IsDone() {
		t.Fatalf("push again = %+v", r)
	}
	sent := e.sender.sent()
	if len(sent) != 2 || sent[0].Topic != "01890a5dac96774bbcceb302099a8059" || sent[0].Topic != sent[1].Topic || sent[0].TTL != appnotification.PushTTL {
		t.Fatalf("sent = %+v, want two messages with the notification's topic", sent)
	}
	var payload appnotification.PushPayload
	if err := json.Unmarshal(sent[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Type != notification.TypeMention || payload.Title == "" || payload.Path != "/@carol@remote.example/status/01890a5d-ac96-774b-bcce-b302099a805b" {
		t.Errorf("payload = %+v", payload)
	}

	// A type that the subscription does not take sends nothing.
	if r := run(sub.ID, notification.TypeFollow); !r.IsDone() || len(e.sender.sent()) != 2 {
		t.Errorf("follow = %+v, %d sent", r, len(e.sender.sent()))
	}

	// A temporary failure is retried; the push service's Retry-After wins.
	e.sender.set(appnotification.PushRetry, time.Minute)
	if r := run(sub.ID, notification.TypeMention); !r.IsRetry() || r.After() != time.Minute {
		t.Errorf("retry = %+v", r)
	}

	// The push service reports the subscription gone: it is deleted, and the job is done.
	e.sender.set(appnotification.PushGone, 0)
	if r := run(sub.ID, notification.TypeMention); !r.IsDone() {
		t.Errorf("gone = %+v", r)
	}
	if _, ok := e.ofFamily(t, NamedID("family-j")); ok {
		t.Error("a gone subscription remains")
	}
	n := len(e.sender.sent())
	if r := run(sub.ID, notification.TypeMention); !r.IsDone() || len(e.sender.sent()) != n {
		t.Errorf("push to a deleted subscription = %+v, %d sent", r, len(e.sender.sent())-n)
	}

	// A grant that ended: its subscription is deleted without a message.
	e.sender.set(appnotification.PushDelivered, 0)
	ended := e.subscribe(t, NamedID("family-ended"), "https://push.example.net/ended", notification.TypeMention)
	e.audience.end(NamedID("family-ended"))
	n = len(e.sender.sent())
	if r := run(ended.ID, notification.TypeMention); !r.IsDone() || len(e.sender.sent()) != n {
		t.Errorf("push to an ended grant = %+v, %d sent", r, len(e.sender.sent())-n)
	}
	if _, ok := e.ofFamily(t, NamedID("family-ended")); ok {
		t.Error("the subscription of an ended grant remains")
	}
}

type stubPushSender struct {
	mu      sync.Mutex
	outcome appnotification.PushOutcome
	after   time.Duration
	msgs    []appnotification.PushMessage
}

func (s *stubPushSender) set(o appnotification.PushOutcome, after time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcome, s.after = o, after
}

func (s *stubPushSender) sent() []appnotification.PushMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]appnotification.PushMessage(nil), s.msgs...)
}

func (s *stubPushSender) ServerKey() string { return contractPushP256DH }

func (s *stubPushSender) Send(_ context.Context, _ notification.PushSubscription, m appnotification.PushMessage) (appnotification.PushResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, m)
	return appnotification.PushResult{Outcome: s.outcome, StatusCode: 201, RetryAfter: s.after}, nil
}

type stubPushAudience struct {
	mu    sync.Mutex
	ended map[string]bool
}

func (a *stubPushAudience) end(family string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ended[family] = true
}

func (a *stubPushAudience) GrantActive(_ context.Context, family string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.ended[family], nil
}

func (a *stubPushAudience) Locale(context.Context, string) (string, error) { return "en", nil }

func (a *stubPushAudience) Account(context.Context, string) (appnotification.PushAccount, error) {
	return appnotification.PushAccount{DisplayName: "Carol", Acct: "carol@remote.example"}, nil
}

// StatusVisible shows every status: the recheck of visibility is the push job's own test.
func (a *stubPushAudience) StatusVisible(context.Context, string, string) (bool, error) {
	return true, nil
}
