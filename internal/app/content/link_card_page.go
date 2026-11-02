// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/rivo/uniseg"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/net/html/charset"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/content"
)

// This file reads a link card from the head of an HTML page (ADR-0062 §2): the Open Graph and
// X Card meta tags, the title, and the JSON-LD of the page, read statically. Nothing of the page
// runs, and nothing past the head is read.

// ErrNoLinkCard reports a page that makes no card: one without a title.
var ErrNoLinkCard = errors.New("content: the page has no title for a link card")

// maxJSONLD is the largest <script type="application/ld+json"> that is read, in bytes.
const maxJSONLD = 64 << 10

// LinkCardPage is what the head of a page declares for its link card.
type LinkCardPage struct {
	// URL is where the card links: the final URL, or the og:url of the same origin.
	URL         string
	Title       string
	Description string
	SiteName    string
	AuthorName  string
	PublishedAt *time.Time
	// ImageURL is the absolute http or https URL of the image, "" for none; ImageAlt its
	// alternative text.
	ImageURL string
	ImageAlt string
	// Creator is the fediverse:creator handle as declared ("@user@domain"), "" for none. It is
	// not verified.
	Creator string
	// Language is the <html lang> of the page, "" for none.
	Language string
}

// metaRobotsNames are the meta tags whose content holds robots directives for a link card: the
// tag for every robot, and the tag of the crawler itself (ADR-0131 §5).
var metaRobotsNames = []string{"robots", kernel.LinkCardAgentToken}

// ApplyRobots removes from the card what directives forbid, from its head or the X-Robots-Tag of
// its response: the description, or the image
// with its alternative text. The stricter of several directives wins, because any one of them
// removes the part and none adds it back. The other parts stay whatever the directives say.
func (p *LinkCardPage) ApplyRobots(values []string) {
	noSnippet, noImage := parseRobotsDirectives(values)
	if noSnippet {
		p.Description = ""
	}
	if noImage {
		p.ImageURL, p.ImageAlt = "", ""
	}
}

// parseRobotsDirectives reads robots directives for this crawler from the content of meta tags
// or the values of X-Robots-Tag. Each value is a comma-separated list; "bot: directive" makes the
// directive and those after it in the value apply to that bot, so only the directives with no
// bot name or with this crawler's are read. noindex, nofollow, noarchive, none, and every other
// directive change nothing in a card.
func parseRobotsDirectives(values []string) (noSnippet, noImage bool) {
	for _, value := range values {
		applies := true
		for directive := range strings.SplitSeq(value, ",") {
			directive = strings.ToLower(strings.TrimSpace(directive))
			name, rest, named := strings.Cut(directive, ":")
			name, rest = strings.TrimSpace(name), strings.TrimSpace(rest)
			if named && !slices.Contains(robotsValuedDirectives, name) && isRobotsBotName(name) {
				applies = name == kernel.LinkCardAgentToken
				directive = rest
				name, rest, named = strings.Cut(directive, ":")
				name, rest = strings.TrimSpace(name), strings.TrimSpace(rest)
			}
			if !applies {
				continue
			}
			switch {
			case !named && name == "nosnippet", named && name == "max-snippet" && rest == "0":
				noSnippet = true
			case !named && name == "noimageindex", named && name == "max-image-preview" && rest == "none":
				noImage = true
			}
		}
	}
	return noSnippet, noImage
}

// robotsValuedDirectives are the directives written "name: value", which must not be read as a
// bot's name.
var robotsValuedDirectives = []string{"max-snippet", "max-image-preview", "max-video-preview", "unavailable_after"}

