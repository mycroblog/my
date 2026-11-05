// SPDX-License-Identifier: AGPL-3.0-only

// Package plugin is the media.Processor that delegates Process and Remux to an operator's plugin, so that heavy
// conversion runs on another machine (ADR-0146 §11, §13). Inspect, InspectAV, and Preview stay on the server's built-in
// processor: they run in the request of an upload and on the read path of the media proxy, which take no plugin's delay.
//
// The bodies travel in a stream of chunks both ways, half-duplex: the server sends the head and the file, closes its side,
// and reads the plugin's head and its outputs. The output is untrusted (ADR-0019 §8): the server detects the format of
// each output from its content, checks it with the built-in processor under the same limits, and checks that it fits its
// box. ContentType and Extension come from the detected format, never from the plugin. An output that breaks a rule is
// media.ErrCorrupt, and one larger than its part or the limits is media.ErrTooLarge, so that media.Rejected marks the file
// as failed as the built-in processor's rejections do. A failure of the call is a temporary one, which the job retries.
//
// The port takes and returns whole files, so the server holds the input and the outputs in memory as with the built-in
// processor, and no more: every output is read into a buffer of its declared size, which the limits bound first.
package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	mediav1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/media/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/media/v1/mediav1connect"
	"github.com/mycroblog/my/internal/app/media"
)

// ChunkSize is the most bytes of a body in one message (ADR-0146 §13).
const ChunkSize = 1 << 20

// Options configure a Processor.
type Options struct {
	// Builtin is the server's own processor. It answers Inspect, InspectAV, and Preview, checks every input before the
	// call, and checks every output after it. Required.
	Builtin media.Processor
	// CoverMaxBytes bounds the cover picture of Remux, which the server later processes as an image: the image limit of
	// the settings. Zero or less is media.DefaultMaxBytes.
	CoverMaxBytes int64
}

// Processor is the media.Processor of a plugin. It is safe for concurrent use.
type Processor struct {
	client   mediav1connect.MediaServiceClient
	builtin  media.Processor
	coverMax int64
}

var _ media.Processor = (*Processor)(nil)

// New returns the Processor that reaches its plugin through conn. It makes no call.
func New(conn *client.Conn, o Options) (*Processor, error) {
	if o.Builtin == nil {
		return nil, errors.New("media plugin: the built-in processor is required")
	}
	if o.CoverMaxBytes <= 0 {
		o.CoverMaxBytes = media.DefaultMaxBytes
	}
	return &Processor{client: mediav1connect.NewMediaServiceClient(conn.RPC), builtin: o.Builtin, coverMax: o.CoverMaxBytes}, nil
}

// Inspect implements media.Processor with the built-in processor.
func (p *Processor) Inspect(ctx context.Context, data []byte, limits media.Limits) (media.Probe, error) {
	return p.builtin.Inspect(ctx, data, limits)
}

// InspectAV implements media.AVProcessor with the built-in processor.
func (p *Processor) InspectAV(ctx context.Context, data []byte, limits media.AVLimits) (media.AVProbe, error) {
	return p.builtin.InspectAV(ctx, data, limits)
}

// Preview implements media.Processor with the built-in processor.
func (p *Processor) Preview(ctx context.Context, data []byte, limits media.Limits, sizes media.Sizes, req media.Request) (media.Variant, error) {
	return p.builtin.Preview(ctx, data, limits, sizes, req)
}

var purposes = map[media.Purpose]mediav1.Purpose{
	media.PurposeAttachment: mediav1.Purpose_PURPOSE_ATTACHMENT,
	media.PurposeAvatar:     mediav1.Purpose_PURPOSE_AVATAR,
	media.PurposeHeader:     mediav1.Purpose_PURPOSE_HEADER,
	media.PurposeEmoji:      mediav1.Purpose_PURPOSE_EMOJI,
}

