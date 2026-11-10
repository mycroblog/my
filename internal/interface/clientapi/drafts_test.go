// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeDrafts holds the drafts of one account by ID, at most max of them (and max is 1 here).
type fakeDrafts struct {
	byID  map[string]appcontent.DraftView
	max   int
	clock time.Time
}

func (f *fakeDrafts) tick() time.Time {
	f.clock = f.clock.Add(time.Second)
	return f.clock
}

func (f *fakeDrafts) Create(_ context.Context, accountID string, in appcontent.DraftInput) (appcontent.DraftView, error) {
	if len(f.byID) >= f.max {
		return appcontent.DraftView{}, &appcontent.DraftLimitError{Limit: appcontent.LimitDrafts, Max: f.max}
	}
	id := "01890a5d-ac96-774b-bcce-b302099a8001"
	at := f.tick()
	v := appcontent.DraftView{
		Draft:   content.Draft{ID: id, AccountID: accountID, CreatedAt: at},
		Content: content.DraftContent{DraftID: id, Text: in.Text, Visibility: in.Visibility, UpdatedAt: at},
	}
	f.byID[id] = v
	return v, nil
}

func (f *fakeDrafts) Get(_ context.Context, accountID, id string) (appcontent.DraftView, error) {
	v, ok := f.byID[id]
	if !ok || v.Draft.AccountID != accountID {
		return appcontent.DraftView{}, appcontent.ErrNotFound
	}
	return v, nil
}

func (f *fakeDrafts) List(context.Context, string, string, *bool, int) ([]appcontent.DraftView, error) {
	return nil, nil
}

func (f *fakeDrafts) Update(ctx context.Context, accountID, id string, ifMatch time.Time, in appcontent.DraftInput) (appcontent.DraftView, error) {
	v, err := f.Get(ctx, accountID, id)
	if err != nil {
		return v, err
	}
	if !v.Content.UpdatedAt.Equal(ifMatch) {
		return appcontent.DraftView{}, appcontent.ErrDraftChanged
	}
	v.Content.Text, v.Content.UpdatedAt = in.Text, f.tick()
	f.byID[id] = v
	return v, nil
}

func (f *fakeDrafts) Delete(context.Context, string, string) error { return nil }

func (f *fakeDrafts) Schedule(context.Context, string, string, time.Time) (appcontent.DraftView, error) {
	return appcontent.DraftView{}, &appcontent.DraftLimitError{Limit: appcontent.LimitScheduleMinLead, Max: 5}
}

func (f *fakeDrafts) Unschedule(context.Context, string, string) (appcontent.DraftView, error) {
	return appcontent.DraftView{}, appcontent.ErrDraftNotScheduled
}

func (f *fakeDrafts) Publish(context.Context, string, string, string) (appcontent.Post, error) {
	return appcontent.Post{}, appcontent.ErrEmptyText
}

func (e *authEnv) draftRequest(t *testing.T, method, path, body string, header map[string]string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, clientapi.BasePath+path, strings.NewReader(body))
	req.Header.Set("Accept-Language", "en")
	req.Header.Set("Authorization", "Bearer write")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	return e.spec.Serve(t, e.h, req)
}

// TestDrafts checks ADR-0156 §9 at the API: a draft comes with its ETag, a save needs If-Match
// (428 without it, 412 with an older one), and the limits and the checks are problems localized
// in the user's locale that name the limit.
func TestDrafts(t *testing.T) {
	f := &fakeDrafts{byID: map[string]appcontent.DraftView{}, max: 1, clock: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Drafts = f })

	res := e.draftRequest(t, http.MethodPost, "/drafts", `{"text": "hello", "visibility": "public"}`, nil)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d, want 201", res.StatusCode)
	}
	created := decode[clientapi.Draft](t, res)
	etag := res.Header.Get("ETag")
	if etag == "" || created.Text != "hello" {
		t.Fatalf("create = %+v with ETag %q, want the draft and its ETag", created, etag)
	}

	res = e.draftRequest(t, http.MethodPost, "/drafts", `{"text": "again", "visibility": "public"}`, nil)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || *p.Detail != "予約していない下書きは 1 件まで保存できます。先にどれかを削除するか公開してください。" {
		t.Errorf("create past max_drafts = %d %+v, want a 422 that names the limit", res.StatusCode, p)
	}

	body := `{"text": "edited", "visibility": "public"}`
	if res := e.draftRequest(t, http.MethodPut, "/drafts/"+created.Id, body, nil); res.StatusCode != http.StatusPreconditionRequired {
		t.Errorf("update without If-Match = %d, want 428", res.StatusCode)
	}
	res = e.draftRequest(t, http.MethodPut, "/drafts/"+created.Id, body, map[string]string{"If-Match": etag})
	if res.StatusCode != http.StatusOK || res.Header.Get("ETag") == etag {
		t.Fatalf("update with If-Match = %d (ETag %q), want 200 with a new ETag", res.StatusCode, res.Header.Get("ETag"))
	}
	for name, tag := range map[string]string{"an older ETag": etag, "a foreign ETag": `"abc"`} {
		if res := e.draftRequest(t, http.MethodPut, "/drafts/"+created.Id, body, map[string]string{"If-Match": tag}); res.StatusCode != http.StatusPreconditionFailed {
			t.Errorf("update with %s = %d, want 412", name, res.StatusCode)
		}
	}

	res = e.draftRequest(t, http.MethodPut, "/drafts/"+created.Id+"/schedule", `{"publish_at": "2026-11-01T00:01:00Z"}`, nil)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || *p.Detail != "予約する時刻は、今から 5 分以上先にしてください。" {
		t.Errorf("schedule too soon = %d %+v, want a 422 that names the 5 minutes", res.StatusCode, p)
	}
	if res := e.draftRequest(t, http.MethodDelete, "/drafts/"+created.Id+"/schedule", "", nil); res.StatusCode != http.StatusConflict {
		t.Errorf("unschedule a draft without a schedule = %d, want 409", res.StatusCode)
	}
	if res := e.draftRequest(t, http.MethodPost, "/drafts/"+created.Id+"/publish", "", nil); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("publish an empty draft = %d, want 422", res.StatusCode)
	}
	if res := e.draftRequest(t, http.MethodGet, "/drafts/01890a5d-ac96-774b-bcce-b302099a8057", "", map[string]string{"Authorization": "Bearer read"}); res.StatusCode != http.StatusNotFound {
		t.Errorf("get an unknown draft = %d, want 404", res.StatusCode)
	}
	var page clientapi.DraftPage
	res = e.draftRequest(t, http.MethodGet, "/drafts?scheduled=true", "", map[string]string{"Authorization": "Bearer read"})
	if err := json.NewDecoder(res.Body).Decode(&page); err != nil || res.StatusCode != http.StatusOK {
		t.Errorf("list = %d, %v; want 200 with a page", res.StatusCode, err)
	}
}
