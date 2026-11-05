// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

type fakeFullTextSearch []search.Visibility

func (f fakeFullTextSearch) Visibilities(context.Context) ([]search.Visibility, error) { return f, nil }

func (f fakeFullTextSearch) Search(context.Context, appcontent.Viewer, search.Query) (search.Page, error) {
	return search.Page{}, nil
}

// TestInstanceFullTextSearch checks that GET /instance tells clients whether the full-text search
// is available and on which visibilities (ADR-0149 §9), with an empty list, not null, without it.
func TestInstanceFullTextSearch(t *testing.T) {
	for _, tc := range []struct {
		name      string
		search    clientapi.FullTextSearch
		available bool
		want      []clientapi.InstanceFullTextSearchVisibilities
	}{
		{"none", nil, false, []clientapi.InstanceFullTextSearchVisibilities{}},
		{"public", fakeFullTextSearch{search.VisibilityPublic}, true, []clientapi.InstanceFullTextSearchVisibilities{clientapi.InstanceFullTextSearchVisibilitiesPublic}},
		{"public and local", fakeFullTextSearch{search.VisibilityPublic, search.VisibilityLocal}, true, []clientapi.InstanceFullTextSearchVisibilities{
			clientapi.InstanceFullTextSearchVisibilitiesPublic, clientapi.InstanceFullTextSearchVisibilitiesLocal,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newAuthEnv(t, func(o *clientapi.Options) {
				o.Instance = &fakeInstance{}
				o.FullTextSearch = tc.search
			})
			res := e.send(t, http.MethodGet, "/instance", "", "")
			if res.StatusCode != http.StatusOK {
				t.Fatalf("GET /instance = %d", res.StatusCode)
			}
			got := decode[clientapi.Instance](t, res).FullTextSearch
			if got.Available != tc.available || got.Visibilities == nil || !slices.Equal(got.Visibilities, tc.want) {
				t.Errorf("full_text_search = %+v, want %v on %v", got, tc.available, tc.want)
			}
		})
	}
}
