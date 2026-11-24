// SPDX-License-Identifier: AGPL-3.0-only

package contract

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"slices"

	"github.com/mycroblog/my/internal/adapter/media/native"
	mediaplugin "github.com/mycroblog/my/internal/adapter/media/plugin"
	"github.com/mycroblog/my/internal/app/media"
)

// locationMarker is what the test files hide in their metadata, as a position and a caption: an output that holds it
// kept the metadata.
const locationMarker = "MY-PLUGIN-TEST-LOCATION-35.6812N-139.7671E"

// videoLocation is the position in the user data (udta ©xyz) of the test video, in ISO 6709 as phones write it.
const videoLocation = "+35.6812+139.7671/"

// mediaImageCases, mediaVideoCases, and mediaAudioCases check a media plugin through the server's adapter, which checks every output as
// the server does. They send only the small files made here, and check that the location in their metadata does not
// come back (ADR-0146 §16).
var mediaImageCases = []Case{
	{
		Name: "describe",
		Run:  mediaDescribe,
		// Unimplemented passes: such a plugin gets the built-in formats only.
		Note: "the server accepts AVIF, HEIC, and APNG uploads only as Describe declares them, and sends them to Process",
	},
	{
		Name: "process_photo",
		Run:  mediaProcessPhoto,
		// The server checks each output's format, dimensions, and box; it cannot know every way to keep metadata.
		Note: "no output held the test location; the plugin must remove every other form of metadata too",
	},
	{Name: "process_emoji", Run: mediaProcessEmoji},
	{Name: "process_animation", Run: mediaProcessAnimation},
}

var mediaVideoCases = []Case{
	{
		Name: "remux_video",
		Run:  mediaRemuxVideo,
		// The server checks the container and its tracks; it cannot know every box that can hold metadata.
		Note: "the output held neither the test location nor the tags; the plugin must remove every other form of metadata too",
	},
}

var mediaAudioCases = []Case{
	{Name: "remux_audio", Run: mediaRemuxAudio},
}

// mediaImageProcessor, mediaVideoProcessor, and mediaAudioProcessor return the server's adapter that sends its kind to the Conn of env.
func mediaImageProcessor(env Env) (*mediaplugin.Processor, error) {
	return mediaplugin.New(mediaplugin.Options{Builtin: native.New(native.Options{Concurrency: 1}), Image: env.Conn})
}

func mediaVideoProcessor(env Env) (*mediaplugin.Processor, error) {
	return mediaplugin.New(mediaplugin.Options{Builtin: native.New(native.Options{Concurrency: 1}), Video: env.Conn})
}

func mediaAudioProcessor(env Env) (*mediaplugin.Processor, error) {
	return mediaplugin.New(mediaplugin.Options{Builtin: native.New(native.Options{Concurrency: 1}), Audio: env.Conn})
}

// mediaVariants returns every variant of out by name.
func mediaVariants(out media.Output) map[string]media.Variant {
	all := map[string]media.Variant{}
	for _, size := range media.AllSizes {
		all["still "+string(size)] = out.Still.Variant(size)
		if out.Animated != nil {
			all["animated "+string(size)] = out.Animated.Variant(size)
		}
	}
	return all
}

// keptMetadata returns the first trace of metadata in the outputs, or "".
func keptMetadata(outputs map[string][]byte) string {
	for name, data := range outputs {
		for _, needle := range []string{locationMarker, "Exif\x00", "GPS", "tEXt", "http://ns.adobe.com/xap/"} {
			if bytes.Contains(data, []byte(needle)) {
				return fmt.Sprintf("the %s output holds %q", name, needle)
			}
		}
	}
	return ""
}

// mediaProcess processes data as an upload for purpose and checks that no output kept its metadata.
func mediaProcess(ctx context.Context, env Env, data []byte, purpose media.Purpose) (media.Output, error) {
	p, err := mediaImageProcessor(env)
	if err != nil {
		return media.Output{}, err
	}
	out, err := p.Process(ctx, data, media.Limits{}, media.Sizes{}, media.Request{Purpose: purpose})
	if err != nil {
		return media.Output{}, describeCallError(env, err)
	}
	outputs := map[string][]byte{}
	for name, v := range mediaVariants(out) {
		outputs[name] = v.Data
	}
	if kept := keptMetadata(outputs); kept != "" {
		return media.Output{}, fmt.Errorf("the metadata of the upload came back: %s", kept)
	}
	return out, nil
}

