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
	"github.com/mycroblog/my/internal/domain/instance"
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

// HeldAccountIDsTx implements the reads in a transaction.
func (r *ModerationRepository) HeldAccountIDsTx(ctx context.Context, tx uow.Tx) ([]string, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&ModerationRepository{db: db}).HeldAccountIDs(ctx)
}

// ReportedVoiceRoomsTx implements the reads in a transaction.
func (r *ModerationRepository) ReportedVoiceRoomsTx(ctx context.Context, tx uow.Tx, roomIDs []string) (map[string]bool, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&ModerationRepository{db: db}).ReportedVoiceRooms(ctx, roomIDs)
}

// FindInstanceActorTx implements the reads in a transaction.
func (r *AccountRepository) FindInstanceActorTx(ctx context.Context, tx uow.Tx) (account.Account, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.Account{}, err
	}
	return (&AccountRepository{db: db}).FindInstanceActor(ctx)
}

// FindRemoteHeadTx implements the reads in a transaction.
func (r *AccountRepository) FindRemoteHeadTx(ctx context.Context, tx uow.Tx, accountID string) (account.RemoteHead, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.RemoteHead{}, err
	}
	return (&AccountRepository{db: db}).FindRemoteHead(ctx, accountID)
}

// HasAliasTx implements the reads in a transaction.
func (r *AccountRepository) HasAliasTx(ctx context.Context, tx uow.Tx, accountID, uri string) (bool, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return false, err
	}
	return (&AccountRepository{db: db}).HasAlias(ctx, accountID, uri)
}

// FindMoveTx implements the reads in a transaction.
func (r *AccountRepository) FindMoveTx(ctx context.Context, tx uow.Tx, id string) (account.Move, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return account.Move{}, err
	}
	return (&AccountRepository{db: db}).FindMove(ctx, id)
}

// FindCustomEmojiByShortcodeTx implements the reads in a transaction.
func (r *StatusRepository) FindCustomEmojiByShortcodeTx(ctx context.Context, tx uow.Tx, shortcode, domain string) (content.CustomEmoji, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return content.CustomEmoji{}, err
	}
	return (&StatusRepository{db: db}).FindCustomEmojiByShortcode(ctx, shortcode, domain)
}

// FindRevisionTx implements the reads in a transaction.
func (r *StatusRepository) FindRevisionTx(ctx context.Context, tx uow.Tx, id string) (content.Revision, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return content.Revision{}, err
	}
	return (&StatusRepository{db: db}).FindRevision(ctx, id)
}

// FindDeletionByURITx implements the reads in a transaction.
func (r *StatusRepository) FindDeletionByURITx(ctx context.Context, tx uow.Tx, uri string) (content.Deletion, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return content.Deletion{}, err
	}
	return (&StatusRepository{db: db}).FindDeletionByURI(ctx, uri)
}

// FindStatusByURITx implements the reads in a transaction.
func (r *StatusRepository) FindStatusByURITx(ctx context.Context, tx uow.Tx, uri string) (content.Status, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return content.Status{}, err
	}
	return (&StatusRepository{db: db}).FindStatusByURI(ctx, uri)
}

// FindBoostRetractionTx implements the reads in a transaction.
func (r *InteractionRepository) FindBoostRetractionTx(ctx context.Context, tx uow.Tx, boostID string) (content.BoostRetraction, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return content.BoostRetraction{}, err
	}
	return (&InteractionRepository{db: db}).FindBoostRetraction(ctx, boostID)
}

// ListLoginProvidersTx implements the reads in a transaction.
func (r *IdentityRepository) ListLoginProvidersTx(ctx context.Context, tx uow.Tx) ([]identity.LoginProvider, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&IdentityRepository{db: db}).ListLoginProviders(ctx)
}

// ListCurrentSettingsTx implements the reads in a transaction.
func (r *InstanceRepository) ListCurrentSettingsTx(ctx context.Context, tx uow.Tx) ([]instance.CurrentSetting, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&InstanceRepository{db: db}).ListCurrentSettings(ctx)
}
