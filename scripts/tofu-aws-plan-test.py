#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
#
# A stub of AWS STS for scripts/tofu-aws-plan-test.sh: the plan of the AWS example reads aws_caller_identity, and this answers
# GetCallerIdentity with an account that does not exist, so that the plan calls nothing on AWS. Any other request is answered
# with an error and written to the calls file, which the test requires to be empty.
#
#   tofu-aws-plan-test.py PORT_FILE CALLS_FILE
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import parse_qs

IDENTITY = b"""<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>arn:aws:iam::111122223333:user/plan-test</Arn>
    <UserId>AIDAPLANTEST</UserId>
    <Account>111122223333</Account>
  </GetCallerIdentityResult>
  <ResponseMetadata>
    <RequestId>00000000-0000-0000-0000-000000000000</RequestId>
  </ResponseMetadata>
</GetCallerIdentityResponse>
"""


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def answer(self, code, body):
        self.send_response(code)
        self.send_header("Content-Type", "text/xml")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        form = parse_qs(self.rfile.read(int(self.headers.get("Content-Length", 0))).decode())
        action = form.get("Action", [""])[0]
        if action == "GetCallerIdentity":
            return self.answer(200, IDENTITY)
        with open(sys.argv[2], "a") as f:
            f.write(f"POST {self.path} {action}\n")
        self.answer(400, b"<ErrorResponse><Error><Code>Stub</Code><Message>stub</Message></Error></ErrorResponse>")

    def do_GET(self):
        with open(sys.argv[2], "a") as f:
            f.write(f"GET {self.path}\n")
        self.answer(400, b"")


server = HTTPServer(("127.0.0.1", 0), Handler)
with open(sys.argv[1], "w") as f:
    f.write(str(server.server_port))
server.serve_forever()
