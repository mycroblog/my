// SPDX-License-Identifier: AGPL-3.0-only

package finance

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/finance"
)

// Jobs and schedules of the billing imports (ADR-0068 §2).
const (
	// JobBillingImport imports the charges of one source (its payload names it). Every source of
	// billing_sources has the schedule "finance.billing_import.<source>" every ImportInterval,
	// and an administrator can queue one now (RequestImport).
	JobBillingImport = "finance.billing_import"
	// ImportInterval is the period of the scheduled imports: the providers update their data
	// about once a day.
	ImportInterval = 24 * time.Hour
	// MaxServiceLength is the longest service name that an import records
	// (imported_cost_revisions.service).
	MaxServiceLength = 200
)

var (
	// ErrUnknownSource reports a billing source that billing_sources does not enable.
	ErrUnknownSource = errors.New("finance: unknown billing source")
	// ErrImportTooSoon reports a manual import of a source that was already requested in the
	// same UTC hour.
	ErrImportTooSoon = errors.New("finance: an import of the source was requested in this hour")
)

// RequestCaps bounds the billed requests of a source, counted by UTC day and UTC calendar month
// over every attempt, scheduled, retried, or manual (ADR-0068 §4).
type RequestCaps struct {
	PerDay   int
	PerMonth int
}

// Default caps of the AWS requests (billing_aws_max_requests_per_day and _per_month): four
// imports a day, for thirty days.
const (
	DefaultAWSRequestsPerDay   = 4
	DefaultAWSRequestsPerMonth = 120
)

// SourceProvider returns the provider's name that the totals and the screens show for a source:
// AWS, Google Cloud, or the source's name for another.
func SourceProvider(source string) string {
	switch source {
	case "aws":
		return "AWS"
	case "gcp":
		return "Google Cloud"
	}
	return source
}

// SourceRule is the basis of a source's billing dates (ADR-0067 §5): the time zone that cuts its
// billing months, kept as the provider reports them.
type SourceRule struct {
	Source   string
	Provider string
	Location *time.Location
}

// Rules is the `rules` of the totals (ADR-0067 §9): weeks start on Monday (ISO 8601), and the
// billing dates of each enabled source keep its time zone.
type Rules struct {
	WeekStart time.Weekday
	Sources   []SourceRule
}

// Rules returns the rules of the totals.
func (s *Service) Rules() Rules {
	out := Rules{WeekStart: time.Monday, Sources: make([]SourceRule, 0, len(s.d.Sources))}
	for _, src := range s.d.Sources {
		out.Sources = append(out.Sources, SourceRule{Source: src.Name(), Provider: SourceProvider(src.Name()), Location: src.Location()})
	}
	return out
}

// importPayload is the payload of JobBillingImport.
type importPayload struct {
	Source string `json:"source"`
}

func importJob(source, key string) (queue.NewJob, error) {
	b, err := json.Marshal(importPayload{Source: source})
	if err != nil {
		return queue.NewJob{}, err
	}
	return queue.NewJob{Type: JobBillingImport, Payload: b, IdempotencyKey: key}, nil
}

// RegisterJobs registers the handler of JobBillingImport and, when sc is not nil, the daily
// schedule of every source.
func (s *Service) RegisterJobs(h *queue.JobHandlerRegistry, sc *queue.Scheduler) {
	h.Register(JobBillingImport, func(ctx context.Context, j queue.Job) queue.Result {
		var p importPayload
		if err := json.Unmarshal(j.Payload, &p); err != nil || p.Source == "" {
			return queue.Fail(fmt.Errorf("finance: the payload of %s: %w", j.ID, err))
		}
		key := j.IdempotencyKey
		if key == "" {
			key = "job:" + j.ID
		}
		return s.Import(ctx, p.Source, key)
	})
	if sc == nil {
		return
	}
	for _, src := range s.d.Sources {
		job, err := importJob(src.Name(), "")
		if err != nil {
			panic(err) // a string always marshals
		}
		sc.Add(queue.Schedule{Name: JobBillingImport + "." + src.Name(), Every: ImportInterval, Job: job})
	}
}

