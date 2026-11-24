// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image/png"
	"slices"
)

// The builders below write the AVIF, HEIC, and APNG images that only a plugin of images decodes (ADR-0146 §13): their
// headers are valid, and the HEIF ones carry a placeholder ("ITEM") instead of coded images, since the server reads them
// without decoding a pixel. internal/app/media, which may import nothing of the repository, keeps a copy for its own
// tests (fixtures_test.go there).

// HEIFOptions describe a HEIF file of HEIFFile.
type HEIFOptions struct {
	// Brands are the major brand of the ftyp box, then its compatible brands ("avif", "mif1", "heic").
	Brands []string
	// Width and Height are the ispe property of the primary item (pitm); zero writes no meta box.
	Width, Height int
	// Thumbnail writes another item before the primary one, with an ispe of its own of a quarter of the primary's, as
	// the thumbnail item of a camera's file.
	Thumbnail bool
	// Tracks are the tracks of the moov box, in order; none writes no moov box.
	Tracks []HEIFTrack
}

// HEIFTrack is a track of an image sequence of HEIFFile.
type HEIFTrack struct {
	// Handler is the handler type: "pict" for an image sequence, "vide", or another one such as "auxv" or "soun".
	Handler string
	// Width and Height are the track header's.
	Width, Height int
	// Samples is the number of samples of the track.
	Samples int
	// Compact writes the sample sizes in a stz2 box instead of stsz.
	Compact bool
}

// HEIFFile writes a HEIF file: its ftyp box, a meta box with the primary item and its properties, a moov box with the
// tracks, and an mdat box of a placeholder.
func HEIFFile(o HEIFOptions) []byte {
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
func heifMeta(o HEIFOptions) []byte {
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
func heifMoov(tracks []HEIFTrack) []byte {
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

// APNGOptions describe an APNG of APNGFile.
type APNGOptions struct {
	// Width and Height are the canvas, the IHDR's and the default image's.
	Width, Height int
	// Frames is the num_frames of the acTL chunk.
	Frames int
	// Controls are the fcTL chunks, in order: the first comes before the IDAT chunk, as the default image is the first
	// frame, and each other with an fdAT chunk after it. Nil is Frames frames of the whole canvas.
	Controls []APNGFrame
	// LateControl writes the acTL chunk after the IDAT chunk, which makes the file a plain PNG.
	LateControl bool
}

// APNGFrame is the region of a frame on the canvas, of its fcTL chunk.
type APNGFrame struct {
	Width, Height, X, Y int
}

// APNGFile writes an APNG of FixtureImage whose frames after the first repeat the default image's data, with a tEXt
// chunk that contains MetadataMarker. The PNG decoder reads it as the still default image.
func APNGFile(o APNGOptions) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, FixtureImage(o.Width, o.Height)); err != nil {
		panic(err)
	}
	chunks := pngChunks(buf.Bytes())
	controls := o.Controls
	if controls == nil {
		for range o.Frames {
			controls = append(controls, APNGFrame{Width: o.Width, Height: o.Height})
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
	fctl := func(f APNGFrame) []byte {
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
			out = append(out, pngChunk("tEXt", []byte("Comment\x00"+MetadataMarker))...)
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
