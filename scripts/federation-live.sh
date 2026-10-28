#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Live interoperability test (ADR-0006, ADR-0015, docs/architecture/federation.md §12): start
# deploy/docker/federation-live/compose.yaml (this server, Mastodon, GoToSocial, Misskey, Akkoma, Activity-Relay,
# a private CA), create
# the users on each server, and run the scenarios in deploy/docker/federation-live (go test -tags federationlive).
#
# Whatever the outcome, it writes to $FEDERATION_LIVE_OUT (default .tmp/federation-live/out):
#   logs/<service>.log   the log of every container
#   captured/<host>/…    every activity this server received, as stored, for new fixtures (README.md)
#   scenarios.log        the go test output
# and removes the stack, unless FEDERATION_LIVE_KEEP=1.
#
# Usage: scripts/federation-live.sh [go test flags, e.g. -run TestFollow]
#   (needs docker with compose, go, curl, jq, openssl)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
dir="$root/deploy/docker/federation-live"
work="${FEDERATION_LIVE_DIR:-$root/.tmp/federation-live}"
out="${FEDERATION_LIVE_OUT:-$work/out}"
certs="$work/certs"
port="${FEDERATION_LIVE_HTTPS_PORT:-18443}"
db_port="${FEDERATION_LIVE_DB_PORT:-15432}"

log() { echo "federation-live: $*" >&2; }
fail() {
	log "$*"
	exit 1
}

rm -rf "$work" "$out"
mkdir -p "$certs" "$out/logs" "$out/captured"

# --- Private CA and the certificate of every host -----------------------------------------------
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 2 \
	-subj "/CN=my federation-live CA" -keyout "$certs/ca.key" -out "$certs/ca.pem" 2>/dev/null
openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes \
	-subj "/CN=my.test" -keyout "$certs/server.key" -out "$certs/server.csr" 2>/dev/null
printf '%s\n' \
	'basicConstraints=critical,CA:FALSE' \
	'keyUsage=critical,digitalSignature' \
	'extendedKeyUsage=serverAuth' \
	'subjectAltName=DNS:my.test,DNS:mastodon.test,DNS:gts.test,DNS:misskey.test,DNS:akkoma.test,DNS:relay.test' >"$certs/server.ext"
openssl x509 -req -in "$certs/server.csr" -CA "$certs/ca.pem" -CAkey "$certs/ca.key" -CAcreateserial \
	-days 2 -extfile "$certs/server.ext" -out "$certs/server.pem" 2>/dev/null
