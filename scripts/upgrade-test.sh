#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# The upgrade test of the release gate (docs/operations/releasing.md, "The upgrade test"): for each dialect, make a
# database with the images of the previous release PREVIOUS (a tag), fill it through that release's server (two local
# accounts, posts, a local follow, and a follow request to an account of remote.test, which the server fetches), then
# upgrade it with the images of the working tree in the order that deploy/docker/compose.yaml runs (my db upgrade
# -check, schema apply, my db upgrade, then the server) and check readiness, `my projection verify`, and that the data
# reads back. The stack is deploy/docker/upgrade-test/compose.yaml.
#
# The previous release's images are built from its tag (git archive), or taken from MY_PREVIOUS_IMAGE and
# MY_PREVIOUS_SCHEMA_IMAGE when both are set (a registry's images of that release). Prints the logs on failure and
# always removes the stack and its volumes.
#
#   scripts/upgrade-test.sh v0.1.3                    # SQLite and PostgreSQL
#   scripts/upgrade-test.sh v0.1.3 postgres
#
# Needs docker with compose, git (with the tags), curl, jq, openssl.
# The jq filters take their values as $variables (--arg), not from the shell.
# shellcheck disable=SC2016
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

fail() {
	echo "upgrade-test: $*" >&2
	exit 1
}

