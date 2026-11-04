// SPDX-License-Identifier: AGPL-3.0-only

package native

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/media"
)

// An MP3 is remuxed by keeping only its MPEG audio Layer III frames: the ID3v2 tags in front,
// the APE and ID3v1 tags at the end, and anything between frames are dropped (ADR-0005 §9).
// The cover picture (APIC) of the ID3v2 tag is kept aside as the cover.

// mp3Bitrates are the bitrates of Layer III in kbit/s by MPEG version 1 and 2 (2.5 too) and
// bitrate index.
var mp3Bitrates = [2][16]int{
	{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0},
	{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
}

// mp3SampleRates are the sample rates by MPEG version (1, 2, 2.5) and index.
var mp3SampleRates = [3][3]int{{44100, 48000, 32000}, {22050, 24000, 16000}, {11025, 12000, 8000}}

// mp3Frame reads the header of a Layer III frame at data[pos:]: its length and its samples and
// sample rate; ok is false when there is none.
func mp3Frame(data []byte, pos int) (length, samples, rate int, ok bool) {
	if len(data)-pos < 4 || data[pos] != 0xFF || data[pos+1]&0xE0 != 0xE0 {
		return 0, 0, 0, false
	}
	h := binary.BigEndian.Uint32(data[pos:])
	version, layer := (h>>19)&3, (h>>17)&3
	bitrateIndex, rateIndex, padding := (h>>12)&0xF, (h>>10)&3, int((h>>9)&1)
	if version == 1 || layer != 1 || rateIndex == 3 {
		return 0, 0, 0, false // reserved version, not Layer III, reserved rate
	}
	var v, bv int
	switch version {
	case 3: // MPEG 1
		v, bv, samples = 0, 0, 1152
	case 2: // MPEG 2
		v, bv, samples = 1, 1, 576
	default: // MPEG 2.5
		v, bv, samples = 2, 1, 576
	}
	bitrate := mp3Bitrates[bv][bitrateIndex] * 1000
	if bitrate == 0 {
		return 0, 0, 0, false // free format or bad
	}
	rate = mp3SampleRates[v][rateIndex]
	length = samples/8*bitrate/rate + padding
	if length < 4 || pos+length > len(data) {
		return 0, 0, 0, false
	}
	return length, samples, rate, true
}

// id3v2Size returns the length of an ID3v2 tag at data[pos:], or 0 when there is none.
func id3v2Size(data []byte, pos int) int {
	if len(data)-pos < 10 || string(data[pos:pos+3]) != "ID3" {
		return 0
	}
	h := data[pos:]
	if h[6]|h[7]|h[8]|h[9] >= 0x80 {
		return 0
	}
	size := int(h[6])<<21 | int(h[7])<<14 | int(h[8])<<7 | int(h[9]) + 10
	if h[5]&0x10 != 0 {
		size += 10 // footer
	}
	return min(size, len(data)-pos)
}

// remuxMP3 keeps the Layer III frames of data.
func remuxMP3(data []byte, limits media.AVLimits) (media.AVOutput, error) {
	out := media.AVOutput{AVProbe: media.AVProbe{Format: media.FormatMP3, Kind: media.AVAudio, ContentType: "audio/mpeg", Extension: "mp3", AudioCodec: "mp3"}}
	end := len(data)
	if end >= 128 && string(data[end-128:end-125]) == "TAG" {
		end -= 128
	}
	if end >= 32 && string(data[end-32:end-24]) == "APETAGEX" {
		size := int(binary.LittleEndian.Uint32(data[end-20:]))
		if binary.LittleEndian.Uint32(data[end-12:])&0x80000000 != 0 {
			size += 32 // header
		}
		end = max(0, end-size)
	}
	data = data[:end]
	frames := make([]byte, 0, len(data))
	var seconds float64
	var rate int
	for pos := 0; pos < len(data); {
		if n := id3v2Size(data, pos); n > 0 {
			if out.Cover == nil {
				out.Cover = id3Cover(data[pos : pos+n])
			}
			pos += n
			continue
		}
		length, samples, r, ok := mp3Frame(data, pos)
		if ok && rate != 0 && r != rate {
			ok = false
		}
		if ok && pos+length < len(data) {
			// A frame must be followed by another, the end, or a tag: a sync pattern
			// inside other data is not a frame.
			_, _, _, next := mp3Frame(data, pos+length)
			ok = next || id3v2Size(data, pos+length) > 0 || len(data)-(pos+length) < 4
		}
		if !ok {
			pos++
			continue
		}
		rate = r
		frames = append(frames, data[pos:pos+length]...)
		seconds += float64(samples) / float64(r)
		pos += length
	}
	if len(frames) == 0 {
		return media.AVOutput{}, fmt.Errorf("%w: no mp3 frames", media.ErrCorrupt)
	}
	out.Duration = time.Duration(seconds * float64(time.Second))
	if err := checkAV(out.AVProbe, limits); err != nil {
		return media.AVOutput{}, err
	}
	out.Data = frames
	return out, nil
}

// id3Cover returns the picture of the first APIC frame of an ID3v2.3 or ID3v2.4 tag, or nil.
func id3Cover(tag []byte) []byte {
	if len(tag) < 10 {
		return nil
	}
	version, flags := tag[3], tag[5]
	if (version != 3 && version != 4) || flags&0x80 != 0 { // unsynchronisation is not read
		return nil
	}
	body := tag[10:]
	if flags&0x40 != 0 && len(body) >= 4 { // extended header
		n := int(binary.BigEndian.Uint32(body))
		if version == 4 {
			n = syncsafe(body)
		} else {
			n += 4
		}
		if n < 0 || n > len(body) {
			return nil
		}
		body = body[n:]
	}
	for len(body) >= 10 && body[0] != 0 {
		size := int(binary.BigEndian.Uint32(body[4:]))
		if version == 4 {
			size = syncsafe(body[4:])
		}
		if size < 0 || size > len(body)-10 {
			return nil
		}
		frame := body[10 : 10+size]
		if string(body[:4]) == "APIC" && body[9]&0x4F == 0 { // not compressed, encrypted, or unsynchronised
			return apicPicture(frame)
		}
		body = body[10+size:]
	}
	return nil
}

// utf16End returns the offset after the two zero bytes that end a UTF-16 string at the start of b.
func utf16End(b []byte) int {
	i := 0
	for i+1 < len(b) && (b[i] != 0 || b[i+1] != 0) {
		i += 2
	}
	return i + 2
}

func syncsafe(b []byte) int {
	return int(b[0]&0x7F)<<21 | int(b[1]&0x7F)<<14 | int(b[2]&0x7F)<<7 | int(b[3]&0x7F)
}

// apicPicture returns the picture data of an APIC frame: after the text encoding, the MIME
// type, the picture type, and the description.
func apicPicture(frame []byte) []byte {
	if len(frame) < 2 {
		return nil
	}
	encoding, rest := frame[0], frame[1:]
	i := bytes.IndexByte(rest, 0)
	if i < 0 || i+2 > len(rest) {
		return nil
	}
	rest = rest[i+2:] // the MIME type, its terminator, and the picture type
	if encoding == 1 || encoding == 2 {
		i = utf16End(rest)
	} else {
		i = bytes.IndexByte(rest, 0) + 1
	}
	if i <= 0 || i > len(rest) {
		return nil
	}
	return bytes.Clone(rest[i:])
}
