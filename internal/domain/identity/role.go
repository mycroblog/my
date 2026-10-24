// SPDX-License-Identifier: AGPL-3.0-only

package identity

import "time"

// Permission is a capability that a role grants.
type Permission string

// Permissions (data-model §4.1, role_permissions.permission).
const (
	PermManageReports  Permission = "manage_reports"
	PermManageAccounts Permission = "manage_accounts"
	PermManageDomains  Permission = "manage_domains"
	PermManageRules    Permission = "manage_rules"
	PermManageSettings Permission = "manage_settings"
	PermManageRoles    Permission = "manage_roles"
	PermManageInvites  Permission = "manage_invites"
	PermViewAudit      Permission = "view_audit"
	// PermManageJobs: read and requeue the dead jobs of the job queue.
	PermManageJobs Permission = "manage_jobs"
	// PermManageLoginRecovery: recover a local user's sign-in, by removing their two-factor
	// authentication (FR-005) or issuing a password reset link for them (FR-002). It is server
	// administration (ADR-0058 §4): only the built-in admin role has it by default.
	PermManageLoginRecovery Permission = "manage_login_recovery"
	// PermManageCustomEmojis: register, change, disable, and delete the custom emoji, local and
	// remote (ADR-0051 §5). It is server administration (ADR-0058 §4).
	PermManageCustomEmojis Permission = "manage_custom_emojis"
	// PermManageWebhooks: set up the instance's webhooks, see their deliveries, and send them
	// again (ADR-0060). It is server administration (ADR-0058 §4).
	PermManageWebhooks Permission = "manage_webhooks"
	// PermManageAnnouncements: write, publish, and withdraw the server-wide announcements
	// (FR-726). It is server administration (ADR-0058 §4).
	PermManageAnnouncements Permission = "manage_announcements"
	// PermManageFinance: record the instance's costs, read their totals, and see and start the
	// billing imports (ADR-0067 §7, ADR-0068 §9). It is server administration (ADR-0058 §4).
	PermManageFinance Permission = "manage_finance"
	// PermViewFinance: read the totals of the instance's costs, with as much of the cost items as
	// the instance settings finance_viewer_* show, without recording or changing anything
	// (ADR-0070 §5). It is a member permission (ADR-0070 §1).
	PermViewFinance Permission = "view_finance"
	// PermManageSupporters: enable and disable the supporter program, create, change, and retire
	// its plans, and see and cancel the subscriptions (ADR-0072 §2). It is server administration
	// (ADR-0058 §4).
	PermManageSupporters Permission = "manage_supporters"
	// PermCreateCustomEmojis: request new local custom emoji, which a holder of
	// manage_custom_emojis approves, and change and delete the ones the holder made, while
	// holding it (ADR-0070 §2, §3). It is a member permission (ADR-0070 §1), whose limit a
	// supporter plan sets (ADR-0072 §11).
	PermCreateCustomEmojis Permission = "create_custom_emojis"
	// PermManageOAuthApps: list every OAuth client, disable one and lift the disablement, and
	// deny the hosts of Client ID Metadata Documents (ADR-0090 §1). It is server administration
	// (ADR-0058 §4): only the built-in admin role has it by default.
	PermManageOAuthApps Permission = "manage_oauth_apps"
)

// AllPermissions lists every permission.
func AllPermissions() []Permission {
	return []Permission{
		PermManageReports, PermManageAccounts, PermManageDomains, PermManageRules,
		PermManageSettings, PermManageRoles, PermManageInvites, PermViewAudit, PermManageJobs,
		PermManageLoginRecovery, PermManageCustomEmojis, PermManageWebhooks, PermManageAnnouncements,
		PermManageFinance, PermViewFinance, PermManageSupporters, PermCreateCustomEmojis, PermManageOAuthApps,
	}
}

// PermissionCategory is the area that a permission belongs to (ADR-0058 §4, ADR-0070 §1):
// moderation decides on people, posts and remote servers case by case; server administration
// changes the server itself as a whole; a member permission lets its holder make and change
// their own things or only read, never change the rows of others or the whole server. Only
// member permissions may be on a role that a supporter plan manages (ADR-0072 §5).
type PermissionCategory string

// Permission categories.
const (
	CategoryModeration PermissionCategory = "moderation"
	CategoryServer     PermissionCategory = "server"
	CategoryMember     PermissionCategory = "member"
)

// Category returns the area that p belongs to; a permission of no known area is server
// administration. The web client's navigation and `my admin role` group the permissions by it.
func (p Permission) Category() PermissionCategory {
	switch p {
	case PermManageReports, PermManageAccounts, PermManageDomains, PermManageInvites, PermViewAudit:
		return CategoryModeration
	case PermViewFinance, PermCreateCustomEmojis:
		return CategoryMember
	default:
		return CategoryServer
	}
}

// Role is a named set of permissions. Position orders roles; a higher position outranks a lower one.
type Role struct {
	ID        string
	Key       string
	Position  int
	CreatedAt time.Time
}

// RoleGrant is a role granted to a user (data-model §4.1, user_role_grants).
type RoleGrant struct {
	ID     string
	UserID string
	RoleID string
	// GrantedByUserID is the user who granted the role, or "" for a grant from the CLI.
	GrantedByUserID string
	GrantedAt       time.Time
}

// Keys of the built-in roles.
const (
	RoleAdmin     = "admin"
	RoleModerator = "moderator"
)

// BuiltinRole describes a role that every instance has.
type BuiltinRole struct {
	Key         string
	Position    int
	Permissions []Permission
}

// BuiltinRoles returns the built-in roles: admin holds every permission; moderator handles
// reports and accounts and reads the audit log.
func BuiltinRoles() []BuiltinRole {
	return []BuiltinRole{
		{Key: RoleAdmin, Position: AdminPosition, Permissions: AllPermissions()},
		{Key: RoleModerator, Position: 100, Permissions: []Permission{PermManageReports, PermManageAccounts, PermViewAudit}},
	}
}
