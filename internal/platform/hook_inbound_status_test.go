// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	apphook "github.com/mycroblog/my/internal/app/hook"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/hook"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// statusPlugin answers by what a post says, as a real plugin reads it: a body with "VERDICT_X" gets
// that verdict and any other gets allow. It records what it was sent.
type statusPlugin struct {
	mu       sync.Mutex
	requests []map[string]any
	fail     bool
}

func (p *statusPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	p.mu.Lock()
	p.requests = append(p.requests, req)
	fail := p.fail
	p.mu.Unlock()
	if fail {
		http.Error(w, "broken", http.StatusServiceUnavailable)
		return
	}
	verdict := "VERDICT_ALLOW"
	raw, _ := json.Marshal(req["inboundStatus"])
	for _, v := range []string{"VERDICT_DROP", "VERDICT_FLAG", "VERDICT_MARK_SENSITIVE"} {
		if strings.Contains(string(raw), v) {
			verdict = v
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"verdict": verdict, "reason": "REASON_SPAM", "note": "from the plugin"})
}

func (p *statusPlugin) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.requests)
}

func (p *statusPlugin) setFail(fail bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fail = fail
}

func (p *statusPlugin) last(t *testing.T) map[string]any {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.requests) == 0 {
		t.Fatal("the plugin got no request")
	}
	return p.requests[len(p.requests)-1]
}

// inboundHook is a server with a plugin of inbound.status, a local account alice, and a remote author.
type inboundHook struct {
	t       *testing.T
	c       *statusesClient
	plugin  *statusPlugin
	network *relayNet
	carol   remoteActor
	alice   string // the URI of alice
	n       int
	exec    func(q string)
	count   func(q string) int64
}

func newInboundHook(t *testing.T, mode, onFailure string) *inboundHook {
	t.Helper()
	plugin := &statusPlugin{}
	mux := http.NewServeMux()
	mux.Handle("POST /my.plugin.hook.v1.HookService/Check", plugin)
	addr := fakeplugin.ServeUnix(t, mux)
	network := &relayNet{docs: map[string][]byte{}}
	dbURL := newDatabase(t, true)
	c := newStatusesClientWith(t, map[string]string{"MY_DATABASE_URL": dbURL, "MY_QUEUE": "inline", "MY_PLUGIN_INBOUND_STATUS_ADDRESS": addr},
		platform.AppOptions{HTTPClient: network}, "alice")
	exec, count := rawSQL(t, "sqlite", dbURL)
	h := &inboundHook{
		t: t, c: c, exec: exec, count: count, plugin: plugin, network: network, alice: "https://example.test/ap/actors/" + c.ids["alice"],
		carol: newRemoteActor(t, network, "https://remote.example/users/carol", "Person", "https://remote.example/users/carol/inbox"),
	}
	h.set(instance.SettingHookInboundStatusMode, "observe")
	h.set(instance.SettingHookInboundStatusMode, mode)
	h.set(instance.SettingHookInboundStatusOnFailure, onFailure)
	return h
}

func (h *inboundHook) set(key instance.SettingKey, value string) {
	h.t.Helper()
	if _, err := h.c.app.Instance.SetSetting(h.t.Context(), appinstance.SetSettingInput{Key: key, Value: value}); err != nil {
		h.t.Fatalf("SetSetting(%s, %s) = %v", key, value, err)
	}
}

// verify fails on any drift of the projections.
func (h *inboundHook) verify() {
	h.t.Helper()
	for _, p := range h.c.app.Projections.List() {
		r, err := h.c.app.Projections.Verify(h.t.Context(), p)
		if err != nil {
			h.t.Fatalf("Verify(%s) = %v", p.Name, err)
		}
		if r.Drifted() {
			h.t.Errorf("Verify(%s) drifted: %+v", p.Name, r)
		}
	}
}

const asPublic = "https://www.w3.org/ns/activitystreams#Public"

// note returns a post of carol that mentions alice, so that it is in scope.
func (h *inboundHook) note(text string, at time.Time) map[string]any {
	h.n++
	return map[string]any{
		"id": h.carol.id + "/statuses/" + strconv.Itoa(h.n), "type": "Note", "attributedTo": h.carol.id, "content": "<p>" + text + "</p>",
		"published": at.Format(time.RFC3339), "to": []string{asPublic}, "cc": []string{h.alice},
		"tag": []map[string]any{{"type": "Mention", "href": h.alice}},
	}
}

