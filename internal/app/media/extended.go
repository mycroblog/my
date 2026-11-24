// SPDX-License-Identifier: AGPL-3.0-only

package media

import (
	"encoding/binary"
	"fmt"
)

// The formats that only a plugin decodes are checked here from their headers, without decoding a
// pixel, so that the server needs no decoder of them (ADR-0146 §13). Every read is bounded by the
// data: a length that leaves the data is ErrCorrupt, and nothing is allocated from a length or a
// count that the file declares.

var pngSignature = []byte("\x89PNG\r\n\x1A\n")

// maxPNGChunk is the largest length of a PNG chunk (2^31 - 1).
const maxPNGChunk = 1<<31 - 1

// InspectExtended reads the format, dimensions, and frames of an AVIF, HEIC, or APNG image from
// its headers and checks them against limits (zero fields take their defaults), as a processor's
// Inspect does for the built-in formats. The dimensions of a HEIF file are the ispe property of
// its primary item, or else the track header of its image sequence; its frames are the samples
// of that sequence, or 1. The frames of an APNG are its acTL's, each with its fcTL. It fails with
// an error wrapping ErrUnsupportedFormat for any other format, and with ErrTooLarge,
// ErrDimensions, or ErrCorrupt as Processor.Inspect does.
func InspectExtended(data []byte, limits Limits) (Probe, error) {
	limits = limits.WithDefaults()
	if int64(len(data)) > limits.MaxBytes {
		return Probe{}, fmt.Errorf("%w: %d bytes, at most %d", ErrTooLarge, len(data), limits.MaxBytes)
	}
	format := Sniff(data)
	var (
		probe Probe
		err   error
	)
	switch format {
	case FormatAVIF, FormatHEIC:
		probe, err = inspectHEIF(data, limits)
	case FormatAPNG:
		probe, err = inspectAPNG(data, limits)
	default:
		return Probe{}, fmt.Errorf("%w: %s is not a format that only a plugin decodes", ErrUnsupportedFormat, describeFormat(format))
	}
	if err != nil {
		return Probe{}, err
	}
	probe.Format = format
	return probe, nil
}

func describeFormat(f Format) string {
	if f == "" {
		return "no known format"
	}
	return string(f)
}

// checkSize checks the dimensions of an image of frames frames that cover pixels in all, which
// are frames × w × h when every frame covers the canvas.
func checkSize(w, h, frames int, pixels int64, limits Limits) error {
	if w <= 0 || h <= 0 {
		return fmt.Errorf("%w: empty image %d×%d", ErrCorrupt, w, h)
	}
	if w > limits.MaxSide || h > limits.MaxSide {
		return fmt.Errorf("%w: %d×%d, at most %d on a side", ErrDimensions, w, h, limits.MaxSide)
	}
	canvas := int64(w) * int64(h)
	if canvas > limits.MaxPixels {
		return fmt.Errorf("%w: %d×%d, at most %d pixels", ErrDimensions, w, h, limits.MaxPixels)
	}
	if frames > limits.MaxFrames {
		return fmt.Errorf("%w: %d frames, at most %d", ErrDimensions, frames, limits.MaxFrames)
	}
	if frames < 2 {
		return nil
	}
	if pixels > limits.MaxPixels {
		return fmt.Errorf("%w: %d pixels in all frames, at most %d", ErrDimensions, pixels, limits.MaxPixels)
	}
	// Every frame is composited on the whole canvas and scaled from it.
	if int64(frames) > CompositedPixelsFactor*limits.MaxPixels/canvas {
		return fmt.Errorf("%w: %d frames of %d×%d", ErrDimensions, frames, w, h)
	}
	return nil
}

// pngAnimated reports whether the chunks of a PNG, read up to its first IDAT chunk, hold an acTL
// chunk. A PNG whose chunks break before that is not one.
func pngAnimated(data []byte) bool {
	for p := len(pngSignature); len(data)-p >= 8; {
		n := int64(binary.BigEndian.Uint32(data[p:]))
		switch string(data[p+4 : p+8]) {
		case "acTL":
			return true
		case "IDAT":
			return false
		}
		if n > maxPNGChunk || n+12 > int64(len(data)-p) {
			return false
		}
		p += int(n) + 12
	}
	return false
}

