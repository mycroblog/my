// SPDX-License-Identifier: AGPL-3.0-only

package cloudtasks_test

import (
	"context"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"

	"github.com/mycroblog/my/internal/adapter/queue/cloudtasks"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/pkg/testkit"
)

const (
	queuePath      = "projects/my-project/locations/asia-northeast1/queues/my-jobs"
	jobsURL        = "https://my.example/_internal/jobs"
	serviceAccount = "my-tasks@my-project.iam.gserviceaccount.com"
)

// fakeTasks stands in for the CreateTask method of the Cloud Tasks API.
type fakeTasks struct {
	cloudtaskspb.UnimplementedCloudTasksServer

	mu     sync.Mutex
	got    []*cloudtaskspb.CreateTaskRequest
	refuse map[string]bool // job IDs whose task is refused
}

func (f *fakeTasks) CreateTask(_ context.Context, req *cloudtaskspb.CreateTaskRequest) (*cloudtaskspb.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refuse[string(req.GetTask().GetHttpRequest().GetBody())] {
		return nil, status.Error(codes.ResourceExhausted, "quota")
	}
	f.got = append(f.got, proto.CloneOf(req))
	t := proto.CloneOf(req.GetTask())
	t.Name = req.GetParent() + "/tasks/" + strings.Repeat("1", len(f.got))
	return t, nil
}

func (f *fakeTasks) requests() []*cloudtaskspb.CreateTaskRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.got)
}

func startFake(t *testing.T) (*fakeTasks, []option.ClientOption) {
	t.Helper()
	f := &fakeTasks{refuse: map[string]bool{}}
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	cloudtaskspb.RegisterCloudTasksServer(gs, f)
	go func() { _ = gs.Serve(lis) }()
	conn, err := grpc.NewClient("passthrough:///cloudtasks",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		gs.Stop()
	})
	return f, []option.ClientOption{option.WithGRPCConn(conn), option.WithoutAuthentication()}
}

func newWaker(t *testing.T, opts cloudtasks.Options) (*cloudtasks.Waker, *fakeTasks) {
	t.Helper()
	f, co := startFake(t)
	opts.ClientOptions = co
	w, err := cloudtasks.New(t.Context(), opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w, f
}

var now = time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)

func TestWakeTaskShape(t *testing.T) {
	t.Parallel()
	clock := testkit.NewClock(now)
	w, f := newWaker(t, cloudtasks.Options{
		Queue: queuePath, URL: jobsURL, ServiceAccount: serviceAccount, Audience: "https://my.example",
		DispatchDeadline: 10 * time.Minute, Clock: clock,
	})
	at := now.Add(90 * time.Second)
	if err := w.Wake(t.Context(), []queue.Wake{{JobID: "019a2f00-0000-7000-8000-000000000001", At: at}}); err != nil {
		t.Fatalf("Wake() error = %v", err)
	}
	reqs := f.requests()
	if len(reqs) != 1 {
		t.Fatalf("CreateTask calls = %d, want 1", len(reqs))
	}
	req := reqs[0]
	if req.GetParent() != queuePath {
		t.Errorf("parent = %q, want %q", req.GetParent(), queuePath)
	}
	task := req.GetTask()
	if task.GetName() != "" {
		t.Errorf("task name = %q, want none: a name would refuse the next signal of the job", task.GetName())
	}
	if got := task.GetScheduleTime().AsTime(); !got.Equal(at) {
		t.Errorf("scheduleTime = %s, want %s", got, at)
	}
	if got := task.GetDispatchDeadline().AsDuration(); got != 10*time.Minute {
		t.Errorf("dispatchDeadline = %s, want 10m", got)
	}
	h := task.GetHttpRequest()
	if h.GetUrl() != jobsURL || h.GetHttpMethod() != cloudtaskspb.HttpMethod_POST {
		t.Errorf("target = %s %s, want POST %s", h.GetHttpMethod(), h.GetUrl(), jobsURL)
	}
	if string(h.GetBody()) != "019a2f00-0000-7000-8000-000000000001" {
		t.Errorf("body = %q, want the job ID", h.GetBody())
	}
	oidc := h.GetOidcToken()
	if oidc.GetServiceAccountEmail() != serviceAccount || oidc.GetAudience() != "https://my.example" {
		t.Errorf("oidcToken = %v, want the service account and the audience", oidc)
	}
}

