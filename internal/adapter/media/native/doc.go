// SPDX-License-Identifier: AGPL-3.0-only

// Package native implements the MediaProcessor port (internal/app/media) in pure Go, so that
// the server keeps building with CGO_ENABLED=0 (ADR-0005). It passes
// mediatest.MediaProcessorContract.
//
// WebP is written with github.com/deepteams/webp, a pure-Go encoder, and animated WebP is read
// with it too (ADR-0005 §5, §6); still WebP is read with golang.org/x/image/webp.
//
// JPEG files have their EXIF orientation applied to the pixels before re-encoding. ICC color
// profiles are dropped with the rest of the metadata, so wide-gamut images are shown as sRGB.
package native