// inspectAPNG reads the canvas of an APNG from its IHDR chunk and its frames from its acTL chunk,
// checking that its fcTL chunks are as many and inside the canvas.
func inspectAPNG(data []byte, limits Limits) (Probe, error) {
	corrupt := fmt.Errorf("%w: truncated or malformed APNG", ErrCorrupt)
	be := binary.BigEndian
	const ihdr = 8 // the IHDR chunk follows the signature
	if len(data) < ihdr+8+13 || be.Uint32(data[ihdr:]) != 13 || string(data[ihdr+4:ihdr+8]) != "IHDR" {
		return Probe{}, corrupt
	}
	cw, ch := be.Uint32(data[ihdr+8:]), be.Uint32(data[ihdr+12:])
	if cw > maxPNGChunk || ch > maxPNGChunk {
		return Probe{}, corrupt
	}
	w, h := int(cw), int(ch)
	if err := checkSize(w, h, 1, 0, limits); err != nil {
		return Probe{}, err
	}
	declared, frames := -1, 0
	var pixels int64
	for p := ihdr; ; {
		if len(data)-p < 12 {
			return Probe{}, corrupt
		}
		n := int64(be.Uint32(data[p:]))
		if n > maxPNGChunk || n+12 > int64(len(data)-p) {
			return Probe{}, corrupt
		}
		typ, body := string(data[p+4:p+8]), data[p+8:p+8+int(n)]
		p += int(n) + 12
		switch typ {
		case "acTL":
			if declared >= 0 || len(body) != 8 {
				return Probe{}, corrupt
			}
			num := be.Uint32(body)
			if num == 0 {
				return Probe{}, corrupt
			}
			if int64(num) > int64(limits.MaxFrames) {
				return Probe{}, fmt.Errorf("%w: %d frames, at most %d", ErrDimensions, num, limits.MaxFrames)
			}
			declared = int(num)
		case "fcTL":
			if len(body) != 26 {
				return Probe{}, corrupt
			}
			fw, fh := uint64(be.Uint32(body[4:])), uint64(be.Uint32(body[8:]))
			fx, fy := uint64(be.Uint32(body[12:])), uint64(be.Uint32(body[16:]))
			if fw == 0 || fh == 0 || fx+fw > uint64(cw) || fy+fh > uint64(ch) {
				return Probe{}, fmt.Errorf("%w: an APNG frame outside the canvas", ErrCorrupt)
			}
			frames++
			if declared < 0 || frames > declared {
				return Probe{}, corrupt
			}
			// Within the canvas, so at most MaxPixels a frame, and the sum stops at the limit.
			pixels += int64(fw * fh) //nolint:gosec // within the canvas, checked above
			if frames > 1 && pixels > limits.MaxPixels {
				return Probe{}, fmt.Errorf("%w: more than %d pixels in all frames", ErrDimensions, limits.MaxPixels)
			}
		case "IEND":
			if declared < 0 || frames != declared {
				return Probe{}, corrupt
			}
			if err := checkSize(w, h, frames, pixels, limits); err != nil {
				return Probe{}, err
			}
			return Probe{Width: w, Height: h, Frames: frames}, nil
		}
	}
}

// heifFormat returns the format that the brands of a HEIF file's ftyp box give, the major brand
// first, or "" for none of AVIF and HEIC.
func heifFormat(data []byte) Format {
	if len(data) < 16 || string(data[4:8]) != "ftyp" {
		return ""
	}
	size := int64(binary.BigEndian.Uint32(data))
	if size < 16 {
		return ""
	}
	end := int(min(size, int64(len(data))))
	if f := heifBrand(string(data[8:12])); f != "" {
		return f
	}
	for p := 16; p+4 <= end; p += 4 {
		if f := heifBrand(string(data[p : p+4])); f != "" {
			return f
		}
	}
	return ""
}

func heifBrand(brand string) Format {
	switch brand {
	case "avif", "avis":
		return FormatAVIF
	case "heic", "heix", "heim", "heis", "hevc", "hevx":
		return FormatHEIC
	}
	return ""
}

// fourCC is the type of an ISO BMFF box.
type fourCC [4]byte

func (t fourCC) is(s string) bool { return string(t[:]) == s }

