#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Smoke test of deploy/docker/compose.yaml, the path an operator takes: build the server image with the override
# compose.build.yaml and give it the published image's name, so that the plain compose.yaml starts it as it starts a
# pulled one. Start the stack on an empty volume (my db upgrade -check, my schema apply, my db upgrade, then the server), wait for readiness, create an admin, check WebFinger, the Actor,
# NodeInfo, and the embedded web client, and sign in as the web client does over plain HTTP on localhost
# (README.md, "Quick start"). Then start the stack again on the same volume, as an upgrade does, and check that the
# one-off services ran in order, that the server is ready with the data kept, and that a failing one-off service
# keeps the server stopped (docs/operations/database.md, "Upgrading"). The settings come from a copy of example.env and
# the KEK from a secret file, as for an operator; the check of the layers is `docker compose config` on each of them.
# Prints the logs on failure and always removes the stack and its volume.
# With SMOKE_PUBLISHED=<image tag> (0.1.0), it pulls that published image instead of building, to check a release.
# Usage: scripts/docker-smoke.sh   (needs docker with compose, curl, jq, openssl)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
port="${SMOKE_HTTP_PORT:-18080}"
base="http://127.0.0.1:$port"

# Throwaway settings for this run only, in a project directory of its own, so that a deploy/docker/.env or
# deploy/docker/secrets stays out of it. It is two levels below the root, as deploy/docker is, for the build context of
# compose.build.yaml.
project="$root/.tmp/docker-smoke"
export MY_HOST="localhost:$port"
export MY_WEB_ORIGIN="http://$MY_HOST"
export MY_HTTP_PORT="$port"
published="${SMOKE_PUBLISHED:-}"
export MY_VERSION="${published:-${MY_VERSION:-smoke}}"
# What the server reports: a release's Git tag, and otherwise the VERSION of the build.
want_version="$MY_VERSION"
if [[ -n "$published" ]]; then want_version="v$published"; fi
published_image="ghcr.io/mycroblog/my:$MY_VERSION"
# A setting that compose.yaml does not name, which reaches the container through the env file alone.
env_file_setting='MY_RELAY_MAX_ACTIVITIES_PER_HOUR=999'

rm -rf "$project"
mkdir -p "$project/secrets"
chmod 700 "$project/secrets"
(
	umask 022
	openssl rand -base64 32 >"$project/secrets/kek"
)
{
	cat "$root/deploy/docker/example.env"
	printf 'MY_VERSION=%s\nMY_HOST=%s\nMY_WEB_ORIGIN=%s\nMY_HTTP_PORT=%s\n%s\n' \
		"$MY_VERSION" "$MY_HOST" "$MY_WEB_ORIGIN" "$MY_HTTP_PORT" "$env_file_setting"
} >"$project/.env"

# shellcheck source=scripts/web-sign-in.sh
. "$root/scripts/web-sign-in.sh"

compose=(docker compose --project-directory "$project" -f "$root/deploy/docker/compose.yaml" -p my-smoke)
build=("${compose[@]}" -f "$root/deploy/docker/compose.build.yaml")

fail() {
	echo "docker-smoke: $*" >&2
	exit 1
}

cleanup() {
	status=$?
	if [[ $status -ne 0 ]]; then
		echo "docker-smoke: failed; container status and logs follow" >&2
		"${compose[@]}" ps -a >&2 || true
		"${compose[@]}" logs --no-color >&2 || true
	fi
	"${compose[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
	if [[ -z "$published" ]]; then docker rmi "$published_image" >/dev/null 2>&1 || true; fi
	rm -rf "$project"
	exit "$status"
}
trap cleanup EXIT

wait_ready() {
	echo "docker-smoke: waiting for $base/readyz" >&2
	for i in $(seq 60); do
		if curl -fsS "$base/readyz" >/dev/null 2>&1; then
			return 0
		fi
		[[ $i -eq 60 ]] && fail "/readyz did not return 200 within 60 s"
		sleep 1
	done
}

