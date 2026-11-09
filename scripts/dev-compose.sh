#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# The commands of the services of the root compose.yaml that run in the development image (the stage `dev` of the
# Dockerfile), from the checkout mounted at /src (CONTRIBUTING.md, "Local development"). They start as root and do their
# work as the owner of the checkout, so that what they write there and in the volumes belongs to the developer:
#   prepare  check that scripts/dev-cert.sh has made the HTTPS certificate of Caddy, make the mount points of the node_modules volumes in the checkout (Docker would create them owned by
#            root), and the certificates of Mailpit (scripts/mailpit-cert.sh, in .tmp/mailpit), which it copies to the
#            volume mounted at /certs for Mailpit
#   my       build `my` with GO_TAGS, apply the schema, and serve with the scripts from the Vite dev server
#   vite     install the npm packages, generate what the web client reads and is not committed, and run Vite
# Usage: scripts/dev-compose.sh prepare|my|vite
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

# The node_modules directories of the workspaces, each a volume of compose.yaml.
node_modules=(app app/ui app/web app/mail app/docs pkg/mykit/ts pkg/mybot/ts tools/i18n)

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
	own "$HOME" "$HOME/.cache" "$HOME/bin" "$GOCACHE" "$GOMODCACHE" "$HOME/go" "$HOME/go/pkg"
	exec_as_owner "$0" serve
	;;
serve)
	# The system's roots and the development CA of Mailpit: mail = smtp verifies Mailpit's certificate, and the server
	# still reaches servers outside.
	cat /etc/ssl/certs/ca-certificates.crt .tmp/mailpit/ca.pem >"$HOME/roots.pem"
	export SSL_CERT_FILE="$HOME/roots.pem"
	echo "dev-compose: building my${GO_TAGS:+ with the tags $GO_TAGS}"
	# A git worktree's .git points outside the mount, so the build stamps no VCS information.
	go build -trimpath -buildvcs=false -tags "${GO_TAGS:-}" -ldflags "-X main.version=dev" -o "$HOME/bin/my" ./cmd/my
	my schema apply
	exec my serve -dev-vite-origin "${VITE_ORIGIN:?}"
	;;
vite)
	own "$HOME" "$HOME/.cache" "$HOME/.bun" "$HOME/.bun/install" "$BUN_INSTALL_CACHE_DIR"
	for dir in "${node_modules[@]}"; do
		own "$dir/node_modules"
	done
	exec_as_owner "$0" vite-serve
	;;
vite-serve)
	(cd app && bun install --frozen-lockfile)
	(cd tools/i18n && bun install --frozen-lockfile && bun run src/gen.ts --ts)
	(cd app/ui && bun run scripts/theme-tokens.ts --ts)
	cd app/web
	exec bun run dev --host 0.0.0.0 --port 5173 --strictPort
	;;
*)
	echo "usage: $0 prepare|my|vite" >&2
	exit 2
	;;
esac
