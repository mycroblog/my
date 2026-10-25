# Webhook reference

What an endpoint of the instance's webhooks receives:
the requests, their signature, the events, and how failures are retried.
Setting up a webhook is in [the operations guide](../operations/webhooks.md).

## Requests

Each message is one `POST` to the endpoint's URL with an `application/json` body and three headers of
[Standard Webhooks](https://github.com/standard-webhooks/standard-webhooks/blob/main/spec/standard-webhooks.md):

| Header | Value |
| --- | --- |
| `webhook-id` | The message ID, a UUIDv7. It is the same on every attempt and redelivery of the message: use it to drop duplicates. |
| `webhook-timestamp` | When this attempt was sent, in Unix seconds. |
| `webhook-signature` | `v1,` and the base64 of the HMAC-SHA256 of `webhook-id.webhook-timestamp.body` under the secret. For 24 hours after the secret is rotated, the header carries a signature under the new secret and one under the previous secret, separated by a space. |

The body is:

```json
{
  "type": "report.created",
  "timestamp": "2026-10-20T12:00:00.000Z",
  "data": { "id": "0192b3c4-...", "category": "spam", "...": "..." }
}
```

- `type` is one of the events below, or `webhook.test`.
- `timestamp` is when the event happened, in RFC 3339 with milliseconds.
- `data` is what the admin API (`/api/v1/admin/...`) answers for the object of the event, as it is when the event happens:
  the server presents it with the same code as the admin API, in the transaction of the event. Its schema for each type is in
  the table below and in `api/openapi.yaml`, where `WebhookMessage` describes the body. A field without a value is left out.
- v0 keeps no version of the body: `data` follows the admin API of `/api/v1`, which may change in a v0 minor release.

The secret is `whsec_` and the base64 of 32 random bytes, as the Standard Webhooks libraries take it. It is shown once, when the
webhook is set up or its secret rotated. Verify with one of
[their libraries](https://github.com/standard-webhooks/standard-webhooks/tree/main/libraries), and reject a timestamp far from your
clock to refuse replays.

## Events

An endpoint subscribes to some of these events. Setting up the subscription needs `manage_webhooks` and the permission listed.

| `type` | When | `data` | Permission |
| --- | --- | --- | --- |
| `report.created` | A report is filed, locally or by another server (a `Flag`) | `AdminReport` (state `open`), as `GET /admin/reports` lists it | `manage_reports` |
| `report.resolved` | A report is resolved | `AdminReport` (state `resolved`, with `outcome`) | `manage_reports` |
| `report.reopened` | A resolved report is reopened | `AdminReport` (state `open`) | `manage_reports` |
| `registration.pending` | A registration awaits a moderator's approval | `AdminRegistration` (state `pending`), with the email address and the reason | `manage_accounts` |
| `account.created` | A registration is approved, at the sign-up or by a moderator, and the local account can be used | `AdminRegistration` (state `approved`) | `manage_accounts` |
| `account.updated` | A local account changes its profile | `AdminAccount`, as `GET /admin/accounts` lists it, with the new profile | `manage_accounts` |
| `moderation_action.created` | An action on an account | `AdminAccountAction` (state `active`) | `manage_accounts` |
| `moderation_action.lifted` | The lift of an action on an account | `AdminAccountAction` (state `lifted`) | `manage_accounts` |
| `appeal.created` | An account appeals an action | `AdminAppeal`, with the action | `manage_accounts` |
| `domain_moderation_action.created` | An action on a domain | `AdminDomainAction` (state `active`) | `manage_domains` |
| `domain_moderation_action.lifted` | The lift of an action on a domain | `AdminDomainAction` (state `lifted`) | `manage_domains` |
| `status.created` | A local account posts publicly | `WebhookStatus` | none besides `manage_webhooks` |
| `custom_emoji.created` | An administrator registers a custom emoji | `WebhookCustomEmoji`: `AdminCustomEmoji` and the `operation` | `manage_custom_emojis` |
| `custom_emoji.updated` | An administrator replaces the image of a custom emoji, changes its settings or aliases, or enables or disables it | `WebhookCustomEmoji` (`operation` says which) | `manage_custom_emojis` |
| `custom_emoji.deleted` | An administrator deletes a custom emoji | `WebhookCustomEmoji`, the emoji as it was before the deletion | `manage_custom_emojis` |
| `webhook.test` | An administrator sends a test message to the endpoint | `WebhookTest` | — (not subscribable) |

- `status.created` is sent for `public` posts only: `unlisted`, `local`, `private`, and `direct` posts never leave the server
  this way, and neither do remote posts.
- `account.updated` is sent for local accounts only, and not for the profile an account is created with.
- `custom_emoji.*` follows the administrators' operations only, never the remote emoji that posts and profiles bring.
- The `data` of the admin types embeds what the admin API embeds: the reporter's and the target's `Account` in `AdminReport`,
  the account's `Account` in `AdminAccount`, the action in `AdminAppeal`, the image URLs in `AdminCustomEmoji`. A registration
  carries the applicant's email address, and a report the reporter's comment: the messages are kept 14 days (below).
- The permissions are checked when a webhook is set up or changed. A webhook keeps its events when the person who set it up later
  loses a permission; someone with `manage_webhooks` disables it when it should stop.

## Answers, retries, and disabling

- A `2xx` answer delivers the message. The body of the answer is not read.
- `410 Gone` disables the webhook at once.
- Any other answer, redirects included (they are never followed), no answer within 15 seconds, and a failed connection fail the
  attempt, which is retried after 5 seconds, 5 minutes, 30 minutes, 2 hours, 5 hours, 10 hours, and 10 hours: 8 attempts over
  about 27 hours. A `Retry-After` header (seconds or an HTTP date, up to 1 hour) replaces the next wait. After the last attempt
  the delivery stops; an administrator can send it again from the administration screen, with the same `webhook-id`.
- When the attempts to a webhook fail without a success for 72 hours and at least 20 times in a row, the server disables it,
  records it in the audit log as the system, and notifies the users with `manage_webhooks`.
- A disabled webhook receives nothing, also of the deliveries waiting to be retried, and enabling it again does not send the
  messages of the time it was disabled.
- The messages and their deliveries are kept for 14 days and then deleted.

## Destinations

The server sends through its SSRF guard, the same one as federation: the URL must be `https` (or `http`) on a public address. A destination on
loopback or a private network, such as an automation tool on the same host or in the same VPC, is reached only when the operator
lists it in `webhook_allowed_destinations` ([configuration reference](configuration.md#webhook_allowed_destinations)); such a
destination may also use plain `http` and any port. An attempt to a destination the guard refuses fails with the outcome
`forbidden_destination`.

The audit log and the logs show the host name of a URL and never the URL itself: chat services put a secret in the path or the
query of their webhook URLs.
