// SPDX-License-Identifier: AGPL-3.0-only

package sqs_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/mycroblog/my/internal/adapter/queue/sqs"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/pkg/testkit"
)

const queueURL = "https://sqs.ap-northeast-1.amazonaws.com/123456789012/my-jobs"

// message is one message that the fake accepted.
type message struct {
	body  string
	delay int32
}

// fakeSQS stands in for SendMessage and SendMessageBatch of the SQS API, with its limits: a batch
// of 1 to 10 entries with distinct IDs, and DelaySeconds from 0 to 900.
type fakeSQS struct {
	mu      sync.Mutex
	single  []message
	batches [][]message
	refuse  map[string]bool // bodies that a batch refuses
	err     error           // the error of every call
}

func (f *fakeSQS) SendMessage(_ context.Context, in *awssqs.SendMessageInput, _ ...func(*awssqs.Options)) (*awssqs.SendMessageOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	if aws.ToString(in.QueueUrl) != queueURL || in.DelaySeconds < 0 || in.DelaySeconds > 900 {
		return nil, fmt.Errorf("invalid request: queue %s, delay %d", aws.ToString(in.QueueUrl), in.DelaySeconds)
	}
	f.single = append(f.single, message{body: aws.ToString(in.MessageBody), delay: in.DelaySeconds})
	return &awssqs.SendMessageOutput{MessageId: aws.String("m")}, nil
}

func (f *fakeSQS) SendMessageBatch(_ context.Context, in *awssqs.SendMessageBatchInput, _ ...func(*awssqs.Options)) (*awssqs.SendMessageBatchOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	if aws.ToString(in.QueueUrl) != queueURL {
		return nil, fmt.Errorf("invalid request: queue %s", aws.ToString(in.QueueUrl))
	}
	if n := len(in.Entries); n == 0 || n > 10 {
		return nil, &types.TooManyEntriesInBatchRequest{Message: aws.String(fmt.Sprintf("%d entries", n))}
	}
	ids := map[string]bool{}
	out := &awssqs.SendMessageBatchOutput{}
	var batch []message
	for _, e := range in.Entries {
		id := aws.ToString(e.Id)
		if id == "" || ids[id] {
			return nil, &types.BatchEntryIdsNotDistinct{Message: aws.String(id)}
		}
		ids[id] = true
		if e.DelaySeconds < 0 || e.DelaySeconds > 900 {
			return nil, fmt.Errorf("invalid DelaySeconds %d", e.DelaySeconds)
		}
		if f.refuse[aws.ToString(e.MessageBody)] {
			out.Failed = append(out.Failed, types.BatchResultErrorEntry{Id: e.Id, Code: aws.String("InternalError"), Message: aws.String("try again"), SenderFault: false})
			continue
		}
		batch = append(batch, message{body: aws.ToString(e.MessageBody), delay: e.DelaySeconds})
		out.Successful = append(out.Successful, types.SendMessageBatchResultEntry{Id: e.Id, MessageId: aws.String("m")})
	}
	f.batches = append(f.batches, batch)
	return out, nil
}

func newWaker(t *testing.T) (*sqs.Waker, *fakeSQS, *testkit.Clock) {
	t.Helper()
	f := &fakeSQS{refuse: map[string]bool{}}
	clock := testkit.NewClock(time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC))
	w, err := sqs.New(f, sqs.Options{QueueURL: queueURL, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	return w, f, clock
}

// TestWakeOne checks the message of one signal: the job ID as its body and the seconds until the
// signal, rounded up, as its delay.
func TestWakeOne(t *testing.T) {
	cases := []struct {
		name  string
		after time.Duration
		want  int32
	}{
		{"due", 0, 0},
		{"past", -time.Hour, 0},
		{"later", 90 * time.Second, 90},
		{"fraction", 1500 * time.Millisecond, 2},
		{"at the limit", 15 * time.Minute, 900},
		{"beyond the limit", 3 * time.Hour, 900},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, f, clock := newWaker(t)
			if err := w.Wake(t.Context(), []queue.Wake{{JobID: "job-1", At: clock.Now().Add(c.after)}}); err != nil {
				t.Fatalf("Wake: %v", err)
			}
			if len(f.single) != 1 || len(f.batches) != 0 {
				t.Fatalf("sent %d messages and %d batches, want 1 message", len(f.single), len(f.batches))
			}
			if got := f.single[0]; got != (message{body: "job-1", delay: c.want}) {
				t.Errorf("message = %+v, want body job-1, delay %d", got, c.want)
			}
		})
	}
}

