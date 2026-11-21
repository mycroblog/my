// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"errors"

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
