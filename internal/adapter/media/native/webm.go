// SPDX-License-Identifier: AGPL-3.0-only

package native

import (
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/media"
)

// A WebM is remuxed by writing its Segment again with the seek index (SeekHead), the segment
// information (Info) without its title and date, the tracks (Tracks) without their names, the
// clusters as they are, and the cue index (Cues); the tags (Tags), attachments (Attachments,
// where cover images go), chapters (Chapters), and padding (Void) are dropped (ADR-0005 §9).
// Dropping only shrinks the file, so every position that the seek index, the cues, and the
// clusters hold is rewritten in the bytes it already has.

// EBML and Matroska element IDs, with their marker bits.
const (
	ebmlHeaderID   = 0x1A45DFA3
	ebmlDocTypeID  = 0x4282
	ebmlVoidID     = 0xEC
	ebmlCRC32ID    = 0xBF
	mkvSegmentID   = 0x18538067
	mkvSeekHeadID  = 0x114D9B74
	mkvSeekID      = 0x4DBB
	mkvSeekIDID    = 0x53AB
	mkvSeekPosID   = 0x53AC
	mkvInfoID      = 0x1549A966
	mkvTimescaleID = 0x2AD7B1
	mkvDurationID  = 0x4489
	mkvTitleID     = 0x7BA9
	mkvDateUTCID   = 0x4461
	mkvTracksID    = 0x1654AE6B
	mkvTrackID     = 0xAE
	mkvTrackNumID  = 0xD7
	mkvTrackTypeID = 0x83
	mkvCodecIDID   = 0x86
	mkvNameID      = 0x536E
	mkvVideoID     = 0xE0
	mkvPixelWID    = 0xB0
	mkvPixelHID    = 0xBA
	mkvDisplayWID  = 0x54B0
	mkvDisplayHID  = 0x54BA
	mkvEncodingsID = 0x6D80
	mkvClusterID   = 0x1F43B675
	mkvTimecodeID  = 0xE7
	mkvPositionID  = 0xA7
	mkvSimpleBlkID = 0xA3
	mkvBlockGrpID  = 0xA0
	mkvBlockID     = 0xA1
	mkvCuesID      = 0x1C53BB6B
	mkvCuePointID  = 0xBB
	mkvCueTrackID  = 0xB7
	mkvCueClustID  = 0xF1
	mkvTagsID      = 0x1254C367
	mkvAttachID    = 0x1941A469
	mkvChaptersID  = 0x1043A770
)

// mkvTopLevel are the IDs that end a Cluster of unknown size.
var mkvTopLevel = []uint32{mkvSeekHeadID, mkvInfoID, mkvTracksID, mkvClusterID, mkvCuesID, mkvTagsID, mkvAttachID, mkvChaptersID, ebmlHeaderID, mkvSegmentID}

// webmCodecs are the accepted codec IDs and the codec each one is.
var webmCodecs = map[string]string{"V_VP8": "vp8", "V_VP9": "vp9", "V_AV1": "av1", "A_OPUS": "opus", "A_VORBIS": "vorbis"}

// ebmlElem is an element of an EBML file, located by absolute offsets.
type ebmlElem struct {
	id               uint32
	start, data, end int // the element, its data, and its end
	idBytes, sizeLen int
	unknown          bool
}

// readEBMLVint reads a variable-length integer at data[pos:]: its length, and its value with
// (size) or without (ID) the length marker. allOnes reports the reserved "unknown" size.
func readEBMLVint(data []byte, pos int, keepMarker bool) (value uint64, n int, allOnes bool, err error) {
	if pos >= len(data) || data[pos] == 0 {
		return 0, 0, false, fmt.Errorf("%w: ebml integer", media.ErrCorrupt)
	}
	n = 1
	for mask := byte(0x80); data[pos]&mask == 0; mask >>= 1 {
		n++
	}
	if pos+n > len(data) {
		return 0, 0, false, fmt.Errorf("%w: ebml integer cut off", media.ErrCorrupt)
	}
	value = uint64(data[pos])
	if !keepMarker {
		value &= 0xFF >> n
	}
	allOnes = value == uint64(0xFF>>n)
	for _, b := range data[pos+1 : pos+n] {
		value = value<<8 | uint64(b)
		allOnes = allOnes && b == 0xFF
	}
	return value, n, allOnes, nil
}

