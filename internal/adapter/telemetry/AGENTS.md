# Metrics and traces: OpenTelemetry, Prometheus, CloudWatch EMF

- Naming a metric, a span, or an attribute: [OpenTelemetry semantic conventions](https://opentelemetry.io/docs/specs/semconv/), search for: `http.server.request.duration`, `db.client.operation.duration`, `go.memory.used`, `error.type`
- Changing an instrument, a view, or the export interval: [OpenTelemetry specification](https://opentelemetry.io/docs/specs/otel/), search for: `OTEL_METRIC_EXPORT_INTERVAL`, Histogram, Explicit Bucket Histogram Aggregation, `OTEL_TRACES_SAMPLER`
- Changing how a metric appears in the Prometheus text format: [OpenTelemetry specification](https://opentelemetry.io/docs/specs/otel/), search for: Prometheus and OpenMetrics Compatibility, `_total`, `target_info`, `otel_scope_name`
- Changing the scrape output: [Prometheus exposition formats](https://prometheus.io/docs/instrumenting/exposition_formats/), search for: Text-based format, `# HELP`, `# TYPE`, `_bucket`
- Reading or writing `traceparent`: [W3C Trace Context](https://www.w3.org/TR/trace-context/), search for: `traceparent`, `trace-id`, `parent-id`, `trace-flags`
- Changing the CloudWatch embedded metric format lines: [Amazon CloudWatch User Guide](https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/llms.txt), search for: Embedded metric format specification, `_aws`, `CloudWatchMetrics`, `Dimensions`, `StorageResolution`
