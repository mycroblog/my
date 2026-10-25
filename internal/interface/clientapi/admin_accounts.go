// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"unicode/utf8"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The admin operations of the account management (FR-714, manage_accounts): the search, the
// details of an account, and showing the email address and the IP addresses of a local one,
// which only revealAdminAccount returns and the identity module records in the audit log; and,
// with manage_login_recovery, removing a user's two-factor authentication and issuing a password
// reset link for them (FR-005, FR-002), which the audit log records too.

// AccountAdmin runs the moderators' account management. *appidentity.AccountAdmin implements it.
type AccountAdmin interface {
	Search(ctx context.Context, q appidentity.AccountSearch, limit int) ([]appidentity.AdminAccount, string, error)
	Detail(ctx context.Context, accountID string) (appidentity.AdminAccountDetail, error)
	Reveal(ctx context.Context, accountID, byUserID string) (appidentity.PrivateData, error)
	DisableTwoFactor(ctx context.Context, accountID, byUserID string) error
	LiftEmailSuppression(ctx context.Context, accountID, byUserID string) error
}

// PasswordResetLinks issues the password reset links that administrators hand to users
// (FR-002). *appidentity.Service implements it.
type PasswordResetLinks interface {
	IssuePasswordResetLink(ctx context.Context, accountID, byUserID string) (appidentity.PasswordResetLink, error)
}

// AccountReports reads the reports of an account for its details. *appmoderation.Service
// implements it.
type AccountReports interface {
	RecentReportsOf(ctx context.Context, targetAccountID string, limit int) ([]moderation.ReportRecord, error)
}

// maxAdminAccountParam bounds the length of the text parameters of the search, in characters.
const maxAdminAccountParam = 320

// newAdminAccounts presents accounts as the moderators see them, reading their images once.
func (s *Server) newAdminAccounts(ctx context.Context, as []appidentity.AdminAccount) ([]AdminAccount, error) {
	return s.newAdminAccountsWith(ctx, s.reads(), as)
}

// newAdminAccountsWith is newAdminAccounts through reads.
func (s *Server) newAdminAccountsWith(ctx context.Context, reads presentReads, as []appidentity.AdminAccount) ([]AdminAccount, error) {
	actors := make([]appaccount.Actor, len(as))
	for i, a := range as {
		actors[i] = a.Actor
	}
	presented, err := s.newAccountsWith(ctx, reads, actors)
	if err != nil {
		return nil, err
	}
	out := make([]AdminAccount, len(as))
	for i, a := range as {
		item := AdminAccount{
			Id: a.Actor.Account.ID, Account: presented[i], Local: a.Actor.Account.IsLocal(), State: AdminAccountState(a.State),
			Restrictions: make([]AdminAccountRestriction, 0, len(a.Restrictions)),
		}
		for _, r := range a.Restrictions {
			item.Restrictions = append(item.Restrictions, AdminAccountRestriction(r))
		}
		if a.UserID != "" {
			st := AdminAccountRegistrationState(identity.RegistrationApproved)
			if a.Registration != nil {
				st = AdminAccountRegistrationState(a.Registration.State)
			}
			item.RegistrationState = &st
		}
		out[i] = item
	}
	return out, nil
}

// ListAdminAccounts serves GET /api/v1/admin/accounts, newest first. The cursor is the position
// of the search after the last account of the previous page.
func (s *Server) ListAdminAccounts(ctx context.Context, req ListAdminAccountsRequestObject) (ListAdminAccountsResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil {
		return nil, err
	} else if p != nil {
		return ListAdminAccountsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	pq := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if pq.problem != nil {
		return ListAdminAccountsdefaultApplicationProblemPlusJSONResponse(*pq.problem), nil
	}
	p := req.Params
	q := appidentity.AccountSearch{After: pq.before}
	for _, f := range []struct {
		name  string
		value *string
		dst   *string
	}{
		{"username", p.Username, &q.Username}, {"domain", p.Domain, &q.Domain}, {"email", p.Email, &q.Email},
		{"email_domain", p.EmailDomain, &q.EmailDomain}, {"ip", p.Ip, &q.IP},
	} {
		if f.value == nil {
			continue
		}
		if utf8.RuneCountInString(*f.value) > maxAdminAccountParam {
			return ListAdminAccountsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, f.name)), nil
		}
		*f.dst = *f.value
	}
	if p.Origin != nil {
		q.Origin = string(*p.Origin)
	}
	if p.State != nil {
		q.State = appidentity.AdminAccountState(*p.State)
	}
	as, next, err := s.o.AccountAdmin.Search(ctx, q, pq.limit)
	if serr := (*appidentity.AccountSearchError)(nil); errors.As(err, &serr) {
		return ListAdminAccountsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, serr.Param)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: search accounts: %w", err)
	}
	items, err := s.newAdminAccounts(ctx, as)
	if err != nil {
		return nil, err
	}
	page := AdminAccountPage{Items: items}
	if next != "" {
		page.NextCursor = encodeCursor(next)
	}
	cache := cacheNoStore
	return ListAdminAccounts200JSONResponse{Body: page, Headers: ListAdminAccounts200ResponseHeaders{CacheControl: &cache}}, nil
}

