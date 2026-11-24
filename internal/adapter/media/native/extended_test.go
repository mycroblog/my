// SPDX-License-Identifier: AGPL-3.0-only

package native_test

import (
	"errors"
	"testing"

	"github.com/mycroblog/my/internal/adapter/media/native"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/pkg/testkit"
)

// With media_image = native, AVIF and HEIC are refused as before, whatever their brands, and an APNG is the still PNG of
// its default image.
func TestTheBuiltInRefusesAVIFAndHEICAndStillsAPNG(t *testing.T) {
	t.Parallel()
	p := native.New(native.Options{})
	for name, data := range map[string][]byte{
		"avif":         testkit.HEIFFile(testkit.HEIFOptions{Brands: []string{"avif", "mif1"}, Width: 64, Height: 48}),
		"avis":         testkit.HEIFFile(testkit.HEIFOptions{Brands: []string{"avis"}, Tracks: []testkit.HEIFTrack{{Handler: "pict", Width: 64, Height: 48, Samples: 3}}}),
		"heic":         testkit.HEIFFile(testkit.HEIFOptions{Brands: []string{"heic", "mif1"}, Width: 64, Height: 48}),
		"heic as mif1": testkit.HEIFFile(testkit.HEIFOptions{Brands: []string{"mif1", "heic"}, Width: 64, Height: 48}),
	} {
		if _, err := p.Inspect(t.Context(), data, media.Limits{}); !errors.Is(err, media.ErrUnsupportedFormat) {
			t.Errorf("Inspect(%s) = %v, want ErrUnsupportedFormat", name, err)
		}
		if _, err := p.Process(t.Context(), data, media.Limits{}, media.Sizes{}, media.Request{}); !errors.Is(err, media.ErrUnsupportedFormat) {
			t.Errorf("Process(%s) = %v, want ErrUnsupportedFormat", name, err)
		}
	}

	apng := testkit.APNGFile(testkit.APNGOptions{Width: 64, Height: 48, Frames: 3})
	probe, err := p.Inspect(t.Context(), apng, media.Limits{})
	if want := (media.Probe{Format: media.FormatPNG, Width: 64, Height: 48, Frames: 1}); err != nil || probe != want {
		t.Errorf("Inspect(apng) = %+v, %v, want %+v", probe, err, want)
	}
	// Its frames count for nothing against the limits of an animation.
	if _, err := p.Inspect(t.Context(), apng, media.Limits{MaxFrames: 1}); err != nil {
		t.Errorf("Inspect(apng) with one frame at most = %v", err)
	}
	out, err := p.Process(t.Context(), apng, media.Limits{}, media.Sizes{}, media.Request{})
	if err != nil {
		t.Fatalf("Process(apng) = %v", err)
	}
	if out.Animated != nil || out.Width != 64 || out.Height != 48 {
		t.Errorf("Process(apng) = %d×%d, moving %t", out.Width, out.Height, out.Animated != nil)
	}
}
