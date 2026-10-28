// SPDX-License-Identifier: AGPL-3.0-only

package pubsub_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/v2/pstest"
	tcpubsub "github.com/testcontainers/testcontainers-go/modules/gcloud/pubsub"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pubsub "github.com/mycroblog/my/internal/adapter/stream/pubsub"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/pkg/testkit"
)

// The environment variables of the emulator tests: TEST_STREAM=container runs the contract on
// the Pub/Sub emulator in a container of TEST_PUBSUB_IMAGE (default emulatorImage) too, and
// TEST_PUBSUB_REQUIRED makes a missing Docker fail the tests instead of skipping them. CI sets
// both on the pushes to main and nightly (scripts/task.sh go:test-stream); pull requests run the contract
// on pstest only.
const (
	envStream   = "TEST_STREAM"
	envImage    = "TEST_PUBSUB_IMAGE"
	envRequired = "TEST_PUBSUB_REQUIRED"
)

// emulatorImage is the Pub/Sub emulator of the tests: a google-cloud-cli release at least three
// days old.
const emulatorImage = "gcr.io/google.com/cloudsdktool/google-cloud-cli:587.0.0-emulators"

const project = "test-project"

// pstestStreamTimeout ends every streaming pull of pstest after it, as the service ends streams
// from time to time; the client reopens them, and a stream reopened on a deleted subscription
// fails, as on the service. pstest does not end the open streams of a subscription it deletes.
const pstestStreamTimeout = 500 * time.Millisecond

func TestMain(m *testing.M) {
	code := m.Run()
	if emulator.stop != nil {
		emulator.stop()
	}
	os.Exit(code)
}

func TestBusContractPstest(t *testing.T) {
	testkit.StreamBusContract(t, func(t *testing.T) testkit.StreamBus {
		srv := pstest.NewServer()
		srv.SetStreamTimeout(pstestStreamTimeout)
		t.Cleanup(func() { _ = srv.Close() })
		return harness(t, srv.Addr)
	})
}

func TestBusContractEmulator(t *testing.T) {
	addr := emulatorAddr(t)
	testkit.StreamBusContract(t, func(t *testing.T) testkit.StreamBus { return harness(t, addr) })
}

func TestSweeperContractPstest(t *testing.T) {
	testkit.StreamSweeperContract(t, func(t *testing.T) testkit.StreamSweeper {
		srv := pstest.NewServer()
		srv.SetStreamTimeout(pstestStreamTimeout)
		t.Cleanup(func() { _ = srv.Close() })
		return sweeperHarness(t, srv.Addr)
	})
}

func TestSweeperContractEmulator(t *testing.T) {
	addr := emulatorAddr(t)
	testkit.StreamSweeperContract(t, func(t *testing.T) testkit.StreamSweeper { return sweeperHarness(t, addr) })
}

