// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// postTagsQueries counts the round trips of a public post with n hashtags, and checks that it
// carries them.
func postTagsQueries(t *testing.T, n int) int {
	t.Helper()
	tracer := testkit.NewTracer()
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, "alice")
	var tags, want []string
	for i := range n {
		tags = append(tags, fmt.Sprintf("#Tag%d", i))
		want = append(want, fmt.Sprintf("tag%d", i))
	}
	// One of the tags exists before the post.
	c.post("alice", map[string]any{"text": "first #tag0", "visibility": "public"})
	was := ops(tracer)
	st := c.post("alice", map[string]any{"text": "tags " + strings.Join(tags, " "), "visibility": "public"})
	count, names := opsSince(tracer, was)
	t.Logf("posting with %d hashtags: %d queries: %v", n, count, names)
	var got []string
	for _, tag := range st.Tags {
		got = append(got, strings.ToLower(tag.Name))
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("the post's tags = %v, want %v", got, want)
	}
	return count
}

// TestPostTagsQueries checks that the round trips of a post do not grow with its hashtags: they
// are ensured, stored with the revision, and presented together.
func TestPostTagsQueries(t *testing.T) {
	t.Parallel()
	small, large := postTagsQueries(t, 2), postTagsQueries(t, 8)
	if large > small {
		t.Errorf("a post makes %d queries with 8 hashtags, more than the %d with 2", large, small)
	}
}
