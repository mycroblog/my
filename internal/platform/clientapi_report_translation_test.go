// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// accountLimiter records whose limit each reservation counts against.
type accountLimiter struct {
	mu       sync.Mutex
	accounts []string
}

func (l *accountLimiter) Reserve(_ context.Context, accountID string, _ int) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.accounts = append(l.accounts, accountID)
	return nil
}

func (l *accountLimiter) reserved() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.accounts)
}

// reportTranslationEnv is a report by alice on bob's public, private, and direct posts, with a
// comment, which the moderator carol translates. The provider "one" takes the default
// visibilities, and "two" takes direct and private posts only.
type reportTranslationEnv struct {
	*translationEnv
	limiter                 *accountLimiter
	two                     *recordingTranslator
	report                  string
	public, private, direct clientapi.Status
	gone                    clientapi.Status
}

func newReportTranslationEnv(t *testing.T) *reportTranslationEnv {
	t.Helper()
	path := newDatabase(t, true)
	base := &translationEnv{providers: &changingProviders{}, limiter: &countingLimiter{}, one: newRecordingTranslator()}
	e := &reportTranslationEnv{translationEnv: base, limiter: &accountLimiter{}, two: newRecordingTranslator()}
	base.c = newStatusesClientWith(t, map[string]string{"MY_DATABASE_URL": path},
		platform.AppOptions{TranslationProviders: base.providers, TranslationLimiter: e.limiter}, "alice", "bob", "carol")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	base.db = db
	e.providers.set(
		translation.Provider{ID: "one", Name: "Provider One", Translator: e.one},
		translation.Provider{ID: "two", Name: "Provider Two", Translator: e.two,
			Visibilities: []appcontent.Visibility{appcontent.VisibilityPrivate, appcontent.VisibilityDirect}},
	)
	c := e.c
	if res := c.do("alice", http.MethodPost, "/accounts/"+c.ids["bob"]+"/follow", nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("follow = %d", res.StatusCode)
	}
	e.public = c.post("bob", map[string]any{"text": "A public post", "language": "en", "visibility": "public"})
	e.private = c.post("bob", map[string]any{"text": "For followers", "language": "en", "visibility": "private"})
	e.direct = c.post("bob", map[string]any{"text": "@alice just you", "language": "en", "visibility": "direct"})
	e.gone = c.post("bob", map[string]any{"text": "Soon deleted", "language": "en", "visibility": "public"})
	var report clientapi.Report
	if res := c.do("alice", http.MethodPost, "/reports", map[string]any{
		"account_id": c.ids["bob"], "status_ids": []string{e.public.Id, e.private.Id, e.direct.Id, e.gone.Id},
		"category": "spam", "comment": "These are spam, see https://example.com/proof",
	}, &report); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /reports = %d", res.StatusCode)
	}
	e.report = report.Id
	return e
}

func (e *reportTranslationEnv) setReportsToServer(on bool) {
	e.c.t.Helper()
	value := "false"
	if on {
		value = "true"
	}
	if _, err := e.c.app.Instance.SetSetting(e.c.t.Context(), appinstance.SetSettingInput{Key: instance.SettingTranslationReportsToServer, Value: value}); err != nil {
		e.c.t.Fatal(err)
	}
}

// translateReported asks for a reported post in ja as carol, a moderator.
func (e *reportTranslationEnv) translateReported(statusID string) (*http.Response, clientapi.StatusTranslation) {
	e.c.t.Helper()
	var out clientapi.StatusTranslation
	res := e.c.do("carol", http.MethodPost, "/admin/reports/"+e.report+"/statuses/"+statusID+"/translation", map[string]any{"target_language": "ja"}, &out)
	return res, out
}

func (e *reportTranslationEnv) translateComment() (*http.Response, clientapi.ReportCommentTranslation) {
	e.c.t.Helper()
	var out clientapi.ReportCommentTranslation
	res := e.c.do("carol", http.MethodPost, "/admin/reports/"+e.report+"/comment/translation", map[string]any{"target_language": "ja"}, &out)
	return res, out
}

