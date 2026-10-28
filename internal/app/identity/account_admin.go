// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The moderators' account management (FR-714): searching the local and remote accounts, the
// details of one, and showing the email address and the IP addresses of a local one. It lives
// here because it combines what this module knows of users (registration, email, roles, where
// they signed in from) with the account module and the moderation module, which this module
// already reads. The email address and the IP addresses are only ever returned by Reveal, which
// records each call in the audit log (ADR-0037, NFR-MOD-5).

// AdminAccountState is the state of an account as moderators see it, derived from the account's
// lifecycle, its own moderation actions, and its registration, in this order of precedence:
// deleted, suspended (suspend, ban), disabled (disable_login), rejected and pending (the
// registration), limited (limit), and active.
type AdminAccountState string

// States of accounts.
const (
	AdminAccountActive    AdminAccountState = "active"
	AdminAccountPending   AdminAccountState = "pending"
	AdminAccountRejected  AdminAccountState = "rejected"
	AdminAccountLimited   AdminAccountState = "limited"
	AdminAccountDisabled  AdminAccountState = "disabled"
	AdminAccountSuspended AdminAccountState = "suspended"
	AdminAccountDeleted   AdminAccountState = "deleted"
)

// Valid reports whether s is one of the states.
func (s AdminAccountState) Valid() bool {
	switch s {
	case AdminAccountActive, AdminAccountPending, AdminAccountRejected, AdminAccountLimited, AdminAccountDisabled,
		AdminAccountSuspended, AdminAccountDeleted:
		return true
	}
	return false
}

// Limits of the account management.
const (
	// MaxAccountSearchScan bounds the accounts that one search reads to fill a page: a search
	// that reads them without filling the page returns what it found with the position to
	// continue from, so a page may be short, or empty, and still have a next one.
	MaxAccountSearchScan = 400
	// MaxRevealedSignIns is the number of the newest sign-ins that Reveal shows.
	MaxRevealedSignIns = 20
	// MaxSameIPAccounts is the number of other accounts per IP address that Reveal shows.
	MaxSameIPAccounts = 20
	// AccountDetailHistory is the number of the newest actions and reports that the details of
	// an account show.
	AccountDetailHistory = 20
)

var (
	// ErrNotLocalAccount reports a reveal of an account that is not a local user's: a remote
	// account or the instance actor, which have no email address and no sign-ins.
	ErrNotLocalAccount = errors.New("identity: not the account of a local user")
)

// AccountSearchError reports a search query whose Param is invalid.
type AccountSearchError struct{ Param string }

func (e *AccountSearchError) Error() string { return "identity: invalid account search: " + e.Param }

// AccountRestrictions reads what the actions on accounts impose. *appmoderation.AccountPolicy
// implements it.
type AccountRestrictions interface {
	OwnRestrictions(ctx context.Context, accountID string) ([]appmoderation.Restriction, error)
	// OwnRestrictionsTx answers as OwnRestrictions does, read in tx with what tx has written.
	OwnRestrictionsTx(ctx context.Context, tx uow.Tx, accountID string) ([]appmoderation.Restriction, error)
	RestrictedAccounts(ctx context.Context, r appmoderation.Restriction, before string, limit int) ([]string, error)
}

// AccountAdminRepository is the part of Repository that the account management reads. The
// database adapters implement it and pass testkit.AdminAccountContract.
type AccountAdminRepository interface {
	// ListUserIDsByEmailDomain returns up to limit users whose primary address is at domain
	// (lowercase), newest first, by ID, whose ID is below before ("" for the first page).
	ListUserIDsByEmailDomain(ctx context.Context, domain, before string, limit int) ([]string, error)
	// ListUserIDsByIP returns up to limit users who registered or signed in from ip (the form
	// that the origins record), newest first, by ID, whose ID is below before.
	ListUserIDsByIP(ctx context.Context, ip, before string, limit int) ([]string, error)
	// ListUserIDsInRegistrationState returns up to limit users whose registration is in state,
	// newest first, by ID, whose ID is below before.
	ListUserIDsInRegistrationState(ctx context.Context, state identity.RegistrationState, before string, limit int) ([]string, error)
	// ListSessionOriginsOfUser returns the newest limit origins of a user's sessions, newest
	// first.
	ListSessionOriginsOfUser(ctx context.Context, userID string, limit int) ([]identity.SessionOrigin, error)
}

