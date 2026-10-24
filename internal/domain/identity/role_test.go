// SPDX-License-Identifier: AGPL-3.0-only

package identity

import "testing"

func TestPermissionCategory(t *testing.T) {
	want := map[Permission]PermissionCategory{
		PermManageReports:       CategoryModeration,
		PermManageAccounts:      CategoryModeration,
		PermManageDomains:       CategoryModeration,
		PermManageInvites:       CategoryModeration,
		PermViewAudit:           CategoryModeration,
		PermManageSettings:      CategoryServer,
		PermManageRules:         CategoryServer,
		PermManageRoles:         CategoryServer,
		PermManageJobs:          CategoryServer,
		PermManageLoginRecovery: CategoryServer,
		PermManageCustomEmojis:  CategoryServer,
		PermManageWebhooks:      CategoryServer,
		PermManageAnnouncements: CategoryServer,
		PermManageFinance:       CategoryServer,
		PermViewFinance:         CategoryMember,
		PermCreateCustomEmojis:  CategoryMember,
		PermManageSupporters:    CategoryServer,
		PermManageOAuthApps:     CategoryServer,
	}
	for _, p := range AllPermissions() {
		w, ok := want[p]
		if !ok {
			t.Errorf("permission %s has no expected category in this test", p)
			continue
		}
		if got := p.Category(); got != w {
			t.Errorf("%s.Category() = %s, want %s", p, got, w)
		}
	}
}
