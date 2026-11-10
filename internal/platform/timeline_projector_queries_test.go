// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// timelineProjectorQueries counts, by operation, the timeline writes of a public post with n
// hashtags and of a direct post to n accounts.
func timelineProjectorQueries(t *testing.T, n int) (int, []string) {
	t.Helper()
	tracer := testkit.NewTracer()
	names := []string{"alice"}
	for i := range n {
		names = append(names, fmt.Sprintf("friend%d", i))
	}
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, names...)
	var tags, mentions []string
	for i := range n {
		tags = append(tags, fmt.Sprintf("#tag%d", i))
		mentions = append(mentions, fmt.Sprintf("@friend%d", i))
	}
	was := ops(tracer)
	c.post("alice", map[string]any{"text": "tags " + strings.Join(tags, " "), "visibility": "public"})
	c.post("alice", map[string]any{"text": "hello " + strings.Join(mentions, " "), "visibility": "direct"})
	count := 0
	var names2 []string
	for op, k := range ops(tracer) {
		if d := k - was[op]; d > 0 && (strings.Contains(op, "FeedDirectRecipient") || strings.Contains(op, "FeedTagEntr") || strings.Contains(op, "TagUsage")) {
			count += d
			names2 = append(names2, fmt.Sprintf("%s×%d", op, d))
		}
	}
	return count, names2
}

// TestTimelineProjectorQueries checks that the timeline projector writes the direct recipients,
// the tag entries, and the tag usage of a post with a number of statements that does not grow
// with them.
func TestTimelineProjectorQueries(t *testing.T) {
	t.Parallel()
	small, smallOps := timelineProjectorQueries(t, 2)
	large, largeOps := timelineProjectorQueries(t, 8)
	t.Logf("timeline writes: %d for 2 (%v), %d for 8 (%v)", small, smallOps, large, largeOps)
	if large > small {
		t.Errorf("the timeline writes of a post make %d statements for 8 tags and recipients, more than the %d for 2", large, small)
	}
}
