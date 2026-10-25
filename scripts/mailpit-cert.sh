#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Creates the private CA and the STARTTLS certificate of the development Mailpit (deploy/docker/mailpit), valid for
# the host names that `my serve` reaches it by: mailpit (in the compose stack), localhost, and 127.0.0.1. mail = smtp
# always verifies the server's certificate, so `my serve` trusts the CA through SSL_CERT_FILE (docs/operations/email.md).
# Keeps an existing certificate unless it expires within a day.
# Usage: scripts/mailpit-cert.sh   (needs openssl; writes to MAILPIT_CERTS, default .tmp/mailpit)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
certs="${MAILPIT_CERTS:-$root/.tmp/mailpit}"
mkdir -p "$certs"

if [ -f "$certs/server.pem" ] && openssl x509 -checkend 86400 -noout -in "$certs/server.pem" >/dev/null 2>&1; then
	echo "$certs/server.pem is still valid"
	exit 0
fi

openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 365 \
	-subj "/CN=my development Mailpit CA" -keyout "$certs/ca.key" -out "$certs/ca.pem" 2>/dev/null
openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes \
	-subj "/CN=mailpit" -keyout "$certs/server.key" -out "$certs/server.csr" 2>/dev/null
printf '%s\n' \
	'basicConstraints=critical,CA:FALSE' \
	'keyUsage=critical,digitalSignature' \
	'extendedKeyUsage=serverAuth' \
	'subjectAltName=DNS:mailpit,DNS:localhost,IP:127.0.0.1' >"$certs/server.ext"
openssl x509 -req -in "$certs/server.csr" -CA "$certs/ca.pem" -CAkey "$certs/ca.key" -CAcreateserial \
	-days 365 -extfile "$certs/server.ext" -out "$certs/server.pem" 2>/dev/null
rm -f "$certs/server.csr" "$certs/server.ext"
# Read by the containers, which run as other users: the keys of a development server only.
chmod 0644 "$certs"/*
echo "wrote $certs/ca.pem, $certs/server.pem, and $certs/server.key"
