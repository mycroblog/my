// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// The subscriptions to domain block lists that others publish (ADR-0159 §4, §5): a list at a URL,
// fetched every day, whose rows become domain actions of this server, either as drafts that a
// moderator accepts or rejects, or directly in the automatic mode. The actions a subscription
// issues are ordinary domain actions; SubscriptionDomainAction records which subscription issued
// each, and a subscription never lifts nor replaces an action it did not issue.

// DomainListFormat is the format of a subscribed list (domain_list_subscriptions.format).
type DomainListFormat string

// Formats of the lists.
const (
	// DomainListMastodonCSV is Mastodon's domain block CSV, read as the import reads it
	// (ParseDomainBlockCSV).
	DomainListMastodonCSV DomainListFormat = "mastodon_csv"
	// DomainListMastodonJSON is the answer of Mastodon's GET /api/v1/instance/domain_blocks: an
	// array of objects with domain, digest, severity, and comment.
	DomainListMastodonJSON DomainListFormat = "mastodon_json"
	// DomainListPlainText is one domain per line, each a suspend.
	DomainListPlainText DomainListFormat = "plain_text"
)

// Check returns an error wrapping ErrInvalidDomainList unless f is one of the formats.
func (f DomainListFormat) Check() error {
	if !f.Valid() {
		return invalidDomainList("format")
	}
	return nil
}

// Valid reports whether f is one of the formats.
func (f DomainListFormat) Valid() bool {
	return f == DomainListMastodonCSV || f == DomainListMastodonJSON || f == DomainListPlainText
}

// DomainListMode is what a subscription does with the differences of a fetch.
type DomainListMode string

// Modes of a subscription.
const (
	// DomainListDraft keeps every difference as a draft for a moderator (the default).
	DomainListDraft DomainListMode = "draft"
	// DomainListAutomatic issues and lifts the subscription's actions directly, except the
	// differences that DomainListChange.Held names.
	DomainListAutomatic DomainListMode = "automatic"
)

// Valid reports whether m is one of the modes.
func (m DomainListMode) Valid() bool { return m == DomainListDraft || m == DomainListAutomatic }

// DomainListSeverityMapping is the domain action that a severity of a list stands for on this
// server (suspend_as, silence_as).
type DomainListSeverityMapping string

// Mappings of a severity.
const (
	DomainListAsSuspend DomainListSeverityMapping = "suspend"
	DomainListAsLimit   DomainListSeverityMapping = "limit"
	// DomainListAsIgnore issues nothing for the severity; the rejections of the row still apply.
	DomainListAsIgnore DomainListSeverityMapping = "ignore"
)

// kind returns the kind of action that m stands for, and false for ignore.
func (m DomainListSeverityMapping) kind() (DomainActionKind, bool) {
	switch m {
	case DomainListAsSuspend:
		return DomainSuspend, true
	case DomainListAsLimit:
		return DomainLimit, true
	}
	return "", false
}

// MaxDomainListTitle is the longest title of a subscription, in characters.
const MaxDomainListTitle = 200

// MaxDomainListURL is the longest URL of a subscription, in bytes.
const MaxDomainListURL = 2048

// MaxDomainListBytes is the largest list that a fetch reads (ADR-0159 §4); a larger one fails
// the fetch and changes nothing. A list holds at most MaxDomainBlockImport rows, as an import.
const MaxDomainListBytes = 2 << 20

// FailingDomainListFetches is the number of failed fetches in a row after which the holders of
// manage_domains are told that a subscription fails.
const FailingDomainListFetches = 3

// MassLiftMin is the number of lifts above which one fetch in the automatic mode holds its lifts
// as drafts when they are also more than half of the subscription's actions in effect, so that a
// broken or empty answer never lifts a list at once.
const MassLiftMin = 10

var (
	// ErrInvalidDomainList reports a subscription whose URL, format, or settings are not valid.
	ErrInvalidDomainList = errors.New("moderation: invalid domain list subscription")
	// ErrMalformedDomainList reports a list that its format cannot read.
	ErrMalformedDomainList = errors.New("moderation: malformed domain list")
)

// DomainListFieldError names the field of a subscription that is not valid; it comes wrapped
// with ErrInvalidDomainList.
type DomainListFieldError struct{ Field string }

func (e *DomainListFieldError) Error() string { return "the " + e.Field + " is not valid" }

func invalidDomainList(field string) error {
	return fmt.Errorf("%w: %w", ErrInvalidDomainList, &DomainListFieldError{Field: field})
}