// AccountAdminDeps are the ports and collaborators of the account management.
type AccountAdminDeps struct {
	UnitOfWork   uow.UnitOfWork
	Repository   Repository
	Accounts     *account.Service
	Restrictions AccountRestrictions
	Clock        kernel.Clock
	IDs          kernel.IDGenerator
}

// AccountAdmin runs the moderators' account management.
type AccountAdmin struct{ d AccountAdminDeps }

// NewAccountAdmin returns the account management on d.
func NewAccountAdmin(d AccountAdminDeps) *AccountAdmin { return &AccountAdmin{d: d} }

// AdminAccount is an account as the search and the details present it. It holds no email
// address and no IP address.
type AdminAccount struct {
	Actor account.Actor
	State AdminAccountState
	// Restrictions are those that the account's own actions impose now, in restriction order;
	// those of its domain are the domain actions'.
	Restrictions []appmoderation.Restriction
	// UserID is the user of a local account, "" otherwise.
	UserID string
	// Registration is where the user's registration stands; nil for an account without a user
	// and a user created by the CLI, who is approved.
	Registration *identity.UserRegistrationState
}

// AdminAccountDetail is the details of an account (GET /api/v1/admin/accounts/{id}).
type AdminAccountDetail struct {
	AdminAccount
	// Application is the user's registration application; nil without one.
	Application *identity.RegistrationApplication
	// Invite is the invite the user signed up with, whose creator invited them (ADR-0108 §5);
	// nil without one.
	Invite *identity.Invite
	// HasEmail reports whether the user has a primary email address, and EmailVerified whether
	// it is confirmed; the address itself is Reveal's.
	HasEmail      bool
	EmailVerified bool
	// EmailSuppression is the suppression of the primary address (ADR-0086 §5); nil when it
	// is not suppressed.
	EmailSuppression *identity.EmailSuppression
	// Roles are the user's roles, highest position first.
	Roles []identity.Role
	// TwoFactorEnabled reports whether the user signs in with two-factor authentication.
	TwoFactorEnabled bool
}

// SameIPAccounts are the other accounts that registered or signed in from one IP address.
type SameIPAccounts struct {
	IP       string
	Accounts []AdminAccount
}

// PrivateData is what Reveal shows of a local account.
type PrivateData struct {
	// Email is the primary email address, "" without one, and EmailVerified whether it is
	// confirmed.
	Email         string
	EmailVerified bool
	// Registration is where the registration came from; nil when it is unknown or purged.
	Registration *identity.RegistrationOrigin
	// SignIns are the newest MaxRevealedSignIns origins of the user's sessions, newest first.
	SignIns []identity.SessionOrigin
	// SameIP lists, for each IP address of Registration and SignIns (newest first), up to
	// MaxSameIPAccounts other accounts that used it, newest first.
	SameIP []SameIPAccounts
}

// AccountSearch selects the accounts of a search (GET /api/v1/admin/accounts). Empty fields
// select every account; the instance actor is never selected. At most one of Email,
// EmailDomain, and IP may be given; they select local accounts only.
type AccountSearch struct {
	// Username keeps the accounts whose handle's username starts with it, case-insensitively.
	Username string
	// Domain keeps the remote accounts of a domain.
	Domain string
	// Origin keeps the local (account.OriginLocal) or remote (account.OriginRemote) accounts.
	Origin string
	// State keeps the accounts in a state.
	State AdminAccountState
	// Email keeps the user whose primary address is Email, case-insensitively.
	Email string
	// EmailDomain keeps the users whose primary address is at a domain.
	EmailDomain string
	// IP keeps the users who registered or signed in from an IP address.
	IP string
	// After is the position after which the page starts (a next position that Search
	// returned); "" starts from the newest.
	After string
}

