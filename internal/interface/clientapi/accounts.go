// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/domain/account"
)

// Accounts reads accounts for the client API. *appaccount.Service implements it.
type Accounts interface {
	// FindByID returns the actor with an account ID, failing with appaccount.ErrNotFound when
	// there is none.
	FindByID(ctx context.Context, id string) (appaccount.Actor, error)
	// FindByHandle returns the actor with a handle (domain is account.LocalDomain for local
	// accounts), failing with appaccount.ErrNotFound when there is none.
	FindByHandle(ctx context.Context, username, domain string) (appaccount.Actor, error)
	// FindByURI returns the actor with an ActivityPub id, failing with appaccount.ErrNotFound
	// when there is none.
	FindByURI(ctx context.Context, uri string) (appaccount.Actor, error)
}

// RemoteProfiles reads the profile pages of remote accounts. *appaccount.Service implements it.
type RemoteProfiles interface {
	// RemoteProfileURLs returns the profile pages of the remote accounts in accountIDs, keyed by
	// account ID; accounts without one are absent.
	RemoteProfileURLs(ctx context.Context, accountIDs []string) (map[string]string, error)
}

// AccountResolver fetches the remote accounts that the instance does not know yet, by handle
// (WebFinger) or by URL (FR-305). Both fail with an error wrapping
// appfederation.ErrUnresolvable when the handle or URL leads to no account.
// *appfederation.RemoteActors implements it.
type AccountResolver interface {
	ResolveHandle(ctx context.Context, username, domain string) (appaccount.Actor, error)
	ResolveURL(ctx context.Context, uri string) (appaccount.Actor, error)
}

// cacheNoStore is the Cache-Control of responses that depend on the viewer (ADR-0016).
const cacheNoStore = "no-store"

// cacheFor returns the Cache-Control of a GET that anonymous requests may make: shared caches
// may store the anonymous answer, never one for a viewer, which blocks change.
func cacheFor(v *Viewer) *string {
	c := cachePublic
	if v != nil {
		c = cacheNoStore
	}
	return &c
}

// accountParts are what the accounts of one response show beyond their own rows, read together
// by Server.accountParts: the URLs of their avatars and headers, and the profile pages of the
// remote ones. The zero value has none.
type accountParts struct {
	images map[appcontent.ImageRef]appcontent.Image
	// profiles are the profile pages of remote accounts, by account ID.
	profiles map[string]string
	// emojis are the custom emoji that can be shown, by ID.
	emojis map[string]appcontent.CustomEmojiView
}

// customEmojis presents the custom emoji with an ID in ids that can be shown, in the order of
// ids, as the CustomEmoji schema; never nil.
func (p accountParts) customEmojis(ids []string) []CustomEmoji {
	out := make([]CustomEmoji, 0, len(ids))
	for _, id := range ids {
		if e, ok := p.emojis[id]; ok {
			out = append(out, CustomEmoji{Shortcode: e.Shortcode, Url: e.URL, StaticUrl: e.StaticURL})
		}
	}
	return out
}

// image returns the variants of an account's image, or nil when it has none to show.
func (p accountParts) image(accountID, mediaID string) *MediaVariants {
	if mediaID == "" {
		return nil
	}
	if img, ok := p.images[appcontent.ImageRef{AccountID: accountID, MediaID: mediaID}]; ok {
		v := newMediaVariants(img)
		return &v
	}
	return nil
}

// accountParts reads the parts of actors (accountParts) with one call of the media use cases
// and one of the account use cases, so that a response presents a page of accounts without a
// lookup per account.
func (s *Server) accountParts(ctx context.Context, actors ...appaccount.Actor) (accountParts, error) {
	return s.readAccountParts(ctx, actors, nil, nil)
}

// readAccountParts reads the images, custom emoji, and profile pages of actors, only the profile
// pages of linked (accounts that a response links to without showing them, such as mentions), and
// the custom emoji with an ID in emojiIDs (those of the statuses of a response).
func (s *Server) readAccountParts(ctx context.Context, actors, linked []appaccount.Actor, emojiIDs []string) (accountParts, error) {
	return s.readAccountPartsWith(ctx, s.reads(), actors, linked, emojiIDs)
}

