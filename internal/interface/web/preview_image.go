// SPDX-License-Identifier: AGPL-3.0-only

package web

import (
	"bytes"
	_ "embed" // the built-in preview image
	"net/http"
	"time"
)

// The built-in preview image is the og:image of the pages without an image of their own when the
// instance has neither a preview_image, a thumbnail, nor a logo (ADR-0028 §2): mycroblog's
// colourful wordmark on white (ADR-0059 §8), one image that fits every instance. The server draws
// no SVG (ADR-0059 §10), so scripts/web-icons.sh renders it from
// app/ui/src/logos/Wordmark-Primary-Color.svg and it is committed beside this file.

// PathDefaultImage is the path of the built-in preview image.
const PathDefaultImage = "/preview-image.png"

// Size of the built-in preview image, the 1.91:1 size that link previews show as a large
// image.
const (
	DefaultImageWidth  = 1200
	DefaultImageHeight = 630
)

//go:embed preview-image.png
var defaultImagePNG []byte

// defaultImageETag is the ETag of the built-in preview image.
var defaultImageETag = etag(defaultImagePNG)

// serveDefaultImage serves GET /preview-image.png.
func (h *Handler) serveDefaultImage(w http.ResponseWriter, r *http.Request) {
	hd := w.Header()
	hd.Set("Content-Type", "image/png")
	hd.Set("Cache-Control", cacheRevalidate)
	hd.Set("ETag", defaultImageETag)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(defaultImagePNG))
}

// builtInImage is the built-in preview image with alt text alt.
func (h *Handler) builtInImage(alt string) *Image {
	return &Image{URL: "https://" + h.o.Host + PathDefaultImage, Width: DefaultImageWidth, Height: DefaultImageHeight, Alt: alt}
}