// candidate is one account that a search source yields, at its position (an account ID or, for
// the sources of this module, a user ID).
type candidate struct{ key, accountID string }

// source yields up to limit candidates newest first, after the position before ("" for the
// first page).
type source func(ctx context.Context, before string, limit int) ([]candidate, error)

// Search returns up to limit accounts that q selects, newest first, and the position of the next
// page ("" on the last page). It fails with an *AccountSearchError naming an invalid parameter.
// The search reads one source in pages (the accounts of an email address, an email domain, an IP
// address, a state, or else every account) and checks the other conditions of each account.
func (s *AccountAdmin) Search(ctx context.Context, q AccountSearch, limit int) ([]AdminAccount, string, error) {
	src, err := s.source(q)
	if err != nil {
		return nil, "", err
	}
	type found struct {
		key string
		a   AdminAccount
	}
	var items []found
	before, scanned, exhausted := q.After, 0, false
scan:
	for {
		batch := limit + 1
		cs, err := src(ctx, before, batch)
		if err != nil {
			return nil, "", err
		}
		for _, c := range cs {
			scanned++
			before = c.key
			a, ok, err := s.match(ctx, c.accountID, q)
			if err != nil {
				return nil, "", err
			}
			if ok {
				if items = append(items, found{c.key, a}); len(items) > limit {
					break scan
				}
			}
		}
		if len(cs) < batch {
			exhausted = true
			break
		}
		if scanned >= MaxAccountSearchScan {
			break
		}
	}
	next := ""
	switch {
	case len(items) > limit:
		items = items[:limit]
		next = items[limit-1].key
	case !exhausted:
		next = before
	}
	out := make([]AdminAccount, len(items))
	for i, f := range items {
		out[i] = f.a
	}
	return out, next, nil
}

// source returns the source that a search reads.
func (s *AccountAdmin) source(q AccountSearch) (source, error) {
	given := 0
	for _, v := range []string{q.Email, q.EmailDomain, q.IP} {
		if v != "" {
			given++
		}
	}
	if given > 1 {
		param := "email_domain"
		if q.IP != "" {
			param = "ip"
		}
		return nil, &AccountSearchError{Param: param}
	}
	switch q.Origin {
	case "", account.OriginLocal, account.OriginRemote:
	default:
		return nil, &AccountSearchError{Param: "origin"}
	}
	if q.State != "" && !q.State.Valid() {
		return nil, &AccountSearchError{Param: "state"}
	}
	repo := s.d.Repository
	switch {
	case q.Email != "":
		email := identity.NormalizeEmail(q.Email)
		return s.users(func(ctx context.Context, before string, _ int) ([]string, error) {
			id, err := repo.FindUserIDByEmail(ctx, email)
			switch {
			case errors.Is(err, ErrNotFound):
				return nil, nil
			case err != nil:
				return nil, err
			case before != "" && id >= before:
				return nil, nil
			}
			return []string{id}, nil
		}), nil
	case q.EmailDomain != "":
		domain := strings.ToLower(strings.TrimPrefix(q.EmailDomain, "@"))
		if domain == "" || strings.Contains(domain, "@") {
			return nil, &AccountSearchError{Param: "email_domain"}
		}
		return s.users(func(ctx context.Context, before string, limit int) ([]string, error) {
			return repo.ListUserIDsByEmailDomain(ctx, domain, before, limit)
		}), nil
	case q.IP != "":
		ip := normalizeIP(q.IP)
		if ip == "" {
			return nil, &AccountSearchError{Param: "ip"}
		}
		return s.users(func(ctx context.Context, before string, limit int) ([]string, error) {
			return repo.ListUserIDsByIP(ctx, ip, before, limit)
		}), nil
	}
	switch q.State {
	case AdminAccountPending, AdminAccountRejected:
		state := identity.RegistrationPending
		if q.State == AdminAccountRejected {
			state = identity.RegistrationRejected
		}
		return s.users(func(ctx context.Context, before string, limit int) ([]string, error) {
			return repo.ListUserIDsInRegistrationState(ctx, state, before, limit)
		}), nil
	case AdminAccountLimited, AdminAccountDisabled, AdminAccountSuspended:
		r := map[AdminAccountState]appmoderation.Restriction{
			AdminAccountLimited:   appmoderation.RestrictVisibilityCeilingPrivate,
			AdminAccountDisabled:  appmoderation.RestrictLogin,
			AdminAccountSuspended: appmoderation.RestrictProfileView,
		}[q.State]
		return accounts(func(ctx context.Context, before string, limit int) ([]string, error) {
			return s.d.Restrictions.RestrictedAccounts(ctx, r, before, limit)
		}), nil
	case AdminAccountDeleted:
		return accounts(func(ctx context.Context, before string, limit int) ([]string, error) {
			return s.d.Accounts.DeletedAccounts(ctx, before, limit)
		}), nil
	}
	return accounts(func(ctx context.Context, before string, limit int) ([]string, error) {
		return s.d.Accounts.Search(ctx, account.SearchQuery{
			UsernamePrefix: q.Username, Domain: q.Domain, Origin: q.Origin, Before: before,
		}, limit)
	}), nil
}

