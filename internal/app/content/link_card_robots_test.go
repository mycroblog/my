// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	stdiotest "testing/iotest"
	"time"

	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	domain "github.com/mycroblog/my/internal/domain/content"
)

// robotsCardsEnv runs LinkCards.Fetch on fakes that keep what ADR-0131 bears on: the cards, the
// store with its expiries, and a web that follows redirects as the HTTPClient does, asking the
// request's CheckRedirect before requesting each hop.
type robotsCardsEnv struct {
	t      *testing.T
	clock  *robotsClock
	kv     *robotsKV
	repo   *robotsCardRepo
	web    *robotsWeb
	ignore bool
	domain *robotsDomains
	cards  *content.LinkCards
}

func newRobotsCardsEnv(t *testing.T) *robotsCardsEnv {
	t.Helper()
	clock := &robotsClock{now: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)}
	e := &robotsCardsEnv{
		t: t, clock: clock, kv: &robotsKV{clock: clock, entries: map[string]robotsKVEntry{}},
		repo:   &robotsCardRepo{links: map[string]string{}, cards: map[string]domain.LinkCard{}, media: map[string]domain.LinkCardMedia{}},
		web:    &robotsWeb{pages: map[string]robotsResponse{}, redirects: map[string]string{}},
		domain: &robotsDomains{refused: map[string]bool{}},
	}
	e.cards = content.NewLinkCards(content.LinkCardsDeps{
		UnitOfWork: robotsUOW{}, Repository: e.repo, HTTP: e.web, KV: e.kv, Domains: e.domain, Clock: clock,
		Logger:          slog.New(slog.DiscardHandler),
		IgnoreRobotsTxt: func(context.Context) (bool, error) { return e.ignore, nil },
	})
	return e
}

// fetch runs the job of a status that links u.
func (e *robotsCardsEnv) fetch(u string) {
	e.t.Helper()
	id := "status:" + u
	e.repo.links[id] = u
	if retry, err := e.cards.Fetch(e.t.Context(), id); err != nil || retry {
		e.t.Fatalf("Fetch(%s) = %v, %v", u, retry, err)
	}
}

// page makes u a page with a title and, when image is set, an image.
func (e *robotsCardsEnv) page(u, title, image string) {
	body := `<html><head><title>` + title + `</title><meta property="og:description" content="words">`
	if image != "" {
		body += `<meta property="og:image" content="` + image + `">`
	}
	e.web.pages[u] = robotsResponse{status: http.StatusOK, contentType: "text/html", body: body + `</head></html>`}
}

func (e *robotsCardsEnv) robots(origin, body string) {
	e.web.pages[origin+"/robots.txt"] = robotsResponse{status: http.StatusOK, contentType: "text/plain", body: body}
}

// stale records a card of u that a fetch would replace.
func (e *robotsCardsEnv) stale(u string) {
	e.repo.cards[u] = domain.LinkCard{URL: u, CardURL: u, Title: "Kept", FetchedAt: e.clock.Now().Add(-domain.LinkCardRefetchAfter)}
}

const (
	robotsDisallowAll = "User-agent: *\nDisallow: /\n"
	robotsOrigin      = "https://site.example"
	robotsPage        = robotsOrigin + "/a"
	robotsFile        = robotsOrigin + "/robots.txt"
)

