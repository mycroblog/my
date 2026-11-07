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

	"github.com/mycroblog/my/internal/adapter/db/postgres/identitygen"
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
	"github.com/mycroblog/my/pkg/testkit/pgtest"
	"github.com/mycroblog/my/pkg/testkit/searchtest"
	"github.com/mycroblog/my/pkg/testkit/supporterstest"
	"github.com/mycroblog/my/pkg/testkit/timelinetest"
	"github.com/mycroblog/my/pkg/testkit/translationtest"
	"github.com/mycroblog/my/pkg/testkit/uowtest"
	"github.com/mycroblog/my/pkg/testkit/voicetest"
	"github.com/mycroblog/my/pkg/testkit/webhooktest"
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

func insertProbe(ctx context.Context, q *genQueries, key string) error {
	return q.InsertUser(ctx, identitygen.InsertUserParams{ID: key, CreatedAt: time.UnixMilli(0)})
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
	identitytest.AccountRepositoryContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		db := newTestDB(t, opts)
		return identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
	})
}

func TestMediaRepositoryContract(t *testing.T) {
	contenttest.MediaRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.MediaHarness {
		db := newTestDB(t, opts)
		return contenttest.MediaHarness{UnitOfWork: db, Media: db.MediaRepository(), Statuses: db.StatusRepository()}
	})
}

func TestLinkCardRepositoryContract(t *testing.T) {
	contenttest.LinkCardRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.LinkCardHarness {
		db := newTestDB(t, opts)
		return contenttest.LinkCardHarness{UnitOfWork: db, Cards: db.StatusRepository(), Media: db.MediaRepository()}
	})
}

func TestTranslationRepositoryContract(t *testing.T) {
	translationtest.TranslationRepositoryContract(t, func(t *testing.T) translationtest.TranslationHarness {
		db := newTestDB(t, uow.Options{})
		return translationtest.TranslationHarness{UnitOfWork: db, Repository: db.TranslationRepository()}
	})
}

func TestSearchRepositoryContract(t *testing.T) {
	searchtest.SearchRepositoryContract(t, func(t *testing.T) searchtest.SearchHarness {
		db := newTestDB(t, uow.Options{})
		return searchtest.SearchHarness{UnitOfWork: db, Repository: db.SearchRepository()}
	})
}

func TestCustomEmojiRepositoryContract(t *testing.T) {
	contenttest.CustomEmojiRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.CustomEmojiHarness {
		db := newTestDB(t, opts)
		return contenttest.CustomEmojiHarness{
			UnitOfWork: db, Media: db.MediaRepository(), Statuses: db.StatusRepository(), Emojis: db.CustomEmojiRepository(),
		}
	})
}

func TestWebhookRepositoryContract(t *testing.T) {
	webhooktest.WebhookRepositoryContract(t, func(t *testing.T, opts uow.Options) webhooktest.WebhookHarness {
		db := newTestDB(t, opts)
		return webhooktest.WebhookHarness{UnitOfWork: db, Webhooks: db.WebhookRepository(), Moderation: db.ModerationRepository()}
	})
}

func TestFinanceRepositoryContract(t *testing.T) {
	financetest.FinanceRepositoryContract(t, func(t *testing.T, opts uow.Options) financetest.FinanceHarness {
		db := newTestDB(t, opts)
		return financetest.FinanceHarness{UnitOfWork: db, Finance: db.FinanceRepository()}
	})
}

func TestVoiceRepositoryContract(t *testing.T) {
	voicetest.VoiceRepositoryContract(t, func(t *testing.T, opts uow.Options) voicetest.VoiceHarness {
		db := newTestDB(t, opts)
		return voicetest.VoiceHarness{UnitOfWork: db, Voice: db.VoiceRepository()}
	})
}

func TestVoiceRoomsContract(t *testing.T) {
	voicetest.VoiceRoomsContract(t, func(t *testing.T, opts uow.Options) voicetest.VoiceHarness {
		db := newTestDB(t, opts)
		return voicetest.VoiceHarness{UnitOfWork: db, Voice: db.VoiceRepository()}
	})
}

// newPaymentWorld is the provider's side of PaymentRepositoryContract: the fake adapter.
func newPaymentWorld(o supporterstest.PaymentWorldOptions) supporterstest.PaymentWorld {
	return fake.New(fake.Options{Name: o.Name, Mode: o.Mode, Now: o.Now, PageSize: o.PageSize})
}

func TestSupporterRepositoryContract(t *testing.T) {
	supporterstest.SupporterRepositoryContract(t, func(t *testing.T, opts uow.Options) supporterstest.SupporterHarness {
		db := newTestDB(t, opts)
		return supporterstest.SupporterHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Supporters:     db.SupporterRepository(), NewWorld: newPaymentWorld,
		}
	})
}

