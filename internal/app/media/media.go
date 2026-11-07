// SPDX-License-Identifier: AGPL-3.0-only

// Package media defines the MediaProcessor port (ADR-0005, ADR-0019 §8): format detection
// from content, size and dimension limits, metadata removal by re-encoding, resizing,
// thumbnails, and blurhash. The adapters live in internal/adapter/media/* and pass
// mediatest.MediaProcessorContract.
//
// The accepted formats are JPEG, PNG (an APNG is read as its default image), GIF, and WebP,
// the last two animated or not. Every file gets small, medium, and large still variants within
// the boxes of its purpose (ADR-0005 §4, §5): the large one is JPEG when opaque and PNG
// otherwise, which every federated server shows (always PNG for a custom emoji, which servers
// do not take as JPEG, ADR-0051 §2), and the small and medium ones are WebP. An
// animation also gets moving variants: a GIF for the large one and animated WebP for the
// others (§3); its still variants show its first frame. Video and audio files are remuxed
// without metadata and never transcoded (§6, AVProcessor).
package media

import (
	"bytes"
	"context"
	"errors"
)

var (
	// ErrUnsupportedFormat reports content that is not one of the accepted formats, whatever
	// its file name or declared type (SVG and HTML included).
	ErrUnsupportedFormat = errors.New("media: unsupported format")
	// ErrTooLarge reports content longer than Limits.MaxBytes.
	ErrTooLarge = errors.New("media: file too large")
	// ErrDimensions reports an image wider or taller than Limits.MaxSide, with more pixels than
	// Limits.MaxPixels, or with more frames than Limits.MaxFrames.
	ErrDimensions = errors.New("media: image dimensions exceed the limits")
	// ErrCorrupt reports content of an accepted format that does not decode.
	ErrCorrupt = errors.New("media: corrupt image")
)

// Format is a detected file format.
type Format string

// Formats.
const (
	FormatJPEG Format = "jpeg"
	FormatPNG  Format = "png"
	FormatGIF  Format = "gif"
	FormatWebP Format = "webp"
)

// ContentType returns the media type of f, or "" for an unknown format.
func (f Format) ContentType() string {
	switch f {
	case FormatJPEG:
		return "image/jpeg"
	case FormatPNG:
		return "image/png"
	case FormatGIF:
		return "image/gif"
	case FormatWebP:
		return "image/webp"
	}
	return ""
}

// Sniff detects the format of data from its leading bytes alone, or returns "" when data is in
// none of the accepted formats. File names and declared types are never consulted.
func Sniff(data []byte) Format {
	switch {
	case bytes.HasPrefix(data, []byte("\xFF\xD8\xFF")):
		return FormatJPEG
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1A\n")):
		return FormatPNG
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return FormatGIF
	case len(data) >= 12 && bytes.HasPrefix(data, []byte("RIFF")) && string(data[8:12]) == "WEBP":
		return FormatWebP
	}
	return ""
}

// Limits bound what a processor accepts. Processors check the dimensions from the file's
// headers before decoding its pixels, so that a small file that expands to a huge image
// (a decompression bomb) is rejected without allocating it.
type Limits struct {
	// MaxBytes is the longest accepted content.
	MaxBytes int64
	// MaxSide is the largest accepted width and height in pixels.
	MaxSide int
	// MaxPixels is the largest accepted width × height. For an animation it also bounds the
	// pixels of all frames together, and CompositedPixelsFactor times it bounds the frames
	// times the canvas, the pixels that compositing every frame on the canvas goes through.
	MaxPixels int64
	// MaxFrames is the largest accepted number of frames of an animation.
	MaxFrames int
}

// Default limits (ADR-0019 §8).
const (
	DefaultMaxBytes  = 16 << 20
	DefaultMaxSide   = 8192
	DefaultMaxPixels = 7680 * 4320 // 33 MP, 8K UHD
	DefaultMaxFrames = 500
)

// CompositedPixelsFactor bounds the frames × canvas pixels of an animation at this multiple of
// Limits.MaxPixels.
const CompositedPixelsFactor = 4

