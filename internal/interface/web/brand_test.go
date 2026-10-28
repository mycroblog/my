// SPDX-License-Identifier: AGPL-3.0-only

package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/web"
	"github.com/mycroblog/my/pkg/testkit"
)

// fakeBrands serves the current brand images and the files of every revision.
type fakeBrands struct {
	current instance.Brands
	files   map[string][]byte // revision/rendition -> content
	revs    map[string]instance.BrandRevision
}

func (f *fakeBrands) brands(context.Context) (instance.Brands, error) { return f.current, nil }

func (f *fakeBrands) file(_ context.Context, id string, r instance.BrandRendition) (instance.BrandFile, io.ReadCloser, bool, error) {
	rev, ok := f.revs[id]
	if !ok {
		return instance.BrandFile{}, nil, false, nil
	}
	file, ok := rev.File(r)
	if !ok {
		return instance.BrandFile{}, nil, false, nil
	}
	return file, io.NopCloser(bytes.NewReader(f.files[id+"/"+string(r)])), true, nil
}

// add sets a revision of kind with every file of it.
func (f *fakeBrands) add(kind instance.BrandKind, name string) *instance.BrandRevision {
	id := testkit.NamedID(name)
	rev := instance.BrandRevision{ID: id, Kind: kind, Action: instance.BrandSet}
	sizes := map[instance.BrandRendition][2]int{}
	vw, vh := 100.0, 100.0
	if kind != instance.BrandIcon {
		vw = 400
	}
	for _, r := range append([]instance.BrandRendition{instance.RenditionSVG}, instance.BrandRenditions(kind)...) {
		w, h := instance.BrandRenditionSize(r, vw, vh)
		if r == instance.RenditionSVG {
			w, h = int(vw), int(vh)
		}
		sizes[r] = [2]int{w, h}
		rev.Files = append(rev.Files, instance.BrandFile{Rendition: r, ContentType: r.ContentType(), Width: w, Height: h, SHA256: strings.Repeat("a", 63) + string('0'+rune(len(f.files)%10))})
		f.files[id+"/"+string(r)] = []byte(string(r) + " of " + name)
	}
	f.revs[id] = rev
	f.current.Set(&rev)
	return &rev
}

func newBrands() *fakeBrands {
	return &fakeBrands{files: map[string][]byte{}, revs: map[string]instance.BrandRevision{}}
}

func brandHandler(t *testing.T, f *fakeBrands, o web.Options) http.Handler {
	t.Helper()
	o.Brands, o.BrandFile = f.brands, f.file
	_, mux := newHandler(t, o)
	return mux
}

func TestBrandFiles(t *testing.T) {
	f := newBrands()
	icon := f.add(instance.BrandIcon, "icon")
	mux := brandHandler(t, f, web.Options{})

	rec := get(mux, "/brand/"+icon.ID+"/svg.svg", nil)
	h := rec.Header()
	if rec.Code != http.StatusOK || rec.Body.String() != "svg of icon" {
		t.Fatalf("GET svg = %d %q", rec.Code, rec.Body)
	}
	for k, want := range map[string]string{
		"Content-Type":            "image/svg+xml",
		"Content-Security-Policy": "default-src 'none'; style-src 'unsafe-inline'; sandbox",
		"X-Content-Type-Options":  "nosniff",
		"Cache-Control":           "public, max-age=31536000, immutable",
	} {
		if got := h.Get(k); got != want {
			t.Errorf("svg %s = %q, want %q", k, got, want)
		}
	}
	if etag := h.Get("ETag"); etag == "" {
		t.Error("svg has no ETag")
	} else if rec := get(mux, "/brand/"+icon.ID+"/svg.svg", map[string]string{"If-None-Match": etag}); rec.Code != http.StatusNotModified {
		t.Errorf("conditional GET = %d", rec.Code)
	}

	rec = get(mux, "/brand/"+icon.ID+"/pwa-192.png", nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || rec.Header().Get("X-Content-Type-Options") != "nosniff" ||
		rec.Header().Get("Content-Security-Policy") != "" || rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("GET png = %d %v", rec.Code, rec.Header())
	}
	for _, p := range []string{
		"/brand/" + icon.ID + "/svg.png",                  // wrong extension
		"/brand/" + icon.ID + "/pwa-192.svg",              // wrong extension
		"/brand/" + icon.ID + "/logo-mail.png",            // a rendition the icon has none of
		"/brand/" + icon.ID + "/script.svg",               // no such rendition
		"/brand/" + testkit.NamedID("other") + "/svg.svg", // no such revision
		"/brand/" + icon.ID,
		"/brand/",
	} {
		if rec := get(mux, p, nil); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, rec.Code)
		}
	}
}

