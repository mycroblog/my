// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// BlockFinder reads the local blocks that outbound Block and Undo Block activities are about.
// *appgraph.Service implements it.
type BlockFinder interface {
	FindBlock(ctx context.Context, id string) (appgraph.Block, error)
}

// RegisterBlockProjectors subscribes the federation module to graph.blocked and graph.unblocked
// (federation.md §7.1): a local account's block of a remote account, and its lift, are recorded
// as a Block (whose URI is the block's) and an Undo Block, delivered to the blocked account.
// Blocks between local accounts, and blocks received from other servers, are not. It needs
// d.Accounts.
func RegisterBlockProjectors(r *uow.Projectors, d RecorderDeps) {
	if d.Accounts == nil {
		panic("federation: the block projectors need RecorderDeps.Accounts")
	}
	rec := recorder{d: d}
	federates := func(ctx context.Context, tx uow.Tx, b appgraph.Block) (bool, error) {
		local, err := d.Accounts.IsLocal(ctx, tx, b.BlockerAccountID)
		if err != nil || !local {
			return false, err
		}
		local, err = d.Accounts.IsLocal(ctx, tx, b.BlockedAccountID)
		return !local, err
	}
	r.Register(appgraph.EventBlocked, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appgraph.Blocked)
		if !ok {
			return nil
		}
		if ok, err := federates(ctx, tx, ev.Block); err != nil || !ok {
			return err
		}
		return rec.record(ctx, tx, federation.OutboundActivity{
			ActivityURI: federation.IRI(ev.Block.URI), Type: federation.ActivityBlock,
			ActorAccountID: ev.Block.BlockerAccountID, ObjectKind: federation.OutboundBlock, ObjectID: ev.Block.ID,
			CreatedAt: ev.Block.CreatedAt,
		}, "")
	})
	r.Register(appgraph.EventBlocksCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appgraph.BlocksCreated)
		if !ok {
			return nil
		}
		var ids []string
		for _, b := range ev.Blocks {
			ids = append(ids, b.Block.BlockerAccountID, b.Block.BlockedAccountID)
		}
		local, err := rec.localAmong(ctx, tx, ids)
		if err != nil {
			return err
		}
		// As federates decides of each: the Block of a local account's block of a remote one.
		var as []federation.OutboundActivity
		for _, b := range ev.Blocks {
			if local[b.Block.BlockerAccountID] && !local[b.Block.BlockedAccountID] {
				as = append(as, federation.OutboundActivity{
					ActivityURI: federation.IRI(b.Block.URI), Type: federation.ActivityBlock,
					ActorAccountID: b.Block.BlockerAccountID, ObjectKind: federation.OutboundBlock, ObjectID: b.Block.ID,
					CreatedAt: b.Block.CreatedAt,
				})
			}
		}
		return rec.recordAll(ctx, tx, as)
	})
	r.Register(appgraph.EventUnblocked, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appgraph.Unblocked)
		if !ok {
			return nil
		}
		if ok, err := federates(ctx, tx, ev.Block); err != nil || !ok {
			return err
		}
		return rec.record(ctx, tx, federation.OutboundActivity{
			Type: federation.ActivityUndo, ActorAccountID: ev.Block.BlockerAccountID,
			ObjectKind: federation.OutboundBlock, ObjectID: ev.Block.ID, CreatedAt: ev.Retraction.RetractedAt,
		}, "")
	})
}

// blockSubject loads the block of a Block or an Undo Block. Its only recipient is the blocked
// account.
func (f *FanOut) blockSubject(ctx context.Context, s subject) (subject, error) {
	act := s.act
	if f.d.Blocks == nil || (act.Type != federation.ActivityBlock && act.Type != federation.ActivityUndo) {
		return subject{}, fmt.Errorf("%w: %s of a block", federation.ErrInvalidOutbound, act.Type)
	}
	b, err := f.d.Blocks.FindBlock(ctx, act.ObjectID)
	switch {
	case errors.Is(err, appgraph.ErrNotFound):
		return subject{}, fmt.Errorf("%w: block %s", errGone, act.ObjectID)
	case err != nil:
		return subject{}, err
	case b.BlockerAccountID != act.ActorAccountID:
		return subject{}, fmt.Errorf("%w: block %s of another account than %s", federation.ErrInvalidOutbound, b.ID, act.ActorAccountID)
	}
	s.block = b
	s.mentioned = []string{b.BlockedAccountID}
	return s, nil
}

// blockedInbox returns the inbox of the blocked account: its own inbox, which a server reads the
// block from as Mastodon delivers it, not its shared inbox.
func (f *FanOut) blockedInbox(ctx context.Context, s subject) ([]target, error) {
	inbox, err := f.d.Accounts.RemoteInbox(ctx, s.block.BlockedAccountID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return nil, nil // a local account, or a remote one never fetched
	}
	if err != nil {
		return nil, err
	}
	t, ok := targetOf(federation.InboxFor(federation.IRI(inbox.InboxURL), ""))
	if !ok {
		return nil, nil
	}
	return []target{t}, nil
}

// blockDocument builds the Block of a local block, addressed to the blocked actor, or the Undo
// that embeds it.
func (f *FanOut) blockDocument(ctx context.Context, act federation.OutboundActivity, s subject) ([]byte, error) {
	actor, err := f.actorIRI(ctx, act.ActorAccountID)
	if err != nil {
		return nil, err
	}
	blocked, err := f.actorIRI(ctx, s.block.BlockedAccountID)
	if err != nil {
		return nil, err
	}
	aud := federation.Audience{To: []federation.IRI{blocked}}
	block := federation.NewActivityDocument(federation.IRI(s.block.URI), federation.ActivityBlock, actor, s.block.CreatedAt, aud, blocked)
	if act.Type == federation.ActivityUndo {
		return federation.NewActivityDocument(act.ActivityURI, act.Type, actor, act.CreatedAt, aud, block.Embedded()).Marshal()
	}
	return block.Marshal()
}
