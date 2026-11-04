// SPDX-License-Identifier: AGPL-3.0-only

package native

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/media"
)

// An MP4 (ISO BMFF, QuickTime included) is remuxed into ftyp, moov, and one mdat (fast start)
// that holds only the chunks of the kept tracks (ADR-0005 §9). The moov keeps the movie header
// and the video and sound tracks with their creation and modification times zeroed, and drops
// everything else: user data (udta, where cameras put the location), metadata (meta, where
// iPhones put it), uuid boxes (XMP), and track references. Fragmented files (moof) are not
// accepted.

// mp4MaxDepth bounds the nesting of the boxes that the parser descends into.
const mp4MaxDepth = 8

// mp4Containers are the boxes whose children the remux reads or filters.
var mp4Containers = map[string]bool{"moov": true, "trak": true, "mdia": true, "minf": true, "stbl": true, "edts": true, "dinf": true}

// mp4Box is a box: a container with children, or a leaf with its payload.
type mp4Box struct {
	typ      string
	payload  []byte
	children []*mp4Box
	// offset is the absolute offset of the payload in the source file.
	offset int
}

func (b *mp4Box) child(typ string) *mp4Box {
	for _, c := range b.children {
		if c.typ == typ {
			return c
		}
	}
	return nil
}

// parseMP4Boxes reads the boxes in data, which starts at offset base of the file.
func parseMP4Boxes(data []byte, base, depth int) ([]*mp4Box, error) {
	if depth > mp4MaxDepth {
		return nil, fmt.Errorf("%w: mp4 boxes nested too deep", media.ErrCorrupt)
	}
	var out []*mp4Box
	for pos := 0; pos < len(data); {
		if len(data)-pos < 8 {
			return nil, fmt.Errorf("%w: mp4 box header cut off", media.ErrCorrupt)
		}
		size := uint64(binary.BigEndian.Uint32(data[pos:]))
		typ := string(data[pos+4 : pos+8])
		header := 8
		switch size {
		case 0:
			size = uint64(len(data) - pos)
		case 1:
			if len(data)-pos < 16 {
				return nil, fmt.Errorf("%w: mp4 box header cut off", media.ErrCorrupt)
			}
			size, header = binary.BigEndian.Uint64(data[pos+8:]), 16
		}
		if size < uint64(header) || size > uint64(len(data)-pos) {
			return nil, fmt.Errorf("%w: mp4 box %q of %d bytes", media.ErrCorrupt, typ, size)
		}
		b := &mp4Box{typ: typ, payload: data[pos+header : pos+int(size)], offset: base + pos + header} //nolint:gosec // G115: size is at most len(data)-pos, checked above
		if depth > 0 && mp4Containers[typ] {
			children, err := parseMP4Boxes(b.payload, b.offset, depth+1)
			if err != nil {
				return nil, err
			}
			b.children = children
		}
		out = append(out, b)
		pos += int(size) //nolint:gosec // G115: size is at most len(data)-pos, checked above
	}
	return out, nil
}

// appendMP4Box appends b to dst: its children, or its payload.
func appendMP4Box(dst []byte, b *mp4Box) []byte {
	start := len(dst)
	dst = append(dst, 0, 0, 0, 0)
	dst = append(dst, b.typ...)
	if b.children != nil {
		for _, c := range b.children {
			dst = appendMP4Box(dst, c)
		}
	} else {
		dst = append(dst, b.payload...)
	}
	binary.BigEndian.PutUint32(dst[start:], uint32(len(dst)-start)) //nolint:gosec // G115: write refuses files of 4 GiB or more
	return dst
}

// mp4Chunk is a chunk of samples in the source file.
type mp4Chunk struct {
	offset, size int64
	// track and index locate the chunk in its track's chunk offsets.
	track, index int
}

// mp4Track is a kept track.
type mp4Track struct {
	kind          media.AVKind
	codec         string
	width, height int
	samples       int64
	duration      float64 // seconds, from the media header
	stco          *mp4Box
	chunks        []mp4Chunk
}

// mp4Codecs are the accepted sample entries and the codec each one is.
var mp4Codecs = map[string]string{
	"avc1": "h264", "avc3": "h264", "vp08": "vp8", "vp09": "vp9", "av01": "av1",
	"mp4a": "aac", "Opus": "opus", ".mp3": "mp3",
}

