// SPDX-License-Identifier: AGPL-3.0-only

// Package otel holds the kernel.Meter and kernel.Tracer adapters on the OpenTelemetry Go SDK.
//
// The Meter (ADR-0055 §1, §2) aggregates the measurements in the process and exports them in
// the Prometheus text format, which Handler serves for a scrape (metrics_exporter = prometheus),
// or pushes them over OTLP/HTTP to a collector (metrics_exporter = otlp), configured by the
// standard OTEL_EXPORTER_OTLP_* and OTEL_METRIC_EXPORT_* environment variables.
//
// Besides the instruments declared through kernel.Meter, the Meter reports the Go runtime
// metrics of ADR-0056 §1 (go.memory.*, go.goroutine.count, go.processor.limit, go.config.gogc,
// go.schedule.duration, and the GC cycles and pauses) from the OpenTelemetry contrib runtime
// instrumentation and runtime/metrics.
//
// The Tracer (ADR-0063) exports the spans over OTLP/HTTP; see Tracer.
//
// The resource carries service.name, service.version, and my.role, and the attributes of
// OTEL_RESOURCE_ATTRIBUTES, through which a platform can add the instance's identifier
// (ADR-0055 §6).
package otel

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/otlptranslator"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Exporters.
const (
	ExporterPrometheus = "prometheus"
	ExporterOTLP       = "otlp"
)

// Options configure New.
type Options struct {
	// Exporter is ExporterPrometheus or ExporterOTLP.
	Exporter string
	// ServiceName is the service.name of the resource; the default is "my".
	ServiceName    string
	ServiceVersion string
	// Role is the my.role of the resource: web, worker, or all.
	Role string
}

// Meter is a kernel.Meter on an OpenTelemetry MeterProvider.
type Meter struct {
	provider *sdkmetric.MeterProvider
	meter    metric.Meter
	handler  http.Handler

	// err is the first instrument declaration that the SDK refused, a programming error (an
	// invalid name).
	errMu sync.Mutex
	err   error
}

var _ kernel.Meter = (*Meter)(nil)

// New returns the Meter that exports through opts.Exporter. Shut it down to flush and stop
// the exporter.
func New(ctx context.Context, opts Options) (*Meter, error) {
	res, err := NewResource(ctx, opts.ServiceName, opts.ServiceVersion, opts.Role)
	if err != nil {
		return nil, fmt.Errorf("metrics resource: %w", err)
	}
	m := &Meter{}
	var reader sdkmetric.Reader
	switch opts.Exporter {
	case ExporterPrometheus:
		reg := promclient.NewRegistry()
		popts := []promexporter.Option{
			promexporter.WithRegisterer(reg),
			promexporter.WithTranslationStrategy(otlptranslator.UnderscoreEscapingWithSuffixes),
			promexporter.WithoutScopeInfo(),
		}
		for _, p := range runtimeProducers() {
			popts = append(popts, promexporter.WithProducer(p))
		}
		exp, err := promexporter.New(popts...)
		if err != nil {
			return nil, fmt.Errorf("prometheus exporter: %w", err)
		}
		reader = exp
		m.handler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	case ExporterOTLP:
		exp, err := otlpmetrichttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("otlp exporter: %w", err)
		}
		var ropts []sdkmetric.PeriodicReaderOption
		for _, p := range runtimeProducers() {
			ropts = append(ropts, sdkmetric.WithProducer(p))
		}
		reader = sdkmetric.NewPeriodicReader(exp, ropts...)
	default:
		return nil, fmt.Errorf("metrics exporter %q is not an OpenTelemetry exporter", opts.Exporter)
	}
	m.provider = sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(reader))
	m.meter = m.provider.Meter("github.com/mycroblog/my")
	if err := startRuntime(m.provider); err != nil {
		_ = m.provider.Shutdown(ctx)
		return nil, fmt.Errorf("runtime metrics: %w", err)
	}
	return m, nil
}

// NewResource returns the resource of the metrics, the traces, and the exported error reports
// (ADR-0055 §6, ADR-0063 §1, ADR-0103 §4 (3)):
// service.name (default "my"), service.version, my.role, and the attributes of
// OTEL_RESOURCE_ATTRIBUTES.
func NewResource(ctx context.Context, name, version, role string) (*resource.Resource, error) {
	if name == "" {
		name = "my"
	}
	return resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			attribute.String("service.name", name),
			attribute.String("service.version", version),
			attribute.String("my.role", role),
		),
	)
}

// Handler serves the metrics in the Prometheus text format; nil for the OTLP exporter.
func (m *Meter) Handler() http.Handler { return m.handler }

