// SPDX-License-Identifier: AGPL-3.0-only

package web

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/i18n"
)

// This file serves the Atom feeds (RFC 4287) of profiles (/@alice.atom) and hashtags
// (/tags/{tag}.atom), FR-213: the newest local public posts, without replies and boosts, for feed
// readers of people without a fediverse account; and the articles-only feed of a profile
// (/@alice/articles.atom, ADR-0160 §11). An article's entry is its title, its summary, and the
// link to its page. A feed that does not exist, or that its owner
// turned off, is the server's 404. A rendered feed is kept in the key-value store for
// feedCacheTTL, so that feed readers polling it do not read the database each time.

// Feeds reads the content of the Atom feeds. *appcontent.Feeds implements it. Both fail with an
// error wrapping appcontent.ErrNotFound for a feed that does not exist.
type Feeds interface {
	Profile(ctx context.Context, username string) (appcontent.Feed, error)
	Articles(ctx context.Context, username string) (appcontent.Feed, error)
	Tag(ctx context.Context, name string) (appcontent.Feed, error)
}

const (
	// feedSuffix ends the path of a feed: the path of its HTML page with it.
	feedSuffix = ".atom"
	// articlesSegment ends the path of a profile's articles, before feedSuffix.
	articlesSegment = "/articles"
	// pathTags is the prefix of the hashtags' pages (ADR-0018).
	pathTags = "/tags/"
	// feedContentType is the media type of an Atom feed.
	feedContentType = "application/atom+xml; charset=utf-8"
	// cacheFeed lets feed readers and shared caches keep a feed as long as the key-value store does.
	cacheFeed = "public, max-age=60"
	// feedCacheTTL is how long a rendered feed is kept in the key-value store.
	feedCacheTTL = time.Minute
	// feedCacheNamespace is the key-value store namespace of the rendered feeds, keyed by kind and
	// name.
	feedCacheNamespace = "web_feed"
	// feedTitleLength is the length of the title of a post in grapheme clusters, the ellipsis
	// included.
	feedTitleLength = 80
	// atomNamespace is the XML namespace of Atom 1.0.
	atomNamespace = "http://www.w3.org/2005/Atom"
	// generatorURI names the software in the feeds' generator element.
	generatorURI = "https://github.com/mycroblog/my"
)

// feedKind tells a profile's feed from a hashtag's.
type feedKind string

const (
	feedProfile  feedKind = "profile"
	feedArticles feedKind = "articles"
	feedTag      feedKind = "tag"
)

// feedRoute returns the feed of a path: /@{name}.atom or /@{name}/articles.atom, with or without
// the local domain in the name as parsePreviewRoute takes it, or /tags/{tag}.atom.
func (h *Handler) feedRoute(p string) (kind feedKind, name string, ok bool) {
	rest, ok := strings.CutSuffix(p, feedSuffix)
	if !ok {
		return "", "", false
	}
	if tag, ok := strings.CutPrefix(rest, pathTags); ok {
		if tag == "" || strings.Contains(tag, "/") {
			return "", "", false
		}
		return feedTag, tag, true
	}
	kind = feedProfile
	if profile, ok := strings.CutSuffix(rest, articlesSegment); ok {
		kind, rest = feedArticles, profile
	}
	route, ok := h.parsePreviewRoute(rest)
	if !ok || route.statusID != "" || route.invalid {
		return "", "", false
	}
	return kind, route.name, true
}

// tagPage returns the hashtag of the path of a hashtag's page, /tags/{tag}.
func tagPage(p string) (appcontent.Hashtag, bool) {
	tag, ok := strings.CutPrefix(p, pathTags)
	if !ok || strings.Contains(tag, "/") {
		return appcontent.Hashtag{}, false
	}
	return appcontent.ParseHashtag(tag)
}

