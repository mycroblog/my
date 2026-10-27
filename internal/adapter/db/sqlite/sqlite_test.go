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

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	"github.com/mycroblog/my/internal/adapter/payment/fake"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/schema"
)

// newTestDB opens a new database file under t.TempDir() with the product schema applied
// (see package schema for why executing the file matches an Atlas apply).
func newTestDB(t *testing.T, dsnSuffix string, opts uow.Options) (*DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(t.Context(), path+dsnSuffix, opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.db.ExecContext(t.Context(), schema.SQLite); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return db, path
}

// The UnitOfWork contract uses users as its probe table: a row per key, keyed by id.

func insertProbe(ctx context.Context, q *sqlitegen.Queries, key string) error {
	return q.InsertUser(ctx, sqlitegen.InsertUserParams{ID: key})
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
	testkit.AccountRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.AccountHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
	})
}

func TestMediaRepositoryContract(t *testing.T) {
	testkit.MediaRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.MediaHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.MediaHarness{UnitOfWork: db, Media: db.MediaRepository(), Statuses: db.StatusRepository()}
	})
}

func TestLinkCardRepositoryContract(t *testing.T) {
	testkit.LinkCardRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.LinkCardHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.LinkCardHarness{UnitOfWork: db, Cards: db.StatusRepository(), Media: db.MediaRepository()}
	})
}

func TestCustomEmojiRepositoryContract(t *testing.T) {
	testkit.CustomEmojiRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.CustomEmojiHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.CustomEmojiHarness{
			UnitOfWork: db, Media: db.MediaRepository(), Statuses: db.StatusRepository(), Emojis: db.CustomEmojiRepository(),
		}
	})
}

func TestWebhookRepositoryContract(t *testing.T) {
	testkit.WebhookRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.WebhookHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.WebhookHarness{UnitOfWork: db, Webhooks: db.WebhookRepository(), Moderation: db.ModerationRepository()}
	})
}

func TestFinanceRepositoryContract(t *testing.T) {
	testkit.FinanceRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.FinanceHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.FinanceHarness{UnitOfWork: db, Finance: db.FinanceRepository()}
	})
}

func TestVoiceRepositoryContract(t *testing.T) {
	testkit.VoiceRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.VoiceHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.VoiceHarness{UnitOfWork: db, Voice: db.VoiceRepository()}
	})
}

func TestVoiceRoomsContract(t *testing.T) {
	testkit.VoiceRoomsContract(t, func(t *testing.T, opts uow.Options) testkit.VoiceHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.VoiceHarness{UnitOfWork: db, Voice: db.VoiceRepository()}
	})
}

// newPaymentWorld is the provider's side of PaymentRepositoryContract: the fake adapter.
func newPaymentWorld(o testkit.PaymentWorldOptions) testkit.PaymentWorld {
	return fake.New(fake.Options{Name: o.Name, Mode: o.Mode, Now: o.Now, PageSize: o.PageSize})
}

func TestSupporterRepositoryContract(t *testing.T) {
	testkit.SupporterRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.SupporterHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.SupporterHarness{
			AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Supporters:     db.SupporterRepository(), NewWorld: newPaymentWorld,
		}
	})
}

func TestPaymentRepositoryContract(t *testing.T) {
	testkit.PaymentRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.PaymentHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.PaymentHarness{UnitOfWork: db, Payments: db.PaymentRepository(), NewWorld: newPaymentWorld}
	})
}

func TestStatusRepositoryContract(t *testing.T) {
	testkit.StatusRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.StatusHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.StatusHarness{UnitOfWork: db, Statuses: db.StatusRepository()}
	})
}

func TestInteractionRepositoryContract(t *testing.T) {
	testkit.InteractionRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.InteractionHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.InteractionHarness{UnitOfWork: db, Statuses: db.StatusRepository(), Interactions: db.InteractionRepository()}
	})
}

func TestAutoDeleteContract(t *testing.T) {
	testkit.AutoDeleteContract(t, func(t *testing.T, opts uow.Options) testkit.AutoDeleteHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.AutoDeleteHarness{
			InteractionHarness: testkit.InteractionHarness{UnitOfWork: db, Statuses: db.StatusRepository(), Interactions: db.InteractionRepository()},
			AutoDeletes:        db.StatusRepository(),
		}
	})
}

