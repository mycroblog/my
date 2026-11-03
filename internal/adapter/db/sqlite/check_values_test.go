// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
)

// TestChecksRejectValuesNothingWrites checks that the lists of the CHECKs hold only what the server
// writes, on projections too: growing a list later is additive, while a value nothing writes only
// misleads.
func TestChecksRejectValuesNothingWrites(t *testing.T) {
	db, _ := newTestDB(t, "", uow.Options{})
	const id = "0192f0c1-0000-7000-8000-000000000001"
	for _, tc := range []struct {
		name, stmt string
		args       []any
		rejected   bool
	}{
		{"expired_cleanup", "INSERT INTO session_revocations (session_id, reason, revoked_at) VALUES (?, ?, 0)", []any{id, "expired_cleanup"}, true},
		{"logout", "INSERT INTO session_revocations (session_id, reason, revoked_at) VALUES (?, ?, 0)", []any{id, "logout"}, false},
		{"status_deleted", "INSERT INTO boost_retractions (boost_id, origin, retracted_at) VALUES (?, ?, 0)", []any{id, "status_deleted"}, true},
		{"moderation", "INSERT INTO boost_retractions (boost_id, origin, retracted_at) VALUES (?, ?, 0)", []any{id, "moderation"}, true},
		{"booster", "INSERT INTO boost_retractions (boost_id, origin, retracted_at) VALUES (?, ?, 0)", []any{id, "booster"}, false},
		{"system", "INSERT INTO moderation_actions (id, target_account_id, kind, reason_category, issuer, notify_target, issued_at) VALUES (?, ?, 'warn', 'other', ?, 0, 0)", []any{id, id, "system"}, true},
		{"remote", "INSERT INTO moderation_actions (id, target_account_id, kind, reason_category, issuer, notify_target, issued_at) VALUES (?, ?, 'warn', 'other', ?, 0, 0)", []any{id, id, "remote"}, true},
		{"moderator", "INSERT INTO moderation_actions (id, target_account_id, kind, reason_category, issuer, notify_target, issued_at) VALUES (?, ?, 'warn', 'other', ?, 0, 0)", []any{id, id, "moderator"}, false},
		{"email of a status", "INSERT INTO notification_settings (account_id, notification_group, channel, enabled, updated_at) VALUES (?, 'status', 'email', 1, 0)", []any{id}, true},
		{"email of an announcement", "INSERT INTO notification_settings (account_id, notification_group, channel, enabled, updated_at) VALUES (?, 'announcement', 'email', 1, 0)", []any{id}, true},
		{"the app for a status", "INSERT INTO notification_settings (account_id, notification_group, channel, enabled, updated_at) VALUES (?, 'status', 'in_app', 1, 0)", []any{id}, false},
		{"email of a mention", "INSERT INTO notification_settings (account_id, notification_group, channel, enabled, updated_at) VALUES (?, 'mention', 'email', 1, 0)", []any{id}, false},
		{"a ceiling on a domain", "INSERT INTO feed_domain_restrictions (domain, restriction) VALUES ('a.example', 'visibility_ceiling_private')", nil, true},
		{"discovery on a domain", "INSERT INTO feed_domain_restrictions (domain, restriction) VALUES ('a.example', 'public_discovery')", nil, false},
		{"profile revision 0", "INSERT INTO account_profile_revisions (id, account_id, seq, display_name, note_html, locked, bot, discoverable, indexable, ai_opt_out, revised_at) VALUES (?, ?, 0, '', '', 0, 0, 0, 0, 0, 0)", []any{id, id}, true},
		{"an unknown plan visibility", "INSERT INTO supporter_plan_heads (plan_id, revision_id, rank, visibility) VALUES (?, ?, 1, 'secret')", []any{id, id}, true},
		{"an unknown price interval", "INSERT INTO supporter_plan_price_heads (plan_id, interval, price_id) VALUES (?, 'week', ?)", []any{id, id}, true},
		{"an unknown subscription state", "INSERT INTO supporter_subscription_heads (subscription_id, transition_id, user_id, plan_id, state) VALUES (?, ?, ?, ?, 'gone')", []any{id, id, id, id}, true},
		{"an unknown brand kind", "INSERT INTO instance_brands (kind, revision_id) VALUES ('favicon', ?)", []any{id}, true},
		{"an unknown cost source", "INSERT INTO imported_cost_heads (source, billing_month, service, charge_type, revision_id) VALUES ('azure', '2026-01', 's', 'usage', ?)", []any{id}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := db.db.ExecContext(t.Context(), tc.stmt, tc.args...)
			if tc.rejected && (err == nil || !strings.Contains(err.Error(), "CHECK constraint")) {
				t.Errorf("the insert = %v, want a CHECK to reject the value", err)
			}
			if !tc.rejected && err != nil {
				t.Errorf("the CHECK rejected a value in use: %v", err)
			}
		})
	}
}
