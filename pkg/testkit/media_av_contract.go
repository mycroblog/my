// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/media"
)

// mediaAVCases are the cases of MediaProcessorContract for video and audio files
// (ADR-0054 §6).
var mediaAVCases = []struct {
	name string
	run  func(t *testing.T, p media.Processor)
}{
	{"RemuxesMP4WithoutMetadata", mediaRemuxesMP4},
	{"PlaysQuickTimeMoviesAsMP4", mediaRemuxesQuickTime},
	{"RemuxesM4AAudio", mediaRemuxesM4A},
	{"RemuxesWebMWithoutMetadata", mediaRemuxesWebM},
	{"ReadsTheDurationOfWebMFromItsBlocks", mediaWebMWithoutDuration},
	{"RemuxesMP3WithoutTags", mediaRemuxesMP3},
	{"RemuxesOggWithoutComments", mediaRemuxesOgg},
	{"RejectsUnsupportedCodecs", mediaRejectsCodecs},
	{"RejectsVideoAndAudioOverTheLimits", mediaRejectsAVLimits},
	{"RejectsBrokenContainers", mediaRejectsBrokenAV},
	{"InspectsAsItRemuxes", mediaInspectsAV},
}

func remux(t *testing.T, p media.Processor, data []byte) media.AVOutput {
	t.Helper()
	out, err := p.Remux(t.Context(), data, media.AVLimits{})
	if err != nil {
		t.Fatalf("Remux: %v", err)
	}
	again, err := p.Remux(t.Context(), data, media.AVLimits{})
	if err != nil || !bytes.Equal(again.Data, out.Data) {
		t.Errorf("a second Remux gave other output (%v)", err)
	}
	if bytes.Contains(out.Data, []byte(AVSecret)) {
		t.Errorf("the remuxed file keeps metadata %q", AVSecret)
	}
	return out
}

func checkProbe(t *testing.T, got media.AVProbe, want media.AVProbe) {
	t.Helper()
	d := got.Duration - want.Duration
	if got.Kind != want.Kind || got.ContentType != want.ContentType || got.Extension != want.Extension ||
		got.Width != want.Width || got.Height != want.Height || got.VideoCodec != want.VideoCodec ||
		got.AudioCodec != want.AudioCodec || got.Format != want.Format || d < -50*time.Millisecond || d > 50*time.Millisecond {
		t.Errorf("probe = %+v, want %+v", got, want)
	}
}

// mp4Find returns the payloads of the boxes of type typ anywhere in the containers of data.
func mp4Find(data []byte, typ string) [][]byte {
	var out [][]byte
	for len(data) >= 8 {
		size := int(binary.BigEndian.Uint32(data))
		if size < 8 || size > len(data) {
			break
		}
		t, payload := string(data[4:8]), data[8:size]
		if t == typ {
			out = append(out, payload)
		}
		switch t {
		case "moov", "trak", "mdia", "minf", "stbl":
			out = append(out, mp4Find(payload, typ)...)
		}
		data = data[size:]
	}
	return out
}

func mediaRemuxesMP4(t *testing.T, p media.Processor) {
	cover := FixturePNG(8, 8, false)
	src := MP4File(MP4Options{VideoCodec: "avc1", AudioCodec: "mp4a", Width: 640, Height: 360, Frames: 30, Seconds: 1, Cover: cover})
	out := remux(t, p, src)
	checkProbe(t, out.AVProbe, media.AVProbe{
		Format: media.FormatMP4, Kind: media.AVVideo, ContentType: "video/mp4", Extension: "mp4",
		Width: 640, Height: 360, Duration: time.Second, VideoCodec: "h264", AudioCodec: "aac",
	})
	if !bytes.Equal(out.Cover, cover) {
		t.Errorf("Cover = %d bytes, want the covr image", len(out.Cover))
	}
	for _, marker := range []string{"\xA9xyz", "+35.6586", "uuid", "udta"} {
		if bytes.Contains(out.Data, []byte(marker)) {
			t.Errorf("the remuxed file keeps %q", marker)
		}
	}
	if bytes.Contains(out.Data, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Error("the remuxed file keeps the creation time")
	}
	if moov, mdat := bytes.Index(out.Data, []byte("moov")), bytes.Index(out.Data, []byte("mdat")); moov < 0 || mdat < moov {
		t.Errorf("moov at %d, mdat at %d: want moov first (fast start)", moov, mdat)
	}
	stcos := mp4Find(out.Data, "stco")
	if len(stcos) != 2 {
		t.Fatalf("%d stco boxes, want 2", len(stcos))
	}
	for i, prefix := range []string{"FRAME", "AUDIO"} {
		stco := stcos[i]
		n := int(binary.BigEndian.Uint32(stco[4:]))
		for c := range n {
			off := int(binary.BigEndian.Uint32(stco[8+4*c:]))
			want := fmt.Sprintf("%s%d", prefix, c)
			if off+len(want) > len(out.Data) || string(out.Data[off:off+len(want)]) != want {
				t.Errorf("chunk %d of track %d points at the wrong data, want %q", c, i, want)
				break
			}
		}
	}
}

