// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"errors"
	"slices"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/voice"
)

// Open opens a room that accountID hosts, with the settings in, and joins the host to it as a
// speaker (ADR-0100 §2, §5): the account must be eligible to host (ErrNotEligible), fewer rooms
// than the operator allows must be open (ErrTooManyRooms), and the account must not host
// another open room (ErrHostingElsewhere). A participation in another room ends (moved).
// Invalid settings fail with a *voice.SettingsError. With in.Post, the host's start post, which
// links the room, is written in the same transaction (ADR-0100 §9): local for a room of the local
// users and private for one of the followers; the other rooms fail with ErrNoStartPost, and a
// post that the content module refuses with its error. The local followers of the host who see
// the room are notified by JobNotifyStarted (§8).
func (s *Service) Open(ctx context.Context, accountID string, in SettingsInput) (Joined, error) {
	if err := s.enabled(); err != nil {
		return Joined{}, err
	}
	settings, err := s.settings(ctx, in)
	if err != nil {
		return Joined{}, err
	}
	ok, err := s.canHost(ctx, accountID)
	if err != nil {
		return Joined{}, err
	}
	if !ok {
		return Joined{}, ErrNotEligible
	}
	roomID := s.d.IDs.NewID()
	post, err := s.prepareStartPost(ctx, accountID, roomID, settings, in.Post)
	if err != nil {
		return Joined{}, err
	}
	prev, err := s.leaveForJoin(ctx, accountID, "")
	if err != nil {
		return Joined{}, err
	}
	now := s.now()
	room := Room{ID: roomID, OpenedByAccountID: accountID, OpenedAt: now, Transport: s.d.Config.Transport}
	rev := Revision{ID: s.d.IDs.NewID(), RoomID: room.ID, Settings: settings, RevisedByAccountID: accountID, RevisedAt: now}
	host := HostChange{ID: s.d.IDs.NewID(), RoomID: room.ID, HostAccountID: accountID, Reason: voice.HostOpened, ChangedAt: now}
	part := Participation{ID: s.d.IDs.NewID(), RoomID: room.ID, AccountID: accountID, JoinedAt: now}
	session := newSessionID()
	// The grant is issued before anything is written, so that a transport that gives no grant
	// (ErrTransportUnavailable) leaves no room behind and the host can try again.
	g, err := s.grant(ctx, room.ID, Session{ID: session, AccountID: accountID}, true)
	if err != nil {
		return Joined{}, err
	}
	if err := s.d.Heartbeats.start(ctx, part.ID, session); err != nil {
		return Joined{}, err
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		// The opening lock serializes the count of the open rooms; the room of the previous
		// participation is locked after it, as every command of a room locks it.
		if err := s.d.Repository.LockOpening(ctx, tx); err != nil {
			return err
		}
		if prev != nil {
			if err := s.d.Repository.LockRoom(ctx, tx, prev.RoomID); err != nil {
				return err
			}
		}
		if limit := s.d.Config.MaxOpenRooms; limit > 0 {
			open, err := s.d.Repository.OpenRooms(ctx, tx, "")
			if err != nil {
				return err
			}
			if len(open) >= limit {
				return ErrTooManyRooms
			}
		}
		if err := s.moveTx(ctx, tx, accountID, "", prev, now); err != nil {
			return err
		}
		if err := s.d.Repository.InsertRoom(ctx, tx, room); err != nil {
			return err
		}
		if err := s.d.Repository.InsertRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := s.d.Repository.InsertHostChange(ctx, tx, host); err != nil {
			return err
		}
		if err := tx.Emit(ctx, RoomOpened{Room: room, Revision: rev, Host: host}); err != nil {
			return err
		}
		if err := s.startParticipationTx(ctx, tx, part, voice.RoleSpeaker); err != nil {
			return err
		}
		if post != nil {
			// The start post is the host's own post, written with the room (ADR-0100 §9).
			if _, err := s.d.Posts.CreateTx(ctx, tx, *post); err != nil {
				return err
			}
		}
		if err := s.enqueueNotifyStarted(ctx, tx, room, rev); err != nil {
			return err
		}
		return s.enqueueApply(ctx, tx, applyPayload{Op: applyOpen, Room: room.ID}, "open")
	})
	if err != nil {
		return Joined{}, s.joinFailed(ctx, part.ID, session, err)
	}
	s.forget(ctx, prev)
	return s.joined(ctx, room.ID, session, g)
}