// send delivers an activity of carol and returns its URI.
func (h *inboundHook) send(typ string, obj map[string]any) string {
	h.t.Helper()
	h.n++
	id := h.carol.id + "/activities/" + strconv.Itoa(h.n)
	if code := inboxCode(h.t, h.c, h.carol, map[string]any{"id": id, "type": typ, "actor": h.carol.id, "object": obj}); code != http.StatusAccepted {
		h.t.Fatalf("%s = %d", typ, code)
	}
	return id
}

func (h *inboundHook) create(obj map[string]any) string {
	h.t.Helper()
	return h.send("Create", obj)
}

// update sends a new revision of obj that says text.
func (h *inboundHook) update(obj map[string]any, text string, at time.Time) string {
	h.t.Helper()
	next := map[string]any{}
	for k, v := range obj {
		next[k] = v
	}
	next["content"], next["updated"] = "<p>"+text+"</p>", at.Format(time.RFC3339)
	return h.send("Update", next)
}

func (h *inboundHook) stored(obj map[string]any) bool {
	h.t.Helper()
	_, err := h.c.app.Content.FindStatusByURI(h.t.Context(), obj["id"].(string))
	return err == nil
}

func (h *inboundHook) statusID(obj map[string]any) string {
	h.t.Helper()
	st, err := h.c.app.Content.FindStatusByURI(h.t.Context(), obj["id"].(string))
	if err != nil {
		h.t.Fatalf("the post %v is not stored: %v", obj["id"], err)
	}
	return st.ID
}

// body returns the text of the current revision of a post.
func (h *inboundHook) body(obj map[string]any) string {
	h.t.Helper()
	p, err := h.c.app.Content.Find(h.t.Context(), h.statusID(obj))
	if err != nil {
		h.t.Fatal(err)
	}
	return p.Revision.Bodies[0].ContentHTML
}

func (h *inboundHook) sensitive(obj map[string]any) bool {
	h.t.Helper()
	return h.count("SELECT COUNT(*) FROM status_sensitivity_overrides WHERE status_id = '"+h.statusID(obj)+"'") > 0
}

func (h *inboundHook) decision(activity string) hook.Record {
	h.t.Helper()
	// The list leaves out allow unless it is asked for.
	for _, outcome := range []hook.Outcome{"", hook.Allow} {
		list, err := h.c.app.Hooks.List(h.t.Context(), apphook.ListQuery{Hook: hook.InboundStatus, Outcome: outcome, Limit: 100})
		if err != nil {
			h.t.Fatal(err)
		}
		for _, r := range list {
			if r.Decision.SubjectKey == activity {
				return r
			}
		}
	}
	h.t.Fatalf("no decision on %s", activity)
	return hook.Record{}
}

func (h *inboundHook) reports() []moderation.ReportRecord {
	h.t.Helper()
	page, err := h.c.app.Moderation.Reports(h.t.Context(), appmoderation.ReportQuery{}, 50)
	if err != nil {
		h.t.Fatal(err)
	}
	return page
}