func mediaRemuxesQuickTime(t *testing.T, p media.Processor) {
	out := remux(t, p, MP4File(MP4Options{VideoCodec: "avc1", Width: 1920, Height: 1080, Rotated: true, Frames: 30, Seconds: 1, QuickTime: true}))
	checkProbe(t, out.AVProbe, media.AVProbe{
		Format: media.FormatMP4, Kind: media.AVVideo, ContentType: "video/mp4", Extension: "mp4",
		Width: 1080, Height: 1920, Duration: time.Second, VideoCodec: "h264",
	})
	if string(out.Data[8:12]) == "qt  " {
		t.Error("the remuxed movie keeps the QuickTime brand")
	}
}

func mediaRemuxesM4A(t *testing.T, p media.Processor) {
	out := remux(t, p, MP4File(MP4Options{AudioCodec: "mp4a", Seconds: 2}))
	checkProbe(t, out.AVProbe, media.AVProbe{
		Format: media.FormatMP4, Kind: media.AVAudio, ContentType: "audio/mp4", Extension: "m4a", Duration: 2 * time.Second, AudioCodec: "aac",
	})
}

// webmPositions returns the values of the elements id (two bytes) with an 8-byte size and an
// 8-byte value, as WebMFile writes them.
func webmPositions(data []byte, id string) []int {
	var out []int
	for i := 0; ; {
		j := bytes.Index(data[i:], []byte(id+"\x01\x00\x00\x00\x00\x00\x00\x08"))
		if j < 0 {
			return out
		}
		i += j + len(id) + 8
		out = append(out, int(binary.BigEndian.Uint64(data[i:]))) //nolint:gosec // G115: positions in a small test file
	}
}

func mediaRemuxesWebM(t *testing.T, p media.Processor) {
	out := remux(t, p, WebMFile(WebMOptions{VideoCodec: "V_VP9", AudioCodec: "A_OPUS", Width: 640, Height: 360, Frames: 30, Seconds: 1}))
	checkProbe(t, out.AVProbe, media.AVProbe{
		Format: media.FormatWebM, Kind: media.AVVideo, ContentType: "video/webm", Extension: "webm",
		Width: 640, Height: 360, Duration: time.Second, VideoCodec: "vp9", AudioCodec: "opus",
	})
	seg := bytes.Index(out.Data, []byte("\x18\x53\x80\x67"))
	if seg < 0 {
		t.Fatal("no segment")
	}
	// The segment's data starts after its ID and its size.
	sizeLen := 1
	for mask := byte(0x80); out.Data[seg+4]&mask == 0; mask >>= 1 {
		sizeLen++
	}
	data := seg + 4 + sizeLen
	ids := map[string]bool{}
	for _, pos := range webmPositions(out.Data, "\x53\xAC") {
		if data+pos+4 > len(out.Data) {
			t.Fatalf("seek position %d past the end", pos)
		}
		ids[fmt.Sprintf("%X", out.Data[data+pos:data+pos+4])] = true
	}
	for _, id := range []string{"1549A966", "1654AE6B", "1C53BB6B"} {
		if !ids[id] {
			t.Errorf("the seek index does not point at %s (points at %v)", id, ids)
		}
	}
	if ids["1254C367"] {
		t.Error("the seek index points at the dropped tags")
	}
	cues := webmPositions(out.Data, "\xF1")
	if len(cues) != 1 || string(out.Data[data+cues[0]:data+cues[0]+4]) != "\x1F\x43\xB6\x75" {
		t.Errorf("cue positions %v do not point at the cluster", cues)
	}
	for _, block := range []string{"FRAME0", "FRAME29", "AUDIO29"} {
		if !bytes.Contains(out.Data, []byte(block)) {
			t.Errorf("block %q is missing", block)
		}
	}
}

