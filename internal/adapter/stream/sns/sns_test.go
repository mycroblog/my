// SPDX-License-Identifier: AGPL-3.0-only

package sns_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssns "github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/mycroblog/my/internal/adapter/stream/sns"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/awstest"
)

// TestMain stops the emulator container of package awstest after the tests.
func TestMain(m *testing.M) {
	os.Exit(awstest.Main(m))
}

// snsAdmin is the SNS client of a test: the Bus's operations and the topic's.
type snsAdmin interface {
	sns.TopicClient
	CreateTopic(ctx context.Context, in *awssns.CreateTopicInput, optFns ...func(*awssns.Options)) (*awssns.CreateTopicOutput, error)
	DeleteTopic(ctx context.Context, in *awssns.DeleteTopicInput, optFns ...func(*awssns.Options)) (*awssns.DeleteTopicOutput, error)
}

// broker is one test's topic and queue prefix on the fake or the emulator.
type broker struct {
	sns    snsAdmin
	sqs    sns.QueueClient
	topic  string
	prefix string
	// clock is the fake's; nil on the emulator, whose time cannot be moved.
	clock *testkit.Clock
	fake  *awstest.Fake
	// unreachable returns a Bus whose endpoint does not answer.
	unreachable func(t *testing.T) *sns.Bus
	timeout     time.Duration
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func newTopic(t *testing.T, c snsAdmin) string {
	t.Helper()
	out, err := c.CreateTopic(t.Context(), &awssns.CreateTopicInput{Name: aws.String("stream-" + randomHex(6))})
	if err != nil {
		t.Fatalf("CreateTopic: %v", err)
	}
	arn := aws.ToString(out.TopicArn)
	t.Cleanup(func() {
		_, _ = c.DeleteTopic(context.Background(), &awssns.DeleteTopicInput{TopicArn: aws.String(arn)})
	})
	return arn
}

// fakeBroker returns a broker on a new Fake.
func fakeBroker(t *testing.T) *broker {
	t.Helper()
	clock := testkit.NewClock(time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	f := awstest.NewFake(clock)
	b := &broker{sns: f.SNS(), sqs: f.SQS(), prefix: "test", clock: clock, fake: f, timeout: 5 * time.Second}
	b.topic = newTopic(t, b.sns)
	b.unreachable = func(t *testing.T) *sns.Bus {
		u := awstest.NewFake(nil)
		topic := newTopic(t, u.SNS())
		u.SetUnreachable(true)
		t.Cleanup(func() { u.SetUnreachable(false) }) // before the topic's cleanup
		bus, err := sns.New(u.SNS(), u.SQS(), sns.Options{TopicARN: topic, QueuePrefix: "test"})
		if err != nil {
			t.Fatal(err)
		}
		return bus
	}
	return b
}

// emulatorBroker returns a broker on the emulator, or skips t.
func emulatorBroker(t *testing.T) *broker {
	t.Helper()
	cfg := awstest.Emulator(t)
	b := &broker{
		sns: awssns.NewFromConfig(cfg), sqs: sqs.NewFromConfig(cfg),
		prefix: "t" + randomHex(4), timeout: 30 * time.Second,
	}
	b.topic = newTopic(t, b.sns)
	b.unreachable = func(t *testing.T) *sns.Bus {
		bus, err := sns.NewFromConfig(awstest.Config("http://127.0.0.1:1"), sns.Options{TopicARN: b.topic, QueuePrefix: b.prefix})
		if err != nil {
			t.Fatal(err)
		}
		return bus
	}
	return b
}

// brokers runs test on the fake and on the emulator.
func brokers(t *testing.T, test func(t *testing.T, b *broker)) {
	t.Run("Fake", func(t *testing.T) { test(t, fakeBroker(t)) })
	t.Run("Emulator", func(t *testing.T) { test(t, emulatorBroker(t)) })
}

// newBus returns a Bus of a new process on b.
func (b *broker) newBus(t *testing.T) *sns.Bus {
	t.Helper()
	o := sns.Options{TopicARN: b.topic, QueuePrefix: b.prefix}
	if b.clock != nil {
		o.Clock = b.clock
	}
	bus, err := sns.New(b.sns, b.sqs, o)
	if err != nil {
		t.Fatal(err)
	}
	return bus
}

// queues returns the URLs of the queues under the prefix.
func (b *broker) queues(t *testing.T) []string {
	t.Helper()
	out, err := b.sqs.ListQueues(t.Context(), &sqs.ListQueuesInput{QueueNamePrefix: aws.String(b.prefix + "-")})
	if err != nil {
		t.Fatalf("ListQueues: %v", err)
	}
	return out.QueueUrls
}

// subscriptions returns the endpoints of the topic's subscriptions.
func (b *broker) subscriptions(t *testing.T) []string {
	t.Helper()
	out, err := b.sns.ListSubscriptionsByTopic(t.Context(), &awssns.ListSubscriptionsByTopicInput{TopicArn: aws.String(b.topic)})
	if err != nil {
		t.Fatalf("ListSubscriptionsByTopic: %v", err)
	}
	var endpoints []string
	for _, s := range out.Subscriptions {
		endpoints = append(endpoints, aws.ToString(s.Endpoint))
	}
	return endpoints
}

// deleteQueues deletes every queue under the prefix, as a sweep would.
func (b *broker) deleteQueues(t *testing.T) {
	t.Helper()
	for _, url := range b.queues(t) {
		if _, err := b.sqs.DeleteQueue(t.Context(), &sqs.DeleteQueueInput{QueueUrl: aws.String(url)}); err != nil {
			t.Fatalf("DeleteQueue: %v", err)
		}
	}
}

func (b *broker) harness() testkit.StreamBus {
	return testkit.StreamBus{
		NewBus:      func(t *testing.T) stream.Bus { return b.newBus(t) },
		Unreachable: func(t *testing.T) stream.Bus { return b.unreachable(t) },
		Break:       b.deleteQueues,
		Resources:   func(t *testing.T) int { return len(b.queues(t)) },
		Timeout:     b.timeout,
	}
}

func TestStreamBusContract(t *testing.T) {
	testkit.StreamBusContract(t, func(t *testing.T) testkit.StreamBus { return fakeBroker(t).harness() })
}

func TestStreamBusContractEmulator(t *testing.T) {
	testkit.StreamBusContract(t, func(t *testing.T) testkit.StreamBus { return emulatorBroker(t).harness() })
}

// sweeper is the harness of the sweeper contract: a process's resources are its queue and
// its subscription.
func (b *broker) sweeper() testkit.StreamSweeper {
	h := testkit.StreamSweeper{
		NewBus: func(t *testing.T) (testkit.SweepingBus, string) {
			bus := b.newBus(t)
			return bus, bus.ProcessID()
		},
		Resources: func(t *testing.T, processID string) int {
			n := 0
			for _, name := range append(b.queues(t), b.subscriptions(t)...) {
				if strings.Contains(name, processID) {
					n++
				}
			}
			return n
		},
		Timeout: b.timeout,
	}
	if b.clock != nil {
		h.Advance = func(d time.Duration) { b.clock.Advance(d) }
	}
	return h
}

func TestStreamSweeperContract(t *testing.T) {
	testkit.StreamSweeperContract(t, func(t *testing.T) testkit.StreamSweeper { return fakeBroker(t).sweeper() })
}

func TestStreamSweeperContractEmulator(t *testing.T) {
	testkit.StreamSweeperContract(t, func(t *testing.T) testkit.StreamSweeper { return emulatorBroker(t).sweeper() })
}

func TestStreamBrokerContractRelay(t *testing.T) {
	testkit.StreamBrokerContract(t, func(t *testing.T, o stream.HubOptions) testkit.StreamBroker {
		b := fakeBroker(t)
		local := stream.NewRelay(b.newBus(t), stream.RelayOptions{Hub: o})
		remote := stream.NewRelay(b.newBus(t), stream.RelayOptions{})
		for _, r := range []*stream.Relay{local, remote} {
			t.Cleanup(func() {
				if err := r.Close(context.Background()); err != nil {
					t.Errorf("Close: %v", err)
				}
			})
		}
		return testkit.StreamBroker{
			Publisher: local, Subscriber: local, Remote: remote,
			Break: b.deleteQueues,
		}
	})
}

func listen(t *testing.T, b *broker, bus *sns.Bus, receive func([]byte)) stream.Listener {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), b.timeout)
	defer cancel()
	l, err := bus.Listen(ctx, receive)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
		defer cancel()
		_ = l.Close(ctx)
	})
	return l
}

