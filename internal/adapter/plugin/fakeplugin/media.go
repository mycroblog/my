// SPDX-License-Identifier: AGPL-3.0-only

package fakeplugin

import (
	"context"
	"errors"
	"io"
	"time"

	"connectrpc.com/connect/v2"

	mediav1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/media/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/media/v1/mediav1connect"
	"github.com/mycroblog/my/internal/app/media"
)

// mediaChunk is the size of the chunks the fake sends, as a plugin keeps to the protocol's 1 MiB.
const mediaChunk = 1 << 20

// maxMediaInput bounds what the fake reads of one call, whatever the head declares.
const maxMediaInput = 256 << 20

type mediaServer struct {
	mediav1connect.UnimplementedMediaServiceHandler

	o Options
}

// Process implements mediav1connect.MediaServiceHandler with the processor of Options.Media: its output goes out as it
// is, with the format and dimensions it reports, right or wrong, so that a test can make the fake break the contract.
func (s *mediaServer) Process(ctx context.Context, stream mediav1connect.MediaServiceProcessServerStream) error {
	if s.o.Media == nil {
		return connect.NewError(connect.CodeUnimplemented, "this fake has no media processor")
	}
	first, err := stream.Receive()
	if err != nil {
		return err
	}
	head := first.GetHead()
	if head == nil {
		return connect.NewError(connect.CodeInvalidArgument, "the first message is not the head")
	}
	data, err := readBody(head.GetSize(), func() ([]byte, error) {
		req, err := stream.Receive()
		if err != nil {
			return nil, err
		}
		chunk, ok := req.GetMessage().(*mediav1.ProcessRequest_Chunk)
		if !ok {
			return nil, connect.NewError(connect.CodeInvalidArgument, "a second head")
		}
		return chunk.Chunk, nil
	})
	if err != nil {
		return err
	}
	if done, err := s.misbehave(ctx, func() error { return stream.Send(&mediav1.ProcessResponse{}) }); done {
		return err
	}
	l, purpose := head.GetLimits(), mediaPurposes[head.GetPurpose()]
	boxes := media.Boxes{Small: mediaBox(head.GetSmall()), Medium: mediaBox(head.GetMedium()), Large: mediaBox(head.GetLarge())}
	var sizes media.Sizes
	switch purpose {
	case media.PurposeAvatar:
		sizes.Avatar = boxes
	case media.PurposeHeader:
		sizes.Header = boxes
	case media.PurposeEmoji:
		sizes.Emoji = boxes
	default:
		sizes.Attachment = boxes
	}
	out, err := s.o.Media.Process(ctx, data,
		media.Limits{MaxBytes: l.GetMaxBytes(), MaxSide: int(l.GetMaxSide()), MaxPixels: l.GetMaxPixels(), MaxFrames: int(l.GetMaxFrames())},
		sizes, media.Request{Purpose: purpose, FocusX: head.GetFocusX(), FocusY: head.GetFocusY()})
	if err != nil {
		return stream.Send(&mediav1.ProcessResponse{Message: &mediav1.ProcessResponse_Head{Head: &mediav1.ProcessResult{Outcome: mediaOutcome(err)}}})
	}
	if err := stream.Send(&mediav1.ProcessResponse{Message: &mediav1.ProcessResponse_Head{Head: &mediav1.ProcessResult{
		Outcome: mediav1.Outcome_OUTCOME_OK, Width: int32(out.Width), Height: int32(out.Height), Blurhash: out.Blurhash, //nolint:gosec // test sizes
	}}}); err != nil {
		return err
	}
	outputs := []struct {
		o mediav1.Output
		v media.Variant
	}{
		{mediav1.Output_OUTPUT_STILL_SMALL, out.Still.Small},
		{mediav1.Output_OUTPUT_STILL_MEDIUM, out.Still.Medium},
		{mediav1.Output_OUTPUT_STILL_LARGE, out.Still.Large},
	}
	if out.Animated != nil {
		outputs = append(outputs, []struct {
			o mediav1.Output
			v media.Variant
		}{
			{mediav1.Output_OUTPUT_ANIMATED_SMALL, out.Animated.Small},
			{mediav1.Output_OUTPUT_ANIMATED_MEDIUM, out.Animated.Medium},
			{mediav1.Output_OUTPUT_ANIMATED_LARGE, out.Animated.Large},
		}...)
	}
	for _, v := range outputs {
		part := &mediav1.Part{
			Output: v.o, Format: mediaFormatOfType(v.v.ContentType),
			Width: int32(v.v.Width), Height: int32(v.v.Height), //nolint:gosec // test sizes
			Size: s.declared(len(v.v.Data)),
		}
		if err := sendOutput(part, v.v.Data,
			func(p *mediav1.Part) error {
				return stream.Send(&mediav1.ProcessResponse{Message: &mediav1.ProcessResponse_Part{Part: p}})
			},
			func(c []byte) error {
				return stream.Send(&mediav1.ProcessResponse{Message: &mediav1.ProcessResponse_Chunk{Chunk: c}})
			}); err != nil {
			return err
		}
	}
	return nil
}

