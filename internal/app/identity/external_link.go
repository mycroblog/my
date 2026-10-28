// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Linking the account at an external login provider to a signed-in user, unlinking it, and
// confirming an operation by signing in again at a linked provider (FR-010, ADR-0106 §3.5–7).
//
// Both a link and a confirmation start from the client API with the user's bearer token:
// BeginExternalLink, once the user confirmed it, and BeginExternalConfirmation seal an intent
// (the user, the provider, the operation, and the return target) into the start URL of the
// provider. The start (BeginExternalIntent) takes it only in a browser signed in as the same user,
// so that another user's intent cannot link the browser's account at the provider to them, and
// goes on as a sign-in does with the intent in the sealed cookie. A confirmation asks the provider
// to authenticate the user again (max_age=0 and prompt=login for OpenID Connect, whose auth_time
// must be later than the request; force_login=true for Mastodon; nothing for GitHub and Discord, which have no
// way to ask). FinishExternalLogin ends either: a link checks the provider's allow_link and
// conditions and refuses an account linked to another user; a confirmation checks that the
// account is the one the user links and returns a token that the operation takes once
// (Confirmation.External). A wrong account or an ID token that does not verify counts against the
// limit of the operation, as a wrong password does.

// ExternalPurpose is what a round trip to a provider other than a sign-in is for.
type ExternalPurpose string

// External purposes.
const (
	// ExternalPurposeLink links the account at the provider to the user.
	ExternalPurposeLink ExternalPurpose = "link"
	// ExternalPurposeConfirm confirms an operation of the user.
	ExternalPurposeConfirm ExternalPurpose = "confirm"
)

// ConfirmedOperation is an operation that a signed-in user confirms (Confirmation).
type ConfirmedOperation string

// Confirmed operations.
const (
	OperationPasskeyRegistration ConfirmedOperation = "passkey_registration"
	OperationPasswordChange      ConfirmedOperation = "password_change"
	OperationPasswordRemoval     ConfirmedOperation = "password_removal"
	OperationAccountDeletion     ConfirmedOperation = "account_deletion"
	OperationAccountMove         ConfirmedOperation = "account_move"
	OperationLink                ConfirmedOperation = "link"
)

// ConfirmedOperations lists every operation.
func ConfirmedOperations() []ConfirmedOperation {
	return []ConfirmedOperation{
		OperationPasskeyRegistration, OperationPasswordChange, OperationPasswordRemoval,
		OperationAccountDeletion, OperationAccountMove, OperationLink,
	}
}

// Valid reports whether op is a known operation.
func (op ConfirmedOperation) Valid() bool { return slices.Contains(ConfirmedOperations(), op) }

// LinkConfirmationLimit bounds the wrong confirmations of a link, and the links whose account
// does not verify, per user.
var LinkConfirmationLimit = kernel.Limit{Name: "link.confirmation", Max: 5, Window: time.Hour}

// Limit returns the limit that the wrong confirmations of op count against, per user.
func (op ConfirmedOperation) Limit() kernel.Limit {
	switch op {
	case OperationPasskeyRegistration:
		return PasskeyPasswordLimit
	case OperationPasswordChange:
		return PasswordChangeLimit
	case OperationPasswordRemoval:
		return PasswordRemovalLimit
	case OperationAccountDeletion:
		return SelfDeletionLimit
	case OperationAccountMove:
		return MoveConfirmationLimit
	}
	return LinkConfirmationLimit
}

