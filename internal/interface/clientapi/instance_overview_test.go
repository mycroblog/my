// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// fakeOverview answers the counts of the instance, and counts the reads.
type fakeOverview struct {
	counts appinstance.Counts
	reads  int
}

func (f *fakeOverview) Counts(context.Context) (appinstance.Counts, error) {
	f.reads++
	return f.counts, nil
}

func newOverviewEnv(t *testing.T, settings ...instance.CurrentSetting) (*authEnv, *fakeOverview) {
	t.Helper()
	counted := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	overview := &fakeOverview{
		counts: appinstance.Counts{
			Members: 12, DailyPosts: []int{0, 1, 2, 3, 4, 5, 6},
			StartedAt: time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC), CountedAt: counted,
		},
	}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Overview = overview
		o.Instance = &fakeInstance{settings: settings}
	})
	return e, overview
}

func TestGetInstanceOverview(t *testing.T) {
	e, _ := newOverviewEnv(t)
	res := e.get(t, "/instance/overview", nil)
	got := decodeJSON[clientapi.InstanceOverview](t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET = %d", res.StatusCode)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("Cache-Control = %q, want public, max-age=60", cc)
	}
	want := clientapi.InstanceCounts{
		Members: 12, DailyPosts: []int{0, 1, 2, 3, 4, 5, 6},
		StartedAt: time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC), CountedAt: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC),
	}
	if got.Counts == nil || !reflect.DeepEqual(*got.Counts, want) {
		t.Errorf("counts = %+v, want %+v", got.Counts, want)
	}

	// A signed-in viewer gets the same answer.
	res = e.get(t, "/instance/overview", map[string]string{"Authorization": "Bearer read"})
	if signedIn := decodeJSON[clientapi.InstanceOverview](t, res); !reflect.DeepEqual(signedIn, got) {
		t.Errorf("signed in = %+v, want %+v", signedIn, got)
	}
}

func TestGetInstanceOverviewHidesTheCountsWhenTheSettingDoes(t *testing.T) {
	off := instance.CurrentSetting{Key: instance.SettingPublicCounts, RevisionID: testkit.NamedID(string(instance.SettingPublicCounts)), Value: "false"}
	e, overview := newOverviewEnv(t, off)
	res := e.get(t, "/instance/overview", nil)
	got := decodeJSON[clientapi.InstanceOverview](t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET = %d", res.StatusCode)
	}
	if got.Counts != nil {
		t.Errorf("counts = %+v, want none", got.Counts)
	}
	if overview.reads != 0 {
		t.Errorf("read the overview %d times, want none (hidden counts are not read)", overview.reads)
	}
}
