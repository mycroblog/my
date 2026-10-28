// SPDX-License-Identifier: AGPL-3.0-only

package instance_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/mycroblog/my/internal/domain/instance"
)

func encodePNG(t *testing.T, w, h int, alpha uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{R: 10, G: 20, B: 30, A: alpha})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// withTextChunk inserts a tEXt chunk (metadata) after the IHDR chunk of a PNG.
func withTextChunk(data []byte) []byte {
	payload := []byte("Comment\x00secret location")
	chunk := make([]byte, 0, len(payload)+12)
	chunk = binary.BigEndian.AppendUint32(chunk, uint32(len(payload))) //nolint:gosec // a short constant
	typed := append([]byte("tEXt"), payload...)
	chunk = append(chunk, typed...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(typed))
	ihdrEnd := 8 + 25
	out := append([]byte{}, data[:ihdrEnd]...)
	out = append(out, chunk...)
	return append(out, data[ihdrEnd:]...)
}

func TestBrandRenditionSize(t *testing.T) {
	for _, tt := range []struct {
		r          instance.BrandRendition
		vw, vh     float64
		wantW, wnH int
	}{
		{instance.RenditionFavicon32, 1, 1, 32, 32},
		{instance.RenditionFavicon48, 1, 1, 48, 48},
		{instance.RenditionAppleTouch, 1, 1, 180, 180},
		{instance.RenditionPWA192, 1, 1, 192, 192},
		{instance.RenditionPWA512, 1, 1, 512, 512},
		{instance.RenditionMaskable512, 1, 1, 512, 512},
		{instance.RenditionPreview, 4, 1, 1200, 630},
		{instance.RenditionLogoMail, 4, 1, 384, 96},
		{instance.RenditionLogoDarkMail, 1, 1, 96, 96},
		{instance.RenditionLogoMail, 8, 1, 600, 75},
		{instance.RenditionLogoMail, 6.25, 1, 600, 96},
	} {
		if w, h := instance.BrandRenditionSize(tt.r, tt.vw, tt.vh); w != tt.wantW || h != tt.wnH {
			t.Errorf("%s of %gx%g = %dx%d, want %dx%d", tt.r, tt.vw, tt.vh, w, h, tt.wantW, tt.wnH)
		}
	}
}

func TestCheckBrandPNG(t *testing.T) {
	good := withTextChunk(encodePNG(t, 32, 32, 128))
	out, w, h, err := instance.CheckBrandPNG(instance.RenditionFavicon32, good, 1, 1)
	if err != nil || w != 32 || h != 32 {
		t.Fatalf("CheckBrandPNG = %d, %d, %v", w, h, err)
	}
	if bytes.Contains(out, []byte("secret location")) || bytes.Contains(out, []byte("tEXt")) {
		t.Error("the re-encoded PNG keeps the metadata")
	}
	if _, err := png.Decode(bytes.NewReader(out)); err != nil {
		t.Errorf("the re-encoded PNG does not decode: %v", err)
	}
	// A mail logo may be a pixel off.
	if _, _, _, err := instance.CheckBrandPNG(instance.RenditionLogoMail, encodePNG(t, 385, 96, 0), 4, 1); err != nil {
		t.Errorf("mail logo a pixel off: %v", err)
	}

	tests := []struct {
		name string
		r    instance.BrandRendition
		data []byte
		want instance.BrandProblemCode
	}{
		{"not a PNG", instance.RenditionFavicon32, []byte("GIF89a"), instance.BrandInvalidPNG},
		{"SVG declared as PNG", instance.RenditionFavicon32, []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), instance.BrandInvalidPNG},
		{"truncated", instance.RenditionFavicon32, good[:60], instance.BrandInvalidPNG},
		{"wrong size", instance.RenditionFavicon48, encodePNG(t, 32, 32, 255), instance.BrandDimensions},
		{"icon a pixel off", instance.RenditionPWA192, encodePNG(t, 191, 192, 255), instance.BrandDimensions},
		{"mail logo two pixels off", instance.RenditionLogoMail, encodePNG(t, 386, 96, 255), instance.BrandDimensions},
		{"transparent home screen icon", instance.RenditionAppleTouch, encodePNG(t, 180, 180, 254), instance.BrandNotOpaque},
		{"transparent preview", instance.RenditionPreview, encodePNG(t, 1200, 630, 0), instance.BrandNotOpaque},
		{"too large", instance.RenditionFavicon32, append(encodePNG(t, 32, 32, 255), make([]byte, instance.MaxBrandPNGBytes)...), instance.BrandTooLarge},
		{"huge dimensions", instance.RenditionPWA512, hugeHeader(t), instance.BrandDimensions},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := instance.CheckBrandPNG(tt.r, tt.data, 4, 1)
			var be *instance.BrandError
			if !errors.As(err, &be) || be.Code != tt.want || be.Rendition != tt.r {
				t.Errorf("CheckBrandPNG = %v, want %s of %s", err, tt.want, tt.r)
			}
		})
	}
	if _, _, _, err := instance.CheckBrandPNG(instance.RenditionAppleTouch, encodePNG(t, 180, 180, 255), 1, 1); err != nil {
		t.Errorf("opaque home screen icon: %v", err)
	}
}

// hugeHeader is a PNG whose header claims 100000x100000 pixels.
func hugeHeader(t *testing.T) []byte {
	data := encodePNG(t, 1, 1, 255)
	binary.BigEndian.PutUint32(data[16:], 100000)
	binary.BigEndian.PutUint32(data[20:], 100000)
	binary.BigEndian.PutUint32(data[29:], crc32.ChecksumIEEE(data[12:29]))
	return data
}

func TestNormalizeBrandBackground(t *testing.T) {
	if got, err := instance.NormalizeBrandBackground("#ABCdef"); err != nil || got != "#abcdef" {
		t.Errorf("NormalizeBrandBackground = %q, %v", got, err)
	}
	for _, bad := range []string{"", "abcdef", "#abc", "#abcdefff", "#ghijkl", "red"} {
		if _, err := instance.NormalizeBrandBackground(bad); !errors.Is(err, instance.ErrInvalidBrand) {
			t.Errorf("NormalizeBrandBackground(%q) = %v", bad, err)
		}
	}
}

func TestParseBrand(t *testing.T) {
	for _, k := range instance.BrandKinds {
		if got, ok := instance.ParseBrandKind(string(k)); !ok || got != k {
			t.Errorf("ParseBrandKind(%s) = %s, %v", k, got, ok)
		}
		for _, r := range instance.BrandRenditions(k) {
			if got, ok := instance.ParseBrandRendition(string(r)); !ok || got != r || r.ContentType() != "image/png" {
				t.Errorf("ParseBrandRendition(%s) = %s, %v", r, got, ok)
			}
		}
	}
	if _, ok := instance.ParseBrandKind("banner"); ok {
		t.Error("ParseBrandKind(banner) is ok")
	}
	if got := instance.BrandFilePath("01J", instance.RenditionSVG); got != "/brand/01J/svg.svg" {
		t.Errorf("BrandFilePath = %s", got)
	}
	if got := instance.BrandBlobKey("01J", instance.RenditionPWA192); got != "instance/brand/01J/pwa-192.png" {
		t.Errorf("BrandBlobKey = %s", got)
	}
}
