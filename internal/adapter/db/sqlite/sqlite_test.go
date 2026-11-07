// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	msqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/identitygen"
	"github.com/mycroblog/my/internal/adapter/payment/fake"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/contenttest"
	"github.com/mycroblog/my/pkg/testkit/diagnosticstest"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
	"github.com/mycroblog/my/pkg/testkit/financetest"
	"github.com/mycroblog/my/pkg/testkit/graphtest"
	"github.com/mycroblog/my/pkg/testkit/hooktest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
	"github.com/mycroblog/my/pkg/testkit/instancetest"
	"github.com/mycroblog/my/pkg/testkit/moderationtest"
	"github.com/mycroblog/my/pkg/testkit/notificationtest"
	"github.com/mycroblog/my/pkg/testkit/searchtest"
	"github.com/mycroblog/my/pkg/testkit/sqlitetest"
	"github.com/mycroblog/my/pkg/testkit/supporterstest"
	"github.com/mycroblog/my/pkg/testkit/timelinetest"
	"github.com/mycroblog/my/pkg/testkit/translationtest"
	"github.com/mycroblog/my/pkg/testkit/uowtest"
	"github.com/mycroblog/my/pkg/testkit/voicetest"
	"github.com/mycroblog/my/pkg/testkit/webhooktest"
)

// newTestDB opens a new database file under t.TempDir() with the product schema applied, a copy of
// the template of package sqlitetest (see package schema for why executing the file matches an
// Atlas apply).
func newTestDB(t *testing.T, dsnSuffix string, opts uow.Options) (*DB, string) {
	t.Helper()
	path := sqlitetest.NewFile(t)
	db, err := Open(t.Context(), path+dsnSuffix, opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

// The UnitOfWork contract uses users as its probe table: a row per key, keyed by id.

func insertProbe(ctx context.Context, q *genQueries, key string) error {
	return q.InsertUser(ctx, identitygen.InsertUserParams{ID: key})
}

func probeKeys(ctx context.Context, db *DB) ([]string, error) {
	rows, err := db.db.QueryContext(ctx, "SELECT id FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	keys := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func TestAccountRepositoryContract(t *testing.T) {
	t.Parallel()
	identitytest.AccountRepositoryContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		db, _ := newTestDB(t, "", opts)
		return identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
	})
}

func TestMediaRepositoryContract(t *testing.T) {
	t.Parallel()
	contenttest.MediaRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.MediaHarness {
		db, _ := newTestDB(t, "", opts)
		return contenttest.MediaHarness{UnitOfWork: db, Media: db.MediaRepository(), Statuses: db.StatusRepository()}
	})
}

func TestLinkCardRepositoryContract(t *testing.T) {
	t.Parallel()
	contenttest.LinkCardRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.LinkCardHarness {
		db, _ := newTestDB(t, "", opts)
		return contenttest.LinkCardHarness{UnitOfWork: db, Cards: db.StatusRepository(), Media: db.MediaRepository()}
	})
}

func TestSearchRepositoryContract(t *testing.T) {
	t.Parallel()
	searchtest.SearchRepositoryContract(t, func(t *testing.T) searchtest.SearchHarness {
		db, _ := newTestDB(t, "", uow.Options{})
		return searchtest.SearchHarness{UnitOfWork: db, Repository: db.SearchRepository()}
	})
}

func TestTranslationRepositoryContract(t *testing.T) {
	t.Parallel()
	translationtest.TranslationRepositoryContract(t, func(t *testing.T) translationtest.TranslationHarness {
		db, _ := newTestDB(t, "", uow.Options{})
		return translationtest.TranslationHarness{UnitOfWork: db, Repository: db.TranslationRepository()}
	})
}

func TestCustomEmojiRepositoryContract(t *testing.T) {
	t.Parallel()
	contenttest.CustomEmojiRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.CustomEmojiHarness {
		db, _ := newTestDB(t, "", opts)
		return contenttest.CustomEmojiHarness{
			UnitOfWork: db, Media: db.MediaRepository(), Statuses: db.StatusRepository(), Emojis: db.CustomEmojiRepository(),
		}
	})
}

func TestWebhookRepositoryContract(t *testing.T) {
	t.Parallel()
	webhooktest.WebhookRepositoryContract(t, func(t *testing.T, opts uow.Options) webhooktest.WebhookHarness {
		db, _ := newTestDB(t, "", opts)
		return webhooktest.WebhookHarness{UnitOfWork: db, Webhooks: db.WebhookRepository(), Moderation: db.ModerationRepository()}
	})
}

