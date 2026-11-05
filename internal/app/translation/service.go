// SPDX-License-Identifier: AGPL-3.0-only

package translation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/i18n"
)

var (
	// ErrInvalidLanguage reports a target language that is not a well-formed BCP 47 tag of a
	// language.
	ErrInvalidLanguage = errors.New("translation: the target language is not valid")
	// ErrNoProvider reports that no enabled provider may receive this post (ADR-0148 §5, §8).
	ErrNoProvider = errors.New("translation: no provider for this post")
	// ErrLimitReached reports that a monthly limit stops the request (ADR-0148 §11).
	ErrLimitReached = errors.New("translation: a monthly limit is reached")
	// ErrFailed reports that no provider returned a usable translation: they failed, or their
	// output did not bring back the markers. Asking again may work.
	ErrFailed = errors.New("translation: no provider returned a translation")
)

const (
	// JobPurgeCache is the type of the periodic job that removes old translations (ADR-0148 §9).
	JobPurgeCache = "translation.purge_cache"
	// PurgeCacheInterval is how often it runs.
	PurgeCacheInterval = 24 * time.Hour
	// DefaultRetentionDays is how long a translation is kept when no setting says otherwise.
	DefaultRetentionDays = 30
	// purgeBatch is how many translations the job removes in one transaction.
	purgeBatch = 100

	// lockNamespace is the namespace of the locks of KeyValueStore, keyed by revision and target
	// language: one request at a time calls a provider for one translation (ADR-0148 §8).
	lockNamespace = "translation.lock"
	// lockTTL outlives a request that dies while it holds the lock; the unique constraint keeps
	// the row single if a slow request outlives the lock.
	lockTTL = 2 * time.Minute
	// lockWait is how long a request waits for another one's translation.
	lockWait = 45 * time.Second
	// lockPoll is how often a waiting request looks for that translation.
	lockPoll = 100 * time.Millisecond
	// providerTimeout bounds one call of a provider.
	providerTimeout = 20 * time.Second
)

// Requests counts the translation requests that passed the view check, by outcome.
var Requests = kernel.Instrument{
	Name: "my.translation.requests", Unit: "{request}", Description: "Requests to translate a post that the viewer may see, by outcome.",
	LabelKeys: []string{labelOutcome},
}

const labelOutcome = "my.outcome"

// The outcomes of Requests.
const (
	outcomeAuthor      = "author"
	outcomeCache       = "cache"
	outcomeProvider    = "provider"
	outcomeNoProvider  = "no_provider"
	outcomeLimit       = "limit"
	outcomeUnsupported = "unsupported"
	outcomeFailed      = "failed"
)

// Provider is a server-side provider as the operator enabled it. ID, Name, URL, and LogoURL are
// its registration, which clients show (ADR-0148 §15); the port does not know them.
type Provider struct {
	ID      string
	Name    string
	URL     string
	LogoURL string
	// Translator is the adapter.
	Translator Translator
	// Visibilities are the visibilities that the operator allows it, from
	// translation_provider_visibilities. Nil means the default of ADR-0148 §5; empty allows none.
	Visibilities []appcontent.Visibility
}

// Allows reports whether the provider may receive a post judged by the visibility.
func (p Provider) Allows(v appcontent.Visibility) bool {
	if p.Visibilities != nil {
		return slices.Contains(p.Visibilities, v)
	}
	return v == appcontent.VisibilityPublic || v == appcontent.VisibilityLocal || v == appcontent.VisibilityUnlisted
}

// Providers tells which providers are enabled, in the operator's order of priority. It is read
// per request, so that a change takes effect at once.
type Providers interface {
	Providers(ctx context.Context) ([]Provider, error)
}

// NoProviders is the Providers of a server with none enabled, which is the default (ADR-0148 §3).
type NoProviders struct{}

// Providers implements Providers.
func (NoProviders) Providers(context.Context) ([]Provider, error) { return nil, nil }

