// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	appdiagnostics "github.com/mycroblog/my/internal/app/diagnostics"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	appwebhook "github.com/mycroblog/my/internal/app/webhook"
	"github.com/mycroblog/my/internal/domain/diagnostics"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/webhook"
)

// WebhookHarness is what WebhookRepositoryContract needs from a database adapter: its unit of
// work, the webhook repository, and the moderation repository, which keeps the audit log.
type WebhookHarness struct {
	UnitOfWork uow.UnitOfWork
	Webhooks   appwebhook.Repository
	Moderation appmoderation.Repository
}

// WebhookFactory returns a harness on a new, empty database whose UnitOfWork uses opts. The
// factory registers the cleanup of everything it creates with t.Cleanup.
type WebhookFactory func(t *testing.T, opts uow.Options) WebhookHarness

// WebhookRepositoryContract verifies the webhook repository of a database adapter through the
// webhook use cases (ADR-0060, FR-722): endpoints are created, revised, enabled, disabled, and
// deleted with their audit log entries, which name the host and never the URL; subscribing needs
// the permissions of the events; an event becomes a message and a delivery job for the enabled
// endpoints that subscribe to it only; a delivery is signed with Standard Webhooks v1 under the
// current secret (and the previous one for a day after a rotation), is not sent again after it
// succeeded unless redelivered with the same webhook-id, is retried after a failure, and 410
// Gone or a run of failures disables its endpoint as the system and emits EndpointDisabled; and
// the messages are purged after webhook.Retention. The error groups are sent as
// error_group.created and error_group.regressed, which need manage_errors, only once a report
// with a session has seen them, and a group muted by then is not sent.
func WebhookRepositoryContract(t *testing.T, newHarness WebhookFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *webhookEnv)
	}{
		{"ManagesEndpointsAndAudits", webhookManage},
		{"ChecksThePermissionsOfTheEvents", webhookPermissions},
		{"SendsEventsToTheSubscribedEndpoints", webhookSources},
		{"SendsTheErrorGroupsWithASession", webhookErrorGroups},
		{"DeliversSignedOnceAndRedelivers", webhookDeliver},
		{"RetriesAndDisablesAGoneEndpoint", webhookGone},
		{"DisablesAnEndpointThatKeepsFailing", webhookFailing},
		{"SignsWithBothSecretsAfterARotation", webhookRotate},
		{"PurgesOldMessages", webhookPurge},
		{"RebuildsItsProjections", webhookRebuild},
		{"CountsConcurrentFailures", webhookConcurrentFailures},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newWebhookEnv(t, newHarness)) })
	}
}

type webhookEnv struct {
	h        WebhookHarness
	clock    *Clock
	ids      *IDGenerator
	svc      *appwebhook.Service
	outbox   *recordingOutbox
	http     *webhookHTTP
	granted  map[string][]webhook.Permission
	disabled []appwebhook.EndpointDisabled
	mu       sync.Mutex
}

// webhookPresenter presents every object as its kind and ID, and whether it is given the
// transaction of the event.
type webhookPresenter struct{}

func (webhookPresenter) present(_ context.Context, tx uow.Tx, kind, id string) (any, error) {
	return map[string]any{"kind": kind, "id": id, "in_transaction": tx != nil}, nil
}

func (p webhookPresenter) WebhookReport(ctx context.Context, tx uow.Tx, id string) (any, error) {
	return p.present(ctx, tx, "report", id)
}

func (p webhookPresenter) WebhookRegistration(ctx context.Context, tx uow.Tx, userID string) (any, error) {
	return p.present(ctx, tx, "registration", userID)
}

func (p webhookPresenter) WebhookAccount(ctx context.Context, tx uow.Tx, id string) (any, error) {
	return p.present(ctx, tx, "account", id)
}

func (p webhookPresenter) WebhookAccountAction(ctx context.Context, tx uow.Tx, id string) (any, error) {
	return p.present(ctx, tx, "account_action", id)
}

func (p webhookPresenter) WebhookAppeal(ctx context.Context, tx uow.Tx, id string) (any, error) {
	return p.present(ctx, tx, "appeal", id)
}

func (p webhookPresenter) WebhookDomainAction(ctx context.Context, tx uow.Tx, id string) (any, error) {
	return p.present(ctx, tx, "domain_action", id)
}

func (p webhookPresenter) WebhookCustomEmoji(ctx context.Context, tx uow.Tx, id string) (any, error) {
	return p.present(ctx, tx, "custom_emoji", id)
}

func (p webhookPresenter) WebhookErrorGroup(ctx context.Context, tx uow.Tx, id string) (any, error) {
	return p.present(ctx, tx, "error_group", id)
}

// webhookHTTP is a kernel.HTTPClient that answers with the next status code (200 when none is
// queued), or with err, and keeps the requests.
type webhookHTTP struct {
	mu       sync.Mutex
	statuses []int
	headers  []http.Header
	err      error
	requests []*kernel.HTTPRequest
}