func wantTranslationProblem(t *testing.T, res *http.Response, status int, typ string) {
	t.Helper()
	if res.StatusCode != status {
		t.Fatalf("status = %d, want %d", res.StatusCode, status)
	}
	if typ == "" {
		return
	}
	var p clientapi.Problem
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if p.Type != typ {
		t.Errorf("problem type = %q, want %q", p.Type, typ)
	}
}

// TestReportTranslationSendsNothingWhileTheSettingIsOff: translation_reports_to_server starts off,
// and then neither a reported post nor the comment reaches a provider, while GET /instance says so.
func TestReportTranslationSendsNothingWhileTheSettingIsOff(t *testing.T) {
	t.Parallel()
	e := newReportTranslationEnv(t)
	var inst clientapi.Instance
	if res := e.c.do("carol", http.MethodGet, "/instance", nil, &inst); res.StatusCode != http.StatusOK || inst.Translation.ReportsToServer {
		t.Fatalf("GET /instance = %d, reports_to_server %v, want false by default", res.StatusCode, inst.Translation.ReportsToServer)
	}
	res, _ := e.translateReported(e.public.Id)
	wantTranslationProblem(t, res, http.StatusUnprocessableEntity, clientapi.ProblemTypeTranslationUnavailable)
	res, _ = e.translateComment()
	wantTranslationProblem(t, res, http.StatusUnprocessableEntity, clientapi.ProblemTypeTranslationUnavailable)
	if n := e.one.calls() + e.two.calls(); n != 0 {
		t.Errorf("providers called %d times with the setting off", n)
	}
	// The readers' own translations do not depend on it.
	if res, got := e.translate("alice", e.public.Id); res.StatusCode != http.StatusOK || got.Provider == nil {
		t.Errorf("a reader's translation = %d %+v", res.StatusCode, got.Provider)
	}
}

// TestReportTranslationKeepsEachProvidersVisibilities: with the setting on, a moderator's access
// lifts no visibility rule. A public post goes to "one", the direct post only to "two", which is
// allowed it, and the characters count against the server's limit only.
func TestReportTranslationKeepsEachProvidersVisibilities(t *testing.T) {
	t.Parallel()
	e := newReportTranslationEnv(t)
	e.setReportsToServer(true)
	res, got := e.translateReported(e.public.Id)
	if res.StatusCode != http.StatusOK || got.Provider == nil || got.Provider.Id != "one" || !strings.Contains(got.Content, "[ja] A public post") {
		t.Fatalf("translate the public post = %d %+v", res.StatusCode, got)
	}
	if res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", res.Header.Get("Cache-Control"))
	}
	res, got = e.translateReported(e.direct.Id)
	if res.StatusCode != http.StatusOK || got.Provider == nil || got.Provider.Id != "two" {
		t.Fatalf("translate the direct post = %d %+v, want provider two", res.StatusCode, got.Provider)
	}
	if e.one.calls() != 1 || e.two.calls() != 1 {
		t.Errorf("calls = one %d, two %d; the direct post must not reach one", e.one.calls(), e.two.calls())
	}
	// The moderator's translations share the cache with the readers who may see the post.
	if n := e.rows("status_translations"); n != 2 {
		t.Errorf("status_translations = %d, want 2", n)
	}
	if res, got := e.translate("alice", e.public.Id); res.StatusCode != http.StatusOK || got.Provider == nil || e.one.calls() != 1 {
		t.Errorf("a reader's translation = %d %+v after %d calls, want the kept one", res.StatusCode, got.Provider, e.one.calls())
	}

	// Without a provider allowed direct posts, the direct post reaches none.
	e.providers.set(translation.Provider{ID: "one", Name: "Provider One", Translator: e.one})
	if _, err := e.db.ExecContext(t.Context(), "DELETE FROM status_translations"); err != nil {
		t.Fatal(err)
	}
	res, _ = e.translateReported(e.direct.Id)
	wantTranslationProblem(t, res, http.StatusUnprocessableEntity, clientapi.ProblemTypeTranslationUnavailable)
	res, _ = e.translateReported(e.private.Id)
	wantTranslationProblem(t, res, http.StatusUnprocessableEntity, clientapi.ProblemTypeTranslationUnavailable)
	if e.one.calls() != 1 {
		t.Errorf("one was called %d times, want once: it is not allowed private or direct posts", e.one.calls())
	}
	// Every reservation was the server's only.
	for _, account := range e.limiter.reserved() {
		if account != "" {
			t.Errorf("a moderator's translation counted against account %q", account)
		}
	}
}