// Process implements media.Processor: the built-in processor inspects data, the plugin makes the variants, and the
// server checks them.
func (p *Processor) Process(ctx context.Context, data []byte, limits media.Limits, sizes media.Sizes, req media.Request) (media.Output, error) {
	limits, sizes = limits.WithDefaults(), sizes.WithDefaults()
	if !req.Purpose.Valid() {
		req.Purpose = media.PurposeAttachment
	}
	probe, err := p.builtin.Inspect(ctx, data, limits)
	if err != nil {
		return media.Output{}, err
	}
	boxes := sizes.For(req.Purpose)

	stream, err := p.client.Process(ctx)
	if err != nil {
		return media.Output{}, failed(err)
	}
	defer func() { _ = stream.Close() }()
	head := &mediav1.ProcessHead{
		Limits: &mediav1.Limits{
			MaxBytes: limits.MaxBytes, MaxSide: int32(min(limits.MaxSide, maxInt32)), //nolint:gosec // bounded by min
			MaxPixels: limits.MaxPixels, MaxFrames: int32(min(limits.MaxFrames, maxInt32)), //nolint:gosec // bounded by min
		},
		Purpose: purposes[req.Purpose],
		Small:   box(boxes.Small), Medium: box(boxes.Medium), Large: box(boxes.Large),
		FocusX: req.FocusX, FocusY: req.FocusY,
		Size: int64(len(data)),
	}
	err = send(data,
		func() error {
			return stream.Send(&mediav1.ProcessRequest{Message: &mediav1.ProcessRequest_Head{Head: head}})
		},
		func(chunk []byte) error {
			return stream.Send(&mediav1.ProcessRequest{Message: &mediav1.ProcessRequest_Chunk{Chunk: chunk}})
		},
		stream.CloseSend)
	if err != nil {
		return media.Output{}, failed(err)
	}

	first, err := stream.Receive()
	if err != nil {
		return media.Output{}, failed(eof(err))
	}
	result := first.GetHead()
	if err := outcome(result.GetOutcome()); err != nil {
		return media.Output{}, err
	}
	parts, err := receive(func() (message, error) {
		res, err := stream.Receive()
		if err != nil {
			return message{}, err
		}
		switch m := res.GetMessage().(type) {
		case *mediav1.ProcessResponse_Part:
			return message{part: m.Part}, nil
		case *mediav1.ProcessResponse_Chunk:
			return message{chunk: m.Chunk, isChunk: true}, nil
		}
		return message{}, fmt.Errorf("%w: a second head or an empty message", errUnexpected)
	}, 2*limits.MaxBytes, func(o mediav1.Output) (int64, bool) {
		if o >= mediav1.Output_OUTPUT_STILL_SMALL && o <= mediav1.Output_OUTPUT_ANIMATED_LARGE {
			return limits.MaxBytes, true
		}
		return 0, false
	})
	if err != nil {
		return media.Output{}, err
	}
	return p.output(ctx, probe, result, parts, limits, boxes, req.Purpose)
}