var (
	// ErrExternalConfirmationExpired reports the token of a confirmation with a provider that
	// is malformed, expired, another user's or operation's, used, or whose link ended: the
	// confirmation starts again.
	ErrExternalConfirmationExpired = errors.New("identity: the confirmation with the login provider expired")
	// ErrExternalAccountMismatch reports a confirmation in which the user signed in at the
	// provider with an account that the user does not link.
	ErrExternalAccountMismatch = errors.New("identity: the account at the login provider is not the user's")
	// ErrExternalSessionMismatch reports the start of a link or a confirmation in a browser that
	// is not signed in as the user who asked for it.
	ErrExternalSessionMismatch = errors.New("identity: the browser is not signed in as the user of the intent")
	// ErrInvalidConfirmedOperation reports an unknown operation.
	ErrInvalidConfirmedOperation = errors.New("identity: unknown confirmed operation")
	// ErrLoginProviderNotLinked reports a confirmation with a provider that the user does not
	// link, or that is not available.
	ErrLoginProviderNotLinked = errors.New("identity: the user has no link to the login provider")
)

// External link and confirmation defaults.
const (
	// ExternalIntentTTL is how long the start URL of a link or a confirmation works.
	ExternalIntentTTL = 10 * time.Minute
	// ExternalConfirmationTTL is how long the token of a confirmation with a provider works.
	ExternalConfirmationTTL = 10 * time.Minute
	// ExternalIntentParam is the query parameter of a provider's start that carries an intent.
	ExternalIntentParam = "intent"
)

var externalIntentAAD = kernel.AAD("external_intent")

func externalConfirmationAAD(userID string) []byte {
	return kernel.AAD(userID, "external_confirmation")
}

// externalIntentToken is what the start URL of a link or a confirmation seals.
type externalIntentToken struct {
	IssuedAt   int64              `json:"iat"` // Unix milliseconds
	Purpose    ExternalPurpose    `json:"purpose"`
	UserID     string             `json:"user"`
	ProviderID string             `json:"provider"`
	Operation  ConfirmedOperation `json:"op,omitempty"`
	ReturnTo   string             `json:"return_to,omitempty"`
}

// externalConfirmationToken is what the token of a confirmation with a provider seals.
type externalConfirmationToken struct {
	IssuedAt   int64              `json:"iat"` // Unix milliseconds
	ProviderID string             `json:"provider"`
	LinkID     string             `json:"link"`
	Operation  ConfirmedOperation `json:"op"`
}

// ExternalIntent is the start of a link or a confirmation: the browser goes to StartURL before
// ExpiresAt.
type ExternalIntent struct {
	StartURL  string
	ExpiresAt time.Time
}

// LinkedIdentity is a current link of a user, with its provider.
type LinkedIdentity struct {
	LinkID       string
	ProviderID   string
	ProviderKind identity.LoginProviderKind
	// ProviderName is the administrator's name for the provider, shown as written.
	ProviderName string
	LinkedAt     time.Time
	// Available tells that the provider is enabled: the link signs in and confirms.
	Available bool
}

// LinkableProvider is an enabled provider that users sign in with.
type LinkableProvider struct {
	ID          string
	Kind        identity.LoginProviderKind
	DisplayName string
	// AllowLink tells that users link accounts at it.
	AllowLink bool
}

// LinkedIdentities are a user's links and the providers that the user can link or confirm with.
type LinkedIdentities struct {
	// Links are the current links to providers that are not deleted, oldest first.
	Links []LinkedIdentity
	// Providers are the enabled providers that users sign in with, oldest first; none without
	// the dependencies of external sign-in.
	Providers []LinkableProvider
}

// LinkedIdentities returns the links of a user and the providers.
func (s *Service) LinkedIdentities(ctx context.Context, userID string) (LinkedIdentities, error) {
	ps, err := s.d.Repository.ListLoginProviders(ctx)
	if err != nil {
		return LinkedIdentities{}, err
	}
	byID := make(map[string]identity.LoginProvider, len(ps))
	var out LinkedIdentities
	for _, p := range slices.Backward(ps) {
		byID[p.ID] = p
		if s.ExternalLoginsEnabled() && p.State == identity.LoginProviderEnabled && externalLoginKind(p.Settings.Kind) {
			out.Providers = append(out.Providers, LinkableProvider{
				ID: p.ID, Kind: p.Settings.Kind, DisplayName: p.Settings.DisplayName, AllowLink: p.Settings.AllowLink,
			})
		}
	}
	links, err := s.d.Repository.ListLinkedExternalIdentities(ctx, userID)
	if err != nil {
		return LinkedIdentities{}, err
	}
	for _, l := range links {
		p, ok := byID[l.ProviderID]
		if !ok {
			continue
		}
		out.Links = append(out.Links, LinkedIdentity{
			LinkID: l.LinkID, ProviderID: p.ID, ProviderKind: p.Settings.Kind, ProviderName: p.Settings.DisplayName,
			LinkedAt: l.LinkedAt, Available: p.State == identity.LoginProviderEnabled && externalLoginKind(p.Settings.Kind),
		})
	}
	return out, nil
}

