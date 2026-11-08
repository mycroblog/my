#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests scripts/release-notes.sh on git repositories in a temporary directory, made of this repository's files and of
# small fixtures; it changes no file of the repository.
#
#   scripts/release-notes-test.sh

# shellcheck disable=SC2016 # The expected texts hold Markdown's backticks, not commands.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/release-notes.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# The fixtures' repositories ignore the configuration of whoever runs this.
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.com GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.com
export RELEASE_NOTES_TODAY=2026-06-01

failed=0
fail() {
  echo "FAIL: $*" >&2
  failed=1
}

# expect_has NAME TEXT WANT...: TEXT has each WANT.
expect_has() {
  local name="$1" text="$2" want
  shift 2
  for want in "$@"; do
    [[ "$text" == *"$want"* ]] || fail "$name: no '$want' in:"$'\n'"$text"
  done
}

# expect_not NAME TEXT UNWANTED...: TEXT has no UNWANTED.
expect_not() {
  local name="$1" text="$2" unwanted
  shift 2
  for unwanted in "$@"; do
    [[ "$text" != *"$unwanted"* ]] || fail "$name: '$unwanted' in:"$'\n'"$text"
  done
}

# section NAME TEXT prints the section of TEXT under the heading "## NAME".
section() {
  awk -v want="## $1" '/^## / { on = ($0 == want) } on' <<< "$2"
}

# commit SUBJECT commits everything in the current repository.
commit() {
  git add -A
  git commit -q --allow-empty -m "$1"
}

draft() {
  "$script" draft "$@"
}

copy_real_files() {
  local file
  for file in internal/platform/setup_dbupgrade.go internal/platform/config/retired.go docs/reference/configuration.md api/openapi.yaml; do
    mkdir -p "$(dirname "$file")"
    cp "$root/$file" "$file"
  done
}

# --- This repository's files: the first release, and a release with nothing to tell. These fail when a file moves or
# takes a form the script cannot read, which would otherwise read as "None" in a release's notes.
mkdir "$work/real"
cd "$work/real"
git init -q
copy_real_files
commit "feat!: everything so far"
git tag v0.0.0-test.1
git tag pkg/mykit/v0.0.0-test.1
git tag v0.1.0-rc.1

first="$(draft v0.1.0)"
expect_has "first release" "$first" \
  "## Upgrading" "None. This is the first release" "## Breaking changes" "## API changes" "first release of the client API" \
  "None. This is the first release of mykit." "None. This is the first release of mybot." \
  "go get github.com/mycroblog/my/pkg/mykit@v0.1.0"
[[ "$first" =~ :\ [0-9]{3,}\ operations ]] || fail "first release: no count of the operations: $first"
expect_not "first release lists no commits" "$first" "everything so far"
expect_not "first release keeps no hand-fill note of the libraries" "$first" "the commits marked with"

git tag v0.1.0
commit "fix: something small"
same="$(draft v0.2.0)"
expect_has "nothing changed" "$same" "compared with v0.1.0" "**Data upgrade steps**: None." "**To do by hand**: None."
expect_has "nothing breaks" "$(section "Breaking changes" "$same")" $'## Breaking changes\n\nNone.'
expect_has "no API change" "$(section "API changes" "$same")" $'## API changes\n\nNone.'
expect_has "no mykit change" "$(section mykit "$same")" $'\nNone.\n'
expect_has "no mybot change" "$(section mybot "$same")" $'\nNone.\n'
expect_not "no patch note for a minor version" "$same" "A patch release is compared"

# --- Fixtures with a change of every kind.
mkdir "$work/fixture"
cd "$work/fixture"
git init -q
mkdir -p internal/platform/config docs/reference api schema/10-t pkg/mykit/ts/src pkg/mybot

upgrades() {
  cat > internal/platform/setup_dbupgrade.go << EOF
package platform

func (a *App) dataUpgrades() DataUpgrades {
	return DataUpgrades{$1}
}
EOF
}

retired() {
  cat > internal/platform/config/retired.go << EOF
package config

var retiredKeys$1
EOF
}

