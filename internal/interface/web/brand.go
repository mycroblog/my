// SPDX-License-Identifier: AGPL-3.0-only

package web

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/domain/instance"
)

// The instance's own icon and logos (ADR-0116): the files of each brand revision at
// /brand/<revision>/<rendition>.svg|.png, whose content never changes, and the shell, the Web App
// Manifest, /apple-touch-icon.png, and link previews pointing at the current ones instead of the
// build's icons.

// PathBrand is the prefix of the brand files.
const PathBrand = "/brand/"

// brandContentSecurityPolicy is the policy of a brand SVG served on its own: nothing loads, no
// script runs, and the sandbox keeps the document from doing anything else, even when its URL is
// opened directly. The inline styles of the sanitized SVG still apply.
const brandContentSecurityPolicy = "default-src 'none'; style-src 'unsafe-inline'; sandbox"

// maxBrandFileBytes bounds what serveBrandFile reads of a stored file, above every file it stores.
const maxBrandFileBytes = 2 << 20

// brandState is what the documents need of the current brand images.
type brandState struct {
	// Version identifies the current revisions (instance.Brands.Version).
	Version string
	// Icon holds the paths of the icon's files, nil for the build's icons.
	Icon *brandIcon
	// Logo and LogoDark are the runtime config's logos, nil when unset.
	Logo, LogoDark *runtimeBrandImage
	// IconImage is the runtime config's icon, nil when unset.
	IconImage *runtimeBrandImage
	// Preview is the link preview image drawn from the logo, nil when there is no logo.
	Preview *Image
	// Manifest are the manifest icons of the icon, nil for the build's icons.
	Manifest []manifestIcon
	// appleTouch is the 180 px file, for /apple-touch-icon.png.
	appleTouch *brandFileRef
}

// brandIcon are the paths of the icon's files that the shell links.
type brandIcon struct {
	Favicon32, Favicon48, SVG, AppleTouch string
}

// brandFileRef names a file of a revision.
type brandFileRef struct {
	revisionID string
	rendition  instance.BrandRendition
}

// runtimeBrandImage is a brand image in the runtime config: the path of its SVG and the size of
// its viewBox, rounded.
type runtimeBrandImage struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// currentBrand returns the current brand images, or none without Options.Brands.
func (h *Handler) currentBrand(ctx context.Context) (brandState, error) {
	if h.o.Brands == nil {
		return brandState{Version: instance.Brands{}.Version()}, nil
	}
	b, err := h.o.Brands(ctx)
	if err != nil {
		return brandState{}, err
	}
	return h.brandStateOf(b), nil
}

func (h *Handler) brandStateOf(b instance.Brands) brandState {
	s := brandState{Version: b.Version()}
	path := func(rev *instance.BrandRevision, r instance.BrandRendition) string {
		if _, ok := rev.File(r); !ok {
			return ""
		}
		return instance.BrandFilePath(rev.ID, r)
	}
	image := func(rev *instance.BrandRevision) *runtimeBrandImage {
		if rev == nil {
			return nil
		}
		f, ok := rev.File(instance.RenditionSVG)
		if !ok {
			return nil
		}
		return &runtimeBrandImage{URL: instance.BrandFilePath(rev.ID, instance.RenditionSVG), Width: f.Width, Height: f.Height}
	}
	s.IconImage, s.Logo, s.LogoDark = image(b.Icon), image(b.Logo), image(b.LogoDark)
	if rev := b.Icon; rev != nil {
		icon := &brandIcon{
			Favicon32: path(rev, instance.RenditionFavicon32), Favicon48: path(rev, instance.RenditionFavicon48),
			SVG: path(rev, instance.RenditionSVG), AppleTouch: path(rev, instance.RenditionAppleTouch),
		}
		if icon.SVG != "" {
			s.Icon = icon
			s.Manifest = []manifestIcon{}
			// The PNG first: Misskey takes the first icon of the manifest as the server's icon.
			for _, m := range []struct {
				r       instance.BrandRendition
				purpose string
			}{
				{instance.RenditionPWA512, "any"}, {instance.RenditionPWA192, "any"}, {instance.RenditionMaskable512, "maskable"},
			} {
				if f, ok := rev.File(m.r); ok {
					s.Manifest = append(s.Manifest, manifestIcon{
						Src: instance.BrandFilePath(rev.ID, m.r), Sizes: strconv.Itoa(f.Width) + "x" + strconv.Itoa(f.Height), Type: f.ContentType, Purpose: m.purpose,
					})
				}
			}
			s.Manifest = append(s.Manifest, manifestIcon{Src: icon.SVG, Sizes: "any", Type: instance.BrandSVGContentType, Purpose: "any"})
		}
		if icon.AppleTouch != "" {
			s.appleTouch = &brandFileRef{revisionID: rev.ID, rendition: instance.RenditionAppleTouch}
		}
	}
	if rev := b.Logo; rev != nil {
		if f, ok := rev.File(instance.RenditionPreview); ok {
			s.Preview = &Image{URL: "https://" + h.o.Host + instance.BrandFilePath(rev.ID, f.Rendition), Width: f.Width, Height: f.Height}
		}
	}
	return s
}

// manifestIcons returns the icons of the manifest: the brand icon's, or the build's.
func (h *Handler) manifestIcons(b brandState) []manifestIcon {
	if b.Manifest != nil {
		return b.Manifest
	}
	return h.icons
}

// serveBrandFile serves GET /brand/{revision}/{file}: a file of a brand revision, current or not,
// cached as immutable since its content never changes. An SVG carries a Content-Security-Policy
// that lets nothing run or load, besides the sanitizing that made it.
func (h *Handler) serveBrandFile(w http.ResponseWriter, r *http.Request) {
	name, ext, ok := strings.Cut(r.PathValue("file"), ".")
	rendition, known := instance.ParseBrandRendition(name)
	if !ok || !known || ext != rendition.Extension() || h.o.BrandFile == nil {
		h.o.NotFound(w, r)
		return
	}
	h.serveBrand(w, r, r.PathValue("revision"), rendition, cacheImmutable)
}

// serveBrand serves a brand file with cacheControl.
func (h *Handler) serveBrand(w http.ResponseWriter, r *http.Request, revisionID string, rendition instance.BrandRendition, cacheControl string) {
	f, rc, found, err := h.o.BrandFile(r.Context(), revisionID, rendition)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if !found {
		h.o.NotFound(w, r)
		return
	}
	defer func() { _ = rc.Close() }()
	body, err := io.ReadAll(io.LimitReader(rc, maxBrandFileBytes))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", rendition.ContentType())
	hd.Set("X-Content-Type-Options", "nosniff")
	if rendition == instance.RenditionSVG {
		hd.Set("Content-Security-Policy", brandContentSecurityPolicy)
	}
	hd.Set("Cache-Control", cacheControl)
	tag := `"` + f.SHA256 + `"`
	if f.SHA256 == "" {
		tag = etag(body)
	}
	hd.Set("ETag", tag)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
}

// serveAppleTouchIcon serves /apple-touch-icon.png from the brand icon when it is set, which iOS
// looks for without a link; it reports false when the build's file applies.
func (h *Handler) serveAppleTouchIcon(w http.ResponseWriter, r *http.Request) bool {
	if h.o.Brands == nil || h.o.BrandFile == nil {
		return false
	}
	b, err := h.currentBrand(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return true
	}
	if b.appleTouch == nil {
		return false
	}
	h.serveBrand(w, r, b.appleTouch.revisionID, b.appleTouch.rendition, cacheRevalidate)
	return true
}