// sweeperHarness is the harness of the sweeper contract: a process's resources are its
// subscriptions, labeled with their creation time by the harness's clock.
func sweeperHarness(t *testing.T, addr string) testkit.StreamSweeper {
	t.Helper()
	b := newBroker(t, addr)
	clock := testkit.NewClock(time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	return testkit.StreamSweeper{
		NewBus: func(t *testing.T) (testkit.SweepingBus, string) {
			bus := b.bus(t, addr, clock.Now)
			return bus, bus.ProcessID()
		},
		Resources: func(t *testing.T, processID string) int {
			n := 0
			for _, name := range b.subscriptions(t) {
				if strings.Contains(name, processID) {
					n++
				}
			}
			return n
		},
		Advance: func(d time.Duration) { clock.Advance(d) },
		Timeout: 10 * time.Second,
	}
}

func TestStreamBrokerContractRelay(t *testing.T) {
	testkit.StreamBrokerContract(t, func(t *testing.T, o stream.HubOptions) testkit.StreamBroker {
		srv := pstest.NewServer()
		srv.SetStreamTimeout(pstestStreamTimeout)
		t.Cleanup(func() { _ = srv.Close() })
		h := harness(t, srv.Addr)
		local := stream.NewRelay(h.NewBus(t), stream.RelayOptions{Hub: o})
		remote := stream.NewRelay(h.NewBus(t), stream.RelayOptions{})
		for _, r := range []*stream.Relay{local, remote} {
			t.Cleanup(func() {
				if err := r.Close(context.Background()); err != nil {
					t.Errorf("Close: %v", err)
				}
			})
		}
		return testkit.StreamBroker{Publisher: local, Subscriber: local, Remote: remote, Break: h.Break}
	})
}

// broker is one test's topic and subscription prefix on a Pub/Sub server at addr.
type broker struct {
	addr   string
	topic  string
	prefix string
}

func newBroker(t *testing.T, addr string) broker {
	t.Helper()
	b := broker{addr: addr, topic: "projects/" + project + "/topics/t-" + random(), prefix: "s-" + random()}
	c := b.admin(t)
	if _, err := c.Client().TopicAdminClient.CreateTopic(t.Context(), &pubsubpb.Topic{Name: b.topic}); err != nil {
		t.Fatalf("create topic: %v", err)
	}
	return b
}

// options returns the client options of a server at addr, without credentials. One connection
// per client instead of the default pool of up to four keeps the tests' loopback ports few.
func options(addr string) []option.ClientOption {
	return []option.ClientOption{
		option.WithEndpoint(addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithGRPCConnectionPool(1),
	}
}

func (b broker) bus(t *testing.T, addr string, now func() time.Time) *pubsub.Bus {
	t.Helper()
	bus, err := pubsub.New(t.Context(), pubsub.Options{
		Topic: b.topic, SubscriptionPrefix: b.prefix, ProcessID: stream.NewProcessID(),
		ClientOptions: options(addr), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bus.Close() })
	return bus
}

// admin returns a Bus whose client manages the broker's topic and subscriptions.
func (b broker) admin(t *testing.T) *pubsub.Bus {
	t.Helper()
	return b.bus(t, b.addr, nil)
}

// subscriptions returns the names of the broker's subscriptions.
func (b broker) subscriptions(t *testing.T) []string {
	t.Helper()
	it := b.admin(t).Client().SubscriptionAdminClient.ListSubscriptions(t.Context(), &pubsubpb.ListSubscriptionsRequest{Project: "projects/" + project})
	var names []string
	for {
		s, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return names
		}
		if err != nil {
			t.Fatalf("list subscriptions: %v", err)
		}
		if s.GetTopic() == b.topic {
			names = append(names, s.GetName())
		}
	}
}

func harness(t *testing.T, addr string) testkit.StreamBus {
	t.Helper()
	b := newBroker(t, addr)
	return testkit.StreamBus{
		NewBus: func(t *testing.T) stream.Bus { return b.bus(t, addr, nil) },
		Unreachable: func(t *testing.T) stream.Bus {
			return b.bus(t, closedAddr(t), nil)
		},
		// Deleting the subscriptions cuts their listeners, as their expiration would.
		Break: func(t *testing.T) {
			c := b.admin(t).Client()
			for _, name := range b.subscriptions(t) {
				if err := c.SubscriptionAdminClient.DeleteSubscription(t.Context(), &pubsubpb.DeleteSubscriptionRequest{Subscription: name}); err != nil {
					t.Fatalf("delete subscription: %v", err)
				}
			}
		},
		Resources: func(t *testing.T) int { return len(b.subscriptions(t)) },
		Timeout:   10 * time.Second,
	}
}

// closedAddr returns a local address where nothing listens.
func closedAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

func random() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// The subscription of Listen has the properties of ADR-0050 §2.
func TestListenCreatesTheSubscription(t *testing.T) {
	srv := pstest.NewServer()
	t.Cleanup(func() { _ = srv.Close() })
	b := newBroker(t, srv.Addr)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	bus := b.bus(t, srv.Addr, func() time.Time { return now })
	l, err := bus.Listen(t.Context(), func([]byte) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close(context.Background()) })
	s, err := bus.Client().SubscriptionAdminClient.GetSubscription(t.Context(), &pubsubpb.GetSubscriptionRequest{Subscription: pubsub.SubscriptionOf(l)})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := s.GetName(), "projects/"+project+"/subscriptions/"+b.prefix+"-"+bus.ProcessID()+"-1"; got != want {
		t.Errorf("name = %s, want %s", got, want)
	}
	if got := s.GetExpirationPolicy().GetTtl().AsDuration(); got != 24*time.Hour {
		t.Errorf("expiration TTL = %s, want 24h", got)
	}
	if got := s.GetMessageRetentionDuration().AsDuration(); got != 10*time.Minute {
		t.Errorf("message retention = %s, want 10m", got)
	}
	if got := s.GetAckDeadlineSeconds(); got != 10 {
		t.Errorf("ack deadline = %ds, want 10s", got)
	}
	if got, want := s.GetLabels(), map[string]string{"my-stream-process": bus.ProcessID(), "my-stream-created": fmt.Sprint(now.Unix())}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("labels = %v, want %v", got, want)
	}
}

