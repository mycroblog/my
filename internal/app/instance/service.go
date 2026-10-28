// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/i18n"
)

// Deps are the ports a Service uses.
type Deps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	Clock      kernel.Clock
	IDs        kernel.IDGenerator
	// Media checks the media files that media settings (preview_image) refer to; nil checks
	// only the shape of their IDs.
	Media MediaFiles
	// Accounts checks the local account that the contact_account setting names; nil checks
	// only the shape of its username.
	Accounts LocalAccounts
	// Documents renders the document texts (the terms of service and the privacy policy) to
	// HTML; nil publishes no documents.
	Documents DocumentRenderer
	// ClosureCounts counts what the closure of the service ends, for its checks; nil counts 0.
	ClosureCounts ClosureCounts
	// ClosureDeliveries reads the progress of the closure's Deletes; nil reads none.
	ClosureDeliveries ClosureDeliveries
	// Blobs stores the files of the brand images (ADR-0116); nil saves none and serves none.
	Blobs blob.Store
}

// DocumentRenderer renders the Markdown of a document text to an HTML fragment that is safe to
// insert into a page as it is. The adapter lives in internal/adapter/markdown and passes
// testkit.DocumentRendererContract.
type DocumentRenderer interface {
	// RenderDocument returns the HTML of the Markdown src, reduced to the document allowlist:
	// raw HTML is dropped, links keep only http, https, and mailto URLs, and the headings start
	// at <h2>, below the page's own <h1>.
	RenderDocument(src string) string
}

// MediaFiles checks media files for the media settings. The content module implements it.
type MediaFiles interface {
	// CheckImage fails when mediaID names no media file that is, or is being processed into,
	// a still image.
	CheckImage(ctx context.Context, mediaID string) error
}

// LocalAccounts checks the local accounts that account settings (contact_account) name. The
// account module implements it.
type LocalAccounts interface {
	// CheckLocalAccount fails when username names no local account.
	CheckLocalAccount(ctx context.Context, username string) error
}

// Service runs the instance module's use cases for settings and texts.
//
// Settings are the instance policies that administrators change at run time; they live in the
// database, versioned. Deployment configuration (adapters, host, secrets) lives in the
// environment and the config file (internal/platform/config, ADR-0010). The two never share a
// key, so neither overrides the other (docs/reference/configuration.md).
type Service struct {
	d Deps
}

// NewService returns a Service on d.
func NewService(d Deps) *Service { return &Service{d: d} }

// Settings returns the current value of every setting, defaults included.
func (s *Service) Settings(ctx context.Context) (instance.Settings, error) {
	cur, err := s.d.Repository.ListCurrentSettings(ctx)
	if err != nil {
		return instance.Settings{}, err
	}
	return instance.NewSettings(cur), nil
}

// DefaultLocale returns the current value of the default_locale setting.
func (s *Service) DefaultLocale(ctx context.Context) (string, error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return "", err
	}
	return settings.DefaultLocale(), nil
}

// RegistrationMode returns the registration mode that sign-ups follow: the value of the
// registration_mode setting, or closed from the announcement of the service closure on
// (instance.EffectiveRegistrationMode). The setting keeps its value.
func (s *Service) RegistrationMode(ctx context.Context) (instance.RegistrationMode, error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return "", err
	}
	c, err := s.Closure(ctx)
	if err != nil {
		return "", err
	}
	return instance.EffectiveRegistrationMode(settings.RegistrationMode(), c.CurrentState()), nil
}

// MaxStatusLength returns the current value of the max_status_length setting (FR-101).
func (s *Service) MaxStatusLength(ctx context.Context) (int, error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return 0, err
	}
	return settings.MaxStatusLength(), nil
}

// RequireMediaDescription returns the current value of the require_media_description setting
// (FR-105).
func (s *Service) RequireMediaDescription(ctx context.Context) (bool, error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return false, err
	}
	return settings.RequireMediaDescription(), nil
}