func mediaWebMWithoutDuration(t *testing.T, p media.Processor) {
	out := remux(t, p, WebMFile(WebMOptions{AudioCodec: "A_OPUS", Frames: 11, Seconds: 2, NoDuration: true}))
	checkProbe(t, out.AVProbe, media.AVProbe{
		Format: media.FormatWebM, Kind: media.AVAudio, ContentType: "audio/webm", Extension: "webm", Duration: 2 * time.Second, AudioCodec: "opus",
	})
}

func mediaRemuxesMP3(t *testing.T, p media.Processor) {
	cover := FixturePNG(8, 8, false)
	out := remux(t, p, MP3File(100, cover))
	checkProbe(t, out.AVProbe, media.AVProbe{
		Format: media.FormatMP3, Kind: media.AVAudio, ContentType: "audio/mpeg", Extension: "mp3",
		Duration: 100 * 1152 * time.Second / 44100, AudioCodec: "mp3",
	})
	if !bytes.Equal(out.Cover, cover) {
		t.Errorf("Cover = %d bytes, want the APIC image", len(out.Cover))
	}
	if len(out.Data) != 100*417 || !bytes.HasPrefix(out.Data, []byte{0xFF, 0xFB}) {
		t.Errorf("the remuxed file is %d bytes, want the 100 frames alone", len(out.Data))
	}
}

func mediaRemuxesOgg(t *testing.T, p media.Processor) {
	out := remux(t, p, OggOpusFile(3))
	checkProbe(t, out.AVProbe, media.AVProbe{
		Format: media.FormatOgg, Kind: media.AVAudio, ContentType: "audio/ogg", Extension: "ogg", Duration: 3 * time.Second, AudioCodec: "opus",
	})
	if !bytes.Contains(out.Data, []byte("libopus")) || !bytes.Contains(out.Data, []byte("AUDIO149")) {
		t.Error("the remuxed file lost the vendor or the audio")
	}
	seq := uint32(0)
	for data := out.Data; len(data) > 0; seq++ {
		if len(data) < 27 || string(data[:4]) != "OggS" {
			t.Fatalf("page %d is not an Ogg page", seq)
		}
		n := int(data[26])
		size := 27 + n
		for _, l := range data[27 : 27+n] {
			size += int(l)
		}
		page := bytes.Clone(data[:size])
		want := binary.LittleEndian.Uint32(page[22:])
		clear(page[22:26])
		if got := binary.LittleEndian.Uint32(page[18:]); got != seq {
			t.Errorf("page %d has the sequence number %d", seq, got)
		}
		if OggCRC(page) != want {
			t.Errorf("page %d has a wrong checksum", seq)
		}
		data = data[size:]
	}
}

func mediaRejectsCodecs(t *testing.T, p media.Processor) {
	for name, data := range map[string][]byte{
		"hevc mp4":    MP4File(MP4Options{VideoCodec: "hvc1", Width: 640, Height: 360, Frames: 30, Seconds: 1}),
		"dolby mp4":   MP4File(MP4Options{VideoCodec: "dvh1", Width: 640, Height: 360, Frames: 30, Seconds: 1}),
		"mpeg-2 mp4":  MP4File(MP4Options{VideoCodec: "mp2v", Width: 640, Height: 360, Frames: 30, Seconds: 1}),
		"ac-3 mp4":    MP4File(MP4Options{AudioCodec: "ac-3", Seconds: 1}),
		"h.264 webm":  WebMFile(WebMOptions{VideoCodec: "V_MPEG4/ISO/AVC", Width: 640, Height: 360, Frames: 30, Seconds: 1}),
		"theora webm": WebMFile(WebMOptions{VideoCodec: "V_THEORA", Width: 640, Height: 360, Frames: 30, Seconds: 1}),
	} {
		if _, err := p.Remux(t.Context(), data, media.AVLimits{}); !errors.Is(err, media.ErrUnsupportedCodec) || !errors.Is(err, media.ErrUnsupportedFormat) {
			t.Errorf("Remux(%s) = %v, want ErrUnsupportedCodec", name, err)
		}
	}
}

