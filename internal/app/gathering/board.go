// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/domain/gathering"
)

// The board of an event (ADR-0161 §17) holds posts of the content module, which asks the
// functions below who reads and who writes on it; this module never reads the posts.

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
// their records now (ADR-0161 §14, §17): those that ended (at their end, or at their
// cancellation) longer than event_records_retention_days ago and that no report points at, and
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
		if end, ok := rec.endedAt(); ok && end.Before(before) {
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

// endedAt returns when the event ended: its cancellation, or the end of its date; false for an
// event that polls its date and was not cancelled.
func (r Record) endedAt() (time.Time, bool) {
	switch {
	case r.Cancellation != nil:
		return r.Cancellation.CancelledAt, true
	case r.Scheduled():
		return r.Confirmation.Schedule.EndsAt, true
	}
	return time.Time{}, false
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