func TestInboundStatusReachesThePluginOnlyInScope(t *testing.T) {
	t.Parallel()
	h := newInboundHook(t, "enforce", "pass")
	now := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)

	// A post that relates to no local account is dropped before the plugin.
	outside := h.note("no one here", now)
	outside["cc"], outside["tag"] = nil, nil
	h.create(outside)
	if h.stored(outside) || h.plugin.calls() != 0 {
		t.Fatalf("out of scope: stored %t, calls %d", h.stored(outside), h.plugin.calls())
	}

	// In scope, on Create, with what the plugin needs.
	in := h.note("hello alice", now)
	activity := h.create(in)
	if !h.stored(in) || h.plugin.calls() != 1 {
		t.Fatalf("in scope: stored %t, calls %d", h.stored(in), h.plugin.calls())
	}
	req := h.plugin.last(t)
	sub, _ := req["inboundStatus"].(map[string]any)
	if req["hook"] != "inbound.status" || req["subjectKey"] != activity || sub == nil {
		t.Fatalf("request = %v", req)
	}
	if sub["statusUri"] != in["id"] || sub["authorUri"] != h.carol.id || sub["visibility"] != "public" || sub["activityType"] != "Create" {
		t.Errorf("subject = %v", sub)
	}
	if sub["localMentionCount"] != float64(1) || sub["mentionCount"] != float64(1) {
		t.Errorf("mentions of the subject = %v", sub)
	}
	if v, seen := sub["anyLocalFollower"]; seen && v != false {
		t.Errorf("any_local_follower = %v, want false", v)
	}
	if bodies, _ := sub["bodies"].([]any); len(bodies) != 1 || !strings.Contains(bodies[0].(map[string]any)["html"].(string), "hello alice") {
		t.Errorf("bodies = %v", sub["bodies"])
	}

	// Its Update, still in scope, asks again, as its own decision.
	second := h.update(in, "hello again", now.Add(time.Minute))
	if h.plugin.calls() != 2 || h.plugin.last(t)["subjectKey"] != second {
		t.Fatalf("Update in scope: calls %d, request %v", h.plugin.calls(), h.plugin.last(t))
	}
	if h.decision(second).Decision.ID == "" || h.plugin.last(t)["inboundStatus"].(map[string]any)["activityType"] != "Update" {
		t.Error("the Update has no decision of its own")
	}

	// An Update that no longer relates to a local account stores its revision without a question.
	loose := map[string]any{}
	for k, v := range in {
		loose[k] = v
	}
	loose["cc"], loose["tag"] = nil, nil
	h.update(loose, "VERDICT_DROP out of scope", now.Add(2*time.Minute))
	if h.plugin.calls() != 2 {
		t.Errorf("Update out of scope: calls %d", h.plugin.calls())
	}
	if !strings.Contains(h.body(in), "out of scope") {
		t.Errorf("Update out of scope: body %q, want the revision", h.body(in))
	}

	// An Update by someone else of the origin is refused, and not sent.
	dave := newRemoteActor(t, h.network, "https://remote.example/users/dave", "Person", "https://remote.example/users/dave/inbox")
	forged := map[string]any{}
	for k, v := range in {
		forged[k] = v
	}
	forged["attributedTo"] = dave.id
	if code := inboxCode(t, h.c, dave, map[string]any{"id": dave.id + "/activities/forged", "type": "Update", "actor": dave.id, "object": forged}); code != http.StatusAccepted {
		t.Fatalf("forged Update = %d", code)
	}
	if h.plugin.calls() != 2 {
		t.Errorf("a forged Update reached the plugin: calls %d", h.plugin.calls())
	}
}

