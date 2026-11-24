// SPDX-License-Identifier: AGPL-3.0-only

package native

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"

	"github.com/deepteams/webp"
	"golang.org/x/image/draw"

	"github.com/mycroblog/my/internal/app/media"
)

// Encoder settings (ADR-0005 §5).
const (
	// jpegQuality is the quality of the large still variant of an opaque image.
	jpegQuality = 85
	// webpQuality is the quality of the small and medium variants, lossy WebP.
	webpQuality = 80
)

// target is where one variant comes from and how large it is: the rectangle of the source to
// keep and the size to scale it to.
type target struct {
	src           image.Rectangle
	width, height int
}

// plan returns the target of box b for a w × h source with the focal point (fx, fy).
func plan(w, h int, b media.Box, fx, fy float64) target {
	full := image.Rect(0, 0, w, h)
	switch {
	case b.MaxPixels > 0:
		if int64(w)*int64(h) <= b.MaxPixels {
			return target{full, w, h}
		}
		s := math.Sqrt(float64(b.MaxPixels) / float64(int64(w)*int64(h)))
		return target{full, max(1, int(math.Floor(float64(w)*s))), max(1, int(math.Floor(float64(h)*s)))}
	case b.Width <= 0 || b.Height <= 0:
		return target{full, w, h}
	case b.Crop:
		src := cropRect(w, h, b.Width, b.Height, fx, fy)
		dw, dh := within(src.Dx(), src.Dy(), b)
		return target{src, dw, dh}
	}
	dw, dh := within(w, h, b)
	return target{full, dw, dh}
}

// within returns w × h scaled down to fit within b.Width × b.Height, keeping the aspect
// ratio; a size that fits is kept.
func within(w, h int, b media.Box) (int, int) {
	if w <= b.Width && h <= b.Height {
		return w, h
	}
	s := min(float64(b.Width)/float64(w), float64(b.Height)/float64(h))
	return min(b.Width, max(1, int(math.Round(float64(w)*s)))), min(b.Height, max(1, int(math.Round(float64(h)*s))))
}

// cropRect returns the largest rectangle of a w × h image with the aspect ratio bw:bh,
// centered on the focal point (fx, fy) as far as the image allows.
func cropRect(w, h, bw, bh int, fx, fy float64) image.Rectangle {
	cw, ch := w, h
	if int64(w)*int64(bh) > int64(h)*int64(bw) {
		cw = max(1, min(w, int(math.Round(float64(h)*float64(bw)/float64(bh)))))
	} else {
		ch = max(1, min(h, int(math.Round(float64(w)*float64(bh)/float64(bw)))))
	}
	if math.IsNaN(fx) || math.IsNaN(fy) {
		fx, fy = 0, 0
	}
	fx, fy = max(-1, min(1, fx)), max(-1, min(1, fy))
	cx := float64(w) / 2 * (1 + fx)
	cy := float64(h) / 2 * (1 - fy) // y grows downwards in pixels, upwards in focal points
	x0 := max(0, min(w-cw, int(math.Round(cx-float64(cw)/2))))
	y0 := max(0, min(h-ch, int(math.Round(cy-float64(ch)/2))))
	return image.Rect(x0, y0, x0+cw, y0+ch)
}

// targets returns the targets of the three boxes of a purpose, smallest first.
func targets(w, h int, boxes media.Boxes, req media.Request) [3]target {
	var out [3]target
	for i, size := range media.AllSizes {
		out[i] = plan(w, h, boxes.Box(size), req.FocusX, req.FocusY)
	}
	return out
}

// render returns the target's part of img scaled to the target's size with kernel; an
// unscaled, uncropped target is img itself.
func render(img *image.NRGBA, t target, kernel *draw.Kernel) *image.NRGBA {
	src := t.src.Add(img.Rect.Min)
	if t.width == src.Dx() && t.height == src.Dy() {
		if src == img.Rect {
			return img
		}
		dst := image.NewNRGBA(image.Rect(0, 0, t.width, t.height))
		draw.Draw(dst, dst.Rect, img, src.Min, draw.Src)
		return dst
	}
	dst := image.NewNRGBA(image.Rect(0, 0, t.width, t.height))
	scale(dst, img, src, kernel)
	return dst
}

// MaxScaleBuffer bounds the temporary buffer of one call of a kernel's scaler, in bytes. The
// scaler of golang.org/x/image/draw holds 32 bytes for every destination column times source
// row, which for the large variant of a photo of 12 megapixels is over 300 MiB; scale keeps
// each call under this bound by scaling one axis at a time, in strips of the other.
const MaxScaleBuffer = 4 << 20

// scaleBufferBytes is the size of the scaler's buffer per destination column and source row.
const scaleBufferBytes = 32

// scale scales the part sr of src to the whole of dst with kernel: along one axis into an
// intermediate image, then along the other, choosing the order that keeps the intermediate
// image smaller. Each pass leaves the other axis as it is, so it splits into strips across that
// axis whose scaler's buffer stays within MaxScaleBuffer.
func scale(dst, src *image.NRGBA, sr image.Rectangle, kernel *draw.Kernel) {
	dw, dh, sw, sh := dst.Rect.Dx(), dst.Rect.Dy(), sr.Dx(), sr.Dy()
	switch {
	case dh == sh:
		scaleAxis(dst, src, sr, kernel, true)
	case dw == sw:
		scaleAxis(dst, src, sr, kernel, false)
	case int64(dw)*int64(sh) <= int64(sw)*int64(dh):
		mid := image.NewNRGBA(image.Rect(0, 0, dw, sh))
		scaleAxis(mid, src, sr, kernel, true)
		scaleAxis(dst, mid, mid.Rect, kernel, false)
	default:
		mid := image.NewNRGBA(image.Rect(0, 0, sw, dh))
		scaleAxis(mid, src, sr, kernel, false)
		scaleAxis(dst, mid, mid.Rect, kernel, true)
	}
}