// mp4HEVC are the sample entries of HEVC, which is not accepted (ADR-0005 §9).
var mp4HEVC = map[string]bool{"hvc1": true, "hev1": true, "dvh1": true, "dvhe": true}

type mp4File struct {
	ftyp   []byte
	moov   *mp4Box
	mdats  [][2]int64 // payload ranges [start, end)
	tracks []*mp4Track
	probe  media.AVProbe
	cover  []byte
}

// parseMP4 parses an MP4 and filters its moov; remuxMP4 writes it.
func parseMP4(data []byte, limits media.AVLimits) (*mp4File, error) {
	top, err := parseMP4Boxes(data, 0, 0)
	if err != nil {
		return nil, err
	}
	f := &mp4File{}
	for i, b := range top {
		switch b.typ {
		case "ftyp":
			if i != 0 || len(b.payload) < 8 {
				return nil, fmt.Errorf("%w: mp4 ftyp", media.ErrCorrupt)
			}
			f.ftyp = b.payload
		case "moov":
			if f.moov != nil {
				return nil, fmt.Errorf("%w: two mp4 moov boxes", media.ErrCorrupt)
			}
			// The remux changes the moov, never the caller's data.
			b.payload = bytes.Clone(b.payload)
			if f.moov, err = parseMP4Moov(b); err != nil {
				return nil, err
			}
		case "mdat":
			f.mdats = append(f.mdats, [2]int64{int64(b.offset), int64(b.offset + len(b.payload))})
		case "moof", "mfra":
			return nil, fmt.Errorf("%w: fragmented mp4", media.ErrUnsupportedFormat)
		}
	}
	if f.ftyp == nil || f.moov == nil {
		return nil, fmt.Errorf("%w: mp4 without ftyp or moov", media.ErrCorrupt)
	}
	if err := f.read(limits); err != nil {
		return nil, err
	}
	return f, nil
}

// parseMP4Moov parses the children of moov down to the sample tables. Its user data stays a
// leaf, read only for the cover.
func parseMP4Moov(b *mp4Box) (*mp4Box, error) {
	children, err := parseMP4Boxes(b.payload, b.offset, 1)
	if err != nil {
		return nil, err
	}
	b.children = children
	return b, nil
}

// read filters the moov, reads the kept tracks, and checks limits.
func (f *mp4File) read(limits media.AVLimits) error {
	mvhd := f.moov.child("mvhd")
	if mvhd == nil {
		return fmt.Errorf("%w: mp4 without mvhd", media.ErrCorrupt)
	}
	timescale, duration, err := mp4Header(mvhd.payload)
	if err != nil {
		return err
	}
	kept := []*mp4Box{mvhd}
	var audioDropped bool
	for _, c := range f.moov.children {
		switch c.typ {
		case "trak":
			t, err := f.readTrack(c)
			if err != nil {
				return err
			}
			if t == nil {
				continue
			}
			if t.codec == "" {
				audioDropped = true
				continue
			}
			kept = append(kept, c)
			f.tracks = append(f.tracks, t)
		case "udta":
			f.cover = mp4Cover(c.payload)
		case "mvex":
			return fmt.Errorf("%w: fragmented mp4", media.ErrUnsupportedFormat)
		}
	}
	f.moov.children = kept
	p := media.AVProbe{Format: media.FormatMP4, Kind: media.AVAudio, ContentType: "audio/mp4", Extension: "m4a"}
	var trackDuration float64
	for _, t := range f.tracks {
		trackDuration = max(trackDuration, t.duration)
		switch {
		case t.kind == media.AVVideo && p.VideoCodec == "":
			p.Kind, p.ContentType, p.Extension, p.VideoCodec = media.AVVideo, "video/mp4", "mp4", t.codec
			p.Width, p.Height = t.width, t.height
			if t.duration > 0 && float64(t.samples)/t.duration > limits.MaxFrameRate+0.5 {
				return fmt.Errorf("%w: %.1f frames per second", media.ErrDimensions, float64(t.samples)/t.duration)
			}
		case t.kind == media.AVAudio && p.AudioCodec == "":
			p.AudioCodec = t.codec
		}
	}
	if p.VideoCodec == "" && p.AudioCodec == "" {
		if audioDropped {
			return fmt.Errorf("%w: mp4 audio codec", media.ErrUnsupportedCodec)
		}
		return fmt.Errorf("%w: mp4 without video or audio", media.ErrCorrupt)
	}
	seconds := trackDuration
	if timescale > 0 && duration > 0 && duration != math.MaxUint32 && duration != math.MaxUint64 {
		seconds = float64(duration) / float64(timescale)
	}
	p.Duration = time.Duration(seconds * float64(time.Second))
	f.probe = p
	return checkAV(p, limits)
}

