// SPDX-License-Identifier: AGPL-3.0-only

package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"slices"
	"time"
)

// Video and audio files are never transcoded (ADR-0005 §9): a processor parses their
// container, checks the codecs and limits, and writes the same streams again without metadata
// (a remux). The accepted containers are MP4 (ISO BMFF, QuickTime included), WebM, MP3, and
// Ogg; the accepted codecs are H.264, VP8, VP9, and AV1 for video and AAC, Opus, Vorbis, and
// MP3 for audio. HEVC is not accepted.

var (
	// ErrUnsupportedCodec reports a video or audio file whose container is accepted but whose
	// codec is not, such as HEVC. It wraps ErrUnsupportedFormat.
	ErrUnsupportedCodec = fmt.Errorf("%w: unsupported codec", ErrUnsupportedFormat)
	// ErrTooLong reports a video or audio file longer than its AVLimits duration.
	ErrTooLong = errors.New("media: too long")
)

// Formats of video and audio containers.
const (
	FormatMP4  Format = "mp4"
	FormatWebM Format = "webm"
	FormatMP3  Format = "mp3"
	FormatOgg  Format = "ogg"
)

// SniffAV detects the video or audio container of data from its leading bytes alone, or
// returns "" when data is in none of the accepted containers.
func SniffAV(data []byte) Format {
	switch {
	case len(data) >= 12 && string(data[4:8]) == "ftyp":
		return FormatMP4
	case bytes.HasPrefix(data, []byte("\x1A\x45\xDF\xA3")):
		return FormatWebM
	case bytes.HasPrefix(data, []byte("OggS")):
		return FormatOgg
	case bytes.HasPrefix(data, []byte("ID3")), len(data) >= 2 && data[0] == 0xFF && data[1]&0xE6 == 0xE2 && data[1]&0x18 != 0x08:
		// An ID3v2 tag, or the sync of a Layer III frame of MPEG 1, 2, or 2.5.
		return FormatMP3
	}
	return ""
}

// declaredTypes are the media types that clients declare for each container.
var declaredTypes = map[Format][]string{
	FormatMP4:  {"video/mp4", "audio/mp4", "audio/x-m4a", "audio/m4a", "video/quicktime", "video/x-m4v", "audio/aac"},
	FormatWebM: {"video/webm", "audio/webm"},
	FormatMP3:  {"audio/mpeg", "audio/mp3", "audio/mpeg3", "audio/x-mpeg-3"},
	FormatOgg:  {"audio/ogg", "application/ogg", "audio/opus", "audio/vorbis", "audio/x-vorbis+ogg"},
}

// Declares reports whether a client that declared the media type declared may have sent a
// file of the container f: a container has several media types (an MP4 may be video/mp4,
// audio/mp4, or video/quicktime). Parameters such as codecs are ignored.
func (f Format) Declares(declared string) bool {
	mt, _, err := mime.ParseMediaType(declared)
	if err != nil {
		return false
	}
	return slices.Contains(declaredTypes[f], mt)
}

// AVKind tells a video file from an audio-only one.
type AVKind string

// Kinds of video and audio files.
const (
	AVVideo AVKind = "video"
	AVAudio AVKind = "audio"
)

// AVLimits bound the video and audio files a processor accepts (ADR-0005 §9).
type AVLimits struct {
	// MaxBytes is the longest accepted file.
	MaxBytes int64
	// MaxVideoDuration and MaxAudioDuration are the longest accepted video and audio file.
	MaxVideoDuration, MaxAudioDuration time.Duration
	// MaxPixels is the largest accepted width × height of a video.
	MaxPixels int64
	// MaxFrameRate is the highest accepted frames per second of a video.
	MaxFrameRate float64
}

// Default video and audio limits (ADR-0019 §8, ADR-0005 §9).
const (
	DefaultMaxAVBytes       = 100 << 20
	DefaultMaxVideoDuration = 5 * time.Minute
	DefaultMaxAudioDuration = 10 * time.Minute
	DefaultMaxVideoPixels   = 3840 * 2160
	DefaultMaxFrameRate     = 120
)

// DefaultAVLimits returns the limits of ADR-0005 §9
func DefaultAVLimits() AVLimits {
	return AVLimits{
		MaxBytes: DefaultMaxAVBytes, MaxVideoDuration: DefaultMaxVideoDuration, MaxAudioDuration: DefaultMaxAudioDuration,
		MaxPixels: DefaultMaxVideoPixels, MaxFrameRate: DefaultMaxFrameRate,
	}
}

// WithDefaults returns l with its zero or negative fields set to the defaults.
func (l AVLimits) WithDefaults() AVLimits {
	d := DefaultAVLimits()
	if l.MaxBytes <= 0 {
		l.MaxBytes = d.MaxBytes
	}
	if l.MaxVideoDuration <= 0 {
		l.MaxVideoDuration = d.MaxVideoDuration
	}
	if l.MaxAudioDuration <= 0 {
		l.MaxAudioDuration = d.MaxAudioDuration
	}
	if l.MaxPixels <= 0 {
		l.MaxPixels = d.MaxPixels
	}
	if l.MaxFrameRate <= 0 {
		l.MaxFrameRate = d.MaxFrameRate
	}
	return l
}

// MaxDuration returns the longest accepted file of kind.
func (l AVLimits) MaxDuration(kind AVKind) time.Duration {
	if kind == AVVideo {
		return l.MaxVideoDuration
	}
	return l.MaxAudioDuration
}

// AVProbe is what a processor learns from the container of a video or audio file.
type AVProbe struct {
	Format Format
	Kind   AVKind
	// ContentType is the media type of the file as served: video/mp4, audio/mp4, video/webm,
	// audio/webm, audio/mpeg, or audio/ogg.
	ContentType string
	// Extension is the file name extension for ContentType, without the dot ("mp4").
	Extension string
	// Width and Height are the display dimensions of a video, after its rotation; zero for
	// audio.
	Width, Height int
	Duration      time.Duration
	// VideoCodec and AudioCodec name the codecs ("h264", "vp9", "aac", "opus"), "" for none.
	VideoCodec, AudioCodec string
}

// AVOutput is a remuxed video or audio file.
type AVOutput struct {
	AVProbe
	// Data is the file without metadata.
	Data []byte
	// Cover is the cover image embedded in the file (ID3 APIC, MP4 covr), nil when it has none.
	// It is not checked: process it as an image.
	Cover []byte
}

// AVProcessor is the part of the MediaProcessor port for video and audio files (ADR-0005 §9).
// Zero fields of AVLimits take their defaults.
type AVProcessor interface {
	// InspectAV parses the container of data and checks its codecs and limits without writing
	// it. It fails with an error wrapping ErrUnsupportedFormat (ErrUnsupportedCodec for a
	// codec that is not accepted), ErrTooLarge, ErrDimensions, ErrTooLong, or ErrCorrupt.
	InspectAV(ctx context.Context, data []byte, limits AVLimits) (AVProbe, error)
	// Remux inspects data as InspectAV does and writes its streams again without metadata:
	// tags, titles, dates, locations, embedded pictures, and tracks other than the video and
	// audio. The same input gives the same output.
	Remux(ctx context.Context, data []byte, limits AVLimits) (AVOutput, error)
}