func TestFinanceRepositoryContract(t *testing.T) {
	t.Parallel()
	financetest.FinanceRepositoryContract(t, func(t *testing.T, opts uow.Options) financetest.FinanceHarness {
		db, _ := newTestDB(t, "", opts)
		return financetest.FinanceHarness{UnitOfWork: db, Finance: db.FinanceRepository()}
	})
}

func TestVoiceRepositoryContract(t *testing.T) {
	t.Parallel()
	voicetest.VoiceRepositoryContract(t, func(t *testing.T, opts uow.Options) voicetest.VoiceHarness {
		db, _ := newTestDB(t, "", opts)
		return voicetest.VoiceHarness{UnitOfWork: db, Voice: db.VoiceRepository()}
	})
}

func TestVoiceRoomsContract(t *testing.T) {
	t.Parallel()
	voicetest.VoiceRoomsContract(t, func(t *testing.T, opts uow.Options) voicetest.VoiceHarness {
		db, _ := newTestDB(t, "", opts)
		return voicetest.VoiceHarness{UnitOfWork: db, Voice: db.VoiceRepository()}
	})
}

// newPaymentWorld is the provider's side of PaymentRepositoryContract: the fake adapter.
func newPaymentWorld(o supporterstest.PaymentWorldOptions) supporterstest.PaymentWorld {
	return fake.New(fake.Options{Name: o.Name, Mode: o.Mode, Now: o.Now, PageSize: o.PageSize})
}

func TestSupporterRepositoryContract(t *testing.T) {
	t.Parallel()
	supporterstest.SupporterRepositoryContract(t, func(t *testing.T, opts uow.Options) supporterstest.SupporterHarness {
		db, _ := newTestDB(t, "", opts)
		return supporterstest.SupporterHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Supporters:     db.SupporterRepository(), NewWorld: newPaymentWorld,
		}
	})
}

func TestPaymentRepositoryContract(t *testing.T) {
	t.Parallel()
	supporterstest.PaymentRepositoryContract(t, func(t *testing.T, opts uow.Options) supporterstest.PaymentHarness {
		db, _ := newTestDB(t, "", opts)
		return supporterstest.PaymentHarness{UnitOfWork: db, Payments: db.PaymentRepository(), NewWorld: newPaymentWorld}
	})
}

func TestStatusRepositoryContract(t *testing.T) {
	t.Parallel()
	contenttest.StatusRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.StatusHarness {
		db, _ := newTestDB(t, "", opts)
		return contenttest.StatusHarness{UnitOfWork: db, Statuses: db.StatusRepository()}
	})
}

func TestInteractionRepositoryContract(t *testing.T) {
	t.Parallel()
	contenttest.InteractionRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.InteractionHarness {
		db, _ := newTestDB(t, "", opts)
		return contenttest.InteractionHarness{UnitOfWork: db, Statuses: db.StatusRepository(), Interactions: db.InteractionRepository()}
	})
}

func TestAutoDeleteContract(t *testing.T) {
	t.Parallel()
	contenttest.AutoDeleteContract(t, func(t *testing.T, opts uow.Options) contenttest.AutoDeleteHarness {
		db, _ := newTestDB(t, "", opts)
		return contenttest.AutoDeleteHarness{
			InteractionHarness: contenttest.InteractionHarness{UnitOfWork: db, Statuses: db.StatusRepository(), Interactions: db.InteractionRepository()},
			AutoDeletes:        db.StatusRepository(),
		}
	})
}

func TestTimelineContract(t *testing.T) {
	t.Parallel()
	timelinetest.TimelineContract(t, func(t *testing.T, opts uow.Options) timelinetest.TimelineHarness {
		db, _ := newTestDB(t, "", opts)
		return timelinetest.TimelineHarness{
			UnitOfWork: db, Statuses: db.StatusRepository(), Interactions: db.InteractionRepository(),
			Graph: db.GraphRepository(), Timeline: db.TimelineRepository(),
		}
	})
}

func TestNotificationContract(t *testing.T) {
	t.Parallel()
	notificationtest.NotificationContract(t, func(t *testing.T, opts uow.Options) notificationtest.NotificationHarness {
		db, _ := newTestDB(t, "", opts)
		return notificationtest.NotificationHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
			Statuses:       db.StatusRepository(),
			Interactions:   db.InteractionRepository(),
			Notifications:  db.NotificationRepository(),
		}
	})
}