// checkAV checks the dimensions and duration of p against limits.
func checkAV(p media.AVProbe, limits media.AVLimits) error {
	if p.Kind == media.AVVideo {
		if p.Width <= 0 || p.Height <= 0 {
			return fmt.Errorf("%w: video of %d×%d", media.ErrCorrupt, p.Width, p.Height)
		}
		if int64(p.Width)*int64(p.Height) > limits.MaxPixels {
			return fmt.Errorf("%w: video of %d×%d, at most %d pixels", media.ErrDimensions, p.Width, p.Height, limits.MaxPixels)
		}
	}
	if p.Duration <= 0 {
		return fmt.Errorf("%w: no duration", media.ErrCorrupt)
	}
	if maxDur := limits.MaxDuration(p.Kind); p.Duration > maxDur {
		return fmt.Errorf("%w: %s, at most %s", media.ErrTooLong, p.Duration, maxDur)
	}
	return nil
}

// mp4Header reads the timescale and duration of an mvhd (movie) or mdhd (media) header and
// zeroes its creation and modification times.
func mp4Header(p []byte) (timescale uint32, duration uint64, err error) {
	if len(p) < 4 {
		return 0, 0, fmt.Errorf("%w: mp4 header", media.ErrCorrupt)
	}
	switch p[0] {
	case 0:
		if len(p) < 20 {
			return 0, 0, fmt.Errorf("%w: mp4 header", media.ErrCorrupt)
		}
		clear(p[4:12])
		return binary.BigEndian.Uint32(p[12:]), uint64(binary.BigEndian.Uint32(p[16:])), nil
	case 1:
		if len(p) < 32 {
			return 0, 0, fmt.Errorf("%w: mp4 header", media.ErrCorrupt)
		}
		clear(p[4:20])
		return binary.BigEndian.Uint32(p[20:]), binary.BigEndian.Uint64(p[24:]), nil
	}
	return 0, 0, fmt.Errorf("%w: mp4 header version %d", media.ErrCorrupt, p[0])
}

// readTrack filters a trak and reads it: nil for a track that is not video or sound, and a
// track without a codec for a sound track whose codec is not accepted (it is dropped). A video
// track whose codec is not accepted fails with media.ErrUnsupportedCodec.
func (f *mp4File) readTrack(trak *mp4Box) (*mp4Track, error) {
	mdia := trak.child("mdia")
	if mdia == nil {
		return nil, nil
	}
	hdlr, mdhd, minf := mdia.child("hdlr"), mdia.child("mdhd"), mdia.child("minf")
	if hdlr == nil || mdhd == nil || minf == nil || len(hdlr.payload) < 12 {
		return nil, nil
	}
	t := &mp4Track{}
	switch string(hdlr.payload[8:12]) {
	case "vide":
		t.kind = media.AVVideo
	case "soun":
		t.kind = media.AVAudio
	default:
		return nil, nil
	}
	timescale, duration, err := mp4Header(mdhd.payload)
	if err != nil {
		return nil, err
	}
	if timescale > 0 {
		t.duration = float64(duration) / float64(timescale)
	}
	stbl := minf.child("stbl")
	if stbl == nil {
		return nil, fmt.Errorf("%w: mp4 track without stbl", media.ErrCorrupt)
	}
	if err := mp4SelfContained(minf.child("dinf")); err != nil {
		return nil, err
	}
	entry, w, h, err := mp4SampleEntry(stbl.child("stsd"))
	if err != nil {
		return nil, err
	}
	switch codec := mp4Codecs[entry]; {
	case mp4HEVC[entry]:
		return nil, fmt.Errorf("%w: hevc", media.ErrUnsupportedCodec)
	case codec == "" && t.kind == media.AVVideo:
		return nil, fmt.Errorf("%w: mp4 video %q", media.ErrUnsupportedCodec, entry)
	case codec == "":
		return t, nil
	case (t.kind == media.AVVideo) != slices.Contains([]string{"h264", "vp8", "vp9", "av1"}, codec):
		return nil, fmt.Errorf("%w: mp4 %s track of %q", media.ErrCorrupt, t.kind, entry)
	default:
		t.codec = codec
	}
	var tkhd *mp4Box
	keptTrak := []*mp4Box{}
	for _, c := range trak.children {
		switch c.typ {
		case "tkhd":
			tkhd = c
			keptTrak = append(keptTrak, c)
		case "edts", "mdia", "tapt":
			keptTrak = append(keptTrak, c)
		}
	}
	if tkhd == nil {
		return nil, fmt.Errorf("%w: mp4 track without tkhd", media.ErrCorrupt)
	}
	trak.children = keptTrak
	mdia.children = slices.DeleteFunc(mdia.children, func(c *mp4Box) bool { return c.typ == "udta" || c.typ == "meta" || c.typ == "uuid" })
	minf.children = slices.DeleteFunc(minf.children, func(c *mp4Box) bool { return c.typ == "udta" || c.typ == "meta" || c.typ == "uuid" })
	if t.kind == media.AVVideo {
		t.width, t.height = w, h
		if tw, th, rotated, ok := mp4TrackSize(tkhd.payload); ok {
			t.width, t.height = tw, th
			if rotated {
				t.width, t.height = th, tw
			}
		}
	} else {
		mp4TrackSize(tkhd.payload) // zeroes the times
	}
	if err := f.readChunks(t, stbl); err != nil {
		return nil, err
	}
	return t, nil
}

