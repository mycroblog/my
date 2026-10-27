# Documentation

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
- [Running the server: starting, stopping, and health checks](operations/running.md)
- [Serving media through a CDN (media_origin, origin pull, caching and purging)](operations/cdn.md)
- [Maintenance (the maintenance mode and the exported maintenance page)](operations/maintenance.md)
- [Production checklist (what to verify before going live, and what my config check checks)](operations/production-checklist.md)
- [Closing the service (announcement, closure, the static closure page, deleting the data)](operations/service-closure.md)
- [External monitoring and status pages (UptimeRobot, Better Stack, Gatus, Uptime Kuma, Upptime)](operations/monitoring.md)
- [Alerts and runbook (Prometheus alerting rules, what to do when an alert fires, telling users about an incident)](operations/alerts.md)
- [Error reporting (the error report log line, error groups and notifications, sending reports to Sentry, GlitchTip, Datadog or OpenTelemetry, scrubbing)](operations/error-reporting.md)
- [Link previews (OGP / X Cards / fediverse:creator)](operations/link-previews.md)
- [Search engines and AI crawlers (robots.txt, noindex, opting out of AI)](operations/crawlers.md)
- [Server webhooks](operations/webhooks.md)
- [Federation relays](operations/relays.md)
- [Releasing](operations/releasing.md)
- [Importing cloud usage costs (AWS Cost Explorer, Google Cloud's BigQuery export)](operations/billing-import.md)
- [Supporter payments with Stripe](operations/payments-stripe.md)
- [Supporter payments with PAY.JP](operations/payments-payjp.md)
- [Supporters (paid plans)](operations/supporters.md)
- [Voice chat (choosing P2P, LiveKit or Sora, STUN and TURN, deployment, moderation)](operations/voice.md)

## Reference

- [Configuration reference](reference/configuration.md) (generated from the configuration schema)
- [Build tags](reference/build-tags.md) (the tags that leave adapters out of the binary, and how they are checked)
- [Commands](reference/commands.md) (the go-task tasks and the pinned toolchain)
- [Webhook reference](reference/webhooks.md) (the headers, signatures, events, and bodies a receiver gets)
- [Building third-party apps](reference/third-party-apps.md) (registering an app, the authorization flow, scopes,
  rate limits, API stability)

## Contributing

- [Contributing guide](../CONTRIBUTING.md)
- [Instructions for coding agents](../AGENTS.md)
- [Working on an issue (playbook)](agents/workflow.md)
- [Writing an ADR (playbook)](agents/writing-adrs.md)

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
- [Deployment targets and low-cost setups](architecture/deployment-targets.md)
- [Web client conventions](architecture/web-client/README.md)
- [Architecture Decision Records](adr/README.md)
