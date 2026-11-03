// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	dwebp "github.com/deepteams/webp"
	"github.com/deepteams/webp/animation"
	xwebp "golang.org/x/image/webp"

	"github.com/mycroblog/my/internal/app/media"
)

// MediaProcessorContract verifies an adapter of the MediaProcessor port (ADR-0019 §8,
// ADR-0054): format detection from content alone, rejection of unsupported, spoofed, oversized,
// and decompression-bomb files, removal of metadata (EXIF and GPS, PNG text, GIF comments,
// WebP EXIF), the EXIF orientation, the small, medium, and large variants within the boxes of
// each purpose (the avatars cropped to squares around the focal point), the still and moving
// variants of animations, the output formats, a valid blurhash, repeatable output, and
// cancellation; and for video and audio files (ADR-0054 §6), the remux of MP4, QuickTime, WebM,
// MP3, and Ogg without metadata with their positions rewritten, the codec allowlist (no HEVC),
// and the size, duration, dimension, and frame rate limits.
func MediaProcessorContract(t *testing.T, p media.Processor) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, p media.Processor)
	}{
		{"DetectsFormatsFromContent", mediaDetectsFormats},
		{"RejectsUnsupportedContent", mediaRejectsUnsupported},
		{"RejectsOversizedFiles", mediaRejectsOversized},
		{"RejectsDimensionsFromHeadersBeforeDecoding", mediaRejectsDimensions},
		{"RejectsTooManyFrames", mediaRejectsTooManyFrames},
		{"StripsMetadata", mediaStripsMetadata},
		{"AppliesTheEXIFOrientation", mediaAppliesOrientation},
		{"ResizesWithinTheBoxesOfThePurpose", mediaResizes},
		{"CropsAvatarsAroundTheFocus", mediaCropsAvatars},
		{"WritesTheDocumentedFormats", mediaWritesFormats},
		{"MakesStillAndMovingVariantsOfAnimations", mediaAnimates},
		{"PreviewsTheSmallStillVariant", mediaPreviews},
		{"ComputesABlurhash", mediaComputesBlurhash},
		{"GivesTheSameOutputForTheSameInput", mediaIsDeterministic},
		{"StopsOnCanceledContext", mediaStopsOnCancel},
	}
	cases = append(cases, mediaAVCases...)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, p) })
	}
}

func mediaProcess(t *testing.T, p media.Processor, data []byte) media.Output {
	t.Helper()
	return mediaProcessFor(t, p, data, media.Sizes{}, media.Request{})
}

func mediaProcessFor(t *testing.T, p media.Processor, data []byte, sizes media.Sizes, req media.Request) media.Output {
	t.Helper()
	out, err := p.Process(t.Context(), data, media.Limits{}, sizes, req)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	return out
}

// mediaVariants returns every variant of out by name ("still small", "animated large").
func mediaVariants(out media.Output) map[string]media.Variant {
	all := map[string]media.Variant{}
	for _, size := range media.AllSizes {
		all["still "+string(size)] = out.Still.Variant(size)
		if out.Animated != nil {
			all["animated "+string(size)] = out.Animated.Variant(size)
		}
	}
	return all
}

func mediaDetectsFormats(t *testing.T, p media.Processor) {
	for _, tc := range []struct {
		name   string
		data   []byte
		format media.Format
		w, h   int
		frames int
	}{
		{"jpeg", FixtureJPEG(40, 20, 1), media.FormatJPEG, 40, 20, 1},
		{"png", FixturePNG(30, 10, false), media.FormatPNG, 30, 10, 1},
		{"gif", FixtureGIF(16, 8, 3), media.FormatGIF, 16, 8, 3},
		{"webp lossy", FixtureWebPLossy, media.FormatWebP, 1, 1, 1},
		{"webp lossless", FixtureWebPLossless, media.FormatWebP, 1, 1, 1},
		{"webp alpha", FixtureWebPAlpha, media.FormatWebP, 1, 1, 1},
		{"webp animated with one frame", FixtureWebPAnimated, media.FormatWebP, 1, 1, 1},
		{"webp animation", FixtureWebPAnimation(16, 8, 3), media.FormatWebP, 16, 8, 3},
	} {
		probe, err := p.Inspect(t.Context(), tc.data, media.Limits{})
		if err != nil {
			t.Errorf("%s: Inspect: %v", tc.name, err)
			continue
		}
		want := media.Probe{Format: tc.format, Width: tc.w, Height: tc.h, Frames: tc.frames}
		if probe != want {
			t.Errorf("%s: Inspect = %+v, want %+v", tc.name, probe, want)
		}
	}
}