func TestPushSubscriptionContract(t *testing.T) {
	t.Parallel()
	notificationtest.PushSubscriptionContract(t, func(t *testing.T) notificationtest.PushHarness {
		db, _ := newTestDB(t, "", uow.Options{})
		return notificationtest.PushHarness{UnitOfWork: db, Push: db.NotificationRepository()}
	})
}

func TestFollowRepositoryContract(t *testing.T) {
	t.Parallel()
	graphtest.FollowRepositoryContract(t, func(t *testing.T, opts uow.Options) graphtest.FollowHarness {
		db, _ := newTestDB(t, "", opts)
		return graphtest.FollowHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
		}
	})
}

func TestPostingContract(t *testing.T) {
	t.Parallel()
	contenttest.PostingContract(t, func(t *testing.T, opts uow.Options) contenttest.PostingHarness {
		db, _ := newTestDB(t, "", opts)
		return contenttest.PostingHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
			Instance:       db.InstanceRepository(),
			Statuses:       db.StatusRepository(),
			Media:          db.MediaRepository(),
		}
	})
}

func TestBlockRepositoryContract(t *testing.T) {
	t.Parallel()
	graphtest.BlockRepositoryContract(t, func(t *testing.T, opts uow.Options) graphtest.FollowHarness {
		db, _ := newTestDB(t, "", opts)
		return graphtest.FollowHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
		}
	})
}

func TestPreferencesRepositoryContract(t *testing.T) {
	t.Parallel()
	identitytest.PreferencesRepositoryContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		db, _ := newTestDB(t, "", opts)
		return identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
	})
}

func TestRegistrationRepositoryContract(t *testing.T) {
	t.Parallel()
	identitytest.RegistrationRepositoryContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		db, _ := newTestDB(t, "", opts)
		return identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
	})
}

func TestRoleRepositoryContract(t *testing.T) {
	t.Parallel()
	identitytest.RoleRepositoryContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		db, _ := newTestDB(t, "", opts)
		return identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
	})
}

func TestInstanceRepositoryContract(t *testing.T) {
	t.Parallel()
	instancetest.InstanceRepositoryContract(t, func(t *testing.T, opts uow.Options) instancetest.InstanceHarness {
		db, _ := newTestDB(t, "", opts)
		return instancetest.InstanceHarness{UnitOfWork: db, Instance: db.InstanceRepository()}
	})
}

func TestRelayContract(t *testing.T) {
	t.Parallel()
	federationtest.RelayContract(t, func(t *testing.T, opts uow.Options) federationtest.RelayHarness {
		db, _ := newTestDB(t, "", opts)
		return federationtest.RelayHarness{UnitOfWork: db, Relays: db.FederationRepository()}
	})
}

func TestFederationRepositoryContract(t *testing.T) {
	t.Parallel()
	federationtest.FederationRepositoryContract(t, func(t *testing.T, opts uow.Options) federationtest.FederationHarness {
		db, _ := newTestDB(t, "", opts)
		return federationtest.FederationHarness{UnitOfWork: db, Federation: db.FederationRepository()}
	})
}

func TestInboxContract(t *testing.T) {
	t.Parallel()
	federationtest.InboxContract(t, func(t *testing.T, opts uow.Options) federationtest.InboxHarness {
		db, _ := newTestDB(t, "", opts)
		feds := db.FederationRepository()
		return federationtest.InboxHarness{UnitOfWork: db, Inbox: feds, Federation: feds, Instances: feds, Accounts: db.AccountRepository()}
	})
}

func TestInboxProcessingContract(t *testing.T) {
	t.Parallel()
	federationtest.InboxProcessingContract(t, func(t *testing.T, opts uow.Options) federationtest.InboxHarness {
		db, _ := newTestDB(t, "", opts)
		feds := db.FederationRepository()
		return federationtest.InboxHarness{
			UnitOfWork: db, Inbox: feds, Federation: feds, Instances: feds, Accounts: db.AccountRepository(), Statuses: db.StatusRepository(),
			Graph: db.GraphRepository(), Moderation: db.ModerationRepository(),
		}
	})
}

