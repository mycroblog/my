// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"

	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	domtimeline "github.com/mycroblog/my/internal/domain/timeline"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// TestDataExportHoldsFollowedHashtagsAndKeywordFilters checks that the archive of a data export (FR-008) holds the
// hashtags the user follows as followed_hashtags.csv, one per line, as GET /csv_exports does, and
// the user's keyword filters as keyword_filters.json (ADR-0157 §8).
func TestDataExportHoldsFollowedHashtagsAndKeywordFilters(t *testing.T) {
	t.Parallel()
	// The inline queue builds the archive before POST /exports answers.
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}
	alice := e.user("alice")
	for _, tag := range []string{"Cats", "#dogs"} {
		if _, err := e.app.Timeline.FollowTag(t.Context(), alice.id, tag); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.app.Timeline.CreateKeywordFilter(t.Context(), alice.id, apptimeline.KeywordFilterInput{
		Title: "Spoilers", Action: domtimeline.FilterWarn, Contexts: []domtimeline.FilterContext{domtimeline.FilterContextHome},
		Words: []apptimeline.KeywordInput{{Word: "Spoiler", WholeWord: true}},
	}); err != nil {
		t.Fatal(err)
	}
	var export clientapi.DataExport
	e.want(e.do(http.MethodPost, "/exports", alice.token, nil, &export), http.StatusAccepted, "POST /exports")
	res := e.do(http.MethodGet, "/exports/"+export.Id+"/archive", alice.token, nil, nil)
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /exports/%s/archive = %d %s", export.Id, res.StatusCode, body)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
		if f.Name == "keyword_filters.json" {
			checkKeywordFiltersFile(t, f)
			continue
		}
		if f.Name != "followed_hashtags.csv" {
			continue
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		csv, _ := io.ReadAll(r)
		_ = r.Close()
		lines := strings.Fields(string(csv))
		slices.Sort(lines)
		if !slices.Equal(lines, []string{"Cats", "dogs"}) {
			t.Errorf("followed_hashtags.csv = %q, want Cats and dogs", csv)
		}
	}
	for _, name := range []string{"followed_hashtags.csv", "keyword_filters.json"} {
		if !slices.Contains(names, name) {
			t.Errorf("the archive holds %v, without %s", names, name)
		}
	}
}

// checkKeywordFiltersFile checks the keyword_filters.json of the archive: the one filter with its
// context and its whole word.
func checkKeywordFiltersFile(t *testing.T, f *zip.File) {
	t.Helper()
	r, err := f.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	var doc struct {
		Filters []struct {
			Title    string   `json:"title"`
			Action   string   `json:"action"`
			Contexts []string `json:"contexts"`
			Keywords []struct {
				Keyword   string `json:"keyword"`
				WholeWord bool   `json:"whole_word"`
			} `json:"keywords"`
		} `json:"filters"`
	}
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Filters) != 1 || doc.Filters[0].Title != "Spoilers" || doc.Filters[0].Action != "warn" ||
		!slices.Equal(doc.Filters[0].Contexts, []string{"home"}) || len(doc.Filters[0].Keywords) != 1 ||
		doc.Filters[0].Keywords[0].Keyword != "Spoiler" || !doc.Filters[0].Keywords[0].WholeWord {
		t.Errorf("keyword_filters.json = %+v, want the Spoilers filter", doc)
	}
}
