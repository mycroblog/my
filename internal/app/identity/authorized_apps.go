// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// LastUsedNamespace is the KeyValueStore namespace of the throttle on recording when a token
// family was last used.
const LastUsedNamespace = "oauth_last_used"

// LastUsedInterval is how often at most the last use of a token family is recorded
// (ADR-0090 §5): the "last used" time of an authorized app is exact to this interval.
const LastUsedInterval = time.Hour

// AuthorizedApp is an app that a user authorized: the user's still working token families of one
// client (ADR-0090 §5). There is no table of it; it is derived from the tokens.
type AuthorizedApp struct {
	Client identity.OAuthClient
	// OwnerUsername is the username of the user who registered the client; "" when it has no
	// owner or the owner has no local account.
	OwnerUsername string
	// Scope is the union of the scopes the user granted to the client's working tokens.
	Scope identity.Scope
	// AuthorizedAt is when the user first authorized the client (the earliest working family).
	AuthorizedAt time.Time
	// LastUsedAt is when any of the families last authenticated a request, to the hour; nil
	// when no use was recorded.
	LastUsedAt *time.Time
}

// recordTokenUse records that the token family used its access token, at most once per
// LastUsedInterval. The throttle is an atomic counter in the KeyValueStore, so that concurrent
// requests and instances write once, and the write is its own short transaction. A failure
// never fails the request that is authenticating. Once this process recorded a use, it takes
// nothing from the counter until the counter's interval ends: every authenticated request would
// otherwise write to the store (#3183).
func (s *Service) recordTokenUse(ctx context.Context, familyID string) {
	if s.d.KV == nil {
		return
	}
	now := s.d.Clock.Now()
	if s.recordedUses.recent(familyID, now) {
		return
	}
	n, err := s.d.KV.IncrBy(ctx, LastUsedNamespace, familyID, 1, LastUsedInterval)
	if err != nil {
		slog.WarnContext(ctx, "identity: throttling the last use of a token family failed", "family_id", familyID, "error", err)
		return
	}
	if n != 1 {
		return
	}
	// The counter started now and ends after LastUsedInterval: until then no use is recorded.
	s.recordedUses.remember(familyID, now.Add(LastUsedInterval))
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.d.Repository.UpsertTokenFamilyLastUsed(ctx, tx, familyID, now)
	})
	if err != nil {
		slog.WarnContext(ctx, "identity: recording the last use of a token family failed", "family_id", familyID, "error", err)
		s.recordedUses.forget(familyID)
		_ = s.d.KV.Delete(ctx, LastUsedNamespace, familyID) // let the next request retry
	}
}

// tokenUsesPruneAt is the number of token families above which remember drops the ones whose
// interval ended.
const tokenUsesPruneAt = 4096

// tokenUses holds, by token family, until when the use that this process recorded keeps the
// next from being recorded. It is safe for concurrent use; its zero value is empty.
type tokenUses struct {
	mu    sync.Mutex
	until map[string]time.Time
}

// recent reports whether familyID's use was recorded by this process less than LastUsedInterval
// before now.
func (u *tokenUses) recent(familyID string, now time.Time) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	t, ok := u.until[familyID]
	return ok && now.Before(t)
}

func (u *tokenUses) remember(familyID string, until time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.until == nil {
		u.until = map[string]time.Time{}
	}
	if len(u.until) >= tokenUsesPruneAt {
		for id, t := range u.until {
			if !t.After(until.Add(-LastUsedInterval)) {
				delete(u.until, id)
			}
		}
	}
	u.until[familyID] = until
}

func (u *tokenUses) forget(familyID string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.until, familyID)
}

