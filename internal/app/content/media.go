// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"slices"
	"strings"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// Job types of the media files.
const (
	// JobProcessMedia processes one upload: payload {"media_id": "..."}.
	JobProcessMedia = "media.process"
	// JobCleanupMedia deletes the uploads that nothing attached in time (FR-803); no payload.
	JobCleanupMedia = "media.cleanup"
)

// Defaults of MediaDeps.
const (
	// DefaultUnattachedTTL is how long an upload may stay unattached before the cleanup deletes
	// it.
	DefaultUnattachedTTL = 24 * time.Hour
	// DefaultCleanupInterval is the period of the cleanup schedule.
	DefaultCleanupInterval = time.Hour
	// cleanupBatch is the number of uploads one cleanup query looks at; cleanupMaxBatches
	// bounds the batches of one run, and the next run continues.
	cleanupBatch      = 100
	cleanupMaxBatches = 10
)

// ErrContentTypeMismatch reports an upload whose declared media type differs from the type of
// its content (ADR-0019 §8).
var ErrContentTypeMismatch = errors.New("content: the declared content type does not match the content")

// MediaReferrer is a module that refers to media files from outside the content module's
// status revisions, such as the account module's avatars and headers. The cleanup keeps every
// file that a referrer reports as referenced.
type MediaReferrer interface {
	IsMediaReferenced(ctx context.Context, mediaID string) (bool, error)
}

// MediaDeps are the ports a MediaService uses.
type MediaDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository MediaRepository
	Blobs      blob.Store
	Processor  media.Processor
	// Outbox enqueues the processing job in the upload's transaction. When it is nil (the
	// inline queue), Jobs enqueues it after the commit.
	Outbox queue.Outbox
	Jobs   queue.JobQueue
	Clock  kernel.Clock
	IDs    kernel.IDGenerator
	// Limits bound uploads; zero fields take media.DefaultLimits.
	Limits media.Limits
	// Sizes bound the variants; zero fields take media.DefaultSizes.
	Sizes media.Sizes
	// AVLimits bound video and audio uploads (ADR-0054 §6); zero fields take
	// media.DefaultAVLimits.
	AVLimits media.AVLimits
	// UnattachedTTL is how long an upload may stay unattached; zero means DefaultUnattachedTTL.
	UnattachedTTL time.Duration
	Referrers     []MediaReferrer
	// Proxy signs the media proxy URLs that present remote media files (federation.md §9);
	// nil presents no remote file.
	Proxy *MediaProxy
	// Logger logs what jobs survive; nil means slog.Default().
	Logger *slog.Logger
}

// MediaService runs the use cases of media files: uploads, their processing, focal points,
// and the cleanup of unattached uploads (FR-105, FR-803, ADR-0005, ADR-0019 §8).
//
// An upload is checked and stored synchronously: the content must be an accepted format
// detected from its bytes, within the size and dimension limits. The file as received stays in
// blob storage under a private key (uploads/<id>) until the media.process job has re-encoded
// it without metadata into public variants for its purpose (ADR-0054): small, medium, and
// large still ones, and for an animation also moving ones, under keys that name the
// processing result (media/<id>/<result>/<size>.<ext> and
// media/<id>/<result>/<size>-animated.<ext>), so that they never change and can be cached for
// good. Then the job deletes the file as received, so that no location data is kept. Using a
// file for another purpose processes its large variant again for that purpose.
//
// A video or audio file is never transcoded (ADR-0054 §6): the job remuxes it without metadata
// into one large animated variant (media/<id>/<result>/large-animated.<ext>), and processes the
// poster that the client sent with it, or else the cover embedded in the file, into the still
// variants and the blurhash of an attachment.
type MediaService struct {
	d MediaDeps
}

// NewMediaService returns a MediaService on d.
func NewMediaService(d MediaDeps) *MediaService {
	d.Limits = d.Limits.WithDefaults()
	d.Sizes = d.Sizes.WithDefaults()
	d.AVLimits = d.AVLimits.WithDefaults()
	if d.UnattachedTTL <= 0 {
		d.UnattachedTTL = DefaultUnattachedTTL
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &MediaService{d: d}
}

// Limits returns the upload limits of images in effect.
func (s *MediaService) Limits() media.Limits { return s.d.Limits }

// MaxUploadBytes returns the size limit of an uploaded file, an image or a video or audio file.
func (s *MediaService) MaxUploadBytes() int64 { return max(s.d.Limits.MaxBytes, s.d.AVLimits.MaxBytes) }

// RegisterJobs registers the media job handlers on reg and the cleanup schedule on sched.
func (s *MediaService) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobProcessMedia, s.handleProcess)
	reg.Register(JobCleanupMedia, s.handleCleanup)
	sched.Add(queue.Schedule{Name: JobCleanupMedia, Every: DefaultCleanupInterval, Job: queue.NewJob{Type: JobCleanupMedia}})
}

// UploadInput describes an upload.
type UploadInput struct {
	OwnerAccountID string
	Content        io.Reader
	// DeclaredContentType is the media type the client sent, or "". It is never trusted: an
	// upload whose content is of another type fails with ErrContentTypeMismatch.
	DeclaredContentType string
	// Description is the alternative text, or "" for none.
	Description string
	// Purpose is what the file is for, which picks the sizes of its variants; "" is
	// content.PurposeAttachment.
	Purpose content.MediaPurpose
	// Poster is the image that a video or audio attachment shows before it plays, which the
	// client takes from the file (ADR-0054 §6); nil for none. It is ignored for an image.
	Poster io.Reader
}

// ErrInvalidPurpose reports an upload for an unknown purpose.
var ErrInvalidPurpose = errors.New("content: unknown media purpose")

