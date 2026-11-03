// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakePermissions grants each user the permissions listed for them.
type fakePermissions map[string][]identity.Permission

func (f fakePermissions) HasPermission(_ context.Context, userID string, p identity.Permission) (bool, error) {
	return slices.Contains(f[userID], p), nil
}

func (f fakePermissions) Permissions(_ context.Context, userID string) ([]identity.Permission, error) {
	return f[userID], nil
}

// fakeHealth keeps the health of servers (in domain order) and their events (oldest first) with
// the semantics of appfederation.Health.
type fakeHealth struct {
	servers []federation.InstanceHealth
	events  []federation.AvailabilityEvent
	now     time.Time
}

func (f *fakeHealth) Find(_ context.Context, domain string) (federation.InstanceHealth, error) {
	for _, h := range f.servers {
		if h.Domain == domain {
			return h, nil
		}
	}
	return federation.InstanceHealth{Domain: domain, State: federation.InstanceAvailable}, nil
}

func (f *fakeHealth) List(_ context.Context, state federation.InstanceState, after string, limit int) ([]federation.InstanceHealth, error) {
	if state != "" && state != federation.InstanceAvailable && state != federation.InstanceUnavailable {
		return nil, appfederation.ErrInvalidInstanceState
	}
	var out []federation.InstanceHealth
	for _, h := range f.servers {
		if (state == "" || h.State == state) && h.Domain > after && len(out) < limit {
			out = append(out, h)
		}
	}
	return out, nil
}

