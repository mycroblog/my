// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The data of the webhook messages (ADR-0060): the admin operations' representation of the
// object of an event, presented by the same serializers as those operations. *Server implements
// webhook.Presenter with these methods; the webhook module calls them in the transaction of the
// event, and they read in it through the modules' reads that take a transaction
// (Options.WebhookReads), so they see what the transaction wrote.

// presentReads are the reads behind the serializers of accounts and of the admin operations:
// the server's use cases outside transactions (Server.reads), or those of a transaction
// (txReads).
type presentReads interface {
	FindByID(ctx context.Context, id string) (appaccount.Actor, error)
	// LocalAccountOfUser fails with appaccount.ErrNotFound for a user without a local account.
	LocalAccountOfUser(ctx context.Context, userID string) (account.LocalAccount, error)
	Images(ctx context.Context, refs []appcontent.ImageRef) (map[appcontent.ImageRef]appcontent.Image, error)
	CustomEmojis(ctx context.Context, ids []string) (map[string]appcontent.CustomEmojiView, error)
	RemoteProfileURLs(ctx context.Context, accountIDs []string) (map[string]string, error)
	// FindByIDs returns the actors of ids that exist, by ID, as FindByID returns each.
	FindByIDs(ctx context.Context, ids []string) (map[string]appaccount.Actor, error)
	// LocalAccountsOfUsers returns the local accounts of the users of userIDs that have one, by
	// user, as LocalAccountOfUser returns each.
	LocalAccountsOfUsers(ctx context.Context, userIDs []string) (map[string]account.LocalAccount, error)
}

// serverReads reads through the server's use cases; those it was built without read nothing.
type serverReads struct{ o *Options }

func (s *Server) reads() presentReads { return serverReads{o: &s.o} }

func (r serverReads) FindByID(ctx context.Context, id string) (appaccount.Actor, error) {
	return r.o.Accounts.FindByID(ctx, id)
}

func (r serverReads) FindByIDs(ctx context.Context, ids []string) (map[string]appaccount.Actor, error) {
	return r.o.Accounts.FindByIDs(ctx, ids)
}

func (r serverReads) LocalAccountsOfUsers(ctx context.Context, userIDs []string) (map[string]account.LocalAccount, error) {
	if r.o.UserAccounts == nil {
		return map[string]account.LocalAccount{}, nil
	}
	return (&Server{o: *r.o}).localAccountsOfUsers(ctx, userIDs)
}

func (r serverReads) LocalAccountOfUser(ctx context.Context, userID string) (account.LocalAccount, error) {
	if r.o.UserAccounts == nil {
		return account.LocalAccount{}, appaccount.ErrNotFound
	}
	return r.o.UserAccounts.LocalAccountOfUser(ctx, userID)
}

func (r serverReads) Images(ctx context.Context, refs []appcontent.ImageRef) (map[appcontent.ImageRef]appcontent.Image, error) {
	if r.o.Media == nil {
		return nil, nil
	}
	return r.o.Media.Images(ctx, refs)
}

func (r serverReads) CustomEmojis(ctx context.Context, ids []string) (map[string]appcontent.CustomEmojiView, error) {
	if r.o.Media == nil {
		return nil, nil
	}
	return r.o.Media.CustomEmojis(ctx, ids)
}

func (r serverReads) RemoteProfileURLs(ctx context.Context, accountIDs []string) (map[string]string, error) {
	if r.o.RemoteProfiles == nil {
		return nil, nil
	}
	return r.o.RemoteProfiles.RemoteProfileURLs(ctx, accountIDs)
}

