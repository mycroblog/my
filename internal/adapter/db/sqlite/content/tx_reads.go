// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// ListReadyVariantsTx implements the reads in a transaction.
func (r *MediaRepository) ListReadyVariantsTx(ctx context.Context, tx uow.Tx, ids []string) ([]appcontent.FileVariant, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&MediaRepository{db: db}).ListReadyVariants(ctx, ids)
}

// ListRemoteMediaTx implements the reads in a transaction.
func (r *MediaRepository) ListRemoteMediaTx(ctx context.Context, tx uow.Tx, ids []string) ([]content.RemoteMedia, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&MediaRepository{db: db}).ListRemoteMedia(ctx, ids)
}

// ListCurrentCustomEmojisTx implements the reads in a transaction.
func (r *MediaRepository) ListCurrentCustomEmojisTx(ctx context.Context, tx uow.Tx, ids []string) ([]content.CurrentCustomEmoji, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&MediaRepository{db: db}).ListCurrentCustomEmojis(ctx, ids)
}

// FindCustomEmojiRecordTx implements the reads in a transaction.
func (r *CustomEmojiRepository) FindCustomEmojiRecordTx(ctx context.Context, tx uow.Tx, id string) (content.CustomEmojiRecord, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return content.CustomEmojiRecord{}, err
	}
	return (&CustomEmojiRepository{db: db}).FindCustomEmojiRecord(ctx, id)
}

// FindCustomEmojiByShortcodeTx implements the reads in a transaction.
func (r *StatusRepository) FindCustomEmojiByShortcodeTx(ctx context.Context, tx uow.Tx, shortcode, domain string) (content.CustomEmoji, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return content.CustomEmoji{}, err
	}
	return (&StatusRepository{db: db}).FindCustomEmojiByShortcode(ctx, shortcode, domain)
}

// FindRevisionTx implements the reads in a transaction.
func (r *StatusRepository) FindRevisionTx(ctx context.Context, tx uow.Tx, id string) (content.Revision, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return content.Revision{}, err
	}
	return (&StatusRepository{db: db}).FindRevision(ctx, id)
}

// FindDeletionByURITx implements the reads in a transaction.
func (r *StatusRepository) FindDeletionByURITx(ctx context.Context, tx uow.Tx, uri string) (content.Deletion, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return content.Deletion{}, err
	}
	return (&StatusRepository{db: db}).FindDeletionByURI(ctx, uri)
}

// FindStatusByURITx implements the reads in a transaction.
func (r *StatusRepository) FindStatusByURITx(ctx context.Context, tx uow.Tx, uri string) (content.Status, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return content.Status{}, err
	}
	return (&StatusRepository{db: db}).FindStatusByURI(ctx, uri)
}

// FindBoostRetractionTx implements the reads in a transaction.
func (r *InteractionRepository) FindBoostRetractionTx(ctx context.Context, tx uow.Tx, boostID string) (content.BoostRetraction, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return content.BoostRetraction{}, err
	}
	return (&InteractionRepository{db: db}).FindBoostRetraction(ctx, boostID)
}
