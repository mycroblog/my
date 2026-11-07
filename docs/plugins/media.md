<!-- Code generated from api/plugin by "scripts/task.sh api:plugin-generate". DO NOT EDIT. -->

# Media protocol

The services, messages, and enums of the package `my.plugin.media.v1`, which [`media.proto`](../../api/plugin/my/plugin/media/v1/media.proto) defines.

Converting media files on another machine: re-encoding images into their variants and remuxing video and audio files,
both without metadata. The server keeps detecting formats and checking uploads itself, and sends only the files that
pass those checks.

Both RPCs stream both ways, which needs HTTP/2: the server speaks the Connect protocol with binary protobuf or gRPC,
never JSON, over TLS with ALPN h2, or h2c on a Unix socket or plain TCP. The calls are half-duplex: the server sends a
head and the body in chunks, then closes its side; the plugin then answers with its head, and with a part and its
chunks for each output.

A chunk holds 1 MiB at most, and a message 2 MiB. The server does not trust the output: it detects the format of each
one from its content, checks it with its own decoders and the same limits as an upload, and checks that it fits its
box. An output that breaks one of these is OUTCOME_CORRUPT, and one larger than its part or the limits below is
OUTCOME_TOO_LARGE, as if the plugin had answered them. The uploads may hold a location in their metadata, and removing
it is the plugin's job: the server cannot check that all of it is gone.

## Services

### `MediaService`

#### `Process`

```proto
rpc Process(stream ProcessRequest) returns (stream ProcessResponse);
```

