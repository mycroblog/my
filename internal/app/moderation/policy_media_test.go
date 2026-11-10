// SPDX-License-Identifier: AGPL-3.0-only

package moderation_test

import (
	"context"
	"slices"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// mediaPolicyRepo is an appmoderation.Repository with the restrictions and the allows of domains.
type mediaPolicyRepo struct {
	appmoderation.Repository
	restrictions map[string][]moderation.Restriction
	allowed      []string
}

func (r mediaPolicyRepo) DomainRestrictionsInEffect(_ context.Context, domains []string, _ time.Time) ([]moderation.Restriction, error) {
	var out []moderation.Restriction
	for _, d := range domains {
		out = append(out, r.restrictions[d]...)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

func (r mediaPolicyRepo) DomainRestrictionsInEffectTx(ctx context.Context, _ uow.Tx, domains []string, now time.Time) ([]moderation.Restriction, error) {
	return r.DomainRestrictionsInEffect(ctx, domains, now)
}

func (r mediaPolicyRepo) AllowedDomains(_ context.Context, domains []string) ([]string, error) {
	var out []string
	for _, d := range domains {
		if slices.Contains(r.allowed, d) {
			out = append(out, d)
		}
	}
	return out, nil
}

func (r mediaPolicyRepo) AllowedDomainsTx(ctx context.Context, _ uow.Tx, domains []string) ([]string, error) {
	return r.AllowedDomains(ctx, domains)
}

// fixedScope is an appmoderation.FederationScopes that reads one scope.
type fixedScope moderation.FederationScope

func (s fixedScope) FederationScope(context.Context) (moderation.FederationScope, error) {
	return moderation.FederationScope(s), nil
}

func (s fixedScope) FederationScopeTx(context.Context, uow.Tx) (moderation.FederationScope, error) {
	return moderation.FederationScope(s), nil
}

type nowClock struct{}

func (nowClock) Now() time.Time { return time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC) }

// TestRejectsMedia checks that the media proxy fetches nothing from a domain that rejects media,
// that is suspended, or, under allowlist, that has no allow (ADR-0159 §2), and that the read in a
// transaction answers the same.
func TestRejectsMedia(t *testing.T) {
	t.Parallel()
	repo := mediaPolicyRepo{
		restrictions: map[string][]moderation.Restriction{
			"media.example":   moderation.DomainRejectMedia.Restrictions(),
			"spam.example":    moderation.DomainSuspend.Restrictions(),
			"limited.example": moderation.DomainLimit.Restrictions(),
		},
		allowed: []string{"friend.example", "spam.example"},
	}
	base := appmoderation.NewFederationPolicy(repo, nowClock{})
	cases := []struct {
		name   string
		policy *appmoderation.FederationPolicy
		want   map[string]bool
	}{
		{"open", base.Scoped(fixedScope(moderation.ScopeOpen)), map[string]bool{
			"media.example": true, "sub.media.example": true, "spam.example": true, "sub.spam.example": true,
			"limited.example": false, "stranger.example": false, "friend.example": false, "": false,
		}},
		{"allowlist", base.Scoped(fixedScope(moderation.ScopeAllowlist)), map[string]bool{
			"media.example": true, "spam.example": true, "limited.example": true, "stranger.example": true,
			"friend.example": false, "sub.friend.example": false, "": false,
		}},
		{"unscoped", base.Scoped(fixedScope(moderation.ScopeAllowlist)).Unscoped(), map[string]bool{
			"media.example": true, "spam.example": true, "stranger.example": false, "friend.example": false,
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			for domain, want := range c.want {
				if got, err := c.policy.RejectsMedia(t.Context(), domain); err != nil || got != want {
					t.Errorf("RejectsMedia(%q) = %v, %v; want %v", domain, got, err, want)
				}
				if got, err := c.policy.RejectsMediaTx(t.Context(), nil, domain); err != nil || got != want {
					t.Errorf("RejectsMediaTx(%q) = %v, %v; want %v", domain, got, err, want)
				}
			}
		})
	}
}
