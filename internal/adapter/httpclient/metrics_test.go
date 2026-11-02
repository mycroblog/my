// SPDX-License-Identifier: AGPL-3.0-only

package httpclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/mycroblog/my/internal/adapter/telemetry/sysmetrics"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestRobotsPurposeLabel checks the my.http.purpose of the fetches of robots.txt.
func TestRobotsPurposeLabel(t *testing.T) {
	if got := purposeLabel(kernel.HTTPPurposeRobots); got != "robots" {
		t.Errorf("purposeLabel(HTTPPurposeRobots) = %q", got)
	}
}

func TestMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }))
	t.Cleanup(srv.Close)
	m := testkit.NewMeter()
	exempt := netip.MustParseAddrPort(srv.Listener.Addr().String())
	c := New(Options{AllowHTTP: true, AnyPort: true, ExemptAddrPorts: []netip.AddrPort{exempt}, Meter: m})

	resp, err := c.Do(t.Context(), &kernel.HTTPRequest{URL: srv.URL, Purpose: kernel.HTTPPurposeImage})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	// The SSRF guard refuses the loopback address on another port.
	refused := "http://127.0.0.1:1/"
	if _, err := c.Do(t.Context(), &kernel.HTTPRequest{URL: refused, Method: http.MethodPost, Purpose: kernel.HTTPPurposeWebPush}); !errors.Is(err, kernel.ErrForbiddenDestination) {
		t.Fatalf("Do(%s) = %v, want ErrForbiddenDestination", refused, err)
	}

	name := sysmetrics.HTTPClientRequestDuration.Name
	if got := m.Measurements(name, map[string]string{
		"my.http.purpose": "image", "http.request.method": "GET", "my.http.status_class": "4xx",
	}); len(got) != 1 || got[0].Labels["error.type"] != "" {
		t.Errorf("the response: %v", got)
	}
	if got := m.Measurements(name, map[string]string{
		"my.http.purpose": "web_push", "http.request.method": "POST", "error.type": "forbidden_destination",
	}); len(got) != 1 {
		t.Errorf("the refused request: %v", m.Measurements(name, nil))
	}
	for _, p := range m.Measurements(name, nil) {
		for _, v := range p.Labels {
			if v == srv.URL || v == exempt.String() || v == refused {
				t.Errorf("a label holds the destination: %v", p.Labels)
			}
		}
	}
	if got := m.Sum(sysmetrics.HTTPClientOpenConnections.Name, nil); got != 1 {
		t.Errorf("open connections = %v, want 1 (kept alive)", got)
	}
	if err := m.Err(); err != nil {
		t.Error(err)
	}
}

func TestStatusClass(t *testing.T) {
	for code, want := range map[int]string{200: "2xx", 301: "3xx", 404: "4xx", 503: "5xx", 99: "_OTHER", 600: "_OTHER"} {
		if got := statusClass(code); got != want {
			t.Errorf("statusClass(%d) = %q, want %q", code, got, want)
		}
	}
}
