// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The reads in a transaction (the TxReads of the repositories): each runs the read of the same
// name on a repository over a copy of the database whose reads run in the transaction.

// FindAccountTx implements the reads in a transaction.
func (r *AccountRepository) FindAccountTx(ctx context.Context, tx uow.Tx, id string) (account.Account, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.Account{}, err
	}
	return (&AccountRepository{db: db}).FindAccount(ctx, id)
}

// FindLocalAccountTx implements the reads in a transaction.
func (r *AccountRepository) FindLocalAccountTx(ctx context.Context, tx uow.Tx, accountID string) (account.LocalAccount, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.LocalAccount{}, err
	}
	return (&AccountRepository{db: db}).FindLocalAccount(ctx, accountID)
}

// FindLocalAccountByUserTx implements the reads in a transaction.
func (r *AccountRepository) FindLocalAccountByUserTx(ctx context.Context, tx uow.Tx, userID string) (account.LocalAccount, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.LocalAccount{}, err
	}
	return (&AccountRepository{db: db}).FindLocalAccountByUser(ctx, userID)
}

// FindHandleOfTx implements the reads in a transaction.
func (r *AccountRepository) FindHandleOfTx(ctx context.Context, tx uow.Tx, accountID string) (account.Handle, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.Handle{}, err
	}
	return (&AccountRepository{db: db}).FindHandleOf(ctx, accountID)
}

// FindCurrentProfileTx implements the reads in a transaction.
func (r *AccountRepository) FindCurrentProfileTx(ctx context.Context, tx uow.Tx, accountID string) (account.ProfileRevision, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.ProfileRevision{}, err
	}
	return (&AccountRepository{db: db}).FindCurrentProfile(ctx, accountID)
}

// FindLifecycleStateTx implements the reads in a transaction.
func (r *AccountRepository) FindLifecycleStateTx(ctx context.Context, tx uow.Tx, accountID string) (account.LifecycleState, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.LifecycleState{}, err
	}
	return (&AccountRepository{db: db}).FindLifecycleState(ctx, accountID)
}

// ListKeysTx implements the reads in a transaction.
func (r *AccountRepository) ListKeysTx(ctx context.Context, tx uow.Tx, accountID string) ([]account.Key, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&AccountRepository{db: db}).ListKeys(ctx, accountID)
}

// ListRemoteProfileURLsTx implements the reads in a transaction.
func (r *AccountRepository) ListRemoteProfileURLsTx(ctx context.Context, tx uow.Tx, accountIDs []string) (map[string]string, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&AccountRepository{db: db}).ListRemoteProfileURLs(ctx, accountIDs)
}

// ListReadyVariantsTx implements the reads in a transaction.
func (r *MediaRepository) ListReadyVariantsTx(ctx context.Context, tx uow.Tx, ids []string) ([]appcontent.FileVariant, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&MediaRepository{db: db}).ListReadyVariants(ctx, ids)
}

// ListRemoteMediaTx implements the reads in a transaction.
func (r *MediaRepository) ListRemoteMediaTx(ctx context.Context, tx uow.Tx, ids []string) ([]content.RemoteMedia, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&MediaRepository{db: db}).ListRemoteMedia(ctx, ids)
}

// ListCurrentCustomEmojisTx implements the reads in a transaction.
func (r *MediaRepository) ListCurrentCustomEmojisTx(ctx context.Context, tx uow.Tx, ids []string) ([]content.CurrentCustomEmoji, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&MediaRepository{db: db}).ListCurrentCustomEmojis(ctx, ids)
}

// FindCustomEmojiRecordTx implements the reads in a transaction.
func (r *CustomEmojiRepository) FindCustomEmojiRecordTx(ctx context.Context, tx uow.Tx, id string) (content.CustomEmojiRecord, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return content.CustomEmojiRecord{}, err
	}
	return (&CustomEmojiRepository{db: db}).FindCustomEmojiRecord(ctx, id)
}

// AccountRestrictionsInEffectTx implements the reads in a transaction.
func (r *ModerationRepository) AccountRestrictionsInEffectTx(ctx context.Context, tx uow.Tx, accountID string, now time.Time) ([]moderation.Restriction, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&ModerationRepository{db: db}).AccountRestrictionsInEffect(ctx, accountID, now)
}

