// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

// The imports and CSV exports of a user's follows, blocks, mutes, bookmarks, and muted domains
// in the CSV
// formats of Mastodon (FR-008, data-model §4.1): the files that Mastodon's "Import and export"
// page exports can be imported here, and the files exported here can be imported by Mastodon.

// ImportKind is what a CSV file lists (data_imports.kind).
type ImportKind string

// Import kinds, each with the file that Mastodon exports.
const (
	// ImportFollowing is the accounts the user follows (following_accounts.csv).
	ImportFollowing ImportKind = "following"
	// ImportBlocks is the accounts the user blocks (blocked_accounts.csv).
	ImportBlocks ImportKind = "blocks"
	// ImportMutes is the accounts the user mutes (muted_accounts.csv).
	ImportMutes ImportKind = "mutes"
	// ImportBookmarks is the posts the user bookmarked (bookmarks.csv).
	ImportBookmarks ImportKind = "bookmarks"
	// ImportDomainMutes is the domains the user mutes (FR-304), as the domains a Mastodon user
	// blocks (blocked_domains.csv).
	ImportDomainMutes ImportKind = "domain_mutes"
	// ImportFollowedTags is the hashtags the user follows (followed_hashtags.csv, one hashtag
	// per line without its #). Mastodon exports no such file.
	ImportFollowedTags ImportKind = "followed_tags"
)

// ImportKinds are the kinds, in the order of the settings page.
var ImportKinds = []ImportKind{ImportFollowing, ImportFollowedTags, ImportBlocks, ImportMutes, ImportDomainMutes, ImportBookmarks}

// Valid reports whether k is a known kind.
func (k ImportKind) Valid() bool {
	switch k {
	case ImportFollowing, ImportBlocks, ImportMutes, ImportBookmarks, ImportDomainMutes, ImportFollowedTags:
		return true
	}
	return false
}

// FileName returns the name Mastodon gives the exported file of k, and for followed hashtags,
// which Mastodon does not export, the name of this server.
func (k ImportKind) FileName() string {
	switch k {
	case ImportFollowing:
		return "following_accounts.csv"
	case ImportBlocks:
		return "blocked_accounts.csv"
	case ImportMutes:
		return "muted_accounts.csv"
	case ImportBookmarks:
		return "bookmarks.csv"
	case ImportDomainMutes:
		return "blocked_domains.csv"
	case ImportFollowedTags:
		return "followed_hashtags.csv"
	}
	return ""
}

// ImportMode is how an import treats what the user has now (data_imports.mode).
type ImportMode string

// Import modes.
const (
	// ImportMerge adds the file's entries and keeps the others.
	ImportMerge ImportMode = "merge"
	// ImportOverwrite adds the file's entries and removes the others: the user ends up with
	// exactly the file's entries.
	ImportOverwrite ImportMode = "overwrite"
)

// Valid reports whether m is a known mode.
func (m ImportMode) Valid() bool { return m == ImportMerge || m == ImportOverwrite }

// MaxImportEntries is the number of entries that one file may hold.
const MaxImportEntries = 10000

// ErrTooManyImportEntries reports a file with more than MaxImportEntries entries.
var ErrTooManyImportEntries = errors.New("identity: the file has too many entries")

// The column headers of Mastodon's CSV files. A file whose first field is one of the known first
// headers starts with a header row; otherwise its columns are the default ones of its kind.
const (
	headerAccountAddress    = "Account address"
	headerShowBoosts        = "Show boosts"
	headerNotify            = "Notify on new posts"
	headerLanguages         = "Languages"
	headerHideNotifications = "Hide notifications"
	headerURI               = "#uri"
	headerDomain            = "#domain"
	headerListName          = "List name"
	// headerHashtag is the column of the hashtags of followed_hashtags.csv, which has no header
	// row.
	headerHashtag = "#hashtag"
)

