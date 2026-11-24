// SPDX-License-Identifier: AGPL-3.0-only

package plugin_test

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/media/native"
	mediaplugin "github.com/mycroblog/my/internal/adapter/media/plugin"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/pkg/testkit"
)

// within bounds the waits on the plugin and on the goroutines of a test, which take milliseconds when they work.
const within = 10 * time.Second

var allExtended = []media.Format{media.FormatAVIF, media.FormatHEIC, media.FormatAPNG}

// The extended fixtures: an AVIF and a HEIC image of 64 × 48 and an APNG of 64 × 48 in 3 frames.
var (
	avifImage = testkit.HEIFFile(testkit.HEIFOptions{Brands: []string{"avif", "mif1"}, Width: 64, Height: 48})
	heicImage = testkit.HEIFFile(testkit.HEIFOptions{Brands: []string{"heic", "mif1"}, Width: 64, Height: 48})
	apngImage = testkit.APNGFile(testkit.APNGOptions{Width: 64, Height: 48, Frames: 3})
)

// decoding is the processor of a plugin that decodes the extended formats: it processes an AVIF or HEIC file as a JPEG
// and an APNG as a GIF of the same dimensions and frames, and counts the files it processes.
type decoding struct {
	*native.Processor

	processes atomic.Int64
	// process, when set, changes the output.
	process func(out *media.Output)
}

func newDecoding() *decoding { return &decoding{Processor: native.New(native.Options{})} }

func (d *decoding) Process(ctx context.Context, data []byte, limits media.Limits, sizes media.Sizes, req media.Request) (media.Output, error) {
	d.processes.Add(1)
	if f := media.Sniff(data); f.Extended() {
		probe, err := media.InspectExtended(data, limits)
		if err != nil {
			return media.Output{}, err
		}
		data = testkit.FixtureJPEG(probe.Width, probe.Height, 1)
		if f == media.FormatAPNG {
			data = testkit.FixtureGIF(probe.Width, probe.Height, probe.Frames)
		}
	}
	out, err := d.Processor.Process(ctx, data, limits, sizes, req)
	if err == nil && d.process != nil {
		d.process(&out)
	}
	return out, err
}

// newProcessorWith returns the Processor of o that sends images to the plugin at d, with the built-in processor.
func newProcessorWith(t *testing.T, d kind.Destination, o mediaplugin.Options) *mediaplugin.Processor {
	t.Helper()
	o.Builtin = native.New(native.Options{})
	o.Image = newConn(t, kind.MediaImage, d)
	p, err := mediaplugin.New(o)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// describe calls Describe and fails the test on an error.
func describe(t *testing.T, p *mediaplugin.Processor) {
	t.Helper()
	if err := p.Describe(t.Context()); err != nil {
		t.Fatalf("Describe = %v", err)
	}
}

// promptly runs f and fails the test when it does not return within the bound, as a call that waits on the plugin would.
func promptly(t *testing.T, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	timer := time.NewTimer(within)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		t.Fatalf("%s did not return", what)
	}
}

// With a plugin that declares AVIF, HEIC, and APNG, Inspect accepts them from their headers without calling the plugin,
// and Process hands them to it.
func TestDeclaredFormatsAreAcceptedAndProcessed(t *testing.T) {
	t.Parallel()
	behind := newDecoding()
	p, rec := serving(t, behind, fakeplugin.Options{MediaInputFormats: allExtended})
	describe(t, p)
	calls := rec.Calls()
	for _, tc := range []struct {
		name string
		data []byte
		want media.Probe
	}{
		{"avif", avifImage, media.Probe{Format: media.FormatAVIF, Width: 64, Height: 48, Frames: 1}},
		{"heic", heicImage, media.Probe{Format: media.FormatHEIC, Width: 64, Height: 48, Frames: 1}},
		{"apng", apngImage, media.Probe{Format: media.FormatAPNG, Width: 64, Height: 48, Frames: 3}},
	} {
		got, err := p.Inspect(t.Context(), tc.data, media.Limits{})
		if err != nil || got != tc.want {
			t.Errorf("Inspect(%s) = %+v, %v, want %+v", tc.name, got, err, tc.want)
		}
		if n := rec.Calls(); n != calls {
			t.Errorf("Inspect(%s) made %d calls of the plugin", tc.name, n-calls)
		}
		before := behind.processes.Load()
		out, err := p.Process(t.Context(), tc.data, media.Limits{}, media.Sizes{}, media.Request{})
		if err != nil {
			t.Errorf("Process(%s) = %v", tc.name, err)
			continue
		}
		if behind.processes.Load() != before+1 || out.Width != 64 || out.Height != 48 {
			t.Errorf("Process(%s) reached the plugin %d times and made %d×%d", tc.name, behind.processes.Load()-before, out.Width, out.Height)
		}
		calls = rec.Calls()
	}
}

