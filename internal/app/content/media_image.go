// SPDX-License-Identifier: AGPL-3.0-only

package content

// ImageVariant is a rendition of an image as clients load it. ContentType, Width, Height, and
// Size are zero when unknown, as for the media proxy URL of a remote file.
type ImageVariant struct {
	URL           string
	ContentType   string
	Width, Height int
	Size          int64
}

// ImageVariants are the renditions of an image in one motion, by size (ADR-0005 §4).
type ImageVariants struct {
	Small, Medium, Large ImageVariant
}

// Image is a media file as clients load it: its still renditions, and for an animation also
// its moving ones (ADR-0005 §6, §8). For a video or audio file, Media is the file itself and
// the still renditions are those of its poster, all empty when it has none (§6).
type Image struct {
	Still ImageVariants
	// Animated are the moving renditions, nil for a still image.
	Animated *ImageVariants
	// Media is the video or audio file, nil for an image.
	Media *ImageVariant
}

// HasStill reports whether the image has still renditions: always for an image, and for a video
// or audio file when it has a poster.
func (i Image) HasStill() bool { return i.Still.Large.URL != "" }

// Primary returns the rendition served as the file itself: the video or audio file, or the
// large image, moving for an animation and still otherwise.
func (i Image) Primary() ImageVariant {
	if i.Media != nil {
		return *i.Media
	}
	if i.Animated != nil {
		return i.Animated.Large
	}
	return i.Still.Large
}
