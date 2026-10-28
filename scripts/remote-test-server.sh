# shellcheck shell=bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Sourced by scripts/upgrade-test.sh and scripts/restore-drill.sh. Needs jq and openssl.
#
# remote_test_files DIR writes the files of remote.test, the static ActivityPub server of
# deploy/docker/upgrade-test/compose.yaml: a private CA and the server's certificate in DIR/certs, and the Person carol
# with her WebFinger answer in DIR/remote. Every file is readable by the containers, which run as other users.

remote_test_files() {
	local dir="$1"
	local certs="$dir/certs" remote="$dir/remote"
	mkdir -p "$certs" "$remote"
	openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 2 \
		-subj "/CN=my test CA" -keyout "$certs/ca.key" -out "$certs/ca.pem" 2>/dev/null
	openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes \
		-subj "/CN=remote.test" -keyout "$certs/server.key" -out "$certs/server.csr" 2>/dev/null
	printf '%s\n' 'basicConstraints=critical,CA:FALSE' 'keyUsage=critical,digitalSignature' 'extendedKeyUsage=serverAuth' \
		'subjectAltName=DNS:remote.test' >"$certs/server.ext"
	openssl x509 -req -in "$certs/server.csr" -CA "$certs/ca.pem" -CAkey "$certs/ca.key" -CAcreateserial \
		-days 2 -extfile "$certs/server.ext" -out "$certs/server.pem" 2>/dev/null

	local carol_id="https://remote.test/users/carol"
	openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$dir/carol.key" 2>/dev/null
	jq -n --arg id "$carol_id" --arg pem "$(openssl pkey -in "$dir/carol.key" -pubout)" '{
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
	chmod 0755 "$dir" "$certs" "$remote"
	chmod 0644 "$certs"/* "$remote"/*
}
