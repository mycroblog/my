// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	appcontent "github.com/mycroblog/my/internal/app/content"
	apphook "github.com/mycroblog/my/internal/app/hook"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/media"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/hook"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// mediaPlugin answers each media.check call with the verdict the test set, and records the calls.
type mediaPlugin struct {
	mu       sync.Mutex
	verdict  string
	fail     bool
	requests []map[string]any
}

func (p *mediaPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	p.mu.Lock()
	p.requests = append(p.requests, req)
	verdict, fail := p.verdict, p.fail
	p.mu.Unlock()
	if fail {
		http.Error(w, "broken", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"verdict": verdict, "reason": "REASON_SEXUAL", "note": "from the plugin"})
}

func (p *mediaPlugin) answer(verdict string, fail bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.verdict, p.fail = verdict, fail
}

func (p *mediaPlugin) calls() []map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]map[string]any(nil), p.requests...)
}

// mediaHook is a server with a plugin of media.check and a local account alice.
type mediaHook struct {
	t      *testing.T
	c      *statusesClient
	plugin *mediaPlugin
	alice  string
	count  func(q string) int64
}

func newMediaHook(t *testing.T, mode, onFailure string) *mediaHook {
	t.Helper()
	plugin := &mediaPlugin{verdict: "VERDICT_ALLOW"}
	mux := http.NewServeMux()
	mux.Handle("POST /my.plugin.hook.v1.HookService/Check", plugin)
	dbURL := newDatabase(t, true)
	c := newStatusesClientWith(t, map[string]string{
		"MY_DATABASE_URL": dbURL, "MY_QUEUE": "inline", "MY_PLUGIN_MEDIA_CHECK_ADDRESS": fakeplugin.ServeUnix(t, mux),
	}, platform.AppOptions{}, "alice")
	_, count := rawSQL(t, "sqlite", dbURL)
	h := &mediaHook{t: t, c: c, plugin: plugin, alice: c.ids["alice"], count: count}
	h.set(instance.SettingHookMediaCheckMode, "observe")
	h.set(instance.SettingHookMediaCheckMode, mode)
	h.set(instance.SettingHookMediaCheckOnFailure, onFailure)
	return h
}

func (h *mediaHook) set(key instance.SettingKey, value string) {
	h.t.Helper()
	if _, err := h.c.app.Instance.SetSetting(h.t.Context(), appinstance.SetSettingInput{Key: key, Value: value}); err != nil {
		h.t.Fatalf("SetSetting(%s, %s) = %v", key, value, err)
	}
}

// upload stores an image of alice for purpose and runs its processing, as the job does.
func (h *mediaHook) upload(original []byte, purpose domcontent.MediaPurpose) (appcontent.Media, bool) {
	h.t.Helper()
	ctx := h.t.Context()
	m, err := h.c.app.Media.Upload(ctx, appcontent.UploadInput{OwnerAccountID: h.alice, Content: bytes.NewReader(original), Purpose: purpose})
	if err != nil {
		h.t.Fatalf("Upload: %v", err)
	}
	done := h.c.app.Media.Process(ctx, m.File.ID, purpose).IsDone()
	if m, err = h.c.app.Media.Media(ctx, m.File.ID); err != nil {
		h.t.Fatal(err)
	}
	return m, done
}

func (h *mediaHook) post(mediaID string) appcontent.Post {
	h.t.Helper()
	p, err := h.c.app.Content.Create(h.t.Context(), appcontent.CreateInput{
		AuthorAccountID: h.alice, Visibility: appcontent.VisibilityPublic,
		TextInput: appcontent.TextInput{Text: "a picture", Media: []appcontent.MediaInput{{MediaID: mediaID}}},
	})
	if err != nil {
		h.t.Fatalf("Create: %v", err)
	}
	return p
}

func (h *mediaHook) decision(mediaID string) apphook.Result {
	h.t.Helper()
	r, err := h.c.app.Hooks.Recorded(h.t.Context(), hook.MediaCheck, mediaID)
	if err != nil {
		h.t.Fatal(err)
	}
	return r
}

func (h *mediaHook) reports() []moderation.ReportRecord {
	h.t.Helper()
	page, err := h.c.app.Moderation.Reports(h.t.Context(), appmoderation.ReportQuery{}, 50)
	if err != nil {
		h.t.Fatal(err)
	}
	return page
}

