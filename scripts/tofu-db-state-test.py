#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
#
# A stub of the Cloud SQL Admin and Secret Manager APIs for scripts/tofu-db-state-test.sh. It keeps resources in memory, answers
# what the Google provider asks, and appends each password and secret value it receives to a file, which is how the test learns
# the values that must not appear anywhere else and that the user's password and the URL hold the same one.
#
#   tofu-db-state-test.py PORT_FILE VALUES_FILE
import base64
import json
import re
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

secrets = {}
versions = {}
payloads = {}
instances = {}
databases = {}
users = {}
operations = {}


def record(kind, name, value):
    with open(sys.argv[2], "a") as f:
        f.write(f"{kind} {name} {value}\n")


def operation(project, target):
    name = f"op-{len(operations) + 1}"
    operations[name] = {
        "kind": "sql#operation",
        "name": name,
        "status": "DONE",
        "targetProject": project,
        "targetId": target,
        "operationType": "CREATE",
        "selfLink": f"http://stub/projects/{project}/operations/{name}",
    }
    return operations[name]


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

    def not_found(self, method):
        self.answer(404, {"error": {"code": 404, "message": f"stub: {method} {self.path}"}})

    def do_POST(self):
        path, _, query = self.path.removeprefix("/sql/v1beta4").partition("?")
        created = re.fullmatch(r"/projects/([^/]+)/instances", path)
        if created:
            body = self.body()
            name = body["name"]
            body["settings"]["settingsVersion"] = "1"
            instances[name] = {
                **body,
                "kind": "sql#instance",
                "project": created.group(1),
                "connectionName": f"{created.group(1)}:{body['region']}:{name}",
                "state": "RUNNABLE",
                "backendType": "SECOND_GEN",
                "instanceType": "CLOUD_SQL_INSTANCE",
                "ipAddresses": [{"type": "PRIMARY", "ipAddress": "192.0.2.1"}],
                "serviceAccountEmailAddress": "stub@example.org",
                "selfLink": f"http://stub/projects/{created.group(1)}/instances/{name}",
            }
            return self.answer(200, operation(created.group(1), name))
        db = re.fullmatch(r"/projects/([^/]+)/instances/([^/]+)/databases", path)
        if db:
            body = self.body()
            databases[(db.group(2), body["name"])] = {
                **body,
                "kind": "sql#database",
                "charset": "UTF8",
                "collation": "en_US.UTF8",
                "selfLink": f"http://stub/projects/{db.group(1)}/instances/{db.group(2)}/databases/{body['name']}",
            }
            return self.answer(200, operation(db.group(1), db.group(2)))
        user = re.fullmatch(r"/projects/([^/]+)/instances/([^/]+)/users", path)
        if user:
            body = self.body()
            record("user", body["name"], body.get("password", ""))
            stored = {k: v for k, v in body.items() if k != "password"}
            users[(user.group(2), body["name"])] = {**stored, "kind": "sql#user", "host": ""}
            return self.answer(200, operation(user.group(1), user.group(2)))
        secret = re.fullmatch(r"/projects/([^/]+)/secrets", path)
        if secret:
            name = re.search(r"secretId=([^&]+)", query).group(1)
            secrets[name] = {
                **self.body(),
                "name": f"projects/{secret.group(1)}/secrets/{name}",
                "createTime": "2026-01-01T00:00:00Z",
            }
            return self.answer(200, secrets[name])
        added = re.fullmatch(r"/projects/([^/]+)/secrets/([^/:]+):addVersion", path)
        if added:
            value = base64.b64decode(self.body()["payload"]["data"]).decode()
            record("secret", added.group(2), value)
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
        self.not_found("POST")

    def do_PUT(self):
        path, _, query = self.path.removeprefix("/sql/v1beta4").partition("?")
        user = re.fullmatch(r"/projects/([^/]+)/instances/([^/]+)/users", path)
        if user:
            body = self.body()
            name = re.search(r"name=([^&]+)", query).group(1)
            record("user", name, body.get("password", ""))
            return self.answer(200, operation(user.group(1), user.group(2)))
        self.not_found("PUT")

    def do_GET(self):
        path = self.path.removeprefix("/sql/v1beta4").partition("?")[0]
        op = re.fullmatch(r"/projects/[^/]+/operations/([^/]+)", path)
        if op and op.group(1) in operations:
            return self.answer(200, operations[op.group(1)])
        inst = re.fullmatch(r"/projects/[^/]+/instances/([^/]+)", path)
        if inst and inst.group(1) in instances:
            return self.answer(200, instances[inst.group(1)])
        db = re.fullmatch(r"/projects/[^/]+/instances/([^/]+)/databases/([^/]+)", path)
        if db and db.group(1, 2) in databases:
            return self.answer(200, databases[db.group(1, 2)])
        listed = re.fullmatch(r"/projects/[^/]+/instances/([^/]+)/users", path)
        if listed:
            items = [u for key, u in users.items() if key[0] == listed.group(1)]
            return self.answer(200, {"kind": "sql#usersList", "items": items})
        one_user = re.fullmatch(r"/projects/[^/]+/instances/([^/]+)/users/([^/]+)", path)
        if one_user and one_user.group(1, 2) in users:
            return self.answer(200, users[one_user.group(1, 2)])
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
        self.not_found("GET")


server = HTTPServer(("127.0.0.1", 0), Handler)
with open(sys.argv[1], "w") as f:
    f.write(str(server.server_port))
server.serve_forever()
