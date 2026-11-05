# Decision hooks

A decision hook asks a [plugin](plugins.md) of yours whether to accept something before the server accepts it. The plugin
answers with a verdict from a fixed list. It never rewrites anything and it takes no action on an account: the most it can do
is stop one post, follow, or sign-up, or file a report for your moderators. People decide what happens to an account.

Nothing is reviewed by default. A server with no hook address makes no call and spends no time on them. This version keeps
the records, the modes, and the admin screens described here, `inbound.status` asks about posts from other servers, and
`signup.check` asks about sign-ups; no other intake path asks a hook yet.

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
- `flag` lets it through and files one report per decision.
- `mark_sensitive` lets it through and shows its attachments as sensitive until a moderator cancels the decision.
- `require_approval` holds the sign-up for approval.
- `reject` refuses it, with fixed text that shows nothing of the plugin's answer. On `signup.check` this also takes
  [the setting below](#sign-ups).

An answer can carry a coded reason (`spam`, `harassment`, `sexual`, `violence`, `illegal`, `malware`, `other`) and a note of
up to 200 characters for your moderators. An answer the hook does not take counts as a failure.

## What `inbound.status` reviews

The server asks about each `Create` of a post and each `Update` of one that it would take in, before it stores anything. Every
revision is its own subject with its own decision.

- A post is asked about only when it is related to a local account: a local account follows its author, it names a local
  account, or it is in a thread with a local post. Whether a local account follows the author does not narrow it further:
  the plugin gets `any_local_follower` and can weigh it. A post only a relay brought in is never asked about, nor is an
  `Update` of a stored post that no longer relates to a local account: its revision is stored without a question.
- A direct post from an author no local account follows is not sent unless you turn on
  `hook_inbound_status_unfollowed_direct`. Posts of your local accounts are never sent, at any setting.
- Polls' votes, deletions, undos, likes, reactions, and boosts are never asked about, so stopping a plugin cannot stop a
  deletion from arriving. A thread fetch or a boost's fetch can still bring in the same post another way.

What each answer does to a post:

- `drop` on a `Create` stores nothing and creates no notification or timeline entry, and a redelivery is ignored as a
  duplicate. `drop` on an `Update` makes no revision and keeps the previous one.
- `flag` files one report per decision, with your instance actor as the reporter, the reason's category (`spam` and
  `malware` as Spam, `illegal` as Legal, `harassment`, `sexual`, and `violence` as Violation, and anything else as Other), and
  the plugin's note as the comment. The report says that a review filed it. It is not forwarded, and resolving it notifies
  nobody. Each decision files its own report, so a wave of spam can fill the queue.
- `mark_sensitive` marks the post, not the revision: the mark stays on later revisions, whatever they are answered, until a
  moderator cancels the decision. It shows attachments as sensitive and leaves the text alone, so a post with no attachment
  looks no different, and other servers do not see it.

## Sign-ups

`signup.check` runs on sign-ups from the registration page and through a login provider, after the CAPTCHA and the
[sign-up block list](moderation.md), so a request either of them refuses never reaches the plugin. A sign-up with an
invite code never calls it. It sends the email address the applicant entered and the client's IP address; the account does
not exist yet, so the key of the subject is the ID the account would get, and an applicant who tries again is a new subject.

The answers act as the block list's do when the hook enforces:

- `require_approval` leaves the sign-up waiting under **Moderation** > **Registrations**, as a block with the same severity
  does, in every registration mode. Approve or reject it there: cancelling the decision does not change the registration.
- `reject` does nothing until you turn on `hook_signup_check_reject` (**Server settings** > **External review**, off by
  default). While it is off, a `reject` is recorded and the sign-up goes on. While it is on, the applicant sees the same fixed
  text as for a block ("This server cannot accept this sign-up."). No account is written, so cancelling such a decision only
  changes its record, and the applicant signs up again.

Applicants never see the reason or the note of a decision.

The applicant's request waits for the plugin for up to `plugin_signup_check_timeout` (2 seconds by default; keep it short, as
a person is waiting). The wait is not part of a database transaction. A sign-up that later fails, such as for a taken
username, keeps its decision until the retention removes it.

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

Each process makes at most 8 calls of a plugin at once, and a call that waits its turn past the hook's deadline counts as a
timeout. Waits show in `my.plugin.slot_wait` ([Running](running.md)). Keep `plugin_inbound_status_timeout` short: while a call
waits, later jobs from the same actor wait with it.

In `observe` a failure never holds anything up. Moderators get a notification once a day per hook when its calls keep failing.
The metrics are `my.hook.decisions`, `my.hook.failures`, and `my.hook.duration` ([Running](running.md)).

## The decisions

The server records one decision per hook and subject, so a job that runs again reads it and does not call the plugin again.
A decision holds the hook, the key of the subject, the mode, the outcome, the reason, the note, and how long the call took. It
holds no text of a post, no email address, and no IP address.

**Moderation** > **Automated reviews** (`manage_reports`) shows the counts and waits of the last 7 days, and what the hooks
stopped, would have stopped in `observe`, or could not check. Cancelling a decision there goes in the audit log as
`hook_decision_overridden`. A `reject` of `local.status` is not cancelled there: its author appeals it.

What cancelling does depends on the answer. A `mark_sensitive` loses its mark, and a mark from a moderation action on the same
post stays. A `flag` keeps its report, which you resolve as usual. A `drop` of `inbound.status` processes the received activity
again without asking the plugin, but only for the 7 days the server keeps a received activity; after that, the screen says so
and cancelling only records the cancellation.

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

Calls, their cost, and sent data grow with the volume the server receives, on `inbound.status` above all: every post in scope
is one call, and every revision another. Review what you send before you turn a hook on:

- `inbound.status` and `local.status` send the text of posts, and refuse plain TCP, on loopback too. `inbound.status` sends
  what the remote server sent: the text, the content warning, the addresses and descriptions of attachments (no media), poll
  options, hashtags, the author, the visibility, and how many accounts the post names and how many of them are local.
  It sends nothing of the post it replies to. The message `InboundStatus` of the `HookService` protocol has the fields.
- `signup.check` sends the email address and the IP address of each applicant, and refuses plain TCP.
- `inbound.follow` sends the two parties of a follow.
- `media.check` sends a small processed version of the media, never the original.
