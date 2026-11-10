// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"testing"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestValidBearersAsAuthenticateBearer checks that the batch check of the stream heartbeats
// accepts exactly the tokens that AuthenticateBearer accepts: valid ones, and neither an unknown
// token nor one of a user whose login moderation disabled.
func TestValidBearersAsAuthenticateBearer(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob", "carol")
	ctx := t.Context()
	// Moderation disables carol's login.
	if _, err := c.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{TargetAccountID: c.ids["carol"], Kind: moderation.ActionDisableLogin, ReasonCategory: moderation.CategorySpam}); err != nil {
		t.Fatalf("disable carol's login: %v", err)
	}
	tokens := []string{c.tokens["alice"], c.tokens["bob"], c.tokens["carol"], "not-a-token", ""}
	valid, err := c.app.Identity.ValidBearers(ctx, tokens)
	if err != nil {
		t.Fatal(err)
	}
	for _, tok := range tokens {
		_, err := c.app.Identity.AuthenticateBearer(ctx, tok)
		if valid[tok] != (err == nil) {
			t.Errorf("ValidBearers[%q] = %v, but AuthenticateBearer = %v", tok, valid[tok], err)
		}
	}
	if !valid[c.tokens["alice"]] || valid[c.tokens["carol"]] {
		t.Errorf("ValidBearers = %v, want alice's token and not carol's", valid)
	}
}

// TestValidBearersQueries checks that the batch check of the tokens of the open streams, once per
// heartbeat, makes a number of round trips that does not grow with the streams.
func TestValidBearersQueries(t *testing.T) {
	t.Parallel()
	queries := func(n int) (batch, single int) {
		tracer := testkit.NewTracer()
		users := make([]string, n)
		for i := range users {
			users[i] = fmt.Sprintf("u%d", i)
		}
		c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, users...)
		tokens := make([]string, n)
		for i, u := range users {
			tokens[i] = c.tokens[u]
		}
		total := func() int {
			count := 0
			for _, v := range ops(tracer) {
				count += v
			}
			return count
		}
		before := total()
		valid, err := c.app.Identity.ValidBearers(t.Context(), tokens)
		if err != nil || len(valid) != n {
			t.Fatalf("ValidBearers = %v, %v", valid, err)
		}
		batch = total() - before
		before = total()
		for _, tok := range tokens {
			if _, err := c.app.Identity.AuthenticateBearer(t.Context(), tok); err != nil {
				t.Fatal(err)
			}
		}
		return batch, total() - before
	}
	smallBatch, smallSingle := queries(3)
	largeBatch, largeSingle := queries(10)
	t.Logf("one heartbeat: %d queries for 3 streams, %d for 10 (one by one: %d and %d)", smallBatch, largeBatch, smallSingle, largeSingle)
	if largeBatch > smallBatch {
		t.Errorf("%d queries to check 10 tokens, more than the %d for 3", largeBatch, smallBatch)
	}
}