// isRobotsBotName reports whether the text before a colon can name a bot, unlike the parts of a
// date in the value of unavailable_after.
func isRobotsBotName(s string) bool {
	return s != "" && strings.IndexFunc(s, func(r rune) bool {
		return r != '-' && r != '_' && (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) < 0
}

// ParseLinkCardPage reads the card of the page that body holds, served with contentType from
// finalURL (after redirects). The body is decoded to UTF-8 from the charset of contentType, of
// <meta charset>, or a guess, in that order. It fails with ErrNoLinkCard when the page has no
// title.
func ParseLinkCardPage(body io.Reader, contentType, finalURL string) (LinkCardPage, error) {
	base, err := url.Parse(finalURL)
	if err != nil || base.Host == "" {
		return LinkCardPage{}, fmt.Errorf("content: link card page URL %q: %w", finalURL, ErrNoLinkCard)
	}
	r, err := charset.NewReader(body, contentType)
	if err != nil {
		return LinkCardPage{}, fmt.Errorf("content: link card page charset: %w", err)
	}
	h, err := readHead(r)
	if err != nil {
		return LinkCardPage{}, err
	}
	ld := h.jsonLD()
	p := LinkCardPage{
		URL:         base.String(),
		Title:       clip(firstOf(h.meta("og:title"), h.meta("twitter:title"), ld.headline, h.title), content.MaxLinkCardTitle),
		Description: clip(firstOf(h.meta("og:description"), h.meta("twitter:description"), ld.description, h.meta("description")), content.MaxLinkCardDescription),
		SiteName:    clip(firstOf(h.meta("og:site_name"), ld.publisher), content.MaxLinkCardSiteName),
		AuthorName:  clip(firstOf(h.metaAuthor(), h.meta("author"), ld.author), content.MaxLinkCardAuthorName),
		ImageAlt:    clip(firstOf(h.meta("og:image:alt"), h.meta("twitter:image:alt")), content.MaxLinkCardImageAlt),
		Creator:     strings.TrimSpace(h.meta("fediverse:creator")),
		Language:    strings.TrimSpace(h.lang),
	}
	if p.Title == "" {
		return LinkCardPage{}, fmt.Errorf("content: link card of %s: %w", finalURL, ErrNoLinkCard)
	}
	if p.SiteName == "" {
		p.SiteName = clip(base.Hostname(), content.MaxLinkCardSiteName)
	}
	for _, v := range []string{h.meta("article:published_time"), ld.published} {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(v)); err == nil {
			t = t.UTC().Truncate(time.Millisecond)
			p.PublishedAt = &t
			break
		}
	}
	if og := resolveLink(base, h.meta("og:url")); og != nil && sameOrigin(og, base) {
		p.URL = og.String()
	}
	for _, v := range []string{h.meta("og:image:secure_url"), h.meta("og:image"), h.meta("og:image:url"), h.meta("twitter:image"), h.meta("twitter:image:src")} {
		if u := resolveLink(base, v); u != nil && len(u.String()) <= content.MaxLinkCardImageURLSize {
			p.ImageURL = u.String()
			break
		}
	}
	p.ApplyRobots(h.robots)
	return p, nil
}

// pageHead is what readHead collects from the head of a page.
type pageHead struct {
	lang  string
	title string
	// metas are the content of the first meta tag of each property or name, lowercase.
	metas  map[string]string
	ldJSON []string
	// robots are the content of every meta tag of metaRobotsNames.
	robots []string
}

func (h *pageHead) meta(key string) string { return h.metas[key] }

// metaAuthor returns article:author unless it is a URL (Open Graph allows a profile URL there).
func (h *pageHead) metaAuthor() string {
	v := h.meta("article:author")
	if u, err := url.Parse(strings.TrimSpace(v)); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		return ""
	}
	return v
}

// readHead tokenizes the page up to the end of its head: the first </head> or <body>.
func readHead(r io.Reader) (*pageHead, error) {
	h := &pageHead{metas: map[string]string{}}
	z := html.NewTokenizer(r)
	var inTitle, inLD bool
	var title strings.Builder
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if errors.Is(z.Err(), io.EOF) {
				h.title = title.String()
				return h, nil
			}
			return nil, fmt.Errorf("content: read link card page: %w", z.Err())
		case html.TextToken:
			switch {
			case inTitle:
				title.Write(z.Text())
			case inLD:
				if t := z.Text(); len(t) <= maxJSONLD {
					h.ldJSON = append(h.ldJSON, string(t))
				}
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch atom.Lookup(name) {
			case atom.Head:
				h.title = title.String()
				return h, nil
			case atom.Title:
				inTitle = false
			case atom.Script:
				inLD = false
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			attrs := map[string]string{}
			for hasAttr {
				var k, v []byte
				k, v, hasAttr = z.TagAttr()
				if _, ok := attrs[string(k)]; !ok {
					attrs[string(k)] = string(v)
				}
			}
			switch atom.Lookup(name) {
			case atom.Body:
				h.title = title.String()
				return h, nil
			case atom.Html:
				h.lang = attrs["lang"]
			case atom.Title:
				inTitle = tt == html.StartTagToken && title.Len() == 0
			case atom.Script:
				inLD = tt == html.StartTagToken && strings.EqualFold(strings.TrimSpace(attrs["type"]), "application/ld+json")
			case atom.Meta:
				if slices.Contains(metaRobotsNames, strings.ToLower(strings.TrimSpace(attrs["name"]))) {
					h.robots = append(h.robots, attrs["content"])
				}
				key := attrs["property"]
				if key == "" {
					key = attrs["name"]
				}
				key = strings.ToLower(strings.TrimSpace(key))
				if _, seen := h.metas[key]; key != "" && !seen {
					if v, ok := attrs["content"]; ok && strings.TrimSpace(v) != "" {
						h.metas[key] = v
					}
				}
			}
		}
	}
}