func TestAdminAccountContract(t *testing.T) {
	t.Parallel()
	identitytest.AdminAccountContract(t, func(t *testing.T, opts uow.Options) identitytest.AdminAccountHarness {
		db, _ := newTestDB(t, "", opts)
		return identitytest.AdminAccountHarness{
			UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository(), Moderation: db.ModerationRepository(),
		}
	})
}

func TestAccountSearchContract(t *testing.T) {
	t.Parallel()
	identitytest.AccountSearchContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountSearchHarness {
		db, _ := newTestDB(t, "", opts)
		return identitytest.AccountSearchHarness{
			UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository(),
			Graph: db.GraphRepository(), Moderation: db.ModerationRepository(),
		}
	})
}

// TestAccountSearchUsesTermIndex checks that the range of the prefix search (SearchAccountTerms
// in queries/sqlite/account/account_search.sql) reads account_search_terms through its term index, in
// order, without sorting (ADR-0092 §1).
func TestAccountSearchUsesTermIndex(t *testing.T) {
	t.Parallel()
	db, _ := newTestDB(t, "", uow.Options{})
	rows, err := db.db.QueryContext(t.Context(), `EXPLAIN QUERY PLAN
SELECT t.account_id, t.kind, t.term, t.is_local, a.domain
FROM account_search_terms t JOIN accounts a ON a.id = t.account_id
WHERE t.term >= ? AND t.term < ?
ORDER BY t.term, t.account_id
LIMIT ?`, "al", "al\U0010FFFF", 100)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan, "\n")
	t.Logf("plan:\n%s", joined)
	if !strings.Contains(joined, "USING INDEX account_search_terms_term_account_id (term>? AND term<?)") || strings.Contains(joined, "TEMP B-TREE") {
		t.Errorf("plan =\n%s\nwant a range of account_search_terms_term_account_id without sorting", joined)
	}
}

func TestModerationContract(t *testing.T) {
	t.Parallel()
	moderationtest.ModerationContract(t, func(t *testing.T, opts uow.Options) moderationtest.ModerationHarness {
		db, _ := newTestDB(t, "", opts)
		return moderationtest.ModerationHarness{UnitOfWork: db, Moderation: db.ModerationRepository()}
	})
}

func TestRemoteStatusContract(t *testing.T) {
	t.Parallel()
	federationtest.RemoteStatusContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteStatusHarness {
		db, _ := newTestDB(t, "", opts)
		return federationtest.RemoteStatusHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
			Statuses:       db.StatusRepository(),
			Media:          db.MediaRepository(),
			Timeline:       db.TimelineRepository(),
			Notifications:  db.NotificationRepository(),
			Inbox:          db.FederationRepository(),
			Federation:     db.FederationRepository(),
			Instances:      db.FederationRepository(),
			Pins:           db.InteractionRepository(),
			Quotes:         db.StatusRepository(),
		}
	})
}

func TestRetentionContract(t *testing.T) {
	t.Parallel()
	federationtest.RetentionContract(t, func(t *testing.T, opts uow.Options) federationtest.RetentionHarness {
		db, _ := newTestDB(t, "", opts)
		return federationtest.RetentionHarness{
			RemoteStatusHarness: federationtest.RemoteStatusHarness{
				AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
				Graph:          db.GraphRepository(),
				Statuses:       db.StatusRepository(),
				Media:          db.MediaRepository(),
				Timeline:       db.TimelineRepository(),
				Notifications:  db.NotificationRepository(),
				Inbox:          db.FederationRepository(),
			},
			Interactions: db.InteractionRepository(),
			Federation:   db.FederationRepository(),
			Retention:    db.StatusRepository(),
			Purges:       db.FederationRepository(),
			Instances:    db.FederationRepository(),
			Profiles:     db.AccountRepository(),
			Emojis:       db.CustomEmojiRepository(),
		}
	})
}

func TestRemoteFollowContract(t *testing.T) {
	t.Parallel()
	federationtest.RemoteFollowContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteFollowHarness {
		db, _ := newTestDB(t, "", opts)
		feds := db.FederationRepository()
		return federationtest.RemoteFollowHarness{
			FollowHarness: graphtest.FollowHarness{
				AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
				Graph:          db.GraphRepository(),
			},
			Notifications: db.NotificationRepository(),
			Federation:    feds,
			Inbox:         feds,
		}
	})
}