// Upload checks and stores an upload and enqueues its processing. An image is checked against
// the image limits; a video or audio file (an attachment only) against the video and audio
// limits, with its container parsed and its codecs checked. It fails with an error wrapping
// media.ErrUnsupportedFormat (media.ErrUnsupportedCodec for a codec such as HEVC),
// media.ErrTooLarge, media.ErrDimensions, media.ErrTooLong, media.ErrCorrupt,
// ErrContentTypeMismatch, or ErrInvalidPoster for content it does not accept, and with
// content.ErrDescriptionTooLong or content.ErrInvalidDescription for a description it does not
// accept.
func (s *MediaService) Upload(ctx context.Context, in UploadInput) (Media, error) {
	if err := content.ValidateMediaDescription(in.Description); err != nil {
		return Media{}, err
	}
	if in.Purpose == "" {
		in.Purpose = content.PurposeAttachment
	}
	if !in.Purpose.Valid() {
		return Media{}, fmt.Errorf("%w: %q", ErrInvalidPurpose, in.Purpose)
	}
	av := s.d.AVLimits
	data, err := io.ReadAll(io.LimitReader(in.Content, s.MaxUploadBytes()+1))
	if err != nil {
		return Media{}, fmt.Errorf("content: read upload: %w", err)
	}
	file := content.MediaFile{ID: s.d.IDs.NewID(), OwnerAccountID: in.OwnerAccountID, Origin: content.MediaOriginUpload, CreatedAt: s.d.Clock.Now()}
	up := content.UploadedMedia{MediaID: file.ID, BlobKey: uploadKey(file.ID), FileSize: int64(len(data))}
	var poster []byte
	if format := media.SniffAV(data); format != "" {
		if up.ContentType, poster, err = s.checkAV(ctx, in, data, format, av); err != nil {
			return Media{}, err
		}
	} else {
		probe, err := s.d.Processor.Inspect(ctx, data, s.d.Limits)
		if err != nil {
			return Media{}, err
		}
		if err := checkDeclaredType(in.DeclaredContentType, probe.Format); err != nil {
			return Media{}, err
		}
		up.ContentType = probe.Format.ContentType()
	}
	// The keys are reserved before the blobs are written, so that a blob whose write fails
	// after it was stored, or whose upload is never recorded, is still found and deleted.
	journal := s.journal()
	keys := []string{up.BlobKey}
	if poster != nil {
		keys = append(keys, posterKey(file.ID))
	}
	if err := journal.reserve(ctx, keys...); err != nil {
		return Media{}, err
	}
	if _, err := s.d.Blobs.Put(ctx, up.BlobKey, bytes.NewReader(data), blob.PutOptions{ContentType: up.ContentType}); err != nil {
		journal.discard(ctx, keys...)
		return Media{}, err
	}
	if poster != nil {
		if _, err := s.d.Blobs.Put(ctx, posterKey(file.ID), bytes.NewReader(poster), blob.PutOptions{}); err != nil {
			journal.discard(ctx, keys...)
			return Media{}, err
		}
	}
	job := processJob(file.ID, in.Purpose, "")
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := journal.keep(ctx, tx, keys...); err != nil {
			return err
		}
		if err := s.d.Repository.InsertMediaFile(ctx, tx, file); err != nil {
			return err
		}
		if err := s.d.Repository.InsertUploadedMedia(ctx, tx, up); err != nil {
			return err
		}
		if in.Description != "" {
			if err := s.d.Repository.UpsertDescription(ctx, tx, content.MediaDescription{MediaID: file.ID, Text: in.Description}); err != nil {
				return err
			}
		}
		if s.d.Outbox != nil {
			return s.d.Outbox.EnqueueTx(ctx, tx, job)
		}
		return nil
	})
	if err != nil {
		// A commit that failed to answer may have recorded the upload.
		if _, ferr := s.d.Repository.FindUploadedMedia(context.WithoutCancel(ctx), file.ID); !errors.Is(ferr, ErrNotFound) {
			return Media{}, err
		}
		journal.discard(ctx, keys...)
		return Media{}, err
	}
	if s.d.Outbox == nil {
		if err := s.d.Jobs.Enqueue(ctx, job); err != nil {
			s.d.Logger.ErrorContext(ctx, "media processing not enqueued", slog.String("media_id", file.ID), slog.Any("error", err))
		}
	}
	return s.Media(ctx, file.ID)
}

// checkAV checks a video or audio upload of the container format against av (ADR-0054 §6): it
// must be an attachment whose declared type, if any, is one of the container's, and its poster,
// if any, an image within the image limits. It returns the media type of the file as served and
// the poster.
func (s *MediaService) checkAV(ctx context.Context, in UploadInput, data []byte, format media.Format, av media.AVLimits) (string, []byte, error) {
	if in.Purpose != content.PurposeAttachment {
		return "", nil, fmt.Errorf("%w: a %s file as %s", media.ErrUnsupportedFormat, format, in.Purpose)
	}
	if mt, _, _ := mime.ParseMediaType(in.DeclaredContentType); in.DeclaredContentType != "" && mt != "application/octet-stream" && !format.Declares(in.DeclaredContentType) {
		return "", nil, fmt.Errorf("%w: declared %s, content is %s", ErrContentTypeMismatch, in.DeclaredContentType, format)
	}
	probe, err := s.d.Processor.InspectAV(ctx, data, av)
	if err != nil {
		return "", nil, err
	}
	if in.Poster == nil {
		return probe.ContentType, nil, nil
	}
	poster, err := io.ReadAll(io.LimitReader(in.Poster, s.d.Limits.MaxBytes+1))
	if err != nil {
		return "", nil, fmt.Errorf("content: read poster: %w", err)
	}
	if _, err := s.d.Processor.Inspect(ctx, poster, s.d.Limits); err != nil {
		return "", nil, fmt.Errorf("%w: the poster: %w", ErrInvalidPoster, err)
	}
	return probe.ContentType, poster, nil
}

