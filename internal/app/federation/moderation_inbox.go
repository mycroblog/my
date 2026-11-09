// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/federation"
)

// BlockReceiver records the blocks that remote accounts make; *appgraph.Service implements it.
type BlockReceiver interface {
	ReceiveBlock(ctx context.Context, in appgraph.RemoteBlockInput) error
	RetractRemoteBlock(ctx context.Context, in appgraph.RemoteUnblockInput) error
}

// ReportFiler files the reports received from other servers; *appmoderation.Service implements
// it.
type ReportFiler interface {
	FileRemoteReport(ctx context.Context, in appmoderation.RemoteReportInput) (appmoderation.Report, bool, error)
}

// ReportPolicy says whether the reports of a domain are dropped (reject_reports);
// *appmoderation.FederationPolicy implements it.
type ReportPolicy interface {
	RejectsReports(ctx context.Context, domain string) (bool, error)
}

// LocalObjects finds the local accounts and statuses that received activities point at.
// *appaccount.Service and *appcontent.Service implement it together (LocalObjectsOf).
type LocalObjects interface {
	FindByURI(ctx context.Context, uri string) (appaccount.Actor, error)
	FindStatus(ctx context.Context, id string) (appcontent.Status, error)
}

// LocalObjectsOf joins the account and content modules' services into LocalObjects.
type LocalObjectsOf struct {
	Accounts *appaccount.Service
	Content  *appcontent.Service
}

// FindByURI implements LocalObjects.
func (l LocalObjectsOf) FindByURI(ctx context.Context, uri string) (appaccount.Actor, error) {
	return l.Accounts.FindByURI(ctx, uri)
}

// FindStatus implements LocalObjects.
func (l LocalObjectsOf) FindStatus(ctx context.Context, id string) (appcontent.Status, error) {
	return l.Content.FindStatus(ctx, id)
}

// FindByURIs implements the batch read of the local accounts of a Flag.
func (l LocalObjectsOf) FindByURIs(ctx context.Context, uris []string) (map[string]appaccount.Actor, error) {
	return l.Accounts.FindByURIs(ctx, uris)
}

// FindStatuses implements the batch read of the local statuses of a Flag.
func (l LocalObjectsOf) FindStatuses(ctx context.Context, ids []string) (map[string]appcontent.Status, error) {
	return l.Content.FindStatuses(ctx, ids)
}

// localObjectsBatch is a LocalObjects that reads many accounts and statuses at once, as
// LocalObjectsOf does.
type localObjectsBatch interface {
	FindByURIs(ctx context.Context, uris []string) (map[string]appaccount.Actor, error)
	FindStatuses(ctx context.Context, ids []string) (map[string]appcontent.Status, error)
}

// ModerationHandlerDeps are the ports and settings of the handlers of received Block, Undo
// Block, and Flag activities.
type ModerationHandlerDeps struct {
	Blocks  BlockReceiver
	Reports ReportFiler
	Policy  ReportPolicy
	Objects LocalObjects
	// Host is the serving domain of the local actor and object URIs (ADR-0018).
	Host string
}

// RegisterModerationHandlers registers the inbox handlers of Block, the Undo of a Block, and Flag
// on p (federation.md §3.4, §11).
//
//   - Block of a local actor: graph.ReceiveBlock, which ends the follows between the two
//     accounts in both directions and hides the blocker's posts from the blocked account.
//   - Undo of a Block: graph.RetractRemoteBlock. An unknown Block leaves a tombstone, so that
//     the Block arriving later is dropped.
//   - Flag of local actors and statuses: moderation.FileRemoteReport, unless the sender's domain
//     has the report_intake restriction (reject_reports), in which case the Flag is dropped.
func RegisterModerationHandlers(p *InboxProcessor, d ModerationHandlerDeps) {
	h := moderationHandlers{d: d}
	p.Handle(federation.ActivityBlock, h.block)
	p.HandleUndo(federation.ActivityBlock, h.undoBlock)
	p.Handle(federation.ActivityFlag, h.flag)
}

type moderationHandlers struct{ d ModerationHandlerDeps }

// localAccount returns the local account with the actor URI iri, false when there is none.
func (h moderationHandlers) localAccount(ctx context.Context, iri federation.IRI) (appaccount.Actor, bool, error) {
	if iri.Host() != h.d.Host {
		return appaccount.Actor{}, false, nil
	}
	a, err := h.d.Objects.FindByURI(ctx, string(iri))
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return appaccount.Actor{}, false, nil
	case err != nil:
		return appaccount.Actor{}, false, err
	}
	return a, a.Account.IsLocal() && a.Local != nil, nil
}

