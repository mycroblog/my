// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appdiagnostics "github.com/mycroblog/my/internal/app/diagnostics"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// recordingUX records the measurements that reach it.
type recordingUX struct {
	mu sync.Mutex
	ms []appdiagnostics.UXMeasurement
}

func (r *recordingUX) Record(_ context.Context, ms []appdiagnostics.UXMeasurement) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ms = append(r.ms, ms...)
}

func (r *recordingUX) LastHours(context.Context, int) (time.Time, []appdiagnostics.UXSummary, error) {
	return time.Time{}, nil, nil
}

func (r *recordingUX) take() []appdiagnostics.UXMeasurement {
	r.mu.Lock()
	defer r.mu.Unlock()
	ms := r.ms
	r.ms = nil
	return ms
}

type clientMeasurementEnv struct {
	*authEnv
	ux *recordingUX
}

func newClientMeasurementEnv(t *testing.T) clientMeasurementEnv {
	t.Helper()
	ux := &recordingUX{}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.UXMeasurements = ux })
	return clientMeasurementEnv{authEnv: e, ux: ux}
}

// post sends body to POST /client_measurements from remote with contentType and checks that the
// answer is a 204 that the spec allows.
func (e clientMeasurementEnv) post(t *testing.T, remote, contentType string, body io.Reader) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, clientapi.BasePath+"/client_measurements", body)
	req.RemoteAddr = remote
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if res := e.spec.Serve(t, e.h, req); res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.StatusCode)
	}
}

func measurements(n int) []byte {
	ms := make([]map[string]any, n)
	for i := range ms {
		ms[i] = map[string]any{"metric": "press", "name": "post.reply", "value": 42.5}
	}
	b, _ := json.Marshal(map[string]any{"measurements": ms})
	return b
}

func TestReportClientMeasurements(t *testing.T) {
	e := newClientMeasurementEnv(t)
	e.post(t, "192.0.2.1:1234", "application/json; charset=utf-8", bytes.NewReader(measurements(2)))
	ms := e.ux.take()
	if len(ms) != 2 || ms[0].Metric != "press" || ms[0].Name != "post.reply" || ms[0].Value != 42.5 {
		t.Errorf("recorded %+v", ms)
	}
}

func TestReportClientMeasurementsIgnored(t *testing.T) {
	big := []byte(`{"measurements":[],"pad":"` + strings.Repeat("x", 17<<10) + `"}`)
	for _, tc := range []struct {
		name, contentType string
		body              io.Reader
	}{
		{"text/plain", "text/plain", bytes.NewReader(measurements(1))},
		{"no content type", "", bytes.NewReader(measurements(1))},
		{"over 16 KiB", "application/json", bytes.NewReader(big)},
		{"over 16 KiB without a length", "application/json", io.MultiReader(bytes.NewReader(big))},
		{"not JSON", "application/json", strings.NewReader("{")},
		{"over 50 samples", "application/json", bytes.NewReader(measurements(51))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newClientMeasurementEnv(t)
			e.post(t, "192.0.2.1:1234", tc.contentType, tc.body)
			if ms := e.ux.take(); len(ms) != 0 {
				t.Errorf("recorded %+v, want nothing", ms)
			}
		})
	}
}

func TestReportClientMeasurementsRateLimit(t *testing.T) {
	e := newClientMeasurementEnv(t)
	for range clientapi.ClientMeasurementLimit.Max + 1 {
		e.post(t, "192.0.2.1:1234", "application/json", bytes.NewReader(measurements(1)))
	}
	if ms := e.ux.take(); int64(len(ms)) != clientapi.ClientMeasurementLimit.Max {
		t.Fatalf("recorded %d, want %d", len(ms), clientapi.ClientMeasurementLimit.Max)
	}
	e.post(t, "192.0.2.2:1234", "application/json", bytes.NewReader(measurements(1)))
	if ms := e.ux.take(); len(ms) != 1 {
		t.Fatalf("recorded %d from another IP, want 1", len(ms))
	}
}