// The plugin gets the smallest processed image, never the original, and a reject keeps nothing of
// the upload: the file fails, has no variant, and its original is gone.
func TestMediaCheckRejectKeepsNothing(t *testing.T) {
	t.Parallel()
	h := newMediaHook(t, "enforce", "pass")
	h.plugin.answer("VERDICT_REJECT", false)
	original := testkit.FixtureJPEG(1280, 960, 1)
	m, done := h.upload(original, domcontent.PurposeAttachment)
	if !done || m.Ready() || m.Result == nil || m.Result.Outcome != domcontent.MediaFailed || len(m.Result.Variants) != 0 {
		t.Fatalf("after a reject: done %t, result %+v", done, m.Result)
	}
	if _, err := h.c.app.Blobs.Head(t.Context(), m.Upload.BlobKey); err == nil {
		t.Error("the original of a rejected upload is kept")
	}
	if n := h.count("SELECT COUNT(*) FROM media_variants WHERE result_id = '" + m.Result.ID + "'"); n != 0 {
		t.Errorf("%d variants of a rejected upload", n)
	}
	calls := h.plugin.calls()
	if len(calls) != 1 {
		t.Fatalf("the plugin got %d calls", len(calls))
	}
	req := calls[0]
	sub, _ := req["media"].(map[string]any)
	image, _ := base64.StdEncoding.DecodeString(sub["image"].(string))
	if req["hook"] != "media.check" || req["subjectKey"] != m.File.ID || sub["purpose"] != "attachment" || sub["mediaType"] != "image" ||
		sub["imageContentType"] != "image/webp" || len(image) == 0 || bytes.Equal(image, original) || media.Sniff(image) != media.FormatWebP {
		t.Errorf("request = %v", req)
	}
	if w, _ := sub["imageWidth"].(float64); w == 0 || w*sub["imageHeight"].(float64) > 640*360 {
		t.Errorf("the image sent is %v×%v, larger than the small variant", w, sub["imageHeight"])
	}
	if _, err := h.c.app.Content.Create(t.Context(), appcontent.CreateInput{
		AuthorAccountID: h.alice, Visibility: appcontent.VisibilityPublic,
		TextInput: appcontent.TextInput{Text: "x", Media: []appcontent.MediaInput{{MediaID: m.File.ID}}},
	}); err == nil {
		t.Error("a post attached a rejected upload")
	}
	// Processing again asks nothing more.
	if !h.c.app.Media.Process(t.Context(), m.File.ID, domcontent.PurposeAttachment).IsDone() || len(h.plugin.calls()) != 1 {
		t.Errorf("processing again called the plugin: %d calls", len(h.plugin.calls()))
	}
}

// mark_sensitive acts in the transaction that creates the post that attaches the file; on an
// avatar there is no post, so it is only recorded.
func TestMediaCheckMarkSensitive(t *testing.T) {
	t.Parallel()
	h := newMediaHook(t, "enforce", "pass")
	h.plugin.answer("VERDICT_MARK_SENSITIVE", false)
	m, _ := h.upload(testkit.FixturePNG(64, 48, false), domcontent.PurposeAttachment)
	if !m.Ready() {
		t.Fatalf("not ready: %+v", m.Result)
	}
	d := h.decision(m.File.ID)
	if d.Effect != hook.MarkSensitive || h.count("SELECT COUNT(*) FROM status_sensitivity_overrides") != 0 {
		t.Fatalf("decision %+v before any post", d)
	}
	p := h.post(m.File.ID)
	if n := h.count("SELECT COUNT(*) FROM status_sensitivity_overrides WHERE status_id = '" + p.Status.ID + "' AND source_kind = 'hook_decision' AND source_id = '" + d.DecisionID + "'"); n != 1 {
		t.Errorf("%d marks of the post", n)
	}
	if n := h.count("SELECT COUNT(*) FROM hook_decision_statuses WHERE decision_id = '" + d.DecisionID + "' AND status_id = '" + p.Status.ID + "'"); n != 1 {
		t.Errorf("%d links of the decision to the post", n)
	}
	// An edit that keeps the file adds nothing.
	if _, err := h.c.app.Content.Edit(t.Context(), appcontent.EditInput{
		AccountID: h.alice, StatusID: p.Status.ID,
		TextInput: appcontent.TextInput{Text: "edited", Media: []appcontent.MediaInput{{MediaID: m.File.ID}}},
	}); err != nil {
		t.Fatal(err)
	}
	if n := h.count("SELECT COUNT(*) FROM audit_log_entries WHERE action = 'hook_decision_applied'"); n != 1 {
		t.Errorf("%d audit log entries of the mark", n)
	}

	avatar, _ := h.upload(testkit.FixturePNG(64, 64, false), domcontent.PurposeAvatar)
	if !avatar.Ready() || h.decision(avatar.File.ID).Effect != hook.MarkSensitive {
		t.Errorf("avatar: ready %t, decision %+v", avatar.Ready(), h.decision(avatar.File.ID))
	}
	if n := h.count("SELECT COUNT(*) FROM status_sensitivity_overrides"); n != 1 {
		t.Errorf("%d marks after the avatar", n)
	}

	// Cancelling the decision takes the mark away.
	if _, err := h.c.app.Hooks.Override(t.Context(), d.DecisionID, ""); err != nil {
		t.Fatal(err)
	}
	if n := h.count("SELECT COUNT(*) FROM status_sensitivity_overrides"); n != 0 {
		t.Errorf("%d marks after the cancellation", n)
	}
}