func (f *fakeHealth) Events(_ context.Context, domain, before string, limit int) ([]federation.AvailabilityEvent, error) {
	var out []federation.AvailabilityEvent
	for _, e := range slices.Backward(f.events) {
		if e.Domain == domain && (before == "" || e.ID < before) && len(out) < limit {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeHealth) MarkAvailable(ctx context.Context, domain string) (federation.InstanceHealth, error) {
	for i, h := range f.servers {
		if h.Domain != domain {
			continue
		}
		next, changed, marked := h.Reached(f.now)
		if changed {
			f.servers[i] = next
		}
		if marked {
			f.events = append(f.events, federation.AvailabilityEvent{
				ID: "01890a5d-ac96-774b-bcce-b302099a7fff", Domain: domain, Kind: federation.MarkedAvailable,
				Reason: federation.ReasonOperator, OccurredAt: f.now,
			})
		}
	}
	return f.Find(ctx, domain)
}

// fakeDeadJobs keeps dead jobs, most recently failed first, and the jobs requeued from them.
type fakeDeadJobs struct {
	dead     []queue.DeadJob
	enqueued []queue.DeadJob
}

func (f *fakeDeadJobs) ListDeadJobs(_ context.Context, jobType string, after queue.DeadJobPosition, limit int) ([]queue.DeadJob, error) {
	var out []queue.DeadJob
	for _, d := range f.dead {
		c := d.FailedAt.Compare(after.FailedAt)
		if (jobType == "" || d.Type == jobType) && (after.ID == "" || c < 0 || (c == 0 && d.ID < after.ID)) && len(out) < limit {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeDeadJobs) RequeueDeadJob(_ context.Context, id string) error {
	i := slices.IndexFunc(f.dead, func(d queue.DeadJob) bool { return d.ID == id })
	if i < 0 {
		return queue.ErrDeadJobNotFound
	}
	f.enqueued = append(f.enqueued, f.dead[i])
	f.dead = slices.Delete(f.dead, i, i+1)
	return nil
}

func (f *fakeDeadJobs) DeleteDeadJob(context.Context, string) error {
	return errors.New("fakeDeadJobs: DeleteDeadJob is not used by the admin API")
}

func (f *fakeDeadJobs) DeleteDeadJobsBefore(context.Context, time.Time, int) (int64, error) {
	return 0, errors.New("fakeDeadJobs: DeleteDeadJobsBefore is not used by the admin API")
}

// fakeInspector lists jobs in order of RunAt, and records the filters and times it was asked for.
type fakeInspector struct {
	jobs    []queue.PendingJob
	summary queue.JobSummary
	filters []queue.JobFilter
	nows    []time.Time
}

func (f *fakeInspector) ListJobs(_ context.Context, now time.Time, filter queue.JobFilter, after queue.JobPosition, limit int) ([]queue.PendingJob, error) {
	f.filters, f.nows = append(f.filters, filter), append(f.nows, now)
	var out []queue.PendingJob
	for _, j := range f.jobs {
		c := j.RunAt.Compare(after.RunAt)
		if (filter.State == "" || j.State == filter.State) && (filter.Type == "" || j.Type == filter.Type) &&
			(after.ID == "" || c > 0 || (c == 0 && j.ID > after.ID)) && len(out) < limit {
			out = append(out, j)
		}
	}
	return out, nil
}

func (f *fakeInspector) SummarizeJobs(_ context.Context, now time.Time) (queue.JobSummary, error) {
	f.nows = append(f.nows, now)
	return f.summary, nil
}

// fakeAttempts lists the records of every job, the most recently finished first.
type fakeAttempts struct{ records []queue.Attempt }

func (f *fakeAttempts) RecordAttempt(context.Context, queue.Attempt) error {
	return errors.New("fakeAttempts: RecordAttempt is not used by the admin API")
}

func (f *fakeAttempts) ListAttempts(_ context.Context, jobID string, after queue.AttemptPosition, limit int) ([]queue.Attempt, error) {
	var out []queue.Attempt
	for _, a := range f.records {
		c := a.FinishedAt.Compare(after.FinishedAt)
		if a.JobID == jobID && (after.Attempt == 0 || c < 0 || (c == 0 && a.Attempt < after.Attempt)) && len(out) < limit {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeAttempts) DeleteAttemptsBefore(context.Context, time.Time, int) (int64, error) {
	return 0, errors.New("fakeAttempts: DeleteAttemptsBefore is not used by the admin API")
}

type fixedClock time.Time

func (c fixedClock) Now() time.Time { return time.Time(c) }

var adminNow = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

const (
	deadJobA = "01890a5d-ac96-774b-bcce-b302099a6001"
	deadJobB = "01890a5d-ac96-774b-bcce-b302099a6002"
	deadJobC = "01890a5d-ac96-774b-bcce-b302099a6003"
	eventOne = "01890a5d-ac96-774b-bcce-b302099a7001"
	eventTwo = "01890a5d-ac96-774b-bcce-b302099a7002"
	jobOne   = "01890a5d-ac96-774b-bcce-b302099a8001"
	jobTwo   = "01890a5d-ac96-774b-bcce-b302099a8002"
	jobThree = "01890a5d-ac96-774b-bcce-b302099a8003"
)

// newAdminEnv is newAuthEnv whose user alice holds every permission and bob none, with the
// tokens "admin" (alice) and "bob-admin" (bob) of the admin scope and "admin-read" (alice) of
// admin:read alone.
func newAdminEnv(t *testing.T) (*authEnv, *fakeHealth, *fakeDeadJobs) {
	e, health, dead, _ := newAdminEnvWithQueue(t)
	return e, health, dead
}

// newAdminEnvWithQueue is newAdminEnv that also returns the pending jobs of the queue dashboard:
// a running job, a retrying one, and a queued one, with the records of the attempts of the
// retrying job and of the dead job deadJobC.
func newAdminEnvWithQueue(t *testing.T) (*authEnv, *fakeHealth, *fakeDeadJobs, *fakeInspector) {
	t.Helper()
	health := &fakeHealth{
		now: adminNow,
		servers: []federation.InstanceHealth{
			{Domain: "a.example", State: federation.InstanceUnavailable, Since: adminNow.Add(-time.Hour), FailureDays: 7, LastFailureDay: "20261016"},
			{Domain: "b.example", State: federation.InstanceAvailable, Since: adminNow.Add(-48 * time.Hour), FailureDays: 2, LastFailureDay: "20261015"},
			{Domain: "c.example", State: federation.InstanceUnavailable, Since: adminNow.Add(-2 * time.Hour), FailureDays: 9, LastFailureDay: "20261017"},
		},
		events: []federation.AvailabilityEvent{
			{ID: eventOne, Domain: "a.example", Kind: federation.MarkedUnavailable, Reason: federation.ReasonFailureDays, OccurredAt: adminNow.Add(-3 * time.Hour)},
			{ID: eventTwo, Domain: "a.example", Kind: federation.MarkedUnavailable, Reason: federation.ReasonFailureDays, OccurredAt: adminNow.Add(-time.Hour)},
		},
	}
	failed := adminNow.Add(-time.Minute)
	dead := &fakeDeadJobs{dead: []queue.DeadJob{
		{ID: deadJobC, Type: "federation.deliver", Payload: json.RawMessage(`{"delivery_id":"x"}`), IdempotencyKey: "deliver:x", OrderingKey: "inbox", Attempt: 9, LastError: "502", CreatedAt: failed.Add(-96 * time.Hour), FailedAt: failed},
		{ID: deadJobB, Type: "federation.deliver", Payload: json.RawMessage(`{}`), Attempt: 1, LastError: "bug", CreatedAt: failed, FailedAt: failed},
		{ID: deadJobA, Type: "kv.cleanup", Payload: json.RawMessage(`null`), Attempt: 3, LastError: "db down", CreatedAt: failed.Add(-time.Hour), FailedAt: failed.Add(-time.Second)},
	}}
	inspector := &fakeInspector{
		jobs: []queue.PendingJob{
			{ID: jobOne, Type: "federation.deliver", Payload: json.RawMessage(`{"delivery_id":"y"}`), State: queue.JobStateRunning, Attempt: 1,
				RunAt: adminNow.Add(-time.Minute), Worker: "worker-1", LockedUntil: adminNow.Add(4 * time.Minute), CreatedAt: adminNow.Add(-time.Minute)},
			{ID: jobTwo, Type: "federation.deliver", Payload: json.RawMessage(`{}`), OrderingKey: "inbox", State: queue.JobStateRetrying, Attempt: 2,
				RunAt: adminNow.Add(time.Minute), LastError: "503", CreatedAt: adminNow.Add(-time.Hour)},
			{ID: jobThree, Type: "kv.cleanup", Payload: json.RawMessage(`null`), IdempotencyKey: "kv", State: queue.JobStateQueued,
				RunAt: adminNow.Add(time.Hour), CreatedAt: adminNow},
		},
		summary: queue.JobSummary{
			Counts: []queue.JobCount{
				{State: queue.JobStateQueued, Type: "kv.cleanup", Count: 1},
				{State: queue.JobStateRunning, Type: "federation.deliver", Count: 1},
				{State: queue.JobStateRetrying, Type: "federation.deliver", Count: 1},
				{State: queue.JobStateDead, Type: "federation.deliver", Count: 2},
				{State: queue.JobStateDead, Type: "kv.cleanup", Count: 1},
			},
			OldestReadyAt: adminNow.Add(-10 * time.Minute),
			OldestReadyByType: map[string]time.Time{
				"kv.cleanup":         adminNow.Add(-time.Minute),
				"federation.deliver": adminNow.Add(-10 * time.Minute),
			},
		},
	}
	attempts := &fakeAttempts{records: []queue.Attempt{
		{JobID: jobTwo, JobType: "federation.deliver", Attempt: 2, Outcome: queue.OutcomeRetry, Error: "503", Worker: "worker-2",
			StartedAt: adminNow.Add(-2 * time.Minute), FinishedAt: adminNow.Add(-time.Minute)},
		{JobID: jobTwo, JobType: "federation.deliver", Attempt: 1, Outcome: queue.OutcomeRetry, Error: "timeout", Worker: "worker-1",
			StartedAt: adminNow.Add(-time.Hour), FinishedAt: adminNow.Add(-time.Hour)},
		{JobID: deadJobC, JobType: "federation.deliver", Attempt: 9, Outcome: queue.OutcomeDead, Error: "502", Worker: "worker-1",
			StartedAt: failed, FinishedAt: failed},
	}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
		o.InstanceHealth = health
		o.DeadJobs = dead
		o.Queue = clientapi.QueueDashboard{Adapter: "db", Inspector: inspector, Attempts: attempts, Clock: fixedClock(adminNow)}
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	return e, health, dead, inspector
}

func (e *authEnv) admin(t *testing.T, method, path, token string, header map[string]string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, clientapi.BasePath+path, nil)
	req.RemoteAddr = "192.0.2.1:1234"
	req.Header.Set("Authorization", "Bearer "+token)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	return e.spec.Serve(t, e.h, req)
}

func decodeJSON[T any](t *testing.T, res *http.Response) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

var adminOperations = []struct{ method, path string }{
	{http.MethodGet, "/admin/instances"},
	{http.MethodGet, "/admin/instances/a.example"},
	{http.MethodGet, "/admin/instances/a.example/availability_events"},
	{http.MethodPost, "/admin/instances/a.example/mark_available"},
	{http.MethodGet, "/admin/queue"},
	{http.MethodGet, "/admin/jobs"},
	{http.MethodGet, "/admin/jobs/" + jobTwo + "/attempts"},
	{http.MethodPost, "/admin/jobs/" + deadJobA + "/retry"},
}

// TestVerifyAccountCredentialsListsThePermissions checks that the viewer's own account lists the
// permissions of the user's roles; TestVerifyAccountCredentialsHasTheSource checks that a user
// without roles has none.
func TestVerifyAccountCredentialsListsThePermissions(t *testing.T) {
	e, _, _ := newAdminEnv(t)
	res := e.get(t, "/accounts/verify_credentials", map[string]string{"Authorization": "Bearer read"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	got := decodeJSON[clientapi.CredentialAccount](t, res).Permissions
	want := []clientapi.Permission{}
	for _, p := range identity.AllPermissions() {
		want = append(want, clientapi.Permission(p))
	}
	if !slices.Equal(got, want) {
		t.Errorf("permissions = %v, want %v", got, want)
	}
}

func TestAdminOperationsNeedAPermission(t *testing.T) {
	e, health, dead := newAdminEnv(t)
	for _, op := range adminOperations {
		res := e.admin(t, op.method, op.path, "bob-admin", map[string]string{"Accept-Language": "en"})
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s by bob = %d, want 403", op.method, op.path, res.StatusCode)
			continue
		}
		if p := decodeProblem(t, res); p.Type != clientapi.ProblemTypeMissingPermission || p.Detail == nil || !strings.Contains(*p.Detail, "manage_") {
			t.Errorf("%s %s by bob: problem = %+v", op.method, op.path, p)
		}
	}
	res := e.admin(t, http.MethodGet, "/admin/jobs?state=dead", "bob-admin", map[string]string{"Accept-Language": "ja"})
	if p := decodeProblem(t, res); p.Detail == nil || *p.Detail != "ロールに権限 manage_jobs が必要です。" || res.Header.Get("Content-Language") != "ja" {
		t.Errorf("localized problem = %+v (%s)", p, res.Header.Get("Content-Language"))
	}
	if len(dead.enqueued) != 0 || health.servers[0].State != federation.InstanceUnavailable {
		t.Error("a refused operation changed something")
	}

	// read and write do not grant the admin scopes; admin:read does not grant admin:write.
	for token, op := range map[string]struct{ method, path string }{
		"read":       {http.MethodGet, "/admin/instances"},
		"write":      {http.MethodPost, "/admin/jobs/" + deadJobA + "/retry"},
		"admin-read": {http.MethodPost, "/admin/instances/a.example/mark_available"},
	} {
		res := e.admin(t, op.method, op.path, token, nil)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeInsufficientScope {
			t.Errorf("%s %s with %s = %d %+v, want 403 insufficient-scope", op.method, op.path, token, res.StatusCode, p)
		}
	}
}

func TestListInstanceHealth(t *testing.T) {
	e, _, _ := newAdminEnv(t)
	res := e.admin(t, http.MethodGet, "/admin/instances?limit=2", "admin-read", nil)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("status = %d, Cache-Control = %q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	page := decodeJSON[clientapi.AdminInstanceHealthPage](t, res)
	if len(page.Items) != 2 || page.Items[0].Domain != "a.example" || page.Items[1].Domain != "b.example" || page.NextCursor == nil {
		t.Fatalf("first page = %+v", page)
	}
	a := page.Items[0]
	if a.State != clientapi.AdminInstanceStateUnavailable || a.FailureDays != 7 || a.Since == nil || !a.Since.Equal(adminNow.Add(-time.Hour)) ||
		a.LastFailureDay == nil || a.LastFailureDay.String() != "2026-10-16" {
		t.Errorf("a.example = %+v", a)
	}
	res = e.admin(t, http.MethodGet, "/admin/instances?limit=2&cursor="+*page.NextCursor, "admin-read", nil)
	if rest := decodeJSON[clientapi.AdminInstanceHealthPage](t, res); len(rest.Items) != 1 || rest.Items[0].Domain != "c.example" || rest.NextCursor != nil {
		t.Errorf("second page = %+v", rest)
	}
	res = e.admin(t, http.MethodGet, "/admin/instances?state=unavailable", "admin-read", nil)
	if got := decodeJSON[clientapi.AdminInstanceHealthPage](t, res); len(got.Items) != 2 || got.Items[1].Domain != "c.example" {
		t.Errorf("unavailable = %+v", got)
	}
	for _, query := range []string{"state=gone", "limit=0", "cursor=%25"} {
		res := e.admin(t, http.MethodGet, "/admin/instances?"+query, "admin-read", nil)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("?%s = %d, want 400", query, res.StatusCode)
		}
	}
}

func TestGetInstanceHealth(t *testing.T) {
	e, _, _ := newAdminEnv(t)
	res := e.admin(t, http.MethodGet, "/admin/instances/B.Example", "admin-read", nil)
	if got := decodeJSON[clientapi.AdminInstanceHealth](t, res); res.StatusCode != http.StatusOK || got.Domain != "b.example" || got.FailureDays != 2 {
		t.Errorf("b.example = %d %+v", res.StatusCode, got)
	}
	res = e.admin(t, http.MethodGet, "/admin/instances/healthy.example", "admin-read", nil)
	got := decodeJSON[clientapi.AdminInstanceHealth](t, res)
	if got.State != clientapi.AdminInstanceStateAvailable || got.Since != nil || got.LastFailureDay != nil || got.FailureDays != 0 {
		t.Errorf("a server without a row = %+v, want available without since", got)
	}
}

func TestListInstanceAvailabilityEvents(t *testing.T) {
	e, _, _ := newAdminEnv(t)
	res := e.admin(t, http.MethodGet, "/admin/instances/a.example/availability_events?limit=1", "admin-read", nil)
	page := decodeJSON[clientapi.AdminAvailabilityEventPage](t, res)
	if res.StatusCode != http.StatusOK || len(page.Items) != 1 || page.Items[0].Id != eventTwo || page.NextCursor == nil ||
		page.Items[0].Kind != clientapi.AdminAvailabilityEventKindMarkedUnavailable || page.Items[0].Reason != clientapi.AdminAvailabilityEventReasonFailureDays {
		t.Fatalf("first page = %d %+v", res.StatusCode, page)
	}
	res = e.admin(t, http.MethodGet, "/admin/instances/a.example/availability_events?limit=1&cursor="+*page.NextCursor, "admin-read", nil)
	if rest := decodeJSON[clientapi.AdminAvailabilityEventPage](t, res); len(rest.Items) != 1 || rest.Items[0].Id != eventOne || rest.NextCursor != nil {
		t.Errorf("second page = %+v", rest)
	}
	res = e.admin(t, http.MethodGet, "/admin/instances/b.example/availability_events", "admin-read", nil)
	if none := decodeJSON[clientapi.AdminAvailabilityEventPage](t, res); len(none.Items) != 0 {
		t.Errorf("b.example = %+v, want no events", none)
	}
}

func TestMarkInstanceAvailable(t *testing.T) {
	e, health, _ := newAdminEnv(t)
	res := e.admin(t, http.MethodPost, "/admin/instances/a.example/mark_available", "admin", nil)
	got := decodeJSON[clientapi.AdminInstanceHealth](t, res)
	if res.StatusCode != http.StatusOK || got.State != clientapi.AdminInstanceStateAvailable || got.FailureDays != 0 || got.LastFailureDay != nil ||
		got.Since == nil || !got.Since.Equal(adminNow) {
		t.Fatalf("marked = %d %+v", res.StatusCode, got)
	}
	if last := health.events[len(health.events)-1]; last.Reason != federation.ReasonOperator || last.Kind != federation.MarkedAvailable {
		t.Errorf("last event = %+v, want marked_available (operator)", last)
	}
	res = e.admin(t, http.MethodGet, "/admin/instances/a.example/availability_events?limit=1", "admin", nil)
	if page := decodeJSON[clientapi.AdminAvailabilityEventPage](t, res); len(page.Items) != 1 || page.Items[0].Reason != clientapi.AdminAvailabilityEventReasonOperator {
		t.Errorf("newest event = %+v", page)
	}
}

func TestGetAdminQueue(t *testing.T) {
	e, _, _, inspector := newAdminEnvWithQueue(t)
	res := e.admin(t, http.MethodGet, "/admin/queue", "admin-read", nil)
	got := decodeJSON[clientapi.AdminQueue](t, res)
	if res.StatusCode != http.StatusOK || got.Adapter != "db" || !got.PendingListing || len(got.Counts) != 5 ||
		got.OldestReadyAt == nil || !got.OldestReadyAt.Equal(adminNow.Add(-10*time.Minute)) {
		t.Fatalf("queue = %d %+v", res.StatusCode, got)
	}
	if len(got.Waits) != 2 || got.Waits[0].Type != "federation.deliver" || !got.Waits[0].OldestReadyAt.Equal(adminNow.Add(-10*time.Minute)) ||
		got.Waits[1].Type != "kv.cleanup" {
		t.Errorf("waits = %+v", got.Waits)
	}
	if c := got.Counts[3]; c.State != clientapi.AdminJobStateDead || c.Type != "federation.deliver" || c.Count != 2 {
		t.Errorf("dead count = %+v", c)
	}
	if len(inspector.nows) != 1 || !inspector.nows[0].Equal(adminNow) {
		t.Errorf("summarized at %v, want %v", inspector.nows, adminNow)
	}
}

// TestAdminQueueWithoutListing checks a queue that cannot list its pending jobs (Cloud Tasks,
// SQS): no counts, empty pages of pending jobs, while the dead jobs are still listed.
func TestAdminQueueWithoutListing(t *testing.T) {
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
		o.DeadJobs = &fakeDeadJobs{dead: []queue.DeadJob{{ID: deadJobA, Type: "kv.cleanup", Payload: json.RawMessage(`null`), Attempt: 3, LastError: "db down", CreatedAt: adminNow, FailedAt: adminNow}}}
		o.Queue = clientapi.QueueDashboard{Adapter: "sqs"}
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	res := e.admin(t, http.MethodGet, "/admin/queue", "admin", nil)
	if got := decodeJSON[clientapi.AdminQueue](t, res); got.Adapter != "sqs" || got.PendingListing || got.Counts == nil || len(got.Counts) != 0 || got.OldestReadyAt != nil {
		t.Errorf("queue = %+v, want no listing and no counts", got)
	}
	res = e.admin(t, http.MethodGet, "/admin/jobs?state=queued", "admin", nil)
	if page := decodeJSON[clientapi.AdminJobPage](t, res); res.StatusCode != http.StatusOK || page.Items == nil || len(page.Items) != 0 || page.NextCursor != nil {
		t.Errorf("queued jobs = %d %+v, want an empty page", res.StatusCode, page)
	}
	res = e.admin(t, http.MethodGet, "/admin/jobs?state=dead", "admin", nil)
	if page := decodeJSON[clientapi.AdminJobPage](t, res); len(page.Items) != 1 || page.Items[0].Id != deadJobA {
		t.Errorf("dead jobs = %+v", page)
	}
	res = e.admin(t, http.MethodGet, "/admin/jobs/"+deadJobA+"/attempts", "admin", nil)
	if page := decodeJSON[clientapi.AdminJobAttemptPage](t, res); res.StatusCode != http.StatusOK || page.Items == nil || len(page.Items) != 0 {
		t.Errorf("attempts without a log = %d %+v, want an empty page", res.StatusCode, page)
	}
}

func TestListAdminJobs(t *testing.T) {
	e, _, _, inspector := newAdminEnvWithQueue(t)
	res := e.admin(t, http.MethodGet, "/admin/jobs?limit=2", "admin-read", nil)
	page := decodeJSON[clientapi.AdminJobPage](t, res)
	if res.StatusCode != http.StatusOK || len(page.Items) != 2 || page.Items[0].Id != jobOne || page.Items[1].Id != jobTwo || page.NextCursor == nil {
		t.Fatalf("first page = %d %+v", res.StatusCode, page)
	}
	running := page.Items[0]
	if running.State != clientapi.AdminJobStateRunning || running.Worker == nil || *running.Worker != "worker-1" || running.LockedUntil == nil ||
		!running.LockedUntil.Equal(adminNow.Add(4*time.Minute)) || running.LastError != nil || running.FailedAt != nil ||
		running.RunAt == nil || running.Payload != `{"delivery_id":"y"}` || running.Attempts != 1 {
		t.Errorf("running job = %+v", running)
	}
	if retrying := page.Items[1]; retrying.State != clientapi.AdminJobStateRetrying || retrying.Worker != nil || retrying.LockedUntil != nil ||
		retrying.LastError == nil || *retrying.LastError != "503" || retrying.OrderingKey == nil || *retrying.OrderingKey != "inbox" {
		t.Errorf("retrying job = %+v", retrying)
	}
	res = e.admin(t, http.MethodGet, "/admin/jobs?limit=2&cursor="+*page.NextCursor, "admin-read", nil)
	if rest := decodeJSON[clientapi.AdminJobPage](t, res); len(rest.Items) != 1 || rest.Items[0].Id != jobThree || rest.NextCursor != nil ||
		rest.Items[0].IdempotencyKey == nil || rest.Items[0].State != clientapi.AdminJobStateQueued {
		t.Errorf("second page = %+v", rest)
	}

	res = e.admin(t, http.MethodGet, "/admin/jobs?state=retrying&type=federation.deliver", "admin-read", nil)
	if got := decodeJSON[clientapi.AdminJobPage](t, res); len(got.Items) != 1 || got.Items[0].Id != jobTwo {
		t.Errorf("retrying deliveries = %+v", got)
	}
	if last := inspector.filters[len(inspector.filters)-1]; last != (queue.JobFilter{State: queue.JobStateRetrying, Type: "federation.deliver"}) {
		t.Errorf("filter = %+v", last)
	}
	for _, query := range []string{"state=failed", "cursor=bm90IGEgY3Vyc29y", "cursor=%25", "limit=0"} {
		res := e.admin(t, http.MethodGet, "/admin/jobs?"+query, "admin-read", nil)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", query, res.StatusCode)
		}
	}
}

func TestListDeadJobs(t *testing.T) {
	e, _, _ := newAdminEnv(t)
	res := e.admin(t, http.MethodGet, "/admin/jobs?state=dead&limit=2", "admin-read", nil)
	page := decodeJSON[clientapi.AdminJobPage](t, res)
	if res.StatusCode != http.StatusOK || len(page.Items) != 2 || page.Items[0].Id != deadJobC || page.Items[1].Id != deadJobB || page.NextCursor == nil {
		t.Fatalf("first page = %d %+v", res.StatusCode, page)
	}
	c := page.Items[0]
	if c.Type != "federation.deliver" || c.State != clientapi.AdminJobStateDead || c.Payload != `{"delivery_id":"x"}` || c.Attempts != 9 ||
		c.LastError == nil || *c.LastError != "502" || c.FailedAt == nil || c.RunAt != nil ||
		c.IdempotencyKey == nil || *c.IdempotencyKey != "deliver:x" || c.OrderingKey == nil || *c.OrderingKey != "inbox" {
		t.Errorf("dead job = %+v", c)
	}
	if b := page.Items[1]; b.IdempotencyKey != nil || b.OrderingKey != nil {
		t.Errorf("dead job without keys = %+v", b)
	}
	res = e.admin(t, http.MethodGet, "/admin/jobs?state=dead&limit=2&cursor="+*page.NextCursor, "admin-read", nil)
	if rest := decodeJSON[clientapi.AdminJobPage](t, res); len(rest.Items) != 1 || rest.Items[0].Id != deadJobA || rest.NextCursor != nil {
		t.Errorf("second page = %+v", rest)
	}
	res = e.admin(t, http.MethodGet, "/admin/jobs?state=dead&type=kv.cleanup", "admin-read", nil)
	if got := decodeJSON[clientapi.AdminJobPage](t, res); len(got.Items) != 1 || got.Items[0].Id != deadJobA {
		t.Errorf("dead kv.cleanup jobs = %+v", got)
	}
}

func TestListAdminJobAttempts(t *testing.T) {
	e, _, _ := newAdminEnv(t)
	res := e.admin(t, http.MethodGet, "/admin/jobs/"+jobTwo+"/attempts?limit=1", "admin-read", nil)
	page := decodeJSON[clientapi.AdminJobAttemptPage](t, res)
	if res.StatusCode != http.StatusOK || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatalf("first page = %d %+v", res.StatusCode, page)
	}
	if a := page.Items[0]; a.Attempt != 2 || a.Outcome != clientapi.AdminJobAttemptOutcomeRetry || a.Error != "503" || a.Worker != "worker-2" ||
		a.Type != "federation.deliver" || !a.StartedAt.Equal(adminNow.Add(-2*time.Minute)) || !a.FinishedAt.Equal(adminNow.Add(-time.Minute)) {
		t.Errorf("attempt = %+v", a)
	}
	res = e.admin(t, http.MethodGet, "/admin/jobs/"+jobTwo+"/attempts?limit=1&cursor="+*page.NextCursor, "admin-read", nil)
	if rest := decodeJSON[clientapi.AdminJobAttemptPage](t, res); len(rest.Items) != 1 || rest.Items[0].Attempt != 1 || rest.NextCursor != nil {
		t.Errorf("second page = %+v", rest)
	}
	res = e.admin(t, http.MethodGet, "/admin/jobs/"+deadJobC+"/attempts", "admin-read", nil)
	if got := decodeJSON[clientapi.AdminJobAttemptPage](t, res); len(got.Items) != 1 || got.Items[0].Outcome != clientapi.AdminJobAttemptOutcomeDead {
		t.Errorf("attempts of the dead job = %+v", got)
	}
	res = e.admin(t, http.MethodGet, "/admin/jobs/"+jobOne+"/attempts", "admin-read", nil)
	if got := decodeJSON[clientapi.AdminJobAttemptPage](t, res); res.StatusCode != http.StatusOK || len(got.Items) != 0 {
		t.Errorf("attempts of a job without failures = %d %+v, want an empty page", res.StatusCode, got)
	}
	bad := base64.RawURLEncoding.EncodeToString([]byte(adminNow.Format(time.RFC3339Nano) + " x"))
	for _, cursor := range []string{"bm90IGEgY3Vyc29y", bad} {
		res := e.admin(t, http.MethodGet, "/admin/jobs/"+jobTwo+"/attempts?cursor="+cursor, "admin-read", nil)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("cursor %s = %d, want 400", cursor, res.StatusCode)
		}
	}
}

func TestRetryDeadJob(t *testing.T) {
	e, _, dead := newAdminEnv(t)
	res := e.admin(t, http.MethodPost, "/admin/jobs/"+deadJobB+"/retry", "admin", nil)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("retry = %d, want 204", res.StatusCode)
	}
	if len(dead.enqueued) != 1 || dead.enqueued[0].ID != deadJobB || slices.ContainsFunc(dead.dead, func(d queue.DeadJob) bool { return d.ID == deadJobB }) {
		t.Fatalf("enqueued = %+v, dead = %+v; want the job enqueued and no longer dead", dead.enqueued, dead.dead)
	}
	res = e.admin(t, http.MethodPost, "/admin/jobs/"+deadJobB+"/retry", "admin", nil)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusNotFound || p.Type != clientapi.ProblemTypeBlank {
		t.Errorf("retry again = %d %+v, want 404", res.StatusCode, p)
	}
}

// TestAdminWithoutPermissions checks that a server without a Permissions port grants no
// permission.
func TestAdminWithoutPermissions(t *testing.T) {
	e := newAuthEnv(t, func(o *clientapi.Options) { o.DeadJobs = &fakeDeadJobs{} })
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	res := e.admin(t, http.MethodGet, "/admin/jobs?state=dead", "admin", nil)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission {
		t.Errorf("without permissions = %d %+v, want 403", res.StatusCode, p)
	}
}
