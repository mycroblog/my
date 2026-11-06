# Sending email

What email the server sends, how to configure the sending, and how to handle failures and unsubscribing.
The settings are listed in the [configuration reference](../reference/configuration.md#mail).
The user-facing flows of email confirmation and password reset are in [registration and email confirmation](registration.md).

## What is sent, and how to configure it

The server never sends email inside a request. It enqueues a job for each purpose in the same transaction as the fact that
triggers the email, and a job worker sends it. A failure to send does not show in the response to a registration or a reset:
the user is told "we sent a confirmation email", and asks for it again if it does not arrive.

| Class | Email | Job type | Can the user unsubscribe? |
| --- | --- | --- | --- |
| Transactional (`transactional`) | Email address confirmation | `identity.verification_email` | No |
| Transactional | Password reset | `identity.password_reset_email` | No |
| Transactional | Notice of a password change or deletion | `identity.password_notice_email` | No |
| Transactional | Notice of the deactivation of the account, with the date of its deletion | `identity.deactivation_email` | No |
| Transactional | Service shutdown (the announcement, a change of date, a cancellation, 7 days and 1 day before the closing date, the closing, and 7 days before the export deadline) | `identity.closure_email` | No |
| Notification (`notification`) | Notice to moderators (on by default, per kind of notice) | `notification.moderator_email` | Yes |
| Notification (`notification`) | A notification that stayed unread for 15 minutes: mentions, follows, boosts, and the other notifications from other users, and notices from the moderators (off by default, per kind) | `notification.email` | Yes |

Notification emails for supporters (`notification.supporter_email`) go through the same job queue.

### Which notifications are emailed

Each user chooses, per kind of notification, whether it arrives in the app and by email, on the **Notifications** page of
the settings (`/settings/notifications` in the web client). The Email column appears there only while the server sends
email. A kind turned off in the app is not created at all, so it is not emailed or pushed either.

| Kind | Email by default | When it is sent |
| --- | --- | --- |
| Notifications from other users (mentions, new followers, follow requests, accepted follow requests, boosts, favourites, emoji reactions, quotes, ended polls, accounts you follow moving, voice chats) | Off | 15 minutes after the notification, only if it is still unread (not marked read, not dismissed) and still visible, and the user still gets that kind by email. One email per notification |
| Notices from the moderators (a moderation action on the account, a resolved report, a decided appeal, follows lost to a suspension, a decided custom emoji request) | Off | The same as above |
| Notices to moderators (new reports, registrations awaiting approval, appeals, disabled webhooks, custom emoji requests, limited accounts moving, the closing date passing, new and recurring errors), to the people whose role has the permission | On | At once, without waiting for it to be read |
| Service shutdown, and the notices of supporter plans | Always | At once; these cannot be turned off |

Notification emails go only to the user's primary address once it is confirmed, and never to an address whose emails are
stopped (see below). The subject of the email of a notification from another user is the same text as its Web Push message,
and the email links to the post, the profile, or the notifications.

Service shutdown emails go to every user whose primary address is confirmed. Missing one can mean losing data, so they are sent
as transactional email, carry no unsubscribe link, and are not stopped by a complaint (a hard bounce does stop them).
The announcement, a change of date, a cancellation, and the closing each enqueue `identity.closure_email_fanout` in the same
transaction as their record. The hourly `identity.queue_closure_reminders` decides when to send the reminders for 7 days and
1 day before the closing date and 7 days before the export deadline. A reminder whose time had already passed when the shutdown
was announced is not sent, because the announcement email states the dates.
The fan-out job records a row of `service_closure_emails` per user and enqueues `identity.closure_email` only for the users it
could record, so however often the job runs, each user gets each email once. Before sending, the job checks that the email is
still valid (that it is not for a cancelled announcement or for a date that has since changed).

The setting `mail` chooses how email is sent.

| `mail` | Behavior |
| --- | --- |
| `none` (default) | Sends nothing and writes `mail not sent (mail = none)` to the server log with the category of the message. With `profile = development`, the line also holds the addresses, the subject, and the body, links included. For setups that do not use email, and for development |
| `smtp` | Sends by SMTP submission. It requires STARTTLS and verifies the certificate before authenticating and sending |
| `ses` | Sends with the Amazon SES v2 API (`SendEmail` with raw content). It authenticates through the AWS SDK's default credential chain (an IAM role, for example) and holds no SMTP password |
| `plugin` | Hands each message to a [plugin](#a-plugin-mail--plugin) that you run, for a mail service that offers only an HTTP API |

The settings of `smtp` are `mail_from`, `mail_smtp_host`, `mail_smtp_port` (default 587), `mail_smtp_username`, and `mail_smtp_password`. The
settings of `ses` are `mail_from`, `mail_ses_region`, and `mail_ses_configuration_set`. The settings of `plugin` are `mail_from`
and `plugin_mailer_address` with the rest of [its destination](plugins.md#the-destination).
`mail_from` is a bare address such as `noreply@social.example`. `From` becomes `"<server name>" <mail_from>`, and the domain
of `mail_from` is also the domain of `Message-ID` and the one SPF, DKIM, and DMARC apply to (see below).
`Reply-To` is the instance setting `contact_email` when it is set ([instance settings](instance-settings.md)).
Every email carries `Auto-Submitted: auto-generated`.
`smtp` and `ses` build the same message. SMTPUTF8 is not used, so addresses are sent in ASCII and internationalized domains are
converted to Punycode. An address whose local part (before the `@`) is not ASCII cannot be sent to, and its job fails.

## Examples by provider

Pass secrets by environment variable or file (`MY_MAIL_SMTP_PASSWORD` or `MY_MAIL_SMTP_PASSWORD_FILE`). Do not put them in the
configuration file.

### Amazon SES (API)

When you run on AWS, `mail = ses` uses the SES v2 API. Credentials come from the AWS SDK's default chain (the task role of
ECS, environment variables, the shared configuration file), so there is no secret to configure.

1. In SES, verify the domain of `mail_from` as an identity and enable Easy DKIM (see below).
2. Request production access to leave the sandbox. In the sandbox you can only send to verified addresses.
3. Allow `ses:SendEmail` for the execution role on the identity of `mail_from` (and on the configuration set, if you use one).

```sh
MY_MAIL=ses
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SES_REGION=ap-northeast-1          # the region where you verified the identity; AWS_REGION when empty
MY_MAIL_SES_CONFIGURATION_SET=my-events    # optional; a configuration set with event destinations
```

When `mail_ses_configuration_set` is set, every email uses that configuration set. SES overwrites the `Message-ID` and `Date` of the
email with its own values.
SES never rejects a recipient synchronously at send time. It accepts a send to an address on the account suppression list and
then does not deliver it, and the bounce arrives later as a notification (SNS).
To record bounces and complaints, set up the SES notifications below to arrive through SNS.

### A plugin (`mail = plugin`)

For a mail service that offers only an HTTP API, run a [plugin](plugins.md) of the kind `mailer` and let it call the service.

```sh
MY_MAIL=plugin
MY_MAIL_FROM=noreply@social.example
MY_PLUGIN_MAILER_ADDRESS=unix:///run/my/mailer.sock
```

`plugin_mailer_address` is required with this value. A message can hold a secret, such as a verification link, so the kind
refuses `http://`, on loopback too: use a Unix socket or `https://`. The server starts without calling the plugin, even when it
is down, and the email waits in its job.

With `plugin_mailer_protocol = json`, the default, a plugin needs an HTTP server and a JSON library alone
([A plugin with no library](plugins.md#a-plugin-with-no-library)). `Send` lasts 30 seconds at most, or `plugin_mailer_timeout`,
below 5 minutes. A request is at most 2 MiB and a response at most 64 KiB.

```text
POST /my.plugin.mailer.v1.MailerService/Send
Content-Type: application/json
Connect-Protocol-Version: 1

{
  "category": "CATEGORY_NOTIFICATION",
  "to": "alice@example.com",
  "envelopeFrom": "noreply@social.example",
  "fromName": "My Server",
  "replyTo": "admin@social.example",
  "subject": "You have a new follower",
  "text": "Bob followed you.\n",
  "html": "<p>Bob followed you.</p>",
  "headers": {
    "List-Unsubscribe": "<https://social.example/unsubscribe?token=abc>",
    "List-Unsubscribe-Post": "List-Unsubscribe=One-Click"
  }
}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_RECIPIENT_REJECTED","enhancedStatus":"5.1.1"}
```

The server checks a message before it calls the plugin, so a header cannot be injected through any of these fields. The request
holds the parts of the message, and the plugin builds what its service needs:

- **The plugin adds three headers:** a `Message-ID` that is unique per message, on the domain of `envelopeFrom`; a `Date`; and
  `Auto-Submitted: auto-generated`, which keeps automatic replies from looping. A service that writes its own `Message-ID` and
  `Date` covers those two.
- `From` is `fromName` and `envelopeFrom`. `fromName`, `replyTo`, `html`, and `headers` may be missing. `headers` holds only the
  two `List-Unsubscribe` headers.
- The addresses are UTF-8. A plugin whose service needs ASCII converts the domain to Punycode, and answers `OUTCOME_INVALID` for a
  local part it cannot send.
- The same message may come twice, since a job that fails is retried. Do not write a message to a log.

`outcome` says what happened to the message:

| `outcome` | Meaning | The server |
| --- | --- | --- |
| `OUTCOME_SENT` | The service took the message | Completes the job |
| `OUTCOME_INVALID` | It cannot be sent as it is | Fails the job at once |
| `OUTCOME_RECIPIENT_REJECTED` | The address does not exist | Fails the job at once, and [suppresses the address](#suppression-of-addresses-that-cannot-receive-email) |
| `OUTCOME_REJECTED` | The service refused it for good, for another reason | Fails the job at once |
| `OUTCOME_TEMPORARY` | The service refused it for now, or did not answer | Retries the job |
| Anything else, or none | | Retries the job |

An HTTP error, or Connect's `{"code":"unavailable"}` with the matching status, is also a temporary failure.
`enhancedStatus` is optional: the enhanced status code of a refusal, such as `5.1.1`. The server keeps a code of class 5 of a
rejection and of class 4 of `OUTCOME_TEMPORARY`, and ignores anything else. It keeps no text from a plugin, so put no reason there:
a reason belongs in the plugin's own log.

The server learns of a bounce only from this answer. It has no endpoint for the later bounces and complaints of a plugin's
service, so a bounce that the service reports afterwards does not suppress the address.

Test the plugin with the `my` binary:

```sh
my plugin test --kind mailer unix:///run/my/mailer.sock
my plugin test --kind mailer --mail-to you@example.com --mail-from noreply@social.example unix:///run/my/mailer.sock
```

The first only checks that the plugin answers, and sends nothing. The second sends a real message to `--mail-to`, and sends the
plugin a request with no recipient, which it must refuse. The server cannot see the message after the plugin has it, so look at
the one that arrives: its headers must hold `Message-ID`, `Date`, and `Auto-Submitted: auto-generated`.
See [Test a plugin](plugins.md#test-a-plugin).

### Receive SES notifications (SNS)

SES bounces and complaints go to an SNS topic, and an HTTPS subscription on that topic delivers them to the server's
`POST /mail/events/ses`.
The endpoint opens only when `mail_ses_sns_topic_arn` is set; otherwise it answers `404`.

1. Create a standard SNS topic in the same region as the SES identity.
2. Set the topic attribute `SignatureVersion` to `2` (SHA256withRSA). A signature with the default `1` is not accepted and the
   server answers `400`.
3. In the topic's access policy, allow `ses.amazonaws.com` to `SNS:Publish` from your own account (the SES console adds this
   when it creates the topic).
4. Point SES notifications at the topic, in either way (the server reads both `notificationType` and `eventType` in the body).
   - Identity notifications: use this topic for the "Bounce" and "Complaint" feedback notifications of the `mail_from` identity.
   - Configuration set event destination: add this topic as a destination of the configuration set in `mail_ses_configuration_set`,
     for the event types `BOUNCE` and `COMPLAINT`.
5. Set `MY_MAIL_SES_SNS_TOPIC_ARN` to the ARN of the topic, check with `my config check`, and restart.
6. Create a subscription on the topic with the protocol `https` and the endpoint `https://<server host>/mail/events/ses`.
   **Do not enable raw message delivery** (a body without a signature is rejected).

```sh
aws sns create-topic --region ap-northeast-1 --name my-ses-feedback
aws sns set-topic-attributes --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --attribute-name SignatureVersion --attribute-value 2
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Bounce --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Complaint --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback

MY_MAIL_SES_SNS_TOPIC_ARN=arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback   # restart, then create the subscription below

aws sns subscribe --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --protocol https --notification-endpoint https://social.example/mail/events/ses
```

When the server receives the subscription confirmation (`SubscriptionConfirmation`), it sends a `GET` to its `SubscribeURL` to
confirm the subscription and logs `SNS subscription of the SES notifications confirmed`.
Check with `aws sns list-subscriptions-by-topic` that the subscription is no longer `PendingConfirmation`.

Before it uses a message, the endpoint checks all of the following. If any fails, it answers `400` and records nothing.

- `TopicArn` equals `mail_ses_sns_topic_arn`.
- `SignatureVersion` is `2` and the signature is valid.
- The certificate URL of the signature (`SigningCertURL`) is `https` and its host is `sns.<region>.amazonaws.com` for the region
  of the topic. The `SubscribeURL` of a subscription confirmation must have the same host.

The certificate is fetched with the outbound HTTP client that guards against SSRF and cached in the KV store for 24 hours. The
body is read up to 1 MiB, and a larger one gets `413`.
When the certificate cannot be fetched, the subscription confirmation fails, or the record cannot be written, the endpoint
answers `503` and leaves it to SNS to retry. It also answers `503` during maintenance mode.

These are the notifications it records. It matches the recipient address against the primary address (case-insensitively, and a
Punycode domain also matches in its Unicode form); if none matches, it does nothing.

| Notification | Recorded as |
| --- | --- |
| A bounce whose `bounceType` is `Permanent` (any subtype such as `General`, `NoEmail`, `Suppressed`, or `OnAccountSuppressionList`) | `hard_bounce` |
| A complaint (except those whose `complaintFeedbackType` is `not-spam`) | `complaint` |
| A bounce that is `Transient` or `Undetermined`, and other notifications such as delivery or send | Not recorded (answers `200`) |

The SES `feedbackId` is the unique key of a record, so however many times SNS sends the same notification, there is one record.

### Amazon SES (SMTP interface)

When you cannot give IAM credentials, such as when you run outside AWS, use the SES SMTP interface with `mail = smtp`. In this
setup only a rejection of the recipient at send time is recorded as a suppression
([suppression of addresses that cannot receive email](#suppression-of-addresses-that-cannot-receive-email)).

1. In SES, verify the domain of `mail_from` as an identity and enable Easy DKIM (see below).
2. Request production access to leave the sandbox. In the sandbox you can only send to verified addresses.
3. Create SMTP credentials in the SES "SMTP settings" (these differ from IAM access keys).

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SMTP_HOST=email-smtp.ap-northeast-1.amazonaws.com   # the endpoint of your SES region
MY_MAIL_SMTP_PORT=587                                       # STARTTLS
MY_MAIL_SMTP_USERNAME=<user name of the SMTP credentials>
MY_MAIL_SMTP_PASSWORD_FILE=/run/secrets/mail_smtp_password
```

SES overwrites `Message-ID` and `Date` with its own values.

### A general SMTP submission server

Use this for your own SMTP server such as Postfix, or for the submission server of an email delivery provider.

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SMTP_HOST=smtp.provider.example     # a STARTTLS server whose certificate is valid for this name
MY_MAIL_SMTP_PORT=587
MY_MAIL_SMTP_USERNAME=<user name>           # empty for a server without authentication
MY_MAIL_SMTP_PASSWORD_FILE=/run/secrets/mail_smtp_password
```

The server does not send to a server that offers no STARTTLS, or whose certificate cannot be verified for the name in
`mail_smtp_host`. Credentials are sent only after STARTTLS.
After you change the settings, check with `my config check` and restart.

## SPF, DKIM, and DMARC

Gmail and Yahoo require SPF, DKIM, DMARC, and one-click unsubscribe from senders that send many emails a day. Even for a small
volume, email without them is more likely to be classified as spam.
Set all of them in the DNS of the domain of `mail_from` (for example `social.example`), with the values your provider gives you.

- **SPF**: put one TXT record on the domain that authorizes the provider you send through (for example, if you use a custom SES
  MAIL FROM domain, the `include:` that the provider specifies, on that domain).
- **DKIM**: put the public key the provider issues in DNS and enable signing on the sending side. For SES this is the three
  Easy DKIM CNAME records. The server itself does not sign, so the provider signs.
- **DMARC**: a TXT record at `_dmarc.social.example`. **Start with `p=none`**, check with the aggregate reports (`rua=`) that SPF
  and DKIM align (match the From domain), and then tighten to `quarantine` and `reject`.

```text
_dmarc.social.example.  TXT  "v=DMARC1; p=none; rua=mailto:dmarc@social.example"
```

To align SPF on SES, set a custom MAIL FROM domain (the default MAIL FROM is `amazonses.com`, which does not match the From
domain). If DKIM aligns, DMARC passes.

## Look at send failures

Look at send failures on the jobs page (`/admin/jobs/list`, permission `manage_jobs`; see
[the job queue and failed jobs](running.md#the-job-queue-and-failed-dead-jobs)).
Filter by job type to see only email jobs (in the admin API, `GET /api/v1/admin/jobs?type=identity.verification_email`).
The types are in the table above.

- **Temporary failures** (connection errors, timeouts, 4xx answers) are retried. Transactional email is tried 6 times (an
  exponential backoff from 30 seconds, about 15 minutes in total); notification email uses the default policy (10 times, about
  4 hours). When the attempts are used up, the job becomes a failed job (dead job). A wrong `mail_smtp_host`, a failed
  authentication, and a blocked network show up here. After you fix the cause, run it again with `my job retry <id>` or on the
  jobs screen.
- **Permanent rejections** (a 5xx for the recipient or the content) are not retried and fail at once.
- With `mail = ses`, SES's `MessageRejected` (the content was rejected) is a permanent rejection. `TooManyRequestsException`
  (the sending rate limit), `SendingPausedException`, `MailFromDomainNotVerifiedException`, `AccountSuspendedException`, and
  requests that do not reach SES are temporary failures, and a retry succeeds once you fix the account or the permissions.
  Only the SES error code is kept in the error text.
- With `mail = plugin`, the [`outcome`](#a-plugin-mail--plugin) of the answer decides. A plugin that cannot be reached, or that
  answers an error, is a temporary failure, and it counts in [`my.plugin.failures`](running.md). Only the outcome and the enhanced
  status code are kept in the error text, so the reason of a failure is in the plugin's log.
- A transactional email that is older than a time limit from its request (15 minutes for a reset, 1 hour for a confirmation) is
  completed without being sent. This keeps old links from arriving late; it is not a failure.
- When the job runs and there is no reason to send (the address is suppressed, it has been confirmed in the meantime, or the user
  lost the permission or the setting for the notification), it completes without sending.

A job payload holds no email address, body, or token, and neither does an error text. Only IDs, the job type, and the SMTP
response code remain.
A successful send can be followed in the structured `job` log line of each run (`my.job.type`).

## Suppression of addresses that cannot receive email

When the SMTP server says the recipient address does not exist (a 5xx to `RCPT TO` with an extended status of `5.1.x`), the
server records the address as a hard bounce and stops sending to it.
With `mail = plugin`, `OUTCOME_RECIPIENT_REJECTED` records the same hard bounce, and nothing else does.
With `mail = smtp`, only this rejection at send time is recorded. Asynchronous bounces that come back after the SMTP server
accepted the email, and complaint feedback loops, are not received.
With `mail = ses`, SES never rejects a recipient synchronously, so nothing is recorded at send time; permanent bounces and
complaints are recorded from the SES notifications
([Receive SES notifications (SNS)](#receive-ses-notifications-sns)).

| Kind | Email that is stopped |
| --- | --- |
| `hard_bounce` | Everything except the confirmation email. The password reset email is stopped too |
| `complaint` (reported as spam) | Notification and announcement emails. Transactional emails are still sent |

- Moderators see the kind and origin of a suppression on the account's Management page of the admin screens (`email_suppression` in
  `GET /api/v1/admin/accounts/{id}`).
- There are two ways to **lift** it. When the user asks for a confirmation email and confirms the address again, the hard-bounce
  suppression is lifted (the confirmation email is sent even while the address is suppressed). The user asks for it through the
  client API (`POST /api/v1/accounts/email/verification`, at most 3 an hour) or on the sign-in
  status page. When you have verified that it is
  fine to send, for example because the mailbox was fixed, a moderator lifts it with
  `POST /api/v1/admin/accounts/{id}/email_suppression/lift` (permission `manage_accounts`). It records
  `email_suppression_lifted` in the audit log ([moderation](moderation.md)).
- For a user whose password reset email is stopped, an administrator issues a reset link and hands it over
  (`my admin account password-reset-link @alice`).
- On the settings screen, users see that email is not reaching their primary address, and since when.

## Unsubscribing

Notification emails carry an unsubscribe link and the headers `List-Unsubscribe` and `List-Unsubscribe-Post:
List-Unsubscribe=One-Click` (RFC 8058). The "unsubscribe" button of email clients uses them.

- The unsubscribe classes map one to one to the rows of the notification settings whose email the user can choose, such as
  `mention` or `admin_report` (one per kind of notice to moderators, and `moderation` for the notices from the moderators).
- The link is `https://{host}/mail/unsubscribe?token=...` and needs no sign-in. A token is issued per email, only its hash is
  kept in the database, and it expires after 90 days.
  - `GET` only shows a confirmation screen and changes nothing (so that scanners that prefetch links do not stop the email).
    The button sends the same `POST`.
  - `POST` (one click) turns off the email of that row of the notification settings, and nothing else. Until the token
    expires, sending it again gives the same result.
- **Transactional email cannot be unsubscribed from.** Confirmation, reset, and the notice of a password change would stop
  security-relevant notices from arriving. These emails carry no such headers either.
- Users can switch the same setting on the **Notifications** page of the settings.

## The HTML version

Every email is `multipart/alternative` with the same content as `text/plain` and `text/html` (both UTF-8). With `mail = none`
in the `development` profile, only the text version is written to the log.

- There is one outer layout, with paragraphs, buttons, and a signature (server name, URL, contact, the reason for the email,
  and the unsubscribe link). `<html lang dir>` comes from the recipient's locale, and languages such as `ar` are laid out right to
  left.
- It loads no external fonts or stylesheets. Colors are the default design tokens, and light and dark follow
  `color-scheme` and `prefers-color-scheme`. The administrator's theme overrides do not affect email.
- It loads no images, except the instance's own logo when one is set ([logo and icon](instance-settings.md#logo-and-icon)):
  the HTML version then starts with the logo (`https://<host>/brand/...`, the dark logo instead where the email client follows
  a dark color scheme), the same URL for every recipient, so it tells nothing about who opened the email. Clients that load no
  external images show the server name (the image's alt text) instead. Without a logo, or when the server URL is not `https`,
  the email has no image.
- Only `https` URLs become links. When a URL is `http`, as in a development setup where `web_origin` is `http://{host}`, the URL
  is written as text instead of a button or an unsubscribe link, and the server URL in the signature is left out.
- The layout is part of the binary, so no JavaScript runs in the server at run time.
