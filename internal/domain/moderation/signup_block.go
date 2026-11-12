// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

// The sign-up block list (FR-719): blocks on sign-ups from the registration page
// by the domain of the email address, the address itself (kept only as a hash), or the client's
// IP range. A block in effect either refuses the sign-up or makes it wait for a moderator.

// SignupBlockKind is what a sign-up block matches (signup_blocks.kind).
type SignupBlockKind string

// Kinds of sign-up blocks.
const (
	// SignupBlockEmailDomain matches the domain of the email address and its subdomains.
	SignupBlockEmailDomain SignupBlockKind = "email_domain"
	// SignupBlockEmailHash matches one email address by the hash of its canonical form
	// (EmailHash), so that the address itself is not kept.
	SignupBlockEmailHash SignupBlockKind = "email_hash"
	// SignupBlockIPRange matches the client addresses in a CIDR prefix.
	SignupBlockIPRange SignupBlockKind = "ip_range"
)

// Valid reports whether k is one of the kinds of sign-up blocks.
func (k SignupBlockKind) Valid() bool {
	switch k {
	case SignupBlockEmailDomain, SignupBlockEmailHash, SignupBlockIPRange:
		return true
	}
	return false
}

// SignupBlockSeverity is what a sign-up block does to the sign-ups it matches
// (signup_blocks.severity).
type SignupBlockSeverity string

// Severities of sign-up blocks, weakest first.
const (
	// SignupRequireApproval starts the registration pending, whatever the registration mode.
	SignupRequireApproval SignupBlockSeverity = "require_approval"
	// SignupReject refuses the sign-up.
	SignupReject SignupBlockSeverity = "reject"
)

// Valid reports whether s is one of the severities.
func (s SignupBlockSeverity) Valid() bool { return s == SignupReject || s == SignupRequireApproval }

// Stronger returns the stronger of s and t; "" is weaker than both.
func (s SignupBlockSeverity) Stronger(t SignupBlockSeverity) SignupBlockSeverity {
	if s == SignupReject || t == SignupReject {
		return SignupReject
	}
	if s == SignupRequireApproval || t == SignupRequireApproval {
		return SignupRequireApproval
	}
	return ""
}

// MaxSignupBlockComment is the number of characters of a sign-up block's comment.
const MaxSignupBlockComment = 1000

// ErrInvalidSignupBlock reports a sign-up block with a missing or invalid field.
var ErrInvalidSignupBlock = errors.New("moderation: invalid sign-up block")

// SignupBlock is one block on sign-ups (signup_blocks, an event). It is immutable; lifting it is
// a separate SignupBlockLift event, and an expired block is not lifted: it stops matching at
// ExpiresAt.
type SignupBlock struct {
	ID   string
	Kind SignupBlockKind
	// Value is an IDNA A-label domain for SignupBlockEmailDomain, the hex EmailHash for
	// SignupBlockEmailHash, and a masked CIDR prefix (192.0.2.0/24) for SignupBlockIPRange.
	Value    string
	Severity SignupBlockSeverity
	Comment  string // "" for none
	// ModerationActionID is the ban the block was made from; "" for none.
	ModerationActionID string
	CreatedByUserID    string     // "" for the CLI
	ExpiresAt          *time.Time // nil: until lifted
	CreatedAt          time.Time
}

// Validate checks the invariants of a new block: a known kind and severity, a value in the
// canonical form of its kind, a valid comment, and an end after its start.
func (b SignupBlock) Validate() error {
	if err := CheckSignupBlockComment(b.Comment); err != nil {
		return err
	}
	switch {
	case b.ID == "" || b.CreatedAt.IsZero():
		return fmt.Errorf("%w: a field is empty", ErrInvalidSignupBlock)
	case !b.Kind.Valid():
		return fmt.Errorf("%w: kind %q", ErrInvalidSignupBlock, b.Kind)
	case !b.Severity.Valid():
		return fmt.Errorf("%w: severity %q", ErrInvalidSignupBlock, b.Severity)
	case !ValidSignupBlockValue(b.Kind, b.Value):
		return fmt.Errorf("%w: %q is not a canonical %s", ErrInvalidSignupBlock, b.Value, b.Kind)
	case b.ExpiresAt != nil && !b.ExpiresAt.After(b.CreatedAt):
		return fmt.Errorf("%w: it must expire after it is created", ErrInvalidSignupBlock)
	}
	return nil
}

// CheckSignupBlockComment checks the comment of a block: a *TextError wrapped with
// ErrInvalidSignupBlock when it is too long or contains U+0000.
func CheckSignupBlockComment(c string) error {
	if err := checkText("comment", c, MaxSignupBlockComment); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSignupBlock, err)
	}
	return nil
}