// defaultHeaders returns the columns of a file of kind without a header row.
func defaultHeaders(k ImportKind) []string {
	switch k {
	case ImportFollowing:
		return []string{headerAccountAddress, headerShowBoosts, headerNotify, headerLanguages}
	case ImportMutes:
		return []string{headerAccountAddress, headerHideNotifications}
	case ImportBookmarks:
		return []string{headerURI}
	case ImportDomainMutes:
		return []string{headerDomain}
	case ImportFollowedTags:
		return []string{headerHashtag}
	}
	return []string{headerAccountAddress}
}

// isKnownFirstHeader reports whether a first field names a column, so that the row is a header.
func isKnownFirstHeader(field string) bool {
	switch field {
	case headerAccountAddress, headerURI, headerDomain, headerListName:
		return true
	}
	return false
}

// ImportEntry is one entry of a CSV file: an account address (username@domain, or a bare
// username for this instance's account), for bookmarks the URL of a post, for domain mutes a
// domain, or for followed hashtags a hashtag, with the settings that its kind carries.
type ImportEntry struct {
	// Line is the line of the file, from 1.
	Line int
	// Target is the account address without a leading "@", the URL of the post, the domain, or
	// the hashtag without a leading "#".
	Target string
	// ShowBoosts and Notify are the settings of a follow.
	ShowBoosts bool
	Notify     bool
	// HideNotifications is the setting of a mute.
	HideNotifications bool
}

// ImportSkipReason is why a line of a file is not an entry.
type ImportSkipReason string

// Import skip reasons.
const (
	// ImportSkipInvalid is a line that names no account address, post URL, or domain.
	ImportSkipInvalid ImportSkipReason = "invalid"
	// ImportSkipDuplicate is a line whose target an earlier line names.
	ImportSkipDuplicate ImportSkipReason = "duplicate"
)

// ImportSkipped is a line of a file that is not an entry.
type ImportSkipped struct {
	Line   int
	Value  string
	Reason ImportSkipReason
}

// ParseImportCSV reads a CSV file of kind as Mastodon writes it, and as Mastodon's import reads
// it: a header row is optional, a leading "@" of an address is dropped, blank lines are
// ignored, and the settings default as in Mastodon (boosts shown, no notifications of new
// posts, notifications of a muted account hidden). It returns the entries in file order and the
// lines it skipped. More than MaxImportEntries entries fail with ErrTooManyImportEntries; a file
// that is not CSV fails with the reader's error.
func ParseImportCSV(k ImportKind, text string) ([]ImportEntry, []ImportSkipped, error) {
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(text, "\uFEFF")))
	r.FieldsPerRecord = -1
	headers := defaultHeaders(k)
	var (
		entries []ImportEntry
		skipped []ImportSkipped
		seen    = map[string]bool{}
		first   = true
	)
	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("identity: read the CSV file: %w", err)
		}
		line, _ := r.FieldPos(0)
		if first {
			first = false
			if isKnownFirstHeader(strings.TrimSpace(record[0])) {
				headers = make([]string, len(record))
				for i, h := range record {
					headers[i] = strings.TrimSpace(h)
				}
				continue
			}
		}
		fields := map[string]string{}
		for i, h := range headers {
			if i < len(record) {
				fields[h] = strings.TrimSpace(record[i])
			}
		}
		e := ImportEntry{
			Line:              line,
			ShowBoosts:        !strings.EqualFold(fields[headerShowBoosts], "false"),
			Notify:            strings.EqualFold(fields[headerNotify], "true"),
			HideNotifications: !strings.EqualFold(fields[headerHideNotifications], "false"),
		}
		var key string
		var ok bool
		switch k {
		case ImportBookmarks:
			e.Target = fields[headerURI]
			key, ok = e.Target, isPostURL(e.Target)
		case ImportDomainMutes:
			e.Target = fields[headerDomain]
			key, ok = DomainKey(e.Target)
		case ImportFollowedTags:
			e.Target = strings.TrimPrefix(fields[headerHashtag], "#")
			key, ok = HashtagKey(e.Target)
		default:
			e.Target = strings.TrimPrefix(fields[headerAccountAddress], "@")
			key, ok = AddressKey(e.Target)
		}
		switch {
		case e.Target == "" && len(record) == 1:
			continue // a blank field on its own
		case !ok:
			skipped = append(skipped, ImportSkipped{Line: line, Value: strings.Join(record, ","), Reason: ImportSkipInvalid})
		case seen[key]:
			skipped = append(skipped, ImportSkipped{Line: line, Value: e.Target, Reason: ImportSkipDuplicate})
		default:
			seen[key] = true
			entries = append(entries, e)
			if len(entries) > MaxImportEntries {
				return nil, nil, ErrTooManyImportEntries
			}
		}
	}
	return entries, skipped, nil
}