func (s *Service) source(name string) (BillingSource, bool) {
	for _, src := range s.d.Sources {
		if src.Name() == name {
			return src, true
		}
	}
	return nil, false
}

func (s *Service) logger() *slog.Logger {
	if s.d.Logger == nil {
		return slog.Default()
	}
	return s.d.Logger
}

// utcDay and utcMonth return the UTC day and the UTC calendar month that hold t, as [from, until).
func utcDay(t time.Time) (time.Time, time.Time) {
	y, m, d := t.UTC().Date()
	from := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return from, from.AddDate(0, 0, 1)
}

func utcMonth(t time.Time) (time.Time, time.Time) {
	y, m, _ := t.UTC().Date()
	from := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	return from, from.AddDate(0, 1, 0)
}

// usage returns the requests of source today and this month (UTC), and its bytes billed this month.
func (s *Service) usage(ctx context.Context, source string, now time.Time) (day, month int, bytes int64, err error) {
	from, until := utcDay(now)
	if day, _, err = s.d.Repository.SumRuns(ctx, source, from, until); err != nil {
		return 0, 0, 0, err
	}
	from, until = utcMonth(now)
	month, bytes, err = s.d.Repository.SumRuns(ctx, source, from, until)
	return day, month, bytes, err
}

// ReservationHold is how long the reservation of an attempt that is not recorded yet keeps the
// other attempts of its source waiting (ADR-0068 §4): longer than a job may run
// (queue.DefaultLease), so that an attempt that stopped before it recorded anything holds the
// source back for a while only. Its requests count toward the caps all the same.
const ReservationHold = 15 * time.Minute

// importRunningRetry is when an attempt that found another attempt of its source running tries
// again.
const importRunningRetry = time.Minute

// ErrImportRunning reports an attempt of a source while another attempt of it is running.
var ErrImportRunning = errors.New("finance: another import of the source is running")

// reservation is how reserve ends.
type reservation int

const (
	// reserved: the attempt may call the provider, up to the requests that reserve returns.
	reserved reservation = iota
	// capped: the caps leave the attempt no request.
	capped
	// alreadySucceeded: an attempt of the same job succeeded, so this one calls nothing.
	alreadySucceeded
	// running: another attempt of the source is running.
	running
)

// reserve reserves the billed requests of the attempt run before it calls the provider, in one
// transaction that runs alone for the source (ADR-0068 §4): the smaller of what the day and the
// month leave, or no limit (math.MaxInt) for a source without caps. The reservation counts toward
// the caps until the attempt is recorded, so that attempts at the same time cannot together
// exceed them, and it keeps the other attempts of the source waiting for ReservationHold at
// most. An attempt of a job (run key) that already succeeded reserves nothing.
func (s *Service) reserve(ctx context.Context, run Run) (reservation, int, error) {
	caps, hasCaps := s.d.Caps[run.Source]
	outcome, allowed := reserved, 0
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		outcome, allowed = reserved, 0
		repo := s.d.Repository
		if err := repo.LockSource(ctx, tx, run.Source); err != nil {
			return err
		}
		if done, err := repo.RunKeySucceeded(ctx, tx, run.Source, run.RunKey); err != nil || done {
			outcome = alreadySucceeded
			return err
		}
		if busy, err := repo.ImportRunning(ctx, tx, run.Source, run.OccurredAt.Add(-ReservationHold)); err != nil || busy {
			outcome = running
			return err
		}
		allowed = math.MaxInt
		requests := 0
		if hasCaps {
			from, until := utcDay(run.OccurredAt)
			day, _, err := repo.SumRunsTx(ctx, tx, run.Source, from, until)
			if err != nil {
				return err
			}
			from, until = utcMonth(run.OccurredAt)
			month, _, err := repo.SumRunsTx(ctx, tx, run.Source, from, until)
			if err != nil {
				return err
			}
			allowed = max(0, min(caps.PerDay-day, caps.PerMonth-month))
			requests = allowed
		}
		if allowed == 0 {
			outcome = capped
			return nil
		}
		return repo.InsertReservation(ctx, tx, Reservation{
			ID: run.ID, Source: run.Source, RunKey: run.RunKey, Requests: requests, ReservedAt: run.OccurredAt,
		})
	})
	if err != nil {
		return reserved, 0, fmt.Errorf("finance: reserve the requests of the billing import of %s: %w", run.Source, err)
	}
	return outcome, allowed, nil
}

