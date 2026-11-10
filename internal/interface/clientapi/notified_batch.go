// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// This file reads the moderation records that a page of notifications tells of together: through
// the batched reads of the ports when they have them (the services of the server do), one by one
// otherwise (the tests' fakes).

type ownActionBatch interface {
	OwnActionsByID(ctx context.Context, accountID string, ids []string) (map[string]appmoderation.TargetActionView, error)
}

type ownAppealActionBatch interface {
	OwnAppealActions(ctx context.Context, accountID string, appealIDs []string) (map[string]appmoderation.TargetActionView, error)
}

type severanceBatch interface {
	SeverancesOf(ctx context.Context, accountID string, ids []string) (map[string]moderation.SeveranceRecord, error)
}

type filedReportBatch interface {
	ReportsFiledByID(ctx context.Context, reporterAccountID string, ids []string) (map[string]moderation.ReportRecord, error)
}

// ownActionsByID returns the actions of ids on accountID, by ID; one that is missing or on
// another account is left out.
func (s *Server) ownActionsByID(ctx context.Context, accountID string, ids []string) (map[string]appmoderation.TargetActionView, error) {
	if b, ok := s.o.OwnModeration.(ownActionBatch); ok {
		return b.OwnActionsByID(ctx, accountID, ids)
	}
	return byOne(ids, func(id string) (appmoderation.TargetActionView, error) {
		return s.o.OwnModeration.OwnAction(ctx, accountID, id)
	}, appmoderation.ErrNotFound)
}

// ownAppealActions returns the actions that the appeals of appealIDs of accountID appeal, by
// appeal ID.
func (s *Server) ownAppealActions(ctx context.Context, accountID string, appealIDs []string) (map[string]appmoderation.TargetActionView, error) {
	if b, ok := s.o.OwnAppeals.(ownAppealActionBatch); ok {
		return b.OwnAppealActions(ctx, accountID, appealIDs)
	}
	return byOne(appealIDs, func(id string) (appmoderation.TargetActionView, error) {
		return s.o.OwnAppeals.OwnAppealAction(ctx, accountID, id)
	}, appmoderation.ErrNotFound)
}

// severancesOf returns the severances of ids of accountID, by ID.
func (s *Server) severancesOf(ctx context.Context, accountID string, ids []string) (map[string]moderation.SeveranceRecord, error) {
	if b, ok := s.o.Severances.(severanceBatch); ok {
		return b.SeverancesOf(ctx, accountID, ids)
	}
	return byOne(ids, func(id string) (moderation.SeveranceRecord, error) {
		return s.o.Severances.SeveranceOf(ctx, accountID, id)
	}, appmoderation.ErrNotFound)
}

// reportsFiledByID returns the reports of ids that reporterAccountID filed, by ID.
func (s *Server) reportsFiledByID(ctx context.Context, reporterAccountID string, ids []string) (map[string]moderation.ReportRecord, error) {
	if b, ok := s.o.FiledReports.(filedReportBatch); ok {
		return b.ReportsFiledByID(ctx, reporterAccountID, ids)
	}
	return byOne(ids, func(id string) (moderation.ReportRecord, error) {
		return s.o.FiledReports.ReportFiledBy(ctx, reporterAccountID, id)
	}, appmoderation.ErrNotFound)
}

// byOne reads each of ids with read, by ID, leaving out the ones that fail with missing.
func byOne[T any](ids []string, read func(id string) (T, error), missing error) (map[string]T, error) {
	out := make(map[string]T, len(ids))
	for _, id := range ids {
		if _, ok := out[id]; ok {
			continue
		}
		v, err := read(id)
		switch {
		case errors.Is(err, missing):
		case err != nil:
			return nil, err
		default:
			out[id] = v
		}
	}
	return out, nil
}