func (c *webhookHTTP) Do(_ context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, req)
	if c.err != nil {
		return nil, c.err
	}
	status, header := http.StatusOK, http.Header{}
	if len(c.statuses) > 0 {
		status, c.statuses = c.statuses[0], c.statuses[1:]
	}
	if len(c.headers) > 0 {
		header, c.headers = c.headers[0], c.headers[1:]
	}
	return &kernel.HTTPResponse{StatusCode: status, Header: header, URL: req.URL, Body: io.NopCloser(strings.NewReader("ok"))}, nil
}

func (c *webhookHTTP) take() []*kernel.HTTPRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.requests
	c.requests = nil
	return out
}

func newWebhookEnv(t *testing.T, newHarness WebhookFactory) *webhookEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	clock := NewClock(time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	ids := NewIDGenerator(clock)
	e := &webhookEnv{h: h, clock: clock, ids: ids, outbox: &recordingOutbox{}, http: &webhookHTTP{}, granted: map[string][]webhook.Permission{}}
	appmoderation.RegisterAuditProjector(projectors, h.Moderation)
	appwebhook.RegisterProjectors(projectors, h.Webhooks)
	e.svc = appwebhook.NewService(appwebhook.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Webhooks, Outbox: e.outbox, Sealer: kernel.NewSealer(NewKeyManager("webhook")),
		HTTP: e.http, Clock: clock, IDs: ids,
		Permissions: func(_ context.Context, userID string) ([]webhook.Permission, error) {
			return e.granted[userID], nil
		},
	})
	e.svc.RegisterSources(projectors)
	e.svc.SetPresenter(webhookPresenter{})
	projectors.Register(appwebhook.EventEndpointDisabled, func(_ context.Context, _ uow.Tx, ev uow.Event) error {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.disabled = append(e.disabled, ev.(appwebhook.EndpointDisabled))
		return nil
	})
	return e
}

func (e *webhookEnv) create(t *testing.T, url string, events ...webhook.EventType) appwebhook.Created {
	t.Helper()
	c, err := e.svc.Create(t.Context(), webhook.EndpointSettings{URL: url, Description: "Chat", Events: events}, "")
	if err != nil {
		t.Fatalf("Create(%s): %v", url, err)
	}
	return c
}

// fileReport emits the ReportFiled of a new report, as the moderation module does.
func (e *webhookEnv) fileReport(t *testing.T) moderation.Report {
	t.Helper()
	r := moderation.Report{
		ID: e.ids.NewID(), ReporterAccountID: e.ids.NewID(), TargetAccountID: e.ids.NewID(),
		Category: moderation.CategorySpam, Comment: "spam", FiledAt: e.clock.Now(),
	}
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		return tx.Emit(t.Context(), appmoderation.ReportFiled{Report: r, FirstOpen: true})
	})
	if err != nil {
		t.Fatalf("emit ReportFiled: %v", err)
	}
	return r
}

// deliveryJobs returns the deliveries of the webhook.deliver jobs enqueued since the last call.
func (e *webhookEnv) deliveryJobs(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, j := range e.outbox.take() {
		if j.Type != appwebhook.JobDeliver {
			t.Errorf("job %s, want %s", j.Type, appwebhook.JobDeliver)
			continue
		}
		var p struct {
			DeliveryID string `json:"delivery_id"`
		}
		if err := json.Unmarshal(j.Payload, &p); err != nil || p.DeliveryID == "" || !strings.HasPrefix(j.IdempotencyKey, appwebhook.JobDeliver+":") {
			t.Errorf("job %s: payload %s, key %q", j.Type, j.Payload, j.IdempotencyKey)
		}
		out = append(out, p.DeliveryID)
	}
	return out
}

func (e *webhookEnv) audit(t *testing.T, endpointID string) []moderation.AuditEntry {
	t.Helper()
	entries, err := e.h.Moderation.ListAuditEntries(t.Context(), appmoderation.AuditQuery{
		TargetKind: moderation.AuditTargetWebhook, TargetID: endpointID,
	}, 100)
	if err != nil {
		t.Fatalf("ListAuditEntries: %v", err)
	}
	return entries
}

