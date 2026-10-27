// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// ErrNotFound reports that no row matches a lookup.
var ErrNotFound = errors.New("identity: not found")

// ErrEmailTaken reports that another user already has the email address as primary address.
var ErrEmailTaken = errors.New("identity: email address is taken")

// Repository is the persistence port of the identity module (data-model §4.1). The database
// adapters implement it and pass testkit.AccountRepositoryContract, testkit.PreferencesRepositoryContract,
// and testkit.RegistrationRepositoryContract.
//
// Methods that take a uow.Tx run inside that transaction; the others read committed data outside
// any transaction. Lookups that find nothing return an error wrapping ErrNotFound.
type Repository interface {
	TxReads

	InsertUser(ctx context.Context, tx uow.Tx, u identity.User) error
	InsertPasswordCredential(ctx context.Context, tx uow.Tx, c identity.PasswordCredential) error
	// EnsureRole inserts a role with perms unless one with the same key exists, and returns the
	// stored role: r when it inserted r, and the existing role, whose permissions it leaves as
	// they are, otherwise.
	EnsureRole(ctx context.Context, tx uow.Tx, r identity.Role, perms []identity.Permission) (identity.Role, error)
	InsertEmailAddress(ctx context.Context, tx uow.Tx, e identity.EmailAddress) error
	// InsertPrimaryEmail makes e the primary email address of its user, who has none yet.
	// It fails with an error wrapping ErrEmailTaken when another user's primary address has
	// the same normalized form.
	InsertPrimaryEmail(ctx context.Context, tx uow.Tx, e identity.EmailAddress) error
	// InsertRoleGrant records g and adds the role to the user's current roles; the user must not
	// hold the role yet.
	InsertRoleGrant(ctx context.Context, tx uow.Tx, g identity.RoleGrant) error

	FindUser(ctx context.Context, id string) (identity.User, error)
	// FindUserIDByEmail returns the user whose primary email address normalizes to normalized.
	FindUserIDByEmail(ctx context.Context, normalized string) (string, error)
	// ListUserRoles returns the roles a user holds, highest position first.
	ListUserRoles(ctx context.Context, userID string) ([]identity.Role, error)
	// FindCurrentPasswordCredential returns the newest password credential of a user unless it
	// has a removal; ErrNotFound when the user has no password.
	FindCurrentPasswordCredential(ctx context.Context, userID string) (identity.PasswordCredential, error)
	// LastPasswordChange returns when the password of a user last changed: when the newest
	// password credential was set, or removed when it has a removal. ErrNotFound when the user
	// never had a password.
	LastPasswordChange(ctx context.Context, userID string) (time.Time, error)
	// InsertPasswordCredentialRemoval ends a password credential; it does nothing for one that
	// ended.
	InsertPasswordCredentialRemoval(ctx context.Context, tx uow.Tx, r identity.PasswordCredentialRemoval) error
	// LockSignInMethods holds the ways to sign in of a user (the password and the passkeys) in tx
	// until it ends, so that the transactions that remove one of them serialize.
	LockSignInMethods(ctx context.Context, tx uow.Tx, userID string) error
	// FindRole returns a role by key with its permissions, sorted.
	FindRole(ctx context.Context, key string) (identity.Role, []identity.Permission, error)
	// ListUserIDsWithPermission returns inside tx the users whose current roles grant p, by ID.
	ListUserIDsWithPermission(ctx context.Context, tx uow.Tx, p identity.Permission) ([]string, error)

	TOTPRepository
	PasskeyRepository
	PreferencesRepository
	OAuthRepository
	OAuthAppRepository
	RegistrationRepository
	OriginRepository
	ExportRepository
	ImportRepository
	RoleRepository
	ManagedRoleRepository
	AccountAdminRepository
	ProjectionRepository
	EmailSuppressionRepository
	ClosureEmailRepository
	LoginProviderRepository

	InsertSession(ctx context.Context, tx uow.Tx, s identity.Session) error
	// InsertSessionRevocation records that a session ended; it does nothing when the session
	// already has a revocation, so that revoking twice keeps the first reason.
	InsertSessionRevocation(ctx context.Context, tx uow.Tx, r identity.SessionRevocation) error
	// RevokeSessionsOfUser revokes every session of a user that has not expired at at and has
	// no revocation yet.
	RevokeSessionsOfUser(ctx context.Context, tx uow.Tx, userID string, reason identity.RevocationReason, at time.Time) error
	UpsertSessionActivity(ctx context.Context, tx uow.Tx, a identity.SessionActivity) error
	// FindUnrevokedSession returns the session whose token hashes to tokenHash, unless it has
	// a revocation, with its activity (a zero LastSeenAt when it has none). Expiry is the
	// caller's to check.
	FindUnrevokedSession(ctx context.Context, tokenHash []byte) (identity.Session, identity.SessionActivity, error)
}

