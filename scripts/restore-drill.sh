#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# The restore drill (docs/operations/backup.md, "Recovery drill"): for each dialect, start this tree's images on the
# stack of deploy/docker/upgrade-test/compose.yaml, fill the instance through the server (two local accounts, a post
# with an image, a followers-only post, a local follow, and a follow of carol on remote.test), and back it up while the
# server runs: `my db backup` (SQLite) or `pg_dump` (PostgreSQL), then the media. Then remove every container and
# volume, restore the backup into new ones, start the server with the same KEK, and check that the accounts sign in,
# the posts and the image read back, the Actor's public key is unchanged, and a delivery to remote.test carries a
# signature that the unchanged key verifies. Last, start the server with another KEK and check that delivery fails with
# `unknown key-encryption key`, and that the server refuses to start without a KEK.
# Prints the logs on failure and always removes the stack and its volumes.
#
#   scripts/restore-drill.sh                 # SQLite and PostgreSQL
#   scripts/restore-drill.sh postgres
#
# Needs docker with compose, curl, jq, openssl.
# The jq filters take their values as $variables (--arg), not from the shell.
# shellcheck disable=SC2016
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

fail() {
	echo "restore-drill: $*" >&2
	exit 1
}

dialects=("$@")
[[ ${#dialects[@]} -gt 0 ]] || dialects=(sqlite postgres)
for d in "${dialects[@]}"; do
	[[ "$d" == sqlite || "$d" == postgres ]] || fail "unknown dialect: $d"
done

# shellcheck source=scripts/web-sign-in.sh
. "$root/scripts/web-sign-in.sh"
# shellcheck source=scripts/remote-test-server.sh
. "$root/scripts/remote-test-server.sh"

port="${RESTORE_DRILL_HTTP_PORT:-18082}"
base="http://127.0.0.1:$port"
project=my-restore-drill
volume="${project}_data"

export MY_HTTP_PORT="$port"
export MY_HOST="localhost:$port"
export MY_WEB_ORIGIN="http://$MY_HOST"
MY_KEK="$(openssl rand -base64 32)"
export MY_KEK

work="$(mktemp -d)"
compose=(docker compose --project-directory "$root/deploy/docker/upgrade-test" -f "$root/deploy/docker/upgrade-test/compose.yaml" -p "$project")

cleanup() {
	status=$?
	if [[ $status -ne 0 ]]; then
		echo "restore-drill: failed; container status and logs follow" >&2
		"${compose[@]}" ps -a >&2 || true
		"${compose[@]}" logs --no-color >&2 || true
	fi
	"${compose[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
	rm -rf "$work"
	exit "$status"
}
trap cleanup EXIT

# --- Images and remote.test ----------------------------------------------------------------------
echo "restore-drill: building the images" >&2
version="$(git -C "$root" describe --tags --match 'v[0-9]*' --always --dirty 2>/dev/null || echo dev)"
export MY_IMAGE="my-restore-drill:$version" MY_SCHEMA_IMAGE="my-restore-drill-schema:$version"
docker build -q --build-arg "VERSION=$version" --target app -t "$MY_IMAGE" "$root" >/dev/null
docker build -q --target schema -t "$MY_SCHEMA_IMAGE" "$root" >/dev/null

remote_test_files "$work"
cp "$root/schema/postgres.bootstrap.sql" "$work/postgres.bootstrap.sql"
chmod 0644 "$work/postgres.bootstrap.sql"
export UPGRADE_TEST_CERTS="$work/certs" UPGRADE_TEST_REMOTE="$work/remote" UPGRADE_TEST_BOOTSTRAP="$work/postgres.bootstrap.sql"
backup="$work/backup"
mkdir -p "$backup"
chmod 0777 "$backup"

# --- Helpers -------------------------------------------------------------------------------------
wait_ready() {
	echo "restore-drill: waiting for $base/readyz" >&2
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

# actor_key USERNAME prints the publicKey (id and PEM) of the local Actor USERNAME as JSON.
actor_key() {
	local id
	id="$(curl -fsS "$base/.well-known/webfinger?resource=acct:$1@$MY_HOST" |
		jq -er '.links[] | select(.rel == "self" and .type == "application/activity+json") | .href')" || fail "WebFinger of $1"
	curl -fsS -H 'Accept: application/activity+json' "$base${id#"https://$MY_HOST"}" | jq -ec '.publicKey | {id, publicKeyPem}' ||
		fail "the Actor of $1"
}

# busybox runs a command in a throwaway container with the data volume at /data and the backup directory at /backup.
busybox() {
	docker run --rm -v "$volume:/data" -v "$backup:/backup" "$busybox_image" "$@"
}

# signed_inbox_posts prints, one JSON object per line, the POSTs to carol's inbox that remote.test logged.
signed_inbox_posts() {
	"${compose[@]}" logs --no-color --no-log-prefix remote 2>/dev/null |
		jq -c 'select(.request.method? == "POST" and .request.uri? == "/users/carol/inbox")' 2>/dev/null || true
}

# verify_signature POST_JSON PEM checks the draft-cavage Signature of a logged POST against PEM: it rebuilds the
# signing string from the covered headers and verifies it with openssl.
verify_signature() {
	local post="$1" pem="$2" sig headers signed="" h value
	sig="$(echo "$post" | jq -er '.request.headers.Signature[0]')" || fail "the POST has no Signature: $post"
	headers="$(echo "$sig" | sed -n 's/.*headers="\([^"]*\)".*/\1/p')"
	[[ -n "$headers" ]] || fail "the Signature covers no headers: $sig"
	for h in $headers; do
		case "$h" in
		"(request-target)") value="post $(echo "$post" | jq -r .request.uri)" ;;
		host) value="$(echo "$post" | jq -r .request.host)" ;;
		*) value="$(echo "$post" | jq -er --arg h "$h" '.request.headers | to_entries[] | select((.key | ascii_downcase) == $h) | .value[0]')" ||
			fail "the POST lacks the signed header $h: $post" ;;
		esac
		signed+="${signed:+$'\n'}$h: $value"
	done
	echo "$sig" | sed -n 's/.*signature="\([^"]*\)".*/\1/p' | openssl base64 -d -A >"$work/signature.bin"
	printf '%s' "$pem" >"$work/actor.pem"
	printf '%s' "$signed" | openssl dgst -sha256 -verify "$work/actor.pem" -signature "$work/signature.bin" >/dev/null ||
		fail "the signature does not verify with the Actor's key: $sig"
}

# wait_for_inbox_post KEY_ID waits for a POST to carol's inbox signed with KEY_ID and prints it.
wait_for_inbox_post() {
	local post
	for i in $(seq 60); do
		post="$(signed_inbox_posts | jq -c --arg k "keyId=\"$1\"" 'select(.request.headers.Signature[0] | contains($k))' | tail -n 1)"
		if [[ -n "$post" ]]; then
			echo "$post"
			return 0
		fi
		[[ $i -eq 60 ]] && fail "no POST to carol's inbox signed with $1 within 60 s"
		sleep 1
	done
}

busybox_image=busybox:1.37.0@sha256:bdf57e528e45e4433820e045b29b4597825a1c9e38353532d90a01445013f82e
password=restore-drill-password
# A 1×1 PNG.
png_b64='iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=='

# --- One dialect ---------------------------------------------------------------------------------
run() {
	local dialect="$1"
	echo "restore-drill: $dialect" >&2
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

	# The instance: start it as deploy/docker/compose.yaml does and fill it through the server.
	"${compose[@]}" up -d --wait db remote
	"${compose[@]}" up -d my
	wait_ready
	local user
	for user in alice bob; do
		printf '%s\n' "$password" | "${compose[@]}" exec -T my my admin create -username "$user" -email "$user@example.com" >/dev/null
	done
	local alice_token bob_token alice_id bob_id carol media_id public_id private_id media_url out
	alice_token="$(web_sign_in "$MY_WEB_ORIGIN" "$MY_WEB_ORIGIN" alice@example.com "$password")"
	bob_token="$(web_sign_in "$MY_WEB_ORIGIN" "$MY_WEB_ORIGIN" bob@example.com "$password")"
	alice_id="$(api GET /accounts/verify_credentials "$alice_token" | jq -r .id)"
	bob_id="$(api GET /accounts/verify_credentials "$bob_token" | jq -r .id)"

	echo "$png_b64" | openssl base64 -d -A >"$work/image.png"
	media_id="$(curl -fsS -H "Authorization: Bearer $alice_token" -F "file=@$work/image.png;type=image/png" "$base/api/v1/media" | jq -er .id)" ||
		fail "the upload failed"
	for i in $(seq 30); do
		out="$(api GET "/media/$media_id" "$alice_token")"
		[[ "$(echo "$out" | jq -r .state)" == ready ]] && break
		[[ $i -eq 30 ]] && fail "the upload was not processed within 30 s: $out"
		sleep 1
	done
	public_id="$(api POST /statuses "$alice_token" "$(jq -nc --arg m "$media_id" '{text: "restore-drill public post", visibility: "public", media: [{id: $m}]}')" | jq -r .id)"
	private_id="$(api POST /statuses "$alice_token" '{"text": "restore-drill followers-only post", "visibility": "private"}' | jq -r .id)"
	expect "bob follows alice" "$(api POST "/accounts/$alice_id/follow" "$bob_token")" '.following == true'
	carol="$(api GET '/accounts/lookup?acct=carol@remote.test&resolve=true' "$alice_token" | jq -r .id)"
	expect "alice follows carol" "$(api POST "/accounts/$carol/follow" "$alice_token")" '.following == true or .requested == true'

	media_url="$(api GET "/statuses/$public_id" "$alice_token" | jq -er '.media_attachments[0].url')" || fail "the post has no media"
	curl -fsS "$media_url" -o "$work/media.before" || fail "GET $media_url"
	local alice_key bob_key
	alice_key="$(actor_key alice)"
	bob_key="$(actor_key bob)"

	# The backup, while the server runs: the database first, then the media.
	echo "restore-drill: $dialect: backing up" >&2
	if [[ "$dialect" == sqlite ]]; then
		out="$("${compose[@]}" exec -T my my db backup /data/backup.db)"
		[[ "$out" == "ok: "* ]] || fail "my db backup: $out"
		busybox sh -c 'mv /data/backup.db /backup/my.db'
	else
		"${compose[@]}" exec -T db pg_dump -U postgres --format=custom my >"$backup/my.dump"
	fi
	busybox tar czf /backup/blobs.tar.gz -C /data --exclude=blobs/tmp blobs

	# The loss: every container and volume goes.
	"${compose[@]}" down -v --remove-orphans >/dev/null 2>&1

	# The restore, into new volumes and a new database: the database, then the media, then the same KEK.
	echo "restore-drill: $dialect: restoring" >&2
	"${compose[@]}" up -d --wait db remote
	# The data volume, labelled as Compose labels the volumes it creates.
	docker volume create --label "com.docker.compose.project=$project" --label com.docker.compose.volume=data "$volume" >/dev/null
	if [[ "$dialect" == sqlite ]]; then
		busybox cp /backup/my.db /data/my.db
	else
		"${compose[@]}" exec -T db pg_restore -U postgres --dbname=my --exit-on-error <"$backup/my.dump"
	fi
	# The server runs as the image's nonroot user (65532), which must own the volume.
	busybox sh -c 'tar xzf /backup/blobs.tar.gz -C /data && chown -R 65532:65532 /data'
	"${compose[@]}" up -d my
	wait_ready
	"${compose[@]}" exec -T my my projection verify || fail "my projection verify found a projection that differs from its sources"

	# The instance is back: sign-in, the posts, the image, the follows, and the Actors' keys.
	alice_token="$(web_sign_in "$MY_WEB_ORIGIN" "$MY_WEB_ORIGIN" alice@example.com "$password")"
	bob_token="$(web_sign_in "$MY_WEB_ORIGIN" "$MY_WEB_ORIGIN" bob@example.com "$password")"
	expect "alice's account" "$(api GET /accounts/verify_credentials "$alice_token")" '.id == $id' --arg id "$alice_id"
	expect "alice's public post" "$(api GET "/statuses/$public_id" "$bob_token")" \
		'(.content | contains("restore-drill public post")) and .media_attachments[0].url == $url' --arg url "$media_url"
	expect "alice's followers-only post as bob" "$(api GET "/statuses/$private_id" "$bob_token")" '.visibility == "private"'
	expect "alice's followers" "$(api GET "/accounts/$alice_id/followers" "$alice_token")" '[.accounts[].id] | index($id) != null' --arg id "$bob_id"
	expect "alice's follow of carol" "$(api GET "/accounts/relationships?id=$carol" "$alice_token")" '.[0].following == true or .[0].requested == true'
	curl -fsS "$media_url" -o "$work/media.after" || fail "GET $media_url after the restore"
	cmp -s "$work/media.before" "$work/media.after" || fail "the image differs after the restore"
	[[ "$(actor_key alice)" == "$alice_key" ]] || fail "alice's public key changed"
	[[ "$(actor_key bob)" == "$bob_key" ]] || fail "bob's public key changed"

	# A signed delivery: alice unfollows carol, and remote.test receives an Undo signed with alice's unchanged key.
	local post
	api POST "/accounts/$carol/unfollow" "$alice_token" >/dev/null
	post="$(wait_for_inbox_post "$(echo "$alice_key" | jq -r .id)")"
	verify_signature "$post" "$(echo "$alice_key" | jq -r .publicKeyPem)"
	echo "restore-drill: $dialect: remote.test verified a POST signed with $(echo "$alice_key" | jq -r .id) after the restore" >&2

	# Another KEK: the server starts, and anything that reads a private key fails with the key ID.
	echo "restore-drill: $dialect: another KEK" >&2
	MY_KEK="$(openssl rand -base64 32)" "${compose[@]}" up -d --no-deps --force-recreate my
	wait_ready
	curl -sS -o /dev/null -X POST -H "Authorization: Bearer $alice_token" "$base/api/v1/accounts/$carol/follow" || true
	for i in $(seq 60); do
		out="$("${compose[@]}" logs --no-color --no-log-prefix my 2>&1 | grep -m 1 'unknown key-encryption key' || true)"
		if [[ -n "$out" ]]; then
			echo "restore-drill: $dialect: with another KEK: $out" >&2
			break
		fi
		[[ $i -eq 60 ]] && fail "no 'unknown key-encryption key' error in the server's log within 60 s"
		sleep 1
	done

	# No KEK: the server does not start.
	out="$("${compose[@]}" run --rm --no-deps -e MY_KEK= my serve 2>&1)" && fail "my serve started without a KEK: $out"
	out="$(echo "$out" | grep -m 1 'kek: ' || true)"
	[[ -n "$out" ]] || fail "my serve without a KEK failed for another reason"
	echo "restore-drill: $dialect: without a KEK: $out" >&2

	"${compose[@]}" down -v --remove-orphans >/dev/null 2>&1
	rm -rf "${backup:?}"/*
	echo "restore-drill: $dialect: ok" >&2
}

for d in "${dialects[@]}"; do
	run "$d"
done
echo "restore-drill: ok (${dialects[*]})" >&2
