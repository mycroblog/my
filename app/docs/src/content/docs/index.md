---
title: my
description: An ActivityPub-compatible microblog server written in Go, which runs cheaply with care on Google Cloud, AWS, or your own machine.
template: splash
hero:
  tagline: An ActivityPub-compatible microblog server written in Go. It runs cheaply with care on Google Cloud, AWS, or your own machine (Docker, microk8s), and is open source.
  image:
    light: ../../assets/wordmark-light.svg
    dark: ../../assets/wordmark-dark.svg
    alt: my
  actions:
    - text: Source on GitHub
      link: https://github.com/mycroblog/my
      icon: github
      variant: primary
---

<!-- SPDX-License-Identifier: AGPL-3.0-only -->

**Status: in development, before the first release (v0).** Data may not survive an upgrade before `v0.1.0`: the
database may have to be recreated. From `v0.1.0` on, every upgrade between releases keeps your data.

## What it does

- **Runs cheaply with care.** The server can scale to zero and needs no always-on middleware, though a server still costs money.
- **Fits your infrastructure.** The database, job queue, storage, and runtime are switched by configuration.
- **Federates.** It talks with Mastodon, Misskey, GoToSocial, and other ActivityPub servers.
- **Speaks many languages.** The server and its web client are translated from day one.
- **Comes with a web client.** An installable web app (PWA) for posting, reading, and running the server.

## License

`my` is free software under the GNU Affero General Public License, version 3 only (AGPL-3.0-only). The client
libraries for its API are under the MIT License.