// DomainListSettings are what a moderator sets of a subscription and may change later
// (domain_list_subscription_revisions).
type DomainListSettings struct {
	Title string
	Mode  DomainListMode
	// SuspendAs is what a suspend of the list issues; SilenceAs what a silence does.
	SuspendAs DomainListSeverityMapping
	SilenceAs DomainListSeverityMapping
	// ApplyRejectMedia and ApplyRejectReports issue the rejections that a row of the list sets.
	ApplyRejectMedia   bool
	ApplyRejectReports bool
	// PublicCommentFromList copies the public comment of a row to the actions it issues.
	PublicCommentFromList bool
}

// DefaultDomainListSettings returns the settings of a new subscription titled title: drafts,
// suspend as suspend, silence as limit, the rejections applied, and no public comment copied.
func DefaultDomainListSettings(title string) DomainListSettings {
	return DomainListSettings{
		Title: title, Mode: DomainListDraft, SuspendAs: DomainListAsSuspend, SilenceAs: DomainListAsLimit,
		ApplyRejectMedia: true, ApplyRejectReports: true,
	}
}

// Validate checks the settings: a title of 1 to MaxDomainListTitle characters, a mode, and the
// mappings (silence is limit or ignore).
func (s DomainListSettings) Validate() error {
	switch {
	case strings.TrimSpace(s.Title) == "" || !validText(s.Title):
		return invalidDomainList("title")
	case utf8.RuneCountInString(s.Title) > MaxDomainListTitle:
		return fmt.Errorf("%w: %w", ErrInvalidDomainList, &TextError{Field: "title", Max: MaxDomainListTitle})
	case !s.Mode.Valid():
		return invalidDomainList("mode")
	case s.SuspendAs != DomainListAsSuspend && s.SuspendAs != DomainListAsLimit && s.SuspendAs != DomainListAsIgnore:
		return invalidDomainList("suspend_as")
	case s.SilenceAs != DomainListAsLimit && s.SilenceAs != DomainListAsIgnore:
		return invalidDomainList("silence_as")
	}
	return nil
}

// CheckDomainListURL checks the URL of a list: an absolute https URL with a host, without user
// information, of at most MaxDomainListURL bytes.
func CheckDomainListURL(raw string) error {
	u, err := url.Parse(raw)
	switch {
	case err != nil || len(raw) > MaxDomainListURL || !validText(raw),
		u.Scheme != "https" || u.Host == "" || u.User != nil || u.Opaque != "":
		return invalidDomainList("url")
	}
	return nil
}

// DomainListSubscription is a subscription to a list (domain_list_subscriptions, a resource):
// what never changes of it.
type DomainListSubscription struct {
	ID              string
	URL             string
	Format          DomainListFormat
	CreatedByUserID string // "" for the CLI
	CreatedAt       time.Time
}

// DomainListRevision is a version of the settings of a subscription
// (domain_list_subscription_revisions, an event); the first one is written with the
// subscription.
type DomainListRevision struct {
	ID              string
	SubscriptionID  string
	Settings        DomainListSettings
	RevisedByUserID string // "" for the CLI
	RevisedAt       time.Time
}

// DomainListDeletion is the deletion of a subscription (domain_list_subscription_deletions, an
// event). The actions it issued stay unless LiftActions; those that stay are then like the
// actions a person issued.
type DomainListDeletion struct {
	SubscriptionID  string
	LiftActions     bool
	DeletedByUserID string // "" for the CLI
	DeletedAt       time.Time
}

// DomainListFetchOutcome is how a fetch ended (domain_list_fetches.outcome).
type DomainListFetchOutcome string

// Outcomes of a fetch.
const (
	// DomainListApplied: the list was read and its entries replaced those of the subscription.
	DomainListApplied DomainListFetchOutcome = "applied"
	// DomainListNotModified: the server answered 304, or the list is the one read last.
	DomainListNotModified DomainListFetchOutcome = "not_modified"
	// DomainListFailed: nothing was read, and nothing changed (DomainListFetch.ErrorCode).
	DomainListFailed DomainListFetchOutcome = "failed"
)

// DomainListFetchError is why a fetch failed (domain_list_fetches.error_code).
type DomainListFetchError string