// Remux implements mediav1connect.MediaServiceHandler with the processor of Options.Media, whose output goes out as it is.
func (s *mediaServer) Remux(ctx context.Context, stream mediav1connect.MediaServiceRemuxServerStream) error {
	if s.o.Media == nil {
		return connect.NewError(connect.CodeUnimplemented, "this fake has no media processor")
	}
	first, err := stream.Receive()
	if err != nil {
		return err
	}
	head := first.GetHead()
	if head == nil {
		return connect.NewError(connect.CodeInvalidArgument, "the first message is not the head")
	}
	data, err := readBody(head.GetSize(), func() ([]byte, error) {
		req, err := stream.Receive()
		if err != nil {
			return nil, err
		}
		chunk, ok := req.GetMessage().(*mediav1.RemuxRequest_Chunk)
		if !ok {
			return nil, connect.NewError(connect.CodeInvalidArgument, "a second head")
		}
		return chunk.Chunk, nil
	})
	if err != nil {
		return err
	}
	if done, err := s.misbehave(ctx, func() error { return stream.Send(&mediav1.RemuxResponse{}) }); done {
		return err
	}
	l := head.GetLimits()
	limits := media.AVLimits{
		MaxBytes: l.GetMaxBytes(), MaxVideoDuration: time.Duration(l.GetMaxVideoDurationMs()) * time.Millisecond,
		MaxAudioDuration: time.Duration(l.GetMaxAudioDurationMs()) * time.Millisecond, MaxPixels: l.GetMaxPixels(), MaxFrameRate: l.GetMaxFrameRate(),
	}
	// The head says whether the file is a video or audio only, as the processor reads it.
	if probe, err := s.o.Media.InspectAV(ctx, data, limits); err == nil && head.GetKind() != avKinds[probe.Kind] {
		return connect.NewError(connect.CodeInvalidArgument, "the kind of the head is "+head.GetKind().String()+", and the file is "+string(probe.Kind))
	}
	out, err := s.o.Media.Remux(ctx, data, limits)
	if err != nil {
		return stream.Send(&mediav1.RemuxResponse{Message: &mediav1.RemuxResponse_Head{Head: &mediav1.RemuxResult{Outcome: mediaOutcome(err)}}})
	}
	if err := stream.Send(&mediav1.RemuxResponse{Message: &mediav1.RemuxResponse_Head{Head: &mediav1.RemuxResult{Outcome: mediav1.Outcome_OUTCOME_OK}}}); err != nil {
		return err
	}
	sendPart := func(p *mediav1.Part) error {
		return stream.Send(&mediav1.RemuxResponse{Message: &mediav1.RemuxResponse_Part{Part: p}})
	}
	sendChunk := func(c []byte) error {
		return stream.Send(&mediav1.RemuxResponse{Message: &mediav1.RemuxResponse_Chunk{Chunk: c}})
	}
	body := &mediav1.Part{Output: mediav1.Output_OUTPUT_AV_BODY, Format: mediaFormats[out.Format], Size: s.declared(len(out.Data))}
	if err := sendOutput(body, out.Data, sendPart, sendChunk); err != nil {
		return err
	}
	if out.Cover == nil {
		return nil
	}
	cover := &mediav1.Part{Output: mediav1.Output_OUTPUT_AV_COVER, Format: mediaFormats[media.Sniff(out.Cover)], Size: s.declared(len(out.Cover))}
	return sendOutput(cover, out.Cover, sendPart, sendChunk)
}

// misbehave holds or breaks a call as Options say, and reports whether it did.
func (s *mediaServer) misbehave(ctx context.Context, sendEmpty func() error) (bool, error) {
	switch {
	case s.o.Block:
		<-ctx.Done()
		return true, ctx.Err()
	case s.o.Broken:
		return true, sendEmpty()
	}
	return false, nil
}

