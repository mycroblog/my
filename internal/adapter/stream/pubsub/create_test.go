// SPDX-License-Identifier: AGPL-3.0-only

package pubsub_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/v2/pstest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pubsub "github.com/mycroblog/my/internal/adapter/stream/pubsub"
)

// failingCreate is a pstest reactor of CreateSubscription that fails while failing is set, and
// records the names of the subscriptions it was asked for.
type failingCreate struct {
	mu      sync.Mutex
	failing bool
	names   []string
}

func (f *failingCreate) React(req any) (bool, any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := req.(*pubsubpb.Subscription); ok {
		f.names = append(f.names, s.GetName())
	}
	if f.failing {
		return true, nil, status.Error(codes.Internal, "injected failure of CreateSubscription")
	}
	return false, nil, nil
}

func (f *failingCreate) setFailing(failing bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failing = failing
}

func (f *failingCreate) requested() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.names...)
}

// failedListen returns a pstest server whose first creation of a subscription fails, a Bus on
// it whose first Listen failed for that, and the name that Listen asked for.
func failedListen(t *testing.T) (*pstest.Server, broker, *pubsub.Bus, string) {
	t.Helper()
	create := &failingCreate{}
	srv := pstest.NewServer(pstest.ServerReactorOption{FuncName: "CreateSubscription", Reactor: create})
	srv.SetStreamTimeout(pstestStreamTimeout)
	t.Cleanup(func() { _ = srv.Close() })
	b := newBroker(t, srv.Addr)
	bus := b.bus(t, srv.Addr, nil)

	create.setFailing(true)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if l, err := bus.Listen(ctx, func([]byte) {}); err == nil {
		_ = l.Close(context.Background())
		t.Fatal("Listen succeeded while CreateSubscription fails")
	}
	create.setFailing(false)
	names := create.requested()
	if len(names) == 0 {
		t.Fatal("the failed Listen did not ask for a subscription")
	}
	for _, n := range names {
		if n != names[0] {
			t.Fatalf("the failed Listen asked for %v, want one name", names)
		}
	}
	if got := b.subscriptions(t); len(got) != 0 {
		t.Fatalf("subscriptions after the failed creation = %v, want none", got)
	}
	return srv, b, bus, names[0]
}

// After a Listen whose creation of the subscription failed, the next Listen creates the
// subscription under the same name, and receives from it.
func TestListenAfterAFailedCreationKeepsTheName(t *testing.T) {
	_, b, bus, name := failedListen(t)

	got := make(chan []byte, 1)
	l, err := bus.Listen(t.Context(), func(p []byte) { got <- p })
	if err != nil {
		t.Fatalf("Listen after the failed creation: %v", err)
	}
	t.Cleanup(func() { _ = l.Close(context.Background()) })
	if sub := pubsub.SubscriptionOf(l); sub != name {
		t.Errorf("subscription = %s, want %s, the name of the failed creation", sub, name)
	}
	if subs := b.subscriptions(t); len(subs) != 1 || subs[0] != name {
		t.Errorf("subscriptions = %v, want [%s]", subs, name)
	}
	if err := bus.Send(t.Context(), []byte(`{"v":1}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-got:
	case <-time.After(10 * time.Second):
		t.Fatal("the listener did not receive")
	}
}

// A creation that failed may have created the subscription all the same (its deadline ended
// first): the next Listen takes the subscription of that name (AlreadyExists counts as
// created), receives from it, and its Close deletes it.
func TestListenTakesTheSubscriptionOfAFailedCreation(t *testing.T) {
	_, b, bus, name := failedListen(t)
	// The subscription that the failed creation made on the service.
	if _, err := b.admin(t).Client().SubscriptionAdminClient.CreateSubscription(t.Context(), &pubsubpb.Subscription{Name: name, Topic: b.topic}); err != nil {
		t.Fatal(err)
	}

	got := make(chan []byte, 1)
	l, err := bus.Listen(t.Context(), func(p []byte) { got <- p })
	if err != nil {
		t.Fatalf("Listen with the subscription of the failed creation: %v, want it taken", err)
	}
	if sub := pubsub.SubscriptionOf(l); sub != name {
		t.Errorf("subscription = %s, want %s", sub, name)
	}
	if subs := b.subscriptions(t); len(subs) != 1 || subs[0] != name {
		t.Errorf("subscriptions = %v, want [%s]", subs, name)
	}
	if err := bus.Send(t.Context(), []byte(`{"v":1}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-got:
	case <-time.After(10 * time.Second):
		t.Fatal("the listener did not receive from the subscription it took")
	}
	if err := l.Close(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if subs := b.subscriptions(t); len(subs) != 0 {
		t.Errorf("subscriptions after Close = %v, want none", subs)
	}
}
