// SPDX-License-Identifier: AGPL-3.0-only

package media_test

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"image/png"
	"slices"
)

// The built-in formats, which the extended ones must not be taken for.

func pngImage(w, h int) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, w, h))); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func jpegImage(w, h int) []byte {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func gifImage(w, h, frames int) []byte {
	g := &gif.GIF{}
	for range frames {
		g.Image = append(g.Image, image.NewPaletted(image.Rect(0, 0, w, h), palette.Plan9))
		g.Delay = append(g.Delay, 10)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// webpLossy and webpAnimated are testkit.FixtureWebPLossy and testkit.FixtureWebPAnimated.
var (
	webpLossy, _    = base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	webpAnimated, _ = base64.StdEncoding.DecodeString("UklGRlIAAABXRUJQVlA4WAoAAAASAAAAAAAAAAAAQU5JTQYAAAD/////AABBTk1GJgAAAAAAAAAAAAAAAAAAAGQAAABWUDhMDQAAAC8AAAAQBxAREYiI/gcA")
)

// mp4File is the head of an MP4: its ftyp box and an empty moov box.
func mp4File() []byte {
	return slices.Concat(mp4Box("ftyp", []byte("isom\x00\x00\x02\x00isomiso2mp41")), mp4Box("moov"))
}

// The builders below are those of testkit.HEIFFile and testkit.APNGFile, which this package may not import (depguard):
// AVIF, HEIC, and APNG images whose headers are valid, and whose HEIF ones carry a placeholder ("ITEM") instead of coded
// images. Keep the two copies alike.

// heifOptions describe a HEIF file of heifFile.
type heifOptions struct {
	// Brands are the major brand of the ftyp box, then its compatible brands ("avif", "mif1", "heic").
	Brands []string
	// Width and Height are the ispe property of the primary item (pitm); zero writes no meta box.
	Width, Height int
	// Thumbnail writes another item before the primary one, with an ispe of its own of a quarter of the primary's, as
	// the thumbnail item of a camera's file.
	Thumbnail bool
	// Tracks are the tracks of the moov box, in order; none writes no moov box.
	Tracks []heifTrack
}

// heifTrack is a track of an image sequence of heifFile.
type heifTrack struct {
	// Handler is the handler type: "pict" for an image sequence, "vide", or another one such as "auxv" or "soun".
	Handler string
	// Width and Height are the track header's.
	Width, Height int
	// Samples is the number of samples of the track.
	Samples int
	// Compact writes the sample sizes in a stz2 box instead of stsz.
	Compact bool
}

// heifFile writes a HEIF file: its ftyp box, a meta box with the primary item and its properties, a moov box with the
// tracks, and an mdat box of a placeholder.
func heifFile(o heifOptions) []byte {
	ftyp := []byte{}
	if len(o.Brands) > 0 {
		ftyp = append(ftyp, o.Brands[0]...)
		ftyp = append(ftyp, u32(0)...)
		for _, b := range o.Brands[1:] {
			ftyp = append(ftyp, b...)
		}
	}
	out := mp4Box("ftyp", ftyp)
	if o.Width > 0 || o.Height > 0 {
		out = append(out, heifMeta(o)...)
	}
	if len(o.Tracks) > 0 {
		out = append(out, heifMoov(o.Tracks)...)
	}
	return append(out, mp4Box("mdat", []byte("ITEM"))...)
}

// heifMeta writes the meta box of the primary item 1 and, with a thumbnail, of item 2 listed and associated first.
func heifMeta(o heifOptions) []byte {
	fullBox := []byte{0, 0, 0, 0}
	hdlr := mp4Box("hdlr", fullBox, u32(0), []byte("pict"), make([]byte, 12), []byte{0})
	pitm := mp4Box("pitm", fullBox, u16(1))
	infe := func(id int) []byte {
		return mp4Box("infe", []byte{2, 0, 0, 0}, u16n(id), u16(0), []byte("av01"), []byte{0})
	}
	ispe := func(w, h int) []byte { return mp4Box("ispe", fullBox, u32n(w), u32n(h)) }
	items := [][]byte{infe(1)}
	properties := [][]byte{ispe(o.Width, o.Height)}
	// Each entry: item ID, one association of the 1-based index of its ispe.
	associations := [][]byte{slices.Concat(u16(1), []byte{1, 1})}
	if o.Thumbnail {
		items = [][]byte{infe(2), infe(1)}
		properties = [][]byte{ispe(max(o.Width/4, 1), max(o.Height/4, 1)), ispe(o.Width, o.Height)}
		associations = [][]byte{slices.Concat(u16(2), []byte{1, 1}), slices.Concat(u16(1), []byte{1, 2})}
	}
	iinf := mp4Box("iinf", slices.Concat(fullBox, u16n(len(items))), slices.Concat(items...))
	ipco := mp4Box("ipco", properties...)
	ipma := mp4Box("ipma", slices.Concat(fullBox, u32n(len(associations))), slices.Concat(associations...))
	iprp := mp4Box("iprp", ipco, ipma)
	return mp4Box("meta", fullBox, hdlr, pitm, iinf, iprp)
}

// heifMoov writes the moov box of the tracks.
func heifMoov(tracks []heifTrack) []byte {
	const timescale = 1000
	fullBox := []byte{0, 0, 0, 0}
	parts := [][]byte{mp4Box("mvhd", fullBox, u32(0), u32(0), u32(timescale), u32(timescale), make([]byte, 80))}
	for i, t := range tracks {
		matrix := make([]byte, 36)
		copy(matrix, u32(0x10000))
		copy(matrix[16:], u32(0x10000))
		copy(matrix[32:], u32(0x40000000))
		tkhd := mp4Box("tkhd", []byte{0, 0, 0, 3}, u32(0), u32(0), u32n(i+1), u32(0), u32(timescale), make([]byte, 16), matrix,
			u32n(t.Width<<16), u32n(t.Height<<16))
		hdlr := mp4Box("hdlr", fullBox, u32(0), []byte(t.Handler), make([]byte, 12), []byte("handler\x00"))
		var sizes []byte
		if t.Compact {
			sizes = slices.Concat(fullBox, []byte{0, 0, 0, 16}, u32n(t.Samples))
			for range t.Samples {
				sizes = append(sizes, u16(7)...)
			}
			sizes = mp4Box("stz2", sizes)
		} else {
			sizes = slices.Concat(fullBox, u32(0), u32n(t.Samples))
			for range t.Samples {
				sizes = append(sizes, u32(7)...)
			}
			sizes = mp4Box("stsz", sizes)
		}
		stts := mp4Box("stts", fullBox, u32(1), u32n(t.Samples), u32(1))
		stsc := mp4Box("stsc", fullBox, u32(1), u32(1), u32(1), u32(1))
		stco := mp4Box("stco", fullBox, u32(0))
		stbl := mp4Box("stbl", mp4Box("stsd", fullBox, u32(0)), stts, stsc, sizes, stco)
		mdhd := mp4Box("mdhd", fullBox, u32(0), u32(0), u32(timescale), u32(timescale), u32(0))
		mdia := mp4Box("mdia", mdhd, hdlr, mp4Box("minf", stbl))
		parts = append(parts, mp4Box("trak", tkhd, mdia))
	}
	return mp4Box("moov", parts...)
}

// apngOptions describe an APNG of apngFile.
type apngOptions struct {
	// Width and Height are the canvas, the IHDR's and the default image's.
	Width, Height int
	// Frames is the num_frames of the acTL chunk.
	Frames int
	// Controls are the fcTL chunks, in order: the first comes before the IDAT chunk, as the default image is the first
	// frame, and each other with an fdAT chunk after it. Nil is Frames frames of the whole canvas.
	Controls []apngFrame
	// LateControl writes the acTL chunk after the IDAT chunk, which makes the file a plain PNG.
	LateControl bool
}

// apngFrame is the region of a frame on the canvas, of its fcTL chunk.
type apngFrame struct {
	Width, Height, X, Y int
}

// apngFile writes an APNG whose frames after the first repeat the default image's data, with a tEXt chunk. The PNG decoder
// reads it as the still default image.
func apngFile(o apngOptions) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, o.Width, o.Height))); err != nil {
		panic(err)
	}
	chunks := pngChunks(buf.Bytes())
	controls := o.Controls
	if controls == nil {
		for range o.Frames {
			controls = append(controls, apngFrame{Width: o.Width, Height: o.Height})
		}
	}
	var idat []byte
	for _, c := range chunks {
		if c.typ == "IDAT" {
			idat = append(idat, c.body...)
		}
	}
	actl := pngChunk("acTL", slices.Concat(u32n(o.Frames), u32(0)))
	seq := 0
	fctl := func(f apngFrame) []byte {
		body := slices.Concat(u32n(seq), u32n(f.Width), u32n(f.Height), u32n(f.X), u32n(f.Y), u16(1), u16(10), []byte{0, 0})
		seq++
		return pngChunk("fcTL", body)
	}
	out := []byte("\x89PNG\r\n\x1A\n")
	wroteData := false
	for _, c := range chunks {
		switch c.typ {
		case "IHDR":
			out = append(out, pngChunk(c.typ, c.body)...)
			out = append(out, pngChunk("tEXt", []byte("Comment\x00a comment"))...)
			if !o.LateControl {
				out = append(out, actl...)
			}
		case "IDAT":
			if wroteData {
				continue
			}
			wroteData = true
			if len(controls) > 0 {
				out = append(out, fctl(controls[0])...)
			}
			out = append(out, pngChunk("IDAT", idat)...)
			if o.LateControl {
				out = append(out, actl...)
			}
			for _, f := range controls[min(1, len(controls)):] {
				out = append(out, fctl(f)...)
				out = append(out, pngChunk("fdAT", slices.Concat(u32n(seq), idat))...)
				seq++
			}
		default:
			out = append(out, pngChunk(c.typ, c.body)...)
		}
	}
	return out
}