// DomainRestrictionsInEffectTx implements the reads in a transaction.
func (r *ModerationRepository) DomainRestrictionsInEffectTx(ctx context.Context, tx uow.Tx, domains []string, now time.Time) ([]moderation.Restriction, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&ModerationRepository{db: db}).DomainRestrictionsInEffect(ctx, domains, now)
}

// FindReportTx implements the reads in a transaction.
func (r *ModerationRepository) FindReportTx(ctx context.Context, tx uow.Tx, id string) (moderation.Report, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return moderation.Report{}, err
	}
	return (&ModerationRepository{db: db}).FindReport(ctx, id)
}

// FindReportStatusTx implements the reads in a transaction.
func (r *ModerationRepository) FindReportStatusTx(ctx context.Context, tx uow.Tx, reportID string) (moderation.ReportStatus, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return moderation.ReportStatus{}, err
	}
	return (&ModerationRepository{db: db}).FindReportStatus(ctx, reportID)
}

// FindActionRecordTx implements the reads in a transaction.
func (r *ModerationRepository) FindActionRecordTx(ctx context.Context, tx uow.Tx, id string) (moderation.ActionRecord, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return moderation.ActionRecord{}, err
	}
	return (&ModerationRepository{db: db}).FindActionRecord(ctx, id)
}

// FindDomainActionRecordTx implements the reads in a transaction.
func (r *ModerationRepository) FindDomainActionRecordTx(ctx context.Context, tx uow.Tx, id string) (moderation.DomainActionRecord, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return moderation.DomainActionRecord{}, err
	}
	return (&ModerationRepository{db: db}).FindDomainActionRecord(ctx, id)
}

// FindAppealTx implements the reads in a transaction.
func (r *ModerationRepository) FindAppealTx(ctx context.Context, tx uow.Tx, id string) (moderation.AppealRecord, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return moderation.AppealRecord{}, err
	}
	return (&ModerationRepository{db: db}).FindAppeal(ctx, id)
}

// FindRegistrationApplicationTx implements the reads in a transaction.
func (r *IdentityRepository) FindRegistrationApplicationTx(ctx context.Context, tx uow.Tx, userID string) (identity.RegistrationApplication, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return identity.RegistrationApplication{}, err
	}
	return (&IdentityRepository{db: db}).FindRegistrationApplication(ctx, userID)
}

// FindRegistrationStateTx implements the reads in a transaction.
func (r *IdentityRepository) FindRegistrationStateTx(ctx context.Context, tx uow.Tx, userID string) (identity.UserRegistrationState, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return identity.UserRegistrationState{}, err
	}
	return (&IdentityRepository{db: db}).FindRegistrationState(ctx, userID)
}

// FindPrimaryEmailTx implements the reads in a transaction.
func (r *IdentityRepository) FindPrimaryEmailTx(ctx context.Context, tx uow.Tx, userID string) (identity.EmailAddress, bool, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return identity.EmailAddress{}, false, err
	}
	return (&IdentityRepository{db: db}).FindPrimaryEmail(ctx, userID)
}

// FindCurrentPasswordCredentialTx implements the reads in a transaction.
func (r *IdentityRepository) FindCurrentPasswordCredentialTx(ctx context.Context, tx uow.Tx, userID string) (identity.PasswordCredential, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return identity.PasswordCredential{}, err
	}
	return (&IdentityRepository{db: db}).FindCurrentPasswordCredential(ctx, userID)
}

// ListActivePasskeyCredentialsTx implements the reads in a transaction.
func (r *IdentityRepository) ListActivePasskeyCredentialsTx(ctx context.Context, tx uow.Tx, userID string) ([]identity.PasskeyCredential, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&IdentityRepository{db: db}).ListActivePasskeyCredentials(ctx, userID)
}

// ListLinkedExternalIdentitiesTx implements the reads in a transaction.
func (r *IdentityRepository) ListLinkedExternalIdentitiesTx(ctx context.Context, tx uow.Tx, userID string) ([]identity.LinkedExternalIdentity, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&IdentityRepository{db: db}).ListLinkedExternalIdentities(ctx, userID)
}