func TestPaymentRepositoryContract(t *testing.T) {
	supporterstest.PaymentRepositoryContract(t, func(t *testing.T, opts uow.Options) supporterstest.PaymentHarness {
		db := newTestDB(t, opts)
		return supporterstest.PaymentHarness{UnitOfWork: db, Payments: db.PaymentRepository(), NewWorld: newPaymentWorld}
	})
}

func TestStatusRepositoryContract(t *testing.T) {
	contenttest.StatusRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.StatusHarness {
		db := newTestDB(t, opts)
		return contenttest.StatusHarness{UnitOfWork: db, Statuses: db.StatusRepository()}
	})
}

func TestInteractionRepositoryContract(t *testing.T) {
	contenttest.InteractionRepositoryContract(t, func(t *testing.T, opts uow.Options) contenttest.InteractionHarness {
		db := newTestDB(t, opts)
		return contenttest.InteractionHarness{UnitOfWork: db, Statuses: db.StatusRepository(), Interactions: db.InteractionRepository()}
	})
}

func TestAutoDeleteContract(t *testing.T) {
	contenttest.AutoDeleteContract(t, func(t *testing.T, opts uow.Options) contenttest.AutoDeleteHarness {
		db := newTestDB(t, opts)
		return contenttest.AutoDeleteHarness{
			InteractionHarness: contenttest.InteractionHarness{UnitOfWork: db, Statuses: db.StatusRepository(), Interactions: db.InteractionRepository()},
			AutoDeletes:        db.StatusRepository(),
		}
	})
}

func TestTimelineContract(t *testing.T) {
	timelinetest.TimelineContract(t, func(t *testing.T, opts uow.Options) timelinetest.TimelineHarness {
		db := newTestDB(t, opts)
		return timelinetest.TimelineHarness{
			UnitOfWork: db, Statuses: db.StatusRepository(), Interactions: db.InteractionRepository(),
			Graph: db.GraphRepository(), Timeline: db.TimelineRepository(),
		}
	})
}

func TestNotificationContract(t *testing.T) {
	notificationtest.NotificationContract(t, func(t *testing.T, opts uow.Options) notificationtest.NotificationHarness {
		db := newTestDB(t, opts)
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
	notificationtest.PushSubscriptionContract(t, func(t *testing.T) notificationtest.PushHarness {
		db := newTestDB(t, uow.Options{})
		return notificationtest.PushHarness{UnitOfWork: db, Push: db.NotificationRepository()}
	})
}

func TestFollowRepositoryContract(t *testing.T) {
	graphtest.FollowRepositoryContract(t, func(t *testing.T, opts uow.Options) graphtest.FollowHarness {
		db := newTestDB(t, opts)
		return graphtest.FollowHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
		}
	})
}

func TestPostingContract(t *testing.T) {
	contenttest.PostingContract(t, func(t *testing.T, opts uow.Options) contenttest.PostingHarness {
		db := newTestDB(t, opts)
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
	graphtest.BlockRepositoryContract(t, func(t *testing.T, opts uow.Options) graphtest.FollowHarness {
		db := newTestDB(t, opts)
		return graphtest.FollowHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
		}
	})
}

func TestPreferencesRepositoryContract(t *testing.T) {
	identitytest.PreferencesRepositoryContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		db := newTestDB(t, opts)
		return identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
	})
}

func TestRegistrationRepositoryContract(t *testing.T) {
	identitytest.RegistrationRepositoryContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		db := newTestDB(t, opts)
		return identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
	})
}

func TestRoleRepositoryContract(t *testing.T) {
	identitytest.RoleRepositoryContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		db := newTestDB(t, opts)
		return identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
	})
}

func TestInstanceRepositoryContract(t *testing.T) {
	instancetest.InstanceRepositoryContract(t, func(t *testing.T, opts uow.Options) instancetest.InstanceHarness {
		db := newTestDB(t, opts)
		return instancetest.InstanceHarness{UnitOfWork: db, Instance: db.InstanceRepository()}
	})
}

func TestRelayContract(t *testing.T) {
	federationtest.RelayContract(t, func(t *testing.T, opts uow.Options) federationtest.RelayHarness {
		db := newTestDB(t, opts)
		return federationtest.RelayHarness{UnitOfWork: db, Relays: db.FederationRepository()}
	})
}

func TestFederationRepositoryContract(t *testing.T) {
	federationtest.FederationRepositoryContract(t, func(t *testing.T, opts uow.Options) federationtest.FederationHarness {
		db := newTestDB(t, opts)
		return federationtest.FederationHarness{UnitOfWork: db, Federation: db.FederationRepository()}
	})
}

func TestInboxContract(t *testing.T) {
	federationtest.InboxContract(t, func(t *testing.T, opts uow.Options) federationtest.InboxHarness {
		db := newTestDB(t, opts)
		feds := db.FederationRepository()
		return federationtest.InboxHarness{UnitOfWork: db, Inbox: feds, Federation: feds, Instances: feds, Accounts: db.AccountRepository()}
	})
}

