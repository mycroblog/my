// SPDX-License-Identifier: AGPL-3.0-only

package translation

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
)

// Stored is a row of status_translations with its child rows (ADR-0148 §9). It is a cache: it is
// never updated, and a provider's output cannot be rebuilt from anything.
type Stored struct {
	ID             string
	StatusID       string
	RevisionID     string
	TargetLanguage string
	// Provider is Provider.ID. It stays a plain string, so that adding a provider does not change
	// the schema.
	Provider       string
	SourceLanguage string
	ContentHTML    string
	SpoilerText    string
	CreatedAt      time.Time
	// Attachments are the translated alt texts, ordered by Position.
	Attachments []StoredAttachment
	// PollOptions are the translated titles, ordered by Position.
	PollOptions []StoredPollOption
}

// StoredAttachment is the translated alt text of the attachment at Position of the revision.
type StoredAttachment struct {
	Position    int
	Description string
}

// StoredPollOption is the translated title of the option at Position of the revision's poll.
type StoredPollOption struct {
	Position int
	Title    string
}

// Usage is the characters sent to providers in one month.
type Usage struct {
	Account int64
	Server  int64
}

// ServerUsage is what the whole server did in one month.
type ServerUsage struct {
	// Characters are those sent to providers.
	Characters int64
	// CacheHits counts the requests answered from a kept translation, and CacheMisses those that
	// went to a provider.
	CacheHits, CacheMisses int64
}

// Limits are the monthly characters that may be sent to providers.
type Limits struct {
	Account int64
	Server  int64
}

// Repository is the module's persistence. The database adapters implement it and pass
// translationtest.TranslationRepositoryContract.
type Repository interface {
	// InsertTranslation stores t with its child rows and reports whether it did: a translation
	// of the same revision, target language, and provider stays, with its child rows.
	InsertTranslation(ctx context.Context, tx uow.Tx, t Stored) (bool, error)
	// FindTranslations returns the translations of a revision into a language by every provider
	// that has one, with their child rows, in no particular order.
	FindTranslations(ctx context.Context, revisionID, targetLanguage string) ([]Stored, error)
	// DeleteStatusTranslations deletes the translations of a status with their child rows.
	DeleteStatusTranslations(ctx context.Context, tx uow.Tx, statusID string) error
	// ListExpiredTranslations returns up to limit translations created before createdBefore,
	// oldest first.
	ListExpiredTranslations(ctx context.Context, createdBefore time.Time, limit int) ([]string, error)
	// DeleteTranslations deletes translations with their child rows by ID.
	DeleteTranslations(ctx context.Context, tx uow.Tx, ids []string) error
	// AddUsageWithin adds characters, which are not negative, to the month's rows of the account
	// and of the server, and reports whether the result is within limits. A month is YYYY-MM in
	// UTC. It decides under the rows' locks, so that concurrent requests cannot pass a limit
	// together. On false the caller ends tx with an error: the account's row may have grown. An
	// empty accountID adds to the server's row only, and limits.Account does not apply.
	AddUsageWithin(ctx context.Context, tx uow.Tx, accountID, month string, characters int64, limits Limits) (bool, error)
	// AddCacheLookups adds to the month's counts of requests answered from the cache (hits) and
	// of requests that called a provider (misses), which are not negative.
	AddCacheLookups(ctx context.Context, tx uow.Tx, month string, hits, misses int64) error
	// FindUsage returns the month's characters of the account and of the server, zero without a row.
	FindUsage(ctx context.Context, accountID, month string) (Usage, error)
	// FindServerUsage returns the month's row of the server, zero without a row.
	FindServerUsage(ctx context.Context, month string) (ServerUsage, error)
	// DeleteAccountUsage deletes every row of the account's usage.
	DeleteAccountUsage(ctx context.Context, tx uow.Tx, accountID string) error
	// DeleteUsageBefore deletes the rows of the accounts and of the server before month.
	DeleteUsageBefore(ctx context.Context, tx uow.Tx, month string) error
}
