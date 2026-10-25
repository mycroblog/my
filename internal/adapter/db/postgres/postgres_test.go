// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	"github.com/mycroblog/my/internal/adapter/payment/fake"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/pgtest"
)

func TestMain(m *testing.M) {
	os.Exit(pgtest.Main(m))
}

// newTestDB opens a new database with the product schema on the test server (package pgtest).
func newTestDB(t *testing.T, opts uow.Options) *DB {
	t.Helper()
	db, err := Open(t.Context(), pgtest.NewDatabase(t), opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

// The UnitOfWork contract uses users as its probe table: a row per key, keyed by id.

func insertProbe(ctx context.Context, q *postgresgen.Queries, key string) error {
	return q.InsertUser(ctx, postgresgen.InsertUserParams{ID: key, CreatedAt: time.UnixMilli(0)})
}

func probeKeys(ctx context.Context, db *DB) ([]string, error) {
	rows, err := db.pool.Query(ctx, "SELECT id FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if keys == nil {
		keys = []string{}
	}
	return keys, err
}

func TestAccountRepositoryContract(t *testing.T) {
	testkit.AccountRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.AccountHarness {
		db := newTestDB(t, opts)
		return testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
	})
}

func TestMediaRepositoryContract(t *testing.T) {
	testkit.MediaRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.MediaHarness {
		db := newTestDB(t, opts)
		return testkit.MediaHarness{UnitOfWork: db, Media: db.MediaRepository(), Statuses: db.StatusRepository()}
	})
}

func TestLinkCardRepositoryContract(t *testing.T) {
	testkit.LinkCardRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.LinkCardHarness {
		db := newTestDB(t, opts)
		return testkit.LinkCardHarness{UnitOfWork: db, Cards: db.StatusRepository(), Media: db.MediaRepository()}
	})
}

func TestCustomEmojiRepositoryContract(t *testing.T) {
	testkit.CustomEmojiRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.CustomEmojiHarness {
		db := newTestDB(t, opts)
		return testkit.CustomEmojiHarness{
			UnitOfWork: db, Media: db.MediaRepository(), Statuses: db.StatusRepository(), Emojis: db.CustomEmojiRepository(),
		}
	})
}

func TestWebhookRepositoryContract(t *testing.T) {
	testkit.WebhookRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.WebhookHarness {
		db := newTestDB(t, opts)
		return testkit.WebhookHarness{UnitOfWork: db, Webhooks: db.WebhookRepository(), Moderation: db.ModerationRepository()}
	})
}

func TestFinanceRepositoryContract(t *testing.T) {
	testkit.FinanceRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.FinanceHarness {
		db := newTestDB(t, opts)
		return testkit.FinanceHarness{UnitOfWork: db, Finance: db.FinanceRepository()}
	})
}

// newPaymentWorld is the provider's side of PaymentRepositoryContract: the fake adapter.
func newPaymentWorld(o testkit.PaymentWorldOptions) testkit.PaymentWorld {
	return fake.New(fake.Options{Name: o.Name, Mode: o.Mode, Now: o.Now, PageSize: o.PageSize})
}

func TestSupporterRepositoryContract(t *testing.T) {
	testkit.SupporterRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.SupporterHarness {
		db := newTestDB(t, opts)
		return testkit.SupporterHarness{
			AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Supporters:     db.SupporterRepository(), NewWorld: newPaymentWorld,
		}
	})
}

func TestPaymentRepositoryContract(t *testing.T) {
	testkit.PaymentRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.PaymentHarness {
		db := newTestDB(t, opts)
		return testkit.PaymentHarness{UnitOfWork: db, Payments: db.PaymentRepository(), NewWorld: newPaymentWorld}
	})
}

func TestStatusRepositoryContract(t *testing.T) {
	testkit.StatusRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.StatusHarness {
		db := newTestDB(t, opts)
		return testkit.StatusHarness{UnitOfWork: db, Statuses: db.StatusRepository()}
	})
}

