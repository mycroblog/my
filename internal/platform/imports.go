// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"fmt"
	"strings"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/account"
	domgraph "github.com/mycroblog/my/internal/domain/graph"
	domidentity "github.com/mycroblog/my/internal/domain/identity"
	domnotification "github.com/mycroblog/my/internal/domain/notification"
)

// setUpImports builds the CSV exports and imports of the users' follows, blocks, mutes,
// bookmarks, and muted domains (FR-008, FR-304) and registers their jobs. The targets read the services that are built
// later (the status reader, the resolver of remote posts) only when an export or a job runs.
func (a *App) setUpImports(repo identity.Repository) {
	outbox := a.Outbox
	if outbox == nil {
		// The inline queue has no transactional enqueue; its jobs are enqueued after the commit.
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	a.Imports = identity.NewImports(identity.ImportDeps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Outbox: outbox, Targets: importTargets{a},
		Host: a.Config.Host, AccountDomain: a.Config.AccountDomain, CanonicalDomain: federation.CanonicalHost,
		Clock: a.Clock, IDs: a.IDs, Logger: a.Logger,
	})
	a.Imports.RegisterJobs(a.Handlers, a.Scheduler)
}

// importTargets implements identity.ImportTargets with the graph and content modules, and
// resolves the accounts and posts of an import as a search does (FR-305).
type importTargets struct{ a *App }

// accountDomain is the domain of the addresses of local accounts.
func (t importTargets) accountDomain() string {
	if d := t.a.Config.AccountDomain; d != "" {
		return d
	}
	return t.a.Config.Host
}