// TestLinkCardRobotsResponses checks each row of the table of responses in ADR-0131 §2.
func TestLinkCardRobotsResponses(t *testing.T) {
	t.Parallel()
	text := func(status int, body string) robotsResponse {
		return robotsResponse{status: status, contentType: "text/plain", body: body}
	}
	const day, hour = 24 * time.Hour, time.Hour
	cases := []struct {
		name     string
		response robotsResponse
		// allowed is whether the page is fetched; ttl how long the result is kept.
		allowed bool
		ttl     time.Duration
	}{
		{"200 without a rule on the URL", text(200, "User-agent: *\nDisallow: /other\n"), true, day},
		{"200 with a rule on the URL", text(200, robotsDisallowAll), false, day},
		{"200 naming the crawler", text(200, "User-agent: MY-LINKCARD\nDisallow: /\n\nUser-agent: *\nAllow: /\n"), false, day},
		{"200 naming another bot", text(200, "User-agent: otherbot\nDisallow: /\n"), true, day},
		{"200 with an empty body", text(200, ""), true, day},
		{"204", text(204, ""), true, day},
		{"400", text(400, robotsDisallowAll), true, day},
		{"401", text(401, robotsDisallowAll), true, day},
		{"403", text(403, robotsDisallowAll), true, day},
		{"404", text(404, robotsDisallowAll), true, day},
		{"410", text(410, robotsDisallowAll), true, day},
		{"429", text(429, ""), false, hour},
		{"500", text(500, ""), false, hour},
		{"503", text(503, ""), false, hour},
		{"a 3xx that is not followed", text(304, robotsDisallowAll), true, day},
		{"a network failure", robotsResponse{err: errors.New("connection refused")}, false, hour},
		{"a timeout", robotsResponse{err: context.DeadlineExceeded}, false, hour},
		{"a redirect that the guard refuses", robotsResponse{err: fmt.Errorf("redirect: %w", kernel.ErrForbiddenDestination)}, false, hour},
		{"too many redirects", robotsResponse{err: fmt.Errorf("redirect: %w", kernel.ErrTooManyRedirects)}, true, day},
		{"a body that fails on the way", robotsResponse{status: 200, reader: stdiotest.ErrReader(errors.New("reset"))}, false, hour},
		{"a body over the limit, its whole lines allowing", robotsResponse{status: 200, reader: readThenTooLarge("User-agent: *\nDisallow: /other\nDisallow: /a")}, true, day},
		{"a body over the limit, its whole lines refusing", robotsResponse{status: 200, reader: readThenTooLarge("User-agent: *\nDisallow: /a\nDisallow: /oth")}, false, day},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newRobotsCardsEnv(t)
			e.page(robotsPage, "Page", "")
			e.web.pages[robotsFile] = tc.response
			e.fetch(robotsPage)

			if got := e.web.count(robotsPage) == 1; got != tc.allowed {
				t.Errorf("page fetched = %v, want %v", got, tc.allowed)
			}
			if _, ok := e.repo.cards[robotsPage]; ok != tc.allowed {
				t.Errorf("card = %v, want %v", ok, tc.allowed)
			}
			if got := e.kv.ttl(content.LinkCardRobotsNamespace, robotsOrigin); got != tc.ttl {
				t.Errorf("kept for %v, want %v", got, tc.ttl)
			}
			if got := e.kv.ttl(content.LinkCardFailureNamespace, ""); tc.allowed == (got != 0) {
				t.Errorf("failure memory for %v, with the page allowed %v", got, tc.allowed)
			} else if !tc.allowed && got != content.LinkCardFailureTTL {
				t.Errorf("failure memory for %v, want %v", got, content.LinkCardFailureTTL)
			}
			req := e.web.request(robotsFile)
			if req == nil || req.Purpose != kernel.HTTPPurposeRobots || req.Agent != kernel.HTTPAgentLinkCard || req.CheckRedirect == nil {
				t.Errorf("request of robots.txt = %+v", req)
			}
		})
	}
}

// TestLinkCardRobotsRefusedRedirect checks a robots.txt that redirects to a host that the
// operator stopped: nothing is requested there, and the file counts as unreachable.
func TestLinkCardRobotsRefusedRedirect(t *testing.T) {
	t.Parallel()
	e := newRobotsCardsEnv(t)
	e.page(robotsPage, "Page", "")
	e.domain.refused["stopped.example"] = true
	e.web.redirects[robotsFile] = "https://stopped.example/robots.txt"
	e.fetch(robotsPage)
	if e.web.count("https://stopped.example/robots.txt") != 0 || e.web.count(robotsPage) != 0 {
		t.Errorf("requests = %v", e.web.hits())
	}
	if got := e.kv.ttl(content.LinkCardRobotsNamespace, robotsOrigin); got != time.Hour {
		t.Errorf("kept for %v", got)
	}
}

// TestLinkCardRobotsRedirectKeepsItsOrigin checks that a redirected file applies to the origin
// that was asked (RFC 9309 §2.3.1.2), across hosts.
func TestLinkCardRobotsRedirectKeepsItsOrigin(t *testing.T) {
	t.Parallel()
	e := newRobotsCardsEnv(t)
	e.page(robotsPage, "Page", "")
	e.web.redirects[robotsFile] = "https://cdn.example/site-robots.txt"
	e.web.pages["https://cdn.example/site-robots.txt"] = robotsResponse{status: 200, body: robotsDisallowAll}
	e.fetch(robotsPage)
	if e.web.count(robotsPage) != 0 {
		t.Error("fetched a page that the redirected file disallows")
	}
	if e.kv.ttl(content.LinkCardRobotsNamespace, "https://cdn.example") != 0 {
		t.Error("kept the rules for the redirect's origin")
	}
}

