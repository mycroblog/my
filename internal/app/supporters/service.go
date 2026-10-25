// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// Roles is what the supporters module needs of the identity module's roles (ADR-0072 §5):
// *identity.Service implements it. The operations that take a transaction run in the one that
// records a subscription's change of state or a plan's change.
type Roles interface {
	// Roles returns every role with its permissions, highest position first.
	Roles(ctx context.Context) ([]appidentity.RoleWithPermissions, error)
	// RolesTx returns every role in tx with whether a module manages it.
	RolesTx(ctx context.Context, tx uow.Tx) ([]appidentity.ManagedRoleView, error)
	// ManagedRoleIDs returns the managed roles, by ID.
	ManagedRoleIDs(ctx context.Context) ([]string, error)
	// UserRoleIDs returns the roles that a user holds, by ID.
	UserRoleIDs(ctx context.Context, userID string) ([]string, error)
	ManageRole(ctx context.Context, tx uow.Tx, key string, manager appidentity.RoleManager, at time.Time) (appidentity.RoleWithPermissions, error)
	UnmanageRole(ctx context.Context, tx uow.Tx, roleID string) (bool, error)
	UpdateManagedRole(ctx context.Context, tx uow.Tx, in appidentity.ManagedRoleUpdate) (appidentity.RoleWithPermissions, bool, error)
	GrantManagedRole(ctx context.Context, tx uow.Tx, userID, roleID string, at time.Time) (bool, error)
	RevokeManagedRole(ctx context.Context, tx uow.Tx, userID, roleID string, at time.Time) (bool, error)
	ReplaceManagedRole(ctx context.Context, tx uow.Tx, userID, fromRoleID, toRoleID string, at time.Time) error
}

// User is what the supporters module reads of a user (ADR-0072 §6).
type User struct {
	ID string
	// AccountID and Handle are of the user's local account; Handle is "@username".
	AccountID string
	Handle    string
	// Email is the primary address, and EmailVerified whether it is verified.
	Email         string
	EmailVerified bool
	// Locale is the user's language, a BCP 47 tag.
	Locale string
	// Suspended reports a suspension in effect, and Closed a deletion, done or scheduled (a ban).
	Suspended bool
	Closed    bool
}

// Users reads the users that subscribe: the platform implements it with the identity, account,
// and moderation modules.
type Users interface {
	// SupporterUser returns a local user; ErrNotFound for an unknown or remote one.
	SupporterUser(ctx context.Context, userID string) (User, error)
	// AccountIDs returns the local accounts of users, by user ID; a user without one is left out.
	AccountIDs(ctx context.Context, userIDs []string) (map[string]string, error)
}

// Accounts finds the local account of an account, and the one of a user, in a transaction:
// *account.Service implements it.
type Accounts interface {
	FindByIDTx(ctx context.Context, tx uow.Tx, id string) (account.Actor, error)
	// LocalAccountIDOfUserTx returns the local account of a user; account.ErrNotFound without one.
	LocalAccountIDOfUserTx(ctx context.Context, tx uow.Tx, userID string) (string, error)
}

// Deps are the ports of Service.
type Deps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	Roles      Roles
	Users      Users
	Accounts   Accounts
	// Provider is the configured payment provider; nil with payments_provider = none, which
	// keeps the program disabled.
	Provider PaymentProvider
	// SendsEmail reports that the server sends email (mail is not none), which enabling the
	// program needs (ADR-0072 §2).
	SendsEmail bool
	// CommerceDisclosure reports whether the instance publishes its commerce disclosures
	// (ADR-0072 §13); nil reports none.
	CommerceDisclosure func(ctx context.Context) (bool, error)
	// Income records the money movements in the finance module; nil records none.
	Income IncomeRecorder
	// Closure stops new subscriptions from the announcement of the service closure on
	// (ADR-0088 §2); nil never stops them.
	Closure ServiceClosure
	Outbox  queue.Outbox
	Clock   kernel.Clock
	IDs     kernel.IDGenerator
	// Logger defaults to slog.Default().
	Logger *slog.Logger
}

// Service runs the supporter program (ADR-0072): its enabling, the plans, the subscriptions,
// and the Syncer that derives their states from the provider's facts.
type Service struct{ d Deps }

var _ Syncer = (*Service)(nil)

// NewService returns a Service on d.
func NewService(d Deps) *Service {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Service{d: d}
}

func (s *Service) now() time.Time { return kernel.Normalize(s.d.Clock.Now()) }

// providerName returns the name of the configured provider, "" without one.
func (s *Service) providerName() string {
	if s.d.Provider == nil {
		return ""
	}
	return s.d.Provider.Name()
}

// The errors of the program.
var (
	// ErrDisabled: the program is disabled; its plans and subscriptions do not show.
	ErrDisabled = errors.New("supporters: the supporter program is disabled")
	// ErrNoProvider: no payment provider is configured (payments_provider = none).
	ErrNoProvider = errors.New("supporters: no payment provider is configured")
	// ErrNoMail: the server sends no email, so nobody can verify an address.
	ErrNoMail = errors.New("supporters: the server sends no email")
	// ErrProviderMismatch: the configured provider is not the one the program was enabled with.
	ErrProviderMismatch = errors.New("supporters: the configured payment provider is not the program's")
	// ErrLiveSubscriptions: the program cannot be disabled while subscriptions go on.
	ErrLiveSubscriptions = errors.New("supporters: subscriptions go on")
)