// The queue of a listener keeps messages for 60 seconds, long-polls for 20, carries the tag of
// its process, accepts messages from the topic only, and is subscribed with raw delivery; Close
// removes the subscription with the queue.
func TestListenQueue(t *testing.T) {
	brokers(t, func(t *testing.T, b *broker) {
		bus := b.newBus(t)
		got := make(chan string, 1)
		l := listen(t, b, bus, func(p []byte) { got <- string(p) })
		urls := b.queues(t)
		if len(urls) != 1 {
			t.Fatalf("queues = %v, want one", urls)
		}
		name := urls[0][strings.LastIndexByte(urls[0], '/')+1:]
		if want := b.prefix + "-" + bus.ProcessID() + "-1"; name != want {
			t.Errorf("queue name = %q, want %q", name, want)
		}
		out, err := b.sqs.GetQueueAttributes(t.Context(), &sqs.GetQueueAttributesInput{
			QueueUrl: aws.String(urls[0]), AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameAll},
		})
		if err != nil {
			t.Fatal(err)
		}
		a := out.Attributes
		if a["MessageRetentionPeriod"] != "60" || a["ReceiveMessageWaitTimeSeconds"] != "20" {
			t.Errorf("retention, wait = %s, %s; want 60, 20", a["MessageRetentionPeriod"], a["ReceiveMessageWaitTimeSeconds"])
		}
		var policy struct {
			Statement []struct {
				Effect    string
				Principal map[string]string
				Action    string
				Resource  string
				Condition map[string]map[string]string
			}
		}
		if err := json.Unmarshal([]byte(a["Policy"]), &policy); err != nil || len(policy.Statement) != 1 {
			t.Fatalf("policy %q: %v", a["Policy"], err)
		}
		s := policy.Statement[0]
		if s.Effect != "Allow" || s.Principal["Service"] != "sns.amazonaws.com" || s.Action != "sqs:SendMessage" ||
			s.Resource != a["QueueArn"] || s.Condition["ArnEquals"]["aws:SourceArn"] != b.topic {
			t.Errorf("policy = %+v; want SendMessage for sns.amazonaws.com from %s on %s", s, b.topic, a["QueueArn"])
		}
		if b.fake != nil {
			if tags := b.fake.QueueTags(name); tags[sns.QueueTag] != bus.ProcessID() {
				t.Errorf("tags = %v, want %s = the process ID", tags, sns.QueueTag)
			}
		}
		if subs := b.subscriptions(t); len(subs) != 1 || subs[0] != a["QueueArn"] {
			t.Errorf("subscriptions = %v, want the queue %s", subs, a["QueueArn"])
		}

		// Raw delivery: the payload arrives unchanged, and the received message is deleted.
		p := `{"v":1,"o":"x","k":"home_entry","e":"0199f0a8-0000-7000-8000-000000000001"}`
		if err := bus.Send(t.Context(), []byte(p)); err != nil {
			t.Fatal(err)
		}
		select {
		case m := <-got:
			if m != p {
				t.Errorf("received %q, want %q", m, p)
			}
		case <-time.After(b.timeout):
			t.Fatal("the payload did not arrive")
		}

		ctx, cancel := context.WithTimeout(t.Context(), b.timeout)
		defer cancel()
		if err := l.Close(ctx); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if subs := b.subscriptions(t); len(subs) != 0 {
			t.Errorf("subscriptions after Close = %v, want none", subs)
		}
	})
}

