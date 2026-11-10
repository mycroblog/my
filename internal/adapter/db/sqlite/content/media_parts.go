// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
)

// FindMediaParts implements content.MediaRepository: one query per part, built as the reads of
// one file build it.
func (r *MediaRepository) FindMediaParts(ctx context.Context, ids []string) (appcontent.MediaParts, error) {
	out := appcontent.MediaParts{
		Files: map[string]content.MediaFile{}, Uploads: map[string]content.UploadedMedia{}, Remotes: map[string]content.RemoteMedia{},
		Results: map[string][]content.MediaProcessingResult{}, Focus: map[string]content.MediaFocus{}, Descriptions: map[string]content.MediaDescription{},
	}
	if len(ids) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return out, err
	}
	q := r.db.reader()
	files, err := q.ListMediaFilesByIDs(ctx, list)
	if err != nil {
		return out, sqlite.Wrap(err, "list media files")
	}
	for _, row := range files {
		out.Files[row.ID] = mediaFileFrom(row)
	}
	uploads, err := q.ListUploadedMediaByIDs(ctx, list)
	if err != nil {
		return out, sqlite.Wrap(err, "list uploaded media")
	}
	for _, row := range uploads {
		out.Uploads[row.MediaID] = uploadedMediaFrom(row)
	}
	remotes, err := r.ListRemoteMedia(ctx, ids)
	if err != nil {
		return out, err
	}
	for _, m := range remotes {
		out.Remotes[m.MediaID] = m
	}
	results, err := q.ListMediaProcessingResultsOf(ctx, list)
	if err != nil {
		return out, sqlite.Wrap(err, "list media processing results")
	}
	variants, err := q.ListMediaVariantsOfMedia(ctx, list)
	if err != nil {
		return out, sqlite.Wrap(err, "list media variants")
	}
	rowsOf := map[string][]contentgen.MediaProcessingResult{}
	for _, row := range results {
		rowsOf[row.MediaID] = append(rowsOf[row.MediaID], row)
	}
	for mediaID, rows := range rowsOf {
		out.Results[mediaID] = processingResultsFrom(rows, variants)
	}
	focus, err := q.ListMediaFocusOf(ctx, list)
	if err != nil {
		return out, sqlite.Wrap(err, "list media focus")
	}
	for _, row := range focus {
		out.Focus[row.MediaID] = mediaFocusFrom(row)
	}
	descriptions, err := q.ListMediaDescriptionsOf(ctx, list)
	if err != nil {
		return out, sqlite.Wrap(err, "list media descriptions")
	}
	for _, row := range descriptions {
		out.Descriptions[row.MediaID] = mediaDescriptionFrom(row)
	}
	return out, nil
}
