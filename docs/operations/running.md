# Running the server: start, stop and health checks

A guide for operators. The [configuration reference](../reference/configuration.md) lists every setting.

## Start the server

```sh
my config check   # report every configuration problem at once
my config check -instance   # also warn about instance settings that are not set (opens the database)
my serve          # start according to the configured role (-config <file> reads a TOML file)
my version        # print the version of the binary
```

`my serve` validates the configuration, builds the database, job queue and key-management adapters, and accepts HTTP on `listen`.
It speaks plain HTTP/1.1 there, without TLS: put a reverse proxy or load balancer in front that terminates HTTPS (and HTTP/2 or HTTP/3 for the clients) and forwards over HTTP/1.1.
The real-time updates (`GET /api/v1/streaming` and the voice chat room streams) are long-lived responses that send a heartbeat comment every 30 seconds (15 for a voice room) and stay open while idle.
The proxy must not buffer these responses (nginx: the server sends `X-Accel-Buffering: no`), and its idle or read timeout must be longer than the heartbeat interval.
It does not apply the schema: apply it with Atlas before starting (see [Database](database.md); upgrades run it as part of [Upgrading](database.md#upgrading)).
In the Docker Compose setup (`deploy/docker/compose.yaml`; see the [README](../../README.md#quick-start-docker-compose)), a one-shot `schema` service applies it before the server starts.
If the schema is not applied, the process still starts and readiness returns 503.

In the `production` profile, `my config check` also checks that the valid settings suit a production instance: errors, such as a `host` that other servers cannot reach or a KEK that is not random, make it exit 1, and warnings, such as `mail = none`, do not.
`my serve` logs both as warnings when it starts but starts anyway, so run `my config check` as a step of the deployment.
The [production checklist](production-checklist.md) lists the checks and what to verify by hand before going live.

| `role` | HTTP | Job worker |
| --- | --- | --- |
| `all` (default) | All routes | Runs |
| `web` | All routes | Does not run |
| `worker` | Health checks only (for platform probes) | Runs |

Logs go to standard error as JSON Lines. Every line has `level` (`DEBUG`, `INFO`, `WARN`, `ERROR`) and `severity`, the same level in the form Cloud Logging reads (`DEBUG`, `INFO`, `WARNING`, `ERROR`, and `CRITICAL` for an error that stops the process).
Logs written while handling a request carry a `request_id`, and the same ID is returned in the `X-Request-Id` response header.
If the request's `X-Request-Id` is at most 128 characters of letters, digits and `-_.:`, it is used as is, as the ID assigned by a load balancer.
Logs inside a trace (requests and jobs) also carry `trace_id` and `span_id` (see [Traces](#traces)).

Each HTTP request and each job execution writes one line when it finishes. The line has the same names and values as the labels of the [metrics](#metrics) below,
so on Cloud Run with `metrics_exporter = none` you can build Cloud Logging log-based metrics (counters and distributions) from these lines.

| Line (`msg`) | Fields |
| --- | --- |
| `http request` (`INFO`; health checks use `DEBUG`) | `http.request.method`, `http.route`, `my.surface` and `http.response.status_code` (a number) have the same values as the labels of `http.server.request.duration`. Also `duration_seconds` (a number of seconds), `url.path` (the path without the query string), `http.response.body.size` and `request_id`. `http.route` and `my.surface` are absent for requests that did not reach a route (health checks, requests refused before routing), and the metric does not count them |
| `job` (`INFO` on success, `WARN` for `retry`, `ERROR` for `dead`) | `my.job.type` and `my.outcome` have the same values as the labels of `my.job.duration`. Also `duration_seconds` (`my.job.duration`), `wait_seconds` (`my.job.wait`), `job_id`, `attempt`, and `error` on failure |
| `error report` (`ERROR`; `CRITICAL` for a failure that stops the process) | One line per reported error ([Error reports](#error-reports)) |

### Error reports

The [Error reporting](error-reporting.md) guide covers where to read these lines on each platform, the Errors screen, notifications, and sending reports to an error tracker.

The server reports the errors that need the operator's attention as one `error report` line each:

- a panic in an HTTP handler, a job, or a background task;
- an error behind a 5xx response of the client API, the admin API, the sign-in pages, ActivityPub, OAuth, or media;
- a job that fails for good (`dead`): it ran out of retries, or refused to run again. Deliveries that gave up because the other server kept failing are not reported;
- a failure to start: an invalid configuration, or a database or adapter that cannot be set up.

4xx responses, job retries, the maintenance mode, failed readiness checks, requests whose client went away, and failures caused by other servers are not reported; the metrics and the other log lines cover them.

The line is in the form Google Cloud Error Reporting reads: on Cloud Run, the reports appear in Error Reporting grouped as errors without any setup. On other platforms it is a plain JSON line:

| Field | Value |
| --- | --- |
| `@type`, `serviceContext` | The Error Reporting event type, and `{"service": "my", "version": "<version>"}` |
| `my.error.fingerprint` | 32 hexadecimal digits that are the same for every occurrence of the same error: built from the source, the place, the error type, and the functions of the server in the stack, without line numbers |
| `my.error.source`, `my.error.kind` | `server` (or `web`), and `panic` or `error` |
| `my.error.where` | Where it happened: `<surface> <method> <route>` for a request (such as `client_api GET /api/v1/statuses/{id}`), the job type for a job, the task name for a background task, `startup` for a failure to start |
| `my.role`, `deployment.environment.name` | The `role` of the process and `errors_environment` |
| `request_id`, `job_id`, `attempt`, `trace_id`, `span_id` | The IDs to find the request or job lines of the same error, when it has them |
| `message` | Last on the line: `<type>: <message>`, a newline, and the Go stack |

Before a report leaves the process, the message has URLs, email addresses, `@user@host` handles, IP addresses, UUIDs and ULIDs, and numbers of 10 digits or more replaced with `<url>`, `<email>`, `<acct>`, `<ip>`, `<id>` and `<n>`, and is cut to 1 KiB.
The stack keeps its file positions but not the argument values of its function lines (they are shown as `(...)`), and is cut to 16 KiB. No report carries a request body, header, query, cookie, account, user name, IP address, or User-Agent.
No other line carries a stack, so Error Reporting finds the stacks of these lines only.

A field name is a single key that contains `.`; it is not nested. Quote it in Cloud Logging filters and extractions (for example `jsonPayload."http.route"`, and `EXTRACT(jsonPayload.duration_seconds)` for a distribution value).
Fields used as labels take values from a finite set only; `url.path`, `request_id` and `job_id` are never labels.

With `metrics_exporter = emf`, these two lines are the EMF lines themselves. The `http request` line carries the value of `http.server.request.duration`, and the `job` line carries the values of `my.job.duration` and `my.job.wait` (keys named after the metrics),
plus the `_aws` directive, `service.name`, `service.version` and `my.role`. These three metrics are not written on any other line. Fields you choose in `metrics_emf_dimensions` are written as strings, as EMF requires (for example the dimension `http.response.status_code` is `"200"`).
Logs go to standard error and the other EMF lines go to standard output. Lambda sends both to CloudWatch Logs, so metrics are extracted from both.

## Publish the source code (AGPL-3.0)

my is licensed under AGPL-3.0-only. Section 13 of the AGPL-3.0 requires anyone who runs the server and lets users interact with it over a network
to offer those users a way to get the corresponding source code of the program that is running. This applies to everyone who interacts with the server, not only to people with an account.

Point to where the source is published with the setting `source_url` (environment variable `MY_SOURCE_URL`; see the [configuration reference](../reference/configuration.md#source_url)).

| Value | Meaning |
| --- | --- |
| Not set | The default, `https://github.com/mycroblog/my`. Use it when you run my unmodified |
| An `https:` URL | A place where the source can be read (the repository of a fork, the tag or archive of a particular version, and so on) |
| A `mailto:` URI | An email address to request the source from (for example `mailto:source@example.org`; `?subject=...` is allowed) |

Any other value (`http:`, `git@...`, a relative path, a URL with user information or a `#`, a `mailto:` without an address, and so on) makes `my config check` fail, and the server does not start.

The value appears in these places:

- `source_url` in the client API's `GET /instance`
- The "Source code" section of the web client's instance information page (`/about`) and a link on the landing page (`/`)
- `metadata.sourceCode` in NodeInfo (`/nodeinfo/2.1`). An `https:` URL is also published as `software.repository`
  (the NodeInfo 2.1 schema allows only an `http(s)` URL in `software.repository`, so a `mailto:` value appears only in `metadata.sourceCode`)

### When you fork or modify my

If you run a modified my (you apply patches, build a fork, or swap dependencies), the default URL no longer points to the source of what is running.
Change `source_url` to where the source of the running version, with your modifications, can be found. Pointing to the commit, tag or archive that matches the running version, rather than the latest state of a repository, lets users reliably get the same source.
If you run an unmodified official release, keep the default.

### When you use `mailto:`

- Be ready to hand over the corresponding source code (including the scripts needed to build it) promptly when someone asks. Only naming an address, without being prepared to deliver, does not meet the obligation.
- Use an address that still works after you stop operating the server or the person in charge changes. A shared operations address is better than a personal inbox.
- Make sure spam filtering does not drop requests.
- Use an address you are happy to make public: anyone can see it on the information page, the landing page and in NodeInfo.

## Metrics

The setting `metrics_exporter` (default `none`) exports service metrics. Its related settings are in the [configuration reference](../reference/configuration.md).

| `metrics_exporter` | Path | Suited to |
| --- | --- | --- |
| `none` | Nothing is exported. Instrumentation costs almost nothing | Cloud Run (use the platform's metrics and log-based metrics instead) |
| `prometheus` | `GET /metrics` on `metrics_listen` (default `127.0.0.1:9464`). It is a listener separate from the public `listen`, with no authentication, and runs for every `role` | Self-hosted (scraped by a Prometheus-compatible server or KEDA) |
| `otlp` | Sent to an OpenTelemetry Collector over OTLP/HTTP. The destination comes from the standard OTel environment variables such as `OTEL_EXPORTER_OTLP_ENDPOINT` | Setups where you can run a collector |
| `emf` | CloudWatch Embedded Metric Format JSON lines. Request and job metrics ride on the closing log lines above; the others are written to standard output, one line per label set, when a request or job finishes | AWS Lambda |

The metrics are listed below. In Prometheus, `.` in a name becomes `_` and a unit suffix (`_seconds`, and `_total` for a counter) is added.

| Metric | Kind | Labels |
| --- | --- | --- |
| `http.server.request.duration` | Histogram, seconds | `http.request.method`, `http.route` (the route pattern's path; `other` when none matches), `http.response.status_code`, `my.surface` (`client_api` / `admin` / `activitypub` / `oauth` / `media` / `web` / `payments` / `voice`) |
| `http.server.active_requests` | UpDownCounter | `my.surface` |
| `my.inbox.activities` | Counter | `activity.type` (a known Activity type, or `other`), `my.outcome` (`accepted` / `duplicate` / `rejected` / `ignored` / `error`), `my.inbox.rejection` (only with `rejected`: `signature` for a signature or integrity proof that is missing, invalid or does not verify, or whose key could not be fetched; `suspended` for a suspended domain; `federation_off`; `rate_limited`; `invalid` for an activity without a valid `id`, `type`, `actor` or `object`) |
| `my.relay.activities` | Counter | `my.outcome` (`admitted` / `over_limit` / `restricted` / `not_receiving`: whether an activity carried by a relay was ingested, and why it was dropped; see [Federation relays](relays.md)) |
| `my.move.activities` | Counter | `my.outcome` (`recorded` / `duplicate` / `not_own_actor` / `same_account` / `target_unavailable` / `alias_missing` / `target_moved` / `suspended` / `too_soon`: whether a received `Move` was recorded, and why it was dropped) |
| `my.delivery.attempts` | Counter | `my.outcome` (`success` / `retry` / `gone` / `failed`; `gone` covers a 410 and deliveries skipped because the server is unavailable or suspended) |
| `my.delivery.duration` | Histogram, seconds | `my.outcome` |
| `my.job.duration` | Histogram, seconds | `my.job.type` (a registered job type, or `other`), `my.outcome` (`success` / `retry` / `dead`) |
| `my.job.wait` | Histogram, seconds | `my.job.type` |
| `my.timeline.delivery.lag` | Histogram, seconds | None (from the creation of a local post to its write into home timelines and the notification to streams) |
| `my.stream.connections` | UpDownCounter | `my.stream.kind` (`user`, `voice` for the streams of the voice chat rooms) |
| `my.accounts.registered` | Counter | None |
| `my.statuses.created` | Counter | `my.visibility` |
| `my.billing.imports` | Counter | `my.billing.source` (`aws` / `gcp`), `my.outcome` (`success` / `failed`), `error.type` (only with `failed`: the kind of failure, as in [Importing cloud usage charges](billing-import.md)). Every attempt counts, including the retries and the failures that wait for the next daily import |
| `my.voice.turn.credentials` | Counter | `my.voice.turn` (`coturn` / `cloudflare`), `my.outcome` (`issued` / `reused` / `failed`). One per TURN credential that a voice chat participant asked for when joining a room or renewing its connection: `reused` means Cloudflare credentials issued earlier for the same connection were returned again, and `failed` means Cloudflare could not be reached or refused, so the participant got an error and could not connect |
| `my.errors.exported` | Counter | `my.outcome` (`success` / `dropped` / `failed`: one per error report for the service of `errors_exporter`. `dropped` means the report was not sent because 1,000 reports were already waiting or the service asked the server to slow down, and the server waits as long as the service asks before sending again; `failed` means the service could not be reached or refused the report. A report is sent once and never retried, and the reports still waiting 2 seconds into a shutdown are lost) |
| `my.queue.ready` | Gauge | `my.job.type` (the number of runnable jobs in the database queue) |
| `my.queue.oldest_ready_age` | Gauge, seconds | None (how long the oldest runnable job in the database queue has waited) |
| `my.queue.dead` | Gauge | None |
| `my.instance.users`, `my.instance.statuses` | Gauge | None (the local user and post counts, as in NodeInfo) |
| `my.media.stored_bytes` | Gauge, bytes | `my.media.origin` (`local` / `remote_cache`) |
| `my.db.size` | Gauge, bytes | None (the size of the database: `pg_database_size` for PostgreSQL; the database file without its write-ahead log for SQLite) |
| `my.billing.import.failing` | Gauge | `my.billing.source` (1 when the newest import of the source failed, 0 otherwise) |
| `my.federation.instances` | Gauge | `my.federation.instance.state` (`available`: servers that deliveries still go to but that failed on at least one day since they last answered; `unavailable`: servers whose deliveries are paused) |
| `my.metrics.snapshot.age` | Gauge, seconds | `my.metrics.snapshot.part` (`queue` / `totals`) |
| `my.errors.reported` | Counter | `my.error.source` (`server` / `web`), `my.error.kind` (`panic` / `error`). One per [error report](#error-reports) |

Health checks (`/healthz`, `/readyz`) are not measured. No label contains a remote domain, account, URL or client IP.
`my.queue.*`, `my.instance.*`, `my.media.stored_bytes`, `my.db.size`, `my.federation.instances` and `my.billing.import.failing` are values for the whole instance: every process reports the same value from one snapshot kept in the key-value store.

### System metrics

Metrics about the process, the Go runtime and connections to resources are exported through the same `metrics_exporter`.
Container CPU, memory, instance count and ingress requests are left to the platform's metrics; these cover saturation inside the process that the platform cannot see.
Do not use them to trigger scaling; use them for alerts and for estimating the instance cap.

| Metric | Kind | Labels |
| --- | --- | --- |
| `go.memory.used`, `go.memory.limit`, `go.memory.gc.goal` | Gauge, bytes | Only `go.memory.used` has `go.memory.type` (`stack` / `other`). `go.memory.limit` appears only when `GOMEMLIMIT` is set |
| `go.memory.allocated`, `go.memory.allocations` | Counter | None |
| `go.goroutine.count`, `go.processor.limit`, `go.config.gogc` | Gauge | None |
| `go.schedule.duration`, `go.memory.gc.pause.duration` | Histogram, seconds | None (the wait from becoming runnable to running, and GC pauses) |
| `go.memory.gc.cycles` | Counter | None |
| `process.cpu.time` | Observable Counter, seconds (total since start) | `cpu.mode` (`user` / `system`) |
| `process.memory.usage` | Gauge, bytes | None (resident memory, RSS) |
| `process.unix.file_descriptor.count`, `my.process.file_descriptor.limit` | Gauge | None (open file descriptors and the limit) |
| `process.uptime` | Gauge, seconds | None |
| `db.client.connection.count` | Gauge | `db.client.connection.pool.name` (`postgres` / `sqlite`), `db.client.connection.state` (`idle` / `used`) |
| `db.client.connection.max` | Gauge | `db.client.connection.pool.name` |
| `db.client.connection.wait_time`, `my.db.client.connection.waits`, `db.client.connection.timeouts` | Observable Counter (total since start; seconds or count) | `db.client.connection.pool.name`. `timeouts` is PostgreSQL only |
| `my.adapter.operation.duration` | Histogram, seconds | `my.adapter` (`queue.db` / `kv.db` / `kv.memory` / `kv.valkey` / `stream.<stream>` / `blob.fs`), `my.operation`, `my.outcome` (`success` / `error`) |
| `my.adapter.connections` | UpDownCounter | `my.adapter` (`valkey` connections, and `stream.<stream>` subscribers) |
| `my.adapter.reconnects` | Counter | `my.adapter` |
| `http.client.request.duration` | Histogram, seconds | `my.http.purpose` (`json` / `html` / `image` / `video` / `human_check` / `web_push` / `webhook` / `client_metadata` / `login_provider`), `http.request.method`, `my.http.status_class` (`2xx` and so on), `error.type` (`forbidden_destination` is a refusal by the SSRF guard) |
| `http.client.open_connections` | UpDownCounter | None |
| `my.readiness.failures` | Counter | None |

- Go runtime metrics (`go.*`) are exported only by `prometheus` and `otlp`. With `emf`, a periodic snapshot job writes the process and connection-pool gauges and observable counters.
- Pool waits and CPU time are Observable Counters: the pool and the kernel keep totals since start, and they are read at collection time. In Prometheus they are `_total` counters (such as `process_cpu_time_seconds_total`), in OTLP they are monotonic sums, and you see their growth with PromQL `rate()`.
  `emf` has no total counters, so it writes the increase since the previous write (add them up with the CloudWatch Sum statistic).
- Queue `claim` is measured only when it takes a job or fails. A waiting worker's polling records nothing.
- `db.client.connection.pending_requests` is not exported: the pgx and `database/sql` statistics do not report how many are waiting, so the number and duration of waits stand in for it.
- Alert on the ratio of the sum of `go.memory.used` to `go.memory.limit`, and on the growth of `db.client.connection.wait_time` (the pool limit is being hit).

### Memory limit (`GOMEMLIMIT`)

`GOMEMLIMIT` is a Go runtime environment variable that the operator sets; the application does not choose it.
Without it, Go runs with its default (no limit), and the heap does not collect eagerly as it approaches the container's memory limit.
On setups with a container memory limit (Cloud Run, Lambda, ECS, Kubernetes), set it to about 90% of that limit (for example `GOMEMLIMIT=460MiB` for a 512 MiB container).
Do not set `GOMAXPROCS`: Go's default follows the container's CPU limit.

## Traces

The setting `traces_exporter` (default `none`) exports distributed traces. Its related settings are in the [configuration reference](../reference/configuration.md).

| Span | Name | Attributes |
| --- | --- | --- |
| HTTP request (server) | `<method> <http.route>` (`<method>` for a request that did not reach a route) | `http.request.method`, `http.route`, `http.response.status_code`, `my.surface`. A 5xx is a failure |
| Job execution (consumer) | The job type (`my.job.type`) | `my.job.type`, `my.job.attempt`, `my.outcome`. Each retry is a separate span |
| Transaction | `my.db.transaction` | `db.system.name`. Each retry is a separate span |
| Query (client) | The sqlc query name (such as `InsertJob`; otherwise the first word of the SQL, such as `BEGIN`) | `db.system.name`, `db.operation.name` |
| Delivery (`my.delivery`) | `my.delivery` | `activity.type`, `my.outcome` |
| Outgoing HTTP (client) | `<method>` | `http.request.method`, `server.address` (the destination host name only), `http.response.status_code`. A 4xx or 5xx is a failure |

A failed span has `error.type` (a finite name such as `timeout`, or a status code). Attributes never contain account, post or job IDs, URLs, paths or queries, client IPs, User-Agent values, SQL text or error strings.
`server.address` leaves the host names of remote servers with the receiver, so you decide how long the receiver keeps them and who can view them. Health checks create no span.

- A job execution is not a child of the span that enqueued it (usually an HTTP request). It is the **root of a new trace** with a link to the enqueuing span. In a viewer, follow the link from the request's trace.
- An incoming `traceparent` becomes the parent only when the connecting peer is in `trusted_proxies`. Otherwise it is discarded and a new trace starts. `tracestate` is always discarded. Outgoing HTTP (federation, WebFinger, media, Web Push and so on) carries no `traceparent`.
- Sampling: a request is recorded if its trusted-proxy parent was recorded, or if the link target of a job was recorded; otherwise it is recorded at the ratio `traces_sample_ratio` (default 0.01). Lowering the ratio never leaves the jobs that continue a recorded request without their trace.

| Target | Settings | Path |
| --- | --- | --- |
| Default, small self-hosted | `traces_exporter = none` | No spans are exported. Follow `request_id` and `job_id` in the logs. If a trusted proxy supplies a `traceparent`, its `trace_id` and `span_id` are added to the logs |
| Self-hosted (Jaeger, Tempo, an OpenTelemetry Collector and so on) | `traces_exporter = otlp`, `traces_flush = batch` | Sent over OTLP/HTTP to the receiver at `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` (or `OTEL_EXPORTER_OTLP_ENDPOINT`, default `http://localhost:4318`). Authentication headers go in `OTEL_EXPORTER_OTLP_HEADERS`. To reliably keep errors and slow traces, use the Collector's tail sampling |
| Google Cloud (Cloud Run) | `none`, or `otlp` with `unit` to enable traces | See below |
| AWS (Lambda) | `none`, or `otlp` with `unit` to enable traces | See below |

### Cloud Run

Even with `none`, the request trace that Cloud Run creates automatically and the `trace_id` in the logs share the same ID (when you trust the connection from in front of Cloud Run with `trusted_proxies`).
Whether Cloud Run always overwrites the `traceparent` of public requests is not yet verified.
The logs are not shaped as the trace field that Cloud Logging recognizes (`logging.googleapis.com/trace`).

To send application spans to Cloud Trace, run an **OpenTelemetry Collector as a sidecar** in the same service. The server holds no Google credentials; the sidecar sends to the Cloud Trace OTLP endpoint with Application Default Credentials.

- Server (the ingress container): `MY_TRACES_EXPORTER=otlp`, `MY_TRACES_FLUSH=unit`, `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`
- Sidecar: an OpenTelemetry Collector that receives with an `otlp` receiver over HTTP on `localhost:4318` and sends with the `googlecloud` exporter, or the `otlphttp` exporter (`https://telemetry.googleapis.com`, with the `googleclientauth` extension). Grant the service account permission to write to Cloud Trace.
- At the end of each recorded request or job, `unit` flushes the spans so far to the sidecar before returning. So that the sidecar can send them while no CPU is allocated, use instance-based billing for a setup with traces enabled.
- The cost is the sidecar's resources and the span volume (Cloud Trace pricing). Choose the ratio with `traces_sample_ratio`.

### AWS Lambda

To send to X-Ray, attach the **AWS Distro for OpenTelemetry (ADOT) Collector Lambda layer** to the function. The server has no SigV4 signing; the layer's Collector signs and sends to the X-Ray OTLP endpoint (`https://xray.<region>.amazonaws.com/v1/traces`).

- Function: `MY_TRACES_EXPORTER=otlp`, `MY_TRACES_FLUSH=unit`, `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`. In the layer's Collector configuration (`OPENTELEMETRY_COLLECTOR_CONFIG_URI`), receive with an `otlp` receiver on `localhost:4318` and send to X-Ray with an `otlphttp` exporter that uses the `sigv4auth` extension.
- Grant the execution role permission to write to X-Ray. Lambda's own X-Ray setting (`Active` / `PassThrough`) decides only Lambda's own segment and does not affect this server's spans.
- With `unit`, the spans of a recorded invocation are flushed to the layer at its end, before the environment freezes.
- The cost is the layer's share of the start-up time, and the number of traces X-Ray records (the first 100,000 per month are free).

## Stop the server

On SIGTERM or SIGINT the server shuts down gracefully.

1. It stops accepting new connections and taking jobs.
2. It waits up to 25 seconds for in-flight requests and running jobs to finish.
3. After 25 seconds it cancels the context of the remaining requests and jobs and closes connections.
   It then gives a cancelled job up to 3 more seconds to record its result, and exits with code 1 (at most 28 seconds in all, within the default 30-second grace period of Kubernetes and ECS).
   A cancelled job is retried. A job that does not stop within those 3 seconds, or that is killed midway, is abandoned and picked up again by a worker when its lease expires, so it may run twice.

A panic in one of the server's background tasks starts the same shutdown, so that the server never keeps running in a broken state.
The `serve failed` log line names the task, and the process exits with code 1 for the platform or the process manager to restart it.

For work on the database, key-value store or queue, restart in maintenance mode (`MY_MAINTENANCE=true`) instead of stopping. See [Maintenance](maintenance.md).

## The job queue and failed (dead) jobs

The admin jobs page (`/admin/jobs`, permission `manage_jobs`) lists the queue's jobs by state (waiting, running, waiting to retry, failed).
It also shows the count per type and state, when the oldest waiting job is scheduled, each job's last error, key and payload, and the history of unsuccessful attempts (result, error, worker, and start and end time of each attempt).
The admin API is `GET /api/v1/admin/queue`, `GET /api/v1/admin/jobs?state=&type=`, `GET /api/v1/admin/jobs/{id}/attempts` and `POST /api/v1/admin/jobs/{id}/retry`.

- Waiting, running and retry-waiting jobs can be listed only for the database queue (`queue = db`) and `queue = inline`. For Cloud Tasks and SQS queues the page says so, and you watch backlog in the platform's metrics. Failed jobs and the attempt history are in the application database for every queue, so they can always be listed.
- The attempt history keeps only unsuccessful attempts (retries and the attempt that finally failed). Follow a successful run through the structured `job` log line (which carries `job_id`).
  The history is removed by the hourly cleanup once `job_attempt_retention` (default 30 days; see the [setting](../reference/configuration.md#job_attempt_retention)) has passed since the attempt ended.
  The history holds no payload or handler log, and an error message is cut to its first 2,000 bytes. Job handlers keep email addresses, IP addresses, tokens and post bodies out of error messages.
  For a view limited to email job types, see [Sending email](email.md).

A job whose handler decided it failed for good, and a job that used up its retries, becomes a dead job in the database queue (`queue = db`).
`my job` lists, retries and deletes them. The "Failed" state of the jobs page and the admin API can also list and retry them.

```sh
my job list                  # newest failures first: ID, type, run count, time of failure, last error (-limit sets the count, default 50)
my job retry <id> [<id>...]  # reset the attempt count and put the job back in the queue to run immediately
my job delete <id> [<id>...] # remove without running
```

- If a job with the same idempotency key is already waiting, that job stands in for the retried one. Job handlers are idempotent, so putting the same job back any number of times is safe.
- An unknown ID is printed and the command exits with code 1; the other IDs are still processed. Exit code 2 means a usage error.
- A dead job that nobody retries is removed by the hourly cleanup once `dead_job_retention` (default 30 days; see the [setting](../reference/configuration.md#dead_job_retention)) has passed since the failure.
- Dead jobs of `queue = inline` live in the server process's memory, so `my job` cannot handle them (exit code 1).

## Health checks

| Path | Kind | When it returns 200 |
| --- | --- | --- |
| `GET /healthz` | liveness | The process can answer HTTP. It does not look at dependencies such as the database (so a slow database does not restart the process) |
| `GET /readyz` | readiness | The database is reachable and the schema is applied |
| `GET /api/v1/instance/health` | Uptime monitoring, status pages and clients (served by the `web` role) | The same as readiness. The result is cached in the process for 60 seconds |

- A 200 from `/healthz` or `/readyz` has the body `{"status":"ok"}`.
- `/readyz` checks the database on every request, and concurrent requests share one check.
- `/api/v1/instance/health` returns 200 and `{"status":"ok","checked_at":"<RFC 3339>"}` when healthy, and 503 and `{"status":"unavailable","checked_at":...}` on failure (`Cache-Control: no-store`).
  Each process touches the database at most once a minute however often it is monitored or requested, and it looks at neither the access token nor the rate limit. Point uptime monitors at this endpoint, not at the probe (`/readyz`).
  For per-service setup, monitoring interval and cost, and how maintenance is treated, see the [uptime monitoring guide](monitoring.md).
- All three log at the Debug level in the access log and create no trace span.
- When readiness is not met, the response is 503 with an RFC 9457 `application/problem+json` body.
  Its `title` is translated according to `Accept-Language` and contains no internal detail. The cause (a connection error, or the names of missing tables) goes to the log.
- One readiness check is cut off after 3 seconds.
- **Schema applied** means every table that the files in `schema/<dialect>/` declare with `CREATE TABLE` exists
  (SQLite is checked through `sqlite_schema`, PostgreSQL through the connection's `current_schema()`). Atlas applies the whole schema, so a missing table means the schema is not applied or is older than the binary.
  Column and index differences are not checked; `scripts/task.sh db:check` detects differences.

## Security headers

Every response carries `Strict-Transport-Security`, `X-Content-Type-Options`, `Referrer-Policy`, `Permissions-Policy`, `Cross-Origin-Opener-Policy`, `Cross-Origin-Resource-Policy: same-site` and `X-Frame-Options`,
and, for responses that are not documents, `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'; base-uri 'none'`.
A handler overrides these for responses that need other values, such as ActivityPub JSON and public media.
`Permissions-Policy` denies the camera, geolocation, payment and USB. It denies the microphone too, except while the voice chat is on (`voice` is not `off`):
then it allows the microphone for pages of the server's own origin only (`microphone=(self)`), so that browsers can speak in a call.
The web client's HTML gets a document CSP that does not use a nonce (`script-src 'self'` and so on).
It requires Trusted Types (`require-trusted-types-for 'script'`) and limits the policies that can be created to the client's two (`trusted-types my-server-sanitized-html my-service-worker`).

## Web client

A process whose `role` is `web` or `all` serves the web client embedded in the binary.

| Path | Content | `Cache-Control` |
| --- | --- | --- |
| Any `GET` that no other route matches (`/`, `/@alice`, `/@alice/status/{id}`, `/settings` and so on) | The SPA's HTML shell. The instance settings are passed in `<script type="application/json" id="config">` | `public, max-age=60, stale-while-revalidate=300`, ETag |
| `GET /assets/*` | Build files with hashed names | `public, max-age=31536000, immutable`, ETag |
| `GET /manifest.webmanifest` | The web app manifest built from the instance settings | `public, max-age=60, stale-while-revalidate=300`, ETag |
| `GET /sw.js`, `/icon.svg` and other files directly under the build | Served as they are | `no-cache`, ETag |
| `GET /licenses/*` | The licenses of what the build bundles (npm package notices, and `LICENSE-Unicode.txt` and `OFL.txt` for bundled non-npm data). Served as `text/plain; charset=utf-8` whatever the extension | `no-cache`, ETag |
| `GET /robots.txt`, `/.well-known/tdmrep.json`, `/.well-known/security.txt` | Built from the instance settings (see [Search engines and AI crawlers](crawlers.md)) | `public, max-age=3600`, ETag |

- Paths under `/api`, `/oauth`, `/auth`, `/.well-known`, `/nodeinfo`, `/ap`, `/media`, `/uploads`, `/theme` (the server theme's stylesheet) and `/licenses`, and `/robots.txt`, never return the shell; if no other route matches they return a 404 problem.
- The shell is served after ActivityPub content negotiation. Requesting a shell path with an ActivityPub `Accept` returns 404 (post pages redirect, as described below).
  The shell always carries `Vary: Accept`. When you put a CDN in front, include the kind of `Accept` (ActivityPub or HTML) in the cache key, or exclude the shell, `/ap/*` and `/@*` from caching.
  Serve only `/assets/*` from the CDN.
- A binary built without the web client (a plain `go build` that skips `scripts/task.sh go:bin`) logs a warning at start-up and serves a shell without scripts.

## Federation endpoints

A process whose `role` is `web` or `all` answers these endpoints, which other Fediverse servers use to find and display your accounts.

| Path | Content |
| --- | --- |
| `GET /.well-known/webfinger` | WebFinger (RFC 7033). `resource` is `acct:alice@{account_domain}` (`acct:alice@{host}` also works), an Actor `id`, or the `/@alice` URL. `subject` is always the handle on the `account_domain` side |
| `GET /.well-known/host-meta`, `/.well-known/host-meta.json` | The WebFinger URI template (XRD / JRD) |
| `GET /.well-known/nodeinfo`, `GET /nodeinfo/2.1` | NodeInfo 2.1. `metadata.sourceCode` is the setting `source_url`, and `software.repository` is set too for an `https:` URL (see [Publish the source code](#publish-the-source-code-agpl-30)) |
| `GET /ap/actors/{uuid}` | The Actor (`publicKey` is the RSA `#main-key`). A request that prefers HTML gets a 303 to `/@{username}` |
| `GET /ap/actors/{uuid}/{outbox,followers,following,featured,featured-tags}` | Collections |
| `POST /ap/actors/{uuid}/inbox`, `POST /ap/inbox` | The inbox of each local actor and the shared inbox. Requests must be HTTP-signed |
| `GET /@{username}` | A 302 to the Actor's `id` for an ActivityPub `Accept`; otherwise the web client's HTML (see below) |
| `GET /@{username}/status/{uuid}` | For an ActivityPub `Accept`, a 302 to the `id` (`/ap/objects/{uuid}`) of the post if a signed-out visitor can see it, and 404 if not. Otherwise the web client's HTML |

- The instance Actor's handle is `{account_domain}@{account_domain}` (the same form as Mastodon).
- An account that does not exist or cannot be shown (suspended, deleted, or remote) always gets the same 404 (`Cache-Control: no-store`).
- Anonymous responses carry `Cache-Control: public, max-age=60`. Responses that vary by `Accept` carry `Vary: Accept`.
  For a CDN that cannot handle `Vary: Accept`, exclude `/ap/*` and `/@*` from caching.

### Separate handle and delivery domains

When `account_domain` differs from `host`, requests to `/.well-known/webfinger`, `/.well-known/host-meta` and `/.well-known/nodeinfo` on the `account_domain` side are redirected to `host` with a 301 (with `Access-Control-Allow-Origin: *`).
If you route requests for `account_domain` to this server, the server returns that redirect itself.
If another server or reverse proxy answers instead, redirect to `https://{host}` keeping the path and query string.
