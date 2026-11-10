// SPDX-License-Identifier: AGPL-3.0-only

package htmlsanitizer

import (
	"strings"

	appcontent "github.com/mycroblog/my/internal/app/content"
)

// The body of a received Article goes through the allowlist of articles too (ADR-0160 §10): its
// images may be of any host, so the content module records them as remote media files and the
// client API shows them through the media proxy.

var _ appcontent.RemoteArticleSanitizer = (*Sanitizer)(nil)

// SanitizeRemoteArticle implements appcontent.RemoteArticleSanitizer.
func (*Sanitizer) SanitizeRemoteArticle(in string, maxImages int) appcontent.SanitizedArticle {
	var images []appcontent.RemoteArticleImage
	out := sanitizeArticle(in, func(src, alt string) (string, bool) {
		u, ok := imageURL(src)
		if !ok {
			return "", false
		}
		for _, img := range images {
			if img.URL == u {
				return u, true
			}
		}
		if len(images) >= maxImages {
			return "", false
		}
		images = append(images, appcontent.RemoteArticleImage{URL: u, Alt: strings.TrimSpace(alt)})
		return u, true
	})
	return appcontent.SanitizedArticle{HTML: strings.TrimSpace(out.html), Images: images, Text: out.text}
}

// ShowArticleImages implements appcontent.RemoteArticleSanitizer.
func (*Sanitizer) ShowArticleImages(in string, src map[string]string) string {
	return strings.TrimSpace(sanitizeArticle(in, func(s, _ string) (string, bool) {
		u, ok := imageURL(s)
		if !ok {
			return "", false
		}
		shown, ok := src[u]
		return shown, ok
	}).html)
}
