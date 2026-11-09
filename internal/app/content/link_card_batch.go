// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/mycroblog/my/internal/domain/content"
)

// page returns LinkCards for the statuses of a page whose current revisions are revisions: it
// reads the link_cards setting once, the cards of the statuses, their images, and their creators
// together, and asks about each host once, so that presenting the page's cards makes a number of
// round trips that does not grow with its statuses. accounts reads the creators.
func (l *LinkCards) page(ctx context.Context, revisions map[string]content.Revision, accounts *pageAccounts) (*LinkCards, error) {
	on, err := l.enabled(ctx)
	if err != nil {
		return nil, err
	}
	p := *l
	p.d.Enabled = func(context.Context) (bool, error) { return on, nil }
	if l.d.Accounts != nil {
		p.d.Accounts = accounts
	}
	if !on {
		return &p, nil
	}
	// Only a revision without attachments or a poll shows a card (View).
	var ids []string
	for id, rev := range revisions {
		if len(rev.Attachments) == 0 && rev.Poll == nil {
			ids = append(ids, id)
		}
	}
	repo := &pageCardRepository{LinkCardRepository: l.d.Repository, statuses: ids}
	if repo.urls, err = l.d.Repository.StatusLinkCardURLs(ctx, ids); err != nil {
		return nil, err
	}
	for _, u := range repo.urls {
		if !slices.Contains(repo.cardURLs, u) {
			repo.cardURLs = append(repo.cardURLs, u)
		}
	}
	if repo.cards, err = l.d.Repository.FindLinkCards(ctx, repo.cardURLs); err != nil {
		return nil, err
	}
	var creators []string
	for _, c := range repo.cards {
		if c.ImageMediaID != "" && l.d.Proxy != nil && !slices.Contains(repo.mediaIDs, c.ImageMediaID) {
			repo.mediaIDs = append(repo.mediaIDs, c.ImageMediaID)
		}
		if c.CreatorAccountID != "" {
			creators = append(creators, c.CreatorAccountID)
		}
	}
	if repo.media, err = l.d.Repository.FindLinkCardMedias(ctx, repo.mediaIDs); err != nil {
		return nil, err
	}
	if l.d.Accounts != nil {
		if err := accounts.load(ctx, creators); err != nil {
			return nil, err
		}
	}
	p.d.Repository = repo
	if l.d.Domains != nil {
		p.d.Domains = &pageCardDomains{LinkCardDomains: l.d.Domains, answers: map[[2]string]bool{}}
	}
	return &p, nil
}

// pageCardRepository is the link card repository with the cards of a page's statuses read
// beforehand. A card or image that the page asked for and does not exist fails with ErrNotFound,
// as its read does.
type pageCardRepository struct {
	LinkCardRepository
	statuses []string
	urls     map[string]string
	cardURLs []string
	cards    map[string]content.LinkCard
	mediaIDs []string
	media    map[string]content.LinkCardMedia
}

func (p *pageCardRepository) StatusLinkCardURL(ctx context.Context, statusID string) (string, error) {
	if slices.Contains(p.statuses, statusID) {
		if u, ok := p.urls[statusID]; ok {
			return u, nil
		}
		return "", fmt.Errorf("status link card %s: %w", statusID, ErrNotFound)
	}
	return p.LinkCardRepository.StatusLinkCardURL(ctx, statusID)
}

func (p *pageCardRepository) FindLinkCard(ctx context.Context, url string) (content.LinkCard, error) {
	if slices.Contains(p.cardURLs, url) {
		if c, ok := p.cards[url]; ok {
			return c, nil
		}
		return content.LinkCard{}, fmt.Errorf("link card %s: %w", url, ErrNotFound)
	}
	return p.LinkCardRepository.FindLinkCard(ctx, url)
}

func (p *pageCardRepository) FindLinkCardMedia(ctx context.Context, mediaID string) (content.LinkCardMedia, error) {
	if slices.Contains(p.mediaIDs, mediaID) {
		if m, ok := p.media[mediaID]; ok {
			return m, nil
		}
		return content.LinkCardMedia{}, fmt.Errorf("link card media %s: %w", mediaID, ErrNotFound)
	}
	return p.LinkCardRepository.FindLinkCardMedia(ctx, mediaID)
}

// pageCardDomains remembers the answers about the hosts of a page's cards, so that each question
// about a host is asked once for the page.
type pageCardDomains struct {
	LinkCardDomains
	mu      sync.Mutex
	answers map[[2]string]bool
}

func (p *pageCardDomains) ask(ctx context.Context, question, host string, read func(context.Context, string) (bool, error)) (bool, error) {
	key := [2]string{question, host}
	p.mu.Lock()
	v, ok := p.answers[key]
	p.mu.Unlock()
	if ok {
		return v, nil
	}
	v, err := read(ctx, host)
	if err != nil {
		return false, err
	}
	p.mu.Lock()
	p.answers[key] = v
	p.mu.Unlock()
	return v, nil
}

func (p *pageCardDomains) FederationSuspended(ctx context.Context, host string) (bool, error) {
	return p.ask(ctx, "suspended", host, p.LinkCardDomains.FederationSuspended)
}

func (p *pageCardDomains) RejectsMedia(ctx context.Context, host string) (bool, error) {
	return p.ask(ctx, "media", host, p.LinkCardDomains.RejectsMedia)
}

func (p *pageCardDomains) FetchStopped(ctx context.Context, host string) (bool, error) {
	return p.ask(ctx, "fetch", host, p.LinkCardDomains.FetchStopped)
}

func (p *pageCardDomains) ImagesStopped(ctx context.Context, host string) (bool, error) {
	return p.ask(ctx, "images", host, p.LinkCardDomains.ImagesStopped)
}
