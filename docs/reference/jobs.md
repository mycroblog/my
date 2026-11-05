# Jobs

Every job type the server runs and every job it starts on a schedule: what the job does, what starts it, how it retries, and what running it
again by hand does. Use this page to read the types on the admin jobs page, in `my job list`, and in the `my.job.*` metrics. How the queue runs, and the commands and pages that manage it, are in
[The job queue and failed (dead) jobs](../operations/running.md#the-job-queue-and-failed-dead-jobs).

The server has 93 job types. 39 periodic jobs start some of them on a schedule; [Periodic jobs](#periodic-jobs) lists those.

A worker can run only some of these types: [Splitting workers by job type](../operations/running.md#splitting-workers-by-job-type).

## Reading the tables

**Started by** says what puts the job in the queue. A *schedule* is listed under [Periodic jobs](#periodic-jobs).

**On failure** says what the job does when it cannot finish:

| Value | Meaning |
| --- | --- |
| Default | Up to 10 runs. The wait starts at 30 seconds and doubles after each failed run, up to 24 hours, with up to 20% taken off at random. After the last run the job becomes a dead job. |
| Transactional mail | Up to 6 runs over about 15 minutes, since a late link is of little use. |
| Mail | The default retries for a temporary failure. A permanent rejection, or a message that is not valid, makes it a dead job at once. |

For every type that has a payload, a payload that cannot be read makes the job dead at once: running it again fails the same way.

**Run again** says what `my job retry`, or Retry on the jobs page, does with a dead job of the type. All job handlers are written to be repeated, since a job can run more than once anyway, but what repeating means varies:

| Value | Meaning |
| --- | --- |
| Safe | Work that is already done is not done again. |
| Next run | Safe, and rarely needed: the next scheduled run does the same work. Delete the dead job once you have fixed the cause. |
| Sends again | Safe for this server, but a message that already went out can go out again. A mail server or push service offers no way to send a message only once, so a run that stops after the server took the message and before the job finished sends it a second time. |

A job type can also be registered or do its work only with some settings; **Needs** in the tables and [Jobs that depend on settings](#jobs-that-depend-on-settings) say which.

## Federation

| Job type | What it does | Started by | On failure | Run again |
| --- | --- | --- | --- | --- |
| `federation.inbox` | Processes one activity received from another server. Activities of one actor run in order. | The inbox accepting an activity | Default. An activity that can never be processed is dead at once. | Safe. What it already recorded stays as it is, and an answer this server sends, such as the Reject of a refused Follow, goes out once. An Accept or Reject that does not name a known follow applies to the follow between the two accounts now. After 7 days the activity's document is gone and the job is dead at once. |
| `federation.fanout` | Works out the inboxes of one outgoing activity, one batch at a time, and queues a `federation.deliver` for each new delivery. Activities whose object is gone or that the federation mode restricts end without error. | A local post, boost, follow, profile change, or other activity being recorded | Default. An unknown activity is dead at once. | Safe |
| `federation.deliver` | Signs and sends one delivery. Skips a suspended or unavailable server. A server behind an open circuit breaker counts as a failed attempt. | `federation.fanout` | Up to 9 runs over about 4 days: 1 minute, 5 minutes, 30 minutes, 2 hours, 12 hours, then 24 hours. A missing delivery, activity, or signing key is dead at once. | Sends again. Inboxes ignore an activity they have seen. |
| `federation.update_poll` | Sends the current counts of a local poll to other servers. Votes that arrive while it waits share one job. | A vote in a local poll, after a short delay, or the end of the poll | Default | Safe |
| `federation.fetch_object` | Fetches one remote post and queues the fetches of its ancestors and replies, within the limits in its payload. A post that cannot be fetched or is not taken ends the job. | A post whose thread is incomplete | Default, only for failures of this server's stores | Safe |
| `federation.fetch_outbox` | Fetches the first page of a remote account's outbox and stores its recent public posts. At most one per account per day. | A local user viewing a remote account | Default, only for failures of this server's stores | Safe |
| `federation.fetch_featured` | Fetches a remote account's featured collection and makes its pins match. A collection that cannot be fetched leaves the pins as they are. | A remote actor being stored, or an activity about its pins | Default, only for failures of this server's stores | Safe |
| `federation.verify_quote` | Fetches the post a remote post quotes and verifies the quote's approval. Without an approval the quote stays pending. | A remote post with a quote | Default, only for failures of this server's stores | Safe |
| `federation.refetch_status` | Fetches one stored remote post again and records what its origin serves now: a revision, poll counts, or its deletion. | `federation.refetch_walk`, or `my admin refetch status` | Default, only for failures of this server's stores | Safe |
| `federation.refetch_walk` | Walks the remote posts of an account or domain, newest first, and queues a `federation.refetch_status` for each, one batch per minute. | `my admin refetch account` or `domain` | Default | Safe |
| `federation.refresh_actor` | Fetches one remote actor again. A fetch failure is recorded on the actor, and the periodic refresh tries again later. | `federation.refresh_stale_actors` | Default, only for failures of this server's stores | Safe |
| `federation.refresh_stale_actors` | Walks the stale remote actors in batches and refreshes those that local accounts follow. | Schedule | Default | Next run |
| `federation.observe_instance` | Reads the NodeInfo of a server seen for the first time. | A document from a new server being stored | Default. A server without NodeInfo ends the job. | Safe |
| `federation.probe_instances` | Fetches the NodeInfo of unavailable servers, 50 at a time, and marks those that answer as available again. | Schedule | Default | Next run |
| `federation.purge_records` | Deletes the inbox and delivery records past their retention: most after 30 days (the first delivery of a post stays until 30 days after the post or its author is deleted), and what is known about other servers after 90 days. | Schedule | Default | Next run |

## Posts, media, and link cards

| Job type | What it does | Started by | On failure | Run again |
| --- | --- | --- | --- | --- |
| `media.process` | Re-encodes one upload into its variants, records the result, and deletes the received file. Content the processor rejects records a failed result. | An upload, or a change of what a file is used for | Default | Safe |
| `media.purge_unattached` | Deletes the uploads that nothing attached in time. | Schedule | Default | Next run |
| `media.purge_replaced` | Deletes the old versions of images that a newer version replaced more than `media_replaced_retention` ago. An image's current version is never deleted. | Schedule | Default | Next run |
| `media.reprocess_walk` | Walks the uploaded images last processed before a time and queues a `media.reprocess` for each, one batch per minute. | `my admin media reprocess` | Default | Safe |
| `media.reprocess` | Processes one image again with the current settings, from its large version. An image processed since the time, or that the processor no longer accepts, stays as it is. | `media.reprocess_walk` | Default | Safe |
| `media.purge_remote_cache` | Deletes the copies in the remote media cache past their expiry or over the size limit. | Schedule | Default | Next run |
| `content.close_poll` | Closes one poll when its end has passed. A job that runs early waits until the end. | A post with a poll, due at the poll's end | Default | Safe |
| `content.fetch_link_card` | Fetches the card of the URL a post links. A busy host, or a host that answers 429 or 503, postpones the job without counting as a failure. A `robots.txt` that cannot be read retries an hour later. Details are in [Link previews](../operations/link-previews.md). | A post created or revised, after a delay for a remote post | Default, for failures of this server's stores | Safe |
| `content.purge_link_cards` | Deletes the cards no post refers to and the images no card uses. | Schedule | Default | Next run |
| `content.auto_delete_statuses` | Deletes local posts past their authors' automatic deletion policies, as their authors would. | Schedule, then itself for the next batch | Default | Next run |
| `content.purge_remote_statuses` | Deletes remote posts past their retention, in batches, and after 30 days the records of those deletions that no interaction or notification still needs. | Schedule, then itself | Default | Next run |
| `content.purge_relayed_statuses` | Deletes the posts that only relays brought in past their retention, in batches. | Schedule, then itself | Default | Next run |
| `content.purge_remote_emojis` | Deletes the remote custom emoji nothing refers to, in batches. | Schedule, then itself | Default | Next run |

## Accounts, follows, and imports

| Job type | What it does | Started by | On failure | Run again |
| --- | --- | --- | --- | --- |
| `account.execute_deletions` | Deletes the accounts whose scheduled deletion is due, from a ban or from the owner's request, and the accounts that their owners deactivated 30 days ago. An account that came back, or whose deletion was cancelled meanwhile, is left alone, and so is a deactivated account that is the last administrator, with a warning in the log. | Schedule | Default | Next run |
| `account.erase_deleted` | Erases the data of deleted accounts, local and remote, in bounded batches. First fetches a remote account again when only a `410 Gone` marked it deleted. | Schedule, then itself | Default | Next run |
| `account.prune_remote_profiles` | Shrinks the profile history of remote accounts that nothing local relates to, in batches, and deletes checks of remote handles after 90 days, keeping the newest of each account. | Schedule, then itself | Default | Next run |
| `graph.expire_mutes` | Deletes the mutes that have expired. | Schedule | Default | Next run |
| `graph.migrate_follows` | Moves one page of the relationships of a moved account, waiting 10 seconds between pages to spread its deliveries. | An account move being recorded | Default. A step whose data is gone is dead at once. | Safe. It does nothing once the move is cancelled or replaced. While the move is in force, a run much later also moves the accounts that follow, block, or mute the account by then. |
| `identity.process_import` | Carries out one batch of a user's data import and records each item's result. An item that keeps failing is recorded as failed after 5 tries, and the import goes on. | A user's import, then itself | Default | Safe |
| `identity.expire_imports` | Removes the imports past their lifetime. | Schedule | Default | Next run |
| `identity.build_export` | Builds the archive of a user's data export. | A user's export request | Up to 8 runs: 5 that build, 3 that only record the failure, with the default waits. A failed export is recorded for the user, not left as a dead job. | Safe. An export with a result stays as it is. |
| `identity.expire_exports` | Removes expired archives and records, and records as failed an export still without a result after 24 hours. | Schedule | Default | Next run |
| `identity.purge_unused_oauth_clients` | Deletes the dynamically registered OAuth clients that nothing uses. | Schedule | Default | Next run |
| `identity.purge_origins` | Deletes the session and registration origins past their retention. | Schedule | Default | Next run |
| `identity.purge_auth_records` | Deletes the sessions, authorization codes, token families, and one-time tokens that expired over 30 days ago. A token family goes only when its last token did. | Schedule | Default | Next run |

## Email

Mail jobs run when `mail` is set to a provider. With `mail = none`, only `identity.verification_email` and `identity.password_notice_email` run: they send nothing and log the category of the message, or, with `profile = development`, the whole message. If the address or the state they check has changed, they end without sending.

| Job type | What it does | Started by | On failure | Run again |
| --- | --- | --- | --- | --- |
| `identity.verification_email` | Issues a token and sends the verification link. Sends nothing more than an hour after the request. | Registration, or a request to send the link again | Transactional mail | Sends again, within the hour, with a new link |
| `identity.password_reset_email` | Issues a token and sends a password reset link. Sends nothing more than 15 minutes after the request, or while sign-in is limited to login providers. | A password reset request | Transactional mail | Sends again, within the 15 minutes, with a new link |
| `identity.password_notice_email` | Tells a user that their password changed or was removed. | A password change or removal | Mail | Sends again |
| `identity.deactivation_email` | Tells a user that their account is deactivated, with the date of its deletion. Sends nothing once the user has come back. | A user deactivating their account | Mail | Sends again |
| `notification.email` | Sends the email of one notification that stayed unread for 15 minutes. Checks the read position, the user's setting, and that the notification is still shown. | A notification with email on, due 15 minutes later | Mail | Sends again |
| `notification.moderator_email` | Sends a moderator notification by email, without waiting. | A moderator notification | Mail | Sends again |
| `notification.supporter_email` | Sends a supporter notification by email, unless the subscriber has deactivated their account since. | A subscription's transition, or a plan being retired | Mail | Sends again |
| `identity.closure_email_fanout` | Queues one service closure email for every user with a verified primary address, page by page, each user once. Above 1,000 of them, the first job only queues jobs of the same type for up to 1,000 each. | The closure being recorded, or `identity.queue_closure_reminders` | Mail | Safe |
| `identity.closure_email` | Sends one service closure email to one user. | `identity.closure_email_fanout` | Mail | Sends again |
| `identity.queue_closure_reminders` | Queues the fan-out of each closure reminder whose time has come. | Schedule | Default | Next run |

## Notifications

| Job type | What it does | Started by | On failure | Run again |
| --- | --- | --- | --- | --- |
| `notification.push` | Sends one notification to one Web Push subscription, if the subscription is still valid, the notification is still shown, and the recipient has not deactivated their account since. A subscription the push service reports gone, or whose sign-in has ended, is removed. | A notification, for each subscription that wants its type | Default, using the wait the push service asks for. A refusal, or a message that cannot be sent, is dead at once. | Sends again. A device shows a repeat in place of the first message. |
| `notification.post_subscribers` | Notifies the subscribers of a post's author. Above 1,000 of them, the first job only queues jobs of the same type for up to 1,000 each. | A new local post, or a delivered remote post | Default | Safe. Each notification is created once. |
| `instance.notify_announcement` | Creates the announcement notifications for local users. Waits if the announcement has not started, and ends if it has ended. Above 1,000 of them, the first job only queues jobs of the same type for up to 1,000 each. | An announcement getting the time it starts to show | Default | Safe. Each notification is created once. |
| `notification.closure_fanout` | Creates the service closure notification for every local user. Above 1,000 of them, the first job only queues jobs of the same type for up to 1,000 each. | The closure being recorded | Default | Safe. Each notification is created once. |
| `notification.closure_due` | Creates the notification for administrators once the closing date has passed and the announcement is still in effect. | The closure being recorded, due at the closing date | Default | Safe. The notification is created once. |
| `notification.purge_error_group_notices` | Deletes the hourly error notices past their retention. | Schedule | Default | Next run |

## Moderation and service closure

| Job type | What it does | Started by | On failure | Run again |
| --- | --- | --- | --- | --- |
| `moderation.sever_domain` | Cuts the follows of local accounts with a suspended domain, and records what each account lost. Above 1,000 of them, the first job only queues jobs of the same type for up to 1,000 each. Running one again repeats nothing it did. | A domain suspension | Default. An action that is gone or is not a domain suspension is dead at once. | Safe |
| `moderation.expire_restrictions` | Removes expired account restrictions, and tells other servers when a suspension ends. | Schedule | Default | Next run |
| `moderation.purge_records` | Erases the moderation records past their retention, including released retention holds. | Schedule | Default | Next run |
| `identity.reject_for_closure` | Rejects the pending registrations when the closure's notice period starts. Stops if the closure is withdrawn. | The closure being recorded | Default | Safe |
| `instance.closure_delete_actors` | Records the `Delete` of one page of local accounts and passes them to the fan-out. | The closure being recorded, then itself | Default | Safe |
| `instance.check_closure_delivery` | Records that the closure's deletes have all been delivered. | Schedule | Default | Next run |

## Supporters and costs

| Job type | What it does | Started by | On failure | Run again |
| --- | --- | --- | --- | --- |
| `supporters.ensure_prices` | Makes the prices of a plan's current terms at the payment provider, and archives those no longer used. | A plan's terms changing | Default | Safe. The provider call is idempotent. |
| `supporters.sync` | Reads a subscription again from the provider and applies what it finds. | Provider notices, a due deadline, a checkout | Default | Safe. It only re-reads and applies. |
| `supporters.command` | Sends a cancellation to the provider, removes what a failed plan change left to collect, expires the payment pages of a closed pending subscription, or cancels and refunds a second subscription of one payment. | A user's or the server's cancellation, a pending subscription closing, a second subscription found, each reconciliation while one is not settled | Default | Safe. A repeat finds the page expired, the subscription ended, and the charge refunded. |
| `supporters.sync_due` | Queues `supporters.sync` for the subscriptions whose deadline has passed. | Schedule | Default | Next run |
| `supporters.cancel_for_closure` | Cancels the subscriptions when a service closure is announced, and tells the subscribers. | The closure being recorded | Default | Safe |
| `supporters.apply_payment_event` | Applies one notice from the payment provider by re-reading what it points at. | A verified webhook from the provider | Default | Safe. Duplicates and reordering come down to re-reading. |
| `supporters.reconcile_payments` | Compares the provider's records with this server's, within a request limit, and continues in a later job when it uses them up. A provider failure is recorded and waits for the next run. | Schedule, then itself | Default, for failures of this server | Next run |
| `supporters.purge_payment_receipts` | Deletes old webhook receipts and expired checkout claims. | Schedule | Default | Next run |
| `finance.import_billing` | Imports the charges of one source of `billing_sources`. A run that already succeeded for its key does nothing, and a source over its request limit ends without retrying. See [Importing hosting costs](../operations/billing-import.md). | Schedule, or an administrator, at most once an hour | Default, for throttling or an unavailable source | Next run |

## Voice

| Job type | What it does | Started by | On failure | Run again |
| --- | --- | --- | --- | --- |
| `voice.apply` | Applies one change of a room to its media transport: open, allow or stop audio, mute, remove, or close. Changes of one room run in order. | A committed change of a room | Default. Once it is dead, `voice.sweep` reconciles the room. | Safe. Opening does nothing for a room that ended, and allowing or stopping audio applies the participant's role now. A mute mutes the session again, and a removal or close that is done stays done. |
| `voice.apply_media_event` | Applies one verified event from a media server to its room. | A webhook from a media server | Default | Safe. It compares the event with the room and the media server as they are now. |
| `voice.sweep` | Evaluates every open room: participants' expiry, hosts' grace, rooms of a voice chat turned off, and rooms over the time limit. | Schedule | Default | Next run |
| `voice.check_account` | Applies a change of an account to its participation: a suspended or deleted account leaves, and a host who is no longer eligible hands over. | A suspension, deletion, or restriction of an account in a room | Default | Safe. It reads the account and the room as they are now. |
| `voice.check_block` | Applies a block between two participants of a room: the one who is not a host or co-host leaves. | A block | Default | Safe. A block that was lifted makes nobody leave. |
| `voice.check_hosts` | Checks the eligibility of every open room's host. | A change of roles or of `voice_room_starters` while rooms are open | Default | Safe. It reads the hosts and the roles as they are now. |
| `voice.notify_started` | Tells the local followers of a host who can see a room that it opened. Above 1,000 of them, the first job only queues jobs of the same type for up to 1,000 each. | A room opening | Default | Safe. Each notification is created once. |
| `voice.purge_records` | Erases the participants' records of rooms that ended past their retention, except those a report or a hold preserves. | Schedule | Default | Next run |

## Full-text search

Each of these types exists on every server and does nothing: nothing enqueues them yet. A server with `search = none`, the default, never will.

| Job type | What it does | Started by | On failure | Run again |
| --- | --- | --- | --- | --- |
| `search.sync_status` | Does nothing. | Nothing | Default | Safe |
| `search.sync_author` | Does nothing. | Nothing | Default | Safe |
| `search.sync_domain` | Does nothing. | Nothing | Default | Safe |
| `search.narrow` | Does nothing. | Nothing | Default | Safe |
| `search.rebuild` | Does nothing. | Nothing | Default | Safe |

## Webhooks

| Job type | What it does | Started by | On failure | Run again |
| --- | --- | --- | --- | --- |
| `webhook.deliver` | Sends one delivery to an endpoint. Sends nothing if the endpoint is not enabled or the delivery already succeeded, unless it is a redelivery. | An event an endpoint subscribes to, or a redelivery | Up to 8 runs over about 27 hours. See the [Webhook reference](webhooks.md). | Sends again, only if it has not succeeded |
| `webhook.purge_messages` | Deletes messages past their retention, with their deliveries and attempts. | Schedule | Default | Next run |

## Upkeep

| Job type | What it does | Started by | On failure | Run again |
| --- | --- | --- | --- | --- |
| `queue.purge_dead_jobs` | Deletes dead jobs after `dead_job_retention` and attempt records after `job_attempt_retention`. | Schedule | Default | Next run |
| `kv.purge_expired` | Deletes the expired entries of the key-value store. | Schedule | Default | Next run |
| `metrics.snapshot` | Computes the gauges of the instance, stores them, and exports them once. | Schedule | Default | Next run |
| `projection.verify` | Rebuilds every projection, writing only what drifted, and logs those that did. One that fails does not stop the others. | Schedule | Default | Next run |
| `diagnostics.purge_error_counts` | Deletes the hourly error counts past their retention. | Schedule | Default | Next run |
| `hook.purge_decisions` | Deletes the decisions of the [decision hooks](../operations/decision-hooks.md#the-decisions) past `retention_hook_decisions`, except those it keeps. | Schedule | Default | Next run |
| `stream.sweep` | Deletes the per-process resources that ended processes left on the broker. | Schedule | Default | Next run |

## Periodic jobs

The scheduler queues one job per period, however many processes run it, and the job then runs like any other. With `queue = cloudtasks` or `queue = sqs` the platform's scheduler starts the periods; see
[The job queue and failed (dead) jobs](../operations/running.md#the-job-queue-and-failed-dead-jobs).

| Periodic job | Every |
| --- | --- |
| `voice.sweep` | 1 minute |
| `moderation.expire_restrictions` | 5 minutes |
| `supporters.sync_due` | 5 minutes |
| `instance.check_closure_delivery` | 10 minutes |
| `metrics.snapshot` | 15 minutes |
| `stream.sweep` | 15 minutes |
| `account.erase_deleted` | 1 hour |
| `account.execute_deletions` | 1 hour |
| `content.auto_delete_statuses` | 1 hour |
| `content.purge_relayed_statuses` | 1 hour |
| `content.purge_remote_statuses` | 1 hour |
| `federation.purge_records` | 1 hour |
| `federation.refresh_stale_actors` | 1 hour |
| `graph.expire_mutes` | 1 hour |
| `identity.queue_closure_reminders` | 1 hour |
| `identity.purge_unused_oauth_clients` | 1 hour |
| `identity.expire_exports` | 1 hour |
| `identity.expire_imports` | 1 hour |
| `identity.purge_auth_records` | 1 hour |
| `identity.purge_origins` | 1 hour |
| `kv.purge_expired` | 1 hour |
| `media.purge_unattached` | 1 hour |
| `media.purge_replaced` | 1 hour |
| `media.purge_remote_cache` | 1 hour |
| `moderation.purge_records` | 1 hour |
| `queue.purge_dead_jobs` | 1 hour |
| `voice.purge_records` | 1 hour |
| `webhook.purge_messages` | 1 hour |
| `account.prune_remote_profiles` | 24 hours |
| `content.purge_link_cards` | 24 hours |
| `content.purge_remote_emojis` | 24 hours |
| `diagnostics.purge_error_counts` | 24 hours |
| `hook.purge_decisions` | 1 hour |
| `federation.probe_instances` | 24 hours |
| `finance.import_billing` | 24 hours, one schedule per source of `billing_sources`, named `finance.import_billing.<source>` |
| `notification.purge_error_group_notices` | 24 hours |
| `projection.verify` | 24 hours |
| `supporters.purge_payment_receipts` | 24 hours |
| `supporters.reconcile_payments` | `payments_reconcile_interval` (6 hours by default) |

## Jobs that depend on settings

Most job types exist on every server. These do not:

| Setting | Job types |
| --- | --- |
| `mail` set to a provider (not `none`) | `notification.email`, `notification.moderator_email`, `notification.supporter_email` |
| `vapid_private_key` set | `notification.push` |
| `metrics_exporter` set (not `none`) | `metrics.snapshot` |
| `stream = sns` or `pubsub` | `stream.sweep` |
| `payments` set | `supporters.apply_payment_event`, `supporters.reconcile_payments`, `supporters.purge_payment_receipts` |
| `billing_sources` set | The `finance.import_billing` schedules. The job type itself always exists. |

A job whose type has no handler on the process that claims it, for example after you change one of these settings, counts as a failure, retries with the default waits, and becomes a dead job. It appears as `other` in the `my.job.*` metrics. Restore the setting and run it again, or delete it.

These types always exist but do nothing without their setting: `supporters.ensure_prices`, `supporters.sync`, and `supporters.command` without `payments`, `voice.apply` without a voice transport, and the `search.*` types, which do nothing on any server for now ([Full-text search](#full-text-search)). The other `voice.*` jobs run with the voice chat off, so that `voice.sweep` can end rooms left open.