// serveTagPage serves the page of a hashtag: the generic shell with the link to the hashtag's feed.
// The link is there whether or not a post has used the hashtag, so that the page needs no query.
func (h *Handler) serveTagPage(w http.ResponseWriter, r *http.Request, tag appcontent.Hashtag) {
	ctx := r.Context()
	locale, name := h.instanceLocale(ctx), h.instanceName(ctx)
	theme, err := h.currentTheme(ctx)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	brand, err := h.currentBrand(ctx)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	settings, err := h.settings(ctx)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	site, err := h.siteMeta(ctx, locale)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	feed := &feedLink{
		Title: h.o.Messages.Localizer(locale).ServerFeedTagTitle(i18n.ServerFeedTagTitleArgs{Tag: tag.Name, Instance: name}),
		URL:   appcontent.TagURL(h.o.Host, tag.NameNormalized) + feedSuffix,
	}
	doc, err := h.renderGeneric(locale, name, theme, brand, settings.CrawlerPolicy().NoIndex(), site, feed)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.shellHeaders(w.Header())
	doc.serve(w, r)
}

// cachedFeed is a rendered feed in the key-value store.
type cachedFeed struct {
	Body     []byte                 `json:"body"`
	ETag     string                 `json:"etag"`
	Modified time.Time              `json:"modified"`
	Policy   instance.CrawlerPolicy `json:"policy"`
}

// serveFeed serves a feed, from the key-value store when it holds it.
func (h *Handler) serveFeed(w http.ResponseWriter, r *http.Request, kind feedKind, name string) {
	ctx := r.Context()
	key := string(kind) + "\x00" + name
	f, ok := h.cachedFeed(ctx, key)
	if !ok {
		var err error
		f, err = h.renderFeed(ctx, kind, name)
		switch {
		case errors.Is(err, appcontent.ErrNotFound):
			h.o.NotFound(w, r)
			return
		case err != nil:
			h.fail(w, r, err)
			return
		}
		h.storeFeed(ctx, key, f)
	}
	hd := w.Header()
	directivesOf(f.Policy).set(hd)
	hd.Set("Content-Type", feedContentType)
	hd.Set("Cache-Control", cacheFeed)
	hd.Set("ETag", f.ETag)
	http.ServeContent(w, r, "", f.Modified, bytes.NewReader(f.Body))
}

// cachedFeed reads a rendered feed from the key-value store; a miss, or a store that fails,
// renders it again.
func (h *Handler) cachedFeed(ctx context.Context, key string) (cachedFeed, bool) {
	if h.o.FeedCache == nil {
		return cachedFeed{}, false
	}
	data, err := h.o.FeedCache.Get(ctx, feedCacheNamespace, key)
	if err != nil {
		if !errors.Is(err, kernel.ErrKeyNotFound) {
			h.o.Logger.WarnContext(ctx, "web: read a cached feed", slog.Any("error", err))
		}
		return cachedFeed{}, false
	}
	var f cachedFeed
	if err := json.Unmarshal(data, &f); err != nil || f.ETag == "" {
		return cachedFeed{}, false
	}
	return f, true
}

// storeFeed keeps a rendered feed in the key-value store for feedCacheTTL.
func (h *Handler) storeFeed(ctx context.Context, key string, f cachedFeed) {
	if h.o.FeedCache == nil {
		return
	}
	data, err := json.Marshal(f)
	if err == nil {
		err = h.o.FeedCache.Set(ctx, feedCacheNamespace, key, data, feedCacheTTL)
	}
	if err != nil {
		h.o.Logger.WarnContext(ctx, "web: cache a feed", slog.Any("error", err))
	}
}

