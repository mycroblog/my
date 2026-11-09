// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"slices"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// memberInviteSettings reads the instance settings of the invites that members create
// (ADR-0106 §14, §15) for the identity module.
func (a *App) memberInviteSettings(ctx context.Context) (appidentity.MemberInviteSettings, error) {
	s, err := a.Instance.Settings(ctx)
	if err != nil {
		return appidentity.MemberInviteSettings{}, err
	}
	caps := s.MemberInviteCaps()
	return appidentity.MemberInviteSettings{
		Everyone: s.InviteCreators() == instance.InviteCreatorsEveryone,
		Caps:     domainidentity.MemberInviteCaps{MaxActive: caps.MaxActive, MaxUses: caps.MaxUses, MaxExpiresIn: caps.MaxExpiresIn},
	}, nil
}

// inviterRestricted reports whether moderation suspends (or bans) or limits a local account,
// which keeps its user from creating invites and its invites from working (ADR-0106 §14, §16).
// A suspension or a ban restricts the federation of the account, and a limit its unsolicited
// reach (data-model §5.2).
func (a *App) inviterRestricted(ctx context.Context, accountID string) (bool, error) {
	rs, err := a.AccountPolicy.Restrictions(ctx, accountID)
	if err != nil {
		return false, err
	}
	return slices.Contains(rs, moderation.RestrictFederation) || slices.Contains(rs, moderation.RestrictUnsolicitedReach), nil
}

// inviterRestrictedAmong is inviterRestricted of many accounts, with one read of their
// restrictions.
func (a *App) inviterRestrictedAmong(ctx context.Context, accountIDs []string) (map[string]bool, error) {
	all, err := a.AccountPolicy.RestrictionsOf(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(accountIDs))
	for _, id := range accountIDs {
		rs := all[id]
		out[id] = slices.Contains(rs, moderation.RestrictFederation) || slices.Contains(rs, moderation.RestrictUnsolicitedReach)
	}
	return out, nil
}