// accounts makes a source of account IDs.
func accounts(list func(ctx context.Context, before string, limit int) ([]string, error)) source {
	return func(ctx context.Context, before string, limit int) ([]candidate, error) {
		ids, err := list(ctx, before, limit)
		if err != nil {
			return nil, err
		}
		out := make([]candidate, len(ids))
		for i, id := range ids {
			out[i] = candidate{key: id, accountID: id}
		}
		return out, nil
	}
}

// users makes a source of the local accounts of user IDs; a user without one yields an account
// that matches nothing.
func (s *AccountAdmin) users(list func(ctx context.Context, before string, limit int) ([]string, error)) source {
	return func(ctx context.Context, before string, limit int) ([]candidate, error) {
		ids, err := list(ctx, before, limit)
		if err != nil {
			return nil, err
		}
		out := make([]candidate, len(ids))
		for i, id := range ids {
			la, err := s.d.Accounts.LocalAccountOfUser(ctx, id)
			if err != nil && !errors.Is(err, account.ErrNotFound) {
				return nil, err
			}
			out[i] = candidate{key: id, accountID: la.AccountID}
		}
		return out, nil
	}
}

// match returns the account accountID and whether it meets the conditions of q that its source
// does not check.
func (s *AccountAdmin) match(ctx context.Context, accountID string, q AccountSearch) (AdminAccount, bool, error) {
	if accountID == "" {
		return AdminAccount{}, false, nil
	}
	actor, err := s.d.Accounts.FindByID(ctx, accountID)
	switch {
	case errors.Is(err, account.ErrNotFound):
		return AdminAccount{}, false, nil
	case err != nil:
		return AdminAccount{}, false, err
	case actor.IsInstanceActor():
		return AdminAccount{}, false, nil
	}
	local := actor.Account.IsLocal()
	switch {
	case q.Origin == account.OriginLocal && !local, q.Origin == account.OriginRemote && local:
		return AdminAccount{}, false, nil
	case q.Domain != "" && actor.Account.Domain != account.NormalizeDomain(q.Domain):
		return AdminAccount{}, false, nil
	case q.Username != "" && !strings.HasPrefix(username(actor), account.NormalizeUsername(q.Username)):
		return AdminAccount{}, false, nil
	}
	a, err := s.present(ctx, actor)
	if err != nil {
		return AdminAccount{}, false, err
	}
	return a, q.State == "" || a.State == q.State, nil
}

// username returns the normalized username of an account's handle, "" without one.
func username(a account.Actor) string {
	switch {
	case a.Local != nil:
		return a.Local.UsernameNormalized
	case a.Handle != nil:
		return a.Handle.UsernameNormalized
	}
	return ""
}

