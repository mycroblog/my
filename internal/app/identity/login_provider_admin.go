// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The administration of the external login providers (ADR-0106 §2): an administrator with
// manage_login_providers, or the CLI, sets a provider up, revises its settings, gives it a new
// client secret, enables, disables, and deletes it. Every operation is audited. The client
// secret is entered on creation and rotation only and never returned.

// LoginProviderView is a provider with the number of users linked to it, which disabling or
// deleting it keeps from signing in with it.
type LoginProviderView struct {
	Provider    identity.LoginProvider
	LinkedUsers int
}

// IsLoginProviderValidation reports whether err is a rejection of the settings or the client
// secret of a provider.
func IsLoginProviderValidation(err error) bool {
	for _, target := range []error{
		identity.ErrInvalidLoginProviderKind, identity.ErrInvalidLoginProviderName, identity.ErrInvalidLoginProviderURL,
		identity.ErrInvalidLoginProviderClientID, identity.ErrInvalidLoginProviderScopes,
		identity.ErrInvalidLoginProviderConditions, identity.ErrLoginProviderKindChanged,
		identity.ErrInvalidLoginProviderClientSecret,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// sealLoginProviderSecretAAD is the additional authenticated data of a sealed client secret:
// its row and column.
func sealLoginProviderSecretAAD(secretID string) []byte {
	return kernel.AAD(secretID, identity.LoginProviderSecretColumn)
}

// ListLoginProviders returns the providers that are not deleted, newest first.
func (s *Service) ListLoginProviders(ctx context.Context) ([]LoginProviderView, error) {
	ps, err := s.d.Repository.ListLoginProviders(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := s.d.Repository.CountLoginProviderLinks(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]LoginProviderView, len(ps))
	for i, p := range ps {
		out[i] = LoginProviderView{Provider: p, LinkedUsers: counts[p.ID]}
	}
	return out, nil
}

// LoginProvider returns a provider; it fails with ErrNotFound for an unknown or deleted one.
func (s *Service) LoginProvider(ctx context.Context, id string) (LoginProviderView, error) {
	p, err := s.d.Repository.FindLoginProvider(ctx, id)
	if err != nil {
		return LoginProviderView{}, err
	}
	counts, err := s.d.Repository.CountLoginProviderLinks(ctx)
	if err != nil {
		return LoginProviderView{}, err
	}
	return LoginProviderView{Provider: p, LinkedUsers: counts[p.ID]}, nil
}

// sealLoginProviderSecret checks a client secret and returns its sealed row.
func (s *Service) sealLoginProviderSecret(ctx context.Context, providerID, secret, byUserID string, now time.Time) (identity.LoginProviderSecret, error) {
	secret = identity.NormalizeLoginProviderClientSecret(secret)
	if err := identity.ValidateLoginProviderClientSecret(secret); err != nil {
		return identity.LoginProviderSecret{}, err
	}
	if s.d.Sealer == nil {
		return identity.LoginProviderSecret{}, errors.New("identity: no sealer for the client secrets of login providers")
	}
	row := identity.LoginProviderSecret{ID: s.d.IDs.NewID(), ProviderID: providerID, CreatedByUserID: byUserID, CreatedAt: now}
	sealed, err := s.d.Sealer.Seal(ctx, []byte(secret), sealLoginProviderSecretAAD(row.ID))
	if err != nil {
		return identity.LoginProviderSecret{}, fmt.Errorf("identity: seal the client secret: %w", err)
	}
	row.Sealed = sealed
	return row, nil
}

// emitLoginProviderAudit emits the audit log entry of an operation on a provider, labeled with
// its display name; byUserID "" is the CLI.
func (s *Service) emitLoginProviderAudit(ctx context.Context, tx uow.Tx, action appmoderation.AuditAction, providerID, label, source, byUserID string, at time.Time) error {
	entry := appmoderation.AuditEntry{
		ID: s.d.IDs.NewID(), Action: action, TargetKind: appmoderation.AuditTargetLoginProvider, TargetID: providerID,
		TargetLabel: label, SourceEventID: source, OccurredAt: at,
	}
	entry.ActorKind, entry.ActorUserID = appmoderation.AuditActor(byUserID)
	return tx.Emit(ctx, appmoderation.AuditRecorded{Entry: entry})
}

// CreateLoginProvider sets up a provider, enabled, with settings and its client secret, and
// audits it. byUserID is the administrator, "" for the CLI. It fails with the errors of
// identity.LoginProviderSettings.Validate and with ErrInvalidLoginProviderClientSecret.
func (s *Service) CreateLoginProvider(ctx context.Context, settings identity.LoginProviderSettings, clientSecret, byUserID string) (LoginProviderView, error) {
	settings = settings.Normalize()
	if err := settings.Validate(); err != nil {
		return LoginProviderView{}, err
	}
	now := kernel.Normalize(s.d.Clock.Now())
	id := s.d.IDs.NewID()
	secret, err := s.sealLoginProviderSecret(ctx, id, clientSecret, byUserID, now)
	if err != nil {
		return LoginProviderView{}, err
	}
	rev := identity.LoginProviderRevision{ID: s.d.IDs.NewID(), ProviderID: id, Settings: settings, RevisedByUserID: byUserID, RevisedAt: now}
	change := identity.LoginProviderStateChange{
		ID: s.d.IDs.NewID(), ProviderID: id, State: identity.LoginProviderEnabled, ChangedByUserID: byUserID, ChangedAt: now,
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		repo := s.d.Repository
		if err := repo.InsertLoginProvider(ctx, tx, id, byUserID, now); err != nil {
			return err
		}
		if err := repo.InsertLoginProviderRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := repo.InsertLoginProviderSecret(ctx, tx, secret); err != nil {
			return err
		}
		if err := repo.InsertLoginProviderStateChange(ctx, tx, change); err != nil {
			return err
		}
		head := identity.LoginProviderHead{ProviderID: id, RevisionID: rev.ID, State: identity.LoginProviderEnabled, SecretID: secret.ID}
		if err := repo.UpsertLoginProviderHead(ctx, tx, head); err != nil {
			return err
		}
		return s.emitLoginProviderAudit(ctx, tx, appmoderation.AuditLoginProviderCreated, id, settings.DisplayName, rev.ID, byUserID, now)
	})
	if err != nil {
		return LoginProviderView{}, err
	}
	return s.LoginProvider(ctx, id)
}

// currentLoginProvider returns the head of a provider in tx, failing with ErrNotFound for a
// deleted one.
func (s *Service) currentLoginProvider(ctx context.Context, tx uow.Tx, id string) (identity.LoginProviderHead, error) {
	h, err := s.d.Repository.LoginProviderHeadForUpdate(ctx, tx, id)
	if err != nil {
		return identity.LoginProviderHead{}, err
	}
	if h.State == identity.LoginProviderDeleted {
		return identity.LoginProviderHead{}, fmt.Errorf("%w: login provider %s", ErrNotFound, id)
	}
	return h, nil
}

// UpdateLoginProvider replaces the settings of a provider with a new revision, unless they are
// the same, and audits it. The kind stays (identity.ErrLoginProviderKindChanged). It fails as
// CreateLoginProvider does, and with ErrNotFound.
func (s *Service) UpdateLoginProvider(ctx context.Context, id string, settings identity.LoginProviderSettings, byUserID string) (LoginProviderView, error) {
	old, err := s.d.Repository.FindLoginProvider(ctx, id)
	if err != nil {
		return LoginProviderView{}, err
	}
	settings = settings.Normalize()
	if err := identity.ValidateLoginProviderChange(old.Settings, settings); err != nil {
		return LoginProviderView{}, err
	}
	if err := settings.Validate(); err != nil {
		return LoginProviderView{}, err
	}
	if reflect.DeepEqual(old.Settings.Normalize(), settings) {
		return s.LoginProvider(ctx, id)
	}
	now := kernel.Normalize(s.d.Clock.Now())
	rev := identity.LoginProviderRevision{ID: s.d.IDs.NewID(), ProviderID: id, Settings: settings, RevisedByUserID: byUserID, RevisedAt: now}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		h, err := s.currentLoginProvider(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.d.Repository.InsertLoginProviderRevision(ctx, tx, rev); err != nil {
			return err
		}
		h.RevisionID = rev.ID
		if err := s.d.Repository.UpsertLoginProviderHead(ctx, tx, h); err != nil {
			return err
		}
		return s.emitLoginProviderAudit(ctx, tx, appmoderation.AuditLoginProviderUpdated, id, settings.DisplayName, rev.ID, byUserID, now)
	})
	if err != nil {
		return LoginProviderView{}, err
	}
	return s.LoginProvider(ctx, id)
}