// output checks what the plugin returned for an image of probe and makes the Output of it.
func (p *Processor) output(ctx context.Context, probe media.Probe, result *mediav1.ProcessResult, parts map[mediav1.Output]received,
	limits media.Limits, boxes media.Boxes, purpose media.Purpose,
) (media.Output, error) {
	w, h := int(result.GetWidth()), int(result.GetHeight())
	if (w != probe.Width || h != probe.Height) && (w != probe.Height || h != probe.Width) {
		return media.Output{}, fmt.Errorf("media plugin: %w: the dimensions %d×%d are not those of the image, %d×%d either way", media.ErrCorrupt, w, h, probe.Width, probe.Height)
	}
	if !validBlurhash(result.GetBlurhash()) {
		return media.Output{}, fmt.Errorf("media plugin: %w: the blurhash is not one", media.ErrCorrupt)
	}
	large := []media.Format{media.FormatJPEG, media.FormatPNG}
	if purpose == media.PurposeEmoji {
		large = []media.Format{media.FormatPNG}
	}
	webp := []media.Format{media.FormatWebP}
	out := media.Output{Width: w, Height: h, Blurhash: result.GetBlurhash()}
	var err error
	for _, v := range []struct {
		output  mediav1.Output
		into    *media.Variant
		formats []media.Format
		box     media.Box
	}{
		{mediav1.Output_OUTPUT_STILL_SMALL, &out.Still.Small, webp, boxes.Small},
		{mediav1.Output_OUTPUT_STILL_MEDIUM, &out.Still.Medium, webp, boxes.Medium},
		{mediav1.Output_OUTPUT_STILL_LARGE, &out.Still.Large, large, boxes.Large},
	} {
		if *v.into, err = p.variant(ctx, parts, v.output, v.formats, v.box, limits); err != nil {
			return media.Output{}, err
		}
	}
	_, small := parts[mediav1.Output_OUTPUT_ANIMATED_SMALL]
	_, medium := parts[mediav1.Output_OUTPUT_ANIMATED_MEDIUM]
	_, big := parts[mediav1.Output_OUTPUT_ANIMATED_LARGE]
	if !small && !medium && !big {
		// A plugin may leave an animation still: its still variants show the first frame.
		return out, nil
	}
	if probe.Frames < 2 {
		return media.Output{}, fmt.Errorf("media plugin: %w: moving outputs of an image of one frame", media.ErrCorrupt)
	}
	out.Animated = new(media.Variants)
	for _, v := range []struct {
		output  mediav1.Output
		into    *media.Variant
		formats []media.Format
		box     media.Box
	}{
		{mediav1.Output_OUTPUT_ANIMATED_SMALL, &out.Animated.Small, webp, boxes.Small},
		{mediav1.Output_OUTPUT_ANIMATED_MEDIUM, &out.Animated.Medium, webp, boxes.Medium},
		{mediav1.Output_OUTPUT_ANIMATED_LARGE, &out.Animated.Large, []media.Format{media.FormatGIF}, boxes.Large},
	} {
		if *v.into, err = p.variant(ctx, parts, v.output, v.formats, v.box, limits); err != nil {
			return media.Output{}, err
		}
	}
	return out, nil
}

// variant checks one image output: its detected format is the one its part declares and one the port allows, the
// built-in processor accepts it under the limits, its dimensions are those its part declares, and it fits its box.
func (p *Processor) variant(ctx context.Context, parts map[mediav1.Output]received, o mediav1.Output, formats []media.Format, b media.Box, limits media.Limits) (media.Variant, error) {
	r, ok := parts[o]
	if !ok {
		return media.Variant{}, fmt.Errorf("media plugin: %w: no %s", media.ErrCorrupt, name(o))
	}
	f := media.Sniff(r.data)
	if f == "" || f != format(r.part.GetFormat()) || !slices.Contains(formats, f) {
		return media.Variant{}, fmt.Errorf("media plugin: %w: the %s is %s, declared %s", media.ErrCorrupt, name(o), describe(f), r.part.GetFormat())
	}
	probe, err := p.builtin.Inspect(ctx, r.data, limits)
	if err != nil {
		return media.Variant{}, rejectOutput(o, err)
	}
	if probe.Width != int(r.part.GetWidth()) || probe.Height != int(r.part.GetHeight()) {
		return media.Variant{}, fmt.Errorf("media plugin: %w: the %s is %d×%d, declared %d×%d", media.ErrCorrupt, name(o), probe.Width, probe.Height, r.part.GetWidth(), r.part.GetHeight())
	}
	if !fits(probe.Width, probe.Height, b) {
		return media.Variant{}, fmt.Errorf("media plugin: %w: the %s, %d×%d, is outside its box", media.ErrCorrupt, name(o), probe.Width, probe.Height)
	}
	return media.Variant{Data: r.data, ContentType: f.ContentType(), Extension: extension(f), Width: probe.Width, Height: probe.Height}, nil
}

