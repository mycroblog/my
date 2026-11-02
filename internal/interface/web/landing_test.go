// SPDX-License-Identifier: AGPL-3.0-only

package web_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/web"
)

// TestLandingPreview checks that the landing page (/) previews the instance in its default
// locale: its name, its summary, and its preview image, without the meta tags of an account.
func TestLandingPreview(t *testing.T) {
	summary := "  Gardeners & friends\n" + strings.Repeat("あ", 250)
	_, mux := newHandler(t, web.Options{
		Settings:            crawlerSettings("allow", "allow"),
		DefaultLocale:       locale("ja"),
		InstanceName:        named("例のサーバー"),
		InstanceDescription: func(context.Context) string { return summary },
		DefaultImage: func(context.Context) (*web.Image, error) {
			return &web.Image{URL: "https://social.example/media/p/full.png", Width: 1200, Height: 630}, nil
		},
	})
	rec := get(mux, "/", map[string]string{"Accept-Language": "en"})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d", rec.Code)
	}
	body := rec.Body.String()
	got := metas(t, body)
	want := map[string]string{
		"og:type":           "website",
		"og:site_name":      "例のサーバー",
		"og:title":          "例のサーバー",
		"og:url":            "https://social.example/",
		"og:locale":         "ja",
		"og:image":          "https://social.example/media/p/full.png",
		"twitter:card":      "summary_large_image",
		"link:canonical":    "https://social.example/",
		"fediverse:creator": "",
		"link:alternate":    "",
		"robots":            "",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	// An image without alt text gets a generic one naming the instance.
	if !strings.Contains(got["og:image:alt"], "例のサーバー") {
		t.Errorf("og:image:alt = %q", got["og:image:alt"])
	}
	if d := []rune(got["og:description"]); !strings.HasPrefix(got["og:description"], "Gardeners & friends") || len(d) > 200 {
		t.Errorf("og:description = %q (%d characters)", got["og:description"], len(d))
	}
	if !strings.Contains(body, `<html lang="ja" dir="ltr">`) || !strings.Contains(body, "<title>例のサーバー</title>") {
		t.Errorf("GET /:\n%s", body)
	}
	h := rec.Header()
	if h.Get("Cache-Control") != "public, max-age=60, stale-while-revalidate=300" || h.Get("Vary") != "Accept" || h.Get("ETag") == "" {
		t.Errorf("headers = %v", h)
	}

	// A new summary is a new version of the page.
	before := h.Get("ETag")
	summary = "Another summary"
	if again := get(mux, "/", nil); again.Header().Get("ETag") == before || metas(t, again.Body.String())["og:description"] != "Another summary" {
		t.Errorf("the page did not change with the summary")
	}
	// Without a summary, there is no og:description.
	summary = " "
	if got := get(mux, "/", nil).Body.String(); strings.Contains(got, "og:description") {
		t.Errorf("an empty summary is described:\n%s", got)
	}
}

// TestAboutPreview checks that the information page (/about) previews the instance like the
// landing page, under its own address and title, in the instance's default locale.
func TestAboutPreview(t *testing.T) {
	_, mux := newHandler(t, web.Options{
		DefaultLocale:       locale("ja"),
		InstanceName:        named("例のサーバー"),
		InstanceDescription: func(context.Context) string { return "Gardeners & friends" },
	})
	rec := get(mux, "/about", map[string]string{"Accept-Language": "en"})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /about = %d", rec.Code)
	}
	body := rec.Body.String()
	got := metas(t, body)
	want := map[string]string{
		"og:type":           "website",
		"og:site_name":      "例のサーバー",
		"og:title":          "例のサーバー について",
		"og:url":            "https://social.example/about",
		"og:description":    "Gardeners & friends",
		"og:locale":         "ja",
		"link:canonical":    "https://social.example/about",
		"fediverse:creator": "",
		"link:alternate":    "",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if !strings.Contains(body, "<title>例のサーバー について</title>") {
		t.Errorf("GET /about:\n%s", body)
	}
	if rec.Header().Get("ETag") == "" || rec.Header().Get("ETag") == get(mux, "/", nil).Header().Get("ETag") {
		t.Errorf("ETag of /about = %q, want its own", rec.Header().Get("ETag"))
	}
}