// SplitAddress splits an account address, "username@domain" or a bare "username" of this
// instance (domain ""), with an optional leading "@". It reports false for anything else.
func SplitAddress(address string) (username, domain string, ok bool) {
	username, domain, hasDomain := strings.Cut(strings.TrimPrefix(address, "@"), "@")
	if username == "" || (hasDomain && domain == "") ||
		strings.ContainsAny(username, " \t/?#:") || strings.ContainsAny(domain, " \t/?#@") {
		return "", "", false
	}
	return username, domain, true
}

// AddressKey returns the form of an account address that identifies its account, to compare
// addresses: the username and the domain in lower case. It reports false for what is not an
// address.
func AddressKey(address string) (string, bool) {
	username, domain, ok := SplitAddress(address)
	if !ok {
		return "", false
	}
	return strings.ToLower(username) + "@" + strings.ToLower(domain), true
}

// DomainKey returns the form of a domain of a file that identifies it, to compare domains: in
// lower case without a trailing dot. It reports false for what is not a host name: empty, or
// with a scheme, path, port, user, or space.
func DomainKey(domain string) (string, bool) {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))
	if d == "" || strings.ContainsAny(d, " \t/?#@:") || strings.HasPrefix(d, ".") || strings.Contains(d, "..") {
		return "", false
	}
	return d, true
}

// HashtagKey returns the form of a hashtag of a file that identifies it, to compare hashtags: in
// lower case without a leading "#". It reports false for what cannot be a hashtag: empty, or with
// a space or punctuation that ends a hashtag in a text. The content module decides whether the
// rest is a hashtag when the import follows it.
func HashtagKey(name string) (string, bool) {
	h := strings.ToLower(strings.TrimPrefix(name, "#"))
	if h == "" || strings.ContainsAny(h, " \t#@/?:,.") {
		return "", false
	}
	return h, true
}

// isPostURL reports whether s is an absolute http(s) URL, as the URIs of posts are.
func isPostURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && !strings.ContainsAny(s, " \t")
}

// ExportRow is an entry of a CSV export: an account address, for bookmarks the URI of a post,
// for domain mutes a domain, or for followed hashtags a hashtag without its #, with the settings
// of its kind.
type ExportRow struct {
	Target            string
	ShowBoosts        bool
	Notify            bool
	HideNotifications bool
}

