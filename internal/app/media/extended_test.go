// SPDX-License-Identifier: AGPL-3.0-only

package media_test

import (
	"errors"
	"testing"

	"github.com/mycroblog/my/internal/app/media"
)

func heif(brands ...string) []byte {
	return heifFile(heifOptions{Brands: brands, Width: 640, Height: 480})
}

func apng(w, h, frames int) []byte {
	return apngFile(apngOptions{Width: w, Height: h, Frames: frames})
}

// Sniff tells AVIF and HEIC by the brands of the ftyp box, the major one first, and APNG by an acTL chunk before the
// first IDAT chunk; any other HEIF file is no image.
func TestSniffTellsTheFormatsOnlyAPluginDecodes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		data []byte
		want media.Format
	}{
		{"avif", heif("avif", "mif1", "miaf"), media.FormatAVIF},
		{"avis", heif("avis", "msf1"), media.FormatAVIF},
		{"avif as a compatible brand", heif("mif1", "miaf", "avif"), media.FormatAVIF},
		{"heic", heif("heic", "mif1"), media.FormatHEIC},
		{"heix", heif("heix"), media.FormatHEIC},
		{"heim", heif("heim"), media.FormatHEIC},
		{"heis", heif("heis"), media.FormatHEIC},
		{"hevc", heif("hevc", "msf1"), media.FormatHEIC},
		{"hevx", heif("hevx"), media.FormatHEIC},
		{"heic as a compatible brand", heif("mif1", "heic"), media.FormatHEIC},
		{"the major brand before the compatible ones", heif("heic", "avif"), media.FormatHEIC},
		{"avif before heic", heif("avif", "heic"), media.FormatAVIF},
		{"a HEIF file of another brand", heif("mif1", "miaf"), ""},
		{"an MP4", mp4File(), ""},
		{"apng", apng(64, 48, 3), media.FormatAPNG},
		{"an APNG of one frame", apng(64, 48, 1), media.FormatAPNG},
		{"acTL after IDAT", apngFile(apngOptions{Width: 64, Height: 48, Frames: 3, LateControl: true}), media.FormatPNG},
		{"png", pngImage(64, 48), media.FormatPNG},
		{"jpeg", jpegImage(64, 48), media.FormatJPEG},
		{"gif", gifImage(64, 48, 2), media.FormatGIF},
		{"webp", webpLossy, media.FormatWebP},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := media.Sniff(tc.data); got != tc.want {
				t.Errorf("Sniff = %q, want %q", got, tc.want)
			}
		})
	}
}