// AuthorizedFetch returns the current value of the authorized_fetch setting (FR-408).
func (s *Service) AuthorizedFetch(ctx context.Context) (bool, error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return false, err
	}
	return settings.AuthorizedFetch(), nil
}

// FederationMode returns the current value of the federation_mode setting (FR-413, ADR-0076).
// It is read per request and per job, so that a change applies at once.
func (s *Service) FederationMode(ctx context.Context) (instance.FederationMode, error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return "", err
	}
	return settings.FederationMode(), nil
}

// SetSettingInput describes a change of a setting.
type SetSettingInput struct {
	Key   instance.SettingKey
	Value string
	// RevisedByUserID is the user making the change, or "" for the CLI.
	RevisedByUserID string
}

// SetSetting validates the value against the key's definition and records it as a new
// revision, which becomes the current value. A locale value is canonicalized first
// ("ja_jp" becomes "ja-JP"). It fails with an error wrapping instance.ErrUnknownKey or
// instance.ErrInvalidValue for bad input. Setting the current value again still records a
// revision. The change is audited in the same transaction.
func (s *Service) SetSetting(ctx context.Context, in SetSettingInput) (instance.SettingRevision, error) {
	revs, err := s.SetSettings(ctx, []SetSettingInput{in})
	if err != nil {
		return instance.SettingRevision{}, err
	}
	return revs[0], nil
}

// SettingError reports the input of SetSettings that its key rejects. It wraps an error
// wrapping instance.ErrUnknownKey or instance.ErrInvalidValue.
type SettingError struct {
	Key instance.SettingKey
	Err error
}

func (e *SettingError) Error() string { return e.Err.Error() }

func (e *SettingError) Unwrap() error { return e.Err }

