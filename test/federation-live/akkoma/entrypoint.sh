#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Start Akkoma in the live interoperability stack (../compose.yaml): install the configuration file without world
# permissions (Akkoma refuses it otherwise), trust the private CA (Akkoma's HTTP client reads the system store), migrate
# the database, and run the server in the foreground.
set -eu

install -D -m 0600 /config/akkoma.exs /etc/akkoma/config.exs
cp /certs/ca.pem /usr/local/share/ca-certificates/federation-live.crt
update-ca-certificates >/dev/null
/opt/akkoma/bin/pleroma_ctl migrate
exec /opt/akkoma/bin/pleroma start
