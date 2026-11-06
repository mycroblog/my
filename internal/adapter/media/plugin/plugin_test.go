// SPDX-License-Identifier: AGPL-3.0-only

package plugin_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/media/native"
	mediaplugin "github.com/mycroblog/my/internal/adapter/media/plugin"
	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/pkg/testkit"
)

func mediaKind(t *testing.T, name string) kind.Kind {
	t.Helper()
	k, ok := kind.ByName(name)
	if !ok {
		t.Fatalf("no kind %s", name)
	}
	return k
}

// newConn returns the Conn of the kind name to d.
func newConn(t *testing.T, name string, d kind.Destination) *client.Conn {
	t.Helper()
	conn, err := client.New(client.Options{Kind: mediaKind(t, name), Destination: d})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	return conn
}

// counting is the built-in processor, counting the calls the adapter makes of it.
type counting struct {
	*native.Processor

	inspects, previews atomic.Int64
}

func (c *counting) Inspect(ctx context.Context, data []byte, limits media.Limits) (media.Probe, error) {
	c.inspects.Add(1)
	return c.Processor.Inspect(ctx, data, limits)
}

func (c *counting) Preview(ctx context.Context, data []byte, limits media.Limits, sizes media.Sizes, req media.Request) (media.Variant, error) {
	c.previews.Add(1)
	return c.Processor.Preview(ctx, data, limits, sizes, req)
}

// newProcessor returns the Processor that sends every kind of media to the one plugin at d.
func newProcessor(t *testing.T, d kind.Destination, builtin media.Processor) *mediaplugin.Processor {
	t.Helper()
	p, err := mediaplugin.New(mediaplugin.Options{
		Builtin: builtin,
		Image:   newConn(t, kind.MediaImage, d), Video: newConn(t, kind.MediaVideo, d), Audio: newConn(t, kind.MediaAudio, d),
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// serving returns a Processor on the Connect fake over a Unix socket with the processor behind it.
func serving(t *testing.T, behind media.Processor, o fakeplugin.Options) (*mediaplugin.Processor, *fakeplugin.Recorder) {
	t.Helper()
	o.Media = behind
	h, rec := fakeplugin.Connect(o)
	return newProcessor(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h)}, native.New(native.Options{Concurrency: 2})), rec
}

// The adapter passes the contract of the port against the fake, over the protocols it speaks; plain TCP is h2c.
func TestContract(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		protocol string
		serve    func(testing.TB, http.Handler) string
	}{
		{"proto_unix", kind.ProtocolProto, fakeplugin.ServeUnix},
		{"grpc_plain_tcp", kind.ProtocolGRPC, fakeplugin.ServeTCP},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, rec := fakeplugin.Connect(fakeplugin.Options{Media: native.New(native.Options{Concurrency: 2})})
			testkit.MediaProcessorContract(t, newProcessor(t, kind.Destination{Address: tc.serve(t, h), Protocol: tc.protocol}, native.New(native.Options{Concurrency: 2})))
			if rec.Calls() == 0 || rec.Proto() != "HTTP/2.0" {
				t.Errorf("%d calls, the last over %s, want HTTP/2.0", rec.Calls(), rec.Proto())
			}
		})
	}
}