// mediaProcessPhoto sends a photo with a location in its EXIF data, rotated by its EXIF orientation.
func mediaProcessPhoto(ctx context.Context, env Env) error {
	out, err := mediaProcess(ctx, env, testJPEG(96, 64), media.PurposeAttachment)
	if err != nil {
		return err
	}
	if out.Width != 64 || out.Height != 96 {
		return fmt.Errorf("the dimensions are %d×%d, want 64×96: the EXIF orientation 6 turns the 96×64 photo", out.Width, out.Height)
	}
	return nil
}

// mediaProcessEmoji sends a transparent image with a caption as a custom emoji, whose large output is a PNG.
func mediaProcessEmoji(ctx context.Context, env Env) error {
	_, err := mediaProcess(ctx, env, testPNG(48, 48), media.PurposeEmoji)
	return err
}

// mediaProcessAnimation sends a GIF of three frames, which gets moving outputs.
func mediaProcessAnimation(ctx context.Context, env Env) error {
	out, err := mediaProcess(ctx, env, testGIF(48, 32, 3), media.PurposeAttachment)
	if err != nil {
		return err
	}
	if out.Animated == nil {
		return fmt.Errorf("an animation of three frames has no moving outputs")
	}
	return nil
}

// mediaDescribe asks the plugin which formats of images it accepts beyond the built-in ones. A plugin that answers
// unimplemented passes, as one that declares none.
func mediaDescribe(ctx context.Context, env Env) error {
	p, err := mediaImageProcessor(env)
	if err != nil {
		return err
	}
	if err := p.Describe(ctx); err != nil {
		return describeCallError(env, err)
	}
	return nil
}

// mediaRemuxAudio sends an MP3 with a title and a cover picture in its tags.
func mediaRemuxAudio(ctx context.Context, env Env) error {
	p, err := mediaAudioProcessor(env)
	if err != nil {
		return err
	}
	out, err := p.Remux(ctx, testMP3(20, testPNG(16, 16)), media.AVLimits{})
	if err != nil {
		return describeCallError(env, err)
	}
	if kept := keptMetadata(map[string][]byte{"remuxed": out.Data}); kept != "" {
		return fmt.Errorf("the tags of the file came back: %s", kept)
	}
	return nil
}

// mediaRemuxVideo sends an MP4 with a video and a sound track, a location in its user data, and a title in its tags.
func mediaRemuxVideo(ctx context.Context, env Env) error {
	p, err := mediaVideoProcessor(env)
	if err != nil {
		return err
	}
	out, err := p.Remux(ctx, testMP4(), media.AVLimits{})
	if err != nil {
		return describeCallError(env, err)
	}
	if bytes.Contains(out.Data, []byte(videoLocation)) {
		return fmt.Errorf("the location of the video came back: the remuxed output holds %q", videoLocation)
	}
	if kept := keptMetadata(map[string][]byte{"remuxed": out.Data}); kept != "" {
		return fmt.Errorf("the tags of the video came back: %s", kept)
	}
	return nil
}