// TestLinkCardRobotsCache checks that one origin asks once within the period, what it keeps, and
// that origins are kept apart by scheme, host, and port.
func TestLinkCardRobotsCache(t *testing.T) {
	t.Parallel()
	e := newRobotsCardsEnv(t)
	e.page(robotsPage, "A", "")
	e.page(robotsOrigin+"/b", "B", "")
	e.page(robotsOrigin+"/secret/x", "Secret", "")
	e.page("https://other.example/secret/x", "Other", "")
	e.page("https://site.example:8443/secret/x", "Port", "")
	e.robots(robotsOrigin, "# a comment for humans\nUser-agent: otherbot\nDisallow: /otherbot-path\n\nUser-agent: *\nDisallow: /secret/\nSitemap: https://site.example/sitemap-file.xml\n")
	e.robots("https://site.example:8443", robotsDisallowAll)

	e.fetch(robotsPage)
	e.fetch(robotsOrigin + "/b")
	e.fetch(robotsOrigin + "/secret/x")
	if got := e.web.count(robotsFile); got != 1 {
		t.Errorf("robots.txt fetched %d times, want once", got)
	}
	if _, ok := e.repo.cards[robotsOrigin+"/secret/x"]; ok || e.web.count(robotsOrigin+"/secret/x") != 0 {
		t.Error("fetched a page that the file disallows")
	}
	kept := e.kv.value(content.LinkCardRobotsNamespace, robotsOrigin)
	if !strings.Contains(kept, "/secret/") {
		t.Errorf("kept %q, without the rule that applies", kept)
	}
	for _, other := range []string{"comment", "otherbot", "sitemap"} {
		if strings.Contains(kept, other) {
			t.Errorf("kept %q, with %s", kept, other)
		}
	}

	// Another host has rules of its own, and so does another port.
	e.fetch("https://other.example/secret/x")
	if _, ok := e.repo.cards["https://other.example/secret/x"]; !ok {
		t.Error("the rules of one origin applied to another host")
	}
	e.fetch("https://site.example:8443/secret/x")
	if e.web.count("https://site.example:8443/robots.txt") != 1 || e.web.count("https://site.example:8443/secret/x") != 0 {
		t.Errorf("another port: %v", e.web.hits())
	}

	// After the period, the file is read again and its new rules apply.
	e.clock.Advance(content.LinkCardRobotsTTL)
	e.page(robotsOrigin+"/secret/y", "Now open", "")
	e.robots(robotsOrigin, "User-agent: *\nAllow: /\n")
	e.fetch(robotsOrigin + "/secret/y")
	if e.web.count(robotsFile) != 2 {
		t.Errorf("robots.txt fetched %d times after its period, want twice", e.web.count(robotsFile))
	}
	if _, ok := e.repo.cards[robotsOrigin+"/secret/y"]; !ok {
		t.Error("the new rules did not apply")
	}
}

// TestLinkCardRobotsUnreachableRecovers checks that a failure is kept for an hour, so that another
// URL of the origin gets a card once the file is back, while the refused URL waits its 24 hours.
func TestLinkCardRobotsUnreachableRecovers(t *testing.T) {
	t.Parallel()
	e := newRobotsCardsEnv(t)
	e.page(robotsPage, "A", "")
	e.page(robotsOrigin+"/b", "B", "")
	e.web.pages[robotsFile] = robotsResponse{status: http.StatusServiceUnavailable}
	e.fetch(robotsPage)
	e.fetch(robotsOrigin + "/b")
	if len(e.repo.cards) != 0 || e.web.count(robotsFile) != 1 {
		t.Errorf("cards %v after %d reads of the file", e.repo.cards, e.web.count(robotsFile))
	}

	e.clock.Advance(time.Hour)
	delete(e.web.pages, robotsFile)
	e.page(robotsOrigin+"/c", "C", "")
	e.fetch(robotsOrigin + "/c")
	if _, ok := e.repo.cards[robotsOrigin+"/c"]; !ok {
		t.Error("no card for another URL of the origin once the file is back")
	}
	e.fetch(robotsPage)
	if e.web.count(robotsPage) != 0 {
		t.Error("a refused URL was fetched within its failure memory")
	}
	e.clock.Advance(content.LinkCardFailureTTL)
	e.fetch(robotsPage)
	if _, ok := e.repo.cards[robotsPage]; !ok {
		t.Error("no card once the file and the memory are gone")
	}
}