// ErrInvalidPoster reports a poster of a video or audio upload that is not an accepted image; it
// wraps the error of the image.
var ErrInvalidPoster = errors.New("content: invalid poster")

func checkDeclaredType(declared string, detected media.Format) error {
	if declared == "" {
		return nil
	}
	mt, _, err := mime.ParseMediaType(declared)
	if err != nil {
		return fmt.Errorf("%w: %q", ErrContentTypeMismatch, declared)
	}
	switch mt {
	case "application/octet-stream":
		return nil
	case "image/jpg", "image/pjpeg":
		mt = "image/jpeg"
	}
	if mt != detected.ContentType() {
		return fmt.Errorf("%w: declared %s, content is %s", ErrContentTypeMismatch, mt, detected.ContentType())
	}
	return nil
}

func uploadKey(mediaID string) string { return "uploads/" + mediaID }

// posterKey returns the private key of the poster of a video or audio upload, which stays there
// until processing ends like the file as received.
func posterKey(mediaID string) string { return "uploads/" + mediaID + ".poster" }

// playable reports whether contentType is a video or audio type.
func playable(contentType string) bool {
	return strings.HasPrefix(contentType, "video/") || strings.HasPrefix(contentType, "audio/")
}

// variantKey returns the blob key of a variant of a processing result. The result is part of
// the key, so that a key never names other content.
func variantKey(mediaID, resultID string, size content.MediaSize, motion content.MediaMotion, ext string) string {
	name := string(size)
	if motion == content.MotionAnimated {
		name += "-animated"
	}
	return blob.PublicPrefix + mediaID + "/" + resultID + "/" + name + "." + ext
}

type processPayload struct {
	MediaID string               `json:"media_id"`
	Purpose content.MediaPurpose `json:"purpose,omitempty"`
}

// processJob returns the job that processes a media file for a purpose. after is the current
// processing result that the job replaces, "" for the first processing, so that each change of
// purpose is a job of its own.
func processJob(mediaID string, purpose content.MediaPurpose, after string) queue.NewJob {
	payload, _ := json.Marshal(processPayload{MediaID: mediaID, Purpose: purpose})
	key := JobProcessMedia + ":" + mediaID
	if after != "" {
		key += ":" + after + ":" + string(purpose)
	}
	return queue.NewJob{Type: JobProcessMedia, Payload: payload, IdempotencyKey: key}
}

// Media is a media file with its current state.
type Media struct {
	File content.MediaFile
	// Upload is the upload subtype, nil for a remote file.
	Upload *content.UploadedMedia
	// Remote is the remote subtype with the declared values, nil for an upload.
	Remote *content.RemoteMedia
	// Result is the newest processing result, nil while the file is being processed.
	Result *content.MediaProcessingResult
	// Focus is the focal point, nil when none is set (the center).
	Focus *content.MediaFocus
	// Description is the alternative text that the uploader set, or "". An attachment of a
	// status revision has its own copy.
	Description string
}

// Ready reports whether the file was processed successfully and can be shown.
func (m Media) Ready() bool { return m.Result != nil && m.Result.Outcome == content.MediaReady }

// Media returns a media file with its current state.
func (s *MediaService) Media(ctx context.Context, id string) (Media, error) {
	file, err := s.d.Repository.FindMediaFile(ctx, id)
	if err != nil {
		return Media{}, err
	}
	m := Media{File: file}
	if up, err := s.d.Repository.FindUploadedMedia(ctx, id); err == nil {
		m.Upload = &up
	} else if !errors.Is(err, ErrNotFound) {
		return Media{}, err
	}
	if file.Origin == content.MediaOriginRemote {
		rm, err := s.d.Repository.FindRemoteMedia(ctx, id)
		if err != nil {
			return Media{}, err
		}
		m.Remote = &rm
		return m, nil
	}
	if r, err := s.d.Repository.FindLatestProcessingResult(ctx, id); err == nil {
		m.Result = &r
	} else if !errors.Is(err, ErrNotFound) {
		return Media{}, err
	}
	if f, err := s.d.Repository.FindFocus(ctx, id); err == nil {
		m.Focus = &f
	} else if !errors.Is(err, ErrNotFound) {
		return Media{}, err
	}
	if d, err := s.d.Repository.FindDescription(ctx, id); err == nil {
		m.Description = d.Text
	} else if !errors.Is(err, ErrNotFound) {
		return Media{}, err
	}
	return m, nil
}

// OwnMedia returns a media file of accountID with its current state. A file of another
// account fails with ErrNotFound like an unknown one, so that nobody learns of the uploads of
// others: other accounts see a file only as an attachment of a status they may see.
func (s *MediaService) OwnMedia(ctx context.Context, accountID, id string) (Media, error) {
	m, err := s.Media(ctx, id)
	if err != nil {
		return Media{}, err
	}
	if m.File.OwnerAccountID != accountID {
		return Media{}, fmt.Errorf("%w: media file %s of another account", ErrNotFound, id)
	}
	return m, nil
}

// ErrMediaAttached reports a change to a media file that a status revision attaches: the
// revision has its own copy of the description, so the file can no longer change on its own.
var ErrMediaAttached = errors.New("content: the media file is attached to a status")

// MediaUpdate changes a media file; nil fields keep their value.
type MediaUpdate struct {
	MediaID string
	// Description is the new alternative text; "" removes it.
	Description *string
	// Focus is the new focal point; its MediaID is ignored.
	Focus *content.MediaFocus
}

