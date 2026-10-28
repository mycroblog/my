// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"

	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// The instance's own icon and logo (ADR-0116): a sanitized SVG of each kind with the PNG
// renditions that the admin screen draws from it, kept in the BlobStore under keys the server
// makes, versioned like the settings.

// BrandRepository is the part of Repository about the brand images (data-model §4.9).
type BrandRepository interface {
	// InsertBrandRevision inserts a revision with its file rows.
	InsertBrandRevision(ctx context.Context, tx uow.Tx, rev instance.BrandRevision) error
	// BrandHead returns the instance_brands row of kind.
	BrandHead(ctx context.Context, tx uow.Tx, kind instance.BrandKind) (instance.BrandHead, error)
	UpsertBrandHead(ctx context.Context, tx uow.Tx, h instance.BrandHead) error
	DeleteBrandHead(ctx context.Context, tx uow.Tx, kind instance.BrandKind) error
	DeleteAllBrandHeads(ctx context.Context, tx uow.Tx) error
	// ListBrandRevisionHeads returns the ID, kind, and action of every revision, oldest first,
	// for rebuilding.
	ListBrandRevisionHeads(ctx context.Context, tx uow.Tx) ([]instance.BrandRevision, error)

	// CurrentBrands returns the current revision of every kind that is set, with its files.
	CurrentBrands(ctx context.Context) (instance.Brands, error)
	// FindBrandRevision returns a revision, current or not, with its files.
	FindBrandRevision(ctx context.Context, id string) (instance.BrandRevision, error)
}

// EventBrandRevised is the event type of BrandRevised.
const EventBrandRevised = "instance.brand_revised"

// BrandRevised is emitted when a brand revision is inserted.
type BrandRevised struct {
	Revision instance.BrandRevision
}

// EventType implements uow.Event.
func (BrandRevised) EventType() string { return EventBrandRevised }

// AuditTargetBrand returns the target ID (and label) of the audit log entries of the changes of
// a brand kind, which are setting changes of the setting named "brand_<kind>".
func AuditTargetBrand(kind instance.BrandKind) string { return "brand_" + string(kind) }

func registerBrandProjectors(r *uow.Projectors, repo Repository) {
	r.Register(EventBrandRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		rev := e.(BrandRevised).Revision
		head, err := repo.BrandHead(ctx, tx, rev.Kind)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if !rev.Supersedes(head) {
			return nil
		}
		if rev.Action == instance.BrandClear {
			return repo.DeleteBrandHead(ctx, tx, rev.Kind)
		}
		return repo.UpsertBrandHead(ctx, tx, instance.BrandHead{Kind: rev.Kind, RevisionID: rev.ID})
	})
}

func rebuildBrandHeads(ctx context.Context, tx uow.Tx, repo Repository) error {
	if err := repo.DeleteAllBrandHeads(ctx, tx); err != nil {
		return err
	}
	revs, err := repo.ListBrandRevisionHeads(ctx, tx)
	if err != nil {
		return err
	}
	last := map[instance.BrandKind]instance.BrandRevision{}
	for _, r := range revs {
		if cur, ok := last[r.Kind]; !ok || r.Supersedes(instance.BrandHead{Kind: r.Kind, RevisionID: cur.ID}) {
			last[r.Kind] = r
		}
	}
	for _, k := range instance.BrandKinds {
		if r, ok := last[k]; ok && r.Action == instance.BrandSet {
			if err := repo.UpsertBrandHead(ctx, tx, instance.BrandHead{Kind: k, RevisionID: r.ID}); err != nil {
				return err
			}
		}
	}
	return nil
}

// Brands returns the current brand images; an unset kind is nil.
func (s *Service) Brands(ctx context.Context) (instance.Brands, error) {
	return s.d.Repository.CurrentBrands(ctx)
}

// SanitizeBrand checks and sanitizes an SVG of kind without saving it (ADR-0116 §2), so that
// the admin screen can draw the renditions from the result. It fails with an
// *instance.BrandError.
func (s *Service) SanitizeBrand(kind instance.BrandKind, svg []byte) (instance.BrandSVG, error) {
	return instance.SanitizeBrandSVG(kind, svg)
}