// mp4SelfContained fails for a track whose data is in another file (a data reference without
// the self-contained flag).
func mp4SelfContained(dinf *mp4Box) error {
	if dinf == nil {
		return nil
	}
	dref := dinf.child("dref")
	if dref == nil || len(dref.payload) < 8 {
		return nil
	}
	entries, err := parseMP4Boxes(dref.payload[8:], dref.offset+8, mp4MaxDepth)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if len(e.payload) < 4 || e.payload[3]&1 == 0 {
			return fmt.Errorf("%w: mp4 data in another file", media.ErrUnsupportedFormat)
		}
	}
	return nil
}

// mp4SampleEntry returns the type of the first sample entry of an stsd, and for a visual
// entry its width and height.
func mp4SampleEntry(stsd *mp4Box) (string, int, int, error) {
	if stsd == nil || len(stsd.payload) < 16 {
		return "", 0, 0, fmt.Errorf("%w: mp4 stsd", media.ErrCorrupt)
	}
	entry := stsd.payload[8:]
	size := binary.BigEndian.Uint32(entry)
	if size < 8 || int64(size) > int64(len(entry)) {
		return "", 0, 0, fmt.Errorf("%w: mp4 sample entry", media.ErrCorrupt)
	}
	typ := string(entry[4:8])
	var w, h int
	if size >= 36 {
		w, h = int(binary.BigEndian.Uint16(entry[32:])), int(binary.BigEndian.Uint16(entry[34:]))
	}
	return typ, w, h, nil
}

// mp4TrackSize reads the presentation size of a tkhd and whether its matrix turns it a quarter,
// and zeroes its creation and modification times.
func mp4TrackSize(p []byte) (w, h int, rotated, ok bool) {
	var matrix int
	switch {
	case len(p) >= 84 && p[0] == 0:
		clear(p[4:12])
		matrix = 40
	case len(p) >= 96 && p[0] == 1:
		clear(p[4:20])
		matrix = 52
	default:
		return 0, 0, false, false
	}
	a, b := int32(binary.BigEndian.Uint32(p[matrix:])), int32(binary.BigEndian.Uint32(p[matrix+4:])) //nolint:gosec // G115: the matrix holds signed 16.16 fixed-point numbers
	w, h = int(binary.BigEndian.Uint32(p[matrix+36:])>>16), int(binary.BigEndian.Uint32(p[matrix+40:])>>16)
	return w, h, a == 0 && b != 0, w > 0 && h > 0
}

