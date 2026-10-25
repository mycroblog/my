# Releasing

How the decider cuts a release. Only `release.yml` creates releases. It runs the slow suite (`slow-suite.yml`) on the chosen commit and,
only when every job passes, tags that commit, drafts a GitHub Release, and publishes the client libraries under the same version
([Libraries](#libraries-mykit-and-mybot)). What the slow suite contains, and where else it runs, is in
[What CI runs where](../agents/workflow.md#what-ci-runs-where).

## Versions

- A version is `vX.Y.Z`, or a prerelease `vX.Y.Z-<identifier>` (for example `v0.1.0-rc.1`). Any other form fails in the first job.
- Branches other than `main` can only produce prereleases. They exist for trying the gate on a branch; real releases come from `main`.
- A version that is already tagged fails in the first job. After the slow suite, the workflow checks again that no release with the tag's name exists.
- Within a minor version, patch releases only add to the schema (or rebuild projection tables); removing data waits for a minor version.
  Operators can upgrade from the same minor version and from the latest patch release of the previous minor version
  ([Skipping releases](database.md#skipping-releases)).

## Steps

1. Check that the latest Nightly passed. If it failed, fix it first ([Failures on main](../agents/workflow.md#failures-on-main-orchestrator)).

   ```sh
   gh run list -R mycroblog/my --workflow nightly.yml --limit 3
   ```

2. Run `release.yml` on `main`. The head commit of `main` at that moment is the commit that is checked and tagged.

   ```sh
   gh workflow run release.yml -R mycroblog/my --ref main -f version=v0.1.0
   gh run list -R mycroblog/my --workflow release.yml --limit 1
   ```

3. Wait for it to finish. The slow suite (the full E2E, federation, real PostgreSQL, the stream contract, the images, [the upgrade test](#the-upgrade-test), CodeQL)
   takes a while even with its jobs in parallel.
   The `release` job does not run if any slow-suite job fails or is cancelled: no tag, no Release, and no library is published.
   On a failure, fix the cause on `main` and run the workflow again with the same version.
   If only the `npm` job fails after `release` passed, fix the cause (usually the trusted publisher registration) and rerun the failed job of that run
   (`gh run rerun <run-id> -R mycroblog/my --failed`); versions already published are skipped.
4. When it passes, open the draft Release (`gh release view v0.1.0 -R mycroblog/my`, or the Releases page), tidy the generated list of changes,
   write the sections that [Release notes](#release-notes) requires, and publish it. A prerelease is drafted as a prerelease.

## The upgrade test

The slow suite's `upgrade` job checks the upgrade that operators are promised: from the latest patch release of the previous minor version.
For each of SQLite and PostgreSQL it:

1. builds the server and schema images of that release from its tag, and of the commit being released;
2. applies the previous release's schema, starts its server, and fills the database through it: two local accounts, a public and a
   followers-only post, a local follow, and a follow request to an account on a static ActivityPub server (`remote.test`), which the
   server fetches and stores as a remote account;
3. stops that server and upgrades in the order of [Upgrading](database.md#upgrading), with the new images: `my db upgrade -check`,
   `atlas schema apply`, `my db upgrade`, then the new server;
4. checks that `/readyz` answers 200, that `my projection verify` finds nothing to correct, and that the data reads back through the
   client API: both accounts sign in with their passwords, and the posts, the follow, the follow request, and the remote account are kept.
   A new post must also succeed.

Which release it starts from:

- `release.yml` with version `vX.Y.Z` (or a prerelease of it): the latest `vA.B.C` tag whose minor version `A.B` is before `X.Y`.
  For `v0.3.0` and `v0.3.1` alike that is the latest `v0.2.Z`. Prerelease tags and the libraries' `pkg/...` tags are never picked.
- Nightly and runs by hand without a version: the latest release, the upgrade that the head of `main` has to support whatever it becomes.
- When there is no such tag, as for `v0.1.0`, the job reports "the upgrade test is skipped" in its summary and passes.

`scripts/previous-minor.sh` picks the tag (tested by `scripts/previous-minor-test.sh`, run with `scripts/task.sh shell:test`).
The test itself is `scripts/upgrade-test.sh` on the stack `deploy/docker/upgrade-test/compose.yaml`. Run it locally with Docker:

```sh
scripts/task.sh docker:upgrade-test PREVIOUS=v0.1.3                  # both dialects
scripts/task.sh docker:upgrade-test PREVIOUS=v0.1.3 DIALECTS=postgres
```

The previous release's images are built from its tag. To use images from a registry instead, set both `MY_PREVIOUS_IMAGE` and
`MY_PREVIOUS_SCHEMA_IMAGE` to that release's server and schema images.

When it fails, the job prints every container's logs. A failure in `my db upgrade -check` or `atlas schema apply` usually means a schema change
that the previous release's data does not satisfy; a failure in the reads means data that the upgrade did not carry over.
Fix it on `main` before releasing: [Versions](#versions) promises this upgrade.

## Release notes

Besides the generated list of changes, every Release has the sections below. Write "None" in a section that has nothing to say;
an empty section and a missing one mean different things to an operator.

### Upgrading

What an operator runs to upgrade to this release ([Upgrading](database.md#upgrading)):

- The data upgrade steps that this release's `my db upgrade` runs, one line each with what it carries over and roughly how long it takes on a large instance.
  "None" when the release has none; operators still run `my db upgrade -check` and `my db upgrade`, which then do nothing.
- The projections that `my db upgrade` rebuilds.
- The releases an operator must have upgraded through: always "the same minor version, or the latest `vX.(Y-1).Z`", and, when this release
  removes data that an earlier release carried over, that release by name (it is what `my db upgrade -check` names when it stops).
- Anything to do by hand before or after, such as a new required setting.

To fill it, compare the data upgrade steps registered in `my` with those of the previous release, and run the schema's `--dry-run`
from the previous release's database.

### Breaking changes

What stops working, or works differently, for an operator or a client that did nothing:

- **Settings**: renamed or removed setting keys and environment variables, with their replacement (the server refuses to start with a retired key and names it),
  and changed defaults.
- **Client API**: removed operations and fields (see [API changes](#api-changes)), and changes made without the deprecation period for a security fix.
- **Schema contract**: tables and columns removed from the schema (only in a minor version, and only after an earlier release carried their data over),
  type changes, and new constraints that existing rows must satisfy. Name the release whose `my db upgrade` carried the data over.

ActivityPub formats and federated identifiers (Actor and post URIs) never change in a release, so this section never lists them.

### API changes

The client API (`/api/v1`) grows by addition. A breaking change first marks the operation deprecated and keeps it for at least one v0.x minor release.

- The section lists:
  - added operations, fields, and scopes;
  - operations marked `deprecated: true` in this release, with their `Deprecation` and `Sunset` dates and the operation that replaces them;
  - operations and fields removed in this release (only those deprecated in an earlier release);
  - anything changed without the deprecation period because of a security fix, with the reason.

  To write it, read the difference of `api/openapi.yaml` from the previous release, and the operations that have an `x-sunset` (`grep -n x-sunset api/openapi.yaml`).
- Before publishing, check what may be removed: an operation may be removed only if a release of an earlier minor version already shipped it with
  `deprecated: true` and its `Sunset` date has passed. Never deprecate and remove an operation within the same minor version.
- A `Sunset` date is never before its `Deprecation` date (the generator checks this). When deprecating, pick a date after the next planned minor release.

### mykit and mybot

The `release` job puts both sections, with how to install each from Go and npm, at the top of the draft. Before publishing, replace the note at the end of each:

- **mykit**: breaking changes to the hand-written layer (retries, paging, streaming, tokens), taken from the commits marked `!` in Conventional Commits
  (allowed during v0), and the types and functions of the generated client deprecated or removed in this release (the same operations as in [API changes](#api-changes)).
- **mybot**: breaking changes to `mybot` (commits marked `!`, as for `mykit`).

## Libraries (mykit and mybot)

The client library `mykit` and the bot library `mybot` are released with the same version number in the same release as the server.
`scripts/release-libraries.sh` sets the versions (tested by `scripts/release-libraries-test.sh`, run with `scripts/task.sh shell:test`).

- **Go tags**: the `release` job tags `pkg/mykit/vX.Y.Z` on the same commit as the server's `vX.Y.Z`. A tag is all it takes to publish a Go module;
  proxy.golang.org fetches it the first time someone asks for it. If the tag already exists, the first job fails.
  - On `main`, `pkg/mybot/go.mod` requires `pkg/mykit` at a placeholder version and replaces it with `../mykit`, so that `mybot` builds with the `mykit` of the same commit.
    Users never read `replace`, so the `release` job creates, through the GitHub API, a commit on top of the checked one that only changes `pkg/mybot/go.mod` to
    `require github.com/mycroblog/my/pkg/mykit vX.Y.Z`, and tags it `pkg/mybot/vX.Y.Z` (`scripts/release-libraries.sh go-mod`).
    That commit is on no branch; only the tag holds it.
- **npm**: after `release`, the `npm` job builds `@mycroblog/mykit` and `@mycroblog/mybot`, sets the version in `package.json` to the release's (`X.Y.Z`, without the `v`),
  and publishes `mykit` and then `mybot`.
  - The `@mycroblog/mykit` dependency of `@mycroblog/mybot` (`workspace:*` in the repository) is pinned to the same `X.Y.Z` before publishing, because `npm publish` does not rewrite `workspace:`.
  - Authentication is npm trusted publishing (GitHub Actions OIDC, with the job permission `id-token: write`); no npm token exists, not even as a secret.
    Trusted publishing needs npm CLI 11.5.1 or later and Node.js 22.14.0 or later; the job uses the Node.js of `.tool-versions` and its npm, and checks their versions before publishing.
  - `npm publish --provenance` attaches provenance: the package version's page on npm shows the workflow and the commit that built it.
  - Prereleases (`vX.Y.Z-<identifier>`) go to the dist-tag `next`, releases to `latest`. `npm install @mycroblog/mykit` installs `latest` only.
  - A version already in the registry is skipped, so a failed job can be rerun. npm never allows a version number again, even after it is unpublished.
- **Before the first publication (the decider, once)**: npm registers a trusted publisher in a package's settings, so it cannot be registered before the package exists.
  1. Create the npm organization `mycroblog` with the decider as owner.
  2. Publish the first version of `@mycroblog/mykit` and `@mycroblog/mybot` once from a local machine (with 2FA; it has no provenance).
     `mybot` depends on the same version of `mykit`, so publish `mykit` first. Run the following at the repository root and revert the changes to `package.json`.

     ```sh
     (cd app && bun install --frozen-lockfile)
     (cd pkg/mykit/ts && bun run build)
     (cd pkg/mybot/ts && bun run build)
     scripts/release-libraries.sh npm-version v0.0.0-placeholder.0 pkg/mykit/ts pkg/mybot/ts
     (cd pkg/mykit/ts && npm publish --access public --tag next)
     (cd pkg/mybot/ts && npm publish --access public --tag next)
     git checkout -- pkg/mykit/ts/package.json pkg/mybot/ts/package.json
     ```

  3. For each of the two packages, open Settings on npmjs.com, choose GitHub Actions under "Trusted Publisher", and register
     Organization or user `mycroblog`, Repository `my`, and Workflow filename `release.yml` (leave Environment empty).
  4. In the same Settings, set "Publishing access" to "Require two-factor authentication and disallow tokens". Trusted publishing uses OIDC and is not affected.

  If `release.yml` runs before the registration, the `npm` job fails to authenticate. Rerun just that job after registering.

## Trying the gate on a branch

A change to `release.yml` can be tried on a branch with a prerelease version, without tagging a real version.

```sh
gh workflow run release.yml -R mycroblog/my --ref <branch> -f version=v0.0.0-test.1
```

- The upgrade test has no previous minor version to start from for a `v0.0.0-test.N` version, so it is skipped. To exercise it, name the tag to
  start from with `previous` (accepted for prereleases only):

  ```sh
  gh workflow run release.yml -R mycroblog/my --ref <branch> -f version=v0.0.0-test.2 -f previous=v0.0.0-test.1
  ```

  To try only the upgrade test (with the rest of the slow suite, but without tagging anything), run the slow suite by hand:

  ```sh
  gh workflow run slow-suite.yml -R mycroblog/my --ref <branch> -f previous=v0.0.0-test.1
  ```

- On a branch where the slow suite fails, the `release` job is skipped and no tag is created (`gh api repos/mycroblog/my/git/ref/tags/v0.0.0-test.1` answers 404).
- On a branch where it passes, the tags (the server's and `pkg/mykit/v0.0.0-test.1` on the branch's head, `pkg/mybot/v0.0.0-test.1` on the commit above it) and a draft Release
  are created, and `@mycroblog/mykit@0.0.0-test.1` and `@mycroblog/mybot@0.0.0-test.1` are published to the dist-tag `next` with provenance.
  Check that `pkg/mybot/go.mod` at `pkg/mybot/v0.0.0-test.1` requires `pkg/mykit v0.0.0-test.1`, that `@mycroblog/mybot@0.0.0-test.1` on npm depends on
  `@mycroblog/mykit` `0.0.0-test.1`, and that the version's page on npm shows the provenance; then delete the tags and the Release.

  ```sh
  gh release delete v0.0.0-test.1 -R mycroblog/my --cleanup-tag --yes
  gh api -X DELETE repos/mycroblog/my/git/refs/tags/pkg/mykit/v0.0.0-test.1
  gh api -X DELETE repos/mycroblog/my/git/refs/tags/pkg/mybot/v0.0.0-test.1
  ```

  A version published to npm can be withdrawn within 72 hours with `npm unpublish @mycroblog/mybot@0.0.0-test.1` (and the same for `mykit`), but its number can never be used again.
  A Go version that proxy.golang.org already fetched stays there after the tag is deleted. Use `v0.0.0-test.N` numbers, which never collide with real versions.

## What a release produces

`release.yml` produces the tags (the server's and the libraries'), the draft Release, and the npm packages.
Publishing the container images and generating the release notes are not part of it yet: the decider writes the notes' sections by hand.
