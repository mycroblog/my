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
mkdir -p internal/platform/config docs/reference api schema/postgres pkg/mykit/ts/src pkg/mybot

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
echo "CREATE TABLE t (id int);" > schema/postgres/10-t.sql
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
echo "CREATE TABLE t (id bigint);" > schema/postgres/10-t.sql
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
expect_has "schema" "$broken" '`schema/postgres/10-t.sql` changed'
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

skeleton="$("$script" skeleton v0.3.0)"
expect_has "skeleton" "$skeleton" "## Upgrading" "## Breaking changes" "## API changes" "## mykit" "## mybot" \
  "release-notes.sh failed" "pkg/mykit@v0.3.0"

if [ "$failed" -ne 0 ]; then
  exit 1
fi
echo "release-notes: ok"