// RotateLoginProviderSecret replaces the client secret of a provider with clientSecret, which
// the administrator got from the provider, and audits it. It fails with
// identity.ErrInvalidLoginProviderClientSecret and ErrNotFound.
func (s *Service) RotateLoginProviderSecret(ctx context.Context, id, clientSecret, byUserID string) (LoginProviderView, error) {
	p, err := s.d.Repository.FindLoginProvider(ctx, id)
	if err != nil {
		return LoginProviderView{}, err
	}
	now := kernel.Normalize(s.d.Clock.Now())
	secret, err := s.sealLoginProviderSecret(ctx, id, clientSecret, byUserID, now)
	if err != nil {
		return LoginProviderView{}, err
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		h, err := s.currentLoginProvider(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.d.Repository.InsertLoginProviderSecret(ctx, tx, secret); err != nil {
			return err
		}
		h.SecretID = secret.ID
		if err := s.d.Repository.UpsertLoginProviderHead(ctx, tx, h); err != nil {
			return err
		}
		return s.emitLoginProviderAudit(ctx, tx, appmoderation.AuditLoginProviderSecretRotated, id, p.Settings.DisplayName, secret.ID, byUserID, now)
	})
	if err != nil {
		return LoginProviderView{}, err
	}
	return s.LoginProvider(ctx, id)
}