// declared is the size a part declares: the real one, or half of it with Options.MediaUnderstate.
func (s *mediaServer) declared(n int) int64 {
	if s.o.MediaUnderstate {
		return int64(n / 2)
	}
	return int64(n)
}

// readBody reads the chunks of a request to its end.
func readBody(size int64, next func() ([]byte, error)) ([]byte, error) {
	if size < 0 || size > maxMediaInput {
		return nil, connect.NewError(connect.CodeInvalidArgument, "the size is out of range")
	}
	data := make([]byte, 0, size)
	for {
		chunk, err := next()
		if errors.Is(err, io.EOF) {
			return data, nil
		}
		if err != nil {
			return nil, err
		}
		if int64(len(data)+len(chunk)) > size {
			return nil, connect.NewError(connect.CodeInvalidArgument, "the body is longer than its size")
		}
		data = append(data, chunk...)
	}
}

// sendOutput sends a part and data in chunks.
func sendOutput(part *mediav1.Part, data []byte, sendPart func(*mediav1.Part) error, sendChunk func([]byte) error) error {
	if err := sendPart(part); err != nil {
		return err
	}
	for off := 0; off < len(data); off += mediaChunk {
		if err := sendChunk(data[off:min(off+mediaChunk, len(data))]); err != nil {
			return err
		}
	}
	return nil
}

var avKinds = map[media.AVKind]mediav1.AVKind{
	media.AVVideo: mediav1.AVKind_AV_KIND_VIDEO,
	media.AVAudio: mediav1.AVKind_AV_KIND_AUDIO,
}

var mediaPurposes = map[mediav1.Purpose]media.Purpose{
	mediav1.Purpose_PURPOSE_ATTACHMENT: media.PurposeAttachment,
	mediav1.Purpose_PURPOSE_AVATAR:     media.PurposeAvatar,
	mediav1.Purpose_PURPOSE_HEADER:     media.PurposeHeader,
	mediav1.Purpose_PURPOSE_EMOJI:      media.PurposeEmoji,
}

var mediaFormats = map[media.Format]mediav1.Format{
	media.FormatJPEG: mediav1.Format_FORMAT_JPEG,
	media.FormatPNG:  mediav1.Format_FORMAT_PNG,
	media.FormatGIF:  mediav1.Format_FORMAT_GIF,
	media.FormatWebP: mediav1.Format_FORMAT_WEBP,
	media.FormatMP4:  mediav1.Format_FORMAT_MP4,
	media.FormatWebM: mediav1.Format_FORMAT_WEBM,
	media.FormatMP3:  mediav1.Format_FORMAT_MP3,
	media.FormatOgg:  mediav1.Format_FORMAT_OGG,
}

// mediaFormatOfType returns the format a variant's content type reports, which may not be what its data is.
func mediaFormatOfType(contentType string) mediav1.Format {
	for _, f := range []media.Format{media.FormatJPEG, media.FormatPNG, media.FormatGIF, media.FormatWebP} {
		if f.ContentType() == contentType {
			return mediaFormats[f]
		}
	}
	return mediav1.Format_FORMAT_UNSPECIFIED
}

func mediaBox(b *mediav1.Box) media.Box {
	return media.Box{MaxPixels: b.GetMaxPixels(), Width: int(b.GetWidth()), Height: int(b.GetHeight()), Crop: b.GetCrop()}
}

// mediaOutcome returns the outcome of a processor's error.
func mediaOutcome(err error) mediav1.Outcome {
	switch {
	case errors.Is(err, media.ErrUnsupportedCodec):
		return mediav1.Outcome_OUTCOME_UNSUPPORTED_CODEC
	case errors.Is(err, media.ErrUnsupportedFormat):
		return mediav1.Outcome_OUTCOME_UNSUPPORTED_FORMAT
	case errors.Is(err, media.ErrTooLarge):
		return mediav1.Outcome_OUTCOME_TOO_LARGE
	case errors.Is(err, media.ErrDimensions):
		return mediav1.Outcome_OUTCOME_DIMENSIONS
	case errors.Is(err, media.ErrCorrupt):
		return mediav1.Outcome_OUTCOME_CORRUPT
	case errors.Is(err, media.ErrTooLong):
		return mediav1.Outcome_OUTCOME_TOO_LONG
	}
	return mediav1.Outcome_OUTCOME_TEMPORARY
}