// SniffAV takes no AVIF or HEIC image for an MP4, and still takes any other ftyp box for one.
func TestSniffAVLeavesHEIFImages(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		data []byte
		want media.Format
	}{
		{"avif", heif("avif", "mif1"), ""},
		{"avif as a compatible brand", heif("mif1", "avif"), ""},
		{"heic", heif("heic", "mif1"), ""},
		{"hevc", heif("hevc"), ""},
		{"an MP4", mp4File(), media.FormatMP4},
		{"a HEIF file of another brand", heif("mif1", "miaf"), media.FormatMP4},
	} {
		if got := media.SniffAV(tc.data); got != tc.want {
			t.Errorf("%s: SniffAV = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// AVIF, HEIC, and APNG are the extended formats, each with its media type; the built-in ones are not extended.
func TestTheExtendedFormats(t *testing.T) {
	t.Parallel()
	for f, want := range map[media.Format]string{media.FormatAVIF: "image/avif", media.FormatHEIC: "image/heic", media.FormatAPNG: "image/apng"} {
		if !f.Extended() || f.ContentType() != want {
			t.Errorf("%s: Extended = %t, ContentType = %q, want true and %q", f, f.Extended(), f.ContentType(), want)
		}
	}
	for _, f := range []media.Format{media.FormatJPEG, media.FormatPNG, media.FormatGIF, media.FormatWebP, media.FormatMP4, ""} {
		if f.Extended() {
			t.Errorf("%q is extended", f)
		}
	}
}

// InspectExtended reads the dimensions of a HEIF file from the ispe of its primary item, or else from the track header of
// the image sequence with the most samples, whose samples are its frames; those of an APNG from IHDR, and its frames from
// acTL.
func TestInspectExtendedReadsTheHeaders(t *testing.T) {
	t.Parallel()
	sequence := func(compact bool) []byte {
		return heifFile(heifOptions{Brands: []string{"avis", "msf1"}, Tracks: []heifTrack{
			{Handler: "pict", Width: 32, Height: 32, Samples: 2},
			{Handler: "soun", Samples: 99},
			{Handler: "pict", Width: 320, Height: 240, Samples: 12, Compact: compact},
			{Handler: "auxv", Width: 16, Height: 16, Samples: 50},
		}})
	}
	for _, tc := range []struct {
		name string
		data []byte
		want media.Probe
	}{
		{"an AVIF image", heif("avif", "mif1"), media.Probe{Format: media.FormatAVIF, Width: 640, Height: 480, Frames: 1}},
		{"a HEIC image", heif("heic", "mif1"), media.Probe{Format: media.FormatHEIC, Width: 640, Height: 480, Frames: 1}},
		{"the primary item, not the thumbnail", heifFile(heifOptions{Brands: []string{"heic"}, Width: 4032, Height: 3024, Thumbnail: true}),
			media.Probe{Format: media.FormatHEIC, Width: 4032, Height: 3024, Frames: 1}},
		{"an image sequence", sequence(false), media.Probe{Format: media.FormatAVIF, Width: 320, Height: 240, Frames: 12}},
		{"an image sequence with compact sizes", sequence(true), media.Probe{Format: media.FormatAVIF, Width: 320, Height: 240, Frames: 12}},
		{"a video track", heifFile(heifOptions{Brands: []string{"hevc"}, Tracks: []heifTrack{{Handler: "vide", Width: 160, Height: 90, Samples: 5}}}),
			media.Probe{Format: media.FormatHEIC, Width: 160, Height: 90, Frames: 5}},
		{"an APNG", apng(64, 48, 3), media.Probe{Format: media.FormatAPNG, Width: 64, Height: 48, Frames: 3}},
		{"an APNG of one frame", apng(64, 48, 1), media.Probe{Format: media.FormatAPNG, Width: 64, Height: 48, Frames: 1}},
		{"an APNG of frames inside the canvas", apngFile(apngOptions{Width: 64, Height: 48, Frames: 3, Controls: []apngFrame{
			{Width: 64, Height: 48}, {Width: 10, Height: 10, X: 54, Y: 38}, {Width: 1, Height: 1},
		}}), media.Probe{Format: media.FormatAPNG, Width: 64, Height: 48, Frames: 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := media.InspectExtended(tc.data, media.Limits{})
			if err != nil || got != tc.want {
				t.Errorf("InspectExtended = %+v, %v, want %+v", got, err, tc.want)
			}
		})
	}
}

// InspectExtended checks the bytes, the sides, the pixels, and the frames against the limits, zero ones taking their
// defaults, and the frames times the canvas against CompositedPixelsFactor times MaxPixels.
func TestInspectExtendedChecksTheLimits(t *testing.T) {
	t.Parallel()
	big := func(w, h int) []byte {
		return heifFile(heifOptions{Brands: []string{"avif"}, Width: w, Height: h})
	}
	frames := func(n int) []byte {
		return heifFile(heifOptions{Brands: []string{"avis"}, Tracks: []heifTrack{{Handler: "pict", Width: 64, Height: 48, Samples: n}}})
	}
	// A canvas of 100 × 100 whose frames after the first are one pixel each: n frames composite n × 10,000 pixels.
	composited := func(n int) []byte {
		controls := []apngFrame{{Width: 100, Height: 100}}
		for range n - 1 {
			controls = append(controls, apngFrame{Width: 1, Height: 1})
		}
		return apngFile(apngOptions{Width: 100, Height: 100, Frames: n, Controls: controls})
	}
	avif := big(640, 480)
	for _, tc := range []struct {
		name   string
		data   []byte
		limits media.Limits
		want   error
	}{
		{"within the limits", avif, media.Limits{MaxBytes: int64(len(avif)), MaxSide: 640, MaxPixels: 640 * 480}, nil},
		{"longer than MaxBytes", avif, media.Limits{MaxBytes: int64(len(avif)) - 1}, media.ErrTooLarge},
		{"longer than the default", append(big(64, 48), make([]byte, media.DefaultMaxBytes)...), media.Limits{}, media.ErrTooLarge},
		{"wider than MaxSide", avif, media.Limits{MaxSide: 639}, media.ErrDimensions},
		{"wider than the default", big(media.DefaultMaxSide+1, 10), media.Limits{}, media.ErrDimensions},
		{"taller than the default", big(10, media.DefaultMaxSide+1), media.Limits{}, media.ErrDimensions},
		{"more pixels than the default", big(8000, 8000), media.Limits{}, media.ErrDimensions},
		{"more pixels than MaxPixels", avif, media.Limits{MaxPixels: 640*480 - 1}, media.ErrDimensions},
		{"a bomb", big(1<<30, 1<<30), media.Limits{}, media.ErrDimensions},
		{"frames within MaxFrames", frames(3), media.Limits{MaxFrames: 3}, nil},
		{"more frames than MaxFrames", frames(4), media.Limits{MaxFrames: 3}, media.ErrDimensions},
		{"more frames than the default", frames(media.DefaultMaxFrames + 1), media.Limits{}, media.ErrDimensions},
		{"more APNG frames than MaxFrames", apng(16, 16, 4), media.Limits{MaxFrames: 3}, media.ErrDimensions},
		{"a composited canvas at the bound", composited(8), media.Limits{MaxPixels: 20_000}, nil},
		{"a composited canvas over the bound", composited(9), media.Limits{MaxPixels: 20_000}, media.ErrDimensions},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := media.InspectExtended(tc.data, tc.limits)
			if tc.want == nil && err != nil || tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("InspectExtended = %v, want %v", err, tc.want)
			}
			if tc.want != nil && !media.Rejected(err) {
				t.Errorf("InspectExtended = %v, not a rejection", err)
			}
		})
	}
}