// Import runs one attempt of the import of source for the job runKey (ADR-0068 §2): it reserves
// its requests within the caps (reserve), fetches the months that RefetchMonths gives outside any
// transaction, and records the attempt and, when it succeeds, a revision for every key whose
// value or kind changed, in one transaction. An attempt of a job that already succeeded calls
// nothing, and one that finds another attempt of the source running tries again later. A key of a fetched month that the source no longer reports gets a revision
// of 0. An attempt that the caps leave no request to calls nothing, is recorded as request_cap,
// and is not retried; throttling and unavailability are retried, and the other failures wait for
// the next scheduled import.
func (s *Service) Import(ctx context.Context, source, runKey string) queue.Result {
	src, ok := s.source(source)
	if !ok {
		s.logger().WarnContext(ctx, "billing import of a source that is not enabled", slog.String("source", source))
		return queue.Done()
	}
	now := kernel.Normalize(s.d.Clock.Now())
	today := finance.DateOf(now.In(src.Location()))
	previous := finance.YearMonthOf(today).AddMonths(-1)
	heads, err := s.d.Repository.ImportedHeads(ctx, source, []finance.YearMonth{previous})
	if err != nil {
		return queue.Retry(0, err)
	}
	previousFinal := len(heads) > 0
	for _, h := range heads {
		previousFinal = previousFinal && h.Kind.Closed()
	}
	months := RefetchMonths(today, previousFinal)
	run := Run{ID: s.d.IDs.NewID(), Source: source, RunKey: runKey, ErrorKind: ImportErrorNone, OccurredAt: now}
	outcome, allowed, err := s.reserve(ctx, run)
	switch {
	case err != nil:
		return queue.Retry(0, err)
	case outcome == alreadySucceeded:
		s.logger().InfoContext(ctx, "billing import of a job that already succeeded", slog.String("source", source))
		return queue.Done()
	case outcome == running:
		return queue.Retry(importRunningRetry, fmt.Errorf("%w: %s", ErrImportRunning, source))
	case outcome == capped:
		return s.failed(ctx, run, fmt.Errorf("%w: no request left today or this month", ErrRequestCap))
	}
	snap, err := src.Fetch(ctx, FetchRequest{Months: months, MaxRequests: allowed})
	run.Requests, run.BytesBilled = max(0, snap.Requests), max(0, snap.BytesBilled)
	if err != nil {
		return s.failed(ctx, run, err)
	}
	lines, err := s.checkLines(source, months, snap.Lines)
	if err != nil {
		return s.failed(ctx, run, err)
	}
	// The IDs stay the same when the transaction is retried, like the IDs generated before one.
	var ids []string
	idAt := func(i int) string {
		for len(ids) <= i {
			ids = append(ids, s.d.IDs.NewID())
		}
		return ids[i]
	}
	written := 0
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		written = 0
		if err := s.d.Repository.InsertRun(ctx, tx, run); err != nil {
			return err
		}
		heads, err := s.d.Repository.ImportedHeadsForUpdate(ctx, tx, source, months)
		if err != nil {
			return err
		}
		for i, r := range changedRevisions(heads, lines, months) {
			r.ID, r.RunID, r.RunKey, r.RecordedAt = idAt(i), run.ID, runKey, now
			inserted, err := s.d.Repository.InsertImportedRevision(ctx, tx, r)
			if err != nil {
				return err
			}
			if !inserted {
				continue // an earlier attempt of the job wrote the key
			}
			if err := s.d.Repository.UpsertImportedHead(ctx, tx, r); err != nil {
				return err
			}
			written++
		}
		return nil
	})
	if err != nil {
		return queue.Retry(0, fmt.Errorf("finance: record the billing import of %s: %w", source, err))
	}
	s.imports.Add(ctx, 1, kernel.NewLabels(LabelBillingSource, source, LabelOutcome, ImportOutcomeSuccess))
	s.logger().InfoContext(ctx, "billing import recorded", slog.String("source", source),
		slog.Int("lines", len(lines)), slog.Int("revisions", written), slog.Int("requests", run.Requests))
	return queue.Done()
}

