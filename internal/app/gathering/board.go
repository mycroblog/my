// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"errors"
	"time"

	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// The board of an event (ADR-0161 §17) holds posts of the content module, which asks the
// functions below who reads and who writes on it; this module never reads the posts.

// BoardPost is a new post on the board of an event, as the composition root passes it on from
// the content module, whose domain event this module does not import.
type BoardPost struct {
	StatusID    string
	GatheringID string
	// WriterAccountID is the author of the post, which is not notified of it.
	WriterAccountID string
	// MentionedReaderIDs are the readers of the board that the post mentions, whom the mention
	// notifies of it instead.
	MentionedReaderIDs []string
	PostedAt           time.Time
}

// BoardPosted is what the composition root calls in the transaction of a new post on the board
// of an event: it enqueues the post's event_board_posted (ADR-0161 §11, §17), whose job tells the
// readers of the board who answered going or maybe of it, deciding them when it runs, but the
// writer, the organizer, and those whom the post mentions.
func (s *Service) BoardPosted(ctx context.Context, tx uow.Tx, p BoardPost) error {
	return s.enqueueNotify(ctx, tx, notifyPayload{
		Kind: appnotification.EventBoardPosted, Gathering: p.GatheringID, Ref: p.StatusID,
		Skip: p.MentionedReaderIDs, Source: p.WriterAccountID, Actor: p.WriterAccountID, At: p.PostedAt,
	})
}

// Board returns what accountID may do with the board of the event id at the time of the clock
// (gathering.Board): the zero BoardAccess for an unknown event and for a viewer who is not
// logged in.
func (s *Service) Board(ctx context.Context, accountID, id string) (gathering.BoardAccess, error) {
	rec, err := s.d.Repository.FindGathering(ctx, nil, id)
	if errors.Is(err, ErrNotFound) {
		return gathering.BoardAccess{}, nil
	}
	if err != nil {
		return gathering.BoardAccess{}, err
	}
	return s.boardAccess(ctx, rec, accountID)
}

// BoardReaders returns which of accountIDs read the board of the event id
// (gathering.CanReadBoard), by account; none of them for an unknown event.
func (s *Service) BoardReaders(ctx context.Context, id string, accountIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}
	rec, err := s.d.Repository.FindGathering(ctx, nil, id)
	if errors.Is(err, ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, accountID := range accountIDs {
		if _, seen := out[accountID]; seen || accountID == "" {
			continue
		}
		a, err := s.boardAccess(ctx, rec, accountID)
		if err != nil {
			return nil, err
		}
		out[accountID] = a.Reads
	}
	return out, nil
}

// BoardsToPurge returns, in their order, the events of gatheringIDs whose boards' posts go with
// their records now (ADR-0161 §14, §17): those that ended longer than
// event_records_retention_days ago, by the end that the purge of the records counts from
// (gathering.BoardTerm.Ended: the earlier of the end, that of the last candidate date while the
// event polls its date, and the cancellation), and that no report points at, and
// the events that do not exist. Without Deps.Retention it returns none.
func (s *Service) BoardsToPurge(ctx context.Context, gatheringIDs []string) ([]string, error) {
	r := s.d.Retention
	if r == nil || len(gatheringIDs) == 0 {
		return nil, nil
	}
	period, err := r.RecordsRetention(ctx)
	if err != nil {
		return nil, err
	}
	before := s.now().Add(-period)
	var ended []string
	for _, id := range gatheringIDs {
		rec, err := s.d.Repository.FindGathering(ctx, nil, id)
		switch {
		case errors.Is(err, ErrNotFound):
			ended = append(ended, id)
			continue
		case err != nil:
			return nil, err
		}
		if end, ok := rec.BoardTerm().Ended(); ok && end.Before(before) {
			ended = append(ended, id)
		}
	}
	if len(ended) == 0 {
		return nil, nil
	}
	reported, err := r.ReportedGatherings(ctx, ended)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ended))
	for _, id := range ended {
		if !reported[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

// boardAccess returns what accountID may do with the board of rec.
func (s *Service) boardAccess(ctx context.Context, rec Record, accountID string) (gathering.BoardAccess, error) {
	viewer, err := s.viewer(ctx, accountID)
	if err != nil {
		return gathering.BoardAccess{}, err
	}
	facts, _, err := s.facts(ctx, viewer, []Record{rec})
	if err != nil {
		return gathering.BoardAccess{}, err
	}
	return gathering.Board(rec.Event(), rec.BoardTerm(), viewer, facts[rec.ID], s.now()), nil
}
