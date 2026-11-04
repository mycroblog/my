// SPDX-License-Identifier: AGPL-3.0-only

package native

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/media"
)

// An Ogg of one Vorbis or Opus stream is remuxed by replacing its comment header (Vorbis
// comment, OpusTags), where tags and cover pictures go, with one that has the encoder's vendor
// string and no comments, and by numbering the pages again (ADR-0005 §9). Ogg files of several
// streams (Ogg video) are not accepted.

// oggPage is a page of an Ogg stream.
type oggPage struct {
	flags   byte
	granule uint64
	serial  uint32
	lacing  []byte
	body    []byte
}

// Ogg page flags.
const (
	oggContinued = 0x01
	oggBOS       = 0x02
)

// oggNoGranule is the granule position of a page on which no packet ends.
const oggNoGranule = ^uint64(0)

var oggCRCTable = func() (t [256]uint32) {
	for i := range t {
		r := uint32(i) << 24
		for range 8 {
			if r&0x80000000 != 0 {
				r = r<<1 ^ 0x04C11DB7
			} else {
				r <<= 1
			}
		}
		t[i] = r
	}
	return t
}()

func oggCRC(b []byte) uint32 {
	var crc uint32
	for _, v := range b {
		crc = crc<<8 ^ oggCRCTable[byte(crc>>24)^v]
	}
	return crc
}

func parseOggPages(data []byte) ([]oggPage, error) {
	var pages []oggPage
	for pos := 0; pos < len(data); {
		if len(data)-pos < 27 || string(data[pos:pos+4]) != "OggS" || data[pos+4] != 0 {
			return nil, fmt.Errorf("%w: ogg page at %d", media.ErrCorrupt, pos)
		}
		h := data[pos:]
		n := int(h[26])
		if len(h) < 27+n {
			return nil, fmt.Errorf("%w: ogg page cut off", media.ErrCorrupt)
		}
		lacing := h[27 : 27+n]
		size := 0
		for _, l := range lacing {
			size += int(l)
		}
		if len(h) < 27+n+size {
			return nil, fmt.Errorf("%w: ogg page cut off", media.ErrCorrupt)
		}
		pages = append(pages, oggPage{
			flags: h[5], granule: binary.LittleEndian.Uint64(h[6:]), serial: binary.LittleEndian.Uint32(h[14:]),
			lacing: lacing, body: h[27+n : 27+n+size],
		})
		pos += 27 + n + size
	}
	return pages, nil
}

// appendOggPage appends a page with its sequence number and checksum.
func appendOggPage(dst []byte, p oggPage, seq uint32) []byte {
	start := len(dst)
	dst = append(dst, "OggS\x00"...)
	dst = append(dst, p.flags&0x07)
	dst = binary.LittleEndian.AppendUint64(dst, p.granule)
	dst = binary.LittleEndian.AppendUint32(dst, p.serial)
	dst = binary.LittleEndian.AppendUint32(dst, seq)
	dst = append(dst, 0, 0, 0, 0, byte(len(p.lacing))) //nolint:gosec // G115: a page has at most 255 lacing values
	dst = append(dst, p.lacing...)
	dst = append(dst, p.body...)
	binary.LittleEndian.PutUint32(dst[start+22:], oggCRC(dst[start:]))
	return dst
}

// oggPacketPages lays out one packet on pages of its own.
func oggPacketPages(packet []byte, serial uint32, granule uint64) []oggPage {
	var pages []oggPage
	var flags byte
	for {
		var lacing []byte
		n, rest := 0, len(packet)
		for len(lacing) < 255 && rest >= 255 {
			lacing, n, rest = append(lacing, 255), n+255, rest-255
		}
		end := len(lacing) < 255
		if end {
			lacing, n = append(lacing, byte(rest)), n+rest //nolint:gosec // G115: rest is under 255 here
		}
		p := oggPage{flags: flags, serial: serial, lacing: lacing, body: packet[:n], granule: oggNoGranule}
		if end {
			p.granule = granule
		}
		pages = append(pages, p)
		packet = packet[n:]
		if end {
			return pages
		}
		flags = oggContinued
	}
}

