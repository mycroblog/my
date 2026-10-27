# Error reporting

A guide for operators: how the server tells you about its own bugs and failures, where to read the reports on each platform, and how to send them to an error tracker.
Metrics tell you that something is wrong; an error report tells you what failed, with its stack. The [configuration reference](../reference/configuration.md) lists every setting.

Three things happen to every report, whatever you configure:

1. One `error report` line goes to standard error, with the stack ([The log line](#the-log-line)).
2. The report is counted in an error group in the database. The administration screen shows the groups, and people with the `manage_errors` permission are notified of new and recurring ones ([Error groups and notifications](#error-groups-and-notifications)).
3. If you set `errors_exporter`, the report also goes to an external service ([Sending reports to a service](#sending-reports-to-a-service)).

Nothing leaves the server unless you set `errors_exporter`.

## What is reported

| Reported | Not reported (the metrics and the other log lines cover them) |
| --- | --- |
| A panic in an HTTP handler, a job, or a background task | 4xx responses (bad input, authorization, rate limits) and the maintenance mode |
| An error behind a 5xx response of the client API, the admin API, the sign-in pages, ActivityPub, OAuth, or media | A request whose client went away |
| A job that failed for good (`dead`) | A job retry. It is reported when it runs out of retries |
| A failure to start: an invalid configuration, or a database or adapter that cannot be set up | A failed readiness check |
| An error the web client did not handle, sent by the browser to the server | Failures caused by other servers: delivery retries and give-ups, rejected signatures, failed remote fetches |
| | The browser's own noise: a lost network, errors from browser extensions, and chunks that a new deploy removed |

A failure of an outside service (mail, storage, payments, voice) is reported only when it makes a job fail for good or a request answer 5xx. A temporary failure shows in the `my.adapter.*` metrics instead ([Running the server](running.md#metrics)).

A panic in a background task stops the process after it is reported, so the server never keeps running in a broken state. Your platform restarts it.

## The log line

The line has `msg` = `error report` and `level` `ERROR` (`severity` is `CRITICAL` when the failure stops the process).
Its fields, and what scrubbing does to them, are in [Running the server](running.md#error-reports). In short:

- `my.error.fingerprint` is the same for every occurrence of the same error. Search or group by it.
- `my.error.where` says where it happened, for example `client_api GET /api/v1/statuses/{id}`.
- `request_id`, `job_id`, `trace_id`, and `span_id` find the other lines of the same request or job.
- `message` is last on the line: the error type and message, a newline, and the Go stack.

No other line carries a stack.

### Google Cloud (Cloud Run)

Cloud Error Reporting reads the line without any setup and lists the errors in the console under Error Reporting. It is free; you pay for Cloud Logging only.

- It groups errors by the type and the top frames of the stack. It cannot take the server's fingerprint, so a group in Error Reporting can differ from a group in the administration screen.
- It keeps groups for 30 days, and it notifies you of new errors and of resolved errors that happen again, at most 20 notifications an hour for the project. Set up its notification channel in the Google Cloud console.
- To find what happened around a report, search Cloud Logging for `jsonPayload."request_id"="<id>"`, or the same with `trace_id`.

### AWS

CloudWatch Logs reads the line as plain JSON.

- Turn on log anomaly detection for the log group. It flags a line pattern it has never seen before and can alarm to SNS. It reads only the first 1,500 characters of a line, which is why `message` is the last field. It does not keep a resolved state.
- To count reports, make a metric filter such as `{ $.msg = "error report" }`. Use no dimension taken from `my.error.fingerprint`: every value of a dimension is billed as a metric of its own. `my.error.source` and `my.error.kind` are safe.
- Find the context with Logs Insights: ``filter `request_id` = "<id>"``.

### Self-hosted

Read the line from the container log: `docker compose logs my | jq -c 'select(.msg == "error report")'`.
With `metrics_exporter = prometheus`, the alert `MyErrorReportsIncreasing` ([Alerts and runbook](alerts.md#myerrorreportsincreasing)) warns when reports rise sharply; the counter behind it is `my.errors.reported`.
For notifications that need no outside service, use the administration screen and the webhooks below. For a tracker, use [`errors_exporter`](#sending-reports-to-a-service).

## Error groups and notifications

The server counts the reports by fingerprint in the database. It needs no setting and no extra service, and it writes little: the first report of a fingerprint is written at once, and the following ones are added up in memory and written once a minute. A hard kill of the process can lose the counts of the last minute; the log lines are not lost that way.
Counts older than 90 days are deleted every day. If the database is what failed, the report stays in the log line only.

### The administration screen

People whose role has `manage_errors` (the built-in `admin` role has it) see "Errors" in the administration menu (`/admin/errors`).

- The list shows the groups, the open ones by default. Filter by state (open, resolved, muted, every state) and by source (the server, the web client). Each row shows the error type, where it happened, the first lines of the message, the counts of the last 24 hours and 7 days, and the first and last time and version.
- A group's page shows the latest occurrence (time, version, process role, request ID, trace ID, the types, the message, and the stack), the occurrences by hour for the last 90 days, and the history of its state changes. The request ID and the trace ID have a "Copy" button: paste them into your log search.
- "Resolve" is for an error you fixed. If it happens again on another version of the server, the group opens again and notifies again. The same error on the version that was resolved is only counted, so the last requests of the old version do not reopen it.
- "Mute" keeps counting but sends no notification or webhook event. "Unmute" opens the group again.

The same actions are available as `GET /api/v1/admin/error_groups` and `POST /api/v1/admin/error_groups/{id}/resolve` (`/mute`, `/unmute`).

### Notifications and webhooks

- People with `manage_errors` get the notification `admin_error_group` for a new error group and for a recurrence. At most 10 arrive an hour for the whole server; the rest of the hour fold into one notification that says more errors were reported. They can also get it by email ([Moderation](moderation.md#notifications-for-moderators-and-the-pending-counts)).
- The webhook events `error_group.created` and `error_group.regressed` carry the same groups, so you can send them to Slack, Discord, or an automation tool ([Server webhooks](webhooks.md), [Webhook reference](../reference/webhooks.md)). The webhook needs `manage_webhooks`, and the events need `manage_errors`.
- A muted group never notifies.

### Errors from the web client

The web client sends the errors its screens did not handle to `POST /api/v1/client_errors` on your own server, so no third party sees them and no content security setting changes. The server answers `204` to everything and drops what it does not accept: a body over 16 KiB, a stack without a file of your own web client, and more than 10 reports a minute from one IP address. The same error is sent once per page view, and at most 20 per page view.
A report sent without a signed-in session is counted and logged but does not notify anyone by itself, so an anonymous script cannot fill your inbox. The group notifies when a signed-in person's browser reports it too.
The stacks are those of the minified build and are not translated back to source, so read the route and the message first. The release is the version of the container image.

## Sending reports to a service

Set `errors_exporter` to send each report also to an error tracker. The log line and the groups in the database stay on. A report is sent once and never retried, and nothing waits for the service.

| Value | Sends to |
| --- | --- |
| `none` (default) | Nowhere |
| `sentry` | Any Sentry-compatible service at `errors_sentry_dsn` |
| `otlp` | An OpenTelemetry collector or backend, as log records |

`errors_environment` (default `production`) names the deployment on every report, such as `staging`.

Both exporters send the same scrubbed fields as the log line, with the fingerprint and the trace and span IDs, and nothing about the host: no host name, IP address, user, request body, header, or cookie.

The server limits what it sends so that the service is not flooded:

- The same fingerprint goes out at most once a minute per process, and all reports at most 60 a minute. The rest stay in the log and the groups.
- Up to 1,000 reports wait to be sent. A report that finds the queue full is dropped. When the server stops, it waits up to 2 seconds for the queue to empty.
- When the service answers `429` or asks to slow down (`Retry-After`, `X-Sentry-Rate-Limits`), the server stops sending for that long.

Watch the counter `my.errors.exported`: `my.outcome` is `success`, `dropped` (over a limit, the queue was full, or the service asked to slow down), or `failed` (the service could not be reached or refused the report) ([Running the server](running.md#metrics)).

### Sentry, GlitchTip, and Datadog

```sh
MY_ERRORS_EXPORTER=sentry
MY_ERRORS_SENTRY_DSN_FILE=/run/secrets/sentry_dsn   # or MY_ERRORS_SENTRY_DSN; the DSN is a secret
```

The server speaks the Sentry protocol itself, so no Sentry SDK is involved and no `SENTRY_*` environment variable is read. It sets the fingerprint, so the service groups by it, and the tags `service` (`my`), `my.role`, `my.error.where`, and `error.fingerprint`. The release is the server's version and the environment is `errors_environment`.

- **Sentry** (hosted or self-hosted): copy the DSN from the project's client keys.
- **GlitchTip**: copy the DSN of the project. It is open source and runs on a small server. Check which alert rules your version offers for new and recurring issues.
- **Datadog Error Tracking**: use the Sentry DSN of your Datadog site, `https://<DD_API_KEY>@sentry-intake.<DD_SITE>/1`. Datadog requires the `service` tag, which the server always sets. Datadog stores these events as logs and groups them in Error Tracking. If you send the log line to Datadog with an agent or a forwarder instead (Error Tracking for Logs), the stack is in `message`; map it to `error.stack` with a log pipeline, or Datadog does not see it as a stack. The `sentry` exporter needs no such mapping.

### OTLP backends

```sh
MY_ERRORS_EXPORTER=otlp
OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318   # or OTEL_EXPORTER_OTLP_LOGS_ENDPOINT
OTEL_EXPORTER_OTLP_HEADERS=authorization=Bearer%20...
```

Each report is an OpenTelemetry log record named `exception`, with severity `ERROR` (`FATAL` when it stops the process), `exception.type`, `exception.message`, `exception.stacktrace`, `my.error.fingerprint`, `my.error.where`, `my.error.source`, `my.error.kind`, and the trace and span IDs. It goes over OTLP/HTTP (protobuf, gzip) with the same resource as the metrics and traces.

Whether the backend turns these records into grouped errors depends on the backend: some build their error views from trace data only, and Datadog needs `error.stack`. For Sentry, GlitchTip, and Datadog, use `sentry`. Use `otlp` when you want the records next to your other logs. The build tag `no_otel` leaves `otlp` out ([Build tags](../reference/build-tags.md)).

## What scrubbing does and does not guarantee

Scrubbing runs once, when the report is created, so the log line, the database, and every exporter receive only the scrubbed text.

- In the message, URLs, email addresses, `@user@host` handles, IP addresses, UUIDs and ULIDs, and numbers of 10 digits or more are replaced with `<url>`, `<email>`, `<acct>`, `<ip>`, `<id>`, and `<n>`, and the message is cut to 1 KiB.
- In the stack, the values of function arguments are replaced with `(...)` and the stack is cut to 16 KiB. File positions stay, relative to the source tree, so the paths of the build machine do not appear.
- No report has a request body, header, query, cookie, account ID, user name, IP address, or User-Agent.

The scrubbing matches patterns, so it does not guarantee that a message holds no personal data. A name, a free text, or a number of fewer than 10 digits that an error message quotes can stay. The `error` field of the ordinary `http request` and `job` lines is not scrubbed this way. Treat the logs and the error tracker as places that may hold fragments of user content, and give them the same access control and retention as the rest of your logs. Once you set `errors_exporter`, the location, retention, and access rules of that service apply to the reports.
