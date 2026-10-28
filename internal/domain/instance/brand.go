// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"bytes"
	"errors"
	"fmt"
	"image/png"
	"math"
	"time"
)

// The instance's own icon and logo (ADR-0116): an SVG that the administrator uploads for each
// kind, sanitized by SanitizeBrandSVG, with PNG renditions of it that the admin screen draws in
// the browser for the places that take no SVG. Each change is a revision; the current one of each
// kind is in the instance_brands projection, and an unset kind uses the built-in images.

// BrandKind is what a brand image stands for.
type BrandKind string

// Brand kinds.
const (
	// BrandIcon is the square mark: the favicon, the app icons, and the instance's avatar.
	BrandIcon BrandKind = "icon"
	// BrandLogo is the wide wordmark, which includes the instance's name.
	BrandLogo BrandKind = "logo"
	// BrandLogoDark is the wordmark for dark backgrounds; without it, BrandLogo is used there too.
	BrandLogoDark BrandKind = "logo_dark"
)

// BrandKinds are the brand kinds, in the order that lists show them.
var BrandKinds = []BrandKind{BrandIcon, BrandLogo, BrandLogoDark}

// ParseBrandKind returns the kind named s, and false for an unknown one.
func ParseBrandKind(s string) (BrandKind, bool) {
	for _, k := range BrandKinds {
		if string(k) == s {
			return k, true
		}
	}
	return "", false
}

// BrandAction is what a brand revision does.
type BrandAction string

// Brand actions.
const (
	// BrandSet makes the revision's files the current images of its kind.
	BrandSet BrandAction = "set"
	// BrandClear brings its kind back to the built-in images.
	BrandClear BrandAction = "clear"
)

// BrandRendition names one file of a brand revision: the sanitized SVG or one of its PNG renditions.
type BrandRendition string

// Brand renditions (ADR-0116 §3).
const (
	RenditionSVG          BrandRendition = "svg"
	RenditionFavicon32    BrandRendition = "favicon-32"
	RenditionFavicon48    BrandRendition = "favicon-48"
	RenditionAppleTouch   BrandRendition = "apple-touch-180"
	RenditionPWA192       BrandRendition = "pwa-192"
	RenditionPWA512       BrandRendition = "pwa-512"
	RenditionMaskable512  BrandRendition = "maskable-512"
	RenditionLogoMail     BrandRendition = "logo-mail"
	RenditionLogoDarkMail BrandRendition = "logo-dark-mail"
	RenditionPreview      BrandRendition = "preview-1200x630"
)

// BrandRenditions returns the PNG renditions that a revision of kind must have, besides the SVG.
func BrandRenditions(kind BrandKind) []BrandRendition {
	switch kind {
	case BrandIcon:
		return []BrandRendition{RenditionFavicon32, RenditionFavicon48, RenditionAppleTouch, RenditionPWA192, RenditionPWA512, RenditionMaskable512}
	case BrandLogo:
		return []BrandRendition{RenditionLogoMail, RenditionPreview}
	case BrandLogoDark:
		return []BrandRendition{RenditionLogoDarkMail}
	}
	return nil
}

// ParseBrandRendition returns the rendition named s, and false for an unknown one.
func ParseBrandRendition(s string) (BrandRendition, bool) {
	if s == string(RenditionSVG) {
		return RenditionSVG, true
	}
	for _, k := range BrandKinds {
		for _, r := range BrandRenditions(k) {
			if string(r) == s {
				return r, true
			}
		}
	}
	return "", false
}

// Extension returns the file name extension of the rendition, without the dot.
func (r BrandRendition) Extension() string {
	if r == RenditionSVG {
		return "svg"
	}
	return "png"
}

// ContentType returns the media type of the rendition.
func (r BrandRendition) ContentType() string {
	if r == RenditionSVG {
		return BrandSVGContentType
	}
	return BrandPNGContentType
}

// opaque reports whether the rendition is drawn on the revision's background color: the places
// that fill transparent pixels on their own (home screens, masks, link previews).
func (r BrandRendition) opaque() bool {
	return r == RenditionAppleTouch || r == RenditionMaskable512 || r == RenditionPreview
}

// Media types of brand files.
const (
	BrandSVGContentType = "image/svg+xml"
	BrandPNGContentType = "image/png"
)

// MaxBrandPNGBytes is the largest PNG rendition accepted (1 MiB).
const MaxBrandPNGBytes = 1 << 20

// Sizes of the mail logo: 96 px high (twice the 48 px it shows at), at most 600 px wide.
const (
	brandMailLogoHeight   = 96
	brandMailLogoMaxWidth = 600
)