// Update changes the description or focal point of an unattached media file of accountID and
// returns the file. It fails with content.ErrDescriptionTooLong, content.ErrInvalidDescription,
// or content.ErrInvalidFocus for invalid values, ErrNotFound for an unknown file or one of
// another account, and ErrMediaAttached for a file that a status revision attaches.
func (s *MediaService) Update(ctx context.Context, accountID string, u MediaUpdate) (Media, error) {
	if u.Description != nil {
		if err := content.ValidateMediaDescription(*u.Description); err != nil {
			return Media{}, err
		}
	}
	var focus content.MediaFocus
	if u.Focus != nil {
		focus = content.MediaFocus{MediaID: u.MediaID, X: u.Focus.X, Y: u.Focus.Y}
		if err := focus.Validate(); err != nil {
			return Media{}, err
		}
	}
	if _, err := s.OwnMedia(ctx, accountID, u.MediaID); err != nil {
		return Media{}, err
	}
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if attached, err := s.d.Repository.IsAttached(ctx, tx, u.MediaID); err != nil || attached {
			if err == nil {
				err = ErrMediaAttached
			}
			return err
		}
		switch {
		case u.Description == nil:
		case *u.Description == "":
			if err := s.d.Repository.DeleteDescription(ctx, tx, u.MediaID); err != nil {
				return err
			}
		default:
			if err := s.d.Repository.UpsertDescription(ctx, tx, content.MediaDescription{MediaID: u.MediaID, Text: *u.Description}); err != nil {
				return err
			}
		}
		if u.Focus != nil {
			return s.d.Repository.UpsertFocus(ctx, tx, focus)
		}
		return nil
	})
	if err != nil {
		return Media{}, err
	}
	return s.Media(ctx, u.MediaID)
}

// ErrNotAnImage reports a media file that is not a still image, or whose processing failed.
var ErrNotAnImage = errors.New("content: the media file is not a still image")

// CheckImage fails with ErrNotFound for an unknown media file, and with ErrNotAnImage for a
// file that is not a still image: a processed file of another media type, a file whose
// processing failed, or an upload being processed whose format is not an image. It implements
// instance.MediaFiles for the instance's preview image.
func (s *MediaService) CheckImage(ctx context.Context, id string) error {
	m, err := s.Media(ctx, id)
	if err != nil {
		return err
	}
	switch {
	case m.Result != nil && (!m.Ready() || m.Result.MediaType != content.MediaTypeImage):
		return fmt.Errorf("media %s: %w", id, ErrNotAnImage)
	case m.Result == nil && (m.Upload == nil || !strings.HasPrefix(m.Upload.ContentType, "image/")):
		return fmt.Errorf("media %s: %w", id, ErrNotAnImage)
	}
	return nil
}

// URL returns the public URL of a variant.
func (s *MediaService) URL(v content.MediaVariant) string { return s.d.Blobs.PublicURL(v.BlobKey) }

// ImageRef names a media file that an account shows as an image of its own, such as the
// avatar or header of its profile.
type ImageRef struct {
	AccountID string
	MediaID   string
}

// Images returns the image of each ref's media file, keyed by ref, for the files that the ref's
// account owns: the variants of an upload whose processing succeeded, or the media proxy URLs
// of a remote file (RemoteImage); the other refs are absent. It reads the uploads in one
// repository call and the remote files in another, so that presenting a page of accounts reads
// their images together instead of one by one.
func (s *MediaService) Images(ctx context.Context, refs []ImageRef) (map[ImageRef]Image, error) {
	return s.images(ctx, s.reads(), refs)
}

// ImagesTx returns the images of refs as Images does, read in tx with what tx has written (the
// data of the webhook messages, ADR-0060).
func (s *MediaService) ImagesTx(ctx context.Context, tx uow.Tx, refs []ImageRef) (map[ImageRef]Image, error) {
	return s.images(ctx, s.readsIn(tx), refs)
}

// mediaReads are the reads that presenting images and custom emoji needs: those of the
// repository and the media policy outside transactions (MediaService.reads), or in one
// (MediaService.readsIn).
type mediaReads interface {
	ListReadyVariants(ctx context.Context, ids []string) ([]FileVariant, error)
	ListRemoteMedia(ctx context.Context, ids []string) ([]content.RemoteMedia, error)
	ListCurrentCustomEmojis(ctx context.Context, ids []string) ([]content.CurrentCustomEmoji, error)
	RejectsMedia(ctx context.Context, domain string) (bool, error)
}

type repositoryReads struct{ s *MediaService }

func (r repositoryReads) ListReadyVariants(ctx context.Context, ids []string) ([]FileVariant, error) {
	return r.s.d.Repository.ListReadyVariants(ctx, ids)
}

func (r repositoryReads) ListRemoteMedia(ctx context.Context, ids []string) ([]content.RemoteMedia, error) {
	return r.s.d.Repository.ListRemoteMedia(ctx, ids)
}

func (r repositoryReads) ListCurrentCustomEmojis(ctx context.Context, ids []string) ([]content.CurrentCustomEmoji, error) {
	return r.s.d.Repository.ListCurrentCustomEmojis(ctx, ids)
}

func (r repositoryReads) RejectsMedia(ctx context.Context, domain string) (bool, error) {
	return r.s.d.Proxy.d.Policy.RejectsMedia(ctx, domain)
}

type txMediaReads struct {
	s  *MediaService
	tx uow.Tx
}

func (r txMediaReads) ListReadyVariants(ctx context.Context, ids []string) ([]FileVariant, error) {
	return r.s.d.Repository.ListReadyVariantsTx(ctx, r.tx, ids)
}