// Limiter holds the monthly limits of ADR-0148 §11. Only a request that goes to a provider reaches
// it: cache hits and the author's own language do not count.
type Limiter interface {
	// Reserve counts characters against the limits of the account and of the server, or fails
	// with ErrLimitReached and counts nothing.
	Reserve(ctx context.Context, accountID string, characters int) error
	// Release gives back characters that no provider received.
	Release(ctx context.Context, accountID string, characters int) error
}

// ClosedLimiter is the Limiter of limits of 0: it sends nothing to a provider.
type ClosedLimiter struct{}

// Reserve implements Limiter.
func (ClosedLimiter) Reserve(context.Context, string, int) error { return ErrLimitReached }

// Release implements Limiter.
func (ClosedLimiter) Release(context.Context, string, int) error { return nil }

// Statuses is what the service reads of the content module, implemented by content.Reader.
type Statuses interface {
	// Readable returns the current revision of a status that viewer may see, by content.CanView,
	// and fails with content.ErrNotFound otherwise.
	Readable(ctx context.Context, viewer appcontent.Viewer, statusID string) (appcontent.ReadableStatus, error)
	// HoldLive fails with content.ErrNotFound when the status is deleted, and keeps a deletion
	// from passing before tx ends.
	HoldLive(ctx context.Context, tx uow.Tx, statusID string) error
}

// Deps are the ports of a Service.
type Deps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	Statuses   Statuses
	Providers  Providers
	Limiter    Limiter
	Sanitizer  kernel.HTMLSanitizer
	KV         kernel.KeyValueStore
	Clock      kernel.Clock
	IDs        kernel.IDGenerator
	// RetentionDays returns translation_retention_days; nil means DefaultRetentionDays.
	RetentionDays func(ctx context.Context) (int, error)
	// Meter records Requests; nil records nothing.
	Meter kernel.Meter
	// Logger reports provider failures, which never hold the text of a post; nil logs nothing.
	Logger *slog.Logger
}

// Service translates posts on a reader's request (ADR-0148 §8).
type Service struct {
	d        Deps
	requests kernel.Counter
}

// NewService returns a Service on d.
func NewService(d Deps) *Service {
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d, requests: kernel.MeterOrNop(d.Meter).Counter(Requests)}
}

// ProviderInfo is what a client shows of the provider of a translation.
type ProviderInfo struct {
	ID, Name, URL, LogoURL string
}

// AttachmentText is the alt text of an attachment in the target language.
type AttachmentText struct {
	MediaID     string
	Description string
}

// Translation is the answer to a request. Provider is nil when these are the author's own words:
// a body that the post already has in the language, or a post with no words to translate.
type Translation struct {
	Provider       *ProviderInfo
	TargetLanguage string
	SourceLanguage string
	ContentHTML    string
	SpoilerText    string
	// Attachments are those with alt text, in order.
	Attachments []AttachmentText
	// PollOptions holds the title of every option of the poll, in order.
	PollOptions []string
}

// Translate returns the post in the target language for viewer, who is a local account. A post
// that the viewer may not see fails with content.ErrNotFound, whatever is cached: every path of
// this method starts from the view check.
func (s *Service) Translate(ctx context.Context, viewer appcontent.Viewer, statusID, target string) (Translation, error) {
	target = i18n.Canonicalize(target)
	if target == "" || target == UndeterminedLanguage {
		return Translation{}, ErrInvalidLanguage
	}
	post, err := s.d.Statuses.Readable(ctx, viewer, statusID)
	if err != nil {
		return Translation{}, err
	}
	rev := post.Revision
	if body, ok := bodyInLanguage(rev.Bodies, target); ok {
		s.count(ctx, outcomeAuthor)
		return authorWords(post, body.ContentHTML, target, body.Language), nil
	}
	candidates, err := s.candidates(ctx, post.EffectiveVisibility)
	if err != nil {
		return Translation{}, err
	}
	if len(candidates) == 0 {
		s.count(ctx, outcomeNoProvider)
		return Translation{}, fmt.Errorf("status %s: %w", statusID, ErrNoProvider)
	}
	if hit, ok, err := s.cached(ctx, post, target, candidates); err != nil {
		return Translation{}, err
	} else if ok {
		s.count(ctx, outcomeCache)
		return hit, nil
	}
	d, err := newDraft(post)
	if err != nil {
		return Translation{}, err
	}
	if len(d.sent) == 0 {
		s.count(ctx, outcomeAuthor)
		return authorWords(post, d.bodyHTML, target, d.sourceLanguage), nil
	}
	out, err := s.request(ctx, viewer.AccountID, post, d, target, candidates)
	switch {
	case err == nil:
	case errors.Is(err, ErrLimitReached):
		s.count(ctx, outcomeLimit)
	case errors.Is(err, ErrUnsupportedLanguage):
		s.count(ctx, outcomeUnsupported)
	case errors.Is(err, ErrFailed):
		s.count(ctx, outcomeFailed)
	}
	return out, err
}