func TestTimelineContract(t *testing.T) {
	testkit.TimelineContract(t, func(t *testing.T, opts uow.Options) testkit.TimelineHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.TimelineHarness{
			UnitOfWork: db, Statuses: db.StatusRepository(), Interactions: db.InteractionRepository(),
			Graph: db.GraphRepository(), Timeline: db.TimelineRepository(),
		}
	})
}

func TestNotificationContract(t *testing.T) {
	testkit.NotificationContract(t, func(t *testing.T, opts uow.Options) testkit.NotificationHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.NotificationHarness{
			AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
			Statuses:       db.StatusRepository(),
			Interactions:   db.InteractionRepository(),
			Notifications:  db.NotificationRepository(),
		}
	})
}

func TestPushSubscriptionContract(t *testing.T) {
	testkit.PushSubscriptionContract(t, func(t *testing.T) testkit.PushHarness {
		db, _ := newTestDB(t, "", uow.Options{})
		return testkit.PushHarness{UnitOfWork: db, Push: db.NotificationRepository()}
	})
}

func TestFollowRepositoryContract(t *testing.T) {
	testkit.FollowRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.FollowHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.FollowHarness{
			AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
		}
	})
}

func TestPostingContract(t *testing.T) {
	testkit.PostingContract(t, func(t *testing.T, opts uow.Options) testkit.PostingHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.PostingHarness{
			AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
			Instance:       db.InstanceRepository(),
			Statuses:       db.StatusRepository(),
			Media:          db.MediaRepository(),
		}
	})
}

func TestBlockRepositoryContract(t *testing.T) {
	testkit.BlockRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.FollowHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.FollowHarness{
			AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
		}
	})
}

func TestPreferencesRepositoryContract(t *testing.T) {
	testkit.PreferencesRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.AccountHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
	})
}

func TestRegistrationRepositoryContract(t *testing.T) {
	testkit.RegistrationRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.AccountHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
	})
}

func TestRoleRepositoryContract(t *testing.T) {
	testkit.RoleRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.AccountHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
	})
}

func TestInstanceRepositoryContract(t *testing.T) {
	testkit.InstanceRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.InstanceHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.InstanceHarness{UnitOfWork: db, Instance: db.InstanceRepository()}
	})
}

func TestRelayContract(t *testing.T) {
	testkit.RelayContract(t, func(t *testing.T, opts uow.Options) testkit.RelayHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.RelayHarness{UnitOfWork: db, Relays: db.FederationRepository()}
	})
}

func TestFederationRepositoryContract(t *testing.T) {
	testkit.FederationRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.FederationHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.FederationHarness{UnitOfWork: db, Federation: db.FederationRepository()}
	})
}

func TestInboxContract(t *testing.T) {
	testkit.InboxContract(t, func(t *testing.T, opts uow.Options) testkit.InboxHarness {
		db, _ := newTestDB(t, "", opts)
		feds := db.FederationRepository()
		return testkit.InboxHarness{UnitOfWork: db, Inbox: feds, Federation: feds, Instances: feds, Accounts: db.AccountRepository()}
	})
}

func TestInboxProcessingContract(t *testing.T) {
	testkit.InboxProcessingContract(t, func(t *testing.T, opts uow.Options) testkit.InboxHarness {
		db, _ := newTestDB(t, "", opts)
		feds := db.FederationRepository()
		return testkit.InboxHarness{
			UnitOfWork: db, Inbox: feds, Federation: feds, Instances: feds, Accounts: db.AccountRepository(), Statuses: db.StatusRepository(),
			Graph: db.GraphRepository(), Moderation: db.ModerationRepository(),
		}
	})
}

func TestAdminAccountContract(t *testing.T) {
	testkit.AdminAccountContract(t, func(t *testing.T, opts uow.Options) testkit.AdminAccountHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.AdminAccountHarness{
			UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository(), Moderation: db.ModerationRepository(),
		}
	})
}

func TestAccountSearchContract(t *testing.T) {
	testkit.AccountSearchContract(t, func(t *testing.T, opts uow.Options) testkit.AccountSearchHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.AccountSearchHarness{
			UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository(),
			Graph: db.GraphRepository(), Moderation: db.ModerationRepository(),
		}
	})
}

