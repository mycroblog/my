// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// inboundMentionQueries counts, by operation, the database round trips of taking in a public
// post by a remote account that mentions n local accounts, with the inline queue, after a first
// post that brought the remote account in. Every query is one round trip (one call in
// pg_stat_statements).
func inboundMentionQueries(t *testing.T, n int) map[string]int {
	t.Helper()
	tracer := testkit.NewTracer()
	network := &relayNet{docs: map[string][]byte{}}
	users := []string{"alice"}
	for i := range n {
		users = append(users, fmt.Sprintf("u%d", i))
	}
	c := newStatusesClientWith(t, map[string]string{"MY_QUEUE": "inline"}, platform.AppOptions{HTTPClient: network, Tracer: tracer}, users...)
	carol := newRemoteActor(t, network, "https://remote.example/users/carol", "Person", "https://remote.example/users/carol/inbox")
	at := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	note := func(i int, mentioned []string) map[string]any {
		var tags []map[string]any
		var cc []string
		for _, u := range mentioned {
			iri := "https://example.test/ap/actors/" + c.ids[u]
			tags, cc = append(tags, map[string]any{"type": "Mention", "href": iri}), append(cc, iri)
		}
		id := fmt.Sprintf("%s/statuses/%d", carol.id, i)
		return map[string]any{
			"id": id, "type": "Note", "attributedTo": carol.id, "content": "<p>hello</p>",
			"published": at.Format(time.RFC3339), "to": []string{asPublic}, "cc": cc, "tag": tags,
		}
	}
	send := func(i int, obj map[string]any) {
		t.Helper()
		id := fmt.Sprintf("%s/activities/%d", carol.id, i)
		if code := inboxCode(t, c, carol, map[string]any{"id": id, "type": "Create", "actor": carol.id, "object": obj}); code != http.StatusAccepted {
			t.Fatalf("Create = %d", code)
		}
	}
	send(1, note(1, users[:1]))
	was := ops(tracer)
	send(2, note(2, users[1:]))
	out := map[string]int{}
	for k, v := range ops(tracer) {
		if d := v - was[k]; d > 0 {
			out[k] = d
		}
	}
	return out
}

// mentionWrites are the writes of the mentions of a status, one per mention, which #2695 batches.
var mentionWrites = map[string]bool{"InsertStatusMention": true, "InsertStatusRevisionMention": true}

// TestInboundMentionQueries checks that taking in a post that mentions local accounts makes a
// number of round trips that does not grow with its mentions, but for their writes.
func TestInboundMentionQueries(t *testing.T) {
	t.Parallel()
	count := func(byOp map[string]int) int {
		n := 0
		for op, c := range byOp {
			if !mentionWrites[op] {
				n += c
			}
		}
		return n
	}
	small, large := count(inboundMentionQueries(t, 5)), count(inboundMentionQueries(t, 15))
	t.Logf("%d queries to take in a post with 5 mentions, %d with 15", small, large)
	if large > small {
		t.Errorf("%d queries to take in a post with 15 mentions, more than the %d with 5", large, small)
	}
}
