// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"slices"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
)

// What the other modules read of the supporters through the platform (ADR-0072 §11, §12): the
// limit of the custom emoji that a plan sets (content.CustomEmojiQuota, ADR-0070 §4) and the
// supporter directory of the finance module (finance.SupporterDirectory, ADR-0067). Both count
// the member states only (active, canceling, past due), and both are empty while the program is
// disabled.

// EmojiLimit returns how many custom emoji a user may create, and false for no limit
// (ADR-0072 §11): a role that no plan manages and that lets its holders create emoji sets no
// limit; otherwise the plan of the user's role sets its limit; and without either, or while the
// program is disabled, there is no limit (the content module checks the permission itself).
func (s *Service) EmojiLimit(ctx context.Context, userID string) (int, bool, error) {
	enabled, err := s.Enabled(ctx)
	if err != nil || !enabled {
		return 0, false, err
	}
	held, err := s.d.Roles.UserRoleIDs(ctx, userID)
	if err != nil || len(held) == 0 {
		return 0, false, err
	}
	roles, err := s.d.Roles.Roles(ctx)
	if err != nil {
		return 0, false, err
	}
	managed, err := s.d.Roles.ManagedRoleIDs(ctx)
	if err != nil {
		return 0, false, err
	}
	planRole := ""
	for _, r := range roles {
		if !slices.Contains(held, r.ID) || !slices.ContainsFunc(r.Permissions, func(p appidentity.Permission) bool {
			return string(p) == PermCreateCustomEmojis
		}) {
			continue
		}
		if !slices.Contains(managed, r.ID) {
			return 0, false, nil
		}
		planRole = r.ID
	}
	if planRole == "" {
		return 0, false, nil
	}
	plans, err := s.d.Repository.ListPlans(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	for _, p := range plans {
		if p.RoleID == planRole {
			return p.Revision.EmojiLimit, true, nil
		}
	}
	return 0, false, nil
}

// PlanCount is the number of members of a plan (ADR-0072 §12).
type PlanCount struct {
	PlanID  string
	Name    string
	Members int64
}

// PlanCounts returns the member counts of the plans that are not retired and of the retired
// plans that still have members, by rank; nothing while the program is disabled.
func (s *Service) PlanCounts(ctx context.Context) ([]PlanCount, error) {
	enabled, err := s.Enabled(ctx)
	if err != nil || !enabled {
		return nil, err
	}
	plans, err := s.d.Repository.ListPlans(ctx, nil)
	if err != nil {
		return nil, err
	}
	members, err := s.d.Repository.CountMembers(ctx)
	if err != nil {
		return nil, err
	}
	var out []PlanCount
	for _, p := range plans {
		if p.Retired && members[p.ID] == 0 {
			continue
		}
		out = append(out, PlanCount{PlanID: p.ID, Name: p.Revision.Name, Members: members[p.ID]})
	}
	return out, nil
}

// SubscriptionUsers returns the subscribers of the subscriptions among ids, in one lookup; an
// unknown ID is left out, and nothing while the program is disabled (ADR-0072 §12).
func (s *Service) SubscriptionUsers(ctx context.Context, ids []string) ([]SubscriptionUser, error) {
	enabled, err := s.Enabled(ctx)
	if err != nil || !enabled || len(ids) == 0 {
		return nil, err
	}
	return s.d.Repository.SubscriptionUsers(ctx, ids)
}

// ProviderLocation returns the name of the configured payment provider and the time zone in
// which its money movements fall on their dates (ADR-0067 §13); false with payments =
// none.
func (s *Service) ProviderLocation() (string, *time.Location, bool) {
	if s.d.Provider == nil {
		return "", nil, false
	}
	return s.d.Provider.Name(), s.d.Provider.Location(), true
}
