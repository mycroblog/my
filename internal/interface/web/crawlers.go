// SPDX-License-Identifier: AGPL-3.0-only

package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/i18n"
)

// This file states what the instance allows search engines and AI crawlers to do (ADR-0057):
// /robots.txt, the meta tags and headers of the pages, and the documents of /.well-known/ about
// it (tdmrep.json, security.txt).

// Paths of the documents about crawlers and security.
const (
	PathRobots        = "/robots.txt"
	PathTDMRep        = "/.well-known/tdmrep.json"
	PathSecurityTxt   = "/.well-known/security.txt"
	cacheCrawlerFiles = "public, max-age=3600"
	// securityTxtLifetime is how long after the response a security.txt expires (its Expires).
	securityTxtLifetime = 30 * 24 * time.Hour
)

// HeaderNoIndex is the X-Robots-Tag of the responses that are not pages: the client API,
// ActivityPub documents, and /.well-known/ (ADR-0057).
const HeaderNoIndex = "noindex"

// robotsDisallowed are the paths that no crawler needs: the APIs, the sign-in and OAuth screens,
// the email unsubscribe links, ActivityPub documents, and the media proxy. /.well-known/ and
// /nodeinfo stay open, for the statistics and server lists that read NodeInfo (FR-402).
var robotsDisallowed = []string{"/api/", "/oauth/", "/auth/", "/mail/", "/ap/", "/media/proxy/"}

// crawlerDirectives are the meta tags and headers that state a CrawlerPolicy on a response.
type crawlerDirectives struct {
	// Robots is the content of <meta name="robots"> and the X-Robots-Tag; "" for none.
	Robots string
	// ContentUsage is the Content-Usage header (IETF AI Preferences); "" for none.
	ContentUsage string
	// TDMReservation is the tdm-reservation of the TDM Reservation Protocol.
	TDMReservation bool
}

// directivesOf returns the directives that state p: noindex and noarchive when search is denied,
// noai and noimageai, a Content-Usage, and tdm-reservation when AI is reserved.
func directivesOf(p instance.CrawlerPolicy) crawlerDirectives {
	var robots []string
	if !p.IndexAllowed() {
		robots = append(robots, "noindex", "noarchive")
	}
	var d crawlerDirectives
	if p.DeniesTraining() {
		robots = append(robots, "noai", "noimageai")
		d.ContentUsage, d.TDMReservation = "train-ai=n", true
		if p.DeniesAIUse() {
			d.ContentUsage += ", ai-use=n"
		}
	}
	d.Robots = strings.Join(robots, ", ")
	return d
}

// set sets the headers of d on h.
func (d crawlerDirectives) set(h http.Header) {
	if d.Robots != "" {
		h.Set("X-Robots-Tag", d.Robots)
	}
	if d.ContentUsage != "" {
		h.Set("Content-Usage", d.ContentUsage)
	}
	if d.TDMReservation {
		h.Set("tdm-reservation", "1")
	}
}

// SetMediaHeaders sets the headers that state the instance's policy on a media file that the
// server serves (ADR-0057). The media of an account carry only the instance's statement: their
// keys do not name the account, and a CDN may serve them directly.
func SetMediaHeaders(h http.Header, s instance.Settings) {
	directivesOf(s.CrawlerPolicy()).set(h)
}

// settings returns the current instance settings; the zero Settings (every setting at its
// default, which denies crawlers) without Options.Settings.
func (h *Handler) settings(ctx context.Context) (instance.Settings, error) {
	if h.o.Settings == nil {
		return instance.Settings{}, nil
	}
	return h.o.Settings(ctx)
}

