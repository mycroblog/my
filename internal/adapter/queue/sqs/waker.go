// SPDX-License-Identifier: AGPL-3.0-only

package sqs

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
)

// MaxDelay is the longest DelaySeconds of a message: a signal for a later time is sent at it,
// and the receiver signals the job again when it arrives early.
const MaxDelay = 900 * time.Second

// MaxBatch is the most messages of one SendMessageBatch call.
const MaxBatch = 10

var (
	// queueHost matches the host of an SQS endpoint and captures its region.
	queueHost = regexp.MustCompile(`^sqs(?:-fips)?\.([a-z0-9-]+)\.amazonaws\.com(?:\.cn)?$`)
	// queueURLPath matches the path of a queue URL: the account and the name of a standard queue,
	// at most 80 letters, digits, hyphens, and underscores.
	queueURLPath = regexp.MustCompile(`^/[0-9]{12}/[A-Za-z0-9_-]{1,80}$`)
)

var errQueueURL = errors.New("must be the URL of an SQS standard queue, https://sqs.{region}.amazonaws.com/{account}/{name}")

// CheckQueueURL reports whether v is the URL of an SQS standard queue,
// https://sqs.{region}.amazonaws.com/{account}/{name}.
func CheckQueueURL(v string) error {
	_, err := queueRegion(v)
	return err
}

// queueRegion returns the region of the queue URL v.
func queueRegion(v string) (string, error) {
	u, err := url.Parse(v)
	if err != nil {
		return "", errQueueURL
	}
	if strings.HasSuffix(u.Path, ".fifo") {
		return "", errors.New("must be a standard queue, not a FIFO queue")
	}
	m := queueHost.FindStringSubmatch(u.Host)
	if u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !queueURLPath.MatchString(u.Path) || m == nil {
		return "", errQueueURL
	}
	return m[1], nil
}

// Client is the part of *sqs.Client that the Waker uses; the tests stand in for it.
type Client interface {
	SendMessage(ctx context.Context, in *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
	SendMessageBatch(ctx context.Context, in *sqs.SendMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageBatchOutput, error)
}

var _ Client = (*sqs.Client)(nil)

// Options configure a Waker. QueueURL and Clock are required.
type Options struct {
	// QueueURL is the URL of the standard queue (CheckQueueURL).
	QueueURL string
	Clock    kernel.Clock
}

// Waker is the queue.Waker of SQS: each signal is a message of the standard queue whose body is
// the job ID and whose DelaySeconds is the time until the signal, at most MaxDelay.
type Waker struct {
	client Client
	opts   Options
}

var _ queue.Waker = (*Waker)(nil)

// New returns a Waker that sends with client.
func New(client Client, opts Options) (*Waker, error) {
	if err := CheckQueueURL(opts.QueueURL); err != nil {
		return nil, fmt.Errorf("queue/sqs: queue URL %q %w", opts.QueueURL, err)
	}
	if client == nil || opts.Clock == nil {
		return nil, errors.New("queue/sqs: a client and Options.Clock are required")
	}
	return &Waker{client: client, opts: opts}, nil
}

// NewFromConfig returns the Waker of opts on an SQS client of cfg (credentials, HTTP client), in
// the region of the queue URL.
func NewFromConfig(cfg aws.Config, opts Options) (*Waker, error) {
	region, err := queueRegion(opts.QueueURL)
	if err != nil {
		return nil, fmt.Errorf("queue/sqs: queue URL %q %w", opts.QueueURL, err)
	}
	return New(sqs.NewFromConfig(cfg, func(o *sqs.Options) { o.Region = region }), opts)
}

// Wake sends one message per signal: one SendMessage for a single signal, and SendMessageBatch
// calls of at most MaxBatch messages for several. It returns the errors of the messages that
// SQS did not accept.
func (w *Waker) Wake(ctx context.Context, wakes []queue.Wake) error {
	now := w.opts.Clock.Now()
	if len(wakes) == 1 {
		wk := wakes[0]
		_, err := w.client.SendMessage(ctx, &sqs.SendMessageInput{
			QueueUrl:     aws.String(w.opts.QueueURL),
			MessageBody:  aws.String(wk.JobID),
			DelaySeconds: delaySeconds(now, wk.At),
		})
		if err != nil {
			return fmt.Errorf("queue/sqs: signal of job %s: %w", wk.JobID, err)
		}
		return nil
	}
	var errs []error
	for start := 0; start < len(wakes); start += MaxBatch {
		if err := w.sendBatch(ctx, now, wakes[start:min(start+MaxBatch, len(wakes))]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// sendBatch sends the signals of one SendMessageBatch call; the ID of an entry is its index.
func (w *Waker) sendBatch(ctx context.Context, now time.Time, wakes []queue.Wake) error {
	entries := make([]types.SendMessageBatchRequestEntry, len(wakes))
	for i, wk := range wakes {
		entries[i] = types.SendMessageBatchRequestEntry{
			Id:           aws.String(strconv.Itoa(i)),
			MessageBody:  aws.String(wk.JobID),
			DelaySeconds: delaySeconds(now, wk.At),
		}
	}
	out, err := w.client.SendMessageBatch(ctx, &sqs.SendMessageBatchInput{QueueUrl: aws.String(w.opts.QueueURL), Entries: entries})
	if err != nil {
		return fmt.Errorf("queue/sqs: signals of %d jobs: %w", len(wakes), err)
	}
	var errs []error
	for _, f := range out.Failed {
		job := aws.ToString(f.Id)
		if i, err := strconv.Atoi(job); err == nil && i >= 0 && i < len(wakes) {
			job = wakes[i].JobID
		}
		errs = append(errs, fmt.Errorf("queue/sqs: signal of job %s: %s: %s", job, aws.ToString(f.Code), aws.ToString(f.Message)))
	}
	return errors.Join(errs...)
}

// delaySeconds is the DelaySeconds of a signal at at: the whole seconds until it, rounded up so
// that it does not arrive early, and at most MaxDelay.
func delaySeconds(now, at time.Time) int32 {
	d := queue.NextVisible(now, at, MaxDelay).Sub(now)
	if d <= 0 {
		return 0
	}
	return int32(min((d+time.Second-1)/time.Second, MaxDelay/time.Second))
}
