// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"archive/zip"
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// TestDataExportListsFollowedHashtags checks that the archive of a data export (FR-008) holds the
// hashtags the user follows as followed_hashtags.csv, one per line, as GET /csv_exports does.
func TestDataExportListsFollowedHashtags(t *testing.T) {
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
	if !slices.Contains(names, "followed_hashtags.csv") {
		t.Errorf("the archive holds %v, without followed_hashtags.csv", names)
	}
}