// The limits of an image hold before the plugin is called: a declared file over them is refused by the server, by
// Inspect and by Process, and never sent.
func TestDeclaredFormatsAreCheckedBeforeThePlugin(t *testing.T) {
	t.Parallel()
	behind := newDecoding()
	p, _ := serving(t, behind, fakeplugin.Options{MediaInputFormats: allExtended})
	describe(t, p)
	for _, tc := range []struct {
		name   string
		data   []byte
		limits media.Limits
		want   error
	}{
		{"an AVIF over MaxBytes", avifImage, media.Limits{MaxBytes: int64(len(avifImage)) - 1}, media.ErrTooLarge},
		{"an AVIF wider than MaxSide", avifImage, media.Limits{MaxSide: 63}, media.ErrDimensions},
		{"a HEIC bomb", testkit.HEIFFile(testkit.HEIFOptions{Brands: []string{"heic"}, Width: 1 << 20, Height: 1 << 20}), media.Limits{}, media.ErrDimensions},
		{"a HEIC sequence over MaxFrames", testkit.HEIFFile(testkit.HEIFOptions{Brands: []string{"hevc"}, Tracks: []testkit.HEIFTrack{
			{Handler: "pict", Width: 64, Height: 48, Samples: 4},
		}}), media.Limits{MaxFrames: 3}, media.ErrDimensions},
		{"an APNG over MaxFrames", apngImage, media.Limits{MaxFrames: 2}, media.ErrDimensions},
		{"an APNG over MaxPixels", apngImage, media.Limits{MaxPixels: 64*48 - 1}, media.ErrDimensions},
		{"an APNG with a frame outside its canvas", testkit.APNGFile(testkit.APNGOptions{Width: 64, Height: 48, Frames: 2, Controls: []testkit.APNGFrame{
			{Width: 64, Height: 48}, {Width: 64, Height: 48, X: 1},
		}}), media.Limits{}, media.ErrCorrupt},
	} {
		if _, err := p.Inspect(t.Context(), tc.data, tc.limits); !errors.Is(err, tc.want) {
			t.Errorf("Inspect(%s) = %v, want %v", tc.name, err, tc.want)
		}
		if _, err := p.Process(t.Context(), tc.data, tc.limits, media.Sizes{}, media.Request{}); !errors.Is(err, tc.want) || !media.Rejected(err) {
			t.Errorf("Process(%s) = %v, want %v", tc.name, err, tc.want)
		}
	}
	if n := behind.processes.Load(); n != 0 {
		t.Errorf("%d files over the limits reached the plugin", n)
	}
}

// A format that the plugin does not declare, or that it declares and the server cannot tell, is refused as the built-in
// processor refuses it: AVIF and HEIC with ErrUnsupportedFormat, and an APNG is a still PNG. Formats of the built-in
// processor in a declaration change nothing.
func TestUndeclaredFormatsAreTheBuiltIns(t *testing.T) {
	t.Parallel()
	behind := newDecoding()
	p, _ := serving(t, behind, fakeplugin.Options{MediaInputFormats: []media.Format{media.FormatAVIF, "jxl", media.FormatJPEG, media.FormatGIF}})
	describe(t, p)
	if got, err := p.Inspect(t.Context(), avifImage, media.Limits{}); err != nil || got.Format != media.FormatAVIF {
		t.Errorf("Inspect(declared avif) = %+v, %v", got, err)
	}
	for name, data := range map[string][]byte{
		"undeclared heic":     heicImage,
		"a HEIF of no brand":  testkit.HEIFFile(testkit.HEIFOptions{Brands: []string{"mif1", "miaf"}, Width: 64, Height: 48}),
		"a declared JPEG XL":  []byte("\xFF\x0A\xFA\x7F\x01\x90\x08"),
		"an unsniffable file": []byte("not an image at all"),
	} {
		if _, err := p.Inspect(t.Context(), data, media.Limits{}); !errors.Is(err, media.ErrUnsupportedFormat) {
			t.Errorf("Inspect(%s) = %v, want ErrUnsupportedFormat", name, err)
		}
		if _, err := p.Process(t.Context(), data, media.Limits{}, media.Sizes{}, media.Request{}); !errors.Is(err, media.ErrUnsupportedFormat) {
			t.Errorf("Process(%s) = %v, want ErrUnsupportedFormat", name, err)
		}
	}
	if n := behind.processes.Load(); n != 0 {
		t.Errorf("%d refused files reached the plugin", n)
	}
	got, err := p.Inspect(t.Context(), apngImage, media.Limits{})
	if want := (media.Probe{Format: media.FormatPNG, Width: 64, Height: 48, Frames: 1}); err != nil || got != want {
		t.Errorf("Inspect(undeclared apng) = %+v, %v, want %+v", got, err, want)
	}
	if got, err := p.Inspect(t.Context(), testkit.FixtureJPEG(64, 48, 1), media.Limits{}); err != nil || got.Format != media.FormatJPEG {
		t.Errorf("Inspect(jpeg) = %+v, %v", got, err)
	}
}