// readAccountPartsWith is readAccountParts through reads.
func (s *Server) readAccountPartsWith(ctx context.Context, reads presentReads, actors, linked []appaccount.Actor, emojiIDs []string) (accountParts, error) {
	var parts accountParts
	var refs []appcontent.ImageRef
	var remote []string
	emojis := slices.Clone(emojiIDs)
	for _, a := range actors {
		if p := a.Profile; p != nil {
			for _, id := range []string{p.AvatarMediaID, p.HeaderMediaID} {
				if id != "" {
					refs = append(refs, appcontent.ImageRef{AccountID: a.Account.ID, MediaID: id})
				}
			}
			emojis = append(emojis, p.EmojiIDs...)
		}
	}
	for _, a := range slices.Concat(actors, linked) {
		if !a.Account.IsLocal() && !slices.Contains(remote, a.Account.ID) {
			remote = append(remote, a.Account.ID)
		}
	}
	if len(refs) > 0 {
		images, err := reads.Images(ctx, refs)
		if err != nil {
			return accountParts{}, fmt.Errorf("clientapi: the profile images: %w", err)
		}
		parts.images = images
	}
	if len(emojis) > 0 {
		views, err := reads.CustomEmojis(ctx, emojis)
		if err != nil {
			return accountParts{}, fmt.Errorf("clientapi: the custom emoji: %w", err)
		}
		parts.emojis = views
	}
	if len(remote) > 0 {
		urls, err := reads.RemoteProfileURLs(ctx, remote)
		if err != nil {
			return accountParts{}, fmt.Errorf("clientapi: the remote profile pages: %w", err)
		}
		parts.profiles = urls
	}
	return parts, nil
}

// newAccount presents an actor as the Account schema, without the follow counts. parts holds the
// actor's images and, for a remote actor, its profile page (Server.accountParts); a remote actor
// without one links to its uri.
func (s *Server) newAccount(a appaccount.Actor, parts accountParts) Account {
	out := Account{
		Id: a.Account.ID, CreatedAt: a.Account.CreatedAt, Uri: a.Account.URI, Url: a.Account.URI,
		Domain: a.Account.Domain, Fields: []ProfileField{}, Emojis: []CustomEmoji{}, Badges: []AccountBadge{},
	}
	if a.Account.IsLocal() {
		out.Domain = s.accountDomain()
	}
	switch {
	case a.Local != nil:
		out.Username, out.Acct = a.Local.Username, a.Local.Username
		out.Url = s.origin() + "/@" + a.Local.Username
	case a.Handle != nil:
		out.Username = a.Handle.Username
		out.Acct = a.Handle.Username
		if a.Handle.Domain != account.LocalDomain {
			out.Acct += "@" + a.Handle.Domain
			out.Domain = a.Handle.Domain
		}
	}
	if u, ok := parts.profiles[a.Account.ID]; ok && !a.Account.IsLocal() {
		out.Url = u
	}
	if p := a.Profile; p != nil {
		out.DisplayName, out.Note, out.Locked, out.Bot = p.DisplayName, p.NoteHTML, p.Locked, p.Bot
		for _, f := range p.Fields {
			out.Fields = append(out.Fields, ProfileField{Name: f.Name, Value: f.ValueHTML})
		}
		out.Avatar, out.Header = parts.image(a.Account.ID, p.AvatarMediaID), parts.image(a.Account.ID, p.HeaderMediaID)
		out.Emojis = parts.customEmojis(p.EmojiIDs)
	}
	return out
}

// newAccounts presents actors in order as the Account schema, reading their images together.
func (s *Server) newAccounts(ctx context.Context, actors []appaccount.Actor) ([]Account, error) {
	return s.newAccountsWith(ctx, s.reads(), actors)
}