upgrades ""
retired " []retiredKey"
cat > docs/reference/configuration.md << 'EOF'
| Key | Environment variable | Default | Required | Secret |
| --- | --- | --- | --- | --- |
| [`listen`](#listen) | `MY_LISTEN` | `:8080` | no | no |
| [`old_host`](#old_host) | `MY_OLD_HOST` | none | no | no |
| [`gone`](#gone) | `MY_GONE` | none | no | no |
| [`vanished`](#vanished) | `MY_VANISHED` | none | no | no |
| [`token`](#token) | `MY_TOKEN` | none | no | yes |
EOF
cat > api/openapi.yaml << 'EOF'
openapi: 3.1.0
paths:
  /things:
    get:
      operationId: listThings
      parameters:
        - $ref: '#/components/parameters/Limit'
        - name: old
          in: query
    post:
      operationId: createThing
  /due:
    get:
      operationId: getDue
      deprecated: true
      x-sunset: {deprecated: 2026-01-01, sunset: 2026-05-01}
  /future:
    get:
      operationId: getFuture
      deprecated: true
      x-sunset: {deprecated: "2026-01-01", sunset: "2027-01-01"}
  /removed-ok:
    get:
      operationId: getRemovedOk
      deprecated: true
      x-sunset: {deprecated: 2026-01-01, sunset: 2026-05-01}
  /removed-early:
    get:
      operationId: getRemovedEarly
      deprecated: true
      x-sunset: {deprecated: 2026-01-01, sunset: 2027-01-01}
  /removed-bad:
    get:
      operationId: getRemovedBad
  /soon:
    get:
      operationId: getSoon
components:
  parameters:
    Limit:
      name: limit
      in: query
  securitySchemes:
    oauth2:
      type: oauth2
      flows:
        authorizationCode:
          scopes:
            read: Read.
            old:scope: Old.
  schemas:
    Thing:
      type: object
      required: [id]
      properties:
        id: {type: string}
        count: {type: integer}
        dropped: {type: string}
        nested:
          type: object
          properties:
            inner: {type: string}
    Gone:
      type: object
      properties:
        x: {type: string}
EOF
echo "CREATE TABLE t (id int);" > schema/10-t/postgres.sql
echo "# docs" > schema/README.md
commit "chore: start"
git tag v0.1.0
commit "fix: a patch"
git tag v0.1.1
git tag pkg/mykit/v0.1.1
git tag pkg/mybot/v0.1.1

# A pre-release is never the previous release.
git tag v0.1.10-rc.1

# Everything below changes after v0.1.1.
upgrades '
		Steps: []dbupgrade.Step{
			{Key: "0001_new", Description: "Carries the " + "old rows over.", Run: runNew, Projections: []string{"counts", "feeds"}},
			{Key: "0002_contracted", Description: "Done before.", Run: nil, Projections: []string{"counts"}},
		},
		Prerequisites: []dbupgrade.Prerequisite{{Key: "0000_old", Via: "v0.1.1"}},
	'
retired ' = []retiredKey{
	{key: "old_host", renamedTo: "host"},
	{key: "gone", removed: "the server always does this"},
}'
cat > docs/reference/configuration.md << 'EOF'
| Key | Environment variable | Default | Required | Secret |
| --- | --- | --- | --- | --- |
| [`listen`](#listen) | `MY_LISTEN` | `:9090` | no | no |
| [`host`](#host) | `MY_HOST` | none | yes | no |
| [`new_optional`](#new_optional) | `MY_NEW_OPTIONAL` | none | no | no |
| [`token`](#token) | `MY_TOKEN` | none | when `x = y` | yes |
EOF
cat > api/openapi.yaml << 'EOF'
openapi: 3.1.0
paths:
  /things:
    get:
      operationId: listThings
      parameters:
        - $ref: '#/components/parameters/Limit'
        - name: fresh
          in: query
    post:
      operationId: createThing
      deprecated: true
      x-sunset: {deprecated: 2026-05-20, sunset: 2026-05-25}
      description: |
        Use putThing
        instead.
  /due:
    get:
      operationId: getDue
      deprecated: true
      x-sunset: {deprecated: 2026-01-01, sunset: 2026-05-01}
  /future:
    get:
      operationId: getFuture
      deprecated: true
      x-sunset: {deprecated: "2026-01-01", sunset: "2027-01-01"}
  /added:
    put:
      operationId: putThing
  /soon:
    get:
      operationId: getSoon
components:
  parameters:
    Limit:
      name: limit
      in: query
  securitySchemes:
    oauth2:
      type: oauth2
      flows:
        authorizationCode:
          scopes:
            read: Read.
            new:scope: New.
  schemas:
    Thing:
      type: object
      required: [id, count]
      properties:
        id: {type: integer}
        count: {type: integer}
        added: {type: string}
        nested:
          type: object
          properties:
            inner: {type: string}
            more: {type: string}
    Fresh:
      type: object
      properties:
        x: {type: string}
EOF
commit "feat(api)!: reshape the things"
echo "CREATE TABLE t (id bigint);" > schema/10-t/postgres.sql
echo "# changed docs" > schema/README.md
commit "feat(db,api)!: widen the id"
echo "// x" > pkg/mykit/client.go
commit "refactor!: change the retries"
echo "// x" > pkg/mybot/bot.go
commit "fix(mybot)!: rename a handler"
echo "// x" > pkg/mykit/api.gen.go
echo "// x" > pkg/mykit/ts/src/schema.gen.ts
echo "// x" > pkg/mykit/client_test.go
commit "feat(web)!: only generated client and tests"
commit "feat(voice): not breaking"
commit "fix!no colon, so not marked"
git merge -q --no-ff -m "feat!: a merge is not a change of its own" "$(git commit-tree "HEAD^{tree}" -p HEAD -m "docs: side")"

notes="$(draft v0.2.0)"
expect_has "the comparison" "$notes" "compared with v0.1.1"

# What the prose must name: the data upgrades, the settings, and the breaking changes; not the schema files.
"$script" facts v0.2.0 "" "$work/required"
required="$(cat "$work/required/required.json")"
expect_has "required items" "$required" '"0001_new"' '"0000_old"' '"old_host"' '"gone"' '"vanished"' '"listen"' \
  '"host"' '"data upgrade"' '"setting"' '"breaking commit"' 'feat(db,api)!: widen the id' "\"$(git log --format=%h --grep='widen the id')\""
expect_not "required items" "$required" "schema/" "new_optional"

upgrading="$(section Upgrading "$notes")"
expect_has "upgrading" "$upgrading" \
  '`0001_new`: Carries the old rows over.' '`0002_contracted`: Done before. (only recorded' \
  '`counts`' '`feeds`' 'latest patch release of the previous one (`v0.1.1`)' \
  '`0000_old`: this release removes data that it carried over, so upgrade through `v0.1.1` first' \
  '`host`: a new required setting' 'Fill by hand: roughly how long' 'my schema apply -dry-run'
expect_not "upgrading: an optional setting needs nothing" "$upgrading" "new_optional"
[ "$(grep -c '`counts`' <<< "$upgrading")" = 1 ] || fail "a projection is listed once: $upgrading"

broken="$(section "Breaking changes" "$notes")"
expect_has "settings" "$broken" \
  '`old_host` was renamed to `host`' '`gone` was removed: the server always does this' \
  '`vanished` is gone without an entry' '`listen`: the default changed from `:8080` to `:9090`' \
  '`token`: required changed from no to when `x = y`'
expect_has "client API" "$broken" '`getRemovedBad`' '`Thing.dropped`' '`Thing.id`: integer, required, was string, required' 'scope `old:scope`'
expect_has "schema" "$broken" '`schema/10-t/postgres.sql` changed'
expect_not "only the schema's SQL" "$broken" "schema/README.md"
expect_has "commits by area" "$broken" \
  $'- api\n' $'- db\n' $'- general\n' $'- mybot\n' $'- web\n' "feat(api)!: reshape the things" \
  "feat(db,api)!: widen the id" "refactor!: change the retries"
expect_not "unmarked commits" "$broken" "not breaking" "no colon" "a merge is not" "docs: side" "chore: start"
expect_has "hand-fill note" "$broken" "Fill by hand: what each change means"

api="$(section "API changes" "$notes")"
expect_has "added" "$api" '`putThing` (`PUT /added`)' '`listThings`: parameter `fresh`' 'schema `Fresh`' \
  '`Thing.added`' '`Thing.nested.more`' '`new:scope`'
expect_not "a new schema is listed as a whole" "$api" 'Fresh.x'
expect_has "deprecated" "$api" \
  '`createThing` (`POST /things`): Deprecation 2026-05-20, Sunset 2026-05-25. Use putThing instead.'
expect_has "removed" "$api" \
  '`getRemovedOk` (`GET /removed-ok`)' \
  '`getRemovedEarly` (`GET /removed-early`): **its Sunset 2027-01-01 has not passed' \
  '`getRemovedBad` (`GET /removed-bad`): **not deprecated in v0.1.1' \
  '`listThings`: parameter `old`' 'schema `Gone`' '`Thing.dropped`'
expect_has "changed" "$api" '`Thing.count`: integer, required, was integer'
expect_has "removable" "$(grep -A3 'May be removed' <<< "$api")" '`getDue` (`GET /due`): its Sunset 2026-05-01 has passed'
expect_not "not removable: deprecated in this release, whatever the Sunset" "$(grep -A3 'May be removed' <<< "$api")" "createThing"
expect_has "kept" "$(grep -A3 'Deprecated earlier and kept' <<< "$api")" '`getFuture` (`GET /future`): until its Sunset 2027-01-01'
expect_not "the Sunset of a future operation" "$(grep -A2 'May be removed' <<< "$api")" "getFuture"
expect_has "api hand-fill" "$api" 'git diff v0.1.1 v0.2.0 -- api/openapi.yaml'

mykit="$(section mykit "$notes")"
expect_has "mykit" "$mykit" "go get github.com/mycroblog/my/pkg/mykit@v0.2.0" "refactor!: change the retries" \
  "Deprecated in the generated client" '`createThing`' "Removed from the generated client" '`getRemovedBad`'
expect_not "mykit: generated files and tests are no hand-written change" "$mykit" "only generated client"
mybot="$(section mybot "$notes")"
expect_has "mybot" "$mybot" "go get github.com/mycroblog/my/pkg/mybot@v0.2.0" "fix(mybot)!: rename a handler"
expect_not "mybot: other commits and operations" "$mybot" "change the retries" "createThing"

# The previous release.
git tag v0.2.0
expect_has "a patch of a later minor version" "$(draft v0.2.3)" "compared with v0.1.1" "A patch release is compared"
expect_has "a pre-release of the next minor version" "$(draft v0.3.0-rc.1)" "compared with v0.2.0"
git tag -d v0.2.0 v0.1.1 > /dev/null
expect_has "a patch of the first minor version" "$(draft v0.1.2)" "compared with v0.1.0" 'the same minor version (`v0.1.Z`).'
expect_has "PREVIOUS names the tag" "$(draft v0.0.0-test.2 v0.1.0)" "compared with v0.1.0"
if draft v0.2.0 v0.1.0 > /dev/null 2>&1; then fail "PREVIOUS on a release: want an error"; fi
if draft v0.2.0-rc.1 v9.9.9 > /dev/null 2>&1; then fail "PREVIOUS that is no tag: want an error"; fi
if draft v0.2.0-rc.1 pkg/mykit/v0.1.1 > /dev/null 2>&1; then fail "PREVIOUS that is a library's tag: want an error"; fi
if draft 0.2.0 > /dev/null 2>&1; then fail "a bad version: want an error"; fi
if "$script" publish v0.2.0 > /dev/null 2>&1; then fail "an unknown command: want an error"; fi

# A source the script cannot read fails instead of saying "None": release.yml then falls back on the skeleton.
upgrades 'Steps: registeredSteps()'
commit "chore: steps elsewhere"
if draft v0.3.0 > /dev/null 2>&1; then fail "steps that are not literals: want an error"; fi
upgrades 'Steps: []dbupgrade.Step{{Key: stepKey}}'
commit "chore: a key elsewhere"
if draft v0.3.0 > /dev/null 2>&1; then fail "a key that is not a literal: want an error"; fi
git rm -q internal/platform/setup_dbupgrade.go
commit "chore: move the steps"
if draft v0.3.0 > /dev/null 2>&1; then fail "a moved file: want an error"; fi

# The steps after the facts, with a fake gh and a fake claude: the PRs come from git and GitHub, the prompt calls nothing,
# and a failed call ends in the facts alone with no failure. tools/agentkit/cmd/releaseprose has the tests of the check.
mkdir "$work/bin"
cat > "$work/bin/gh" <<'FAKE'
#!/bin/sh
case "$1" in
  pr) printf '%s\n' '[{"number":7,"title":"feat: seven","body":"## Summary\n\nThe seventh.\n\n## Changes\n\nhidden\n"},{"number":8,"title":"feat: eight","body":""},{"number":1,"title":"before the release","body":""}]' ;;
  issue) printf '%s\n' '[{"number":6,"title":"the closed issue","closedAt":"2999-01-01T00:00:00Z","stateReason":"COMPLETED"}]' ;;
esac
FAKE
cat > "$work/bin/claude" <<'FAKE'
#!/bin/sh
cat > /dev/null
printf '%s\n' '{"type":"result","subtype":"success","is_error":true,"result":"Not logged in"}'
exit 1
FAKE
chmod +x "$work/bin/gh" "$work/bin/claude"
mkdir "$work/prs"
cd "$work/prs"
git init -q
copy_real_files
commit "chore: the start (#1)"
git tag v0.4.0
commit "feat: squashed (#8)"
git checkout -q -b topic
commit "feat: on the branch (#9)"
git checkout -q -
git merge -q --no-ff -m "Merge pull request #7 from mycroblog/topic" topic
"$script" facts v0.5.0 "" "$work/prs-dir"
expect_has "the facts' body" "$(cat "$work/prs-dir/body.md")" "<!-- release-notes: facts only." \
  "**Full Changelog**: https://github.com/mycroblog/my/compare/v0.4.0...v0.5.0"
[ -f "$work/prs-dir/required.json" ] || fail "no required.json"
expect_has "the draft" "$(draft v0.5.0)" "**Full Changelog**: https://github.com/mycroblog/my/compare/v0.4.0...v0.5.0"
expect_not "the draft lists no commits" "$(draft v0.5.0)" "squashed" "on the branch"
if RELEASE_NOTES_BODY_LIMIT=10 "$script" facts v0.5.0 "" "$work/over" > /dev/null 2>&1; then
  fail "facts over the limit: want an error"
fi
PATH="$work/bin:$PATH" "$script" prs "$work/prs-dir"
[ "$(sort -n "$work/prs-dir/pr-numbers" | paste -sd ,)" = "7,8" ] ||
  fail "merged PRs: $(cat "$work/prs-dir/pr-numbers")"
prompt="$(PATH="$work/bin:$PATH" "$script" prompt v0.5.0 "$work/prs-dir")"
expect_has "prompt" "$prompt" "Release v0.5.0, compared with v0.4.0" "## Upgrading" "<untrusted-" "The seventh." "- #8 feat: eight" "the closed issue"
expect_not "prompt" "$prompt" "hidden" "before the release"
before="$(cat "$work/prs-dir/body.md")"
PATH="$work/bin:$PATH" "$script" prose v0.5.0 "$work/prs-dir" > "$work/prose.out" 2> /dev/null ||
  fail "a failed call must not fail the command"
expect_has "failed call" "$(cat "$work/prose.out")" "The draft has the facts alone"
after="$(cat "$work/prs-dir/body.md")"
expect_has "failed call" "$after" "<!-- release-notes: facts only." "the call failed" "## Upgrading" \
  "**Full Changelog**: https://github.com/mycroblog/my/compare/v0.4.0...v0.5.0"
[ "${before#*-->}" = "${after#*-->}" ] || fail "a failed call changed the facts"
cat > "$work/bin/claude" <<'FAKE'
#!/bin/sh
cat > /dev/null
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"stop_reason":"end_turn","result":"## Main changes\n\nThe seventh (#7).\n\n## For operators\n\nNone.\n\n## For API and library users\n\nNone.\n"}'
FAKE
PATH="$work/bin:$PATH" "$script" prose v0.5.0 "$work/prs-dir" > /dev/null
with="$(cat "$work/prs-dir/body.md")"
expect_has "prose" "$with" "The seventh (#7)." "## Upgrading" "**Full Changelog**"
expect_not "prose" "$with" "<!-- release-notes: facts only."
cd "$work/real"

skeleton="$("$script" skeleton v0.3.0)"
expect_has "skeleton" "$skeleton" "**Full Changelog**" "## Upgrading" "## Breaking changes" "## API changes" "## mykit" "## mybot" \
  "release-notes.sh failed" "pkg/mykit@v0.3.0"

if [ "$failed" -ne 0 ]; then
  exit 1
fi
echo "release-notes: ok"