// DefaultLimits returns the limits of ADR-0019 §8.
func DefaultLimits() Limits {
	return Limits{MaxBytes: DefaultMaxBytes, MaxSide: DefaultMaxSide, MaxPixels: DefaultMaxPixels, MaxFrames: DefaultMaxFrames}
}

// WithDefaults returns l with its zero or negative fields set to the defaults.
func (l Limits) WithDefaults() Limits {
	d := DefaultLimits()
	if l.MaxBytes <= 0 {
		l.MaxBytes = d.MaxBytes
	}
	if l.MaxSide <= 0 {
		l.MaxSide = d.MaxSide
	}
	if l.MaxPixels <= 0 {
		l.MaxPixels = d.MaxPixels
	}
	if l.MaxFrames <= 0 {
		l.MaxFrames = d.MaxFrames
	}
	return l
}

// Probe is what Inspect learns from a file's headers.
type Probe struct {
	Format Format
	// Width and Height are the stored dimensions, before any EXIF orientation.
	Width, Height int
	// Frames is the number of frames: 1 for a still image.
	Frames int
}

// Purpose is what a media file is used for; it picks the boxes of its variants (ADR-0005 §4).
type Purpose string

// Purposes.
const (
	PurposeAttachment Purpose = "attachment"
	PurposeAvatar     Purpose = "avatar"
	PurposeHeader     Purpose = "header"
	PurposeEmoji      Purpose = "emoji"
)

// Valid reports whether p is one of the purposes.
func (p Purpose) Valid() bool {
	switch p {
	case PurposeAttachment, PurposeAvatar, PurposeHeader, PurposeEmoji:
		return true
	}
	return false
}

// Size names one of the three variants of a file in one motion.
type Size string

// Sizes of variants.
const (
	SizeSmall  Size = "small"
	SizeMedium Size = "medium"
	SizeLarge  Size = "large"
)

// AllSizes lists the sizes, smallest first.
var AllSizes = [...]Size{SizeSmall, SizeMedium, SizeLarge}

// Box bounds one variant. With MaxPixels set, the variant keeps the source's aspect ratio
// within that area; otherwise it keeps the aspect ratio within Width × Height, or, with Crop,
// is first cropped to the aspect ratio of Width × Height around the focal point. A source that
// already fits is not enlarged.
type Box struct {
	MaxPixels     int64
	Width, Height int
	Crop          bool
}

// Boxes are the boxes of the three variants of one purpose.
type Boxes struct {
	Small, Medium, Large Box
}

// Box returns the box of size.
func (b Boxes) Box(size Size) Box {
	switch size {
	case SizeSmall:
		return b.Small
	case SizeMedium:
		return b.Medium
	}
	return b.Large
}

// Sizes are the boxes of the variants of each purpose (ADR-0005 §4).
type Sizes struct {
	Attachment, Avatar, Header, Emoji Boxes
}

// DefaultSizes returns the boxes of ADR-0005 §4: attachments within 640 × 360, 1920 × 1080,
// and 3840 × 2160 pixels of area; avatars cropped to squares of 96, 192, and 400 pixels;
// headers within 750 × 250, 1500 × 500, and 3000 × 1000; custom emoji within squares of 64,
// 128, and 256 pixels.
func DefaultSizes() Sizes {
	within := func(w, h int) Box { return Box{Width: w, Height: h} }
	square := func(n int) Box { return Box{Width: n, Height: n, Crop: true} }
	return Sizes{
		Attachment: Boxes{Small: Box{MaxPixels: 640 * 360}, Medium: Box{MaxPixels: 1920 * 1080}, Large: Box{MaxPixels: 3840 * 2160}},
		Avatar:     Boxes{Small: square(96), Medium: square(192), Large: square(400)},
		Header:     Boxes{Small: within(750, 250), Medium: within(1500, 500), Large: within(3000, 1000)},
		Emoji:      Boxes{Small: within(64, 64), Medium: within(128, 128), Large: within(256, 256)},
	}
}