// Sweep deletes the old subscriptions of the prefix whose process is not alive, and keeps the
// rest: new ones, live ones, other prefixes, and ones without the labels of Listen.
func TestSweep(t *testing.T) {
	srv := pstest.NewServer()
	t.Cleanup(func() { _ = srv.Close() })
	b := newBroker(t, srv.Addr)
	ctx := t.Context()
	start := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	now := start
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }

	listen := func(bus *pubsub.Bus) string {
		t.Helper()
		l, err := bus.Listen(ctx, func([]byte) {})
		if err != nil {
			t.Fatal(err)
		}
		return pubsub.SubscriptionOf(l)
	}
	dead, live := b.bus(t, srv.Addr, clock), b.bus(t, srv.Addr, clock)
	listen(dead)
	liveSub := listen(live)
	other := newBroker(t, srv.Addr)
	otherDead := other.bus(t, srv.Addr, clock)
	otherDeadSub := listen(otherDead)
	// A subscription of the prefix that Listen did not create.
	unlabeled := "projects/" + project + "/subscriptions/" + b.prefix + "-" + stream.NewProcessID() + "-1"
	if _, err := dead.Client().SubscriptionAdminClient.CreateSubscription(ctx, &pubsubpb.Subscription{Name: unlabeled, Topic: b.topic}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	now = start.Add(4 * time.Minute)
	mu.Unlock()
	young := b.bus(t, srv.Addr, clock)
	youngSub := listen(young)
	mu.Lock()
	now = start.Add(5 * time.Minute)
	mu.Unlock()

	alive := func(_ context.Context, processID string) (bool, error) { return processID == live.ProcessID(), nil }
	n, err := dead.Sweep(ctx, 5*time.Minute, alive)
	if err != nil || n != 1 {
		t.Fatalf("Sweep = %d, %v; want 1, nil", n, err)
	}
	want := map[string]bool{liveSub: true, youngSub: true, unlabeled: true}
	got := b.subscriptions(t)
	if len(got) != len(want) {
		t.Errorf("subscriptions = %v, want %v", got, want)
	}
	for _, name := range got {
		if !want[name] {
			t.Errorf("subscription %s was kept", name)
		}
	}
	if got := other.subscriptions(t); len(got) != 1 || got[0] != otherDeadSub {
		t.Errorf("subscriptions of another prefix = %v, want [%s]", got, otherDeadSub)
	}

	// A failing liveness check keeps the subscription and reports the error.
	failing := func(context.Context, string) (bool, error) { return false, errors.New("kv down") }
	mu.Lock()
	now = start.Add(time.Hour)
	mu.Unlock()
	if n, err := dead.Sweep(ctx, 5*time.Minute, failing); err == nil || n != 0 {
		t.Errorf("Sweep with a failing liveness check = %d, %v; want 0 and an error", n, err)
	}
	if got := b.subscriptions(t); len(got) != len(want) {
		t.Errorf("subscriptions after a failing sweep = %v", got)
	}
}