func (r txMediaReads) ListRemoteMedia(ctx context.Context, ids []string) ([]content.RemoteMedia, error) {
	return r.s.d.Repository.ListRemoteMediaTx(ctx, r.tx, ids)
}

func (r txMediaReads) ListCurrentCustomEmojis(ctx context.Context, ids []string) ([]content.CurrentCustomEmoji, error) {
	return r.s.d.Repository.ListCurrentCustomEmojisTx(ctx, r.tx, ids)
}

func (r txMediaReads) RejectsMedia(ctx context.Context, domain string) (bool, error) {
	return r.s.d.Proxy.d.Policy.RejectsMediaTx(ctx, r.tx, domain)
}

func (s *MediaService) reads() mediaReads { return repositoryReads{s} }

func (s *MediaService) readsIn(tx uow.Tx) mediaReads { return txMediaReads{s: s, tx: tx} }

func (s *MediaService) images(ctx context.Context, reads mediaReads, refs []ImageRef) (map[ImageRef]Image, error) {
	out := map[ImageRef]Image{}
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.MediaID != "" && !slices.Contains(ids, ref.MediaID) {
			ids = append(ids, ref.MediaID)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	variants, err := reads.ListReadyVariants(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("content: the images %v: %w", ids, err)
	}
	files := map[string]content.MediaFile{}
	byID := map[string][]content.MediaVariant{}
	for _, v := range variants {
		files[v.File.ID] = v.File
		byID[v.File.ID] = append(byID[v.File.ID], v.Variant)
	}
	var rest []string
	for _, ref := range refs {
		if f, ok := files[ref.MediaID]; ok {
			if img, ok := s.image(byID[ref.MediaID]); ok && f.OwnerAccountID == ref.AccountID {
				out[ref] = img
			}
		} else if !slices.Contains(rest, ref.MediaID) {
			rest = append(rest, ref.MediaID)
		}
	}
	if len(rest) == 0 || s.d.Proxy == nil {
		return out, nil
	}
	remote, err := reads.ListRemoteMedia(ctx, rest)
	if err != nil {
		return nil, fmt.Errorf("content: the remote images %v: %w", rest, err)
	}
	byRemote := make(map[string]content.RemoteMedia, len(remote))
	for _, m := range remote {
		byRemote[m.MediaID] = m
	}
	for _, ref := range refs {
		if m, ok := byRemote[ref.MediaID]; ok && m.OwnerAccountID == ref.AccountID {
			out[ref], _ = s.RemoteImage(m)
		}
	}
	return out, nil
}

// Image returns the image of a media file as clients load it: the variants of an upload whose
// processing succeeded, or the media proxy URLs of a remote file; false for a file that has
// neither.
func (s *MediaService) Image(m Media) (Image, bool) {
	if m.Remote != nil {
		return s.RemoteImage(*m.Remote)
	}
	if !m.Ready() {
		return Image{}, false
	}
	return s.image(m.Result.Variants)
}

// image presents the variants of a processing result; false without the still ones, unless it
// is a video or audio file, whose poster is optional.
func (s *MediaService) image(variants []content.MediaVariant) (Image, bool) {
	var (
		img             Image
		still, animated int
		moving          ImageVariants
	)
	for _, v := range variants {
		iv := ImageVariant{URL: s.URL(v), ContentType: v.ContentType, Width: v.Width, Height: v.Height, Size: v.FileSize}
		if playable(v.ContentType) {
			img.Media = &iv
			continue
		}
		set := &img.Still
		if v.Motion == content.MotionAnimated {
			set, animated = &moving, animated+1
		} else {
			still++
		}
		switch v.Size {
		case content.SizeSmall:
			set.Small = iv
		case content.SizeMedium:
			set.Medium = iv
		case content.SizeLarge:
			set.Large = iv
		}
	}
	if still != len(content.MediaSizes) {
		if img.Media != nil {
			return Image{Media: img.Media}, true
		}
		return Image{}, false
	}
	if animated == len(content.MediaSizes) {
		img.Animated = &moving
	}
	return img, true
}

// RemoteImage returns the image of a remote media file through the media proxy (federation.md
// §9): the file itself as the medium and large variants, with the declared dimensions, and, for
// a file that may be an image, the proxy's small still variant as the small one (ADR-0054 §7);
// a video or audio file (gifv included) is its Media alone. It returns false when no proxy is
// configured.
func (s *MediaService) RemoteImage(m content.RemoteMedia) (Image, bool) {
	if s.d.Proxy == nil {
		return Image{}, false
	}
	full := ImageVariant{URL: s.d.Proxy.URL(m.MediaID, ProxyFull), Width: m.Width, Height: m.Height}
	small := full
	switch m.DeclaredMediaType {
	case content.MediaTypeVideo, content.MediaTypeGIFV, content.MediaTypeAudio:
		full.ContentType = m.DeclaredContentType
		return Image{Media: &full}, true
	default:
		if s.d.Proxy.d.Processor != nil {
			small = ImageVariant{URL: s.d.Proxy.URL(m.MediaID, ProxySmall)}
		}
	}
	return Image{Still: ImageVariants{Small: small, Medium: full, Large: full}}, true
}

// ErrNotOwner reports a change to a media file of another account.
var ErrNotOwner = errors.New("content: the media file belongs to another account")

// CheckProfileImage reports whether accountID may use a media file as its avatar or header
// (purpose): the account uploaded it and its processing succeeded. A file processed for
// another purpose is processed again for this one in the background, and shows its current
// variants until then. It implements the account module's CheckProfileImage and fails with an
// error wrapping appaccount.ErrUnusableImage otherwise.
func (s *MediaService) CheckProfileImage(ctx context.Context, accountID, mediaID string, purpose content.MediaPurpose) error {
	m, err := s.Media(ctx, mediaID)
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("media %s: %w", mediaID, appaccount.ErrUnusableImage)
	}
	if err != nil {
		return err
	}
	if m.Upload == nil || m.File.OwnerAccountID != accountID || !m.Ready() || m.Result.MediaType != content.MediaTypeImage {
		return fmt.Errorf("media %s: %w", mediaID, appaccount.ErrUnusableImage)
	}
	return s.RequirePurpose(ctx, m, purpose)
}

