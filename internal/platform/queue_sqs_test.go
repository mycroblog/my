// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_sqs

package platform_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/pkg/testkit"
)

func sqsConfig(t *testing.T) *config.Config {
	t.Helper()
	// queue = sqs needs a shared database; the test runs it on SQLite all the same.
	cfg := loadConfig(t, map[string]string{
		"MY_DB": "postgres", "MY_DATABASE_URL": "postgres://unused/my", "MY_QUEUE": "sqs",
		"MY_QUEUE_SQS_QUEUE_URL": "https://sqs.ap-northeast-1.amazonaws.com/123456789012/my-jobs",
	})
	cfg.DB, cfg.DatabaseURL = "sqlite", "file:"+newDatabase(t, true)
	return cfg
}

// postEvent posts an SQS event of Lambda to url as the Lambda Web Adapter passes it through when
// requestContext is "null", and returns the status and the message IDs it reports as failed.
func postEvent(t *testing.T, url, requestContext string, bodies ...string) (int, []string) {
	t.Helper()
	type record struct {
		MessageID string `json:"messageId"`
		Body      string `json:"body"`
	}
	var ev struct {
		Records []record `json:"Records"`
	}
	for i, b := range bodies {
		ev.Records = append(ev.Records, record{MessageID: fmt.Sprintf("m%d", i+1), Body: b})
	}
	body, _ := json.Marshal(ev)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Amzn-Request-Context", requestContext)
	req.Header.Set("X-Amzn-Lambda-Context", fmt.Sprintf(`{"request_id":"r1","deadline":%d}`, time.Now().Add(15*time.Minute).UnixMilli()))
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		BatchItemFailures []struct {
			ItemIdentifier string `json:"itemIdentifier"`
		} `json:"batchItemFailures"`
	}
	var failed []string
	if json.NewDecoder(resp.Body).Decode(&out) == nil {
		for _, f := range out.BatchItemFailures {
			failed = append(failed, f.ItemIdentifier)
		}
	}
	return resp.StatusCode, failed
}

// TestSQSQueue checks the wiring of queue = sqs: an enqueued job is signaled, no runner polls
// the table, the SQS event of the signal at queue_sqs_events_path runs it, and the endpoints of
// Cloud Tasks are not served.
func TestSQSQueue(t *testing.T) {
	waker := &testkit.Waker{}
	var runs atomic.Int32
	s := startApp(t, sqsConfig(t), platform.AppOptions{Waker: waker}, platform.ServeOptions{},
		func(a *platform.App) {
			a.Handlers.Register("test.sqs", func(context.Context, queue.Job) queue.Result {
				runs.Add(1)
				return queue.Done()
			})
		})
	if err := s.app.Jobs.Enqueue(t.Context(), queue.NewJob{Type: "test.sqs"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	wakes := waker.Take()
	if len(wakes) != 1 || wakes[0].JobID == "" {
		t.Fatalf("signals = %+v, want one for the job", wakes)
	}
	// A pull runner would take the job within its poll interval.
	time.Sleep(queue.DefaultPollInterval + 500*time.Millisecond)
	if n := runs.Load(); n != 0 {
		t.Fatalf("the job ran %d times before its signal arrived", n)
	}

	// A Function URL request to the path is refused.
	if code, _ := postEvent(t, s.url+"/events", `{"http":{"method":"POST","path":"/events"}}`, wakes[0].JobID); code != http.StatusNotFound {
		t.Errorf("Function URL request = %d, want 404", code)
	}
	if n := runs.Load(); n != 0 {
		t.Fatalf("the job ran %d times on a Function URL request", n)
	}
	code, failed := postEvent(t, s.url+"/events", "null", wakes[0].JobID, `{"type":"tick"}`)
	if code != http.StatusOK || len(failed) != 0 {
		t.Errorf("event = %d %v, want 200 and no failures", code, failed)
	}
	if n := runs.Load(); n != 1 {
		t.Errorf("runs after the signal = %d, want 1", n)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.url+"/_internal/tick", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("tick = %d, want 404", resp.StatusCode)
	}
}