// WebhookReads are the reads in a transaction that the data of the webhook messages needs: each
// answers as the use case of the same name without Tx does, read in tx with what tx has
// written. The composition root implements it over the modules' use cases.
type WebhookReads interface {
	FindByIDTx(ctx context.Context, tx uow.Tx, id string) (appaccount.Actor, error)
	LocalAccountOfUserTx(ctx context.Context, tx uow.Tx, userID string) (account.LocalAccount, error)
	ImagesTx(ctx context.Context, tx uow.Tx, refs []appcontent.ImageRef) (map[appcontent.ImageRef]appcontent.Image, error)
	CustomEmojisTx(ctx context.Context, tx uow.Tx, ids []string) (map[string]appcontent.CustomEmojiView, error)
	RemoteProfileURLsTx(ctx context.Context, tx uow.Tx, accountIDs []string) (map[string]string, error)
	ReportRecordTx(ctx context.Context, tx uow.Tx, id string) (moderation.ReportRecord, error)
	AccountActionTx(ctx context.Context, tx uow.Tx, id string) (appmoderation.AccountActionView, error)
	AppealTx(ctx context.Context, tx uow.Tx, id string) (appmoderation.AppealView, error)
	DomainActionTx(ctx context.Context, tx uow.Tx, id string) (appmoderation.DomainActionView, error)
	RegistrationTx(ctx context.Context, tx uow.Tx, userID string) (appidentity.RegistrationEntry, error)
	AdminAccountTx(ctx context.Context, tx uow.Tx, id string) (appidentity.AdminAccount, error)
	CustomEmojiTx(ctx context.Context, tx uow.Tx, id string) (appcontent.AdminCustomEmoji, error)
}

// txReads reads in a transaction through WebhookReads.
type txReads struct {
	w  WebhookReads
	tx uow.Tx
}

func (r txReads) FindByID(ctx context.Context, id string) (appaccount.Actor, error) {
	return r.w.FindByIDTx(ctx, r.tx, id)
}

// FindByIDs reads each account in the transaction: the webhook data present one entity at a time.
func (r txReads) FindByIDs(ctx context.Context, ids []string) (map[string]appaccount.Actor, error) {
	return byOne(ids, func(id string) (appaccount.Actor, error) { return r.w.FindByIDTx(ctx, r.tx, id) }, appaccount.ErrNotFound)
}

// LocalAccountsOfUsers reads each local account in the transaction, as FindByIDs.
func (r txReads) LocalAccountsOfUsers(ctx context.Context, userIDs []string) (map[string]account.LocalAccount, error) {
	return byOne(userIDs, func(id string) (account.LocalAccount, error) { return r.w.LocalAccountOfUserTx(ctx, r.tx, id) }, appaccount.ErrNotFound)
}

func (r txReads) LocalAccountOfUser(ctx context.Context, userID string) (account.LocalAccount, error) {
	return r.w.LocalAccountOfUserTx(ctx, r.tx, userID)
}

func (r txReads) Images(ctx context.Context, refs []appcontent.ImageRef) (map[appcontent.ImageRef]appcontent.Image, error) {
	return r.w.ImagesTx(ctx, r.tx, refs)
}

func (r txReads) CustomEmojis(ctx context.Context, ids []string) (map[string]appcontent.CustomEmojiView, error) {
	return r.w.CustomEmojisTx(ctx, r.tx, ids)
}

func (r txReads) RemoteProfileURLs(ctx context.Context, accountIDs []string) (map[string]string, error) {
	return r.w.RemoteProfileURLsTx(ctx, r.tx, accountIDs)
}

// errNoWebhookReads reports a presenter method of a server built without Options.WebhookReads.
var errNoWebhookReads = errors.New("clientapi: the reads of the webhook data are not configured")

// webhookReads returns the reads in tx, or errNoWebhookReads.
func (s *Server) webhookReads(tx uow.Tx) (WebhookReads, txReads, error) {
	if s.o.WebhookReads == nil {
		return nil, txReads{}, errNoWebhookReads
	}
	return s.o.WebhookReads, txReads{w: s.o.WebhookReads, tx: tx}, nil
}

