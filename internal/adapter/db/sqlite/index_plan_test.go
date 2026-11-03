// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
)

// TestQueriesUseTheirIndexes checks with EXPLAIN QUERY PLAN that frequent or growing queries read an
// index instead of scanning their table.
func TestQueriesUseTheirIndexes(t *testing.T) {
	db, _ := newTestDB(t, "", uow.Options{})
	for _, tc := range []struct{ name, query, index string }{
		{
			"the sessions of a browser's sign-out",
			"SELECT code_hash FROM oauth_authorization_codes WHERE session_id = 'x'",
			"oauth_authorization_codes_session_id",
		},
		{
			"the erasure of a user's authorization codes",
			"DELETE FROM oauth_authorization_codes WHERE user_id = 'x'",
			"oauth_authorization_codes_user_id",
		},
		{
			"the erasure of a user's one-time tokens",
			"DELETE FROM user_tokens WHERE user_id = 'x'",
			"user_tokens_user_id",
		},
		{
			"the local custom emoji by shortcode",
			"SELECT id FROM custom_emojis WHERE domain = '' ORDER BY shortcode",
			"custom_emojis_domain_shortcode",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := db.db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+tc.query)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = rows.Close() }()
			var plan strings.Builder
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				plan.WriteString(detail + "\n")
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(plan.String(), tc.index) {
				t.Errorf("the plan does not use %s:\n%s", tc.index, plan.String())
			}
		})
	}
}