// present returns an account with its state as moderators see it.
func (s *AccountAdmin) present(ctx context.Context, actor account.Actor) (AdminAccount, error) {
	out := AdminAccount{Actor: actor}
	rs, err := s.d.Restrictions.OwnRestrictions(ctx, actor.Account.ID)
	if err != nil {
		return AdminAccount{}, fmt.Errorf("identity: restrictions of %s: %w", actor.Account.ID, err)
	}
	out.Restrictions = rs
	if actor.Local != nil && actor.Local.UserID != "" {
		out.UserID = actor.Local.UserID
		st, err := s.d.Repository.FindRegistrationState(ctx, out.UserID)
		switch {
		case err == nil:
			out.Registration = &st
		case !errors.Is(err, ErrNotFound):
			return AdminAccount{}, err
		}
	}
	out.State = adminState(out)
	return out, nil
}

// adminState derives the state of an account in the order of precedence of AdminAccountState.
func adminState(a AdminAccount) AdminAccountState {
	has := func(r appmoderation.Restriction) bool { return slices.Contains(a.Restrictions, r) }
	switch {
	case a.Actor.Deleted():
		return AdminAccountDeleted
	case has(appmoderation.RestrictProfileView):
		return AdminAccountSuspended
	case has(appmoderation.RestrictLogin):
		return AdminAccountDisabled
	case a.Registration != nil && a.Registration.State == identity.RegistrationRejected:
		return AdminAccountRejected
	case a.Registration != nil && a.Registration.State == identity.RegistrationPending:
		return AdminAccountPending
	case has(appmoderation.RestrictVisibilityCeilingPrivate):
		return AdminAccountLimited
	}
	return AdminAccountActive
}

// Account returns an account, local or remote, as moderators see it, without its details. It
// fails with account.ErrNotFound for an unknown account.
func (s *AccountAdmin) Account(ctx context.Context, accountID string) (AdminAccount, error) {
	actor, err := s.d.Accounts.FindByID(ctx, accountID)
	if err != nil {
		return AdminAccount{}, err
	}
	return s.present(ctx, actor)
}

// Detail returns the details of an account, local or remote. It fails with account.ErrNotFound
// for an unknown account.
func (s *AccountAdmin) Detail(ctx context.Context, accountID string) (AdminAccountDetail, error) {
	a, err := s.Account(ctx, accountID)
	if err != nil {
		return AdminAccountDetail{}, err
	}
	out := AdminAccountDetail{AdminAccount: a, Roles: []identity.Role{}}
	if a.UserID == "" {
		return out, nil
	}
	repo := s.d.Repository
	app, err := repo.FindRegistrationApplication(ctx, a.UserID)
	switch {
	case err == nil:
		out.Application = &app
	case !errors.Is(err, ErrNotFound):
		return AdminAccountDetail{}, err
	}
	if out.Application != nil && out.Application.InviteID != "" {
		inv, err := repo.FindInviteByID(ctx, out.Application.InviteID)
		switch {
		case err == nil:
			out.Invite = &inv
		case !errors.Is(err, ErrNotFound):
			return AdminAccountDetail{}, err
		}
	}
	email, verified, err := repo.FindPrimaryEmail(ctx, a.UserID)
	switch {
	case err == nil:
		out.HasEmail, out.EmailVerified = true, verified
		sup, ok, err := findSuppression(ctx, repo, email.ID)
		if err != nil {
			return AdminAccountDetail{}, err
		}
		if ok {
			out.EmailSuppression = &sup
		}
	case !errors.Is(err, ErrNotFound):
		return AdminAccountDetail{}, err
	}
	if out.Roles, err = repo.ListUserRoles(ctx, a.UserID); err != nil {
		return AdminAccountDetail{}, err
	}
	switch _, err := repo.FindActiveTOTPEnrollment(ctx, a.UserID); {
	case err == nil:
		out.TwoFactorEnabled = true
	case !errors.Is(err, ErrNotFound):
		return AdminAccountDetail{}, err
	}
	return out, nil
}

