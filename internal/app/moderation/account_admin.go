// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/domain/moderation"
)

// What the moderators' account management (FR-714, the identity module's AccountAdmin) reads of
// this module: the restrictions that an account's own actions impose, the accounts on which they
// impose one, and the reports of an account.

// Restriction is moderation.Restriction.
type Restriction = moderation.Restriction

// Restrictions that the account management derives the state of an account from.
const (
	// RestrictVisibilityCeilingPrivate is imposed by limit only.
	RestrictVisibilityCeilingPrivate = moderation.RestrictVisibilityCeilingPrivate
)

// The audited operations of the account management and their target.
const (
	AuditRevealedPrivateData     = moderation.AuditRevealedPrivateData
	AuditTwoFactorDisabled       = moderation.AuditTwoFactorDisabled
	AuditPasswordResetLinkIssued = moderation.AuditPasswordResetLinkIssued
	AuditEmailSuppressionLifted  = moderation.AuditEmailSuppressionLifted
	AuditTargetAccount           = moderation.AuditTargetAccount
)

// AccountAdminRepository is the part of Repository that the account management reads.
type AccountAdminRepository interface {
	// ListRestrictedAccountIDs returns up to limit accounts on which their own actions impose r
	// in effect at now, newest first, by ID, whose ID is below before ("" for the first page).
	ListRestrictedAccountIDs(ctx context.Context, r moderation.Restriction, now time.Time, before string, limit int) ([]string, error)
}

// OwnRestrictions returns the restrictions in effect that the actions on an account impose, in
// restriction order, without those of its domain.
func (p *AccountPolicy) OwnRestrictions(ctx context.Context, accountID string) ([]moderation.Restriction, error) {
	return p.repo.AccountRestrictionsInEffect(ctx, accountID, p.clock.Now())
}

// RestrictedAccounts returns up to limit accounts on which their own actions impose r now,
// newest first, by ID, whose ID is below before ("" for the first page).
func (p *AccountPolicy) RestrictedAccounts(ctx context.Context, r moderation.Restriction, before string, limit int) ([]string, error) {
	return p.repo.ListRestrictedAccountIDs(ctx, r, p.clock.Now(), before, limit)
}

// RecentReportsOf returns the newest limit reports of a target account with their states, newest
// first.
func (s *Service) RecentReportsOf(ctx context.Context, targetAccountID string, limit int) ([]moderation.ReportRecord, error) {
	rs, err := s.d.Repository.ListReportsOf(ctx, targetAccountID)
	if err != nil {
		return nil, err
	}
	slices.Reverse(rs)
	if len(rs) > limit {
		rs = rs[:limit]
	}
	out := make([]moderation.ReportRecord, 0, len(rs))
	for _, r := range rs {
		st, err := s.d.Repository.FindReportStatus(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, moderation.ReportRecord{Report: r, Status: st})
	}
	return out, nil
}
