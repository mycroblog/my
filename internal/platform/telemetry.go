// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/adapter/telemetry/emf"
	"github.com/mycroblog/my/internal/adapter/telemetry/none"
	"github.com/mycroblog/my/internal/adapter/telemetry/sysmetrics"
	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/metrics"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	domainfederation "github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/platform/config"
)

// PathMetrics is the path of the Prometheus scrape on metrics_listen (ADR-0055 §2).
const PathMetrics = "/metrics"

// meterCloseTimeout bounds the last export of Close.
const meterCloseTimeout = 5 * time.Second

// setUpMeter builds the Meter that cfg.MetricsExporter selects (ADR-0055 §2).
func (a *App) setUpMeter(ctx context.Context, cfg *config.Config, opts AppOptions) error {
	switch {
	case opts.Meter != nil:
		a.Meter = opts.Meter
	case cfg.MetricsExporter == "" || cfg.MetricsExporter == "none":
		a.Meter = none.New()
	case cfg.MetricsExporter == "prometheus" || cfg.MetricsExporter == "otlp":
		if err := a.setUpOTelMeter(ctx, cfg, opts); err != nil {
			return err
		}
	case cfg.MetricsExporter == "emf":
		w := opts.MetricsWriter
		if w == nil {
			w = os.Stdout
		}
		a.Meter = emf.New(emf.Options{
			Writer: w, Namespace: cfg.MetricsEMFNamespace, Dimensions: cfg.MetricsEMFDimensions, Clock: a.Clock,
			Properties: map[string]string{"service.name": "my", "service.version": opts.Version, "my.role": string(cfg.Role)},
		})
	default:
		return fmt.Errorf("metrics_exporter %q is not implemented yet", cfg.MetricsExporter)
	}
	a.http = newHTTPMetrics(a.Meter)
	a.readinessFailures = a.Meter.Counter(ReadinessFailures)
	sysmetrics.RegisterProcess(a.Meter, processStart)
	return nil
}

// processStart is when the process started, as near as the platform knows it.
var processStart = time.Now()

// ReadinessFailures counts the readiness checks that failed (ADR-0056 §3): the check itself
// never looks at a metric.
var ReadinessFailures = kernel.Instrument{
	Name: "my.readiness.failures", Unit: "{check}", Description: "Readiness checks that failed.",
}

// MetricsHandler returns the handler of the Prometheus scrape, or nil when metrics_exporter is
// not prometheus.
func (a *App) MetricsHandler() http.Handler { return a.metricsHandler }

// The surfaces of the HTTP server, the my.surface label (ADR-0055 §3, ADR-0075).
const (
	surfaceClientAPI   = "client_api"
	surfaceActivityPub = "activitypub"
	surfaceOAuth       = "oauth"
	surfaceWeb         = "web"
	surfaceMedia       = "media"
	surfaceAdmin       = "admin"
	// surfacePayments is the endpoint that the payment provider calls (ADR-0074 §3).
	surfacePayments = "payments"
	// surfaceVoice is the endpoints that the media servers of the voice chat call (ADR-0102 §4).
	surfaceVoice = "voice"
)

// routeOther is the http.route of a request that no route pattern matched.
const routeOther = "other"

// Label keys of the HTTP server metrics.
const (
	labelMethod  = "http.request.method"
	labelRoute   = "http.route"
	labelStatus  = "http.response.status_code"
	labelSurface = "my.surface"
)

// The HTTP server metrics of ADR-0055 §3, named after the OpenTelemetry semantic conventions.
var (
	HTTPRequestDuration = kernel.Instrument{
		Name: "http.server.request.duration", Unit: "s", Description: "Duration of HTTP server requests.",
		LabelKeys: []string{labelMethod, labelRoute, labelStatus, labelSurface}, Buckets: kernel.DurationBuckets,
	}
	HTTPActiveRequests = kernel.Instrument{
		Name: "http.server.active_requests", Unit: "{request}", Description: "Number of HTTP server requests in progress.",
		LabelKeys: []string{labelSurface},
	}
)

// httpMetrics records the HTTP server metrics; nil when the Meter records nothing, so that
// the middleware costs nothing either.
type httpMetrics struct {
	duration kernel.Histogram
	active   kernel.UpDownCounter
}

func newHTTPMetrics(m kernel.Meter) *httpMetrics {
	if kernel.IsNop(m) {
		return nil
	}
	return &httpMetrics{duration: m.Histogram(HTTPRequestDuration), active: m.UpDownCounter(HTTPActiveRequests)}
}

// requestRoute is the route of a request, which problemFallback fills in once the mux has
// matched it and the access log reads at the end.
type requestRoute struct {
	route   string
	surface string
	// method is the http.request.method of the request, for the place of its error reports.
	method string
}

type requestRouteKey struct{}

// routeTarget is the http.route and my.surface of a registered pattern.
type routeTarget struct{ route, surface string }

// routeOf returns the http.route of a ServeMux pattern: its path template, without the method
// and the host.
func routeOf(pattern string) string {
	if _, path, ok := strings.Cut(pattern, " "); ok {
		pattern = path
	}
	if i := strings.IndexByte(pattern, '/'); i > 0 {
		pattern = pattern[i:]
	}
	return pattern
}

// methodLabel returns the http.request.method of a request: a method of RFC 9110 or PATCH, and
// _OTHER for any other, as the semantic conventions ask, so that clients cannot add labels.
func methodLabel(m string) string {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodDelete,
		http.MethodConnect, http.MethodOptions, http.MethodTrace, http.MethodPatch:
		return m
	}
	return "_OTHER"
}