// renderFeed reads and renders a feed. Its Last-Modified is the newest time of what it shows: the
// posts' revisions, the profile's revision, or the hashtag's creation.
func (h *Handler) renderFeed(ctx context.Context, kind feedKind, name string) (cachedFeed, error) {
	if h.o.Feeds == nil {
		return cachedFeed{}, appcontent.ErrNotFound
	}
	settings, err := h.settings(ctx)
	if err != nil {
		return cachedFeed{}, err
	}
	instanceName := h.instanceName(ctx)
	var (
		feed   appcontent.Feed
		doc    atomFeed
		policy instance.CrawlerPolicy
	)
	switch kind {
	case feedProfile, feedArticles:
		read, path := h.o.Feeds.Profile, ""
		if kind == feedArticles {
			read, path = h.o.Feeds.Articles, articlesSegment
		}
		if feed, err = read(ctx, name); err != nil {
			return cachedFeed{}, err
		}
		p := *feed.Author
		lang := h.previewLanguage(ctx, p.Language)
		l := h.o.Messages.Localizer(lang)
		url := profileURL(h.o.Host, p.Username())
		handle := p.Username() + "@" + h.o.AccountDomain
		title := l.ServerPreviewTitle(i18n.ServerPreviewTitleArgs{Handle: handle, Name: displayName(p)})
		if kind == feedArticles {
			title = l.ServerFeedArticlesTitle(i18n.ServerFeedArticlesTitleArgs{Handle: handle, Name: displayName(p)})
		}
		self := url + path + feedSuffix
		doc = atomFeed{
			Lang:  lang,
			ID:    self,
			Title: title,
			Links: []atomLink{
				{Rel: "alternate", Type: "text/html", Href: url},
				{Rel: "self", Type: "application/atom+xml", Href: self},
			},
			Author:  &atomPerson{Name: displayName(p), URI: url},
			Updated: atomTime(p.Actor.Profile.RevisedAt),
		}
		if note := plainText(p.Actor.Profile.NoteHTML); note != "" {
			doc.Subtitle = note
		}
		if p.Avatar != nil {
			doc.Icon = p.Avatar.URL
		}
		policy = settings.CrawlerPolicy().ForOwner(p.Actor.Profile.Indexable, p.Actor.Profile.AIOptOut)
	case feedTag:
		if feed, err = h.o.Feeds.Tag(ctx, name); err != nil {
			return cachedFeed{}, err
		}
		lang := h.instanceLocale(ctx)
		url := appcontent.TagURL(h.o.Host, feed.Tag.NameNormalized)
		doc = atomFeed{
			Lang:  lang,
			ID:    url + feedSuffix,
			Title: h.o.Messages.Localizer(lang).ServerFeedTagTitle(i18n.ServerFeedTagTitleArgs{Tag: feed.Tag.Name, Instance: instanceName}),
			Links: []atomLink{
				{Rel: "alternate", Type: "text/html", Href: url},
				{Rel: "self", Type: "application/atom+xml", Href: url + feedSuffix},
			},
			Updated: atomTime(feed.Tag.CreatedAt),
		}
		// A hashtag's page is the generic shell, which is never indexed; the feed states the same, and
		// AI is denied for all of it when any of its authors denies it.
		policy = settings.CrawlerPolicy().NoIndex()
		for _, e := range feed.Entries {
			if prof := e.Author.Actor.Profile; prof != nil && prof.AIOptOut {
				policy = policy.ForOwner(false, true)
			}
		}
	}
	doc.Xmlns = atomNamespace
	doc.Generator = atomGenerator{URI: generatorURI, Version: h.o.Version, Name: "my"}
	modified := time.Time(doc.Updated)
	for _, e := range feed.Entries {
		entry := h.feedEntry(ctx, e)
		if t := time.Time(entry.Updated); t.After(modified) {
			modified = t
		}
		doc.Entries = append(doc.Entries, entry)
	}
	doc.Updated = atomTime(modified)
	body, err := xml.Marshal(doc)
	if err != nil {
		return cachedFeed{}, fmt.Errorf("web: render the feed: %w", err)
	}
	body = append([]byte(xml.Header), body...)
	body = append(body, '\n')
	return cachedFeed{Body: body, ETag: etag(body), Modified: modified.UTC().Truncate(time.Second), Policy: policy}, nil
}