// TestWakeBatches checks that several signals go in batches of at most 10 messages, each with
// its own delay.
func TestWakeBatches(t *testing.T) {
	w, f, clock := newWaker(t)
	var wakes []queue.Wake
	for i := range 23 {
		wakes = append(wakes, queue.Wake{JobID: fmt.Sprintf("job-%02d", i), At: clock.Now().Add(time.Duration(i) * time.Minute)})
	}
	if err := w.Wake(t.Context(), wakes); err != nil {
		t.Fatalf("Wake: %v", err)
	}
	if len(f.single) != 0 {
		t.Errorf("sent %d single messages, want none", len(f.single))
	}
	var sizes []int
	i := 0
	for _, b := range f.batches {
		sizes = append(sizes, len(b))
		for _, m := range b {
			want := message{body: fmt.Sprintf("job-%02d", i), delay: int32(min(i*60, 900))}
			if m != want {
				t.Errorf("message %d = %+v, want %+v", i, m, want)
			}
			i++
		}
	}
	if fmt.Sprint(sizes) != "[10 10 3]" {
		t.Errorf("batch sizes = %v, want [10 10 3]", sizes)
	}
}

// TestWakeFailures checks that Wake reports the signals that SQS refused and still sends the
// other batches.
func TestWakeFailures(t *testing.T) {
	w, f, clock := newWaker(t)
	f.refuse["job-03"] = true
	var wakes []queue.Wake
	for i := range 12 {
		wakes = append(wakes, queue.Wake{JobID: fmt.Sprintf("job-%02d", i), At: clock.Now()})
	}
	err := w.Wake(t.Context(), wakes)
	if err == nil || !strings.Contains(err.Error(), "job-03") || !strings.Contains(err.Error(), "InternalError") {
		t.Errorf("Wake = %v, want the refusal of job-03", err)
	}
	if len(f.batches) != 2 || len(f.batches[0]) != 9 || len(f.batches[1]) != 2 {
		t.Errorf("batches = %+v, want 9 and 2 messages sent", f.batches)
	}

	f.err = errors.New("unreachable")
	if err := w.Wake(t.Context(), wakes[:1]); err == nil || !strings.Contains(err.Error(), "job-00") {
		t.Errorf("Wake of one on an unreachable SQS = %v, want its error", err)
	}
	if err := w.Wake(t.Context(), wakes); err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("Wake of several on an unreachable SQS = %v, want its error", err)
	}
	if err := w.Wake(t.Context(), nil); err != nil {
		t.Errorf("Wake of nothing = %v, want nil", err)
	}
}

func TestCheckQueueURL(t *testing.T) {
	for _, v := range []string{
		queueURL,
		"https://sqs.us-gov-west-1.amazonaws.com/123456789012/a_b-c",
		"https://sqs-fips.us-east-1.amazonaws.com/123456789012/q",
		"https://sqs.cn-north-1.amazonaws.com.cn/123456789012/q",
	} {
		if err := sqs.CheckQueueURL(v); err != nil {
			t.Errorf("CheckQueueURL(%q) = %v, want nil", v, err)
		}
	}
	for _, v := range []string{
		"",
		"http://sqs.ap-northeast-1.amazonaws.com/123456789012/my-jobs",
		"https://sqs.ap-northeast-1.amazonaws.com/123456789012/my-jobs.fifo",
		"https://sqs.ap-northeast-1.amazonaws.com/12345/my-jobs",
		"https://sqs.ap-northeast-1.amazonaws.com/123456789012/",
		"https://sqs.ap-northeast-1.amazonaws.com/123456789012/my-jobs?x=1",
		"https://example.com/123456789012/my-jobs",
		"arn:aws:sqs:ap-northeast-1:123456789012:my-jobs",
		"https://sqs.ap-northeast-1.amazonaws.com/123456789012/" + strings.Repeat("q", 81),
	} {
		if err := sqs.CheckQueueURL(v); err == nil {
			t.Errorf("CheckQueueURL(%q) = nil, want an error", v)
		}
	}
	if _, err := sqs.New(&fakeSQS{}, sqs.Options{QueueURL: "bad", Clock: testkit.NewClock(time.Now())}); err == nil {
		t.Error("New with a bad queue URL = nil error")
	}
	if _, err := sqs.NewFromConfig(aws.Config{}, sqs.Options{QueueURL: queueURL, Clock: testkit.NewClock(time.Now())}); err != nil {
		t.Errorf("NewFromConfig = %v", err)
	}
}