# The one-off services run in this order, each after the previous one exited with 0, and then the server starts.
# check_order WANT also checks that my schema apply printed WANT: statements applied on an empty volume, none after.
order=(upgrade-check schema upgrade my)
check_order() {
	local prev_end="" svc id state started
	for svc in "${order[@]}"; do
		id="$("${compose[@]}" ps -a -q "$svc")"
		[[ -n "$id" ]] || fail "no container of the service $svc"
		state="$(docker inspect -f '{{.State.Status}} {{.State.ExitCode}}' "$id")"
		started="$(date -d "$(docker inspect -f '{{.State.StartedAt}}' "$id")" +%s%N)"
		if [[ "$svc" == my ]]; then
			[[ "$state" == "running 0" ]] || fail "the server is not running: $state"
		else
			[[ "$state" == "exited 0" ]] || fail "the one-off service $svc did not succeed: $state"
		fi
		[[ -z "$prev_end" || "$started" -ge "$prev_end" ]] || fail "$svc started before the service ahead of it ended"
		prev_end="$(date -d "$(docker inspect -f '{{.State.FinishedAt}}' "$id")" +%s%N)"
	done
	local out
	out="$("${compose[@]}" logs --no-color upgrade-check)"
	[[ "$out" == *"ok: the database can be upgraded to this release"* ]] || fail "upgrade-check did not report ok: $out"
	out="$("${compose[@]}" logs --no-color schema)"
	[[ "$out" == *"$1"* ]] || fail "schema did not report \"$1\": $out"
	out="$("${compose[@]}" logs --no-color upgrade)"
	[[ "$out" == *"ok: "*"data upgrade step"* ]] || fail "upgrade did not report ok: $out"
}

# Each layer, alone and all together, is a valid Compose file with the settings its header asks for.
echo "docker-smoke: checking the layers" >&2
for layers in postgres s3 caddy "postgres s3 caddy"; do
	files=()
	for layer in $layers; do files+=(-f "$root/deploy/docker/compose.$layer.yaml"); done
	POSTGRES_PASSWORD=smoke "${compose[@]}" "${files[@]}" config -q
done

# An image is never a test build: the Dockerfile refuses the test-only build tag e2e_fakes before it compiles anything.
echo "docker-smoke: checking that the Dockerfile refuses GO_TAGS with e2e_fakes" >&2
if out="$(docker build --progress=plain --target build --build-arg GO_TAGS=no_valkey,e2e_fakes "$root" 2>&1)"; then
	fail "docker build with GO_TAGS=no_valkey,e2e_fakes succeeded"
fi
[[ "$out" == *"which only test builds may have"* ]] || fail "docker build with GO_TAGS=no_valkey,e2e_fakes failed for another reason: $out"

if [[ -n "$published" ]]; then
	"${compose[@]}" pull
else
	# The building option, and then the local image under the published name: nothing below can reach the registry.
	"${build[@]}" build
	docker tag "my:$MY_VERSION" "$published_image"
fi
"${compose[@]}" up -d --pull never
wait_ready
check_order "statement(s) applied"

"${compose[@]}" exec -T my my version

# The env file reaches the container, without the KEK; and with the KEK in it the server refuses to start.
env="$(docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$("${compose[@]}" ps -q my)")"
grep -qx "$env_file_setting" <<<"$env" || fail "$env_file_setting of the env file did not reach the container"
grep -qx 'MY_KEK_FILE=/run/secrets/my_kek' <<<"$env" || fail "the container has no MY_KEK_FILE"
if grep -q '^MY_KEK=' <<<"$env"; then fail "the KEK reached the container as an environment variable"; fi
if out="$("${compose[@]}" run --rm --no-deps -e MY_KEK="$(cat "$project/secrets/kek")" my config check 2>&1)"; then
	fail "the server accepted MY_KEK together with MY_KEK_FILE: $out"
fi
[[ "$out" == *"MY_KEK"* ]] || fail "unexpected error with both MY_KEK and MY_KEK_FILE: $out"
# The variables that only the compose files read do not count as misspelled settings.
# (It exits with 1 here for being no production setup, such as the host localhost.)
out="$("${compose[@]}" exec -T my my config check 2>&1 || true)"
[[ "$out" == *relay_max_activities_per_hour* ]] || fail "unexpected output of my config check: $out"
if [[ "$out" == *MY_VERSION* || "$out" == *MY_HTTP_PORT* ]]; then fail "my config check warns about a variable of the compose files: $out"; fi

printf 'smoke-test-password\n' | "${compose[@]}" exec -T my my admin create -username alice -email alice@example.com

accept_ap='application/activity+json'
webfinger="$(curl -fsS "$base/.well-known/webfinger?resource=acct:alice@$MY_HOST")"
echo "$webfinger" | jq -e --arg subject "acct:alice@$MY_HOST" '.subject == $subject' >/dev/null ||
	fail "WebFinger subject is not acct:alice@$MY_HOST: $webfinger"
actor_id="$(echo "$webfinger" | jq -er '.links[] | select(.rel == "self" and .type == "application/activity+json") | .href')" ||
	fail "WebFinger has no ActivityPub self link: $webfinger"
