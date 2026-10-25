# testkit

Port contract test suites, deterministic fakes (`Clock`, `IDGenerator`), and fixtures for adapter authors and for this
repository's own tests ([ADR-0015](../../docs/adr/0015-testing-strategy.md)).

## PostgreSQL test databases

`pgtest.NewDatabase(t)` (package `pkg/testkit/pgtest`) returns the URL of a new database with the product schema, copied
from a template database on one server per test binary ([ADR-0041](../../docs/adr/0041-postgres-tests-on-pgmem.md)).
The server is pgmem, PostgreSQL in memory inside the test process, unless `TEST_POSTGRES_URL` names an existing server
or `TEST_POSTGRES=container` starts a container with testcontainers-go (`scripts/task.sh go:test-postgres`). The package's `TestMain`
calls `os.Exit(pgtest.Main(m))` so that the server stops after the tests.

## Valkey test servers

`valkeytest.New(t)` (package `pkg/testkit/valkeytest`) returns the URL of a Valkey server and a key prefix of the
test's own, whose keys are deleted when the test ends. The server is a `valkey/valkey` container started with
testcontainers-go on the first call, unless `TEST_VALKEY_URL` names an existing server; without Docker the tests are
skipped, or fail when `TEST_VALKEY_REQUIRED` is set (CI). The package's `TestMain` calls `os.Exit(valkeytest.Main(m))`.

## Amazon SNS and SQS

`awstest.NewFake(clock)` (package `pkg/testkit/awstest`) is SNS and SQS in memory, with the aws-sdk-go-v2 signatures of
the operations that the adapters on an SNS topic fanning out to SQS queues use (`stream/sns`): its `SNS()` and `SQS()`
stand in for `*sns.Client` and `*sqs.Client` behind the adapters' narrow interfaces. It keeps the queue policy, raw
message delivery, the visibility timeout, the retention, the long polls, and the 60 seconds before a deleted queue's
name can be used again; `SetUnreachable` makes every call wait for its context. Every test run uses it.

`awstest.Emulator(t)` returns the `aws.Config` of the AWS emulator [Floci](https://github.com/floci-io/floci): a
`floci/floci` container started with testcontainers-go when `TEST_AWS=container` (`scripts/task.sh go:test-stream`, CI on main and
nightly), or a running emulator at `TEST_AWS_ENDPOINT`; otherwise the test is skipped. `TEST_AWS_REQUIRED` makes a
missing Docker fail the tests. The package's `TestMain` calls `os.Exit(awstest.Main(m))`.

## A whole server in the test process

testkit holds no server: it depends only on application ports and domain types, never on the composition root
(depguard, [ADR-0002](../../docs/adr/0002-modular-monolith-hexagonal.md)). The tests that drive the whole server over
HTTP, such as the integration tests of the client library in `pkg/mykit/integration`
([ADR-0093](../../docs/adr/0093-api-client-library-mykit.md) §8), use `internal/platform/platformtest`:
`platformtest.Start(t, opts)` builds the App on a new SQLite database with the schema applied and serves it with
`platform.Serve` on a free local port until the test ends, `User` creates a user, and `AuthorizationCode` issues the
code that the web client's authorization page would, for the client to exchange at `POST /oauth/token`.
`Options.AccountLimit` shrinks the client API's per-account rate limit to reach a 429 in a few requests.

## Federation fixtures

`fixtures/federation/` holds AS2 documents (activities, objects, actors) that other implementations send, for the
regression tests of [federation.md §12](../../docs/architecture/federation.md#12-テスト) ([ADR-0006](../../docs/adr/0006-federation-protocol-scope.md)).
`testkit.FederationFixtures()` returns them and `testkit.FederationFixturesFS()` exposes the tree.

### Layout

```text
fixtures/federation/<implementation>/<version>/<name>.json
fixtures/federation/<implementation>/README.md      provenance of every file of that implementation
testdata/golden/normalize/<implementation>/<version>/<name>.json   federation.Normalize result
```

- `<implementation>` is the lower-case software name: `mastodon`, `misskey`, `gotosocial`, `akkoma`.
- `<version>` is the release that produced the document (`4.8`, `0.22`, `2025.4`). When the source does not record it,
  it is `circa-<year>` with the year of the document's timestamps.
- `<name>` is the activity type and, where it helps, the object type or variant, in kebab case:
  `follow`, `accept-follow`, `create-note`, `update-person`, `delete-note`, `undo-like`, `like-reaction`, `actor`.

### Sources

A fixture comes from a public source whose license allows copying it into this AGPL-3.0-only repository:

- the implementation's own test data or test expectations (Mastodon, Misskey, GoToSocial, and Akkoma are AGPL-3.0);
- the implementation's documentation, when its license is compatible (Akkoma's `docs/` is CC BY 4.0, which requires
  attribution; Mastodon's documentation is GFDL and must not be copied);
- a document captured from a running server, recorded with the server's version and the capture date. The live
  interoperability test writes every activity it receives from Mastodon, GoToSocial, and Misskey to its output
  ([deploy/docker/federation-live/README.md](../../deploy/docker/federation-live/README.md#adding-a-captured-activity-as-a-fixture)).
- the document that the implementation's own rendering code produces, rendered from its source at a pinned release or
  commit, when no test data has it; the README names the functions and links the source.

Never write a fixture by hand from memory: a fixture shows what an implementation really sends.

### Adding a fixture

1. Take the document from the source unchanged.
2. Replace the example values:
   - every host of a real server with an example host (RFC 2606): the sender becomes `<implementation>.example`
     (or keeps a host that is already an example host, such as `mastodon.example.org`), other servers become `remote.example`
     and `other.example`. Replace the host everywhere it appears: URLs, `acct:` handles, `@user@host` mentions, `tag:` URIs.
   - the user names, display names, bios, and post texts of real people with neutral ones (`alice`, `bob`, `Alice`),
     keeping the markup around them. Names that the source's test data made up (`the_mighty_zork`, `akko`) may stay.
   - every `publicKeyPem` with `testkit.ExamplePublicKeyPEM` and every Linked Data `signatureValue` with
     `RVhBTVBMRS1TSUdOQVRVUkU=`.
   - nothing else: keep property names, ids, value types, nulls, and dialect extensions as they are, since they are what
     the test is about.
3. Indent the JSON with two spaces and save it at `fixtures/federation/<implementation>/<version>/<name>.json`.
4. Add a row to `fixtures/federation/<implementation>/README.md`: the file, the source (a commit permalink, or the
   server version and capture date), the license, and what was replaced.
5. Run `go test ./pkg/testkit -run TestFederationFixtures -update`, read the new golden file under
   `testdata/golden/normalize/`, and commit both.

`TestFederationFixturesAnonymized` fails on a URL whose host is neither an example host nor a JSON-LD namespace host,
and on a key other than the example key; `TestFederationFixturesProvenance` fails on a fixture without a README row.

### Golden files

`TestFederationFixturesNormalize` normalizes every fixture with `federation.Normalize` and compares the result with its
golden file; a document that cannot be normalized records the error. When a change to the normalization changes a
result on purpose, run the test with `-update` and review the golden diff in the PR. `testkit.CompareGolden` is the
same comparison for other golden tests.
