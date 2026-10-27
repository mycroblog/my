// SPDX-License-Identifier: AGPL-3.0-only

// Package sns implements the cross-process broker of the real-time updates, stream.Bus, on an
// Amazon SNS standard topic that fans out to one SQS queue per listening process (stream =
// "sns", ADR-0050 §2), and stream.Sweeper for the queues that processes leave behind.
//
// SQS alone is point-to-point, so a payload goes to the topic (sns:Publish, the raw payload as
// the message) and SNS delivers it to every queue subscribed to the topic. Listen creates the
// queue <QueuePrefix>-<process ID>-<n> (n counts the listeners of the process, since a deleted
// queue's name cannot be used again for 60 seconds) with a MessageRetentionPeriod of 60 seconds
// (the minimum), a ReceiveMessageWaitTimeSeconds of 20 seconds, the tag my:stream, and a policy
// that lets sns.amazonaws.com send messages to it only from the topic (aws:SourceArn), and
// subscribes it to the topic with RawMessageDelivery; a subscription to a queue of the same
// account is confirmed by itself. The listener long-polls the queue (20 seconds, up to 10
// messages), hands each body to receive, and deletes the batch (DeleteMessageBatch). Close
// unsubscribes and deletes the queue. The listener breaks when the queue no longer exists (a
// sweep deleted it while the process was frozen) or when receiving keeps failing for 30
// seconds, half the retention.
//
// The topic is created by the deployment template, and the queue is created in the topic's
// account and region. The execution role needs:
//
//   - on the topic: sns:Publish, sns:Subscribe (conditions sns:Protocol = sqs and sns:Endpoint
//     like the queue ARN prefix arn:aws:sqs:<region>:<account>:<QueuePrefix>-*),
//     sns:Unsubscribe (whose resource type is the topic, by the Service Authorization
//     Reference), and sns:ListSubscriptionsByTopic (the sweep);
//   - on the queues arn:aws:sqs:<region>:<account>:<QueuePrefix>-*: sqs:CreateQueue,
//     sqs:TagQueue (needed to tag on creation), sqs:SetQueueAttributes (the policy),
//     sqs:GetQueueAttributes, sqs:ReceiveMessage, sqs:DeleteMessage (DeleteMessageBatch), and
//     sqs:DeleteQueue;
//   - on *: sqs:ListQueues (the sweep).
//
// The SNS and SQS operations go through the narrow interfaces TopicClient (SNS) and QueueClient
// (SQS), which *sns.Client and *sqs.Client of aws-sdk-go-v2 satisfy; the tests run on the
// in-memory fake of pkg/testkit/awstest and on an emulator.
package sns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/smithy-go"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/stream"
)