// Program is the state of the supporter program for the administrators (ADR-0072 §2).
type Program struct {
	Enabled bool
	// Provider is the provider the program was enabled with, "" while disabled.
	Provider string
	// ChangedAt is when it was last enabled or disabled; zero for never.
	ChangedAt time.Time
	// ConfiguredProvider is the provider of payments_provider, "" for none.
	ConfiguredProvider string
	// SendsEmail reports that the server sends email.
	SendsEmail bool
	// CommerceDisclosure reports that the instance publishes its commerce disclosures.
	CommerceDisclosure bool
	// LiveSubscriptions counts the subscriptions that go on.
	LiveSubscriptions int64
}

// CanEnable reports whether the program can be enabled: a provider and email.
func (p Program) CanEnable() bool { return p.ConfiguredProvider != "" && p.SendsEmail }

// ProviderMismatch reports an enabled program whose provider is not the configured one: new
// subscriptions and changes are refused until it is disabled and enabled again.
func (p Program) ProviderMismatch() bool { return p.Enabled && p.Provider != p.ConfiguredProvider }

// lastChange returns the newest program change, the zero value (disabled) without one.
func (s *Service) lastChange(ctx context.Context, tx uow.Tx) (ProgramChange, error) {
	c, err := s.d.Repository.LastProgramChange(ctx, tx)
	if errors.Is(err, ErrNotFound) {
		return ProgramChange{}, nil
	}
	return c, err
}

// Program returns the state of the program.
func (s *Service) Program(ctx context.Context) (Program, error) {
	c, err := s.lastChange(ctx, nil)
	if err != nil {
		return Program{}, err
	}
	p := Program{Enabled: c.Enabled, Provider: c.Provider, ChangedAt: c.ChangedAt, ConfiguredProvider: s.providerName(), SendsEmail: s.d.SendsEmail}
	if s.d.CommerceDisclosure != nil {
		if p.CommerceDisclosure, err = s.d.CommerceDisclosure(ctx); err != nil {
			return Program{}, err
		}
	}
	if p.LiveSubscriptions, err = s.d.Repository.CountLiveSubscriptions(ctx, nil); err != nil {
		return Program{}, err
	}
	return p, nil
}

// Enabled reports whether the program is enabled: its plans show and users may subscribe. It
// is false while disabled, and it stays true with a mismatched provider so that the subscribers
// keep their screens (accepting refuses the new subscriptions).
func (s *Service) Enabled(ctx context.Context) (bool, error) {
	c, err := s.lastChange(ctx, nil)
	return c.Enabled, err
}

// accepting fails with ErrDisabled while the program is disabled, and with ErrProviderMismatch
// when the configured provider is not the program's: new subscriptions and changes need both.
func (s *Service) accepting(ctx context.Context) error {
	c, err := s.lastChange(ctx, nil)
	switch {
	case err != nil:
		return err
	case !c.Enabled:
		return ErrDisabled
	case s.d.Provider == nil || c.Provider != s.d.Provider.Name():
		return ErrProviderMismatch
	}
	return nil
}

// SetProgram enables or disables the program as byUserID ("" for the CLI) and returns its
// state, audited. Enabling needs a provider (ErrNoProvider) and email (ErrNoMail); disabling is
// refused while subscriptions go on (ErrLiveSubscriptions, D1): retire the plans first. Asking
// for the state the program is in records nothing.
func (s *Service) SetProgram(ctx context.Context, enabled bool, byUserID string) (Program, error) {
	if enabled {
		switch {
		case s.d.Provider == nil:
			return Program{}, ErrNoProvider
		case !s.d.SendsEmail:
			return Program{}, ErrNoMail
		}
	}
	now := s.now()
	id, auditID := s.d.IDs.NewID(), s.d.IDs.NewID()
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		cur, err := s.lastChange(ctx, tx)
		if err != nil {
			return err
		}
		c := ProgramChange{ID: id, Enabled: enabled, ChangedByUserID: byUserID, ChangedAt: now}
		action, label := appmoderation.AuditSupporterProgramDisabled, cur.Provider
		if enabled {
			if cur.Enabled && cur.Provider == s.d.Provider.Name() {
				return nil
			}
			c.Provider, action, label = s.d.Provider.Name(), appmoderation.AuditSupporterProgramEnabled, s.d.Provider.Name()
		} else {
			if !cur.Enabled {
				return nil
			}
			n, err := s.d.Repository.CountLiveSubscriptions(ctx, tx)
			if err != nil {
				return err
			}
			if n > 0 {
				return fmt.Errorf("%w: %d", ErrLiveSubscriptions, n)
			}
		}
		if err := s.d.Repository.InsertProgramChange(ctx, tx, c); err != nil {
			return err
		}
		return tx.Emit(ctx, audit(auditID, byUserID, action, appmoderation.AuditTargetSupporterProgram, c.ID, label, c.ID, now))
	})
	if err != nil {
		return Program{}, err
	}
	return s.Program(ctx)
}

// audit returns an audit log entry of an administrator's operation (ADR-0072 §16).
func audit(id, byUserID string, action appmoderation.AuditAction, kind appmoderation.AuditTargetKind, targetID, label, sourceID string, at time.Time) appmoderation.AuditRecorded {
	actor, user := appmoderation.AuditActor(byUserID)
	return appmoderation.AuditRecorded{Entry: appmoderation.AuditEntry{
		ID: id, ActorKind: actor, ActorUserID: user, Action: action, TargetKind: kind,
		TargetID: targetID, TargetLabel: label, SourceEventID: sourceID, OccurredAt: at,
	}}
}
