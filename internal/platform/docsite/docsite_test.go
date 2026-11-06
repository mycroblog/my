// SPDX-License-Identifier: AGPL-3.0-only

package docsite

import "testing"

func TestSiteVersion(t *testing.T) {
	t.Parallel()
	for build, want := range map[string]string{
		"v0.1.0":             "v0.1",
		"v0.1.3":             "v0.1",
		"v1.12.0":            "v1.12",
		"v0.2.0-rc.1":        Next,
		"v0.1.0-3-gabcdef0":  Next,
		"v0.1.0-dirty":       Next,
		"dev":                Next,
		"":                   Next,
		"v0.0.0-2026-abcdef": Next,
		"0.1.0":              Next,
	} {
		if got := SiteVersion(build); got != want {
			t.Errorf("SiteVersion(%q) = %q, want %q", build, got, want)
		}
	}
}

func TestPageURL(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ version, document, want string }{
		{Next, "docs/reference/build-tags.md", "https://mycroblog.dev/next/reference/build-tags/"},
		{"v0.1", "docs/operations/alerts.md#myqueuebacklog", "https://mycroblog.dev/v0.1/operations/alerts/#myqueuebacklog"},
		{"latest", "docs/developers/README.md", "https://mycroblog.dev/latest/developers/"},
		{Next, "docs/operations/Running.md", "https://mycroblog.dev/next/operations/running/"},
		{"v0.1", "api/openapi.yaml", "https://mycroblog.dev/v0.1/api/"},
	} {
		if got := PageURL(tc.version, tc.document); got != tc.want {
			t.Errorf("PageURL(%q, %q) = %q, want %q", tc.version, tc.document, got, tc.want)
		}
	}
}

// TestURL is not parallel: it sets the version of the process.
func TestURL(t *testing.T) {
	t.Cleanup(func() { running.Store(nil) })
	running.Store(nil)
	if got, want := URL("docs/operations/running.md"), "https://mycroblog.dev/next/operations/running/"; got != want {
		t.Errorf("URL before SetBuildVersion = %q, want %q", got, want)
	}
	SetBuildVersion("v0.3.1")
	if got, want := URL("docs/operations/running.md"), "https://mycroblog.dev/v0.3/operations/running/"; got != want {
		t.Errorf("URL of v0.3.1 = %q, want %q", got, want)
	}
	SetBuildVersion("dev")
	if got, want := URL("docs/operations/running.md"), "https://mycroblog.dev/next/operations/running/"; got != want {
		t.Errorf("URL of dev = %q, want %q", got, want)
	}
}