func TestInboundStatusDrop(t *testing.T) {
	t.Parallel()
	h := newInboundHook(t, "enforce", "pass")
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)

	dropped := h.note("VERDICT_DROP this", now)
	activity := h.create(dropped)
	if h.stored(dropped) {
		t.Fatal("a dropped post is stored")
	}
	var page clientapi.NotificationPage
	if res := h.c.do("alice", http.MethodGet, "/notifications", nil, &page); res.StatusCode != http.StatusOK || len(page.Items) != 0 {
		t.Fatalf("notifications = %d, %+v; want none", res.StatusCode, page.Items)
	}
	rec := h.decision(activity)
	if rec.Decision.Outcome != hook.Drop || rec.Decision.AccountID == "" || rec.Decision.Reason != hook.ReasonSpam {
		t.Fatalf("decision = %+v", rec.Decision)
	}

	// A redelivery is a duplicate, and the job that runs again reads the decision.
	calls := h.plugin.calls()
	if code := inboxCode(t, h.c, h.carol, map[string]any{"id": activity, "type": "Create", "actor": h.carol.id, "object": dropped}); code != http.StatusAccepted {
		t.Fatalf("redelivery = %d", code)
	}
	if outcome, err := h.c.app.InboxProcessor.Process(ctx, federation.IRI(activity), federation.TrustSigned); err != nil || outcome != appfederation.InboxDropped {
		t.Fatalf("Process again = %s, %v", outcome, err)
	}
	if h.plugin.calls() != calls || h.stored(dropped) {
		t.Fatalf("after the redelivery: calls %d, want %d; stored %t", h.plugin.calls(), calls, h.stored(dropped))
	}

	// Cancelling the decision processes the activity again with no call: it is the post's way back.
	if _, err := h.c.app.Hooks.Override(ctx, rec.Decision.ID, h.c.userIDs["alice"]); err != nil {
		t.Fatal(err)
	}
	if !h.stored(dropped) || h.plugin.calls() != calls {
		t.Fatalf("after the cancellation: stored %t, calls %d, want %d", h.stored(dropped), h.plugin.calls(), calls)
	}

	// A drop of a revision makes none and keeps the previous one.
	kept := h.note("the first words", now)
	h.create(kept)
	update := h.update(kept, "VERDICT_DROP the second words", now.Add(time.Minute))
	if b := h.body(kept); !strings.Contains(b, "first") || strings.Contains(b, "second") {
		t.Fatalf("body after a dropped Update = %q", b)
	}
	if got := h.decision(update).Decision.Outcome; got != hook.Drop {
		t.Fatalf("decision of the Update = %s", got)
	}

	// Past the retention of the received document, cancelling brings nothing back.
	gone := h.note("VERDICT_DROP too late", now)
	late := h.create(gone)
	h.exec("DELETE FROM received_activity_documents WHERE activity_uri = '" + late + "'")
	if _, err := h.c.app.Hooks.Override(ctx, h.decision(late).Decision.ID, ""); err != nil {
		t.Fatal(err)
	}
	if h.stored(gone) {
		t.Error("a post came back without its document")
	}
}