// BrandRenditionSize returns the width and height in pixels of rendition for an SVG whose
// viewBox is viewW by viewH: fixed for the icon renditions and the link preview, from the aspect
// ratio for the mail logos.
func BrandRenditionSize(r BrandRendition, viewW, viewH float64) (int, int) {
	switch r {
	case RenditionFavicon32:
		return 32, 32
	case RenditionFavicon48:
		return 48, 48
	case RenditionAppleTouch:
		return 180, 180
	case RenditionPWA192:
		return 192, 192
	case RenditionPWA512, RenditionMaskable512:
		return 512, 512
	case RenditionPreview:
		return 1200, 630
	case RenditionLogoMail, RenditionLogoDarkMail:
		if viewW <= 0 || viewH <= 0 {
			return 0, 0
		}
		ratio := viewW / viewH
		w := int(math.Round(brandMailLogoHeight * ratio))
		if w <= brandMailLogoMaxWidth {
			return w, brandMailLogoHeight
		}
		return brandMailLogoMaxWidth, int(math.Round(brandMailLogoMaxWidth / ratio))
	}
	return 0, 0
}

// BrandFile is one stored file of a brand revision (instance_brand_files).
type BrandFile struct {
	Rendition BrandRendition
	// Key is the BlobStore key, which the server chooses (BrandBlobKey).
	Key         string
	ContentType string
	// Width and Height are in pixels for a PNG and the viewBox's, rounded, for the SVG.
	Width, Height int
	Size          int64
	// SHA256 is the hex SHA-256 of the content.
	SHA256 string
}

// BrandBlobKey returns the BlobStore key of a file of a revision.
func BrandBlobKey(revisionID string, r BrandRendition) string {
	return "instance/brand/" + revisionID + "/" + string(r) + "." + r.Extension()
}

// BrandFilePath returns the path that serves a file of a revision. Its content never changes, so
// it is cached as immutable.
func BrandFilePath(revisionID string, r BrandRendition) string {
	return "/brand/" + revisionID + "/" + string(r) + "." + r.Extension()
}

// BrandRevision is one change of a brand image (instance_brand_revisions with its
// instance_brand_files). The revision with the greatest ID of each kind decides it: a set
// revision makes its files current, a clear revision brings back the built-in images.
type BrandRevision struct {
	ID     string
	Kind   BrandKind
	Action BrandAction
	// Background is the #rrggbb color behind the opaque renditions of an icon, "" otherwise.
	Background string
	// Files are the SVG and the PNG renditions of a set revision, none for a clear one.
	Files []BrandFile
	// RevisedByUserID is the user who made the change, or "" for a change from the CLI.
	RevisedByUserID string
	RevisedAt       time.Time
}

// File returns the file of rendition r, and false when the revision has none.
func (r BrandRevision) File(rendition BrandRendition) (BrandFile, bool) {
	for _, f := range r.Files {
		if f.Rendition == rendition {
			return f, true
		}
	}
	return BrandFile{}, false
}

// BrandHead points at the current revision of a kind (the instance_brands projection).
type BrandHead struct {
	Kind       BrandKind
	RevisionID string
}

// Supersedes reports whether r should replace the head h of its kind, which is the zero value
// when the kind has no current revision.
func (r BrandRevision) Supersedes(h BrandHead) bool {
	return h.RevisionID == "" || r.ID > h.RevisionID
}

// Brands are the current brand images of the instance; a nil field is unset and uses the
// built-in images.
type Brands struct {
	Icon, Logo, LogoDark *BrandRevision
}

// Get returns the current revision of kind, or nil when it is unset.
func (b Brands) Get(kind BrandKind) *BrandRevision {
	switch kind {
	case BrandIcon:
		return b.Icon
	case BrandLogo:
		return b.Logo
	case BrandLogoDark:
		return b.LogoDark
	}
	return nil
}

// Set replaces the current revision of rev.Kind.
func (b *Brands) Set(rev *BrandRevision) {
	switch rev.Kind {
	case BrandIcon:
		b.Icon = rev
	case BrandLogo:
		b.Logo = rev
	case BrandLogoDark:
		b.LogoDark = rev
	}
}

// Version identifies the current revisions, for the ETags of the pages that show them.
func (b Brands) Version() string {
	v := ""
	for _, k := range BrandKinds {
		v += string(k) + "="
		if r := b.Get(k); r != nil {
			v += r.ID
		}
		v += ";"
	}
	return v
}

// ErrInvalidBrand is wrapped by the errors of brand files that cannot be saved.
var ErrInvalidBrand = errors.New("instance: invalid brand image")

// BrandProblemCode says why a brand file is rejected.
type BrandProblemCode string

