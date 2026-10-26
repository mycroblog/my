// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/voice"
)

// JobCheckBlock applies a block between two participants of the same room (ADR-0100 §11): the
// one who is not the host or a co-host leaves (blocked). Its payload names the blocker and the
// blocked account.
const JobCheckBlock = "voice.check_block"

type blockPayload struct {
	Blocker string `json:"blocker"`
	Blocked string `json:"blocked"`
}

// registerBlockProjector subscribes the rooms to graph.blocked: a block between two
// participants of the same room enqueues JobCheckBlock in the transaction of the block.
func (s *Service) registerBlockProjector(r *uow.Projectors) {
	r.Register(graph.EventBlocked, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		b := e.(graph.Blocked).Block
		return s.enqueueCheckBlock(ctx, tx, b.ID, b.BlockerAccountID, b.BlockedAccountID)
	})
}

// enqueueCheckBlock enqueues JobCheckBlock in tx when blocker and blocked are in the same room.
func (s *Service) enqueueCheckBlock(ctx context.Context, tx uow.Tx, blockID, blocker, blocked string) error {
	a, err := s.d.Repository.FindParticipant(ctx, tx, blocker)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	b, err := s.d.Repository.FindParticipant(ctx, tx, blocked)
	if errors.Is(err, ErrNotFound) || (err == nil && b.RoomID != a.RoomID) {
		return nil
	}
	if err != nil {
		return err
	}
	payload, err := json.Marshal(blockPayload{Blocker: blocker, Blocked: blocked})
	if err != nil {
		return err
	}
	return s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{Type: JobCheckBlock, Payload: payload, IdempotencyKey: JobCheckBlock + ":" + blockID})
}

// blockLeaver returns which of the blocker and the blocked participant of a room leaves after a
// block between them (ADR-0100 §11): the other one of the host, or the one who is not a co-host,
// or the blocked one of two co-hosts. A block between two participants who are neither the host
// nor co-hosts makes nobody leave.
func blockLeaver(host string, blocker, blocked Participant) (Participant, bool) {
	switch {
	case blocker.AccountID == host, blocker.Cohost():
		return blocked, true
	case blocked.AccountID == host, blocked.Cohost():
		return blocker, true
	}
	return Participant{}, false
}

// CheckBlock applies a block of blocked by blocker to their room, as JobCheckBlock does.
func (s *Service) CheckBlock(ctx context.Context, blocker, blocked string) error {
	a, err := s.d.Repository.FindParticipant(ctx, nil, blocker)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	b, err := s.d.Repository.FindParticipant(ctx, nil, blocked)
	if errors.Is(err, ErrNotFound) || (err == nil && b.RoomID != a.RoomID) {
		return nil
	}
	if err != nil {
		return err
	}
	rec, err := s.d.Repository.FindRoom(ctx, nil, a.RoomID)
	if err != nil || rec.Head.State != voice.StateOpen {
		return err
	}
	// The streams of the room read their block marks again, and stop showing the reactions of
	// one of the two to the other (ADR-0100 §10).
	defer s.publishRoom(a.RoomID)
	leaver, ok := blockLeaver(rec.Head.HostAccountID, a, b)
	if !ok {
		return nil
	}
	session, err := s.d.Heartbeats.current(ctx, leaver.ParticipationID)
	if err != nil {
		return err
	}
	now := s.now()
	left := false
	err = s.roomTx(ctx, []string{leaver.RoomID}, func(tx uow.Tx, rec RoomRecord) error {
		left = false
		if rec.Head.State != voice.StateOpen || rec.Head.HostAccountID == leaver.AccountID {
			return nil
		}
		p, err := s.currentTx(ctx, tx, leaver, nil)
		if err != nil || p.ParticipationID == "" {
			return err
		}
		if left, err = s.endParticipationTx(ctx, tx, p, voice.BlockedInRoom, session, now); err != nil {
			return err
		}
		return s.endIfEmptyTx(ctx, tx, p.RoomID, now)
	})
	if err != nil {
		return err
	}
	if left {
		s.forget(ctx, &previous{Participant: leaver, session: session})
	}
	return nil
}