// Get returns a room that accountID sees (voice.CanSee), after evaluating its expiry;
// ErrNotFound for any other.
func (s *Service) Get(ctx context.Context, accountID, roomID string) (RoomView, error) {
	if err := s.enabled(); err != nil {
		return RoomView{}, err
	}
	if err := s.evaluate(ctx, roomID, false); err != nil {
		return RoomView{}, err
	}
	return s.visibleRoom(ctx, accountID, roomID)
}

// visibleRoom returns a room that accountID sees; ErrNotFound for any other.
func (s *Service) visibleRoom(ctx context.Context, accountID, roomID string) (RoomView, error) {
	v, err := s.view(ctx, roomID)
	if err != nil {
		return RoomView{}, err
	}
	ok, err := s.visible(ctx, v, accountID)
	if err != nil {
		return RoomView{}, err
	}
	if !ok {
		return RoomView{}, ErrNotFound
	}
	return v, nil
}

// ListKey is the position of a room in the list of a viewer: the rooms where the viewer
// follows the host or a speaker first, then by the most participants, then by ID.
type ListKey struct {
	Followed     bool
	Participants int
	RoomID       string
}

// before reports whether k comes before o in the list.
func (k ListKey) before(o ListKey) bool {
	switch {
	case k.Followed != o.Followed:
		return k.Followed
	case k.Participants != o.Participants:
		return k.Participants > o.Participants
	}
	return k.RoomID < o.RoomID
}

// ListQuery selects a page of the room list.
type ListQuery struct {
	// Tag keeps the rooms whose settings have the hashtag, "" every room.
	Tag string
	// After starts the page after the room at this position; nil starts at the top.
	After *ListKey
	// Limit is the length of the page.
	Limit int
}

// ListedRoom is a room of the list with its position.
type ListedRoom struct {
	RoomView
	Key ListKey
}

// List returns a page of the open rooms that accountID sees (ADR-0100 §8), after evaluating
// the expiry of each, and whether more follow.
func (s *Service) List(ctx context.Context, accountID string, q ListQuery) ([]ListedRoom, bool, error) {
	if err := s.enabled(); err != nil {
		return nil, false, err
	}
	tag := ""
	if q.Tag != "" {
		tag = NormalizeTag(q.Tag)
	}
	open, err := s.d.Repository.OpenRooms(ctx, nil, tag)
	if err != nil {
		return nil, false, err
	}
	var out []ListedRoom
	for _, r := range open {
		if err := s.evaluate(ctx, r.ID, false); err != nil {
			return nil, false, err
		}
		v, err := s.visibleRoom(ctx, accountID, r.ID)
		switch {
		case errors.Is(err, ErrNotFound):
			continue
		case err != nil:
			return nil, false, err
		}
		if v.Head.State != voice.StateOpen {
			continue
		}
		key, err := s.listKey(ctx, accountID, v)
		if err != nil {
			return nil, false, err
		}
		if q.After != nil && !q.After.before(key) {
			continue
		}
		out = append(out, ListedRoom{RoomView: v, Key: key})
	}
	slices.SortFunc(out, func(a, b ListedRoom) int {
		switch {
		case a.Key.before(b.Key):
			return -1
		case b.Key.before(a.Key):
			return 1
		}
		return 0
	})
	if q.Limit > 0 && len(out) > q.Limit {
		return out[:q.Limit], true, nil
	}
	return out, false, nil
}