// Every way to reach a plugin carries the stream over HTTP/2: h2c on plain TCP and a Unix socket, h2 through ALPN on TLS
// with a client certificate and a token.
func TestEveryTransportStreamsOverHTTP2(t *testing.T) {
	t.Parallel()
	behind := native.New(native.Options{Concurrency: 2})
	tlsFiles := fakeplugin.NewTLSFiles(t, true)
	for _, tc := range []struct {
		name  string
		token string
		serve func(http.Handler) kind.Destination
	}{
		{"proto_plain_tcp", "", func(h http.Handler) kind.Destination {
			return kind.Destination{Address: fakeplugin.ServeTCP(t, h), Protocol: kind.ProtocolProto}
		}},
		{"proto_unix", "", func(h http.Handler) kind.Destination {
			return kind.Destination{Address: fakeplugin.ServeUnix(t, h), Protocol: kind.ProtocolProto}
		}},
		{"proto_mtls_token", "s3cret", func(h http.Handler) kind.Destination {
			return kind.Destination{
				Address: fakeplugin.ServeTLS(t, h, tlsFiles.Server), Protocol: kind.ProtocolProto, Token: "s3cret",
				TLSCAFile: tlsFiles.CAFile, TLSCertFile: tlsFiles.CertFile, TLSKeyFile: tlsFiles.KeyFile,
			}
		}},
		{"grpc_mtls", "", func(h http.Handler) kind.Destination {
			return kind.Destination{
				Address: fakeplugin.ServeTLS(t, h, tlsFiles.Server), Protocol: kind.ProtocolGRPC,
				TLSCAFile: tlsFiles.CAFile, TLSCertFile: tlsFiles.CertFile, TLSKeyFile: tlsFiles.KeyFile,
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, rec := fakeplugin.Connect(fakeplugin.Options{Media: behind, Token: tc.token})
			p := newProcessor(t, tc.serve(h), behind)
			out, err := p.Process(t.Context(), testkit.FixtureJPEG(64, 48, 1), media.Limits{}, media.Sizes{}, media.Request{})
			if err != nil {
				t.Fatal(err)
			}
			if out.Width != 64 || out.Height != 48 || out.Still.Large.ContentType != "image/jpeg" {
				t.Errorf("output = %d×%d, large %s", out.Width, out.Height, out.Still.Large.ContentType)
			}
			if rec.Proto() != "HTTP/2.0" {
				t.Errorf("the call went over %s", rec.Proto())
			}
		})
	}
}

// Inspect, InspectAV, and Preview never reach the plugin, and Process inspects its input with the built-in processor
// before the call, so that a file the server refuses is never sent.
func TestTheBuiltInAnswersWhatIsNotDelegated(t *testing.T) {
	t.Parallel()
	builtin := &counting{Processor: native.New(native.Options{})}
	h, rec := fakeplugin.Connect(fakeplugin.Options{Media: native.New(native.Options{})})
	p := newProcessor(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h)}, builtin)
	jpeg := testkit.FixtureJPEG(64, 48, 1)
	if _, err := p.Inspect(t.Context(), jpeg, media.Limits{}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Preview(t.Context(), jpeg, media.Limits{}, media.Sizes{}, media.Request{}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.InspectAV(t.Context(), testkit.MP3File(10, nil), media.AVLimits{}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Process(t.Context(), []byte("<svg/>"), media.Limits{}, media.Sizes{}, media.Request{}); !errors.Is(err, media.ErrUnsupportedFormat) {
		t.Errorf("Process(svg) = %v", err)
	}
	if _, err := p.Remux(t.Context(), []byte("<svg/>"), media.AVLimits{}); !media.Rejected(err) {
		t.Errorf("Remux(svg) = %v", err)
	}
	if n := rec.Calls(); n != 0 {
		t.Errorf("%d calls reached the plugin", n)
	}
	if builtin.inspects.Load() < 2 || builtin.previews.Load() != 1 {
		t.Errorf("the built-in processor had %d inspections and %d previews", builtin.inspects.Load(), builtin.previews.Load())
	}
}

// lying is a plugin's processor that returns what its functions say instead of the built-in's output.
type lying struct {
	*native.Processor

	process func(out *media.Output)
	remux   func(out *media.AVOutput)
}

func (l lying) Process(ctx context.Context, data []byte, limits media.Limits, sizes media.Sizes, req media.Request) (media.Output, error) {
	out, err := l.Processor.Process(ctx, data, limits, sizes, req)
	if err == nil && l.process != nil {
		l.process(&out)
	}
	return out, err
}

func (l lying) Remux(ctx context.Context, data []byte, limits media.AVLimits) (media.AVOutput, error) {
	out, err := l.Processor.Remux(ctx, data, limits)
	if err == nil && l.remux != nil {
		l.remux(&out)
	}
	return out, err
}

// The output is untrusted: an output larger than the limits or its part is ErrTooLarge, and one whose content is not
// what its part declares or what the port allows is ErrCorrupt, as when the built-in processor rejects a file.
func TestTheServerChecksTheOutput(t *testing.T) {
	t.Parallel()
	jpeg := testkit.FixtureJPEG(64, 48, 1)
	gif := testkit.FixtureGIF(64, 48, 3)
	png := testkit.FixturePNG(64, 48, false)
	webm := testkit.WebMFile(testkit.WebMOptions{VideoCodec: "V_VP9", Width: 640, Height: 360, Frames: 30, Seconds: 1})
	mp4 := testkit.MP4File(testkit.MP4Options{VideoCodec: "avc1", Width: 640, Height: 360, Frames: 30, Seconds: 1})
	limits := media.Limits{MaxBytes: int64(len(jpeg) + 1000)}
	for _, tc := range []struct {
		name       string
		input      []byte
		understate bool
		process    func(out *media.Output)
		want       error
	}{
		{"larger than the limit", jpeg, false, func(out *media.Output) {
			out.Still.Small.Data = append(bytes.Clone(out.Still.Small.Data), make([]byte, limits.MaxBytes)...)
		}, media.ErrTooLarge},
		{"longer than its part", jpeg, true, nil, media.ErrTooLarge},
		{"another format than it declares", jpeg, false, func(out *media.Output) {
			out.Still.Small.Data = png // declared WebP
		}, media.ErrCorrupt},
		{"a format the port does not allow", jpeg, false, func(out *media.Output) {
			out.Still.Small = media.Variant{Data: png, ContentType: "image/png", Width: 64, Height: 48}
		}, media.ErrCorrupt},
		{"other dimensions than it declares", jpeg, false, func(out *media.Output) {
			out.Still.Small.Width++
		}, media.ErrCorrupt},
		{"outside its box", testkit.FixtureJPEG(2000, 1000, 1), false, func(out *media.Output) {
			out.Still.Small = out.Still.Medium // 2000 × 1000 pixels, more than the 640 × 360 of the small box
		}, media.ErrCorrupt},
		{"an empty output", jpeg, false, func(out *media.Output) {
			out.Still.Medium = media.Variant{}
		}, media.ErrCorrupt},
		{"moving outputs of a still image", jpeg, false, func(out *media.Output) {
			out.Animated = &media.Variants{Small: out.Still.Small, Medium: out.Still.Medium, Large: out.Still.Large}
		}, media.ErrCorrupt},
		{"other dimensions of the image", jpeg, false, func(out *media.Output) {
			out.Width, out.Height = 48, 48
		}, media.ErrCorrupt},
		{"a broken blurhash", jpeg, false, func(out *media.Output) {
			out.Blurhash = "not a blurhash"
		}, media.ErrCorrupt},
		{"an animation left still", gif, false, func(out *media.Output) {
			out.Animated = nil
		}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			l := limits
			if len(tc.input) > len(jpeg) {
				l = media.Limits{}
			}
			p, _ := serving(t, lying{Processor: native.New(native.Options{}), process: tc.process}, fakeplugin.Options{MediaUnderstate: tc.understate})
			out, err := p.Process(t.Context(), tc.input, l, media.Sizes{}, media.Request{})
			if tc.want == nil {
				if err != nil {
					t.Fatalf("Process = %v", err)
				}
				if tc.name == "an animation left still" && out.Animated != nil {
					t.Error("moving outputs appeared")
				}
				return
			}
			if !errors.Is(err, tc.want) || !media.Rejected(err) {
				t.Errorf("Process = %v, want %v", err, tc.want)
			}
		})
	}

	t.Run("a JPEG large emoji", func(t *testing.T) {
		t.Parallel()
		p, _ := serving(t, lying{Processor: native.New(native.Options{}), process: func(out *media.Output) {
			out.Still.Large = media.Variant{Data: jpeg, ContentType: "image/jpeg", Width: 64, Height: 48}
		}}, fakeplugin.Options{})
		if _, err := p.Process(t.Context(), jpeg, media.Limits{}, media.Sizes{}, media.Request{Purpose: media.PurposeEmoji}); !errors.Is(err, media.ErrCorrupt) {
			t.Errorf("Process = %v, want ErrCorrupt", err)
		}
	})

	for _, tc := range []struct {
		name  string
		remux func(out *media.AVOutput)
		want  error
	}{
		{"a body of another container", func(out *media.AVOutput) {
			out.Data = webm
		}, media.ErrCorrupt},
		{"a body over the limit", func(out *media.AVOutput) {
			out.Data = append(bytes.Clone(out.Data), make([]byte, media.DefaultMaxAVBytes)...)
		}, media.ErrTooLarge},
		{"a body that does not parse", func(out *media.AVOutput) {
			out.Data = out.Data[:len(out.Data)-40]
		}, media.ErrCorrupt},
		{"a cover over the limit", func(out *media.AVOutput) {
			out.Cover = append(bytes.Clone(png), make([]byte, media.DefaultMaxBytes)...)
		}, media.ErrTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, _ := serving(t, lying{Processor: native.New(native.Options{}), remux: tc.remux}, fakeplugin.Options{})
			_, err := p.Remux(t.Context(), mp4, media.AVLimits{})
			if !errors.Is(err, tc.want) || !media.Rejected(err) {
				t.Errorf("Remux = %v, want %v", err, tc.want)
			}
		})
	}
}

