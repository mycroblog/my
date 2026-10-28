// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// The account_search_terms projection (ADR-0092 §1, FR-307, data-model §4.2): the words that
// the search of accounts matches by their start. It is synchronous (ADR-0020 rule 7): the projector rewrites the
// rows of an account from its handle, its profile head, its lifecycle state, and whether the
// moderation module suspends it, whenever one of them changes.

// SearchTermRepository is the part of the account module's persistence that keeps and reads the
// account_search_terms projection. The database adapters implement it and pass
// testkit.AccountSearchContract.
type SearchTermRepository interface {
	// ReplaceSearchTerms replaces the search terms of an account with terms (none deletes them).
	ReplaceSearchTerms(ctx context.Context, tx uow.Tx, accountID string, terms []account.SearchTerm) error
	DeleteAllSearchTerms(ctx context.Context, tx uow.Tx) error
	// ListAccountIDsWithUsernameTerm returns inside tx, in ID order, the accounts that have a
	// username term.
	ListAccountIDsWithUsernameTerm(ctx context.Context, tx uow.Tx, term string) ([]string, error)
	// ListHandleAccountIDs returns inside tx, in ID order, the accounts that have a handle.
	ListHandleAccountIDs(ctx context.Context, tx uow.Tx) ([]string, error)
	// ListSearchTerms returns every search term, ordered by account ID, kind, and term, without
	// Domain.
	ListSearchTerms(ctx context.Context) ([]account.SearchTerm, error)
	// SearchTerms returns up to limit search terms in [low, high), in term and account ID order,
	// with the domain of their accounts.
	SearchTerms(ctx context.Context, low, high string, limit int) ([]account.SearchTerm, error)
}

// SuspendedFunc reports inside tx whether the moderation module suspends an account (the
// federation restriction of suspend and ban, from the account's own actions). The composition
// root passes the moderation module's AccountPolicy.SuspendedTx: the account module does not
// import it, since the moderation module imports the account module.
type SuspendedFunc func(ctx context.Context, tx uow.Tx, accountID string) (bool, error)

// SearchTermProjector keeps the account_search_terms projection.
type SearchTermProjector struct {
	repo      Repository
	suspended SuspendedFunc
}

// NewSearchTermProjector returns the projector on repo; a nil suspended suspends no account.
func NewSearchTermProjector(repo Repository, suspended SuspendedFunc) *SearchTermProjector {
	if suspended == nil {
		suspended = func(context.Context, uow.Tx, string) (bool, error) { return false, nil }
	}
	return &SearchTermProjector{repo: repo, suspended: suspended}
}

