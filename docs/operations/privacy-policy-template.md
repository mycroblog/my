# Privacy policy template

A template for operators: a privacy policy that describes what this server keeps, why, for how long, and who receives it.

**This is a template and not legal advice.** It describes what the software does with its default settings. It cannot know how you run your instance, and it does not decide which laws apply to you. What a policy must say depends on your jurisdiction and on your users' (for example the GDPR in the EU, the UK GDPR, or Japan's Act on the Protection of Personal Information). Have it checked for your jurisdiction before you publish it, and treat the legal bases and the transfer statement below as questions to settle, not as answers.

The record of processing activities that goes with it is in [Record of processing activities template](processing-record-template.md).

## How to use it

1. Copy [the template](#the-template) into a file, `privacy-policy.md`.
2. Replace every `{{...}}`. A value in the braces, such as `{{180 days}}`, is the default of the software: confirm it against your instance ([The periods and their settings](#the-periods-and-their-settings)) and remove the braces.
3. A row or a sentence that starts with `{{IF ...}}` applies only to that setup ([The services you choose](#the-services-you-choose)). Delete it when it does not apply. When it does, remove the marker.
4. Check that nothing is left: `grep -n '{{' privacy-policy.md` prints no line.
5. Publish it with `my instance set privacy_policy - < privacy-policy.md`, or under "Terms and policies" (`/admin/documents`) in the administration. Readers find it at `/about/privacy` ([instance settings](instance-settings.md#terms-and-privacy-policy)).

A written privacy policy adds a required agreement checkbox to the registration screen. That checkbox shows that someone read the document. Do not assume that it is consent to processing where your law asks for consent.

The text has one value for every locale. If you have readers in several languages, write the languages you need into the one text.

Review the policy when you change a setting below or a service, and after each upgrade whose release notes mention personal data.

## The periods and their settings

`my instance list` shows every instance setting and whether it is the default. A change to a period takes effect at the next run of its hourly job. Values outside the range are refused.

| Record | Default | Instance setting | Range (days) | Starts from |
| --- | --- | --- | --- | --- |
| Sign-in IP address and User-Agent | 180 days | `session_origins_retention_days` | 90-3650 | The sign-in |
| Registration IP address | 180 days | `registration_origins_retention_days` | 90-3650 | The registration |
| Reports | 365 days | `reports_retention_days` | 180-3650 | The last resolution |
| Actions against accounts, and appeals | 365 days | `moderation_actions_retention_days` | 180-3650 | The end of the action |
| Audit log | 730 days | `audit_log_retention_days` | 365-3650 | The record |
| Who took part in a voice chat room | 90 days | `voice_participants_retention_days` | 30-3650 | The end of the room |
| The decisions of an external review, when you use one | 90 days | `retention_hook_decisions` | 30-3650 | The decision |
| Remote posts nothing here refers to | 90 days | `remote_status_retention_days` | 1-3650 | The post's publication |
| Any remote post | No limit (`0`) | `remote_status_max_retention_days` | 0-3650 | The post's publication |
| Remote posts only a relay brought | 7 days | `relay_status_retention_days` | 1-3650 | The post's publication |
| Copies of remote media | 1 day, and none kept while `remote_media_policy` is `none` | `remote_media_cache_ttl_seconds`, `remote_media_policy` | 60-31536000 seconds | The fetch |
| Translations of posts | 30 days | `translation_retention_days` | 1-3650 | The translation. Reading it does not extend the period, and it goes when the post is deleted |

These are settings of the deployment ([configuration reference](../reference/configuration.md)), not instance settings:

| Record | Default | Setting |
| --- | --- | --- |
| Uploads not attached to anything | 24 hours | `media_unattached_retention` |
| Old versions of images, after a newer version replaces them | 30 days | `media_replaced_retention` |
| Jobs that failed for good | 30 days | `dead_job_retention` |
| Records of job runs that did not succeed | 30 days | `job_attempt_retention` |

These periods are fixed by the software, and no setting changes them:

| Record | Period |
| --- | --- |
| Erasing a deleted account | 7 days after the deletion; up to 30 days for a remote account when only a `410 Gone` from its server said so and the server does not answer again |
| Deleting a deactivated account | 30 days after the deactivation |
| Sign-up blocks | 30 days after the lift or the expiry; without either, as long as they are in effect |
| Sessions, tokens, and authorization codes | 30 days after they expire |
| Archives of data exports, and data imports | 7 days |
| Records of activities received from other servers | 30 days, and the document itself 7 days |
| Records of activities delivered to other servers | 30 days; the delivery of a post until 30 days after the post or its author is deleted |
| Checks of remote handles | 90 days, keeping the newest of each account |
| Webhook messages and deliveries | 14 days |
| Payment webhook receipts | 45 days |
| Hourly error counts and error notices | 90 days |

A [retention hold](moderation.md#holds) stops erasure for one account, so every period above can run longer for an account under a hold. If you use holds, say so, as the template does.

## Records with no automatic end

The server removes these only when an account is erased, or never. A law may ask you to justify keeping them, or to remove them by other means, so decide what your policy says about each.

- The rows of an active account: email addresses, past profile versions, past post versions, notifications, follows, and the registration application of a rejected user. An account that nobody deletes stays.
- What stays after erasure: the account's URI and username (so that neither can be reused), its lifecycle events, the record that a post was deleted, moderation records until their own period ends, invites and role grants, account moves, voice chat rooms without their names and descriptions.
- Supporter records (subscriptions, state changes, payments, refunds, chargebacks), kept for accounting and for reconciling with the provider. The server does not delete a customer at the payment provider: do that in the provider's dashboard if your obligations allow it.
- An action with no end (a `ban`, or a `suspend` without an end time), until it is lifted. Open reports.
- Remote accounts, until their server deletes them.
- The latest report of each error group, which the next report of that group replaces. It is scrubbed by pattern, so a fragment of user text can remain.
- Backups, your logs, and the logs of your platform and proxy. The server does not manage these: [backup](backup.md#how-often-and-where) describes the generations to keep, and a restored backup brings erased data back.

## The services you choose

Each service that receives personal data appears in the policy only if you turned it on. Fill the table of section 6 from this list, and read the service's own policy and contract: what the service does with the data after it arrives is not something this document can tell you.

| Marker | Setting | What the service receives |
| --- | --- | --- |
| Hosting | `database`, `blob`, `queue`, `key_manager` | Everything the database and blob storage hold, in whatever form the provider stores it. Job payloads in a cloud queue hold IDs and categories, without addresses, message bodies, or secrets. |
| `{{IF cdn}}` | `media_origin` | Readers' IP addresses and requests for media ([CDN](cdn.md)). |
| Proxy | Your reverse proxy or load balancer, which the server needs for TLS | Every request with its IP address, under the logging you configure. |
| `{{IF mail}}` | `mail` is `smtp` or `ses` | Recipient addresses, subjects, and bodies. Notification email carries the same text as the notification ([email](email.md)). |
| `{{IF captcha}}` | `captcha` is not `none` | The IP address, browser and device information, cookies, and widget interactions of everyone who opens the sign-in, sign-up, or password-reset request page, directly from their browser. The server sends the service only the token and adds no IP address ([CAPTCHA](captcha.md#what-to-write-in-the-privacy-policy)). |
| `{{IF login providers}}` | [Login providers](login-providers.md) | The provider sees the sign-in. The server receives the provider's identifier for the person, keeps that, and at sign-up uses the email address and username the provider gives. |
| `{{IF stripe}}` | `payments = stripe` | The email address that prefills Checkout, the plan, and the local subscription ID. Stripe collects the payment details on its own pages ([Stripe](payments-stripe.md#card-data-and-pci-dss)). |
| `{{IF payjp}}` | `payments = payjp` | The email address and the plan. The browser loads payjp.js on the server's payment pages, the card goes into PAY.JP's iframe, and the name and email address go to PAY.JP for 3-D Secure ([PAY.JP](payments-payjp.md#card-data-and-pci-dss)). |
| `{{IF error reports}}` | `errors_exporter` | Scrubbed reports without account, IP address, body, or header. Scrubbing matches patterns, so a fragment of user text can remain ([error reporting](error-reporting.md#what-scrubbing-does-and-does-not-guarantee)). |
| `{{IF telemetry}}` | `metrics_exporter`, `traces_exporter` | Metrics and spans without accounts or client IP addresses. Spans of outbound requests name the host names of remote servers ([running](running.md)). |
| `{{IF voice media server}}` | `voice` is `livekit` or `sora` | Participants' IP addresses, their audio while speaking, a session ID, and the room ID. The account's ID is not sent ([voice chat](voice.md)). |
| `{{IF turn}}` | `voice_stun_urls`, `voice_turn` | STUN and TURN servers see participants' IP addresses, and a TURN server relays their encrypted audio. |
| `{{IF webhooks}}` | [Server webhooks](webhooks.md) | Your receiver gets event bodies. Some carry an applicant's email address or a report's comment. |
| `{{IF follow review}}` | `plugin_inbound_follow_address` is set and `hook_inbound_follow_mode` is not `off` | The addresses of the two accounts of each follow from another server, sent by the server when the follow arrives ([decision hooks](decision-hooks.md)). The recorded decision keeps the activity's address and which account sent it. |
| `{{IF sign-up review}}` | `plugin_signup_check_address` is set and `hook_signup_check_mode` is not `off` | The email address and IP address of each applicant without an invite, sent by the server when they sign up ([decision hooks](decision-hooks.md#sign-ups)). The recorded decision holds neither. Name the service in section 6 unless you run the plugin yourself. |
| `{{IF post review}}` | `plugin_local_status_address` is set and `hook_local_status_mode` is not `off` | The text of each `public`, `local`, and `unlisted` post of an account younger than `hook_local_status_account_age_days` (7 days by default; `0` is every account), with its content warning, poll options, image descriptions, hashtags, the author's username, and when the account was created, sent by the server when the post is made ([decision hooks](decision-hooks.md#local-posts)). `private` and `direct` posts are never sent. The recorded decision holds no text. An author who appeals a refused post sends its text, which the server keeps with the decision. Name the service in section 6 unless you run the plugin yourself. |
| `{{IF external review of posts}}` | `plugin_inbound_status_address` is set and `hook_inbound_status_mode` is not `off` | The text of the posts other servers send that relate to an account here, with their author, attachments' addresses and descriptions, and how many accounts they name ([decision hooks](decision-hooks.md#what-is-sent-and-what-it-costs)). Direct posts from authors nobody here follows go only with `hook_inbound_status_unfollowed_direct`. |
| `{{IF translation}}` | `translation_providers` is not empty | The text of a post when a reader asks to translate it: its body, content warning, poll options, and image descriptions. The post's address, its author, the reader, mentions, and links are not sent. By default a provider receives `public`, `local`, and `unlisted` posts. It works as your service provider, like hosting, so `local` posts reach it although they do not federate: keep the `local` sentence in section 5. A provider that keeps the text for its own use is not a service provider in this sense, and the policy must say so. |
| `{{IF translation to private and direct}}` | `translation_provider_visibilities` allows a provider `private` or `direct` | The same, for those posts. Use this marker only when you allowed it, per provider. It adds lines to sections 2 and 5, and it makes your promise about those posts wider, so announce it before you change the setting. |
| `{{IF browser translation}}` | `translation_browser` | Nothing reaches this server or a provider. The browser translates on the reader's device, and what the browser's maker does is up to that browser. |

The services are your choices, so this template does not say where each is established or what contract covers it. Confirm both yourself.

Two things reach third parties without a setting of yours:

- **Web Push.** When a user turns it on, their browser chooses the push service and gives the server its address. The message body is encrypted for that browser, so the push service should not be able to read it. It does see the subscription address, the size and time of each message, and the contact (VAPID subject) of your server, and it holds a message for up to 48 hours while the browser is offline.
- **Federation.** See section 5 of the template. `federation_mode` turns it off or limits its direction, and relays are yours to add ([relays](relays.md)).

## The template

```markdown
Last updated: {{DATE}}

This policy describes how {{OPERATOR NAME}} ("we") handles personal data on {{INSTANCE NAME}} ({{https://INSTANCE DOMAIN}}).

# 1. Who is responsible

- Controller: {{OPERATOR NAME, POSTAL ADDRESS}}
- Contact for privacy questions: {{EMAIL OR FORM}}
- {{IF you have a representative in the EU or the UK: name and contact}}
- {{IF you have a data protection officer: name and contact}}

# 2. Whom this covers

This policy covers people with an account here, people who read pages here without an account, and people on other servers whose posts and profiles reach this server.

{{IF translation}} If you are on another server, the text of your posts that reach this server can go to the translation services in section 6 when a reader here asks for a translation. Your server cannot tell you when this happens.

{{IF external review of posts}} If you are on another server, the text of your posts that relate to an account here goes to the external review in section 6 before this server accepts them. It can stop a post or mark it, or file a report for our moderators; people decide on any action against an account.

# 3. What we keep, why, and for how long

"Until your account is erased" means: deleting your account hides it at once and erases it 7 days later. A deactivated account stays hidden, can be reactivated for 30 days, and is then deleted the same way.

| Data | Why | How long |
| --- | --- | --- |
| Email addresses | Signing in, confirming the address, resetting the password, and the email you choose to get | Until your account is erased |
| Password hash, two-factor secret (encrypted) and recovery codes (hashed), passkeys, the identifier of an account you linked at a login provider | Signing you in | Until your account is erased |
| Language, time zone, default post settings, notification settings, and other preferences | Running the service the way you asked | Until your account is erased |
| Registration application: the reason you gave, the invite you used, and the decision | Deciding who joins | Until your account is erased |
| IP address at registration | Preventing abuse | {{180 days}} from registration |
| IP address and browser name (User-Agent) at each sign-in | Security and preventing abuse | {{180 days}} from the sign-in |
| Sessions and app access (stored as hashes) | Keeping you signed in | 30 days after they expire. A session lasts 30 days |
| IP address, and for password-reset requests the email address, in rate limit counters | Protecting the service | Removed automatically, within twice the window of the limit |
| Profile: display name, bio, extra fields, avatar, header, aliases, and earlier versions of them | Showing your profile, which is public | Until your account is erased |
| Posts, their earlier versions, and attached media | Publishing what you write | Until you delete them, your automatic deletion policy deletes them, or your account is erased. Uploads attached to nothing go after {{24 hours}} |
| Favourites, reactions, boosts, bookmarks, pins, poll votes, follows, blocks, mutes, lists | Running the features you use | Until you undo them, or your account is erased |
| Notifications and what you have read | Telling you what happened | Until your account is erased or the post they are about is removed |
| Web Push subscription: your browser's push address and keys | Sending you push notifications | Until you turn it off, the push service ends it, or your account is erased |
| Records that an email bounced or was reported as spam | Not sending mail that is unwanted or cannot arrive | Until your account is erased |
| Data export archives and data imports | Giving you your data, and bringing it in | 7 days |
| Reports: the comment, the posts and rules named, and moderators' notes | Moderation | {{365 days}} from the last resolution, or as long as an action refers to them. Open reports stay |
| Actions against accounts, and appeals | Moderation, and handling repeat abuse | {{365 days}} from the end of the action. An action with no end stays until it is lifted |
| Posts that a report or an action points to | Evidence for moderation | Hidden when deleted, and removed with that record |
| Audit log of staff actions, which can name accounts | Accountability of staff | {{730 days}} |
| Sign-up blocks: an email domain, a hashed email address, or an IP range | Preventing repeat abuse | While in effect, and 30 days after the lift or expiry |
| {{IF voice}} Voice chat rooms: name, description, hosts, and how they ended | Showing and moderating rooms | Kept. The name and description you wrote are emptied when your account is erased |
| {{IF voice}} Who took part in a room: participation, speakers, raised hands, invitations, removals | Moderating rooms | {{90 days}} from the end of the room, or as long as a report about the room. We record neither audio nor text chat |
| {{IF payments}} Supporter plans: the provider's IDs for you and your subscription, amounts, periods, and states. Not your card details | Running your subscription, and accounting | {{PERIOD AND REASON, e.g. as long as tax law requires}}. The link to the provider's customer goes when your account is erased |
| {{IF payments}} Your choice to appear in the list of supporters | Showing the list | Until the subscription ends or your account is erased |
| Copies of activities sent to and received from other servers | Delivering, retrying, and rejecting duplicates | 30 days. The first delivery of a post stays until 30 days after the post or its author is deleted |
| Posts and profiles of people on other servers | Showing them here | Posts: {{90 days}} when nothing here refers to them, {{7 days}} when only a relay brought them, and {{no limit}} otherwise. Profiles: until their server deletes the account. Then erased after 7 days |
| Copies of remote media | Showing it | {{Not kept, or the period you set}} |
| Server logs: request paths, which can contain a username or a post ID. Our server software writes no IP address in them | Running and debugging the service | {{PERIOD}} |
| {{IF proxy logs}} Logs of our proxy or load balancer: IP addresses and requests | Running and protecting the service | {{PERIOD}} |
| Error reports, with personal details removed by pattern | Fixing faults | The latest report of each kind of error stays until a newer one replaces it. Hourly counts: 90 days |
| {{IF webhooks}} Messages to our webhook receivers, which can carry an applicant's email address or a report's comment | Connecting our own tools | 14 days here. {{PERIOD AT THE RECEIVER}} |
| {{IF post review}} The text of a refused post that you send with an appeal, and the decision | Letting staff review the appeal | {{90 days}} from the refusal, and longer while you can still appeal or staff have not decided. Removed when your account is erased |
| {{IF translation}} Translations of posts that readers asked for, which every reader who may see the post can reuse | Showing a translation again without sending the post again | {{30 days}} from the translation, and no longer than the post |
| {{IF translation}} How many characters your requests sent for translation in a month | Keeping translation within its limits | Until the end of the following month, or until your account is erased |
| Backups of all the above | Restoring the service after a failure | {{PERIOD}}. Data you erased stays in a backup until the backup expires |

We can stop erasure for one account when a court order or a legal request requires it. The records of that account then stay until the hold is released.

After erasure we keep the account's address and username so that nobody can reuse them, the record that its posts were deleted, its moderation records until their period ends, invites and roles it took part in, moves it made, and {{IF payments}} the supporter records above.

# 4. Why we are allowed to

{{CHOOSE WITH YOUR ADVISER. A common starting point under GDPR Article 6(1), not a conclusion:}}

- Running your account and the features you use: {{performance of a contract, point (b)}}.
- Security, preventing abuse, moderation, the audit log, logs, and backups: {{legitimate interests, point (f): keeping the service safe and working}}.
- Storing and showing what people on other servers publish: {{legitimate interests, point (f)}}.
- Records the law makes us keep: {{legal obligation, point (c)}}.
- {{IF you rely on consent for anything: what, and how to withdraw it}}.

You need an email address {{OR a login provider}} to create an account here, and without one we cannot provide an account.

# 5. What other people and other servers receive

**This server is part of a federated network.** Other servers run by other people, in other countries, receive your data as part of how the service works, and their own policies apply to it. We cannot make them delete it.

| What | Who receives it |
| --- | --- |
| Your profile, including your avatar, header, and aliases | Anyone, on the web and through other servers |
| `public` and `unlisted` posts | Anyone with the link. The servers of your followers and of the people you mention. {{IF relays}} `public` posts also go to relays we joined, and through them to other servers |
| `local` posts | People signed in to this server, and your followers on other servers. {{IF translation}} When a reader asks for a translation, the text also goes to the translation services in section 6, which work for us. They do not receive the post's address or your handle |
| `private` posts | Your followers and the people you mention, on any server |
| `direct` posts | The people you mention, on any server. They are not end-to-end encrypted: we can technically read them |
| {{IF translation to private and direct}} {{`private` AND/OR `direct`, AS YOU ALLOWED}} posts | {{TRANSLATION SERVICES YOU ALLOWED THEM}}, when a reader who may see the post asks for a translation. They do not receive the post's address, your handle, the reader, or who is mentioned |
| Follows, blocks, boosts, favourites, reactions, poll votes, quotes | The other person's server. Boosts also go to your followers' servers |
| That your account moved, was deactivated, suspended, or deleted | Your followers' servers and, except for a move, other servers we know |
| A report you chose to forward | The reported account's server receives your comment and the posts named. It does not receive your name, IP address, or email address, so do not put those in the comment |

When you edit or delete a post or delete your account, we tell those servers. Most honour it, but we cannot guarantee it, and copies, including ones made by others, can remain.

Other people here see what you publish. Staff with the right permission can reveal your email address and your registration and sign-in IP addresses, and each reveal is recorded in the audit log. Staff can also see email addresses of pending registrations.

Search engines and AI services: {{DESCRIBE YOUR search_indexing, indexable_default, AND ai_usage SETTINGS. By default we ask them not to index and not to use the content}}. These are requests, and not every crawler follows them. You can change whether your own profile appears in search in your profile settings.

{{IF the full-text search runs on another company's service}} Full-text search: the text of posts in search ({{public / local}} posts of people who chose to appear in search) goes to {{SERVICE}} to make them searchable, and is removed there when the post is deleted or you stop appearing in search. {{SAY HOW LONG ITS BACKUPS KEEP IT}}. {{IF you keep logs that hold search terms}} What you search for can appear in our request logs, which we keep for {{PERIOD}}.

Apps you authorize can read and do what the permissions you granted allow. Their own policies apply. You can revoke them in your settings.

{{IF the list of supporters is public}} If you choose to appear in the list of supporters, it shows your avatar, display name, handle, and plan.

{{IF browser translation}} A reader can also translate a post with the translation built into their browser. That sends nothing to us or to a translation service, and the translation is not stored.

# 6. Service providers

We use these services to run the instance. {{SAY WHETHER EACH ACTS ON YOUR BEHALF AND WHAT CONTRACT APPLIES.}}

| Service | Purpose | Data | Location | Their policy |
| --- | --- | --- | --- | --- |
| {{HOSTING PROVIDER}} | Server, database, and file storage | Everything in section 3 | {{COUNTRY}} | {{LINK}} |
| {{IF cdn}} {{CDN PROVIDER}} | Delivering media | IP address and requests for media | {{COUNTRY}} | {{LINK}} |
| {{IF mail}} {{MAIL PROVIDER}} | Sending email | Your address, and the subject and text of each email | {{COUNTRY}} | {{LINK}} |
| {{IF captcha}} {{CAPTCHA SERVICE}} | Stopping automated sign-ins, sign-ups, and password-reset requests | IP address, browser and device information, cookies, and interactions with the widget. Your browser sends these directly, from the sign-in, sign-up, and password-reset request pages only | {{COUNTRY}} | {{LINK}} |
| {{IF login providers}} {{PROVIDER}} | Signing in with an account there | That you sign in. We receive their identifier for you, and at sign-up your email address and username | {{COUNTRY}} | {{LINK}} |
| {{IF follow review}} {{FOLLOW REVIEW SERVICE}} | Screening follows from other servers for abuse | The addresses of your account and of the account on the other server, when someone there follows you | {{COUNTRY}} | {{LINK}} |
| {{IF sign-up review}} {{SIGN-UP REVIEW SERVICE}} | Screening sign-ups for abuse | The email address and IP address you sign up with, unless you sign up with an invite | {{COUNTRY}} | {{LINK}} |
| {{IF post review}} {{POST REVIEW SERVICE}} | Screening new posts for abuse | The text of your `public`, `local`, and `unlisted` posts and your username, while your account is younger than {{7 days}} | {{COUNTRY}} | {{LINK}} |
| {{IF translation}} {{TRANSLATION SERVICE}} | Translating a post when a reader asks | The text of the post: its body, content warning, poll options, and image descriptions. Not its address, its author, the reader, mentions, or links. Text sent to the service cannot be recalled, and deleting the post does not remove it there: {{HOW LONG THE SERVICE KEEPS IT}} | {{COUNTRY}} | {{LINK}} |
| {{IF external review of posts}} {{REVIEW SERVICE}} | Reviewing posts from other servers before they are accepted | The post as its server sent it: text, content warning, poll options, hashtags, its author, its visibility, attachments' addresses and descriptions, and how many accounts it names. No post of this server's accounts | {{COUNTRY}} | {{LINK}} |
| {{IF stripe}} Stripe | Taking payments for supporter plans | Your email address and plan. You enter payment details on Stripe's pages | {{COUNTRY}} | <https://stripe.com/privacy> |
| {{IF payjp}} PAY.JP | Taking payments for supporter plans | Your email address, cardholder name, and plan. You enter card details in PAY.JP's frame, whose script runs on our payment pages | {{COUNTRY}} | {{LINK}} |
| {{IF error reports}} {{ERROR TRACKER}} | Fixing faults | Error reports with personal details removed by pattern | {{COUNTRY}} | {{LINK}} |
| {{IF telemetry}} {{METRICS OR TRACING SERVICE}} | Watching the health of the service | Measurements, and the names of other servers we contact | {{COUNTRY}} | {{LINK}} |
| {{IF voice media server}} {{VOICE SERVICE}} | Carrying voice chat | Your IP address and audio while you speak in a room, and a session ID. Not your account | {{COUNTRY}} | {{LINK}} |
| {{IF turn}} {{STUN OR TURN SERVICE}} | Connecting voice chat | Your IP address, and encrypted audio when relayed | {{COUNTRY}} | {{LINK}} |

{{IF peer-to-peer voice}} In voice chat rooms, your browser connects directly to other participants, so they can see your IP address.

If you turn on push notifications, your browser's maker or its push service delivers them. The content is encrypted for your browser, but the service sees your browser's push address and when messages are sent.

# 7. Transfers abroad

Delivery to other servers (section 5) sends your data to servers abroad, wherever those servers are. We do not choose or know their countries in advance, and we cannot limit delivery by country.

{{YOUR STATEMENT ON THIS AND ON THE SERVICES OF SECTION 6: which transfer rules you consider apply, and the safeguards you rely on, if any. Settle this with your adviser: whether federated delivery is a restricted transfer is not settled, and this template takes no position.}}

# 8. Cookies and storage in your browser

We use no advertising or analytics cookies. Our pages use these strictly to work:

| Name | Purpose | Lasts |
| --- | --- | --- |
| `__Host-my_session` | Keeps you signed in to the sign-in pages | Up to 30 days |
| `__Host-my_rt` | Keeps the app signed in | Up to 30 days |
| `__Host-my_csrf` | Protects forms against forged requests | Until you close your browser |
| `__Host-my_2fa` | Carries a sign-in to the two-factor step | 5 minutes |
| `__Host-my_external` | Carries a sign-in through a login provider | At most 30 minutes |

The app also stores data in your browser only: display settings, recently used emoji and hashtags, the draft of a post (for the tab), and the newest posts of your home timeline for offline reading, without followers-only and direct posts. The drafts and the timeline are removed when you sign out.

{{IF captcha}} The CAPTCHA service sets its own cookies on the sign-in, sign-up, and password-reset request pages.

{{IF payjp}} PAY.JP's script runs on our payment pages and may set its own cookies. {{CHECK AGAINST PAY.JP'S POLICY.}}

# 9. How we protect it

Passwords are stored as Argon2id hashes. Two-factor secrets and the keys that sign your activities are encrypted, and the encryption key is kept outside the database. Session and access tokens are stored as hashes. Traffic uses HTTPS. {{YOUR MEASURES: encryption at rest and of backups, who has access, and how you handle a breach.}}

# 10. Your rights

{{ADJUST TO THE RIGHTS OF YOUR JURISDICTION. Under the GDPR these are access, rectification, erasure, restriction, objection, and portability, and the right to withdraw consent.}}

- **See and take your data:** your settings offer a data export (your profile, your posts, and your moderation records) and CSV files of follows, blocks, mutes, muted domains, and bookmarks. For everything else, including your email and IP addresses, write to us.
- **Correct it:** edit your profile and account settings, or write to us.
- **Delete it:** delete your account in your settings, or deactivate it first. This is what section 3 and section 5 describe.
- **Move:** you can move your followers to an account on another server.
- **Limit it:** choose your notifications in your settings. Every notification email has an unsubscribe link. You can also object to or ask us to restrict a use of your data by writing to us.
- **Reports about you:** we do not reveal who reported you.

Write to {{CONTACT}}. We answer within {{one month, or the period your law sets}}. You can complain to a data protection authority: {{YOUR AUTHORITY, OR "the authority where you live"}}.

We keep some records even when you ask us to delete them, where we need them for moderation or the law requires them (section 3).

# 11. Children

{{YOUR MINIMUM AGE AND HOW YOU APPLY IT. The server does not ask for a birth date or check age.}}

# 12. Automated decisions

We make no decisions about you by automated means that have legal or similarly significant effects, {{CONFIRM FOR YOUR USE. These are automatic: a sign-up that matches a block, or that an external review answers about, is refused or sent for review, {{IF post review}} a post that an external review refuses, which you can appeal to our staff, the CAPTCHA check, and rate limits. Your staff decide the rest.}}

# 13. Changes

We change this policy when what we do changes, and show the new date at the top. {{HOW YOU TELL PEOPLE, for example an announcement.}} If we close the service, we tell you in advance, with a deadline for taking your data. {{HOW YOU TELL PEOPLE, for example by email and an announcement.}}
```