// request is what is left when nothing is cached: the limit, the lock, the provider, and the row.
func (s *Service) request(ctx context.Context, accountID string, post appcontent.ReadableStatus, d *draft, target string, candidates []Provider) (Translation, error) {
	characters := CountCharacters(d.fragments())
	if err := s.d.Limiter.Reserve(ctx, accountID, characters); err != nil {
		return Translation{}, err
	}
	// The characters stay counted once a provider has answered, whatever the answer was worth: it
	// has received them.
	received := false
	defer func() {
		if received {
			return
		}
		if err := s.d.Limiter.Release(context.WithoutCancel(ctx), accountID, characters); err != nil {
			s.d.Logger.WarnContext(ctx, "translation usage not released", slog.Any("error", err))
		}
	}()

	key := post.Revision.ID + "|" + target
	token := []byte(s.d.IDs.NewID())
	hit, ok, err := s.lock(ctx, key, token, post, target, candidates)
	if err != nil {
		return Translation{}, err
	}
	if ok {
		s.count(ctx, outcomeCache)
		return hit, nil
	}
	defer s.unlock(ctx, key, token)

	provider, stored, answered, err := s.callProviders(ctx, post, d, target, candidates)
	received = answered
	if err != nil {
		return Translation{}, err
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Statuses.HoldLive(ctx, tx, post.Status.ID); err != nil {
			return err
		}
		_, err := s.d.Repository.InsertTranslation(ctx, tx, stored)
		return err
	})
	if err != nil {
		return Translation{}, err
	}
	s.count(ctx, outcomeProvider)
	return present(post, stored, provider), nil
}

// lock takes the lock of the translation. While another request holds it, lock waits for that
// request's row and returns it; a request that ends without one frees the lock for this one.
func (s *Service) lock(ctx context.Context, key string, token []byte, post appcontent.ReadableStatus, target string, candidates []Provider) (Translation, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, lockWait)
	defer cancel()
	for {
		got, err := s.d.KV.CompareAndSwap(ctx, lockNamespace, key, nil, token, lockTTL)
		if err != nil {
			return Translation{}, false, fmt.Errorf("translation: take the lock: %w", err)
		}
		// Also after taking the lock: the last holder may have stored the row and freed the lock
		// since the first look at the cache.
		hit, ok, err := s.cached(ctx, post, target, candidates)
		if err != nil {
			if got {
				s.unlock(ctx, key, token)
			}
			return Translation{}, false, err
		}
		if ok {
			if got {
				s.unlock(ctx, key, token)
			}
			return hit, true, nil
		}
		if got {
			return Translation{}, false, nil
		}
		select {
		case <-ctx.Done():
			return Translation{}, false, fmt.Errorf("translation: wait for the lock: %w: %w", ErrFailed, ctx.Err())
		case <-time.After(lockPoll):
		}
	}
}

func (s *Service) unlock(ctx context.Context, key string, token []byte) {
	if _, err := s.d.KV.CompareAndSwap(context.WithoutCancel(ctx), lockNamespace, key, token, nil, 0); err != nil {
		s.d.Logger.WarnContext(ctx, "translation lock not released", slog.Any("error", err))
	}
}

