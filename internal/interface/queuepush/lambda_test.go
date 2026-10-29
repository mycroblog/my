// SPDX-License-Identifier: AGPL-3.0-only

package queuepush_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/interface/queuepush"
)

var start = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

// lambdaReceiver records the job IDs, fails those in fail, and moves the clock by step a run.
type lambdaReceiver struct {
	now  *time.Time
	step time.Duration
	fail map[string]bool
	ids  []string
}

func (f *lambdaReceiver) Receive(_ context.Context, id string) error {
	f.ids = append(f.ids, id)
	*f.now = f.now.Add(f.step)
	if f.fail[id] {
		return errors.New("database unreachable")
	}
	return nil
}

func (f *lambdaReceiver) Lease() time.Duration { return 5 * time.Minute }

func lambdaSetup(t *testing.T, o queuepush.LambdaOptions) (*http.ServeMux, *lambdaReceiver, *fakeTrigger) {
	t.Helper()
	now := start
	r, tr := &lambdaReceiver{now: &now, fail: map[string]bool{}}, &fakeTrigger{}
	o.Receiver, o.Trigger, o.Now = r, tr, func() time.Time { return now }
	mux := http.NewServeMux()
	queuepush.NewLambda(o).Register(mux)
	return mux, r, tr
}

// sqsEvent is an SQS event of Lambda as the Lambda Web Adapter passes it through, one record per
// body, whose message IDs are m1, m2, and so on.
func sqsEvent(bodies ...string) string {
	type record struct {
		MessageID      string `json:"messageId"`
		ReceiptHandle  string `json:"receiptHandle"`
		Body           string `json:"body"`
		EventSource    string `json:"eventSource"`
		EventSourceARN string `json:"eventSourceARN"`
		AWSRegion      string `json:"awsRegion"`
	}
	var ev struct {
		Records []record `json:"Records"`
	}
	for i, b := range bodies {
		ev.Records = append(ev.Records, record{
			MessageID: fmt.Sprintf("m%d", i+1), ReceiptHandle: "AQEB", Body: b, EventSource: "aws:sqs",
			EventSourceARN: "arn:aws:sqs:ap-northeast-1:123456789012:my-jobs", AWSRegion: "ap-northeast-1",
		})
	}
	b, _ := json.Marshal(ev)
	return string(b)
}

// lambdaContext is the x-amzn-lambda-context of an invocation whose deadline is at deadline.
func lambdaContext(deadline time.Time) string {
	return fmt.Sprintf(`{"request_id":"r1","deadline":%d,"invoked_function_arn":"arn:aws:lambda:ap-northeast-1:123456789012:function:my","xray_trace_id":null,"client_context":null,"identity":null,"env_config":{}}`, deadline.UnixMilli())
}

