// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	appaccount "github.com/mycroblog/my/internal/app/account"
	apphook "github.com/mycroblog/my/internal/app/hook"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/hook"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// TestLocalProfileHook runs the local.profile hook on the assembled server, from PATCH
// /accounts/update_credentials to a real plugin over a Unix socket.
func TestLocalProfileHook(t *testing.T) {
	t.Parallel()
	plugin := new(followPlugin)
	plugin.verdict.Store("VERDICT_REJECT")
	mux := http.NewServeMux()
	mux.Handle("POST /my.plugin.hook.v1.HookService/Check", plugin)
	clock := laterClock{steadyClock: newSteadyClock(), ahead: &atomic.Int64{}}
	c := newStatusesClientWith(t, map[string]string{
		"MY_QUEUE": "inline", "MY_PLUGIN_LOCAL_PROFILE_ADDRESS": fakeplugin.ServeUnix(t, mux),
	}, platform.AppOptions{Clock: clock}, "alice", "bob")
	ctx := t.Context()

	set := func(key instance.SettingKey, value string) {
		t.Helper()
		if _, err := c.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: key, Value: value}); err != nil {
			t.Fatal(err)
		}
	}
	decisions := func(o hook.Outcome) []hook.Record {
		t.Helper()
		records, err := c.app.Hooks.List(ctx, apphook.ListQuery{Hook: hook.LocalProfile, Outcome: o, Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		return records
	}
	update := func(user string, body map[string]any) *http.Response {
		t.Helper()
		return c.do(user, http.MethodPatch, "/accounts/update_credentials", body, nil)
	}
	displayName := func(user string) string {
		t.Helper()
		var me clientapi.CredentialAccount
		if res := c.do(user, http.MethodGet, "/accounts/verify_credentials", nil, &me); res.StatusCode != http.StatusOK {
			t.Fatalf("verify_credentials = %d", res.StatusCode)
		}
		return me.DisplayName
	}
	spam := map[string]any{
		"display_name": "Cheap pills", "note": "visit https://spam.example",
		"fields": []map[string]string{{"name": "shop", "value": "https://spam.example"}},
	}

	// The information page states the account age the profiles are sent for, the default 7 days.
	var info clientapi.Instance
	if res := c.do("alice", http.MethodGet, "/instance", nil, &info); res.StatusCode != http.StatusOK || info.ExternalReviewScope == nil ||
		info.ExternalReviewScope.LocalProfileAccountAgeDays == nil || *info.ExternalReviewScope.LocalProfileAccountAgeDays != 7 {
		t.Errorf("GET /instance = %d, external_review_scope %+v", res.StatusCode, info.ExternalReviewScope)
	}

	// observe is the default: the verdict is recorded and the update is saved.
	if res := update("alice", spam); res.StatusCode != http.StatusOK {
		t.Fatalf("the update in observe = %d", res.StatusCode)
	}
	if got := displayName("alice"); got != "Cheap pills" {
		t.Errorf("the display name in observe = %q", got)
	}
	observed := decisions(hook.Reject)
	if len(observed) != 1 || observed[0].Decision.Mode != hook.ModeObserve || observed[0].Decision.AccountID != c.ids["alice"] ||
		!strings.HasPrefix(observed[0].Decision.SubjectKey, c.ids["alice"]+"#") {
		t.Fatalf("the decisions in observe = %+v", observed)
	}
	body := plugin.last.Load().(map[string]any)
	sent, _ := body["localProfile"].(map[string]any)
	fields, _ := sent["fields"].([]any)
	if body["hook"] != "local.profile" || body["subjectKey"] != observed[0].Decision.SubjectKey || sent["displayName"] != "Cheap pills" ||
		sent["note"] != "visit https://spam.example" || sent["username"] != "alice" || sent["accountCreatedAt"] == nil ||
		len(fields) != 1 || sent["avatarChanged"] != nil || sent["headerChanged"] != nil {
		t.Errorf("the request = %v", body)
	}

	// An update that changes nothing the hook sees is not sent.
	calls := plugin.calls.Load()
	if res := update("alice", map[string]any{"locked": true}); res.StatusCode != http.StatusOK {
		t.Fatalf("locking = %d", res.StatusCode)
	}
	if plugin.calls.Load() != calls {
		t.Error("an update of the lock alone was sent")
	}

	set(instance.SettingHookLocalProfileMode, instance.HookModeEnforce)

	// A reject shows fixed text, with no decision to appeal, and leaves the profile as it was.
	res := update("alice", map[string]any{"display_name": "More pills"})
	var p clientapi.Problem
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeProfileRejected ||
		p.Title != "This change of your profile was not saved." || p.Detail == nil {
		t.Errorf("the rejected update = %d %+v", res.StatusCode, p)
	}
	if got := displayName("alice"); got != "Cheap pills" {
		t.Errorf("the display name after a reject = %q", got)
	}
	if got := decisions(hook.Reject); len(got) != 2 || got[0].Decision.Mode != hook.ModeEnforce {
		t.Errorf("the decisions after a reject = %+v", got)
	}

	// A flag in enforce saves the update and files one report on the account, by the instance actor.
	plugin.verdict.Store("VERDICT_FLAG")
	if res := update("bob", map[string]any{"display_name": "Maybe spam"}); res.StatusCode != http.StatusOK {
		t.Fatalf("the flagged update = %d", res.StatusCode)
	}
	if got := displayName("bob"); got != "Maybe spam" {
		t.Errorf("the flagged display name = %q", got)
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
	flags := decisions(hook.Flag)
	report := reports[0].Report
	if len(flags) != 1 || report.ReporterAccountID != instanceActor.ID || report.TargetAccountID != c.ids["bob"] ||
		report.Category != moderation.CategorySpam || len(report.StatusIDs) != 0 {
		t.Fatalf("the report = %+v, flags %+v", report, flags)
	}
	if o := report.HookOrigin; o == nil || o.DecisionID != flags[0].Decision.ID || o.Hook != "local.profile" || o.ReasonCode != "spam" {
		t.Errorf("the origin = %+v", report.HookOrigin)
	}

	// A failure passes.
	plugin.verdict.Store("")
	if res := update("bob", map[string]any{"display_name": "While the plugin is down"}); res.StatusCode != http.StatusOK {
		t.Errorf("the update while the plugin fails = %d", res.StatusCode)
	}

	// Accounts older than hook_local_profile_account_age_days are not sent; 0 sends every account.
	// The access token would expire a week later, so these updates go to the use case itself.
	plugin.verdict.Store("VERDICT_REJECT")
	clock.ahead.Store(int64(8 * 24 * time.Hour))
	calls = plugin.calls.Load()
	rename := func(name string) error {
		_, err := c.app.Accounts.UpdateProfile(ctx, c.ids["alice"], appaccount.ProfileUpdate{DisplayName: &name})
		return err
	}
	if err := rename("Old enough"); err != nil {
		t.Errorf("the update of an old account: %v", err)
	}
	if plugin.calls.Load() != calls {
		t.Error("the profile of an old account was sent")
	}
	set(instance.SettingHookLocalProfileAccountAgeDays, "0")
	if err := rename("Every account"); !errors.Is(err, appaccount.ErrProfileRejected) {
		t.Errorf("the update at age 0: %v", err)
	}
	if got := plugin.calls.Load(); got != calls+1 {
		t.Errorf("the plugin was called %d times at age 0", got-calls)
	}
}