// EnableLoginProvider enables a provider; an enabled one stays as it is.
func (s *Service) EnableLoginProvider(ctx context.Context, id, byUserID string) (LoginProviderView, error) {
	return s.changeLoginProviderState(ctx, id, identity.LoginProviderEnabled, byUserID)
}

// DisableLoginProvider disables a provider: nobody signs in, signs up, or links with it, and
// the sign-in screen no longer shows it. The links stay and work again once it is enabled. A
// disabled provider stays as it is.
func (s *Service) DisableLoginProvider(ctx context.Context, id, byUserID string) (LoginProviderView, error) {
	return s.changeLoginProviderState(ctx, id, identity.LoginProviderDisabled, byUserID)
}

// DeleteLoginProvider deletes a provider for good; it returns the provider as it was, with the
// number of users that were linked to it.
func (s *Service) DeleteLoginProvider(ctx context.Context, id, byUserID string) (LoginProviderView, error) {
	return s.changeLoginProviderState(ctx, id, identity.LoginProviderDeleted, byUserID)
}

var loginProviderStateActions = map[identity.LoginProviderState]appmoderation.AuditAction{
	identity.LoginProviderEnabled:  appmoderation.AuditLoginProviderEnabled,
	identity.LoginProviderDisabled: appmoderation.AuditLoginProviderDisabled,
	identity.LoginProviderDeleted:  appmoderation.AuditLoginProviderDeleted,
}

func (s *Service) changeLoginProviderState(ctx context.Context, id string, state identity.LoginProviderState, byUserID string) (LoginProviderView, error) {
	old, err := s.LoginProvider(ctx, id)
	if err != nil {
		return LoginProviderView{}, err
	}
	now := kernel.Normalize(s.d.Clock.Now())
	change := identity.LoginProviderStateChange{ID: s.d.IDs.NewID(), ProviderID: id, State: state, ChangedByUserID: byUserID, ChangedAt: now}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		h, err := s.currentLoginProvider(ctx, tx, id)
		if err != nil || h.State == state {
			return err
		}
		if err := s.d.Repository.InsertLoginProviderStateChange(ctx, tx, change); err != nil {
			return err
		}
		h.State = state
		if err := s.d.Repository.UpsertLoginProviderHead(ctx, tx, h); err != nil {
			return err
		}
		return s.emitLoginProviderAudit(ctx, tx, loginProviderStateActions[state], id, old.Provider.Settings.DisplayName, change.ID, byUserID, now)
	})
	if err != nil {
		return LoginProviderView{}, err
	}
	if state == identity.LoginProviderDeleted {
		old.Provider.State, old.Provider.StateChangedAt = state, now
		return old, nil
	}
	return s.LoginProvider(ctx, id)
}

// ExistingLoginProviderIDs returns, in the order of ids, the providers among ids that are not
// deleted.
func (s *Service) ExistingLoginProviderIDs(ctx context.Context, ids []string) ([]string, error) {
	ps, err := s.d.Repository.ListLoginProviders(ctx)
	if err != nil {
		return nil, err
	}
	live := make(map[string]bool, len(ps))
	for _, p := range ps {
		live[p.ID] = true
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if live[id] {
			out = append(out, id)
		}
	}
	return out, nil
}