// testImage returns a w × h image whose left half is red and right half blue, with alpha in its right half if
// transparent.
func testImage(w, h int, transparent bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.NRGBA{R: 220, G: 20, B: 20, A: 255}
			if x >= w/2 {
				c = color.NRGBA{R: 20, G: 20, B: 220, A: 255}
				if transparent {
					c.A = 128
				}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// testJPEG returns a w × h JPEG with an EXIF segment that holds the orientation 6 (turned a quarter clockwise), an
// ImageDescription, and a GPS position with locationMarker as its area.
func testJPEG(w, h int) []byte {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(w, h, false), &jpeg.Options{Quality: 90}); err != nil {
		panic(err)
	}
	be := binary.BigEndian
	desc := locationMarker + "\x00"
	const ifd0, entrySize = 8, 12
	descOff := ifd0 + 2 + 3*entrySize + 4
	gpsOff := descOff + len(desc)
	latOff := gpsOff + 2 + 3*entrySize + 4
	areaOff := latOff + 24
	t := []byte("MM\x00\x2A")
	t = be.AppendUint32(t, ifd0)
	entry := func(tag, typ uint16, count, value int) {
		t = be.AppendUint16(t, tag)
		t = be.AppendUint16(t, typ)
		t = be.AppendUint32(t, uint32(count)) //nolint:gosec // a few hundred bytes
		t = be.AppendUint32(t, uint32(value)) //nolint:gosec // a few hundred bytes
	}
	t = be.AppendUint16(t, 3)
	entry(0x010E, 2, len(desc), descOff) // ImageDescription
	entry(0x0112, 3, 1, 6<<16)           // Orientation, a left-justified SHORT
	entry(0x8825, 4, 1, gpsOff)          // GPSInfo
	t = be.AppendUint32(t, 0)
	t = append(t, desc...)
	t = be.AppendUint16(t, 3)
	entry(0x0001, 2, 2, 'N'<<24)                   // GPSLatitudeRef
	entry(0x0002, 5, 3, latOff)                    // GPSLatitude
	entry(0x001C, 7, len(locationMarker), areaOff) // GPSAreaInformation
	t = be.AppendUint32(t, 0)
	for _, r := range [][2]uint32{{35, 1}, {40, 1}, {5232, 100}} {
		t = be.AppendUint32(t, r[0])
		t = be.AppendUint32(t, r[1])
	}
	t = append(t, locationMarker...)

	app1 := []byte{0xFF, 0xE1}
	app1 = be.AppendUint16(app1, uint16(2+6+len(t))) //nolint:gosec // a few hundred bytes
	app1 = append(app1, "Exif\x00\x00"...)
	app1 = append(app1, t...)
	data := buf.Bytes()
	out := append([]byte{}, data[:2]...) // SOI
	out = append(out, app1...)
	return append(out, data[2:]...)
}

// testPNG returns a w × h PNG, transparent in its right half, with a tEXt chunk that holds locationMarker.
func testPNG(w, h int) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, testImage(w, h, true)); err != nil {
		panic(err)
	}
	data := buf.Bytes()
	body := append([]byte("tEXtComment\x00"), locationMarker...)
	chunk := binary.BigEndian.AppendUint32(nil, uint32(len(body)-4)) //nolint:gosec // a few dozen bytes
	chunk = append(chunk, body...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(body))
	const afterIHDR = 8 + 4 + 4 + 13 + 4 // the signature and the IHDR chunk
	out := append([]byte{}, data[:afterIHDR]...)
	out = append(out, chunk...)
	return append(out, data[afterIHDR:]...)
}

// testGIF returns a w × h GIF of frames frames with a comment that holds locationMarker.
func testGIF(w, h, frames int) []byte {
	palette := color.Palette{color.NRGBA{A: 255}, color.NRGBA{R: 220, G: 20, B: 20, A: 255}, color.NRGBA{R: 20, G: 20, B: 220, A: 255}}
	g := &gif.GIF{}
	for i := range frames {
		img := image.NewPaletted(image.Rect(0, 0, w, h), palette)
		for y := range h {
			for x := range w {
				img.SetColorIndex(x, y, uint8(1+(x/max(1, w/frames)+i)%2))
			}
		}
		g.Image = append(g.Image, img)
		g.Delay = append(g.Delay, 10)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		panic(err)
	}
	data := buf.Bytes()
	// A comment extension right after the logical screen and its global color table.
	comment := append([]byte{0x21, 0xFE, byte(len(locationMarker))}, locationMarker...)
	comment = append(comment, 0)
	at := 13
	if data[10]&0x80 != 0 {
		at += 3 << (data[10]&0x07 + 1)
	}
	out := append([]byte{}, data[:at]...)
	out = append(out, comment...)
	return append(out, data[at:]...)
}