// assertBuiltInOnly checks that p accepts only the built-in formats: AVIF and HEIC are refused, and an APNG is a still
// PNG, each at once.
func assertBuiltInOnly(t *testing.T, p *mediaplugin.Processor) {
	t.Helper()
	promptly(t, "Inspect", func() {
		for name, data := range map[string][]byte{"avif": avifImage, "heic": heicImage} {
			if _, err := p.Inspect(t.Context(), data, media.Limits{}); !errors.Is(err, media.ErrUnsupportedFormat) {
				t.Errorf("Inspect(%s) = %v, want ErrUnsupportedFormat", name, err)
			}
		}
		got, err := p.Inspect(t.Context(), apngImage, media.Limits{})
		if want := (media.Probe{Format: media.FormatPNG, Width: 64, Height: 48, Frames: 1}); err != nil || got != want {
			t.Errorf("Inspect(apng) = %+v, %v, want %+v", got, err, want)
		}
		if _, err := p.Inspect(t.Context(), testkit.FixturePNG(64, 48, false), media.Limits{}); err != nil {
			t.Errorf("Inspect(png) = %v", err)
		}
	})
}

// A plugin without Describe, one that answers nothing, or one out of reach leaves the built-in formats alone, and Inspect
// never waits on it; New calls nothing, so that until a declaration arrives the built-in formats alone pass too.
func TestWithoutADeclarationTheBuiltInFormatsAlone(t *testing.T) {
	t.Parallel()

	t.Run("unimplemented", func(t *testing.T) {
		t.Parallel()
		behind := newDecoding()
		p, _ := serving(t, behind, fakeplugin.Options{})
		if err := p.Describe(t.Context()); err != nil {
			t.Errorf("Describe of a plugin without it = %v, want nil", err)
		}
		assertBuiltInOnly(t, p)
		if _, err := p.Process(t.Context(), avifImage, media.Limits{}, media.Sizes{}, media.Request{}); !errors.Is(err, media.ErrUnsupportedFormat) {
			t.Errorf("Process(avif) = %v, want ErrUnsupportedFormat", err)
		}
		if n := behind.processes.Load(); n != 0 {
			t.Errorf("%d refused files reached the plugin", n)
		}
	})

	t.Run("an empty answer", func(t *testing.T) {
		t.Parallel()
		p, _ := serving(t, newDecoding(), fakeplugin.Options{MediaInputFormats: allExtended, Broken: true})
		_ = p.Describe(t.Context())
		assertBuiltInOnly(t, p)
	})

	t.Run("before any declaration", func(t *testing.T) {
		t.Parallel()
		p, rec := serving(t, newDecoding(), fakeplugin.Options{MediaInputFormats: allExtended})
		assertBuiltInOnly(t, p)
		if n := rec.Calls(); n != 0 {
			t.Errorf("New and Inspect made %d calls of the plugin", n)
		}
	})

	t.Run("unreachable", func(t *testing.T) {
		t.Parallel()
		p := newProcessorWith(t, kind.Destination{Address: "unix:///nowhere/media.sock"}, mediaplugin.Options{})
		if err := p.Describe(t.Context()); err == nil || media.Rejected(err) {
			t.Errorf("Describe of an unreachable plugin = %v, want a temporary failure", err)
		}
		assertBuiltInOnly(t, p)
		if _, err := p.Process(t.Context(), avifImage, media.Limits{}, media.Sizes{}, media.Request{}); err == nil {
			t.Error("Process(avif) of an unreachable plugin succeeded")
		}
	})

	t.Run("past its deadline", func(t *testing.T) {
		t.Parallel()
		h, _ := fakeplugin.Connect(fakeplugin.Options{Media: newDecoding(), MediaInputFormats: allExtended, Block: true})
		p := newProcessorWith(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h), Timeout: 200 * time.Millisecond}, mediaplugin.Options{})
		if err := p.Describe(t.Context()); err == nil || media.Rejected(err) {
			t.Errorf("Describe of a plugin that holds the call = %v, want a temporary failure", err)
		}
		assertBuiltInOnly(t, p)
	})
}