func TestInteractionRepositoryContract(t *testing.T) {
	testkit.InteractionRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.InteractionHarness {
		db := newTestDB(t, opts)
		return testkit.InteractionHarness{UnitOfWork: db, Statuses: db.StatusRepository(), Interactions: db.InteractionRepository()}
	})
}

func TestAutoDeleteContract(t *testing.T) {
	testkit.AutoDeleteContract(t, func(t *testing.T, opts uow.Options) testkit.AutoDeleteHarness {
		db := newTestDB(t, opts)
		return testkit.AutoDeleteHarness{
			InteractionHarness: testkit.InteractionHarness{UnitOfWork: db, Statuses: db.StatusRepository(), Interactions: db.InteractionRepository()},
			AutoDeletes:        db.StatusRepository(),
		}
	})
}

func TestTimelineContract(t *testing.T) {
	testkit.TimelineContract(t, func(t *testing.T, opts uow.Options) testkit.TimelineHarness {
		db := newTestDB(t, opts)
		return testkit.TimelineHarness{
			UnitOfWork: db, Statuses: db.StatusRepository(), Interactions: db.InteractionRepository(),
			Graph: db.GraphRepository(), Timeline: db.TimelineRepository(),
		}
	})
}

func TestNotificationContract(t *testing.T) {
	testkit.NotificationContract(t, func(t *testing.T, opts uow.Options) testkit.NotificationHarness {
		db := newTestDB(t, opts)
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
		db := newTestDB(t, uow.Options{})
		return testkit.PushHarness{UnitOfWork: db, Push: db.NotificationRepository()}
	})
}

func TestFollowRepositoryContract(t *testing.T) {
	testkit.FollowRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.FollowHarness {
		db := newTestDB(t, opts)
		return testkit.FollowHarness{
			AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
		}
	})
}

func TestPostingContract(t *testing.T) {
	testkit.PostingContract(t, func(t *testing.T, opts uow.Options) testkit.PostingHarness {
		db := newTestDB(t, opts)
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
		db := newTestDB(t, opts)
		return testkit.FollowHarness{
			AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
		}
	})
}

func TestPreferencesRepositoryContract(t *testing.T) {
	testkit.PreferencesRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.AccountHarness {
		db := newTestDB(t, opts)
		return testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
	})
}

func TestRegistrationRepositoryContract(t *testing.T) {
	testkit.RegistrationRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.AccountHarness {
		db := newTestDB(t, opts)
		return testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
	})
}

func TestRoleRepositoryContract(t *testing.T) {
	testkit.RoleRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.AccountHarness {
		db := newTestDB(t, opts)
		return testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
	})
}

func TestInstanceRepositoryContract(t *testing.T) {
	testkit.InstanceRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.InstanceHarness {
		db := newTestDB(t, opts)
		return testkit.InstanceHarness{UnitOfWork: db, Instance: db.InstanceRepository()}
	})
}

func TestRelayContract(t *testing.T) {
	testkit.RelayContract(t, func(t *testing.T, opts uow.Options) testkit.RelayHarness {
		db := newTestDB(t, opts)
		return testkit.RelayHarness{UnitOfWork: db, Relays: db.FederationRepository()}
	})
}

func TestFederationRepositoryContract(t *testing.T) {
	testkit.FederationRepositoryContract(t, func(t *testing.T, opts uow.Options) testkit.FederationHarness {
		db := newTestDB(t, opts)
		return testkit.FederationHarness{UnitOfWork: db, Federation: db.FederationRepository()}
	})
}

func TestInboxContract(t *testing.T) {
	testkit.InboxContract(t, func(t *testing.T, opts uow.Options) testkit.InboxHarness {
		db := newTestDB(t, opts)
		feds := db.FederationRepository()
		return testkit.InboxHarness{UnitOfWork: db, Inbox: feds, Federation: feds, Instances: feds, Accounts: db.AccountRepository()}
	})
}

func TestInboxProcessingContract(t *testing.T) {
	testkit.InboxProcessingContract(t, func(t *testing.T, opts uow.Options) testkit.InboxHarness {
		db := newTestDB(t, opts)
		feds := db.FederationRepository()
		return testkit.InboxHarness{
			UnitOfWork: db, Inbox: feds, Federation: feds, Instances: feds, Accounts: db.AccountRepository(), Statuses: db.StatusRepository(),
			Graph: db.GraphRepository(), Moderation: db.ModerationRepository(),
		}
	})
}

