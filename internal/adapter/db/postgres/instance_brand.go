// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"math"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// InsertBrandRevision implements instance.BrandRepository.
func (r *InstanceRepository) InsertBrandRevision(ctx context.Context, tx uow.Tx, rev instance.BrandRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInstanceBrandRevision(ctx, postgresgen.InsertInstanceBrandRevisionParams{
		ID: rev.ID, Kind: string(rev.Kind), Action: string(rev.Action), BackgroundColor: toNullString(rev.Background),
		RevisedByUserID: toNullString(rev.RevisedByUserID), RevisedAt: rev.RevisedAt,
	})
	if err != nil {
		return wrap(err, "insert instance brand revision")
	}
	for _, f := range rev.Files {
		err := q.InsertInstanceBrandFile(ctx, postgresgen.InsertInstanceBrandFileParams{
			BrandRevisionID: rev.ID, Rendition: string(f.Rendition), BlobKey: f.Key, ContentType: f.ContentType,
			Width: int32(min(f.Width, math.MaxInt32)), Height: int32(min(f.Height, math.MaxInt32)), ByteSize: f.Size, Sha256: f.SHA256, //nolint:gosec // bounded above
		})
		if err != nil {
			return wrap(err, "insert instance brand file")
		}
	}
	return nil
}

// BrandHead implements instance.BrandRepository.
func (r *InstanceRepository) BrandHead(ctx context.Context, tx uow.Tx, kind instance.BrandKind) (instance.BrandHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.BrandHead{}, err
	}
	id, err := q.GetInstanceBrandHead(ctx, string(kind))
	if err != nil {
		return instance.BrandHead{}, notFound(err, appinstance.ErrNotFound, "instance brand "+string(kind))
	}
	return instance.BrandHead{Kind: kind, RevisionID: id}, nil
}

// UpsertBrandHead implements instance.BrandRepository.
func (r *InstanceRepository) UpsertBrandHead(ctx context.Context, tx uow.Tx, h instance.BrandHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertInstanceBrandHead(ctx, postgresgen.UpsertInstanceBrandHeadParams{Kind: string(h.Kind), RevisionID: h.RevisionID}), "upsert instance brand")
}

// DeleteBrandHead implements instance.BrandRepository.
func (r *InstanceRepository) DeleteBrandHead(ctx context.Context, tx uow.Tx, kind instance.BrandKind) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteInstanceBrandHead(ctx, string(kind)), "delete instance brand")
}

// DeleteAllBrandHeads implements instance.BrandRepository.
func (r *InstanceRepository) DeleteAllBrandHeads(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteAllInstanceBrandHeads(ctx), "delete instance brands")
}

// ListBrandRevisionHeads implements instance.BrandRepository.
func (r *InstanceRepository) ListBrandRevisionHeads(ctx context.Context, tx uow.Tx) ([]instance.BrandRevision, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListInstanceBrandRevisionHeads(ctx)
	if err != nil {
		return nil, wrap(err, "list instance brand revisions")
	}
	out := make([]instance.BrandRevision, len(rows))
	for i, row := range rows {
		out[i] = instance.BrandRevision{ID: row.ID, Kind: instance.BrandKind(row.Kind), Action: instance.BrandAction(row.Action)}
	}
	return out, nil
}

// CurrentBrands implements instance.BrandRepository.
func (r *InstanceRepository) CurrentBrands(ctx context.Context) (instance.Brands, error) {
	q := r.db.reader()
	heads, err := q.ListInstanceBrandHeads(ctx)
	if err != nil {
		return instance.Brands{}, wrap(err, "list instance brands")
	}
	var out instance.Brands
	for _, h := range heads {
		rev, err := findBrandRevision(ctx, q, h.RevisionID)
		if err != nil {
			return instance.Brands{}, err
		}
		out.Set(&rev)
	}
	return out, nil
}

// FindBrandRevision implements instance.BrandRepository.
func (r *InstanceRepository) FindBrandRevision(ctx context.Context, id string) (instance.BrandRevision, error) {
	return findBrandRevision(ctx, r.db.reader(), id)
}

func findBrandRevision(ctx context.Context, q *postgresgen.Queries, id string) (instance.BrandRevision, error) {
	row, err := q.GetInstanceBrandRevision(ctx, id)
	if err != nil {
		return instance.BrandRevision{}, notFound(err, appinstance.ErrNotFound, "instance brand revision "+id)
	}
	files, err := q.ListInstanceBrandFiles(ctx, id)
	if err != nil {
		return instance.BrandRevision{}, wrap(err, "list instance brand files")
	}
	rev := instance.BrandRevision{
		ID: row.ID, Kind: instance.BrandKind(row.Kind), Action: instance.BrandAction(row.Action),
		Background: fromNullString(row.BackgroundColor), RevisedByUserID: fromNullString(row.RevisedByUserID), RevisedAt: utc(row.RevisedAt),
	}
	for _, f := range files {
		rev.Files = append(rev.Files, instance.BrandFile{
			Rendition: instance.BrandRendition(f.Rendition), Key: f.BlobKey, ContentType: f.ContentType,
			Width: int(f.Width), Height: int(f.Height), Size: f.ByteSize, SHA256: f.Sha256,
		})
	}
	return rev, nil
}