// A process that listens again after its listener broke takes a new queue name, since SQS
// refuses the name of a queue deleted less than 60 seconds ago.
func TestListenAgainTakesANewQueue(t *testing.T) {
	brokers(t, func(t *testing.T, b *broker) {
		bus := b.newBus(t)
		l := listen(t, b, bus, func([]byte) {})
		ctx, cancel := context.WithTimeout(t.Context(), b.timeout)
		defer cancel()
		if err := l.Close(ctx); err != nil {
			t.Fatal(err)
		}
		listen(t, b, bus, func([]byte) {})
		urls := b.queues(t)
		if len(urls) != 1 || !strings.HasSuffix(urls[0], "-"+bus.ProcessID()+"-2") {
			t.Errorf("queues = %v, want the second queue of the process", urls)
		}
	})
}

// When the subscription fails, Listen deletes the queue it created.
func TestListenDeletesTheQueueWhenSubscribeFails(t *testing.T) {
	brokers(t, func(t *testing.T, b *broker) {
		missing := b.topic + "-missing"
		bus, err := sns.New(b.sns, b.sqs, sns.Options{TopicARN: missing, QueuePrefix: b.prefix})
		if err != nil {
			t.Fatal(err)
		}
		if l, err := bus.Listen(t.Context(), func([]byte) {}); err == nil {
			_ = l.Close(t.Context())
			t.Fatal("Listen on a missing topic succeeded")
		}
		if urls := b.queues(t); len(urls) != 0 {
			t.Errorf("queues = %v, want none", urls)
		}
	})
}

