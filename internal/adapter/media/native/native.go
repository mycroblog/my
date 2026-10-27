// SPDX-License-Identifier: AGPL-3.0-only

package native

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"runtime"

	"golang.org/x/image/draw"
	xwebp "golang.org/x/image/webp"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/media"
)

// Options configure a Processor.
type Options struct {
	// Concurrency is the number of files processed at the same time; the default is half the
	// CPUs, at least 1. Decoding a large image takes hundreds of megabytes, so this bounds the
	// memory too.
	Concurrency int
}

// Processor implements media.Processor in pure Go with the standard library's JPEG, PNG,
// and GIF codecs, golang.org/x/image for still WebP decoding and resampling, and
// github.com/deepteams/webp for WebP encoding and animated WebP decoding.
type Processor struct {
	sem chan struct{}
}

var _ media.Processor = (*Processor)(nil)

// New returns a Processor.
func New(o Options) *Processor {
	if o.Concurrency <= 0 {
		o.Concurrency = max(1, runtime.GOMAXPROCS(0)/2)
	}
	return &Processor{sem: make(chan struct{}, o.Concurrency)}
}

// Inspect implements media.Processor.
func (p *Processor) Inspect(ctx context.Context, data []byte, limits media.Limits) (probe media.Probe, err error) {
	if err := ctx.Err(); err != nil {
		return media.Probe{}, err
	}
	defer func() {
		if r := recover(); r != nil {
			probe, err = media.Probe{}, fmt.Errorf("%w: decoder panicked: %v", media.ErrCorrupt, r)
		}
	}()
	return inspect(data, limits.WithDefaults())
}

// Process implements media.Processor.
func (p *Processor) Process(ctx context.Context, data []byte, limits media.Limits, sizes media.Sizes, req media.Request) (media.Output, error) {
	sizes = sizes.WithDefaults()
	return run(ctx, p, data, limits, func(probe media.Probe) (media.Output, error) { return process(data, probe, sizes, req) })
}

// Preview implements media.Processor: it decodes only the first frame, and scales it only to
// the small box.
func (p *Processor) Preview(ctx context.Context, data []byte, limits media.Limits, sizes media.Sizes, req media.Request) (media.Variant, error) {
	sizes = sizes.WithDefaults()
	return run(ctx, p, data, limits, func(probe media.Probe) (media.Variant, error) { return preview(data, probe, sizes, req) })
}

// run inspects data, then waits for a free slot of Options.Concurrency and calls fn in a
// goroutine; when ctx ends first, it returns ctx's error at once and fn finishes in the
// background, still holding its slot. A panic of fn is ErrCorrupt.
func run[T any](ctx context.Context, p *Processor, data []byte, limits media.Limits, fn func(media.Probe) (T, error)) (T, error) {
	var zero T
	probe, err := p.Inspect(ctx, data, limits.WithDefaults())
	if err != nil {
		return zero, err
	}
	select {
	case p.sem <- struct{}{}:
	case <-ctx.Done():
		return zero, ctx.Err()
	}
	type result struct {
		out T
		err error
	}
	done := make(chan result, 1)
	// The goroutine recovers the decoder's panic itself, as ErrCorrupt: kernel.Go has nothing to report.
	kernel.Go(ctx, "media.native.process", nil, func(context.Context) {
		defer func() { <-p.sem }()
		defer func() {
			if r := recover(); r != nil {
				done <- result{err: fmt.Errorf("%w: decoder panicked: %v", media.ErrCorrupt, r)}
			}
		}()
		out, err := fn(probe)
		done <- result{out: out, err: err}
	})
	select {
	case r := <-done:
		return r.out, r.err
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

// preview makes the small still variant of data: of an animation, its first frame.
func preview(data []byte, probe media.Probe, sizes media.Sizes, req media.Request) (media.Variant, error) {
	var (
		img *image.NRGBA
		err error
	)
	switch {
	case probe.Format == media.FormatGIF:
		img, err = firstGIFFrame(data)
	case probe.Format == media.FormatWebP && webpAnimated(data):
		var src animationSource
		if src, err = webpSource(data); err == nil {
			img, err = firstFrame(src)
		}
	default:
		var still image.Image
		if still, err = decodeStill(data, probe.Format); err == nil {
			img = toNRGBA(still)
			if probe.Format == media.FormatJPEG {
				img = orient(img, exifOrientation(data))
			}
		}
	}
	if err != nil {
		return media.Variant{}, err
	}
	t := plan(img.Rect.Dx(), img.Rect.Dy(), sizes.For(req.Purpose).Small, req.FocusX, req.FocusY)
	return encodeWebP(render(img, t, draw.CatmullRom))
}

// firstGIFFrame decodes only the first frame of a GIF and draws it on the logical screen.
func firstGIFFrame(data []byte) (*image.NRGBA, error) {
	cfg, err := gif.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", media.ErrCorrupt, err)
	}
	frame, err := gif.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", media.ErrCorrupt, err)
	}
	w, h := cfg.Width, cfg.Height
	if w <= 0 || h <= 0 {
		w, h = frame.Bounds().Max.X, frame.Bounds().Max.Y
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
	return img, nil
}

func process(data []byte, probe media.Probe, sizes media.Sizes, req media.Request) (media.Output, error) {
	boxes := sizes.For(req.Purpose)
	switch {
	case probe.Format == media.FormatGIF:
		g, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return media.Output{}, fmt.Errorf("%w: %w", media.ErrCorrupt, err)
		}
		src := gifSource(g)
		if len(g.Image) > 1 {
			return animate(src, targets(src.width, src.height, boxes, req), g, pngLarge(req))
		}
		return firstFrameOutput(src, boxes, req)
	case probe.Format == media.FormatWebP && webpAnimated(data):
		src, err := webpSource(data)
		if err != nil {
			return media.Output{}, err
		}
		if probe.Frames > 1 {
			return animate(src, targets(src.width, src.height, boxes, req), nil, pngLarge(req))
		}
		return firstFrameOutput(src, boxes, req)
	}
	src, err := decodeStill(data, probe.Format)
	if err != nil {
		return media.Output{}, err
	}
	img := toNRGBA(src)
	if probe.Format == media.FormatJPEG {
		img = orient(img, exifOrientation(data))
	}
	return stillOutput(img, boxes, req)
}

