// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeAutoDeletes holds the policies by account, validating them as the content module does.
// Its preview returns preview for any valid policy and records the policy it got.
type fakeAutoDeletes struct {
	byAccount map[string]content.AutoDeletePolicy
	preview   appcontent.AutoDeletePreview
	previewed []content.AutoDeletePolicy
}

func (f *fakeAutoDeletes) PreviewAutoDeletePolicy(_ context.Context, accountID string, p content.AutoDeletePolicy) (appcontent.AutoDeletePreview, error) {
	p.AccountID = accountID
	p, err := p.Normalize()
	if err != nil {
		return appcontent.AutoDeletePreview{}, err
	}
	f.previewed = append(f.previewed, p)
	return f.preview, nil
}

func (f *fakeAutoDeletes) AutoDeletePolicy(_ context.Context, accountID string) (content.AutoDeletePolicy, error) {
	if p, ok := f.byAccount[accountID]; ok {
		return p, nil
	}
	return content.DefaultAutoDeletePolicy(accountID), nil
}

func (f *fakeAutoDeletes) UpdateAutoDeletePolicy(_ context.Context, accountID string, p content.AutoDeletePolicy) (content.AutoDeletePolicy, error) {
	p.AccountID = accountID
	p, err := p.Normalize()
	if err != nil {
		return content.AutoDeletePolicy{}, err
	}
	f.byAccount[accountID] = p
	return p, nil
}

func newAutoDeleteEnv(t *testing.T) (*authEnv, *fakeAutoDeletes) {
	t.Helper()
	f := &fakeAutoDeletes{byAccount: map[string]content.AutoDeletePolicy{}}
	return newAuthEnv(t, func(o *clientapi.Options) { o.AutoDelete = f }), f
}

