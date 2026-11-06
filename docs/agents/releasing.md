# Releasing

How the decider cuts a release. Only `release.yml` creates releases. It runs the slow suite (`slow-suite.yml`) on the chosen commit and,
only when every job passes, tags that commit, drafts a GitHub Release, publishes the client libraries under the same version
([Libraries](#libraries-mykit-and-mybot)), and pushes the [container image](#the-container-image) by digest. Publishing the draft gives the image its tags. What the slow suite contains, and where else it runs, is in
[What CI runs where](workflow.md#what-ci-runs-where).

## Versions

- A version is `vX.Y.Z`, or a prerelease `vX.Y.Z-<identifier>` (for example `v0.1.0-rc.1`). Any other form fails in the first job.
- Branches other than `main` can only produce prereleases. They exist for trying the gate on a branch; real releases come from `main`.
- A version that is already tagged fails in the first job. After the slow suite, the workflow checks again that no release with the tag's name exists.
- Within a minor version, patch releases only add to the schema (or rebuild projection tables); removing data waits for a minor version.
  Operators can upgrade from the same minor version and from the latest patch release of the previous minor version
  ([Skipping releases](../operations/database.md#skipping-releases)).

## Steps

1. Check that the latest full Nightly on `main` passed: the newest run titled exactly `Nightly`. If it failed, fix it first ([Failures on main](workflow.md#failures-on-main-orchestrator)).

   ```sh
   gh run list -R mycroblog/my --workflow nightly.yml --branch main --limit 10
   ```

   A run by hand of some of the jobs is titled for them, such as `Nightly (l4)`, and does not count, whatever its result.
   A run titled `Nightly` either ran every job, or, on a schedule, skipped them because a full run had already passed on that commit.

2. For a stable version, set the module ref of the OpenTofu examples to it, and merge that first: the first job of the release fails while they differ. A prerelease is not checked, and operators of a prerelease set `ref` themselves.

   ```sh
   scripts/tofu-examples.sh bump v0.1.0
   ```

   For the first stable version of a minor version (`vX.Y.0`), add the minor version to the top of `app/docs/versions.json` in the same change,
   and mark a minor version whose support ends with it as `ended` ([The documentation site](#the-documentation-site)). Every deploy of the site fails while a published release's minor version is not listed.

3. Run `release.yml` on `main`. The head commit of `main` at that moment is the commit that is checked and tagged.

   ```sh
   gh workflow run release.yml -R mycroblog/my --ref main -f version=v0.1.0
   gh run list -R mycroblog/my --workflow release.yml --limit 1
   ```

4. Wait for it to finish. The slow suite (the full E2E, federation, real PostgreSQL, the stream contract, the images, [the upgrade test](#the-upgrade-test), [the restore drill](../operations/backup.md#recovery-drill), CodeQL)
   takes a while even with its jobs in parallel.
   The `release` job does not run if any slow-suite job fails or is cancelled: no tag, no Release, and no library is published.
   On a failure, fix the cause on `main` and run the workflow again with the same version.
   If only the `npm` job or an `image` job fails after `release` passed, fix the cause (for `npm`, usually the trusted publisher registration) and rerun the failed jobs of that run
   (`gh run rerun <run-id> -R mycroblog/my --failed`); versions already published are skipped.
5. When it passes, open the draft Release (`gh release view v0.1.0 -R mycroblog/my`, or the Releases page) and read its [notes](#release-notes):
   the prose is a draft from a model, and the facts after it come from the repository. Fill in what the draft's comments ask for.
6. [Review the image](#review-the-image-in-the-draft): run its digest and read the `image-scan` report.
7. Publish the draft. A prerelease is drafted as a prerelease. `release-image.yml` then gives the image its tags within minutes,
   and `docs-site-deploy.yml` publishes the version's documentation under `/latest/` and `/vX.Y/` for a stable version; check that both runs passed.

   ```sh
   gh run list -R mycroblog/my --workflow release-image.yml --limit 1
   gh run list -R mycroblog/my --workflow docs-site-deploy.yml --limit 1
   ```

## The upgrade test

The slow suite's `upgrade` job checks the upgrade that operators are promised: from the latest patch release of the previous minor version.
For each of SQLite and PostgreSQL it:

1. builds the server image of that release from its tag, and of the commit being released;
2. applies the previous release's schema with that release's own tools, starts its server, and fills the database through it: two local accounts, a public and a
   followers-only post, a local follow, and a follow request to an account on a static ActivityPub server (`remote.test`), which the
   server fetches and stores as a remote account;
3. stops that server and upgrades in the order of [Upgrading](../operations/database.md#upgrading), with the new server image: `my db upgrade -check`,
   `my schema apply`, `my db upgrade`, then the new server;
4. checks that `/readyz` answers 200, that `my projection verify` finds nothing to correct, and that the data reads back through the
   client API: both accounts sign in with their passwords, and the posts, the follow, the follow request, and the remote account are kept.
   A new post must also succeed.

Which release it starts from:

- `release.yml` with version `vX.Y.Z` (or a prerelease of it): the latest `vA.B.C` tag whose minor version `A.B` is before `X.Y`.
  For `v0.3.0` and `v0.3.1` alike that is the latest `v0.2.Z`. Prerelease tags and the libraries' `pkg/...` tags are never picked.
- Nightly and runs by hand without a version: the latest release, the upgrade that the head of `main` has to support whatever it becomes.
- When there is no such tag, as for `v0.1.0`, the job reports "the upgrade test is skipped" in its summary and passes.

`scripts/previous-minor.sh` picks the tag (tested by `scripts/previous-minor-test.sh`, run with `scripts/task.sh shell:test`).
The test itself is `scripts/upgrade-test.sh` on the stack `test/upgrade-test/compose.yaml`. Run it locally with Docker:

```sh
scripts/task.sh docker:upgrade-test PREVIOUS=v0.1.3                  # both dialects
scripts/task.sh docker:upgrade-test PREVIOUS=v0.1.3 DIALECTS=postgres
```

Locally, the previous release's images are built from its tag; in the slow suite, the job takes that release's published image when the registry has one. A release that still shipped a separate schema image (Atlas Community Edition and
its schema, the `schema` target of its `Dockerfile`) applies its schema with that image; a later one with its own `my schema apply`.
To use images from a registry instead, set `MY_PREVIOUS_IMAGE` to that release's server image, and, for a release with a schema image,
`MY_PREVIOUS_SCHEMA_IMAGE` to that image.

When it fails, the job prints every container's logs. A failure in `my db upgrade -check` or `my schema apply` usually means a schema change
that the previous release's data does not satisfy; a failure in the reads means data that the upgrade did not carry over.
Fix it on `main` before releasing: [Versions](#versions) promises this upgrade.

## Release notes

Every Release has the prose for its three readers, the sections below, and a link to the comparison with the previous release, which stands for the list of changes.
Write "None" in a section that has nothing to say; an empty section and a missing one mean different things to an operator.

The notes are made in two steps, and the decider reads the result and publishes it:

1. **The facts** come from the repository, never from a model. The `release` job writes them with `scripts/release-notes.sh` (tested by `scripts/release-notes-test.sh`,
   run with `scripts/task.sh shell:test`) and creates the draft with them. It compares this release with the previous one, picked as for [the upgrade test](#the-upgrade-test): `previous` when
   a prerelease names it, otherwise the latest patch release of the previous minor version, ignoring prerelease tags and the libraries' `pkg/...` tags.
2. **The prose** comes from Claude. The `release-notes-prose` job gives it the facts, the Summary of every PR merged since the previous release, and the titles of the closed issues,
   and asks for three sections: Main changes, For operators, and For API and library users. A program checks the answer, and the `release-notes-edit` job puts the prose before the facts in the draft.

**The check.** Every data upgrade, removed or changed setting, breaking client API change, and commit marked `!` must have a line in the prose with its name in backticks and a PR number.
Every PR number must be of a PR merged in this release, as both `git log` and GitHub's list of merged PRs have it. The three headings must be exactly those, and the prose must have no HTML, no image, no @mention outside backticks, and no link outside this repository.
The model is asked again with what failed, up to three attempts. The check does not tell whether what the prose says is true, and it does not tell that a PR is the right one for an item: that is why the decider reads it.

**When the prose fails.** The draft keeps the facts alone, with a comment that starts `release-notes: facts only` and says why; the log of the job has the details. The release does not stop, because the tags exist by then.
This covers a failed call, a failed check after three attempts, missing configuration, and the job itself failing. To add the prose then, rerun the job
(`gh run rerun <run-id> -R mycroblog/my --failed`, or the job from the run's page); it replaces only a draft that still has that comment, so it never overwrites an edit or a published release.
A prerelease run on a branch other than `main` has the facts alone, because the job's environment deploys only from `main`.

**The bound of the input.** A PR's Summary is what goes in, cut at 4,000 bytes, and the Summaries together at 600,000 bytes. Above that, the PRs whose titles are marked `!` come first and then the newest, and every other PR goes in by its title alone.
The titles of those PRs and of the closed issues together stop at 300,000 bytes: the oldest PR titles and then the oldest issue titles are left out. The check still accepts every merged PR, and the draft's first comment and the job's summary say how much was left out.
A release with no earlier one, as `v0.1.0`, passes the bound of the Summaries, so its prose rests mostly on the newest PRs.

**PR text is untrusted.** Anyone who opens a PR writes its Summary. That text goes to the model in an element it cannot close, marked as material and never as instructions, and the model has no tools.
What the check requires comes from the repository and GitHub's list of merged PRs, so no PR text can change it. A PR can still mislead the prose within the check, so never publish the prose unread.

The model is named once, by `claudeModel` in `tools/agentkit/cmd/releaseprose/claude.go`; the `Install Claude Code` step of `release.yml` pins the Claude Code that must know it.
To see what the model would get, without calling it, run the commands below on a checkout with the tags (`gh` reads the PRs); the output goes to the given directory and the terminal:

```sh
scripts/release-notes.sh facts v0.1.0 "" .tmp/notes
scripts/release-notes.sh prs .tmp/notes
scripts/release-notes.sh prompt v0.1.0 .tmp/notes
```

The facts, and what the decider adds:

| Section | The facts say | Left to the prose and the decider |
| --- | --- | --- |
| Upgrading | The data upgrade steps new since the previous release, the projections they rebuild, the releases to upgrade from and through, new required settings | How long each step takes on a large instance; anything else to do by hand; the result of `my schema apply -dry-run` |
| Breaking changes | Renamed and removed settings, changed defaults and requirements, what the client API removed or changed, the schema files that changed, the commits marked `!` by area (their Conventional Commits scope) | What each one means for an operator or a client; whether a schema change removes data, and which release carried it over; which commits matter outside the repository |
| API changes | The difference of `api/openapi.yaml`: added and removed operations, fields, and scopes, changed fields, deprecations with their dates and description, the operations that may be removed | The replacement of a deprecated operation when its description does not name one; the reason for a change made without the deprecation period |
| mykit, mybot | The commits marked `!` with the library's scope or on its hand-written files; for `mykit`, the operations deprecated or removed | What each change means for a user of the library |

Without any earlier release, as for `v0.1.0`, the facts are those of a first release: "None" in Upgrading, Breaking changes, and the libraries, because nothing can
break yet, and the size of the client API; the link goes to the commits of the tag. A patch release is compared with the previous minor version too, so remove what an earlier patch of its minor version already announced.

The facts read the literals in `dataUpgrades` and `retiredKeys`, the generated [configuration reference](../reference/configuration.md), and `api/openapi.yaml`,
as they are at each release. If they cannot be read, or they pass GitHub's limit of 125,000 characters on a Release's body, the job still creates the release, with a warning, and the sections hold a request to write them by hand. There is then no prose either.

### Set up the prose (the decider, once)

Without these, the call fails and the draft has the facts alone. The job authenticates to the Claude API with Workload Identity Federation, as the nightly L4 job does, and no API key exists.

1. In the Claude Console, under Workload identity, add a federation rule for the service account and workspace of the `ux-nightly` rule: `subject_prefix` `repo:mycroblog@346242925/my@1421587381:environment:release-notes`,
   `audience` `https://api.anthropic.com`, `claims` `repository_owner: mycroblog` and `ref: refs/heads/main`, and a short token lifetime (600 seconds). The workspace's spend limit then covers both jobs, so check that it leaves room for a release.
2. In the repository's settings, under Environments, create `release-notes` with deployments from `main` only and no required reviewers, which would hold the job for hours after the tags exist.
3. Give that environment four variables, not secrets: `ANTHROPIC_FEDERATION_RULE_ID` (of the new rule), and `ANTHROPIC_ORGANIZATION_ID`, `ANTHROPIC_SERVICE_ACCOUNT_ID`, and `ANTHROPIC_WORKSPACE_ID` (as in `ux-nightly`).
4. Check it on the next prerelease from `main`: `release-notes-prose` passes, its summary says that the prose passed the check, and the draft has the three sections. A failed exchange shows in the Claude Console's authentication history.

### Upgrading

What an operator runs to upgrade to this release ([Upgrading](../operations/database.md#upgrading)):

- The data upgrade steps that this release's `my db upgrade` runs, one line each with what it carries over and roughly how long it takes on a large instance.
  "None" when the release has none; operators still run `my db upgrade -check` and `my db upgrade`, which then do nothing.
- The projections that `my db upgrade` rebuilds.
- The releases an operator must have upgraded through: always "the same minor version, or the latest `vX.(Y-1).Z`", and, when this release
  removes data that an earlier release carried over, that release by name (it is what `my db upgrade -check` names when it stops).
- Anything to do by hand before or after, such as a new required setting.

To check it, run the new release's `my schema apply -dry-run` on a database of the previous release.

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

  To check it, read the difference of `api/openapi.yaml` from the previous release, and the operations that have an `x-sunset` (`grep -n x-sunset api/openapi.yaml`).
- Before publishing, check what may be removed: an operation may be removed only if a release of an earlier minor version already shipped it with
  `deprecated: true` and its `Sunset` date has passed. Never deprecate and remove an operation within the same minor version.
  The draft lists the operations that may be removed, and marks in bold each removal that breaks this rule.
- A `Sunset` date is never before its `Deprecation` date (the generator checks this). When deprecating, pick a date after the next planned minor release.

### mykit and mybot

The facts have both sections, with how to install each from Go and npm and the changes it found. Before publishing, check them against:

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

## The container image

The server image is `ghcr.io/mycroblog/my`, for `linux/amd64` and `linux/arm64`, built with every adapter. Nothing else is published: there are no images of `main`, and no binaries on the Release.

### Tags

Image tags have no `v`; the Git tag and `my version` do (`v0.1.0`).

| The version | `X.Y.Z` | `X.Y` |
| --- | --- | --- |
| The largest patch of its `X.Y` (`v0.1.5` after `v0.2.0` too, which moves `0.1` alone) | Added, once | Moved to it |
| Not the largest patch of its `X.Y` | Added, once | Left alone |
| A prerelease (`v0.1.0-rc.1`) | `0.1.0-rc.1` only | Left alone |

There is no `0` and no `latest` during v0, because both would cross a minor version, which can need steps of its own ([Skipping releases](../operations/database.md#skipping-releases)).
`docker pull ghcr.io/mycroblog/my` without a tag therefore fails with `manifest unknown`, and every example in the documentation names a version.

`scripts/release-image.sh tags` computes the list (tested by `scripts/release-image-test.sh`, run with `scripts/task.sh shell:test`).

### How it is published

`release.yml`, while the Release is a draft:

1. `image` builds both platforms from the tested commit, with no cache and `VERSION` set to the version, and pushes them **by digest, with no tag**. It adds the OCI labels and annotations
   (`scripts/release-image.sh metadata`) and an SBOM of the image and of the `web` stage. Buildx's provenance is off, because the signed one below replaces it.
2. `image-check` runs on a runner of each platform: it takes the binary out of the image, runs `scripts/check-release-build.sh` on it, which fails on the test-only build tag `e2e_fakes`, and checks what `my version` prints.
3. `image-attest` signs the digest's provenance with `actions/attest` (keyless, through Sigstore's public instance, which ties the workflow and the commit to the digest), pushes it next to the image, and attaches the digest to the draft as `image-digest.txt`. `release-notes-image` then writes the image in the draft's notes.
4. `image-scan` reports what Grype finds in both platforms in the job's summary. It stops nothing.

`release-image.yml`, when the Release is published (a prerelease too), never builds:

1. It reads `image-digest.txt`, verifies the attestation, and checks that the provenance names the commit of the version's tag. On any difference it fails with no tag made.
2. It adds the [tags](#tags), and fails rather than move an `X.Y.Z` that exists at another digest.
3. It checks that each tag is at the digest, and that it can be pulled without credentials.

A rerun of a failed `image` job builds a new digest; the old one stays on the registry, unreferenced, and does no harm. So does the digest of a draft that was deleted. A rerun of `release-image.yml` changes nothing that is already right.

```sh
gh workflow run release-image.yml -R mycroblog/my --ref main -f version=v0.1.0
```

The image is built from the same commit as the slow suite tested, with the same pinned base images and lockfiles and a different `VERSION`, but it is not the very image the suite ran; the bits are not guaranteed to match.

### Review the image in the draft

The digest is in the `image-attest` job's summary, and on the draft:

```sh
gh release download v0.1.0 -R mycroblog/my --pattern image-digest.txt --output -
docker run --rm ghcr.io/mycroblog/my@sha256:... version
```

- Run the digest as far as this release calls for; nothing has a tag yet, so no operator receives it.
- Read the `image-scan` job's summary, and decide whether a finding should hold the release. A finding with a fix in a base image or a dependency means another commit and a new version.
- The `release-notes-image` job writes the image at the end of the notes, as `ghcr.io/mycroblog/my:0.1.0@sha256:...`, the form operators pin. If that job failed, write it by hand.

### Vulnerability scan

The Nightly `image-scan` job scans the `X.Y` tags of the two latest minor versions without credentials, and fails on a finding of HIGH or above that has a fix.
Such a failure is about a published version and not about `main`: raise the base image or the dependency on `main` and release a patch of each minor version it applies to.
Before any image is public, the job reports that there is nothing to scan and passes. Run it alone with `gh workflow run nightly.yml -R mycroblog/my -f jobs=image-scan`, or locally with `scripts/release-image.sh scan --fail ghcr.io/mycroblog/my:0.1`.

### A faulty version

Versions stay on the registry for good: operators pin digests, the latest patch of each minor version is on the upgrade path, and a package version downloaded more than 5,000 times cannot be deleted anyway.
Replace a faulty version with the next patch and say so in its notes. A fix in a base image also ships as a new patch; an existing tag is never rebuilt.
If `X.Y` must go back before the patch is ready, move it to the previous patch, which does not touch any `X.Y.Z`:

```sh
gh workflow run release-image.yml -R mycroblog/my --ref main -f version=v0.1.4 -f roll_back=true
```

### Before the first release (the decider, once)

The workflows cannot do these; each is a setting of the repository or of the package.

1. The first `image` job of `release.yml` creates the package, private, as ghcr.io does for every new package.
2. On the package's settings page (`https://github.com/orgs/mycroblog/packages/container/my/settings`), check that it is linked to this repository, which its `org.opencontainers.image.source` label asks for, and that `mycroblog/my` has the Admin role under "Manage Actions access", and change its visibility to **public**. A public package cannot be made private again.
3. Publish the draft. If `release-image.yml` ran before the package was public, it failed at the anonymous pull with its tags made: run it again.
4. In the repository's settings, turn on **Immutable releases**, under General. It freezes the tag and the assets of a published Release, so `image-digest.txt` cannot change and the tag that `source_url` names cannot disappear. Nothing freezes a draft. Under this setting the tag of a published `v0.0.0-test.N` stays too, so give each trial a new number.

## The documentation site

The documentation site (`app/docs`) is served by the Cloudflare Worker `my-docs` at the root of `mycroblog.dev`, with one path per version:

| Path | Built from |
| --- | --- |
| `/next/` | `main` |
| `/latest/` | the tag of the newest published stable release of the first minor version of `app/docs/versions.json` that has one |
| `/vX.Y/` | the tag of the newest published stable release of `vX.Y`, for each minor version of `app/docs/versions.json` that has one |
| `/` | the landing page and the 404 page of `/latest/` |

Each version is built from its own tag's tree, so the documents of a version are those of its tag; nothing keeps copies of old documents on `main`.
Before the first stable release, `/` is the landing page of `/next/` and `/latest/` redirects to `/next/`.

- `docs-site-deploy.yml` runs on a push to `main` that changes the published documents or `app/docs/`, when a stable release is published, and by hand
  (`gh workflow run docs-site-deploy.yml -R mycroblog/my --ref main`). A deploy replaces every asset of the Worker,
  so every run builds every version from its tag and combines them (`scripts/docs-site.sh build`): a push to `main` changes `/next/` only,
  and a release changes `/latest/` and its minor version. Drafts and prereleases are never published.
- Every build reads the list of `main`'s `app/docs/versions.json`, resolved against the published releases, so the version switcher in the header
  and the support status of old versions follow `main`. The switcher opens the same page in the chosen version when that version has it, and the version's landing page otherwise.
- `app/docs/versions.json` lists the released minor versions, newest first, each `supported` or `ended`. A minor version whose support ends stays on the site:
  set it to `ended`, and every page of it carries an end-of-support banner with a link to the same page under `/latest/`. Its tags are never deleted, since its pages are built from them.
- The deploy job runs in the environment `doc-site` (deployment refs `main` and the tags `v*`), which holds the secret `CLOUDFLARE_API_TOKEN` (Workers Scripts: Edit) and the variable `CLOUDFLARE_ACCOUNT_ID`.
  The Worker config (`scripts/docs-site/wrangler.jsonc`) declares no routes and no custom domain: `mycroblog.dev` is bound to the Worker, with its DNS records, in the infrastructure repository `mycroblog/infra`,
  after the Worker exists from a first deploy on `workers.dev`.
- To build the combined site locally, list the releases in a file (an empty file before the first release) and run the build; `DRY_RUN=1` checks the deploy without an account:

  ```sh
  : > .tmp/releases
  DOCS_SITE_RELEASES=.tmp/releases scripts/docs-site.sh build .tmp/docs-site/site
  DRY_RUN=1 scripts/docs-site.sh deploy .tmp/docs-site/site
  ```

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

  This also pushes an image by digest and attaches `image-digest.txt` to the draft. Check the digest as [Verifying an image](../operations/running.md#verify-an-image) does, and the SBOM's Go modules and npm packages. Trying `release-image.yml` means publishing the `v0.0.0-test.1` Release, which adds the image tag `0.0.0-test.1`. Delete a test version's image on the package's page, or with the REST API:

  ```sh
  gh api 'orgs/mycroblog/packages/container/my/versions?per_page=100' --jq '.[] | select(.metadata.container.tags | index("0.0.0-test.1")) | .id'
  gh api -X DELETE orgs/mycroblog/packages/container/my/versions/<id>
  ```

  An attestation or an untagged platform manifest shows as an untagged version on that page; a test digest's are deleted the same way, by the digest in `name`. Never delete an untagged version of a real release, because it can be a part of the image. A version published to npm can be withdrawn within 72 hours with `npm unpublish @mycroblog/mybot@0.0.0-test.1` (and the same for `mykit`), but its number can never be used again.
  A Go version that proxy.golang.org already fetched stays there after the tag is deleted. Use `v0.0.0-test.N` numbers, which never collide with real versions.

## What a release produces

| When | Workflow | What |
| --- | --- | --- |
| The slow suite passed | `release.yml` | The tags (the server's and the libraries'), the draft Release with the facts of [its notes](#release-notes), and the npm packages, which are public from then on |
| After the tags | `release.yml` | The prose of [the notes](#release-notes) in the draft, or the facts alone when it fails |
| The slow suite passed | `release.yml` | The [container image](#the-container-image) by digest, with no tag, its signed provenance and SBOM, and `image-digest.txt` on the draft |
| The decider published the draft | `release-image.yml` | The image's tags `X.Y.Z` and, for the latest patch of a minor version, `X.Y` |
| The decider published a stable release | `docs-site-deploy.yml` | The version's documentation under `/latest/` and `/vX.Y/` of [the documentation site](#the-documentation-site) |

A draft that is deleted still uses up its version number, on npm as on Git; the digest stays on the registry with no tag.