// ValidSignupBlockValue reports whether v is a value of kind k in its canonical form.
func ValidSignupBlockValue(k SignupBlockKind, v string) bool {
	switch k {
	case SignupBlockEmailDomain:
		return ValidDomain(v) && !strings.Contains(v, "_")
	case SignupBlockEmailHash:
		if len(v) != 2*sha256.Size || strings.ToLower(v) != v {
			return false
		}
		_, err := hex.DecodeString(v)
		return err == nil
	case SignupBlockIPRange:
		p, err := netip.ParsePrefix(v)
		return err == nil && p == p.Masked() && p.String() == v
	}
	return false
}

// Matches reports whether the block matches a sign-up whose email address has the canonical
// domain domain and the hash hash (EmailHash), from the client address ip (invalid when
// unknown). It does not look at whether the block is in effect.
func (b SignupBlock) Matches(domain, hash string, ip netip.Addr) bool {
	switch b.Kind {
	case SignupBlockEmailDomain:
		return domain != "" && (domain == b.Value || strings.HasSuffix(domain, "."+b.Value))
	case SignupBlockEmailHash:
		return hash != "" && hash == b.Value
	case SignupBlockIPRange:
		p, err := netip.ParsePrefix(b.Value)
		return err == nil && ip.IsValid() && p.Contains(ip.Unmap())
	}
	return false
}

// SignupBlockLift is the event that a sign-up block was lifted (signup_block_lifts).
type SignupBlockLift struct {
	BlockID        string
	LiftedByUserID string // "" for the CLI
	LiftedAt       time.Time
}

// SignupBlockRecord is a sign-up block with its lift, as the moderators list it.
type SignupBlockRecord struct {
	Block SignupBlock
	Lift  *SignupBlockLift // nil until lifted
}

// State returns whether the block is in effect at now.
func (r SignupBlockRecord) State(now time.Time) ActionState {
	return stateAt(r.Block.ExpiresAt, r.Lift != nil, now)
}

// SignupDomains returns the canonical domain of an email address and its parent domains, the
// values of the email_domain blocks that match it: mail.spam.example gives mail.spam.example,
// spam.example, and example.
func SignupDomains(domain string) []string {
	if domain == "" {
		return nil
	}
	out := []string{domain}
	for {
		i := strings.IndexByte(domain, '.')
		if i < 0 {
			return out
		}
		domain = domain[i+1:]
		if domain != "" {
			out = append(out, domain)
		}
	}
}

// EmailHash returns the value of the email_hash block of an address whose domain is already in
// canonical form (IDNA A-labels): the hex SHA-256 of the address in lower case, without the part
// of the local part from its first "+", so that alice+spam@example.com and Alice@example.com hash
// the same.
func EmailHash(address string) string {
	a := strings.ToLower(address)
	if at := strings.LastIndexByte(a, '@'); at >= 0 {
		local, domain := a[:at], a[at+1:]
		if plus := strings.IndexByte(local, '+'); plus >= 0 {
			local = local[:plus]
		}
		a = local + "@" + domain
	}
	sum := sha256.Sum256([]byte(a))
	return hex.EncodeToString(sum[:])
}

// SignupBlockLabel returns the display name of a block in the audit log: its kind and value,
// such as "email_domain spam.example".
func SignupBlockLabel(b SignupBlock) string { return string(b.Kind) + " " + b.Value }

// MaxSignupBlockImport is the number of values that one import of a list of sign-up blocks
// (ParseSignupBlockList) may hold, enough for the public lists of disposable email domains.
const MaxSignupBlockImport = 10000

// SignupBlockListEntry is one value of a list of sign-up blocks and the line it is on, from 1.
type SignupBlockListEntry struct {
	Line  int
	Value string
}

// ParseSignupBlockList returns the values of a list of sign-up blocks: one value per line, or
// the first field of a CSV row (quotes around it are dropped). Blank lines and lines whose first
// character other than spaces is "#" are skipped, as is a UTF-8 byte order mark at the start.
// It does not validate the values.
func ParseSignupBlockList(text string) []SignupBlockListEntry {
	text = strings.TrimPrefix(text, "\uFEFF")
	var out []SignupBlockListEntry
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if c := strings.IndexByte(line, ','); c >= 0 {
			line = strings.TrimSpace(line[:c])
		}
		if len(line) >= 2 && line[0] == '"' && line[len(line)-1] == '"' {
			line = strings.TrimSpace(line[1 : len(line)-1])
		}
		out = append(out, SignupBlockListEntry{Line: i + 1, Value: line})
	}
	return out
}

// SignupBlockSkip is why an import of a list of sign-up blocks did not add the block of one of
// its values.
type SignupBlockSkip string

// Reasons to skip a value of an import.
const (
	// SignupBlockSkipInvalid: the value is not valid for the kind of the import.
	SignupBlockSkipInvalid SignupBlockSkip = "invalid"
	// SignupBlockSkipDuplicate: an earlier line of the list has the same canonical value.
	SignupBlockSkipDuplicate SignupBlockSkip = "duplicate"
	// SignupBlockSkipAlreadyBlocked: a block in effect has the same kind and canonical value and
	// a severity at least as strong.
	SignupBlockSkipAlreadyBlocked SignupBlockSkip = "already_blocked"
)