// TestLinkCardRobotsStoreFailure checks that a failure of the store is an error for the job, and
// that the end of the job's context keeps nothing about the file.
func TestLinkCardRobotsStoreFailure(t *testing.T) {
	t.Parallel()
	e := newRobotsCardsEnv(t)
	e.page(robotsPage, "A", "")
	e.repo.links["s"] = robotsPage
	e.kv.getErr = map[string]error{content.LinkCardRobotsNamespace: errors.New("store down")}
	if _, err := e.cards.Fetch(t.Context(), "s"); err == nil {
		t.Error("a failure of the store is not an error")
	}
	if e.web.count(robotsPage) != 0 {
		t.Error("fetched the page without knowing the rules")
	}

	e = newRobotsCardsEnv(t)
	e.page(robotsPage, "A", "")
	e.repo.links["s"] = robotsPage
	ctx, cancel := context.WithCancel(t.Context())
	e.web.pages[robotsFile] = robotsResponse{err: context.Canceled, before: cancel}
	if _, err := e.cards.Fetch(ctx, "s"); !errors.Is(err, context.Canceled) {
		t.Errorf("Fetch with its context ended = %v", err)
	}
	if e.kv.ttl(content.LinkCardRobotsNamespace, robotsOrigin) != 0 {
		t.Error("kept a result of a request that the job gave up")
	}
}

// TestLinkCardRobotsRedirects checks the hops of a page and an image against the rules of their
// own origins, before requesting them.
func TestLinkCardRobotsRedirects(t *testing.T) {
	t.Parallel()
	const short, final = "https://short.example/x", "https://final.example/page"
	t.Run("a disallowed final URL is not requested and makes no card", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.page(final, "Moved", "")
		e.web.redirects[short] = final
		e.robots("https://final.example", robotsDisallowAll)
		e.fetch(short)
		if e.web.count(final) != 0 {
			t.Error("requested a URL that its origin disallows")
		}
		if _, ok := e.repo.cards[short]; ok {
			t.Error("made a card")
		}
		if e.kv.value(content.LinkCardFailureNamespace, "") != "robots" {
			t.Errorf("failure memory = %q", e.kv.value(content.LinkCardFailureNamespace, ""))
		}
	})
	t.Run("an allowed final URL is judged by its own origin", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.page(final, "Moved", "")
		e.web.redirects[short] = final
		e.robots("https://short.example", "User-agent: *\nDisallow: /other\n")
		e.robots("https://final.example", "User-agent: *\nDisallow: /other\n")
		e.fetch(short)
		if _, ok := e.repo.cards[short]; !ok {
			t.Error("no card")
		}
		if e.web.count("https://final.example/robots.txt") != 1 {
			t.Error("the final URL's origin was not asked")
		}
	})
	t.Run("the rules of the first origin do not judge the final URL", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.page(final, "Moved", "")
		e.web.redirects[short] = final
		e.robots("https://short.example", "User-agent: *\nDisallow: /page\n")
		e.fetch(short)
		if _, ok := e.repo.cards[short]; !ok {
			t.Error("no card")
		}
	})
	t.Run("a refused hop removes a card only when the file says so", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.page(final, "Moved", "")
		e.web.redirects[short] = final
		e.stale(short)
		e.web.pages["https://final.example/robots.txt"] = robotsResponse{status: http.StatusBadGateway}
		e.fetch(short)
		if _, ok := e.repo.cards[short]; !ok {
			t.Error("an unreachable file removed the card")
		}

		e = newRobotsCardsEnv(t)
		e.page(final, "Moved", "")
		e.web.redirects[short] = final
		e.stale(short)
		e.robots("https://final.example", robotsDisallowAll)
		e.fetch(short)
		if _, ok := e.repo.cards[short]; ok {
			t.Error("a disallowed final URL kept the card")
		}
	})
	t.Run("an image's redirect is checked too", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.page(robotsPage, "Page", robotsOrigin+"/a.png")
		e.web.redirects[robotsOrigin+"/a.png"] = "https://images.example/a.png"
		e.robots("https://images.example", robotsDisallowAll)
		e.fetch(robotsPage)
		if c, ok := e.repo.cards[robotsPage]; !ok || c.ImageMediaID != "" {
			t.Errorf("card = %+v, %v", c, ok)
		}
		if e.web.count("https://images.example/a.png") != 0 {
			t.Error("requested an image that its origin disallows")
		}
	})
}

