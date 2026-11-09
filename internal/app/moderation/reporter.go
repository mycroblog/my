// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"

	"github.com/mycroblog/my/internal/domain/moderation"
)

// The reporter's use cases: a local user's own reports and whether each is resolved (FR-711).
// They return the reports with their states, and the client API presents only whether each is
// open or resolved: never the outcome, the actions it led to, or who handles it.

// ReportsFiledBy lists up to limit reports that reporterAccountID filed, newest first, those
// whose ID is below before ("" for the first page).
func (s *Service) ReportsFiledBy(ctx context.Context, reporterAccountID, before string, limit int) ([]moderation.ReportRecord, error) {
	return s.d.Repository.ListReportsByReporter(ctx, reporterAccountID, before, limit)
}

// ReportFiledBy returns a report that reporterAccountID filed with its state; it fails with
// ErrNotFound for an unknown ID and for a report that someone else filed.
// ReportsFiledByID returns the reports of ids that reporterAccountID filed, by ID, as ReportFiledBy
// returns one, reading them and their statuses together.
func (s *Service) ReportsFiledByID(ctx context.Context, reporterAccountID string, ids []string) (map[string]moderation.ReportRecord, error) {
	reports, err := s.d.Repository.FindReports(ctx, ids)
	if err != nil {
		return nil, err
	}
	statuses, err := s.d.Repository.FindReportStatuses(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := map[string]moderation.ReportRecord{}
	for id, r := range reports {
		st, ok := statuses[id]
		if !ok || r.ReporterAccountID != reporterAccountID {
			continue // a report has a status from its filing, as ReportRecord requires
		}
		out[id] = moderation.ReportRecord{Report: r, Status: st}
	}
	return out, nil
}

func (s *Service) ReportFiledBy(ctx context.Context, reporterAccountID, id string) (moderation.ReportRecord, error) {
	r, err := s.ReportRecord(ctx, id)
	if err != nil {
		return moderation.ReportRecord{}, err
	}
	if r.Report.ReporterAccountID != reporterAccountID {
		return moderation.ReportRecord{}, ErrNotFound
	}
	return r, nil
}
