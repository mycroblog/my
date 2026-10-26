# Alerts and runbook

A guide for operators. It lists what the server's own metrics should alert you about, how urgent each alert is, and what to check
and do when one fires. Watching the server from outside (uptime monitors and a status page) is in [Monitoring the server from outside](monitoring.md);
set that up too, because no metric arrives from a server that is down.

## Getting the metrics

The server exports its metrics when `metrics_exporter` is set ([configuration reference](../reference/configuration.md#metrics_exporter));
the [running guide](running.md) lists every metric and its labels. The alerts below are written for Prometheus, and the names are
the ones Prometheus sees: dots become underscores, and the unit and counter suffixes are added
(`my.queue.oldest_ready_age` is `my_queue_oldest_ready_age_seconds`, `my.delivery.attempts` is `my_delivery_attempts_total`).

| Target | Setting | Alerting |
| --- | --- | --- |
| Self-hosted (Docker Compose, Kubernetes, a VM) | `metrics_exporter = prometheus` | Prometheus scrapes `GET /metrics` on `metrics_listen`, Alertmanager routes the alerts. The rules below work as they are |
| Any target with an OpenTelemetry Collector | `metrics_exporter = otlp` | The Collector forwards to the backend of your choice. A Prometheus-compatible backend that adds the unit and type suffixes (as Prometheus' own OTLP receiver does by default) sees the same names |
| Google Cloud (Cloud Run) | `metrics_exporter = none` | Cloud Monitoring alerting policies on the platform metrics and on log-based metrics (see [Cloud Run](#google-cloud-cloud-run)) |
| AWS (Lambda) | `metrics_exporter = emf` | CloudWatch alarms on the metrics that CloudWatch Logs extracts (see [AWS Lambda](#aws-lambda)) |

### Prometheus and Alertmanager

The rules are in [`deploy/monitoring/prometheus/alerts.yml`](../../deploy/monitoring/prometheus/alerts.yml). Copy the file next to your
`prometheus.yml` and load it:

```yaml
rule_files:
  - alerts.yml

scrape_configs:
  - job_name: my          # the rules expect this job name
    static_configs:
      - targets: ["my:9464"]
```

- `metrics_listen` defaults to `127.0.0.1:9464`, which only a Prometheus on the same host reaches. In Docker Compose or Kubernetes, set
  `MY_METRICS_LISTEN=0.0.0.0:9464` and keep the port on the private network: the listener has no authentication.
- Scrape every process (each `role = web` and `role = worker` process has its own listener). The queue, media, database size,
  remote server, billing import, and snapshot gauges carry the same instance-wide value in every process, so the rules take their maximum.
- Every rule has a `severity` label: **`page`** needs someone now (users are affected or soon will be), **`ticket`** can wait for working hours.
  Route them to different receivers in Alertmanager. Each rule's `runbook_url` points to its section below.
- The thresholds are starting points for a small server. Watch the values for a week and tune the thresholds and the `for` durations to your traffic.
  `MyRemoteMediaCacheOverLimit`, `MyMediaStorageLarge`, and `MyDatabaseStorageLarge` carry sizes you must set to your own limits.
- During the maintenance mode the server serves no metrics and answers `503`, so `MyTargetDown` and the HTTP alerts fire.
  Silence the alerts in Alertmanager for the window before you start ([Maintenance](maintenance.md#steps)).

`scripts/task.sh docker:alert-rules` checks the file and runs its unit tests (`alerts.test.yml`) with `promtool`. Run it after you change the rules.

### Google Cloud (Cloud Run)

With `metrics_exporter = none`, build the alerts from what Cloud Run and Cloud Logging already have:

- **HTTP errors and latency**: the Cloud Run request count by response code class and the request latency metric of the service.
- **Jobs and requests**: the server writes one log line per HTTP request (`msg` = `http request`) and per job run (`msg` = `job`), with the
  same labels as the metrics. Make log-based metrics from them, for example a counter of `jsonPayload.msg="job" AND jsonPayload."my.outcome"="dead"`
  for failed jobs, or a distribution of `jsonPayload.duration_seconds` filtered on `jsonPayload."my.surface"="client_api"` for the latency.
- **Queue backlog**: with `queue = cloudtasks`, the queue depth metric of Cloud Tasks. The job screen cannot list pending jobs of that queue.
- **Database**: the disk utilization and connection metrics of Cloud SQL, or the usage alerts of your database provider.
- **Cost**: a Cloud Billing budget with alert thresholds ([Cost and budget](#cost-and-budget)).

The deployment templates of each target will carry ready-made policies; until then, recreate the alerts in the table below by hand.

### AWS Lambda

With `metrics_exporter = emf`, CloudWatch Logs turns the server's lines into metrics in the namespace `metrics_emf_namespace`, with the dotted
names (`my.job.duration`, `my.queue.oldest_ready_age`). The request and job duration metrics come from the request and job log lines, and the
dimensions are the labels you list in `metrics_emf_dimensions`; include `my.outcome` to alarm on failed jobs and `http.response.status_code`
to alarm on 5xx. The instance-wide gauges (queue, media, database size, remote servers, billing imports, snapshot) are written by the periodic snapshot job. The Go runtime metrics (`go.*`)
are not written in EMF; use the Lambda memory and error metrics instead.

- **Queue backlog**: with `queue = sqs`, the SQS metric `ApproximateAgeOfOldestMessage` of the job queues.
- **Database**: the storage and connection metrics of RDS or Aurora, or the usage alerts of your database provider.
- **Email** (`mail = ses`): the SES reputation metrics (bounce and complaint rates) in CloudWatch.
- **Cost**: an AWS Budgets budget with alerts ([Cost and budget](#cost-and-budget)).

## The alerts

Severity: **page** = act now, **ticket** = during working hours. "–" in the rule column means there is no metric for it yet; check it as described.

| Area | Alert | Signal | Threshold | Severity | Likely causes |
| --- | --- | --- | --- | --- | --- |
| Queue | [`MyQueueBacklog`](#myqueuebacklog) | `my_queue_oldest_ready_age_seconds` | over 5 min for 10 min | ticket | Too few workers for a burst (a popular post, a relay), slow jobs, a worker that stopped |
| Queue | [`MyQueueStalled`](#myqueuestalled) | `my_queue_oldest_ready_age_seconds` | over 30 min for 5 min | page | No worker runs (crashed, not deployed, `role = web` only), workers stuck on the database |
| Queue | [`MyDeadJobsIncreasing`](#mydeadjobsincreasing) | `delta(my_queue_dead[1h])` | over 10 an hour for 15 min | ticket | A bug in a job, a misconfigured adapter (mail, blob, billing), a remote service that refuses |
| Queue | [`MyMetricsSnapshotStale`](#mymetricssnapshotstale) | `my_metrics_snapshot_age_seconds` | queue part over 5 min, totals part over 1 h, for 15 min | ticket | The KV store is out of reach, the database queries of the snapshot fail or time out |
| Federation | [`MyDeliveryFailuresHigh`](#mydeliveryfailureshigh) | `my_delivery_attempts_total` with `my_outcome` `retry` or `failed` | over 50% of attempts for 30 min | ticket | Many followers on servers that are down, a large server is down, outbound network trouble |
| Federation | [`MyDeliveryAllFailing`](#mydeliveryallfailing) | same | over 95% of attempts for 15 min | page | Outbound network or DNS broken, egress blocked, the server's keys or `host` wrong |
| Federation | [`MyRemoteServersPaused`](#myremoteserverspaused) | `my_federation_instances{my_federation_instance_state="unavailable"}` | 5 more than a day before | ticket | Servers that shut down or block this one, a network problem on this side that lasted for days |
| Federation | [`MyInboxRejectionsHigh`](#myinboxrejectionshigh) | `my_inbox_activities_total{my_outcome="rejected"}` | over 25% of POSTs for 30 min | ticket | Signature verification failing (clock skew, key fetches failing), a spammer or a suspended domain retrying, rate limits |
| Federation | [`MyInboxSignatureFailures`](#myinboxsignaturefailures) | `my_inbox_activities_total{my_inbox_rejection="signature"}` | over 10% of POSTs for 30 min | ticket | The server's clock is off, fetching the senders' keys fails, a server that changed its keys |
| Federation | [`MyInboxErrors`](#myinboxerrors) | `my_inbox_activities_total{my_outcome="error"}` | over 5% of POSTs for 15 min | ticket | The database failing, a bug in the inbox |
| HTTP | [`MyHTTPErrorsHigh`](#myhttperrorshigh) | 5xx of `http_server_request_duration_seconds_count` | over 5% for 10 min | page | The database or KV down, a bad release, the maintenance mode |
| HTTP | [`MyHTTPErrorsElevated`](#myhttperrorselevated) | same | over 1% for 30 min | ticket | One failing route, an adapter that fails now and then |
| HTTP | [`MyAPILatencyHigh`](#myapilatencyhigh) | p95 of the client API (streaming left out) | over 300 ms for 30 min | ticket | Database saturated or far away, a slow query, too little CPU, cold starts |
| Resources | [`MyTargetDown`](#mytargetdown) | `up{job="my"}` | 0 for 5 min | page | The process is down, crash-looping, or in the maintenance mode; the scrape path is broken |
| Resources | [`MyNotReady`](#mynotready) | `my_readiness_failures_total` | any increase for 5 min | page | The database is out of reach, or the schema is not applied |
| Resources | [`MyProcessRestarting`](#myprocessrestarting) | `resets(process_uptime_seconds[1h])` | 3 restarts in an hour | ticket | Killed for memory (OOM), a panic, a failing liveness probe |
| Resources | [`MyMemoryNearLimit`](#mymemorynearlimit) | `go_memory_used_bytes` / `go_memory_limit_bytes` | over 90% for 15 min | ticket | `GOMEMLIMIT` too low for the load, many open streams, a leak |
| Resources | [`MyDBPoolSaturated`](#mydbpoolsaturated) | `db_client_connection_count{db_client_connection_state="used"}` / `db_client_connection_max` | over 90% for 15 min | ticket | Pool too small for the traffic, slow queries holding connections |
| Resources | [`MyDBPoolWaiting`](#mydbpoolwaiting) | `db_client_connection_wait_time_seconds_total`, `db_client_connection_timeouts_total` | waiting over 0.5 s per second, or any timeout, for 10 min | ticket | Same as above; the database's own connection limit reached |
| Resources | [`MyDatabaseStorageLarge`](#mydatabasestoragelarge) | `my_db_size_bytes` | over your plan's limit less a margin (example 8 GiB) for 1 h | ticket | Growth of posts and cached remote content; time to raise the plan or shorten the retention |
| Resources | [`MyFileDescriptorsNearLimit`](#myfiledescriptorsnearlimit) | `process_unix_file_descriptor_count` / `my_process_file_descriptor_limit` | over 80% for 15 min | ticket | Many open streams and connections, a low `ulimit -n` |
| Resources | [`MyAdapterErrors`](#myadaptererrors) | `my_adapter_operation_duration_seconds_count{my_outcome="error"}` | over 5% of one adapter's operations for 15 min | ticket | Valkey, the blob directory, or the stream backend failing |
| Resources | [`MyRemoteMediaCacheOverLimit`](#myremotemediacacheoverlimit) | `my_media_stored_bytes{my_media_origin="remote_cache"}` | over 1.2 × `remote_media_cache_max_bytes` for 3 h | ticket | The hourly cleanup does not run or fails |
| Resources | [`MyMediaStorageLarge`](#mymediastoragelarge) | `my_media_stored_bytes` | over your planned size (example 50 GiB) for 1 h | ticket | Growth of uploads; time to raise the plan or the budget |
| Email | [`MyEmailFailing`](#myemailfailing) | `my_job_duration_seconds_count` of the email jobs with `my_outcome="dead"` | any in an hour | ticket | Wrong SMTP or SES settings, an account suspended or out of quota, addresses refused for good |
| Email | [`MyEmailRetrying`](#myemailretrying) | same, `my_outcome="retry"` | over 50% of runs for 15 min | ticket | The mail server is out of reach or refuses the login, a sending rate limit |
| Cost | [`MyBillingImportFailing`](#mybillingimportfailing) | `my_billing_import_failing` | 1 for 6 h | ticket | Missing permissions, the billing export not set up, the request cap reached, the provider unavailable |
| Cost | Budget exceeded | – | – | – | Traffic, storage, or a misconfiguration costing more than planned ([below](#cost-and-budget)) |

## Where to look

These tools come back in most of the runbook entries below.

- **The job screen** (Administration → Jobs, `/admin/jobs`, needs `manage_jobs`): jobs by state (queued, running, retrying, failed) and type, the counts,
  the oldest waiting job, and for each job its last error and the attempts that did not succeed. Pending jobs are listed only for `queue = db`;
  failed jobs and attempts are listed for every queue.
- **The `my job` command** on a host that has the server's configuration (`queue = db` only):
  `my job list` lists the failed jobs, newest first; `my job retry <id>...` puts them back in the queue; `my job delete <id>...` drops them.
  Jobs are safe to run again.
- **The remote servers screen** (Moderation → Remote servers, `/moderation/instances`, needs `manage_domains`): the servers that deliveries failed to reach,
  and those whose deliveries are paused, with why and when. "Resume deliveries" marks a server available again.
- **The logs**: JSON lines on standard error. Each HTTP request ends with a line whose `msg` is `http request`, each job run with a line whose `msg` is `job`
  (`INFO` for success, `WARN` for a retry, `ERROR` for a job that failed for good, with the `error`, `job_id`, and `my.job.type`).
  Lines inside a request carry its `request_id`, which the client sees as `X-Request-Id`.
  - Docker Compose: `docker compose logs my | jq -c 'select(.msg == "job" and ."my.outcome" == "dead")'`
  - Cloud Logging: `jsonPayload.msg="job" AND jsonPayload."my.outcome"="dead"`
  - CloudWatch Logs Insights: ``filter msg = "job" and `my.outcome` = "dead"``
- **Traces**, when `traces_exporter = otlp`: the log lines carry `trace_id`; open it in your trace backend to see where a slow request spent its time.
- **The maintenance mode** ([Maintenance](maintenance.md)): answers every request with a maintenance page and touches no adapter. Use it while you repair the database
  or the KV store.

## Runbook

### MyQueueBacklog

Jobs (deliveries, notifications, emails, timeline fan-out) wait more than 5 minutes for a worker.

Check:

- The job screen: which types are queued, and whether many are retrying. A burst of one type (`federation.deliver` after a post with many followers,
  relay activities) clears by itself; check that the age goes down.
- That the worker processes run (`role = all` or `role = worker`) and their logs show `job` lines.
- `MyDBPoolSaturated` or `MyAPILatencyHigh` at the same time: the workers wait on the database.

Fix:

- Run more worker processes (`role = worker`), or give the existing ones more CPU.
- If one job type is slow, find it in the `job` log lines (`duration_seconds`) and look for errors of that type.

### MyQueueStalled

The oldest ready job has waited over 30 minutes: nothing is being processed, or almost nothing.

Check:

- That at least one process runs with `role = all` or `role = worker`, and that it is not crash-looping (`MyProcessRestarting`).
- The worker's logs for errors that repeat on every job (database, KV, a panic).
- `MyNotReady`: a worker that cannot reach the database cannot take jobs.

Fix:

- Start or restart the workers. Jobs that a stopped worker held go back to the queue when their lease runs out; they are safe to run again.
- If the database is the cause, follow [MyNotReady](#mynotready).

### MyDeadJobsIncreasing

More than 10 jobs an hour fail for good: they ran out of retries, or the job refused to run again.

Check:

- The job screen, filtered on Failed: the types and the last error. The attempts of a job show every error on the way.
- The same errors in the `job` log lines with `my.outcome` = `dead`.

Fix:

- A configuration error (mail, blob, billing import, a webhook destination): fix it, then run the jobs again from the job screen ("Run again") or with `my job retry <id>...`.
- Deliveries to servers that are gone fail for good after about 4 days of retries; that is expected and needs nothing.
- Failed jobs that nobody retries are removed after `dead_job_retention` (30 days by default). Delete the ones you will not retry with `my job delete`.
- An error that looks like a bug: report it with the job type and the error (it holds no personal data).

### MyMetricsSnapshotStale

The queue and storage gauges come from one snapshot that the processes share through the KV store. It is old, so the queue and storage alerts
see old values and may stay silent.

Check:

- The logs for errors about the metrics snapshot or the KV store.
- `MyAdapterErrors` for `kv.valkey` or `kv.db`, and the database health.

Fix:

- Repair the KV store or the database connection. The snapshot is computed again on the next collection; the periodic snapshot job also runs every 15 minutes.

### MyDeliveryFailuresHigh

More than half of the delivery attempts to other servers fail. Some failures are normal: servers go away, and the server keeps retrying them for about 4 days.

Check:

- The remote servers screen: whether the failures concentrate on a few servers (they are down; nothing to do) or are spread over many (suspect this server).
- `http_client_request_duration_seconds_count` by `error_type`: timeouts, DNS errors, or `forbidden_destination` (the SSRF guard refused the destination).

Fix:

- Failures on a few servers: none. After failures on 7 different days, deliveries to that server are paused, and the server is probed daily to resume them.
- Failures everywhere: follow [MyDeliveryAllFailing](#mydeliveryallfailing).

### MyDeliveryAllFailing

Almost every delivery fails. This is nearly always this server's side.

Check:

- From the server's host or container, that outbound HTTPS works (DNS, egress firewall, NAT, a proxy).
- The `job` log lines of `federation.deliver` for the error: TLS errors, timeouts, or `401` from many servers (they cannot verify this server's signature).
- After a domain or key change: `host` must stay the same once the server has federated, and the KEK must be the one the keys were sealed with
  ([KEK management](key-management.md)).

Fix:

- Repair the outbound network. Failed attempts are retried with backoff (1 minute, 5 minutes, 30 minutes, 2 hours, and so on), so deliveries catch up by themselves.
- If deliveries ran out of retries, run the failed `federation.deliver` jobs again from the job screen.
- If remote servers were paused during the outage, they resume on the next successful delivery, signed request, or the daily probe; you can also press
  "Resume deliveries" on the remote servers screen.

### MyRemoteServersPaused

Deliveries to 5 or more servers were paused in the last day. A server is paused after deliveries to it failed on 7 different days, so a few pauses a week
are normal: servers shut down. Many at once mean that many servers failed for days at the same time. `my_federation_instances` with
`my_federation_instance_state` = `available` counts the servers that are still delivered to but failed on some day; a jump there comes before the pauses.

The metric has no per-server label (labels never carry a remote domain). Check:

- The remote servers screen: the "Paused" tab lists the paused servers, with when and why. Whether they share something (one hosting provider, one software)
  or are spread out.
- `MyDeliveryFailuresHigh` and `MyDeliveryAllFailing` over the last week: a network problem on this side that lasted for days pauses many servers.

Fix:

- Servers that are gone: nothing. Paused servers are probed daily and resume by themselves when they answer.
- After a problem on this side: repair it, then press "Resume deliveries" for the servers you care about, or wait for the daily probe.
- A server whose operator reports that they block this one: nothing on this side helps. To stop federating with a server on purpose, use a domain action
  (Moderation → Domain actions).

### MyInboxRejectionsHigh

More than a quarter of the activities other servers POST to the inboxes are refused.

Check:

- `my_inbox_activities_total{my_outcome="rejected"}` by `my_inbox_rejection`: `signature` (follow [MyInboxSignatureFailures](#myinboxsignaturefailures)),
  `suspended` (a suspended domain that keeps retrying), `federation_off` (federation is turned off), `rate_limited` (a client IP or a server over the
  rate limit), or `invalid` (activities without a valid `id`, `type`, `actor`, or `object`).
- The `http request` log lines of the inbox routes (`my.surface` = `activitypub`) carry the same split by `http.response.status_code`
  (`401`, `403`, `429`, `400`) and the client's address, when you need to know where the requests come from.

Fix:

- Refusals of a suspended domain, or with federation turned off, need nothing: they are refused as intended.
- Rate-limited requests from one server: a spammer or a burst; suspend the domain if it keeps on.
- Invalid activities from one server: that server's software sends something this server does not read; report it with the server's software and version.

### MyInboxSignatureFailures

More than 10% of the activities other servers POST to the inboxes are refused because their signature or integrity proof is missing or does not verify,
or because the sender's key could not be fetched.

Check:

- The server's clock: signatures are refused outside a time window. Check NTP on the host or the container runtime.
- Fetching the senders' keys: `MyDeliveryAllFailing` and `http_client_request_duration_seconds_count{my_http_purpose="json"}` by `error_type`
  (the outbound network, DNS, the SSRF guard).
- Whether the refusals come from many servers or one (the `http request` lines with status `401`): one server that changed its keys, or that signs
  in a way this server does not accept, affects only that server.

Fix:

- Fix the clock or the outbound network. Other servers retry their deliveries, so nothing is lost if it is fixed within a day or so.
- One server that fails: report it to that server's operator, or to this project with the server's software and version.

### MyInboxErrors

The inbox answers `500` to more than 5% of the activities. The senders retry, but activities are late.

**Check** the `http request` and error log lines of the inbox routes for the error, and the database health (`MyNotReady`, `MyDBPoolWaiting`).

**Fix** the database or report the error as a bug. Nothing needs replaying: the senders retry.

### MyHTTPErrorsHigh

More than 5% of all requests answer 5xx. Users see errors.

Check:

- Whether you are in the maintenance mode (expected `503`; silence the alert next time).
- `MyNotReady` (the database), `MyAdapterErrors` (KV, blob, stream), and a recent release.
- The `http request` lines with `http.response.status_code` ≥ 500 by `http.route`, and the error lines with the same `request_id`.

Fix:

- Repair the failing dependency. If the database needs work, switch to the maintenance mode so users see a clear page instead of errors.
- A bad release: roll back to the previous image.
- Tell users ([Telling users about an incident](#telling-users-about-an-incident)) if it lasts.

### MyHTTPErrorsElevated

More than 1% of requests answer 5xx over half an hour. Usually one route or one adapter.

**Check** the `http request` lines with a 5xx by `http.route`, and the error lines of those requests. **Fix** the dependency, or report the error as a bug.

### MyAPILatencyHigh

The 95th percentile of the client API is over 300 ms (the target for a home timeline page of 20 posts, warm). The streaming endpoint is left out.

Check:

- `http_server_request_duration_seconds` by `http_route`: one slow route, or all of them.
- The database: `MyDBPoolWaiting`, the database's CPU and slow query log, the network distance between the server and the database.
- The process: CPU throttling, `MyMemoryNearLimit` (a GC under memory pressure), `go_schedule_duration_seconds` (goroutines waiting for a CPU).
- Traces of slow requests, if enabled.

Fix:

- Give the database or the server more resources, or place them closer.
- Cold starts after scale to zero raise the percentile on a quiet server; that is expected.

### MyTargetDown

Prometheus cannot scrape a process.

Check:

- That the process runs: `docker compose ps`, `kubectl get pods`, or the platform's console. Its last log lines tell why it stopped.
- Whether it is in the maintenance mode (no metrics are served; expected).
- That `metrics_exporter = prometheus` and `metrics_listen` is reachable from Prometheus.

Fix:

- Start the process again. If it does not start, the log says why: a configuration error (`my config check` shows it), the database out of reach, a schema not applied.

### MyNotReady

The readiness check fails: the database is out of reach, or its tables do not match this version. The load balancer stops sending traffic to the process,
and the external health endpoint reports `unavailable`.

Check:

- The log lines of the readiness check: a connection error (network, credentials, the database down or out of connections), or the missing tables.
- The database provider's status page and console: is it up, out of storage, or in maintenance?
- After an upgrade: was the schema applied before the new version started ([Initializing the database](database.md))?

Fix:

- Database down or unreachable: restore it, or fail over. While you work, the maintenance mode shows users a clear page.
- Database out of storage: raise the plan or free space ([MyDatabaseStorageLarge](#mydatabasestoragelarge)).
- Schema not applied: apply it with the schema image of the same version, then the processes become ready by themselves.
- Data lost or corrupted: restore from your backup, and keep the KEK that the backup's keys were sealed with ([KEK management](key-management.md)).

### MyProcessRestarting

A process restarted 3 times in an hour.

Check:

- The platform's reason for the restart: `OOMKilled` (memory), a failed liveness probe, or the exit code.
- The last log lines before each restart, for a panic or a fatal configuration error.

Fix:

- Out of memory: set `GOMEMLIMIT` to about 90% of the container's memory limit, or give the container more memory ([MyMemoryNearLimit](#mymemorynearlimit)).
- A panic: report it with the log lines around it.

### MyMemoryNearLimit

The Go runtime uses more than 90% of `GOMEMLIMIT`. Close to the limit, the garbage collector runs much more often, which costs CPU and latency,
and past it the platform may kill the process. This alert needs `GOMEMLIMIT`; without it, watch the container's memory metrics instead.

**Check** `http_server_active_requests` and the open streams (`my_stream_connections`), and whether the memory keeps growing while the load is flat (a leak).

**Fix** raise the container's memory and `GOMEMLIMIT` with it, or run more processes. Report a steady growth that does not follow the load.

### MyDBPoolSaturated

More than 90% of a process's database connections are in use. Requests soon wait for a connection (`MyDBPoolWaiting`).

**Check** whether the traffic grew or queries got slower (the database's slow query log, `my_adapter_operation_duration_seconds`).

Fix:

- Raise the pool size in `database_url` (for PostgreSQL, `pool_max_conns`), keeping the total of all processes under the database's connection limit.
- Put a connection pooler in front of the database if many processes share it.

### MyDBPoolWaiting

Work waits for a free database connection, or gave up waiting (timeouts are PostgreSQL only). Follow [MyDBPoolSaturated](#mydbpoolsaturated);
also check that the database itself is not at its connection limit.

### MyDatabaseStorageLarge

The database takes more than you planned for. `my_db_size_bytes` is the size of the PostgreSQL database (`pg_database_size`, what the providers' storage
limits count) or of the SQLite file (its write-ahead log left out). This is growth, not a failure. Set the threshold in the rule a margin below the storage
limit of your database plan, or of the volume that holds the SQLite file, which must also keep room for the write-ahead log and for backups.

Fix:

- Raise the plan or the volume.
- Remote posts are removed after `remote_status_retention`; lowering it slows the growth.
- Deleted rows free space that both databases reuse, but the size rarely goes down: PostgreSQL keeps the space of its tables, and SQLite keeps the free
  pages in the file until a `VACUUM` (run with `sqlite3` while the server is stopped).

### MyFileDescriptorsNearLimit

The process has more than 80% of its file descriptor limit open. Past it, new connections fail.

**Fix** raise the limit (`ulimit -n`, the container runtime's `nofile`), or run more processes. Check `my_stream_connections` and
`http_client_open_connections` for what holds them.

### MyAdapterErrors

More than 5% of the operations of one adapter fail: `kv.valkey` (Valkey), `kv.db` (the KV table), `blob.fs` (the media directory),
`queue.db` (the queue table), or a `stream.<backend>` (the notifications between processes).

**Check** the error lines of that adapter, and the service behind it: Valkey up and reachable, the media volume mounted, writable, and not full.

**Fix** the service. Failed job operations are retried; failed stream notifications only delay live updates.

### MyRemoteMediaCacheOverLimit

The copies of remote media stay over `remote_media_cache_max_bytes`. The hourly cleanup normally removes expired and least recently used copies.
Set the threshold in the rule to your value of `remote_media_cache_max_bytes`.

**Check** the job screen and logs for failures of the cleanup jobs, and `MyAdapterErrors` for the blob storage.

**Fix** the cause of the failures, or lower the cache size or the instance's remote media policy.

### MyMediaStorageLarge

The media files take more than you planned for. This is growth, not a failure. Set the threshold in the rule to what your storage plan or budget allows.

**Fix** raise the plan or the budget, or lower the upload limits (`media_max_image_bytes`, `media_max_av_bytes`).

### MyEmailFailing

Email jobs failed for good in the last hour: verification, password reset, password notices, notifications, or the closure emails.
Users who wait for a verification or reset email cannot finish.

**Check** the job screen filtered on the email job types (they end in `_email`), and the last errors. [Sending email](email.md#look-at-send-failures)
lists the errors and what they mean.

Fix:

- Wrong SMTP host, port, or credentials, or an SES account or permission problem: fix it, then run the failed jobs again.
  Verification and reset emails older than their link's lifetime complete without sending when retried, so ask those users to request a new one.
- Addresses refused for good are suppressed; nothing to do.

### MyEmailRetrying

Most email sends fail and wait for a retry: the mail server is out of reach, refuses the login, or limits the rate. Retries run for about 15 minutes for
the transactional emails and about 4 hours for notifications before they fail for good. Follow [MyEmailFailing](#myemailfailing) before they do.

### Cost and budget

The server does not alert on cost. Set a budget with alerts at the provider, which sees the charges first:

- AWS: an AWS Budgets cost budget with alerts at, for example, 80% and 100% of the monthly amount, filtered by the cost allocation tag of this server.
- Google Cloud: a Cloud Billing budget for the project, with alert thresholds at 50%, 90%, and 100%. The same budget's notifications can feed the
  cost import ([Importing cloud usage charges](billing-import.md)).

The imported charges and the month-to-date estimate appear in Administration → Costs. Failed imports are recorded there and retried by the next daily import;
[`MyBillingImportFailing`](#mybillingimportfailing) tells you before the month's figures go stale.

### MyBillingImportFailing

The newest import of a billing source's usage charges failed, and no later import has succeeded for 6 hours. Imports run once a day; throttling and
unavailability are retried within hours, and the other failures wait for the next day's import, so the costs screen shows older figures until then.

Check:

- Administration → Costs: the last import of the source and its error.
- `my_billing_imports_total{my_outcome="failed"}` by `error_type`, or the `WARN` log line whose `msg` is `billing import failed`, with the `source` and the
  `error_kind`. [Importing cloud usage charges](billing-import.md) lists the kinds and how to fix each one.

Fix:

- Fix the cause (permissions, the billing export, the request cap), then press "Import now" on the costs screen instead of waiting for the next day.

Without Prometheus (Cloud Run with `metrics_exporter = none`), make a log-based alert on the `billing import failed` line instead.

## Telling users about an incident

1. **Acknowledge** the alert so others on call know someone is on it.
2. **Post on the status page** (`status_page_url`, see [Monitoring the server from outside](monitoring.md#a-status-page)) as soon as users are affected:
   what is broken, from when, and when you will post again. The status page stays reachable when this server is not, and the web client links to it.
3. **Post an announcement** if users can still reach the server (degraded, not down): Administration → Announcements, write the text and publish it.
   Announcements show above everyone's home timeline. Use the maintenance mode with a `maintenance_message` instead if the server must stop.
4. **Update** both at the time you promised, even if nothing changed.
5. **Close**: mark the incident resolved on the status page, withdraw the announcement (or publish a short follow-up), and write down what happened
   and what you changed while it is fresh.