// Register registers the projector for the account module's events that change a search term:
// a local account's creation, a profile revision, a lifecycle event, and a remote handle check.
// The composition root also calls Refresh on the moderation module's
// EventAccountSuspensionChanged. Register it after RegisterProjectors, whose projections it
// reads.
func (p *SearchTermProjector) Register(r *uow.Projectors) {
	r.Register(EventLocalAccountCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return p.Refresh(ctx, tx, e.(LocalAccountCreated).Account.ID)
	})
	r.Register(EventProfileRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return p.Refresh(ctx, tx, e.(ProfileRevised).Revision.AccountID)
	})
	r.Register(EventLifecycleEventRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return p.Refresh(ctx, tx, e.(LifecycleEventRecorded).Event.AccountID)
	})
	r.Register(EventHandleChecked, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		v := e.(HandleChecked).Verification
		ids := []string{v.AccountID}
		// A verified handle is taken from the account that verified it before (applyHandleCheck),
		// which loses its handle and so its terms.
		if h, ok := v.Handle(); ok {
			held, err := p.repo.ListAccountIDsWithUsernameTerm(ctx, tx, remoteHandleTerm(h))
			if err != nil {
				return err
			}
			ids = append(ids, held...)
		}
		for _, id := range ids {
			if err := p.Refresh(ctx, tx, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// Refresh rewrites inside tx the search terms of an account from what tx sees.
func (p *SearchTermProjector) Refresh(ctx context.Context, tx uow.Tx, accountID string) error {
	terms, err := p.derive(ctx, tx, accountID)
	if err != nil {
		return err
	}
	return p.repo.ReplaceSearchTerms(ctx, tx, accountID, terms)
}

// derive returns the search terms that an account has now: none when it is unknown, an
// Application actor, without a handle, not active, or suspended.
func (p *SearchTermProjector) derive(ctx context.Context, tx uow.Tx, accountID string) ([]account.SearchTerm, error) {
	a, err := p.repo.FindAccountTx(ctx, tx, accountID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if a.ActorType == account.ActorApplication {
		return nil, nil
	}
	h, err := p.repo.FindHandleOfTx(ctx, tx, accountID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	state, err := p.repo.FindLifecycleStateTx(ctx, tx, accountID)
	switch {
	case errors.Is(err, ErrNotFound):
	case err != nil:
		return nil, err
	case !state.Active():
		return nil, nil
	}
	suspended, err := p.suspended(ctx, tx, accountID)
	if err != nil || suspended {
		return nil, err
	}
	displayName, discoverable := "", false
	switch rev, err := p.repo.FindCurrentProfileTx(ctx, tx, accountID); {
	case err == nil:
		displayName, discoverable = rev.DisplayName, rev.Discoverable
	case !errors.Is(err, ErrNotFound):
		return nil, err
	}
	return SearchTerms(h, a.IsLocal(), discoverable, displayName), nil
}

// Rebuild empties the account_search_terms projection and derives it again for every account
// with a handle, in one transaction (data-model §9).
func (p *SearchTermProjector) Rebuild(ctx context.Context, u uow.UnitOfWork) error {
	return u.Tx(ctx, func(tx uow.Tx) error {
		if err := p.repo.DeleteAllSearchTerms(ctx, tx); err != nil {
			return err
		}
		ids, err := p.repo.ListHandleAccountIDs(ctx, tx)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := p.Refresh(ctx, tx, id); err != nil {
				return err
			}
		}
		return nil
	})
}

const (
	// MaxSearchQueryLength is the longest query of the prefix search, in characters after the
	// leading @; a term is cut to the same length, which no longer query could match beyond.
	MaxSearchQueryLength = 100
	// maxDisplayNameWords bounds the words of one display name that become terms.
	maxDisplayNameWords = 20
)

// SearchTerms returns the search terms of an account with handle h (ADR-0092 §1, FR-307): the
// normalized username, username@domain for a remote account, and the display name under
// NormalizeSearchText without its :shortcode:s, whole and each of its words separated by white
// space. A CJK display name has no spaces to split at, so it matches from its start.
//
// A local account that is discoverable also has the suffixes of its username and its whole
// display name that start after their first character (SearchTermInfix, none starting with a
// space), so that a query matches the middle of its names. A local account that is not
// discoverable has its username only, as SearchTermExactUsername. A remote account's
// discoverable does not matter.
func SearchTerms(h account.Handle, local, discoverable bool, displayName string) []account.SearchTerm {
	var out []account.SearchTerm
	add := func(kind account.SearchTermKind, term string) {
		term = cutRunes(term, MaxSearchQueryLength)
		if term == "" {
			return
		}
		for _, t := range out {
			if t.Kind == kind && t.Term == term {
				return
			}
		}
		out = append(out, account.SearchTerm{AccountID: h.AccountID, Kind: kind, Term: term, IsLocal: local})
	}
	if local && !discoverable {
		add(account.SearchTermExactUsername, h.UsernameNormalized)
		return out
	}
	add(account.SearchTermUsername, h.UsernameNormalized)
	if !local {
		add(account.SearchTermUsername, remoteHandleTerm(h))
	}
	words := strings.Fields(NormalizeSearchText(stripShortcodes(displayName)))
	whole := cutRunes(strings.Join(words, " "), MaxSearchQueryLength)
	add(account.SearchTermDisplayName, whole)
	if len(words) > 1 {
		for _, w := range words[:min(len(words), maxDisplayNameWords)] {
			add(account.SearchTermDisplayName, w)
		}
	}
	if local {
		for _, name := range []string{h.UsernameNormalized, whole} {
			for i, r := range name {
				if i == 0 || r == ' ' {
					continue
				}
				add(account.SearchTermInfix, name[i:])
			}
		}
	}
	return out
}

// remoteHandleTerm is the username term username@domain of a remote handle.
func remoteHandleTerm(h account.Handle) string { return h.UsernameNormalized + "@" + h.Domain }

// NormalizeSearchText returns the form in which the prefix search compares text: NFKC, then
// lowercase, as hashtags compare (ADR-0009 §6).
func NormalizeSearchText(s string) string {
	return strings.ToLower(norm.NFKC.String(s))
}

func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for range n {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return s[:i]
}

// stripShortcodes replaces each :shortcode: of s with a space. It finds them as the content
// module's ScanShortcodes does (Mastodon's CustomEmoji::SCAN_RE): two or more ASCII letters,
// digits, and underscores between colons, with neither a letter, a digit, nor a colon right
// before the opening colon or after the closing one.
func stripShortcodes(s string) string {
	var b strings.Builder
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] != ':' || i > 0 && blocksShortcode(s, i, true) {
			continue
		}
		j := i + 1
		for j < len(s) && shortcodeByte(s[j]) {
			j++
		}
		if j >= len(s) || s[j] != ':' || j-i-1 < 2 || j+1 < len(s) && blocksShortcode(s, j+1, false) {
			continue
		}
		b.WriteString(s[last:i])
		b.WriteByte(' ')
		last = j + 1
		i = j
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

func shortcodeByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

// blocksShortcode reports whether the character before (before is true) the byte at i of s, or
// the one starting at i, is a letter, a digit, or a colon.
func blocksShortcode(s string, i int, before bool) bool {
	var r rune
	if before {
		r, _ = utf8.DecodeLastRuneInString(s[:i])
	} else {
		r, _ = utf8.DecodeRuneInString(s[i:])
	}
	return r == ':' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
