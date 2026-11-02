// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/content"
	domain "github.com/mycroblog/my/internal/domain/content"
)

func TestParseLinkCardPage(t *testing.T) {
	t.Parallel()
	published := time.Date(2026, 10, 1, 9, 30, 0, 0, time.UTC)
	cases := []struct {
		name, html, contentType string
		want                    content.LinkCardPage
	}{
		{
			name: "Open Graph first",
			html: `<html lang="ja"><head><title>T</title>
<meta name="description" content="meta description">
<meta name="twitter:title" content="X title">
<meta property="og:title" content=" OG   &lt;b&gt;title&lt;/b&gt; ">
<meta property="og:description" content="OG description">
<meta property="og:site_name" content="Site">
<meta property="og:url" content="/canonical">
<meta property="og:image" content="/img.png">
<meta property="og:image:secure_url" content="https://cdn.example/img.png">
<meta property="og:image:alt" content="Alt">
<meta property="article:author" content="https://example.com/profile">
<meta name="author" content="Ann">
<meta property="article:published_time" content="2026-10-01T18:30:00+09:00">
<meta name="fediverse:creator" content="@ann@social.example">
<meta name="fediverse:creator" content="@other@social.example">
</head><body><meta property="og:title" content="body"></body></html>`,
			want: content.LinkCardPage{
				URL: "https://example.com/canonical", Title: "OG title", Description: "OG description", SiteName: "Site",
				AuthorName: "Ann", PublishedAt: &published, ImageURL: "https://cdn.example/img.png", ImageAlt: "Alt",
				Creator: "@ann@social.example", Language: "ja",
			},
		},
		{
			name: "X Card and the title",
			html: `<head><title>Plain &amp; title</title><meta name="twitter:description" content="X description">
<meta name="twitter:image" content="pic.jpg"><meta property="og:url" content="https://elsewhere.example/">`,
			want: content.LinkCardPage{
				URL: "https://example.com/a/page", Title: "Plain & title", Description: "X description",
				SiteName: "example.com", ImageURL: "https://example.com/a/pic.jpg",
			},
		},
		{
			name: "JSON-LD after the meta tags",
			html: `<head><title>Title tag</title><meta name="description" content="meta">
<script type="application/ld+json">{"@context":"https://schema.org","@graph":[{"@type":"WebSite","name":"x"},
{"@type":["NewsArticle"],"headline":"Headline","description":"LD description","datePublished":"2026-10-01T09:30:00Z",
"author":[{"@type":"Person","name":"A"},{"@type":"Person","name":"B"}],"publisher":{"name":"Publisher"}}]}</script>
<script>{"headline":"not JSON-LD"}</script></head>`,
			want: content.LinkCardPage{
				URL: "https://example.com/a/page", Title: "Headline", Description: "LD description", SiteName: "Publisher",
				AuthorName: "A, B", PublishedAt: &published,
			},
		},
		{
			name:        "charset of the meta tag",
			html:        "<head><meta charset=\"shift_jis\"><title>\x93\xfa\x96\x7b</title></head>",
			contentType: "text/html",
			want:        content.LinkCardPage{URL: "https://example.com/a/page", Title: "日本", SiteName: "example.com"},
		},
		{
			name: "images that are not http",
			html: `<head><title>T</title><meta property="og:image" content="javascript:alert(1)"><meta property="og:image:url" content="data:image/png;base64,AA"></head>`,
			want: content.LinkCardPage{URL: "https://example.com/a/page", Title: "T", SiteName: "example.com"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ct := tc.contentType
			if ct == "" {
				ct = "text/html; charset=utf-8"
			}
			got, err := content.ParseLinkCardPage(strings.NewReader(tc.html), ct, "https://example.com/a/page")
			if err != nil {
				t.Fatal(err)
			}
			if (got.PublishedAt == nil) != (tc.want.PublishedAt == nil) || got.PublishedAt != nil && !got.PublishedAt.Equal(*tc.want.PublishedAt) {
				t.Errorf("PublishedAt = %v, want %v", got.PublishedAt, tc.want.PublishedAt)
			}
			got.PublishedAt, tc.want.PublishedAt = nil, nil
			if got != tc.want {
				t.Errorf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestParseLinkCardPageLimits(t *testing.T) {
	t.Parallel()
	if _, err := content.ParseLinkCardPage(strings.NewReader(`<head><meta property="og:description" content="d"></head>`), "text/html", "https://example.com/"); !errors.Is(err, content.ErrNoLinkCard) {
		t.Errorf("a page without a title: %v, want ErrNoLinkCard", err)
	}
	// The title is cut at 200 grapheme clusters, never inside one.
	family := "👨‍👩‍👧"
	page := `<head><meta property="og:title" content="` + strings.Repeat(family, 250) + `"><meta property="og:image" content="https://example.com/` + strings.Repeat("a", 2100) + `"></head>`
	got, err := content.ParseLinkCardPage(strings.NewReader(page), "text/html", "https://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != strings.Repeat(family, domain.MaxLinkCardTitle) || got.ImageURL != "" {
		t.Errorf("title of %d bytes, image %q", len(got.Title), got.ImageURL)
	}
	// Only the head is read: a title in the body makes no card.
	if _, err := content.ParseLinkCardPage(strings.NewReader(`<body><title>late</title></body>`), "text/html", "https://example.com/"); !errors.Is(err, content.ErrNoLinkCard) {
		t.Errorf("a title in the body: %v, want ErrNoLinkCard", err)
	}
}

func TestLinkCardURL(t *testing.T) {
	t.Parallel()
	local := []string{"example.test"}
	cases := []struct {
		name string
		rev  domain.Revision
		want string
	}{
		{
			name: "local text",
			rev:  domain.Revision{Source: &domain.Source{Text: "#tag @bob https://example.test/@bob see HTTPS://News.Example:443/a?b=1#frag and https://other.example"}},
			want: "https://news.example/a?b=1",
		},
		{
			name: "remote HTML",
			rev: domain.Revision{Bodies: []domain.Body{{ContentHTML: `<p><span class="h-card"><a href="https://remote.example/@bob" class="u-url mention">@<span>bob</span></a></span>
<a href="https://remote.example/tags/go" class="mention hashtag" rel="tag">#<span>go</span></a>
<a href="https://remote.example/@carol">@carol</a> <a href="https://example.test/@alice/1">mine</a>
<a href="https://blog.example/post" rel="nofollow">blog.example/post</a></p>`}}},
			want: "https://blog.example/post",
		},
		{
			name: "attachments",
			rev:  domain.Revision{Source: &domain.Source{Text: "https://news.example/a"}, Attachments: []domain.Attachment{{MediaID: "01890a5d-ac96-7000-8000-000000000001"}}},
		},
		{
			name: "poll",
			rev:  domain.Revision{Source: &domain.Source{Text: "https://news.example/a"}, Poll: &domain.Poll{}},
		},
		{
			name: "no link",
			rev:  domain.Revision{Bodies: []domain.Body{{ContentHTML: `<p>ftp://x.example <a href="mailto:a@example.com">mail</a></p>`}}},
		},
	}
	for _, tc := range cases {
		if got := content.LinkCardURL(tc.rev, local); got != tc.want {
			t.Errorf("%s: LinkCardURL = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestLinkCardPageDirectives checks the robots meta tags of a page (ADR-0131 §5): which tags are
// read, what each directive takes away, that the stricter wins, and what always stays.
func TestLinkCardPageDirectives(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, metas            string
		noDescription, noImage bool
	}{
		{"no directive", ``, false, false},
		{"nosnippet", `<meta name="robots" content="nosnippet">`, true, false},
		{"max-snippet:0", `<meta name="robots" content="max-snippet:0">`, true, false},
		{"max-snippet with a space", `<meta name="robots" content="max-snippet: 0">`, true, false},
		{"a positive max-snippet", `<meta name="robots" content="max-snippet:50">`, false, false},
		{"max-snippet:-1", `<meta name="robots" content="max-snippet:-1">`, false, false},
		{"noimageindex", `<meta name="robots" content="noimageindex">`, false, true},
		{"max-image-preview:none", `<meta name="robots" content="max-image-preview:none">`, false, true},
		{"max-image-preview:large", `<meta name="robots" content="max-image-preview:large">`, false, false},
		{"noindex", `<meta name="robots" content="noindex">`, false, false},
		{"nofollow", `<meta name="robots" content="nofollow">`, false, false},
		{"noarchive", `<meta name="robots" content="noarchive">`, false, false},
		{"none", `<meta name="robots" content="none">`, false, false},
		{"unavailable_after", `<meta name="robots" content="unavailable_after: 25 Jun 2010 15:00:00 PST, nosnippet">`, true, false},
		{"a list", `<meta name="robots" content="noindex, nosnippet, noimageindex">`, true, true},
		{"any case", `<META NAME="ROBOTS" CONTENT="NoSnippet">`, true, false},
		{"the crawler's tag", `<meta name="my-linkcard" content="nosnippet">`, true, false},
		{"the crawler's tag in another case", `<meta name="MY-LINKCARD" content="noimageindex">`, false, true},
		{"another bot's tag", `<meta name="googlebot" content="nosnippet, noimageindex">`, false, false},
		{"another bot by name", `<meta name="robots" content="googlebot: nosnippet">`, false, false},
		{"this crawler by name", `<meta name="robots" content="my-linkcard: nosnippet">`, true, false},
		{"a bot's name covers the list after it", `<meta name="robots" content="googlebot: nosnippet, noimageindex">`, false, false},
		{"two tags add up", `<meta name="robots" content="nosnippet"><meta name="my-linkcard" content="noimageindex">`, true, true},
		{"the stricter wins in any order", `<meta name="robots" content="max-snippet:-1"><meta name="robots" content="max-snippet:0">`, true, false},
		{"a property is not a tag name", `<meta property="robots" content="nosnippet">`, false, false},
		{"a tag in the body is not read", `</head><body><meta name="robots" content="nosnippet">`, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			page := `<html><head><title>T</title><meta name="description" content="meta">
<meta property="og:description" content="og"><meta name="twitter:description" content="tw">
<meta property="og:site_name" content="Site"><meta name="author" content="Ann">
<meta property="og:image" content="/a.png"><meta property="og:image:alt" content="alt">` + tc.metas + `</head></html>`
			got, err := content.ParseLinkCardPage(strings.NewReader(page), "text/html", "https://example.com/a")
			if err != nil {
				t.Fatal(err)
			}
			if got.Title != "T" || got.SiteName != "Site" || got.AuthorName != "Ann" {
				t.Errorf("title, site name, or author changed: %+v", got)
			}
			if (got.Description == "") != tc.noDescription {
				t.Errorf("description = %q, want removed = %v", got.Description, tc.noDescription)
			}
			if (got.ImageURL == "") != tc.noImage || (got.ImageAlt == "") != tc.noImage {
				t.Errorf("image = %q, alt = %q, want removed = %v", got.ImageURL, got.ImageAlt, tc.noImage)
			}
		})
	}
}

// TestLinkCardPageNoSnippetTakesEverySource checks that a description comes from no source once
// it is refused, JSON-LD included.
func TestLinkCardPageNoSnippetTakesEverySource(t *testing.T) {
	t.Parallel()
	page := `<head><title>T</title><meta name="robots" content="nosnippet"><meta name="description" content="meta">
<script type="application/ld+json">{"@type":"Article","description":"ld"}</script></head>`
	got, err := content.ParseLinkCardPage(strings.NewReader(page), "text/html", "https://example.com/a")
	if err != nil || got.Description != "" {
		t.Errorf("description = %q, %v", got.Description, err)
	}
}

// TestLinkCardPageRobotsHeader checks the values of X-Robots-Tag: with no bot's name or this
// crawler's, value by value.
func TestLinkCardPageRobotsHeader(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                   string
		values                 []string
		noDescription, noImage bool
	}{
		{"none", nil, false, false},
		{"for every bot", []string{"nosnippet"}, true, false},
		{"this crawler", []string{"my-linkcard: nosnippet, max-image-preview:none"}, true, true},
		{"another bot", []string{"otherbot: nosnippet, noimageindex"}, false, false},
		{"values stand alone", []string{"otherbot: nosnippet", "noimageindex"}, false, true},
		{"the scope changes in a value", []string{"otherbot: nosnippet, my-linkcard: noimageindex"}, false, true},
		{"this crawler, then another", []string{"my-linkcard: nosnippet, otherbot: noimageindex"}, true, false},
		{"none of the card's directives", []string{"noindex, nofollow, noarchive, none"}, false, false},
	}
	for _, tc := range cases {
		p := content.LinkCardPage{Description: "d", ImageURL: "https://example.com/a.png", ImageAlt: "a", Title: "T"}
		p.ApplyRobots(tc.values)
		if p.Title != "T" || (p.Description == "") != tc.noDescription || (p.ImageURL == "") != tc.noImage || (p.ImageAlt == "") != tc.noImage {
			t.Errorf("%s: %+v", tc.name, p)
		}
	}
}