func TestAdminAccountContract(t *testing.T) {
	testkit.AdminAccountContract(t, func(t *testing.T, opts uow.Options) testkit.AdminAccountHarness {
		db := newTestDB(t, opts)
		return testkit.AdminAccountHarness{
			UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository(), Moderation: db.ModerationRepository(),
		}
	})
}

func TestAccountSearchContract(t *testing.T) {
	testkit.AccountSearchContract(t, func(t *testing.T, opts uow.Options) testkit.AccountSearchHarness {
		db := newTestDB(t, opts)
		return testkit.AccountSearchHarness{
			UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository(),
			Graph: db.GraphRepository(), Moderation: db.ModerationRepository(),
		}
	})
}

// TestAccountSearchUsesTermIndex checks that the range of the prefix search (SearchAccountTerms
// in queries/postgres/account_search.sql) can read account_search_terms through its term index
// in order (ADR-0092 §1): term is COLLATE "C", so the range and the order are the index's.
// Sequential and bitmap scans are off, since the planner prefers them on an empty table.
func TestAccountSearchUsesTermIndex(t *testing.T) {
	db := newTestDB(t, uow.Options{})
	ctx := t.Context()
	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SET enable_seqscan = off; SET enable_bitmapscan = off"); err != nil {
		t.Fatal(err)
	}
	rows, err := conn.Query(ctx, `EXPLAIN
SELECT t.account_id, t.kind, t.term, t.is_local, a.domain
FROM account_search_terms t JOIN accounts a ON a.id = t.account_id
WHERE t.term >= $1::text AND t.term < $2::text
ORDER BY t.term, t.account_id
LIMIT $3`, "al", "al\U0010FFFF", 100)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan, "\n")
	t.Logf("plan:\n%s", joined)
	if !strings.Contains(joined, "account_search_terms_term_account_id") || strings.Contains(joined, "Sort") {
		t.Errorf("plan =\n%s\nwant a range of account_search_terms_term_account_id without sorting", joined)
	}
}

func TestModerationContract(t *testing.T) {
	testkit.ModerationContract(t, func(t *testing.T, opts uow.Options) testkit.ModerationHarness {
		db := newTestDB(t, opts)
		return testkit.ModerationHarness{UnitOfWork: db, Moderation: db.ModerationRepository()}
	})
}

func TestRemoteStatusContract(t *testing.T) {
	testkit.RemoteStatusContract(t, func(t *testing.T, opts uow.Options) testkit.RemoteStatusHarness {
		db := newTestDB(t, opts)
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
		db := newTestDB(t, opts)
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
		db := newTestDB(t, opts)
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
		db := newTestDB(t, opts)
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
		db := newTestDB(t, opts)
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
		db := newTestDB(t, opts)
		feds := db.FederationRepository()
		return testkit.RemoteActorHarness{UnitOfWork: db, Accounts: db.AccountRepository(), Federation: feds, Instances: feds}
	})
}