func TestInboundStatusFlag(t *testing.T) {
	t.Parallel()
	h := newInboundHook(t, "enforce", "pass")
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)

	flagged := h.note("VERDICT_FLAG this", now)
	activity := h.create(flagged)
	if !h.stored(flagged) {
		t.Fatal("a flag stops the post")
	}
	reports := h.reports()
	if len(reports) != 1 {
		t.Fatalf("reports = %+v, want one", reports)
	}
	r := reports[0].Report
	instanceActor, err := h.c.app.Accounts.EnsureInstanceActor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r.ReporterAccountID != instanceActor.ID || r.URI != "" || r.Forwarded || r.Category != moderation.CategorySpam || r.Comment != "from the plugin" ||
		len(r.StatusIDs) != 1 || r.StatusIDs[0] != h.statusID(flagged) {
		t.Errorf("report = %+v", r)
	}
	rec := h.decision(activity)
	if o := r.HookOrigin; o == nil || o.DecisionID != rec.Decision.ID || o.Hook != "inbound.status" || o.ReasonCode != "spam" {
		t.Errorf("origin = %+v", r.HookOrigin)
	}

	// The admin API says it was automated.
	var detail clientapi.AdminReportDetail
	if res := h.c.do("alice", http.MethodGet, "/admin/reports/"+r.ID, nil, &detail); res.StatusCode != http.StatusOK {
		t.Fatalf("GET report = %d", res.StatusCode)
	}
	if detail.HookOrigin == nil || detail.HookOrigin.Hook != clientapi.HookNameInboundStatus || detail.HookOrigin.DecisionId != rec.Decision.ID {
		t.Errorf("hook_origin = %+v", detail.HookOrigin)
	}
	// It is not forwarded, and a moderator can forward it to the author's server.
	if detail.Forwarded || !detail.Forwardable {
		t.Errorf("forwarded %v, forwardable %v; want a report a moderator can forward", detail.Forwarded, detail.Forwardable)
	}

	// A second run of the same decision files nothing more.
	err = h.c.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		filed, err := h.c.app.Moderation.FileHookReport(ctx, tx, appmoderation.HookReportInput{
			ReporterAccountID: instanceActor.ID, TargetAccountID: r.TargetAccountID, StatusIDs: r.StatusIDs,
			DecisionID: rec.Decision.ID, Hook: "inbound.status", ReasonCode: "spam",
		})
		if filed {
			t.Error("a decision filed a second report")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(h.reports()); got != 1 {
		t.Errorf("reports = %d after a second run", got)
	}

	// Each decision files its own report.
	h.update(flagged, "VERDICT_FLAG again", now.Add(time.Minute))
	if got := len(h.reports()); got != 2 {
		t.Errorf("reports = %d after a flagged revision, want 2", got)
	}

	// Resolving it tells nobody: the instance actor is no reader.
	if _, err := h.c.app.Moderation.ResolveReport(ctx, r.ID, moderation.OutcomeDismissed, ""); err != nil {
		t.Fatal(err)
	}
	notified, err := h.c.app.Notifications.List(ctx, instanceActor.ID, appnotification.Query{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range notified {
		if n.Type == notification.TypeReportResolved {
			t.Errorf("the instance actor got %+v", n)
		}
	}

	// A flag in observe records and files nothing.
	h.set(instance.SettingHookInboundStatusMode, "observe")
	before := len(h.reports())
	h.create(h.note("VERDICT_FLAG quietly", now))
	if got := len(h.reports()); got != before {
		t.Errorf("a flag in observe filed a report: %d, want %d", got, before)
	}
}

func TestInboundStatusMarkSensitive(t *testing.T) {
	t.Parallel()
	h := newInboundHook(t, "enforce", "pass")
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)

	marked := h.note("VERDICT_MARK_SENSITIVE this", now)
	activity := h.create(marked)
	if !h.sensitive(marked) {
		t.Fatal("a mark_sensitive left no mark")
	}
	plain := h.note("nothing to see", now)
	h.create(plain)
	if h.sensitive(plain) {
		t.Fatal("an allow left a mark")
	}

	// A later revision that the plugin allows keeps the mark.
	h.update(marked, "clean now", now.Add(time.Minute))
	if !h.sensitive(marked) {
		t.Fatal("a later revision took the mark away")
	}

	// A revision that gets the mark, of a post that had none.
	h.update(plain, "VERDICT_MARK_SENSITIVE later", now.Add(time.Minute))
	if !h.sensitive(plain) {
		t.Fatal("a revision left no mark")
	}

	// The projection comes back from the decisions alone.
	h.verify()

	// A moderator's cancellation is what ends it, and only its own source.
	h.exec("INSERT INTO status_sensitivity_overrides (status_id, source_kind, source_id) VALUES ('" + h.statusID(marked) + "', 'action', '" + h.c.app.IDs.NewID() + "')")
	if _, err := h.c.app.Hooks.Override(ctx, h.decision(activity).Decision.ID, ""); err != nil {
		t.Fatal(err)
	}
	if !h.sensitive(marked) {
		t.Fatal("cancelling a decision took away the mark of a moderation action")
	}
	h.exec("DELETE FROM status_sensitivity_overrides WHERE source_kind = 'action'")
	if h.sensitive(marked) {
		t.Fatal("the mark stayed after its sources ended")
	}
	h.verify()
}

func TestInboundStatusUnfollowedDirect(t *testing.T) {
	t.Parallel()
	h := newInboundHook(t, "enforce", "pass")
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)

	direct := func(text string) map[string]any {
		n := h.note(text, now)
		n["to"], n["cc"] = []string{h.alice}, nil
		return n
	}
	unsent := direct("VERDICT_DROP between us")
	h.create(unsent)
	if !h.stored(unsent) || h.plugin.calls() != 0 {
		t.Fatalf("direct from an unfollowed author: stored %t, calls %d", h.stored(unsent), h.plugin.calls())
	}

	h.set(instance.SettingHookInboundStatusUnfollowedDirect, "true")
	sent := direct("between us again")
	h.create(sent)
	if h.plugin.calls() != 1 {
		t.Fatalf("with the setting on: calls %d", h.plugin.calls())
	}
	if v := h.plugin.last(t)["inboundStatus"].(map[string]any)["visibility"]; v != "direct" {
		t.Errorf("visibility = %v", v)
	}

	// From an author someone follows, it goes either way.
	h.set(instance.SettingHookInboundStatusUnfollowedDirect, "false")
	carol, err := h.c.app.Accounts.FindByURI(ctx, h.carol.id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.c.app.Graph.Follow(ctx, h.c.ids["alice"], carol.Account.ID); err != nil {
		t.Fatal(err)
	}
	var follow any
	for _, post := range h.network.taken()[h.carol.id+"/inbox"] {
		if post["type"] == "Follow" {
			follow = post["id"]
		}
	}
	postInbox(t, h.c, h.carol, map[string]any{"id": h.carol.id + "/accepts/1", "type": "Accept", "actor": h.carol.id, "object": follow})
	h.create(direct("from someone followed"))
	if h.plugin.calls() != 2 {
		t.Fatalf("direct from a followed author: calls %d", h.plugin.calls())
	}
	if v := h.plugin.last(t)["inboundStatus"].(map[string]any)["anyLocalFollower"]; v != true {
		t.Errorf("any_local_follower = %v", v)
	}
}

func TestInboundStatusWhenThePluginFails(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)

	t.Run("pass", func(t *testing.T) {
		t.Parallel()
		h := newInboundHook(t, "enforce", "pass")
		h.plugin.setFail(true)
		n := h.note("VERDICT_DROP unseen", now)
		activity := h.create(n)
		if !h.stored(n) {
			t.Fatal("the post stopped when the plugin failed")
		}
		if got := h.decision(activity).Decision.Outcome; got != hook.FailedError {
			t.Errorf("outcome = %s", got)
		}
	})

	t.Run("retry", func(t *testing.T) {
		t.Parallel()
		h := newInboundHook(t, "enforce", "retry")
		ctx := t.Context()
		h.plugin.setFail(true)
		n := h.note("hello", now)
		h.n++
		id := h.carol.id + "/activities/" + strconv.Itoa(h.n)
		if code := inboxCode(t, h.c, h.carol, map[string]any{"id": id, "type": "Create", "actor": h.carol.id, "object": n}); code != http.StatusAccepted {
			t.Fatalf("Create = %d", code)
		}
		if h.stored(n) {
			t.Fatal("the post was stored though the job must run again")
		}
		// The job fails unchanged, so the queue's policy retries it; a dead job releases the key
		// (testkit: "A dead job no longer holds back its key").
		payload, err := json.Marshal(map[string]any{"activity_uri": id, "trust": federation.TrustSigned})
		if err != nil {
			t.Fatal(err)
		}
		res := h.c.app.InboxProcessor.HandleJob(ctx, queue.Job{Type: appfederation.JobInbox, Payload: payload})
		if !res.IsRetry() || !errors.Is(res.Err(), apphook.ErrRetryLater) {
			t.Fatalf("HandleJob = retry %t, %v", res.IsRetry(), res.Err())
		}
		list, err := h.c.app.Hooks.List(ctx, apphook.ListQuery{Hook: hook.InboundStatus, Limit: 10})
		if err != nil || len(list) != 0 {
			t.Fatalf("decisions = %+v, %v; want none", list, err)
		}
		h.plugin.setFail(false)
		if res := h.c.app.InboxProcessor.HandleJob(ctx, queue.Job{Type: appfederation.JobInbox, Payload: payload}); !res.IsDone() {
			t.Fatalf("HandleJob after the outage = %v", res.Err())
		}
		if !h.stored(n) {
			t.Fatal("the post is not stored after the outage")
		}
	})
}

