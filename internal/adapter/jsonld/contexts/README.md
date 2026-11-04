# Bundled JSON-LD contexts

The contexts that the linked data signatures of public posts resolve (ADR-0006), embedded in the binary so that
canonicalization never fetches a context from the network. Mastodon and Misskey bundle the same three.

| File | URL | Source |
| --- | --- | --- |
| `as.jsonld` | `https://www.w3.org/ns/activitystreams` | served at the URL (W3C, W3C Software and Document License) |
| `security-v1.jsonld` | `https://w3id.org/security/v1` | served at the URL (W3C Credentials Community Group) |
| `identity-v1.jsonld` | `https://w3id.org/identity/v1` | `contexts/identity-v1.jsonld` of the `web-payments/web-payments.org` repository, which the URL redirects to and which no longer answers |

Replace a file only with the document its URL serves, and run `go test ./internal/adapter/jsonld`: `testkit.CanonicalizerContract` compares the
canonicalization with jsonld.js (`pkg/testkit/testdata/jsonld/sign.mjs`).