// RequirePurpose enqueues the processing of a ready media file for purpose unless its current
// result is for that purpose already.
func (s *MediaService) RequirePurpose(ctx context.Context, m Media, purpose content.MediaPurpose) error {
	if !m.Ready() || m.Result.Purpose == purpose {
		return nil
	}
	if err := s.d.Jobs.Enqueue(ctx, processJob(m.File.ID, purpose, m.Result.ID)); err != nil {
		return fmt.Errorf("content: process media %s for %s: %w", m.File.ID, purpose, err)
	}
	return nil
}

// SetFocus sets the focal point of a media file that accountID owns. It fails with
// content.ErrInvalidFocus for a point outside the image, ErrNotFound for an unknown file, and
// ErrNotOwner for a file of another account.
func (s *MediaService) SetFocus(ctx context.Context, accountID string, f content.MediaFocus) error {
	if err := f.Validate(); err != nil {
		return err
	}
	file, err := s.d.Repository.FindMediaFile(ctx, f.MediaID)
	if err != nil {
		return err
	}
	if file.OwnerAccountID != accountID {
		return ErrNotOwner
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Repository.UpsertFocus(ctx, tx, f) })
}

func (s *MediaService) handleProcess(ctx context.Context, j queue.Job) queue.Result {
	var p processPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil || p.MediaID == "" {
		return queue.Fail(fmt.Errorf("media.process: bad payload %s", j.Payload))
	}
	return s.Process(ctx, p.MediaID, p.Purpose)
}

// Process processes a media file for purpose ("" is content.PurposeAttachment). An upload is
// re-encoded from the file as received into its variants, the result is recorded, and the file
// as received is deleted. A file whose current result is for another purpose is processed
// again from its large variant. It is idempotent: a file whose current result is for purpose
// only has its received file deleted again, and a file that is gone (cleaned up) is done.
// Content the processor rejects records a failed result when the file is processed for the
// first time.
func (s *MediaService) Process(ctx context.Context, mediaID string, purpose content.MediaPurpose) queue.Result {
	if purpose == "" {
		purpose = content.PurposeAttachment
	}
	if !purpose.Valid() {
		return queue.Fail(fmt.Errorf("media.process: %w: %q", ErrInvalidPurpose, purpose))
	}
	var up *content.UploadedMedia
	switch u, err := s.d.Repository.FindUploadedMedia(ctx, mediaID); {
	case err == nil:
		up = &u
	case !errors.Is(err, ErrNotFound):
		return queue.Retry(0, err)
	}
	var current *content.MediaProcessingResult
	switch r, err := s.d.Repository.FindLatestProcessingResult(ctx, mediaID); {
	case err == nil:
		current = &r
	case !errors.Is(err, ErrNotFound):
		return queue.Retry(0, err)
	}
	switch {
	case current != nil && current.Purpose == purpose:
		return s.deleteReceived(ctx, up)
	case current == nil && up == nil:
		return queue.Done() // cleaned up
	case current != nil && current.Outcome != content.MediaReady:
		return s.deleteReceived(ctx, up) // a failed file stays failed
	case current != nil && current.MediaType != content.MediaTypeImage:
		s.d.Logger.WarnContext(ctx, "a video or audio file is not processed for another purpose", slog.String("media_id", mediaID), slog.String("purpose", string(purpose)))
		return s.deleteReceived(ctx, up)
	}

	req := media.Request{Purpose: media.Purpose(purpose)}
	switch f, err := s.d.Repository.FindFocus(ctx, mediaID); {
	case err == nil:
		req.FocusX, req.FocusY = f.X, f.Y
	case !errors.Is(err, ErrNotFound):
		return queue.Retry(0, err)
	}
	result := content.MediaProcessingResult{
		ID: s.d.IDs.NewID(), MediaID: mediaID, Purpose: purpose, MediaType: content.MediaTypeImage, ProcessedAt: s.d.Clock.Now(),
	}
	if current == nil && playable(up.ContentType) {
		if res := s.processAV(ctx, up, &result, req); res != nil {
			return *res
		}
		return s.recordResult(ctx, up, result)
	}
	key := ""
	if current == nil {
		key = up.BlobKey
	} else {
		key = sourceVariant(*current).BlobKey
	}
	out, err := s.processBlob(ctx, key, req)
	switch {
	case err == nil:
		result.Outcome = content.MediaReady
		result.Width, result.Height, result.Blurhash = out.Width, out.Height, out.Blurhash
		if current != nil && current.Width > 0 {
			// The source is a variant: keep the size of the original.
			result.Width, result.Height = current.Width, current.Height
		}
		if err := s.storeVariants(ctx, &result, stillAndAnimated(out)); err != nil {
			return queue.Retry(0, err)
		}
	case current != nil && (media.Rejected(err) || errors.Is(err, blob.ErrNotFound)):
		s.d.Logger.WarnContext(ctx, "media reprocessing failed", slog.String("media_id", mediaID), slog.String("purpose", string(purpose)), slog.Any("error", err))
		return s.deleteReceived(ctx, up)
	case media.Rejected(err) || errors.Is(err, blob.ErrNotFound):
		s.d.Logger.WarnContext(ctx, "media processing failed", slog.String("media_id", mediaID), slog.Any("error", err))
		result.Outcome, result.MediaType = content.MediaFailed, content.MediaTypeUnknown
	default:
		return queue.Retry(0, err)
	}
	return s.recordResult(ctx, up, result)
}