// AuthorizedApps returns the apps the user authorized, one per client, most recently used
// first. A client counts while one of the user's token families of it works; it is derived
// from the tokens, so a revoked or expired grant is not listed.
func (s *Service) AuthorizedApps(ctx context.Context, userID string) ([]AuthorizedApp, error) {
	tokens, err := s.d.Repository.ListAuthorizedAppTokens(ctx, userID, s.d.Clock.Now())
	if err != nil {
		return nil, err
	}
	var apps []AuthorizedApp
	index := map[string]int{}
	scopes := map[string][]string{}
	for _, t := range tokens {
		i, ok := index[t.Client.ID]
		if !ok {
			i = len(apps)
			index[t.Client.ID] = i
			apps = append(apps, AuthorizedApp{Client: t.Client, AuthorizedAt: t.AuthorizedAt})
		}
		a := &apps[i]
		scopes[t.Client.ID] = append(scopes[t.Client.ID], t.Scope.Tokens()...)
		if t.AuthorizedAt.Before(a.AuthorizedAt) {
			a.AuthorizedAt = t.AuthorizedAt
		}
		if t.LastUsedAt != nil && (a.LastUsedAt == nil || t.LastUsedAt.After(*a.LastUsedAt)) {
			used := *t.LastUsedAt
			a.LastUsedAt = &used
		}
	}
	owners := map[string]string{}
	for i := range apps {
		a := &apps[i]
		scope, err := identity.ParseScope(strings.Join(scopes[a.Client.ID], " "))
		if err != nil {
			return nil, err
		}
		a.Scope = scope
		owner := a.Client.OwnerUserID
		if owner == "" {
			continue
		}
		name, ok := owners[owner]
		if !ok {
			la, err := s.d.Accounts.LocalAccountOfUser(ctx, owner)
			switch {
			case err == nil:
				name = la.Username
			case !errors.Is(err, account.ErrNotFound):
				return nil, err
			}
			owners[owner] = name
		}
		a.OwnerUsername = name
	}
	slices.SortStableFunc(apps, func(x, y AuthorizedApp) int {
		switch {
		case x.LastUsedAt != nil && y.LastUsedAt != nil && !x.LastUsedAt.Equal(*y.LastUsedAt):
			return y.LastUsedAt.Compare(*x.LastUsedAt)
		case x.LastUsedAt != nil && y.LastUsedAt == nil:
			return -1
		case x.LastUsedAt == nil && y.LastUsedAt != nil:
			return 1
		}
		return y.AuthorizedAt.Compare(x.AuthorizedAt)
	})
	return apps, nil
}

// RevokeAuthorizedApp revokes every token family that the user holds of the client with the
// given ID (the OAuthClient ID), with the revocation reason `user`, so the app's access and
// refresh tokens stop working at once (ADR-0090 §5). It fails with ErrNotFound when the user has
// no working grant of the client.
//
// The server's own web client is listed and revocable like any app: revoking it signs the user
// out of every browser, the current one included, ending the browser sessions of the grants as
// a sign-out of the web client does (RevokeToken).
func (s *Service) RevokeAuthorizedApp(ctx context.Context, userID, clientID string) error {
	now := s.d.Clock.Now()
	tokens, err := s.d.Repository.ListAuthorizedAppTokens(ctx, userID, now)
	if err != nil {
		return err
	}
	var families []string
	var client identity.OAuthClient
	for _, t := range tokens {
		if t.Client.ID == clientID {
			client = t.Client
			if !slices.Contains(families, t.FamilyID) {
				families = append(families, t.FamilyID)
			}
		}
	}
	if len(families) == 0 {
		return ErrNotFound
	}
	web := client.FirstParty()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.LockTokenGrants(ctx, tx, userID); err != nil {
			return err
		}
		if _, err := s.d.Repository.RevokeTokensOfUserAndClient(ctx, tx, userID, clientID, identity.TokenRevokedUser, now); err != nil {
			return err
		}
		if !web {
			return nil
		}
		for _, f := range families {
			if err := s.d.Repository.RevokeSessionOfTokenFamily(ctx, tx, f, identity.RevokedLogout, now); err != nil {
				return err
			}
		}
		return nil
	})
}