// testMP3 returns an MP3 of frames MPEG 1 Layer III frames between an ID3v2.3 tag, with a title that holds
// locationMarker and the cover picture, and an ID3v1 tag.
func testMP3(frames int, cover []byte) []byte {
	frame := func(id string, body []byte) []byte {
		out := binary.BigEndian.AppendUint32([]byte(id), uint32(len(body))) //nolint:gosec // a few hundred bytes
		return append(append(out, 0, 0), body...)
	}
	body := frame("TIT2", append([]byte{0}, locationMarker...))
	body = append(body, frame("APIC", append([]byte("\x00image/png\x00\x03cover\x00"), cover...))...)
	n := len(body)
	out := append([]byte("ID3\x03\x00\x00"), byte(n>>21&0x7F), byte(n>>14&0x7F), byte(n>>7&0x7F), byte(n&0x7F))
	out = append(out, body...)
	for i := range frames {
		f := make([]byte, 417) // 144 × 128000 / 44100, no padding
		copy(f, []byte{0xFF, 0xFB, 0x90, 0x00})
		copy(f[4:], fmt.Sprintf("FRAME%d", i))
		out = append(out, f...)
	}
	tag := make([]byte, 128)
	copy(tag, "TAG"+locationMarker)
	return append(out, tag...)
}

// mp4Box returns an MP4 box of typ whose body is parts.
func mp4Box(typ string, parts ...[]byte) []byte {
	body := slices.Concat(parts...)
	return slices.Concat(binary.BigEndian.AppendUint32(nil, uint32(8+len(body))), []byte(typ), body) //nolint:gosec // a few hundred bytes
}

// be32 returns n as 4 big-endian bytes.
func be32(n int) []byte { return binary.BigEndian.AppendUint32(nil, uint32(n)) } //nolint:gosec // small numbers

// The coded media of testMP4, made once by FFmpeg 4.3 (libx264 and its native AAC encoder) and read out of its MP4:
//
//	ffmpeg -f lavfi -i color=c=gray:size=64x48:rate=10:duration=1 \
//	  -f lavfi -i "anullsrc=sample_rate=8000:channel_layout=mono,atrim=duration=0.5" -pix_fmt yuv420p \
//	  -c:v libx264 -profile:v baseline -preset veryslow -crf 40 -g 10 -bf 0 -bsf:v filter_units=remove_types=6 \
//	  -c:a aac -b:a 16k -map_metadata -1 -fflags +bitexact -flags:v +bitexact -flags:a +bitexact src.mp4
//
// mp4AVCConfig and mp4AudioConfig are the bodies of its avcC and esds boxes, mp4Frames the 10 frames of 64 × 48 at
// 10 fps (the first one an IDR frame) and mp4AudioFrame the 5 frames of silence, mono at 8 kHz, which are all the same.
const (
	mp4AVCConfig   = "0142c00affe100176742c00aa61111ec0440000003004000000503c489846001000668c8420392c8"
	mp4AudioConfig = "0000000003808080250002000480808017401500000000003e80000000fe0580808005158856e500068080800102"
	mp4AudioFrame  = "01182007"
)

var mp4Frames = []string{
	"0000001065888207f93146fbefaebaebaebaebc0",
	"00000006419a1c0ff1b0", "00000006419a2a03fc6c", "00000006419a3b03fc6c", "00000006419a4900ff1b",
	"00000006419a5940ff1b", "00000006419a6980ff1b", "00000006419a79c0ff1b", "00000007419a88803bc6c0",
	"00000007419a989037c6c0",
}