// firstFrameOutput makes the still variants of an animation of one frame.
func firstFrameOutput(src animationSource, boxes media.Boxes, req media.Request) (media.Output, error) {
	first, err := firstFrame(src)
	if err != nil {
		return media.Output{}, err
	}
	return stillOutput(first, boxes, req)
}

// errFirstFrame stops an animation after its first frame.
var errFirstFrame = errors.New("media/native: first frame")

// firstFrame returns a copy of the first composited frame of src, without decoding the others.
func firstFrame(src animationSource) (*image.NRGBA, error) {
	var first *image.NRGBA
	err := src.each(func(f frame) error {
		first = cloneNRGBA(f.canvas)
		return errFirstFrame
	})
	if err != nil && !errors.Is(err, errFirstFrame) {
		return nil, err
	}
	if first == nil {
		return nil, fmt.Errorf("%w: no frames", media.ErrCorrupt)
	}
	return first, nil
}

// pngLarge reports whether the large still variant of req is PNG whatever the image: the image
// of a custom emoji, which servers take as PNG, GIF, or WebP only (ADR-0051 §2).
func pngLarge(req media.Request) bool { return req.Purpose == media.PurposeEmoji }

// stillOutput makes the variants of a still image.
func stillOutput(img *image.NRGBA, boxes media.Boxes, req media.Request) (media.Output, error) {
	imgs := renderAll(img, targets(img.Rect.Dx(), img.Rect.Dy(), boxes, req), draw.CatmullRom)
	still, err := stillVariants(imgs, pngLarge(req))
	if err != nil {
		return media.Output{}, err
	}
	return media.Output{Width: img.Rect.Dx(), Height: img.Rect.Dy(), Still: still, Blurhash: blurhash(imgs[0], blurhashX, blurhashY)}, nil
}

func decodeStill(data []byte, f media.Format) (image.Image, error) {
	var (
		img image.Image
		err error
	)
	r := bytes.NewReader(data)
	switch f {
	case media.FormatJPEG:
		img, err = jpeg.Decode(r)
	case media.FormatPNG:
		img, err = png.Decode(r)
	case media.FormatWebP:
		img, err = xwebp.Decode(r)
	default:
		return nil, media.ErrUnsupportedFormat
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", media.ErrCorrupt, err)
	}
	return img, nil
}

// toNRGBA copies img into a new NRGBA image whose bounds start at the origin.
func toNRGBA(img image.Image) *image.NRGBA {
	b := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Rect, img, b.Min, draw.Src)
	return dst
}
