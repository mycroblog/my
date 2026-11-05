#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
#
# A stub of the Secret Manager API for scripts/tofu-state-test.sh: it stores secrets in memory, answers what the
# Google provider asks, and appends each value added as a version to a file, which is how the test learns the values that
# must not appear anywhere else.
#
#   tofu-state-test.py PORT_FILE VALUES_FILE
import base64
import json
import re
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

secrets = {}
versions = {}
payloads = {}


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def answer(self, code, body):
        data = json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def body(self):
        return json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")

    def do_POST(self):
        path, _, query = self.path.partition("?")
        created = re.fullmatch(r"/projects/([^/]+)/secrets", path)
        if created:
            secret = re.search(r"secretId=([^&]+)", query).group(1)
            name = f"projects/{created.group(1)}/secrets/{secret}"
            secrets[secret] = {**self.body(), "name": name, "createTime": "2026-01-01T00:00:00Z"}
            return self.answer(200, secrets[secret])
        added = re.fullmatch(r"/projects/([^/]+)/secrets/([^/:]+):addVersion", path)
        if added:
            value = base64.b64decode(self.body()["payload"]["data"]).decode()
            with open(sys.argv[2], "a") as f:
                f.write(f"{added.group(2)} {value}\n")
            number = sum(1 for k in versions if k[0] == added.group(2)) + 1
            version = {
                "name": f"projects/{added.group(1)}/secrets/{added.group(2)}/versions/{number}",
                "state": "ENABLED",
                "createTime": "2026-01-01T00:00:00Z",
            }
            versions[(added.group(2), str(number))] = version
            payloads[(added.group(2), str(number))] = base64.b64encode(value.encode()).decode()
            return self.answer(200, version)
        toggled = re.fullmatch(r"/projects/[^/]+/secrets/([^/:]+)/versions/([^/:]+):(enable|disable)", path)
        if toggled and toggled.group(1, 2) in versions:
            return self.answer(200, versions[toggled.group(1, 2)])
        self.answer(404, {"error": {"code": 404, "message": "stub: POST " + self.path}})

    def do_GET(self):
        path = self.path.partition("?")[0]
        one = re.fullmatch(r"/projects/[^/]+/secrets/([^/:]+)", path)
        if one and one.group(1) in secrets:
            return self.answer(200, secrets[one.group(1)])
        # A provider that read the value back to store it would be caught: this answers with the real value.
        access = re.fullmatch(r"/projects/[^/]+/secrets/([^/:]+)/versions/([^/:]+):access", path)
        if access and access.group(1, 2) in payloads:
            name = versions[access.group(1, 2)]["name"]
            return self.answer(200, {"name": name, "payload": {"data": payloads[access.group(1, 2)]}})
        version = re.fullmatch(r"/projects/[^/]+/secrets/([^/:]+)/versions/([^/:]+)", path)
        if version and tuple(version.groups()) in versions:
            return self.answer(200, versions[tuple(version.groups())])
        self.answer(404, {"error": {"code": 404, "message": "stub: GET " + self.path}})


server = HTTPServer(("127.0.0.1", 0), Handler)
with open(sys.argv[1], "w") as f:
    f.write(str(server.server_port))
server.serve_forever()
