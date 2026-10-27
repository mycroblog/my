// SPDX-License-Identifier: AGPL-3.0-only

// Package pubsub implements stream.Bus and stream.Sweeper on Google Cloud Pub/Sub (ADR-0050 §2):
// the cross-process broker of the streams on Google Cloud (Cloud Run).
//
// A topic, which the deployment template creates, carries the payloads. A Pub/Sub subscription
// shares its messages among its subscribers, so every listening process gets a subscription of
// its own, <prefix>-<process id>-<n>, which Listen creates and Close deletes. n counts the
// listeners of the process, so that the late Close of a broken listener never deletes the
// subscription of the next one:
//
//   - its expiration policy deletes it after SubscriptionTTL (1 day, the least Pub/Sub allows)
//     without subscriber activity, and its messages are kept MessageRetention (10 minutes, the
//     least), so that a subscription a process left behind costs nothing for long;
//   - the listener receives by streaming pull and acknowledges each message as soon as it
//     arrives, within the ack deadline AckDeadline (10 seconds, the least);
//   - when the streaming pull ends for good (the subscription was deleted or expired), the
//     listener is broken. A stream that the service closes and the client reopens loses
//     nothing: the messages wait in the subscription.
//
// The labels of a subscription record its process ID and its creation time, which Pub/Sub does
// not report; Sweep deletes, by them, the subscriptions of the prefix whose process has no
// liveness marker, before the expiration policy would, so that the topic stays far from the
// limit of 10,000 subscriptions.
//
// # IAM
//
// The service account of the server needs, in a custom role (roles/pubsub.editor grants far more):
//
//   - on the topic: pubsub.topics.publish (Send) and pubsub.topics.attachSubscription (Listen);
//   - on the project: pubsub.subscriptions.create (Listen) and pubsub.subscriptions.list (Sweep);
//   - on the subscriptions, granted on the project since their names are not known in advance:
//     pubsub.subscriptions.consume (the streaming pull and its acknowledgements) and
//     pubsub.subscriptions.delete (Close, Sweep).
package pubsub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pubsubv2 "cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/stream"
)

// The properties of the per-process subscriptions (ADR-0050 §2): the least values Pub/Sub
// allows, since a subscription lives only as long as its process.
const (
	SubscriptionTTL  = 24 * time.Hour
	MessageRetention = 10 * time.Minute
	AckDeadline      = 10 * time.Second
)

// PublishTimeout bounds how long the publisher retries a message, and so how long Close waits
// for the messages it holds: a signal older than it is stale, and stream.Relay has given up on
// it (RelayOptions.SendTimeout).
const PublishTimeout = 10 * time.Second

// The labels of a per-process subscription.
const (
	labelProcess = "my-stream-process"
	labelCreated = "my-stream-created"
)

// maxSubscriptionID is the longest subscription ID that Pub/Sub accepts.
const maxSubscriptionID = 255

// maxSubscriptionSuffix is the longest -<process id>-<n> that Listen appends to the prefix.
const maxSubscriptionSuffix = 1 + 32 + 1 + 20