// SetSettings changes several settings at once, each as SetSetting changes one. It validates
// every value first and records nothing when one is rejected (a *SettingError naming the first
// such key); otherwise it records the revisions and their audit entries in one transaction, in
// the order of ins, so that a form of several settings is saved whole or not at all.
func (s *Service) SetSettings(ctx context.Context, ins []SetSettingInput) ([]instance.SettingRevision, error) {
	now := s.d.Clock.Now()
	revs := make([]instance.SettingRevision, 0, len(ins))
	audits := make([]appmoderation.AuditRecorded, 0, len(ins))
	for _, in := range ins {
		value, err := s.parseSetting(ctx, in)
		if err != nil {
			return nil, &SettingError{Key: in.Key, Err: err}
		}
		rev := instance.SettingRevision{ID: s.d.IDs.NewID(), Key: in.Key, Value: value, RevisedByUserID: in.RevisedByUserID, RevisedAt: now}
		revs = append(revs, rev)
		audits = append(audits, s.audit(appmoderation.AuditSettingChanged, appmoderation.AuditTargetSetting, string(in.Key), string(in.Key), rev.ID, in.RevisedByUserID, now))
	}
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		for i, rev := range revs {
			if err := s.d.Repository.InsertSettingRevision(ctx, tx, rev); err != nil {
				return err
			}
			if err := tx.Emit(ctx, SettingRevised{Revision: rev}); err != nil {
				return err
			}
			if err := tx.Emit(ctx, audits[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return revs, nil
}

// parseSetting returns the canonical value of in (instance.Definition.Parse), with a locale
// canonicalized first and a media file checked. It fails with an error wrapping
// instance.ErrUnknownKey or instance.ErrInvalidValue.
func (s *Service) parseSetting(ctx context.Context, in SetSettingInput) (string, error) {
	def, err := instance.Lookup(in.Key)
	if err != nil {
		return "", err
	}
	raw := in.Value
	if def.Kind == instance.KindLocale {
		raw = i18n.Canonicalize(raw)
	}
	value, err := def.Parse(raw)
	if err != nil {
		return "", err
	}
	if def.Kind == instance.KindMedia && value != "" && s.d.Media != nil {
		if err := s.d.Media.CheckImage(ctx, value); err != nil {
			return "", fmt.Errorf("setting %s: media %s: %w: %w", in.Key, value, instance.ErrInvalidValue, err)
		}
	}
	if def.Kind == instance.KindAccount && value != "" && s.d.Accounts != nil {
		if err := s.d.Accounts.CheckLocalAccount(ctx, value); err != nil {
			return "", fmt.Errorf("setting %s: account %s: %w: %w", in.Key, value, instance.ErrInvalidValue, err)
		}
	}
	return value, nil
}

// IsMediaReferenced reports whether any revision of a media setting, current or not, refers
// to a media file. It implements content.MediaReferrer, so that the cleanup of unattached
// uploads keeps the instance's images and a setting can go back to an earlier revision.
func (s *Service) IsMediaReferenced(ctx context.Context, mediaID string) (bool, error) {
	if mediaID == "" {
		return false, nil // the cleared value
	}
	for _, key := range instance.MediaKeys() {
		used, err := s.d.Repository.IsSettingValueUsed(ctx, key, mediaID)
		if err != nil || used {
			return used, err
		}
	}
	return false, nil
}

// SettingHistory returns the revisions of a setting, newest first, at most limit of them.
func (s *Service) SettingHistory(ctx context.Context, key instance.SettingKey, limit int) ([]instance.SettingRevision, error) {
	if _, err := instance.Lookup(key); err != nil {
		return nil, err
	}
	if err := checkLimit(limit); err != nil {
		return nil, err
	}
	return s.d.Repository.ListSettingHistory(ctx, key, limit)
}

// SetTextInput describes a change of a text.
type SetTextInput struct {
	Key instance.TextKey
	// Body is the new text; "" clears it.
	Body string
	// RevisedByUserID is the user making the change, or "" for the CLI.
	RevisedByUserID string
}

// SetText records a new revision of a text, which becomes its current text in every locale.
// The change is audited in the same transaction. It fails with an error wrapping
// instance.ErrUnknownKey or instance.ErrInvalidValue for bad input.
func (s *Service) SetText(ctx context.Context, in SetTextInput) (instance.TextRevision, error) {
	def, err := instance.LookupText(in.Key)
	if err != nil {
		return instance.TextRevision{}, err
	}
	if err := def.Validate(in.Body); err != nil {
		return instance.TextRevision{}, err
	}
	rev := instance.TextRevision{
		ID: s.d.IDs.NewID(), Key: in.Key, Body: in.Body,
		RevisedByUserID: in.RevisedByUserID, RevisedAt: s.d.Clock.Now(),
	}
	audit := s.audit(appmoderation.AuditTextChanged, appmoderation.AuditTargetSetting, string(in.Key),
		string(in.Key), rev.ID, in.RevisedByUserID, rev.RevisedAt)
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.InsertTextRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := tx.Emit(ctx, TextRevised{Revision: rev}); err != nil {
			return err
		}
		return tx.Emit(ctx, audit)
	})
	if err != nil {
		return instance.TextRevision{}, err
	}
	return rev, nil
}

// Text returns the current text of key, the same for readers of every locale. It reports
// false when the key has no text or its text is cleared.
func (s *Service) Text(ctx context.Context, key instance.TextKey) (instance.Text, bool, error) {
	if _, err := instance.LookupText(key); err != nil {
		return instance.Text{}, false, err
	}
	t, err := s.d.Repository.FindCurrentText(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return instance.Text{}, false, nil
	}
	if err != nil {
		return instance.Text{}, false, err
	}
	return t, t.Body != "", nil
}

// Document is a document text rendered for readers.
type Document struct {
	Key instance.TextKey
	// HTML is the sanitized HTML of the text (DocumentRenderer).
	HTML      string
	RevisedAt time.Time
}

// Document returns the document text key (instance.TextDefinition.Document) rendered. It
// reports false when the key has no text, and always without Deps.Documents. A key that is
// not a document is an error wrapping instance.ErrUnknownKey.
func (s *Service) Document(ctx context.Context, key instance.TextKey) (Document, bool, error) {
	if err := checkDocument(key); err != nil || s.d.Documents == nil {
		return Document{}, false, err
	}
	t, ok, err := s.Text(ctx, key)
	if err != nil || !ok {
		return Document{}, false, err
	}
	return Document{Key: key, HTML: s.d.Documents.RenderDocument(t.Body), RevisedAt: t.RevisedAt}, true, nil
}

// HasDocument reports whether the document text key is published: what Document reports,
// without rendering it.
func (s *Service) HasDocument(ctx context.Context, key instance.TextKey) (bool, error) {
	if err := checkDocument(key); err != nil || s.d.Documents == nil {
		return false, err
	}
	_, ok, err := s.Text(ctx, key)
	return ok, err
}

func checkDocument(key instance.TextKey) error {
	def, err := instance.LookupText(key)
	if err != nil {
		return err
	}
	if !def.Document {
		return fmt.Errorf("text %q is not a document: %w", key, instance.ErrUnknownKey)
	}
	return nil
}

// Image is one of the instance's images, a media setting with its alt text.
type Image struct {
	// MediaID is the value of the setting, "" when unset.
	MediaID string
	// Alt is the alt text, "" when there is none.
	Alt string
}

// PreviewImage returns the instance's link preview image (ADR-0028): the preview_image setting
// with its alt text, the preview_image_alt text.
func (s *Service) PreviewImage(ctx context.Context) (Image, error) {
	return s.image(ctx, instance.SettingPreviewImage, instance.TextPreviewImageAlt)
}

// Thumbnail returns the image that represents the instance: the thumbnail setting with its alt
// text, the thumbnail_alt text.
func (s *Service) Thumbnail(ctx context.Context) (Image, error) {
	return s.image(ctx, instance.SettingThumbnail, instance.TextThumbnailAlt)
}

func (s *Service) image(ctx context.Context, key instance.SettingKey, alt instance.TextKey) (Image, error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return Image{}, err
	}
	out := Image{}
	out.MediaID, _ = settings.Value(key)
	if out.MediaID == "" {
		return out, nil
	}
	t, ok, err := s.Text(ctx, alt)
	if err != nil {
		return Image{}, err
	}
	if ok {
		out.Alt = t.Body
	}
	return out, nil
}