func postEvent(mux http.Handler, path, requestContext, lc, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if requestContext != "" {
		req.Header.Set("X-Amzn-Request-Context", requestContext)
	}
	if lc != "" {
		req.Header.Set("X-Amzn-Lambda-Context", lc)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// failures decodes the partial batch response of rec, which must be JSON.
func failures(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var resp struct {
		BatchItemFailures *[]struct {
			ItemIdentifier string `json:"itemIdentifier"`
		} `json:"batchItemFailures"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.BatchItemFailures == nil {
		t.Fatalf("body = %s, want a partial batch response (%v)", rec.Body, err)
	}
	ids := []string{}
	for _, f := range *resp.BatchItemFailures {
		ids = append(ids, f.ItemIdentifier)
	}
	return ids
}

func TestLambdaAllSucceed(t *testing.T) {
	t.Parallel()
	mux, r, tr := lambdaSetup(t, queuepush.LambdaOptions{})
	rec := postEvent(mux, "/events", "null", lambdaContext(start.Add(15*time.Minute)), sqsEvent("job-1", " job-2\n"))
	if got := failures(t, rec); len(got) != 0 {
		t.Errorf("failures = %v, want none", got)
	}
	if !slices.Equal(r.ids, []string{"job-1", "job-2"}) || tr.ticks != 0 {
		t.Errorf("signals = %v, ticks = %d", r.ids, tr.ticks)
	}
}

func TestLambdaPartialFailure(t *testing.T) {
	t.Parallel()
	mux, r, _ := lambdaSetup(t, queuepush.LambdaOptions{})
	r.fail["job-2"] = true
	rec := postEvent(mux, "/events", "null", lambdaContext(start.Add(15*time.Minute)), sqsEvent("job-1", "job-2", "job-3"))
	if got := failures(t, rec); !slices.Equal(got, []string{"m2"}) {
		t.Errorf("failures = %v, want [m2]", got)
	}
	if !slices.Equal(r.ids, []string{"job-1", "job-2", "job-3"}) {
		t.Errorf("signals = %v", r.ids)
	}
}

// TestLambdaDeadline checks that the records left when the time to the deadline is short of a
// lease go back unrun, from the deadline of the invocation or else from the budget.
func TestLambdaDeadline(t *testing.T) {
	t.Parallel()
	t.Run("deadline of the invocation", func(t *testing.T) {
		t.Parallel()
		mux, r, _ := lambdaSetup(t, queuepush.LambdaOptions{Budget: time.Hour})
		r.step = time.Minute
		// 7 minutes: the first two records start with at least 5m30s left, the third does not.
		rec := postEvent(mux, "/events", "null", lambdaContext(start.Add(7*time.Minute)), sqsEvent("job-1", "job-2", "job-3", "job-4"))
		if got := failures(t, rec); !slices.Equal(got, []string{"m3", "m4"}) {
			t.Errorf("failures = %v, want [m3 m4]", got)
		}
		if !slices.Equal(r.ids, []string{"job-1", "job-2"}) {
			t.Errorf("signals = %v", r.ids)
		}
	})
	t.Run("budget", func(t *testing.T) {
		t.Parallel()
		mux, r, _ := lambdaSetup(t, queuepush.LambdaOptions{Budget: 6 * time.Minute})
		r.step = time.Minute
		rec := postEvent(mux, "/events", "null", "", sqsEvent("job-1", "job-2"))
		if got := failures(t, rec); !slices.Equal(got, []string{"m2"}) {
			t.Errorf("failures = %v, want [m2]", got)
		}
		if !slices.Equal(r.ids, []string{"job-1"}) {
			t.Errorf("signals = %v", r.ids)
		}
	})
}

func TestLambdaTick(t *testing.T) {
	t.Parallel()
	mux, r, tr := lambdaSetup(t, queuepush.LambdaOptions{})
	rec := postEvent(mux, "/events", "null", lambdaContext(start.Add(15*time.Minute)),
		sqsEvent(`{"type":"tick"}`, "job-1", `{ "type": "tick" }`, `{"type":"other"}`))
	if got := failures(t, rec); len(got) != 0 {
		t.Errorf("failures = %v, want none", got)
	}
	// Two ticks of one batch tick once; a message of another type is dropped.
	if tr.ticks != 1 || !slices.Equal(r.ids, []string{"job-1"}) {
		t.Errorf("ticks = %d, signals = %v", tr.ticks, r.ids)
	}

	mux, _, tr = lambdaSetup(t, queuepush.LambdaOptions{})
	tr.err = errors.New("sweep failed")
	rec = postEvent(mux, "/events", "null", "", sqsEvent(`{"type":"tick"}`))
	if got := failures(t, rec); !slices.Equal(got, []string{"m1"}) {
		t.Errorf("failures of a failed tick = %v, want [m1]", got)
	}
}

// TestLambdaRefusesOtherRequests checks that only the requests that the Lambda Web Adapter passes
// through, whose x-amzn-request-context is null, reach the receiver.
func TestLambdaRefusesOtherRequests(t *testing.T) {
	t.Parallel()
	mux, r, tr := lambdaSetup(t, queuepush.LambdaOptions{Path: "/lwa/events"})
	body := sqsEvent("job-1", `{"type":"tick"}`)
	for name, rc := range map[string]string{
		"Function URL": `{"routeKey":"$default","accountId":"anonymous","http":{"method":"POST","path":"/lwa/events"}}`,
		"no context":   "",
		"empty object": "{}",
	} {
		if rec := postEvent(mux, "/lwa/events", rc, "", body); rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", name, rec.Code)
		}
	}
	if len(r.ids) != 0 || tr.ticks != 0 {
		t.Errorf("signals = %v, ticks = %d, want none", r.ids, tr.ticks)
	}
	if rec := postEvent(mux, "/events", "null", "", body); rec.Code != http.StatusNotFound {
		t.Errorf("default path = %d, want 404 for another path", rec.Code)
	}
	if got := failures(t, postEvent(mux, "/lwa/events", "null", "", body)); len(got) != 0 || len(r.ids) != 1 {
		t.Errorf("failures = %v, signals = %v", got, r.ids)
	}
}

// TestLambdaNoSQSEvent checks that a body that is no SQS event still gets a JSON answer.
func TestLambdaNoSQSEvent(t *testing.T) {
	t.Parallel()
	mux, r, _ := lambdaSetup(t, queuepush.LambdaOptions{})
	for _, body := range []string{"", "not json", `{"detail-type":"Scheduled Event"}`} {
		if got := failures(t, postEvent(mux, "/events", "null", "", body)); len(got) != 0 {
			t.Errorf("%q: failures = %v", body, got)
		}
	}
	if len(r.ids) != 0 {
		t.Errorf("signals = %v", r.ids)
	}
}