// WithDefaults returns s with the boxes of each purpose that are all zero set to the defaults.
func (s Sizes) WithDefaults() Sizes {
	d := DefaultSizes()
	for _, p := range []struct{ got, def *Boxes }{
		{&s.Attachment, &d.Attachment}, {&s.Avatar, &d.Avatar}, {&s.Header, &d.Header}, {&s.Emoji, &d.Emoji},
	} {
		if *p.got == (Boxes{}) {
			*p.got = *p.def
		}
	}
	return s
}

// For returns the boxes of purpose p; an unknown purpose has the boxes of an attachment.
func (s Sizes) For(p Purpose) Boxes {
	switch p {
	case PurposeAvatar:
		return s.Avatar
	case PurposeHeader:
		return s.Header
	case PurposeEmoji:
		return s.Emoji
	}
	return s.Attachment
}

// Request says what to make of a file.
type Request struct {
	// Purpose picks the boxes of the variants; "" is PurposeAttachment.
	Purpose Purpose
	// FocusX and FocusY are the focal point that a crop keeps in view, from -1.0 (left,
	// bottom) to 1.0 (right, top); (0, 0) is the center.
	FocusX, FocusY float64
}

// Variant is one processed rendition of a file.
type Variant struct {
	Data        []byte
	ContentType string
	// Extension is the file name extension for ContentType, without the dot ("jpg").
	Extension     string
	Width, Height int
}

// Variants are the three renditions of a file in one motion.
type Variants struct {
	Small, Medium, Large Variant
}

// Variant returns the variant of size.
func (v Variants) Variant(size Size) Variant {
	switch size {
	case SizeSmall:
		return v.Small
	case SizeMedium:
		return v.Medium
	}
	return v.Large
}

// Output is the result of processing a file (ADR-0005 §4 to §6).
type Output struct {
	// Width and Height are the dimensions of the source after its EXIF orientation.
	Width, Height int
	// Still are the still variants; of an animation, they show its first frame. Large is
	// JPEG when opaque and PNG otherwise (always PNG for PurposeEmoji); Small and Medium are
	// WebP.
	Still Variants
	// Animated are the moving variants of an animation, nil for a still image. Large is a
	// GIF; Small and Medium are animated WebP.
	Animated *Variants
	// Blurhash is the blurhash of Still.Small (https://blurha.sh).
	Blurhash string
}

// Processor is the MediaProcessor port. Zero fields of Limits and Sizes take their defaults
// (WithDefaults). Implementations bound the time (through ctx) and the
// number of concurrent decodes, and turn a panic of a decoder into ErrCorrupt.
type Processor interface {
	// Inspect detects the format of data from its content and reads its dimensions from its
	// headers, without decoding the pixels. It fails with an error wrapping
	// ErrUnsupportedFormat, ErrTooLarge, ErrDimensions, or ErrCorrupt when data breaks limits.
	Inspect(ctx context.Context, data []byte, limits Limits) (Probe, error)
	// Process inspects data as Inspect does, then decodes it, applies its EXIF orientation,
	// and re-encodes it without metadata into the variants within the boxes of req.Purpose
	// in sizes. The same input and request give the same output, so that processing is safe
	// to repeat.
	Process(ctx context.Context, data []byte, limits Limits, sizes Sizes, req Request) (Output, error)
	// Preview inspects data as Inspect does and makes only the variant that Process makes as
	// Output.Still.Small, decoding no more than the first frame. The media proxy makes it of
	// remote files (ADR-0005 §10).
	Preview(ctx context.Context, data []byte, limits Limits, sizes Sizes, req Request) (Variant, error)
	// AVProcessor checks and remuxes video and audio files.
	AVProcessor
}

// Rejected reports whether err means the content is not acceptable (as opposed to a failure
// such as a canceled context, after which a retry may succeed).
func Rejected(err error) bool {
	return errors.Is(err, ErrUnsupportedFormat) || errors.Is(err, ErrTooLarge) ||
		errors.Is(err, ErrDimensions) || errors.Is(err, ErrCorrupt) || errors.Is(err, ErrTooLong)
}