// failed records a failed attempt and returns the result of the job: a retry for the failures
// that ImportErrorKind.Retried passes.
func (s *Service) failed(ctx context.Context, run Run, cause error) queue.Result {
	run.ErrorKind = ImportErrorKindOf(cause)
	if run.ErrorKind == ImportErrorNone {
		run.ErrorKind = ImportErrorUnavailable
	}
	// The attempt is recorded even when ctx ended, so that its requests count toward the caps.
	rec := context.WithoutCancel(ctx)
	if err := s.d.UnitOfWork.Tx(rec, func(tx uow.Tx) error { return s.d.Repository.InsertRun(rec, tx, run) }); err != nil {
		return queue.Retry(0, fmt.Errorf("finance: record the failed billing import of %s: %w", run.Source, err))
	}
	s.imports.Add(ctx, 1, kernel.NewLabels(
		LabelBillingSource, run.Source, LabelOutcome, ImportOutcomeFailed, LabelErrorType, string(run.ErrorKind)))
	s.logger().WarnContext(ctx, "billing import failed", slog.String("source", run.Source),
		slog.String("error_kind", string(run.ErrorKind)), slog.Int("requests", run.Requests), slog.Any("error", cause))
	if run.ErrorKind.Retried() {
		return queue.Retry(0, cause)
	}
	return queue.Done()
}

// importedLine is a CostLine checked and rounded to minor units.
type importedLine struct {
	key      ImportedKey
	category finance.Category
	covered  finance.DateRange
	amount   finance.Money
	final    bool
}

// checkLines checks the lines of a snapshot against the port's contract (ADR-0068 §1) and rounds
// their amounts; it fails with ErrInvalidBillingResponse.
func (s *Service) checkLines(source string, months []finance.YearMonth, lines []CostLine) ([]importedLine, error) {
	out := make([]importedLine, 0, len(lines))
	seen := map[ImportedKey]bool{}
	for _, l := range lines {
		key := ImportedKey{Source: source, Month: l.Month, Service: l.Service, ChargeType: l.ChargeType}
		bad := func(why string) error {
			return fmt.Errorf("%w: %s %s %q %s: %s", ErrInvalidBillingResponse, source, l.Month, l.Service, l.ChargeType, why)
		}
		switch {
		case !slices.Contains(months, l.Month):
			return nil, bad("a month that was not asked for")
		case l.Service == "" || !utf8.ValidString(l.Service) || utf8.RuneCountInString(l.Service) > MaxServiceLength:
			return nil, bad("an empty, invalid, or too long service")
		case !l.ChargeType.Valid():
			return nil, bad("an unknown charge type")
		case !l.Category.Valid():
			return nil, bad("an unknown category")
		case !l.Covered.Valid():
			return nil, bad("an empty covered range")
		case seen[key]:
			return nil, bad("a repeated key")
		}
		if month := l.Month.Range(); !month.Contains(l.Covered.Start) || !month.Contains(l.Covered.End) {
			return nil, bad("covered days outside the month")
		}
		currency, err := finance.ParseCurrency(l.Currency)
		if err != nil {
			return nil, bad(err.Error())
		}
		minor, err := finance.MinorUnitsOf(l.Amount, currency)
		if err != nil {
			return nil, bad(err.Error())
		}
		seen[key] = true
		out = append(out, importedLine{
			key: key, category: l.Category, covered: l.Covered, amount: finance.Money{Minor: minor, Currency: currency}, final: l.Final,
		})
	}
	return out, nil
}