// Remux implements media.AVProcessor: the built-in processor inspects data, the plugin remuxes it, and the built-in
// processor reads the output again, which gives the probe of the result.
func (p *Processor) Remux(ctx context.Context, data []byte, limits media.AVLimits) (media.AVOutput, error) {
	limits = limits.WithDefaults()
	in, err := p.builtin.InspectAV(ctx, data, limits)
	if err != nil {
		return media.AVOutput{}, err
	}
	stream, err := p.client.Remux(ctx)
	if err != nil {
		return media.AVOutput{}, failed(err)
	}
	defer func() { _ = stream.Close() }()
	head := &mediav1.RemuxHead{
		Limits: &mediav1.AVLimits{
			MaxBytes: limits.MaxBytes, MaxVideoDurationMs: limits.MaxVideoDuration.Milliseconds(),
			MaxAudioDurationMs: limits.MaxAudioDuration.Milliseconds(), MaxPixels: limits.MaxPixels, MaxFrameRate: limits.MaxFrameRate,
		},
		Size: int64(len(data)),
	}
	err = send(data,
		func() error {
			return stream.Send(&mediav1.RemuxRequest{Message: &mediav1.RemuxRequest_Head{Head: head}})
		},
		func(chunk []byte) error {
			return stream.Send(&mediav1.RemuxRequest{Message: &mediav1.RemuxRequest_Chunk{Chunk: chunk}})
		},
		stream.CloseSend)
	if err != nil {
		return media.AVOutput{}, failed(err)
	}

	first, err := stream.Receive()
	if err != nil {
		return media.AVOutput{}, failed(eof(err))
	}
	if err := outcome(first.GetHead().GetOutcome()); err != nil {
		return media.AVOutput{}, err
	}
	parts, err := receive(func() (message, error) {
		res, err := stream.Receive()
		if err != nil {
			return message{}, err
		}
		switch m := res.GetMessage().(type) {
		case *mediav1.RemuxResponse_Part:
			return message{part: m.Part}, nil
		case *mediav1.RemuxResponse_Chunk:
			return message{chunk: m.Chunk, isChunk: true}, nil
		}
		return message{}, fmt.Errorf("%w: a second head or an empty message", errUnexpected)
	}, limits.MaxBytes+p.coverMax, func(o mediav1.Output) (int64, bool) {
		switch o {
		case mediav1.Output_OUTPUT_AV_BODY:
			return limits.MaxBytes, true
		case mediav1.Output_OUTPUT_AV_COVER:
			return p.coverMax, true
		}
		return 0, false
	})
	if err != nil {
		return media.AVOutput{}, err
	}

	body, ok := parts[mediav1.Output_OUTPUT_AV_BODY]
	if !ok {
		return media.AVOutput{}, fmt.Errorf("media plugin: %w: no %s", media.ErrCorrupt, name(mediav1.Output_OUTPUT_AV_BODY))
	}
	if f := media.SniffAV(body.data); f != in.Format || f != format(body.part.GetFormat()) {
		return media.AVOutput{}, fmt.Errorf("media plugin: %w: the remuxed file is %s, declared %s, from %s", media.ErrCorrupt, describe(f), body.part.GetFormat(), in.Format)
	}
	probe, err := p.builtin.InspectAV(ctx, body.data, limits)
	if err != nil {
		return media.AVOutput{}, rejectOutput(mediav1.Output_OUTPUT_AV_BODY, err)
	}
	if probe.Kind != in.Kind {
		return media.AVOutput{}, fmt.Errorf("media plugin: %w: the remuxed file is %s, from %s", media.ErrCorrupt, probe.Kind, in.Kind)
	}
	out := media.AVOutput{AVProbe: probe, Data: body.data}
	if cover, ok := parts[mediav1.Output_OUTPUT_AV_COVER]; ok {
		// The port leaves the cover unchecked, for the caller to process as an image; only its declaration must hold.
		if f := media.Sniff(cover.data); f != format(cover.part.GetFormat()) {
			return media.AVOutput{}, fmt.Errorf("media plugin: %w: the cover is %s, declared %s", media.ErrCorrupt, describe(f), cover.part.GetFormat())
		}
		out.Cover = cover.data
	}
	return out, nil
}

const maxInt32 = 1<<31 - 1