// A post that both the hook and a moderator's mark_statuses_sensitive marked keeps the mark of the
// one source when the other goes: lifting the action leaves the hook's mark, and cancelling the
// decision leaves the action's. The projection rebuilds to the same rows after each step.
func TestMediaCheckMarkAndModerationMarkOnOnePost(t *testing.T) {
	t.Parallel()
	h := newMediaHook(t, "enforce", "pass")
	ctx := t.Context()
	h.plugin.answer("VERDICT_MARK_SENSITIVE", false)
	marks := func(statusID string) (byHook, byAction int64) {
		t.Helper()
		where := "SELECT COUNT(*) FROM status_sensitivity_overrides WHERE status_id = '" + statusID + "' AND "
		return h.count(where + "source_hook_decision_id IS NOT NULL"), h.count(where + "source_action_id IS NOT NULL")
	}
	verify := func() {
		t.Helper()
		for _, p := range h.c.app.Projections.List() {
			r, err := h.c.app.Projections.Verify(ctx, p)
			if err != nil {
				t.Fatalf("Verify(%s) = %v", p.Name, err)
			}
			if r.Drifted() {
				t.Errorf("Verify(%s) drifted: %+v", p.Name, r)
			}
		}
	}
	both := func() (statusID, decisionID, actionID string) {
		t.Helper()
		m, _ := h.upload(testkit.FixturePNG(64, 48, false), domcontent.PurposeAttachment)
		p := h.post(m.File.ID)
		act, err := h.c.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
			TargetAccountID: h.alice, Kind: moderation.ActionMarkStatusesSensitive, ReasonCategory: moderation.CategoryOther,
			StatusIDs: []string{p.Status.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		if byHook, byAction := marks(p.Status.ID); byHook != 1 || byAction != 1 {
			t.Fatalf("marks of a post both marked = hook %d, action %d", byHook, byAction)
		}
		return p.Status.ID, h.decision(m.File.ID).DecisionID, act.ID
	}

	liftedPost, _, action := both()
	if err := h.c.app.Moderation.LiftAccountAction(ctx, action, "", ""); err != nil {
		t.Fatal(err)
	}
	if byHook, byAction := marks(liftedPost); byHook != 1 || byAction != 0 {
		t.Errorf("after lifting the action: hook %d, action %d; want the hook's mark alone", byHook, byAction)
	}
	verify()

	cancelledPost, decision, _ := both()
	if _, err := h.c.app.Hooks.Override(ctx, decision, ""); err != nil {
		t.Fatal(err)
	}
	if byHook, byAction := marks(cancelledPost); byHook != 0 || byAction != 1 {
		t.Errorf("after cancelling the decision: hook %d, action %d; want the action's mark alone", byHook, byAction)
	}
	verify()
}

// flag files one report: when the file is processed for an avatar, a header, or an emoji, and in
// the transaction of the post for an attachment, which the report points at.
func TestMediaCheckFlag(t *testing.T) {
	t.Parallel()
	h := newMediaHook(t, "enforce", "pass")
	h.plugin.answer("VERDICT_FLAG", false)
	header, _ := h.upload(testkit.FixturePNG(150, 50, false), domcontent.PurposeHeader)
	reports := h.reports()
	if !header.Ready() || len(reports) != 1 {
		t.Fatalf("header: ready %t, %d reports", header.Ready(), len(reports))
	}
	r := reports[0].Report
	if r.TargetAccountID != h.alice || len(r.StatusIDs) != 0 || r.HookOrigin == nil || r.HookOrigin.Hook != "media.check" ||
		r.Category != moderation.CategoryViolation || r.Comment != "from the plugin" {
		t.Errorf("report of the header = %+v", r)
	}

	m, _ := h.upload(testkit.FixturePNG(64, 48, false), domcontent.PurposeAttachment)
	if got := len(h.reports()); got != 1 {
		t.Fatalf("processing an attachment filed a report: %d", got)
	}
	p := h.post(m.File.ID)
	reports = h.reports()
	if len(reports) != 2 {
		t.Fatalf("%d reports after the post", len(reports))
	}
	for _, rec := range reports {
		if rec.Report.HookOrigin.DecisionID == h.decision(m.File.ID).DecisionID &&
			(len(rec.Report.StatusIDs) != 1 || rec.Report.StatusIDs[0] != p.Status.ID) {
			t.Errorf("report of the attachment = %+v", rec.Report)
		}
	}
}

// On failure the media passes by default; retry fails the job and records nothing, so the next
// run asks again. observe acts on nothing.
func TestMediaCheckWhenThePluginFails(t *testing.T) {
	t.Parallel()
	pass := newMediaHook(t, "enforce", "pass")
	pass.plugin.answer("", true)
	if m, done := pass.upload(testkit.FixturePNG(32, 32, false), domcontent.PurposeAttachment); !done || !m.Ready() {
		t.Errorf("pass: done %t, ready %t", done, m.Ready())
	}

	retry := newMediaHook(t, "enforce", "retry")
	retry.plugin.answer("", true)
	m, done := retry.upload(testkit.FixturePNG(32, 32, false), domcontent.PurposeAttachment)
	if done || m.Result != nil || retry.decision(m.File.ID).DecisionID != "" {
		t.Fatalf("retry: done %t, result %+v", done, m.Result)
	}
	retry.plugin.answer("VERDICT_ALLOW", false)
	if !retry.c.app.Media.Process(t.Context(), m.File.ID, domcontent.PurposeAttachment).IsDone() {
		t.Fatal("the job did not finish once the plugin answered")
	}
	if m, _ = retry.c.app.Media.Media(t.Context(), m.File.ID); !m.Ready() || len(retry.plugin.calls()) < 2 {
		t.Errorf("retry: ready %t after %d calls", m.Ready(), len(retry.plugin.calls()))
	}

	observe := newMediaHook(t, "observe", "pass")
	observe.plugin.answer("VERDICT_REJECT", false)
	if m, _ := observe.upload(testkit.FixturePNG(32, 32, false), domcontent.PurposeAttachment); !m.Ready() {
		t.Error("observe acted on a reject")
	}
}

// A video or audio file sends its cover or poster, and nothing of the file itself; one with none
// sends no image. A reject keeps nothing of it either.
func TestMediaCheckAudio(t *testing.T) {
	t.Parallel()
	h := newMediaHook(t, "enforce", "pass")
	h.plugin.answer("VERDICT_ALLOW", false)
	if m, _ := h.upload(testkit.MP3File(40, testkit.FixturePNG(300, 300, false)), domcontent.PurposeAttachment); !m.Ready() {
		t.Fatalf("not ready: %+v", m.Result)
	}
	sub, _ := h.plugin.calls()[0]["media"].(map[string]any)
	if sub["mediaType"] != "audio" || sub["imageContentType"] != "image/webp" || sub["image"] == nil || sub["durationMs"] == nil || sub["animated"] != nil {
		t.Errorf("audio with a cover = %v", sub)
	}

	h.plugin.answer("VERDICT_REJECT", false)
	m, _ := h.upload(testkit.MP3File(40, nil), domcontent.PurposeAttachment)
	if m.Result == nil || m.Result.Outcome != domcontent.MediaFailed || len(m.Result.Variants) != 0 {
		t.Errorf("rejected audio: %+v", m.Result)
	}
	if _, err := h.c.app.Blobs.Head(t.Context(), m.Upload.BlobKey); err == nil {
		t.Error("the original of rejected audio is kept")
	}
	if sub, _ := h.plugin.calls()[1]["media"].(map[string]any); sub["mediaType"] != "audio" || sub["image"] != nil {
		t.Errorf("audio without a cover = %v", sub)
	}
}
