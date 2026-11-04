// SPDX-License-Identifier: AGPL-3.0-only

package httpclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestTracing checks the client spans of ADR-0055: the method, the destination's host alone,
// and the status, and no traceparent to the remote server, even when the caller sets one.
func TestTracing(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	tr := testkit.NewTracer()
	exempt := netip.MustParseAddrPort(srv.Listener.Addr().String())
	c := New(Options{AllowHTTP: true, AnyPort: true, ExemptAddrPorts: []netip.AddrPort{exempt}, Tracer: tr})

	ctx, parent := tr.Start(t.Context(), kernel.SpanSpec{Name: "my.job"}, kernel.Attrs{})
	h := http.Header{}
	h.Set("Traceparent", tr.Inject(ctx))
	h.Set("Tracestate", "vendor=value")
	resp, err := c.Do(ctx, &kernel.HTTPRequest{URL: srv.URL + "/users/alice/inbox?token=secret", Method: http.MethodPost, Header: h, Purpose: kernel.HTTPPurposeJSON})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	refused := "http://127.0.0.1:1/users/bob"
	if _, err := c.Do(ctx, &kernel.HTTPRequest{URL: refused, Purpose: kernel.HTTPPurposeJSON}); !errors.Is(err, kernel.ErrForbiddenDestination) {
		t.Fatalf("Do(%s) = %v, want ErrForbiddenDestination", refused, err)
	}
	parent.End("")

	if got.Get("Traceparent") != "" || got.Get("Tracestate") != "" {
		t.Errorf("the remote server got the trace context: %v", got)
	}
	spans := tr.Named("POST")
	if len(spans) != 1 {
		t.Fatalf("POST spans = %v", tr.Spans())
	}
	s := spans[0]
	if s.Kind != kernel.SpanClient || s.Attrs["server.address"] != "127.0.0.1" || s.Attrs["http.response.status_code"] != "503" ||
		s.ErrorType != "503" || s.Attrs["http.request.method"] != "POST" {
		t.Errorf("POST span = %+v", s)
	}
	refusedSpans := tr.Named("GET")
	if len(refusedSpans) != 1 || refusedSpans[0].ErrorType != "forbidden_destination" {
		t.Errorf("GET spans = %+v, want the refused request", refusedSpans)
	}
	for _, s := range tr.Spans() {
		for k, v := range s.Attrs {
			if strings.Contains(v, "/") || strings.Contains(v, "alice") || strings.Contains(v, "secret") {
				t.Errorf("span %s attribute %s = %q holds more than the host", s.Name, k, v)
			}
		}
	}
	if err := tr.Err(); err != nil {
		t.Error(err)
	}
}
