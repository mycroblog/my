// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
)

// TestChecksRejectValuesNothingWrites checks that the lists of the CHECKs hold only what the server
// writes: growing a list later is additive, while a value nothing writes only misleads.
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := db.db.ExecContext(t.Context(), tc.stmt, tc.args...)
			if tc.rejected && err == nil {
				t.Error("the CHECK accepted a value nothing writes")
			}
			if !tc.rejected && err != nil {
				t.Errorf("the CHECK rejected a value in use: %v", err)
			}
		})
	}
}
