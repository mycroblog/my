// SPDX-License-Identifier: AGPL-3.0-only

package native

import (
	"image"
	"strings"
	"testing"

	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/mediatest"
)

func TestMediaProcessorContract(t *testing.T) {
	mediatest.MediaProcessorContract(t, New(Options{Concurrency: 2}))
}

func TestBlurhashOfASolidColor(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 24))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255 // opaque black
	}
	// No AC energy: every AC component quantizes to 9,9,9, written "fQ".
	if got, want := blurhash(img, 4, 3), "L00000"+strings.Repeat("fQ", 11); got != want {
		t.Errorf("blurhash(black, 4, 3) = %q, want %q", got, want)
	}
	if got, want := blurhash(img, 4, 4), "U00000"+strings.Repeat("fQ", 15); got != want {
		t.Errorf("blurhash(black, 4, 4) = %q, want %q", got, want)
	}
	white := image.NewNRGBA(img.Rect)
	for i := range white.Pix {
		white.Pix[i] = 255
	}
	if got := blurhash(white, 4, 3); got[2:6] != "TSUA" { // 0xFFFFFF in base 83
		t.Errorf("blurhash(white) DC = %q, want TSUA", got[2:6])
	}
}

func TestExifOrientationIgnoresMalformedSegments(t *testing.T) {
	jpeg := testkit.FixtureJPEG(8, 8, 6)
	if got := exifOrientation(jpeg); got != 6 {
		t.Fatalf("exifOrientation = %d, want 6", got)
	}
	for n := range len(jpeg) {
		if o := exifOrientation(jpeg[:n]); o < 1 || o > 8 {
			t.Fatalf("exifOrientation(truncated to %d) = %d", n, o)
		}
	}
	if got := exifOrientation([]byte("\xFF\xD8\xFF\xE1\x00\x02")); got != 1 {
		t.Errorf("empty APP1: %d", got)
	}
}

func TestOrientRoundTrips(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for i := range src.Pix {
		src.Pix[i] = uint8(i)
	}
	// Orientations 6 and 8 undo each other, as do 5 with itself and 7 with itself.
	for _, pair := range [][2]int{{6, 8}, {8, 6}, {5, 5}, {7, 7}, {2, 2}, {3, 3}, {4, 4}} {
		got := orient(orient(src, pair[0]), pair[1])
		if got.Rect != src.Rect || string(got.Pix) != string(src.Pix) {
			t.Errorf("orient %d then %d changed the image", pair[0], pair[1])
		}
	}
	if got := orient(src, 6); got.Rect.Dx() != 2 || got.Rect.Dy() != 3 || got.NRGBAAt(1, 0) != src.NRGBAAt(0, 0) {
		t.Errorf("orient 6: top-left pixel did not move to the top-right")
	}
}

func TestGIFFramesRejectsMalformedStructure(t *testing.T) {
	data := testkit.FixtureGIF(4, 4, 2)
	if frames, _, err := gifFrames(data, 10); err != nil || frames != 2 {
		t.Fatalf("gifFrames = %d, %v", frames, err)
	}
	for n := range len(data) - 1 {
		if _, _, err := gifFrames(data[:n], 10); err == nil {
			t.Fatalf("gifFrames accepted the file truncated to %d bytes", n)
		}
	}
}