func webhookManage(t *testing.T, e *webhookEnv) {
	ctx := t.Context()
	c := e.create(t, "https://hooks.example.com/services/T0/B0/secret-token?x=1", webhook.EventReportResolved, webhook.EventReportCreated)
	if _, err := webhook.ParseSecret(c.Secret); err != nil || !strings.HasPrefix(c.Secret, webhook.SecretPrefix) {
		t.Errorf("secret %q: %v", c.Secret, err)
	}
	got := c.Endpoint
	if got.State != webhook.StateEnabled || got.StateReason != webhook.ReasonAdmin || got.Failures != nil ||
		got.Settings.Description != "Chat" || len(got.Settings.Events) != 2 || got.Settings.Events[0] != webhook.EventReportCreated {
		t.Errorf("created endpoint = %+v", got)
	}
	id := got.ID
	updated, err := e.svc.Update(ctx, id, webhook.EndpointSettings{URL: "https://hooks.example.com/v2", Events: []webhook.EventType{webhook.EventStatusCreated}}, "")
	if err != nil || updated.Settings.URL != "https://hooks.example.com/v2" || updated.Settings.Description != "" ||
		len(updated.Settings.Events) != 1 || updated.RevisionID == got.RevisionID {
		t.Errorf("Update = %+v, %v", updated, err)
	}
	// The same settings again record nothing.
	if again, err := e.svc.Update(ctx, id, updated.Settings, ""); err != nil || again.RevisionID != updated.RevisionID {
		t.Errorf("Update(same) = %+v, %v; want revision %s", again, err, updated.RevisionID)
	}
	if _, err := e.svc.Update(ctx, id, webhook.EndpointSettings{URL: "ftp://x.example", Events: []webhook.EventType{webhook.EventStatusCreated}}, ""); !errors.Is(err, webhook.ErrInvalidURL) {
		t.Errorf("Update(ftp) = %v, want ErrInvalidURL", err)
	}
	if d, err := e.svc.Disable(ctx, id, ""); err != nil || d.State != webhook.StateDisabled {
		t.Errorf("Disable = %+v, %v", d, err)
	}
	if d, err := e.svc.Disable(ctx, id, ""); err != nil || d.State != webhook.StateDisabled {
		t.Errorf("Disable again = %+v, %v", d, err)
	}
	if d, err := e.svc.Enable(ctx, id, ""); err != nil || d.State != webhook.StateEnabled {
		t.Errorf("Enable = %+v, %v", d, err)
	}
	other := e.create(t, "https://other.example/hook", webhook.EventStatusCreated)
	list, err := e.svc.List(ctx)
	if err != nil || len(list) != 2 || list[0].ID != other.Endpoint.ID {
		t.Errorf("List = %+v, %v; want the newest first", list, err)
	}
	if got, err := e.svc.ExistingIDs(ctx, []string{other.Endpoint.ID, id}); err != nil || !slices.Equal(got, sorted(id, other.Endpoint.ID)) {
		t.Errorf("ExistingIDs = %v, %v; want both", got, err)
	}
	if err := e.svc.Delete(ctx, id, ""); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, err := e.svc.ExistingIDs(ctx, []string{other.Endpoint.ID, id}); err != nil || !slices.Equal(got, []string{other.Endpoint.ID}) {
		t.Errorf("ExistingIDs after delete = %v, %v; want the other endpoint", got, err)
	}
	if _, err := e.svc.Get(ctx, id); !errors.Is(err, appwebhook.ErrNotFound) {
		t.Errorf("Get(deleted) = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.Enable(ctx, id, ""); !errors.Is(err, appwebhook.ErrNotFound) {
		t.Errorf("Enable(deleted) = %v, want ErrNotFound", err)
	}
	if list, _ := e.svc.List(ctx); len(list) != 1 {
		t.Errorf("List after delete = %+v", list)
	}
	entries := e.audit(t, id)
	want := []moderation.AuditAction{
		moderation.AuditWebhookDeleted, moderation.AuditWebhookEnabled, moderation.AuditWebhookDisabled,
		moderation.AuditWebhookUpdated, moderation.AuditWebhookCreated,
	}
	if len(entries) != len(want) {
		t.Fatalf("audit = %+v, want %v", entries, want)
	}
	for i, en := range entries {
		if en.Action != want[i] || en.ActorKind != moderation.AuditActorCLI || strings.Contains(en.TargetLabel, "secret-token") ||
			!strings.Contains(en.TargetLabel, "hooks.example.com") {
			t.Errorf("audit[%d] = %+v, want %s by the CLI labeled with the host only", i, en, want[i])
		}
	}
}

func webhookPermissions(t *testing.T, e *webhookEnv) {
	ctx := t.Context()
	user := e.ids.NewID()
	e.granted[user] = []webhook.Permission{webhook.PermissionManageWebhooks, webhook.PermissionManageDomains}
	settings := webhook.EndpointSettings{URL: "https://hooks.example.com/", Events: []webhook.EventType{
		webhook.EventReportCreated, webhook.EventAccountCreated, webhook.EventDomainModerationActionCreated, webhook.EventStatusCreated,
	}}
	_, err := e.svc.Create(ctx, settings, user)
	var missing *appwebhook.MissingPermissionError
	if !errors.As(err, &missing) || len(missing.Missing) != 2 ||
		missing.Missing[0] != webhook.PermissionManageAccounts || missing.Missing[1] != webhook.PermissionManageReports {
		t.Fatalf("Create = %v, want the missing manage_accounts and manage_reports", err)
	}
	settings.Events = []webhook.EventType{webhook.EventDomainModerationActionLifted, webhook.EventStatusCreated}
	c, err := e.svc.Create(ctx, settings, user)
	if err != nil {
		t.Fatalf("Create with the permissions held: %v", err)
	}
	settings.Events = []webhook.EventType{webhook.EventAppealCreated}
	if _, err := e.svc.Update(ctx, c.Endpoint.ID, settings, user); !errors.As(err, &missing) {
		t.Errorf("Update = %v, want a MissingPermissionError", err)
	}
	if entries := e.audit(t, c.Endpoint.ID); len(entries) != 1 || entries[0].ActorUserID != user {
		t.Errorf("audit = %+v, want the creation by %s", entries, user)
	}
	for _, s := range []webhook.EndpointSettings{
		{URL: "https://hooks.example.com/"},
		{URL: "https://hooks.example.com/", Events: []webhook.EventType{webhook.EventTest}},
		{URL: "https://user@hooks.example.com/", Events: []webhook.EventType{webhook.EventStatusCreated}},
		{URL: "https://hooks.example.com/#x", Events: []webhook.EventType{webhook.EventStatusCreated}},
		{URL: "/relative", Events: []webhook.EventType{webhook.EventStatusCreated}},
		{URL: "https://hooks.example.com/", Description: strings.Repeat("x", webhook.MaxDescription+1), Events: []webhook.EventType{webhook.EventStatusCreated}},
	} {
		if _, err := e.svc.Create(ctx, s, ""); !appwebhook.IsValidation(err) {
			t.Errorf("Create(%+v) = %v, want a validation error", s, err)
		}
	}
}

func webhookSources(t *testing.T, e *webhookEnv) {
	ctx := t.Context()
	// Nothing subscribes yet: nothing is written.
	e.fileReport(t)
	if jobs := e.deliveryJobs(t); len(jobs) != 0 {
		t.Fatalf("jobs without an endpoint = %v", jobs)
	}
	reports := e.create(t, "https://reports.example/hook", webhook.EventReportCreated)
	e.create(t, "https://statuses.example/hook", webhook.EventStatusCreated)
	off := e.create(t, "https://off.example/hook", webhook.EventReportCreated)
	if _, err := e.svc.Disable(ctx, off.Endpoint.ID, ""); err != nil {
		t.Fatal(err)
	}
	r := e.fileReport(t)
	jobs := e.deliveryJobs(t)
	if len(jobs) != 1 {
		t.Fatalf("jobs = %v, want one for the enabled endpoint that subscribes", jobs)
	}
	ds, err := e.svc.Deliveries(ctx, reports.Endpoint.ID, "", 10)
	if err != nil || len(ds) != 1 || ds[0].Delivery.ID != jobs[0] || ds[0].EventType != webhook.EventReportCreated ||
		ds[0].Status() != webhook.DeliveryPending || ds[0].Last != nil {
		t.Fatalf("Deliveries = %+v, %v", ds, err)
	}
	if res := e.svc.Deliver(ctx, jobs[0], false); !res.IsDone() {
		t.Fatalf("Deliver = %+v", res)
	}
	reqs := e.http.take()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d", len(reqs))
	}
	var body struct {
		Type      string          `json:"type"`
		Timestamp string          `json:"timestamp"`
		Data      json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil || body.Type != "report.created" || body.Timestamp == "" {
		t.Fatalf("body %s: %v", reqs[0].Body, err)
	}
	var data map[string]any
	if err := json.Unmarshal(body.Data, &data); err != nil || data["kind"] != "report" || data["id"] != r.ID || data["in_transaction"] != true {
		t.Errorf("data = %s, want report %s presented in the transaction of the event", body.Data, r.ID)
	}
	if _, err := e.svc.Deliveries(ctx, e.ids.NewID(), "", 10); !errors.Is(err, appwebhook.ErrNotFound) {
		t.Errorf("Deliveries(unknown) = %v, want ErrNotFound", err)
	}
}

// signedBy checks the Standard Webhooks headers of a request against the secrets.
func signedBy(t *testing.T, req *kernel.HTTPRequest, secrets ...string) string {
	t.Helper()
	id, ts := req.Header.Get(webhook.HeaderID), req.Header.Get(webhook.HeaderTimestamp)
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || id == "" || req.Method != http.MethodPost || req.Purpose != kernel.HTTPPurposeWebhook ||
		req.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("request %s %s, headers %v", req.Method, req.URL, req.Header)
	}
	sigs := strings.Fields(req.Header.Get(webhook.HeaderSignature))
	if len(sigs) != len(secrets) {
		t.Errorf("signatures %v, want %d", sigs, len(secrets))
	}
	for _, s := range secrets {
		b, err := webhook.ParseSecret(s)
		if err != nil {
			t.Fatal(err)
		}
		if !webhook.Verify(b, req.Header.Get(webhook.HeaderSignature), id, time.Unix(sec, 0), req.Body) {
			t.Errorf("the signatures %v do not verify under %s", sigs, s)
		}
	}
	return id
}

