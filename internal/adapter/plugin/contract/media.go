// SPDX-License-Identifier: AGPL-3.0-only

package contract

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"

	"github.com/mycroblog/my/internal/adapter/media/native"
	mediaplugin "github.com/mycroblog/my/internal/adapter/media/plugin"
	"github.com/mycroblog/my/internal/app/media"
)

// locationMarker is what the test files hide in their metadata, as a position and a caption: an output that holds it
// kept the metadata.
const locationMarker = "MY-PLUGIN-TEST-LOCATION-35.6812N-139.7671E"

// mediaImageCases and mediaAudioCases check a media plugin through the server's adapter, which checks every output as
// the server does. They send only the small files made here, and check that the location in their metadata does not
// come back (ADR-0146 §16).
var mediaImageCases = []Case{
	{
		Name: "process_photo",
		Run:  mediaProcessPhoto,
		// The server checks each output's format, dimensions, and box; it cannot know every way to keep metadata.
		Note: "no output held the test location; the plugin must remove every other form of metadata too",
	},
	{Name: "process_emoji", Run: mediaProcessEmoji},
	{Name: "process_animation", Run: mediaProcessAnimation},
}

var mediaAudioCases = []Case{
	{Name: "remux_audio", Run: mediaRemuxAudio},
}

// mediaImageProcessor and mediaAudioProcessor return the server's adapter that sends its kind to the Conn of env.
func mediaImageProcessor(env Env) (*mediaplugin.Processor, error) {
	return mediaplugin.New(mediaplugin.Options{Builtin: native.New(native.Options{Concurrency: 1}), Image: env.Conn})
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