// TestReportCommentTranslation: the comment counts as a private post, reaches only a provider
// allowed those, without its link, and is never kept.
func TestReportCommentTranslation(t *testing.T) {
	t.Parallel()
	e := newReportTranslationEnv(t)
	e.setReportsToServer(true)
	res, got := e.translateComment()
	if res.StatusCode != http.StatusOK || got.Provider == nil || got.Provider.Id != "two" || got.Language != "ja" {
		t.Fatalf("translate the comment = %d %+v", res.StatusCode, got)
	}
	if !strings.HasPrefix(got.Comment, "[ja] These are spam") || !strings.Contains(got.Comment, "https://example.com/proof") {
		t.Errorf("comment = %q", got.Comment)
	}
	if sent := e.two.lastRequest().Fragments; len(sent) != 1 || strings.Contains(sent[0], "example.com") {
		t.Errorf("sent = %q, want the comment without its link", sent)
	}
	if e.one.calls() != 0 {
		t.Errorf("one received the comment, which is not allowed private posts")
	}
	if n := e.rows("status_translations"); n != 0 {
		t.Errorf("status_translations = %d after a comment, want none", n)
	}
	if res, _ := e.translateComment(); res.StatusCode != http.StatusOK || e.two.calls() != 2 {
		t.Errorf("again = %d after %d calls: a comment is never kept, so it is asked again", res.StatusCode, e.two.calls())
	}
	e.providers.set(translation.Provider{ID: "one", Name: "Provider One", Translator: e.one})
	res, _ = e.translateComment()
	wantTranslationProblem(t, res, http.StatusUnprocessableEntity, clientapi.ProblemTypeTranslationUnavailable)
	if slices.ContainsFunc(e.limiter.reserved(), func(a string) bool { return a != "" }) {
		t.Errorf("reservations = %q, want the server's only", e.limiter.reserved())
	}
}

// TestReportTranslationOfADeletedPost: a report keeps a post that its author deleted, and its
// moderators translate it, but the translation is not kept. A post that the report does not point
// at, and an unknown report, are 404s.
func TestReportTranslationOfADeletedPost(t *testing.T) {
	t.Parallel()
	e := newReportTranslationEnv(t)
	e.setReportsToServer(true)
	if res := e.c.do("bob", http.MethodDelete, "/statuses/"+e.gone.Id, nil, nil); res.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE = %d", res.StatusCode)
	}
	res, got := e.translateReported(e.gone.Id)
	if res.StatusCode != http.StatusOK || got.Provider == nil || !strings.Contains(got.Content, "[ja] Soon deleted") {
		t.Fatalf("translate the deleted post = %d %+v", res.StatusCode, got)
	}
	if n := e.rows("status_translations"); n != 0 {
		t.Errorf("status_translations = %d, want none for a deleted post", n)
	}

	other := e.c.post("bob", map[string]any{"text": "Not reported", "language": "en", "visibility": "public"})
	res, _ = e.translateReported(other.Id)
	wantTranslationProblem(t, res, http.StatusNotFound, "")
	var out clientapi.StatusTranslation
	res = e.c.do("carol", http.MethodPost, "/admin/reports/"+other.Id+"/statuses/"+e.public.Id+"/translation", map[string]any{"target_language": "ja"}, &out)
	wantTranslationProblem(t, res, http.StatusNotFound, "")
	var comment clientapi.ReportCommentTranslation
	res = e.c.do("carol", http.MethodPost, "/admin/reports/"+other.Id+"/comment/translation", map[string]any{"target_language": "ja"}, &comment)
	wantTranslationProblem(t, res, http.StatusNotFound, "")
}