// readEBMLElem reads the header of the element at pos, which must end by limit.
func readEBMLElem(data []byte, pos, limit int) (ebmlElem, error) {
	id, idLen, _, err := readEBMLVint(data[:limit], pos, true)
	if err != nil || idLen > 4 {
		return ebmlElem{}, fmt.Errorf("%w: ebml id", media.ErrCorrupt)
	}
	size, sizeLen, unknown, err := readEBMLVint(data[:limit], pos+idLen, false)
	if err != nil {
		return ebmlElem{}, err
	}
	e := ebmlElem{id: uint32(id), start: pos, data: pos + idLen + sizeLen, idBytes: idLen, sizeLen: sizeLen, unknown: unknown} //nolint:gosec // G115: an ID has at most 4 bytes, checked above
	if unknown {
		e.end = limit
		return e, nil
	}
	if size > uint64(limit-e.data) { //nolint:gosec // G115: e.data is at most limit
		return ebmlElem{}, fmt.Errorf("%w: ebml element %X of %d bytes", media.ErrCorrupt, id, size)
	}
	e.end = e.data + int(size) //nolint:gosec // G115: size is at most limit-e.data, checked above
	return e, nil
}

// ebmlChildren reads the children of a master element of known size.
func ebmlChildren(data []byte, parent ebmlElem) ([]ebmlElem, error) {
	var out []ebmlElem
	for pos := parent.data; pos < parent.end; {
		e, err := readEBMLElem(data, pos, parent.end)
		if err != nil {
			return nil, err
		}
		if e.unknown {
			return nil, fmt.Errorf("%w: ebml element %X of unknown size", media.ErrCorrupt, e.id)
		}
		out = append(out, e)
		pos = e.end
	}
	return out, nil
}

func ebmlUint(data []byte, e ebmlElem) uint64 {
	var v uint64
	for _, b := range data[e.data:e.end] {
		v = v<<8 | uint64(b)
	}
	return v
}

func ebmlFloat(data []byte, e ebmlElem) float64 {
	switch e.end - e.data {
	case 4:
		return float64(math.Float32frombits(binary.BigEndian.Uint32(data[e.data:])))
	case 8:
		return math.Float64frombits(binary.BigEndian.Uint64(data[e.data:]))
	}
	return 0
}

// appendEBMLSize appends size as a vint of width bytes, or of the fewest bytes when it does not
// fit in width.
func appendEBMLSize(dst []byte, size uint64, width int) []byte {
	for width < 8 && size >= 1<<(7*width)-1 {
		width++
	}
	v := size | 1<<(7*width)
	for i := width - 1; i >= 0; i-- {
		dst = append(dst, byte(v>>(8*i))) //nolint:gosec // G115: one byte of v at a time
	}
	return dst
}

// webmPatch is a position to rewrite in the output: width bytes at offset hold the
// segment-relative position old.
type webmPatch struct {
	offset, width int
	old           uint64
}

// webmPart is a kept top-level element as written: its bytes, and the positions in it.
type webmPart struct {
	oldStart, oldEnd int // relative to the segment's data
	out              []byte
	patches          []webmPatch
}

type webmTrack struct {
	number                      uint64
	kind                        media.AVKind
	codec                       string
	width, height               int
	displayWidth, displayHeight int
}

type webmFile struct {
	data      []byte
	segData   int // absolute offset of the segment's data
	head      []byte
	segHeader ebmlElem
	parts     []webmPart
	timescale uint64
	duration  float64 // in timescale units, 0 when the Info has none
	tracks    []webmTrack
	maxTime   int64 // the latest block timestamp, in timescale units
	frames    int64 // blocks of the video track
	probe     media.AVProbe
}

// parseWebM parses a WebM and prepares its kept elements; write puts them together.
func parseWebM(data []byte, limits media.AVLimits) (*webmFile, error) {
	header, err := readEBMLElem(data, 0, len(data))
	if err != nil || header.id != ebmlHeaderID || header.unknown {
		return nil, fmt.Errorf("%w: ebml header", media.ErrCorrupt)
	}
	fields, err := ebmlChildren(data, header)
	if err != nil {
		return nil, err
	}
	if i := slices.IndexFunc(fields, func(e ebmlElem) bool { return e.id == ebmlDocTypeID }); i < 0 || string(data[fields[i].data:fields[i].end]) != "webm" {
		return nil, fmt.Errorf("%w: matroska that is not webm", media.ErrUnsupportedFormat)
	}
	seg, err := readEBMLElem(data, header.end, len(data))
	if err != nil || seg.id != mkvSegmentID {
		return nil, fmt.Errorf("%w: webm without a segment", media.ErrCorrupt)
	}
	f := &webmFile{data: data, segData: seg.data, head: data[:header.end], segHeader: seg, timescale: 1_000_000}
	for pos := seg.data; pos < seg.end; {
		e, err := readEBMLElem(data, pos, seg.end)
		if err != nil {
			return nil, err
		}
		if e.unknown {
			if e.id != mkvClusterID {
				return nil, fmt.Errorf("%w: webm element %X of unknown size", media.ErrCorrupt, e.id)
			}
			if e.end, err = f.clusterEnd(e, seg.end); err != nil {
				return nil, err
			}
		}
		if err := f.add(e); err != nil {
			return nil, err
		}
		pos = e.end
	}
	if err := f.read(limits); err != nil {
		return nil, err
	}
	return f, nil
}

