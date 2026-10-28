#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Creates the private development CA and two certificates it signs:
#   server.pem / server.key              the STARTTLS certificate of the development Mailpit (deploy/docker/mailpit),
#                                        valid for the host names that `my serve` reaches it by: mailpit (in the compose
#                                        stack), localhost, and 127.0.0.1;
#   push-service.pem / push-service.key  the HTTPS certificate of the fake Web Push service of the test builds
#                                        (`my dev push-service`), valid for localhost and 127.0.0.1.
# mail = smtp and the Web Push sender always verify the server's certificate, so `my serve` trusts the CA through
# SSL_CERT_FILE (docs/operations/email.md). Keeps the CA and each certificate unless it expires within a day; a
# certificate that the current CA did not sign is created again.
# Usage: scripts/mailpit-cert.sh   (needs openssl; writes to MAILPIT_CERTS, default .tmp/mailpit)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
certs="${MAILPIT_CERTS:-$root/.tmp/mailpit}"
mkdir -p "$certs"

# valid FILE succeeds when the certificate FILE exists and does not expire within a day.
valid() {
	[ -f "$1" ] && openssl x509 -checkend 86400 -noout -in "$1" >/dev/null 2>&1
}

if ! valid "$certs/ca.pem"; then
	openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 365 \
		-subj "/CN=my development Mailpit CA" -keyout "$certs/ca.key" -out "$certs/ca.pem" 2>/dev/null
	echo "wrote $certs/ca.pem"
fi

# issue NAME CN SAN writes NAME.pem and NAME.key, signed by the CA, unless a valid one signed by it exists.
issue() {
	local name="$1" cn="$2" san="$3"
	if valid "$certs/$name.pem" && openssl verify -CAfile "$certs/ca.pem" "$certs/$name.pem" >/dev/null 2>&1; then
		echo "$certs/$name.pem is still valid"
		return
	fi
	openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes \
		-subj "/CN=$cn" -keyout "$certs/$name.key" -out "$certs/$name.csr" 2>/dev/null
	printf '%s\n' \
		'basicConstraints=critical,CA:FALSE' \
		'keyUsage=critical,digitalSignature' \
		'extendedKeyUsage=serverAuth' \
		"subjectAltName=$san" >"$certs/$name.ext"
	openssl x509 -req -in "$certs/$name.csr" -CA "$certs/ca.pem" -CAkey "$certs/ca.key" -CAcreateserial \
		-days 365 -extfile "$certs/$name.ext" -out "$certs/$name.pem" 2>/dev/null
	rm -f "$certs/$name.csr" "$certs/$name.ext"
	echo "wrote $certs/$name.pem and $certs/$name.key"
}

issue server mailpit "DNS:mailpit,DNS:localhost,IP:127.0.0.1"
issue push-service localhost "DNS:localhost,IP:127.0.0.1"
# Read by the containers, which run as other users: the keys of a development server only.
chmod 0644 "$certs"/*