var (
	topicPattern  = regexp.MustCompile(`^projects/([^/]+)/topics/[A-Za-z][A-Za-z0-9\-_.~+%]{2,254}$`)
	prefixPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9\-_.~+%]*$`)
	processIDHex  = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

// CheckTopic reports whether topic is the full name of a Pub/Sub topic,
// projects/<project>/topics/<topic>.
func CheckTopic(topic string) error {
	if !topicPattern.MatchString(topic) || strings.HasPrefix(topic[strings.LastIndex(topic, "/")+1:], "goog") {
		return fmt.Errorf("not a Pub/Sub topic name projects/<project>/topics/<topic>: %q", topic)
	}
	return nil
}

// CheckSubscriptionPrefix reports whether prefix can start the ID of a Pub/Sub subscription
// that Listen names <prefix>-<process id>-<n>.
func CheckSubscriptionPrefix(prefix string) error {
	if !prefixPattern.MatchString(prefix) || strings.HasPrefix(prefix, "goog") {
		return fmt.Errorf("not the start of a Pub/Sub subscription ID (a letter, then letters, digits, and - _ . ~ + %%; not goog): %q", prefix)
	}
	if len(prefix)+maxSubscriptionSuffix > maxSubscriptionID {
		return fmt.Errorf("subscription prefix longer than %d characters: %q", maxSubscriptionID-maxSubscriptionSuffix, prefix)
	}
	return nil
}

// Options configure a Bus.
type Options struct {
	// Topic is the full name of the topic, projects/<project>/topics/<topic>; the subscriptions
	// are created in its project. Required.
	Topic string
	// SubscriptionPrefix starts the IDs of the per-process subscriptions,
	// <SubscriptionPrefix>-<ProcessID>-<n>; every process of the instance uses the same one, and
	// Sweep looks only at the subscriptions that it starts. Required.
	SubscriptionPrefix string
	// ProcessID identifies this process (stream.NewProcessID); the Relay of the process uses the
	// same one for its liveness marker. Required.
	ProcessID string
	// ClientOptions configure the Pub/Sub client: credentials, an endpoint. Without them, the
	// client uses Application Default Credentials, or the emulator at PUBSUB_EMULATOR_HOST.
	ClientOptions []option.ClientOption
	// Logger receives the end of a broken listener; nil discards it.
	Logger *slog.Logger
	// Now returns the current time, for the creation label of a subscription and the age of a
	// swept one; nil means time.Now.
	Now func() time.Time
	// Reporter receives a panic of a listener's goroutine (kernel.Go); nil only panics again.
	Reporter kernel.ErrorReporter
}

// Bus is a stream.Bus and a stream.Sweeper on a Pub/Sub topic. It is safe for concurrent use.
type Bus struct {
	o         Options
	project   string
	client    *pubsubv2.Client
	publisher *pubsubv2.Publisher
	log       *slog.Logger

	// listens counts the calls of Listen, for the names of their subscriptions.
	listens atomic.Uint64
}

var (
	_ stream.Bus     = (*Bus)(nil)
	_ stream.Sweeper = (*Bus)(nil)
)

// New returns a Bus on the topic of o. It does not reach Pub/Sub: a topic that does not exist
// or cannot be reached makes Send and Listen fail.
func New(ctx context.Context, o Options) (*Bus, error) {
	if err := CheckTopic(o.Topic); err != nil {
		return nil, err
	}
	if err := CheckSubscriptionPrefix(o.SubscriptionPrefix); err != nil {
		return nil, err
	}
	if !processIDHex.MatchString(o.ProcessID) {
		return nil, fmt.Errorf("process ID is not 32 lowercase hexadecimal characters: %q", o.ProcessID)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	project := topicPattern.FindStringSubmatch(o.Topic)[1]
	client, err := pubsubv2.NewClient(ctx, project, o.ClientOptions...)
	if err != nil {
		return nil, fmt.Errorf("pubsub client: %w", err)
	}
	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	publisher := client.Publisher(o.Topic)
	publisher.PublishSettings.Timeout = PublishTimeout
	return &Bus{o: o, project: project, client: client, publisher: publisher, log: log}, nil
}

// Close stops the publisher, after it sends what it holds, and closes the client. It does not
// close an open listener.
func (b *Bus) Close() error {
	b.publisher.Stop()
	return b.client.Close()
}

// subscriptionStart returns the start of the full names of the subscriptions of processID.
func (b *Bus) subscriptionStart(processID string) string {
	return "projects/" + b.project + "/subscriptions/" + b.o.SubscriptionPrefix + "-" + processID + "-"
}

// Send publishes payload on the topic and returns once Pub/Sub has it.
func (b *Bus) Send(ctx context.Context, payload []byte) error {
	if _, err := b.publisher.Publish(ctx, &pubsubv2.Message{Data: payload}).Get(ctx); err != nil {
		return fmt.Errorf("pubsub publish: %w", err)
	}
	return nil
}

// Listen creates the subscription of this process and receives from it. A payload published
// after the subscription exists waits in it, so Listen returns once it is created.
func (b *Bus) Listen(ctx context.Context, receive func(payload []byte)) (stream.Listener, error) {
	name := b.subscriptionStart(b.o.ProcessID) + strconv.FormatUint(b.listens.Add(1), 10)
	_, err := b.client.SubscriptionAdminClient.CreateSubscription(ctx, &pubsubpb.Subscription{
		Name:                     name,
		Topic:                    b.o.Topic,
		AckDeadlineSeconds:       int32(AckDeadline / time.Second),
		MessageRetentionDuration: durationpb.New(MessageRetention),
		ExpirationPolicy:         &pubsubpb.ExpirationPolicy{Ttl: durationpb.New(SubscriptionTTL)},
		Labels: map[string]string{
			labelProcess: b.o.ProcessID,
			labelCreated: strconv.FormatInt(b.o.Now().Unix(), 10),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("pubsub create subscription %s: %w", name, err)
	}
	sub := b.client.Subscriber(name)
	sub.ReceiveSettings.NumGoroutines = 1
	// Every message is acknowledged on arrival: nothing is outstanding at shutdown.
	sub.ReceiveSettings.ShutdownOptions = &pubsubv2.ShutdownOptions{Timeout: 0}

	recvCtx, cancel := context.WithCancel(context.Background())
	l := &listener{bus: b, name: name, cancel: cancel, broken: make(chan struct{}), done: make(chan struct{}), receive: receive}
	kernel.Go(recvCtx, "stream.pubsub.receive", b.o.Reporter, func(ctx context.Context) { l.run(ctx, sub) })
	return l, nil
}

// listener is the open subscription of a process.
type listener struct {
	bus     *Bus
	name    string
	cancel  context.CancelFunc
	broken  chan struct{}
	done    chan struct{} // closed when Receive returned
	receive func([]byte)

	// mu serializes receive, which Receive calls from several goroutines, and closed stops it.
	mu     sync.Mutex
	closed bool

	closeOnce sync.Once
	closeErr  error
}

func (l *listener) run(ctx context.Context, sub *pubsubv2.Subscriber) {
	defer close(l.done)
	err := sub.Receive(ctx, func(_ context.Context, m *pubsubv2.Message) {
		m.Ack()
		l.mu.Lock()
		defer l.mu.Unlock()
		if !l.closed {
			l.receive(m.Data)
		}
	})
	if ctx.Err() != nil {
		return // Close
	}
	// The streaming pull ended for good: the subscription is gone, or the service refuses it.
	l.bus.log.Warn("the Pub/Sub listener stopped", slog.String("subscription", l.name), slog.Any("error", err))
	close(l.broken)
}

func (l *listener) Broken() <-chan struct{} { return l.broken }

// Close stops receiving and deletes the subscription; a subscription that is already gone is
// not an error.
func (l *listener) Close(ctx context.Context) error {
	l.closeOnce.Do(func() {
		l.mu.Lock()
		l.closed = true
		l.mu.Unlock()
		l.cancel()
		select {
		case <-l.done:
		case <-ctx.Done():
		}
		err := l.bus.client.SubscriptionAdminClient.DeleteSubscription(ctx, &pubsubpb.DeleteSubscriptionRequest{Subscription: l.name})
		if err != nil && status.Code(err) != codes.NotFound {
			l.closeErr = fmt.Errorf("pubsub delete subscription %s: %w", l.name, err)
		}
	})
	return l.closeErr
}

// Sweep deletes the subscriptions of the prefix that were created at least minAge ago and whose
// process is not alive. A subscription without the labels of Listen is kept.
func (b *Bus) Sweep(ctx context.Context, minAge time.Duration, alive func(ctx context.Context, processID string) (bool, error)) (int, error) {
	subs, err := b.subscriptions(ctx)
	if err != nil {
		return 0, err
	}
	deleted := 0
	var errs []error
	for _, s := range subs {
		processID := s.GetLabels()[labelProcess]
		created, err := strconv.ParseInt(s.GetLabels()[labelCreated], 10, 64)
		if err != nil || !processIDHex.MatchString(processID) || !strings.HasPrefix(s.GetName(), b.subscriptionStart(processID)) {
			continue
		}
		if b.o.Now().Sub(time.Unix(created, 0)) < minAge {
			continue
		}
		ok, err := alive(ctx, processID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if ok {
			continue
		}
		err = b.client.SubscriptionAdminClient.DeleteSubscription(ctx, &pubsubpb.DeleteSubscriptionRequest{Subscription: s.GetName()})
		switch {
		case err == nil:
			deleted++
		case status.Code(err) == codes.NotFound:
		default:
			errs = append(errs, fmt.Errorf("pubsub delete subscription %s: %w", s.GetName(), err))
		}
	}
	return deleted, errors.Join(errs...)
}

// subscriptions lists the subscriptions of the project whose ID starts with the prefix.
func (b *Bus) subscriptions(ctx context.Context) ([]*pubsubpb.Subscription, error) {
	start := "projects/" + b.project + "/subscriptions/" + b.o.SubscriptionPrefix + "-"
	it := b.client.SubscriptionAdminClient.ListSubscriptions(ctx, &pubsubpb.ListSubscriptionsRequest{Project: "projects/" + b.project})
	var subs []*pubsubpb.Subscription
	for {
		s, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return subs, nil
		}
		if err != nil {
			return nil, fmt.Errorf("pubsub list subscriptions: %w", err)
		}
		if strings.HasPrefix(s.GetName(), start) {
			subs = append(subs, s)
		}
	}
}
