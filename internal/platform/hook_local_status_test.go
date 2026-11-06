// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/hook"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// TestLocalStatusHook runs the local.status hook on the assembled server, from POST /statuses to a
// real plugin over a Unix socket, and the appeal of a refused post through its review.
func TestLocalStatusHook(t *testing.T) {
	t.Parallel()
	plugin := new(followPlugin)
	plugin.verdict.Store("VERDICT_REJECT")
	mux := http.NewServeMux()
	mux.Handle("POST /my.plugin.hook.v1.HookService/Check", plugin)
	clock := laterClock{steadyClock: newSteadyClock(), ahead: &atomic.Int64{}}
	c := newStatusesClientWith(t, map[string]string{
		"MY_QUEUE": "inline", "MY_PLUGIN_LOCAL_STATUS_ADDRESS": fakeplugin.ServeUnix(t, mux),
	}, platform.AppOptions{Clock: clock}, "alice", "bob", "carol")
	ctx := t.Context()

	set := func(key instance.SettingKey, value string) {
		t.Helper()
		if _, err := c.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: key, Value: value}); err != nil {
			t.Fatal(err)
		}
	}
	// decisionOf finds a reject by the ID of its decision or of its subject.
	decisionOf := func(id string) hook.Record {
		t.Helper()
		records, err := c.app.Hooks.List(ctx, apphook.ListQuery{Hook: hook.LocalStatus, Outcome: hook.Reject, Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range records {
			if r.Decision.SubjectKey == id || r.Decision.ID == id {
				return r
			}
		}
		t.Fatalf("no decision %s in %+v", id, records)
		return hook.Record{}
	}
	text := func(s string) map[string]any { return map[string]any{"text": s, "visibility": "public"} }
	rejected := func(user, body string) clientapi.StatusRejectedProblem {
		t.Helper()
		res := c.do(user, http.MethodPost, "/statuses", text(body), nil)
		var p clientapi.StatusRejectedProblem
		if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeStatusRejected || p.HookDecisionId == nil || p.AppealDeadline == nil {
			t.Fatalf("POST /statuses = %d %+v", res.StatusCode, p)
		}
		return p
	}

	// The information page states the account age the posts are sent for, the default 7 days.
	var info clientapi.Instance
	if res := c.do("alice", http.MethodGet, "/instance", nil, &info); res.StatusCode != http.StatusOK || info.ExternalReviewScope == nil ||
		info.ExternalReviewScope.LocalStatusAccountAgeDays == nil || *info.ExternalReviewScope.LocalStatusAccountAgeDays != 7 ||
		info.ExternalReviewScope.InboundStatusUnfollowedDirect != nil {
		t.Errorf("GET /instance = %d, external_review_scope %+v", res.StatusCode, info.ExternalReviewScope)
	}

	// observe is the default: the verdict is recorded under the ID the post got, and the post goes on.
	observed := c.post("alice", text("buy now"))
	if d := decisionOf(observed.Id).Decision; d.Mode != hook.ModeObserve || d.AccountID != c.ids["alice"] {
		t.Errorf("the decision in observe = %+v", d)
	}
	body := plugin.last.Load().(map[string]any)
	sent, _ := body["localStatus"].(map[string]any)
	if body["hook"] != "local.status" || body["subjectKey"] != observed.Id || sent["text"] != "buy now" || sent["authorUsername"] != "alice" ||
		sent["visibility"] != "public" || sent["accountCreatedAt"] == nil {
		t.Errorf("the request = %v", body)
	}

	set(instance.SettingHookLocalStatusMode, instance.HookModeEnforce)

	// A reject shows fixed text and the decision to appeal; nothing is posted.
	p := rejected("alice", "buy now, really")
	if p.Title != "This post was not published." || p.Detail == nil {
		t.Errorf("the problem = %+v", p)
	}
	upheldID := *p.HookDecisionId
	if d := decisionOf(upheldID).Decision; d.Mode != hook.ModeEnforce || d.ID != upheldID {
		t.Errorf("the decision = %+v", d)
	}

	// Private and direct posts are never sent; a replay of an Idempotency-Key returns before the hook.
	calls := plugin.calls.Load()
	c.post("alice", map[string]any{"text": "to my followers", "visibility": "private"})
	plugin.verdict.Store("VERDICT_ALLOW")
	keyed := func() *http.Response {
		req := map[string]any{"text": "once", "visibility": "public"}
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, clientapi.BasePath+"/statuses", bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+c.tokens["alice"])
		r.Header.Set("Idempotency-Key", "k-1")
		return c.spec.Serve(t, c.h, r)
	}
	if res := keyed(); res.StatusCode != http.StatusCreated {
		t.Fatalf("the first keyed post = %d", res.StatusCode)
	}
	if got := plugin.calls.Load(); got != calls+1 {
		t.Errorf("the plugin was called %d times for a private post and a keyed post", got-calls)
	}
	plugin.verdict.Store("VERDICT_REJECT")
	if res := keyed(); res.StatusCode != http.StatusCreated {
		t.Errorf("the replay = %d", res.StatusCode)
	}
	if got := plugin.calls.Load(); got != calls+1 {
		t.Error("the replay called the plugin")
	}

	// A plugin failure never stops a post.
	plugin.verdict.Store("")
	failed := c.post("alice", text("while the plugin is down"))
	records, err := c.app.Hooks.List(ctx, apphook.ListQuery{Hook: hook.LocalStatus, Outcome: hook.FailedError, Limit: 10})
	if err != nil || len(records) != 1 || records[0].Decision.SubjectKey != failed.Id {
		t.Errorf("the failure = %+v, %v", records, err)
	}
	plugin.verdict.Store("VERDICT_REJECT")

	// The author appeals with the text; no plugin is called, and only the author can.
	calls = plugin.calls.Load()
	appeal := func(user, decisionID string) *http.Response {
		return c.do(user, http.MethodPost, "/hook_decisions/"+decisionID+"/appeal",
			map[string]any{"status_text": "buy now, really", "comment": "It is my shop."}, nil)
	}
	var own clientapi.OwnHookAppeal
	if res := c.do("alice", http.MethodPost, "/hook_decisions/"+upheldID+"/appeal",
		map[string]any{"status_text": "buy now, really"}, &own); res.StatusCode != http.StatusOK || own.State != clientapi.HookAppealStatePending {
		t.Fatalf("the appeal = %d %+v", res.StatusCode, own)
	}
	if res := appeal("alice", upheldID); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("a second appeal = %d", res.StatusCode)
	}
	if res := appeal("bob", upheldID); res.StatusCode != http.StatusNotFound {
		t.Errorf("another account's appeal = %d", res.StatusCode)
	}
	if res := appeal("alice", observed.Id); res.StatusCode != http.StatusNotFound {
		t.Errorf("an appeal of no decision = %d", res.StatusCode)
	}
	// The author reads where it stands; nobody else can.
	readAppeal := func(user, decisionID string) (clientapi.OwnHookAppeal, int) {
		t.Helper()
		var got clientapi.OwnHookAppeal
		res := c.do(user, http.MethodGet, "/hook_decisions/"+decisionID+"/appeal", nil, &got)
		return got, res.StatusCode
	}
	if got, code := readAppeal("alice", upheldID); code != http.StatusOK || got.Id != own.Id || got.State != clientapi.HookAppealStatePending || got.DecidedAt != nil {
		t.Errorf("GET the appeal = %d %+v", code, got)
	}
	for _, read := range []struct{ user, id string }{{"bob", upheldID}, {"alice", observed.Id}} {
		if _, code := readAppeal(read.user, read.id); code != http.StatusNotFound {
			t.Errorf("GET the appeal of %s as %s = %d", read.id, read.user, code)
		}
	}
	// The moderators are told of it, the author is not.
	notified := func(user string, typ clientapi.NotificationType) []clientapi.Notification {
		t.Helper()
		var page clientapi.NotificationPage
		if res := c.do(user, http.MethodGet, "/notifications", nil, &page); res.StatusCode != http.StatusOK {
			t.Fatalf("GET /notifications as %s = %d", user, res.StatusCode)
		}
		var out []clientapi.Notification
		for _, n := range page.Items {
			if n.Type == typ {
				out = append(out, n)
			}
		}
		return out
	}
	if got := notified("bob", clientapi.NotificationTypeAdminHookAppeal); len(got) != 1 || got[0].HookAppealId == nil || *got[0].HookAppealId != own.Id {
		t.Errorf("the moderator's notifications = %+v", got)
	}
	if got := notified("alice", clientapi.NotificationTypeAdminHookAppeal); len(got) != 0 {
		t.Errorf("the author was told of their own appeal: %+v", got)
	}

	// A moderator upholds it: the decision stays and the author is still reviewed.
	var page clientapi.AdminHookAppealPage
	if res := c.do("bob", http.MethodGet, "/admin/hook_appeals?state=pending", nil, &page); res.StatusCode != http.StatusOK ||
		len(page.Items) != 1 || page.Items[0].StatusText != "buy now, really" || page.Items[0].Decision.Id != upheldID {
		t.Fatalf("the pending appeals = %+v", page)
	}
	var decided clientapi.AdminHookAppeal
	if res := c.do("bob", http.MethodPost, "/admin/hook_appeals/"+own.Id+"/uphold", nil, &decided); res.StatusCode != http.StatusOK ||
		decided.State != clientapi.HookAppealStateUpheld || decided.Decision.OverriddenAt != nil {
		t.Fatalf("uphold = %d %+v", res.StatusCode, decided)
	}
	if res := c.do("bob", http.MethodPost, "/admin/hook_appeals/"+own.Id+"/overturn", nil, nil); res.StatusCode != http.StatusConflict {
		t.Errorf("overturning an upheld appeal = %d", res.StatusCode)
	}
	// The author is told, reads the outcome, and the review is audited.
	if got := notified("alice", clientapi.NotificationTypeHookAppealDecided); len(got) != 1 || got[0].HookAppeal == nil ||
		got[0].HookAppeal.Id != own.Id || got[0].HookAppeal.State != clientapi.HookAppealStateUpheld || got[0].Account != nil {
		t.Errorf("the author's notifications = %+v", got)
	}
	if got, code := readAppeal("alice", upheldID); code != http.StatusOK || got.State != clientapi.HookAppealStateUpheld || got.DecidedAt == nil {
		t.Errorf("GET the upheld appeal = %d %+v", code, got)
	}
	audit, err := c.app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetHookDecision}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 1 || audit[0].Action != moderation.AuditHookAppealUpheld || audit[0].ActorUserID != c.userIDs["bob"] || audit[0].TargetID != upheldID {
		t.Errorf("the audit log = %+v", audit)
	}
	if got := plugin.calls.Load(); got != calls {
		t.Errorf("the appeal and its review called the plugin %d times", got-calls)
	}
	// An ID that is not one never reaches the database.
	for _, path := range []string{"/admin/hook_appeals/x/uphold", "/admin/hook_decisions/x/override"} {
		if res := c.do("bob", http.MethodPost, path, nil, nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("POST %s = %d", path, res.StatusCode)
		}
	}
	for _, path := range []string{"/admin/hook_appeals?cursor=eA", "/admin/hook_decisions?cursor=eA"} {
		if res := c.do("bob", http.MethodGet, path, nil, nil); res.StatusCode != http.StatusBadRequest {
			t.Errorf("GET %s = %d", path, res.StatusCode)
		}
	}

	// Overturning cancels the decision and takes the account out of the hook.
	second := rejected("alice", "buy now, one more time")
	if res := appeal("alice", *second.HookDecisionId); res.StatusCode != http.StatusOK {
		t.Fatalf("the second appeal = %d", res.StatusCode)
	}
	if res := c.do("bob", http.MethodGet, "/admin/hook_appeals?state=pending", nil, &page); res.StatusCode != http.StatusOK || len(page.Items) != 1 {
		t.Fatalf("the pending appeals = %+v", page)
	}
	if res := c.do("bob", http.MethodPost, "/admin/hook_appeals/"+page.Items[0].Id+"/overturn", nil, &decided); res.StatusCode != http.StatusOK ||
		decided.State != clientapi.HookAppealStateOverturned || decided.Decision.OverriddenAt == nil {
		t.Fatalf("overturn = %d %+v", res.StatusCode, decided)
	}
	if d := decisionOf(*second.HookDecisionId); d.Override == nil || d.Override.ByUserID != c.userIDs["bob"] {
		t.Errorf("the overturned decision = %+v", d)
	}
	if got := notified("alice", clientapi.NotificationTypeHookAppealDecided); len(got) != 2 || got[0].HookAppeal == nil ||
		got[0].HookAppeal.State != clientapi.HookAppealStateOverturned {
		t.Errorf("the author's notifications after overturning = %+v", got)
	}
	// The list keeps the appeals of one state.
	for state, want := range map[string]string{"upheld": own.Id, "overturned": decided.Id} {
		page = clientapi.AdminHookAppealPage{}
		if res := c.do("bob", http.MethodGet, "/admin/hook_appeals?state="+state, nil, &page); res.StatusCode != http.StatusOK ||
			len(page.Items) != 1 || page.Items[0].Id != want {
			t.Errorf("the %s appeals = %d %+v", state, res.StatusCode, page)
		}
	}
	if res := c.do("bob", http.MethodGet, "/admin/hook_appeals?state=gone", nil, nil); res.StatusCode != http.StatusBadRequest {
		t.Errorf("an unknown state = %d", res.StatusCode)
	}
	calls = plugin.calls.Load()
	c.post("alice", text("buy now, one more time"))
	if got := plugin.calls.Load(); got != calls {
		t.Error("an exempt account was sent to the plugin")
	}
	rejected("bob", "spam from bob")

	// A flag in enforce posts the post and files one report of it, by the instance actor, with the decision as its origin.
	plugin.verdict.Store("VERDICT_FLAG")
	flagged := c.post("bob", text("maybe spam from bob"))
	plugin.verdict.Store("VERDICT_REJECT")
	if res := c.do("carol", http.MethodGet, "/statuses/"+flagged.Id, nil, nil); res.StatusCode != http.StatusOK {
		t.Errorf("GET the flagged post = %d", res.StatusCode)
	}
	reports, err := c.app.Moderation.Reports(ctx, appmoderation.ReportQuery{}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("reports = %+v, want one", reports)
	}
	instanceActor, err := c.app.Accounts.EnsureInstanceActor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	report := reports[0].Report
	if report.ReporterAccountID != instanceActor.ID || report.TargetAccountID != c.ids["bob"] || report.Category != moderation.CategorySpam ||
		len(report.StatusIDs) != 1 || report.StatusIDs[0] != flagged.Id {
		t.Errorf("the report = %+v", report)
	}
	flags, err := c.app.Hooks.List(ctx, apphook.ListQuery{Hook: hook.LocalStatus, Outcome: hook.Flag, Limit: 10})
	if err != nil || len(flags) != 1 || flags[0].Decision.SubjectKey != flagged.Id {
		t.Fatalf("the flag = %+v, %v", flags, err)
	}
	if o := report.HookOrigin; o == nil || o.DecisionID != flags[0].Decision.ID || o.Hook != "local.status" || o.ReasonCode != "spam" {
		t.Errorf("the origin = %+v", report.HookOrigin)
	}

	// An edit is reviewed too, each as its own subject: a reject refuses that edit alone, with no
	// appeal, and leaves the post as it was; a flag stores the edit and files a report.
	edit := func(body string) *http.Response {
		return c.do("bob", http.MethodPut, "/statuses/"+flagged.Id, map[string]any{"text": body}, nil)
	}
	res := edit("edited into spam")
	var refused clientapi.Problem
	if err := json.NewDecoder(res.Body).Decode(&refused); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusUnprocessableEntity || refused.Type != clientapi.ProblemTypeStatusRejected || refused.Title != "This edit was not saved." {
		t.Fatalf("the refused edit = %d %+v", res.StatusCode, refused)
	}
	var shown clientapi.Status
	if res := c.do("bob", http.MethodGet, "/statuses/"+flagged.Id, nil, &shown); res.StatusCode != http.StatusOK || !bytes.Contains([]byte(shown.Content), []byte("maybe spam from bob")) {
		t.Errorf("the post after a refused edit = %d %q", res.StatusCode, shown.Content)
	}
	editRejects, err := c.app.Hooks.List(ctx, apphook.ListQuery{Hook: hook.LocalStatus, Outcome: hook.Reject, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	var editReject hook.Record
	for _, r := range editRejects {
		if hook.IsEditSubject(r.Decision.SubjectKey) {
			editReject = r
		}
	}
	if editReject.Decision.SubjectKey == "" || editReject.Decision.SubjectKey[:len(flagged.Id)+1] != flagged.Id+"#" {
		t.Fatalf("the decision of the edit = %+v", editReject)
	}
	if res := c.do("bob", http.MethodPost, "/hook_decisions/"+editReject.Decision.ID+"/appeal",
		map[string]any{"status_text": "edited into spam"}, nil); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("an appeal of a refused edit = %d", res.StatusCode)
	}
	plugin.verdict.Store("VERDICT_FLAG")
	if res := edit("edited, maybe spam"); res.StatusCode != http.StatusOK {
		t.Fatalf("the flagged edit = %d", res.StatusCode)
	}
	plugin.verdict.Store("VERDICT_REJECT")
	if reports, err := c.app.Moderation.Reports(ctx, appmoderation.ReportQuery{}, 50); err != nil || len(reports) != 2 {
		t.Errorf("reports after a flagged edit = %d, %v", len(reports), err)
	}

	// Established accounts are not sent by default.
	calls = plugin.calls.Load()
	// The access token would expire a week later, so these posts go to the use case itself.
	clock.ahead.Store(int64(8 * 24 * time.Hour))
	create := func(body string) error {
		_, err := c.app.Content.Create(ctx, appcontent.CreateInput{
			TextInput: appcontent.TextInput{Text: body}, AuthorAccountID: c.ids["carol"], Visibility: content.VisibilityPublic,
		})
		return err
	}
	if err := create("an old account"); err != nil {
		t.Errorf("an old account's post: %v", err)
	}
	if got := plugin.calls.Load(); got != calls {
		t.Error("an account older than a week was sent")
	}
	set(instance.SettingHookLocalStatusAccountAgeDays, "0")
	if err := create("every account"); !errors.Is(err, appcontent.ErrRejectedByReview) {
		t.Errorf("with every account sent: %v", err)
	}
	clock.ahead.Store(0)

	// Deleting the account removes its appeals and its exemption.
	if err := c.app.Identity.DeleteOwnAccount(ctx, c.userIDs["alice"], identity.Confirmation{Password: "correct horse battery staple"}); err != nil {
		t.Fatalf("DeleteOwnAccount: %v", err)
	}
	handler, _, ok := c.app.Handlers.Lookup(appaccount.JobEraseDeletedAccounts)
	if !ok {
		t.Fatalf("no handler of %s", appaccount.JobEraseDeletedAccounts)
	}
	clock.ahead.Store(int64(appaccount.ErasureDelay + time.Minute))
	if res := handler(ctx, queue.Job{Type: appaccount.JobEraseDeletedAccounts}); res.Err() != nil {
		t.Fatalf("erasure: %v", res.Err())
	}
	if _, err := c.app.Runner.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	clock.ahead.Store(0)
	appeals, err := c.app.Hooks.ListAppeals(ctx, apphook.AppealQuery{Limit: 10})
	if err != nil || len(appeals) != 0 {
		t.Errorf("the appeals after the erasure = %+v, %v", appeals, err)
	}
	calls = plugin.calls.Load()
	if _, err := c.app.Hooks.Decide(ctx, apphook.Call{Hook: hook.LocalStatus, SubjectKey: "after-erasure", AccountID: c.ids["alice"]}); err != nil {
		t.Fatal(err)
	}
	if got := plugin.calls.Load(); got != calls+1 {
		t.Error("the erased account is still exempt")
	}
}
