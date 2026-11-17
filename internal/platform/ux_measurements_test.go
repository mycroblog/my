// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestUXMeasurements drives the UX measurements on SQLite (ADR-0167 §3): the samples that a
// client sends, signed in or not, appear summed in GET /admin/ux_measurements and in the metric
// my.ux.duration; the invalid ones are dropped; and while the setting ux_measurement is off, as it
// is by default, the instance tells the client so and the server takes nothing.
func TestUXMeasurements(t *testing.T) {
	t.Parallel()
	meter := testkit.NewMeter()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), Meter: meter})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}
	admin := e.user("alice")

	report := map[string]any{"measurements": []map[string]any{
		{"metric": "inp", "name": "/_app/home", "value": 120},
		{"metric": "inp", "name": "/_app/home", "value": 90},
		{"metric": "press", "name": "post.reply", "value": 30},
		{"metric": "press", "name": "https://example.com/", "value": 30},
		{"metric": "lcp", "name": "/_app/home", "value": -1},
	}}

	// Off by default: the instance says so, and nothing is taken.
	var info clientapi.Instance
	e.want(e.do(http.MethodGet, "/instance", "", nil, &info), http.StatusOK, "GET /instance")
	if info.UxMeasurement == nil || *info.UxMeasurement {
		t.Errorf("ux_measurement = %v, want false by default", info.UxMeasurement)
	}
	e.want(e.do(http.MethodPost, "/client_measurements", "", report, nil), http.StatusNoContent, "POST /client_measurements by default")
	e.want(e.do(http.MethodPatch, "/admin/settings", admin.token, map[string]any{"values": map[string]string{"ux_measurement": "true"}}, nil),
		http.StatusOK, "PATCH /admin/settings on")

	e.want(e.do(http.MethodPost, "/client_measurements", "", report, nil), http.StatusNoContent, "POST /client_measurements")
	e.want(e.do(http.MethodPost, "/client_measurements", admin.token, map[string]any{"measurements": []map[string]any{
		{"metric": "inp", "name": "/_app/home", "value": 600},
	}}, nil), http.StatusNoContent, "POST /client_measurements signed in")
	if err := app.UXMeasurements.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}

	var list clientapi.UXMeasurementList
	e.want(e.do(http.MethodGet, "/admin/ux_measurements", admin.token, nil, &list), http.StatusOK, "GET /admin/ux_measurements")
	if len(list.Items) != 2 {
		t.Fatalf("items = %+v, want inp and press", list.Items)
	}
	inp := list.Items[0]
	if inp.Metric != clientapi.UXMetricInp || inp.Name != "/_app/home" || inp.Samples != 3 || inp.P75 == nil || *inp.P75 != 800 {
		t.Errorf("inp = %+v, want 3 samples with a p75 of at most 800 ms", inp)
	}
	if press := list.Items[1]; press.Metric != clientapi.UXMetricPress || press.Name != "post.reply" || press.Samples != 1 {
		t.Errorf("press = %+v", press)
	}
	if got := len(meter.Measurements("my.ux.duration", map[string]string{"my.ux.metric": "inp", "my.ux.name": "/_app/home"})); got != 3 {
		t.Errorf("my.ux.duration of inp = %d measurements, want 3", got)
	}
	if err := meter.Err(); err != nil {
		t.Error(err)
	}
	e.want(e.do(http.MethodGet, "/admin/ux_measurements?hours=0", admin.token, nil, nil), http.StatusBadRequest, "GET /admin/ux_measurements?hours=0")
	e.want(e.do(http.MethodGet, "/admin/ux_measurements", "", nil, nil), http.StatusUnauthorized, "GET /admin/ux_measurements anonymous")

	// Off: the instance says so, and nothing more is taken.
	e.want(e.do(http.MethodPatch, "/admin/settings", admin.token, map[string]any{"values": map[string]string{"ux_measurement": "false"}}, nil),
		http.StatusOK, "PATCH /admin/settings")
	e.want(e.do(http.MethodGet, "/instance", "", nil, &info), http.StatusOK, "GET /instance")
	if info.UxMeasurement == nil || *info.UxMeasurement {
		t.Errorf("ux_measurement = %v, want false", info.UxMeasurement)
	}
	e.want(e.do(http.MethodPost, "/client_measurements", "", report, nil), http.StatusNoContent, "POST /client_measurements while off")
	if err := app.UXMeasurements.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	e.want(e.do(http.MethodGet, "/admin/ux_measurements", admin.token, nil, &list), http.StatusOK, "GET /admin/ux_measurements")
	if len(list.Items) != 2 || list.Items[0].Samples != 3 {
		t.Errorf("items while off = %+v, want those from before", list.Items)
	}
}
