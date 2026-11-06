// SPDX-License-Identifier: AGPL-3.0-only

// Package docsite links the pages of the documentation site (ADR-0150) from what the server prints for operators,
// such as command help and configuration errors, at the version of the running binary.
//
// The site serves each version under a path of its own: /vX.Y/ for a minor version, from its newest stable release,
// and /next/ for main. A page's URL mirrors the path of its document in the repository, as app/docs/src/published.ts
// maps it: docs/operations/running.md is /<version>/operations/running/, with the document's #anchors.
package docsite

import (
	"regexp"
	"strings"
	"sync/atomic"
)

// Origin is the documentation site.
const Origin = "https://mycroblog.dev"

// Next is the site version of main, which every build that is not a stable release links.
const Next = "next"

// stableRelease matches the version of a stable release's tag, vX.Y.Z. A pre-release (v0.2.0-rc.1) has no version of
// its own on the site, which publishes stable releases only, nor has a commit after a tag or a dirty tree.
var stableRelease = regexp.MustCompile(`^(v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))\.(?:0|[1-9][0-9]*)$`)

// running is the site version of the running binary, set once at start by SetBuildVersion.
var running atomic.Pointer[string]

// SiteVersion returns the site version of a build version: vX.Y for the stable release vX.Y.Z, Next otherwise (dev,
// a pre-release, a commit after a tag).
func SiteVersion(build string) string {
	if m := stableRelease.FindStringSubmatch(build); m != nil {
		return m[1]
	}
	return Next
}

// SetBuildVersion records the version of the running binary (`my version`), which URL links. cmd/my calls it once at
// start; until then URL links Next.
func SetBuildVersion(build string) {
	v := SiteVersion(build)
	running.Store(&v)
}

// URL returns the page of a published document of the repository, such as docs/reference/build-tags.md or
// docs/operations/alerts.md#myqueuebacklog, at the version of the running binary.
func URL(document string) string {
	v := Next
	if p := running.Load(); p != nil {
		v = *p
	}
	return PageURL(v, document)
}

// PageURL returns the page of a published document at a site version (Next, vX.Y, or latest). The document is a
// repository path under docs/, optionally with an #anchor; the page path is lowercase, and a README is its
// directory's page.
func PageURL(version, document string) string {
	path, anchor, _ := strings.Cut(document, "#")
	slug := strings.TrimSuffix(strings.TrimPrefix(path, "docs/"), ".md")
	slug = strings.TrimSuffix(strings.ToLower(slug), "/readme")
	u := Origin + "/" + version + "/" + slug + "/"
	if anchor != "" {
		u += "#" + anchor
	}
	return u
}