// changedRevisions returns the revisions that an import of months records over the heads of the
// source (ADR-0068 §2, §3): one for each line whose amount, currency, finality, or covered days
// differ from its head, and one of 0 for each head of the months that no line reports any more.
// Their IDs, runs, and times are left to the caller.
func changedRevisions(heads []ImportedRevision, lines []importedLine, months []finance.YearMonth) []ImportedRevision {
	byKey := make(map[ImportedKey]ImportedRevision, len(heads))
	for _, h := range heads {
		byKey[h.Key] = h
	}
	monthFinal := map[finance.YearMonth]bool{}
	reported := map[ImportedKey]bool{}
	var out []ImportedRevision
	for _, l := range lines {
		monthFinal[l.key.Month] = monthFinal[l.key.Month] || l.final
		reported[l.key] = true
		prev, had := byKey[l.key]
		if had && prev.Amount == l.amount && prev.Kind.Closed() == l.final && prev.Covered == l.covered {
			continue
		}
		out = append(out, ImportedRevision{
			Key: l.key, Category: l.category, Covered: l.covered, Amount: l.amount, Kind: finance.NextRevisionKind(prev.Kind, l.final),
		})
	}
	for _, h := range heads {
		if reported[h.Key] || !slices.Contains(months, h.Key.Month) || h.Amount.Minor == 0 {
			continue
		}
		out = append(out, ImportedRevision{
			Key: h.Key, Category: h.Category, Covered: h.Covered, Amount: finance.Money{Currency: h.Amount.Currency},
			Kind: finance.NextRevisionKind(h.Kind, monthFinal[h.Key.Month]),
		})
	}
	return out
}

// ManualRunKey returns the idempotency key of a manual import of source at t: one per UTC hour.
func ManualRunKey(source string, t time.Time) string {
	return "manual:" + source + ":" + t.UTC().Format("2006-01-02T15")
}

// RequestImport queues an import of source now, at most once per UTC hour (ADR-0068 §2), and
// records the request in the audit log. byUserID is the administrator, "" for the CLI. It fails
// with ErrUnknownSource for a source that is not enabled and with ErrImportTooSoon when an
// import of the hour already ran. Its requests count toward the caps like the others.
func (s *Service) RequestImport(ctx context.Context, source, byUserID string) error {
	if _, ok := s.source(source); !ok || s.d.Outbox == nil {
		return fmt.Errorf("%w: %q", ErrUnknownSource, source)
	}
	now := kernel.Normalize(s.d.Clock.Now())
	key := ManualRunKey(source, now)
	used, err := s.d.Repository.RunKeyUsed(ctx, source, key)
	if err != nil {
		return err
	}
	if used {
		return fmt.Errorf("%w: %s", ErrImportTooSoon, source)
	}
	job, err := importJob(source, key)
	if err != nil {
		return err
	}
	e := appmoderation.AuditEntry{
		ID: s.d.IDs.NewID(), Action: appmoderation.AuditBillingImportQueued, TargetKind: appmoderation.AuditTargetBillingSource,
		TargetID: source, TargetLabel: SourceProvider(source), OccurredAt: now,
	}
	e.ActorKind, e.ActorUserID = appmoderation.AuditActor(byUserID)
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := tx.Emit(ctx, appmoderation.AuditRecorded{Entry: e}); err != nil {
			return err
		}
		return s.d.Outbox.EnqueueTx(ctx, tx, job)
	})
}

// StatusMonths is how many billing months, the current one included, ImportStatuses shows.
const StatusMonths = 12

// ImportStatus is the state of the imports of a source for the administrators (ADR-0068 §9).
type ImportStatus struct {
	Source   string
	Provider string
	Location *time.Location
	// Method is the source's BillingSource.Method.
	Method string
	// LastRun is the newest attempt, and LastSuccess the newest success; nil without one.
	LastRun     *Run
	LastSuccess *Run
	// RequestsToday and RequestsThisMonth count the billed requests by UTC day and month, and
	// BytesBilledThisMonth the bytes billed this UTC month.
	RequestsToday        int
	RequestsThisMonth    int
	BytesBilledThisMonth int64
	// Caps bounds the requests; nil for a source without caps.
	Caps *RequestCaps
	// Months are the billing months with imported costs among the last StatusMonths, newest first.
	Months []MonthStatus
}

// MonthStatus is the state of a billing month of a source: an estimate while any key is, a
// correction once any key was corrected after the month closed, final otherwise; and its
// totals by currency.
type MonthStatus struct {
	Month  finance.YearMonth
	Kind   finance.RevisionKind
	Totals []finance.Money
}