type rawChunk struct {
	typ  string
	body []byte
}

// pngChunks splits an encoded PNG into its chunks.
func pngChunks(data []byte) []rawChunk {
	var out []rawChunk
	for p := 8; p+12 <= len(data); {
		n := int(binary.BigEndian.Uint32(data[p:]))
		out = append(out, rawChunk{typ: string(data[p+4 : p+8]), body: data[p+8 : p+8+n]})
		p += 12 + n
	}
	return out
}

// pngChunk writes a chunk with its CRC.
func pngChunk(typ string, body []byte) []byte {
	out := slices.Concat(u32n(len(body)), []byte(typ), body)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(append([]byte(typ), body...)))
}

func mp4Box(typ string, parts ...[]byte) []byte {
	n := 8
	for _, p := range parts {
		n += len(p)
	}
	out := binary.BigEndian.AppendUint32(make([]byte, 0, n), uint32(n))
	out = append(out, typ...)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func u32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }
func u16(v uint16) []byte { return binary.BigEndian.AppendUint16(nil, v) }

// u32n and u16n write the small sizes and counts of the test files.
func u32n(n int) []byte { return u32(uint32(n)) } //nolint:gosec // G115: small numbers of the test files
func u16n(n int) []byte { return u16(uint16(n)) } //nolint:gosec // G115: small numbers of the test files