// InspectExtended refuses every format but AVIF, HEIC, and APNG, the built-in ones too.
func TestInspectExtendedRefusesOtherFormats(t *testing.T) {
	t.Parallel()
	for name, data := range map[string][]byte{
		"jpeg":             jpegImage(64, 48),
		"png":              pngImage(64, 48),
		"acTL after IDAT":  apngFile(apngOptions{Width: 64, Height: 48, Frames: 3, LateControl: true}),
		"gif":              gifImage(64, 48, 2),
		"webp":             webpAnimated,
		"mp4":              mp4File(),
		"another HEIF":     heif("mif1", "miaf"),
		"svg":              []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
		"jpeg xl":          []byte("\xFF\x0A\xFA\x7F\x01\x90\x08"),
		"empty":            nil,
		"a PNG signature":  []byte("\x89PNG\r\n\x1A\n"),
		"a bare ftyp head": []byte("\x00\x00\x00\x08ftyp"),
	} {
		if _, err := media.InspectExtended(data, media.Limits{}); !errors.Is(err, media.ErrUnsupportedFormat) {
			t.Errorf("%s: InspectExtended = %v, want ErrUnsupportedFormat", name, err)
		}
	}
}

// A file whose headers do not hold together is ErrCorrupt: a HEIF file without dimensions or cut inside a box, and an
// APNG whose fcTL chunks are not as many as acTL says or leave the canvas.
func TestInspectExtendedRefusesMalformedHeaders(t *testing.T) {
	t.Parallel()
	avif := heif("avif", "mif1")
	ftypSize := int(avif[3])
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"a HEIF file without meta or moov", heifFile(heifOptions{Brands: []string{"avif"}})},
		{"a HEIF file cut inside its meta box", avif[:ftypSize+20]},
		{"fewer fcTL chunks than frames", apngFile(apngOptions{Width: 64, Height: 48, Frames: 3, Controls: []apngFrame{
			{Width: 64, Height: 48}, {Width: 64, Height: 48},
		}})},
		{"more fcTL chunks than frames", apngFile(apngOptions{Width: 64, Height: 48, Frames: 2, Controls: []apngFrame{
			{Width: 64, Height: 48}, {Width: 64, Height: 48}, {Width: 64, Height: 48},
		}})},
		{"a frame past the right edge", apngFile(apngOptions{Width: 64, Height: 48, Frames: 2, Controls: []apngFrame{
			{Width: 64, Height: 48}, {Width: 10, Height: 10, X: 60},
		}})},
		{"a frame past the bottom edge", apngFile(apngOptions{Width: 64, Height: 48, Frames: 2, Controls: []apngFrame{
			{Width: 64, Height: 48}, {Width: 10, Height: 10, Y: 39},
		}})},
		{"a frame larger than the canvas", apngFile(apngOptions{Width: 64, Height: 48, Frames: 2, Controls: []apngFrame{
			{Width: 64, Height: 48}, {Width: 65, Height: 48},
		}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := media.InspectExtended(tc.data, media.Limits{}); !errors.Is(err, media.ErrCorrupt) {
				t.Errorf("InspectExtended = %v, want ErrCorrupt", err)
			}
		})
	}
}