[[ "$actor_id" == "https://$MY_HOST/ap/actors/"* ]] || fail "unexpected Actor id in WebFinger: $actor_id"

# The id is https://{host}/...; fetch it from the published port over plain HTTP.
actor="$(curl -fsS -H "Accept: $accept_ap" "$base${actor_id#"https://$MY_HOST"}")"
echo "$actor" | jq -e --arg id "$actor_id" '
	.id == $id and .type == "Person" and .preferredUsername == "alice"
	and (.inbox | startswith($id)) and (.outbox | startswith($id))
	and .publicKey.id == ($id + "#main-key") and .publicKey.owner == $id
	and (.publicKey.publicKeyPem | startswith("-----BEGIN PUBLIC KEY-----"))
' >/dev/null || fail "unexpected Actor: $actor"

nodeinfo="$(curl -fsS "$base/nodeinfo/2.1")"
echo "$nodeinfo" | jq -e --arg v "$want_version" '.version == "2.1" and .software.version == $v and .usage.users.total == 1' >/dev/null ||
	fail "unexpected NodeInfo: $nodeinfo"

# The image embeds the web client: the shell loads its entry module from /assets/ (ADR-0028).
shell="$(curl -fsS -H 'Accept: text/html' "$base/@alice")"
entry="$(echo "$shell" | grep -o '<script type="module" src="/assets/[^"]*"' | head -n 1 | sed 's/.*src="//; s/"$//' || true)"
[[ -n "$entry" ]] || fail "the HTML shell loads no entry module from /assets/ (web client not embedded?): $shell"
echo "$shell" | grep -q '<script type="application/json" id="config">' || fail "the HTML shell has no runtime config: $shell"
curl -fsS -o /dev/null "$base$entry" || fail "the entry module $entry is not served"
curl -fsS "$base/manifest.webmanifest" | jq -e --arg name "$MY_HOST" '.name == $name' >/dev/null ||
	fail "unexpected Web App Manifest"

# Sign in without a browser, as the web client does (OAuth 2.0 authorization code with PKCE, ADR-0090): the shell's
# apiOrigin, the authorization endpoint, the sign-in form, the redirect back with a code and the issuer, the token
# endpoint, and an API call with the access token. Over plain HTTP on localhost, every step stays on http://.
config="$(echo "$shell" | sed -n 's|.*<script type="application/json" id="config">\([^<]*\)</script>.*|\1|p')"
api_origin="$(echo "$config" | jq -r '.apiOrigin')"
[[ "$api_origin" == "$MY_WEB_ORIGIN" ]] || fail "the shell's apiOrigin is $api_origin, want $MY_WEB_ORIGIN"

access="$(web_sign_in "$api_origin" "$MY_WEB_ORIGIN" alice@example.com smoke-test-password)"
me="$(curl -fsS -H "Authorization: Bearer $access" "$api_origin/api/v1/accounts/verify_credentials")"
echo "$me" | jq -e '.username == "alice"' >/dev/null || fail "unexpected verify_credentials: $me"

# Start again on the same volume, as an upgrade to a new image does: the one-off services run again in order on the
# existing database, and the server comes back ready with the account kept.
echo "docker-smoke: starting again on the same volume" >&2
"${compose[@]}" up -d --force-recreate
wait_ready
check_order "ok: the database matches the schema; nothing to apply"
curl -fsS "$base/.well-known/webfinger?resource=acct:alice@$MY_HOST" | jq -e --arg subject "acct:alice@$MY_HOST" '.subject == $subject' >/dev/null ||
	fail "the account is gone after the second start"
curl -fsS "$base/nodeinfo/2.1" | jq -e '.usage.users.total == 1' >/dev/null || fail "unexpected NodeInfo after the second start"

# A failing one-off service keeps the server stopped: upgrade-check exits with 2 on an unknown flag here.
echo "docker-smoke: checking that a failing upgrade-check keeps the server stopped" >&2
"${compose[@]}" stop my >/dev/null
failing="$(mktemp)"
printf 'services:\n  upgrade-check:\n    command: [db, upgrade, -no-such-flag]\n' >"$failing"
if "${compose[@]}" -f "$failing" up -d >/dev/null 2>&1; then
	rm -f "$failing"
	fail "docker compose up succeeded although upgrade-check failed"
fi
rm -f "$failing"
state="$(docker inspect -f '{{.State.Status}}' "$("${compose[@]}" ps -a -q my)")"
[[ "$state" != running ]] || fail "the server started although upgrade-check failed"

echo "docker-smoke: ok ($actor_id)" >&2
