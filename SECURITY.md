# Security policy

This policy covers vulnerabilities in the software of this repository. Please report them privately, not in a public
issue, pull request, or discussion.

## Reporting a vulnerability

Report through GitHub's private vulnerability reporting: open the **Security** tab of
[mycroblog/my](https://github.com/mycroblog/my/security) and choose **Report a vulnerability**. Only you and the
maintainers see the report and the conversation about it.

Include as much of the following as you can:

- The affected component (server, web client, mobile app) and version, or the commit of `main`.
- What an attacker can do, and what they need first (an account, a role, a federated server they control, ...).
- Steps to reproduce, a proof of concept, or the requests and responses involved.
- The configuration that matters: database, queue and storage adapters, build tags, instance settings.
- Whether you plan to publish your findings, and when.

## In scope

- The server (`cmd/my` and everything it is built from), including federation with other ActivityPub servers.
- The web client and the mobile app.
- The client libraries `pkg/mykit` and `pkg/mybot`, the client API specification, and the deployment files in `deploy/`.

## Out of scope

- **A particular server running this software.** A problem with how a server is configured or run, or with its
  moderation, goes to that server's operator: see its `/.well-known/security.txt` or its contact details on `/about`.
  If you find a vulnerability in the software through one server, report it here as well.
- **Vulnerabilities in dependencies.** Report them to the upstream project. If one affects this software in a way that
  needs a change here (an upgrade, a workaround), tell us too.

## Supported versions

Until version 1.0, fixes go only into `main` and the next release. Run the latest release; older releases do not get
security fixes.

| Version | Supported |
| --- | --- |
| The latest release | Yes |
| `main` | Yes |
| Older releases | No |

## What happens after you report

- We acknowledge the report within 3 business days.
- We confirm or reject it, with our assessment of its severity, within 10 business days.
- We aim to release a fix within 90 days of the report, sooner for severe issues. We keep you informed of progress
  and agree a disclosure date with you.
- When the fix is released, we publish a GitHub Security Advisory, request a CVE for it, and credit you by the name
  you choose (or not at all, if you prefer).

Please keep the details private until the advisory is published.

## Safe research

We will not pursue action against research done in good faith within these limits:

- Test only against a server you run yourself, with accounts you own. Do not attack other people's servers or
  access, change, or delete other people's data.
- Do not run denial-of-service attacks, mass scans, or spam against any server, including federated delivery to
  servers you do not control.
- Stop and report as soon as you have shown that the problem exists; do not use it further.
