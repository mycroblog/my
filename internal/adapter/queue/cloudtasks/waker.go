// SPDX-License-Identifier: AGPL-3.0-only

package cloudtasks

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
)

// MaxDelay is the furthest ahead that Cloud Tasks schedules a task: a signal for a later time is
// sent at it, and the receiver signals the job again when it arrives early.
const MaxDelay = 30 * 24 * time.Hour

// The bounds of the dispatch deadline of an HTTP-target task.
const (
	MinDispatchDeadline = 15 * time.Second
	MaxDispatchDeadline = 30 * time.Minute
)

// DefaultParallel is how many tasks one Wake creates at a time.
const DefaultParallel = 8

// queuePath matches the resource name of a queue. A project may be a domain-scoped ID
// ("example.com:project"); a queue ID is at most 100 letters, digits, and hyphens.
var queuePath = regexp.MustCompile(`^projects/[^/\s]+/locations/[a-z0-9-]+/queues/[A-Za-z0-9-]{1,100}$`)

// CheckQueuePath reports whether v is the resource name of a Cloud Tasks queue,
// projects/{project}/locations/{location}/queues/{queue}.
func CheckQueuePath(v string) error {
	if !queuePath.MatchString(v) {
		return errors.New("must be the resource name of a Cloud Tasks queue, projects/{project}/locations/{location}/queues/{queue}")
	}
	return nil
}

// Options configure a Waker. Queue, URL, ServiceAccount, and Clock are required.
type Options struct {
	// Queue is the resource name of the queue (CheckQueuePath).
	Queue string
	// URL is the absolute URL of the receiver's endpoint, POST /_internal/jobs.
	URL string
	// ServiceAccount is the email of the service account whose OIDC token each task carries.
	ServiceAccount string
	// Audience is the audience of the token; empty makes Cloud Tasks use URL.
	Audience string
	// DispatchDeadline bounds one delivery, from MinDispatchDeadline to MaxDispatchDeadline; zero
	// leaves the default of Cloud Tasks (10 minutes).
	DispatchDeadline time.Duration
	Clock            kernel.Clock
	// Parallel is how many tasks one Wake creates at a time; DefaultParallel when zero.
	Parallel int
	// Reporter reports a panic of the goroutines of Wake; nil only panics again.
	Reporter kernel.ErrorReporter
	// ClientOptions are passed to the Cloud Tasks client, after the defaults (tests only: an
	// endpoint of a test server without authentication).
	ClientOptions []option.ClientOption
}

// Waker is the queue.Waker of Cloud Tasks: each signal is an unnamed HTTP-target task whose body
// is the job ID and whose scheduleTime is the signal's time. A task is never named, because
// Cloud Tasks refuses a name for a day after its task ran, which would refuse the next signal of
// the same job.
type Waker struct {
	client *cloudtasks.Client
	opts   Options
}

var _ queue.Waker = (*Waker)(nil)

// New returns a Waker on a Cloud Tasks client with Application Default Credentials. Close
// releases the client.
func New(ctx context.Context, opts Options) (*Waker, error) {
	if err := CheckQueuePath(opts.Queue); err != nil {
		return nil, fmt.Errorf("cloudtasks: queue %q %w", opts.Queue, err)
	}
	if opts.URL == "" || opts.ServiceAccount == "" || opts.Clock == nil {
		return nil, errors.New("cloudtasks: Options.URL, ServiceAccount, and Clock are required")
	}
	if d := opts.DispatchDeadline; d != 0 && (d < MinDispatchDeadline || d > MaxDispatchDeadline) {
		return nil, fmt.Errorf("cloudtasks: the dispatch deadline %s is not from %s to %s", d, MinDispatchDeadline, MaxDispatchDeadline)
	}
	if opts.Parallel <= 0 {
		opts.Parallel = DefaultParallel
	}
	client, err := cloudtasks.NewClient(ctx, opts.ClientOptions...)
	if err != nil {
		return nil, fmt.Errorf("cloudtasks: %w", err)
	}
	return &Waker{client: client, opts: opts}, nil
}

// Close releases the Cloud Tasks client.
func (w *Waker) Close() error { return w.client.Close() }

// Wake creates one task per signal, at most Options.Parallel at a time, and returns the errors of
// the tasks it could not create.
func (w *Waker) Wake(ctx context.Context, wakes []queue.Wake) error {
	if len(wakes) == 1 {
		return w.create(ctx, wakes[0])
	}
	var (
		mu   sync.Mutex
		errs []error
		wg   sync.WaitGroup
	)
	next := make(chan queue.Wake)
	for range min(w.opts.Parallel, len(wakes)) {
		wg.Add(1)
		kernel.Go(ctx, "queue.cloudtasks.wake", w.opts.Reporter, func(ctx context.Context) {
			defer wg.Done()
			for wk := range next {
				if err := w.create(ctx, wk); err != nil {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
				}
			}
		})
	}
	for _, wk := range wakes {
		next <- wk
	}
	close(next)
	wg.Wait()
	return errors.Join(errs...)
}

// create creates the task of one signal.
func (w *Waker) create(ctx context.Context, wk queue.Wake) error {
	at := queue.NextVisible(w.opts.Clock.Now(), wk.At, MaxDelay)
	task := &cloudtaskspb.Task{
		ScheduleTime: timestamppb.New(at),
		MessageType: &cloudtaskspb.Task_HttpRequest{HttpRequest: &cloudtaskspb.HttpRequest{
			Url:        w.opts.URL,
			HttpMethod: cloudtaskspb.HttpMethod_POST,
			Headers:    map[string]string{"Content-Type": "text/plain; charset=utf-8"},
			Body:       []byte(wk.JobID),
			AuthorizationHeader: &cloudtaskspb.HttpRequest_OidcToken{OidcToken: &cloudtaskspb.OidcToken{
				ServiceAccountEmail: w.opts.ServiceAccount,
				Audience:            w.opts.Audience,
			}},
		}},
	}
	if w.opts.DispatchDeadline > 0 {
		task.DispatchDeadline = durationpb.New(w.opts.DispatchDeadline)
	}
	if _, err := w.client.CreateTask(ctx, &cloudtaskspb.CreateTaskRequest{Parent: w.opts.Queue, Task: task}); err != nil {
		return fmt.Errorf("cloudtasks: signal of job %s: %w", wk.JobID, err)
	}
	return nil
}