// FailingSources reports for every enabled source whether its newest import attempt failed, for
// the metrics snapshot; a source without an attempt is not failing.
func (s *Service) FailingSources(ctx context.Context) (map[string]bool, error) {
	out := make(map[string]bool, len(s.d.Sources))
	for _, src := range s.d.Sources {
		r, err := s.d.Repository.LastRun(ctx, src.Name(), false)
		switch {
		case errors.Is(err, ErrNotFound):
			out[src.Name()] = false
		case err != nil:
			return nil, err
		default:
			out[src.Name()] = !r.Succeeded()
		}
	}
	return out, nil
}

// ImportStatuses returns the state of the imports of every enabled source, in the order of
// billing_sources.
func (s *Service) ImportStatuses(ctx context.Context) ([]ImportStatus, error) {
	now := kernel.Normalize(s.d.Clock.Now())
	out := make([]ImportStatus, 0, len(s.d.Sources))
	for _, src := range s.d.Sources {
		st := ImportStatus{Source: src.Name(), Provider: SourceProvider(src.Name()), Location: src.Location(), Method: src.Method()}
		if caps, ok := s.d.Caps[src.Name()]; ok {
			st.Caps = &caps
		}
		for _, succeeded := range []bool{false, true} {
			r, err := s.d.Repository.LastRun(ctx, src.Name(), succeeded)
			switch {
			case errors.Is(err, ErrNotFound):
				continue
			case err != nil:
				return nil, err
			}
			if succeeded {
				st.LastSuccess = &r
			} else {
				st.LastRun = &r
			}
		}
		var err error
		if st.RequestsToday, st.RequestsThisMonth, st.BytesBilledThisMonth, err = s.usage(ctx, src.Name(), now); err != nil {
			return nil, err
		}
		current := finance.YearMonthOf(finance.DateOf(now.In(src.Location())))
		months := make([]finance.YearMonth, StatusMonths)
		for i := range months {
			months[i] = current.AddMonths(-i)
		}
		heads, err := s.d.Repository.ImportedHeads(ctx, src.Name(), months)
		if err != nil {
			return nil, err
		}
		st.Months = monthStatuses(heads)
		out = append(out, st)
	}
	return out, nil
}

func monthStatuses(heads []ImportedRevision) []MonthStatus {
	var out []MonthStatus
	index := map[finance.YearMonth]int{}
	totals := map[finance.YearMonth]map[finance.Currency]int64{}
	for _, h := range heads {
		i, ok := index[h.Key.Month]
		if !ok {
			i = len(out)
			index[h.Key.Month] = i
			out = append(out, MonthStatus{Month: h.Key.Month, Kind: finance.KindFinal})
			totals[h.Key.Month] = map[finance.Currency]int64{}
		}
		switch {
		case h.Kind == finance.KindEstimate:
			out[i].Kind = finance.KindEstimate
		case h.Kind == finance.KindCorrection && out[i].Kind == finance.KindFinal:
			out[i].Kind = finance.KindCorrection
		}
		totals[h.Key.Month][h.Amount.Currency] += h.Amount.Minor
	}
	for i := range out {
		for c, n := range totals[out[i].Month] {
			out[i].Totals = append(out[i].Totals, finance.Money{Minor: n, Currency: c})
		}
		slices.SortFunc(out[i].Totals, func(a, b finance.Money) int { return cmp.Compare(a.Currency, b.Currency) })
	}
	slices.SortFunc(out, func(a, b MonthStatus) int { return b.Month.Compare(a.Month) })
	return out
}

// importedEntries returns the newest revision of every imported key as entries of the totals.
func (s *Service) importedEntries(ctx context.Context) ([]finance.Entry, error) {
	heads, err := s.d.Repository.ImportedHeads(ctx, "", nil)
	if err != nil {
		return nil, err
	}
	out := make([]finance.Entry, len(heads))
	for i, h := range heads {
		out[i] = finance.Entry{
			Range: h.Covered, Amount: h.Amount, Provider: NormalizeProvider(SourceProvider(h.Key.Source)), Category: h.Category,
			Spelling:  finance.Spelling{Name: SourceProvider(h.Key.Source)},
			Estimated: h.Kind == finance.KindEstimate,
			Item:      finance.ItemRef{Key: "import:" + h.Key.Source + ":" + h.Key.Service, Name: h.Key.Service},
		}
	}
	return out, nil
}