// readChunks reads the chunks of t from its sample table: their offsets (stco, co64), and their
// sizes from the samples per chunk (stsc) and the sample sizes (stsz, stz2). Every chunk must
// lie within an mdat.
func (f *mp4File) readChunks(t *mp4Track, stbl *mp4Box) error {
	offsets, err := mp4ChunkOffsets(stbl, t)
	if err != nil {
		return err
	}
	sizes, constant, count, err := mp4SampleSizes(stbl)
	if err != nil {
		return err
	}
	t.samples = count
	stsc := stbl.child("stsc")
	if stsc == nil || len(stsc.payload) < 8 {
		return fmt.Errorf("%w: mp4 stsc", media.ErrCorrupt)
	}
	n := int(binary.BigEndian.Uint32(stsc.payload[4:]))
	if n < 0 || n > (len(stsc.payload)-8)/12 {
		return fmt.Errorf("%w: mp4 stsc entries", media.ErrCorrupt)
	}
	var sample int64
	for i := range n {
		e := stsc.payload[8+12*i:]
		first, perChunk := int64(binary.BigEndian.Uint32(e)), int64(binary.BigEndian.Uint32(e[4:]))
		last := int64(len(offsets))
		if i+1 < n {
			last = int64(binary.BigEndian.Uint32(stsc.payload[8+12*(i+1):])) - 1
		}
		if first < 1 || last > int64(len(offsets)) || (i > 0 && first <= int64(binary.BigEndian.Uint32(stsc.payload[8+12*(i-1):]))) {
			return fmt.Errorf("%w: mp4 stsc order", media.ErrCorrupt)
		}
		for c := first; c <= last; c++ {
			if perChunk > count-sample {
				return fmt.Errorf("%w: mp4 chunks hold more samples than the track", media.ErrCorrupt)
			}
			var size int64
			if sizes == nil {
				if constant > 0 && perChunk > math.MaxInt32/constant {
					return fmt.Errorf("%w: mp4 chunk too large", media.ErrCorrupt)
				}
				size = perChunk * constant
			} else {
				for _, s := range sizes[sample : sample+perChunk] {
					size += int64(s)
				}
			}
			sample += perChunk
			t.chunks[c-1].size = size
		}
	}
	for _, c := range t.chunks {
		if !slices.ContainsFunc(f.mdats, func(m [2]int64) bool { return c.offset >= m[0] && c.offset+c.size <= m[1] }) {
			return fmt.Errorf("%w: mp4 chunk at %d outside the media data", media.ErrCorrupt, c.offset)
		}
	}
	return nil
}

// mp4ChunkOffsets reads the chunk offsets of t and keeps its stco or co64 to rewrite.
func mp4ChunkOffsets(stbl *mp4Box, t *mp4Track) ([]int64, error) {
	box, width := stbl.child("stco"), 4
	if box == nil {
		box, width = stbl.child("co64"), 8
	}
	if box == nil || len(box.payload) < 8 {
		return nil, fmt.Errorf("%w: mp4 chunk offsets", media.ErrCorrupt)
	}
	n := int64(binary.BigEndian.Uint32(box.payload[4:]))
	if n > int64(len(box.payload)-8)/int64(width) {
		return nil, fmt.Errorf("%w: mp4 chunk offsets cut off", media.ErrCorrupt)
	}
	offsets := make([]int64, n)
	t.chunks = make([]mp4Chunk, n)
	for i := range offsets {
		if width == 4 {
			offsets[i] = int64(binary.BigEndian.Uint32(box.payload[8+4*i:]))
		} else {
			offsets[i] = int64(binary.BigEndian.Uint64(box.payload[8+8*i:]) & math.MaxInt64)
		}
		t.chunks[i] = mp4Chunk{offset: offsets[i], index: i}
	}
	box.typ = "stco"
	t.stco = box
	return offsets, nil
}

// mp4SampleSizes reads the sample sizes of an stsz or stz2: the table, or nil and the size
// of every sample; and the number of samples.
func mp4SampleSizes(stbl *mp4Box) (table []uint32, constant, count int64, err error) {
	if stsz := stbl.child("stsz"); stsz != nil && len(stsz.payload) >= 12 {
		constant, count = int64(binary.BigEndian.Uint32(stsz.payload[4:])), int64(binary.BigEndian.Uint32(stsz.payload[8:]))
		if constant != 0 {
			return nil, constant, count, nil
		}
		if count > int64(len(stsz.payload)-12)/4 {
			return nil, 0, 0, fmt.Errorf("%w: mp4 stsz cut off", media.ErrCorrupt)
		}
		table = make([]uint32, count)
		for i := range table {
			table[i] = binary.BigEndian.Uint32(stsz.payload[12+4*i:])
		}
		return table, 0, count, nil
	}
	stz2 := stbl.child("stz2")
	if stz2 == nil || len(stz2.payload) < 12 {
		return nil, 0, 0, fmt.Errorf("%w: mp4 sample sizes", media.ErrCorrupt)
	}
	field, count := int(stz2.payload[7]), int64(binary.BigEndian.Uint32(stz2.payload[8:]))
	if (field != 4 && field != 8 && field != 16) || count > int64(len(stz2.payload)-12)*8/int64(field) {
		return nil, 0, 0, fmt.Errorf("%w: mp4 stz2", media.ErrCorrupt)
	}
	p := stz2.payload[12:]
	table = make([]uint32, count)
	for i := range table {
		switch field {
		case 4:
			table[i] = uint32(p[i/2]>>(4*(1-i%2))) & 0x0F
		case 8:
			table[i] = uint32(p[i])
		case 16:
			table[i] = uint32(binary.BigEndian.Uint16(p[2*i:]))
		}
	}
	return table, 0, count, nil
}