// signInLinks returns the current links of a user in tx that count as ways to sign in: those to
// providers that are not deleted.
func (s *Service) signInLinks(ctx context.Context, tx uow.Tx, userID string) ([]identity.LinkedExternalIdentity, error) {
	links, err := s.d.Repository.ListLinkedExternalIdentitiesTx(ctx, tx, userID)
	if err != nil || len(links) == 0 {
		return nil, err
	}
	ps, err := s.d.Repository.ListLoginProviders(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(links, func(l identity.LinkedExternalIdentity) bool {
		return !slices.ContainsFunc(ps, func(p identity.LoginProvider) bool { return p.ID == l.ProviderID })
	}), nil
}

// UnlinkExternalIdentity removes a link of a user. It fails with ErrNotFound for a link that is
// not the user's current one, and with ErrLastSignInMethod when the user would have no way to
// sign in: no password, no passkey, and no other link.
func (s *Service) UnlinkExternalIdentity(ctx context.Context, userID, linkID string) error {
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		repo := s.d.Repository
		if err := repo.LockSignInMethods(ctx, tx, userID); err != nil {
			return err
		}
		links, err := s.signInLinks(ctx, tx, userID)
		if err != nil {
			return err
		}
		all, err := repo.ListLinkedExternalIdentitiesTx(ctx, tx, userID)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(all, func(l identity.LinkedExternalIdentity) bool { return l.LinkID == linkID }) {
			return ErrNotFound
		}
		others := slices.DeleteFunc(links, func(l identity.LinkedExternalIdentity) bool { return l.LinkID == linkID })
		if len(others) == 0 {
			if err := s.keepsSignInMethod(ctx, tx, userID); err != nil {
				return err
			}
		}
		return repo.InsertExternalIdentityUnlink(ctx, tx, identity.ExternalIdentityUnlink{LinkID: linkID, UnlinkedAt: s.d.Clock.Now()})
	})
}

// keepsSignInMethod fails with ErrLastSignInMethod when the user has neither a password nor a
// passkey.
func (s *Service) keepsSignInMethod(ctx context.Context, tx uow.Tx, userID string) error {
	repo := s.d.Repository
	switch _, err := repo.FindCurrentPasswordCredentialTx(ctx, tx, userID); {
	case err == nil:
		return nil
	case !errors.Is(err, ErrNotFound):
		return err
	}
	passkeys, err := repo.ListActivePasskeyCredentialsTx(ctx, tx, userID)
	if err != nil {
		return err
	}
	if len(passkeys) == 0 {
		return ErrLastSignInMethod
	}
	return nil
}

