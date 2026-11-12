// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/federation"
)

// ErrUnresolvable reports a handle or URL that does not lead to a remote actor with a confirmed
// handle: WebFinger does not name an actor, the actor cannot be fetched or stored, its server is
// refused or unavailable, or WebFinger does not confirm its handle.
var ErrUnresolvable = errors.New("federation: the account could not be resolved")

// ResolveHandle returns the account with the handle username@domain (FR-305):
// the stored one, or the actor that WebFinger of acct:username@domain on https://domain
// names as self, fetched and stored with Resolve. The actor must hold this handle, confirmed by
// WebFinger; a stored actor that holds another one is fetched again to check it anew. It fails
// with ErrUnresolvable; other errors come from the stores. It does network I/O, so callers call
// it outside transactions and bound it with ctx.
func (r *RemoteActors) ResolveHandle(ctx context.Context, username, domain string) (appaccount.Actor, error) {
	host, err := CanonicalHost(domain)
	if err != nil || username == "" || strings.ContainsAny(username, "@/?#: ") {
		return appaccount.Actor{}, fmt.Errorf("%w: %q@%q is not a handle", ErrUnresolvable, username, domain)
	}
	a, err := r.d.Accounts.FindByHandle(ctx, username, host)
	switch {
	case err == nil:
		if !a.Account.IsLocal() {
			r.refreshIfStale(ctx, a.Account.ID)
		}
		return a, nil
	case !errors.Is(err, appaccount.ErrNotFound):
		return appaccount.Actor{}, err
	}
	// The rules of fetching apply to WebFinger too: never this instance, a suspended server, or
	// an unavailable one.
	origin := federation.IRI("https://" + host + "/")
	if err := r.d.Fetcher.check(ctx, origin); err != nil {
		if isFetchError(err) {
			return appaccount.Actor{}, fmt.Errorf("%w: %w", ErrUnresolvable, err)
		}
		return appaccount.Actor{}, err
	}
	answer, err := r.webFinger(ctx, "https://"+host, "acct:"+username+"@"+host)
	if err != nil {
		return appaccount.Actor{}, fmt.Errorf("%w: %w", ErrUnresolvable, err)
	}
	self := answer.self()
	if self == "" {
		return appaccount.Actor{}, fmt.Errorf("%w: WebFinger of %s@%s names no actor", ErrUnresolvable, username, host)
	}
	a, err = r.ResolveURL(ctx, string(self))
	if err != nil || holdsHandle(a, username, host) {
		return a, err
	}
	// A stored actor may hold another handle until its handle is checked again, as when the
	// handle is on an account domain that WebFinger of its server names.
	a, err = r.Refresh(ctx, string(self))
	switch {
	case isFetchError(err):
		return appaccount.Actor{}, fmt.Errorf("%w: %w", ErrUnresolvable, err)
	case err != nil:
		return appaccount.Actor{}, err
	case !holdsHandle(a, username, host):
		return appaccount.Actor{}, fmt.Errorf("%w: %s does not hold the handle %s@%s", ErrUnresolvable, a.Account.URI, username, host)
	}
	return a, nil
}

// holdsHandle reports whether a is a remote actor whose confirmed handle is username@host: the
// username compared case-insensitively, host canonical.
func holdsHandle(a appaccount.Actor, username, host string) bool {
	if a.Account.IsLocal() || a.Handle == nil || !strings.EqualFold(a.Handle.Username, username) {
		return false
	}
	h, err := CanonicalHost(a.Handle.Domain)
	return err == nil && h == host
}

// ResolveURL returns the account whose actor is at uri (its id, or a URL that serves it, such as
// a profile page): the stored one, or the fetched one when it is unknown (Resolve). The actor
// must have a confirmed handle. It fails with ErrUnresolvable; other errors come from the
// stores.
func (r *RemoteActors) ResolveURL(ctx context.Context, uri string) (appaccount.Actor, error) {
	a, err := r.Resolve(ctx, uri)
	switch {
	case isFetchError(err):
		return appaccount.Actor{}, fmt.Errorf("%w: %w", ErrUnresolvable, err)
	case err != nil:
		return appaccount.Actor{}, err
	case a.Handle == nil:
		return appaccount.Actor{}, fmt.Errorf("%w: WebFinger does not confirm the handle of %s", ErrUnresolvable, a.Account.URI)
	}
	return a, nil
}

// Resolved is what Resolver.Resolve found: an account or a status, exactly one of them.
type Resolved struct {
	Account *appaccount.Actor
	Status  *appcontent.Status
}

// Resolver resolves what a user searches for (FR-305): a handle
// @username@domain, or the URL of an account or a status.
type Resolver struct {
	actors   *RemoteActors
	statuses *StatusHandlers
	fetcher  DocumentFetcher
}