// TestLinkCardRobotsImage checks that the image is judged by its own origin.
func TestLinkCardRobotsImage(t *testing.T) {
	t.Parallel()
	const image = "https://images.example/a.png"
	known := domain.LinkCardMedia{MediaID: "known", RemoteURL: image}
	t.Run("a disallowed image on another origin", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.page(robotsPage, "Page", image)
		e.robots("https://images.example", robotsDisallowAll)
		e.repo.media[image] = known
		e.fetch(robotsPage)
		if c, ok := e.repo.cards[robotsPage]; !ok || c.ImageMediaID != "" || c.Description != "words" {
			t.Errorf("card = %+v, %v", c, ok)
		}
		if e.web.count(image) != 0 {
			t.Error("requested the image")
		}
		if e.kv.ttl(content.LinkCardFailureNamespace, "") != 0 {
			t.Error("remembered the page as failed")
		}
	})
	t.Run("an unreachable file of the image's origin", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.page(robotsPage, "Page", image)
		e.web.pages["https://images.example/robots.txt"] = robotsResponse{status: http.StatusServiceUnavailable}
		e.fetch(robotsPage)
		if c, ok := e.repo.cards[robotsPage]; !ok || c.ImageMediaID != "" || e.web.count(image) != 0 {
			t.Errorf("card = %+v, %v after %d requests", c, ok, e.web.count(image))
		}
	})
	t.Run("the page's origin does not judge the image", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.page(robotsPage, "Page", image)
		e.robots(robotsOrigin, "User-agent: *\nDisallow: /a.png\n")
		e.repo.media[image] = known
		e.fetch(robotsPage)
		if c := e.repo.cards[robotsPage]; c.ImageMediaID != "known" {
			t.Errorf("card = %+v", c)
		}
	})
	t.Run("a page disallowed on an allowed image's origin", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.page(robotsPage, "Page", image)
		e.robots(robotsOrigin, robotsDisallowAll)
		e.fetch(robotsPage)
		if e.web.count("https://images.example/robots.txt") != 0 || e.web.count(image) != 0 {
			t.Errorf("requests = %v", e.web.hits())
		}
	})
}

// TestLinkCardRobotsRemoval checks the card of a refetch, and that nothing rechecks a fresh one.
func TestLinkCardRobotsRemoval(t *testing.T) {
	t.Parallel()
	t.Run("a real disallow removes the card", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.page(robotsPage, "New", "")
		e.stale(robotsPage)
		e.robots(robotsOrigin, robotsDisallowAll)
		e.fetch(robotsPage)
		if _, ok := e.repo.cards[robotsPage]; ok {
			t.Error("the card stays")
		}
		if e.web.count(robotsPage) != 0 {
			t.Error("requested the page")
		}
	})
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprintf("%d keeps the card", status), func(t *testing.T) {
			t.Parallel()
			e := newRobotsCardsEnv(t)
			e.page(robotsPage, "New", "")
			e.stale(robotsPage)
			e.web.pages[robotsFile] = robotsResponse{status: status}
			e.fetch(robotsPage)
			if c, ok := e.repo.cards[robotsPage]; !ok || c.Title != "Kept" {
				t.Errorf("card = %+v, %v", c, ok)
			}
			if e.repo.deleted != 0 {
				t.Errorf("%d cards deleted", e.repo.deleted)
			}
		})
	}
	t.Run("a network failure keeps the card", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.stale(robotsPage)
		e.web.pages[robotsFile] = robotsResponse{err: errors.New("no route")}
		e.fetch(robotsPage)
		if _, ok := e.repo.cards[robotsPage]; !ok {
			t.Error("the card is gone")
		}
	})
	t.Run("a refetch that is allowed replaces the card", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.page(robotsPage, "New", "")
		e.stale(robotsPage)
		e.fetch(robotsPage)
		if e.repo.cards[robotsPage].Title != "New" {
			t.Errorf("card = %+v", e.repo.cards[robotsPage])
		}
	})
	t.Run("a refetch whose image is disallowed drops the image", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.page(robotsPage, "New", "https://images.example/a.png")
		e.repo.cards[robotsPage] = domain.LinkCard{URL: robotsPage, Title: "Kept", ImageMediaID: "known", FetchedAt: e.clock.Now().Add(-domain.LinkCardRefetchAfter)}
		e.repo.media["https://images.example/a.png"] = domain.LinkCardMedia{MediaID: "known", RemoteURL: "https://images.example/a.png"}
		e.robots("https://images.example", robotsDisallowAll)
		e.fetch(robotsPage)
		if c := e.repo.cards[robotsPage]; c.Title != "New" || c.ImageMediaID != "" {
			t.Errorf("card = %+v", c)
		}
	})
	t.Run("a fresh card is not checked", func(t *testing.T) {
		t.Parallel()
		e := newRobotsCardsEnv(t)
		e.repo.cards[robotsPage] = domain.LinkCard{URL: robotsPage, Title: "Fresh", FetchedAt: e.clock.Now()}
		e.robots(robotsOrigin, robotsDisallowAll)
		e.fetch(robotsPage)
		if len(e.web.hits()) != 0 {
			t.Errorf("requests = %v", e.web.hits())
		}
		if _, ok := e.repo.cards[robotsPage]; !ok {
			t.Error("the card is gone")
		}
	})
}