// Texts returns every current, non-empty text, by key.
func (s *Service) Texts(ctx context.Context) ([]instance.Text, error) {
	all, err := s.d.Repository.ListAllCurrentTexts(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(all, func(t instance.Text) bool { return t.Body == "" }), nil
}

// TextHistory returns the revisions of a text, newest first, at most limit of them.
func (s *Service) TextHistory(ctx context.Context, key instance.TextKey, limit int) ([]instance.TextRevision, error) {
	if _, err := instance.LookupText(key); err != nil {
		return nil, err
	}
	if err := checkLimit(limit); err != nil {
		return nil, err
	}
	return s.d.Repository.ListTextHistory(ctx, key, limit)
}

// MaxHistoryLimit bounds the limit of SettingHistory and TextHistory.
const MaxHistoryLimit = 1000

func checkLimit(limit int) error {
	if limit < 1 || limit > MaxHistoryLimit {
		return fmt.Errorf("history limit %d is outside 1..%d: %w", limit, MaxHistoryLimit, instance.ErrInvalidValue)
	}
	return nil
}

// audit returns the audit log entry of one of this module's operations (ADR-0037), with a new
// ID, performed by byUserID ("" for the CLI). The operation emits it in its transaction.
func (s *Service) audit(action appmoderation.AuditAction, kind appmoderation.AuditTargetKind, targetID, label, sourceID, byUserID string, at time.Time) appmoderation.AuditRecorded {
	actor, user := appmoderation.AuditActor(byUserID)
	return appmoderation.AuditRecorded{Entry: appmoderation.AuditEntry{
		ID: s.d.IDs.NewID(), ActorKind: actor, ActorUserID: user, Action: action, TargetKind: kind,
		TargetID: targetID, TargetLabel: label, SourceEventID: sourceID, OccurredAt: at,
	}}
}
