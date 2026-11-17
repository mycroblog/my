#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# The commands of the services of the root compose.yaml that run in the development image (the stage `dev` of the
# Dockerfile), from the checkout mounted at /src (CONTRIBUTING.md, "Local development"). They start as root and do their
# work as the owner of the checkout, so that what they write there and in the volumes belongs to the developer:
#   prepare  check that scripts/dev-cert.sh has made the HTTPS certificate of Caddy, make the mount points of the
#            volumes in the checkout, bin and the node_modules (Docker would create them owned by root), and the
#            certificates of Mailpit (scripts/mailpit-cert.sh, in .tmp/mailpit), which it copies to the volume mounted
#            at /certs for Mailpit
#   my       give the owner the volumes in HOME and in the checkout, build the web client and `my` with GO_TAGS, apply
#            the schema, and serve
# Usage: scripts/dev-compose.sh prepare|my
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

# The node_modules directories of the Bun workspace (the root and each member), each a volume of compose.yaml, as is
# bin, where scripts/task.sh builds go-task for this image rather than for the developer's machine.
node_modules=(. web/app web/app-components web/app-data web/app-features web/design-system
	web/design-tokens web/devtools web/docs web/i18n web/platform web/storybook pkg/mykit/ts pkg/mybot/ts tools/i18n)

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
	as_owner mkdir -p bin
	for dir in "${node_modules[@]}"; do
		as_owner mkdir -p "$dir/node_modules"
	done
	as_owner env MAILPIT_CERTS="$root/.tmp/mailpit" scripts/mailpit-cert.sh
	cp .tmp/mailpit/ca.pem .tmp/mailpit/server.pem .tmp/mailpit/server.key /certs/
	;;
my)
	own "$HOME" "$HOME/.cache" "$HOME/bin" "$GOCACHE" "$GOMODCACHE" "$HOME/go" "$HOME/go/pkg" \
		"$HOME/.bun" "$HOME/.bun/install" "$BUN_INSTALL_CACHE_DIR" bin
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
	# What the server embeds: the build of `scripts/task.sh web:build`, as the image has it. The install comes first,
	# since web:install skips while node_modules exists, and the volume mounted there may still be empty.
	echo "dev-compose: building the web client"
	bun install --frozen-lockfile
	scripts/task.sh web:build
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