// mp4Cover returns the cover image of an iTunes-style item list in udta/meta/ilst/covr/data, or
// nil.
func mp4Cover(udta []byte) []byte {
	boxes, err := parseMP4Boxes(udta, 0, mp4MaxDepth)
	if err != nil {
		return nil
	}
	for _, meta := range boxes {
		if meta.typ != "meta" || len(meta.payload) < 8 {
			continue
		}
		p := meta.payload
		if string(p[4:8]) != "hdlr" {
			p = p[4:] // a full box: version and flags first
		}
		items, err := parseMP4Boxes(p, 0, mp4MaxDepth)
		if err != nil {
			continue
		}
		for _, ilst := range items {
			if ilst.typ != "ilst" {
				continue
			}
			entries, err := parseMP4Boxes(ilst.payload, 0, mp4MaxDepth)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.typ != "covr" {
					continue
				}
				data, err := parseMP4Boxes(e.payload, 0, mp4MaxDepth)
				if err == nil && len(data) > 0 && data[0].typ == "data" && len(data[0].payload) > 8 {
					return bytes.Clone(data[0].payload[8:])
				}
			}
		}
	}
	return nil
}

// write writes the file: ftyp, moov, and one mdat of the chunks of the kept tracks in their
// order in the source, with the chunk offsets rewritten.
func (f *mp4File) write(data []byte) ([]byte, error) {
	var chunks []mp4Chunk
	var total int64
	for i, t := range f.tracks {
		for _, c := range t.chunks {
			c.track = i
			chunks = append(chunks, c)
			total += c.size
		}
	}
	if total > int64(len(data)) {
		return nil, fmt.Errorf("%w: mp4 chunks overlap", media.ErrCorrupt)
	}
	slices.SortStableFunc(chunks, func(a, b mp4Chunk) int { return cmp.Compare(a.offset, b.offset) })
	ftyp := f.ftyp
	if string(ftyp[:4]) == "qt  " {
		// A QuickTime movie of the accepted codecs plays as an MP4.
		ftyp = []byte("isom\x00\x00\x02\x00isomiso2mp41")
	}
	for _, t := range f.tracks {
		t.stco.payload = make([]byte, 8+4*len(t.chunks))
		binary.BigEndian.PutUint32(t.stco.payload[4:], uint32(len(t.chunks))) //nolint:gosec // G115: the count was read from a 32-bit field
	}
	head := appendMP4Box(nil, &mp4Box{typ: "ftyp", payload: ftyp})
	moovSize := len(appendMP4Box(nil, f.moov))
	mdatHeader := 8
	if total+8 > math.MaxUint32 {
		mdatHeader = 16
	}
	pos := int64(len(head) + moovSize + mdatHeader)
	if pos+total > math.MaxUint32 {
		return nil, fmt.Errorf("%w: mp4 larger than 4 GiB", media.ErrTooLarge)
	}
	for _, c := range chunks {
		binary.BigEndian.PutUint32(f.tracks[c.track].stco.payload[8+4*c.index:], uint32(pos)) //nolint:gosec // G115: pos stays under 4 GiB, checked above
		pos += c.size
	}
	out := make([]byte, 0, int(pos))
	out = append(out, head...)
	out = appendMP4Box(out, f.moov)
	if mdatHeader == 16 {
		out = binary.BigEndian.AppendUint32(out, 1)
		out = append(out, "mdat"...)
		out = binary.BigEndian.AppendUint64(out, uint64(total+16))
	} else {
		out = binary.BigEndian.AppendUint32(out, uint32(total+8))
		out = append(out, "mdat"...)
	}
	for _, c := range chunks {
		out = append(out, data[c.offset:c.offset+c.size]...)
	}
	return out, nil
}