// TestInboundStatusOffAsksNothing covers the mode off, and a relayed post, which never asks.
func TestInboundStatusOffAsksNothing(t *testing.T) {
	t.Parallel()
	h := newInboundHook(t, "off", "pass")
	now := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	n := h.note("VERDICT_DROP", now)
	h.create(n)
	h.update(n, "VERDICT_DROP again", now.Add(time.Minute))
	if !h.stored(n) || h.plugin.calls() != 0 {
		t.Fatalf("off: stored %t, calls %d", h.stored(n), h.plugin.calls())
	}
}

// TestInboundStatusRelayedPostAsksNothing runs the path of a post that only a relay brought in.
func TestInboundStatusRelayedPostAsksNothing(t *testing.T) {
	t.Parallel()
	h := newInboundHook(t, "enforce", "pass")
	c := h.c
	relay := newRemoteActor(t, h.network, "https://relay.example/actor", "Application", "https://relay.example/inbox")
	if res := c.do("alice", http.MethodPost, "/admin/relays", map[string]any{"url": "https://relay.example/inbox"}, nil); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /admin/relays = %d", res.StatusCode)
	}
	follow := h.network.taken()["https://relay.example/inbox"][0]
	postInbox(t, c, relay, map[string]any{
		"id": "https://relay.example/activities/accept", "type": "Accept", "actor": relay.id,
		"object": map[string]any{"id": follow["id"], "type": "Follow", "actor": follow["actor"], "object": asPublic},
	})
	id := h.carol.id + "/statuses/relayed"
	doc := map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams", "id": id, "type": "Note", "attributedTo": h.carol.id,
		"content": "<p>VERDICT_DROP from afar</p>", "published": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), "to": []string{asPublic},
	}
	h.network.serve(t, id, doc)
	postInbox(t, c, relay, map[string]any{
		"id": id + "/activity", "type": "Create", "actor": h.carol.id, "to": []string{asPublic},
		"object": map[string]any{"id": id, "type": "Note", "attributedTo": h.carol.id, "content": "forged"},
	})
	if !h.stored(doc) || h.plugin.calls() != 0 {
		t.Fatalf("relayed: stored %t, calls %d", h.stored(doc), h.plugin.calls())
	}

	// Its Update, which a relay carries too and no local account relates to, stores with no call.
	doc["content"], doc["updated"] = "<p>VERDICT_DROP a new revision</p>", time.Now().UTC().Format(time.RFC3339)
	h.network.serve(t, id, doc)
	postInbox(t, c, relay, map[string]any{
		"id": id + "/update", "type": "Update", "actor": h.carol.id, "to": []string{asPublic},
		"object": map[string]any{"id": id, "type": "Note", "attributedTo": h.carol.id, "content": "forged"},
	})
	if !strings.Contains(h.body(doc), "new revision") || h.plugin.calls() != 0 {
		t.Fatalf("relayed Update: body %q, calls %d", h.body(doc), h.plugin.calls())
	}
}