// newAccountsWith is newAccounts through reads.
func (s *Server) newAccountsWith(ctx context.Context, reads presentReads, actors []appaccount.Actor) ([]Account, error) {
	parts, err := s.readAccountPartsWith(ctx, reads, actors, nil, nil)
	if err != nil {
		return nil, err
	}
	out := make([]Account, 0, len(actors))
	for _, a := range actors {
		out = append(out, s.newAccount(a, parts))
	}
	return out, nil
}

// newProfile presents an actor as the Account schema with its follow counts and, for a local
// account, the role badges its user shows (ADR-0071): the answer of the account's own endpoints.
func (s *Server) newProfile(ctx context.Context, a appaccount.Actor) (Account, error) {
	parts, err := s.accountParts(ctx, a)
	if err != nil {
		return Account{}, err
	}
	out := s.newAccount(a, parts)
	c, err := s.o.Graph.Counters(ctx, a.Account.ID)
	if err != nil {
		return Account{}, fmt.Errorf("clientapi: the follow counts of %s: %w", a.Account.ID, err)
	}
	followers, following := int(c.FollowersCount), int(c.FollowingCount)
	out.FollowersCount, out.FollowingCount = &followers, &following
	if a.Local != nil {
		if out.Badges, err = s.profileBadges(ctx, a.Local.UserID); err != nil {
			return Account{}, err
		}
	}
	if out.Moved, err = s.movedTo(ctx, a.Account.ID); err != nil {
		return Account{}, err
	}
	return out, nil
}

// AccountMoves reads the moves of accounts (ADR-0089). *appaccount.Service implements it.
type AccountMoves interface {
	// MoveState returns the move in force of an account, failing with appaccount.ErrNotFound
	// when it has not moved.
	MoveState(ctx context.Context, accountID string) (appaccount.MoveState, error)
	// FindMove returns a move by ID, failing with appaccount.ErrNotFound when there is none.
	FindMove(ctx context.Context, id string) (appaccount.Move, error)
}

// movedTo presents the account that accountID moved to, nil when it has not moved, its target
// is gone or not active, or the server has no Options.Moves.
func (s *Server) movedTo(ctx context.Context, accountID string) (*Account, error) {
	if s.o.Moves == nil {
		return nil, nil
	}
	st, err := s.o.Moves.MoveState(ctx, accountID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: the move of %s: %w", accountID, err)
	}
	return s.presentedTarget(ctx, st.TargetAccountID)
}

// presentedTarget presents the target of a move, nil when it is gone or not active.
func (s *Server) presentedTarget(ctx context.Context, targetID string) (*Account, error) {
	t, err := s.o.Accounts.FindByID(ctx, targetID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: the move target %s: %w", targetID, err)
	}
	if !t.Lifecycle.Active() {
		return nil, nil
	}
	parts, err := s.accountParts(ctx, t)
	if err != nil {
		return nil, err
	}
	out := s.newAccount(t, parts)
	return &out, nil
}

// ProfileRestrictions reads whether moderation hides an account's profile from everyone but
// itself (profile_view, data-model §5.2). *moderation.AccountPolicy implements it.
type ProfileRestrictions interface {
	ProfileHidden(ctx context.Context, accountID string) (bool, error)
}

// presentable reports whether an account may be shown: it is a user's account (not the
// instance actor) and active, and, unless it is the viewer's own, moderation does not hide its
// profile and, when blockCheck is set, it does not block the viewer. An
// account that is not presentable is answered as if it did not exist, so that a blocked
// viewer cannot tell it apart from an unknown one.
func (s *Server) presentable(ctx context.Context, a appaccount.Actor, viewer *Viewer, blockCheck bool) (bool, error) {
	if a.Account.ActorType == account.ActorApplication || !a.Lifecycle.Active() {
		return false, nil
	}
	if viewer != nil && viewer.AccountID == a.Account.ID {
		return true, nil
	}
	if s.o.ProfileRestrictions != nil {
		hidden, err := s.o.ProfileRestrictions.ProfileHidden(ctx, a.Account.ID)
		if err != nil || hidden {
			return false, err
		}
	}
	if !blockCheck || viewer == nil {
		return true, nil
	}
	blocked, err := s.o.Graph.IsBlocking(ctx, a.Account.ID, viewer.AccountID)
	return !blocked, err
}