func mediaRejectsUnsupported(t *testing.T, p media.Processor) {
	for name, data := range map[string][]byte{
		"empty":        nil,
		"svg":          []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"></svg>`),
		"svg with xml": []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`),
		"html":         []byte("<!DOCTYPE html><script>alert(1)</script>"),
		"pdf":          []byte("%PDF-1.7\n"),
		"riff wave":    []byte("RIFF\x24\x00\x00\x00WAVEfmt "),
		"text":         []byte("just text"),
		// HTML after a JPEG signature: detected as JPEG, but it does not decode.
		"spoofed jpeg":       []byte("\xFF\xD8\xFF<html><script>alert(1)</script></html>"),
		"spoofed png":        []byte("\x89PNG\r\n\x1A\n<script>alert(1)</script>"),
		"spoofed gif":        []byte("GIF89a<script>alert(1)</script>"),
		"truncated":          FixtureJPEG(40, 20, 1)[:200],
		"truncated webp":     FixtureWebPAnimation(16, 8, 3)[:60],
		"webp without frame": FixtureWebPAnimated[:len(FixtureWebPAnimated)-46],
	} {
		if _, err := p.Inspect(t.Context(), data, media.Limits{}); !media.Rejected(err) {
			if _, err := p.Process(t.Context(), data, media.Limits{}, media.Sizes{}, media.Request{}); !media.Rejected(err) {
				t.Errorf("%s: Inspect and Process accepted it (Process: %v)", name, err)
			}
		}
		if _, err := p.Process(t.Context(), data, media.Limits{}, media.Sizes{}, media.Request{}); !media.Rejected(err) {
			t.Errorf("%s: Process = %v, want a rejection", name, err)
		}
	}
	if _, err := p.Inspect(t.Context(), []byte("<svg/>"), media.Limits{}); !errors.Is(err, media.ErrUnsupportedFormat) {
		t.Errorf("svg: %v, want ErrUnsupportedFormat", err)
	}
}

func mediaRejectsOversized(t *testing.T, p media.Processor) {
	data := FixturePNG(30, 10, false)
	limits := media.Limits{MaxBytes: int64(len(data) - 1)}
	if _, err := p.Inspect(t.Context(), data, limits); !errors.Is(err, media.ErrTooLarge) {
		t.Errorf("Inspect = %v, want ErrTooLarge", err)
	}
	if _, err := p.Process(t.Context(), data, limits, media.Sizes{}, media.Request{}); !errors.Is(err, media.ErrTooLarge) {
		t.Errorf("Process = %v, want ErrTooLarge", err)
	}
	if _, err := p.Inspect(t.Context(), data, media.Limits{MaxBytes: int64(len(data))}); err != nil {
		t.Errorf("Inspect at the limit = %v", err)
	}
}

func mediaRejectsDimensions(t *testing.T, p media.Processor) {
	// A file of a few kilobytes whose header claims 4 gigapixels: decoding it would allocate them.
	bomb := FixturePNGHeader(65535, 65535)
	if _, err := p.Inspect(t.Context(), bomb, media.Limits{}); !errors.Is(err, media.ErrDimensions) {
		t.Errorf("bomb: Inspect = %v, want ErrDimensions", err)
	}
	if _, err := p.Process(t.Context(), bomb, media.Limits{}, media.Sizes{}, media.Request{}); !errors.Is(err, media.ErrDimensions) {
		t.Errorf("bomb: Process = %v, want ErrDimensions", err)
	}
	// Within the default side limit but over the default pixel limit (8000 × 8000 = 64 MP).
	if _, err := p.Inspect(t.Context(), FixturePNGHeader(8000, 8000), media.Limits{}); !errors.Is(err, media.ErrDimensions) {
		t.Errorf("64 MP: Inspect = %v, want ErrDimensions", err)
	}
	img := FixtureJPEG(40, 20, 1)
	for name, limits := range map[string]media.Limits{
		"side":   {MaxSide: 39},
		"pixels": {MaxPixels: 799},
	} {
		if _, err := p.Inspect(t.Context(), img, limits); !errors.Is(err, media.ErrDimensions) {
			t.Errorf("%s: Inspect = %v, want ErrDimensions", name, err)
		}
	}
	if _, err := p.Inspect(t.Context(), img, media.Limits{MaxSide: 40, MaxPixels: 800}); err != nil {
		t.Errorf("at the limits: Inspect = %v", err)
	}
}

func mediaRejectsTooManyFrames(t *testing.T, p media.Processor) {
	for name, data := range map[string][]byte{"gif": FixtureGIF(16, 8, 3), "webp": FixtureWebPAnimation(16, 8, 3)} {
		if _, err := p.Inspect(t.Context(), data, media.Limits{MaxFrames: 2}); !errors.Is(err, media.ErrDimensions) {
			t.Errorf("%s frames: Inspect = %v, want ErrDimensions", name, err)
		}
		// The pixel limit covers all frames together: 3 × 128 pixels.
		if _, err := p.Inspect(t.Context(), data, media.Limits{MaxPixels: 3*128 - 1}); !errors.Is(err, media.ErrDimensions) {
			t.Errorf("%s frame pixels: Inspect = %v, want ErrDimensions", name, err)
		}
		if _, err := p.Inspect(t.Context(), data, media.Limits{MaxFrames: 3, MaxPixels: 3 * 128}); err != nil {
			t.Errorf("%s at the limits: Inspect = %v", name, err)
		}
	}
}

func mediaStripsMetadata(t *testing.T, p media.Processor) {
	for name, data := range map[string][]byte{
		"jpeg":           FixtureJPEG(64, 48, 1),
		"png":            FixturePNG(64, 48, false),
		"gif":            FixtureGIF(64, 48, 2),
		"webp animation": FixtureWebPAnimation(64, 48, 2),
	} {
		if !bytes.Contains(data, []byte(MetadataMarker)) {
			t.Fatalf("%s: the fixture has no metadata", name)
		}
		for _, purpose := range []media.Purpose{media.PurposeAttachment, media.PurposeAvatar} {
			out := mediaProcessFor(t, p, data, media.Sizes{}, media.Request{Purpose: purpose})
			for vname, v := range mediaVariants(out) {
				for _, needle := range []string{MetadataMarker, "Exif\x00", "tEXt", "zTXt", "iTXt", "eXIf", "EXIF", "XMP ", "ICCP", "GPS", "http://ns.adobe.com/xap/"} {
					if bytes.Contains(v.Data, []byte(needle)) {
						t.Errorf("%s %s %s: contains %q", name, purpose, vname, needle)
					}
				}
				if v.ContentType == "image/jpeg" && hasJPEGSegment(v.Data, 0xE1) {
					t.Errorf("%s %s %s: has an APP1 segment", name, purpose, vname)
				}
			}
		}
	}
}

// hasJPEGSegment reports whether a JPEG has a segment with the given marker before its scan.
func hasJPEGSegment(data []byte, marker byte) bool {
	for p := 2; p+4 <= len(data) && data[p] == 0xFF; {
		m := data[p+1]
		if m == 0xDA {
			return false
		}
		if m == marker {
			return true
		}
		p += 2 + int(data[p+2])<<8 + int(data[p+3])
	}
	return false
}

// webpAnimationFlag reports whether a WebP file is an extended file with the animation flag.
func webpAnimationFlag(data []byte) bool {
	return len(data) > 20 && string(data[12:16]) == "VP8X" && data[20]&0x02 != 0
}

// decodeVariant decodes a variant, the first frame of a moving one, with a decoder other than
// the encoder's where there is one, and checks its size.
func decodeVariant(t *testing.T, v media.Variant) image.Image {
	t.Helper()
	var (
		img image.Image
		err error
	)
	switch {
	case v.ContentType == "image/jpeg":
		img, err = jpeg.Decode(bytes.NewReader(v.Data))
	case v.ContentType == "image/png":
		img, err = png.Decode(bytes.NewReader(v.Data))
	case v.ContentType == "image/gif":
		img, err = gif.Decode(bytes.NewReader(v.Data))
	case v.ContentType == "image/webp" && webpAnimationFlag(v.Data):
		img, err = dwebp.Decode(bytes.NewReader(v.Data))
	case v.ContentType == "image/webp":
		img, err = xwebp.Decode(bytes.NewReader(v.Data))
	default:
		t.Fatalf("unexpected content type %q", v.ContentType)
	}
	if err != nil {
		t.Fatalf("decode %s: %v", v.ContentType, err)
	}
	if b := img.Bounds(); b.Dx() != v.Width || b.Dy() != v.Height {
		t.Fatalf("%s decodes to %v, variant says %d×%d", v.ContentType, b, v.Width, v.Height)
	}
	return img
}

// near reports whether two colors differ by at most 40 in every channel (lossy codecs).
func near(a color.Color, b color.NRGBA) bool {
	c := color.NRGBAModel.Convert(a).(color.NRGBA)
	d := func(x, y uint8) bool { return max(x, y)-min(x, y) <= 40 }
	return d(c.R, b.R) && d(c.G, b.G) && d(c.B, b.B)
}

func mediaAppliesOrientation(t *testing.T, p media.Processor) {
	// The source is 40 × 20: left half FixtureLeft, right half FixtureRight.
	for _, tc := range []struct {
		orientation    int
		w, h           int
		first, second  color.NRGBA
		firstAt, secAt image.Point
	}{
		{1, 40, 20, FixtureLeft, FixtureRight, image.Pt(5, 10), image.Pt(35, 10)},
		{2, 40, 20, FixtureRight, FixtureLeft, image.Pt(5, 10), image.Pt(35, 10)},
		{3, 40, 20, FixtureRight, FixtureLeft, image.Pt(5, 10), image.Pt(35, 10)},
		{6, 20, 40, FixtureLeft, FixtureRight, image.Pt(10, 5), image.Pt(10, 35)},
		{8, 20, 40, FixtureRight, FixtureLeft, image.Pt(10, 5), image.Pt(10, 35)},
	} {
		out := mediaProcess(t, p, FixtureJPEG(40, 20, tc.orientation))
		if out.Width != tc.w || out.Height != tc.h {
			t.Errorf("orientation %d: %d×%d, want %d×%d", tc.orientation, out.Width, out.Height, tc.w, tc.h)
			continue
		}
		for _, size := range media.AllSizes {
			img := decodeVariant(t, out.Still.Variant(size))
			if !near(img.At(tc.firstAt.X, tc.firstAt.Y), tc.first) || !near(img.At(tc.secAt.X, tc.secAt.Y), tc.second) {
				t.Errorf("orientation %d %s: colors at %v and %v are %v and %v", tc.orientation, size, tc.firstAt, tc.secAt,
					img.At(tc.firstAt.X, tc.firstAt.Y), img.At(tc.secAt.X, tc.secAt.Y))
			}
		}
	}
}

func mediaResizes(t *testing.T, p media.Processor) {
	sizes := media.Sizes{
		Attachment: media.Boxes{Small: media.Box{MaxPixels: 40 * 30}, Medium: media.Box{MaxPixels: 80 * 60}, Large: media.Box{MaxPixels: 100 * 75}},
		Header:     media.Boxes{Small: media.Box{Width: 30, Height: 10}, Medium: media.Box{Width: 60, Height: 20}, Large: media.Box{Width: 120, Height: 40}},
	}
	check := func(name string, v media.Variant, w, h int) {
		t.Helper()
		decodeVariant(t, v)
		if v.Width != w || v.Height != h {
			t.Errorf("%s: %d×%d, want %d×%d", name, v.Width, v.Height, w, h)
		}
	}
	for _, src := range []struct {
		name string
		data []byte
	}{{"png", FixturePNG(400, 300, false)}, {"gif", FixtureGIF(400, 300, 2)}, {"webp animation", FixtureWebPAnimation(400, 300, 2)}} {
		out := mediaProcessFor(t, p, src.data, sizes, media.Request{Purpose: media.PurposeAttachment})
		if out.Width != 400 || out.Height != 300 {
			t.Errorf("%s: source size %d×%d, want 400×300", src.name, out.Width, out.Height)
		}
		// Within an area, the aspect ratio is kept and the pixels stay under the bound.
		check(src.name+" attachment small", out.Still.Small, 40, 30)
		check(src.name+" attachment medium", out.Still.Medium, 80, 60)
		check(src.name+" attachment large", out.Still.Large, 100, 75)
		if out.Animated != nil {
			check(src.name+" attachment animated small", out.Animated.Small, 40, 30)
			check(src.name+" attachment animated medium", out.Animated.Medium, 80, 60)
			check(src.name+" attachment animated large", out.Animated.Large, 100, 75)
		}
		// Within a box, the aspect ratio is kept and nothing is cropped.
		out = mediaProcessFor(t, p, src.data, sizes, media.Request{Purpose: media.PurposeHeader})
		check(src.name+" header small", out.Still.Small, 13, 10)
		check(src.name+" header medium", out.Still.Medium, 27, 20)
		check(src.name+" header large", out.Still.Large, 53, 40)
	}

	// The default boxes of ADR-0054 §1.
	out := mediaProcessFor(t, p, FixturePNG(1000, 500, false), media.Sizes{}, media.Request{Purpose: media.PurposeAvatar})
	check("default avatar small", out.Still.Small, 96, 96)
	check("default avatar medium", out.Still.Medium, 192, 192)
	check("default avatar large", out.Still.Large, 400, 400)
	out = mediaProcessFor(t, p, FixturePNG(1000, 500, false), media.Sizes{}, media.Request{Purpose: media.PurposeEmoji})
	check("default emoji small", out.Still.Small, 64, 32)
	check("default emoji large", out.Still.Large, 256, 128)
	out = mediaProcessFor(t, p, FixturePNG(4000, 1000, false), media.Sizes{}, media.Request{Purpose: media.PurposeHeader})
	check("default header small", out.Still.Small, 750, 188)
	check("default header large", out.Still.Large, 3000, 750)

	// A source smaller than the boxes is not enlarged; an avatar is still cropped square.
	small := mediaProcessFor(t, p, FixturePNG(30, 10, false), sizes, media.Request{})
	for name, v := range mediaVariants(small) {
		check("small source "+name, v, 30, 10)
	}
	small = mediaProcessFor(t, p, FixturePNG(30, 10, false), media.Sizes{}, media.Request{Purpose: media.PurposeAvatar})
	for name, v := range mediaVariants(small) {
		check("small avatar "+name, v, 10, 10)
	}
}

func mediaCropsAvatars(t *testing.T, p media.Processor) {
	// The source is 200 × 50: left half FixtureLeft, right half FixtureRight. A square of 50
	// around the left edge shows only the left half, one around the right edge only the right.
	for _, tc := range []struct {
		name   string
		focusX float64
		want   color.NRGBA
	}{{"left", -1, FixtureLeft}, {"right", 1, FixtureRight}, {"left quarter", -0.5, FixtureLeft}} {
		for name, data := range map[string][]byte{"png": FixturePNG(200, 50, false), "gif": FixtureGIF(200, 50, 2)} {
			out := mediaProcessFor(t, p, data, media.Sizes{}, media.Request{Purpose: media.PurposeAvatar, FocusX: tc.focusX})
			for vname, v := range mediaVariants(out) {
				if strings.HasPrefix(vname, "animated") {
					continue // the later frames are plain colors
				}
				img := decodeVariant(t, v)
				if v.Width != 50 || v.Height != 50 {
					t.Errorf("%s %s %s: %d×%d, want 50×50", tc.name, name, vname, v.Width, v.Height)
				}
				for _, pt := range []image.Point{{2, 2}, {25, 25}, {47, 47}} {
					if c := img.At(pt.X, pt.Y); !near(c, tc.want) {
						t.Errorf("%s %s %s: %v at %v, want %v", tc.name, name, vname, c, pt, tc.want)
					}
				}
			}
		}
	}
	// With the focus in the center, the square shows both halves.
	img := decodeVariant(t, mediaProcessFor(t, p, FixturePNG(200, 50, false), media.Sizes{}, media.Request{Purpose: media.PurposeAvatar}).Still.Large)
	if !near(img.At(5, 25), FixtureLeft) || !near(img.At(45, 25), FixtureRight) {
		t.Errorf("centered: %v and %v at the edges", img.At(5, 25), img.At(45, 25))
	}
}

func mediaWritesFormats(t *testing.T, p media.Processor) {
	ext := map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/gif": "gif", "image/webp": "webp"}
	for _, tc := range []struct {
		name     string
		data     []byte
		large    string
		animated bool
	}{
		{"jpeg", FixtureJPEG(40, 20, 1), "image/jpeg", false},
		{"png", FixturePNG(40, 20, false), "image/jpeg", false},
		{"png with alpha", FixturePNG(40, 20, true), "image/png", false},
		{"webp lossy", FixtureWebPLossy, "image/jpeg", false},
		{"webp lossless, transparent", FixtureWebPLossless, "image/png", false},
		{"webp alpha", FixtureWebPAlpha, "image/png", false},
		{"webp animated with one transparent frame", FixtureWebPAnimated, "image/png", false},
		{"gif with one frame", FixtureGIF(16, 8, 1), "image/jpeg", false},
		{"gif", FixtureGIF(16, 8, 3), "image/jpeg", true},
		{"webp animation", FixtureWebPAnimation(16, 8, 3), "image/jpeg", true},
	} {
		out := mediaProcess(t, p, tc.data)
		want := map[media.Size]string{media.SizeSmall: "image/webp", media.SizeMedium: "image/webp", media.SizeLarge: tc.large}
		for _, size := range media.AllSizes {
			v := out.Still.Variant(size)
			if v.ContentType != want[size] || v.Extension != ext[v.ContentType] {
				t.Errorf("%s: still %s is %s (.%s), want %s", tc.name, size, v.ContentType, v.Extension, want[size])
				continue
			}
			if v.ContentType == "image/webp" && webpAnimationFlag(v.Data) {
				t.Errorf("%s: still %s is an animated WebP", tc.name, size)
			}
			decodeVariant(t, v)
		}
		if (out.Animated != nil) != tc.animated {
			t.Errorf("%s: moving variants %v, want %v", tc.name, out.Animated != nil, tc.animated)
		}
	}
	// The large still variant of a custom emoji is PNG even when opaque, still or animated: no
	// server takes JPEG as an emoji (ADR-0051 §2).
	for name, data := range map[string][]byte{"opaque png": FixturePNG(40, 20, false), "jpeg": FixtureJPEG(40, 20, 1), "gif": FixtureGIF(16, 8, 3)} {
		out := mediaProcessFor(t, p, data, media.Sizes{}, media.Request{Purpose: media.PurposeEmoji})
		if v := out.Still.Large; v.ContentType != "image/png" || v.Extension != "png" {
			t.Errorf("emoji of %s: still large is %s (.%s), want image/png", name, v.ContentType, v.Extension)
		} else {
			decodeVariant(t, v)
		}
	}
	// Transparency gives the small and medium variants an alpha channel.
	out := mediaProcess(t, p, FixturePNG(40, 20, true))
	for _, v := range []media.Variant{out.Still.Small, out.Still.Medium} {
		if _, _, _, a := decodeVariant(t, v).At(v.Width-1, 0).RGBA(); a == 0xFFFF || a == 0 {
			t.Errorf("%d×%d WebP: alpha %d in the half transparent corner", v.Width, v.Height, a)
		}
	}
}

func mediaAnimates(t *testing.T, p media.Processor) {
	for name, data := range map[string][]byte{"gif": FixtureGIF(16, 8, 3), "webp": FixtureWebPAnimation(16, 8, 3)} {
		out := mediaProcess(t, p, data)
		if out.Animated == nil {
			t.Errorf("%s: no moving variants", name)
			continue
		}
		// The still variants show the first frame.
		img := decodeVariant(t, out.Still.Large)
		if !near(img.At(2, 4), FixtureLeft) || !near(img.At(13, 4), FixtureRight) {
			t.Errorf("%s: the still variant is not the first frame: %v, %v", name, img.At(2, 4), img.At(13, 4))
		}
		for _, size := range []media.Size{media.SizeSmall, media.SizeMedium} {
			v := out.Animated.Variant(size)
			if v.ContentType != "image/webp" || v.Extension != "webp" || !webpAnimationFlag(v.Data) {
				t.Errorf("%s: moving %s is %s, not an animated WebP", name, size, v.ContentType)
				continue
			}
			decodeVariant(t, v)
			anim, err := animation.DecodeBytes(v.Data)
			if err != nil || len(anim.Frames) == 0 {
				t.Errorf("%s: moving %s: %v", name, size, err)
				continue
			}
			if err := anim.DecodeFrames(); err != nil {
				t.Errorf("%s: moving %s: %v", name, size, err)
			}
			dec, err := animation.NewAnimDecoder(anim)
			if err != nil {
				t.Fatalf("%s: moving %s: %v", name, size, err)
			}
			frames := 0
			for dec.HasNext() {
				if _, _, err := dec.NextFrame(); err != nil {
					t.Fatalf("%s: moving %s frame %d: %v", name, size, frames, err)
				}
				frames++
			}
			if frames != 3 || anim.LoopCount != 0 {
				t.Errorf("%s: moving %s has %d frames looping %d times, want 3 forever", name, size, frames, anim.LoopCount)
			}
		}
		large := out.Animated.Large
		g, err := gif.DecodeAll(bytes.NewReader(large.Data))
		if large.ContentType != "image/gif" || large.Extension != "gif" || err != nil || len(g.Image) != 3 || g.LoopCount != 0 {
			t.Errorf("%s: moving large is %s with %d frames (%v), want a GIF of 3 frames looping forever", name, large.ContentType, len(g.Image), err)
		}
	}
	// A resized GIF keeps its frames, delays, and loop count.
	out := mediaProcessFor(t, p, FixtureGIF(64, 32, 3), media.Sizes{}, media.Request{Purpose: media.PurposeAvatar})
	g, err := gif.DecodeAll(bytes.NewReader(out.Animated.Large.Data))
	if err != nil || len(g.Image) != 3 || g.Delay[1] != 10 || g.Config.Width != 32 || g.Config.Height != 32 {
		t.Errorf("resized gif: %v frames, %v, %v", len(g.Image), g, err)
	}
}

func mediaPreviews(t *testing.T, p media.Processor) {
	for name, data := range map[string][]byte{
		"jpeg": FixtureJPEG(400, 200, 6), "png with alpha": FixturePNG(400, 200, true), "gif": FixtureGIF(200, 100, 3),
		"webp animation": FixtureWebPAnimation(200, 100, 3), "webp lossy": FixtureWebPLossy,
	} {
		for _, req := range []media.Request{{}, {Purpose: media.PurposeAvatar, FocusX: -1}, {Purpose: media.PurposeEmoji}} {
			v, err := p.Preview(t.Context(), data, media.Limits{}, media.Sizes{}, req)
			if err != nil {
				t.Errorf("%s %s: Preview: %v", name, req.Purpose, err)
				continue
			}
			small := mediaProcessFor(t, p, data, media.Sizes{}, req).Still.Small
			if v.ContentType != "image/webp" || v.Extension != "webp" || webpAnimationFlag(v.Data) || v.Width != small.Width || v.Height != small.Height {
				t.Errorf("%s %s: Preview = %s %d×%d, want a still WebP of %d×%d", name, req.Purpose, v.ContentType, v.Width, v.Height, small.Width, small.Height)
				continue
			}
			decodeVariant(t, v)
			if bytes.Contains(v.Data, []byte(MetadataMarker)) {
				t.Errorf("%s: Preview keeps the metadata", name)
			}
		}
	}
	for name, data := range map[string][]byte{"svg": []byte("<svg/>"), "bomb": FixturePNGHeader(65535, 65535), "truncated": FixtureJPEG(40, 20, 1)[:200]} {
		if _, err := p.Preview(t.Context(), data, media.Limits{}, media.Sizes{}, media.Request{}); !media.Rejected(err) {
			t.Errorf("%s: Preview = %v, want a rejection", name, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := p.Preview(ctx, FixtureJPEG(40, 20, 1), media.Limits{}, media.Sizes{}, media.Request{}); err == nil || media.Rejected(err) {
		t.Errorf("Preview with a canceled context = %v, want the context's error", err)
	}
}

func mediaComputesBlurhash(t *testing.T, p media.Processor) {
	const chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz#$%*+,-.:;=?@[]^_{|}~"
	h := mediaProcess(t, p, FixtureJPEG(64, 48, 1)).Blurhash
	if len(h) < 6 {
		t.Fatalf("Blurhash = %q", h)
	}
	size := strings.IndexByte(chars, h[0])
	nx, ny := size%9+1, size/9+1
	if want := 4 + 2*nx*ny; len(h) != want {
		t.Errorf("Blurhash %q has %d characters, want %d for %d×%d components", h, len(h), want, nx, ny)
	}
	for _, c := range h {
		if !strings.ContainsRune(chars, c) {
			t.Errorf("Blurhash %q contains %q", h, c)
		}
	}
}

func mediaIsDeterministic(t *testing.T, p media.Processor) {
	for name, data := range map[string][]byte{
		"jpeg": FixtureJPEG(64, 48, 6), "png": FixturePNG(64, 48, true), "gif": FixtureGIF(16, 8, 2),
		"webp animation": FixtureWebPAnimation(16, 8, 2),
	} {
		req := media.Request{Purpose: media.PurposeAvatar, FocusX: 0.3}
		a, b := mediaProcessFor(t, p, data, media.Sizes{}, req), mediaProcessFor(t, p, data, media.Sizes{}, req)
		va, vb := mediaVariants(a), mediaVariants(b)
		for k, v := range va {
			if !bytes.Equal(v.Data, vb[k].Data) {
				t.Errorf("%s: two runs gave different %s variants", name, k)
			}
		}
		if len(va) != len(vb) || a.Blurhash != b.Blurhash {
			t.Errorf("%s: two runs gave different output", name)
		}
	}
}

func mediaStopsOnCancel(t *testing.T, p media.Processor) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := p.Process(ctx, FixtureJPEG(40, 20, 1), media.Limits{}, media.Sizes{}, media.Request{})
	if err == nil || media.Rejected(err) {
		t.Errorf("Process with a canceled context = %v, want the context's error", err)
	}
}
