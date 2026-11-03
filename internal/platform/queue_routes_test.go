// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_sqs

package platform_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	sqsqueue "github.com/mycroblog/my/internal/adapter/queue/sqs"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/pkg/testkit"
)

const (
	defaultSQSQueue = "https://sqs.ap-northeast-1.amazonaws.com/123456789012/my-jobs"
	mailSQSQueue    = "https://sqs.ap-northeast-1.amazonaws.com/123456789012/my-mail"
)

// fakeSQS is an SQS that records the bodies of the messages per queue.
type fakeSQS struct {
	mu     sync.Mutex
	bodies map[string][]string
}

func (s *fakeSQS) SendMessage(_ context.Context, in *sqs.SendMessageInput, _ ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bodies[*in.QueueUrl] = append(s.bodies[*in.QueueUrl], *in.MessageBody)
	return &sqs.SendMessageOutput{}, nil
}

func (s *fakeSQS) SendMessageBatch(_ context.Context, in *sqs.SendMessageBatchInput, _ ...func(*sqs.Options)) (*sqs.SendMessageBatchOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := &sqs.SendMessageBatchOutput{}
	for _, e := range in.Entries {
		s.bodies[*in.QueueUrl] = append(s.bodies[*in.QueueUrl], *e.MessageBody)
		out.Successful = append(out.Successful, types.SendMessageBatchResultEntry{Id: e.Id})
	}
	return out, nil
}

func (s *fakeSQS) take(queueURL string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.bodies[queueURL]
	delete(s.bodies, queueURL)
	return b
}

func (s *fakeSQS) waker(t *testing.T, queueURL string) queue.Waker {
	t.Helper()
	w, err := sqsqueue.New(s, sqsqueue.Options{QueueURL: queueURL, Clock: testkit.NewClock(time.Now())})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// routedSQSApp starts an app with queue = sqs whose signals reach the queues of server through the
// real SQS adapter, with the given value of queue_sqs_routes.
func routedSQSApp(t *testing.T, server *fakeSQS, routes string, setup func(*platform.App)) *server {
	t.Helper()
	cfg := loadConfig(t, map[string]string{
		"MY_DB": "postgres", "MY_DATABASE_URL": "postgres://unused/my", "MY_QUEUE": "sqs",
		"MY_QUEUE_SQS_QUEUE_URL": defaultSQSQueue, "MY_QUEUE_SQS_ROUTES": routes,
	})
	cfg.DB, cfg.DatabaseURL = "sqlite", "file:"+newDatabase(t, true)
	return startApp(t, cfg, platform.AppOptions{
		Waker:      server.waker(t, defaultSQSQueue),
		RouteWaker: func(r config.QueueRoute) queue.Waker { return server.waker(t, r.Queue) },
	}, platform.ServeOptions{}, func(a *platform.App) {
		if setup != nil {
			setup(a)
		}
	})
}

func TestQueueRoutesSendSignalsToTheirQueue(t *testing.T) {
	server := &fakeSQS{bodies: map[string][]string{}}
	s := routedSQSApp(t, server, "notification.*="+mailSQSQueue, nil)

	// The signal is the job's ID alone, on the queue of the job's type.
	if err := s.app.Jobs.Enqueue(t.Context(), queue.NewJob{Type: "notification.push"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	routed := server.take(mailSQSQueue)
	if len(routed) != 1 || !kernel.ValidID(routed[0]) {
		t.Fatalf("the routed queue got %v, want a job ID", routed)
	}
	if got := server.take(defaultSQSQueue); len(got) != 0 {
		t.Errorf("the default queue got %v for a routed type", got)
	}

	if err := s.app.Jobs.Enqueue(t.Context(), queue.NewJob{Type: "media.process"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if got := server.take(defaultSQSQueue); len(got) != 1 {
		t.Errorf("the default queue got %v, want the signal of a type without a route", got)
	}
	if got := server.take(mailSQSQueue); len(got) != 0 {
		t.Errorf("the routed queue got %v for another type", got)
	}

	for deadline := time.Now().Add(5 * time.Second); !strings.Contains(s.logs.String(), `"routes_hash"`) && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	if logs := s.logs.String(); !strings.Contains(logs, mailSQSQueue) {
		t.Errorf("the startup log lacks the routes:\n%s", logs)
	}
}

// Which queue a signal came from does not matter to the receiver, so a misrouted signal, as from a
// process with other routes, still runs its job.
func TestQueueRoutesMisroutedSignalStillRuns(t *testing.T) {
	server := &fakeSQS{bodies: map[string][]string{}}
	var ran sync.WaitGroup
	ran.Add(1)
	s := routedSQSApp(t, server, "notification.*="+mailSQSQueue, func(a *platform.App) {
		a.Handlers.Register("test.misrouted", func(context.Context, queue.Job) queue.Result {
			ran.Done()
			return queue.Done()
		})
	})
	if err := s.app.Jobs.Enqueue(t.Context(), queue.NewJob{Type: "test.misrouted"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	id := server.take(defaultSQSQueue)
	if len(id) != 1 {
		t.Fatalf("signals = %v", id)
	}
	status, failed := postEvent(t, s.url+config.DefaultSQSEventsPath, "null", id[0])
	if status != 200 || len(failed) != 0 {
		t.Fatalf("event = %d, %v", status, failed)
	}
	ran.Wait()
}

// A route that matches no job type stops the startup, whichever role the process has.
func TestQueueRoutesUnknownTypeFailsStartup(t *testing.T) {
	for _, role := range []string{"web", "worker", "all"} {
		cfg := loadConfig(t, map[string]string{
			"MY_ROLE": role, "MY_DB": "postgres", "MY_DATABASE_URL": "postgres://unused/my", "MY_QUEUE": "sqs",
			"MY_QUEUE_SQS_QUEUE_URL": defaultSQSQueue, "MY_QUEUE_SQS_ROUTES": "nonexistent.*=" + mailSQSQueue,
		})
		cfg.DB, cfg.DatabaseURL = "sqlite", "file:"+newDatabase(t, true)
		a, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Waker: &testkit.Waker{}, RouteWaker: func(config.QueueRoute) queue.Waker { return &testkit.Waker{} }})
		if err == nil {
			_ = a.Close()
			t.Fatalf("role %s: NewApp succeeded", role)
		}
		if !strings.Contains(err.Error(), "nonexistent.*") {
			t.Errorf("role %s: NewApp = %v, want it to name the pattern", role, err)
		}
	}
}
