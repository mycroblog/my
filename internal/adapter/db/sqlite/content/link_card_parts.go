// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/domain/content"
)

// StatusLinkCardURLs implements content.LinkCardRepository.
func (r *StatusRepository) StatusLinkCardURLs(ctx context.Context, statusIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(statusIDs) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListStatusLinkCardURLs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list status link cards")
	}
	for _, row := range rows {
		out[row.StatusID] = row.Url
	}
	return out, nil
}

// FindLinkCards implements content.LinkCardRepository.
func (r *StatusRepository) FindLinkCards(ctx context.Context, urls []string) (map[string]content.LinkCard, error) {
	out := map[string]content.LinkCard{}
	if len(urls) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(urls)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListLinkCardsByURLs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list link cards")
	}
	for _, row := range rows {
		out[row.Url] = linkCard(row)
	}
	return out, nil
}

// FindLinkCardMedias implements content.LinkCardRepository.
func (r *StatusRepository) FindLinkCardMedias(ctx context.Context, mediaIDs []string) (map[string]content.LinkCardMedia, error) {
	out := map[string]content.LinkCardMedia{}
	if len(mediaIDs) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(mediaIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListLinkCardMediaByIDs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list link card media")
	}
	for _, row := range rows {
		out[row.MediaID] = linkCardMedia(row)
	}
	return out, nil
}