// List implements identity.ImportTargets: the active follows, the blocks, the mutes in effect,
// and the bookmarks of statuses the account may still see, newest first, and the muted domains
// in the order of their domains. Accounts without an
// address (a remote actor whose handle is unconfirmed) are left out.
func (t importTargets) List(ctx context.Context, accountID string, kind domidentity.ImportKind) ([]identity.ImportTarget, error) {
	var out []identity.ImportTarget
	add := func(targetID string, row domidentity.ExportRow) error {
		actor, err := t.a.Accounts.FindByID(ctx, targetID)
		if errors.Is(err, appaccount.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if row.Target = t.address(actor); row.Target != "" {
			out = append(out, identity.ImportTarget{ID: targetID, Row: row})
		}
		return nil
	}
	switch kind {
	case domidentity.ImportFollowing:
		for p := (graph.Page{Limit: graph.MaxPageLimit}); ; {
			edges, err := t.a.Graph.Following(ctx, accountID, p)
			if err != nil {
				return nil, err
			}
			for _, e := range edges {
				rel, err := t.a.Graph.Relationship(ctx, accountID, e.FolloweeAccountID)
				if err != nil {
					return nil, err
				}
				notify, err := t.a.Subscriptions.Subscribed(ctx, accountID, e.FolloweeAccountID)
				if err != nil {
					return nil, err
				}
				if err := add(e.FolloweeAccountID, domidentity.ExportRow{ShowBoosts: rel.ShowBoosts, Notify: notify}); err != nil {
					return nil, err
				}
			}
			if len(edges) < p.Limit {
				return out, nil
			}
			p.Before = edges[len(edges)-1].FollowID
		}
	case domidentity.ImportBlocks:
		for p := (graph.Page{Limit: graph.MaxPageLimit}); ; {
			edges, err := t.a.Graph.Blocks(ctx, accountID, p)
			if err != nil {
				return nil, err
			}
			for _, e := range edges {
				if err := add(e.BlockedAccountID, domidentity.ExportRow{}); err != nil {
					return nil, err
				}
			}
			if len(edges) < p.Limit {
				return out, nil
			}
			p.Before = edges[len(edges)-1].BlockID
		}
	case domidentity.ImportMutes:
		for p := (graph.MutePage{Limit: graph.MaxPageLimit}); ; {
			mutes, err := t.a.Graph.Mutes(ctx, accountID, p)
			if err != nil {
				return nil, err
			}
			for _, m := range mutes {
				if err := add(m.TargetAccountID, domidentity.ExportRow{HideNotifications: m.HideNotifications}); err != nil {
					return nil, err
				}
			}
			if len(mutes) < p.Limit {
				return out, nil
			}
			p.Before = graph.MuteCursorOf(mutes[len(mutes)-1])
		}
	case domidentity.ImportDomainMutes:
		for p := (graph.DomainMutePage{Limit: graph.MaxPageLimit}); ; {
			mutes, err := t.a.Graph.DomainMutes(ctx, accountID, p)
			if err != nil {
				return nil, err
			}
			for _, m := range mutes {
				out = append(out, identity.ImportTarget{ID: m.Domain, Row: domidentity.ExportRow{Target: m.Domain}})
			}
			if len(mutes) < p.Limit {
				return out, nil
			}
			p.After = mutes[len(mutes)-1].Domain
		}
	case domidentity.ImportBookmarks:
		for before := ""; ; {
			page, err := t.a.Statuses.Bookmarks(ctx, accountID, before, graph.MaxPageLimit)
			if err != nil {
				return nil, err
			}
			for _, v := range page.Statuses {
				out = append(out, identity.ImportTarget{ID: v.Status.ID, Row: domidentity.ExportRow{Target: v.Status.URI}})
			}
			if page.Next == "" {
				return out, nil
			}
			before = page.Next
		}
	}
	return nil, fmt.Errorf("platform: unknown import kind %q", kind)
}

// address returns the address of an account in the CSV files: username@domain, with the account
// domain for a local account; "" for a remote actor without a confirmed handle.
func (t importTargets) address(a appaccount.Actor) string {
	switch {
	case a.Local != nil:
		return a.Local.Username + "@" + t.accountDomain()
	case a.Handle != nil && a.Handle.Domain != account.LocalDomain:
		return a.Handle.Username + "@" + a.Handle.Domain
	}
	return ""
}

// Add implements identity.ImportTargets. A follow keeps the item's settings, and a follow of a
// remote account is sent to its server by the federation module.
func (t importTargets) Add(ctx context.Context, accountID string, kind domidentity.ImportKind, item domidentity.DataImportItem) error {
	if kind == domidentity.ImportBookmarks {
		res, err := t.a.Resolver.Resolve(ctx, item.Target)
		if errors.Is(err, federation.ErrUnresolvable) || (err == nil && res.Status == nil) {
			return &identity.ImportItemError{Failure: domidentity.ImportFailureUnresolvable, Err: fmt.Errorf("%s is not a post: %w", item.Target, err)}
		}
		if err != nil {
			return err
		}
		return itemError(t.a.Interactions.Bookmark(ctx, accountID, res.Status.ID))
	}
	if kind == domidentity.ImportDomainMutes {
		domain, err := federation.CanonicalHost(item.Target)
		if err != nil {
			return &identity.ImportItemError{Failure: domidentity.ImportFailureUnresolvable, Err: err}
		}
		_, err = t.a.Graph.MuteDomain(ctx, accountID, domain)
		return itemError(err)
	}
	target, err := t.resolve(ctx, item.Target)
	if err != nil {
		return err
	}
	switch kind {
	case domidentity.ImportFollowing:
		if _, err := t.a.Graph.Follow(ctx, accountID, target); err != nil {
			return itemError(err)
		}
		_, err := t.a.Graph.UpdatePreferences(ctx, accountID, target, graph.PreferencesInput{ShowBoosts: item.ShowBoosts})
		if errors.Is(err, graph.ErrNotFound) {
			return nil // the follow ended in the meantime
		}
		if err != nil || !item.Notify {
			return itemError(err)
		}
		// Notify on new posts is the subscription (ADR-0123 §1); for a remote account it starts
		// when its server accepts the follow. A subscription that cannot be made leaves the
		// follow as it is.
		err = t.a.Subscriptions.SubscribeWithFollow(ctx, accountID, target)
		if errors.Is(err, notification.ErrNotSubscribable) || errors.Is(err, domnotification.ErrSubscriptionLimit) {
			return nil
		}
		return err
	case domidentity.ImportBlocks:
		_, err := t.a.Graph.Block(ctx, accountID, target)
		return itemError(err)
	case domidentity.ImportMutes:
		_, err := t.a.Graph.Mute(ctx, accountID, target, graph.MuteInput{HideNotifications: item.HideNotifications})
		return itemError(err)
	}
	return fmt.Errorf("platform: unknown import kind %q", kind)
}

// resolve returns the ID of the account at address: a local account by its username, or a
// remote one, fetched from its server when it is unknown.
func (t importTargets) resolve(ctx context.Context, address string) (string, error) {
	username, domain, ok := domidentity.SplitAddress(address)
	if !ok {
		return "", &identity.ImportItemError{Failure: domidentity.ImportFailureUnresolvable, Err: fmt.Errorf("%q is not an address", address)}
	}
	var (
		a   appaccount.Actor
		err error
	)
	if domain == "" || strings.EqualFold(domain, t.a.Config.Host) || strings.EqualFold(domain, t.accountDomain()) {
		a, err = t.a.Accounts.FindByHandle(ctx, username, account.LocalDomain)
	} else {
		a, err = t.a.RemoteActors.ResolveHandle(ctx, username, domain)
	}
	if errors.Is(err, appaccount.ErrNotFound) || errors.Is(err, federation.ErrUnresolvable) {
		return "", &identity.ImportItemError{Failure: domidentity.ImportFailureUnresolvable, Err: err}
	}
	if err != nil {
		return "", err
	}
	return a.Account.ID, nil
}

// itemError turns the errors of a command that mean the item cannot be carried out into
// *identity.ImportItemError; others stay worth retrying.
func itemError(err error) error {
	var failure domidentity.ImportFailure
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domgraph.ErrSelfFollow), errors.Is(err, domgraph.ErrSelfBlock), errors.Is(err, domgraph.ErrSelfMute):
		failure = domidentity.ImportFailureSelf
	case errors.Is(err, domgraph.ErrInvalidDomain):
		failure = domidentity.ImportFailureUnresolvable
	case errors.Is(err, domgraph.ErrTooManyDomainMutes):
		failure = domidentity.ImportFailureLimit
	case errors.Is(err, graph.ErrBlocked):
		failure = domidentity.ImportFailureBlocked
	case errors.Is(err, graph.ErrInteractionRestricted), errors.Is(err, content.ErrInteractionRestricted):
		failure = domidentity.ImportFailureRestricted
	case errors.Is(err, graph.ErrAccountUnavailable), errors.Is(err, graph.ErrRemoteAccount), errors.Is(err, graph.ErrNotFound),
		errors.Is(err, content.ErrNotFound), errors.Is(err, content.ErrActorUnavailable):
		failure = domidentity.ImportFailureUnavailable
	default:
		return err
	}
	return &identity.ImportItemError{Failure: failure, Err: err}
}

// Remove implements identity.ImportTargets.
func (t importTargets) Remove(ctx context.Context, accountID string, kind domidentity.ImportKind, targetID string) error {
	var err error
	switch kind {
	case domidentity.ImportFollowing:
		_, err = t.a.Graph.Unfollow(ctx, accountID, targetID)
	case domidentity.ImportBlocks:
		_, err = t.a.Graph.Unblock(ctx, accountID, targetID)
	case domidentity.ImportMutes:
		_, err = t.a.Graph.Unmute(ctx, accountID, targetID)
	case domidentity.ImportBookmarks:
		err = t.a.Interactions.Unbookmark(ctx, accountID, targetID)
	case domidentity.ImportDomainMutes:
		err = t.a.Graph.UnmuteDomain(ctx, accountID, targetID)
	default:
		return fmt.Errorf("platform: unknown import kind %q", kind)
	}
	if errors.Is(err, graph.ErrNotFound) {
		return nil
	}
	return itemError(err)
}
