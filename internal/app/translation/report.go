// SPDX-License-Identifier: AGPL-3.0-only

package translation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	appcontent "github.com/mycroblog/my/internal/app/content"
)

// Reports is what the service reads of the moderation module, for the moderators who translate a
// report (ADR-0148 §13). *moderation.Service implements it; its callers serve the holders of the
// manage_reports permission only.
type Reports interface {
	// ReportedStatus returns a status that the report points at, whatever its visibility and
	// also when it is deleted. An unknown report, a status that it does not point at, and a status
	// that is gone fail with an error wrapping the moderation module's ErrNotFound.
	ReportedStatus(ctx context.Context, reportID, statusID string) (appcontent.ReviewedStatus, error)
	// ReportComment returns the comment of the report, empty when it has none.
	ReportComment(ctx context.Context, reportID string) (string, error)
}

// ErrNoReports reports a service built without Reports.
var ErrNoReports = errors.New("translation: reports are not translated here")

// TranslateReportedStatus returns a status that a report points at in the target language, for
// its moderators (ADR-0148 §13). Their access reads the status past content.CanView, but lifts no
// rule of what a provider may receive: a provider still receives only the visibilities that it is
// allowed, and only while translation_reports_to_server is on. The characters count against the
// server's limit only. The translation shares the cache of the readers' translations, except for a
// deleted status, whose translation is not kept.
func (s *Service) TranslateReportedStatus(ctx context.Context, reportID, statusID, target string) (Translation, error) {
	target, err := targetLanguage(target)
	if err != nil {
		return Translation{}, err
	}
	if s.d.Reports == nil {
		return Translation{}, ErrNoReports
	}
	st, err := s.d.Reports.ReportedStatus(ctx, reportID, statusID)
	if err != nil {
		return Translation{}, err
	}
	post := appcontent.ReadableStatus{Status: st.Status, Revision: st.Revision, EffectiveVisibility: st.EffectiveVisibility}
	return s.translate(ctx, post, target, requester{report: true, deleted: !st.DeletedAt.IsZero()})
}

// CommentTranslation is the comment of a report in the target language. Provider is nil when
// nothing was sent: the comment has no words to translate.
type CommentTranslation struct {
	Provider       *ProviderInfo
	TargetLanguage string
	SourceLanguage string
	// Text is plain text.
	Text string
}

// TranslateReportComment returns the comment of a report in the target language, for its
// moderators (ADR-0148 §13). Only the reporter and the moderators read a comment, so it goes only
// to a provider allowed private posts, and only while translation_reports_to_server is on. Its
// translation is never kept, and its characters count against the server's limit only.
func (s *Service) TranslateReportComment(ctx context.Context, reportID, target string) (CommentTranslation, error) {
	target, err := targetLanguage(target)
	if err != nil {
		return CommentTranslation{}, err
	}
	if s.d.Reports == nil {
		return CommentTranslation{}, ErrNoReports
	}
	comment, err := s.d.Reports.ReportComment(ctx, reportID)
	if err != nil {
		return CommentTranslation{}, err
	}
	text := maskPlain(comment)
	if !text.sendable() {
		s.count(ctx, outcomeAuthor)
		return CommentTranslation{TargetLanguage: target, SourceLanguage: UndeterminedLanguage, Text: comment}, nil
	}
	candidates, err := s.candidates(ctx, appcontent.VisibilityPrivate, true)
	if err != nil {
		return CommentTranslation{}, err
	}
	if len(candidates) == 0 {
		s.count(ctx, outcomeNoProvider)
		return CommentTranslation{}, fmt.Errorf("report %s: %w", reportID, ErrNoProvider)
	}
	req := Request{Fragments: []string{text.fragment}, SourceLanguage: UndeterminedLanguage, TargetLanguage: target}
	var source string
	p, err := s.ask(ctx, "", req, candidates, func(res Result) error {
		if len(res.Translations) != 1 {
			return fmt.Errorf("translation: %d translations of 1 fragment", len(res.Translations))
		}
		if !MarkersIntact(text.fragment, res.Translations[0]) {
			return errors.New("translation: the markers did not come back intact")
		}
		if strings.TrimSpace(res.Translations[0]) == "" {
			return errors.New("translation: the translation is empty")
		}
		text.translated, source = res.Translations[0], res.DetectedLanguage
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrLimitReached):
			s.count(ctx, outcomeLimit)
		case errors.Is(err, ErrUnsupportedLanguage):
			s.count(ctx, outcomeUnsupported)
		case errors.Is(err, ErrFailed):
			s.count(ctx, outcomeFailed)
		}
		return CommentTranslation{}, err
	}
	s.count(ctx, outcomeProvider)
	if source == "" {
		source = UndeterminedLanguage
	}
	return CommentTranslation{
		Provider:       new(p.Info()),
		TargetLanguage: target,
		SourceLanguage: source,
		Text:           s.d.Sanitizer.PlainText(text.restore(func(s string) string { return s })),
	}, nil
}
