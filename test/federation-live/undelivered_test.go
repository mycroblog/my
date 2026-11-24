// SPDX-License-Identifier: AGPL-3.0-only

//go:build federationlive

package federationlive

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestFetchesAnUndeliveredPost posts, as ivan, whom no other server follows, an unlisted post,
// which no relay takes either: its fan-out delivers to no one and stores no document of its
// Create (#3489). judy@mastodon.test then searches its URI, which makes Mastodon fetch the Note
// from this server with a signed GET, and finds it as posted: its text, its author, and its
// visibility.
func TestFetchesAnUndeliveredPost(t *testing.T) {
	e := stack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	ivan, err := signInLocalAs(ctx, e.http.Transport, relayUser, os.Getenv("FEDERATION_LIVE_MY_PASSWORD"))
	if err != nil {
		t.Fatalf("signing in as %s: %v", relayUser, err)
	}
	judy := &mastodonAPI{host: mastodonHost, token: os.Getenv("FEDERATION_LIVE_MASTODON_TOKEN_JUDY"), http: e.http}

	text := unique("fetched, never delivered")
	posted, err := ivan.post(ctx, text, "unlisted")
	if err != nil {
		t.Fatalf("posting as %s: %v", relayUser, err)
	}
	var found status
	eventually(t, deliveryTimeout, "Mastodon fetches the undelivered post", func() (bool, string) {
		s, err := judy.resolveStatus(ctx, posted.URI)
		if err != nil {
			return false, err.Error()
		}
		found = s
		return true, ""
	})
	if found.URI != posted.URI || !strings.Contains(found.Content, text) || found.Visibility != "unlisted" {
		t.Errorf("the post as Mastodon fetched it = %+v, want %s with %q, unlisted", found, posted.URI, text)
	}
}
