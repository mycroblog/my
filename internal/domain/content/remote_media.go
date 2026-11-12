// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"bytes"
	"slices"
	"time"
)

// RemoteMedia is the remote subtype of a media file: a file on another server as the received
// document declared it (remote_media). The file itself is fetched only by the media proxy
// (ADR-0005 §10). Width, Height, and Blurhash are zero when undeclared;
// Focus is nil when undeclared.
type RemoteMedia struct {
	MediaID        string
	OwnerAccountID string
	RemoteURL      string
	// DeclaredContentType is the AS2 mediaType, "" when undeclared. It is never trusted: the
	// media proxy detects the type from the content.
	DeclaredContentType string
	DeclaredMediaType   MediaType
	// Purpose is what the file was recorded for: an attachment, an avatar, a header, or a
	// custom emoji. It sizes the media proxy's small variant.
	Purpose       MediaPurpose
	Width, Height int
	Blurhash      string
	Focus         *MediaFocus
	Description   string
}

// RemoteMediaCacheEntry is a remote file that the media proxy copied into blob storage
// (remote_media_cache): the cache of the remote_media_policy avatars and all (FR-409). It is
// keyed by the remote URL, so that the media files of several owners with the same URL share
// one copy. The proxy serves it until ExpiresAt; the cleanup deletes it after ExpiresAt, or
// earlier, least recently accessed first, when the cache is over its size limit.
type RemoteMediaCacheEntry struct {
	URL string
	// BlobKey is the private key of the copy, under "remote/".
	BlobKey string
	// ContentType is the media type detected from the content when it was fetched.
	ContentType string
	FileSize    int64
	FetchedAt   time.Time
	ExpiresAt   time.Time
	// LastAccessedAt is when the proxy last served the copy, updated at a coarse granularity.
	LastAccessedAt time.Time
}

// SniffHeadLength is how many leading bytes SniffRemoteMedia needs to tell every format apart.
const SniffHeadLength = 64

// SniffRemoteMedia detects the format of a remote media file from its leading bytes (at least
// SniffHeadLength of them when the file is that long), never from its name or declared type
// (ADR-0019 §8). It returns the media type to serve the file with and its kind (image, video,
// or audio), or "" and MediaTypeUnknown for a format outside the allow list: JPEG, PNG, GIF,
// WebP, AVIF, MP4, WebM, MP3, and Ogg. SVG, HTML, and anything else a browser could run are
// outside it.
func SniffRemoteMedia(head []byte) (contentType string, kind MediaType) {
	switch {
	case bytes.HasPrefix(head, []byte("\xFF\xD8\xFF")):
		return "image/jpeg", MediaTypeImage
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1A\n")):
		return "image/png", MediaTypeImage
	case bytes.HasPrefix(head, []byte("GIF87a")), bytes.HasPrefix(head, []byte("GIF89a")):
		return "image/gif", MediaTypeImage
	case len(head) >= 12 && bytes.HasPrefix(head, []byte("RIFF")) && string(head[8:12]) == "WEBP":
		return "image/webp", MediaTypeImage
	case len(head) >= 12 && string(head[4:8]) == "ftyp":
		return sniffISOBMFF(head)
	case bytes.HasPrefix(head, []byte("\x1A\x45\xDF\xA3")):
		// EBML: Matroska and WebM share the container; only the WebM document type is allowed.
		if bytes.Contains(head[:min(len(head), SniffHeadLength)], []byte("webm")) {
			return "video/webm", MediaTypeVideo
		}
	case bytes.HasPrefix(head, []byte("OggS")):
		return "audio/ogg", MediaTypeAudio
	case bytes.HasPrefix(head, []byte("ID3")):
		return "audio/mpeg", MediaTypeAudio
	case len(head) >= 2 && head[0] == 0xFF && head[1]&0xE0 == 0xE0 && head[1]&0x06 == 0x02:
		// An MPEG audio frame header of Layer III without an ID3 tag.
		return "audio/mpeg", MediaTypeAudio
	}
	return "", MediaTypeUnknown
}

// mp4Brands are the ISO base media file brands served as MP4 video; M4A is MP4 audio.
var mp4Brands = []string{
	"isom", "iso2", "iso3", "iso4", "iso5", "iso6", "mp41", "mp42", "avc1", "dash", "M4V ", "mmp4", "MSNV",
}

// sniffISOBMFF tells AVIF, MP4 video, and MP4 audio apart by the brands of the ftyp box. Other
// brands (QuickTime, HEIF) are outside the allow list.
func sniffISOBMFF(head []byte) (string, MediaType) {
	size := int(head[0])<<24 | int(head[1])<<16 | int(head[2])<<8 | int(head[3])
	end := min(len(head), max(size, 16))
	major := string(head[8:12])
	var compatible []string
	for i := 16; i+4 <= end; i += 4 {
		compatible = append(compatible, string(head[i:i+4]))
	}
	switch {
	case major == "avif" || major == "avis",
		(major == "mif1" || major == "msf1") && slices.Contains(compatible, "avif"):
		return "image/avif", MediaTypeImage
	case major == "M4A ":
		return "audio/mp4", MediaTypeAudio
	case slices.Contains(mp4Brands, major):
		return "video/mp4", MediaTypeVideo
	}
	return "", MediaTypeUnknown
}