// clusterEnd finds the end of a Cluster of unknown size: the next top-level element.
func (f *webmFile) clusterEnd(c ebmlElem, limit int) (int, error) {
	pos := c.data
	for pos < limit {
		id, n, _, err := readEBMLVint(f.data[:limit], pos, true)
		if err != nil || n > 4 {
			return 0, fmt.Errorf("%w: webm cluster", media.ErrCorrupt)
		}
		if slices.Contains(mkvTopLevel, uint32(id)) { //nolint:gosec // G115: an ID has at most 4 bytes, checked above
			break
		}
		e, err := readEBMLElem(f.data, pos, limit)
		if err != nil || e.unknown {
			return 0, fmt.Errorf("%w: webm cluster", media.ErrCorrupt)
		}
		pos = e.end
	}
	return pos, nil
}

// rel returns an absolute offset relative to the segment's data.
func (f *webmFile) rel(abs int) int { return abs - f.segData }

// add prepares a top-level element of the segment.
func (f *webmFile) add(e ebmlElem) error {
	part := webmPart{oldStart: f.rel(e.start), oldEnd: f.rel(e.end)}
	var err error
	switch e.id {
	case mkvSeekHeadID:
		err = f.seekHead(e, &part)
	case mkvInfoID:
		err = f.info(e, &part)
	case mkvTracksID:
		err = f.readTracks(e, &part)
	case mkvClusterID:
		err = f.cluster(e, &part)
	case mkvCuesID:
		err = f.cues(e, &part)
	default:
		return nil // Tags, Attachments, Chapters, Void, CRC-32, and unknown elements
	}
	if err != nil {
		return err
	}
	f.parts = append(f.parts, part)
	return nil
}

// master writes a master element of kept children: each child either as it is, or through fn,
// which appends it and returns false to drop it. The size keeps its width, which still fits:
// children only shrink.
func (f *webmFile) master(e ebmlElem, part *webmPart, fn func(c ebmlElem, out []byte) ([]byte, bool, error)) ([]byte, error) {
	children, err := ebmlChildren(f.data, e)
	if err != nil {
		return nil, err
	}
	var body []byte
	for _, c := range children {
		if c.id == ebmlVoidID || c.id == ebmlCRC32ID {
			continue
		}
		out, keep, err := fn(c, body)
		if err != nil {
			return nil, err
		}
		if keep {
			body = out
		}
	}
	head := append([]byte(nil), f.data[e.start:e.start+e.idBytes]...)
	head = appendEBMLSize(head, uint64(len(body)), e.sizeLen)
	for i := range part.patches {
		part.patches[i].offset += len(head)
	}
	return append(head, body...), nil
}

func (f *webmFile) raw(c ebmlElem, out []byte) []byte { return append(out, f.data[c.start:c.end]...) }

// seekHead keeps the Seek entries of kept elements, and their positions to rewrite.
func (f *webmFile) seekHead(e ebmlElem, part *webmPart) error {
	var err error
	part.out, err = f.master(e, part, func(seek ebmlElem, out []byte) ([]byte, bool, error) {
		if seek.id != mkvSeekID {
			return f.raw(seek, out), true, nil
		}
		fields, err := ebmlChildren(f.data, seek)
		if err != nil {
			return nil, false, err
		}
		var target []byte
		var pos ebmlElem
		for _, c := range fields {
			switch c.id {
			case mkvSeekIDID:
				target = f.data[c.data:c.end]
			case mkvSeekPosID:
				pos = c
			}
		}
		var id uint32
		for _, b := range target {
			id = id<<8 | uint32(b)
		}
		if pos.end == 0 || !slices.Contains([]uint32{mkvSeekHeadID, mkvInfoID, mkvTracksID, mkvClusterID, mkvCuesID}, id) {
			return nil, false, nil
		}
		start := len(out)
		out = f.raw(seek, out)
		part.patches = append(part.patches, webmPatch{offset: start + (pos.data - seek.start), width: pos.end - pos.data, old: ebmlUint(f.data, pos)})
		return out, true, nil
	})
	return err
}

