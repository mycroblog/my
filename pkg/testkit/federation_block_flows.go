// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/graph"
)

// fedBlocks stands in for the graph module's blocks, by ID.
type fedBlocks struct {
	mu     sync.Mutex
	blocks map[string]graph.Block
}

func (b *fedBlocks) FindBlock(_ context.Context, id string) (graph.Block, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	blk, ok := b.blocks[id]
	if !ok {
		return graph.Block{}, fmt.Errorf("block %s: %w", id, appgraph.ErrNotFound)
	}
	return blk, nil
}

// IsLocal implements federation.LocalityReader for the stand-in accounts.
func (a *fedAccounts) IsLocal(_ context.Context, _ uow.Tx, id string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	actor, ok := a.actors[id]
	return ok && actor.Account.IsLocal(), nil
}

// block records a block from blocker to blocked and emits it as the graph module does.
func (e *fedEnv) block(t *testing.T, blocker, blocked string) graph.Block {
	t.Helper()
	e.clock.Advance(time.Second)
	id := e.ids.NewID()
	b := graph.Block{ID: id, URI: graph.BlockURI(contractHost, id), BlockerAccountID: blocker, BlockedAccountID: blocked, CreatedAt: e.clock.Now()}
	e.blocks.mu.Lock()
	e.blocks.blocks[id] = b
	e.blocks.mu.Unlock()
	e.emit(t, appgraph.Blocked{Block: b})
	e.drain(t)
	return b
}

func fedDeliversBlocks(t *testing.T, e *fedEnv) {
	ctx := t.Context()
	alice, carol := e.local(), e.local()
	bob := e.remote("b.example", true)

	// A local block of a remote account is a Block delivered to the blocked actor's own inbox.
	b := e.block(t, alice, bob)
	act := e.activity(t, federation.OutboundBlock, b.ID, federation.ActivityBlock)
	if act.ActivityURI != federation.IRI(b.URI) || act.ActorAccountID != alice {
		t.Errorf("Block = %+v, want alice's with the block's URI %s", act, b.URI)
	}
	if got, want := e.inboxes(t, act.ID), []string{e.uri(bob) + "/inbox"}; !slices.Equal(got, want) {
		t.Errorf("Block inboxes = %v, want %v", got, want)
	}
	doc := e.document(t, act.ID)
	if doc["type"] != "Block" || doc["id"] != b.URI || doc["actor"] != e.uri(alice) || doc["object"] != e.uri(bob) {
		t.Errorf("Block document = %v, want alice's Block %s of bob", doc, b.URI)
	}
	if to, _ := doc["to"].([]any); len(to) != 1 || to[0] != e.uri(bob) {
		t.Errorf("Block to = %v, want only bob", doc["to"])
	}

	// Its lift is an Undo that embeds it.
	e.clock.Advance(time.Second)
	e.emit(t, appgraph.Unblocked{Block: b, Retraction: graph.BlockRetraction{BlockID: b.ID, RetractedAt: e.clock.Now()}})
	e.drain(t)
	undo := e.activity(t, federation.OutboundBlock, b.ID, federation.ActivityUndo)
	if got, want := e.inboxes(t, undo.ID), []string{e.uri(bob) + "/inbox"}; !slices.Equal(got, want) {
		t.Errorf("Undo inboxes = %v, want %v", got, want)
	}
	doc = e.document(t, undo.ID)
	inner, _ := doc["object"].(map[string]any)
	if doc["type"] != "Undo" || doc["id"] != string(undo.ActivityURI) || inner["type"] != "Block" || inner["id"] != b.URI || inner["object"] != e.uri(bob) {
		t.Errorf("Undo document = %v, want an Undo of Block %s", doc, b.URI)
	}

	// Blocks between local accounts, and blocks that remote accounts made, are not delivered.
	for _, pair := range [][2]string{{alice, carol}, {bob, carol}} {
		other := e.block(t, pair[0], pair[1])
		if got, err := e.h.Federation.ListOutboundActivitiesOf(ctx, federation.OutboundBlock, other.ID); err != nil || len(got) != 0 {
			t.Errorf("activities about the block %s -> %s = %+v, %v; want none", pair[0], pair[1], got, err)
		}
	}

	// A suspended server gets no Block.
	dave := e.remote("d.example", false)
	e.domains["d.example"] = true
	suspended := e.block(t, carol, dave)
	act = e.activity(t, federation.OutboundBlock, suspended.ID, federation.ActivityBlock)
	if got := e.inboxes(t, act.ID); len(got) != 0 {
		t.Errorf("Block inboxes on a suspended server = %v, want none", got)
	}
}