// Errors of a fetch.
const (
	// DomainListErrorHTTPStatus: the server answered a status other than 2xx and 304, such as
	// 401, 403, or 404 of a server that does not publish its domain blocks.
	DomainListErrorHTTPStatus DomainListFetchError = "http_status"
	// DomainListErrorNetwork: the server could not be reached or did not answer in time.
	DomainListErrorNetwork DomainListFetchError = "network"
	// DomainListErrorForbiddenDestination: the URL, or a redirect, leads to an address that the
	// server does not fetch from.
	DomainListErrorForbiddenDestination DomainListFetchError = "forbidden_destination"
	// DomainListErrorTooLarge: the list is larger than MaxDomainListBytes.
	DomainListErrorTooLarge DomainListFetchError = "too_large"
	// DomainListErrorTooManyEntries: the list has more than MaxDomainBlockImport rows.
	DomainListErrorTooManyEntries DomainListFetchError = "too_many_entries"
	// DomainListErrorMalformed: the list cannot be read in the subscription's format.
	DomainListErrorMalformed DomainListFetchError = "malformed"
)

// DomainListFetch is one fetch of a list (domain_list_fetches, an event).
type DomainListFetch struct {
	ID             string
	SubscriptionID string
	Outcome        DomainListFetchOutcome
	HTTPStatus     int                  // 0 when no answer came
	ErrorCode      DomainListFetchError // "" unless failed
	// EntryCount is the number of entries the subscription holds after the fetch.
	EntryCount int
	FetchedAt  time.Time
}

// DomainListHead is the current value of a subscription (domain_list_subscription_heads): its
// settings in effect, and what the next fetch sends and counts.
type DomainListHead struct {
	SubscriptionID string
	RevisionID     string
	// ETag and LastModified are the validators of the last list read, "" for none.
	ETag         string
	LastModified string
	// Digest is the hex SHA-256 of the last list read, "" before the first one.
	Digest string
	// ConsecutiveFailures counts the failed fetches since the last one that did not fail.
	ConsecutiveFailures int
	// LastFetchID is the last fetch, "" before the first one.
	LastFetchID string
}

// DomainListEntry is one domain of the list as the last fetch that read it listed it
// (domain_list_entries, a projection of the fetches). The kinds of actions it stands for depend
// on the settings in effect (DomainListSettings.Kinds).
type DomainListEntry struct {
	SubscriptionID string
	// Domain is canonical (federation.CanonicalHost).
	Domain string
	// Severity is the severity as listed, in lower case: suspend, silence, noop, or one that
	// this server does not know.
	Severity      DomainBlockSeverity
	RejectMedia   bool
	RejectReports bool
	PublicComment string
	Obfuscate     bool
	// SinceFetchID is the fetch from which the domain is listed with this content without a
	// break: a rejected draft of it stays rejected until it changes.
	SinceFetchID string
}

// SameListing reports whether e and o list their domain alike.
func (e DomainListEntry) SameListing(o DomainListEntry) bool {
	return e.Domain == o.Domain && e.Severity == o.Severity && e.RejectMedia == o.RejectMedia &&
		e.RejectReports == o.RejectReports && e.PublicComment == o.PublicComment && e.Obfuscate == o.Obfuscate
}

// UnknownSeverity reports whether the severity of e is not one that this server knows: its
// actions are always drafts.
func (e DomainListEntry) UnknownSeverity() bool { return !e.Severity.Valid() }

// Kinds returns the kinds of the actions that e stands for under s, the severity first: suspend
// as s.SuspendAs, silence as s.SilenceAs, noop as none, and a severity this server does not know
// as a suspend; then reject_media and reject_reports when the row and s both set them.
func (s DomainListSettings) Kinds(e DomainListEntry) []DomainActionKind {
	var out []DomainActionKind
	mapping := DomainListAsIgnore
	switch {
	case e.Severity == DomainBlockSuspend || e.UnknownSeverity():
		mapping = s.SuspendAs
	case e.Severity == DomainBlockSilence:
		mapping = s.SilenceAs
	}
	if k, ok := mapping.kind(); ok {
		out = append(out, k)
	}
	if e.RejectMedia && s.ApplyRejectMedia {
		out = append(out, DomainRejectMedia)
	}
	if e.RejectReports && s.ApplyRejectReports {
		out = append(out, DomainRejectReports)
	}
	return out
}

// SubscriptionDomainAction records that a subscription issued a domain action
// (subscription_domain_actions, an event): automatically in a fetch (FetchID), or as a draft a
// moderator accepted (FetchID is then the subscription's last fetch, "" for none).
type SubscriptionDomainAction struct {
	ActionID       string
	SubscriptionID string
	FetchID        string
}

// DomainListChangeKind is whether a difference issues an action or lifts one.
type DomainListChangeKind string

// Kinds of differences.
const (
	DomainListIssue DomainListChangeKind = "issue"
	DomainListLift  DomainListChangeKind = "lift"
)

// Valid reports whether k is one of the kinds of differences.
func (k DomainListChangeKind) Valid() bool { return k == DomainListIssue || k == DomainListLift }

// DomainListHold is why a difference waits for a moderator.
type DomainListHold string