// visibleAccount returns the account with an ID as the viewer may see it; ok is false when
// there is none, it is not active, moderation hides its profile, or it blocks the viewer (nil
// for an anonymous request).
func (s *Server) visibleAccount(ctx context.Context, viewer *Viewer, id string) (appaccount.Actor, bool, error) {
	return s.presentableByID(ctx, viewer, id, true)
}

// presentableByID returns the account with an ID; ok is false when there is none or it is not
// presentable.
func (s *Server) presentableByID(ctx context.Context, viewer *Viewer, id string, blockCheck bool) (a appaccount.Actor, ok bool, err error) {
	a, err = s.o.Accounts.FindByID(ctx, id)
	if errors.Is(err, appaccount.ErrNotFound) {
		return appaccount.Actor{}, false, nil
	}
	if err != nil {
		return appaccount.Actor{}, false, err
	}
	ok, err = s.presentable(ctx, a, viewer, blockCheck)
	return a, ok, err
}

// viewerOf returns the Viewer of an operation that also serves anonymous requests, or nil.
func viewerOf(ctx context.Context) *Viewer {
	if v, ok := ViewerFrom(ctx); ok {
		return &v
	}
	return nil
}

// notFoundResponse is notFound as a default response, for an account that is not visible.
func (s *Server) notFoundResponse(ctx context.Context) problem {
	body, headers := s.notFound(ctx)
	return problem{Body: body, Headers: headers, StatusCode: http.StatusNotFound}
}