// scaleAxis scales the part sr of src to the whole of dst along one axis, horizontally or
// vertically, the other being the same size in both. It scales strips of rows (or columns)
// one at a time: a kernel at a scale of 1 weighs only the pixel itself, so a strip scales as it
// would within the whole image.
func scaleAxis(dst, src *image.NRGBA, sr image.Rectangle, kernel *draw.Kernel, horizontal bool) {
	dw, dh, sh := dst.Rect.Dx(), dst.Rect.Dy(), sr.Dy()
	// A strip is n rows of dw columns (horizontally) or n columns of sh rows (vertically).
	across, length := dh, dw
	if !horizontal {
		across, length = dw, sh
	}
	n := max(1, min(across, MaxScaleBuffer/(scaleBufferBytes*length)))
	var scaler draw.Scaler
	for at := 0; at < across; at += n {
		m := min(n, across-at)
		var dr, s image.Rectangle
		if horizontal {
			dr = image.Rect(0, at, dw, at+m)
			s = image.Rect(sr.Min.X, sr.Min.Y+at, sr.Max.X, sr.Min.Y+at+m)
		} else {
			dr = image.Rect(at, 0, at+m, dh)
			s = image.Rect(sr.Min.X+at, sr.Min.Y, sr.Min.X+at+m, sr.Max.Y)
		}
		if scaler == nil || m != n {
			scaler = kernel.NewScaler(dr.Dx(), dr.Dy(), s.Dx(), s.Dy())
		}
		scaler.Scale(dst, dr.Add(dst.Rect.Min), src, s, draw.Src, nil)
	}
}

// renderAll renders the three targets of img, largest first so that a smaller one with the
// same source rectangle scales the larger result instead of the whole source, and returns them
// smallest first.
func renderAll(img *image.NRGBA, ts [3]target, kernel *draw.Kernel) [3]*image.NRGBA {
	var out [3]*image.NRGBA
	var larger *image.NRGBA
	var largerTarget target
	for i := len(ts) - 1; i >= 0; i-- {
		t := ts[i]
		if larger != nil && largerTarget.src == t.src && largerTarget.width >= t.width && largerTarget.height >= t.height {
			out[i] = render(larger, target{larger.Rect, t.width, t.height}, kernel)
		} else {
			out[i] = render(img, t, kernel)
		}
		larger, largerTarget = out[i], t
	}
	return out
}

// stillVariants encodes the still variants: the large one as JPEG when opaque and PNG
// otherwise, or always as PNG with pngLarge (custom emoji, which no server takes as JPEG), the
// others as lossy WebP.
func stillVariants(imgs [3]*image.NRGBA, pngLarge bool) (media.Variants, error) {
	var out media.Variants
	var err error
	if out.Small, err = encodeWebP(imgs[0]); err != nil {
		return media.Variants{}, err
	}
	if out.Medium, err = encodeWebP(imgs[1]); err != nil {
		return media.Variants{}, err
	}
	if out.Large, err = encodeLarge(imgs[2], pngLarge); err != nil {
		return media.Variants{}, err
	}
	return out, nil
}

// encodeLarge writes img as PNG when it has transparency or with forcePNG, and as JPEG otherwise.
// Neither encoder writes metadata.
func encodeLarge(img *image.NRGBA, forcePNG bool) (media.Variant, error) {
	var buf bytes.Buffer
	v := media.Variant{Width: img.Rect.Dx(), Height: img.Rect.Dy()}
	if forcePNG || !img.Opaque() {
		if err := png.Encode(&buf, img); err != nil {
			return media.Variant{}, fmt.Errorf("media/native: encode png: %w", err)
		}
		v.ContentType, v.Extension = "image/png", "png"
	} else {
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return media.Variant{}, fmt.Errorf("media/native: encode jpeg: %w", err)
		}
		v.ContentType, v.Extension = "image/jpeg", "jpg"
	}
	v.Data = buf.Bytes()
	return v, nil
}

// webpOptions returns the options of the lossy WebP encoder. They start from the encoder's
// defaults: a zero EncoderOptions turns the alpha quality down to 0.
func webpOptions() *webp.EncoderOptions {
	o := webp.DefaultOptions()
	o.Quality = webpQuality
	return o
}

// encodeWebP writes img as lossy WebP, with an alpha channel when it has transparency. The
// encoder writes no metadata unless asked to.
func encodeWebP(img *image.NRGBA) (media.Variant, error) {
	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, webpOptions()); err != nil {
		return media.Variant{}, fmt.Errorf("media/native: encode webp: %w", err)
	}
	return media.Variant{Data: buf.Bytes(), ContentType: "image/webp", Extension: "webp", Width: img.Rect.Dx(), Height: img.Rect.Dy()}, nil
}