// BeginExternalLink starts linking the account at provider providerID to a user, once c
// confirms it, and returns to returnTo, a path on this site, afterwards. It fails with
// ErrLoginProviderUnavailable for a provider that users cannot sign in with or link to, with
// ErrLoginProviderLinked when the user links an account at it already, and with the errors of
// the confirmation (counted against LinkConfirmationLimit).
func (s *Service) BeginExternalLink(ctx context.Context, userID, providerID string, c Confirmation, returnTo string) (ExternalIntent, error) {
	p, err := s.enabledLoginProvider(ctx, providerID)
	if err != nil {
		return ExternalIntent{}, err
	}
	if !p.Settings.AllowLink {
		return ExternalIntent{}, ErrLoginProviderUnavailable
	}
	if _, ok, err := s.userLink(ctx, userID, p.ID); err != nil {
		return ExternalIntent{}, err
	} else if ok {
		return ExternalIntent{}, ErrLoginProviderLinked
	}
	if err := s.confirm(ctx, OperationLink, userID, c); err != nil {
		return ExternalIntent{}, err
	}
	return s.sealIntent(ctx, externalIntentToken{Purpose: ExternalPurposeLink, UserID: userID, ProviderID: p.ID, ReturnTo: returnTo})
}

// BeginExternalConfirmation starts a confirmation of operation op by a user with the linked
// provider providerID, which returns to returnTo, a path on this site, afterwards. It fails with
// ErrInvalidConfirmedOperation for an unknown operation, with ErrLoginProviderNotLinked when the
// user links no account at an available provider providerID, and with a
// *kernel.RateLimitedError after too many wrong confirmations of the operation.
func (s *Service) BeginExternalConfirmation(ctx context.Context, userID, providerID string, op ConfirmedOperation, returnTo string) (ExternalIntent, error) {
	if !op.Valid() {
		return ExternalIntent{}, ErrInvalidConfirmedOperation
	}
	p, err := s.enabledLoginProvider(ctx, providerID)
	if errors.Is(err, ErrLoginProviderUnavailable) {
		return ExternalIntent{}, ErrLoginProviderNotLinked
	}
	if err != nil {
		return ExternalIntent{}, err
	}
	if _, ok, err := s.userLink(ctx, userID, p.ID); err != nil {
		return ExternalIntent{}, err
	} else if !ok {
		return ExternalIntent{}, ErrLoginProviderNotLinked
	}
	if err := s.checkLimit(ctx, op.Limit(), userID); err != nil {
		return ExternalIntent{}, err
	}
	return s.sealIntent(ctx, externalIntentToken{
		Purpose: ExternalPurposeConfirm, UserID: userID, ProviderID: p.ID, Operation: op, ReturnTo: returnTo,
	})
}

// userLink returns the current link of a user to a provider.
func (s *Service) userLink(ctx context.Context, userID, providerID string) (identity.LinkedExternalIdentity, bool, error) {
	links, err := s.d.Repository.ListLinkedExternalIdentities(ctx, userID)
	if err != nil {
		return identity.LinkedExternalIdentity{}, false, err
	}
	for _, l := range links {
		if l.ProviderID == providerID {
			return l, true, nil
		}
	}
	return identity.LinkedExternalIdentity{}, false, nil
}

func (s *Service) checkLimit(ctx context.Context, limit kernel.Limit, userID string) error {
	d, err := s.d.RateLimiter.Check(ctx, limit, userID)
	if err != nil {
		return err
	}
	if !d.Allowed {
		return &kernel.RateLimitedError{RetryAfter: d.RetryAfter}
	}
	return nil
}

func (s *Service) sealIntent(ctx context.Context, t externalIntentToken) (ExternalIntent, error) {
	now := s.d.Clock.Now()
	t.IssuedAt = now.UnixMilli()
	sealed, err := s.sealJSON(ctx, t, externalIntentAAD)
	if err != nil {
		return ExternalIntent{}, err
	}
	start := strings.TrimSuffix(s.d.BaseURL, "/") + ExternalLoginPathPrefix + url.PathEscape(t.ProviderID) + "/start?" +
		url.Values{ExternalIntentParam: {sealed}}.Encode()
	return ExternalIntent{StartURL: start, ExpiresAt: now.Add(ExternalIntentTTL)}, nil
}