// remuxOgg replaces the comment header of an Ogg Vorbis or Ogg Opus file.
func remuxOgg(data []byte, limits media.AVLimits) (media.AVOutput, error) {
	pages, err := parseOggPages(data)
	if err != nil {
		return media.AVOutput{}, err
	}
	if len(pages) < 2 || pages[0].flags&oggBOS == 0 {
		return media.AVOutput{}, fmt.Errorf("%w: ogg without a stream", media.ErrCorrupt)
	}
	serial := pages[0].serial
	for _, p := range pages[1:] {
		if p.serial != serial || p.flags&oggBOS != 0 {
			return media.AVOutput{}, fmt.Errorf("%w: ogg of several streams", media.ErrUnsupportedFormat)
		}
	}
	// The header packets, which end on a page of their own: Opus and Vorbis start the audio on
	// a new page.
	var packets [][]byte
	var packet []byte
	headers, audio := 0, 0
	for i, p := range pages {
		pos := 0
		for _, l := range p.lacing {
			packet = append(packet, p.body[pos:pos+int(l)]...)
			pos += int(l)
			if l < 255 {
				packets, packet = append(packets, packet), nil
			}
		}
		if len(packets) > 0 && headers == 0 {
			switch {
			case bytes.HasPrefix(packets[0], []byte("OpusHead")):
				headers = 2
			case bytes.HasPrefix(packets[0], []byte("\x01vorbis")):
				headers = 3
			default:
				return media.AVOutput{}, fmt.Errorf("%w: ogg codec", media.ErrUnsupportedCodec)
			}
		}
		if headers > 0 && len(packets) >= headers {
			if len(packets) > headers || packet != nil {
				return media.AVOutput{}, fmt.Errorf("%w: ogg audio on a header page", media.ErrCorrupt)
			}
			audio = i + 1
			break
		}
	}
	if headers == 0 || audio == 0 || audio == len(pages) {
		return media.AVOutput{}, fmt.Errorf("%w: ogg headers", media.ErrCorrupt)
	}
	out := media.AVOutput{AVProbe: media.AVProbe{Format: media.FormatOgg, Kind: media.AVAudio, ContentType: "audio/ogg", Extension: "ogg"}}
	id := packets[0]
	var comment []byte
	var seconds float64
	last := oggNoGranule
	for _, p := range pages[audio:] {
		if p.granule != oggNoGranule {
			last = p.granule
		}
	}
	if last == oggNoGranule {
		return media.AVOutput{}, fmt.Errorf("%w: ogg without a granule position", media.ErrCorrupt)
	}
	if headers == 2 {
		if len(id) < 19 || !bytes.HasPrefix(packets[1], []byte("OpusTags")) {
			return media.AVOutput{}, fmt.Errorf("%w: opus headers", media.ErrCorrupt)
		}
		out.AudioCodec = "opus"
		preSkip := uint64(binary.LittleEndian.Uint16(id[10:]))
		seconds = float64(last-min(last, preSkip)) / 48000
		comment = append([]byte("OpusTags"), oggVendor(packets[1], 8)...)
		comment = binary.LittleEndian.AppendUint32(comment, 0)
	} else {
		if len(id) < 30 || !bytes.HasPrefix(packets[1], []byte("\x03vorbis")) || !bytes.HasPrefix(packets[2], []byte("\x05vorbis")) {
			return media.AVOutput{}, fmt.Errorf("%w: vorbis headers", media.ErrCorrupt)
		}
		out.AudioCodec = "vorbis"
		rate := binary.LittleEndian.Uint32(id[12:])
		if rate == 0 {
			return media.AVOutput{}, fmt.Errorf("%w: vorbis sample rate 0", media.ErrCorrupt)
		}
		seconds = float64(last) / float64(rate)
		comment = append([]byte("\x03vorbis"), oggVendor(packets[1], 7)...)
		comment = binary.LittleEndian.AppendUint32(comment, 0)
		comment = append(comment, 1) // framing bit
	}
	out.Duration = time.Duration(seconds * float64(time.Second))
	if err := checkAV(out.AVProbe, limits); err != nil {
		return media.AVOutput{}, err
	}
	written := []oggPage{{flags: oggBOS, serial: serial, lacing: pages[0].lacing, body: pages[0].body}}
	if len(pages[0].lacing) != 1 || len(pages[0].body) != len(id) {
		written = oggPacketPages(id, serial, 0)
		written[0].flags |= oggBOS
	}
	written = append(written, oggPacketPages(comment, serial, 0)...)
	for _, p := range packets[2:] {
		written = append(written, oggPacketPages(p, serial, 0)...)
	}
	written = append(written, pages[audio:]...)
	buf := make([]byte, 0, len(data))
	for i, p := range written {
		buf = appendOggPage(buf, p, uint32(i))
	}
	out.Data = buf
	return out, nil
}

// oggVendor returns the vendor string of a comment header with its length, which starts at
// offset, or an empty one.
func oggVendor(packet []byte, offset int) []byte {
	if len(packet) >= offset+4 {
		n := int(binary.LittleEndian.Uint32(packet[offset:]))
		if n >= 0 && n <= len(packet)-offset-4 && n <= 1024 {
			return packet[offset : offset+4+n]
		}
	}
	return []byte{0, 0, 0, 0}
}