func TestInboxProcessingContract(t *testing.T) {
	federationtest.InboxProcessingContract(t, func(t *testing.T, opts uow.Options) federationtest.InboxHarness {
		db := newTestDB(t, opts)
		feds := db.FederationRepository()
		return federationtest.InboxHarness{
			UnitOfWork: db, Inbox: feds, Federation: feds, Instances: feds, Accounts: db.AccountRepository(), Statuses: db.StatusRepository(),
			Graph: db.GraphRepository(), Moderation: db.ModerationRepository(),
		}
	})
}

func TestAdminAccountContract(t *testing.T) {
	identitytest.AdminAccountContract(t, func(t *testing.T, opts uow.Options) identitytest.AdminAccountHarness {
		db := newTestDB(t, opts)
		return identitytest.AdminAccountHarness{
			UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository(), Moderation: db.ModerationRepository(),
		}
	})
}

func TestAccountSearchContract(t *testing.T) {
	identitytest.AccountSearchContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountSearchHarness {
		db := newTestDB(t, opts)
		return identitytest.AccountSearchHarness{
			UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository(),
			Graph: db.GraphRepository(), Moderation: db.ModerationRepository(),
		}
	})
}

// TestAccountSearchUsesTermIndex checks that the range of the prefix search (SearchAccountTerms
// in queries/postgres/account/account_search.sql) can read account_search_terms through its term index
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
	moderationtest.ModerationContract(t, func(t *testing.T, opts uow.Options) moderationtest.ModerationHarness {
		db := newTestDB(t, opts)
		return moderationtest.ModerationHarness{UnitOfWork: db, Moderation: db.ModerationRepository()}
	})
}

func TestRemoteStatusContract(t *testing.T) {
	federationtest.RemoteStatusContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteStatusHarness {
		db := newTestDB(t, opts)
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
	federationtest.RetentionContract(t, func(t *testing.T, opts uow.Options) federationtest.RetentionHarness {
		db := newTestDB(t, opts)
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
	federationtest.RemoteFollowContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteFollowHarness {
		db := newTestDB(t, opts)
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
	federationtest.AccountMoveContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteFollowHarness {
		db := newTestDB(t, opts)
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
	notificationtest.SeveranceContract(t, func(t *testing.T, opts uow.Options) notificationtest.SeveranceHarness {
		db := newTestDB(t, opts)
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
	federationtest.RemoteActorContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteActorHarness {
		db := newTestDB(t, opts)
		feds := db.FederationRepository()
		return federationtest.RemoteActorHarness{UnitOfWork: db, Accounts: db.AccountRepository(), Federation: feds, Instances: feds}
	})
}

func TestUnitOfWorkContract(t *testing.T) {
	uowtest.UnitOfWorkContract(t, func(t *testing.T, opts uow.Options) uowtest.UnitOfWorkHarness {
		db := newTestDB(t, opts)
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

func TestTransactionsAreReadCommitted(t *testing.T) {
	db := newTestDB(t, uow.Options{})
	err := db.Tx(t.Context(), func(tx uow.Tx) error {
		pgTx, err := db.txOf(tx)
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
	setup := newGenQueries(db.pool)
	x, y := testkit.NamedID("x"), testkit.NamedID("y")
	for _, name := range []string{x, y} {
		if err := insertProbe(t.Context(), setup, name); err != nil {
			t.Fatal(err)
		}
	}

	var bothLocked sync.WaitGroup
	bothLocked.Add(2)
	lock := func(tx uow.Tx, key string) error {
		pgTx, err := db.txOf(tx)
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
			q, err := db.queries(tx)
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
	if _, err := (&DB{}).txOf(foreignTx{}); !errors.Is(err, ErrForeignTx) {
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
	federationtest.RemoteInteractionContract(t, func(t *testing.T, opts uow.Options) federationtest.RemoteInteractionHarness {
		db := newTestDB(t, opts)
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
	hooktest.HookRepositoryContract(t, func(t *testing.T) hooktest.HookHarness {
		db := newTestDB(t, uow.Options{})
		return hooktest.HookHarness{UnitOfWork: db, Repository: db.HookRepository()}
	})
}

func TestDiagnosticsRepositoryContract(t *testing.T) {
	diagnosticstest.DiagnosticsRepositoryContract(t, func(t *testing.T, opts uow.Options) diagnosticstest.DiagnosticsHarness {
		db := newTestDB(t, opts)
		return diagnosticstest.DiagnosticsHarness{UnitOfWork: db, Diagnostics: db.DiagnosticsRepository()}
	})
}

func TestLoginProviderRepositoryContract(t *testing.T) {
	identitytest.LoginProviderRepositoryContract(t, func(t *testing.T) identitytest.LoginProviderHarness {
		db := newTestDB(t, uow.Options{})
		return identitytest.LoginProviderHarness{UnitOfWork: db, Identity: db.IdentityRepository()}
	})
}