// callProviders asks the candidates in order until one returns a usable translation, and reports
// whether any provider answered. It runs outside transactions (ADR-0003).
func (s *Service) callProviders(ctx context.Context, post appcontent.ReadableStatus, d *draft, target string, candidates []Provider) (Provider, Stored, bool, error) {
	req := Request{Fragments: d.fragments(), SourceLanguage: d.sourceLanguage, TargetLanguage: target}
	answered, onlyUnsupported := false, true
	for _, p := range candidates {
		callCtx, cancel := context.WithTimeout(ctx, providerTimeout)
		res, err := p.Translator.Translate(callCtx, req)
		cancel()
		if err != nil {
			onlyUnsupported = onlyUnsupported && errors.Is(err, ErrUnsupportedLanguage)
			s.d.Logger.WarnContext(ctx, "translation provider failed", slog.String("provider", p.ID), slog.Any("error", err))
			continue
		}
		answered, onlyUnsupported = true, false
		stored, err := d.apply(res, s.d.Sanitizer)
		if err != nil {
			s.d.Logger.WarnContext(ctx, "translation output refused", slog.String("provider", p.ID), slog.Any("error", err))
			continue
		}
		stored.ID, stored.StatusID, stored.RevisionID = s.d.IDs.NewID(), post.Status.ID, post.Revision.ID
		stored.TargetLanguage, stored.Provider, stored.CreatedAt = target, p.ID, s.d.Clock.Now()
		return p, stored, true, nil
	}
	if onlyUnsupported {
		return Provider{}, Stored{}, false, fmt.Errorf("%w: %s", ErrUnsupportedLanguage, target)
	}
	return Provider{}, Stored{}, answered, ErrFailed
}

// candidates returns the enabled providers allowed the visibility, by priority.
func (s *Service) candidates(ctx context.Context, v appcontent.Visibility) ([]Provider, error) {
	all, err := s.d.Providers.Providers(ctx)
	if err != nil {
		return nil, err
	}
	var out []Provider
	for _, p := range all {
		if p.Allows(v) {
			out = append(out, p)
		}
	}
	return out, nil
}

// cached returns the stored translation of the first candidate that has one. The translation of a
// provider that is not a candidate any more stays stored until it expires, and is not returned.
func (s *Service) cached(ctx context.Context, post appcontent.ReadableStatus, target string, candidates []Provider) (Translation, bool, error) {
	rows, err := s.d.Repository.FindTranslations(ctx, post.Revision.ID, target)
	if err != nil {
		return Translation{}, false, err
	}
	for _, p := range candidates {
		for _, row := range rows {
			if row.Provider == p.ID {
				return present(post, row, p), true, nil
			}
		}
	}
	return Translation{}, false, nil
}

func (s *Service) count(ctx context.Context, outcome string) {
	s.requests.Add(ctx, 1, kernel.NewLabels(labelOutcome, outcome))
}

// present returns a row as the answer of its provider.
func present(post appcontent.ReadableStatus, row Stored, p Provider) Translation {
	out := Translation{
		Provider:       &ProviderInfo{ID: p.ID, Name: p.Name, URL: p.URL, LogoURL: p.LogoURL},
		TargetLanguage: row.TargetLanguage,
		SourceLanguage: row.SourceLanguage,
		ContentHTML:    row.ContentHTML,
		SpoilerText:    row.SpoilerText,
	}
	for _, a := range row.Attachments {
		for _, at := range post.Revision.Attachments {
			if at.Position == a.Position {
				out.Attachments = append(out.Attachments, AttachmentText{MediaID: at.MediaID, Description: a.Description})
			}
		}
	}
	for _, o := range row.PollOptions {
		out.PollOptions = append(out.PollOptions, o.Title)
	}
	return out
}