[[ $# -ge 1 ]] || fail "usage: scripts/upgrade-test.sh PREVIOUS_TAG [sqlite|postgres]..."
previous="$1"
shift
dialects=("$@")
[[ ${#dialects[@]} -gt 0 ]] || dialects=(sqlite postgres)
for d in "${dialects[@]}"; do
	[[ "$d" == sqlite || "$d" == postgres ]] || fail "unknown dialect: $d"
done
git -C "$root" rev-parse -q --verify "refs/tags/$previous" >/dev/null || fail "no tag $previous"

# shellcheck source=scripts/web-sign-in.sh
. "$root/scripts/web-sign-in.sh"

port="${UPGRADE_TEST_HTTP_PORT:-18081}"
base="http://127.0.0.1:$port"
candidate="$(git -C "$root" describe --tags --match 'v[0-9]*' --always --dirty 2>/dev/null || echo candidate)"

export MY_HTTP_PORT="$port"
export MY_HOST="localhost:$port"
export MY_WEB_ORIGIN="http://$MY_HOST"
MY_KEK="$(openssl rand -base64 32)"
export MY_KEK

work="$(mktemp -d)"
compose=(docker compose --project-directory "$root/deploy/docker/upgrade-test" -f "$root/deploy/docker/upgrade-test/compose.yaml" -p my-upgrade-test)

cleanup() {
	status=$?
	if [[ $status -ne 0 ]]; then
		echo "upgrade-test: failed; container status and logs follow" >&2
		"${compose[@]}" ps -a >&2 || true
		"${compose[@]}" logs --no-color >&2 || true
	fi
	"${compose[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
	rm -rf "$work"
	exit "$status"
}
trap cleanup EXIT

# --- Images --------------------------------------------------------------------------------------
previous_image="${MY_PREVIOUS_IMAGE:-}"
previous_schema_image="${MY_PREVIOUS_SCHEMA_IMAGE:-}"
if [[ -n "$previous_image" && -n "$previous_schema_image" ]]; then
	echo "upgrade-test: pulling the images of $previous" >&2
	docker pull -q "$previous_image" >/dev/null
	docker pull -q "$previous_schema_image" >/dev/null
else
	echo "upgrade-test: building the images of $previous from its tag" >&2
	previous_image="my-upgrade-test:previous"
	previous_schema_image="my-upgrade-test-schema:previous"
	git -C "$root" archive --format=tar "$previous" |
		docker build -q --build-arg "VERSION=$previous" --target app -t "$previous_image" - >/dev/null
	git -C "$root" archive --format=tar "$previous" |
		docker build -q --target schema -t "$previous_schema_image" - >/dev/null
fi
echo "upgrade-test: building the images of $candidate" >&2
candidate_image="my-upgrade-test:candidate"
candidate_schema_image="my-upgrade-test-schema:candidate"
docker build -q --build-arg "VERSION=$candidate" --target app -t "$candidate_image" "$root" >/dev/null
docker build -q --target schema -t "$candidate_schema_image" "$root" >/dev/null

# --- The private CA, remote.test, and the previous release's PostgreSQL preparation ---------------
certs="$work/certs"
remote="$work/remote"
mkdir -p "$certs" "$remote"
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 2 \
	-subj "/CN=my upgrade-test CA" -keyout "$certs/ca.key" -out "$certs/ca.pem" 2>/dev/null
openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes \
	-subj "/CN=remote.test" -keyout "$certs/server.key" -out "$certs/server.csr" 2>/dev/null
printf '%s\n' 'basicConstraints=critical,CA:FALSE' 'keyUsage=critical,digitalSignature' 'extendedKeyUsage=serverAuth' \
	'subjectAltName=DNS:remote.test' >"$certs/server.ext"
openssl x509 -req -in "$certs/server.csr" -CA "$certs/ca.pem" -CAkey "$certs/ca.key" -CAcreateserial \
	-days 2 -extfile "$certs/server.ext" -out "$certs/server.pem" 2>/dev/null

carol_id="https://remote.test/users/carol"
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$work/carol.key" 2>/dev/null
jq -n --arg id "$carol_id" --arg pem "$(openssl pkey -in "$work/carol.key" -pubout)" '{
	"@context": ["https://www.w3.org/ns/activitystreams", "https://w3id.org/security/v1"],
	id: $id, type: "Person", preferredUsername: "carol", name: "Carol",
	inbox: ($id + "/inbox"), outbox: ($id + "/outbox"), followers: ($id + "/followers"), following: ($id + "/following"),
	endpoints: {sharedInbox: "https://remote.test/inbox"}, url: "https://remote.test/@carol",
	manuallyApprovesFollowers: false,
	publicKey: {id: ($id + "#main-key"), owner: $id, publicKeyPem: $pem}
}' >"$remote/carol.json"
jq -n --arg id "$carol_id" '{
	subject: "acct:carol@remote.test",
	links: [{rel: "self", type: "application/activity+json", href: $id}]
}' >"$remote/webfinger.json"
# Throwaway keys and files, read by containers that run as other users.
chmod 0755 "$work" "$certs" "$remote"
chmod 0644 "$certs"/* "$remote"/*
git -C "$root" show "$previous:schema/postgres.bootstrap.sql" >"$work/postgres.bootstrap.sql"
chmod 0644 "$work/postgres.bootstrap.sql"
export UPGRADE_TEST_CERTS="$certs" UPGRADE_TEST_REMOTE="$remote" UPGRADE_TEST_BOOTSTRAP="$work/postgres.bootstrap.sql"

# --- Helpers -------------------------------------------------------------------------------------
wait_ready() {
	echo "upgrade-test: waiting for $base/readyz" >&2
	for i in $(seq 90); do
		if curl -fsS "$base/readyz" >/dev/null 2>&1; then
			return 0
		fi
		[[ $i -eq 90 ]] && fail "/readyz did not return 200 within 90 s"
		sleep 1
	done
}

# api METHOD PATH TOKEN [JSON]: a client API call that must succeed; prints the response.
api() {
	local args=(-fsS -X "$1" -H "Authorization: Bearer $3")
	if [[ $# -ge 4 ]]; then
		args+=(-H 'Content-Type: application/json' --data "$4")
	fi
	curl "${args[@]}" "$base/api/v1$2" || fail "$1 $2 failed"
}

# expect NAME JSON FILTER [jq args...]: fails unless the jq FILTER is true on JSON.
expect() {
	local name="$1" json="$2" filter="$3"
	shift 3
	echo "$json" | jq -e "$@" "$filter" >/dev/null || fail "$name: $json"
}

password=upgrade-test-password

# --- One dialect ---------------------------------------------------------------------------------
run() {
	local dialect="$1"
	echo "upgrade-test: $dialect: $previous -> $candidate" >&2
	export MY_DB="$dialect"
	if [[ "$dialect" == sqlite ]]; then
		export MY_DATABASE_URL=/data/my.db
		export SCHEMA_URL=sqlite:///data/my.db
		export SCHEMA_DEV_URL='sqlite://dev?mode=memory'
	else
		export MY_DATABASE_URL='postgres://my:my@db:5432/my?sslmode=disable'
		export SCHEMA_URL='postgres://my:my@db:5432/my?sslmode=disable&search_path=public'
		export SCHEMA_DEV_URL='postgres://my:my@db:5432/my_dev?sslmode=disable&search_path=public'
	fi

	# The previous release: apply its schema, start its server, and fill the database through it.
	export MY_IMAGE="$previous_image" MY_SCHEMA_IMAGE="$previous_schema_image"
	"${compose[@]}" up -d --wait db remote
	"${compose[@]}" run --rm --no-deps schema >/dev/null
	"${compose[@]}" up -d --no-deps my
	wait_ready

	local user
	for user in alice bob; do
		printf '%s\n' "$password" | "${compose[@]}" exec -T my my admin create -username "$user" -email "$user@example.com" >/dev/null
	done
	local alice_token bob_token
	alice_token="$(web_sign_in "$MY_WEB_ORIGIN" "$MY_WEB_ORIGIN" alice@example.com "$password")"
	bob_token="$(web_sign_in "$MY_WEB_ORIGIN" "$MY_WEB_ORIGIN" bob@example.com "$password")"

	local alice_id bob_id carol public_id private_id out
	alice_id="$(api GET '/accounts/verify_credentials' "$alice_token" | jq -r .id)"
	bob_id="$(api GET '/accounts/verify_credentials' "$bob_token" | jq -r .id)"
	public_id="$(api POST /statuses "$alice_token" '{"text": "upgrade-test public post", "visibility": "public"}' | jq -r .id)"
	private_id="$(api POST /statuses "$alice_token" '{"text": "upgrade-test followers-only post", "visibility": "private"}' | jq -r .id)"
	out="$(api POST "/accounts/$alice_id/follow" "$bob_token")"
	expect "bob follows alice" "$out" '.following == true'
	carol="$(api GET '/accounts/lookup?acct=carol@remote.test&resolve=true' "$alice_token")"
	expect "remote.test's carol is resolved" "$carol" '.acct == "carol@remote.test"'
	carol="$(echo "$carol" | jq -r .id)"
	out="$(api POST "/accounts/$carol/follow" "$alice_token")"
	expect "alice follows carol" "$out" '.following == true or .requested == true'

	# The candidate: stop the previous server, then upgrade-check, schema, upgrade, and the new server, in order.
	"${compose[@]}" stop my >/dev/null
	export MY_IMAGE="$candidate_image" MY_SCHEMA_IMAGE="$candidate_schema_image"
	"${compose[@]}" up -d my
	wait_ready
	local svc id state
	for svc in upgrade-check schema upgrade; do
		id="$("${compose[@]}" ps -a -q "$svc")"
		[[ -n "$id" ]] || fail "no container of the service $svc"
		state="$(docker inspect -f '{{.State.Status}} {{.State.ExitCode}}' "$id")"
		[[ "$state" == "exited 0" ]] || fail "the one-off service $svc did not succeed: $state"
	done
	out="$("${compose[@]}" logs --no-color upgrade-check)"
	[[ "$out" == *"ok: "* ]] || fail "upgrade-check did not report ok: $out"
	out="$("${compose[@]}" logs --no-color upgrade)"
	[[ "$out" == *"ok: "*"data upgrade step"* ]] || fail "upgrade did not report ok: $out"
	out="$("${compose[@]}" exec -T my my version)"
	[[ "$out" == *"$candidate"* ]] || fail "the server is not the candidate $candidate: $out"
	"${compose[@]}" exec -T my my projection verify || fail "my projection verify found a projection that differs from its sources"

	# The data reads back through the new server: the accounts sign in, and the posts and the follows are kept.
	alice_token="$(web_sign_in "$MY_WEB_ORIGIN" "$MY_WEB_ORIGIN" alice@example.com "$password")"
	bob_token="$(web_sign_in "$MY_WEB_ORIGIN" "$MY_WEB_ORIGIN" bob@example.com "$password")"
	expect "alice's account" "$(api GET /accounts/verify_credentials "$alice_token")" '.id == $id and .username == "alice"' --arg id "$alice_id"
	out="$(api GET "/accounts/$alice_id/statuses" "$bob_token")"
	expect "alice's posts as bob, her follower" "$out" '[.items[].id] | contains([$a, $b])' --arg a "$public_id" --arg b "$private_id"
	expect "alice's public post" "$(api GET "/statuses/$public_id" "$alice_token")" '.content | contains("upgrade-test public post")'
	expect "alice's followers-only post" "$(api GET "/statuses/$private_id" "$bob_token")" \
		'.visibility == "private" and (.content | contains("upgrade-test followers-only post"))'
	expect "alice's followers" "$(api GET "/accounts/$alice_id/followers" "$alice_token")" '[.accounts[].id] | index($id) != null' --arg id "$bob_id"
	expect "carol is known without fetching" "$(api GET '/accounts/lookup?acct=carol@remote.test' "$alice_token")" '.id == $id' --arg id "$carol"
	expect "alice's follow of carol" "$(api GET "/accounts/relationships?id=$carol" "$alice_token")" '.[0].following == true or .[0].requested == true'
	expect "a post after the upgrade" "$(api POST /statuses "$bob_token" '{"text": "upgrade-test after the upgrade", "visibility": "public"}')" '.id != null'
	expect "NodeInfo" "$(curl -fsS "$base/nodeinfo/2.1")" '.usage.users.total == 2 and .usage.localPosts == 3'

	"${compose[@]}" down -v --remove-orphans >/dev/null 2>&1
	echo "upgrade-test: $dialect: ok" >&2
}

for d in "${dialects[@]}"; do
	run "$d"
done
echo "upgrade-test: ok ($previous -> $candidate: ${dialects[*]})" >&2
