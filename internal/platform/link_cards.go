// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"net/url"
	"slices"
	"strings"

	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpLinkCards builds the link cards (ADR-0062): their projector, which enqueues the fetches
// through outbox, their jobs, and their cleanup. It comes after the media proxy, which serves
// their images; the reader of statuses presents them.
func (a *App) setUpLinkCards(cfg *config.Config, repo content.LinkCardRepository, statuses content.Repository, outbox queue.Outbox) {
	a.LinkCards = content.NewLinkCards(content.LinkCardsDeps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Statuses: statuses, Outbox: outbox,
		HTTP: a.HTTPClient, KV: a.KV, Processor: a.MediaProcessor, Proxy: a.MediaProxy,
		Domains: linkCardDomains{a}, Accounts: a.Accounts,
		// Reads a.RemoteActors, built by setUpRemoteActors.
		Creators: linkCardCreators{a},
		// The instance setting is read per fetch and per status, so that a change applies at once.
		Enabled: func(ctx context.Context) (bool, error) {
			s, err := a.Instance.Settings(ctx)
			return s.LinkCardsEnabled(), err
		},
		IgnoreRobotsTxt: func(ctx context.Context) (bool, error) {
			s, err := a.Instance.Settings(ctx)
			return s.LinkCardsIgnoreRobotsTxt(), err
		},
		Language:   a.Instance.DefaultLocale,
		LocalHosts: localHosts(cfg),
		Clock:      a.Clock, IDs: a.IDs, Logger: a.Logger, Meter: a.Meter,
	})
	a.LinkCards.RegisterProjectors(a.Projectors)
	a.LinkCards.RegisterJobs(a.Handlers, a.Scheduler)
}

// localHosts returns the hosts of this instance, lowercase and without a port: the serving
// domain, the account domain, and the web client's origin.
func localHosts(cfg *config.Config) []string {
	hosts := []string{strings.ToLower(hostOnly(cfg.Host))}
	if cfg.AccountDomain != "" {
		hosts = append(hosts, strings.ToLower(hostOnly(cfg.AccountDomain)))
	}
	if u, err := url.Parse(cfg.WebOrigin); err == nil && u.Hostname() != "" {
		hosts = append(hosts, strings.ToLower(u.Hostname()))
	}
	slices.Sort(hosts)
	return slices.Compact(hosts)
}

// linkCardDomains reads the domain actions on the hosts of link cards from the moderation
// policy and the rules of the crawler from the instance module, which take canonical hosts.
type linkCardDomains struct{ a *App }

func (d linkCardDomains) FederationSuspended(ctx context.Context, host string) (bool, error) {
	h, err := federation.CanonicalHost(host)
	if err != nil {
		return false, nil
	}
	return d.a.ModerationPolicy.FederationSuspended(ctx, h)
}

func (d linkCardDomains) RejectsMedia(ctx context.Context, host string) (bool, error) {
	h, err := federation.CanonicalHost(host)
	if err != nil {
		return false, nil
	}
	return d.a.ModerationPolicy.RejectsMedia(ctx, h)
}

func (d linkCardDomains) FetchStopped(ctx context.Context, host string) (bool, error) {
	h, err := federation.CanonicalHost(host)
	if err != nil {
		return false, nil
	}
	crawl, err := d.a.Instance.LinkCardCrawl(ctx, h)
	return crawl.NoFetch, err
}

func (d linkCardDomains) ImagesStopped(ctx context.Context, host string) (bool, error) {
	h, err := federation.CanonicalHost(host)
	if err != nil {
		return false, nil
	}
	crawl, err := d.a.Instance.LinkCardCrawl(ctx, h)
	return crawl.NoImages, err
}

// HostRules implements the batch read of content.LinkCardDomains: the restrictions of the hosts and
// their parents with one query, and the crawler rules with another; a host that is not canonical
// is refused nothing, as each read answers.
func (d linkCardDomains) HostRules(ctx context.Context, hosts []string) (map[string]content.HostRule, error) {
	canonical := map[string]string{}
	var all []string
	for _, h := range hosts {
		c, err := federation.CanonicalHost(h)
		if err != nil {
			continue
		}
		canonical[h] = c
		if !slices.Contains(all, c) {
			all = append(all, c)
		}
	}
	restrictions, err := d.a.ModerationPolicy.DomainRestrictionsAmong(ctx, all)
	if err != nil {
		return nil, err
	}
	crawls, err := d.a.Instance.LinkCardCrawls(ctx, all)
	if err != nil {
		return nil, err
	}
	out := make(map[string]content.HostRule, len(hosts))
	for _, h := range hosts {
		c, ok := canonical[h]
		if !ok {
			out[h] = content.HostRule{}
			continue
		}
		rs := restrictions[c]
		out[h] = content.HostRule{
			Suspended: slices.Contains(rs, moderation.RestrictFederation), RejectsMedia: slices.Contains(rs, moderation.RestrictMediaFetch),
			FetchStopped: crawls[c].NoFetch, ImagesStopped: crawls[c].NoImages,
		}
	}
	return out, nil
}

// linkCardCreators verifies the fediverse:creator of pages with the remote actors.
type linkCardCreators struct{ a *App }

func (c linkCardCreators) VerifyCreator(ctx context.Context, handle, host string) (string, error) {
	if c.a.RemoteActors == nil {
		return "", nil
	}
	return c.a.RemoteActors.VerifyCreator(ctx, handle, host)
}