// SetBrandInput replaces a brand image.
type SetBrandInput struct {
	Kind instance.BrandKind
	// SVG is the uploaded SVG; it is sanitized again and only the result is kept.
	SVG []byte
	// Renditions are the PNG renditions of the kind (instance.BrandRenditions), every one of them.
	Renditions map[instance.BrandRendition][]byte
	// Background is the #rrggbb color behind the icon's opaque renditions; "" takes the theme's
	// light surface. Logos take none.
	Background string
	// RevisedByUserID is the user making the change, or "" for the CLI.
	RevisedByUserID string
}

// SetBrandResult is a saved brand revision with what the sanitizer dropped from its SVG.
type SetBrandResult struct {
	Revision instance.BrandRevision
	Dropped  []instance.BrandDropped
}

// SetBrand sanitizes the SVG, checks every PNG rendition (instance.CheckBrandPNG), writes the files
// to the BlobStore, and records them as a new revision, which becomes the current image of the
// kind; nothing is recorded when one file is rejected (an *instance.BrandError). The files are
// written before the transaction (ADR-0003) and removed again when it fails. The change is
// audited in the same transaction.
func (s *Service) SetBrand(ctx context.Context, in SetBrandInput) (SetBrandResult, error) {
	if s.d.Blobs == nil {
		return SetBrandResult{}, errors.New("instance: no blob store for the brand images")
	}
	if _, ok := instance.ParseBrandKind(string(in.Kind)); !ok {
		return SetBrandResult{}, fmt.Errorf("%w: brand kind %q", instance.ErrInvalidValue, in.Kind)
	}
	svg, err := instance.SanitizeBrandSVG(in.Kind, in.SVG)
	if err != nil {
		return SetBrandResult{}, err
	}
	background, err := s.brandBackground(ctx, in)
	if err != nil {
		return SetBrandResult{}, err
	}
	want := instance.BrandRenditions(in.Kind)
	for r := range in.Renditions {
		if !slices.Contains(want, r) {
			return SetBrandResult{}, &instance.BrandError{Code: instance.BrandUnexpectedRendition, Rendition: r, Detail: "the " + string(in.Kind) + " has no such rendition"}
		}
	}
	id := s.d.IDs.NewID()
	type pending struct {
		file instance.BrandFile
		data []byte
	}
	files := []pending{{
		file: instance.BrandFile{
			Rendition: instance.RenditionSVG, Width: max(1, int(math.Round(svg.ViewWidth))), Height: max(1, int(math.Round(svg.ViewHeight))),
		},
		data: svg.SVG,
	}}
	for _, r := range want {
		data, ok := in.Renditions[r]
		if !ok {
			return SetBrandResult{}, &instance.BrandError{Code: instance.BrandMissingRendition, Rendition: r, Detail: "every rendition is needed"}
		}
		out, w, h, err := instance.CheckBrandPNG(r, data, svg.ViewWidth, svg.ViewHeight)
		if err != nil {
			return SetBrandResult{}, err
		}
		files = append(files, pending{file: instance.BrandFile{Rendition: r, Width: w, Height: h}, data: out})
	}
	rev := instance.BrandRevision{
		ID: id, Kind: in.Kind, Action: instance.BrandSet, Background: background,
		RevisedByUserID: in.RevisedByUserID, RevisedAt: s.d.Clock.Now(),
	}
	for i := range files {
		f := &files[i].file
		sum := sha256.Sum256(files[i].data)
		f.Key, f.ContentType = instance.BrandBlobKey(id, f.Rendition), f.Rendition.ContentType()
		f.Size, f.SHA256 = int64(len(files[i].data)), hex.EncodeToString(sum[:])
		rev.Files = append(rev.Files, *f)
	}
	var written []string
	cleanup := func() {
		// The keys are this revision's own, so removing them never touches a recorded file.
		for _, k := range written {
			_ = s.d.Blobs.Delete(context.WithoutCancel(ctx), k)
		}
	}
	for _, p := range files {
		if _, err := s.d.Blobs.Put(ctx, p.file.Key, bytes.NewReader(p.data), blob.PutOptions{ContentType: p.file.ContentType}); err != nil {
			cleanup()
			return SetBrandResult{}, fmt.Errorf("set brand: store %s: %w", p.file.Rendition, err)
		}
		written = append(written, p.file.Key)
	}
	audit := s.audit(appmoderation.AuditSettingChanged, appmoderation.AuditTargetSetting, AuditTargetBrand(in.Kind), AuditTargetBrand(in.Kind),
		rev.ID, in.RevisedByUserID, rev.RevisedAt)
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.InsertBrandRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := tx.Emit(ctx, BrandRevised{Revision: rev}); err != nil {
			return err
		}
		return tx.Emit(ctx, audit)
	})
	if err != nil {
		cleanup()
		return SetBrandResult{}, fmt.Errorf("set brand: %w", err)
	}
	return SetBrandResult{Revision: rev, Dropped: svg.Dropped}, nil
}