func TestShellWithBrandIcon(t *testing.T) {
	f := newBrands()
	mux := brandHandler(t, f, web.Options{})
	before := get(mux, "/", nil)
	if body := before.Body.String(); !strings.Contains(body, `<link rel="icon" href="/icon.svg" type="image/svg+xml">`) ||
		!strings.Contains(body, `<link rel="apple-touch-icon" href="/apple-touch-icon.png">`) || strings.Contains(body, web.PathBrand) {
		t.Errorf("shell without a brand icon:\n%s", body)
	}
	if rec := get(mux, "/apple-touch-icon.png", nil); rec.Body.String() != "png apple" {
		t.Errorf("/apple-touch-icon.png without a brand icon = %q", rec.Body)
	}

	icon := f.add(instance.BrandIcon, "icon")
	logo := f.add(instance.BrandLogo, "logo")
	rec := get(mux, "/", nil)
	body := rec.Body.String()
	base := "/brand/" + icon.ID + "/"
	for _, want := range []string{
		`<link rel="icon" href="` + base + `favicon-32.png" type="image/png" sizes="32x32">`,
		`<link rel="icon" href="` + base + `favicon-48.png" type="image/png" sizes="48x48">`,
		`<link rel="icon" href="` + base + `svg.svg" type="image/svg+xml">`,
		`<link rel="apple-touch-icon" href="` + base + `apple-touch-180.png">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("shell lacks %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, `href="/icon.svg"`) || strings.Contains(body, `href="/apple-touch-icon.png"`) {
		t.Errorf("shell with a brand icon links the build's icons:\n%s", body)
	}
	if rec.Header().Get("ETag") == before.Header().Get("ETag") {
		t.Error("the shell's ETag did not change with the brand icon")
	}
	cfg := shellConfig(t, body)
	if cfg["icon"] == nil || cfg["logo"] == nil || cfg["logoDark"] != nil {
		t.Errorf("runtime config = %v", cfg)
	}
	if l := cfg["logo"].(map[string]any); l["url"] != "/brand/"+logo.ID+"/svg.svg" || l["width"] != 400.0 || l["height"] != 100.0 {
		t.Errorf("runtime config logo = %v", l)
	}

	// iOS asks for /apple-touch-icon.png without a link.
	rec = get(mux, "/apple-touch-icon.png", nil)
	if rec.Body.String() != "apple-touch-180 of icon" || rec.Header().Get("Cache-Control") != "no-cache" || rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("/apple-touch-icon.png = %q %v", rec.Body, rec.Header())
	}

	var m struct {
		Icons []struct{ Src, Sizes, Type, Purpose string } `json:"icons"`
	}
	if err := json.Unmarshal(get(mux, web.PathWebManifest, nil).Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	want := []string{
		base + "pwa-512.png 512x512 image/png any", base + "pwa-192.png 192x192 image/png any",
		base + "maskable-512.png 512x512 image/png maskable", base + "svg.svg any image/svg+xml any",
	}
	if len(m.Icons) != len(want) {
		t.Fatalf("manifest icons = %+v", m.Icons)
	}
	for i, ic := range m.Icons {
		if got := ic.Src + " " + ic.Sizes + " " + ic.Type + " " + ic.Purpose; got != want[i] {
			t.Errorf("manifest icon %d = %s, want %s", i, got, want[i])
		}
	}

	// Cleared: back to the build's icons.
	f.current.Icon = nil
	if body := get(mux, "/", nil).Body.String(); !strings.Contains(body, `href="/icon.svg"`) || strings.Contains(body, base) {
		t.Errorf("shell after clearing the icon:\n%s", body)
	}
	if rec := get(mux, "/apple-touch-icon.png", nil); rec.Body.String() != "png apple" {
		t.Errorf("/apple-touch-icon.png after clearing = %q", rec.Body)
	}
}

func TestLinkPreviewWithBrandLogo(t *testing.T) {
	f := newBrands()
	var preview *web.Image
	mux := brandHandler(t, f, web.Options{
		InstanceName: named("Example Social"),
		DefaultImage: func(context.Context) (*web.Image, error) { return preview, nil },
	})
	if got := ogImage(t, mux); got != "https://social.example"+web.PathDefaultImage {
		t.Errorf("og:image without a logo = %s", got)
	}
	logo := f.add(instance.BrandLogo, "logo")
	if got := ogImage(t, mux); got != "https://social.example/brand/"+logo.ID+"/preview-1200x630.png" {
		t.Errorf("og:image with a logo = %s", got)
	}
	preview = &web.Image{URL: "https://social.example/media/p.png", Width: 1200, Height: 630}
	if got := ogImage(t, mux); got != preview.URL {
		t.Errorf("og:image with a preview image = %s, want the preview image first", got)
	}
}

func ogImage(t *testing.T, mux http.Handler) string {
	t.Helper()
	body := get(mux, "/", nil).Body.String()
	const prefix = `<meta property="og:image" content="`
	i := strings.Index(body, prefix)
	if i < 0 {
		t.Fatalf("no og:image:\n%s", body)
	}
	rest := body[i+len(prefix):]
	return rest[:strings.IndexByte(rest, '"')]
}

func shellConfig(t *testing.T, body string) map[string]any {
	t.Helper()
	const open = `<script type="application/json" id="config">`
	i := strings.Index(body, open)
	if i < 0 {
		t.Fatalf("no config:\n%s", body)
	}
	rest := body[i+len(open):]
	var cfg map[string]any
	end := strings.Index(rest, "</script>")
	if end < 0 {
		t.Fatalf("no end of the config:\n%s", body)
	}
	if err := json.Unmarshal([]byte(rest[:end]), &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}