// nextBox splits the box at the start of b into its type and body, and returns the boxes after
// it; ok is false when its header is malformed or it leaves b. A size of 0 runs to the end of b.
func nextBox(b []byte) (typ fourCC, body, rest []byte, ok bool) {
	if len(b) < 8 {
		return typ, nil, nil, false
	}
	size, header := uint64(binary.BigEndian.Uint32(b)), uint64(8)
	copy(typ[:], b[4:8])
	switch size {
	case 0:
		size = uint64(len(b))
	case 1:
		if len(b) < 16 {
			return typ, nil, nil, false
		}
		size, header = binary.BigEndian.Uint64(b[8:]), 16
	}
	if size < header || size > uint64(len(b)) {
		return typ, nil, nil, false
	}
	return typ, b[header:size], b[size:], true
}

// eachBox calls fn with every box of b in order, until fn returns false; it reports whether the
// boxes were well formed up to there.
func eachBox(b []byte, fn func(typ fourCC, body []byte) bool) bool {
	for len(b) > 0 {
		typ, body, rest, ok := nextBox(b)
		if !ok {
			return false
		}
		if !fn(typ, body) {
			return true
		}
		b = rest
	}
	return true
}

// maxIPMA bounds the item property association boxes kept from one iprp box.
const maxIPMA = 8

// heif is what inspectHEIF reads of a HEIF file.
type heif struct {
	hasPrimary bool
	primary    uint32
	ipco       []byte
	ipma       [][]byte
	// track is the image sequence with the most samples, if any.
	trackW, trackH int
	samples        uint32
}

// inspectHEIF reads the dimensions of a HEIF file's primary item from its ispe property, or of
// its image sequence from the track header, and the frames of the image sequence.
func inspectHEIF(data []byte, limits Limits) (Probe, error) {
	corrupt := fmt.Errorf("%w: truncated or malformed HEIF", ErrCorrupt)
	var f heif
	metaSeen, moovSeen := false, false
	broken := false
	if !eachBox(data, func(typ fourCC, body []byte) bool {
		switch {
		case typ.is("meta") && !metaSeen:
			metaSeen = true
			broken = !f.meta(body)
		case typ.is("moov") && !moovSeen:
			moovSeen = true
			broken = !f.moov(body)
		}
		return !broken
	}) || broken {
		return Probe{}, corrupt
	}
	w, h, ok := 0, 0, false
	if f.hasPrimary {
		if w, h, ok = f.primarySize(); !ok {
			return Probe{}, corrupt
		}
	}
	if !ok {
		if f.samples == 0 {
			return Probe{}, corrupt
		}
		w, h = f.trackW, f.trackH
	}
	frames := 1
	if f.samples > 1 {
		if int64(f.samples) > int64(limits.MaxFrames) {
			return Probe{}, fmt.Errorf("%w: %d frames, at most %d", ErrDimensions, f.samples, limits.MaxFrames)
		}
		frames = int(f.samples)
	}
	if err := checkSize(w, h, 1, 0, limits); err != nil {
		return Probe{}, err
	}
	// Every frame of an image sequence covers the canvas.
	var pixels int64
	if frames > 1 {
		canvas := int64(w) * int64(h)
		if int64(frames) > limits.MaxPixels/canvas {
			return Probe{}, fmt.Errorf("%w: %d frames of %d×%d, more than %d pixels in all", ErrDimensions, frames, w, h, limits.MaxPixels)
		}
		pixels = int64(frames) * canvas
	}
	if err := checkSize(w, h, frames, pixels, limits); err != nil {
		return Probe{}, err
	}
	return Probe{Width: w, Height: h, Frames: frames}, nil
}

// meta reads the primary item and the item properties of a meta box (a full box).
func (f *heif) meta(body []byte) bool {
	if len(body) < 4 {
		return false
	}
	broken := false
	ok := eachBox(body[4:], func(typ fourCC, b []byte) bool {
		switch {
		case typ.is("pitm"):
			if len(b) < 4 {
				broken = true
				break
			}
			if b[0] == 0 {
				if len(b) < 6 {
					broken = true
					break
				}
				f.primary = uint32(binary.BigEndian.Uint16(b[4:]))
			} else {
				if len(b) < 8 {
					broken = true
					break
				}
				f.primary = binary.BigEndian.Uint32(b[4:])
			}
			f.hasPrimary = true
		case typ.is("iprp"):
			broken = !eachBox(b, func(typ fourCC, b []byte) bool {
				switch {
				case typ.is("ipco") && f.ipco == nil:
					f.ipco = b
				case typ.is("ipma"):
					if len(f.ipma) == maxIPMA {
						broken = true
						return false
					}
					f.ipma = append(f.ipma, b)
				}
				return true
			}) || broken
		}
		return !broken
	})
	return ok && !broken
}