// localObjects returns, in the order of refs, the local accounts and the local statuses that
// refs name, as localAccount and then localStatus find each: with one read of the accounts and
// one of the statuses when Objects can, one by one otherwise.
func (h moderationHandlers) localObjects(ctx context.Context, refs []federation.Ref) (accounts []string, statuses []appcontent.Status, err error) {
	b, ok := h.d.Objects.(localObjectsBatch)
	if !ok {
		for _, ref := range refs {
			acc, ok, err := h.localAccount(ctx, ref.ID)
			if err != nil {
				return nil, nil, err
			}
			if ok {
				accounts = append(accounts, acc.Account.ID)
				continue
			}
			st, ok, err := h.localStatus(ctx, ref.ID)
			if err != nil {
				return nil, nil, err
			}
			if ok {
				statuses = append(statuses, st)
			}
		}
		return accounts, statuses, nil
	}
	var uris, ids []string
	for _, ref := range refs {
		if ref.ID.Host() == h.d.Host {
			uris = append(uris, string(ref.ID))
		}
		if id, ok := localStatusID(h.d.Host, ref.ID); ok {
			ids = append(ids, id)
		}
	}
	actors, err := b.FindByURIs(ctx, uris)
	if err != nil {
		return nil, nil, err
	}
	sts, err := b.FindStatuses(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	for _, ref := range refs {
		if ref.ID.Host() == h.d.Host {
			if a, found := actors[string(ref.ID)]; found && a.Account.IsLocal() && a.Local != nil {
				accounts = append(accounts, a.Account.ID)
				continue
			}
		}
		if id, ok := localStatusID(h.d.Host, ref.ID); ok {
			if st, found := sts[id]; found && st.IsLocal && st.URI == string(ref.ID) {
				statuses = append(statuses, st)
			}
		}
	}
	return accounts, statuses, nil
}

// localStatusID returns the ID of the local status that iri would name, false for an IRI that
// is not of a local status.
func localStatusID(host string, iri federation.IRI) (string, bool) {
	id, ok := strings.CutPrefix(string(iri), appcontent.StatusURI(host, ""))
	if !ok || id == "" || strings.ContainsAny(id, "/?#") {
		return "", false
	}
	return id, true
}

// localStatus returns the local status with the object URI iri, false when there is none.
func (h moderationHandlers) localStatus(ctx context.Context, iri federation.IRI) (appcontent.Status, bool, error) {
	id, ok := localStatusID(h.d.Host, iri)
	if !ok {
		return appcontent.Status{}, false, nil
	}
	st, err := h.d.Objects.FindStatus(ctx, id)
	switch {
	case errors.Is(err, appcontent.ErrNotFound):
		return appcontent.Status{}, false, nil
	case err != nil:
		return appcontent.Status{}, false, err
	}
	return st, st.IsLocal && st.URI == string(iri), nil
}

// dropGraph turns the refusals of the graph module into a dropped activity.
func dropGraph(err error) error {
	if errors.Is(err, appgraph.ErrAccountUnavailable) || errors.Is(err, appgraph.ErrRemoteAccount) ||
		errors.Is(err, appgraph.ErrLocalAccount) || errors.Is(err, appgraph.ErrNotFound) {
		return fmt.Errorf("%w: %w", ErrDropActivity, err)
	}
	return err
}

func (h moderationHandlers) block(ctx context.Context, a *InboxActivity) error {
	object := a.Activity.Object().ID
	target, ok, err := h.localAccount(ctx, object)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: the Block of %s is not of a local actor", ErrDropActivity, object)
	}
	err = h.d.Blocks.ReceiveBlock(ctx, appgraph.RemoteBlockInput{
		URI: string(a.Activity.ID), BlockerAccountID: a.Actor.Account.ID, BlockedAccountID: target.Account.ID,
	})
	return dropGraph(err)
}

func (h moderationHandlers) undoBlock(ctx context.Context, a *InboxActivity) error {
	ref := a.Activity.Object()
	in := appgraph.RemoteUnblockInput{BlockURI: string(ref.ID), BlockerAccountID: a.Actor.Account.ID}
	if in.BlockURI == "" {
		// A Block without an id is found by its accounts: its object, when its actor is the
		// actor of the Undo.
		doc := ref.Untrusted()
		if doc == nil || doc.Activity == nil || doc.Activity.Actor != a.Activity.Actor {
			return fmt.Errorf("%w: an Undo of a Block without an id or an object", ErrDropActivity)
		}
		target, ok, err := h.localAccount(ctx, doc.Activity.Object().ID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: the Block undone is not of a local actor", ErrUnknownObject)
		}
		in.BlockedAccountID = target.Account.ID
	}
	err := h.d.Blocks.RetractRemoteBlock(ctx, in)
	if errors.Is(err, appgraph.ErrNotFound) {
		return fmt.Errorf("%w: %w", ErrUnknownObject, err)
	}
	return err
}

func (h moderationHandlers) flag(ctx context.Context, a *InboxActivity) error {
	domain := a.Activity.Actor.Host()
	rejected, err := h.d.Policy.RejectsReports(ctx, domain)
	if err != nil {
		return err
	}
	if rejected {
		return fmt.Errorf("%w: the reports of %s are rejected", ErrDropActivity, domain)
	}
	var (
		accounts []string
		statuses []appcontent.Status
	)
	accounts, statuses, err = h.localObjects(ctx, a.Activity.Objects)
	if err != nil {
		return err
	}
	// The report is of the first local account named, or else of the author of the first local
	// status; it keeps that account's statuses.
	var target string
	switch {
	case len(accounts) > 0:
		target = accounts[0]
	case len(statuses) > 0:
		target = statuses[0].AuthorAccountID
	default:
		return fmt.Errorf("%w: the Flag names no local account or status", ErrDropActivity)
	}
	var ids []string
	for _, st := range statuses {
		if st.AuthorAccountID == target {
			ids = append(ids, st.ID)
		}
	}
	_, _, err = h.d.Reports.FileRemoteReport(ctx, appmoderation.RemoteReportInput{
		URI: string(a.Received.ActivityURI), ReporterAccountID: a.Actor.Account.ID, TargetAccountID: target,
		StatusIDs: ids, Comment: a.Activity.Content,
	})
	return err
}
