# Monitoring the server from outside

A guide for operators. It shows how to point an uptime monitor at the server, how to keep the monitoring from raising the running cost,
and how to tell the maintenance mode from an outage. The decisions are in [ADR-0085](../adr/0085-uptime-monitoring-status-page.md)
(and [ADR-0084](../adr/0084-maintenance-mode.md) for the maintenance mode); the probe paths are in
[the health check section of the running guide](running.md#health-checks).

Third-party facts below carry the date they were confirmed and their source. Services change their plans and screens; check the linked page before relying on a limit.

## Which URL to monitor

| Path | For | Touches the DB | In the maintenance mode |
| --- | --- | --- | --- |
| `GET /api/v1/instance/health` | Uptime monitors, status pages, the web client | At most once per 60 s per process | `503` with `Retry-After` and `{"status":"maintenance"}` |
| `GET /healthz` | The platform's liveness probe | Never | `200` `{"status":"maintenance"}` |
| `GET /readyz` | The load balancer's or Kubernetes' readiness probe | On every request (concurrent requests share one check) | `200` `{"status":"maintenance"}` |

Point the external monitor at **`/api/v1/instance/health`**, not at `/readyz`: `/readyz` queries the database on every request,
so a monitor with a short interval keeps a scale-to-zero database awake. The probes are for your platform.

Responses of `/api/v1/instance/health` (no authentication, `Cache-Control: no-store`):

| State | HTTP | Body |
| --- | --- | --- |
| Healthy | `200` | `{"status":"ok","checked_at":"<RFC 3339>"}` |
| Unavailable (the database is unreachable or the schema is not applied) | `503` | `{"status":"unavailable","checked_at":"<RFC 3339>"}` |
| Maintenance mode | `503` with `Retry-After` | `{"status":"maintenance"}` |

`/healthz` and `/readyz` answer `200` with `{"status":"ok"}` when healthy, and `/readyz` answers `503` with a problem document when the database is not ready.
The body never carries a reason, host name, or version. The cause is in the server log.

`checked_at` is the time of the check the answer is based on. The server runs the database check at most once a minute per process and reuses the result,
so a monitor that polls more often than every 60 s sees the same `checked_at` and gains nothing. With several processes, each runs its own check.

## Keep the monitoring cheap

- **Scale-to-zero databases.** If the monitor's interval is shorter than the time the database waits before it stops, the database never stops.
  Neon suspends a compute after 5 minutes without activity; the Free plan cannot turn this off
  (confirmed 2026-10-25, [Neon: Scale to zero](https://neon.com/docs/introduction/scale-to-zero)).
  As a rule of thumb, an interval of 5 minutes or more lets an idle database stop between checks, and each check wakes it for the check plus the idle time.
  A federated server receives deliveries from other servers around the clock, so its database is awake for much of the day anyway
  ([deployment targets](../architecture/deployment-targets.md)); the extra cost is limited to the quiet hours.
- **Prefer a cheaper split if idle cost matters most.** Monitor `/healthz` (it never touches the database) at a short interval, and the database-backed
  `/api/v1/instance/health` at a long one (15 minutes or more). `/healthz` answers `200` during the maintenance mode too, so match `"status":"ok"` in the body.
- **The requests themselves are small.** One check every 5 minutes is about 8,640 requests a month, every minute about 43,200. That fits in the free tier of AWS Lambda
  (1 million requests and 400,000 GB-seconds a month, then $0.20 per million requests; confirmed 2026-10-23, [AWS Lambda pricing](https://aws.amazon.com/lambda/pricing/))
  and of Cloud Run (2 million requests, 180,000 vCPU-seconds, and 360,000 GiB-seconds a month; time rounded up to 100 ms; confirmed 2026-10-23, [Cloud Run pricing](https://cloud.google.com/run/pricing)).

## Telling the maintenance mode from an outage

During the maintenance mode ([maintenance guide](maintenance.md)) the health endpoint answers `503`, which most monitors treat like an outage.
The status code alone cannot tell them apart, so decide on the body:

- Treat `"status":"ok"` in the body as healthy, and anything else as down. A keyword or JSON value monitor does this.
- To avoid alerts during planned maintenance, set the monitor's maintenance window if the service has one (see the table below),
  or pause the monitor by hand before you start.
- UptimeRobot's Free plan has no maintenance window: pause the monitor or accept the alert.

## Setting up the monitors

All of these check the same thing: `GET https://<your-host>/api/v1/instance/health` is healthy when the answer is `200` and the body contains `"status":"ok"`.

| Service | Monitor type | Healthy when | Maintenance window | Confirmed |
| --- | --- | --- | --- | --- |
| UptimeRobot | Keyword | The body contains `"status":"ok"` | Not in Free (Solo and above) | 2026-10-25, [pricing](https://uptimerobot.com/pricing.md) |
| Better Stack | HTTP status, or Keyword | `2xx`, or the body contains `"status":"ok"` | Unverified | 2026-10-25 |
| Gatus | Endpoint conditions | `[STATUS] == 200` and `[BODY].status == ok` | `maintenance` in the config | 2026-10-25, [README](https://github.com/TwiN/gatus) |
| Uptime Kuma | HTTP(s) - Keyword, or JSON Query | The body contains `"status":"ok"`, or `status` is `ok` | Built in | 2026-10-23, ADR-0085 §5 |
| Upptime | `sites` in `.upptimerc.yml` | `200` and the body contains `"status":"ok"` | None | 2026-10-25, [Configuration](https://upptime.js.org/docs/configuration/) |

### UptimeRobot

Create a monitor of type **Keyword**: URL `https://<your-host>/api/v1/instance/health`, keyword `"status":"ok"`, alert when the keyword does **not** exist.
The Free plan allows 50 monitors at an interval of 5 minutes, and includes keyword monitors.
"Custom headers & statuses" and maintenance windows start at the Solo plan (paid, 60-second interval)
(confirmed 2026-10-25, [pricing](https://uptimerobot.com/pricing.md)). A keyword monitor is enough here, because the check is on the body.

### Better Stack

Two forms work (confirmed 2026-10-25, [Uptime monitor](https://betterstack.com/docs/uptime/uptime-monitor/), [Keyword monitor](https://betterstack.com/docs/uptime/keyword-monitor/)):

- An **HTTP status** monitor: it treats a `2xx` answer as up, so `200` is up and `503` (unavailable or maintenance) opens an incident.
- A **Keyword** monitor: "Alert us when" is "URL doesn't contain keyword", keyword `"status":"ok"`. The match is case-insensitive.

The check frequency and the maintenance windows of the Free plan are unverified: the official pages do not state them for the Free plan
(open question in ADR-0085). The free tier's counts (10 monitors and heartbeats together, one status page) are from the [pricing page](https://betterstack.com/pricing), confirmed 2026-10-23.

### Gatus

In `config.yaml` (default interval `60s`; confirmed 2026-10-25, [README](https://github.com/TwiN/gatus)):

```yaml
endpoints:
  - name: my
    url: "https://<your-host>/api/v1/instance/health"
    interval: 5m
    conditions:
      - "[STATUS] == 200"
      - "[BODY].status == ok"
```

Gatus has a top-level `maintenance` section (`start` as `hh:mm`, `duration`, `timezone` defaulting to `UTC`, `every` as a list of weekdays, empty meaning every day)
for planned work. The README only says it is for avoiding alerts during maintenance windows; check the behavior on your version.

### Uptime Kuma

Add a monitor of type **HTTP(s) - Keyword** with the keyword `"status":"ok"`, or **HTTP(s) - Json Query** with the query `status` and the expected value `ok`.
The accepted status codes default to `200-299`, so the maintenance `503` and the outage `503` are both down. Uptime Kuma has a maintenance feature
for monitor windows ([wiki](https://github.com/louislam/uptime-kuma/wiki/Monitor-Types) lists it; its details are unverified).
The monitor types were confirmed 2026-10-23 in the source ([monitor.js](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/server/model/monitor.js#L478-L720),
[EditMonitor.vue](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/src/pages/EditMonitor.vue#L3349)).

### Upptime

In `.upptimerc.yml` (confirmed 2026-10-25, [Configuration](https://upptime.js.org/docs/configuration/)):

```yaml
sites:
  - name: My server
    url: https://<your-host>/api/v1/instance/health
    __dangerous__body_down_if_text_missing: '"status":"ok"'
```

By default `20x` and `30x` count as up; set `expectedStatusCodes: [200]` to accept only `200`. The check runs from GitHub Actions on a schedule set for all sites
(`workflowSchedule.uptime`, default every 5 minutes, which is also the shortest GitHub Actions allows). The option name says it is not recommended,
but it is the way to test the body.

## A status page

Set `status_page_url` (`MY_STATUS_PAGE_URL`, an `https:` URL; see the [configuration reference](../reference/configuration.md#status_page_url))
to the public page of your monitor. The server shows the link in `GET /api/v1/instance`, in the sign-in and error pages it renders, in the web client
(also when the server does not answer), and on the maintenance page, so a user who cannot reach the server can find out why.
It is read from the configuration, not from the database, so it works when the database is down. A change takes effect after a restart.

Options for the page itself (this server does not import a status page's state, so also post announcements as a server-wide notice):

- The hosted status page of the monitoring service you chose (UptimeRobot's Free plan has one status page without customization; Better Stack's free tier has one),
  the pages that Gatus and Uptime Kuma serve themselves, or Upptime's static site on GitHub Pages.
- Atlassian Statuspage, which is a place to publish and not a monitor. A component has one of five states: Operational, Under maintenance, Degraded performance,
  Partial outage, Major outage (API values `operational`, `under_maintenance`, `degraded_performance`, `partial_outage`, `major_outage`;
  confirmed 2026-10-23, [What is a component?](https://support.atlassian.com/statuspage/docs/what-is-a-component/), [Statuspage API](https://developer.statuspage.io/)).
  How to change a component's state automatically from a monitor's alert is unverified (it needs a per-service integration set up on an actual account).