// BeginExternalIntent starts the link or the confirmation of intent, the sealed value of the
// start URL of provider providerID, in a browser signed in as sessionUserID ("" for none). It
// fails with ErrExternalLoginExpired for an intent that is malformed, expired, or another
// provider's; with ErrExternalSessionMismatch when sessionUserID is not the intent's user; and
// as BeginExternalLogin does.
func (s *Service) BeginExternalIntent(ctx context.Context, providerID, intent, sessionUserID string) (ExternalLoginStart, error) {
	if err := s.needExternalLoginDeps(); err != nil {
		return ExternalLoginStart{}, err
	}
	var t externalIntentToken
	if err := s.openJSON(ctx, intent, externalIntentAAD, ExternalIntentTTL, &t); err != nil {
		return ExternalLoginStart{}, err
	}
	if t.ProviderID != providerID || (t.Purpose != ExternalPurposeLink && t.Purpose != ExternalPurposeConfirm) {
		return ExternalLoginStart{}, ErrExternalLoginExpired
	}
	if sessionUserID == "" || sessionUserID != t.UserID {
		return ExternalLoginStart{}, ErrExternalSessionMismatch
	}
	p, err := s.enabledLoginProvider(ctx, providerID)
	if err != nil {
		return ExternalLoginStart{}, err
	}
	now := s.d.Clock.Now()
	lt := externalLoginToken{
		IssuedAt: now.UnixMilli(), ProviderID: p.ID, ReturnTo: t.ReturnTo,
		State: randomToken(), Nonce: randomToken(), Verifier: randomToken(),
		Purpose: t.Purpose, UserID: t.UserID, Operation: t.Operation,
	}
	if t.Purpose == ExternalPurposeConfirm {
		lt.AuthAfter = now.UnixMilli()
	}
	return s.beginAuthorization(ctx, p, lt)
}

// reauthenticationParams adds to the authorization request of an OpenID Connect provider what
// asks the user to sign in again (ADR-0106 §3.7). A Mastodon provider gets force_login=true
// (mastodonAuthorizationURL), and GitHub and Discord have no way to ask.
func reauthenticationParams(k identity.LoginProviderKind, q url.Values) {
	if k == identity.LoginProviderOIDC {
		q.Set("max_age", "0")
		q.Set("prompt", "login")
	}
}

// finishExternalIntent ends a link or a confirmation whose cookie and state checked out.
func (s *Service) finishExternalIntent(ctx context.Context, in ExternalLoginInput, t externalLoginToken, out ExternalLogin) (ExternalLogin, error) {
	limit := LinkConfirmationLimit
	if t.Purpose == ExternalPurposeConfirm {
		limit = t.Operation.Limit()
	}
	if err := s.checkLimit(ctx, limit, t.UserID); err != nil {
		return out, err
	}
	// fail counts a failure against the operation's limit.
	fail := func(err error) error {
		if rerr := s.d.RateLimiter.Record(ctx, limit, t.UserID); rerr != nil {
			return rerr
		}
		return err
	}
	p, err := s.enabledLoginProvider(ctx, t.ProviderID)
	if err != nil {
		return out, err
	}
	if err := s.checkActive(ctx, t.UserID); err != nil {
		return out, err
	}
	if in.Error != "" || in.Code == "" {
		return out, fmt.Errorf("%w: provider %s answered %q", ErrExternalLoginFailed, p.ID, in.Error)
	}
	tok, err := s.externalIdentity(ctx, p, in.Code, t)
	if errors.Is(err, ErrInvalidIDToken) {
		return out, fail(fmt.Errorf("%w: %w", ErrExternalLoginFailed, err))
	}
	if err != nil {
		return out, err
	}
	if !meetsClaims(p.Settings.Claims, tok.Claims) {
		return out, fail(ErrExternalLoginDenied)
	}
	if t.Purpose == ExternalPurposeConfirm {
		link, err := s.d.Repository.FindLinkedExternalIdentity(ctx, p.ID, tok.Subject)
		switch {
		case errors.Is(err, ErrNotFound) || (err == nil && link.UserID != t.UserID):
			return out, fail(ErrExternalAccountMismatch)
		case err != nil:
			return out, err
		}
		token, err := s.sealJSON(ctx, externalConfirmationToken{
			IssuedAt: s.d.Clock.Now().UnixMilli(), ProviderID: p.ID, LinkID: link.LinkID, Operation: t.Operation,
		}, externalConfirmationAAD(t.UserID))
		if err != nil {
			return out, err
		}
		out.Confirmation = token
		return out, nil
	}
	if err := s.linkExternalIdentity(ctx, p, t.UserID, tok.Subject); err != nil {
		return out, err
	}
	out.Linked = true
	return out, nil
}

