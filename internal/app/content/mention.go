// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"cmp"
	"context"
	"errors"
	"strings"

	appaccount "github.com/mycroblog/my/internal/app/account"
)

// mentionResolver resolves the mentions of one text (data-model §4.4, federation.md §6.2):
//
//   - @user, @user@{account_domain}, and @user@{host} are local, active accounts, linked to
//     their profile page on host.
//   - @user@domain of another domain is a stored remote account with that handle, or else one
//     that RemoteMentions resolves: at most MaxResolvedMentions unknown handles per text, all
//     within MentionResolveTimeout. It links to the account's url, or its URI when it has none.
//
// Any other handle, and one whose resolution fails, stays text.
type mentionResolver struct {
	s *Service
	// resolved counts the unknown handles given to RemoteMentions.
	resolved int
	// budget bounds the resolutions of the text; nil until the first one.
	budget context.Context
	cancel context.CancelFunc
	// asked are the handles read beforehand (preload), found those that exist, and urls the
	// profile pages of the remote ones.
	asked map[appaccount.HandleQuery]bool
	found map[appaccount.HandleQuery]appaccount.Actor
	urls  map[string]string
}

// handleBatch is an Accounts that reads many handles at once, as *appaccount.Service does.
type handleBatch interface {
	FindByHandles(ctx context.Context, queries []appaccount.HandleQuery) (map[appaccount.HandleQuery]appaccount.Actor, error)
}

// profileURLBatch is a RemoteMentions that reads many profile pages at once, as the platform's does.
type profileURLBatch interface {
	RemoteProfileURLs(ctx context.Context, accountIDs []string) (map[string]string, error)
}

// preload reads the stored accounts of every handle of a text together, and the profile pages of
// the remote ones, when Accounts and RemoteMentions can; find and resolve then answer from them.
func (r *mentionResolver) preload(ctx context.Context, handles []MentionHandle) error {
	b, ok := r.s.d.Accounts.(handleBatch)
	if !ok || len(handles) == 0 {
		return nil
	}
	d := r.s.d
	r.asked = map[appaccount.HandleQuery]bool{}
	var queries []appaccount.HandleQuery
	for _, h := range handles {
		q := appaccount.HandleQuery{Username: h.Username, Domain: h.Domain}
		if h.Domain == "" || h.Domain == strings.ToLower(d.AccountDomain) || h.Domain == strings.ToLower(d.Host) {
			q.Domain = ""
		}
		if !r.asked[q] {
			r.asked[q] = true
			queries = append(queries, q)
		}
	}
	var err error
	if r.found, err = b.FindByHandles(ctx, queries); err != nil {
		return err
	}
	if pb, ok := d.RemoteMentions.(profileURLBatch); ok {
		var remote []string
		for _, a := range r.found {
			if !a.Account.IsLocal() {
				remote = append(remote, a.Account.ID)
			}
		}
		if len(remote) > 0 {
			urls, err := pb.RemoteProfileURLs(ctx, remote)
			if err != nil {
				return err
			}
			// A remote account without a page reads as "", as RemoteProfileURL answers.
			r.urls = make(map[string]string, len(remote))
			for _, id := range remote {
				r.urls[id] = urls[id]
			}
		}
	}
	return nil
}

func (r *mentionResolver) close() {
	if r.cancel != nil {
		r.cancel()
	}
}

// resolve returns the target and the account ID of a mention, or false when it stays text.
func (r *mentionResolver) resolve(ctx context.Context, h MentionHandle) (MentionTarget, string, bool, error) {
	d := r.s.d
	if h.Domain == "" || h.Domain == strings.ToLower(d.AccountDomain) || h.Domain == strings.ToLower(d.Host) {
		// "" is account.LocalDomain, the domain of local handles.
		a, found, err := r.find(ctx, h.Username, "")
		if err != nil || !found || !usable(a) || !a.Account.IsLocal() {
			return MentionTarget{}, "", false, err
		}
		return MentionTarget{Username: a.Handle.Username, URL: profileURL(d.Host, a.Handle.Username)}, a.Account.ID, true, nil
	}
	a, found, err := r.find(ctx, h.Username, h.Domain)
	if err != nil {
		return MentionTarget{}, "", false, err
	}
	if !found {
		if a, found, err = r.fetch(ctx, h); err != nil || !found {
			return MentionTarget{}, "", false, err
		}
	}
	if !usable(a) || a.Account.IsLocal() {
		return MentionTarget{}, "", false, nil
	}
	u, read := r.urls[a.Account.ID]
	if d.RemoteMentions != nil && !read {
		u, err = d.RemoteMentions.RemoteProfileURL(ctx, a.Account.ID)
		if err != nil && !errors.Is(err, appaccount.ErrNotFound) {
			return MentionTarget{}, "", false, err
		}
	}
	return MentionTarget{Username: a.Handle.Username, URL: cmp.Or(u, a.Account.URI)}, a.Account.ID, true, nil
}

// find returns the stored account with a handle, and whether there is one.
func (r *mentionResolver) find(ctx context.Context, username, domain string) (appaccount.Actor, bool, error) {
	if q := (appaccount.HandleQuery{Username: username, Domain: domain}); r.asked[q] {
		a, ok := r.found[q]
		return a, ok, nil
	}
	a, err := r.s.d.Accounts.FindByHandle(ctx, username, domain)
	if errors.Is(err, appaccount.ErrNotFound) {
		return appaccount.Actor{}, false, nil
	}
	return a, err == nil, err
}

// usable reports whether a mention of a links to it: an active account with a handle.
func usable(a appaccount.Actor) bool { return a.Lifecycle.Active() && a.Handle != nil }

// fetch resolves an unknown remote handle through RemoteMentions, within the text's bounds, and
// reports whether it resolved. A handle that does not resolve stays text; only the caller's own
// cancellation fails the text.
func (r *mentionResolver) fetch(ctx context.Context, h MentionHandle) (appaccount.Actor, bool, error) {
	rm := r.s.d.RemoteMentions
	if rm == nil || r.resolved >= MaxResolvedMentions {
		return appaccount.Actor{}, false, nil
	}
	if r.budget == nil {
		r.budget, r.cancel = context.WithTimeout(ctx, MentionResolveTimeout)
	}
	if r.budget.Err() != nil {
		return appaccount.Actor{}, false, ctx.Err()
	}
	r.resolved++
	a, err := rm.ResolveHandle(r.budget, h.Username, h.Domain)
	if err != nil {
		return appaccount.Actor{}, false, ctx.Err()
	}
	return a, true, nil
}
