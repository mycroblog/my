# interop-advisor: index

How other servers behave and how `my` adapts. Read this file, then the files of the peers and FEPs in question.

| File | Holds |
| --- | --- |
| `decisions.md` | Interoperability decisions taken: one line each, with reason, origin, date, and how often it held |
| `mastodon.md` | Mastodon: signatures, delivery, fetching, collections, moderation, migration |
| `mastodon-content.md` | Mastodon: how it reads and sends posts, articles, quotes, polls, replies, boosts |
| `misskey.md` | Misskey: follows, reactions, quotes, articles, limits, migration |
| `gotosocial.md` | GoToSocial: signatures, interaction policy and requests, articles, closure |
| `pleroma.md` | Pleroma and Akkoma: relay actor, articles, limits |
| `others.md` | Bonfire, Activity-Relay, Fedibird, and behaviour shared by several servers |
| `fep.md` | The FEPs `my` implements or reads, and how |

## Rules for a line

- `- <what the peer does> -> <how my adapts> [<source>]`. No paragraphs.
- The source is the peer's code or doc at a version (a path under the base URL the file names, or a full URL), or
  the test of `my` that holds it (`ft:` is `pkg/testkit/federationtest`, `live:` is `test/federation-live`).
- A behaviour seen in a live test names the date and the versions. Unconfirmed: end the line with `(unconfirmed)`.
- Every file is at most 4096 bytes (`scripts/docs-growth.sh`). Near it: merge similar lines, delete lines a test now
  holds, or split the file by topic and add the new file to the table above.
- Change the knowledge by adding, merging, or deleting lines, never by rewriting a file.

## Test data

- Fixtures captured from real servers: `pkg/testkit/fixtures/federation/<peer>/<version>/` (procedure and licences in
  `pkg/testkit/README.md` and each peer's `README.md`); a new interop bug adds its document there.
- Live scenarios against Mastodon, GoToSocial, Misskey, Akkoma, Bonfire, and Activity-Relay: `test/federation-live/`.