// Inspect does not wait for a Describe in flight: an upload never waits on the plugin.
func TestInspectDoesNotWaitOnDescribe(t *testing.T) {
	t.Parallel()
	h, rec := fakeplugin.Connect(fakeplugin.Options{Media: newDecoding(), MediaInputFormats: allExtended, Block: true})
	p := newProcessorWith(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h)}, mediaplugin.Options{})
	ctx, cancel := context.WithCancel(t.Context())
	described := make(chan error, 1)
	go func() { described <- p.Describe(ctx) }()
	if !testkit.Eventually(t, within, func() bool { return rec.Calls() > 0 }) {
		t.Fatal("Describe did not reach the plugin")
	}
	assertBuiltInOnly(t, p)
	cancel()
	if err := <-described; err == nil || media.Rejected(err) {
		t.Errorf("Describe ended by its context = %v, want a temporary failure", err)
	}
}

// switchable is a plugin that is down while down is set, answering 503 to every call.
type switchable struct {
	h    http.Handler
	down atomic.Bool
}

func (s *switchable) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.down.Load() {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	s.h.ServeHTTP(w, r)
}

// A failed Describe forgets the last declaration, so that the built-in formats alone pass until a call succeeds again.
func TestAFailedDescribeForgetsTheDeclaration(t *testing.T) {
	t.Parallel()
	h, _ := fakeplugin.Connect(fakeplugin.Options{Media: newDecoding(), MediaInputFormats: allExtended})
	s := &switchable{h: h}
	p := newProcessorWith(t, kind.Destination{Address: fakeplugin.ServeUnix(t, s)}, mediaplugin.Options{})
	describe(t, p)
	if _, err := p.Inspect(t.Context(), avifImage, media.Limits{}); err != nil {
		t.Fatalf("Inspect(declared avif) = %v", err)
	}
	s.down.Store(true)
	if err := p.Describe(t.Context()); err == nil || media.Rejected(err) {
		t.Errorf("Describe of a plugin that is down = %v, want a temporary failure", err)
	}
	assertBuiltInOnly(t, p)
	s.down.Store(false)
	describe(t, p)
	if _, err := p.Inspect(t.Context(), heicImage, media.Limits{}); err != nil {
		t.Errorf("Inspect(declared heic) after the plugin is back = %v", err)
	}
}

// Run calls Describe at once and then every interval until its context ends; without a plugin of images it returns at
// once, and Describe does nothing.
func TestRunDescribesAtOnceAndAtTheInterval(t *testing.T) {
	t.Parallel()

	run := func(t *testing.T, p *mediaplugin.Processor) (stop func()) {
		t.Helper()
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			defer close(done)
			p.Run(ctx)
		}()
		return func() {
			cancel()
			promptly(t, "Run after its context ended", func() { <-done })
		}
	}

	t.Run("at once", func(t *testing.T) {
		t.Parallel()
		h, rec := fakeplugin.Connect(fakeplugin.Options{Media: newDecoding(), MediaInputFormats: allExtended})
		p := newProcessorWith(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h)}, mediaplugin.Options{DescribeInterval: time.Hour})
		stop := run(t, p)
		accepted := testkit.Eventually(t, within, func() bool {
			_, err := p.Inspect(t.Context(), avifImage, media.Limits{})
			return err == nil
		})
		stop()
		if !accepted {
			t.Fatal("Run did not describe the plugin")
		}
		if n := rec.Calls(); n != 1 {
			t.Errorf("Run made %d calls within an hour's interval, want 1", n)
		}
	})

	t.Run("every interval", func(t *testing.T) {
		t.Parallel()
		h, rec := fakeplugin.Connect(fakeplugin.Options{Media: newDecoding(), MediaInputFormats: allExtended})
		p := newProcessorWith(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h)}, mediaplugin.Options{DescribeInterval: 5 * time.Millisecond})
		stop := run(t, p)
		again := testkit.Eventually(t, within, func() bool { return rec.Calls() >= 3 })
		stop()
		if !again {
			t.Fatalf("Run made %d calls", rec.Calls())
		}
	})

	t.Run("without a plugin of images", func(t *testing.T) {
		t.Parallel()
		h, rec := fakeplugin.Connect(fakeplugin.Options{Media: newDecoding(), MediaInputFormats: allExtended})
		p, err := mediaplugin.New(mediaplugin.Options{
			Builtin: native.New(native.Options{}), Video: newConn(t, kind.MediaVideo, kind.Destination{Address: fakeplugin.ServeUnix(t, h)}),
			DescribeInterval: time.Millisecond,
		})
		if err != nil {
			t.Fatal(err)
		}
		promptly(t, "Run without a plugin of images", func() { p.Run(t.Context()) })
		if err := p.Describe(t.Context()); err != nil {
			t.Errorf("Describe without a plugin of images = %v", err)
		}
		if n := rec.Calls(); n != 0 {
			t.Errorf("%d calls reached the plugin of videos", n)
		}
		assertBuiltInOnly(t, p)
	})
}