# The actor key of the relay: Activity-Relay reads a PKCS #1 RSA key.
openssl genrsa -traditional -out "$certs/relay-actor.pem" 2048 2>/dev/null
# Throwaway keys, read by containers that run as other users.
chmod 0644 "$certs"/*

# --- Secrets of this run only -------------------------------------------------------------------
rand() { openssl rand -hex "$1"; }
my_password="$(rand 16)"
gts_password="Aa1!$(rand 16)"
misskey_password="$(rand 16)"
akkoma_password="$(rand 16)"
export FEDERATION_LIVE_CERTS="$certs"
export FEDERATION_LIVE_HTTPS_PORT="$port"
export FEDERATION_LIVE_DB_PORT="$db_port"
export MY_VERSION="${MY_VERSION:-federation-live}"
MY_KEK="$(openssl rand -base64 32)"
MASTODON_SECRET_KEY_BASE="$(rand 64)"
MASTODON_OTP_SECRET="$(rand 64)"
MASTODON_AR_DETERMINISTIC_KEY="$(rand 16)"
MASTODON_AR_KEY_DERIVATION_SALT="$(rand 16)"
MASTODON_AR_PRIMARY_KEY="$(rand 16)"
AKKOMA_SECRET_KEY_BASE="$(rand 64)"
AKKOMA_SIGNING_SALT="$(rand 8)"
export AKKOMA_SECRET_KEY_BASE AKKOMA_SIGNING_SALT
export MY_KEK MASTODON_SECRET_KEY_BASE MASTODON_OTP_SECRET MASTODON_AR_DETERMINISTIC_KEY \
	MASTODON_AR_KEY_DERIVATION_SALT MASTODON_AR_PRIMARY_KEY

compose=(docker compose --project-directory "$dir" -f "$dir/compose.yaml" -p my-federation-live)

collect() {
	"${compose[@]}" ps -a >"$out/logs/ps.txt" 2>&1 || true
	for service in $("${compose[@]}" config --services); do
		"${compose[@]}" logs --no-color --timestamps "$service" >"$out/logs/$service.log" 2>&1 || true
	done
	# Every received activity as stored (received_activity_documents), one file per activity under the
	# sender's host with the bytes of the request body, for adding fixtures (README.md,
	# pkg/testkit/README.md "Federation fixtures"). The document travels base64-encoded on one line.
	"${compose[@]}" exec -T db psql -U postgres -d my -AtX -c "
		SELECT json_build_object('actor', a.actor_uri, 'type', a.type,
			'document', translate(encode(d.document, 'base64'), E'\\n', ''))
		FROM received_activities a JOIN received_activity_documents d USING (activity_uri)
		ORDER BY a.received_at, a.activity_uri" 2>/dev/null |
		{
			n=0
			while IFS= read -r row; do
				n=$((n + 1))
				host="$(jq -r '.actor | sub("^https://"; "") | sub("/.*$"; "")' <<<"$row")"
				name="$(jq -r '.type | ascii_downcase' <<<"$row")"
				mkdir -p "$out/captured/$host"
				jq -r '.document' <<<"$row" | base64 -d >"$out/captured/$host/$(printf '%03d' "$n")-$name.json"
			done
		} || true
}

cleanup() {
	status=$?
	collect
	if [[ $status -ne 0 ]]; then
		log "failed; the logs of every server are in $out/logs"
		"${compose[@]}" ps -a >&2 || true
	fi
	if [[ "${FEDERATION_LIVE_KEEP:-0}" == 1 ]]; then
		log "keeping the stack (FEDERATION_LIVE_KEEP=1); remove it with: docker compose -p my-federation-live down -v"
	else
		"${compose[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
	fi
	exit "$status"
}
trap cleanup EXIT

log "starting the stack"
"${compose[@]}" up -d --build --quiet-pull

# Reach a server through the proxy as the other servers do: by its host name, over TLS with the private CA.
https() {
	local host="$1" path="$2"
	shift 2
	curl -fsS --cacert "$certs/ca.pem" --connect-to "$host:443:127.0.0.1:$port" "$@" "https://$host$path"
}

wait_ready() {
	local host="$1" path="$2"
	log "waiting for https://$host$path"
	for i in $(seq 300); do
		if https "$host" "$path" -o /dev/null 2>/dev/null; then
			return 0
		fi
		if [[ $i -eq 300 ]]; then
			fail "https://$host$path did not answer within 300 s"
		fi
		sleep 1
	done
}
wait_ready my.test /readyz
wait_ready mastodon.test /health
wait_ready gts.test /readyz
wait_ready misskey.test /healthz
wait_ready akkoma.test /api/v1/instance
wait_ready relay.test /actor

# --- Users --------------------------------------------------------------------------------------
# alice@my.test, frank@my.test (the user that the suspension scenario suspends), and otto@my.test (the
# federation mode scenario's user, whom no peer knows before it). On Mastodon: bob
# (alice's follower), dave (never follows alice: the mentioned non-follower of the visibility scenario),
# eve (the signature scenario), judy (the relay scenario: nobody on my.test follows her), and the
# administrator mod. ivan@my.test is the relay scenario's user, whom nobody on Mastodon follows. On GoToSocial: carol (alice's follower) and
# erin (never follows alice). On Misskey: grace (alice's follower) and heidi (never follows alice), made
# by its administrator mkadmin. The move scenario (TestMove) has arrive and leave on every server, and
# leavemastodon, leavegotosocial, and leavemisskey on my.test: each of them moves once.
log "creating alice, frank, ivan, otto, arrive, and leave<peer> @my.test, bob, dave, eve, judy, mod, arrive, and leave @mastodon.test, carol, erin, arrive, and leave @gts.test, and grace, heidi, arrive, and leave @misskey.test"
printf '%s\n' "$my_password" | "${compose[@]}" exec -T my my admin create -username alice -email alice@my.test >/dev/null
printf '%s\n' "$my_password" | "${compose[@]}" exec -T my my admin create -username frank -email frank@my.test >/dev/null
printf '%s\n' "$my_password" | "${compose[@]}" exec -T my my admin create -username ivan -email ivan@my.test >/dev/null
printf '%s\n' "$my_password" | "${compose[@]}" exec -T my my admin create -username otto -email otto@my.test >/dev/null
for user in arrive leavemastodon leavegotosocial leavemisskey; do
	printf '%s\n' "$my_password" | "${compose[@]}" exec -T my my admin create -username "$user" -email "$user@my.test" >/dev/null
done

for user in bob dave eve judy arrive leave; do
	"${compose[@]}" exec -T mastodon-web bin/tootctl accounts create "$user" --email "$user@mastodon.test" --confirmed --approve >/dev/null
done
# mod is the administrator whose token reads the reports (TestReportForwarding).
"${compose[@]}" exec -T mastodon-web bin/tootctl accounts create mod --email mod@mastodon.test --confirmed --approve --role Owner >/dev/null
mastodon_tokens="$("${compose[@]}" exec -T mastodon-web bin/rails runner '
	app = Doorkeeper::Application.create!(name: "federation-live", redirect_uri: "urn:ietf:wg:oauth:2.0:oob", scopes: "read write follow")
	%w[bob dave eve judy].each do |name|
		user = Account.find_local!(name).user
		token = Doorkeeper::AccessToken.create!(application: app, resource_owner_id: user.id, scopes: "read write follow")
		puts "token_#{name}=#{token.plaintext_token}"
	end
	puts "uri_eve=#{ActivityPub::TagManager.instance.uri_for(Account.find_local!("eve"))}"
	admin = Doorkeeper::Application.create!(name: "federation-live-admin", redirect_uri: "urn:ietf:wg:oauth:2.0:oob", scopes: "admin:read")
	token = Doorkeeper::AccessToken.create!(application: admin, resource_owner_id: Account.find_local!("mod").user.id, scopes: "admin:read")
	puts "token_mod=#{token.plaintext_token}"
	# Mastodon joins the relay (TestRelay); Sidekiq sends the Follow, and the relay accepts it at once.
	Relay.create!(inbox_url: "https://relay.test/inbox").enable!
')"
mastodon_token() { sed -n "s/^token_$1=//p" <<<"$mastodon_tokens"; }
mastodon_token_bob="$(mastodon_token bob)"
mastodon_token_dave="$(mastodon_token dave)"
mastodon_token_eve="$(mastodon_token eve)"
mastodon_token_mod="$(mastodon_token mod)"
mastodon_token_judy="$(mastodon_token judy)"
[[ -n "$mastodon_token_bob" && -n "$mastodon_token_dave" && -n "$mastodon_token_eve" && -n "$mastodon_token_mod" && -n "$mastodon_token_judy" ]] ||
	fail "could not issue the access tokens of the Mastodon users"

# The proxy refuses eve's draft-cavage POSTs in both directions (Caddyfile, TestSignatureProfile); it
# learns her actor URI, which Mastodon numbers when it creates her, by being recreated with it.
FEDERATION_LIVE_SIGNER_ACTOR="$(sed -n 's/^uri_eve=//p' <<<"$mastodon_tokens")"
[[ "$FEDERATION_LIVE_SIGNER_ACTOR" == https://mastodon.test/* ]] || fail "could not read the actor URI of eve@mastodon.test"
FEDERATION_LIVE_SIGNER_INBOX="${FEDERATION_LIVE_SIGNER_ACTOR#https://mastodon.test}/inbox"
export FEDERATION_LIVE_SIGNER_ACTOR FEDERATION_LIVE_SIGNER_INBOX
"${compose[@]}" up -d --no-deps --quiet-pull proxy
wait_ready my.test /readyz
wait_ready mastodon.test /health

gts=(exec -T gotosocial /gotosocial/gotosocial admin account)
for user in carol erin arrive leave; do
	"${compose[@]}" "${gts[@]}" create --username "$user" --email "$user@gts.test" --password "$gts_password" >/dev/null
	"${compose[@]}" "${gts[@]}" confirm --username "$user" >/dev/null
done

# Misskey: the first account that admin/accounts/create makes is the administrator, who needs no token; it
# makes the others. Each response carries the user's own API token ("i"). A new Misskey federates with no
# one (federation "none"), so the administrator opens it to every server; a rate limit factor of 0 lifts the
# rate limits of the API (ap/show allows 30 calls an hour), which the scenarios call far more often.
misskey() {
	https misskey.test "/api/$1" -X POST -H 'Content-Type: application/json' -d "$2"
}
misskey_create() {
	misskey admin/accounts/create "$(jq -nc --arg u "$1" --arg p "$misskey_password" --arg i "${2:-}" \
		'{username: $u, password: $p} + (if $i == "" then {} else {i: $i} end)')" | jq -r '.token // empty'
}
misskey_token_admin="$(misskey_create mkadmin)"
[[ -n "$misskey_token_admin" ]] || fail "could not create the administrator of misskey.test"
misskey_token_grace="$(misskey_create grace "$misskey_token_admin")"
misskey_token_heidi="$(misskey_create heidi "$misskey_token_admin")"
misskey_token_arrive="$(misskey_create arrive "$misskey_token_admin")"
misskey_token_leave="$(misskey_create leave "$misskey_token_admin")"
[[ -n "$misskey_token_grace" && -n "$misskey_token_heidi" && -n "$misskey_token_arrive" && -n "$misskey_token_leave" ]] || fail "could not create the Misskey users"
misskey admin/update-meta "$(jq -nc --arg i "$misskey_token_admin" '{i: $i, federation: "all"}')" >/dev/null
misskey admin/roles/update-default-policies \
	"$(jq -nc --arg i "$misskey_token_admin" '{i: $i, policies: {rateLimitFactor: 0}}')" >/dev/null

# Akkoma: kate posts what its relay actor announces to the servers that follow it (TestRelayLitePub). The
# password grant of its OAuth API issues her token.
log "creating kate@akkoma.test"
"${compose[@]}" exec -T akkoma /opt/akkoma/bin/pleroma_ctl user new kate kate@akkoma.test \
	--password "$akkoma_password" -y >/dev/null
akkoma_app="$(https akkoma.test /api/v1/apps -X POST -H 'Content-Type: application/json' \
	-d '{"client_name":"federation-live","redirect_uris":"urn:ietf:wg:oauth:2.0:oob","scopes":"read write follow"}')"
akkoma_token="$(https akkoma.test /oauth/token -X POST -H 'Content-Type: application/json' -d "$(jq -nc \
	--argjson app "$akkoma_app" --arg p "$akkoma_password" \
	'{grant_type: "password", client_id: $app.client_id, client_secret: $app.client_secret,
	  username: "kate", password: $p, scope: "read write follow"}')" | jq -r '.access_token // empty')"
[[ -n "$akkoma_token" ]] || fail "could not issue the access token of kate@akkoma.test"

# --- Scenarios ----------------------------------------------------------------------------------
log "running the scenarios"
# Also written to $work/scenarios.env, to rerun the scenarios against a stack kept with FEDERATION_LIVE_KEEP=1
# (README.md).
export FEDERATION_LIVE_ADDR="127.0.0.1:$port"
export FEDERATION_LIVE_CA="$certs/ca.pem"
export FEDERATION_LIVE_MY_PASSWORD="$my_password"
export FEDERATION_LIVE_MASTODON_TOKEN="$mastodon_token_bob"
export FEDERATION_LIVE_MASTODON_TOKEN_DAVE="$mastodon_token_dave"
export FEDERATION_LIVE_MASTODON_TOKEN_EVE="$mastodon_token_eve"
export FEDERATION_LIVE_MASTODON_TOKEN_JUDY="$mastodon_token_judy"
export FEDERATION_LIVE_MASTODON_ADMIN_TOKEN="$mastodon_token_mod"
export FEDERATION_LIVE_GTS_PASSWORD="$gts_password"
export FEDERATION_LIVE_MISSKEY_TOKEN="$misskey_token_grace"
export FEDERATION_LIVE_MISSKEY_TOKEN_HEIDI="$misskey_token_heidi"
export FEDERATION_LIVE_MISSKEY_TOKEN_ARRIVE="$misskey_token_arrive"
export FEDERATION_LIVE_MISSKEY_TOKEN_LEAVE="$misskey_token_leave"
export FEDERATION_LIVE_MISSKEY_ADMIN_TOKEN="$misskey_token_admin"
export FEDERATION_LIVE_AKKOMA_TOKEN="$akkoma_token"
env | grep '^FEDERATION_LIVE_' >"$work/scenarios.env"
cd "$root"
# The scenarios take longer than go test's default 10 minutes: TestClosure alone waits up to 15 for
# delivery_completed.
go test -tags federationlive -count=1 -timeout 45m -v "$@" ./deploy/docker/federation-live 2>&1 | tee "$out/scenarios.log"