// ContentType and Extension come from the detected format, whatever the plugin's processor called them; the probe of a
// remuxed file comes from the built-in processor's reading of it.
func TestTheTypeComesFromTheContent(t *testing.T) {
	t.Parallel()
	p, _ := serving(t, lying{
		Processor: native.New(native.Options{}),
		process: func(out *media.Output) {
			out.Still.Large.Extension = "exe"
			out.Still.Small.Extension = "html"
		},
		remux: func(out *media.AVOutput) {
			out.ContentType, out.Extension, out.Duration, out.AudioCodec = "text/html", "html", time.Hour, "flac"
		},
	}, fakeplugin.Options{})
	out, err := p.Process(t.Context(), testkit.FixtureJPEG(64, 48, 1), media.Limits{}, media.Sizes{}, media.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if v := out.Still.Large; v.ContentType != "image/jpeg" || v.Extension != "jpg" {
		t.Errorf("large = %s .%s", v.ContentType, v.Extension)
	}
	if v := out.Still.Small; v.ContentType != "image/webp" || v.Extension != "webp" {
		t.Errorf("small = %s .%s", v.ContentType, v.Extension)
	}
	mp3 := testkit.MP3File(10, nil)
	want, err := native.New(native.Options{}).InspectAV(t.Context(), mp3, media.AVLimits{})
	if err != nil {
		t.Fatal(err)
	}
	av, err := p.Remux(t.Context(), mp3, media.AVLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if av.AVProbe != want {
		t.Errorf("probe = %+v, want %+v", av.AVProbe, want)
	}
}

// A failure of the plugin is a temporary failure, as one of the built-in processor is: the job retries it. Its own
// rejections are the port's, which mark the file as failed.
func TestFailuresAreThoseOfTheBuiltIn(t *testing.T) {
	t.Parallel()
	jpeg := testkit.FixtureJPEG(64, 48, 1)
	behind := native.New(native.Options{})
	for _, tc := range []struct {
		name string
		o    fakeplugin.Options
	}{
		{"an empty head", fakeplugin.Options{Broken: true}},
		{"no media service", fakeplugin.Options{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := tc.o
			if tc.name != "no media service" {
				o.Media = behind
			}
			h, _ := fakeplugin.Connect(o)
			p := newProcessor(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h)}, behind)
			if _, err := p.Process(t.Context(), jpeg, media.Limits{}, media.Sizes{}, media.Request{}); err == nil || media.Rejected(err) {
				t.Errorf("Process = %v, want a temporary failure", err)
			}
			if _, err := p.Remux(t.Context(), testkit.MP3File(10, nil), media.AVLimits{}); err == nil || media.Rejected(err) {
				t.Errorf("Remux = %v, want a temporary failure", err)
			}
		})
	}

	t.Run("down", func(t *testing.T) {
		t.Parallel()
		p := newProcessor(t, kind.Destination{Address: "unix:///nowhere/media.sock"}, behind)
		if _, err := p.Process(t.Context(), jpeg, media.Limits{}, media.Sizes{}, media.Request{}); err == nil || media.Rejected(err) {
			t.Errorf("Process = %v, want a temporary failure", err)
		}
	})

	t.Run("deadline", func(t *testing.T) {
		t.Parallel()
		h, _ := fakeplugin.Connect(fakeplugin.Options{Media: behind, Block: true})
		p := newProcessor(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h), Timeout: 200 * time.Millisecond}, behind)
		if _, err := p.Process(t.Context(), jpeg, media.Limits{}, media.Sizes{}, media.Request{}); err == nil || media.Rejected(err) {
			t.Errorf("Process = %v, want a temporary failure", err)
		}
	})

	t.Run("rejections", func(t *testing.T) {
		t.Parallel()
		for _, want := range []error{media.ErrUnsupportedFormat, media.ErrUnsupportedCodec, media.ErrTooLarge, media.ErrDimensions, media.ErrCorrupt, media.ErrTooLong} {
			p, _ := serving(t, refusing{Processor: behind, err: want}, fakeplugin.Options{})
			if _, err := p.Process(t.Context(), jpeg, media.Limits{}, media.Sizes{}, media.Request{}); !errors.Is(err, want) {
				t.Errorf("Process = %v, want %v", err, want)
			}
			if _, err := p.Remux(t.Context(), testkit.MP3File(10, nil), media.AVLimits{}); !errors.Is(err, want) {
				t.Errorf("Remux = %v, want %v", err, want)
			}
		}
		p, _ := serving(t, refusing{Processor: behind, err: errors.New("disk full")}, fakeplugin.Options{})
		if _, err := p.Process(t.Context(), jpeg, media.Limits{}, media.Sizes{}, media.Request{}); err == nil || media.Rejected(err) {
			t.Errorf("Process of OUTCOME_TEMPORARY = %v, want a temporary failure", err)
		}
	})
}