func webhookDeliver(t *testing.T, e *webhookEnv) {
	ctx := t.Context()
	c := e.create(t, "https://hooks.example.com/in", webhook.EventStatusCreated)
	d, err := e.svc.Test(ctx, c.Endpoint.ID)
	if err != nil || d.EventType != webhook.EventTest {
		t.Fatalf("Test = %+v, %v", d, err)
	}
	jobs := e.deliveryJobs(t)
	if len(jobs) != 1 || jobs[0] != d.Delivery.ID {
		t.Fatalf("jobs = %v, want %s", jobs, d.Delivery.ID)
	}
	if res := e.svc.Deliver(ctx, d.Delivery.ID, false); !res.IsDone() {
		t.Fatalf("Deliver = %+v", res)
	}
	reqs := e.http.take()
	if len(reqs) != 1 || reqs[0].URL != "https://hooks.example.com/in" {
		t.Fatalf("requests = %+v", reqs)
	}
	msgID := signedBy(t, reqs[0], c.Secret)
	if !bytes.Contains(reqs[0].Body, []byte(`"type":"webhook.test"`)) || !bytes.Contains(reqs[0].Body, []byte(c.Endpoint.ID)) {
		t.Errorf("body %s", reqs[0].Body)
	}
	got, err := e.svc.Deliveries(ctx, c.Endpoint.ID, "", 10)
	if err != nil || len(got) != 1 || got[0].Status() != webhook.DeliverySucceeded || got[0].Attempts != 1 ||
		got[0].Last == nil || got[0].Last.StatusCode != http.StatusOK || got[0].Last.Outcome != webhook.OutcomeSucceeded {
		t.Fatalf("Deliveries = %+v, %v", got, err)
	}
	// The job again sends nothing; a redelivery sends the same message again.
	if res := e.svc.Deliver(ctx, d.Delivery.ID, false); !res.IsDone() || len(e.http.take()) != 0 {
		t.Errorf("Deliver again = %+v, want nothing sent", res)
	}
	if _, err := e.svc.Redeliver(ctx, c.Endpoint.ID, d.Delivery.ID); err != nil {
		t.Fatalf("Redeliver: %v", err)
	}
	re := e.outbox.take()
	if len(re) != 1 || re[0].IdempotencyKey == appwebhook.JobDeliver+":"+d.Delivery.ID {
		t.Fatalf("redelivery jobs = %+v, want one with its own key", re)
	}
	if res := e.svc.Deliver(ctx, d.Delivery.ID, true); !res.IsDone() {
		t.Fatalf("Deliver(redeliver) = %+v", res)
	}
	reqs = e.http.take()
	if len(reqs) != 1 || signedBy(t, reqs[0], c.Secret) != msgID {
		t.Errorf("redelivery = %+v, want the same webhook-id %s", reqs, msgID)
	}
	if _, err := e.svc.Redeliver(ctx, c.Endpoint.ID, e.ids.NewID()); !errors.Is(err, appwebhook.ErrNotFound) {
		t.Errorf("Redeliver(unknown) = %v, want ErrNotFound", err)
	}
	// A disabled endpoint receives nothing.
	d2, err := e.svc.Test(ctx, c.Endpoint.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Disable(ctx, c.Endpoint.ID, ""); err != nil {
		t.Fatal(err)
	}
	if res := e.svc.Deliver(ctx, d2.Delivery.ID, false); !res.IsDone() || len(e.http.take()) != 0 {
		t.Errorf("Deliver(disabled) = %+v, want nothing sent", res)
	}
	// A delivery that is gone (purged) is done.
	if res := e.svc.Deliver(ctx, e.ids.NewID(), false); !res.IsDone() {
		t.Errorf("Deliver(unknown) = %+v, want Done", res)
	}
}

func webhookGone(t *testing.T, e *webhookEnv) {
	ctx := t.Context()
	c := e.create(t, "https://hooks.example.com/in", webhook.EventStatusCreated)
	d, err := e.svc.Test(ctx, c.Endpoint.ID)
	if err != nil {
		t.Fatal(err)
	}
	e.http.statuses = []int{http.StatusServiceUnavailable, http.StatusFound, http.StatusGone}
	e.http.headers = []http.Header{{"Retry-After": []string{"120"}}}
	res := e.svc.Deliver(ctx, d.Delivery.ID, false)
	if !res.IsRetry() || res.After() != 2*time.Minute {
		t.Errorf("Deliver(503, Retry-After 120) = %+v, want a retry after 2m", res)
	}
	if res := e.svc.Deliver(ctx, d.Delivery.ID, false); !res.IsRetry() || res.After() != 0 {
		t.Errorf("Deliver(302) = %+v, want a retry on the policy's backoff", res)
	}
	got, err := e.svc.Get(ctx, c.Endpoint.ID)
	if err != nil || got.Failures == nil || got.Failures.Consecutive != 2 || got.State != webhook.StateEnabled {
		t.Fatalf("endpoint after two failures = %+v, %v", got, err)
	}
	e.http.err = errors.Join(kernel.ErrForbiddenDestination)
	if res := e.svc.Deliver(ctx, d.Delivery.ID, false); !res.IsRetry() {
		t.Errorf("Deliver(forbidden) = %+v, want a retry", res)
	}
	e.http.err = nil
	ds, _ := e.svc.Deliveries(ctx, c.Endpoint.ID, "", 10)
	if len(ds) != 1 || ds[0].Status() != webhook.DeliveryFailing || ds[0].Attempts != 3 || ds[0].Last.Outcome != webhook.OutcomeForbidden ||
		ds[0].Last.StatusCode != 0 {
		t.Errorf("Deliveries = %+v", ds)
	}
	if res := e.svc.Deliver(ctx, d.Delivery.ID, false); !res.IsDone() {
		t.Errorf("Deliver(410) = %+v, want Done", res)
	}
	got, err = e.svc.Get(ctx, c.Endpoint.ID)
	if err != nil || got.State != webhook.StateDisabled || got.StateReason != webhook.ReasonGone {
		t.Fatalf("endpoint after 410 = %+v, %v", got, err)
	}
	if len(e.disabled) != 1 || e.disabled[0].EndpointID != c.Endpoint.ID || e.disabled[0].Reason != webhook.ReasonGone {
		t.Errorf("EndpointDisabled = %+v", e.disabled)
	}
	entries := e.audit(t, c.Endpoint.ID)
	if len(entries) != 2 || entries[0].Action != moderation.AuditWebhookDisabled || entries[0].ActorKind != moderation.AuditActorSystem {
		t.Errorf("audit = %+v, want the disabling by the system", entries)
	}
}

func webhookFailing(t *testing.T, e *webhookEnv) {
	ctx := t.Context()
	c := e.create(t, "https://hooks.example.com/in", webhook.EventStatusCreated)
	e.http.err = errors.New("connection refused")
	fail := func(n int) {
		t.Helper()
		for range n {
			d, err := e.svc.Test(ctx, c.Endpoint.ID)
			if err != nil {
				t.Fatal(err)
			}
			if res := e.svc.Deliver(ctx, d.Delivery.ID, false); !res.IsRetry() {
				t.Fatalf("Deliver = %+v, want a retry", res)
			}
			e.clock.Advance(4 * time.Hour)
		}
	}
	fail(10)
	// A success ends the run.
	e.http.err = nil
	d, _ := e.svc.Test(ctx, c.Endpoint.ID)
	if res := e.svc.Deliver(ctx, d.Delivery.ID, false); !res.IsDone() {
		t.Fatalf("Deliver = %+v", res)
	}
	if got, _ := e.svc.Get(ctx, c.Endpoint.ID); got.Failures != nil {
		t.Fatalf("failures after a success = %+v", got.Failures)
	}
	e.http.err = errors.New("connection refused")
	// 19 failures over 72 hours, then the 20th: still enabled until both hold.
	fail(webhook.DisableAfterFailures - 1)
	got, _ := e.svc.Get(ctx, c.Endpoint.ID)
	if got.State != webhook.StateEnabled || got.Failures == nil || got.Failures.Consecutive != webhook.DisableAfterFailures-1 {
		t.Fatalf("endpoint after 19 failures = %+v", got)
	}
	fail(1)
	got, _ = e.svc.Get(ctx, c.Endpoint.ID)
	if got.State != webhook.StateDisabled || got.StateReason != webhook.ReasonFailing {
		t.Fatalf("endpoint after 20 failures over 72h = %+v", got)
	}
	if len(e.disabled) != 1 || e.disabled[0].Reason != webhook.ReasonFailing {
		t.Errorf("EndpointDisabled = %+v", e.disabled)
	}
	// Enabling it again keeps the run until a success.
	if _, err := e.svc.Enable(ctx, c.Endpoint.ID, ""); err != nil {
		t.Fatal(err)
	}
}

func webhookRotate(t *testing.T, e *webhookEnv) {
	ctx := t.Context()
	c := e.create(t, "https://hooks.example.com/in", webhook.EventStatusCreated)
	e.clock.Advance(time.Hour)
	next, err := e.svc.RotateSecret(ctx, c.Endpoint.ID, "")
	if err != nil || next == c.Secret {
		t.Fatalf("RotateSecret = %q, %v", next, err)
	}
	send := func() *kernel.HTTPRequest {
		t.Helper()
		d, err := e.svc.Test(ctx, c.Endpoint.ID)
		if err != nil {
			t.Fatal(err)
		}
		if res := e.svc.Deliver(ctx, d.Delivery.ID, false); !res.IsDone() {
			t.Fatalf("Deliver = %+v", res)
		}
		reqs := e.http.take()
		if len(reqs) != 1 {
			t.Fatalf("requests = %d", len(reqs))
		}
		return reqs[0]
	}
	signedBy(t, send(), next, c.Secret)
	e.clock.Advance(webhook.SecretOverlap)
	signedBy(t, send(), next)
	entries := e.audit(t, c.Endpoint.ID)
	if len(entries) != 2 || entries[0].Action != moderation.AuditWebhookSecretRotated {
		t.Errorf("audit = %+v", entries)
	}
}

func webhookPurge(t *testing.T, e *webhookEnv) {
	ctx := t.Context()
	c := e.create(t, "https://hooks.example.com/in", webhook.EventStatusCreated)
	old, _ := e.svc.Test(ctx, c.Endpoint.ID)
	if res := e.svc.Deliver(ctx, old.Delivery.ID, false); !res.IsDone() {
		t.Fatal(res)
	}
	e.clock.Advance(webhook.Retention - time.Hour)
	recent, _ := e.svc.Test(ctx, c.Endpoint.ID)
	e.clock.Advance(2 * time.Hour)
	n, err := e.svc.Purge(ctx)
	if err != nil || n != 1 {
		t.Fatalf("Purge = %d, %v; want 1", n, err)
	}
	ds, _ := e.svc.Deliveries(ctx, c.Endpoint.ID, "", 10)
	if len(ds) != 1 || ds[0].Delivery.ID != recent.Delivery.ID {
		t.Errorf("Deliveries after the purge = %+v, want only %s", ds, recent.Delivery.ID)
	}
	if n, err := e.svc.Purge(ctx); err != nil || n != 0 {
		t.Errorf("Purge again = %d, %v", n, err)
	}
	if res := e.svc.Deliver(ctx, old.Delivery.ID, true); !res.IsDone() {
		t.Errorf("Deliver(purged) = %+v, want Done", res)
	}
}

// webhookRebuild checks that rebuilding the projections derives what the projectors wrote:
// heads after revisions, rotations, state changes, and a deletion, and runs of failures after
// failures, a success, and a purge that removed the first failure of a run, which stays.
func webhookRebuild(t *testing.T, e *webhookEnv) {
	ctx := t.Context()
	revised := e.create(t, "https://hooks.example.com/a", webhook.EventStatusCreated).Endpoint.ID
	if _, err := e.svc.Update(ctx, revised, webhook.EndpointSettings{URL: "https://hooks.example.com/a2", Events: []webhook.EventType{webhook.EventReportCreated}}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.RotateSecret(ctx, revised, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Disable(ctx, revised, ""); err != nil {
		t.Fatal(err)
	}
	deleted := e.create(t, "https://hooks.example.com/d", webhook.EventStatusCreated).Endpoint.ID
	if err := e.svc.Delete(ctx, deleted, ""); err != nil {
		t.Fatal(err)
	}
	attempt := func(id string, ok bool) {
		t.Helper()
		e.http.err = nil
		if !ok {
			e.http.err = errors.New("connection refused")
		}
		d, err := e.svc.Test(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		e.svc.Deliver(ctx, d.Delivery.ID, false)
		e.clock.Advance(time.Minute)
	}
	recovered := e.create(t, "https://hooks.example.com/r", webhook.EventStatusCreated).Endpoint.ID
	failing := e.create(t, "https://hooks.example.com/f", webhook.EventStatusCreated).Endpoint.ID
	purged := e.create(t, "https://hooks.example.com/p", webhook.EventStatusCreated).Endpoint.ID
	healthy := e.create(t, "https://hooks.example.com/h", webhook.EventStatusCreated).Endpoint.ID
	// The purge removes the first failures of purged, whose run goes on.
	attempt(purged, false)
	attempt(purged, false)
	e.clock.Advance(webhook.Retention)
	attempt(purged, false)
	if n, err := e.svc.Purge(ctx); err != nil || n != 2 {
		t.Fatalf("Purge = %d, %v; want 2", n, err)
	}
	attempt(recovered, false)
	attempt(recovered, true)
	attempt(failing, false)
	attempt(failing, true)
	attempt(failing, false)
	attempt(failing, false)
	attempt(healthy, true)
	endpoints := []string{revised, deleted, recovered, failing, purged, healthy}

	type state struct {
		Head     appwebhook.Head
		Failures *webhook.Failures
	}
	read := func() map[string]state {
		t.Helper()
		out := map[string]state{}
		err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			for _, id := range endpoints {
				h, err := e.h.Webhooks.HeadForUpdate(ctx, tx, id)
				if err != nil {
					return err
				}
				f, err := e.h.Webhooks.Failures(ctx, tx, id)
				if err != nil {
					return err
				}
				out[id] = state{Head: h, Failures: f}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("read the projections: %v", err)
		}
		return out
	}
	want := read()
	if f := want[failing].Failures; f == nil || f.Consecutive != 2 {
		t.Errorf("failures of the failing endpoint = %+v, want 2", f)
	}
	if f := want[purged].Failures; f == nil || f.Consecutive != 3 {
		t.Errorf("failures of the purged endpoint = %+v, want 3", f)
	}
	if want[recovered].Failures != nil || want[healthy].Failures != nil || want[deleted].Head.State != webhook.StateDeleted {
		t.Errorf("projections = %+v", want)
	}
	// Spoil the projections, then rebuild them.
	ds, err := e.svc.Deliveries(ctx, healthy, "", 1)
	if err != nil || len(ds) != 1 || ds[0].Last == nil {
		t.Fatalf("Deliveries(healthy) = %+v, %v", ds, err)
	}
	success := ds[0]
	err = e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		stale := want[revised].Head
		stale.State = webhook.StateEnabled
		if err := e.h.Webhooks.UpsertHead(ctx, tx, stale); err != nil {
			return err
		}
		if err := e.h.Webhooks.DeleteFailures(ctx, tx, failing); err != nil {
			return err
		}
		return e.h.Webhooks.AddFailure(ctx, tx, healthy, success.Last.ID, success.Last.AttemptedAt)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := appwebhook.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Webhooks); err != nil {
		t.Fatalf("RebuildProjections = %v", err)
	}
	got := read()
	for _, id := range endpoints {
		g, w := got[id], want[id]
		if g.Head != w.Head || (g.Failures == nil) != (w.Failures == nil) ||
			(g.Failures != nil && (g.Failures.FirstFailureAttemptID != w.Failures.FirstFailureAttemptID ||
				!g.Failures.FirstFailureAt.Equal(w.Failures.FirstFailureAt) || g.Failures.Consecutive != w.Failures.Consecutive)) {
			t.Errorf("rebuilt %s = %+v %+v, want %+v %+v", id, g.Head, g.Failures, w.Head, w.Failures)
		}
	}
}

// webhookConcurrentFailures records failures of one endpoint in concurrent transactions: each
// counts, and the run keeps its first failure.
func webhookConcurrentFailures(t *testing.T, e *webhookEnv) {
	ctx := t.Context()
	id := e.create(t, "https://hooks.example.com/c", webhook.EventStatusCreated).Endpoint.ID
	first, firstAt := e.ids.NewID(), e.clock.Now()
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return e.h.Webhooks.AddFailure(ctx, tx, id, first, firstAt)
	}); err != nil {
		t.Fatalf("AddFailure = %v", err)
	}
	const n = 8
	attempts := make([]string, n)
	for i := range attempts {
		attempts[i] = e.ids.NewID()
	}
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Go(func() {
			errs[i] = e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				return e.h.Webhooks.AddFailure(ctx, tx, id, attempts[i], firstAt.Add(time.Duration(i+1)*time.Second))
			})
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("AddFailure = %v", err)
		}
	}
	var got *webhook.Failures
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		got, err = e.h.Webhooks.Failures(ctx, tx, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Consecutive != n+1 || got.FirstFailureAttemptID != first || !got.FirstFailureAt.Equal(firstAt) {
		t.Errorf("Failures = %+v, want %d failures since %s", got, n+1, first)
	}
}