// TOTPRepository is the part of Repository that keeps TOTP enrollments and their recovery codes
// (data-model §4.1). The database adapters pass testkit.AccountRepositoryContract.
type TOTPRepository interface {
	// InsertTOTPEnrollment records an enrollment with the digests of its recovery codes.
	InsertTOTPEnrollment(ctx context.Context, tx uow.Tx, e identity.TOTPEnrollment, recoveryCodeHashes [][]byte) error
	// FindActiveTOTPEnrollment returns the newest enrollment of a user, unless it has a
	// revocation; ErrNotFound otherwise.
	FindActiveTOTPEnrollment(ctx context.Context, userID string) (identity.TOTPEnrollment, error)
	// InsertTOTPEnrollmentRevocation ends an enrollment; it does nothing for one that ended.
	InsertTOTPEnrollmentRevocation(ctx context.Context, tx uow.Tx, r identity.TOTPEnrollmentRevocation) error
	// UseTOTPRecoveryCode records the use of a recovery code and reports whether the enrollment
	// has the code and it was not used before; it records nothing otherwise.
	UseTOTPRecoveryCode(ctx context.Context, tx uow.Tx, u identity.TOTPRecoveryCodeUse) (bool, error)
	// CountUnusedTOTPRecoveryCodes returns how many recovery codes of an enrollment are unused.
	CountUnusedTOTPRecoveryCodes(ctx context.Context, enrollmentID string) (int, error)
}

// PasskeyRepository is the part of Repository that keeps passkeys (data-model §4.1). The
// database adapters pass testkit.AccountRepositoryContract.
type PasskeyRepository interface {
	// InsertPasskeyCredential records a passkey.
	InsertPasskeyCredential(ctx context.Context, tx uow.Tx, c identity.PasskeyCredential) error
	// ListActivePasskeyCredentials returns the passkeys of a user without a revocation, oldest
	// first.
	ListActivePasskeyCredentials(ctx context.Context, userID string) ([]identity.PasskeyCredential, error)
	// FindPasskeyCredential returns the passkey with a credential ID, revoked or not;
	// ErrNotFound when there is none.
	FindPasskeyCredential(ctx context.Context, credentialID []byte) (identity.PasskeyCredential, error)
	// FindActivePasskeyCredential returns the passkey with a credential ID unless it has a
	// revocation; ErrNotFound otherwise.
	FindActivePasskeyCredential(ctx context.Context, credentialID []byte) (identity.PasskeyCredential, error)
	// InsertPasskeyCredentialRevocation ends a passkey; it does nothing for one that ended.
	InsertPasskeyCredentialRevocation(ctx context.Context, tx uow.Tx, r identity.PasskeyCredentialRevocation) error
}

// TxReads are the reads of Repository that run in a transaction tx, with what tx has written, for the
// use cases that present what an event's transaction wrote (the data of the webhook messages,
// ADR-0060) and for the checks that a transaction makes under a lock (LockSignInMethods). Each one
// reads as the method of the same name without Tx does.
type TxReads interface {
	FindCurrentPasswordCredentialTx(ctx context.Context, tx uow.Tx, userID string) (identity.PasswordCredential, error)
	ListActivePasskeyCredentialsTx(ctx context.Context, tx uow.Tx, userID string) ([]identity.PasskeyCredential, error)
	FindRegistrationApplicationTx(ctx context.Context, tx uow.Tx, userID string) (identity.RegistrationApplication, error)
	FindRegistrationStateTx(ctx context.Context, tx uow.Tx, userID string) (identity.UserRegistrationState, error)
	FindPrimaryEmailTx(ctx context.Context, tx uow.Tx, userID string) (identity.EmailAddress, bool, error)
}