// TopicClient is the part of *sns.Client that the Bus uses.
type TopicClient interface {
	Publish(ctx context.Context, in *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
	Subscribe(ctx context.Context, in *sns.SubscribeInput, optFns ...func(*sns.Options)) (*sns.SubscribeOutput, error)
	Unsubscribe(ctx context.Context, in *sns.UnsubscribeInput, optFns ...func(*sns.Options)) (*sns.UnsubscribeOutput, error)
	ListSubscriptionsByTopic(ctx context.Context, in *sns.ListSubscriptionsByTopicInput, optFns ...func(*sns.Options)) (*sns.ListSubscriptionsByTopicOutput, error)
}

// QueueClient is the part of *sqs.Client that the Bus uses.
type QueueClient interface {
	CreateQueue(ctx context.Context, in *sqs.CreateQueueInput, optFns ...func(*sqs.Options)) (*sqs.CreateQueueOutput, error)
	GetQueueAttributes(ctx context.Context, in *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)
	ListQueues(ctx context.Context, in *sqs.ListQueuesInput, optFns ...func(*sqs.Options)) (*sqs.ListQueuesOutput, error)
	ReceiveMessage(ctx context.Context, in *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessageBatch(ctx context.Context, in *sqs.DeleteMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error)
	DeleteQueue(ctx context.Context, in *sqs.DeleteQueueInput, optFns ...func(*sqs.Options)) (*sqs.DeleteQueueOutput, error)
}

var (
	_ TopicClient = (*sns.Client)(nil)
	_ QueueClient = (*sqs.Client)(nil)
)

// The queue and receive settings (ADR-0050 §2) and the limits of SQS that they keep to.
const (
	// retentionSeconds is the queue's MessageRetentionPeriod: the minimum, so that a queue
	// nobody reads holds little.
	retentionSeconds = 60
	// waitSeconds is the long poll of ReceiveMessage and the queue's
	// ReceiveMessageWaitTimeSeconds: the maximum, so that an idle listener makes 3 requests a
	// minute.
	waitSeconds = 20
	// maxMessages is the most messages that ReceiveMessage returns and DeleteMessageBatch
	// deletes.
	maxMessages = 10
	// MaxQueuePrefix is the longest QueuePrefix: a queue name has at most 80 characters, and the
	// listener appends "-", the process ID (32), "-", and its number.
	MaxQueuePrefix = 40
	// QueueTag is the tag key of the queues of the Bus; its value is the process ID.
	QueueTag = "my:stream"
)

// The waits of the receive loop; variables so that tests can shorten them.
var (
	// receiveTimeout bounds one ReceiveMessage: the long poll and a margin.
	receiveTimeout = (waitSeconds + 10) * time.Second
	// brokenAfter is how long receiving may keep failing before the listener breaks: half the
	// retention, so that what the queue still holds is not lost yet.
	brokenAfter = retentionSeconds * time.Second / 2
	// retryMin and retryMax bound the wait between two failed receives.
	retryMin = 500 * time.Millisecond
	retryMax = 5 * time.Second
	// deleteTimeout bounds the DeleteMessageBatch of a received batch.
	deleteTimeout = 10 * time.Second
)

// queuePrefixRe is the form of QueuePrefix: the characters of an SQS queue name.
var queuePrefixRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Options configure a Bus.
type Options struct {
	// TopicARN is the ARN of the SNS standard topic (config stream_sns_topic_arn). Required.
	TopicARN string
	// QueuePrefix starts the name of every queue of the Bus (config stream_sqs_queue_prefix):
	// at most MaxQueuePrefix letters, digits, hyphens, and underscores. The sweep deletes the
	// queues of ended processes under it, so it belongs to one instance. Required.
	QueuePrefix string
	// ProcessID identifies this process (stream.NewProcessID); give its stream.Relay the same
	// one, so that the sweep keeps the queues of live processes. Empty takes a new one.
	ProcessID string
	// Clock tells the age of queues to the sweep; nil means the system time.
	Clock kernel.Clock
	// Logger receives the failures that the Bus survives (a receive that fails and is retried, a
	// batch that could not be deleted); nil discards them.
	Logger *slog.Logger
	// Reporter receives a panic of a listener's goroutine (kernel.Go); nil only panics again.
	Reporter kernel.ErrorReporter
}

// Bus is the SNS → SQS stream.Bus and its stream.Sweeper. It is safe for concurrent use.
type Bus struct {
	sns         TopicClient
	sqs         QueueClient
	topicARN    string
	queueARN    string // the ARN of the queues without the name: arn:<partition>:sqs:<region>:<account>:
	queuePrefix string
	processID   string
	clock       kernel.Clock
	log         *slog.Logger
	reporter    kernel.ErrorReporter
	listeners   atomic.Uint64
}

var (
	_ stream.Bus     = (*Bus)(nil)
	_ stream.Sweeper = (*Bus)(nil)
)

// CheckTopicARN reports whether topicARN is the ARN of an SNS topic,
// arn:<partition>:sns:<region>:<account>:<topic> (Options.TopicARN).
func CheckTopicARN(topicARN string) error {
	_, err := parseTopicARN(topicARN)
	return err
}

func parseTopicARN(topicARN string) (arn.ARN, error) {
	topic, err := arn.Parse(topicARN)
	if err != nil || topic.Service != "sns" || topic.Region == "" || topic.AccountID == "" || topic.Resource == "" {
		return arn.ARN{}, fmt.Errorf("not the ARN of an SNS topic arn:<partition>:sns:<region>:<account>:<topic>: %q", topicARN)
	}
	return topic, nil
}

// CheckQueuePrefix reports whether prefix can start the names of the queues of a Bus
// (Options.QueuePrefix).
func CheckQueuePrefix(prefix string) error {
	if len(prefix) > MaxQueuePrefix || !queuePrefixRe.MatchString(prefix) {
		return fmt.Errorf("not 1 to %d letters, digits, hyphens, and underscores: %q", MaxQueuePrefix, prefix)
	}
	return nil
}

// New returns the Bus on the topic and queues of o through topics and queues, which must
// reach the topic's region.
func New(topics TopicClient, queues QueueClient, o Options) (*Bus, error) {
	topic, err := parseTopicARN(o.TopicARN)
	if err != nil {
		return nil, fmt.Errorf("stream/sns: topic: %w", err)
	}
	if err := CheckQueuePrefix(o.QueuePrefix); err != nil {
		return nil, fmt.Errorf("stream/sns: queue prefix: %w", err)
	}
	processID := o.ProcessID
	if processID == "" {
		processID = stream.NewProcessID()
	}
	if !processIDRe.MatchString(processID) {
		return nil, fmt.Errorf("stream/sns: process ID %q is not 32 lowercase hexadecimal characters", processID)
	}
	b := &Bus{
		sns: topics, sqs: queues,
		topicARN:    o.TopicARN,
		queueARN:    "arn:" + topic.Partition + ":sqs:" + topic.Region + ":" + topic.AccountID + ":",
		queuePrefix: o.QueuePrefix, processID: processID,
		clock: o.Clock, log: o.Logger, reporter: o.Reporter,
	}
	if b.clock == nil {
		b.clock = systemClock{}
	}
	if b.log == nil {
		b.log = slog.New(slog.DiscardHandler)
	}
	return b, nil
}

// NewFromConfig returns the Bus of o with the SNS and SQS clients of cfg (credentials, HTTP
// client, endpoint), in the region of the topic.
func NewFromConfig(cfg aws.Config, o Options) (*Bus, error) {
	topic, err := arn.Parse(o.TopicARN)
	if err != nil {
		return New(nil, nil, o) // reports the ARN
	}
	region := func(r *string) { *r = topic.Region }
	return New(
		sns.NewFromConfig(cfg, func(o *sns.Options) { region(&o.Region) }),
		sqs.NewFromConfig(cfg, func(o *sqs.Options) { region(&o.Region) }),
		o,
	)
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// ProcessID returns the ID of this process in the names of its queues.
func (b *Bus) ProcessID() string { return b.processID }

// Send publishes payload to the topic.
func (b *Bus) Send(ctx context.Context, payload []byte) error {
	_, err := b.sns.Publish(ctx, &sns.PublishInput{TopicArn: aws.String(b.topicARN), Message: aws.String(string(payload))})
	if err != nil {
		return fmt.Errorf("stream/sns: publish: %w", err)
	}
	return nil
}

// queuePolicy is the policy of a queue: sns.amazonaws.com may send it messages from the topic
// only.
func (b *Bus) queuePolicy(queueARN string) string {
	type statement struct {
		Sid       string
		Effect    string
		Principal map[string]string
		Action    string
		Resource  string
		Condition map[string]map[string]string
	}
	p, _ := json.Marshal(struct {
		Version   string
		Statement []statement
	}{
		Version: "2012-10-17",
		Statement: []statement{{
			Sid: "TopicSendMessage", Effect: "Allow",
			Principal: map[string]string{"Service": "sns.amazonaws.com"},
			Action:    "sqs:SendMessage", Resource: queueARN,
			Condition: map[string]map[string]string{"ArnEquals": {"aws:SourceArn": b.topicARN}},
		}},
	})
	return string(p)
}

// Listen creates this process's queue, subscribes it to the topic, and starts receiving.
func (b *Bus) Listen(ctx context.Context, receive func(payload []byte)) (stream.Listener, error) {
	name := b.queuePrefix + "-" + b.processID + "-" + strconv.FormatUint(b.listeners.Add(1), 10)
	queueARN := b.queueARN + name
	created, err := b.sqs.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(name),
		Attributes: map[string]string{
			string(sqstypes.QueueAttributeNameMessageRetentionPeriod):        strconv.Itoa(retentionSeconds),
			string(sqstypes.QueueAttributeNameReceiveMessageWaitTimeSeconds): strconv.Itoa(waitSeconds),
			string(sqstypes.QueueAttributeNamePolicy):                        b.queuePolicy(queueARN),
		},
		Tags: map[string]string{QueueTag: b.processID},
	})
	if err != nil {
		return nil, fmt.Errorf("stream/sns: create queue %s: %w", name, err)
	}
	l := &listener{
		b: b, name: name, url: aws.ToString(created.QueueUrl), receive: receive,
		broken: make(chan struct{}), stopped: make(chan struct{}),
	}
	sub, err := b.sns.Subscribe(ctx, &sns.SubscribeInput{
		TopicArn: aws.String(b.topicARN), Protocol: aws.String("sqs"), Endpoint: aws.String(queueARN),
		Attributes:            map[string]string{"RawMessageDelivery": "true"},
		ReturnSubscriptionArn: true,
	})
	if err != nil {
		b.deleteFailedQueue(ctx, l)
		return nil, fmt.Errorf("stream/sns: subscribe queue %s: %w", name, err)
	}
	l.subscription = aws.ToString(sub.SubscriptionArn)
	var run context.Context
	run, l.cancel = context.WithCancel(context.WithoutCancel(ctx))
	kernel.Go(run, "stream.sns.receive", b.reporter, l.run)
	return l, nil
}

// deleteFailedQueue deletes the queue of a Listen whose subscription failed. It deletes it
// within ctx, so that Listen still returns by its deadline, and, when ctx ends first, goes on
// in the background for up to deleteTimeout; a queue left behind is the sweep's.
func (b *Bus) deleteFailedQueue(ctx context.Context, l *listener) {
	warn := func(err error) {
		b.log.Warn("stream/sns: could not delete the queue of a failed listen; the sweep deletes it", slog.String("queue", l.name), slog.Any("error", err))
	}
	if ctx.Err() == nil {
		err := l.deleteQueue(ctx)
		if err == nil {
			return
		}
		if ctx.Err() == nil {
			warn(err)
			return
		}
	}
	kernel.Go(context.WithoutCancel(ctx), "stream.sns.cleanup", b.reporter, func(bg context.Context) {
		bg, cancel := context.WithTimeout(bg, deleteTimeout)
		defer cancel()
		if err := l.deleteQueue(bg); err != nil {
			warn(err)
		}
	})
}

// listener is the open listener of one queue.
type listener struct {
	b            *Bus
	name, url    string
	subscription string
	receive      func(payload []byte)

	cancel  context.CancelFunc
	broken  chan struct{}
	stopped chan struct{}

	// mu orders the calls of receive with Close: once closed is set, receive is not called.
	mu     sync.Mutex
	closed bool
	once   sync.Once
}

func (l *listener) Broken() <-chan struct{} { return l.broken }

// run receives until ctx ends or the listener breaks.
func (l *listener) run(ctx context.Context) {
	defer close(l.stopped)
	var failingSince time.Time
	wait := retryMin
	for ctx.Err() == nil {
		msgs, err := l.receiveBatch(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			if queueGone(err) {
				l.b.log.WarnContext(ctx, "stream/sns: the queue of the listener is gone", slog.String("queue", l.name))
				close(l.broken)
				return
			}
			now := time.Now()
			if failingSince.IsZero() {
				failingSince = now
			}
			if now.Sub(failingSince) >= brokenAfter {
				l.b.log.WarnContext(ctx, "stream/sns: receiving keeps failing; the listener breaks", slog.String("queue", l.name), slog.Any("error", err))
				close(l.broken)
				return
			}
			l.b.log.WarnContext(ctx, "stream/sns: receive failed; retrying", slog.String("queue", l.name), slog.Any("error", err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			wait = min(2*wait, retryMax)
			continue
		}
		failingSince, wait = time.Time{}, retryMin
		if len(msgs) == 0 {
			continue
		}
		if !l.deliver(msgs) {
			return
		}
		l.deleteBatch(ctx, msgs)
	}
}

// receiveBatch makes one long poll of the queue.
func (l *listener) receiveBatch(ctx context.Context) ([]sqstypes.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, receiveTimeout)
	defer cancel()
	out, err := l.b.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl: aws.String(l.url), WaitTimeSeconds: waitSeconds, MaxNumberOfMessages: maxMessages,
	})
	if err != nil {
		return nil, err
	}
	return out.Messages, nil
}