// primarySize returns the ispe property of the primary item: the first one among the properties
// that the item property associations give it.
func (f *heif) primarySize() (w, h int, ok bool) {
	// An item has at most 255 associations in one ipma box.
	var indices [255]uint16
	n := 0
	for _, b := range f.ipma {
		if n == 0 {
			n = primaryAssociations(b, f.primary, &indices)
			if n < 0 {
				return 0, 0, false
			}
		}
	}
	if n == 0 {
		return 0, 0, false
	}
	index := uint16(0)
	found := false
	eachBox(f.ipco, func(typ fourCC, b []byte) bool {
		index++
		if !typ.is("ispe") || len(b) < 12 {
			return index < 0x7FFF
		}
		for _, i := range indices[:n] {
			if i == index {
				pw, ph := binary.BigEndian.Uint32(b[4:]), binary.BigEndian.Uint32(b[8:])
				if pw > maxPNGChunk || ph > maxPNGChunk {
					return false
				}
				w, h, found = int(pw), int(ph), true
				return false
			}
		}
		return index < 0x7FFF
	})
	return w, h, found
}

// primaryAssociations puts the property indices that an ipma box (a full box) associates with the
// item into indices and returns how many there are: 0 when the box does not name the item, and -1
// when it is malformed.
func primaryAssociations(b []byte, item uint32, indices *[255]uint16) int {
	if len(b) < 8 {
		return -1
	}
	version, wide := b[0], b[3]&1 != 0
	count := binary.BigEndian.Uint32(b[4:])
	p := 8
	for range count {
		var id uint32
		if version < 1 {
			if len(b)-p < 2 {
				return -1
			}
			id = uint32(binary.BigEndian.Uint16(b[p:]))
			p += 2
		} else {
			if len(b)-p < 4 {
				return -1
			}
			id = binary.BigEndian.Uint32(b[p:])
			p += 4
		}
		if len(b)-p < 1 {
			return -1
		}
		n := int(b[p])
		p++
		size := 1
		if wide {
			size = 2
		}
		if len(b)-p < n*size {
			return -1
		}
		if id == item {
			for i := range n {
				if wide {
					indices[i] = binary.BigEndian.Uint16(b[p+2*i:]) & 0x7FFF
				} else {
					indices[i] = uint16(b[p+i] & 0x7F)
				}
			}
			return n
		}
		p += n * size
	}
	return 0
}

// moov reads the image sequence of a moov box: the track of pictures or video with the most
// samples, with the dimensions of its track header.
func (f *heif) moov(body []byte) bool {
	broken := false
	ok := eachBox(body, func(typ fourCC, b []byte) bool {
		if !typ.is("trak") {
			return true
		}
		w, h, handler, samples, ok := track(b)
		if !ok {
			broken = true
			return false
		}
		if (handler.is("pict") || handler.is("vide")) && samples > f.samples {
			f.trackW, f.trackH, f.samples = w, h, samples
		}
		return true
	})
	return ok && !broken
}

// track reads the dimensions of a trak box's track header, its handler, and its number of
// samples.
func track(trak []byte) (w, h int, handler fourCC, samples uint32, ok bool) {
	ok = eachBox(trak, func(typ fourCC, b []byte) bool {
		switch {
		case typ.is("tkhd"):
			// After the version and flags: the times, the track ID, and the duration, 20 or 32
			// bytes as the version says, then 52 bytes, then the width and height in 16.16.
			at := 4 + 20 + 52
			if len(b) > 0 && b[0] == 1 {
				at = 4 + 32 + 52
			}
			if len(b) < at+8 {
				return true
			}
			w, h = int(binary.BigEndian.Uint32(b[at:])>>16), int(binary.BigEndian.Uint32(b[at+4:])>>16)
		case typ.is("mdia"):
			eachBox(b, func(typ fourCC, b []byte) bool {
				switch {
				case typ.is("hdlr") && len(b) >= 12:
					copy(handler[:], b[8:12])
				case typ.is("minf"):
					eachBox(b, func(typ fourCC, b []byte) bool {
						if typ.is("stbl") {
							eachBox(b, func(typ fourCC, b []byte) bool {
								if (typ.is("stsz") || typ.is("stz2")) && len(b) >= 12 {
									samples = binary.BigEndian.Uint32(b[8:])
								}
								return true
							})
						}
						return true
					})
				}
				return true
			})
		}
		return true
	})
	return w, h, handler, samples, ok
}