// TestStatusSensitivitySourceIsExclusive checks the key on both dialects: every row has exactly one
// source of a known kind, and a source marks a status once (ADR-0147 §8).
func TestStatusSensitivitySourceIsExclusive(t *testing.T) {
	t.Parallel()
	const a, b, st = "018f0000-0000-7000-8000-000000000001", "018f0000-0000-7000-8000-000000000002", "018f0000-0000-7000-8000-000000000003"
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			t.Parallel()
			dbURL := newDatabase(t, true)
			if dialect == "postgres" {
				dbURL = newPostgresDatabase(t)
			}
			cfg := loadConfig(t, map[string]string{"MY_DATABASE": dialect, "MY_DATABASE_URL": dbURL, "MY_WEB_ORIGIN": webOrigin, "MY_MAIL": "none"})
			app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = app.Close() })
			try := func(q string) error {
				if dialect == "postgres" {
					conn, err := pgx.Connect(t.Context(), dbURL)
					if err != nil {
						t.Fatal(err)
					}
					defer func() { _ = conn.Close(context.Background()) }()
					_, err = conn.Exec(t.Context(), q)
					return err
				}
				db, err := sql.Open("sqlite", dbURL)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = db.Close() }()
				_, err = db.ExecContext(t.Context(), q)
				return err
			}
			const insert = "INSERT INTO status_sensitivity_overrides (status_id, source_kind, source_id) VALUES "
			for _, values := range []string{"('" + st + "', NULL, '" + a + "')", "('" + st + "', 'action', NULL)", "('" + st + "', 'report', '" + a + "')"} {
				if try(insert+values) == nil {
					t.Errorf("the table took %s", values)
				}
			}
			for _, values := range []string{"('" + st + "', 'action', '" + a + "')", "('" + st + "', 'hook_decision', '" + b + "')"} {
				if err := try(insert + values); err != nil {
					t.Errorf("the table refused %s: %v", values, err)
				}
			}
			for _, values := range []string{"('" + st + "', 'action', '" + a + "')", "('" + st + "', 'hook_decision', '" + b + "')"} {
				if try(insert+values) == nil {
					t.Errorf("the table took a second row of %s", values)
				}
			}
		})
	}
}