func TestGetAutoDeletePolicy(t *testing.T) {
	e, _ := newAutoDeleteEnv(t)
	res := e.get(t, "/preferences/auto_delete", map[string]string{"Authorization": "Bearer read-accounts"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var got clientapi.AutoDeletePolicy
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	want := clientapi.AutoDeletePolicy{
		MinAgeDays: content.DefaultAutoDeleteAgeDays, KeepPinned: true, KeepBookmarked: true, KeepFavourited: true,
		KeepVisibilities: []clientapi.Visibility{clientapi.VisibilityDirect},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("policy = %+v, want the default %+v", got, want)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	if res := e.get(t, "/preferences/auto_delete", map[string]string{"Authorization": "Bearer write"}); res.StatusCode != http.StatusForbidden {
		t.Errorf("write token = %d, want 403", res.StatusCode)
	}
}

func TestUpdateAutoDeletePolicy(t *testing.T) {
	e, f := newAutoDeleteEnv(t)
	res := e.sendJSON(t, http.MethodPut, "/preferences/auto_delete", "write", `{
		"enabled": true, "min_age_days": 14, "keep_pinned": true, "keep_bookmarked": false, "keep_favourited": false,
		"keep_media": true, "keep_polls": false, "keep_visibilities": ["direct", "private"],
		"keep_min_boosts": 5, "keep_min_favourites": 0
	}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var got clientapi.AutoDeletePolicy
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	want := clientapi.AutoDeletePolicy{
		Enabled: true, MinAgeDays: 14, KeepPinned: true, KeepMedia: true,
		KeepVisibilities: []clientapi.Visibility{clientapi.VisibilityPrivate, clientapi.VisibilityDirect}, KeepMinBoosts: 5,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("policy = %+v, want %+v", got, want)
	}
	if p := f.byAccount[aliceAccountID]; !p.Enabled || p.MinAgeDays != 14 {
		t.Errorf("stored policy = %+v", p)
	}
	if res := e.sendJSON(t, http.MethodPut, "/preferences/auto_delete", "read", `{"invalid-on-purpose": true}`); res.StatusCode != http.StatusForbidden {
		t.Errorf("read token = %d, want 403", res.StatusCode)
	}

	res = e.sendJSON(t, http.MethodPut, "/preferences/auto_delete", "write", `{
		"enabled": true, "min_age_days": 7, "keep_pinned": true, "keep_bookmarked": false, "keep_favourited": false,
		"keep_media": true, "keep_polls": false, "keep_visibilities": ["invalid-on-purpose"], "keep_min_boosts": 0, "keep_min_favourites": 0
	}`)
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unknown visibility: status = %d, want 422", res.StatusCode)
	}
	if p := decodeProblem(t, res); p.Detail == nil || *p.Detail != "keep_visibilities の値が正しくありません。" {
		t.Errorf("problem = %+v (detail %v), want keep_visibilities named", p, p.Detail)
	}
}

func TestPreviewAutoDeletePolicy(t *testing.T) {
	const statusID = "01890a5d-ac96-774b-bcce-b302099a9001"
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	st := content.Status{
		ID: statusID, URI: "https://social.example/ap/statuses/" + statusID,
		AuthorAccountID: aliceAccountID, IsLocal: true, Visibility: content.VisibilityPublic, PublishedAt: at,
	}
	f := &fakeAutoDeletes{
		byAccount: map[string]content.AutoDeletePolicy{},
		preview:   appcontent.AutoDeletePreview{Count: 1000, More: true, Examples: []content.Status{st}},
	}
	views := &fakeViews{statuses: map[string]appcontent.StatusView{statusID: {
		Status:   st,
		Revision: content.Revision{Seq: 1, Language: "en", RevisedAt: at, Bodies: []content.Body{{Language: "en", ContentHTML: "<p>old</p>"}}},
		Author:   aliceActor,
		Counters: content.Counters{StatusID: statusID},
		Relation: &appcontent.ViewerRelation{Author: true},
	}}}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.AutoDelete, o.TimelineReader = f, views })
	body := `{
		"enabled": false, "min_age_days": 14, "keep_pinned": true, "keep_bookmarked": false, "keep_favourited": true,
		"keep_media": true, "keep_polls": true, "keep_visibilities": ["direct", "private"],
		"keep_min_boosts": 5, "keep_min_favourites": 3
	}`
	res := e.sendJSON(t, http.MethodPost, "/preferences/auto_delete/preview", "read-accounts", body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	got := decode[clientapi.AutoDeletePreview](t, res)
	if got.Count != 1000 || !got.More || len(got.Examples) != 1 || got.Examples[0].Id != statusID {
		t.Errorf("preview = %+v, want 1000 and more with the example", got)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	want := content.AutoDeletePolicy{
		AccountID: aliceAccountID, MinAgeDays: 14, KeepPinned: true, KeepFavourited: true, KeepMedia: true, KeepPolls: true,
		KeepVisibilities: []content.Visibility{content.VisibilityPrivate, content.VisibilityDirect}, KeepMinBoosts: 5, KeepMinFavourites: 3,
	}
	if len(f.previewed) != 1 || !reflect.DeepEqual(f.previewed[0], want) {
		t.Errorf("previewed = %+v, want every condition %+v", f.previewed, want)
	}
	if len(f.byAccount) != 0 {
		t.Errorf("the preview saved a policy: %+v", f.byAccount)
	}

	res = e.sendJSON(t, http.MethodPost, "/preferences/auto_delete/preview", "read-accounts", `{
		"enabled": true, "min_age_days": 7, "keep_pinned": true, "keep_bookmarked": false, "keep_favourited": false,
		"keep_media": true, "keep_polls": false, "keep_visibilities": ["invalid-on-purpose"], "keep_min_boosts": 0, "keep_min_favourites": 0
	}`)
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unknown visibility: status = %d, want 422", res.StatusCode)
	}
	if res := e.sendJSON(t, http.MethodPost, "/preferences/auto_delete/preview", "write", body); res.StatusCode != http.StatusForbidden {
		t.Errorf("write token = %d, want 403", res.StatusCode)
	}
}
