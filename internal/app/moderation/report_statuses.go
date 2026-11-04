// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"errors"

	appcontent "github.com/mycroblog/my/internal/app/content"
)

// Evidence is the part of the content module's public API that presents the statuses of a
// report to the moderators. *appcontent.Reader implements it.
type Evidence interface {
	// Review returns a status whatever its visibility, deleted ones included; it fails with an
	// error wrapping appcontent.ErrNotFound for a status that is gone.
	Review(ctx context.Context, statusID string) (appcontent.ReviewedStatus, error)
}

// ReportStatuses returns the statuses that a report points at (report_statuses), in the order
// of its StatusIDs, for the moderators who handle it (FR-710, NFR-MOD-4). They are presented
// whatever their visibility and also when their author deleted them, because the report keeps
// them as its evidence (ADR-0058); the statuses that are gone are left out. It shows the
// moderators statuses past content.CanView, so it reads no status that the report does not
// point at, and its callers serve it to holders of the manage_reports permission only. An
// unknown report fails with ErrNotFound.
func (s *Service) ReportStatuses(ctx context.Context, reportID string) ([]appcontent.ReviewedStatus, error) {
	r, err := s.d.Repository.FindReport(ctx, reportID)
	if err != nil {
		return nil, err
	}
	if s.d.Evidence == nil {
		return nil, nil
	}
	out := make([]appcontent.ReviewedStatus, 0, len(r.StatusIDs))
	for _, id := range r.StatusIDs {
		st, err := s.d.Evidence.Review(ctx, id)
		if errors.Is(err, appcontent.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}