// robotsTxt renders /robots.txt for the instance's policy (RFC 9309): the AI crawlers of the
// bundled list that p reserves against, then the rules for every crawler, with the draft
// statements of IETF AI Preferences (Content-Usage) and Content Signals, which parsers that do
// not know them ignore. Search engines are never disallowed the pages: a search engine that may
// not fetch a page cannot read its noindex, and link preview fetchers follow robots.txt too.
func robotsTxt(p instance.CrawlerPolicy) []byte {
	var b bytes.Buffer
	b.WriteString("# The AI crawlers are from " + aiCrawlersSource + " (MIT).\n")
	group := func(comment string, agents []string) {
		b.WriteString("\n# " + comment + "\n")
		for _, a := range agents {
			b.WriteString("User-agent: " + a + "\n")
		}
		b.WriteString("Disallow: /\n")
	}
	if p.DeniesTraining() {
		group("Crawlers that collect data to train AI models.", aiTrainingCrawlers)
	}
	if p.DeniesAIUse() {
		group("Crawlers of AI search, assistants, and agents.", aiUseCrawlers)
	}
	yn := func(allowed bool, yes, no string) string {
		if allowed {
			return yes
		}
		return no
	}
	search := p.IndexAllowed()
	train, use := !p.DeniesTraining(), !p.DeniesAIUse()
	b.WriteString("\nUser-agent: *\n")
	b.WriteString("Content-Usage: search=" + yn(search, "y", "n") + ", train-ai=" + yn(train, "y", "n") + ", ai-use=" + yn(use, "y", "n") + "\n")
	b.WriteString("Content-Signal: search=" + yn(search, "yes", "no") + ", ai-train=" + yn(train, "yes", "no") + ", ai-input=" + yn(use, "yes", "no") + "\n")
	for _, path := range robotsDisallowed {
		b.WriteString("Disallow: " + path + "\n")
	}
	b.WriteString("Allow: /\n")
	return b.Bytes()
}

// serveRobots serves GET /robots.txt from the instance settings and the bundled list of AI
// crawlers; operators add no rules of their own (ADR-0057).
func (h *Handler) serveRobots(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.serveText(w, r, "text/plain; charset=utf-8", robotsTxt(s.CrawlerPolicy()))
}

// tdmRep is an entry of /.well-known/tdmrep.json (TDM Reservation Protocol, W3C CG Final Report).
type tdmRep struct {
	Location       string `json:"location"`
	TDMReservation int    `json:"tdm-reservation"`
}

// serveTDMRep serves GET /.well-known/tdmrep.json: the reservation of text and data mining of
// every path while the instance reserves AI, and 404 while it allows AI (it then states nothing).
func (h *Handler) serveTDMRep(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if !s.CrawlerPolicy().DeniesTraining() {
		h.o.NotFound(w, r)
		return
	}
	body, err := json.Marshal([]tdmRep{{Location: "/*", TDMReservation: 1}})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.serveText(w, r, "application/json", append(body, '\n'))
}

// serveSecurityTxt serves GET /.well-known/security.txt (RFC 9116) while the instance has a
// security contact, and 404 otherwise. It expires 30 days after the response, and prefers the
// instance's default locale and English.
func (h *Handler) serveSecurityTxt(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	contact := s.SecurityContact()
	if contact == "" {
		h.o.NotFound(w, r)
		return
	}
	langs := []string{h.instanceLocale(r.Context())}
	if langs[0] != i18n.SourceLocale {
		langs = append(langs, i18n.SourceLocale)
	}
	var b bytes.Buffer
	b.WriteString("Contact: " + contact + "\n")
	b.WriteString("Expires: " + h.now().Add(securityTxtLifetime).UTC().Format(time.RFC3339) + "\n")
	b.WriteString("Preferred-Languages: " + strings.Join(langs, ", ") + "\n")
	b.WriteString("Canonical: https://" + h.o.Host + PathSecurityTxt + "\n")
	h.serveText(w, r, "text/plain; charset=utf-8", b.Bytes())
}

// serveText serves a small generated document with an ETag, cached for an hour.
func (h *Handler) serveText(w http.ResponseWriter, r *http.Request, contentType string, body []byte) {
	hd := w.Header()
	hd.Set("Cache-Control", cacheCrawlerFiles)
	document{body: body, etag: etag(body), contentType: contentType}.serve(w, r)
}

// now returns the current time, from Options.Now when it is set.
func (h *Handler) now() time.Time {
	if h.o.Now != nil {
		return h.o.Now()
	}
	return time.Now()
}
