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

// Repository is the module's persistence. The database adapters implement it and pass
// testkit.TranslationRepositoryContract.
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
	// AddUsage adds characters, which are not negative, to the month's rows of the account and of
	// the server. A month is YYYY-MM in UTC.
	AddUsage(ctx context.Context, tx uow.Tx, accountID, month string, characters int64) error
	// FindUsage returns the month's characters of the account and of the server, zero without a row.
	FindUsage(ctx context.Context, accountID, month string) (Usage, error)
	// DeleteAccountUsage deletes every row of the account's usage.
	DeleteAccountUsage(ctx context.Context, tx uow.Tx, accountID string) error
	// DeleteUsageBefore deletes the rows of the accounts and of the server before month.
	DeleteUsageBefore(ctx context.Context, tx uow.Tx, month string) error
}