// extendedSeeds are the corpus of the fuzz targets: every shape of the fixtures and some cut ones.
func extendedSeeds() [][]byte {
	avif := heif("avif", "mif1")
	seq := heifFile(heifOptions{Brands: []string{"avis"}, Width: 64, Height: 48, Thumbnail: true, Tracks: []heifTrack{
		{Handler: "pict", Width: 64, Height: 48, Samples: 3}, {Handler: "pict", Width: 8, Height: 8, Samples: 2, Compact: true},
	}})
	a := apng(16, 12, 3)
	return [][]byte{
		avif, avif[:len(avif)/2], heif("heic", "mif1"), heif("mif1", "hevc"), seq, seq[:len(seq)-30],
		a, a[:len(a)/2], apngFile(apngOptions{Width: 16, Height: 12, Frames: 2, Controls: []apngFrame{{Width: 16, Height: 12}, {Width: 4, Height: 4, X: 14}}}),
		pngImage(4, 3), jpegImage(4, 3), gifImage(4, 3, 2), webpAnimated,
		mp4File(), nil,
	}
}

// Sniff never panics, names a format or none, and a file it takes for AVIF or HEIC is no MP4 to SniffAV. (SniffAV may
// still take it for another container whose leading bytes it also has, such as the frame sync of an MP3.)
func FuzzSniff(f *testing.F) {
	for _, seed := range extendedSeeds() {
		f.Add(seed)
	}
	known := map[media.Format]bool{
		"": true, media.FormatJPEG: true, media.FormatPNG: true, media.FormatGIF: true, media.FormatWebP: true,
		media.FormatAVIF: true, media.FormatHEIC: true, media.FormatAPNG: true,
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		got := media.Sniff(data)
		if !known[got] {
			t.Fatalf("Sniff = %q", got)
		}
		if (got == media.FormatAVIF || got == media.FormatHEIC) && media.SniffAV(data) == media.FormatMP4 {
			t.Fatalf("SniffAV of a %s = %q", got, media.SniffAV(data))
		}
	})
}

// InspectExtended never panics, fails only with the errors of the port, and a probe it returns is of the format Sniff
// tells, within the limits.
func FuzzInspectExtended(f *testing.F) {
	for _, seed := range extendedSeeds() {
		f.Add(seed)
	}
	limits := media.Limits{MaxBytes: 1 << 20, MaxSide: 4096, MaxPixels: 1 << 22, MaxFrames: 100}
	f.Fuzz(func(t *testing.T, data []byte) {
		probe, err := media.InspectExtended(data, limits)
		if err != nil {
			for _, want := range []error{media.ErrTooLarge, media.ErrUnsupportedFormat, media.ErrDimensions, media.ErrCorrupt} {
				if errors.Is(err, want) {
					return
				}
			}
			t.Fatalf("InspectExtended = %v, none of the port's errors", err)
		}
		if !probe.Format.Extended() || probe.Format != media.Sniff(data) {
			t.Fatalf("probe of %s, Sniff %s", probe.Format, media.Sniff(data))
		}
		if probe.Width < 1 || probe.Height < 1 || probe.Width > limits.MaxSide || probe.Height > limits.MaxSide ||
			int64(probe.Width)*int64(probe.Height) > limits.MaxPixels || probe.Frames < 1 || probe.Frames > limits.MaxFrames {
			t.Fatalf("probe %+v outside %+v", probe, limits)
		}
	})
}