// WriteExportCSV writes rows as the CSV file of kind that Mastodon exports: following and
// mutes with their header row, blocks, bookmarks, and domains one value per line; followed
// hashtags, which Mastodon does not export, one hashtag per line as well.
func WriteExportCSV(w io.Writer, k ImportKind, rows []ExportRow) error {
	cw := csv.NewWriter(w)
	flag := func(b bool) string {
		if b {
			return "true"
		}
		return "false"
	}
	switch k {
	case ImportFollowing:
		if err := cw.Write([]string{headerAccountAddress, headerShowBoosts, headerNotify, headerLanguages}); err != nil {
			return err
		}
	case ImportMutes:
		if err := cw.Write([]string{headerAccountAddress, headerHideNotifications}); err != nil {
			return err
		}
	}
	for _, row := range rows {
		var record []string
		switch k {
		case ImportFollowing:
			record = []string{row.Target, flag(row.ShowBoosts), flag(row.Notify), ""}
		case ImportMutes:
			record = []string{row.Target, flag(row.HideNotifications)}
		default:
			record = []string{row.Target}
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// ExportCSV returns the CSV file of kind for rows (WriteExportCSV).
func ExportCSV(k ImportKind, rows []ExportRow) ([]byte, error) {
	var b bytes.Buffer
	err := WriteExportCSV(&b, k, rows)
	return b.Bytes(), err
}

// ImportAction is what an item of an import does (data_import_items.action).
type ImportAction string

// Import actions.
const (
	// ImportAdd adds the entry of a line of the file.
	ImportAdd ImportAction = "add"
	// ImportRemove removes what the user had that the file of an overwrite does not list.
	ImportRemove ImportAction = "remove"
)

// DataImport is a user's import of a CSV file (data_imports): its items are what the file
// adds and, for an overwrite, what it removes, which a job carries out one by one.
type DataImport struct {
	ID        string
	UserID    string
	AccountID string
	Kind      ImportKind
	Mode      ImportMode
	CreatedAt time.Time
}

// DataImportItem is one thing an import does (data_import_items). An ImportAdd item holds the
// entry of a line, its Target the address or URL as the file wrote it; an ImportRemove item's
// Target is the ID of the account or status to remove.
type DataImportItem struct {
	ImportID          string
	Position          int
	Action            ImportAction
	Target            string
	ShowBoosts        bool
	Notify            bool
	HideNotifications bool
}

// ImportOutcome is how an item of an import ended (data_import_item_results.outcome).
type ImportOutcome string

// Import outcomes.
const (
	// ImportDone is an item carried out, or found already done.
	ImportDone ImportOutcome = "done"
	// ImportFailed is an item that could not be carried out; its ImportFailure says why.
	ImportFailed ImportOutcome = "failed"
)

// ImportFailure is why an item of an import failed.
type ImportFailure string

// Import failures.
const (
	// ImportFailureUnresolvable: the account or post could not be found here or on its server,
	// or the domain is not one that can be muted.
	ImportFailureUnresolvable ImportFailure = "unresolvable"
	// ImportFailureUnavailable: the account or post exists but cannot be acted on now (it is
	// suspended, deleted, or not visible to the user).
	ImportFailureUnavailable ImportFailure = "unavailable"
	// ImportFailureBlocked: a block between the user and the account stops a follow.
	ImportFailureBlocked ImportFailure = "blocked"
	// ImportFailureRestricted: a moderation action stops the user from acting.
	ImportFailureRestricted ImportFailure = "restricted"
	// ImportFailureSelf: the entry is the user's own account.
	ImportFailureSelf ImportFailure = "self"
	// ImportFailureLimit: the user already has the most of the kind allowed (the muted domains).
	ImportFailureLimit ImportFailure = "limit"
)

// Valid reports whether f is a known failure.
func (f ImportFailure) Valid() bool {
	switch f {
	case ImportFailureUnresolvable, ImportFailureUnavailable, ImportFailureBlocked, ImportFailureRestricted, ImportFailureSelf, ImportFailureLimit:
		return true
	}
	return false
}

// DataImportResult is the end of an item of an import (data_import_item_results). Failure is
// set exactly for ImportFailed.
type DataImportResult struct {
	ImportID    string
	Position    int
	Outcome     ImportOutcome
	Failure     ImportFailure
	ProcessedAt time.Time
}

// DataImportProgress is an import with the numbers of its items.
type DataImportProgress struct {
	DataImport
	// Total is the number of items; Processed of those with a result, Failed of those that
	// failed.
	Total, Processed, Failed int
}

// Finished reports whether every item of the import has a result.
func (p DataImportProgress) Finished() bool { return p.Processed >= p.Total }