func TestAccountMoveContract(t *testing.T) {
	t.Parallel()
	federationtest.AccountMoveContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteFollowHarness {
		db, _ := newTestDB(t, "", opts)
		feds := db.FederationRepository()
		return federationtest.RemoteFollowHarness{
			FollowHarness: graphtest.FollowHarness{
				AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
				Graph:          db.GraphRepository(),
			},
			Notifications: db.NotificationRepository(),
			Federation:    feds,
			Inbox:         feds,
		}
	})
}

func TestSeveranceContract(t *testing.T) {
	t.Parallel()
	notificationtest.SeveranceContract(t, func(t *testing.T, opts uow.Options) notificationtest.SeveranceHarness {
		db, _ := newTestDB(t, "", opts)
		feds := db.FederationRepository()
		return notificationtest.SeveranceHarness{
			RemoteFollowHarness: federationtest.RemoteFollowHarness{
				FollowHarness: graphtest.FollowHarness{
					AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
					Graph:          db.GraphRepository(),
				},
				Notifications: db.NotificationRepository(),
				Federation:    feds,
				Inbox:         feds,
			},
			Moderation: db.ModerationRepository(),
		}
	})
}

func TestRemoteActorContract(t *testing.T) {
	t.Parallel()
	federationtest.RemoteActorContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteActorHarness {
		db, _ := newTestDB(t, "", opts)
		feds := db.FederationRepository()
		return federationtest.RemoteActorHarness{UnitOfWork: db, Accounts: db.AccountRepository(), Federation: feds, Instances: feds}
	})
}

func TestUnitOfWorkContract(t *testing.T) {
	t.Parallel()
	uowtest.UnitOfWorkContract(t, func(t *testing.T, opts uow.Options) uowtest.UnitOfWorkHarness {
		db, _ := newTestDB(t, "", opts)
		return uowtest.UnitOfWorkHarness{
			UnitOfWork: db,
			Insert: func(ctx context.Context, tx uow.Tx, name string) error {
				q, err := db.queries(tx)
				if err != nil {
					return err
				}
				return insertProbe(ctx, q, name)
			},
			Keys: func(ctx context.Context) ([]string, error) { return probeKeys(ctx, db) },
		}
	})
}

func TestOpenSetsRequiredParameters(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		dsn  string
		want []string
		not  []string
	}{
		{
			dsn:  "my.db",
			want: []string{"file:my.db?", "_txlock=immediate", "busy_timeout%285000%29", "journal_mode%28wal%29", "foreign_keys%281%29", "synchronous%28normal%29"},
		},
		{
			dsn:  "file:/var/lib/my/my.db?_txlock=deferred&_pragma=busy_timeout(100)",
			want: []string{"file:/var/lib/my/my.db?", "_txlock=immediate", "busy_timeout%28100%29"},
			not:  []string{"deferred", "busy_timeout%285000%29"},
		},
	} {
		got, err := normalizeDSN(tc.dsn)
		if err != nil {
			t.Fatalf("normalizeDSN(%q) = %v", tc.dsn, err)
		}
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("normalizeDSN(%q) = %q, missing %q", tc.dsn, got, w)
			}
		}
		for _, n := range tc.not {
			if strings.Contains(got, n) {
				t.Errorf("normalizeDSN(%q) = %q, must not contain %q", tc.dsn, got, n)
			}
		}
	}
	for _, dsn := range []string{"", "file:", ":memory:", "file:x?mode=memory"} {
		if _, err := normalizeDSN(dsn); err == nil {
			t.Errorf("normalizeDSN(%q) succeeded, want an error", dsn)
		}
	}
}

func TestConnectionsUseWAL(t *testing.T) {
	t.Parallel()
	db, _ := newTestDB(t, "", uow.Options{})
	var mode string
	if err := db.db.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}

