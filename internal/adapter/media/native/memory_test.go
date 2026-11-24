// SPDX-License-Identifier: AGPL-3.0-only

package native_test

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"runtime"
	"sync"
	"testing"

	"golang.org/x/image/webp"

	"github.com/mycroblog/my/internal/adapter/media/native"
	"github.com/mycroblog/my/internal/app/media"
)

// quadrantColors are the colors of the quadrants of quadrantJPEG: top left, top right, bottom
// left, bottom right.
var quadrantColors = [4]color.NRGBA{
	{R: 220, G: 20, B: 20, A: 255},
	{R: 20, G: 20, B: 220, A: 255},
	{R: 20, G: 200, B: 20, A: 255},
	{R: 230, G: 230, B: 230, A: 255},
}

func quadrantOf(x, y, w, h int) int {
	q := 0
	if x >= w/2 {
		q++
	}
	if y >= h/2 {
		q += 2
	}
	return q
}

// quadrantJPEG returns a w × h JPEG of quality 90, as a camera writes, whose quadrants have the
// colors of quadrantColors.
func quadrantJPEG(tb testing.TB, w, h int) []byte {
	tb.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := quadrantColors[quadrantOf(x, y, w, h)]
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

var photo = sync.OnceValue(func() []byte {
	img := image.NewYCbCr(image.Rect(0, 0, 4032, 3024), image.YCbCrSubsampleRatio420)
	for i := range img.Y {
		img.Y[i] = uint8(i * 7)
	}
	for i := range img.Cb {
		img.Cb[i], img.Cr[i] = uint8(i*3), uint8(i*5)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		panic(err)
	}
	return buf.Bytes()
})

// processAttachment processes data as an attachment with the default limits and sizes on a
// processor of one file at a time.
func processAttachment(tb testing.TB, data []byte) media.Output {
	tb.Helper()
	out, err := native.New(native.Options{Concurrency: 1}).Process(context.Background(), data, media.Limits{}, media.Sizes{}, media.Request{Purpose: media.PurposeAttachment})
	if err != nil {
		tb.Fatalf("Process: %v", err)
	}
	return out
}

// allocated returns the bytes that run allocates, from the runtime's count of every allocation,
// which does not depend on when the collector runs.
func allocated(run func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	run()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// TestProcessOfAPhotoBoundsItsAllocation guards the bound on the scaler's temporary buffer
// (native.MaxScaleBuffer, #3440): processing a photo of 12 megapixels, the size phones take,
// allocated about 611 MB when the scaler held 32 bytes for every destination column times source
// row, and about 256 MB with the buffer bounded. Total allocation, unlike a sampled heap peak,
// does not depend on the collector's timing. It takes a few seconds, so -short skips it.
func TestProcessOfAPhotoBoundsItsAllocation(t *testing.T) {
	if testing.Short() {
		t.Skip("processes a 12-megapixel photo")
	}
	data := photo()
	const bound = 400 << 20
	var out media.Output
	got := allocated(func() { out = processAttachment(t, data) })
	t.Logf("Process(4032×3024 JPEG) allocated %d MiB", got>>20)
	if got > bound {
		t.Errorf("Process(4032×3024 JPEG) allocated %d MiB, want at most %d MiB", got>>20, bound>>20)
	}
	if out.Width != 4032 || out.Height != 3024 || len(out.Still.Large.Data) == 0 {
		t.Errorf("Process(4032×3024 JPEG) = %d×%d with a large variant of %d bytes", out.Width, out.Height, len(out.Still.Large.Data))
	}
}

func BenchmarkProcessPhoto(b *testing.B) {
	data := photo()
	p := native.New(native.Options{Concurrency: 1})
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		if _, err := p.Process(b.Context(), data, media.Limits{}, media.Sizes{}, media.Request{Purpose: media.PurposeAttachment}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPreviewPhoto(b *testing.B) {
	data := photo()
	p := native.New(native.Options{Concurrency: 1})
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		if _, err := p.Preview(b.Context(), data, media.Limits{}, media.Sizes{}, media.Request{Purpose: media.PurposeAttachment}); err != nil {
			b.Fatal(err)
		}
	}
}

// TestProcessKeepsTheVariantsOfAPhoto checks that scaling in strips under the bound gives the
// variants the dimensions they had when the scaler scaled the whole image at once, and the
// picture in the right place: each quadrant keeps its color away from the edges between them,
// where a strip that was misplaced or left out would show.
func TestProcessKeepsTheVariantsOfAPhoto(t *testing.T) {
	type dims struct{ w, h int }
	for _, tc := range []struct {
		name                 string
		w, h                 int
		small, medium, large dims
		long                 bool
	}{
		{name: "landscape", w: 4032, h: 3024, small: dims{554, 415}, medium: dims{1662, 1247}, large: dims{3325, 2494}, long: true},
		{name: "portrait", w: 3024, h: 4032, small: dims{415, 554}, medium: dims{1247, 1662}, large: dims{2494, 3325}, long: true},
		{name: "panorama", w: 7680, h: 960, small: dims{1357, 169}, medium: dims{4072, 509}, large: dims{7680, 960}, long: true},
		{name: "small", w: 300, h: 200, small: dims{300, 200}, medium: dims{300, 200}, large: dims{300, 200}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.long && testing.Short() {
				t.Skip("processes a photo of megapixels")
			}
			out := processAttachment(t, quadrantJPEG(t, tc.w, tc.h))
			if out.Width != tc.w || out.Height != tc.h {
				t.Errorf("Output = %d×%d, want %d×%d", out.Width, out.Height, tc.w, tc.h)
			}
			for _, v := range []struct {
				size    string
				variant media.Variant
				want    dims
			}{
				{"small", out.Still.Small, tc.small},
				{"medium", out.Still.Medium, tc.medium},
				{"large", out.Still.Large, tc.large},
			} {
				if got := (dims{v.variant.Width, v.variant.Height}); got != v.want {
					t.Errorf("%s = %d×%d, want %d×%d", v.size, got.w, got.h, v.want.w, v.want.h)
				}
				checkQuadrants(t, v.size, v.variant)
			}
		})
	}
}

// checkQuadrants decodes v and checks that it has its stated dimensions and that a grid of points
// away from the edges between the quadrants has the color of its quadrant.
func checkQuadrants(t *testing.T, size string, v media.Variant) {
	t.Helper()
	var decode func(r *bytes.Reader) (image.Image, error)
	switch v.ContentType {
	case "image/jpeg":
		decode = func(r *bytes.Reader) (image.Image, error) { return jpeg.Decode(r) }
	case "image/png":
		decode = func(r *bytes.Reader) (image.Image, error) { return png.Decode(r) }
	case "image/webp":
		decode = func(r *bytes.Reader) (image.Image, error) { return webp.Decode(r) }
	default:
		t.Errorf("%s is %s", size, v.ContentType)
		return
	}
	img, err := decode(bytes.NewReader(v.Data))
	if err != nil {
		t.Errorf("%s does not decode: %v", size, err)
		return
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if w != v.Width || h != v.Height {
		t.Errorf("%s decodes to %d×%d, stated %d×%d", size, w, h, v.Width, v.Height)
		return
	}
	const steps, margin, tolerance = 24, 0.04, 40
	var wrong []string
	for i := range steps {
		for j := range steps {
			fx, fy := (float64(i)+0.5)/steps, (float64(j)+0.5)/steps
			if abs(fx-0.5) < margin || abs(fy-0.5) < margin {
				continue
			}
			x, y := int(fx*float64(w)), int(fy*float64(h))
			want := quadrantColors[quadrantOf(x, y, w, h)]
			got := color.NRGBAModel.Convert(img.At(img.Bounds().Min.X+x, img.Bounds().Min.Y+y)).(color.NRGBA)
			if far(got.R, want.R, tolerance) || far(got.G, want.G, tolerance) || far(got.B, want.B, tolerance) {
				wrong = append(wrong, fmt.Sprintf("(%d,%d)=%v want %v", x, y, got, want))
			}
		}
	}
	if len(wrong) > 0 {
		t.Errorf("%s has %d points of the wrong color, first %s", size, len(wrong), wrong[0])
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func far(a, b uint8, tolerance int) bool {
	d := int(a) - int(b)
	return d > tolerance || d < -tolerance
}
