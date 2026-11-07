// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
)

var _ appidentity.ErasureRepository = (*Repository)(nil)

// EraseUserData implements identity.ErasureRepository.
func (r *Repository) EraseUserData(ctx context.Context, tx uow.Tx, userID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	// Children before their parents.
	return sqlite.RunSteps(ctx, userID, "erase user data",
		q.EraseEmailSuppressions, q.EraseEmailSuppressionLifts, q.EraseEmailDeliveryFailures,
		q.EraseEmailVerifications, q.EraseUserPrimaryEmail, q.EraseEmailAddresses, q.ErasePasswordCredentialRemovals, q.ErasePasswordCredentials,
		q.EraseTOTPRecoveryCodeUses, q.EraseTOTPRecoveryCodes, q.EraseTOTPEnrollmentRevocations, q.EraseTOTPEnrollments,
		q.ErasePasskeyCredentialRevocations, q.ErasePasskeyCredentials,
		q.EraseUserExternalIdentities, q.EraseExternalIdentityUnlinks, q.EraseExternalIdentities, q.EraseExternalIdentityLinks,
		q.EraseUserTokenRedemptions, q.EraseUserTokens, q.EraseSessionRevocations, q.EraseSessionActivity,
		q.EraseSessions, q.EraseAuthorizationCodeRedemptions, q.EraseAuthorizationCodes,
		q.EraseAccessTokenRevocations, q.EraseTokenFamilyLastUsed, q.EraseAccessTokens, q.EraseUserPreferences, q.EraseUserRoleBadgeDisplays, q.EraseUserTimelineLanguages, q.EraseUserTimeZone,
		q.EraseRegistrationDecisions, q.EraseRegistrationApplications, q.EraseRegistrationState, q.EraseServiceClosureEmails)
}

// ListOwnedOAuthClientIDs implements identity.ErasureRepository.
func (r *Repository) ListOwnedOAuthClientIDs(ctx context.Context, tx uow.Tx, userID string) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListOAuthClientIDsOwnedBy(ctx, sqlite.ToNullString(userID))
	return ids, sqlite.Wrap(err, "list owned oauth clients")
}

// BlankOwnedOAuthClientDetails implements identity.ErasureRepository.
func (r *Repository) BlankOwnedOAuthClientDetails(ctx context.Context, tx uow.Tx, userID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.BlankOAuthClientRevisionDetailsOf(ctx, sqlite.ToNullString(userID)); err != nil {
		return sqlite.Wrap(err, "blank owned oauth client revisions")
	}
	return sqlite.Wrap(q.BlankOAuthClientDetailsOf(ctx, sqlite.ToNullString(userID)), "blank owned oauth clients")
}