// info keeps the segment information without its title and date, and reads the timescale and
// the duration.
func (f *webmFile) info(e ebmlElem, part *webmPart) error {
	var err error
	part.out, err = f.master(e, part, func(c ebmlElem, out []byte) ([]byte, bool, error) {
		switch c.id {
		case mkvTitleID, mkvDateUTCID:
			return nil, false, nil
		case mkvTimescaleID:
			if f.timescale = ebmlUint(f.data, c); f.timescale == 0 {
				return nil, false, fmt.Errorf("%w: webm timescale 0", media.ErrCorrupt)
			}
		case mkvDurationID:
			f.duration = ebmlFloat(f.data, c)
		}
		return f.raw(c, out), true, nil
	})
	return err
}

// readTracks keeps the tracks without their names and reads their codecs and sizes.
func (f *webmFile) readTracks(e ebmlElem, part *webmPart) error {
	var err error
	part.out, err = f.master(e, part, func(entry ebmlElem, out []byte) ([]byte, bool, error) {
		if entry.id != mkvTrackID {
			return f.raw(entry, out), true, nil
		}
		var t webmTrack
		var codec string
		var trackType uint64
		body, err := f.master(entry, &webmPart{}, func(c ebmlElem, out []byte) ([]byte, bool, error) {
			switch c.id {
			case mkvNameID:
				return nil, false, nil
			case mkvEncodingsID:
				return nil, false, fmt.Errorf("%w: webm content encoding", media.ErrUnsupportedFormat)
			case mkvTrackNumID:
				t.number = ebmlUint(f.data, c)
			case mkvTrackTypeID:
				trackType = ebmlUint(f.data, c)
			case mkvCodecIDID:
				codec = string(f.data[c.data:c.end])
			case mkvVideoID:
				fields, err := ebmlChildren(f.data, c)
				if err != nil {
					return nil, false, err
				}
				for _, v := range fields {
					switch v.id {
					case mkvPixelWID:
						t.width = int(min(ebmlUint(f.data, v), math.MaxInt32))
					case mkvPixelHID:
						t.height = int(min(ebmlUint(f.data, v), math.MaxInt32))
					case mkvDisplayWID:
						t.displayWidth = int(min(ebmlUint(f.data, v), math.MaxInt32))
					case mkvDisplayHID:
						t.displayHeight = int(min(ebmlUint(f.data, v), math.MaxInt32))
					}
				}
			}
			return f.raw(c, out), true, nil
		})
		if err != nil {
			return nil, false, err
		}
		switch trackType {
		case 1:
			t.kind = media.AVVideo
		case 2:
			t.kind = media.AVAudio
		default:
			return nil, false, fmt.Errorf("%w: webm track of type %d", media.ErrUnsupportedFormat, trackType)
		}
		t.codec = webmCodecs[codec]
		if t.codec == "" || (t.kind == media.AVVideo) != (codec[0] == 'V') {
			return nil, false, fmt.Errorf("%w: webm codec %q", media.ErrUnsupportedCodec, codec)
		}
		f.tracks = append(f.tracks, t)
		return append(out, body...), true, nil
	})
	return err
}