func TestNewRejectsBadOptions(t *testing.T) {
	ok := pubsub.Options{Topic: "projects/p/topics/stream", SubscriptionPrefix: "my-stream", ProcessID: stream.NewProcessID(), ClientOptions: options("127.0.0.1:1")}
	for name, edit := range map[string]func(*pubsub.Options){
		"topic without project": func(o *pubsub.Options) { o.Topic = "stream" },
		"topic goog":            func(o *pubsub.Options) { o.Topic = "projects/p/topics/googstream" },
		"empty prefix":          func(o *pubsub.Options) { o.SubscriptionPrefix = "" },
		"prefix with slash":     func(o *pubsub.Options) { o.SubscriptionPrefix = "a/b" },
		"prefix with digit":     func(o *pubsub.Options) { o.SubscriptionPrefix = "1a" },
		"process ID":            func(o *pubsub.Options) { o.ProcessID = "NOT-HEX" },
	} {
		t.Run(name, func(t *testing.T) {
			o := ok
			edit(&o)
			if b, err := pubsub.New(t.Context(), o); err == nil {
				_ = b.Close()
				t.Error("New succeeded")
			}
		})
	}
	b, err := pubsub.New(t.Context(), ok)
	if err != nil {
		t.Fatal(err)
	}
	_ = b.Close()
}

// The late Close of a broken listener does not delete the subscription of the next listener of
// the process.
func TestLateCloseKeepsTheNextListener(t *testing.T) {
	srv := pstest.NewServer()
	srv.SetStreamTimeout(pstestStreamTimeout)
	t.Cleanup(func() { _ = srv.Close() })
	h := harness(t, srv.Addr)
	bus := h.NewBus(t)
	broken, err := bus.Listen(t.Context(), func([]byte) {})
	if err != nil {
		t.Fatal(err)
	}
	h.Break(t)
	select {
	case <-broken.Broken():
	case <-time.After(h.Timeout):
		t.Fatal("the listener did not break")
	}
	got := make(chan []byte, 1)
	next, err := bus.Listen(t.Context(), func(p []byte) { got <- p })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = next.Close(context.Background()) })
	if err := broken.Close(t.Context()); err != nil {
		t.Fatalf("Close of the broken listener: %v", err)
	}
	if err := bus.Send(t.Context(), []byte(`{"v":1}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-got:
	case <-time.After(h.Timeout):
		t.Fatal("the next listener did not receive after the late Close")
	}
}

var emulator struct {
	once sync.Once
	addr string
	skip string
	err  error
	stop func()
}

// emulatorAddr returns the address of the emulator container, started on the first call, or
// skips t when TEST_STREAM is not container or Docker is unavailable.
func emulatorAddr(t *testing.T) string {
	t.Helper()
	if os.Getenv(envStream) != "container" {
		t.Skipf("%s=container runs the contract on the Pub/Sub emulator (scripts/task.sh go:test-stream)", envStream)
	}
	emulator.once.Do(func() {
		if err := exec.Command("docker", "info").Run(); err != nil {
			reason := fmt.Sprintf("Docker is unavailable for the Pub/Sub emulator (%v)", err)
			if os.Getenv(envRequired) != "" {
				emulator.err = fmt.Errorf("%s, but %s is set", reason, envRequired)
				return
			}
			emulator.skip = reason
			return
		}
		image := os.Getenv(envImage)
		if image == "" {
			image = emulatorImage
		}
		c, err := tcpubsub.Run(context.Background(), image, tcpubsub.WithProjectID(project))
		if c != nil {
			emulator.stop = func() { _ = c.Terminate(context.Background()) }
		}
		if err != nil {
			emulator.err = fmt.Errorf("start container %s: %w", image, err)
			return
		}
		emulator.addr = c.URI()
	})
	if emulator.err != nil {
		t.Fatal(emulator.err)
	}
	if emulator.addr == "" {
		t.Skip(emulator.skip)
	}
	return emulator.addr
}