// recordResult records a processing result, then deletes the file as received. The variants of
// a result that is not recorded, because the transaction failed or a concurrent run of the same
// job recorded its result first, are deleted.
func (s *MediaService) recordResult(ctx context.Context, up *content.UploadedMedia, result content.MediaProcessingResult) queue.Result {
	journal := s.journal()
	keys := make([]string, 0, len(result.Variants))
	for _, v := range result.Variants {
		keys = append(keys, v.BlobKey)
	}
	lost := false
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		lost = false
		// A concurrent run of the same job may have recorded its result first.
		done, err := s.d.Repository.HasProcessingResult(ctx, tx, result.MediaID, result.Purpose)
		if err != nil {
			return err
		}
		if done {
			lost = true
			return nil
		}
		if err := journal.keep(ctx, tx, keys...); err != nil {
			return err
		}
		return s.d.Repository.InsertProcessingResult(ctx, tx, result)
	})
	if err != nil {
		// A commit that failed to answer may have recorded the result.
		if r, ferr := s.d.Repository.FindLatestProcessingResult(context.WithoutCancel(ctx), result.MediaID); ferr == nil && r.ID != result.ID || errors.Is(ferr, ErrNotFound) {
			journal.discard(ctx, keys...)
		}
		return queue.Retry(0, err)
	}
	if lost {
		journal.discard(ctx, keys...)
	}
	return s.deleteReceived(ctx, up)
}

// processAV remuxes a video or audio upload into result (ADR-0054 §6): the file without
// metadata as its large animated variant, and the poster, or else the cover embedded in the
// file, as its still variants and blurhash. A poster that is not an accepted image is left out.
// Content the processor rejects makes result a failure. It returns the job's result when the
// job must stop or be retried, nil when result is to be recorded.
func (s *MediaService) processAV(ctx context.Context, up *content.UploadedMedia, result *content.MediaProcessingResult, req media.Request) *queue.Result {
	retry := func(err error) *queue.Result { r := queue.Retry(0, err); return &r }
	av := s.d.AVLimits
	data, err := s.readBlob(ctx, up.BlobKey, av.MaxBytes)
	var out media.AVOutput
	if err == nil {
		out, err = s.d.Processor.Remux(ctx, data, av)
	}
	switch {
	case media.Rejected(err) || errors.Is(err, blob.ErrNotFound):
		s.d.Logger.WarnContext(ctx, "media processing failed", slog.String("media_id", up.MediaID), slog.Any("error", err))
		result.Outcome, result.MediaType = content.MediaFailed, content.MediaTypeUnknown
		return nil
	case err != nil:
		return retry(err)
	}
	result.Outcome, result.MediaType = content.MediaReady, content.MediaTypeVideo
	if out.Kind == media.AVAudio {
		result.MediaType = content.MediaTypeAudio
	}
	result.Width, result.Height, result.DurationMS = out.Width, out.Height, int(out.Duration.Milliseconds())
	poster, err := s.readBlob(ctx, posterKey(up.MediaID), s.d.Limits.MaxBytes)
	switch {
	case errors.Is(err, blob.ErrNotFound):
		poster = out.Cover
	case err != nil:
		return retry(err)
	}
	var variants []sizedVariant
	if poster != nil {
		req.Purpose = media.PurposeAttachment
		still, err := s.d.Processor.Process(ctx, poster, s.d.Limits, s.d.Sizes, req)
		switch {
		case media.Rejected(err):
			s.d.Logger.WarnContext(ctx, "media poster left out", slog.String("media_id", up.MediaID), slog.Any("error", err))
		case err != nil:
			return retry(err)
		default:
			still.Animated = nil
			result.Blurhash = still.Blurhash
			variants = stillAndAnimated(still)
		}
	}
	variants = append(variants, sizedVariant{
		size: content.SizeLarge, motion: content.MotionAnimated,
		v: media.Variant{Data: out.Data, ContentType: out.ContentType, Extension: out.Extension, Width: out.Width, Height: out.Height},
	})
	if err := s.storeVariants(ctx, result, variants); err != nil {
		return retry(err)
	}
	return nil
}

// readBlob reads the blob of key, failing past maxBytes with media.ErrTooLarge.
func (s *MediaService) readBlob(ctx context.Context, key string, maxBytes int64) ([]byte, error) {
	r, _, err := s.d.Blobs.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("content: read %s: %w", key, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w: %s", media.ErrTooLarge, key)
	}
	return data, nil
}

// sourceVariant returns the variant that processing a file again starts from: the large moving
// variant of an animation, the large still one otherwise.
func sourceVariant(r content.MediaProcessingResult) content.MediaVariant {
	if v, ok := r.Variant(content.SizeLarge, content.MotionAnimated); ok {
		return v
	}
	v, _ := r.Variant(content.SizeLarge, content.MotionStill)
	return v
}

// sizedVariant is a variant of a processing result with its size and motion.
type sizedVariant struct {
	size   content.MediaSize
	motion content.MediaMotion
	v      media.Variant
}

// stillAndAnimated returns the variants of out: the still ones, and for an animation the
// animated ones.
func stillAndAnimated(out media.Output) []sizedVariant {
	var vs []sizedVariant
	sets := []struct {
		motion content.MediaMotion
		v      *media.Variants
	}{{content.MotionStill, &out.Still}, {content.MotionAnimated, out.Animated}}
	for _, set := range sets {
		if set.v == nil {
			continue
		}
		for i, size := range content.MediaSizes {
			vs = append(vs, sizedVariant{size: size, motion: set.motion, v: set.v.Variant(media.AllSizes[i])})
		}
	}
	return vs
}