// TestCrawlerPagePreview checks that the page about the link card crawler is served to everyone,
// signed out and to crawlers, and previews the instance under its own address and title.
func TestCrawlerPagePreview(t *testing.T) {
	_, mux := newHandler(t, web.Options{DefaultLocale: locale("ja"), InstanceName: named("例のサーバー")})
	rec := get(mux, "/about/crawler", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /about/crawler = %d", rec.Code)
	}
	got := metas(t, rec.Body.String())
	want := map[string]string{
		"og:type":        "website",
		"og:site_name":   "例のサーバー",
		"og:title":       "リンクのカードのクローラー | 例のサーバー",
		"og:url":         "https://social.example/about/crawler",
		"link:canonical": "https://social.example/about/crawler",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if rec.Header().Get("ETag") == "" || rec.Header().Get("ETag") == get(mux, "/about", nil).Header().Get("ETag") {
		t.Errorf("ETag of /about/crawler = %q, want its own", rec.Header().Get("ETag"))
	}
}

// documents is a fake web.Documents: the published documents, or err.
type documents struct {
	published map[instance.TextKey]bool
	err       error
}

func (d *documents) HasDocument(_ context.Context, key instance.TextKey) (bool, error) {
	return d.published[key], d.err
}

// TestDocumentPreview checks that the pages of the published documents preview the instance under
// the document's name, and that the page of a document that is not published, or whose state
// cannot be read, is the generic shell, which follows publishing and unpublishing at once.
func TestDocumentPreview(t *testing.T) {
	docs := &documents{published: map[instance.TextKey]bool{instance.TextTerms: true, instance.TextPrivacyPolicy: true, instance.TextCommerceDisclosure: true}}
	_, mux := newHandler(t, web.Options{
		DefaultLocale:       locale("ja"),
		InstanceName:        named("例のサーバー"),
		InstanceDescription: func(context.Context) string { return "Gardeners & friends" },
		Documents:           docs,
	})
	generic := get(mux, "/home", nil)
	for path, title := range map[string]string{
		web.PathAboutTerms:              "利用規約 | 例のサーバー",
		web.PathAboutPrivacyPolicy:      "プライバシーポリシー | 例のサーバー",
		web.PathAboutCommerceDisclosure: "特定商取引法に基づく表記 | 例のサーバー",
	} {
		rec := get(mux, path, map[string]string{"Accept-Language": "en"})
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, rec.Code)
		}
		body := rec.Body.String()
		got := metas(t, body)
		want := map[string]string{
			"og:type":           "website",
			"og:site_name":      "例のサーバー",
			"og:title":          title,
			"og:url":            "https://social.example" + path,
			"og:description":    "Gardeners & friends",
			"og:locale":         "ja",
			"link:canonical":    "https://social.example" + path,
			"fediverse:creator": "",
			"link:alternate":    "",
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("GET %s: %s = %q, want %q", path, k, got[k], v)
			}
		}
		if !strings.Contains(body, "<title>"+title+"</title>") {
			t.Errorf("GET %s:\n%s", path, body)
		}
		if h := rec.Header(); h.Get("Cache-Control") != "public, max-age=60, stale-while-revalidate=300" || h.Get("ETag") == "" || h.Get("ETag") == generic.Header().Get("ETag") {
			t.Errorf("GET %s: headers = %v", path, h)
		}
	}

	// Unpublishing a document makes its page the generic shell, under a new ETag, at once.
	before := get(mux, web.PathAboutTerms, nil).Header().Get("ETag")
	docs.published[instance.TextTerms] = false
	rec := get(mux, web.PathAboutTerms, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != generic.Body.String() || rec.Header().Get("ETag") == before {
		t.Errorf("GET %s of an unpublished document = %d, ETag %q:\n%s", web.PathAboutTerms, rec.Code, rec.Header().Get("ETag"), rec.Body.String())
	}
	// Publishing it again brings its preview back.
	docs.published[instance.TextTerms] = true
	if rec := get(mux, web.PathAboutTerms, nil); rec.Header().Get("ETag") != before {
		t.Errorf("ETag of a published document again = %q, want %q", rec.Header().Get("ETag"), before)
	}
	// A failure to read the documents serves the generic shell rather than an error.
	docs.err = errors.New("database down")
	if rec := get(mux, web.PathAboutPrivacyPolicy, nil); rec.Code != http.StatusOK || rec.Body.String() != generic.Body.String() {
		t.Errorf("GET %s with a failing read = %d:\n%s", web.PathAboutPrivacyPolicy, rec.Code, rec.Body.String())
	}
}
