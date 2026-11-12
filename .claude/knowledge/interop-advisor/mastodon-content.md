# Mastodon: posts, articles, quotes, polls, replies, boosts

Base: <https://github.com/mastodon/mastodon/blob/v4.7.3/> (read 2026-11-10; v4.7.5 checked for differences).

## Reading what others send

- Drops a `Create` with no relation to a local account -> my ingest scope is the same idea [ADR-0006 §11]
- An `Update` of an unknown post counts as a `Create` only when `published` is within 1 day -> same window [internal/app/federation/statuses.go UpdateAsCreateWindow]
- `CONVERTED_TYPES` (Page, Event, Image, Video, Audio, Article) become a body of name, summary, and a link to url -> my converts the same types except Article, which it ingests as an article [app/lib/activitypub/activity.rb; normalize_test.go TestNormalizeDialects]
- Post language: `contentMap`, else the first key of `nameMap`, else of `summaryMap` (`StatusParser#raw_language_code`) -> my reads the same order and sends a one-language article without `contentMap` [parser/status_parser.rb]
- Embeds the first page of `replies` -> my reads embedded `items`/`first` [TestNormalizeDialects]
- Reads only `conversation` on receive, not `context` -> my sends both with the same IRI; a `tag:` conversation goes in `conversation` only [ADR-0154 §5]
- Its HTML allowlist `MASTODON_STRICT` is the base of my sanitizer [lib/sanitize_ext/sanitize_config.rb; contenttest.HTMLSanitizerContract]

## Articles my sends (Article)

- Converts them: `<h2>{name}</h2>`, summary, a link to url (else id); summary is not a CW; content, contentMap, and image are not read; name and summary are embedded unescaped, then sanitized -> my always sends a summary, and title and summary as plain text (a `<` may show broken) [parser/status_parser.rb#text]
- inbox POST and fetches take 1 MiB (`MAX_JSON_SIZE`); no length check of remote bodies [app/lib/request.rb]
- Redoes the conversion on `Update`: title and summary edits arrive [activity/update.rb]

## Quotes, polls, interaction policy

- Verifies a quote like `VerifyQuoteService`: the authorization's host and attributedTo are the quoted author, interactingObject the quoting post, interactionTarget the quoted post -> my checks the same [app/services/activitypub/verify_quote_service.rb; ft: FetchesQuotedPostsAndVerifiesApprovals]
- Quotes round-trip both ways with `QuoteRequest`; approve and reject become the quote's state [live: TestInteractionApproval, 4.7.3, 2026-11-10]
- Sends a `<p class="quote-inline">RE: …</p>` fallback before the body -> my keeps it on receive and sends the same
- Reads neither `canReply` nor `canAnnounce` and sends no `ReplyRequest`: its replies and boosts to a post that needs approval wait for the author, enter on approval, can be withdrawn by `Delete`/`Undo` [live: TestInteractionApprovalWithoutRequests]
- Poll vote: a `Create` of a `Note` with `name` = the option and `inReplyTo` = the `Question`; Mastodon, Misskey, Akkoma, GoToSocial count it -> my sends and reads that shape [ft: RemoteStatusContract]
- A poll-only `Update` without `updated` does not mark the post edited -> my omits `updated` on vote-count updates
- Attachments: `type` Document whatever the media, kind in mediaType; video poster in `icon`, length in `duration` -> same shape [internal/interface/activitypub peers tests]
- `sensitive` is true whenever there is a CW -> same