// VerifyAccountCredentials serves GET /api/v1/accounts/verify_credentials: the viewer's own
// account with the source of its profile.
func (s *Server) VerifyAccountCredentials(ctx context.Context, _ VerifyAccountCredentialsRequestObject) (VerifyAccountCredentialsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	a, err := s.o.Accounts.FindByID(ctx, v.AccountID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the viewer's account %s: %w", v.AccountID, err)
	}
	body, err := s.newCredentialAccount(ctx, v.UserID, a)
	if err != nil {
		return nil, err
	}
	cache := cacheNoStore
	return VerifyAccountCredentials200JSONResponse{
		Body:    body,
		Headers: VerifyAccountCredentials200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// GetAccount serves GET /api/v1/accounts/{id}.
func (s *Server) GetAccount(ctx context.Context, req GetAccountRequestObject) (GetAccountResponseObject, error) {
	v := viewerOf(ctx)
	a, ok, err := s.visibleAccount(ctx, v, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return GetAccountdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	body, err := s.newProfile(ctx, a)
	if err != nil {
		return nil, err
	}
	if body.VoiceRoom, err = s.voiceRoomEntry(ctx, v, a.Account.ID); err != nil {
		return nil, err
	}
	return GetAccount200JSONResponse{AccountProfileJSONResponse{Body: body, Headers: AccountProfileResponseHeaders{CacheControl: cacheFor(v)}}}, nil
}

// LookupAccount serves GET /api/v1/accounts/lookup: an account by its handle or actor URL,
// fetched from its server when resolve is set and the instance does not know it (FR-305).
func (s *Server) LookupAccount(ctx context.Context, req LookupAccountRequestObject) (LookupAccountResponseObject, error) {
	v := viewerOf(ctx)
	resolve := req.Params.Resolve != nil && *req.Params.Resolve
	if resolve && v == nil {
		l := s.Localizer(ctx)
		body, headers := s.NewProblem(ctx, http.StatusUnauthorized, ProblemTypeUnauthenticated, l.ServerErrorsUnauthenticated(), "")
		challenge := "Bearer"
		headers.WWWAuthenticate = &challenge
		return LookupAccountdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusUnauthorized}, nil
	}
	a, found, err := s.lookup(ctx, req.Params.Acct)
	if err != nil {
		return nil, err
	}
	if !found && resolve && s.o.Resolver != nil && s.resolvable(req.Params.Acct) {
		// A mode that takes nothing in finds only the known accounts (ADR-0076).
		if ok, err := s.federationAllows(ctx, remoteAccountFlows...); err != nil {
			return nil, err
		} else if !ok {
			return LookupAccountdefaultApplicationProblemPlusJSONResponse(s.federationRestricted(ctx)), nil
		}
		if p, h, limited := s.LimitProblem(ctx, ResolveLimit, v.AccountID); limited {
			return LookupAccountdefaultApplicationProblemPlusJSONResponse{Body: p, Headers: h, StatusCode: http.StatusTooManyRequests}, nil
		}
		if a, found, err = s.resolve(ctx, req.Params.Acct); err != nil {
			return nil, err
		}
	}
	if !found {
		return LookupAccountdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if ok, err := s.presentable(ctx, a, v, true); err != nil {
		return nil, err
	} else if !ok {
		return LookupAccountdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	body, err := s.newProfile(ctx, a)
	if err != nil {
		return nil, err
	}
	return LookupAccount200JSONResponse{AccountProfileJSONResponse{Body: body, Headers: AccountProfileResponseHeaders{CacheControl: cacheFor(v)}}}, nil
}

// lookup finds the stored account of acct, a handle or an actor URL; found is false when there
// is none.
func (s *Server) lookup(ctx context.Context, acct string) (a appaccount.Actor, found bool, err error) {
	if isURL(acct) {
		a, err = s.o.Accounts.FindByURI(ctx, acct)
	} else if username, domain, ok := s.parseAcct(acct); ok {
		a, err = s.o.Accounts.FindByHandle(ctx, username, domain)
	} else {
		return appaccount.Actor{}, false, nil
	}
	if errors.Is(err, appaccount.ErrNotFound) {
		return appaccount.Actor{}, false, nil
	}
	return a, err == nil, err
}

// resolvable reports whether acct can name a remote account: an actor URL, or a handle of
// another domain.
func (s *Server) resolvable(acct string) bool {
	if isURL(acct) {
		return true
	}
	_, domain, ok := s.parseAcct(acct)
	return ok && domain != account.LocalDomain
}

// resolve fetches the remote account of acct, which is resolvable; found is false when it
// leads to none.
func (s *Server) resolve(ctx context.Context, acct string) (a appaccount.Actor, found bool, err error) {
	if isURL(acct) {
		a, err = s.o.Resolver.ResolveURL(ctx, acct)
	} else {
		username, domain, _ := s.parseAcct(acct)
		a, err = s.o.Resolver.ResolveHandle(ctx, username, domain)
	}
	if errors.Is(err, appfederation.ErrUnresolvable) {
		return appaccount.Actor{}, false, nil
	}
	return a, err == nil, err
}

// isURL reports whether a lookup names an actor by URL rather than by handle.
func isURL(acct string) bool {
	return strings.HasPrefix(acct, "https://") || strings.HasPrefix(acct, "http://")
}

// origin is the origin of the links to the server's own pages (Options.Origin).
func (s *Server) origin() string {
	if s.o.Origin != "" {
		return s.o.Origin
	}
	return "https://" + s.o.Host
}

// pageURL returns the link to the page of this instance that u, a URL under https://Host, names,
// under origin; any other u is returned as it is.
func (s *Server) pageURL(u string) string {
	if rest, ok := strings.CutPrefix(u, "https://"+s.o.Host+"/"); ok {
		return s.origin() + "/" + rest
	}
	return u
}

// accountDomain is the domain of local handles.
func (s *Server) accountDomain() string {
	if s.o.AccountDomain != "" {
		return s.o.AccountDomain
	}
	return s.o.Host
}

// parseAcct splits a handle, "username" or "username@domain" with an optional leading "@",
// into the username and the domain; the instance's own domains mean a local account.
func (s *Server) parseAcct(acct string) (username, domain string, ok bool) {
	username, domain, _ = strings.Cut(strings.TrimPrefix(acct, "@"), "@")
	if username == "" || strings.Contains(domain, "@") {
		return "", "", false
	}
	if strings.EqualFold(domain, s.o.Host) || (s.o.AccountDomain != "" && strings.EqualFold(domain, s.o.AccountDomain)) {
		domain = account.LocalDomain
	}
	return username, domain, true
}
