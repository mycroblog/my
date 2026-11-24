// SPDX-License-Identifier: AGPL-3.0-only

package native

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/webp"

	"github.com/mycroblog/my/internal/app/media"
)

// inspect detects the format of data and checks it against limits using only its headers.
// For a GIF it also walks the block structure to count the frames and their pixels, which
// skips the compressed data without decoding it.
func inspect(data []byte, limits media.Limits) (media.Probe, error) {
	if int64(len(data)) > limits.MaxBytes {
		return media.Probe{}, fmt.Errorf("%w: %d bytes, at most %d", media.ErrTooLarge, len(data), limits.MaxBytes)
	}
	format := media.Sniff(data)
	if format == media.FormatAPNG {
		// The PNG decoder reads the default image of an APNG, which is all of it that this
		// processor makes variants of (ADR-0146 §13).
		format = media.FormatPNG
	}
	var decodeConfig func(r *bytes.Reader) (image.Config, error)
	switch format {
	case media.FormatJPEG:
		decodeConfig = func(r *bytes.Reader) (image.Config, error) { return jpeg.DecodeConfig(r) }
	case media.FormatPNG:
		decodeConfig = func(r *bytes.Reader) (image.Config, error) { return png.DecodeConfig(r) }
	case media.FormatGIF:
		decodeConfig = func(r *bytes.Reader) (image.Config, error) { return gif.DecodeConfig(r) }
	case media.FormatWebP:
		decodeConfig = func(r *bytes.Reader) (image.Config, error) { return webp.DecodeConfig(r) }
	default:
		return media.Probe{}, media.ErrUnsupportedFormat
	}
	cfg, err := decodeConfig(bytes.NewReader(data))
	if err != nil {
		return media.Probe{}, fmt.Errorf("%w: %s header: %w", media.ErrCorrupt, format, err)
	}
	probe := media.Probe{Format: format, Width: cfg.Width, Height: cfg.Height, Frames: 1}
	if err := checkDimensions(probe.Width, probe.Height, limits); err != nil {
		return media.Probe{}, err
	}
	var countFrames func([]byte, int) (int, int64, error)
	switch {
	case format == media.FormatGIF:
		countFrames = gifFrames
	case format == media.FormatWebP && webpAnimated(data):
		countFrames = webpFrames
	}
	if countFrames != nil {
		frames, pixels, err := countFrames(data, limits.MaxFrames)
		if err != nil {
			return media.Probe{}, err
		}
		if pixels > limits.MaxPixels {
			return media.Probe{}, fmt.Errorf("%w: %d pixels in all frames, at most %d", media.ErrDimensions, pixels, limits.MaxPixels)
		}
		// Every frame is composited on the whole canvas and scaled from it.
		if composited := int64(frames) * int64(probe.Width) * int64(probe.Height); composited > media.CompositedPixelsFactor*limits.MaxPixels {
			return media.Probe{}, fmt.Errorf("%w: %d frames of %d×%d", media.ErrDimensions, frames, probe.Width, probe.Height)
		}
		probe.Frames = frames
	}
	return probe, nil
}

func checkDimensions(w, h int, limits media.Limits) error {
	if w <= 0 || h <= 0 {
		return fmt.Errorf("%w: empty image %d×%d", media.ErrCorrupt, w, h)
	}
	if w > limits.MaxSide || h > limits.MaxSide {
		return fmt.Errorf("%w: %d×%d, at most %d on a side", media.ErrDimensions, w, h, limits.MaxSide)
	}
	if int64(w)*int64(h) > limits.MaxPixels {
		return fmt.Errorf("%w: %d×%d, at most %d pixels", media.ErrDimensions, w, h, limits.MaxPixels)
	}
	return nil
}

// webpAnimated reports whether an extended WebP file (VP8X) has its animation flag set.
func webpAnimated(data []byte) bool {
	const vp8xFlags = 12 + 8 // RIFF header, then the VP8X chunk header
	return len(data) > vp8xFlags && string(data[12:16]) == "VP8X" && data[vp8xFlags]&0x02 != 0
}

// webpFrames counts the ANMF chunks of an animated WebP and the pixels they cover, failing as
// soon as there are more than maxFrames frames or a frame leaves the canvas.
func webpFrames(data []byte, maxFrames int) (frames int, pixels int64, err error) {
	corrupt := fmt.Errorf("%w: truncated or malformed WebP", media.ErrCorrupt)
	if len(data) < 30 {
		return 0, 0, corrupt
	}
	u24 := func(b []byte) int { return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 }
	canvasW, canvasH := u24(data[24:])+1, u24(data[27:])+1
	end := min(len(data), 8+int(binary.LittleEndian.Uint32(data[4:])))
	for p := 12; p+8 <= end; {
		size := int(binary.LittleEndian.Uint32(data[p+4:]))
		body := p + 8
		if size < 0 || body+size > end {
			return 0, 0, corrupt
		}
		if string(data[p:p+4]) == "ANMF" {
			if size < 16 {
				return 0, 0, corrupt
			}
			x, y := 2*u24(data[body:]), 2*u24(data[body+3:])
			w, h := u24(data[body+6:])+1, u24(data[body+9:])+1
			if x+w > canvasW || y+h > canvasH {
				return 0, 0, fmt.Errorf("%w: a WebP frame outside the canvas", media.ErrCorrupt)
			}
			frames++
			pixels += int64(w) * int64(h)
			if frames > maxFrames {
				return 0, 0, fmt.Errorf("%w: more than %d frames", media.ErrDimensions, maxFrames)
			}
		}
		p = body + size + size&1
	}
	if frames == 0 {
		return 0, 0, corrupt
	}
	return frames, pixels, nil
}

// gifFrames counts the image descriptors of a GIF and the pixels they cover, failing as soon
// as there are more than maxFrames frames.
func gifFrames(data []byte, maxFrames int) (frames int, pixels int64, err error) {
	corrupt := fmt.Errorf("%w: truncated or malformed GIF", media.ErrCorrupt)
	if len(data) < 13 {
		return 0, 0, corrupt
	}
	p := 13
	if flags := data[10]; flags&0x80 != 0 {
		p += 3 << ((flags & 0x07) + 1) // global color table
	}
	skipSubBlocks := func() bool {
		for {
			if p >= len(data) {
				return false
			}
			n := int(data[p])
			p++
			if n == 0 {
				return true
			}
			p += n
		}
	}
	for {
		if p >= len(data) {
			return 0, 0, corrupt
		}
		switch data[p] {
		case 0x21: // extension: introducer, label, sub-blocks
			p += 2
			if !skipSubBlocks() {
				return 0, 0, corrupt
			}
		case 0x2C: // image descriptor
			if p+10 > len(data) {
				return 0, 0, corrupt
			}
			w := binary.LittleEndian.Uint16(data[p+5:])
			h := binary.LittleEndian.Uint16(data[p+7:])
			flags := data[p+9]
			p += 10
			if flags&0x80 != 0 {
				p += 3 << ((flags & 0x07) + 1) // local color table
			}
			p++ // LZW minimum code size
			if !skipSubBlocks() {
				return 0, 0, corrupt
			}
			frames++
			pixels += int64(w) * int64(h)
			if frames > maxFrames {
				return 0, 0, fmt.Errorf("%w: more than %d frames", media.ErrDimensions, maxFrames)
			}
		case 0x3B: // trailer
			if frames == 0 {
				return 0, 0, corrupt
			}
			return frames, pixels, nil
		default:
			return 0, 0, corrupt
		}
	}
}
