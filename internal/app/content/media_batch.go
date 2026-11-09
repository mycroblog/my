// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"fmt"
	"slices"

	"github.com/mycroblog/my/internal/domain/content"
)

// page returns a MediaService for the attachments of a page: it reads the parts of the files of
// ids together (MediaRepository.FindMediaParts), and Media builds each of them from those parts as
// it builds one from its reads.
func (s *MediaService) page(ctx context.Context, ids []string) (*MediaService, error) {
	parts, err := s.d.Repository.FindMediaParts(ctx, ids)
	if err != nil {
		return nil, err
	}
	p := *s
	p.d.Repository = &pageMediaRepository{MediaRepository: s.d.Repository, ids: ids, parts: parts}
	return &p, nil
}

// pageMediaRepository is the media repository with the parts of a page's files read beforehand. A
// part that a file of the page does not have fails with ErrNotFound, as its read does.
type pageMediaRepository struct {
	MediaRepository
	ids   []string
	parts MediaParts
}

// mediaPart returns the part of a file of the page from m; ok is false for a file outside the page.
func mediaPart[T any](p *pageMediaRepository, m map[string]T, mediaID, what string) (part T, ok bool, err error) {
	if !slices.Contains(p.ids, mediaID) {
		return part, false, nil
	}
	if v, found := m[mediaID]; found {
		return v, true, nil
	}
	return part, true, fmt.Errorf("%s of %s: %w", what, mediaID, ErrNotFound)
}

func (p *pageMediaRepository) FindMediaFile(ctx context.Context, id string) (content.MediaFile, error) {
	if f, ok, err := mediaPart(p, p.parts.Files, id, "media file"); ok {
		return f, err
	}
	return p.MediaRepository.FindMediaFile(ctx, id)
}

func (p *pageMediaRepository) FindUploadedMedia(ctx context.Context, mediaID string) (content.UploadedMedia, error) {
	if u, ok, err := mediaPart(p, p.parts.Uploads, mediaID, "uploaded media"); ok {
		return u, err
	}
	return p.MediaRepository.FindUploadedMedia(ctx, mediaID)
}

func (p *pageMediaRepository) FindRemoteMedia(ctx context.Context, mediaID string) (content.RemoteMedia, error) {
	if r, ok, err := mediaPart(p, p.parts.Remotes, mediaID, "remote media"); ok {
		return r, err
	}
	return p.MediaRepository.FindRemoteMedia(ctx, mediaID)
}

// FindLatestProcessingResult returns the newest result of a file of the page, as the repository's
// returns the last of its results.
func (p *pageMediaRepository) FindLatestProcessingResult(ctx context.Context, mediaID string) (content.MediaProcessingResult, error) {
	if rs, ok, err := mediaPart(p, p.parts.Results, mediaID, "media processing result"); ok {
		if err != nil || len(rs) == 0 {
			return content.MediaProcessingResult{}, fmt.Errorf("media processing result of %s: %w", mediaID, ErrNotFound)
		}
		return rs[len(rs)-1], nil
	}
	return p.MediaRepository.FindLatestProcessingResult(ctx, mediaID)
}

func (p *pageMediaRepository) FindFocus(ctx context.Context, mediaID string) (content.MediaFocus, error) {
	if f, ok, err := mediaPart(p, p.parts.Focus, mediaID, "media focus"); ok {
		return f, err
	}
	return p.MediaRepository.FindFocus(ctx, mediaID)
}

func (p *pageMediaRepository) FindDescription(ctx context.Context, mediaID string) (content.MediaDescription, error) {
	if d, ok, err := mediaPart(p, p.parts.Descriptions, mediaID, "media description"); ok {
		return d, err
	}
	return p.MediaRepository.FindDescription(ctx, mediaID)
}