// cluster keeps a cluster as it is, with its position to rewrite, and reads the timestamps of
// its blocks.
func (f *webmFile) cluster(e ebmlElem, part *webmPart) error {
	part.out = f.data[e.start:e.end]
	if e.unknown {
		e.unknown = false // its end is known now
	}
	children, err := ebmlChildren(f.data, e)
	if err != nil {
		return err
	}
	var timecode uint64
	video := uint64(0)
	if i := slices.IndexFunc(f.tracks, func(t webmTrack) bool { return t.kind == media.AVVideo }); i >= 0 {
		video = f.tracks[i].number
	}
	block := func(b ebmlElem) error {
		track, n, _, err := readEBMLVint(f.data[:b.end], b.data, false)
		if err != nil || b.data+n+2 > b.end {
			return fmt.Errorf("%w: webm block", media.ErrCorrupt)
		}
		rel := int16(binary.BigEndian.Uint16(f.data[b.data+n:])) //nolint:gosec // G115: the block's timestamp is a signed 16-bit number
		f.maxTime = max(f.maxTime, int64(min(timecode, math.MaxInt32))+int64(rel))
		if video != 0 && track == video {
			f.frames++
		}
		return nil
	}
	for _, c := range children {
		switch c.id {
		case mkvTimecodeID:
			timecode = ebmlUint(f.data, c)
		case mkvPositionID:
			part.patches = append(part.patches, webmPatch{offset: c.data - e.start, width: c.end - c.data, old: ebmlUint(f.data, c)})
		case mkvSimpleBlkID:
			if err := block(c); err != nil {
				return err
			}
		case mkvBlockGrpID:
			fields, err := ebmlChildren(f.data, c)
			if err != nil {
				return err
			}
			for _, b := range fields {
				if b.id == mkvBlockID {
					if err := block(b); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// cues keeps the cue index with the cluster positions to rewrite.
func (f *webmFile) cues(e ebmlElem, part *webmPart) error {
	children, err := ebmlChildren(f.data, e)
	if err != nil {
		return err
	}
	part.out = f.data[e.start:e.end]
	for _, point := range children {
		if point.id != mkvCuePointID {
			continue
		}
		fields, err := ebmlChildren(f.data, point)
		if err != nil {
			return err
		}
		for _, tp := range fields {
			if tp.id != mkvCueTrackID {
				continue
			}
			positions, err := ebmlChildren(f.data, tp)
			if err != nil {
				return err
			}
			for _, c := range positions {
				if c.id == mkvCueClustID {
					part.patches = append(part.patches, webmPatch{offset: c.data - e.start, width: c.end - c.data, old: ebmlUint(f.data, c)})
				}
			}
		}
	}
	return nil
}

// read checks the tracks, the duration, and the limits.
func (f *webmFile) read(limits media.AVLimits) error {
	if len(f.tracks) == 0 {
		return fmt.Errorf("%w: webm without tracks", media.ErrCorrupt)
	}
	p := media.AVProbe{Format: media.FormatWebM, Kind: media.AVAudio, ContentType: "audio/webm", Extension: "webm"}
	for _, t := range f.tracks {
		switch {
		case t.kind == media.AVVideo && p.VideoCodec == "":
			p.Kind, p.ContentType, p.VideoCodec = media.AVVideo, "video/webm", t.codec
			p.Width, p.Height = t.width, t.height
			if t.displayWidth > 0 && t.displayHeight > 0 {
				p.Width, p.Height = t.displayWidth, t.displayHeight
			}
		case t.kind == media.AVAudio && p.AudioCodec == "":
			p.AudioCodec = t.codec
		}
	}
	units := f.duration
	if units <= 0 {
		units = float64(f.maxTime)
	}
	seconds := units * float64(f.timescale) / 1e9
	if math.IsNaN(seconds) || seconds > math.MaxInt64/float64(time.Second) {
		return fmt.Errorf("%w: webm duration", media.ErrCorrupt)
	}
	p.Duration = time.Duration(seconds * float64(time.Second))
	if p.Kind == media.AVVideo && seconds > 0 && float64(f.frames)/seconds > limits.MaxFrameRate+0.5 {
		return fmt.Errorf("%w: %.1f frames per second", media.ErrDimensions, float64(f.frames)/seconds)
	}
	f.probe = p
	return checkAV(p, limits)
}

// write puts the kept elements together and rewrites the positions.
func (f *webmFile) write() ([]byte, error) {
	starts := make([]int, len(f.parts))
	var size int
	for i, p := range f.parts {
		starts[i] = size
		size += len(p.out)
	}
	moved := func(old uint64) uint64 {
		for i, p := range f.parts {
			if old < uint64(p.oldEnd) { //nolint:gosec // G115: offsets are not negative
				if old < uint64(p.oldStart) { //nolint:gosec // G115: offsets are not negative
					return uint64(starts[i]) //nolint:gosec // G115: offsets are not negative
				}
				return uint64(starts[i]) + old - uint64(p.oldStart) //nolint:gosec // G115: offsets are not negative
			}
		}
		return uint64(size) //nolint:gosec // G115: sizes are not negative
	}
	out := make([]byte, 0, len(f.head)+12+size)
	out = append(out, f.head...)
	out = append(out, f.data[f.segHeader.start:f.segHeader.start+f.segHeader.idBytes]...)
	if f.segHeader.unknown {
		out = append(out, f.data[f.segHeader.start+f.segHeader.idBytes:f.segHeader.data]...)
	} else {
		out = appendEBMLSize(out, uint64(size), max(f.segHeader.sizeLen, 8)) //nolint:gosec // G115: sizes are not negative
	}
	for _, p := range f.parts {
		start := len(out)
		out = append(out, p.out...)
		for _, patch := range p.patches {
			v := moved(patch.old)
			if patch.width < 8 && v >= 1<<(8*patch.width) {
				return nil, fmt.Errorf("%w: webm position does not fit", media.ErrCorrupt)
			}
			for i := range patch.width {
				out[start+patch.offset+i] = byte(v >> (8 * (patch.width - 1 - i))) //nolint:gosec // G115: one byte of v at a time
			}
		}
	}
	return out, nil
}