Request: [`ProcessRequest`](#processrequest). Response: [`ProcessResponse`](#processresponse).

Process re-encodes an image into its variants, as the server's own processor does. The same input and head give the
same output, and the same call may come again after a failure. It lasts plugin_media_image_timeout at most.

The server sends ProcessRequest.head first, then the image in chunks, then closes its side. The plugin answers
ProcessResponse.head; with OUTCOME_OK, it then sends one part per output, each followed by its chunks, in any
order: the three still outputs always, and the three animated ones only for an image of more than one frame.
The server reads 2 × limits.max_bytes in all at most.

#### `Remux`

```proto
rpc Remux(stream RemuxRequest) returns (stream RemuxResponse);
```

Request: [`RemuxRequest`](#remuxrequest). Response: [`RemuxResponse`](#remuxresponse).

Remux writes the streams of a video or audio file again, in the same container, without metadata: tags, titles,
dates, locations, embedded pictures, and tracks other than the video and audio. It never transcodes. The same input
gives the same output. It lasts plugin_media_video_timeout or plugin_media_audio_timeout at most, as head.kind says.

The server sends RemuxRequest.head first, then the file in chunks, then closes its side. The plugin answers
RemuxResponse.head; with OUTCOME_OK, it then sends the part OUTPUT_AV_BODY and its chunks, and the part
OUTPUT_AV_COVER and its chunks when the file embeds a cover picture (ID3 APIC, MP4 covr). The server reads the body
up to limits.max_bytes, and the cover up to its own limit of an image.

## Messages

### `ProcessRequest`

- `head` ([`ProcessHead`](#processhead), field 1, one of `message`): The first message, and only the first.
- `chunk` (`bytes`, field 2, one of `message`): A piece of the image, 1 MiB at most, in order after the head.

### `ProcessHead`

- `limits` ([`Limits`](#limits), field 1): The limits that the image passed, which the outputs must pass too.
- `purpose` ([`Purpose`](#purpose), field 2): What the image is for. PURPOSE_EMOJI takes a PNG as its large still output; the others take a JPEG when the image is
  opaque and a PNG otherwise.
- `small` ([`Box`](#box), field 3): The boxes of the purpose's outputs.
- `medium` ([`Box`](#box), field 4)
- `large` ([`Box`](#box), field 5)
- `focus_x` (`double`, field 6): The focal point that a crop keeps in view, from -1.0 (left, bottom) to 1.0 (right, top); (0, 0) is the center.
- `focus_y` (`double`, field 7)
- `size` (`int64`, field 8): The length of the image, which the chunks add up to.

### `Limits`

The limits of an image, as the server's settings set them.

- `max_bytes` (`int64`, field 1): The longest file, in bytes.
- `max_side` (`int32`, field 2): The largest width and height, in pixels.
- `max_pixels` (`int64`, field 3): The largest width × height. For an animation it bounds the pixels of all frames together too, and four times it
  bounds the frames × the canvas.
- `max_frames` (`int32`, field 4): The most frames of an animation.

### `Box`

The bounds of one output. With max_pixels above 0, the output keeps the aspect ratio of the image within that area;
otherwise it keeps the aspect ratio within width × height, or, with crop, is first cropped to the aspect ratio of width
× height around the focal point. An image that already fits is not enlarged. All zero bounds nothing.

- `max_pixels` (`int64`, field 1)
- `width` (`int32`, field 2)
- `height` (`int32`, field 3)
- `crop` (`bool`, field 4)

### `ProcessResponse`

- `head` ([`ProcessResult`](#processresult), field 1, one of `message`): The first message, and only the first.
- `part` ([`Part`](#part), field 2, one of `message`): The start of an output, which its chunks follow.
- `chunk` (`bytes`, field 3, one of `message`): A piece of the output of the last part, 1 MiB at most.

### `ProcessResult`

- `outcome` ([`Outcome`](#outcome), field 1): Anything but a value below, an empty head included, is a temporary failure. With anything but OUTCOME_OK, the server
  reads no further.
- `width` (`int32`, field 2): The dimensions of the image after its EXIF orientation.
- `height` (`int32`, field 3)
- `blurhash` (`string`, field 4): The blurhash (<https://blurha.sh>) of the small still output.

### `RemuxRequest`

- `head` ([`RemuxHead`](#remuxhead), field 1, one of `message`): The first message, and only the first.
- `chunk` (`bytes`, field 2, one of `message`): A piece of the file, 1 MiB at most, in order after the head.

### `RemuxHead`

- `limits` ([`AVLimits`](#avlimits), field 1): The limits that the file passed, which the output must pass too.
- `size` (`int64`, field 2): The length of the file, which the chunks add up to.
- `kind` ([`AVKind`](#avkind), field 3): Whether the file is a video or an audio-only file, as the server read it. The server sends a video to
  plugin_media_video_address and an audio file to plugin_media_audio_address, which may be the same plugin.

### `AVLimits`

The limits of a video or audio file, as the server's settings set them.

- `max_bytes` (`int64`, field 1): The longest file, in bytes.
- `max_video_duration_ms` (`int64`, field 2): The longest video and audio file, in milliseconds.
- `max_audio_duration_ms` (`int64`, field 3)
- `max_pixels` (`int64`, field 4): The largest width × height of a video.
- `max_frame_rate` (`double`, field 5): The most frames per second of a video.

### `RemuxResponse`

- `head` ([`RemuxResult`](#remuxresult), field 1, one of `message`): The first message, and only the first.
- `part` ([`Part`](#part), field 2, one of `message`): The start of an output, which its chunks follow.
- `chunk` (`bytes`, field 3, one of `message`): A piece of the output of the last part, 1 MiB at most.

### `RemuxResult`

- `outcome` ([`Outcome`](#outcome), field 1): Anything but a value below, an empty head included, is a temporary failure. With anything but OUTCOME_OK, the server
  reads no further. The server reads the kind, dimensions, duration, and codecs of the output itself.

### `Part`

What one output is, before its chunks.

- `output` ([`Output`](#output), field 1)
- `format` ([`Format`](#format), field 2): The format of the output, which the server checks against its content.
- `width` (`int32`, field 3): The dimensions of an image output; zero for OUTPUT_AV_BODY.
- `height` (`int32`, field 4)
- `size` (`int64`, field 5): The length of the output, which its chunks add up to.

## Enums

### `Purpose`

What an image is for. The boxes of the head already follow from it.

- `PURPOSE_UNSPECIFIED` = 0
- `PURPOSE_ATTACHMENT` = 1
- `PURPOSE_AVATAR` = 2
- `PURPOSE_HEADER` = 3
- `PURPOSE_EMOJI` = 4

### `AVKind`

What a video or audio file holds.

- `AV_KIND_UNSPECIFIED` = 0
- `AV_KIND_VIDEO` = 1: A file with a video track, with or without sound.
- `AV_KIND_AUDIO` = 2: A file with sound only.

### `Output`

The outputs. Each one comes once at most.

- `OUTPUT_UNSPECIFIED` = 0
- `OUTPUT_STILL_SMALL` = 1: The still outputs of Process: a WebP for small and medium; a JPEG or a PNG for large (see Purpose). Of an animation,
  they show its first frame.
- `OUTPUT_STILL_MEDIUM` = 2
- `OUTPUT_STILL_LARGE` = 3
- `OUTPUT_ANIMATED_SMALL` = 4: The moving outputs of Process of an animation: an animated WebP for small and medium, a GIF for large.
- `OUTPUT_ANIMATED_MEDIUM` = 5
- `OUTPUT_ANIMATED_LARGE` = 6
- `OUTPUT_AV_BODY` = 7: The file that Remux writes, in the container of its input.
- `OUTPUT_AV_COVER` = 8: The cover picture embedded in the input of Remux, which the server checks as an image later.

### `Format`

- `FORMAT_UNSPECIFIED` = 0
- `FORMAT_JPEG` = 1
- `FORMAT_PNG` = 2
- `FORMAT_GIF` = 3
- `FORMAT_WEBP` = 4
- `FORMAT_MP4` = 5: MP4, QuickTime included.
- `FORMAT_WEBM` = 6
- `FORMAT_MP3` = 7
- `FORMAT_OGG` = 8

### `Outcome`

The result of a call. Connect errors say only that the call failed in transport, which the server retries like
OUTCOME_TEMPORARY. The other outcomes reject the file: the server marks it as failed, as when its own processor rejects
one, and does not retry.

- `OUTCOME_UNSPECIFIED` = 0
- `OUTCOME_OK` = 1: The outputs follow.
- `OUTCOME_TEMPORARY` = 2: The plugin failed. Asking again may work.
- `OUTCOME_UNSUPPORTED_FORMAT` = 3: The file is in none of the accepted formats.
- `OUTCOME_UNSUPPORTED_CODEC` = 4: The container of a video or audio file is accepted, but a codec is not: H.264, VP8, VP9, and AV1 for video, and AAC,
  Opus, Vorbis, and MP3 for audio are.
- `OUTCOME_TOO_LARGE` = 5: The file is longer than limits.max_bytes.
- `OUTCOME_DIMENSIONS` = 6: The dimensions, pixels, frames, or frame rate exceed the limits.
- `OUTCOME_CORRUPT` = 7: The file does not decode.
- `OUTCOME_TOO_LONG` = 8: A video or audio file is longer than its limit of duration.
