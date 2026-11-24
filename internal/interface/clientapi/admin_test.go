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

// fakeHealth keeps the health of servers (in domain order), the servers seen in a document and
// the software they reported, and the events of servers (oldest first) with the semantics of
// appfederation.Health.
type fakeHealth struct {
	servers []federation.InstanceHealth
	// seen are the first sightings of servers, with or without a health row.
	seen map[string]time.Time
	// software is the newest NodeInfo observation of servers.
	software map[string]federation.NodeInfoObservation
	events   []federation.AvailabilityEvent
	now      time.Time
	// markedBy are the users who cleared a server.
	markedBy []string
	// filters and afters are the filters and cursors List was called with.
	filters []appfederation.InstanceFilter
	afters  []string
}

func (f *fakeHealth) Find(_ context.Context, domain string) (federation.KnownInstance, error) {
	k := federation.KnownInstance{InstanceHealth: federation.InstanceHealth{Domain: domain, State: federation.InstanceAvailable}}
	for _, h := range f.servers {
		if h.Domain == domain {
			k.InstanceHealth = h
		}
	}
	k.FirstSeenAt, k.Software = f.seen[domain], f.software[domain]
	return k, nil
}

// List lists the servers with a health row or a sighting, in domain order; troubled keeps those
// with a health row.
func (f *fakeHealth) List(ctx context.Context, filter appfederation.InstanceFilter, after string, limit int) ([]federation.KnownInstance, error) {
	f.filters, f.afters = append(f.filters, filter), append(f.afters, after)
	state := filter.State
	if state != "" && state != federation.InstanceAvailable && state != federation.InstanceUnavailable {
		return nil, appfederation.ErrInvalidInstanceState
	}
	var domains []string
	for _, h := range f.servers {
		domains = append(domains, h.Domain)
	}
	if !filter.Troubled {
		for d := range f.seen {
			domains = append(domains, d)
		}
	}
	slices.Sort(domains)
	var out []federation.KnownInstance
	for _, d := range slices.Compact(domains) {
		k, _ := f.Find(ctx, d)
		if (state == "" || k.State == state) && d > after && len(out) < limit {
			out = append(out, k)
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

func (f *fakeHealth) MarkAvailable(ctx context.Context, domain, byUserID string) (federation.KnownInstance, error) {
	f.markedBy = append(f.markedBy, byUserID)
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

func (f *fakeDeadJobs) GetDeadJob(_ context.Context, id string) (queue.DeadJob, error) {
	i := slices.IndexFunc(f.dead, func(d queue.DeadJob) bool { return d.ID == id })
	if i < 0 {
		return queue.DeadJob{}, queue.ErrDeadJobNotFound
	}
	return f.dead[i], nil
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

func (f *fakeInspector) GetJob(_ context.Context, now time.Time, id string) (queue.PendingJob, error) {
	f.nows = append(f.nows, now)
	i := slices.IndexFunc(f.jobs, func(j queue.PendingJob) bool { return j.ID == id })
	if i < 0 {
		return queue.PendingJob{}, queue.ErrJobNotFound
	}
	return f.jobs[i], nil
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
		{ID: deadJobA, Type: "kv.purge_expired", Payload: json.RawMessage(`null`), Attempt: 3, LastError: "db down", CreatedAt: failed.Add(-time.Hour), FailedAt: failed.Add(-time.Second)},
	}}
	inspector := &fakeInspector{
		jobs: []queue.PendingJob{
			{ID: jobOne, Type: "federation.deliver", Payload: json.RawMessage(`{"delivery_id":"y"}`), State: queue.JobStateRunning, Attempt: 1,
				RunAt: adminNow.Add(-time.Minute), Worker: "worker-1", LockedUntil: adminNow.Add(4 * time.Minute), CreatedAt: adminNow.Add(-time.Minute)},
			{ID: jobTwo, Type: "federation.deliver", Payload: json.RawMessage(`{}`), OrderingKey: "inbox", State: queue.JobStateRetrying, Attempt: 2,
				RunAt: adminNow.Add(time.Minute), LastError: "503", CreatedAt: adminNow.Add(-time.Hour)},
			{ID: jobThree, Type: "kv.purge_expired", Payload: json.RawMessage(`null`), IdempotencyKey: "kv", State: queue.JobStateQueued,
				RunAt: adminNow.Add(time.Hour), CreatedAt: adminNow},
		},
		summary: queue.JobSummary{
			Counts: []queue.JobCount{
				{State: queue.JobStateQueued, Type: "kv.purge_expired", Count: 1},
				{State: queue.JobStateRunning, Type: "federation.deliver", Count: 1},
				{State: queue.JobStateRetrying, Type: "federation.deliver", Count: 1},
				{State: queue.JobStateDead, Type: "federation.deliver", Count: 2},
				{State: queue.JobStateDead, Type: "kv.purge_expired", Count: 1},
			},
			OldestReadyAt: adminNow.Add(-10 * time.Minute),
			OldestReadyByType: map[string]time.Time{
				"kv.purge_expired":   adminNow.Add(-time.Minute),
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
	{http.MethodGet, "/admin/jobs/" + jobTwo},
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
	if a.State != clientapi.AdminInstanceStateUnavailable || a.FailureDays != 7 || a.StateChangedAt == nil || !a.StateChangedAt.Equal(adminNow.Add(-time.Hour)) ||
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

// A domain of a path that is not a host is a 400 on every operation of a remote server.
func TestInstanceOperationsRefuseAMalformedDomain(t *testing.T) {
	e, health, _ := newAdminEnv(t)
	for _, op := range []struct{ method, path, token string }{
		{http.MethodGet, "/admin/instances/bad%20host", "admin-read"},
		{http.MethodGet, "/admin/instances/bad%20host/availability_events", "admin-read"},
		{http.MethodPost, "/admin/instances/bad%20host/mark_available", "admin"},
	} {
		res := e.admin(t, op.method, op.path, op.token, nil)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusBadRequest || p.Detail == nil || !strings.Contains(*p.Detail, "domain") {
			t.Errorf("%s %s = %d %+v, want 400 naming domain", op.method, op.path, res.StatusCode, p)
		}
	}
	if len(health.markedBy) != 0 {
		t.Errorf("marked = %v, want none", health.markedBy)
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
	if got.State != clientapi.AdminInstanceStateAvailable || got.StateChangedAt != nil || got.LastFailureDay != nil || got.FailureDays != 0 {
		t.Errorf("a server without a row = %+v, want available without since", got)
	}
}

// newKnownServersEnv is newAdminEnv where b.example, which has a health row, and d.example,
// which has none, were seen in a document, and both reported their software, d.example without
// a version.
func newKnownServersEnv(t *testing.T) (*authEnv, *fakeHealth) {
	t.Helper()
	e, health, _ := newAdminEnv(t)
	health.seen = map[string]time.Time{"b.example": adminNow.Add(-72 * time.Hour), "d.example": adminNow.Add(-24 * time.Hour)}
	health.software = map[string]federation.NodeInfoObservation{
		"b.example": {ID: "01890a5d-ac96-774b-bcce-b302099a9001", Domain: "b.example", SoftwareName: "mastodon", SoftwareVersion: "4.3.1", ObservedAt: adminNow.Add(-time.Hour)},
		"d.example": {ID: "01890a5d-ac96-774b-bcce-b302099a9002", Domain: "d.example", SoftwareName: "misskey", ObservedAt: adminNow.Add(-time.Hour)},
	}
	return e, health
}

func domainsOfPage(page clientapi.AdminInstanceHealthPage) []string {
	var ds []string
	for _, it := range page.Items {
		ds = append(ds, it.Domain)
	}
	return ds
}

// The list passes troubled and state to Health.List as one filter, and its cursor as the last
// domain of the page; a bad state or troubled is a 400.
func TestListKnownInstancesFilters(t *testing.T) {
	e, health := newKnownServersEnv(t)
	for _, c := range []struct {
		query   string
		filter  appfederation.InstanceFilter
		domains []string
	}{
		{"", appfederation.InstanceFilter{}, []string{"a.example", "b.example", "c.example", "d.example"}},
		{"troubled=false", appfederation.InstanceFilter{}, []string{"a.example", "b.example", "c.example", "d.example"}},
		{"troubled=true", appfederation.InstanceFilter{Troubled: true}, []string{"a.example", "b.example", "c.example"}},
		{"state=available", appfederation.InstanceFilter{State: federation.InstanceAvailable}, []string{"b.example", "d.example"}},
		{"troubled=true&state=available", appfederation.InstanceFilter{State: federation.InstanceAvailable, Troubled: true}, []string{"b.example"}},
		{"state=unavailable", appfederation.InstanceFilter{State: federation.InstanceUnavailable}, []string{"a.example", "c.example"}},
		{"state=unavailable&troubled=true", appfederation.InstanceFilter{State: federation.InstanceUnavailable, Troubled: true}, []string{"a.example", "c.example"}},
	} {
		health.filters = nil
		res := e.admin(t, http.MethodGet, "/admin/instances?"+c.query, "admin-read", nil)
		if res.StatusCode != http.StatusOK {
			t.Errorf("?%s = %d, want 200", c.query, res.StatusCode)
			continue
		}
		page := decodeJSON[clientapi.AdminInstanceHealthPage](t, res)
		if got := domainsOfPage(page); !slices.Equal(got, c.domains) {
			t.Errorf("?%s = %v, want %v", c.query, got, c.domains)
		}
		if !slices.Equal(health.filters, []appfederation.InstanceFilter{c.filter}) {
			t.Errorf("?%s asked Health.List for %+v, want %+v", c.query, health.filters, c.filter)
		}
	}

	res := e.admin(t, http.MethodGet, "/admin/instances?limit=3", "admin-read", nil)
	first := decodeJSON[clientapi.AdminInstanceHealthPage](t, res)
	if got := domainsOfPage(first); !slices.Equal(got, []string{"a.example", "b.example", "c.example"}) || first.NextCursor == nil {
		t.Fatalf("first page = %v, cursor %v", got, first.NextCursor)
	}
	health.afters = nil
	res = e.admin(t, http.MethodGet, "/admin/instances?limit=3&troubled=false&cursor="+*first.NextCursor, "admin-read", nil)
	rest := decodeJSON[clientapi.AdminInstanceHealthPage](t, res)
	if got := domainsOfPage(rest); !slices.Equal(got, []string{"d.example"}) || rest.NextCursor != nil {
		t.Errorf("second page = %v, cursor %v; want d.example and no cursor", got, rest.NextCursor)
	}
	if !slices.Equal(health.afters, []string{"c.example"}) {
		t.Errorf("the cursor of the second page reached Health.List as %q, want the last domain c.example", health.afters)
	}

	for _, query := range []string{"state=gone", "troubled=maybe", "troubled=true&state=gone"} {
		res := e.admin(t, http.MethodGet, "/admin/instances?"+query, "admin-read", nil)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("?%s = %d, want 400", query, res.StatusCode)
		}
	}
}

// Each server carries its first sighting and its software when it has them, on the list, on its
// own, and once marked available.
func TestKnownInstanceFields(t *testing.T) {
	e, _ := newKnownServersEnv(t)
	res := e.admin(t, http.MethodGet, "/admin/instances", "admin-read", nil)
	items := map[string]clientapi.AdminInstanceHealth{}
	for _, it := range decodeJSON[clientapi.AdminInstanceHealthPage](t, res).Items {
		items[it.Domain] = it
	}
	b := items["b.example"]
	if b.FirstSeenAt == nil || !b.FirstSeenAt.Equal(adminNow.Add(-72*time.Hour)) || b.Software == nil ||
		*b.Software != (clientapi.AdminInstanceSoftware{Name: "mastodon", Version: "4.3.1"}) || b.FailureDays != 2 {
		t.Errorf("b.example = %+v (software %+v)", b, b.Software)
	}
	d := items["d.example"]
	if d.State != clientapi.AdminInstanceStateAvailable || d.FailureDays != 0 || d.StateChangedAt != nil || d.LastFailureDay != nil ||
		d.FirstSeenAt == nil || !d.FirstSeenAt.Equal(adminNow.Add(-24*time.Hour)) ||
		d.Software == nil || *d.Software != (clientapi.AdminInstanceSoftware{Name: "misskey", Version: ""}) {
		t.Errorf("d.example, seen without a health row = %+v (software %+v)", d, d.Software)
	}
	if a := items["a.example"]; a.FirstSeenAt != nil || a.Software != nil {
		t.Errorf("a.example, known only from its health = %+v, want neither a sighting nor software", a)
	}

	// The software of d.example reports no version: the version is there, empty.
	res = e.admin(t, http.MethodGet, "/admin/instances/d.example", "admin-read", nil)
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || string(raw["software"]) != `{"name":"misskey","version":""}` || raw["first_seen_at"] == nil {
		t.Errorf("GET d.example = %d %s", res.StatusCode, raw)
	}
	res = e.admin(t, http.MethodGet, "/admin/instances/unknown.example", "admin-read", nil)
	if got := decodeJSON[clientapi.AdminInstanceHealth](t, res); res.StatusCode != http.StatusOK || got.State != clientapi.AdminInstanceStateAvailable ||
		got.FirstSeenAt != nil || got.Software != nil {
		t.Errorf("GET unknown.example = %d %+v, want available without a sighting or software", res.StatusCode, got)
	}
	res = e.admin(t, http.MethodPost, "/admin/instances/b.example/mark_available", "admin", nil)
	if got := decodeJSON[clientapi.AdminInstanceHealth](t, res); res.StatusCode != http.StatusOK || got.State != clientapi.AdminInstanceStateAvailable ||
		got.FirstSeenAt == nil || got.Software == nil || got.Software.Name != "mastodon" {
		t.Errorf("mark_available b.example = %d %+v, want its sighting and software", res.StatusCode, got)
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
		got.StateChangedAt == nil || !got.StateChangedAt.Equal(adminNow) {
		t.Fatalf("marked = %d %+v", res.StatusCode, got)
	}
	if want := []string{userOf(aliceAccountID)}; !slices.Equal(health.markedBy, want) {
		t.Errorf("cleared by %v, want %v, for the audit log", health.markedBy, want)
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
		got.Waits[1].Type != "kv.purge_expired" {
		t.Errorf("waits = %+v", got.Waits)
	}
	if c := got.Counts[3]; c.State != clientapi.AdminJobStateDead || c.Type != "federation.deliver" || c.Count != 2 {
		t.Errorf("dead count = %+v", c)
	}
	if len(inspector.nows) != 1 || !inspector.nows[0].Equal(adminNow) {
		t.Errorf("summarized at %v, want %v", inspector.nows, adminNow)
	}
}

// TestAdminQueueWithoutListing checks a queue that cannot list its pending jobs (Cloud Tasks):
// no counts, empty pages of pending jobs, while the dead jobs are still listed.
func TestAdminQueueWithoutListing(t *testing.T) {
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
		o.DeadJobs = &fakeDeadJobs{dead: []queue.DeadJob{{ID: deadJobA, Type: "kv.purge_expired", Payload: json.RawMessage(`null`), Attempt: 3, LastError: "db down", CreatedAt: adminNow, FailedAt: adminNow}}}
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
	res = e.admin(t, http.MethodGet, "/admin/jobs?state=dead&type=kv.purge_expired", "admin-read", nil)
	if got := decodeJSON[clientapi.AdminJobPage](t, res); len(got.Items) != 1 || got.Items[0].Id != deadJobA {
		t.Errorf("dead kv.purge_expired jobs = %+v", got)
	}
}

func TestGetAdminJob(t *testing.T) {
	e, _, _, inspector := newAdminEnvWithQueue(t)
	res := e.admin(t, http.MethodGet, "/admin/jobs/"+jobOne, "admin-read", nil)
	job := decodeJSON[clientapi.AdminJob](t, res)
	if res.StatusCode != http.StatusOK || job.Id != jobOne || job.State != clientapi.AdminJobStateRunning || job.Worker == nil ||
		*job.Worker != "worker-1" || job.RunAt == nil || job.FailedAt != nil || job.Payload != `{"delivery_id":"y"}` {
		t.Errorf("pending job = %d %+v", res.StatusCode, job)
	}
	if got := res.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if last := inspector.nows[len(inspector.nows)-1]; !last.Equal(adminNow) {
		t.Errorf("state read at %v, want %v", last, adminNow)
	}
	res = e.admin(t, http.MethodGet, "/admin/jobs/"+deadJobC, "admin-read", nil)
	job = decodeJSON[clientapi.AdminJob](t, res)
	if res.StatusCode != http.StatusOK || job.State != clientapi.AdminJobStateDead || job.FailedAt == nil || job.RunAt != nil ||
		job.LastError == nil || *job.LastError != "502" || job.Attempts != 9 {
		t.Errorf("dead job = %d %+v", res.StatusCode, job)
	}
	for _, id := range []string{"01890a5d-ac96-774b-bcce-b302099a9999", "not-an-id"} {
		res := e.admin(t, http.MethodGet, "/admin/jobs/"+id, "admin-read", nil)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusNotFound || p.Type != clientapi.ProblemTypeBlank {
			t.Errorf("job %s = %d %+v, want 404", id, res.StatusCode, p)
		}
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
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("retry = %d, want 202", res.StatusCode)
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