// TestAccountSearchUsesTermIndex checks that the range of the prefix search (SearchAccountTerms
// in queries/sqlite/account_search.sql) reads account_search_terms through its term index, in
// order, without sorting (ADR-0092 §1).
func TestAccountSearchUsesTermIndex(t *testing.T) {
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
	testkit.ModerationContract(t, func(t *testing.T, opts uow.Options) testkit.ModerationHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.ModerationHarness{UnitOfWork: db, Moderation: db.ModerationRepository()}
	})
}

func TestRemoteStatusContract(t *testing.T) {
	testkit.RemoteStatusContract(t, func(t *testing.T, opts uow.Options) testkit.RemoteStatusHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.RemoteStatusHarness{
			AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
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
	testkit.RetentionContract(t, func(t *testing.T, opts uow.Options) testkit.RetentionHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.RetentionHarness{
			RemoteStatusHarness: testkit.RemoteStatusHarness{
				AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
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
			Profiles:     db.AccountRepository(),
			Emojis:       db.CustomEmojiRepository(),
		}
	})
}

func TestRemoteFollowContract(t *testing.T) {
	testkit.RemoteFollowContract(t, func(t *testing.T, opts uow.Options) testkit.RemoteFollowHarness {
		db, _ := newTestDB(t, "", opts)
		feds := db.FederationRepository()
		return testkit.RemoteFollowHarness{
			FollowHarness: testkit.FollowHarness{
				AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
				Graph:          db.GraphRepository(),
			},
			Notifications: db.NotificationRepository(),
			Federation:    feds,
			Inbox:         feds,
		}
	})
}

func TestAccountMoveContract(t *testing.T) {
	testkit.AccountMoveContract(t, func(t *testing.T, opts uow.Options) testkit.RemoteFollowHarness {
		db, _ := newTestDB(t, "", opts)
		feds := db.FederationRepository()
		return testkit.RemoteFollowHarness{
			FollowHarness: testkit.FollowHarness{
				AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
				Graph:          db.GraphRepository(),
			},
			Notifications: db.NotificationRepository(),
			Federation:    feds,
			Inbox:         feds,
		}
	})
}

func TestSeveranceContract(t *testing.T) {
	testkit.SeveranceContract(t, func(t *testing.T, opts uow.Options) testkit.SeveranceHarness {
		db, _ := newTestDB(t, "", opts)
		feds := db.FederationRepository()
		return testkit.SeveranceHarness{
			RemoteFollowHarness: testkit.RemoteFollowHarness{
				FollowHarness: testkit.FollowHarness{
					AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
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
	testkit.RemoteActorContract(t, func(t *testing.T, opts uow.Options) testkit.RemoteActorHarness {
		db, _ := newTestDB(t, "", opts)
		feds := db.FederationRepository()
		return testkit.RemoteActorHarness{UnitOfWork: db, Accounts: db.AccountRepository(), Federation: feds, Instances: feds}
	})
}

func TestUnitOfWorkContract(t *testing.T) {
	testkit.UnitOfWorkContract(t, func(t *testing.T, opts uow.Options) testkit.UnitOfWorkHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.UnitOfWorkHarness{
			UnitOfWork: db,
			Insert: func(ctx context.Context, tx uow.Tx, name string) error {
				q, err := queries(tx)
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
	if err := insertProbe(t.Context(), sqlitegen.New(blocker), testkit.NamedID("blocker")); err != nil {
		t.Fatal(err)
	}

	err = db.Tx(t.Context(), func(tx uow.Tx) error {
		q, err := queries(tx)
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
	if _, err := txOf(foreignTx{}); err == nil {
		t.Fatal("txOf accepted a transaction of another adapter")
	}
}

type foreignTx struct{}

func (foreignTx) Emit(context.Context, uow.Event) error { return nil }

func TestReady(t *testing.T) {
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
	testkit.RemoteInteractionContract(t, func(t *testing.T, opts uow.Options) testkit.RemoteInteractionHarness {
		db, _ := newTestDB(t, "", opts)
		feds := db.FederationRepository()
		return testkit.RemoteInteractionHarness{
			RemoteStatusHarness: testkit.RemoteStatusHarness{
				AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
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

func TestDiagnosticsRepositoryContract(t *testing.T) {
	testkit.DiagnosticsRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.DiagnosticsHarness {
		db, _ := newTestDB(t, "", opts)
		return testkit.DiagnosticsHarness{UnitOfWork: db, Diagnostics: db.DiagnosticsRepository()}
	})
}