// Brand problem codes.
const (
	BrandMalformed           BrandProblemCode = "malformed"
	BrandDoctype             BrandProblemCode = "doctype"
	BrandNotSVG              BrandProblemCode = "not_svg"
	BrandNoViewBox           BrandProblemCode = "no_viewbox"
	BrandTooLarge            BrandProblemCode = "too_large"
	BrandTooManyElements     BrandProblemCode = "too_many_elements"
	BrandTooDeep             BrandProblemCode = "too_deep"
	BrandTooManyUses         BrandProblemCode = "too_many_uses"
	BrandAspectRatio         BrandProblemCode = "aspect_ratio"
	BrandText                BrandProblemCode = "text"
	BrandUnsafeCSS           BrandProblemCode = "unsafe_css"
	BrandMissingRendition    BrandProblemCode = "missing_rendition"
	BrandUnexpectedRendition BrandProblemCode = "unexpected_rendition"
	BrandInvalidPNG          BrandProblemCode = "invalid_png"
	BrandDimensions          BrandProblemCode = "dimensions"
	BrandNotOpaque           BrandProblemCode = "not_opaque"
	BrandInvalidColor        BrandProblemCode = "invalid_color"
)

// BrandError reports why a brand file is rejected. It wraps ErrInvalidBrand.
type BrandError struct {
	Code BrandProblemCode
	// Rendition is the file the problem is about, "" for the request as a whole.
	Rendition BrandRendition
	// Detail says more for the administrator, in English.
	Detail string
}

func (e *BrandError) Error() string {
	s := "brand: " + string(e.Code)
	if e.Rendition != "" {
		s += " (" + string(e.Rendition) + ")"
	}
	if e.Detail != "" {
		s += ": " + e.Detail
	}
	return s
}

// Unwrap makes errors.Is(err, ErrInvalidBrand) hold.
func (e *BrandError) Unwrap() error { return ErrInvalidBrand }

func brandError(code BrandProblemCode, r BrandRendition, format string, args ...any) *BrandError {
	return &BrandError{Code: code, Rendition: r, Detail: fmt.Sprintf(format, args...)}
}

// NormalizeBrandBackground checks the background color of an icon: #rrggbb, returned lowercase.
func NormalizeBrandBackground(s string) (string, error) {
	if !isHexColor(s, 6) {
		return "", brandError(BrandInvalidColor, "", "the background must be a #rrggbb color")
	}
	return toLowerASCII(s), nil
}

func isHexColor(s string, digits int) bool {
	if len(s) != digits+1 || s[0] != '#' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func toLowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// pngSignature starts every PNG file.
var pngSignature = []byte("\x89PNG\r\n\x1a\n")

// CheckBrandPNG checks a PNG rendition of an SVG whose viewBox is viewW by viewH (ADR-0116 §3):
// a PNG by its content, at most MaxBrandPNGBytes, of exactly the rendition's size (a mail logo may
// be 1 px off, for the rounding of the browser that drew it), and opaque where the rendition
// must be. It decodes the image and encodes it again with image/png, which drops every chunk of
// metadata, and returns the new file with its size. The dimensions are read from the header
// before the pixels, so a small file that claims a huge image is never decoded.
func CheckBrandPNG(r BrandRendition, data []byte, viewW, viewH float64) ([]byte, int, int, error) {
	if len(data) > MaxBrandPNGBytes {
		return nil, 0, 0, brandError(BrandTooLarge, r, "a rendition is at most %d bytes", MaxBrandPNGBytes)
	}
	if !bytes.HasPrefix(data, pngSignature) {
		return nil, 0, 0, brandError(BrandInvalidPNG, r, "the file is not a PNG")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, brandError(BrandInvalidPNG, r, "%v", err)
	}
	wantW, wantH := BrandRenditionSize(r, viewW, viewH)
	tolerance := 0
	if r == RenditionLogoMail || r == RenditionLogoDarkMail {
		tolerance = 1
	}
	if wantW == 0 || abs(cfg.Width-wantW) > tolerance || abs(cfg.Height-wantH) > tolerance {
		return nil, 0, 0, brandError(BrandDimensions, r, "the rendition must be %dx%d pixels, not %dx%d", wantW, wantH, cfg.Width, cfg.Height)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, brandError(BrandInvalidPNG, r, "%v", err)
	}
	if r.opaque() {
		if o, ok := img.(interface{ Opaque() bool }); !ok || !o.Opaque() {
			return nil, 0, 0, brandError(BrandNotOpaque, r, "the rendition must be opaque")
		}
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, 0, 0, fmt.Errorf("brand: encode %s: %w", r, err)
	}
	b := img.Bounds()
	return buf.Bytes(), b.Dx(), b.Dy(), nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