// listKey returns the position of a room in the list of accountID.
func (s *Service) listKey(ctx context.Context, accountID string, v RoomView) (ListKey, error) {
	ids := []string{v.Head.HostAccountID}
	for _, p := range v.Participants {
		if p.Role == voice.RoleSpeaker && p.AccountID != v.Head.HostAccountID {
			ids = append(ids, p.AccountID)
		}
	}
	ids = slices.DeleteFunc(ids, func(id string) bool { return id == accountID })
	following, err := s.d.Relations.Following(ctx, accountID, ids)
	if err != nil {
		return ListKey{}, err
	}
	followed := slices.ContainsFunc(ids, func(id string) bool { return following[id] })
	return ListKey{Followed: followed, Participants: len(v.Participants), RoomID: v.ID}, nil
}

// hostRoom returns a room that accountID hosts, after evaluating its expiry: ErrNotFound when
// the account does not see it, ErrNotHost when it is not the host. A host who is no longer
// eligible hands the room over (ADR-0100 §2) and gets ErrNotHost.
func (s *Service) hostRoom(ctx context.Context, accountID, roomID string, checkEligible bool) (RoomView, error) {
	v, err := s.Get(ctx, accountID, roomID)
	if err != nil {
		return RoomView{}, err
	}
	if v.Head.HostAccountID != accountID {
		return RoomView{}, ErrNotHost
	}
	if !checkEligible {
		return v, nil
	}
	ok, err := s.canHost(ctx, accountID)
	if err != nil {
		return RoomView{}, err
	}
	if !ok {
		if err := s.hostIneligible(ctx, roomID, accountID, false); err != nil {
			return RoomView{}, err
		}
		return RoomView{}, ErrNotHost
	}
	return v, nil
}

// Update changes the settings of a room that accountID hosts, as a new revision (ADR-0100
// §3). Invalid settings fail with a *voice.SettingsError.
func (s *Service) Update(ctx context.Context, accountID, roomID string, p SettingsPatch) (RoomView, error) {
	v, err := s.hostRoom(ctx, accountID, roomID, true)
	if err != nil {
		return RoomView{}, err
	}
	settings, err := s.patched(ctx, v.Revision.Settings, p)
	if err != nil {
		return RoomView{}, err
	}
	rev := Revision{ID: s.d.IDs.NewID(), RoomID: roomID, Settings: settings, RevisedByAccountID: accountID, RevisedAt: s.now()}
	err = s.roomTx(ctx, []string{roomID}, func(tx uow.Tx, rec RoomRecord) error {
		switch {
		case rec.Head.State != voice.StateOpen:
			return ErrNotFound
		case rec.Head.HostAccountID != accountID:
			return ErrNotHost
		}
		if err := s.d.Repository.InsertRevision(ctx, tx, rev); err != nil {
			return err
		}
		return tx.Emit(ctx, RoomRevised{Revision: rev})
	})
	if err != nil {
		return RoomView{}, err
	}
	return s.view(ctx, roomID)
}

// End ends a room that accountID hosts (host, ADR-0100 §6).
func (s *Service) End(ctx context.Context, accountID, roomID string) error {
	if _, err := s.hostRoom(ctx, accountID, roomID, false); err != nil {
		return err
	}
	ps, err := s.d.Repository.Participants(ctx, nil, roomID)
	if err != nil {
		return err
	}
	now := s.now()
	err = s.roomTx(ctx, []string{roomID}, func(tx uow.Tx, rec RoomRecord) error {
		switch {
		case rec.Head.State != voice.StateOpen:
			return ErrNotFound
		case rec.Head.HostAccountID != accountID:
			return ErrNotHost
		}
		_, err = s.endRoomTx(ctx, tx, roomID, voice.EndHost, accountID, now)
		return err
	})
	if err != nil {
		return err
	}
	s.forgetAll(ctx, ps)
	return nil
}