// stalledSubscribe fails Subscribe only when its context ends.
type stalledSubscribe struct{ sns.TopicClient }

func (stalledSubscribe) Subscribe(ctx context.Context, _ *awssns.SubscribeInput, _ ...func(*awssns.Options)) (*awssns.SubscribeOutput, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// slowDeleteQueue takes delay for each DeleteQueue.
type slowDeleteQueue struct {
	sns.QueueClient
	delay time.Duration
}

func (c slowDeleteQueue) DeleteQueue(ctx context.Context, in *sqs.DeleteQueueInput, optFns ...func(*sqs.Options)) (*sqs.DeleteQueueOutput, error) {
	select {
	case <-time.After(c.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return c.QueueClient.DeleteQueue(ctx, in, optFns...)
}

// A Listen whose subscription fails at its deadline returns by that deadline, and deletes the
// queue it created afterwards.
func TestListenReturnsByItsDeadlineWhenSubscribeFails(t *testing.T) {
	b := fakeBroker(t)
	const deadline, deleteDelay = 50 * time.Millisecond, 500 * time.Millisecond
	bus, err := sns.New(stalledSubscribe{b.sns}, slowDeleteQueue{b.sqs, deleteDelay}, sns.Options{TopicARN: b.topic, QueuePrefix: b.prefix})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), deadline)
	defer cancel()
	start := time.Now()
	if l, err := bus.Listen(ctx, func([]byte) {}); err == nil {
		_ = l.Close(t.Context())
		t.Fatal("Listen with a stalled subscription succeeded")
	}
	if took := time.Since(start); took >= deleteDelay {
		t.Errorf("Listen took %s, past its deadline %s", took, deadline)
	}
	for wait := time.Now().Add(5 * time.Second); len(b.queues(t)) != 0; {
		if time.Now().After(wait) {
			t.Fatalf("queues = %v, want none after the cleanup", b.queues(t))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A listener whose receives keep failing breaks.
func TestListenerBreaksWhenReceivingKeepsFailing(t *testing.T) {
	t.Cleanup(sns.SetReceiveWaits(200*time.Millisecond, time.Second, 50*time.Millisecond))
	b := fakeBroker(t)
	l := listen(t, b, b.newBus(t), func([]byte) {})
	b.fake.SetUnreachable(true)
	select {
	case <-l.Broken():
	case <-time.After(5 * time.Second):
		t.Fatal("the listener did not break")
	}
	b.fake.SetUnreachable(false)
}

func TestNewChecksOptions(t *testing.T) {
	f := awstest.NewFake(nil)
	topic := "arn:aws:sns:us-east-1:000000000000:stream"
	for name, o := range map[string]sns.Options{
		"no topic":            {QueuePrefix: "my"},
		"queue ARN":           {TopicARN: "arn:aws:sqs:us-east-1:000000000000:stream", QueuePrefix: "my"},
		"no prefix":           {TopicARN: topic},
		"prefix with a colon": {TopicARN: topic, QueuePrefix: "my:stream"},
		"long prefix":         {TopicARN: topic, QueuePrefix: strings.Repeat("a", sns.MaxQueuePrefix+1)},
		"bad process ID":      {TopicARN: topic, QueuePrefix: "my", ProcessID: "p1"},
	} {
		if _, err := sns.New(f.SNS(), f.SQS(), o); err == nil {
			t.Errorf("%s: New succeeded", name)
		}
	}
	if _, err := sns.New(f.SNS(), f.SQS(), sns.Options{TopicARN: topic, QueuePrefix: strings.Repeat("a", sns.MaxQueuePrefix)}); err != nil {
		t.Errorf("New with the longest prefix: %v", err)
	}
}

// The sweep deletes the queues and subscriptions of ended processes and keeps the ones of live
// processes, of processes it cannot tell, and of queues younger than minAge.
func TestSweep(t *testing.T) {
	brokers(t, func(t *testing.T, b *broker) {
		live, dead, orphan, unknown := b.newBus(t), b.newBus(t), b.newBus(t), b.newBus(t)
		for _, bus := range []*sns.Bus{live, unknown, orphan} {
			listen(t, b, bus, func([]byte) {})
		}
		deadListener := listen(t, b, dead, func([]byte) {})
		// The orphan's queue is gone and its subscription left behind.
		for _, url := range b.queues(t) {
			if strings.Contains(url, orphan.ProcessID()) {
				if _, err := b.sqs.DeleteQueue(t.Context(), &sqs.DeleteQueueInput{QueueUrl: aws.String(url)}); err != nil {
					t.Fatal(err)
				}
			}
		}
		minAge := time.Duration(0)
		var young *sns.Bus
		if b.clock != nil {
			minAge = 5 * time.Minute
			b.clock.Advance(minAge)
			young = b.newBus(t)
			listen(t, b, young, func([]byte) {})
		}
		errUnknown := errors.New("liveness store unavailable")
		alive := func(_ context.Context, id string) (bool, error) {
			switch id {
			case live.ProcessID():
				return true, nil
			case unknown.ProcessID():
				return false, errUnknown
			}
			return false, nil
		}
		sweeper := b.newBus(t)
		n, err := sweeper.Sweep(t.Context(), minAge, alive)
		if !errors.Is(err, errUnknown) {
			t.Errorf("Sweep error = %v, want the liveness error", err)
		}
		// dead: its queue and subscription; orphan: its subscription.
		if n != 3 {
			t.Errorf("Sweep deleted %d, want 3", n)
		}
		kept := []*sns.Bus{live, unknown}
		if young != nil {
			kept = append(kept, young)
		}
		urls := b.queues(t)
		if len(urls) != len(kept) {
			t.Errorf("queues after the sweep = %v, want those of %d processes", urls, len(kept))
		}
		subs := b.subscriptions(t)
		if len(subs) != len(kept) {
			t.Errorf("subscriptions after the sweep = %v, want those of %d processes", subs, len(kept))
		}
		for _, bus := range kept {
			if !containsProcess(urls, bus.ProcessID()) || !containsProcess(subs, bus.ProcessID()) {
				t.Errorf("the sweep deleted the resources of process %s", bus.ProcessID())
			}
		}
		select {
		case <-deadListener.Broken():
		case <-time.After(b.timeout):
			t.Error("the listener whose queue was swept did not break")
		}

		// Sweeping again deletes nothing more.
		if n, _ := sweeper.Sweep(t.Context(), minAge, alive); n != 0 {
			t.Errorf("second Sweep deleted %d, want 0", n)
		}
	})
}

func containsProcess(names []string, processID string) bool {
	for _, n := range names {
		if strings.Contains(n, processID) {
			return true
		}
	}
	return false
}
