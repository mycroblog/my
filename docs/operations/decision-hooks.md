# Decision hooks

A decision hook asks a [plugin](plugins.md) of yours whether to accept something before the server accepts it. The plugin
answers with a verdict from a fixed list. It never rewrites anything and it takes no action on an account: the most it can do
is stop one post, follow, or sign-up, or file a report for your moderators. People decide what happens to an account.

Nothing is reviewed by default. A server with no hook address makes no call and spends no time on them. This version keeps
the records, the modes, and the admin screens described here. Of the intake paths, only follows from other servers ask a hook
(`inbound.follow`), and the other hooks are not called yet.

## The hooks

| Hook | What it reviews | What it can answer | When the plugin fails |
| --- | --- | --- | --- |
| `inbound.status` | Posts from other servers | `allow`, `drop`, `flag`, `mark_sensitive` | `pass` (default) or `retry` |
| `inbound.follow` | Follows from other servers | `allow`, `drop`, `flag` | `pass` (default) or `retry` |
| `signup.check` | Sign-ups, after the CAPTCHA and the block list | `allow`, `require_approval`, `reject` | `pass` (default) or `require_approval` |
| `media.check` | Uploaded media | `allow`, `mark_sensitive`, `flag` | `pass` (default) or `retry` |
| `local.status` | Posts of local accounts | `allow`, `flag`, `reject` | `pass` |

Set `plugin_<hook>_address` to give a hook a plugin, with a dot in the name becoming `_`
(`plugin_inbound_status_address`). [Plugins](plugins.md) covers the destination, its authentication, and how to test a
plugin with `my plugin test --kind inbound.status <address>`. The call is `POST /my.plugin.hook.v1.HookService/Check`.

What each answer does:

- `drop` stops the subject and stores nothing of it. A wrong `drop` silently hides a post from its followers too, so read
  what a hook would drop in `observe` before you enforce it.
- `flag` lets it through and files one report per decision. On `inbound.follow` this version only records the decision and
  files no report.
- `mark_sensitive` lets it through and shows its attachments as sensitive until a moderator cancels the decision.
- `require_approval` holds the sign-up for approval.
- `reject` refuses it, with fixed text that shows nothing of the plugin's answer.

An answer can carry a coded reason (`spam`, `harassment`, `sexual`, `violence`, `illegal`, `malware`, `other`) and a note of
up to 200 characters for your moderators. An answer the hook does not take counts as a failure.

## Modes

Each hook has a mode, an [instance setting](instance-settings.md) (`hook_inbound_status_mode` and so on) you change under
**Server settings** > **External review**. Changes go in the audit log as `setting_changed`.

| Mode | What it does |
| --- | --- |
| `off` | Calls nothing. |
| `observe` | Records the verdict and acts on nothing. This is the default of every hook. |
| `enforce` | Acts on the verdict. |

A hook goes from `off` to `enforce` only through `observe`.

## When the plugin fails

A timeout, an error, and an invalid answer are recorded as outcomes of their own (`timeout`, `error`, `invalid`), never as
verdicts. After 5 failed calls in a row the server skips the plugin for 30 seconds and then tries one call, so an outage
costs each subject no wait; a skipped call is recorded as `breaker_open`. Each server process counts by itself.

What happens to the subject then depends on `hook_<hook>_on_failure`:

- `pass` lets it through unreviewed. Spam can get in during an outage, but nothing else stops. This is the default.
- `retry` fails the job, which runs again and calls the plugin again, and records no decision. Jobs from one actor run in
  order, so everything later from that actor waits meanwhile, deletions included, for up to about 4 hours and 15 minutes.
  After its last run the job becomes a [dead job](running.md#the-job-queue-and-failed-dead-jobs), which releases the others.
- `require_approval` holds the sign-up for a moderator.

In `observe` a failure never holds anything up. Moderators get a notification once a day per hook when its calls keep failing.
The metrics are `my.hook.decisions`, `my.hook.failures`, and `my.hook.duration` ([Running](running.md)).

## The decisions

The server records one decision per hook and subject, so a job that runs again reads it and does not call the plugin again.
A decision holds the hook, the key of the subject, the mode, the outcome, the reason, the note, and how long the call took. It
holds no text of a post, no email address, and no IP address.

**Moderation** > **Automated reviews** (`manage_reports`) shows the counts and waits of the last 7 days, and what the hooks
stopped, would have stopped in `observe`, or could not check. Cancelling a decision there goes in the audit log as
`hook_decision_overridden`. A `reject` of `local.status` is not cancelled there: its author appeals it.

Cancelling a `drop` of `inbound.follow` runs the follow again, without calling the plugin, for as long as the server keeps
the activity it received (7 days). After that the follow cannot come back and the cancellation only goes on record; the
other server has to send a new follow.

A follow only asks the hook when it would be recorded. Taking a follow back (`Undo`) never asks, and neither does a follow that
the server refuses anyway: one that the federation mode refuses, one of a moved or deactivated account, or one of the instance
actor. A dropped follow gets no answer, so the other server keeps showing it as requested. Sending the same activity again is
ignored as a duplicate.

An automated effect goes in the audit log, with the system as the actor, only when it leaves state: `mark_sensitive`, a
filed `flag`, and `require_approval` (`hook_decision_applied`). `drop` and `reject` store nothing, so they stay in the
decisions alone.

The job `hook.purge_decisions` removes decisions after `retention_hook_decisions` days (default 90, at least 30). It keeps
those that a sensitive mark on an existing post rests on, those of an account under a
[retention hold](moderation.md#record-retention-and-holds), and each `reject` of `local.status` its author can still appeal. Deleting
an account removes its decisions.

## What you tell your users

While a hook has a plugin and is not `off`, your information page (`/about`) states that an external review is in use, what
it covers, and what is sent to it. The server writes this from your settings.

## What is sent, and what it costs

Calls and sent data grow with what the server receives. Review what you send before you turn a hook on:

- `inbound.status` and `local.status` send the text of posts, and refuse plain TCP.
- `signup.check` sends email addresses and IP addresses. It allows plain TCP, which sends them unencrypted.
- `inbound.follow` sends the two parties of a follow.
- `media.check` sends a small processed version of the media, never the original.