// GetAdminAccount serves GET /api/v1/admin/accounts/{id}.
func (s *Server) GetAdminAccount(ctx context.Context, req GetAdminAccountRequestObject) (GetAdminAccountResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil {
		return nil, err
	} else if p != nil {
		return GetAdminAccountdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	d, err := s.o.AccountAdmin.Detail(ctx, req.Id)
	if errors.Is(err, appaccount.ErrNotFound) {
		return GetAdminAccountdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: account %s: %w", req.Id, err)
	}
	presented, err := s.newAdminAccounts(ctx, []appidentity.AdminAccount{d.AdminAccount})
	if err != nil {
		return nil, err
	}
	a := presented[0]
	out := AdminAccountDetail{
		Id: a.Id, Account: a.Account, Local: a.Local, State: a.State, Restrictions: a.Restrictions,
		RegistrationState: a.RegistrationState, Roles: make([]string, 0, len(d.Roles)),
	}
	for _, r := range d.Roles {
		out.Roles = append(out.Roles, r.Key)
	}
	if d.UserID != "" {
		enabled := d.TwoFactorEnabled
		out.TwoFactorEnabled = &enabled
	}
	if d.HasEmail {
		v := d.EmailVerified
		out.EmailVerified = &v
	}
	if d.EmailSuppression != nil {
		out.EmailSuppression = newEmailSuppression(*d.EmailSuppression, true)
	}
	if app := d.Application; app != nil {
		reg := AdminAccountRegistration{SubmittedAt: app.SubmittedAt.UTC(), StateChangedAt: app.SubmittedAt.UTC()}
		if d.Registration != nil {
			reg.StateChangedAt = d.Registration.Since.UTC()
		}
		if app.Reason != "" {
			reg.Reason = &app.Reason
		}
		if app.InviteID != "" {
			reg.InviteId = &app.InviteID
		}
		out.Registration = &reg
	}
	actions, err := s.o.AccountModeration.AccountActions(ctx, appmoderation.AccountActionFilter{AccountID: req.Id}, appidentity.AccountDetailHistory)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the actions on account %s: %w", req.Id, err)
	}
	if out.Actions, err = s.newAdminAccountActions(ctx, actions); err != nil {
		return nil, err
	}
	reports, err := s.o.AccountReports.RecentReportsOf(ctx, req.Id, appidentity.AccountDetailHistory)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the reports of account %s: %w", req.Id, err)
	}
	if out.Reports, err = s.newAdminReports(ctx, reports); err != nil {
		return nil, err
	}
	cache := cacheNoStore
	return GetAdminAccount200JSONResponse{Body: out, Headers: GetAdminAccount200ResponseHeaders{CacheControl: &cache}}, nil
}

// DisableAdminAccountTwoFactor serves POST /api/v1/admin/accounts/{id}/two_factor/disable.
func (s *Server) DisableAdminAccountTwoFactor(ctx context.Context, req DisableAdminAccountTwoFactorRequestObject) (DisableAdminAccountTwoFactorResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageLoginRecovery); err != nil {
		return nil, err
	} else if p != nil {
		return DisableAdminAccountTwoFactordefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	err = s.o.AccountAdmin.DisableTwoFactor(ctx, req.Id, v.UserID)
	l := s.Localizer(ctx)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return DisableAdminAccountTwoFactordefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appidentity.ErrNotLocalAccount):
		return DisableAdminAccountTwoFactordefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("id"))), nil
	case errors.Is(err, appidentity.ErrTwoFactorNotEnabled):
		return DisableAdminAccountTwoFactordefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusConflict, ProblemTypeTwoFactorNotEnabled, l.ServerErrorsTwoFactorNotEnabled(), "")), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: remove the two-factor authentication of account %s: %w", req.Id, err)
	}
	return DisableAdminAccountTwoFactor204Response{}, nil
}

// ProblemTypeEmailNotSuppressed is the problem of a lift of an email address that is not
// suppressed.
const ProblemTypeEmailNotSuppressed = "/api/v1/problems/email-not-suppressed"