// refusing is a plugin's processor that fails every call with err.
type refusing struct {
	*native.Processor

	err error
}

func (r refusing) Process(context.Context, []byte, media.Limits, media.Sizes, media.Request) (media.Output, error) {
	return media.Output{}, r.err
}

func (r refusing) Remux(context.Context, []byte, media.AVLimits) (media.AVOutput, error) {
	return media.AVOutput{}, r.err
}

// shallow reads only the leading bytes of an audio file, so that the memory a test measures is the transport's: the
// built-in processor parses the whole file, which costs what it costs without a plugin too.
type shallow struct {
	*native.Processor
}

func (shallow) InspectAV(_ context.Context, data []byte, _ media.AVLimits) (media.AVProbe, error) {
	return media.AVProbe{Format: media.SniffAV(data), Kind: media.AVAudio, ContentType: "audio/mpeg", Extension: "mp3", AudioCodec: "mp3"}, nil
}

// Remux returns the file as it is, as a plugin's processor.
func (s shallow) Remux(ctx context.Context, data []byte, limits media.AVLimits) (media.AVOutput, error) {
	probe, err := s.InspectAV(ctx, data, limits)
	return media.AVOutput{AVProbe: probe, Data: data}, err
}

// A body of 100 MiB, the largest video or audio file, crosses both ways in chunks of 1 MiB over plain TCP, and memory
// stays bounded: each end holds one buffer of the body, of the size its sender declared, and the chunks it decodes one
// at a time, instead of growing a buffer or holding the messages.
func TestA100MiBBodyRoundTrips(t *testing.T) {
	data := make([]byte, media.DefaultMaxAVBytes)
	copy(data, testkit.MP3File(1, nil))
	behind := shallow{Processor: native.New(native.Options{})}
	h, rec := fakeplugin.Connect(fakeplugin.Options{Media: behind})
	p := newProcessor(t, kind.Destination{Address: fakeplugin.ServeTCP(t, h)}, behind)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	out, err := p.Remux(t.Context(), data, media.AVLimits{})
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Data, data) {
		t.Fatal("the body changed on the way")
	}
	if rec.Proto() != "HTTP/2.0" {
		t.Errorf("the call went over %s", rec.Proto())
	}
	// Both ends run in this process, so the count holds four bodies, the buffer and the decoded chunks of each end, and the
	// buffers of the transport, which stay at tens of chunks.
	// Growing a buffer by appending, or keeping the messages, would pass it.
	if allocated, most := after.TotalAlloc-before.TotalAlloc, uint64(4*len(data)+64<<20); allocated > most && !raceEnabled {
		t.Errorf("the round trip allocated %d MiB for a body of %d MiB, more than %d MiB", allocated>>20, len(data)>>20, most>>20)
	}
}