// brandBackground returns the background color of a revision: the given one or the theme's
// light surface for an icon, none for a logo.
func (s *Service) brandBackground(ctx context.Context, in SetBrandInput) (string, error) {
	if in.Kind != instance.BrandIcon {
		if in.Background != "" {
			return "", &instance.BrandError{Code: instance.BrandInvalidColor, Detail: "only the icon has a background color"}
		}
		return "", nil
	}
	if in.Background != "" {
		return instance.NormalizeBrandBackground(in.Background)
	}
	var tokens []instance.ThemeTokenValue
	if rev, ok, err := s.Theme(ctx); err != nil {
		return "", err
	} else if ok {
		tokens = rev.Tokens
	}
	return instance.NormalizeBrandBackground(instance.ThemeColor(tokens, "color-surface", instance.SchemeLight))
}

// ClearBrand brings a brand kind back to the built-in images by recording a clear revision. The
// files of the earlier revisions stay, since their URLs never change. The change is audited in
// the same transaction.
func (s *Service) ClearBrand(ctx context.Context, kind instance.BrandKind, revisedByUserID string) (instance.BrandRevision, error) {
	if _, ok := instance.ParseBrandKind(string(kind)); !ok {
		return instance.BrandRevision{}, fmt.Errorf("%w: brand kind %q", instance.ErrInvalidValue, kind)
	}
	rev := instance.BrandRevision{
		ID: s.d.IDs.NewID(), Kind: kind, Action: instance.BrandClear, RevisedByUserID: revisedByUserID, RevisedAt: s.d.Clock.Now(),
	}
	audit := s.audit(appmoderation.AuditSettingChanged, appmoderation.AuditTargetSetting, AuditTargetBrand(kind), AuditTargetBrand(kind),
		rev.ID, revisedByUserID, rev.RevisedAt)
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.InsertBrandRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := tx.Emit(ctx, BrandRevised{Revision: rev}); err != nil {
			return err
		}
		return tx.Emit(ctx, audit)
	})
	if err != nil {
		return instance.BrandRevision{}, fmt.Errorf("clear brand: %w", err)
	}
	return rev, nil
}

// BrandFile opens a file of a revision, current or not, whose URL never changes
// (instance.BrandFilePath); close the reader when done. It fails with an error wrapping
// ErrNotFound for an unknown revision or rendition.
func (s *Service) BrandFile(ctx context.Context, revisionID string, rendition instance.BrandRendition) (instance.BrandFile, io.ReadCloser, error) {
	if !kernel.ValidID(revisionID) || s.d.Blobs == nil {
		return instance.BrandFile{}, nil, ErrNotFound
	}
	rev, err := s.d.Repository.FindBrandRevision(ctx, revisionID)
	if err != nil {
		return instance.BrandFile{}, nil, err
	}
	f, ok := rev.File(rendition)
	if !ok {
		return instance.BrandFile{}, nil, fmt.Errorf("brand file %s of %s: %w", rendition, revisionID, ErrNotFound)
	}
	r, _, err := s.d.Blobs.Get(ctx, f.Key)
	if errors.Is(err, blob.ErrNotFound) {
		return instance.BrandFile{}, nil, fmt.Errorf("brand file %s: %w", f.Key, ErrNotFound)
	}
	if err != nil {
		return instance.BrandFile{}, nil, err
	}
	return f, r, nil
}