// TestLinkCardRobotsIgnored checks link_cards_ignore_robots_txt: on, the file is neither fetched
// nor read from the store; off again, the next fetch obeys it.
func TestLinkCardRobotsIgnored(t *testing.T) {
	t.Parallel()
	const image = "https://images.example/a.png"
	e := newRobotsCardsEnv(t)
	e.page(robotsPage, "Page", image)
	e.repo.media[image] = domain.LinkCardMedia{MediaID: "known", RemoteURL: image}
	e.robots(robotsOrigin, robotsDisallowAll)
	e.robots("https://images.example", robotsDisallowAll)

	// Refused first, so that rules and the failure memory are kept.
	e.fetch(robotsPage)
	if _, ok := e.repo.cards[robotsPage]; ok {
		t.Fatal("the setting is off and the card exists")
	}

	e.ignore = true
	e.fetch(robotsPage)
	c, ok := e.repo.cards[robotsPage]
	if !ok || c.ImageMediaID != "known" {
		t.Errorf("card with the setting on = %+v, %v", c, ok)
	}
	if e.web.count(robotsFile) != 1 || e.web.count("https://images.example/robots.txt") != 0 {
		t.Errorf("robots.txt fetched with the setting on: %v", e.web.hits())
	}
	if req := e.web.request(robotsPage); req == nil || req.Agent != kernel.HTTPAgentLinkCard {
		t.Errorf("the page's request with the setting on = %+v", req)
	}

	// A stale card of another URL is not removed while the setting is on.
	e.page(robotsOrigin+"/b", "B", "")
	e.stale(robotsOrigin + "/b")
	e.fetch(robotsOrigin + "/b")
	if e.repo.cards[robotsOrigin+"/b"].Title != "B" {
		t.Errorf("card with the setting on = %+v", e.repo.cards[robotsOrigin+"/b"])
	}

	// Off again: a fresh card stays until its refetch, which obeys the file.
	e.ignore = false
	e.fetch(robotsPage)
	if _, ok := e.repo.cards[robotsPage]; !ok {
		t.Error("turning the setting off removed a fresh card")
	}
	e.clock.Advance(domain.LinkCardRefetchAfter)
	e.fetch(robotsPage)
	if _, ok := e.repo.cards[robotsPage]; ok {
		t.Error("the refetch did not obey the file")
	}
}

// TestLinkCardRobotsIgnoredKeepsTheOtherRefusals checks that the setting overrules only what
// robots.txt refused: domain rules, page failures, and page directives still apply.
func TestLinkCardRobotsIgnoredKeepsTheOtherRefusals(t *testing.T) {
	t.Parallel()
	e := newRobotsCardsEnv(t)
	e.ignore = true

	e.page("https://stopped.example/a", "Stopped", "")
	e.domain.refused["stopped.example"] = true
	e.fetch("https://stopped.example/a")
	if e.web.count("https://stopped.example/a") != 0 {
		t.Error("the setting overruled a rule of the crawler")
	}

	// A failure of the page stays remembered.
	e.web.pages[robotsPage] = robotsResponse{status: http.StatusNotFound}
	e.fetch(robotsPage)
	e.fetch(robotsPage)
	if got := e.web.count(robotsPage); got != 1 {
		t.Errorf("a failed page was fetched %d times, want once", got)
	}

	e.web.pages[robotsOrigin+"/d"] = robotsResponse{
		status: http.StatusOK, contentType: "text/html", header: http.Header{"X-Robots-Tag": {"nosnippet"}},
		body: `<head><title>T</title><meta name="description" content="words"><meta property="og:image" content="/a.png"><meta name="robots" content="noimageindex"></head>`,
	}
	e.fetch(robotsOrigin + "/d")
	if c := e.repo.cards[robotsOrigin+"/d"]; c.Title != "T" || c.Description != "" || c.ImageMediaID != "" {
		t.Errorf("card = %+v", c)
	}
	if e.web.count(robotsOrigin+"/a.png") != 0 {
		t.Error("fetched an image that the page refuses")
	}
}