// feedEntry renders a post of a feed. An article shows its title and its summary, and leaves its
// body to its page. A post behind a content warning shows only the warning, and leaves its body
// and media to its page; the others show the body with their images, unless these are
// sensitive, and every attachment as an enclosure.
func (h *Handler) feedEntry(ctx context.Context, e appcontent.FeedEntry) atomEntry {
	st, rev := e.Post.Status, e.Post.Revision
	lang := h.previewLanguage(ctx, e.Language)
	l := h.o.Messages.Localizer(lang)
	url := content.StatusURL(h.o.Host, e.Author.Username(), st.ID)
	authorURL := profileURL(h.o.Host, e.Author.Username())
	out := atomEntry{
		Lang:      lang,
		ID:        st.URI,
		Links:     []atomLink{{Rel: "alternate", Type: "text/html", Href: url}},
		Published: atomTime(st.PublishedAt),
		Updated:   atomTime(rev.RevisedAt),
		Author:    atomPerson{Name: displayName(e.Author), URI: authorURL},
	}
	for _, tag := range e.Tags {
		out.Categories = append(out.Categories, atomCategory{Term: tag})
	}
	if a := rev.Article; a != nil {
		out.Title = atomText{Type: "text", Text: a.Title}
		if a.Summary != "" {
			out.Summary = &atomText{Type: "text", Text: a.Summary}
		}
		return out
	}
	if rev.SpoilerText != "" {
		out.Title = atomText{Type: "text", Text: feedTitle(rev.SpoilerText)}
		out.Content = &atomText{Type: "text", Text: l.ServerFeedContentWarning(rev.SpoilerText)}
		return out
	}
	body := bodyHTML(rev)
	title := feedTitle(plainText(body))
	if title == "" {
		title = l.ServerFeedUntitled(displayName(e.Author))
	}
	out.Title = atomText{Type: "text", Text: title}
	var b strings.Builder
	b.WriteString(body)
	for _, m := range e.Media {
		out.Links = append(out.Links, atomLink{Rel: "enclosure", Type: mediaType(m.ContentType), Href: m.URL, Title: m.Alt})
		if m.MediaType == content.MediaTypeImage && !m.Sensitive {
			b.WriteString(`<p><img src="` + html.EscapeString(m.URL) + `" alt="` + html.EscapeString(m.Alt) + `"></p>`)
		}
	}
	out.Content = &atomText{Type: "html", Text: b.String()}
	return out
}

// mediaType returns the media type of an enclosure without its parameters.
func mediaType(contentType string) string {
	t, _, _ := strings.Cut(contentType, ";")
	return strings.TrimSpace(t)
}

// feedTitle shortens text to feedTitleLength grapheme clusters for the title of a post.
func feedTitle(text string) string { return truncate(text, feedTitleLength) }

// The elements of an Atom 1.0 document (RFC 4287). encoding/xml escapes every text and attribute.

type atomFeed struct {
	XMLName   xml.Name      `xml:"feed"`
	Xmlns     string        `xml:"xmlns,attr"`
	Lang      string        `xml:"xml:lang,attr,omitempty"`
	ID        string        `xml:"id"`
	Title     string        `xml:"title"`
	Subtitle  string        `xml:"subtitle,omitempty"`
	Links     []atomLink    `xml:"link"`
	Updated   atomTime      `xml:"updated"`
	Author    *atomPerson   `xml:"author,omitempty"`
	Generator atomGenerator `xml:"generator"`
	Icon      string        `xml:"icon,omitempty"`
	Entries   []atomEntry   `xml:"entry"`
}

type atomEntry struct {
	Lang       string         `xml:"xml:lang,attr,omitempty"`
	ID         string         `xml:"id"`
	Title      atomText       `xml:"title"`
	Links      []atomLink     `xml:"link"`
	Published  atomTime       `xml:"published"`
	Updated    atomTime       `xml:"updated"`
	Author     atomPerson     `xml:"author"`
	Categories []atomCategory `xml:"category"`
	Summary    *atomText      `xml:"summary,omitempty"`
	Content    *atomText      `xml:"content,omitempty"`
}

type atomLink struct {
	Rel   string `xml:"rel,attr"`
	Type  string `xml:"type,attr,omitempty"`
	Href  string `xml:"href,attr"`
	Title string `xml:"title,attr,omitempty"`
}

type atomPerson struct {
	Name string `xml:"name"`
	URI  string `xml:"uri,omitempty"`
}

type atomText struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
}

type atomCategory struct {
	Term string `xml:"term,attr"`
}

type atomGenerator struct {
	URI     string `xml:"uri,attr"`
	Version string `xml:"version,attr,omitempty"`
	Name    string `xml:",chardata"`
}

// atomTime is a date construct of Atom: RFC 3339 in UTC, to the second.
type atomTime time.Time

// MarshalText implements encoding.TextMarshaler.
func (t atomTime) MarshalText() ([]byte, error) {
	return []byte(time.Time(t).UTC().Truncate(time.Second).Format(time.RFC3339)), nil
}