// begin starts measuring a request whose route problemFallback has found: it counts the request
// as active. It returns the function that ends it.
func (m *httpMetrics) begin(r *http.Request, t routeTarget) func() {
	l := kernel.NewLabels(labelSurface, t.surface)
	m.active.Add(r.Context(), 1, l)
	return func() { m.active.Add(context.WithoutCancel(r.Context()), -1, l) }
}

// record records the duration of a request at its end.
func (m *httpMetrics) record(ctx context.Context, r *http.Request, rr *requestRoute, status int, d time.Duration) {
	m.duration.Record(ctx, d.Seconds(), kernel.NewLabels(
		labelMethod, methodLabel(r.Method), labelRoute, rr.route, labelStatus, strconv.Itoa(status), labelSurface, rr.surface,
	))
}

// surfaceRouter registers the routes of one surface on the App.
type surfaceRouter struct {
	a       *App
	surface string
}

func (s surfaceRouter) Handle(pattern string, h http.Handler) { s.a.handle(pattern, h, s.surface) }

// clientAPIRouter registers the client API, whose /admin operations are the admin surface.
type clientAPIRouter struct{ a *App }

func (c clientAPIRouter) Handle(pattern string, h http.Handler) {
	surface := surfaceClientAPI
	if strings.HasPrefix(routeOf(pattern), "/api/v1/admin/") {
		surface = surfaceAdmin
	}
	c.a.handle(pattern, h, surface)
}

// Business metrics of ADR-0055 §3.
var (
	AccountsRegistered = kernel.Instrument{
		Name: "my.accounts.registered", Unit: "{account}", Description: "Local accounts registered.",
	}
	StatusesCreated = kernel.Instrument{
		Name: "my.statuses.created", Unit: "{status}", Description: "Local statuses posted, by visibility.",
		LabelKeys: []string{"my.visibility"},
	}
	TimelineDeliveryLag = kernel.Instrument{
		Name: "my.timeline.delivery.lag", Unit: "s",
		Description: "Time from the creation of a local status to its home timeline entries and the signal to the streams.",
		Buckets:     kernel.DurationBuckets,
	}
)

// registerBusinessMetrics counts the registrations and posts, and the timeline delivery lag,
// from the events of committed transactions. It runs after timeline.RegisterCommitHooks, so
// that the lag includes the signal to the streams: the home timeline entries are written in
// the transaction that creates the status (data-model §4.6).
func (a *App) registerBusinessMetrics(r *uow.Projectors) {
	if kernel.IsNop(a.Meter) {
		return
	}
	registered := a.Meter.Counter(AccountsRegistered)
	created := a.Meter.Counter(StatusesCreated)
	lag := a.Meter.Histogram(TimelineDeliveryLag)
	r.OnCommit(account.EventLocalAccountCreated, func(ctx context.Context, e uow.Event) {
		if e.(account.LocalAccountCreated).Local.UserID != "" { // not the instance actor
			registered.Add(ctx, 1, kernel.Labels{})
		}
	})
	r.OnCommit(content.EventStatusCreated, func(ctx context.Context, e uow.Event) {
		st := e.(content.StatusCreated).Status
		if !st.IsLocal {
			return
		}
		created.Add(ctx, 1, kernel.NewLabels("my.visibility", string(st.Visibility)))
		lag.Record(ctx, max(a.Clock.Now().Sub(st.PublishedAt), 0).Seconds(), kernel.Labels{})
	})
}

// registerMetricsSnapshot reports the instance-wide gauges from the snapshot that the processes
// share in the key-value store, and registers its periodic job (ADR-0055 §4), unless the Meter
// records nothing.
func (a *App) registerMetricsSnapshot() {
	if kernel.IsNop(a.Meter) {
		return
	}
	backlog, _ := a.source.(queue.BacklogReader)
	a.MetricsSnapshot = metrics.NewService(metrics.Deps{
		KV: a.KV, Clock: a.Clock, Logger: a.Logger, Backlog: backlog, Totals: a.instanceTotals,
		KnownJobType: func(t string) bool { _, _, ok := a.Handlers.Lookup(t); return ok },
	})
	a.MetricsSnapshot.RegisterGauges(a.Meter)
	a.MetricsSnapshot.RegisterJobs(a.Handlers, a.Scheduler, a.Meter)
}

// instanceTotals computes the counts and sizes of the metrics snapshot: the local users and
// posts as NodeInfo reports them, the bytes of the media files and of the database, the
// remote servers that deliveries fail to reach, and the billing sources whose import fails.
func (a *App) instanceTotals(ctx context.Context) (metrics.Totals, error) {
	users, err := a.Accounts.CountLocalUsers(ctx)
	if err != nil {
		return metrics.Totals{}, err
	}
	statuses, err := a.Content.LocalPostCount(ctx)
	if err != nil {
		return metrics.Totals{}, err
	}
	t := metrics.Totals{Users: int64(users), Statuses: statuses}
	if a.Media != nil {
		if t.MediaLocalBytes, t.MediaRemoteCacheBytes, err = a.Media.StoredBytes(ctx); err != nil {
			return metrics.Totals{}, err
		}
	}
	if a.dbSize != nil {
		n, err := a.dbSize.DatabaseSize(ctx)
		if err != nil {
			return metrics.Totals{}, err
		}
		t.DatabaseBytes = &n
	}
	if a.InstanceHealth != nil {
		n, err := a.InstanceHealth.Counts(ctx)
		if err != nil {
			return metrics.Totals{}, err
		}
		t.InstancesFailing, t.InstancesUnavailable = n[domainfederation.InstanceAvailable], n[domainfederation.InstanceUnavailable]
	}
	if a.Finance != nil {
		if t.BillingImportFailing, err = a.Finance.FailingSources(ctx); err != nil {
			return metrics.Totals{}, err
		}
	}
	return t, nil
}