func box(b media.Box) *mediav1.Box {
	return &mediav1.Box{
		MaxPixels: b.MaxPixels,
		Width:     int32(min(b.Width, maxInt32)), Height: int32(min(b.Height, maxInt32)), //nolint:gosec // bounded by min
		Crop: b.Crop,
	}
}

// fits reports whether w × h is within the box. A box crops or scales, and never enlarges, so an output that fits it is
// never larger than it.
func fits(w, h int, b media.Box) bool {
	switch {
	case b.MaxPixels > 0:
		return int64(w)*int64(h) <= b.MaxPixels
	case b.Width <= 0 || b.Height <= 0:
		return true
	}
	return w <= b.Width && h <= b.Height
}

var errUnexpected = errors.New("an unexpected message")

// send sends the head, data in chunks of ChunkSize, and closes the request side. An error that wraps io.EOF means the
// plugin ended the call: its own error, which the response holds, is what matters, so send stops there with no error.
func send(data []byte, head func() error, chunk func([]byte) error, closeSend func() error) error {
	if err := head(); err != nil {
		return stopped(err)
	}
	for off := 0; off < len(data); off += ChunkSize {
		if err := chunk(data[off:min(off+ChunkSize, len(data))]); err != nil {
			return stopped(err)
		}
	}
	return stopped(closeSend())
}