func webhookErrorGroups(t *testing.T, e *webhookEnv) {
	ctx := t.Context()
	user := e.ids.NewID()
	e.granted[user] = []webhook.Permission{webhook.PermissionManageWebhooks}
	events := []webhook.EventType{webhook.EventErrorGroupCreated, webhook.EventErrorGroupRegressed}
	_, err := e.svc.Create(ctx, webhook.EndpointSettings{URL: "https://errors.example/hook", Events: events}, user)
	var missing *appwebhook.MissingPermissionError
	if !errors.As(err, &missing) || len(missing.Missing) != 1 || missing.Missing[0] != webhook.PermissionManageErrors {
		t.Fatalf("Create without manage_errors = %v, want the missing manage_errors", err)
	}
	e.granted[user] = append(e.granted[user], webhook.PermissionManageErrors)
	if _, err := e.svc.Create(ctx, webhook.EndpointSettings{URL: "https://errors.example/hook", Events: events}, user); err != nil {
		t.Fatalf("Create with manage_errors: %v", err)
	}
	emit := func(ev uow.Event) {
		t.Helper()
		if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return tx.Emit(ctx, ev) }); err != nil {
			t.Fatalf("emit %s: %v", ev.EventType(), err)
		}
	}
	group := func(unauthenticated bool) appdiagnostics.Group {
		return appdiagnostics.Group{ID: e.ids.NewID(), Fingerprint: e.ids.NewID(), Unauthenticated: unauthenticated, FirstSeenAt: e.clock.Now()}
	}
	regressed := func(g appdiagnostics.Group, unauthenticated bool) appdiagnostics.GroupRegressed {
		return appdiagnostics.GroupRegressed{Group: g, Unauthenticated: unauthenticated, Event: appdiagnostics.Event{
			ID: e.ids.NewID(), GroupID: g.ID, Kind: diagnostics.EventRegressed, OccurredAt: e.clock.Now(),
		}}
	}
	anonymous, muted, signed := group(true), group(true), group(false)
	emit(appdiagnostics.GroupCreated{Group: anonymous})
	emit(regressed(anonymous, true))
	emit(appdiagnostics.GroupCreated{Group: muted})
	emit(appdiagnostics.GroupConfirmed{Group: muted, Head: appdiagnostics.Head{GroupID: muted.ID, State: diagnostics.StateMuted}})
	if jobs := e.deliveryJobs(t); len(jobs) != 0 {
		t.Fatalf("jobs of the reports without a session and of a muted group = %v", jobs)
	}
	emit(appdiagnostics.GroupConfirmed{Group: anonymous, Head: appdiagnostics.Head{GroupID: anonymous.ID, State: diagnostics.StateOpen}})
	emit(appdiagnostics.GroupCreated{Group: signed})
	emit(regressed(signed, false))
	jobs := e.deliveryJobs(t)
	if len(jobs) != 3 {
		t.Fatalf("jobs = %v, want 3", jobs)
	}
	want := []struct {
		typ string
		id  string
	}{{"error_group.created", anonymous.ID}, {"error_group.created", signed.ID}, {"error_group.regressed", signed.ID}}
	for i, j := range jobs {
		if res := e.svc.Deliver(ctx, j, false); !res.IsDone() {
			t.Fatalf("Deliver = %+v", res)
		}
		reqs := e.http.take()
		var body struct {
			Type string         `json:"type"`
			Data map[string]any `json:"data"`
		}
		if len(reqs) != 1 || json.Unmarshal(reqs[0].Body, &body) != nil || body.Type != want[i].typ ||
			body.Data["kind"] != "error_group" || body.Data["id"] != want[i].id || body.Data["in_transaction"] != true {
			t.Errorf("delivery %d = %d requests, %+v; want %s of %s", i, len(reqs), body, want[i].typ, want[i].id)
		}
	}
}