// WebhookReport presents a report as AdminReport, as GET /admin/reports lists it.
func (s *Server) WebhookReport(ctx context.Context, tx uow.Tx, id string) (any, error) {
	w, reads, err := s.webhookReads(tx)
	if err != nil {
		return nil, err
	}
	rec, err := w.ReportRecordTx(ctx, tx, id)
	if err != nil {
		return nil, fmt.Errorf("clientapi: report %s: %w", id, err)
	}
	items, err := s.newAdminReportsWith(ctx, reads, []moderation.ReportRecord{rec})
	if err != nil {
		return nil, err
	}
	return items[0], nil
}

// WebhookRegistration presents the registration of a user as AdminRegistration.
func (s *Server) WebhookRegistration(ctx context.Context, tx uow.Tx, userID string) (any, error) {
	w, _, err := s.webhookReads(tx)
	if err != nil {
		return nil, err
	}
	e, err := w.RegistrationTx(ctx, tx, userID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the registration of %s: %w", userID, err)
	}
	return newAdminRegistration(e), nil
}

// WebhookAccount presents an account as AdminAccount, as GET /admin/accounts lists it.
func (s *Server) WebhookAccount(ctx context.Context, tx uow.Tx, id string) (any, error) {
	w, reads, err := s.webhookReads(tx)
	if err != nil {
		return nil, err
	}
	a, err := w.AdminAccountTx(ctx, tx, id)
	if err != nil {
		return nil, fmt.Errorf("clientapi: account %s: %w", id, err)
	}
	items, err := s.newAdminAccountsWith(ctx, reads, []appidentity.AdminAccount{a})
	if err != nil {
		return nil, err
	}
	return items[0], nil
}

// WebhookAccountAction presents an action on an account as AdminAccountAction.
func (s *Server) WebhookAccountAction(ctx context.Context, tx uow.Tx, id string) (any, error) {
	w, reads, err := s.webhookReads(tx)
	if err != nil {
		return nil, err
	}
	v, err := w.AccountActionTx(ctx, tx, id)
	if err != nil {
		return nil, fmt.Errorf("clientapi: account action %s: %w", id, err)
	}
	items, err := s.newAdminAccountActionsWith(ctx, reads, []appmoderation.AccountActionView{v})
	if err != nil {
		return nil, err
	}
	return items[0], nil
}

// WebhookAppeal presents an appeal as AdminAppeal, with its action.
func (s *Server) WebhookAppeal(ctx context.Context, tx uow.Tx, id string) (any, error) {
	w, reads, err := s.webhookReads(tx)
	if err != nil {
		return nil, err
	}
	v, err := w.AppealTx(ctx, tx, id)
	if err != nil {
		return nil, fmt.Errorf("clientapi: appeal %s: %w", id, err)
	}
	items, err := s.newAdminAppealsWith(ctx, reads, []appmoderation.AppealView{v})
	if err != nil {
		return nil, err
	}
	return items[0], nil
}

// WebhookDomainAction presents an action on a domain as AdminDomainAction.
func (s *Server) WebhookDomainAction(ctx context.Context, tx uow.Tx, id string) (any, error) {
	w, _, err := s.webhookReads(tx)
	if err != nil {
		return nil, err
	}
	v, err := w.DomainActionTx(ctx, tx, id)
	if err != nil {
		return nil, fmt.Errorf("clientapi: domain action %s: %w", id, err)
	}
	return newAdminDomainAction(v), nil
}

// WebhookCustomEmoji presents a custom emoji as AdminCustomEmoji.
func (s *Server) WebhookCustomEmoji(ctx context.Context, tx uow.Tx, id string) (any, error) {
	w, reads, err := s.webhookReads(tx)
	if err != nil {
		return nil, err
	}
	e, err := w.CustomEmojiTx(ctx, tx, id)
	if err != nil {
		return nil, fmt.Errorf("clientapi: custom emoji %s: %w", id, err)
	}
	out, err := s.withCreatorsWith(ctx, reads, []appcontent.AdminCustomEmoji{e})
	if err != nil {
		return nil, err
	}
	return out[0], nil
}