// Reveal returns the email address and the IP addresses of a local user's account, with the
// other accounts that used the same IP addresses, for byUserID ("" for the CLI). The call is
// recorded in the audit log (revealed_private_data) in a transaction of its own before anything
// is returned: when the entry cannot be written, nothing is shown. It fails with
// account.ErrNotFound for an unknown account and ErrNotLocalAccount for an account without a
// user.
func (s *AccountAdmin) Reveal(ctx context.Context, accountID, byUserID string) (PrivateData, error) {
	actor, err := s.d.Accounts.FindByID(ctx, accountID)
	if err != nil {
		return PrivateData{}, err
	}
	if actor.Local == nil || actor.Local.UserID == "" {
		return PrivateData{}, ErrNotLocalAccount
	}
	userID, repo := actor.Local.UserID, s.d.Repository
	out := PrivateData{SignIns: []identity.SessionOrigin{}, SameIP: []SameIPAccounts{}}
	email, verified, err := repo.FindPrimaryEmail(ctx, userID)
	switch {
	case err == nil:
		out.Email, out.EmailVerified = email.Address, verified
	case !errors.Is(err, ErrNotFound):
		return PrivateData{}, err
	}
	reg, err := repo.FindRegistrationOrigin(ctx, userID)
	switch {
	case err == nil:
		out.Registration = &reg
	case !errors.Is(err, ErrNotFound):
		return PrivateData{}, err
	}
	if out.SignIns, err = repo.ListSessionOriginsOfUser(ctx, userID, MaxRevealedSignIns); err != nil {
		return PrivateData{}, err
	}
	if out.SameIP, err = s.sameIP(ctx, userID, out); err != nil {
		return PrivateData{}, err
	}
	actorKind, actorUser := appmoderation.AuditActor(byUserID)
	audit := appmoderation.AuditRecorded{Entry: appmoderation.AuditEntry{
		ID: s.d.IDs.NewID(), ActorKind: actorKind, ActorUserID: actorUser, Action: appmoderation.AuditRevealedPrivateData,
		TargetKind: appmoderation.AuditTargetAccount, TargetID: accountID, TargetLabel: "@" + actor.Local.Username,
		OccurredAt: s.d.Clock.Now(),
	}}
	if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return tx.Emit(ctx, audit) }); err != nil {
		return PrivateData{}, fmt.Errorf("identity: audit the reveal of %s: %w", accountID, err)
	}
	return out, nil
}

// sameIP returns, for each IP address that d shows, newest first, the other accounts that used
// it.
func (s *AccountAdmin) sameIP(ctx context.Context, userID string, d PrivateData) ([]SameIPAccounts, error) {
	var ips []string
	for _, o := range d.SignIns {
		if o.IP != "" && !slices.Contains(ips, o.IP) {
			ips = append(ips, o.IP)
		}
	}
	if d.Registration != nil && !slices.Contains(ips, d.Registration.IP) {
		ips = append(ips, d.Registration.IP)
	}
	out := []SameIPAccounts{}
	for _, ip := range ips {
		ids, err := s.d.Repository.ListUserIDsByIP(ctx, ip, "", MaxSameIPAccounts+1)
		if err != nil {
			return nil, err
		}
		group := SameIPAccounts{IP: ip, Accounts: []AdminAccount{}}
		for _, id := range ids {
			if id == userID || len(group.Accounts) == MaxSameIPAccounts {
				continue
			}
			la, err := s.d.Accounts.LocalAccountOfUser(ctx, id)
			switch {
			case errors.Is(err, account.ErrNotFound):
				continue
			case err != nil:
				return nil, err
			}
			a, ok, err := s.match(ctx, la.AccountID, AccountSearch{})
			if err != nil {
				return nil, err
			}
			if ok {
				group.Accounts = append(group.Accounts, a)
			}
		}
		out = append(out, group)
	}
	return out, nil
}