// linkExternalIdentity links the account subject at provider p to a user. A link that the user
// has already succeeds. It fails with ErrLoginProviderUnavailable when p no longer allows links,
// with ErrExternalIdentityLinked when another user links the account, and with
// ErrLoginProviderLinked when the user links another account at p.
func (s *Service) linkExternalIdentity(ctx context.Context, p identity.LoginProvider, userID, subject string) error {
	if !p.Settings.AllowLink {
		return ErrLoginProviderUnavailable
	}
	switch l, err := s.d.Repository.FindLinkedExternalIdentity(ctx, p.ID, subject); {
	case err == nil && l.UserID == userID:
		return nil
	case err == nil:
		return ErrExternalIdentityLinked
	case !errors.Is(err, ErrNotFound):
		return err
	}
	now := s.d.Clock.Now()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		repo := s.d.Repository
		if err := repo.LockSignInMethods(ctx, tx, userID); err != nil {
			return err
		}
		x, err := repo.EnsureExternalIdentity(ctx, tx, identity.ExternalIdentity{ID: s.d.IDs.NewID(), ProviderID: p.ID, Subject: subject, CreatedAt: now})
		if err != nil {
			return err
		}
		return repo.InsertExternalIdentityLink(ctx, tx, x, identity.ExternalIdentityLink{
			ID: s.d.IDs.NewID(), ExternalIdentityID: x.ID, UserID: userID, LinkedAt: now,
		})
	})
}

// confirmExternal checks the token of a confirmation of operation op by a user with a linked
// provider, which works once. It fails with ErrExternalConfirmationExpired for a token that is
// malformed, expired, another user's or operation's, used, or whose link ended, and with a
// *kernel.RateLimitedError after too many wrong confirmations of the operation.
func (s *Service) confirmExternal(ctx context.Context, op ConfirmedOperation, userID, token string) error {
	if err := s.needExternalLoginDeps(); err != nil {
		return err
	}
	if err := s.checkLimit(ctx, op.Limit(), userID); err != nil {
		return err
	}
	var t externalConfirmationToken
	if err := s.openJSON(ctx, token, externalConfirmationAAD(userID), ExternalConfirmationTTL, &t); err != nil {
		if errors.Is(err, ErrExternalLoginExpired) {
			return ErrExternalConfirmationExpired
		}
		return err
	}
	if t.Operation != op {
		return ErrExternalConfirmationExpired
	}
	// The confirmation works once; the mark outlives the token.
	digest := sha256.Sum256([]byte(token))
	n, err := s.d.KV.IncrBy(ctx, ExternalLoginNamespace, "confirmed:"+hex.EncodeToString(digest[:]), 1, ExternalConfirmationTTL+2*time.Minute)
	if err != nil {
		return fmt.Errorf("identity: record a used external confirmation: %w", err)
	}
	if n != 1 {
		return ErrExternalConfirmationExpired
	}
	link, ok, err := s.userLink(ctx, userID, t.ProviderID)
	if err != nil {
		return err
	}
	if !ok || link.LinkID != t.LinkID {
		return ErrExternalConfirmationExpired
	}
	if _, err := s.enabledLoginProvider(ctx, t.ProviderID); err != nil {
		if errors.Is(err, ErrLoginProviderUnavailable) {
			return ErrExternalConfirmationExpired
		}
		return err
	}
	return nil
}