// authorWords returns the post as the author wrote it, with the body given.
func authorWords(post appcontent.ReadableStatus, bodyHTML, target, source string) Translation {
	rev := post.Revision
	out := Translation{TargetLanguage: target, SourceLanguage: source, ContentHTML: bodyHTML, SpoilerText: rev.SpoilerText}
	for _, a := range rev.Attachments {
		if a.Description != "" {
			out.Attachments = append(out.Attachments, AttachmentText{MediaID: a.MediaID, Description: a.Description})
		}
	}
	if rev.Poll != nil {
		for _, o := range rev.Poll.Options {
			out.PollOptions = append(out.PollOptions, o.Title)
		}
	}
	return out
}

// bodyInLanguage returns the body in the target language: the same tag, or, when neither tag has a
// script subtag, the same primary language, so that zh-Hant and zh-Hans never mix (ADR-0148 §8).
func bodyInLanguage(bodies []appcontent.Body, target string) (appcontent.Body, bool) {
	for _, b := range bodies {
		if strings.EqualFold(b.Language, target) {
			return b, true
		}
	}
	if hasScript(target) {
		return appcontent.Body{}, false
	}
	primary, _, _ := strings.Cut(target, "-")
	for _, b := range bodies {
		p, _, _ := strings.Cut(b.Language, "-")
		if strings.EqualFold(p, primary) && !hasScript(b.Language) {
			return b, true
		}
	}
	return appcontent.Body{}, false
}

// hasScript reports whether a BCP 47 tag has a script subtag: four letters after the language.
func hasScript(tag string) bool {
	parts := strings.Split(tag, "-")
	return len(parts) > 1 && len(parts[1]) == 4 && strings.IndexFunc(parts[1], func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z')
	}) < 0
}

// draft is a post taken apart for a provider: one Request with every text that it holds.
type draft struct {
	sourceLanguage string
	bodyHTML       string // the body as it is
	body           *maskedBody
	spoiler        *maskedText
	attachments    []positioned
	options        []positioned
	// sent holds the fragments with words, in the order of the Request.
	sent []*maskedText
}

type positioned struct {
	position int
	text     *maskedText
}

// newDraft takes apart the current revision. The body is the one that clients show of a revision,
// its first (ordered by language).
func newDraft(post appcontent.ReadableStatus) (*draft, error) {
	rev := post.Revision
	d := &draft{sourceLanguage: rev.Language, spoiler: maskPlain(rev.SpoilerText)}
	if len(rev.Bodies) > 0 {
		d.bodyHTML = rev.Bodies[0].ContentHTML
		d.sourceLanguage = rev.Bodies[0].Language
	}
	if d.sourceLanguage == "" {
		d.sourceLanguage = UndeterminedLanguage
	}
	body, err := parseBody(d.bodyHTML)
	if err != nil {
		return nil, err
	}
	d.body = body
	all := append([]*maskedText{}, body.fragments()...)
	all = append(all, d.spoiler)
	for _, a := range rev.Attachments {
		if a.Description == "" {
			continue
		}
		t := maskPlain(a.Description)
		d.attachments = append(d.attachments, positioned{a.Position, t})
		all = append(all, t)
	}
	if rev.Poll != nil {
		for _, o := range rev.Poll.Options {
			t := maskPlain(o.Title)
			d.options = append(d.options, positioned{o.Position, t})
			all = append(all, t)
		}
	}
	for _, t := range all {
		if t.sendable() {
			d.sent = append(d.sent, t)
		}
	}
	return d, nil
}

// fragments returns what the provider receives.
func (d *draft) fragments() []string {
	out := make([]string, len(d.sent))
	for i, t := range d.sent {
		out[i] = t.fragment
	}
	return out
}

