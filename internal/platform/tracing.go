// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"

	"github.com/mycroblog/my/internal/adapter/telemetry/none"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpTracer builds the Tracer that cfg.TracesExporter selects (ADR-0055 §17) and makes the
// logger write the trace and span IDs of every line logged with a traced context (§6).
func (a *App) setUpTracer(ctx context.Context, cfg *config.Config, opts AppOptions) error {
	switch {
	case opts.Tracer != nil:
		a.Tracer = opts.Tracer
	case cfg.TracesExporter == "" || cfg.TracesExporter == "none":
		a.Tracer = none.NewTracer()
	case cfg.TracesExporter == "otlp":
		if err := a.setUpOTelTracer(ctx, cfg, opts); err != nil {
			return err
		}
	default:
		return fmt.Errorf("traces_exporter %q is not implemented yet", cfg.TracesExporter)
	}
	a.Logger = slog.New(traceIDHandler{Handler: a.Logger.Handler(), tracer: a.Tracer, gcp: cfg.LogsExporter == config.LogsGCP})
	return nil
}

// traceIDHandler adds the trace and span IDs of the context (kernel.Tracer.LogIDs) to every
// record, as trace_id and span_id, so that the logs and the traces of a request or a job
// correlate (ADR-0055 §18). With gcp (logs_exporter = gcp) it also writes them in the fields that
// Cloud Logging links to Cloud Trace: the bare trace ID, the span ID, and the sampled flag.
type traceIDHandler struct {
	slog.Handler
	tracer kernel.Tracer
	gcp    bool
}

func (h traceIDHandler) Handle(ctx context.Context, r slog.Record) error {
	if p := h.tracer.LogIDs(ctx); p.TraceID != "" {
		r.AddAttrs(slog.String("trace_id", p.TraceID), slog.String("span_id", p.SpanID))
		if h.gcp {
			r.AddAttrs(slog.String(gcpTraceKey, p.TraceID), slog.String(gcpSpanIDKey, p.SpanID), slog.Bool(gcpTraceSampledKey, p.Sampled))
		}
	}
	return h.Handler.Handle(ctx, r)
}

func (h traceIDHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceIDHandler{Handler: h.Handler.WithAttrs(attrs), tracer: h.tracer, gcp: h.gcp}
}

func (h traceIDHandler) WithGroup(name string) slog.Handler {
	return traceIDHandler{Handler: h.Handler.WithGroup(name), tracer: h.tracer, gcp: h.gcp}
}

// headerTraceParent carries the W3C trace context (Trace Context §3.2).
const headerTraceParent = "Traceparent"

// HTTPServerSpan is the span of an HTTP request (ADR-0055 §13), named "<method> <http.route>"
// once the router has found the route, and "<method>" for a request that reached no route. It
// carries the attributes of http.server.request.duration: never the path, the query, the
// client IP, or the User-Agent (§4).
var HTTPServerSpan = kernel.SpanSpec{
	Name: "HTTP", Kind: kernel.SpanServer, AttrKeys: []string{labelMethod, labelRoute, labelStatus, labelSurface},
}

// traceParentOf returns the traceparent of r when the connection's peer is a trusted proxy
// (trusted_proxies, ADR-0019 §5), and "" otherwise: the trace of any other client is restarted,
// so that it can neither join our traces nor ask to record them (ADR-0055 §15, Trace Context
// §7.2). A tracestate is never taken.
func traceParentOf(r *http.Request, trusted []netip.Prefix) string {
	values := r.Header.Values(headerTraceParent)
	if len(values) != 1 || len(trusted) == 0 {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || !newClientIPResolver(trusted, "").trusts(normalizeAddr(peer)) {
		return ""
	}
	return values[0]
}

// startHTTPSpan starts the span of r, as the child of a trusted proxy's traceparent. It returns
// r with the span's context and the function that ends the span with the request's route and
// status, or nil when there is no span: the health checks (isHealthCheck), which the
// platform's probes and the uptime monitors call constantly, the requests that the origin check
// refused, which anyone can send, and every request when the Tracer records nothing, which still keeps the trusted
// traceparent for the log lines (ADR-0055 §18).
func (a *App) startHTTPSpan(r *http.Request) (*http.Request, func(rr *requestRoute, status int)) {
	if tp := traceParentOf(r, a.Config.TrustedProxies); tp != "" {
		r = r.WithContext(a.Tracer.Continue(r.Context(), tp))
	}
	if kernel.IsNopTracer(a.Tracer) || isHealthCheck(r.URL.Path) || originRefused(r.Context()) {
		return r, nil
	}
	ctx := r.Context()
	method := methodLabel(r.Method)
	ctx, span := a.Tracer.Start(ctx, HTTPServerSpan.Named(method), kernel.NewAttrs(kernel.StringAttr(labelMethod, method)))
	return r.WithContext(ctx), func(rr *requestRoute, status int) {
		if span.Recording() {
			if rr.route != "" {
				span.SetName(method + " " + rr.route)
				span.Set(kernel.StringAttr(labelRoute, rr.route))
				span.Set(kernel.StringAttr(labelSurface, rr.surface))
			}
			span.Set(kernel.IntAttr(labelStatus, int64(status)))
		}
		errorType := ""
		if status >= http.StatusInternalServerError {
			errorType = fmt.Sprint(status)
		}
		span.End(errorType)
	}
}
