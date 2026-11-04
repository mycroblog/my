// SPDX-License-Identifier: AGPL-3.0-only

package native

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"time"

	"github.com/deepteams/webp/animation"
	"golang.org/x/image/draw"

	"github.com/mycroblog/my/internal/app/media"
)

// Frame delays of the animated WebP variants: browsers show the GIF frames shorter than
// minFrameDelay for defaultFrameDelay, and the WebP variants keep the timing the GIF had.
const (
	minFrameDelay     = 20 * time.Millisecond
	defaultFrameDelay = 100 * time.Millisecond
)

// frame is one composited frame of an animation on its canvas. Canvas belongs to the source
// and changes after the callback returns. Palette is the frame's GIF palette, nil for WebP.
type frame struct {
	canvas  *image.NRGBA
	delay   time.Duration
	palette color.Palette
}

// animationSource is a decoded animation that yields its composited frames in order.
type animationSource struct {
	width, height int
	// loops is the WebP loop count: 0 for forever, n to play n times.
	loops int
	each  func(fn func(frame) error) error
}

// gifSource composites the frames of a GIF on its logical screen, following their disposal.
func gifSource(g *gif.GIF) animationSource {
	w, h := gifScreen(g)
	loops := 0
	switch {
	case g.LoopCount < 0:
		loops = 1
	case g.LoopCount > 0:
		loops = g.LoopCount + 1
	}
	return animationSource{width: w, height: h, loops: loops, each: func(fn func(frame) error) error {
		canvas := image.NewNRGBA(image.Rect(0, 0, w, h))
		var saved []byte
		for i, img := range g.Image {
			disposal := byte(0)
			if i < len(g.Disposal) {
				disposal = g.Disposal[i]
			}
			if disposal == gif.DisposalPrevious {
				saved = append(saved[:0], canvas.Pix...)
			}
			r := img.Bounds().Intersect(canvas.Rect)
			draw.Draw(canvas, r, img, r.Min, draw.Over)
			delay := defaultFrameDelay
			if i < len(g.Delay) {
				delay = time.Duration(g.Delay[i]) * 10 * time.Millisecond
			}
			if err := fn(frame{canvas: canvas, delay: delay, palette: img.Palette}); err != nil {
				return err
			}
			switch disposal {
			case gif.DisposalBackground:
				draw.Draw(canvas, r, image.Transparent, image.Point{}, draw.Src)
			case gif.DisposalPrevious:
				copy(canvas.Pix, saved)
			}
		}
		return nil
	}}
}

// gifScreen returns the logical screen of a GIF, or the extent of its first frame when the
// screen is empty.
func gifScreen(g *gif.GIF) (int, int) {
	w, h := g.Config.Width, g.Config.Height
	if w <= 0 || h <= 0 {
		b := g.Image[0].Bounds()
		w, h = b.Max.X, b.Max.Y
	}
	return w, h
}

// webpSource composites the frames of an animated WebP, decoding one frame at a time.
func webpSource(data []byte) (animationSource, error) {
	anim, err := animation.DecodeBytes(data)
	if err != nil {
		return animationSource{}, fmt.Errorf("%w: %w", media.ErrCorrupt, err)
	}
	dec, err := animation.NewAnimDecoder(anim)
	if err != nil {
		return animationSource{}, fmt.Errorf("%w: %w", media.ErrCorrupt, err)
	}
	if animation.FrameDecoderFunc == nil {
		return animationSource{}, fmt.Errorf("media/native: no WebP frame decoder")
	}
	return animationSource{width: anim.CanvasWidth, height: anim.CanvasHeight, loops: anim.LoopCount, each: func(fn func(frame) error) error {
		for i := range anim.Frames {
			f := &anim.Frames[i]
			img, err := animation.FrameDecoderFunc(f.BitstreamData, f.AlphaData)
			if err != nil {
				return fmt.Errorf("%w: frame %d: %w", media.ErrCorrupt, i, err)
			}
			f.Image = img
			canvas, delay, err := dec.NextFrame()
			f.Image, f.BitstreamData, f.AlphaData = nil, nil, nil
			if err != nil {
				return fmt.Errorf("%w: frame %d: %w", media.ErrCorrupt, i, err)
			}
			if err := fn(frame{canvas: canvas, delay: delay}); err != nil {
				return err
			}
		}
		return nil
	}}, nil
}

