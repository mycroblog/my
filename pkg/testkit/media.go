// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/media"
)

// FakeMediaProcessor is a media.Processor for tests of the code around processing. It detects
// formats with media.Sniff, reports every file as 4 × 3 pixels, rejects files that contain
// "corrupt", and "processes" a file into still PNG variants, and a GIF also into moving GIF
// variants, whose content is derived from the input's digest and the purpose, so that the same
// input gives the same output.
type FakeMediaProcessor struct{}

var _ media.Processor = FakeMediaProcessor{}

// Inspect implements media.Processor.
func (FakeMediaProcessor) Inspect(ctx context.Context, data []byte, limits media.Limits) (media.Probe, error) {
	if err := ctx.Err(); err != nil {
		return media.Probe{}, err
	}
	limits = limits.WithDefaults()
	if int64(len(data)) > limits.MaxBytes {
		return media.Probe{}, media.ErrTooLarge
	}
	f := media.Sniff(data)
	if f == "" {
		return media.Probe{}, media.ErrUnsupportedFormat
	}
	if bytes.Contains(data, []byte("corrupt")) {
		return media.Probe{}, media.ErrCorrupt
	}
	return media.Probe{Format: f, Width: 4, Height: 3, Frames: 1}, nil
}

// Preview implements media.Processor: the small still variant of Process.
func (p FakeMediaProcessor) Preview(ctx context.Context, data []byte, limits media.Limits, sizes media.Sizes, req media.Request) (media.Variant, error) {
	out, err := p.Process(ctx, data, limits, sizes, req)
	return out.Still.Small, err
}

// Process implements media.Processor. A GIF also gets moving variants, as an animation would.
func (p FakeMediaProcessor) Process(ctx context.Context, data []byte, limits media.Limits, _ media.Sizes, req media.Request) (media.Output, error) {
	probe, err := p.Inspect(ctx, data, limits)
	if err != nil {
		return media.Output{}, err
	}
	// The variants depend on the purpose and, as a crop would, on the focal point.
	seed := []byte(req.Purpose)
	if req.FocusX != 0 || req.FocusY != 0 {
		seed = fmt.Appendf(seed, "@%g,%g", req.FocusX, req.FocusY)
	}
	sum := sha256.Sum256(append(seed, data...))
	digest := hex.EncodeToString(sum[:8])
	variant := func(kind string, w, h int) media.Variant {
		return media.Variant{Data: fmt.Appendf(nil, "\x89PNG\r\n\x1A\n%s:%s", kind, digest), ContentType: "image/png", Extension: "png", Width: w, Height: h}
	}
	moving := func(kind string, w, h int) media.Variant {
		return media.Variant{Data: fmt.Appendf(nil, "GIF89a%s:%s", kind, digest), ContentType: "image/gif", Extension: "gif", Width: w, Height: h}
	}
	out := media.Output{
		Width: 4, Height: 3, Blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
		Still: media.Variants{Small: variant("small", 1, 1), Medium: variant("medium", 2, 1), Large: variant("large", 4, 3)},
	}
	if probe.Format == media.FormatGIF {
		out.Animated = &media.Variants{Small: moving("small", 1, 1), Medium: moving("medium", 2, 1), Large: moving("large", 4, 3)}
	}
	return out, nil
}

// InspectAV implements media.AVProcessor: it detects the container with media.SniffAV, reports
// a file as 3 seconds long (an MP4 or WebM that contains "audio-only" as audio, the others of
// those as 640 × 360 video), and rejects files that contain "corrupt", "hevc" (an unsupported
// codec), or "too-long".
func (FakeMediaProcessor) InspectAV(ctx context.Context, data []byte, limits media.AVLimits) (media.AVProbe, error) {
	if err := ctx.Err(); err != nil {
		return media.AVProbe{}, err
	}
	limits = limits.WithDefaults()
	if int64(len(data)) > limits.MaxBytes {
		return media.AVProbe{}, media.ErrTooLarge
	}
	f := media.SniffAV(data)
	switch {
	case f == "":
		return media.AVProbe{}, media.ErrUnsupportedFormat
	case bytes.Contains(data, []byte("corrupt")):
		return media.AVProbe{}, media.ErrCorrupt
	case bytes.Contains(data, []byte("hevc")):
		return media.AVProbe{}, media.ErrUnsupportedCodec
	case bytes.Contains(data, []byte("too-long")):
		return media.AVProbe{}, media.ErrTooLong
	}
	p := media.AVProbe{Format: f, Kind: media.AVAudio, Duration: 3 * time.Second, AudioCodec: "opus"}
	switch {
	case f == media.FormatMP3:
		p.ContentType, p.Extension, p.AudioCodec = "audio/mpeg", "mp3", "mp3"
	case f == media.FormatOgg:
		p.ContentType, p.Extension = "audio/ogg", "ogg"
	case f == media.FormatMP4 && bytes.Contains(data, []byte("audio-only")):
		p.ContentType, p.Extension, p.AudioCodec = "audio/mp4", "m4a", "aac"
	case f == media.FormatWebM && bytes.Contains(data, []byte("audio-only")):
		p.ContentType, p.Extension = "audio/webm", "webm"
	case f == media.FormatMP4:
		p.Kind, p.ContentType, p.Extension, p.VideoCodec, p.AudioCodec = media.AVVideo, "video/mp4", "mp4", "h264", "aac"
		p.Width, p.Height = 640, 360
	default:
		p.Kind, p.ContentType, p.Extension, p.VideoCodec = media.AVVideo, "video/webm", "webm", "vp9"
		p.Width, p.Height = 640, 360
	}
	return p, nil
}

// Remux implements media.AVProcessor: the file without the bytes "location", and as its cover
// the bytes after "cover:", if any.
func (p FakeMediaProcessor) Remux(ctx context.Context, data []byte, limits media.AVLimits) (media.AVOutput, error) {
	probe, err := p.InspectAV(ctx, data, limits)
	if err != nil {
		return media.AVOutput{}, err
	}
	out := media.AVOutput{AVProbe: probe, Data: bytes.ReplaceAll(data, []byte("location"), nil)}
	if _, cover, ok := bytes.Cut(data, []byte("cover:")); ok {
		out.Cover = bytes.Clone(cover)
	}
	return out, nil
}