// LiftAdminAccountEmailSuppression serves POST /api/v1/admin/accounts/{id}/email_suppression/lift.
func (s *Server) LiftAdminAccountEmailSuppression(ctx context.Context, req LiftAdminAccountEmailSuppressionRequestObject) (LiftAdminAccountEmailSuppressionResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil {
		return nil, err
	} else if p != nil {
		return LiftAdminAccountEmailSuppressiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	err = s.o.AccountAdmin.LiftEmailSuppression(ctx, req.Id, v.UserID)
	l := s.Localizer(ctx)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return LiftAdminAccountEmailSuppressiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appidentity.ErrNotLocalAccount):
		return LiftAdminAccountEmailSuppressiondefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("id"))), nil
	case errors.Is(err, appidentity.ErrNotSuppressed):
		return LiftAdminAccountEmailSuppressiondefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusConflict, ProblemTypeEmailNotSuppressed, l.ServerErrorsEmailNotSuppressed(), "")), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: lift the email suppression of account %s: %w", req.Id, err)
	}
	return LiftAdminAccountEmailSuppression204Response{}, nil
}

// IssueAdminAccountPasswordResetLink serves POST /api/v1/admin/accounts/{id}/password_reset_link.
// Like removing two-factor authentication, it lets the holder of the link into the account, so it
// needs manage_login_recovery (server administration, ADR-0058) rather than manage_accounts.
func (s *Server) IssueAdminAccountPasswordResetLink(ctx context.Context, req IssueAdminAccountPasswordResetLinkRequestObject) (IssueAdminAccountPasswordResetLinkResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageLoginRecovery); err != nil {
		return nil, err
	} else if p != nil {
		return IssueAdminAccountPasswordResetLinkdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if s.o.PasswordResetLinks == nil {
		return IssueAdminAccountPasswordResetLinkdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	link, err := s.o.PasswordResetLinks.IssuePasswordResetLink(ctx, req.Id, v.UserID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return IssueAdminAccountPasswordResetLinkdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appidentity.ErrNotLocalAccount):
		l := s.Localizer(ctx)
		return IssueAdminAccountPasswordResetLinkdefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("id"))), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: issue a password reset link for account %s: %w", req.Id, err)
	}
	cache := cacheNoStore
	return IssueAdminAccountPasswordResetLink201JSONResponse{
		Body:    AdminPasswordResetLink{Url: link.URL, ExpiresAt: link.ExpiresAt.UTC()},
		Headers: IssueAdminAccountPasswordResetLink201ResponseHeaders{CacheControl: &cache},
	}, nil
}

// RevealAdminAccount serves POST /api/v1/admin/accounts/{id}/reveal.
func (s *Server) RevealAdminAccount(ctx context.Context, req RevealAdminAccountRequestObject) (RevealAdminAccountResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageAccounts); err != nil {
		return nil, err
	} else if p != nil {
		return RevealAdminAccountdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	d, err := s.o.AccountAdmin.Reveal(ctx, req.Id, v.UserID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return RevealAdminAccountdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appidentity.ErrNotLocalAccount):
		l := s.Localizer(ctx)
		return RevealAdminAccountdefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("id"))), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: reveal account %s: %w", req.Id, err)
	}
	out := AdminAccountPrivateData{SignIns: make([]AdminAccountSignIn, 0, len(d.SignIns)), SameIpAccounts: make([]AdminSameIpAccounts, 0, len(d.SameIP))}
	if d.Email != "" {
		email, verified := d.Email, d.EmailVerified
		out.Email, out.EmailVerified = &email, &verified
	}
	if r := d.Registration; r != nil {
		ip, at := r.IP, r.RecordedAt.UTC()
		out.RegistrationIp, out.RegisteredAt = &ip, &at
	}
	for _, o := range d.SignIns {
		item := AdminAccountSignIn{SignedInAt: o.CreatedAt.UTC()}
		if o.IP != "" {
			ip := o.IP
			item.Ip = &ip
		}
		if o.UserAgent != "" {
			ua := o.UserAgent
			item.UserAgent = &ua
		}
		out.SignIns = append(out.SignIns, item)
	}
	for _, g := range d.SameIP {
		accounts, err := s.newAdminAccounts(ctx, g.Accounts)
		if err != nil {
			return nil, err
		}
		out.SameIpAccounts = append(out.SameIpAccounts, AdminSameIpAccounts{Ip: g.IP, Accounts: accounts})
	}
	cache := cacheNoStore
	return RevealAdminAccount200JSONResponse{Body: out, Headers: RevealAdminAccount200ResponseHeaders{CacheControl: &cache}}, nil
}