// deliver hands the bodies of msgs to receive, unless the listener is closed; it reports
// whether the listener is still open.
func (l *listener) deliver(msgs []sqstypes.Message) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return false
	}
	for _, m := range msgs {
		l.receive([]byte(aws.ToString(m.Body)))
	}
	return true
}

// deleteBatch deletes received messages from the queue; a message that stays is received again
// after the visibility timeout, a duplicate that the Bus allows.
func (l *listener) deleteBatch(ctx context.Context, msgs []sqstypes.Message) {
	entries := make([]sqstypes.DeleteMessageBatchRequestEntry, len(msgs))
	for i, m := range msgs {
		entries[i] = sqstypes.DeleteMessageBatchRequestEntry{Id: aws.String(strconv.Itoa(i)), ReceiptHandle: m.ReceiptHandle}
	}
	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()
	out, err := l.b.sqs.DeleteMessageBatch(ctx, &sqs.DeleteMessageBatchInput{QueueUrl: aws.String(l.url), Entries: entries})
	switch {
	case err != nil && ctx.Err() == nil:
		l.b.log.WarnContext(ctx, "stream/sns: could not delete received messages", slog.String("queue", l.name), slog.Any("error", err))
	case err == nil && len(out.Failed) > 0:
		l.b.log.WarnContext(ctx, "stream/sns: could not delete some received messages", slog.String("queue", l.name), slog.Int("failed", len(out.Failed)))
	}
}