// Reasons to hold a difference.
const (
	// DomainListHoldDraftMode: the subscription is in the draft mode.
	DomainListHoldDraftMode DomainListHold = "draft_mode"
	// DomainListHoldUnknownSeverity: the list gives the domain a severity this server does not
	// know.
	DomainListHoldUnknownSeverity DomainListHold = "unknown_severity"
	// DomainListHoldMassLift: the fetch would lift more than MassLiftMin actions and more than
	// half of the subscription's actions.
	DomainListHoldMassLift DomainListHold = "mass_lift"
)

// DomainListChange is one difference between a subscription's entries and the actions it
// issued: an action of Kind to issue on Domain, or the subscription's action ActionID to lift.
type DomainListChange struct {
	Change DomainListChangeKind
	Domain string
	Kind   DomainActionKind
	// Entry is the listing that an issue comes from; the zero value for a lift.
	Entry DomainListEntry
	// ActionID is the action that a lift lifts; "" for an issue.
	ActionID string
	// Held is why the difference is a draft; "" for one the automatic mode applies.
	Held DomainListHold
}

// Key returns the key of the difference that the drafts are accepted and rejected by:
// "<change>:<kind>:<domain>".
func (c DomainListChange) Key() string { return DomainListChangeKey(c.Change, c.Kind, c.Domain) }

// DomainListChangeKey returns the key of a difference (DomainListChange.Key).
func DomainListChangeKey(change DomainListChangeKind, kind DomainActionKind, domain string) string {
	return string(change) + ":" + string(kind) + ":" + domain
}

// DomainListRejection is a draft that a moderator rejected, or an action of the subscription
// that a person lifted (domain_list_draft_rejections, an event). A rejected issue stays rejected
// while the entry lists the domain from the same fetch (EntrySinceFetchID); a rejected lift
// while the action stays in effect (ActionID).
type DomainListRejection struct {
	ID                string
	SubscriptionID    string
	Change            DomainListChangeKind
	Domain            string
	Kind              DomainActionKind
	EntrySinceFetchID string // for an issue; "" for a lift
	ActionID          string // for a lift; "" for an issue
	RejectedByUserID  string // "" for the CLI
	RejectedAt        time.Time
}

// rejects reports whether r rejects c.
func (r DomainListRejection) rejects(c DomainListChange) bool {
	if r.Change != c.Change || r.Domain != c.Domain || r.Kind != c.Kind {
		return false
	}
	if c.Change == DomainListIssue {
		return r.EntrySinceFetchID == c.Entry.SinceFetchID
	}
	return r.ActionID == c.ActionID
}

// DomainListPlan is what DiffDomainList reads: a subscription's settings, entries, the actions in
// effect that it issued, and the kinds of every action in effect by domain.
type DomainListPlan struct {
	Settings DomainListSettings
	Entries  []DomainListEntry
	// Own are the subscription's actions in effect.
	Own []DomainAction
	// Covered maps each domain with actions in effect, the subscription's included, to their
	// kinds.
	Covered    map[string][]DomainActionKind
	Rejections []DomainListRejection
}