// TestLinkCardDirectivesOfTheResponse checks X-Robots-Tag through a fetch, in which the headers
// and the head of the page add up.
func TestLinkCardDirectivesOfTheResponse(t *testing.T) {
	t.Parallel()
	const image = "https://images.example/a.png"
	head := `<head><title>T</title><meta name="description" content="words"><meta property="og:image" content="` + image + `">`
	cases := []struct {
		name, head   string
		header       []string
		description  string
		imageFetched bool
	}{
		{"nothing", "", nil, "words", true},
		{"nosnippet", "", []string{"nosnippet"}, "", true},
		{"max-snippet:0", "", []string{"max-snippet:0"}, "", true},
		{"noimageindex", "", []string{"noimageindex"}, "words", false},
		{"max-image-preview:none", "", []string{"max-image-preview:none"}, "words", false},
		{"noindex and friends", "", []string{"noindex, nofollow, noarchive", "none"}, "words", true},
		{"this crawler by name", "", []string{"my-linkcard: nosnippet"}, "", true},
		{"this crawler by name in another case", "", []string{"My-LinkCard: NoSnippet"}, "", true},
		{"another bot by name", "", []string{"googlebot: nosnippet, noimageindex"}, "words", true},
		{"another bot's header beside one for all", "", []string{"googlebot: noimageindex", "nosnippet"}, "", true},
		{"the head and the header add up", `<meta name="robots" content="nosnippet">`, []string{"noimageindex"}, "", false},
		{"a loose header does not undo a strict head", `<meta name="my-linkcard" content="nosnippet">`, []string{"max-snippet:-1"}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newRobotsCardsEnv(t)
			e.web.pages[robotsPage] = robotsResponse{
				status: http.StatusOK, contentType: "text/html", header: http.Header{"X-Robots-Tag": tc.header},
				body: head + tc.head + `</head>`,
			}
			e.fetch(robotsPage)
			c, ok := e.repo.cards[robotsPage]
			if !ok || c.Title != "T" || c.Description != tc.description {
				t.Errorf("card = %+v, %v", c, ok)
			}
			if got := e.web.count(image) == 1; got != tc.imageFetched {
				t.Errorf("image fetched = %v, want %v", got, tc.imageFetched)
			}
		})
	}
}

// TestLinkCardAgent checks who the three fetches of a card say they are.
func TestLinkCardAgent(t *testing.T) {
	t.Parallel()
	const image = "https://images.example/a.png"
	e := newRobotsCardsEnv(t)
	e.page(robotsPage, "Page", image)
	e.fetch(robotsPage)
	for _, u := range []string{robotsFile, robotsPage, "https://images.example/robots.txt", image} {
		if req := e.web.request(u); req == nil || req.Agent != kernel.HTTPAgentLinkCard {
			t.Errorf("request of %s = %+v", u, req)
		}
	}
}

// robotsClock is a clock that moves when a test says so.
type robotsClock struct{ now time.Time }

func (c *robotsClock) Now() time.Time          { return c.now }
func (c *robotsClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

// robotsResponse is the answer of a URL in robotsWeb.
type robotsResponse struct {
	status      int
	contentType string
	body        string
	header      http.Header
	reader      io.Reader
	err         error
	before      func()
}

// readThenTooLarge returns a reader of a body cut at the limit, as the HTTPClient's body is.
func readThenTooLarge(body string) io.Reader {
	return io.MultiReader(strings.NewReader(body), stdiotest.ErrReader(fmt.Errorf("body: %w", kernel.ErrResponseTooLarge)))
}

type robotsWeb struct {
	mu        sync.Mutex
	pages     map[string]robotsResponse
	redirects map[string]string
	requests  map[string]*kernel.HTTPRequest
	counts    map[string]int
}

func (w *robotsWeb) Do(ctx context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	w.record(req.URL, req)
	u := req.URL
	for hop := 0; ; hop++ {
		w.mu.Lock()
		to, ok := w.redirects[u]
		w.mu.Unlock()
		if !ok {
			break
		}
		if req.CheckRedirect != nil {
			if err := req.CheckRedirect(ctx, to); err != nil {
				return nil, fmt.Errorf("redirect to %s: %w", to, err)
			}
		}
		u = to
		w.record(u, req)
	}
	w.mu.Lock()
	p, ok := w.pages[u]
	w.mu.Unlock()
	if !ok {
		p = robotsResponse{status: http.StatusNotFound, contentType: "text/plain"}
	}
	if p.before != nil {
		p.before()
	}
	if p.err != nil {
		return nil, p.err
	}
	h := http.Header{"Content-Type": {p.contentType}}
	for k, v := range p.header {
		h[k] = v
	}
	var body io.Reader = strings.NewReader(p.body)
	if p.reader != nil {
		body = p.reader
	}
	return &kernel.HTTPResponse{StatusCode: p.status, URL: u, Header: h, Body: io.NopCloser(body)}, nil
}

func (w *robotsWeb) record(u string, req *kernel.HTTPRequest) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.counts == nil {
		w.counts, w.requests = map[string]int{}, map[string]*kernel.HTTPRequest{}
	}
	w.counts[u]++
	w.requests[u] = req
}