// linkedData is what a link card takes from the JSON-LD of a page.
type linkedData struct {
	headline, description, author, published, publisher string
}

// jsonLDTypes are the schema.org types whose objects a link card reads.
var jsonLDTypes = []string{"NewsArticle", "Article", "BlogPosting", "WebPage"}

// jsonLD reads the first object of a jsonLDTypes type at the top of a script or in its @graph.
// Contexts are not fetched or expanded: the terms are read as schema.org compacts them.
func (h *pageHead) jsonLD() linkedData {
	for _, raw := range h.ldJSON {
		var doc any
		if json.Unmarshal([]byte(raw), &doc) != nil {
			continue
		}
		var candidates []any
		switch d := doc.(type) {
		case []any:
			candidates = d
		case map[string]any:
			candidates = []any{d}
			if g, ok := d["@graph"].([]any); ok {
				candidates = append(candidates, g...)
			}
		}
		for _, c := range candidates {
			o, ok := c.(map[string]any)
			if !ok || !hasJSONLDType(o["@type"]) {
				continue
			}
			return linkedData{
				headline: jsonString(o["headline"]), description: jsonString(o["description"]),
				author: jsonNames(o["author"]), published: jsonString(o["datePublished"]),
				publisher: jsonNames(o["publisher"]),
			}
		}
	}
	return linkedData{}
}

func hasJSONLDType(v any) bool {
	switch t := v.(type) {
	case string:
		return slices.Contains(jsonLDTypes, t)
	case []any:
		for _, x := range t {
			if s, ok := x.(string); ok && slices.Contains(jsonLDTypes, s) {
				return true
			}
		}
	}
	return false
}

func jsonString(v any) string {
	s, _ := v.(string)
	return s
}

// jsonNames returns the name of an object, or the names of an array of objects joined with ", ".
func jsonNames(v any) string {
	var names []string
	add := func(x any) {
		if o, ok := x.(map[string]any); ok {
			if n := strings.TrimSpace(jsonString(o["name"])); n != "" {
				names = append(names, n)
			}
		}
	}
	if a, ok := v.([]any); ok {
		for _, x := range a {
			add(x)
		}
	} else {
		add(v)
	}
	return strings.Join(names, ", ")
}

// firstOf returns the first value that has text once made plain.
func firstOf(values ...string) string {
	for _, v := range values {
		if p := plainText(v); p != "" {
			return p
		}
	}
	return ""
}

// plainText turns a value of a page into plain text: character references decoded, tags
// removed, and white space collapsed to single spaces.
func plainText(s string) string {
	if strings.ContainsAny(s, "<&") {
		var b strings.Builder
		z := html.NewTokenizer(strings.NewReader(s))
		for {
			tt := z.Next()
			if tt == html.ErrorToken {
				break
			}
			if tt == html.TextToken {
				b.Write(z.Text())
				b.WriteByte(' ')
			}
		}
		s = b.String()
	}
	s = strings.Map(func(r rune) rune {
		if r == 0 || unicode.IsControl(r) && !unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// clip cuts s to at most n grapheme clusters, so that no emoji or combining sequence is split.
func clip(s string, n int) string {
	if uniseg.GraphemeClusterCount(s) <= n {
		return s
	}
	var b strings.Builder
	g := uniseg.NewGraphemes(s)
	for i := 0; i < n && g.Next(); i++ {
		b.WriteString(g.Str())
	}
	return strings.TrimRightFunc(b.String(), unicode.IsSpace)
}

// resolveLink resolves ref against base and returns it when it is an http or https URL with a
// host, nil otherwise.
func resolveLink(base *url.URL, ref string) *url.URL {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	u, err := base.Parse(ref)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil
	}
	u.User = nil
	return u
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}