// DiffDomainList returns the differences of a subscription, the lifts first, each in domain
// and kind order, without the rejected ones:
//
//   - an issue for each kind that an entry stands for (DomainListSettings.Kinds) when no action
//     of the kind is in effect on the domain or a parent domain, whoever issued it: an action a
//     person or another subscription issued counts as applied;
//   - a lift for each of the subscription's own actions whose domain and kind no entry stands
//     for. Only the subscription's own actions are ever lifted.
//
// Each difference is held when the mode is draft; in the automatic mode, the issues of an entry
// with an unknown severity are held, and every lift is held when there are more than
// MassLiftMin of them and they are more than half of the subscription's actions.
func DiffDomainList(p DomainListPlan) []DomainListChange {
	wanted := make(map[string]DomainListEntry, len(p.Entries))
	var issues []DomainListChange
	for _, e := range p.Entries {
		wanted[e.Domain] = e
	}
	own := make(map[string]bool, len(p.Own))
	for _, a := range p.Own {
		own[a.Domain+"\x00"+string(a.Kind)] = true
	}
	for _, e := range p.Entries {
		for _, k := range p.Settings.Kinds(e) {
			if own[e.Domain+"\x00"+string(k)] || CoveredBy(p.Covered, e.Domain, k) {
				continue
			}
			c := DomainListChange{Change: DomainListIssue, Domain: e.Domain, Kind: k, Entry: e}
			if e.UnknownSeverity() {
				c.Held = DomainListHoldUnknownSeverity
			}
			issues = append(issues, c)
		}
	}
	var lifts []DomainListChange
	for _, a := range p.Own {
		if e, ok := wanted[a.Domain]; ok && slices.Contains(p.Settings.Kinds(e), a.Kind) {
			continue
		}
		lifts = append(lifts, DomainListChange{Change: DomainListLift, Domain: a.Domain, Kind: a.Kind, ActionID: a.ID})
	}
	if len(lifts) > MassLiftMin && 2*len(lifts) > len(p.Own) {
		for i := range lifts {
			lifts[i].Held = DomainListHoldMassLift
		}
	}
	out := slices.Concat(lifts, issues)
	out = slices.DeleteFunc(out, func(c DomainListChange) bool {
		return slices.ContainsFunc(p.Rejections, func(r DomainListRejection) bool { return r.rejects(c) })
	})
	if p.Settings.Mode == DomainListDraft {
		for i := range out {
			if out[i].Held == "" {
				out[i].Held = DomainListHoldDraftMode
			}
		}
	}
	slices.SortStableFunc(out, func(a, b DomainListChange) int {
		if a.Change != b.Change {
			if a.Change == DomainListLift {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Domain+"\x00"+string(a.Kind), b.Domain+"\x00"+string(b.Kind))
	})
	return out
}

// CoveredBy reports whether an action of kind k in covered (domain → kinds in effect) covers
// domain, on it or on a parent domain.
func CoveredBy(covered map[string][]DomainActionKind, domain string, k DomainActionKind) bool {
	for _, d := range DomainAndParents(domain) {
		if slices.Contains(covered[d], k) {
			return true
		}
	}
	return false
}

// mastodonDomainBlock is one object of Mastodon's GET /api/v1/instance/domain_blocks.
type mastodonDomainBlock struct {
	Domain   string `json:"domain"`
	Severity string `json:"severity"`
	Comment  string `json:"comment"`
}

// ParseDomainList reads a list in format as rows of Mastodon's domain block CSV, with the
// parser of the import (ParseDomainBlockCSV) for mastodon_csv and plain_text, whose lines it
// reads as domains to suspend. mastodon_json becomes the same rows: each object's severity as
// #severity (a missing one is suspend) and its comment as #public_comment, without the domains
// that Mastodon obfuscates (those with "*"). A row whose severity is not one the CSV knows keeps
// it, marked Invalid. It fails with ErrMalformedDomainList for a list that the format cannot
// read, and with ErrTooManyDomainBlocks for more than MaxDomainBlockImport rows.
func ParseDomainList(format DomainListFormat, body []byte) ([]DomainBlockRow, error) {
	switch format {
	case DomainListMastodonCSV, DomainListPlainText:
		// The parser reads a file without the #domain header as a list of domains to suspend,
		// one per row in the first column: a plain list.
		rows, err := ParseDomainBlockCSV(string(body))
		if errors.Is(err, ErrMalformedDomainBlockCSV) {
			return nil, fmt.Errorf("%w: %w", ErrMalformedDomainList, err)
		}
		return rows, err
	case DomainListMastodonJSON:
		var blocks []mastodonDomainBlock
		if err := json.Unmarshal(body, &blocks); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrMalformedDomainList, err)
		}
		if len(blocks) > MaxDomainBlockImport {
			return nil, ErrTooManyDomainBlocks
		}
		out := make([]DomainBlockRow, 0, len(blocks))
		for i, b := range blocks {
			domain := strings.TrimSpace(b.Domain)
			if domain == "" || strings.Contains(domain, "*") {
				continue
			}
			row := DomainBlockRow{Line: i + 1, DomainBlock: DomainBlock{
				Domain: domain, Severity: DomainBlockSeverity(strings.ToLower(strings.TrimSpace(b.Severity))),
				PublicComment: strings.TrimSpace(b.Comment),
			}}
			if row.Severity == "" {
				row.Severity = DomainBlockSuspend
			}
			row.Invalid = !row.Severity.Valid()
			out = append(out, row)
		}
		return out, nil
	}
	return nil, invalidDomainList("format")
}

// MaxListedSeverity is the longest severity as listed that an entry keeps, in bytes; a longer
// one is cut, and stays unknown.
const MaxListedSeverity = 32

// DomainListSubscriptionRecord is a subscription with its settings in effect (the newest
// revision), its head, its last fetch (nil before the first), and its deletion (nil while it is
// not deleted).
type DomainListSubscriptionRecord struct {
	Subscription DomainListSubscription
	Revision     DomainListRevision
	Head         DomainListHead
	LastFetch    *DomainListFetch
	Deletion     *DomainListDeletion
}