// NewResolver returns a Resolver that resolves accounts with actors, and fetches documents and
// stores posts with statuses.
func NewResolver(actors *RemoteActors, statuses *StatusHandlers) *Resolver {
	return &Resolver{actors: actors, statuses: statuses, fetcher: statuses.d.Fetcher}
}

// Resolve returns the account or status that query names. A handle ("@username@domain" or
// "username@domain") is resolved with RemoteActors.ResolveHandle. A URL is looked up among the
// stored accounts and statuses (its canonical IRI); otherwise its document is fetched once and,
// by its type, stored as a remote actor with a confirmed handle or as a remote post
// (StatusHandlers' rules of fetched posts; its parent is fetched one level, as on receipt). URLs
// of this instance are never fetched. It fails with ErrUnresolvable; other errors come from the
// stores. It does network I/O, so callers call it outside transactions and bound it with ctx.
func (r *Resolver) Resolve(ctx context.Context, query string) (Resolved, error) {
	query = strings.TrimSpace(query)
	if !strings.HasPrefix(query, "https://") && !strings.HasPrefix(query, "http://") {
		username, domain, ok := strings.Cut(strings.TrimPrefix(query, "@"), "@")
		if !ok {
			return Resolved{}, fmt.Errorf("%w: %q is neither a handle nor a URL", ErrUnresolvable, query)
		}
		a, err := r.actors.ResolveHandle(ctx, username, domain)
		if err != nil {
			return Resolved{}, err
		}
		return Resolved{Account: &a}, nil
	}
	target, err := fetchTarget(query)
	if err != nil {
		return Resolved{}, fmt.Errorf("%w: %w", ErrUnresolvable, err)
	}
	if res, ok, err := r.stored(ctx, target); err != nil || ok {
		return res, err
	}
	got, err := r.fetcher.Fetch(ctx, string(target))
	if err != nil {
		if isFetchError(err) {
			return Resolved{}, fmt.Errorf("%w: %w", ErrUnresolvable, err)
		}
		return Resolved{}, err
	}
	if id := got.Document.ID(); id != target {
		// A page of the account or post: the document names its id.
		if res, ok, err := r.stored(ctx, id); err != nil || ok {
			return res, err
		}
	}
	switch doc := got.Document; {
	case doc.Actor != nil:
		actor, raw, err := usableActor(got)
		if err != nil {
			return Resolved{}, fmt.Errorf("%w: %w", ErrUnresolvable, err)
		}
		if _, err := r.actors.store(ctx, actor, raw); err != nil {
			return Resolved{}, err
		}
		a, err := r.actors.ResolveURL(ctx, string(actor.ID))
		if err != nil {
			return Resolved{}, err
		}
		return Resolved{Account: &a}, nil
	case doc.Object != nil:
		st, err := r.statuses.fetched(ctx, got, true)
		switch {
		case errors.Is(err, ErrNotTaken), isFetchError(err):
			return Resolved{}, fmt.Errorf("%w: %w", ErrUnresolvable, err)
		case err != nil:
			return Resolved{}, err
		}
		return Resolved{Status: &st}, nil
	}
	return Resolved{}, fmt.Errorf("%w: %s is neither an actor nor a post", ErrUnresolvable, target)
}

// Stored returns the stored account or status that the URL uri names by its IRI, without
// fetching anything; found is false when the instance knows none, and a URL of this instance
// that names nothing stored fails with ErrUnresolvable, as Resolve does.
func (r *Resolver) Stored(ctx context.Context, uri string) (res Resolved, found bool, err error) {
	target, err := fetchTarget(strings.TrimSpace(uri))
	if err != nil {
		return Resolved{}, false, fmt.Errorf("%w: %w", ErrUnresolvable, err)
	}
	return r.stored(ctx, target)
}

// stored returns the stored account or status with the IRI uri. An account must have a
// confirmed handle, as ResolveURL requires.
func (r *Resolver) stored(ctx context.Context, uri federation.IRI) (Resolved, bool, error) {
	switch _, err := r.actors.d.Accounts.FindByURI(ctx, string(uri)); {
	case err == nil:
		a, err := r.actors.ResolveURL(ctx, string(uri))
		if err != nil {
			return Resolved{}, false, err
		}
		return Resolved{Account: &a}, true, nil
	case !errors.Is(err, appaccount.ErrNotFound):
		return Resolved{}, false, err
	}
	switch st, err := r.statuses.d.Statuses.FindStatusByURI(ctx, string(uri)); {
	case err == nil:
		return Resolved{Status: &st}, true, nil
	case !errors.Is(err, appcontent.ErrNotFound):
		return Resolved{}, false, err
	}
	if strings.EqualFold(uri.Host(), r.statuses.d.Host) {
		return Resolved{}, false, fmt.Errorf("%w: %s is not on this server", ErrUnresolvable, uri)
	}
	return Resolved{}, false, nil
}
