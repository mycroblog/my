# Documentation

The documents for users, operators, and developers are published at [mycroblog.dev](https://mycroblog.dev/latest/),
one version per release (`/latest/` for the newest, `/v0.1/` and the like) and `/next/` for `main`; the client API
reference is there too. The contributing and design documents below are read here.

## Using my

- [Signing up and signing in](guide/signing-in.md)
- [Posting and choosing who sees a post](guide/posting.md) (public, this server, unlisted, followers, and mentioned people)
- [Following people and reading timelines](guide/following.md)
- [Notifications](guide/notifications.md)
- [Blocking, muting, and reporting](guide/safety.md)
- [Moving your account](guide/moving-your-account.md)
- [Voice chat](guide/voice-chat.md)

## Operation

- [Administrators and roles](operations/admin-cli.md)
- [Instance settings](operations/instance-settings.md)
- [Registration (registration modes, invitations, approval, email verification)](operations/registration.md)
- [External login providers (OpenID Connect, GitHub, Mastodon)](operations/login-providers.md)
- [Sending email (SMTP settings, SPF/DKIM/DMARC, failures and suppressions, unsubscribing)](operations/email.md)
- [CAPTCHA against bots (Turnstile / hCaptcha / reCAPTCHA)](operations/captcha.md)
- [Domain blocks and handling reports](operations/moderation.md)
- [Managing the key encryption key (KEK)](operations/key-management.md)
- [Database: initializing, upgrading, and recreating](operations/database.md)
- [Backup and restore (database, media, KEK, configuration, and the recovery drill)](operations/backup.md)
- [Running the server: starting, stopping, and health checks](operations/running.md)
- [Deploying on Cloud Run](operations/deploy-cloud-run.md)
- [Serving media through a CDN, and refusing requests that did not come through it (media_origin, origin pull, caching and purging, origin_secret)](operations/cdn.md)
- [Maintenance (the maintenance mode and the exported maintenance page)](operations/maintenance.md)
- [Production checklist (what to verify before going live, and what my config check checks)](operations/production-checklist.md)
- [Closing the service (announcement, closure, the static closure page, deleting the data)](operations/service-closure.md)
- [Privacy policy template (what the server keeps, why, for how long, and who receives it)](operations/privacy-policy-template.md)
- [Record of processing activities template (GDPR Article 30)](operations/processing-record-template.md)
- [External monitoring and status pages (UptimeRobot, Better Stack, Gatus, Uptime Kuma, Upptime)](operations/monitoring.md)
- [Alerts and runbook (Prometheus alerting rules, what to do when an alert fires, telling users about an incident)](operations/alerts.md)
- [Error reporting (the error report log line, error groups and notifications, sending reports to Sentry, GlitchTip, Datadog or OpenTelemetry, scrubbing)](operations/error-reporting.md)
- [Link previews (OGP / X Cards / fediverse:creator)](operations/link-previews.md)
- [Search engines and AI crawlers (robots.txt, noindex, opting out of AI)](operations/crawlers.md)
- [Full-text search of posts (what the index holds, rebuilding it, the query in URLs and logs, its backups)](operations/search.md)
- [Server webhooks](operations/webhooks.md)
- [Federation relays](operations/relays.md)
- [Importing cloud usage costs (AWS Cost Explorer, Google Cloud's BigQuery export, a plugin)](operations/billing-import.md)
- [Supporter payments with Stripe](operations/payments-stripe.md)
- [Supporter payments with PAY.JP](operations/payments-payjp.md)
- [Supporters (paid plans)](operations/supporters.md)
- [Voice chat (choosing P2P, LiveKit or Sora, STUN and TURN, deployment, moderation)](operations/voice.md)
- [Plugins (destinations, authentication, protocols, `my plugin test`, a plugin with no library)](operations/plugins.md)
- [Decision hooks](operations/decision-hooks.md): ask a plugin of yours before accepting posts, follows, sign-ups, and media

## Reference

- [Configuration reference](reference/configuration.md) (generated from the configuration schema)
- [Build tags](reference/build-tags.md) (the tags that leave adapters out of the binary, and the sets that are checked)
- [Jobs](reference/jobs.md) (every job type and periodic job: what it does, what starts it, how it retries, and whether to run it again by hand)
- [Commands](reference/commands.md) (the go-task tasks and the pinned toolchain)
- [Webhook reference](reference/webhooks.md) (the headers, signatures, events, and bodies a receiver gets)

## Developers

Guides for people who build clients and bots against the client API.

- [Getting a token](developers/authentication.md) (registering an app, the authorization flow, tokens, redirect URIs, scopes)
- [Paging](developers/paging.md) (cursors and limits)
- [Errors](developers/errors.md) (problem documents and the problem types to handle)
- [Rate limits](developers/rate-limits.md) (what to read and how to back off)
- [The event stream](developers/streaming.md) (Server-Sent Events, reconnecting, catching up)
- [Client libraries](developers/libraries.md) (getting started with mykit and mybot in Go and TypeScript)
- [Stability of the API](developers/stability.md) (what changes without notice and what is announced)

## Contributing

- [Contributing guide](../CONTRIBUTING.md)
- [Instructions for coding agents](../AGENTS.md)
- [Working on an issue (playbook)](agents/workflow.md)
- [Writing an ADR (playbook)](agents/writing-adrs.md)
- [Releasing (playbook)](agents/releasing.md)

## Design (in Japanese)

The requirements, the architecture, and the decision records are written in Japanese during v0.

- [Vision and scope](requirements/00-vision.md)
- [Functional requirements (v0)](requirements/01-functional.md)
- [Non-functional requirements](requirements/02-non-functional.md)
- [Glossary](requirements/03-glossary.md)
- [Architecture overview](architecture/overview.md)
- [Logical data model](architecture/data-model/README.md)
- [Visibility checks (canView)](architecture/visibility.md)
- [Federation design (ActivityPub server-to-server)](architecture/federation.md)
- [Job system: components, lifecycle, and fan-out](architecture/job-system.md)
- [Deployment targets and low-cost setups](architecture/deployment-targets.md)
- [Web client conventions](architecture/web-client/README.md)
- [Architecture Decision Records](adr/README.md)
