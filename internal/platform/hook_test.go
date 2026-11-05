// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	apphook "github.com/mycroblog/my/internal/app/hook"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/hook"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// verdictPlugin is a plugin of net/http and JSON that answers the verdict a test sets and counts its
// calls.
func verdictPlugin(verdict *atomic.Value, calls *atomic.Int64) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /my.plugin.hook.v1.HookService/Check", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"verdict": verdict.Load().(string), "reason": "REASON_SPAM", "note": "from the plugin"})
	})
	return mux
}

// TestDecisionHookThroughTheServer runs the base of the decision hooks on the assembled server: a
// real plugin over a Unix socket, the settings, the admin API, and the audit log. The test takes
// the place of the module that owns the subject.
func TestDecisionHookThroughTheServer(t *testing.T) {
	t.Parallel()
	var verdict atomic.Value
	var calls atomic.Int64
	verdict.Store("VERDICT_DROP")
	addr := fakeplugin.ServeUnix(t, verdictPlugin(&verdict, &calls))
	c := newStatusesClientWith(t, map[string]string{"MY_PLUGIN_INBOUND_STATUS_ADDRESS": addr}, platform.AppOptions{}, "carol")
	ctx := t.Context()

	var info clientapi.Instance
	if res := c.do("carol", http.MethodGet, "/instance", nil, &info); res.StatusCode != http.StatusOK ||
		info.ExternalReviews == nil || len(*info.ExternalReviews) != 1 || (*info.ExternalReviews)[0] != clientapi.HookNameInboundStatus {
		t.Fatalf("GET /instance = %d, external_reviews %v", res.StatusCode, info.ExternalReviews)
	}

	// observe is the default: the verdict is recorded and has no effect.
	observed, err := c.app.Hooks.Decide(ctx, apphook.Call{Hook: hook.InboundStatus, SubjectKey: "https://remote.example/observed"})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Effect != hook.Allow || observed.DecisionID == "" {
		t.Errorf("observe = %+v", observed)
	}
	if res := c.do("carol", http.MethodPost, "/admin/hook_decisions/"+observed.DecisionID+"/override", nil, nil); res.StatusCode != http.StatusConflict {
		t.Errorf("override of an observed decision = %d, want 409", res.StatusCode)
	}

	// enforce acts on it, and running the job again reads the record.
	setMode := func(value string) error {
		_, err := c.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingHookInboundStatusMode, Value: value})
		return err
	}
	if err := setMode("enforce"); err != nil {
		t.Fatal(err)
	}
	subject := apphook.Call{Hook: hook.InboundStatus, SubjectKey: "https://remote.example/enforced"}
	enforced, err := c.app.Hooks.Decide(ctx, subject)
	if err != nil {
		t.Fatal(err)
	}
	if enforced.Effect != hook.Drop {
		t.Errorf("enforce = %+v", enforced)
	}
	before := calls.Load()
	if again, err := c.app.Hooks.Decide(ctx, subject); err != nil || again.DecisionID != enforced.DecisionID || calls.Load() != before {
		t.Errorf("again = %+v, %v, after %d more calls", again, err, calls.Load()-before)
	}

	// The list leaves out what was allowed, and shows the rest with its count and latency.
	verdict.Store("VERDICT_ALLOW")
	if _, err := c.app.Hooks.Decide(ctx, apphook.Call{Hook: hook.InboundStatus, SubjectKey: "https://remote.example/allowed"}); err != nil {
		t.Fatal(err)
	}
	var page clientapi.AdminHookDecisionPage
	if res := c.do("carol", http.MethodGet, "/admin/hook_decisions", nil, &page); res.StatusCode != http.StatusOK || len(page.Items) != 2 {
		t.Fatalf("GET /admin/hook_decisions = %d, %d items", res.StatusCode, len(page.Items))
	}
	if got := page.Items[0]; got.Id != enforced.DecisionID || !got.Overridable || got.Reason == nil || *got.Reason != clientapi.HookReasonSpam ||
		got.Note == nil || *got.Note != "from the plugin" {
		t.Errorf("the newest decision = %+v", got)
	}
	var summary clientapi.AdminHookSummary
	if res := c.do("carol", http.MethodGet, "/admin/hook_decisions/summary", nil, &summary); res.StatusCode != http.StatusOK || len(summary.Hooks) != len(hook.Names()) {
		t.Fatalf("GET summary = %d, %+v", res.StatusCode, summary)
	}
	if h := summary.Hooks[0]; !h.Configured || h.Mode != clientapi.HookModeEnforce || len(h.Outcomes) != 3 {
		t.Errorf("the summary of inbound.status = %+v", h)
	}
	if h := summary.Hooks[1]; h.Configured || len(h.Outcomes) != 0 {
		t.Errorf("the summary of inbound.follow = %+v", h)
	}

	// Overriding is its own event, in the audit log with the moderator as the actor.
	var overridden clientapi.AdminHookDecision
	if res := c.do("carol", http.MethodPost, "/admin/hook_decisions/"+enforced.DecisionID+"/override", nil, &overridden); res.StatusCode != http.StatusOK ||
		overridden.OverriddenAt == nil || overridden.Overridable {
		t.Fatalf("override = %d, %+v", res.StatusCode, overridden)
	}
	if res := c.do("carol", http.MethodPost, "/admin/hook_decisions/"+enforced.DecisionID+"/override", nil, nil); res.StatusCode != http.StatusConflict {
		t.Errorf("a second override = %d, want 409", res.StatusCode)
	}
	if res := c.do("carol", http.MethodPost, "/admin/hook_decisions/"+c.ids["carol"]+"/override", nil, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("override of an unknown decision = %d, want 404", res.StatusCode)
	}
	if again, err := c.app.Hooks.Decide(ctx, subject); err != nil || again.Effect != hook.Allow {
		t.Errorf("after the override = %+v, %v", again, err)
	}
	audit, err := c.app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetHookDecision}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 1 || audit[0].Action != moderation.AuditHookDecisionOverridden || audit[0].ActorKind != moderation.AuditActorUser || audit[0].TargetID != enforced.DecisionID {
		t.Errorf("audit = %+v", audit)
	}

	// An automated effect that leaves state is audited with the system as the actor; a drop is not.
	verdict.Store("VERDICT_FLAG")
	flagged, err := c.app.Hooks.Decide(ctx, apphook.Call{Hook: hook.InboundStatus, SubjectKey: "https://remote.example/flagged"})
	if err != nil {
		t.Fatal(err)
	}
	if flagged.Effect != hook.Flag {
		t.Errorf("flag = %+v", flagged)
	}
	verdict.Store("VERDICT_DROP")
	dropped, err := c.app.Hooks.Decide(ctx, apphook.Call{Hook: hook.InboundStatus, SubjectKey: "https://remote.example/dropped"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []apphook.Result{flagged, dropped} {
		if err := c.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return c.app.Hooks.Applied(ctx, tx, r) }); err != nil {
			t.Fatal(err)
		}
	}
	audit, err = c.app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetHookDecision}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 2 || audit[0].Action != moderation.AuditHookDecisionApplied || audit[0].ActorKind != moderation.AuditActorSystem || audit[0].TargetID != flagged.DecisionID {
		t.Errorf("audit = %+v", audit)
	}

	// A mode change is audited, and off goes to enforce only through observe.
	if err := setMode("off"); err != nil {
		t.Fatal(err)
	}
	if err := setMode("enforce"); !errors.Is(err, instance.ErrInvalidValue) {
		t.Errorf("off to enforce = %v, want ErrInvalidValue", err)
	}
	settings, err := c.app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetSetting}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings) != 2 || settings[0].Action != moderation.AuditSettingChanged || settings[0].TargetID != string(instance.SettingHookInboundStatusMode) {
		t.Errorf("the audit of the settings = %+v", settings)
	}
	before = calls.Load()
	off, err := c.app.Hooks.Decide(ctx, apphook.Call{Hook: hook.InboundStatus, SubjectKey: "https://remote.example/off"})
	if err != nil || off.Effect != hook.Allow || calls.Load() != before {
		t.Errorf("off = %+v, %v, after %d more calls", off, err, calls.Load()-before)
	}
	var withOff clientapi.Instance
	if res := c.do("carol", http.MethodGet, "/instance", nil, &withOff); res.StatusCode != http.StatusOK || withOff.ExternalReviews != nil {
		t.Errorf("GET /instance with the hook off: %v", withOff.ExternalReviews)
	}
}

// TestNoDecisionHookConfigured checks what a server with no plugin does: nothing.
func TestNoDecisionHookConfigured(t *testing.T) {
	t.Parallel()
	c := newStatusesClientWith(t, nil, platform.AppOptions{}, "carol")
	var info clientapi.Instance
	if res := c.do("carol", http.MethodGet, "/instance", nil, &info); res.StatusCode != http.StatusOK || info.ExternalReviews != nil {
		t.Errorf("GET /instance = %d, external_reviews %v", res.StatusCode, info.ExternalReviews)
	}
	r, err := c.app.Hooks.Decide(t.Context(), apphook.Call{Hook: hook.InboundStatus, SubjectKey: "k"})
	if err != nil || r.Effect != hook.Allow || r.DecisionID != "" {
		t.Errorf("Decide = %+v, %v", r, err)
	}
	var page clientapi.AdminHookDecisionPage
	if res := c.do("carol", http.MethodGet, "/admin/hook_decisions", nil, &page); res.StatusCode != http.StatusOK || len(page.Items) != 0 {
		t.Errorf("GET /admin/hook_decisions = %d, %+v", res.StatusCode, page)
	}
}