// Process, which a job runs, describes the plugin first when it meets an extended format before any declaration.
func TestProcessDescribesFirst(t *testing.T) {
	t.Parallel()
	behind := newDecoding()
	p, _ := serving(t, behind, fakeplugin.Options{MediaInputFormats: allExtended})
	out, err := p.Process(t.Context(), heicImage, media.Limits{}, media.Sizes{}, media.Request{})
	if err != nil || out.Width != 64 || out.Height != 48 {
		t.Fatalf("Process(heic) before a declaration = %d×%d, %v", out.Width, out.Height, err)
	}
	if n := behind.processes.Load(); n != 1 {
		t.Errorf("the plugin processed %d files, want 1", n)
	}
	if _, err := p.Inspect(t.Context(), avifImage, media.Limits{}); err != nil {
		t.Errorf("Inspect(avif) after Process described the plugin = %v", err)
	}
}

// Moving outputs are accepted by the frames read from each of them, not from the input: an APNG gets the moving outputs
// the plugin makes, and moving outputs of one frame are ErrCorrupt.
func TestMovingOutputsAreCountedFromTheOutput(t *testing.T) {
	t.Parallel()
	// oneFrame replaces the moving outputs with ones of a single frame, in the formats the port allows.
	oneFrame := func(out *media.Output) {
		large := testkit.FixtureGIF(out.Width, out.Height, 1)
		out.Animated = &media.Variants{
			Small: out.Still.Small, Medium: out.Still.Medium,
			Large: media.Variant{Data: large, ContentType: "image/gif", Extension: "gif", Width: out.Width, Height: out.Height},
		}
	}
	// moving replaces the outputs with those of a GIF of three frames of the same dimensions.
	moving := func(out *media.Output) {
		gif, err := native.New(native.Options{}).Process(context.Background(), testkit.FixtureGIF(out.Width, out.Height, 3), media.Limits{}, media.Sizes{}, media.Request{})
		if err != nil {
			panic(err)
		}
		*out = gif
	}
	jpeg := testkit.FixtureJPEG(64, 48, 1)
	for _, tc := range []struct {
		name       string
		input      []byte
		process    func(out *media.Output)
		wantMoving bool
		want       error
	}{
		{"an APNG", apngImage, nil, true, nil},
		{"an APNG left still", apngImage, func(out *media.Output) { out.Animated = nil }, false, nil},
		{"an APNG with moving outputs of one frame", apngImage, oneFrame, false, media.ErrCorrupt},
		{"a GIF with moving outputs of one frame", testkit.FixtureGIF(64, 48, 3), oneFrame, false, media.ErrCorrupt},
		{"a JPEG with moving outputs of one frame", jpeg, oneFrame, false, media.ErrCorrupt},
		{"a JPEG with moving outputs of three frames", jpeg, moving, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			behind := newDecoding()
			behind.process = tc.process
			p, _ := serving(t, behind, fakeplugin.Options{MediaInputFormats: allExtended})
			describe(t, p)
			out, err := p.Process(t.Context(), tc.input, media.Limits{}, media.Sizes{}, media.Request{})
			if tc.want != nil {
				if !errors.Is(err, tc.want) || !media.Rejected(err) {
					t.Errorf("Process = %v, want %v", err, tc.want)
				}
				return
			}
			if err != nil {
				t.Fatalf("Process = %v", err)
			}
			if (out.Animated != nil) != tc.wantMoving {
				t.Fatalf("moving outputs: %t, want %t", out.Animated != nil, tc.wantMoving)
			}
			if out.Animated != nil && (out.Animated.Large.ContentType != "image/gif" || out.Animated.Small.ContentType != "image/webp") {
				t.Errorf("moving outputs = large %s, small %s", out.Animated.Large.ContentType, out.Animated.Small.ContentType)
			}
		})
	}
}