func (w *robotsWeb) count(u string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.counts[u]
}

func (w *robotsWeb) request(u string) *kernel.HTTPRequest {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.requests[u]
}

func (w *robotsWeb) hits() map[string]int {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make(map[string]int, len(w.counts))
	for k, v := range w.counts {
		out[k] = v
	}
	return out
}

// robotsKV is a store that expires its entries by a clock and keeps the TTL it was given.
type robotsKV struct {
	kernel.KeyValueStore
	clock   *robotsClock
	entries map[string]robotsKVEntry
	getErr  map[string]error
}

type robotsKVEntry struct {
	value   []byte
	ttl     time.Duration
	expires time.Time
}

func (k *robotsKV) Get(_ context.Context, ns, key string) ([]byte, error) {
	if err := k.getErr[ns]; err != nil {
		return nil, err
	}
	e, ok := k.entries[ns+"/"+key]
	if !ok || !k.clock.Now().Before(e.expires) {
		return nil, kernel.ErrKeyNotFound
	}
	return e.value, nil
}

func (k *robotsKV) Set(_ context.Context, ns, key string, value []byte, ttl time.Duration) error {
	k.entries[ns+"/"+key] = robotsKVEntry{value: bytes.Clone(value), ttl: ttl, expires: k.clock.Now().Add(ttl)}
	return nil
}

func (k *robotsKV) Delete(_ context.Context, ns, key string) error {
	delete(k.entries, ns+"/"+key)
	return nil
}

func (k *robotsKV) IncrBy(context.Context, string, string, int64, time.Duration) (int64, error) {
	return 1, nil
}

// find returns the entry of key in ns, or the only entry of ns for an empty key.
func (k *robotsKV) find(ns, key string) robotsKVEntry {
	if key != "" {
		return k.entries[ns+"/"+key]
	}
	for name, e := range k.entries {
		if strings.HasPrefix(name, ns+"/") {
			return e
		}
	}
	return robotsKVEntry{}
}

func (k *robotsKV) ttl(ns, key string) time.Duration { return k.find(ns, key).ttl }
func (k *robotsKV) value(ns, key string) string      { return string(k.find(ns, key).value) }

// robotsCardRepo keeps the cards in maps.
type robotsCardRepo struct {
	content.LinkCardRepository
	links   map[string]string
	cards   map[string]domain.LinkCard
	media   map[string]domain.LinkCardMedia
	deleted int
}

func (r *robotsCardRepo) StatusLinkCardURL(_ context.Context, id string) (string, error) {
	u, ok := r.links[id]
	if !ok {
		return "", content.ErrNotFound
	}
	return u, nil
}

func (r *robotsCardRepo) FindLinkCard(_ context.Context, u string) (domain.LinkCard, error) {
	c, ok := r.cards[u]
	if !ok {
		return c, content.ErrNotFound
	}
	return c, nil
}

func (r *robotsCardRepo) UpsertLinkCard(_ context.Context, _ uow.Tx, c domain.LinkCard) error {
	r.cards[c.URL] = c
	return nil
}

func (r *robotsCardRepo) DeleteLinkCard(_ context.Context, _ uow.Tx, u string) error {
	delete(r.cards, u)
	r.deleted++
	return nil
}

func (r *robotsCardRepo) LinkCardMediaByURL(_ context.Context, _ uow.Tx, u string) (domain.LinkCardMedia, error) {
	m, ok := r.media[u]
	if !ok {
		return m, content.ErrNotFound
	}
	return m, nil
}

type robotsUOW struct{ uow.UnitOfWork }

func (robotsUOW) Tx(_ context.Context, fn func(uow.Tx) error) error { return fn(nil) }

// robotsDomains refuses fetching from the listed hosts.
type robotsDomains struct{ refused map[string]bool }

func (d *robotsDomains) FederationSuspended(context.Context, string) (bool, error) { return false, nil }
func (d *robotsDomains) RejectsMedia(context.Context, string) (bool, error)        { return false, nil }
func (d *robotsDomains) ImagesStopped(context.Context, string) (bool, error)       { return false, nil }
func (d *robotsDomains) FetchStopped(_ context.Context, host string) (bool, error) {
	return d.refused[host], nil
}