// The adapter needs the built-in processor and at least one plugin.
func TestNewNeedsTheBuiltInAndAPlugin(t *testing.T) {
	t.Parallel()
	conn := newConn(t, kind.MediaImage, kind.Destination{Address: "unix:///run/media.sock"})
	if _, err := mediaplugin.New(mediaplugin.Options{Image: conn}); err == nil || !strings.Contains(err.Error(), "built-in") {
		t.Errorf("New without the built-in = %v", err)
	}
	if _, err := mediaplugin.New(mediaplugin.Options{Builtin: native.New(native.Options{})}); err == nil || !strings.Contains(err.Error(), "plugin") {
		t.Errorf("New without a plugin = %v", err)
	}
}

// Each kind of media goes to its own plugin, with the kind of a video or audio file in the head, and a kind without a
// plugin stays on the built-in processor.
func TestEachKindGoesToItsPlugin(t *testing.T) {
	t.Parallel()
	behind := native.New(native.Options{Concurrency: 2})
	serve := func() (string, *fakeplugin.Recorder) {
		h, rec := fakeplugin.Connect(fakeplugin.Options{Media: behind})
		return fakeplugin.ServeUnix(t, h), rec
	}
	video := testkit.MP4File(testkit.MP4Options{VideoCodec: "avc1", AudioCodec: "mp4a", Width: 64, Height: 48, Frames: 10, Seconds: 1})
	audio := testkit.MP4File(testkit.MP4Options{AudioCodec: "mp4a", Seconds: 1})
	image := testkit.FixtureJPEG(64, 48, 1)
	for _, tc := range []struct {
		name                string
		image, video, audio bool
	}{
		{"images only", true, false, false},
		{"videos only", false, true, false},
		{"audio only", false, false, true},
		{"every kind", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := mediaplugin.Options{Builtin: behind}
			recs := map[string]*fakeplugin.Recorder{}
			for _, k := range []struct {
				on   bool
				name string
				conn **client.Conn
			}{{tc.image, kind.MediaImage, &o.Image}, {tc.video, kind.MediaVideo, &o.Video}, {tc.audio, kind.MediaAudio, &o.Audio}} {
				if k.on {
					address, rec := serve()
					*k.conn, recs[k.name] = newConn(t, k.name, kind.Destination{Address: address}), rec
				}
			}
			p, err := mediaplugin.New(o)
			if err != nil {
				t.Fatal(err)
			}
			ctx := t.Context()
			if _, err := p.Process(ctx, image, media.Limits{}, media.Sizes{}, media.Request{}); err != nil {
				t.Fatalf("Process: %v", err)
			}
			if _, err := p.Remux(ctx, video, media.AVLimits{}); err != nil {
				t.Fatalf("Remux of a video: %v", err)
			}
			if _, err := p.Remux(ctx, audio, media.AVLimits{}); err != nil {
				t.Fatalf("Remux of an audio file: %v", err)
			}
			for name, rec := range recs {
				if rec.Calls() != 1 {
					t.Errorf("the %s plugin got %d calls, want 1", name, rec.Calls())
				}
			}
		})
	}
}
