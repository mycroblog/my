#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# The commands of the services of the root compose.yaml that run in the development image (the stage `dev` of the
# Dockerfile), from the checkout mounted at /src (CONTRIBUTING.md, "Local development"). They start as root and do their
# work as the owner of the checkout, so that what they write there and in the volumes belongs to the developer:
#   prepare  check that scripts/dev-cert.sh has made the HTTPS certificate of Caddy, make the mount points of the node_modules volumes in the checkout (Docker would create them owned by
#            root), and the certificates of Mailpit (scripts/mailpit-cert.sh, in .tmp/mailpit), which it copies to the
#            volume mounted at /certs for Mailpit
#   my       give the owner the volumes in HOME and the node_modules volumes, build `my` with GO_TAGS, apply the
#            schema, and serve
# Usage: scripts/dev-compose.sh prepare|my
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

# The node_modules directories of the Bun workspace (the root and each member), each a volume of compose.yaml.
node_modules=(. web/docs pkg/mykit/ts pkg/mybot/ts tools/i18n)

owner="$(stat -c %u:%g "$root")"

# as_owner runs a command as the owner of the checkout.
as_owner() {
	if [ "$(id -u)" = "${owner%:*}" ]; then
		"$@"
	else
		setpriv --reuid="${owner%:*}" --regid="${owner#*:}" --clear-groups "$@"
	fi
}

# exec_as_owner replaces this process with a command run as the owner of the checkout.
exec_as_owner() {
	if [ "$(id -u)" = "${owner%:*}" ]; then
		exec "$@"
	fi
	exec setpriv --reuid="${owner%:*}" --regid="${owner#*:}" --clear-groups "$@"
}

# own gives the owner of the checkout the volumes mounted at the paths, which Docker creates owned by root.
own() {
	if [ "$(id -u)" = 0 ]; then
		chown "$owner" "$@"
	fi
}

case "${1:-}" in
prepare)
	if [ ! -f .tmp/dev-certs/my.localhost.pem ] || [ ! -f .tmp/dev-certs/my.localhost-key.pem ]; then
		echo "dev-compose: no HTTPS certificate for my.localhost in .tmp/dev-certs; create it once with" \
			"scripts/task.sh docker:dev-cert (mkcert), then run docker compose up again" >&2
		exit 1
	fi
	for dir in "${node_modules[@]}"; do
		as_owner mkdir -p "$dir/node_modules"
	done
	as_owner env MAILPIT_CERTS="$root/.tmp/mailpit" scripts/mailpit-cert.sh
	cp .tmp/mailpit/ca.pem .tmp/mailpit/server.pem .tmp/mailpit/server.key /certs/
	;;
my)
	own "$HOME" "$HOME/.cache" "$HOME/bin" "$GOCACHE" "$GOMODCACHE" "$HOME/go" "$HOME/go/pkg" \
		"$HOME/.bun" "$HOME/.bun/install" "$BUN_INSTALL_CACHE_DIR"
	for dir in "${node_modules[@]}"; do
		own "$dir/node_modules"
	done
	exec_as_owner "$0" serve
	;;
serve)
	# The system's roots and the development CA of Mailpit: mail = smtp verifies Mailpit's certificate, and the server
	# still reaches servers outside.
	cat /etc/ssl/certs/ca-certificates.crt .tmp/mailpit/ca.pem >"$HOME/roots.pem"
	export SSL_CERT_FILE="$HOME/roots.pem"
	# What the server embeds, as `scripts/task.sh web:build` places it.
	find internal/interface/web/dist -mindepth 1 ! -name .gitkeep -delete
	cp web/brand/* internal/interface/web/dist/
	echo "dev-compose: building my${GO_TAGS:+ with the tags $GO_TAGS}"
	# A git worktree's .git points outside the mount, so the build stamps no VCS information.
	go build -trimpath -buildvcs=false -tags "${GO_TAGS:-}" -ldflags "-X main.version=dev" -o "$HOME/bin/my" ./cmd/my
	my schema apply
	my db upgrade
	exec my serve
	;;
*)
	echo "usage: $0 prepare|my" >&2
	exit 2
	;;
esac