// apply checks the provider's answer and returns the row that it makes: the body goes through the
// sanitizer, and the other fields are plain text (ADR-0148 §8). An answer that fails the check is
// not stored.
func (d *draft) apply(res Result, sanitizer kernel.HTMLSanitizer) (Stored, error) {
	if len(res.Translations) != len(d.sent) {
		return Stored{}, fmt.Errorf("translation: %d translations of %d fragments", len(res.Translations), len(d.sent))
	}
	for i, t := range d.sent {
		if !MarkersIntact(t.fragment, res.Translations[i]) {
			return Stored{}, fmt.Errorf("translation: fragment %d: the markers did not come back intact", i)
		}
		if strings.TrimSpace(res.Translations[i]) == "" {
			return Stored{}, fmt.Errorf("translation: fragment %d: the translation is empty", i)
		}
		t.translated = res.Translations[i]
	}
	source := res.DetectedLanguage
	if source == "" {
		source = d.sourceLanguage
	}
	plain := func(t *maskedText) string {
		return sanitizer.PlainText(t.restore(func(s string) string { return s }))
	}
	out := Stored{SourceLanguage: source, ContentHTML: sanitizer.SanitizeHTML(d.body.render()), SpoilerText: plain(d.spoiler)}
	for _, a := range d.attachments {
		out.Attachments = append(out.Attachments, StoredAttachment{Position: a.position, Description: plain(a.text)})
	}
	for _, o := range d.options {
		out.PollOptions = append(out.PollOptions, StoredPollOption{Position: o.position, Title: plain(o.text)})
	}
	return out, nil
}

// RegisterProjectors deletes the translations of a status in the transaction of its deletion,
// which every path of a deletion emits (ADR-0148 §10). Register it after the content module's
// projectors, which take the lock that HoldLive waits for.
func RegisterProjectors(r *uow.Projectors, repo Repository) {
	r.Register(appcontent.EventStatusDeleted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.DeleteStatusTranslations(ctx, tx, e.(appcontent.StatusDeleted).Status.ID)
	})
}

// RegisterJobs registers the handler and the schedule of the periodic job.
func (s *Service) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobPurgeCache, func(ctx context.Context, _ queue.Job) queue.Result {
		if err := s.PurgeCache(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	sched.Add(queue.Schedule{Name: JobPurgeCache, Every: PurgeCacheInterval, Job: queue.NewJob{Type: JobPurgeCache}})
}

// PurgeCache removes the translations older than the retention, counted from when they were made,
// since reading one does not extend it, and the usage of the months before the last one
// (ADR-0148 §9).
func (s *Service) PurgeCache(ctx context.Context) error {
	days := DefaultRetentionDays
	if s.d.RetentionDays != nil {
		var err error
		if days, err = s.d.RetentionDays(ctx); err != nil {
			return err
		}
	}
	now := s.d.Clock.Now().UTC()
	before := now.AddDate(0, 0, -days)
	for {
		ids, err := s.d.Repository.ListExpiredTranslations(ctx, before, purgeBatch)
		if err != nil {
			return err
		}
		if len(ids) > 0 {
			if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				return s.d.Repository.DeleteTranslations(ctx, tx, ids)
			}); err != nil {
				return err
			}
		}
		if len(ids) < purgeBatch {
			break
		}
	}
	lastMonth := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, time.UTC).Format(monthFormat)
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.d.Repository.DeleteUsageBefore(ctx, tx, lastMonth)
	})
}

// monthFormat is the format of a month in the usage rows.
const monthFormat = "2006-01"

// Eraser removes what an erased account left in this module: its usage rows.
type Eraser struct {
	uow  uow.UnitOfWork
	repo Repository
}

// NewEraser returns the module's eraser.
func NewEraser(u uow.UnitOfWork, r Repository) *Eraser { return &Eraser{uow: u, repo: r} }

// EraseAccountData implements account.DataEraser.
func (e *Eraser) EraseAccountData(ctx context.Context, t appaccount.ErasureTarget, _ string, _ int) (appaccount.ErasureStep, error) {
	err := e.uow.Tx(ctx, func(tx uow.Tx) error {
		return e.repo.DeleteAccountUsage(ctx, tx, t.AccountID)
	})
	return appaccount.ErasureStep{Done: err == nil}, err
}