// storeVariants puts variants in blob storage under keys of result and adds them to result.
// The keys are reserved first, so that the blobs of a run that fails or loses to a concurrent
// run are found and deleted (blobJournal); recordResult keeps them. A failed write deletes what
// the run wrote.
func (s *MediaService) storeVariants(ctx context.Context, result *content.MediaProcessingResult, variants []sizedVariant) error {
	keys := make([]string, len(variants))
	for i, sv := range variants {
		keys[i] = variantKey(result.MediaID, result.ID, sv.size, sv.motion, sv.v.Extension)
	}
	journal := s.journal()
	if err := journal.reserve(ctx, keys...); err != nil {
		return err
	}
	for i, sv := range variants {
		info, err := s.d.Blobs.Put(ctx, keys[i], bytes.NewReader(sv.v.Data), blob.PutOptions{ContentType: sv.v.ContentType})
		if err != nil {
			journal.discard(ctx, keys...)
			return err
		}
		result.Variants = append(result.Variants, content.MediaVariant{
			ResultID: result.ID, Size: sv.size, Motion: sv.motion, BlobKey: keys[i], ContentType: sv.v.ContentType,
			FileSize: info.Size, Width: sv.v.Width, Height: sv.v.Height,
		})
	}
	return nil
}

// journal returns the work list of blob deletions on the service's ports.
func (s *MediaService) journal() blobJournal {
	return blobJournal{uow: s.d.UnitOfWork, repo: s.d.Repository, blobs: s.d.Blobs, clock: s.d.Clock, logger: s.d.Logger}
}

func (s *MediaService) processBlob(ctx context.Context, key string, req media.Request) (media.Output, error) {
	r, _, err := s.d.Blobs.Get(ctx, key)
	if err != nil {
		return media.Output{}, err
	}
	defer func() { _ = r.Close() }()
	data, err := io.ReadAll(io.LimitReader(r, s.d.Limits.MaxBytes+1))
	if err != nil {
		return media.Output{}, fmt.Errorf("content: read %s: %w", key, err)
	}
	return s.d.Processor.Process(ctx, data, s.d.Limits, s.d.Sizes, req)
}

// deleteReceived deletes the file as received once it has a result; a failed deletion is
// retried with the job.
func (s *MediaService) deleteReceived(ctx context.Context, up *content.UploadedMedia) queue.Result {
	if up == nil {
		return queue.Done()
	}
	for _, key := range []string{up.BlobKey, posterKey(up.MediaID)} {
		if err := s.d.Blobs.Delete(ctx, key); err != nil {
			return queue.Retry(0, err)
		}
	}
	return queue.Done()
}

func (s *MediaService) handleCleanup(ctx context.Context, _ queue.Job) queue.Result {
	if _, err := s.CleanupUnattached(ctx); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// CleanupUnattached deletes the uploads older than the unattached TTL that no status revision
// attaches and no referrer references, with their blobs, and returns how many it deleted. It
// first deletes the blobs that are due on the work list of blob deletions (blobJournal): those
// of earlier deletions whose blob deletion failed, and those that a failed upload or processing
// left. One run looks at a bounded number of uploads; the next run continues.
func (s *MediaService) CleanupUnattached(ctx context.Context) (int, error) {
	if _, err := s.journal().sweep(ctx); err != nil {
		return 0, err
	}
	before := s.d.Clock.Now().Add(-s.d.UnattachedTTL)
	deleted, after := 0, ""
	for range cleanupMaxBatches {
		ids, err := s.d.Repository.ListUnattachedUploads(ctx, after, before, cleanupBatch)
		if err != nil {
			return deleted, err
		}
		for _, id := range ids {
			ok, err := s.deleteUnattached(ctx, id)
			if err != nil {
				return deleted, err
			}
			if ok {
				deleted++
			}
		}
		if len(ids) < cleanupBatch {
			break
		}
		after = ids[len(ids)-1]
	}
	return deleted, nil
}

// deleteUnattached deletes an unattached upload. Its rows go in one transaction that records
// its blob keys on the work list of blob deletions, unless a status revision attached it since
// it was listed; then the blobs go, and a blob whose deletion fails stays on the list for the
// next run.
func (s *MediaService) deleteUnattached(ctx context.Context, id string) (bool, error) {
	for _, r := range s.d.Referrers {
		referenced, err := r.IsMediaReferenced(ctx, id)
		if err != nil || referenced {
			return false, err
		}
	}
	keys, err := mediaBlobKeys(ctx, s.d.Repository, id)
	if err != nil {
		return false, err
	}
	journal := s.journal()
	var deleted bool
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		if deleted, err = s.d.Repository.DeleteUnattachedMedia(ctx, tx, id); err != nil || !deleted {
			return err
		}
		return journal.release(ctx, tx, keys...)
	})
	if err != nil || !deleted {
		return false, err
	}
	journal.deleteNow(ctx, keys...)
	return true, nil
}

// StoredBytes returns the bytes that the media files keep in blob storage: those of the local
// uploads (their current variants and the originals waiting for processing), and those of
// the cache of remote files (ADR-0055 §3, my.media.stored_bytes).
func (s *MediaService) StoredBytes(ctx context.Context) (local, remoteCache int64, err error) {
	if local, err = s.d.Repository.LocalMediaSize(ctx); err != nil {
		return 0, 0, err
	}
	if remoteCache, err = s.d.Repository.RemoteMediaCacheSize(ctx); err != nil {
		return 0, 0, err
	}
	return local, remoteCache, nil
}
