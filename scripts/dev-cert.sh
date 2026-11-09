#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Creates the HTTPS certificate of the development environment of the root compose.yaml (CONTRIBUTING.md, "Local
# development"): one certificate for my.localhost and *.my.localhost, signed by the local CA of mkcert
# (https://github.com/FiloSottile/mkcert, BSD-3-Clause), which `mkcert -install` makes the browsers on this machine
# trust. Under WSL it runs mkcert.exe, the Windows build, so that the CA goes into the Windows store that the browsers
# on Windows read; elsewhere mkcert. Caddy (the service caddy) serves the certificate.
# Usage: scripts/dev-cert.sh   (writes to .tmp/dev-certs, where compose.yaml reads it; keeps one valid for 30 days)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
certs="$root/.tmp/dev-certs"
cert="$certs/my.localhost.pem"
key="$certs/my.localhost-key.pem"
mkdir -p "$certs"

if grep -qi microsoft /proc/version 2>/dev/null; then
	mkcert=mkcert.exe
	# mkcert.exe takes Windows paths; the files land in this directory through the WSL share.
	to_mkcert() { wslpath -w "$1"; }
else
	mkcert=mkcert
	to_mkcert() { printf '%s\n' "$1"; }
fi
if ! command -v "$mkcert" >/dev/null; then
	echo "dev-cert: $mkcert is not installed; install mkcert (https://github.com/FiloSottile/mkcert#installation)" >&2
	if [ "$mkcert" = mkcert.exe ]; then
		echo "dev-cert: on Windows, for example: winget install FiloSottile.mkcert" >&2
	fi
	exit 1
fi

if [ -f "$cert" ] && [ -f "$key" ] && openssl x509 -checkend $((30 * 86400)) -noout -in "$cert" >/dev/null 2>&1; then
	echo "dev-cert: $cert is still valid"
	exit 0
fi

# Adds the CA to the system and browser stores; it does nothing when it is there already.
"$mkcert" -install
"$mkcert" -cert-file "$(to_mkcert "$cert")" -key-file "$(to_mkcert "$key")" my.localhost "*.my.localhost"
# Caddy runs as another user in its container: the key of a development certificate only.
chmod 0644 "$cert" "$key"
echo "dev-cert: wrote $cert and $key"