func TestUnitOfWorkContract(t *testing.T) {
	testkit.UnitOfWorkContract(t, func(t *testing.T, opts uow.Options) testkit.UnitOfWorkHarness {
		db := newTestDB(t, opts)
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

func TestTransactionsAreReadCommitted(t *testing.T) {
	db := newTestDB(t, uow.Options{})
	err := db.Tx(t.Context(), func(tx uow.Tx) error {
		pgTx, err := txOf(tx)
		if err != nil {
			return err
		}
		var level string
		if err := pgTx.QueryRow(t.Context(), "SHOW transaction_isolation").Scan(&level); err != nil {
			return err
		}
		if level != "read committed" {
			t.Errorf("transaction_isolation = %q, want read committed", level)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Tx = %v", err)
	}
}

// TestRetriesDeadlocks makes two transactions lock two rows in opposite orders. PostgreSQL
// aborts one of them with deadlock_detected; the UnitOfWork must retry it to success.
func TestRetriesDeadlocks(t *testing.T) {
	var (
		mu      sync.Mutex
		retries []error
	)
	db := newTestDB(t, uow.Options{Retry: uow.RetryPolicy{OnRetry: func(_ int, err error) {
		mu.Lock()
		defer mu.Unlock()
		retries = append(retries, err)
	}}})
	setup := postgresgen.New(db.pool)
	x, y := testkit.NamedID("x"), testkit.NamedID("y")
	for _, name := range []string{x, y} {
		if err := insertProbe(t.Context(), setup, name); err != nil {
			t.Fatal(err)
		}
	}

	var bothLocked sync.WaitGroup
	bothLocked.Add(2)
	lock := func(tx uow.Tx, key string) error {
		pgTx, err := txOf(tx)
		if err != nil {
			return err
		}
		_, err = pgTx.Exec(t.Context(), "SELECT 1 FROM users WHERE id = $1 FOR UPDATE", key)
		return err
	}
	lockBoth := func(first, second string) error {
		attempt := 0
		return db.Tx(t.Context(), func(tx uow.Tx) error {
			attempt++
			if err := lock(tx, first); err != nil {
				return err
			}
			if attempt == 1 {
				bothLocked.Done()
				bothLocked.Wait()
			}
			if err := lock(tx, second); err != nil {
				return err
			}
			q, err := queries(tx)
			if err != nil {
				return err
			}
			return insertProbe(t.Context(), q, testkit.NamedID(first+"-done"))
		})
	}
	errs := make(chan error, 2)
	go func() { errs <- lockBoth(x, y) }()
	go func() { errs <- lockBoth(y, x) }()
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("Tx = %v", err)
		}
	}

	if len(retries) != 1 {
		t.Fatalf("retried %d times, want 1", len(retries))
	}
	var pgErr *pgconn.PgError
	if !errors.Is(retries[0], uow.ErrRetryable) || !errors.As(retries[0], &pgErr) || pgErr.Code != "40P01" {
		t.Errorf("retried error = %v, want a retryable deadlock_detected (40P01)", retries[0])
	}
	got, err := probeKeys(t.Context(), db)
	want := []string{x, y, testkit.NamedID(x + "-done"), testkit.NamedID(y + "-done")}
	slices.Sort(want)
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("committed keys = %q (%v), want both transactions committed", got, err)
	}
}

func TestIsRetryable(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{&pgconn.PgError{Code: "40001"}, true},
		{&pgconn.PgError{Code: "40P01"}, true},
		{fmt.Errorf("postgres: commit: %w", &pgconn.PgError{Code: "40001"}), true},
		{&pgconn.PgError{Code: "23505"}, false},
		{errors.New("40001"), false},
		{nil, false},
	} {
		if got := isRetryable(tc.err); got != tc.want {
			t.Errorf("isRetryable(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestOpenHidesTheURLInErrors(t *testing.T) {
	_, err := Open(t.Context(), "postgres://user:s3cret@host:notaport/db", uow.Options{})
	if err == nil {
		t.Fatal("Open succeeded with an invalid URL")
	}
	if got := err.Error(); strings.Contains(got, "s3cret") {
		t.Errorf("error %q leaks the password", got)
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
	db := newTestDB(t, uow.Options{})
	if err := db.Ready(t.Context()); err != nil {
		t.Fatalf("Ready on an applied schema = %v", err)
	}
	if _, err := db.pool.Exec(t.Context(), "DROP TABLE dead_jobs"); err != nil {
		t.Fatal(err)
	}
	if err := db.Ready(t.Context()); err == nil || !strings.Contains(err.Error(), "dead_jobs") {
		t.Errorf("Ready with a missing table = %v, want an error naming dead_jobs", err)
	}
	db.Close()
	if err := db.Ready(t.Context()); err == nil {
		t.Error("Ready on a closed pool = nil, want an error")
	}
}

func TestRemoteInteractionContract(t *testing.T) {
	testkit.RemoteInteractionContract(t, func(t *testing.T, opts uow.Options) testkit.RemoteInteractionHarness {
		db := newTestDB(t, opts)
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