// TestRetriesWhenTheDatabaseIsLocked holds the write lock from a second connection so that the
// UnitOfWork's BEGIN IMMEDIATE times out with SQLITE_BUSY, and releases it on the first retry.
func TestRetriesWhenTheDatabaseIsLocked(t *testing.T) {
	t.Parallel()
	var (
		retries []error
		blocker *sql.Tx
	)
	opts := uow.Options{Retry: uow.RetryPolicy{OnRetry: func(_ int, err error) {
		retries = append(retries, err)
		if blocker != nil {
			if err := blocker.Commit(); err != nil {
				t.Errorf("release the lock: %v", err)
			}
			blocker = nil
		}
	}}}
	db, path := newTestDB(t, "?_pragma=busy_timeout(10)", opts)

	other, err := Open(t.Context(), path, uow.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close() })
	if blocker, err = other.db.BeginTx(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if err := insertProbe(t.Context(), newGenQueries(blocker), testkit.NamedID("blocker")); err != nil {
		t.Fatal(err)
	}

	err = db.Tx(t.Context(), func(tx uow.Tx) error {
		q, err := db.queries(tx)
		if err != nil {
			return err
		}
		return insertProbe(t.Context(), q, testkit.NamedID("a"))
	})
	if err != nil {
		t.Fatalf("Tx = %v", err)
	}
	if len(retries) != 1 {
		t.Fatalf("retried %d times, want 1", len(retries))
	}
	var se *msqlite.Error
	if !errors.Is(retries[0], uow.ErrRetryable) || !errors.As(retries[0], &se) || se.Code()&0xff != sqlite3.SQLITE_BUSY {
		t.Errorf("retried error = %v, want a retryable SQLITE_BUSY", retries[0])
	}
	got, err := probeKeys(t.Context(), db)
	want := []string{testkit.NamedID("a"), testkit.NamedID("blocker")}
	slices.Sort(want)
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("committed keys = %q (%v), want %q", got, err, want)
	}
}

func TestRejectsForeignTransactions(t *testing.T) {
	t.Parallel()
	if _, err := (&DB{}).txOf(foreignTx{}); !errors.Is(err, ErrForeignTx) {
		t.Fatal("txOf accepted a transaction of another adapter")
	}
}

type foreignTx struct{}

func (foreignTx) Emit(context.Context, uow.Event) error { return nil }

func TestReady(t *testing.T) {
	t.Parallel()
	db, _ := newTestDB(t, "", uow.Options{})
	if err := db.Ready(t.Context()); err != nil {
		t.Fatalf("Ready on an applied schema = %v", err)
	}
	if _, err := db.db.ExecContext(t.Context(), "DROP TABLE dead_jobs"); err != nil {
		t.Fatal(err)
	}
	if err := db.Ready(t.Context()); err == nil || !strings.Contains(err.Error(), "dead_jobs") {
		t.Errorf("Ready with a missing table = %v, want an error naming dead_jobs", err)
	}

	empty, err := Open(t.Context(), filepath.Join(t.TempDir(), "empty.db"), uow.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := empty.Ready(t.Context()); err == nil {
		t.Error("Ready on an empty database = nil, want an error")
	}
	_ = empty.Close()
	if err := empty.Ready(t.Context()); err == nil {
		t.Error("Ready on a closed database = nil, want an error")
	}
}

func TestRemoteInteractionContract(t *testing.T) {
	t.Parallel()
	federationtest.RemoteInteractionContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteInteractionHarness {
		db, _ := newTestDB(t, "", opts)
		feds := db.FederationRepository()
		return federationtest.RemoteInteractionHarness{
			RemoteStatusHarness: federationtest.RemoteStatusHarness{
				AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
				Graph:          db.GraphRepository(),
				Statuses:       db.StatusRepository(),
				Media:          db.MediaRepository(),
				Timeline:       db.TimelineRepository(),
				Notifications:  db.NotificationRepository(),
				Inbox:          feds,
				Quotes:         db.StatusRepository(),
			},
			Interactions: db.InteractionRepository(),
			Federation:   feds,
		}
	})
}

func TestHookRepositoryContract(t *testing.T) {
	t.Parallel()
	hooktest.HookRepositoryContract(t, func(t *testing.T) hooktest.HookHarness {
		db, _ := newTestDB(t, "", uow.Options{})
		return hooktest.HookHarness{UnitOfWork: db, Repository: db.HookRepository()}
	})
}

func TestDiagnosticsRepositoryContract(t *testing.T) {
	t.Parallel()
	diagnosticstest.DiagnosticsRepositoryContract(t, func(t *testing.T, opts uow.Options) diagnosticstest.DiagnosticsHarness {
		db, _ := newTestDB(t, "", opts)
		return diagnosticstest.DiagnosticsHarness{UnitOfWork: db, Diagnostics: db.DiagnosticsRepository()}
	})
}

func TestLoginProviderRepositoryContract(t *testing.T) {
	t.Parallel()
	identitytest.LoginProviderRepositoryContract(t, func(t *testing.T) identitytest.LoginProviderHarness {
		db, _ := newTestDB(t, "", uow.Options{})
		return identitytest.LoginProviderHarness{UnitOfWork: db, Identity: db.IdentityRepository()}
	})
}
