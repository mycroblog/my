#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Smoke test of deploy/docker/compose.yaml: build the images, start the stack on an empty volume (my db upgrade -check,
# schema apply, my db upgrade, then the server), wait for readiness, create an admin, check WebFinger, the Actor,
# NodeInfo, and the embedded web client, and sign in as the web client does over plain HTTP on localhost
# (README.md, "Quick start"). Then start the stack again on the same volume, as an upgrade does, and check that the
# one-off services ran in order, that the server is ready with the data kept, and that a failing one-off service
# keeps the server stopped (docs/operations/database.md, "Upgrading").
# Prints the logs on failure and always removes the stack and its volume.
# Usage: scripts/docker-smoke.sh   (needs docker with compose, curl, jq, openssl)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
port="${SMOKE_HTTP_PORT:-18080}"
base="http://127.0.0.1:$port"

# Throwaway settings for this run only; the shell environment takes precedence over any deploy/docker/.env.
export MY_HOST="localhost:$port"
export MY_WEB_ORIGIN="http://$MY_HOST"
export MY_HTTP_PORT="$port"
export MY_VERSION="${MY_VERSION:-smoke}"
MY_KEK="$(openssl rand -base64 32)"
export MY_KEK

compose=(docker compose --project-directory "$root/deploy/docker" -f "$root/deploy/docker/compose.yaml" -p my-smoke)

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
	out="$("${compose[@]}" logs --no-color upgrade)"
	[[ "$out" == *"ok: "*"data upgrade step"* ]] || fail "upgrade did not report ok: $out"
}

"${compose[@]}" up -d --build
wait_ready
check_order

"${compose[@]}" exec -T my my version
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
echo "$nodeinfo" | jq -e --arg v "$MY_VERSION" '.version == "2.1" and .software.version == $v and .usage.users.total == 1' >/dev/null ||
	fail "unexpected NodeInfo: $nodeinfo"

# The image embeds the web client: the shell loads its entry module from /assets/ (ADR-0028).
shell="$(curl -fsS -H 'Accept: text/html' "$base/@alice")"
entry="$(echo "$shell" | grep -o '<script type="module" src="/assets/[^"]*"' | head -n 1 | sed 's/.*src="//; s/"$//' || true)"
[[ -n "$entry" ]] || fail "the HTML shell loads no entry module from /assets/ (web client not embedded?): $shell"
echo "$shell" | grep -q '<script type="application/json" id="config">' || fail "the HTML shell has no runtime config: $shell"
curl -fsS -o /dev/null "$base$entry" || fail "the entry module $entry is not served"
curl -fsS "$base/manifest.webmanifest" | jq -e --arg name "$MY_HOST" '.name == $name' >/dev/null ||
	fail "unexpected Web App Manifest"

# Sign in without a browser, as the web client does (OAuth 2.0 authorization code with PKCE, ADR-0030): the shell's
# apiOrigin, the authorization endpoint, the sign-in form, the redirect back with a code and the issuer, the token
# endpoint, and an API call with the access token. Over plain HTTP on localhost, every step stays on http://.
config="$(echo "$shell" | sed -n 's|.*<script type="application/json" id="config">\([^<]*\)</script>.*|\1|p')"
api_origin="$(echo "$config" | jq -r '.apiOrigin')"
[[ "$api_origin" == "$MY_WEB_ORIGIN" ]] || fail "the shell's apiOrigin is $api_origin, want $MY_WEB_ORIGIN"

b64url() { openssl base64 -A | tr '+/' '-_' | tr -d '='; }
# The value of a query parameter of a URL, percent-decoded.
param() {
	local v
	v="$(printf '%s' "${1#*\?}" | tr '&' '\n' | sed -n "s/^$2=//p" | head -n 1)"
	v="${v//+/ }"
	printf '%b' "${v//%/\\x}"
}
# A hidden field of the sign-in form.
field() { echo "$2" | grep -o "name=\"$1\" value=\"[^\"]*\"" | head -n 1 | sed 's/.*value="//; s/"$//; s/&amp;/\&/g'; }

verifier="$(openssl rand 32 | b64url)"
challenge="$(printf '%s' "$verifier" | openssl dgst -sha256 -binary | b64url)"
state="$(openssl rand 16 | b64url)"
callback="$MY_WEB_ORIGIN/login/callback"
jar="$(mktemp)"
# The cookie jar keeps the session and CSRF cookies: curl, like browsers, takes Secure cookies from http://localhost.
browser=(curl -fsS -b "$jar" -c "$jar")
redirect=("${browser[@]}" -o /dev/null -w '%{redirect_url}')

sign_in="$("${redirect[@]}" -G --data-urlencode response_type=code --data-urlencode client_id=my-web \
	--data-urlencode "redirect_uri=$callback" --data-urlencode "code_challenge=$challenge" \
	--data-urlencode code_challenge_method=S256 --data-urlencode "state=$state" "$api_origin/oauth/authorize")"
[[ "$sign_in" == "$api_origin/auth/sign_in?"* ]] || fail "the authorization endpoint redirects to '$sign_in', want the sign-in page"
form="$("${browser[@]}" "$sign_in")"
csrf="$(field csrf_token "$form")"
return_to="$(field return_to "$form")"
[[ -n "$csrf" && -n "$return_to" ]] || fail "the sign-in page has no csrf_token or return_to: $form"
authorize="$("${redirect[@]}" -H "Origin: $api_origin" --data-urlencode "csrf_token=$csrf" --data-urlencode "return_to=$return_to" \
	--data-urlencode email=alice@example.com --data-urlencode password=smoke-test-password "$api_origin/auth/sign_in")"
[[ "$authorize" == "$api_origin/oauth/authorize?"* ]] || fail "signing in redirects to '$authorize', want the authorization endpoint"
back="$("${redirect[@]}" "$authorize")"
[[ "$back" == "$callback?"* ]] || fail "the authorization endpoint redirects to '$back', want $callback"
[[ "$(param "$back" state)" == "$state" ]] || fail "the redirect does not carry the state: $back"
[[ "$(param "$back" iss)" == "$api_origin" ]] || fail "the redirect's issuer is not $api_origin: $back"
code="$(param "$back" code)"
[[ -n "$code" ]] || fail "the redirect has no code: $back"

tokens="$("${browser[@]}" -H "Origin: $MY_WEB_ORIGIN" --data-urlencode grant_type=authorization_code \
	--data-urlencode "code=$code" --data-urlencode "redirect_uri=$callback" --data-urlencode client_id=my-web \
	--data-urlencode "code_verifier=$verifier" "$api_origin/oauth/token")"
access="$(echo "$tokens" | jq -er '.access_token')" || fail "unexpected token response: $tokens"
grep -q '__Host-my_rt' "$jar" || fail "the token endpoint set no refresh token cookie"
me="$(curl -fsS -H "Authorization: Bearer $access" "$api_origin/api/v1/accounts/verify_credentials")"
echo "$me" | jq -e '.username == "alice"' >/dev/null || fail "unexpected verify_credentials: $me"
rm -f "$jar"

# Start again on the same volume, as an upgrade to a new image does: the one-off services run again in order on the
# existing database, and the server comes back ready with the account kept.
echo "docker-smoke: starting again on the same volume" >&2
"${compose[@]}" up -d --force-recreate
wait_ready
check_order
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