func mediaRejectsAVLimits(t *testing.T, p media.Processor) {
	for _, tc := range []struct {
		name   string
		data   []byte
		limits media.AVLimits
		want   error
	}{
		{"bytes", MP3File(10, nil), media.AVLimits{MaxBytes: 100}, media.ErrTooLarge},
		{"video duration", MP4File(MP4Options{VideoCodec: "avc1", Width: 640, Height: 360, Frames: 30, Seconds: 301}), media.AVLimits{}, media.ErrTooLong},
		{"set video duration", WebMFile(WebMOptions{VideoCodec: "V_VP9", Width: 640, Height: 360, Frames: 30, Seconds: 2}), media.AVLimits{MaxVideoDuration: time.Second}, media.ErrTooLong},
		{"audio duration", MP3File(100, nil), media.AVLimits{MaxAudioDuration: 2 * time.Second}, media.ErrTooLong},
		{"ogg duration", OggOpusFile(3), media.AVLimits{MaxAudioDuration: 2 * time.Second}, media.ErrTooLong},
		{"pixels", MP4File(MP4Options{VideoCodec: "avc1", Width: 7680, Height: 4320, Frames: 30, Seconds: 1}), media.AVLimits{}, media.ErrDimensions},
		{"frame rate", MP4File(MP4Options{VideoCodec: "avc1", Width: 640, Height: 360, Frames: 240, Seconds: 1}), media.AVLimits{}, media.ErrDimensions},
	} {
		if _, err := p.Remux(t.Context(), tc.data, tc.limits); !errors.Is(err, tc.want) {
			t.Errorf("%s: Remux = %v, want %v", tc.name, err, tc.want)
		}
	}
	long := MP3File(100, nil)
	if _, err := p.Remux(t.Context(), long, media.AVLimits{MaxVideoDuration: time.Second}); err != nil {
		t.Errorf("Remux of 2.6 s of audio with a 1 s video limit = %v, want the audio limit", err)
	}
}

func mediaRejectsBrokenAV(t *testing.T, p media.Processor) {
	mp4 := MP4File(MP4Options{VideoCodec: "avc1", Width: 640, Height: 360, Frames: 30, Seconds: 1})
	webm := WebMFile(WebMOptions{VideoCodec: "V_VP9", Width: 640, Height: 360, Frames: 30, Seconds: 1})
	ogg := OggOpusFile(1)
	cases := map[string][]byte{
		"cut mp4":       mp4[:len(mp4)-40],
		"cut webm":      webm[:len(webm)/2],
		"cut ogg":       ogg[:len(ogg)-5],
		"mp4 no moov":   mp4[:bytes.Index(mp4, []byte("moov"))-4],
		"mp3 no frames": append([]byte("ID3\x03\x00\x00\x00\x00\x00\x00"), bytes.Repeat([]byte{0}, 100)...),
		"not a media":   []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>"),
		"matroska":      bytes.Replace(webm, []byte("webm"), []byte("mkvx"), 1),
	}
	for name, data := range cases {
		_, err := p.Remux(t.Context(), data, media.AVLimits{})
		if !media.Rejected(err) {
			t.Errorf("Remux(%s) = %v, want a rejection", name, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := p.Remux(ctx, mp4, media.AVLimits{}); err == nil || media.Rejected(err) {
		t.Errorf("Remux with a canceled context = %v, want the context's error", err)
	}
}

func mediaInspectsAV(t *testing.T, p media.Processor) {
	for name, data := range map[string][]byte{
		"mp4":  MP4File(MP4Options{VideoCodec: "avc1", AudioCodec: "mp4a", Width: 640, Height: 360, Frames: 30, Seconds: 1}),
		"webm": WebMFile(WebMOptions{VideoCodec: "V_VP9", Width: 640, Height: 360, Frames: 30, Seconds: 1}),
		"mp3":  MP3File(10, nil),
		"ogg":  OggOpusFile(1),
	} {
		original := bytes.Clone(data)
		probe, err := p.InspectAV(t.Context(), data, media.AVLimits{})
		if err != nil {
			t.Errorf("InspectAV(%s): %v", name, err)
			continue
		}
		if !bytes.Equal(data, original) {
			t.Errorf("InspectAV(%s) changed its input", name)
		}
		out := remux(t, p, data)
		if probe != out.AVProbe {
			t.Errorf("InspectAV(%s) = %+v, Remux = %+v", name, probe, out.AVProbe)
		}
		if again, err := p.InspectAV(t.Context(), out.Data, media.AVLimits{}); err != nil || again != probe {
			t.Errorf("InspectAV of the remuxed %s = %+v, %v; want %+v", name, again, err, probe)
		}
	}
}
