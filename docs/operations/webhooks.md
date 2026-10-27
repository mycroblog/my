# Server webhooks

This page is for operators. What a receiver gets (headers, signature, events, body) is described in the
[webhook reference](../reference/webhooks.md).

Webhooks send reports, registration requests, account creations and profile changes, moderation actions, appeals,
domain actions, local public posts, and custom emoji management actions to a Slack or Discord relay, a ticketing system,
or an automation tool such as n8n. A server with no webhooks configured stores no extra rows and runs no extra jobs.

## Set up a webhook

Anyone with the `manage_webhooks` permission can set webhooks up on the admin screen "Webhooks" (`/admin/webhooks`).
The built-in `admin` role has it.

1. Create a webhook with a destination URL, a description, and the events to subscribe to. Each event also needs its own
   permission besides `manage_webhooks`: `manage_reports` for reports, `manage_accounts` for registrations, accounts,
   moderation actions and appeals, `manage_domains` for domain actions, `manage_custom_emojis` for custom emoji, and
   `manage_errors` for error groups.
   Public posts need no extra permission. You cannot select an event whose permission you lack.
2. The secret (`whsec_...`) is shown once, when the webhook is created. Set it on the receiver. It cannot be shown again.
3. Choose "Send a test" to send `webhook.test`, and check in the deliveries that it arrived.

Creating, changing, deleting, enabling, disabling, and rotating the secret of a webhook are recorded in the audit log.
The target is shown as the description and the host name of the URL; the URL itself is not recorded.

## Rotate the secret

"New secret" creates a new secret and shows it once. For the next 24 hours messages are signed with the old secret too
(`webhook-signature` carries two signatures), so you can switch the receiver to the new secret in that window.
Secrets are sealed with `kernel.Sealer` before they are stored in the database; the key is the
[KEK](key-management.md).

## Failures and redelivery

- If the destination answers with anything other than `2xx`, does not answer within 15 seconds, or cannot be reached,
  the message is retried after 5 seconds, 5 minutes, 30 minutes, 2 hours, 5 hours, 10 hours, and 10 hours (8 attempts in
  total, about 27 hours). A delivery that has used them all becomes a dead job, and "Send again" in the deliveries sends
  it again with the same `webhook-id`.
- If the destination answers `410 Gone`, the webhook is disabled immediately.
- The server disables a destination that has not succeeded for 72 hours since its first failure and has failed 20 or more
  times in a row. The audit log gets a `webhook_disabled` entry by `system`, and everyone with `manage_webhooks` gets an
  in-app notification (and an email, if they chose to receive notifications by email).
- The list on the admin screen shows the number of failures in a row and the reason the server disabled a webhook
  (`gone` or `failing`). Enable the webhook again after fixing the destination. Messages from the time it was disabled are
  not sent again.
- Messages and deliveries are deleted after 14 days (fixed). Bodies contain personal data such as the comment of a report.

## Destinations on an internal network

Delivery goes through the SSRF guard of outbound HTTP. By default it only reaches public addresses over `https` (ports 443
and 80). To send to an internal destination, such as an n8n on the same host or a chat inside your VPC, list host names,
IP addresses, and CIDRs, separated by commas, in the setting `webhook_allowed_destinations` (environment variable
`MY_WEBHOOK_ALLOWED_DESTINATIONS`; see the [configuration reference](../reference/configuration.md#webhook_allowed_destinations)).

```sh
MY_WEBHOOK_ALLOWED_DESTINATIONS=n8n.internal,10.0.0.0/8
```

- Only the listed destinations, and only for webhook delivery, are exempt from the guard. Federation, media, and link
  previews are unchanged. An exempt destination may also use `http` and any port.
- A host name is allowed whatever address it resolves to. A CIDR is allowed when the URL names an IP address in that
  range. If you use a host name that resolves into the range, list the host name too.
- Only an operator with access to the server can change the setting. The permissions of the admin screen alone cannot reach
  an internal network.
- An attempt at a destination the guard refused shows `forbidden_destination` in the deliveries.
- The development setting `http_allow_private_networks` lifts the guard for every purpose, webhooks included.