// Close stops receiving, unsubscribes the queue, and deletes it.
func (l *listener) Close(ctx context.Context) error {
	err := error(nil)
	l.once.Do(func() {
		l.mu.Lock()
		l.closed = true
		l.mu.Unlock()
		l.cancel()
		select {
		case <-l.stopped:
		case <-ctx.Done():
			err = ctx.Err()
			return
		}
		err = errors.Join(l.unsubscribe(ctx), l.deleteQueue(ctx))
	})
	return err
}

func (l *listener) unsubscribe(ctx context.Context) error {
	_, err := l.b.sns.Unsubscribe(ctx, &sns.UnsubscribeInput{SubscriptionArn: aws.String(l.subscription)})
	if err != nil && !subscriptionGone(err) {
		return fmt.Errorf("stream/sns: unsubscribe queue %s: %w", l.name, err)
	}
	return nil
}

func (l *listener) deleteQueue(ctx context.Context) error {
	_, err := l.b.sqs.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(l.url)})
	if err != nil && !queueGone(err) {
		return fmt.Errorf("stream/sns: delete queue %s: %w", l.name, err)
	}
	return nil
}

// queueGone reports whether err says that the queue does not exist. Through the query
// compatibility of SQS, the code may also be the one of the query protocol.
func queueGone(err error) bool {
	var notExist *sqstypes.QueueDoesNotExist
	if errors.As(err, &notExist) {
		return true
	}
	var api smithy.APIError
	return errors.As(err, &api) && (api.ErrorCode() == "QueueDoesNotExist" || api.ErrorCode() == "AWS.SimpleQueueService.NonExistentQueue")
}

// subscriptionGone reports whether err says that the subscription does not exist.
func subscriptionGone(err error) bool {
	var notFound *snstypes.NotFoundException
	if errors.As(err, &notFound) {
		return true
	}
	var api smithy.APIError
	return errors.As(err, &api) && (strings.Contains(api.ErrorCode(), "NotFound") || strings.Contains(api.ErrorCode(), "NonExistentSubscription"))
}