// testMP4 returns a 1-second 64 × 48 MP4 with its moov after its mdat, as cameras write it: a video track of 10 H.264
// frames (avc1 with its avcC), a sound track of 5 AAC frames (mp4a with its esds), one sample per chunk, and user data
// with videoLocation (©xyz) and an item list whose title holds locationMarker. A demuxer that reads the samples, as
// FFmpeg does, reads it as a real camera's file.
func testMP4() []byte {
	ftyp := mp4Box("ftyp", []byte("isom\x00\x00\x02\x00isomiso2avc1mp41"))
	const w, h, sampleRate = 64, 48, 8000
	type track struct {
		handler            string
		entry              []byte // the sample entry, with its codec's configuration
		timescale, delta   int    // the track's ticks per second and per sample
		samples            [][]byte
		offsets            []int
		syncOnlyFirstFrame bool
	}
	zero := make([]byte, 4)
	video := &track{handler: "vide", timescale: 1000, delta: 100, syncOnlyFirstFrame: true,
		entry: mp4Box("avc1", make([]byte, 6), []byte{0, 1}, make([]byte, 16), be32(w<<16|h),
			be32(0x480000), be32(0x480000), zero, []byte{0, 1}, make([]byte, 32), []byte{0, 0x18, 0xFF, 0xFF},
			mp4Box("avcC", unhex(mp4AVCConfig)))}
	for _, f := range mp4Frames {
		video.samples = append(video.samples, unhex(f))
	}
	audio := &track{handler: "soun", timescale: sampleRate, delta: 1024,
		entry: mp4Box("mp4a", make([]byte, 6), []byte{0, 1}, make([]byte, 8), []byte{0, 1, 0, 16}, zero,
			be32(sampleRate<<16), mp4Box("esds", unhex(mp4AudioConfig)))}
	for range 5 {
		audio.samples = append(audio.samples, unhex(mp4AudioFrame))
	}
	tracks := []*track{video, audio}
	var mdat []byte
	for i := range len(video.samples) {
		for _, t := range tracks {
			if i < len(t.samples) {
				t.offsets = append(t.offsets, len(ftyp)+8+len(mdat))
				mdat = append(mdat, t.samples[i]...)
			}
		}
	}
	const timescale = 1000
	matrix := slices.Concat(be32(0x10000), make([]byte, 12), be32(0x10000), make([]byte, 12), be32(0x40000000))
	moov := [][]byte{mp4Box("mvhd", zero, zero, zero, be32(timescale), be32(timescale), be32(0x10000), []byte{1, 0},
		make([]byte, 10), matrix, make([]byte, 24), be32(len(tracks)+1))}
	for ti, t := range tracks {
		duration := len(t.samples) * t.delta
		size, volume := slices.Concat(zero, zero), []byte{1, 0, 0, 0}
		if t == video {
			size, volume = slices.Concat(be32(w<<16), be32(h<<16)), zero
		}
		tkhd := mp4Box("tkhd", []byte{0, 0, 0, 3}, zero, zero, be32(ti+1), zero, be32(duration*timescale/t.timescale),
			make([]byte, 12), volume, matrix, size)
		mdhd := mp4Box("mdhd", zero, zero, zero, be32(t.timescale), be32(duration), []byte{0x55, 0xC4, 0, 0})
		hdlr := mp4Box("hdlr", zero, zero, []byte(t.handler), make([]byte, 12), []byte("handler\x00"))
		stsz := slices.Concat(zero, zero, be32(len(t.samples)))
		for _, s := range t.samples {
			stsz = append(stsz, be32(len(s))...)
		}
		stco := slices.Concat(zero, be32(len(t.offsets)))
		for _, off := range t.offsets {
			stco = append(stco, be32(off)...)
		}
		boxes := [][]byte{
			mp4Box("stsd", zero, be32(1), t.entry),
			mp4Box("stts", zero, be32(1), be32(len(t.samples)), be32(t.delta)),
			mp4Box("stsc", zero, be32(1), be32(1), be32(1), be32(1)),
			mp4Box("stsz", stsz), mp4Box("stco", stco),
		}
		if t.syncOnlyFirstFrame {
			boxes = append(boxes, mp4Box("stss", zero, be32(1), be32(1)))
		}
		header := mp4Box("smhd", zero, zero)
		if t == video {
			header = mp4Box("vmhd", []byte{0, 0, 0, 1}, make([]byte, 8))
		}
		dinf := mp4Box("dinf", mp4Box("dref", zero, be32(1), mp4Box("url ", []byte{0, 0, 0, 1})))
		minf := mp4Box("minf", header, dinf, mp4Box("stbl", boxes...))
		moov = append(moov, mp4Box("trak", tkhd, mp4Box("mdia", mdhd, hdlr, minf)))
	}
	meta := mp4Box("meta", zero, mp4Box("hdlr", zero, zero, []byte("mdir"), make([]byte, 12), []byte{0}),
		mp4Box("ilst", mp4Box("\xA9nam", mp4Box("data", be32(1), zero, []byte(locationMarker)))))
	xyz := slices.Concat([]byte{0, byte(len(videoLocation)), 0x15, 0xC7}, []byte(videoLocation)) // its length and the language "eng"
	moov = append(moov, mp4Box("udta", mp4Box("\xA9xyz", xyz), meta))
	return slices.Concat(ftyp, mp4Box("mdat", mdat), mp4Box("moov", moov...))
}

// unhex returns the bytes of the hexadecimal s, a constant of this file.
func unhex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}
