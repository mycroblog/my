// SPDX-License-Identifier: AGPL-3.0-only

// Package web serves the web client (ADR-0028): the HTML shell that starts the SPA, the
// Vite build output, and the Web App Manifest generated from the instance settings.
//
// The Vite build of the web client is embedded from dist/, which `scripts/task.sh web:build` fills with the
// build of web/app and the brand assets of web/brand (the Dockerfile with the brand assets alone, without a
// manifest). Only dist/.gitkeep is
// committed, so the package compiles and its tests run without Bun; a binary built that way
// serves a shell without scripts and logs a warning at start.
//
// Requests for HTML are always treated as anonymous, so every shell is the same for every
// visitor and is cached by shared caches. The generic shell is the same for every SPA route.
// Profile pages (/@alice) and post pages (/@alice/status/{id}) get a shell with link preview meta
// tags (OGP, the X card, fediverse:creator) when an anonymous visitor may see the profile or
// the post (Options.Previews); otherwise they get the generic shell with 404 and noindex,
// identical whatever the reason (docs/operations/link-previews.md).
//
// The instance's runtime settings reach the client as a JSON data block in the shell
// (<script type="application/json" id="config">), not through the build, so the
// Content-Security-Policy needs no nonce and allows only scripts from the server itself.
//
// Routing between the shell and ActivityPub documents (content negotiation) is the caller's:
// this package serves only HTML.
package web