// animate makes the variants of an animation: the still ones from its first frame, and the
// moving ones (ADR-0005 §6). When original is not nil and the large target keeps the whole
// screen unscaled, the large moving variant is original re-encoded, which keeps its frames and
// palettes exactly. pngLarge makes the large still variant PNG (stillVariants).
func animate(src animationSource, ts [3]target, original *gif.GIF, pngLarge bool) (media.Output, error) {
	out := media.Output{Width: src.width, Height: src.height}
	var (
		webpBufs [2]bytes.Buffer
		webpEncs [2]*animation.AnimEncoder
		large    = &gif.GIF{Config: image.Config{Width: ts[2].width, Height: ts[2].height}, LoopCount: -1}
		first    [3]*image.NRGBA
		n        int
	)
	keepOriginal := original != nil && ts[2].src == image.Rect(0, 0, src.width, src.height) &&
		ts[2].width == src.width && ts[2].height == src.height
	switch {
	case original != nil:
		large.LoopCount = original.LoopCount
	case src.loops == 0:
		large.LoopCount = 0 // forever
	case src.loops == 1:
		large.LoopCount = -1 // once
	default:
		large.LoopCount = src.loops - 1 // repeats after the first play
	}
	for i := range webpEncs {
		webpEncs[i] = animation.NewEncoder(&webpBufs[i], ts[i].width, ts[i].height,
			&animation.EncodeOptions{LoopCount: src.loops, Quality: webpQuality})
		if webpEncs[i] == nil {
			return media.Output{}, fmt.Errorf("%w: canvas %d×%d", media.ErrCorrupt, ts[i].width, ts[i].height)
		}
	}
	err := src.each(func(f frame) error {
		imgs := renderAll(f.canvas, ts, draw.BiLinear)
		for i, img := range imgs {
			if img == f.canvas {
				imgs[i] = cloneNRGBA(img) // the encoders may keep the previous frame
			}
		}
		if n == 0 {
			first = imgs
		}
		n++
		delay := f.delay
		if delay < minFrameDelay {
			delay = defaultFrameDelay
		}
		for i, enc := range webpEncs {
			if err := enc.AddFrame(imgs[i], delay); err != nil {
				return fmt.Errorf("media/native: encode animated webp: %w", err)
			}
		}
		if !keepOriginal {
			large.Image = append(large.Image, quantize(imgs[2], f.palette))
			large.Delay = append(large.Delay, int(f.delay/(10*time.Millisecond)))
			large.Disposal = append(large.Disposal, gif.DisposalNone)
		}
		return nil
	})
	if err != nil {
		return media.Output{}, err
	}
	if n == 0 {
		return media.Output{}, fmt.Errorf("%w: no frames", media.ErrCorrupt)
	}
	if out.Still, err = stillVariants(first, pngLarge); err != nil {
		return media.Output{}, err
	}
	out.Blurhash = blurhash(first[0], blurhashX, blurhashY)
	moving := &media.Variants{}
	for i, v := range []*media.Variant{&moving.Small, &moving.Medium} {
		if err := webpEncs[i].Close(); err != nil {
			return media.Output{}, fmt.Errorf("media/native: encode animated webp: %w", err)
		}
		*v = media.Variant{Data: webpBufs[i].Bytes(), ContentType: "image/webp", Extension: "webp", Width: ts[i].width, Height: ts[i].height}
	}
	if keepOriginal {
		large = original
	}
	var buf bytes.Buffer
	// Re-encoding keeps the frames, their delays, and the loop count, and drops comments and
	// application extensions (XMP).
	if err := gif.EncodeAll(&buf, large); err != nil {
		return media.Output{}, fmt.Errorf("media/native: encode gif: %w", err)
	}
	moving.Large = media.Variant{Data: buf.Bytes(), ContentType: "image/gif", Extension: "gif", Width: ts[2].width, Height: ts[2].height}
	out.Animated = moving
	return out, nil
}

func cloneNRGBA(img *image.NRGBA) *image.NRGBA {
	c := image.NewNRGBA(img.Rect)
	copy(c.Pix, img.Pix)
	return c
}

// quantize maps img onto pal, the palette of the GIF frame it was scaled from, or onto the
// Plan 9 palette for a WebP frame. A transparent color is added when img has transparency and
// the palette has none.
func quantize(img *image.NRGBA, pal color.Palette) *image.Paletted {
	if len(pal) == 0 {
		pal = palette.Plan9[:255:255]
	}
	if !img.Opaque() && !hasTransparent(pal) {
		pal = append(pal[:min(len(pal), 255):min(len(pal), 255)], color.NRGBA{})
	}
	dst := image.NewPaletted(img.Rect, pal)
	cache := make(map[uint32]uint8, 512)
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		row := img.Pix[img.PixOffset(img.Rect.Min.X, y):]
		out := dst.Pix[dst.PixOffset(dst.Rect.Min.X, y):]
		for x := range img.Rect.Dx() {
			p := row[x*4 : x*4+4 : x*4+4]
			key := uint32(p[0])<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3])
			if p[3] == 0 {
				key = 0
			}
			idx, ok := cache[key]
			if !ok {
				idx = uint8(pal.Index(color.NRGBA{p[0], p[1], p[2], p[3]})) //nolint:gosec // a palette has at most 256 colors
				cache[key] = idx
			}
			out[x] = idx
		}
	}
	return dst
}

func hasTransparent(pal color.Palette) bool {
	for _, c := range pal {
		if _, _, _, a := c.RGBA(); a == 0 {
			return true
		}
	}
	return false
}