func TestWakeDefaultsAndDelayLimit(t *testing.T) {
	t.Parallel()
	w, f := newWaker(t, cloudtasks.Options{Queue: queuePath, URL: jobsURL, ServiceAccount: serviceAccount, Clock: testkit.NewClock(now)})
	if err := w.Wake(t.Context(), []queue.Wake{{JobID: "a", At: now.Add(45 * 24 * time.Hour)}}); err != nil {
		t.Fatalf("Wake() error = %v", err)
	}
	task := f.requests()[0].GetTask()
	if got, want := task.GetScheduleTime().AsTime(), now.Add(cloudtasks.MaxDelay); !got.Equal(want) {
		t.Errorf("scheduleTime of a signal beyond the delay limit = %s, want %s", got, want)
	}
	if task.GetDispatchDeadline() != nil {
		t.Errorf("dispatchDeadline = %v, want the queue's default", task.GetDispatchDeadline())
	}
	if aud := task.GetHttpRequest().GetOidcToken().GetAudience(); aud != "" {
		t.Errorf("audience = %q, want none (Cloud Tasks then uses the URL)", aud)
	}
}

func TestWakeMany(t *testing.T) {
	t.Parallel()
	w, f := newWaker(t, cloudtasks.Options{Queue: queuePath, URL: jobsURL, ServiceAccount: serviceAccount, Clock: testkit.NewClock(now), Parallel: 3})
	f.refuse["j3"] = true
	var wakes []queue.Wake
	var want []string
	for i := range 20 {
		id := "j" + string(rune('a'+i))
		if i == 3 {
			id = "j3"
		} else {
			want = append(want, id)
		}
		wakes = append(wakes, queue.Wake{JobID: id, At: now})
	}
	err := w.Wake(t.Context(), wakes)
	if err == nil || !strings.Contains(err.Error(), "j3") {
		t.Errorf("Wake() error = %v, want the refused signal of j3", err)
	}
	var got []string
	for _, r := range f.requests() {
		got = append(got, string(r.GetTask().GetHttpRequest().GetBody()))
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("tasks = %q, want every other signal %q", got, want)
	}
}

func TestNewChecks(t *testing.T) {
	t.Parallel()
	_, co := startFake(t)
	for name, o := range map[string]cloudtasks.Options{
		"queue":    {Queue: "my-jobs", URL: jobsURL, ServiceAccount: serviceAccount, Clock: testkit.NewClock(now)},
		"url":      {Queue: queuePath, ServiceAccount: serviceAccount, Clock: testkit.NewClock(now)},
		"account":  {Queue: queuePath, URL: jobsURL, Clock: testkit.NewClock(now)},
		"deadline": {Queue: queuePath, URL: jobsURL, ServiceAccount: serviceAccount, Clock: testkit.NewClock(now), DispatchDeadline: time.Hour},
	} {
		o.ClientOptions = co
		if w, err := cloudtasks.New(t.Context(), o); err == nil {
			_ = w.Close()
			t.Errorf("New(bad %s) = nil error", name)
		}
	}
}

func TestCheckQueuePath(t *testing.T) {
	t.Parallel()
	for _, v := range []string{queuePath, "projects/example.com:p/locations/us-central1/queues/q-1"} {
		if err := cloudtasks.CheckQueuePath(v); err != nil {
			t.Errorf("CheckQueuePath(%q) = %v", v, err)
		}
	}
	for _, v := range []string{"", "my-jobs", queuePath + "/tasks/1", "projects/p/locations/us/queues/q_1", "projects/p/locations/us/queues/" + strings.Repeat("q", 101)} {
		if err := cloudtasks.CheckQueuePath(v); err == nil {
			t.Errorf("CheckQueuePath(%q) = nil, want an error", v)
		}
	}
}