// Shutdown flushes the pending measurements and stops the exporter.
func (m *Meter) Shutdown(ctx context.Context) error { return m.provider.Shutdown(ctx) }

// Err returns the first instrument declaration that the SDK refused, or nil.
func (m *Meter) Err() error {
	m.errMu.Lock()
	defer m.errMu.Unlock()
	return m.err
}

func (m *Meter) fail(err error) {
	m.errMu.Lock()
	defer m.errMu.Unlock()
	if m.err == nil {
		m.err = err
	}
}

// Counter implements kernel.Meter.
func (m *Meter) Counter(i kernel.Instrument) kernel.Counter {
	c, err := m.meter.Int64Counter(i.Name, metric.WithDescription(i.Description), metric.WithUnit(i.Unit))
	if err != nil {
		m.fail(err)
	}
	return counter{c: c, i: i}
}

// UpDownCounter implements kernel.Meter.
func (m *Meter) UpDownCounter(i kernel.Instrument) kernel.UpDownCounter {
	c, err := m.meter.Int64UpDownCounter(i.Name, metric.WithDescription(i.Description), metric.WithUnit(i.Unit))
	if err != nil {
		m.fail(err)
	}
	return upDownCounter{c: c, i: i}
}

// Histogram implements kernel.Meter.
func (m *Meter) Histogram(i kernel.Instrument) kernel.Histogram {
	opts := []metric.Float64HistogramOption{metric.WithDescription(i.Description), metric.WithUnit(i.Unit)}
	if len(i.Buckets) > 0 {
		opts = append(opts, metric.WithExplicitBucketBoundaries(i.Buckets...))
	}
	h, err := m.meter.Float64Histogram(i.Name, opts...)
	if err != nil {
		m.fail(err)
	}
	return histogram{h: h, i: i}
}

// Gauge implements kernel.Meter.
func (m *Meter) Gauge(i kernel.Instrument, f kernel.GaugeFunc) {
	_, err := m.meter.Float64ObservableGauge(i.Name, metric.WithDescription(i.Description), metric.WithUnit(i.Unit),
		metric.WithFloat64Callback(func(ctx context.Context, o metric.Float64Observer) error {
			return f(ctx, observer{o: o, i: i})
		}))
	if err != nil {
		m.fail(err)
	}
}

// ObservableCounter implements kernel.Meter: an asynchronous monotonic Sum, which the
// Prometheus exporter shows as a counter (_total).
func (m *Meter) ObservableCounter(i kernel.Instrument, f kernel.GaugeFunc) {
	_, err := m.meter.Float64ObservableCounter(i.Name, metric.WithDescription(i.Description), metric.WithUnit(i.Unit),
		metric.WithFloat64Callback(func(ctx context.Context, o metric.Float64Observer) error {
			return f(ctx, observer{o: o, i: i})
		}))
	if err != nil {
		m.fail(err)
	}
}

// Begin implements kernel.Meter: the SDK aggregates every measurement, so a unit of work needs
// nothing.
func (m *Meter) Begin(ctx context.Context) (context.Context, func()) { return ctx, func() {} }

// attributes converts the declared labels of l; labels that i did not declare are dropped.
func attributes(i kernel.Instrument, l kernel.Labels) metric.MeasurementOption {
	kvs := make([]attribute.KeyValue, 0, l.Len())
	for k, v := range l.All() {
		if i.Accepts(k) {
			kvs = append(kvs, attribute.String(k, v))
		}
	}
	return metric.WithAttributeSet(attribute.NewSet(kvs...))
}

type counter struct {
	c metric.Int64Counter
	i kernel.Instrument
}

func (c counter) Add(ctx context.Context, n int64, l kernel.Labels) {
	if c.c != nil && n >= 0 {
		c.c.Add(ctx, n, attributes(c.i, l))
	}
}

type upDownCounter struct {
	c metric.Int64UpDownCounter
	i kernel.Instrument
}

func (c upDownCounter) Add(ctx context.Context, n int64, l kernel.Labels) {
	if c.c != nil {
		c.c.Add(ctx, n, attributes(c.i, l))
	}
}

type histogram struct {
	h metric.Float64Histogram
	i kernel.Instrument
}

func (h histogram) Record(ctx context.Context, v float64, l kernel.Labels) {
	if h.h != nil {
		h.h.Record(ctx, v, attributes(h.i, l))
	}
}

type observer struct {
	o metric.Float64Observer
	i kernel.Instrument
}

func (o observer) Observe(v float64, l kernel.Labels) { o.o.Observe(v, attributes(o.i, l)) }