func stopped(err error) error {
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// eof turns the end of a response with no message into a failure.
func eof(err error) error {
	if errors.Is(err, io.EOF) {
		return errors.New("the plugin answered nothing")
	}
	return err
}

// failed is a failure of the call, which the job retries: the client keeps no text of the plugin's in err.
func failed(err error) error { return fmt.Errorf("media plugin: %w", err) }

// errTemporary is the plugin's own temporary failure.
var errTemporary = errors.New("the plugin failed for now")

// outcome returns the error of a head's outcome, or nil for OUTCOME_OK. A rejection is the port's; anything else, an
// empty head and a value the server does not know included, is a temporary failure (ADR-0146 §10).
func outcome(o mediav1.Outcome) error {
	var err error
	switch o {
	case mediav1.Outcome_OUTCOME_OK:
		return nil
	case mediav1.Outcome_OUTCOME_UNSUPPORTED_FORMAT:
		err = media.ErrUnsupportedFormat
	case mediav1.Outcome_OUTCOME_UNSUPPORTED_CODEC:
		err = media.ErrUnsupportedCodec
	case mediav1.Outcome_OUTCOME_TOO_LARGE:
		err = media.ErrTooLarge
	case mediav1.Outcome_OUTCOME_DIMENSIONS:
		err = media.ErrDimensions
	case mediav1.Outcome_OUTCOME_CORRUPT:
		err = media.ErrCorrupt
	case mediav1.Outcome_OUTCOME_TOO_LONG:
		err = media.ErrTooLong
	case mediav1.Outcome_OUTCOME_TEMPORARY:
		err = errTemporary
	default:
		return fmt.Errorf("media plugin: the outcome is %d, which is not a result", int32(o))
	}
	return fmt.Errorf("media plugin: %w", err)
}

// rejectOutput returns the error of an output that the built-in processor did not accept: too large is media.ErrTooLarge,
// any other rejection is media.ErrCorrupt, and a failure such as an ended context stays what it is.
func rejectOutput(o mediav1.Output, err error) error {
	switch {
	case errors.Is(err, media.ErrTooLarge):
		return fmt.Errorf("media plugin: the %s: %w", name(o), err)
	case media.Rejected(err):
		return fmt.Errorf("media plugin: %w: the %s: %w", media.ErrCorrupt, name(o), err)
	}
	return err
}

// message is one message of a response after its head.
type message struct {
	part    *mediav1.Part
	chunk   []byte
	isChunk bool
}

// received is one output and what its part declared.
type received struct {
	part *mediav1.Part
	data []byte
}

// receive reads the outputs to the end of the response. Each output comes once, with a part that declares its size, and
// then chunks that add up to it. It stops reading at the first output larger than its part, its own limit, or what is
// left of budget, with media.ErrTooLarge, and allocates each output only after its size passed those limits.
func receive(next func() (message, error), budget int64, limit func(mediav1.Output) (int64, bool)) (map[mediav1.Output]received, error) {
	parts := map[mediav1.Output]received{}
	var current *received
	complete := func() error {
		if current != nil && int64(len(current.data)) != current.part.GetSize() {
			return fmt.Errorf("media plugin: %w: the %s has %d bytes, declared %d", media.ErrCorrupt, name(current.part.GetOutput()), len(current.data), current.part.GetSize())
		}
		if current != nil {
			parts[current.part.GetOutput()] = *current
		}
		current = nil
		return nil
	}
	for {
		m, err := next()
		switch {
		case errors.Is(err, io.EOF):
			if err := complete(); err != nil {
				return nil, err
			}
			return parts, nil
		case errors.Is(err, errUnexpected):
			return nil, fmt.Errorf("media plugin: %w: %w", media.ErrCorrupt, err)
		case err != nil:
			return nil, failed(err)
		}
		if m.isChunk {
			if current == nil {
				return nil, fmt.Errorf("media plugin: %w: a chunk before any part", media.ErrCorrupt)
			}
			if int64(len(current.data))+int64(len(m.chunk)) > current.part.GetSize() {
				return nil, fmt.Errorf("media plugin: %w: the %s is longer than its declared %d bytes", media.ErrTooLarge, name(current.part.GetOutput()), current.part.GetSize())
			}
			current.data = append(current.data, m.chunk...)
			continue
		}
		if err := complete(); err != nil {
			return nil, err
		}
		o, size := m.part.GetOutput(), m.part.GetSize()
		most, ok := limit(o)
		if _, seen := parts[o]; !ok || seen {
			return nil, fmt.Errorf("media plugin: %w: an output %s that this call does not make, or a second one", media.ErrCorrupt, o)
		}
		if size <= 0 {
			return nil, fmt.Errorf("media plugin: %w: the %s is empty", media.ErrCorrupt, name(o))
		}
		if size > most || size > budget {
			return nil, fmt.Errorf("media plugin: %w: the %s is %d bytes, over its limit of %d", media.ErrTooLarge, name(o), size, min(most, budget))
		}
		budget -= size
		current = &received{part: m.part, data: make([]byte, 0, size)}
	}
}

var formats = map[mediav1.Format]media.Format{
	mediav1.Format_FORMAT_JPEG: media.FormatJPEG,
	mediav1.Format_FORMAT_PNG:  media.FormatPNG,
	mediav1.Format_FORMAT_GIF:  media.FormatGIF,
	mediav1.Format_FORMAT_WEBP: media.FormatWebP,
	mediav1.Format_FORMAT_MP4:  media.FormatMP4,
	mediav1.Format_FORMAT_WEBM: media.FormatWebM,
	mediav1.Format_FORMAT_MP3:  media.FormatMP3,
	mediav1.Format_FORMAT_OGG:  media.FormatOgg,
}

// format returns the format of an enum value; "" for none the server knows.
func format(f mediav1.Format) media.Format { return formats[f] }

func describe(f media.Format) string {
	if f == "" {
		return "no known format"
	}
	return string(f)
}

// extension returns the file name extension of an image format, as the built-in processor writes it.
func extension(f media.Format) string {
	if f == media.FormatJPEG {
		return "jpg"
	}
	return string(f)
}

// name returns the words of an output for an error.
func name(o mediav1.Output) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(o.String(), "OUTPUT_")), "_", " ") + " output"
}

const blurhashChars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz#$%*+,-.:;=?@[]^_{|}~"

// validBlurhash reports whether h is a blurhash: base 83, with the length that its first character's components give.
func validBlurhash(h string) bool {
	if len(h) < 6 {
		return false
	}
	for _, c := range h {
		if !strings.ContainsRune(blurhashChars, c) {
			return false
		}
	}
	size := strings.IndexByte(blurhashChars, h[0])
	nx, ny := size%9+1, size/9+1
	return len(h) == 4+2*nx*ny
}
