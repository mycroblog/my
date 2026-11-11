# Error reports: Sentry envelopes and OTLP logs

- Changing the envelope or its authentication: [Sentry developer documentation](https://develop.sentry.dev/llms.txt), search for: Envelopes, `X-Sentry-Auth`, `sentry_version`, `sent_at`, `application/x-sentry-envelope`
- Changing the payload of an event: [Sentry developer documentation](https://develop.sentry.dev/llms.txt), search for: Event Payloads, Stack Trace Interface, `in_app`, `fingerprint`
- Following the rate limits of the receiver: [Sentry developer documentation](https://develop.sentry.dev/llms.txt), search for: Rate Limiting, `X-Sentry-Rate-Limits`, `Retry-After`
- Changing the attributes of an exception log record: [OpenTelemetry semantic conventions](https://opentelemetry.io/docs/specs/semconv/), search for: `exception.type`, `exception.message`, `exception.stacktrace`
- Changing the OTLP exporter's endpoint or its settings: [OpenTelemetry specification](https://opentelemetry.io/docs/specs/otel/), search for: `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_HEADERS`
