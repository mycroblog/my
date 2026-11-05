# Record of processing activities template

A template for operators: the record of processing activities that GDPR Article 30(1) asks a controller to keep, filled in as far as the software decides.

**This is a template and not legal advice.** It describes what the software does with its default settings, and it does not decide whether the GDPR, or this record, applies to you. Have it checked for your jurisdiction.

Article 30(5) exempts an organisation with fewer than 250 employees, unless its processing is likely to risk people's rights and freedoms, is not occasional, or includes special categories or criminal data. Running an instance keeps accounts continuously, so do not assume the exemption without checking it. Keeping the record also helps you answer requests and write the [privacy policy](privacy-policy-template.md).

## How to use it

1. Copy everything from [Record](#record) into your own records. This is an internal document: do not publish it with the instance's texts, and give it to a supervisory authority on request.
2. Replace every `{{...}}`. A value in the braces, such as `{{180 days}}`, is the default of the software. Confirm it against your instance. The [privacy policy template](privacy-policy-template.md#the-periods-and-their-settings) names the setting of each period.
3. Delete an activity, or a line that starts with `{{IF ...}}`, that does not apply to your setup. [The services you choose](privacy-policy-template.md#the-services-you-choose) says which setting each service follows.
4. Check that nothing is left: `grep -n '{{' <your file>` prints no line.
5. Review it when you change a setting or a service, and date each review.

Article 30 does not ask for the legal basis, and the record has a line for it because you need it for the policy. Treat the entries there as questions to settle.

If you run an instance on behalf of someone else, you may be a processor for them. Article 30(2) then asks for a different record, which this template does not cover.

## Record

### Controller (Article 30(1)(a))

| | |
| --- | --- |
| Controller | {{NAME, POSTAL ADDRESS, EMAIL}} |
| Joint controller | {{NAME AND CONTACT, OR "none"}} |
| Representative in the Union | {{NAME AND CONTACT, OR "not applicable"}} |
| Data protection officer | {{NAME AND CONTACT, OR "none"}} |
| Instance | {{INSTANCE NAME AND URL}} |
| Software | my, version {{VERSION}} |
| Last reviewed | {{DATE, BY WHOM}} |

### Recipients used in this record

These names are used under the activities below. List each one you use.

| Recipient | Name, contact, and contract | Country |
| --- | --- | --- |
| Hosting (server, database, blob storage, queue, key management) | {{...}} | {{...}} |
| Reverse proxy or load balancer | {{...}} | {{...}} |
| {{IF cdn}} CDN | {{...}} | {{...}} |
| {{IF mail}} Mail provider | {{...}} | {{...}} |
| {{IF captcha}} CAPTCHA service | {{...}} | {{...}} |
| {{IF login providers}} Login providers | {{...}} | {{...}} |
| {{IF payments}} Payment provider (Stripe or PAY.JP) | {{...}} | {{...}} |
| {{IF error reports}} Error tracker | {{...}} | {{...}} |
| {{IF telemetry}} Metrics and tracing service | {{...}} | {{...}} |
| {{IF voice}} Voice media server, STUN and TURN | {{...}} | {{...}} |
| {{IF webhooks}} Webhook receivers | {{...}} | {{...}} |
| {{IF translation}} Translation providers | {{...}} | {{...}} |
| Backup storage | {{...}} | {{...}} |
| Federated servers | Other servers of the network, run by other controllers, which change over time. `my` keeps the list of known servers | Any country |
| {{IF relays}} Relays | {{EACH RELAY YOU JOINED}} | {{...}} |
| Push services | Chosen by each user's browser | Depends on the browser |
| Apps | Authorized by each user | Depends on the app |

### Transfers (Article 30(1)(e))

- Federated delivery sends personal data to servers abroad, in countries that are not known in advance and cannot be limited. {{YOUR ASSESSMENT: whether this is a transfer under GDPR Chapter V, and on what basis. This template leaves it open.}}
- Transfers to the services above: {{FOR EACH, THE THIRD COUNTRY, AND THE ADEQUACY DECISION OR THE SAFEGUARD}}.

### 1. Accounts and signing in

| | |
| --- | --- |
| Purposes | Creating accounts, signing in, recovering access, deciding registrations |
| Data subjects | People with an account, people applying for one |
| Personal data | Email addresses, password hash, two-factor secret and hashed recovery codes, passkeys, the identifier at a login provider, preferences (language, time zone, posting defaults), registration reason, invite and decision, roles, hashed session and app tokens |
| Recipients | Hosting, mail provider, login providers, staff with `manage_accounts` |
| Erasure | Account erasure: 7 days after deletion. A deactivated account is deleted after 30 days. Sessions, tokens, and codes: 30 days after expiry. Invites and role grants, the account URI, and the username stay. An account that nobody deletes stays |
| Legal basis | {{e.g. contract}} |

### 2. Protecting sign-in and registration

| | |
| --- | --- |
| Purposes | Preventing abuse, finding accounts of the same person during moderation, rate limiting, stopping bots |
| Data subjects | People with an account, applicants, visitors of the sign-in pages |
| Personal data | Registration IP address, sign-in IP address and User-Agent, IP address (and for password-reset requests the email address) in rate limit counters, sign-up blocks (email domain, hashed address, IP range). With CAPTCHA: IP address, browser and device information, cookies |
| Recipients | Hosting, staff with `manage_accounts` (each reveal is in the audit log), CAPTCHA service |
| Erasure | Registration IP addresses `{{180 days}}`, sign-in origins `{{180 days}}`, counters within twice the window of the limit, sign-up blocks 30 days after the lift or expiry. Stopped by a retention hold |
| Legal basis | {{e.g. legitimate interests}} |

### 3. Publishing and social features

| | |
| --- | --- |
| Purposes | Publishing profiles and posts, and the interactions people ask for |
| Data subjects | People with an account, people they mention |
| Personal data | Profile and its versions, posts and their versions, media, favourites, reactions, boosts, bookmarks, pins, poll votes, follows, blocks, mutes, lists. Posts can hold any data a person writes, including special categories |
| Recipients | Hosting, CDN, other users, the public web for public content, federated servers (activity 4), search engines and AI crawlers under `search_indexing` and `ai_usage`, `{{the operator of the full-text search engine, if another company runs it}}`, apps the person authorized |
| Erasure | When the person deletes it, by their automatic deletion policy, or at account erasure. Uploads attached to nothing `{{24 hours}}`. Old versions of images after a newer version replaces them `{{30 days}}`. The record of a deleted post's URI stays |
| Legal basis | {{e.g. contract}} |

### 4. Federation

| | |
| --- | --- |
| Purposes | Delivering local activity to other servers, and receiving theirs |
| Data subjects | People with an account, people on other servers |
| Personal data | Sent: actor document, posts, follows, blocks, boosts, likes, reactions, poll votes, quotes, moves, forwarded report comments (without the reporter), lifecycle changes. Kept: copies of sent and received activities, delivery attempts |
| Recipients | Federated servers, relays |
| Erasure | Here: received 30 days and their documents 7 days. Sent 30 days, and the delivery of a post until 30 days after the post or its author is deleted. There: not under your control. `Delete` is sent, and other servers are expected but not forced to follow it |
| Legal basis | {{e.g. contract, legitimate interests}} |

### 5. Storing data of people on other servers

| | |
| --- | --- |
| Purposes | Showing remote profiles and posts to local users |
| Data subjects | People on other servers, who usually have no relationship with you |
| Personal data | Actor documents, profile versions, handles, posts, media references, copies of media, interactions |
| Recipients | Hosting, CDN, local users, public pages |
| Erasure | Posts `{{90 days}}` when nothing here refers to them, `{{7 days}}` when only a relay brought them, and `{{no limit}}` for any post otherwise. Copies of media `{{not kept}}`. Handle checks 90 days. Everything of an account 7 days after its server deletes it, or up to 30 days when only a `410 Gone` said so and the server does not answer again. Its URI and domain stay |
| Legal basis | {{e.g. legitimate interests}} |

### 6. Notifications

| | |
| --- | --- |
| Purposes | Telling people about activity and about their account, by email and Web Push |
| Data subjects | People with an account, people named in a notification |
| Personal data | Notifications and read markers, notification settings, email address, the text of each email, bounce and complaint records, Web Push address and keys, post subscriptions |
| Recipients | Hosting, mail provider, push services (the message body is encrypted, and they hold it up to 48 hours) |
| Erasure | At account erasure, or with the post they are about. Web Push: also when the person or the push service ends it. The mail provider: {{ITS PERIOD}} |
| Legal basis | {{e.g. contract, legitimate interests}} |

### 7. Moderation and audit

| | |
| --- | --- |
| Purposes | Handling reports, actions and appeals, proving what staff did, holds for legal requests |
| Data subjects | Reported people, reporters, staff, people named in a comment or a note |
| Personal data | Reports (comment, posts, rules, notes, assignments, resolutions), actions (reason, note, posts), appeals, audit log (actor, action, the target's name), holds and their reason, posts kept as evidence. Can include criminal data or special categories if a report or a post holds them |
| Recipients | Hosting, staff with the matching permission, the person concerned through their own moderation records (never the reporter's identity), the reported account's server for a forwarded report |
| Erasure | Reports `{{365 days}}` from the last resolution, or with an action that refers to them. Actions and appeals `{{365 days}}` from the end of the action. Audit log `{{730 days}}`. Open reports and actions with no end stay. A released hold goes after the period for actions |
| Legal basis | {{e.g. legitimate interests, legal obligation}} |

### 8. Requests from people

| | |
| --- | --- |
| Purposes | Answering requests for access and portability, and imports |
| Data subjects | People with an account |
| Personal data | Export archives (actor, posts, own moderation records), CSV files, imports and their results, the correspondence of a request |
| Recipients | Hosting, the person |
| Erasure | Archives and imports 7 days. Correspondence: {{PERIOD}} |
| Legal basis | {{e.g. legal obligation, contract}} |

The user's own export does not hold email addresses, IP addresses, notifications, or preferences. For a full answer, use `my admin account export-moderation` for moderation records and [reveal](moderation.md#account-management) for email and IP addresses, and read the rest from the database. The moderation export leaves out reporters' identities: check that a comment does not reveal one before you hand it over.

### 9. Voice chat

Delete this activity when `voice = off`.

| | |
| --- | --- |
| Purposes | Running voice chat rooms, and moderating them |
| Data subjects | Hosts and participants |
| Personal data | Room name, description, hosts, and how the room ended; who took part, spoke, raised a hand, was invited, or was removed. IP addresses and audio pass through the voice media server or TURN server, or reach other participants directly with `voice = p2p`. The server records no audio |
| Recipients | Hosting, voice media server, STUN and TURN, participants |
| Erasure | Participants `{{90 days}}` from the end of the room, or as long as a report about it. Rooms stay, with the name and description emptied at the author's erasure. At the services: {{THEIR PERIOD}} |
| Legal basis | {{e.g. contract, legitimate interests}} |

### 10. Supporter payments

Delete this activity when `payments = none`.

| | |
| --- | --- |
| Purposes | Taking subscriptions, granting roles, accounting |
| Data subjects | Subscribers |
| Personal data | Provider object IDs, amounts, periods, states, refunds, chargebacks, the choice to be listed. Email address sent to the provider. No card numbers, last four digits, or expiry dates |
| Recipients | Hosting, payment provider, people who can see the list of supporters, staff with the matching permission |
| Erasure | Webhook receipts 45 days. The customer link and the listing at account erasure. Subscription and payment records have no automatic end: {{PERIOD AND THE LAW THAT REQUIRES IT}}. At the provider: {{ITS PERIOD, AND WHETHER YOU DELETE THE CUSTOMER}} |
| Legal basis | {{e.g. contract, legal obligation}} |

### 11. Running the service

| | |
| --- | --- |
| Purposes | Finding faults, restoring after failure, connecting your own tools |
| Data subjects | Everyone above |
| Personal data | Logs (paths that can name a username or post, error text), proxy logs with IP addresses, error reports scrubbed by pattern, job records holding IDs, webhook bodies, backups of everything |
| Recipients | Hosting, reverse proxy, error tracker, metrics and tracing service, webhook receivers, backup storage |
| Erasure | Failed jobs and run records `{{30 days}}`. Webhook messages 14 days. Hourly error counts 90 days. Application and proxy logs: {{PERIOD}}. The latest report of each error group: replaced by the next one, with no automatic end. Backups: {{PERIOD}}. At the error tracker and the receivers: {{THEIR PERIOD}} |
| Legal basis | {{e.g. legitimate interests}} |

### 12. Translating posts

Delete this activity when `translation_providers` is empty.

| | |
| --- | --- |
| Purposes | Translating a post when a reader asks, and keeping translation within its limits |
| Data subjects | Authors of posts, here and on other servers, and people the text names. Readers, for the count of characters |
| Personal data | Whatever the text of a post holds: its body, content warning, poll options, and image descriptions. The post's address, its author, the reader, mentions, and links are replaced or left out before sending. Kept: the translations, and the characters per account and month |
| Recipients | Translation providers, as your processors: `public`, `local`, and `unlisted` posts by default. {{IF translation to private and direct}} Also `private` and `direct` posts, for {{THE PROVIDERS YOU ALLOWED}} |
| Erasure | Translations `{{30 days}}` from the translation, and with the post when it is deleted. Characters per account until the end of the following month, or at account erasure. At the providers: what was sent cannot be recalled. {{THEIR PERIODS}} |
| Legal basis | {{e.g. legitimate interests}} |

### Security measures (Article 30(1)(g))

What the software does:

- Passwords are stored as Argon2id hashes. Session, access, and one-time tokens and recovery codes are stored as hashes.
- Private signing keys, two-factor secrets, and provider and webhook secrets are encrypted with AES-256-GCM under a key-encryption key kept outside the database ([key management](key-management.md)).
- Email and IP addresses are shown only to staff with `manage_accounts` through an explicit action that the audit log records.
- Sign-in, sign-up, and password-reset requests are rate limited. Staff permissions go through roles.
- Outbound requests that remote servers can influence go through a guard against reaching internal addresses.

What you do:

- TLS at the reverse proxy: {{CONFIGURATION}}
- Encryption of the database, blob storage, and backups at rest: {{MEASURES}}
- Who can reach the host, the database, and the KEK, and how access is reviewed: {{MEASURES}}
- Staff accounts, roles, and two-factor authentication: {{POLICY}}
- Updates of the server and its host: {{POLICY}}
- Handling a personal data breach: {{PROCEDURE AND CONTACT}}
