---
name: release
description: Cut a release of my with release.yml - choose the commit, run it, review the draft's notes and image, publish it. Use when asked to cut or publish a release, or when a release, its notes, its image, or its npm job failed.
---

# Cut a release

Only the decider publishes a release. What each job does is in the comments of the workflows `release.yml`,
`slow-suite.yml`, and `release-image.yml`, and of the scripts they call.

## Before

1. The latest full Nightly on `main` passed: the newest run titled exactly `Nightly`
   (`gh run list -R mycroblog/my --workflow nightly.yml --branch main --limit 10`). If it failed, fix `main` first.
2. Pick the version: `vX.Y.Z` from `main`, or a prerelease `vX.Y.Z-<id>`, the only kind a branch can make. A patch
   only adds to the schema; removing data waits for the next minor version.
3. For a stable version, `scripts/tofu-examples.sh bump vX.Y.Z` and merge it first. For `vX.Y.0`, in the same change,
   add the minor version to the top of `web/docs/versions.json` and set any minor version whose support ends to `ended`.
4. Bring the translations up to date, so that the tag carries them: `/translate-docs` for the documents, and the
   untranslated keys of the UI catalogs (`scripts/i18n.sh`). Neither blocks the release.

## Run

`gh workflow run release.yml -R mycroblog/my --ref main -f version=vX.Y.Z`. The head of `main` then is the commit
tested and tagged.

- A prerelease before v0.1.0 that drops a column or a table on purpose (a `!` commit): `-f allow_destructive=true`,
  and tell operators in **For operators** to run `my schema apply -allow-destructive`.
- A prerelease tried before a previous minor version exists: `-f previous=<tag>`.
- Only in an emergency, a prerelease held up by a failure unrelated to it (a flaky test, a service slow to start):
  `-f skip_slow_suite=true`. Fix that failure on `main` before the next release.

## When a job fails

- **The slow suite**: nothing is tagged. Fix the cause on `main` and run again with the same version.
- **`upgrade`**: a failure in `my db upgrade -check` or `my schema apply` usually means a schema change that the
  previous release's data does not satisfy; in the reads, data the upgrade did not carry over. Operators are promised
  this upgrade: fix it on `main`. Reproduce it with `scripts/task.sh docker:upgrade-test PREVIOUS=<tag>`.
- **`npm` or an image job after `release` passed**: fix the cause (for `npm`, usually the trusted publisher), then
  `gh run rerun <run-id> -R mycroblog/my --failed`. Versions already published are skipped.
- **`release-notes-prose`**: the draft keeps the facts alone under a `release-notes: facts only` comment. Rerun the
  failed jobs; they replace only such a draft. `scripts/release-notes.sh prompt` shows what the model gets.
- **`release-notes-image`**: write `ghcr.io/mycroblog/my:X.Y.Z@sha256:...` at the end of the notes by hand.
- **`release-image.yml`**: fix the cause and run it again with `-f version=vX.Y.Z`.

## Review and publish

1. `gh release view vX.Y.Z -R mycroblog/my`, and read the notes as [notes.md](notes.md) says. Never publish the
   prose unread.
2. The image: `gh release download vX.Y.Z -R mycroblog/my --pattern image-digest.txt --output -`, then
   `docker run --rm ghcr.io/mycroblog/my@<digest> version`, and run it as far as the release calls for. Read the
   `image-scan` summary: a finding with a fix in a base image or a dependency means another commit and a new version.
3. Publish the draft. Check that the runs of `release-image.yml` and, for a stable version, `docs-site-deploy.yml`
   that it starts pass.

## Afterwards

- A faulty version stays on the registry and is never rebuilt: release the next patch and say so in its notes. To
  move `X.Y` back meanwhile: `gh workflow run release-image.yml -R mycroblog/my --ref main -f version=<previous patch>
  -f roll_back=true`.
- A Nightly `image-scan` failure is about a published version: raise the base image or the dependency on `main`, and
  release a patch of each minor version it applies to.
