// SPDX-License-Identifier: AGPL-3.0-only

package otel_test

import (
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/mycroblog/my/internal/adapter/telemetry/otel"
	"github.com/mycroblog/my/internal/app/kernel"
)

// resourceAttrs returns the attributes of r by key.
func resourceAttrs(r *resource.Resource) map[string]string {
	out := map[string]string{}
	for _, kv := range r.Attributes() {
		out[string(kv.Key)] = kv.Value.Emit()
	}
	return out
}

// TestResourceNamespace checks that the instance's host is the service.namespace of the resource
// of the metrics, the traces, and the exported errors, which tells apart the instances that share
// a backend, and that the code leaves service.instance.id to the platform.
func TestResourceNamespace(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	r, err := otel.NewResource(t.Context(), "", "1.2.3", "web", "example.test")
	if err != nil {
		t.Fatal(err)
	}
	got := resourceAttrs(r)
	for k, v := range map[string]string{"service.name": "my", "service.version": "1.2.3", "my.role": "web", "service.namespace": "example.test"} {
		if got[k] != v {
			t.Errorf("resource %s = %q, want %q (%v)", k, got[k], v, got)
		}
	}
	if v, ok := got["service.instance.id"]; ok {
		t.Errorf("resource service.instance.id = %q, want none from the code", v)
	}
}

// TestResourceWithoutNamespace checks that an empty namespace leaves service.namespace out.
func TestResourceWithoutNamespace(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	r, err := otel.NewResource(t.Context(), "", "1.2.3", "web", "")
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Set().Value(attribute.Key("service.namespace")); ok {
		t.Errorf("resource service.namespace = %q, want none", v.Emit())
	}
}

// TestResourceAttributesFromEnvironment checks that OTEL_RESOURCE_ATTRIBUTES replaces the
// namespace of the host, adds attributes, and keeps the service.instance.id the platform gives.
func TestResourceAttributesFromEnvironment(t *testing.T) {
	tests := []struct {
		name, env string
		want      map[string]string
	}{
		{
			name: "replaces the namespace and adds", env: "service.namespace=other,foo=bar",
			want: map[string]string{"service.namespace": "other", "foo": "bar", "service.name": "my", "my.role": "web"},
		},
		{
			name: "keeps the instance ID", env: "service.instance.id=run-revision-0001",
			want: map[string]string{"service.namespace": "example.test", "service.instance.id": "run-revision-0001"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_RESOURCE_ATTRIBUTES", tt.env)
			r, err := otel.NewResource(t.Context(), "", "1.2.3", "web", "example.test")
			if err != nil {
				t.Fatal(err)
			}
			got := resourceAttrs(r)
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("resource %s = %q, want %q (%v)", k, got[k], v, got)
				}
			}
		})
	}
}

// TestMetricsCarryNamespace checks the namespace on the resource of the metrics: the target_info
// of the Prometheus scrape.
func TestMetricsCarryNamespace(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	m, err := otel.New(t.Context(), otel.Options{Exporter: otel.ExporterPrometheus, Role: "web", Namespace: "example.test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Shutdown(t.Context()) })
	m.Counter(kernel.Instrument{Name: "my.job.runs", Unit: "{job}"}).Add(t.Context(), 1, kernel.Labels{})
	out := scrape(t, m)
	var info string
	for line := range strings.Lines(out) {
		if strings.HasPrefix(line, "target_info{") {
			info = line
		}
	}
	if !strings.Contains(info, `service_namespace="example.test"`) {
		t.Errorf("target_info = %q, want service_namespace=\"example.test\"\n%s", info, out)
	}
}

// TestTracesCarryNamespace checks the namespace on the resource of every span.
func TestTracesCarryNamespace(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	exp := tracetest.NewInMemoryExporter()
	tr, err := otel.NewTracer(t.Context(), otel.TracerOptions{Role: "web", SampleRatio: 1, Namespace: "example.test", Exporter: exp})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Shutdown(t.Context()) })
	_, s := tr.Start(t.Context(), kernel.SpanSpec{Name: "GET /api/v1/instance", Kind: kernel.SpanServer}, kernel.Attrs{})
	s.End("")
	// The in-memory exporter forgets its spans on Shutdown: flush instead.
	spans := flushed(t, tr, exp)
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	got := resourceAttrs(spans[0].Resource)
	if got["service.namespace"] != "example.test" {
		t.Errorf("span resource service.namespace = %q, want example.test (%v)", got["service.namespace"], got)
	}
	if v, ok := got["service.instance.id"]; ok {
		t.Errorf("span resource service.instance.id = %q, want none from the code", v)
	}
}
