// SPDX-License-Identifier: AGPL-3.0-only

package identity_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqliteidentity "github.com/mycroblog/my/internal/adapter/db/sqlite/identity"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestRefreshReplayFlows(t *testing.T) {
	t.Parallel()
	identitytest.RefreshReplayFlows(t, accountHarness)
}

// The pair a refresh replays is stored so that the database alone cannot open it (ADR-0090 §18):
// the successor's row holds neither its tokens nor the refresh token it replaced in clear.
func TestRefreshReplayPairIsNotReadableFromTheDatabase(t *testing.T) {
	t.Parallel()
	var db *sqlite.DB
	e := identitytest.NewAccountEnv(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		db = flowtest.SQLite(t, opts)
		return identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)}
	})
	ctx := t.Context()
	first, _ := e.WebTokens(t)
	second, err := e.Identity.RefreshWebClient(ctx, first.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshWebClient = %v", err)
	}

	pairs := map[string][]byte{}
	rows, err := db.SQLDB().QueryContext(ctx, `SELECT id, replay_pair FROM oauth_access_tokens WHERE replay_pair IS NOT NULL`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		var pair []byte
		if err := rows.Scan(&id, &pair); err != nil {
			t.Fatal(err)
		}
		pairs[id] = pair
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	pair, ok := pairs[second.Token.ID]
	if len(pairs) != 1 || !ok || len(pair) == 0 {
		t.Fatalf("rows with a replay pair = %d, the successor's %d bytes; want the successor's row only", len(pairs), len(pair))
	}
	for name, secret := range map[string]string{
		"the successor's access token": second.AccessToken, "the successor's refresh token": second.RefreshToken,
		"the replaced refresh token": first.RefreshToken,
	} {
		random := strings.TrimPrefix(strings.TrimPrefix(secret, appidentity.AccessTokenPrefix), appidentity.RefreshTokenPrefix)
		if bytes.Contains(pair, []byte(secret)) || bytes.Contains(pair, []byte(random)) {
			t.Errorf("the stored replay pair holds %s in clear", name)
		}
	}
}
