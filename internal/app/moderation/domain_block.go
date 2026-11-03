// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"io"
	"slices"
	"strings"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The domain block list in Mastodon's CSV (FR-706): the export of the domain actions in
// effect, and the import of a list in two steps, a preview of what each row would do and the
// import of the rows the moderator keeps. Each action an import issues is audited like one
// issued on its own.

// DomainBlockImportStatus is what the import of a row of a domain block CSV would do.
type DomainBlockImportStatus string

// Statuses of a row of an import.
const (
	// DomainBlockImportable: the row issues at least one action (DomainBlockPreviewRow.Issue).
	DomainBlockImportable DomainBlockImportStatus = "importable"
	// DomainBlockAlreadyApplied: an action in effect of each of the row's kinds covers the
	// domain already, on it or on a parent domain.
	DomainBlockAlreadyApplied DomainBlockImportStatus = "already_applied"
	// DomainBlockNoAction: the row stands for no action (a noop without rejections).
	DomainBlockNoAction DomainBlockImportStatus = "no_action"
	// DomainBlockDuplicate: an earlier row has the same domain in canonical form.
	DomainBlockDuplicate DomainBlockImportStatus = "duplicate"
	// DomainBlockOwnDomain: the domain is this instance's, which takes no domain action.
	DomainBlockOwnDomain DomainBlockImportStatus = "own_domain"
	// DomainBlockInvalid: the domain, the severity, or the comment is not valid.
	DomainBlockInvalid DomainBlockImportStatus = "invalid"
)

// domainBlockPage is the number of domain actions that one read of those in effect lists.
const domainBlockPage = 500

// domainBlockImportChunk is the number of actions that one transaction of an import writes.
const domainBlockImportChunk = 50

// ExportDomainBlocks writes the domain actions in effect as Mastodon's domain block CSV
// (moderation.DomainBlocksOf).
func (s *Service) ExportDomainBlocks(ctx context.Context, w io.Writer) error {
	actions, err := s.domainActionsInEffect(ctx)
	if err != nil {
		return err
	}
	return moderation.WriteDomainBlockCSV(w, moderation.DomainBlocksOf(actions))
}

// domainActionsInEffect lists every domain action in effect now.
func (s *Service) domainActionsInEffect(ctx context.Context) ([]moderation.DomainAction, error) {
	now := kernel.Normalize(s.d.Clock.Now())
	var out []moderation.DomainAction
	before := ""
	for {
		rs, err := s.d.Repository.ListDomainActions(ctx, DomainActionQuery{InEffectAt: &now, Before: before}, domainBlockPage)
		if err != nil {
			return nil, err
		}
		for _, r := range rs {
			out = append(out, r.Action)
		}
		if len(rs) < domainBlockPage {
			return out, nil
		}
		before = rs[len(rs)-1].Action.ID
	}
}

// DomainBlockPreviewRow is a row of a domain block CSV and what its import would do. Written
// is the domain as written in the row; Block.Domain is its canonical form, or Written when it
// has none.
type DomainBlockPreviewRow struct {
	Line    int
	Written string
	Block   moderation.DomainBlock
	Status  DomainBlockImportStatus
	// Issue is the kinds of the actions that the import would issue: those of the row that no
	// action in effect covers.
	Issue []moderation.DomainActionKind
}

// PreviewDomainBlocks parses a domain block CSV (moderation.ParseDomainBlockCSV) and returns
// what importing each row would do, in row order, without changing anything. It fails with
// moderation.ErrMalformedDomainBlockCSV or moderation.ErrTooManyDomainBlocks like the parser.
func (s *Service) PreviewDomainBlocks(ctx context.Context, text string) ([]DomainBlockPreviewRow, error) {
	rows, err := moderation.ParseDomainBlockCSV(text)
	if err != nil {
		return nil, err
	}
	covered, err := s.domainCoverage(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]DomainBlockPreviewRow, len(rows))
	seen := make(map[string]bool, len(rows))
	for i, r := range rows {
		block, status, issue := s.planDomainBlock(r.DomainBlock, r.Invalid, seen, covered)
		out[i] = DomainBlockPreviewRow{Line: r.Line, Written: r.Domain, Block: block, Status: status, Issue: issue}
	}
	return out, nil
}

// domainCoverage maps each domain with actions in effect to their kinds.
func (s *Service) domainCoverage(ctx context.Context) (map[string][]moderation.DomainActionKind, error) {
	actions, err := s.domainActionsInEffect(ctx)
	if err != nil {
		return nil, err
	}
	covered := make(map[string][]moderation.DomainActionKind)
	for _, a := range actions {
		covered[a.Domain] = append(covered[a.Domain], a.Kind)
	}
	return covered, nil
}

// planDomainBlock canonicalizes a block's domain and decides what importing it does: the
// block with its canonical domain, its status, and the kinds of the actions to issue. seen
// collects the canonical domains of the blocks planned before.
func (s *Service) planDomainBlock(b moderation.DomainBlock, invalid bool, seen map[string]bool,
	covered map[string][]moderation.DomainActionKind,
) (moderation.DomainBlock, DomainBlockImportStatus, []moderation.DomainActionKind) {
	domain, err := s.canonicalDomain(b.Domain)
	if err != nil || !moderation.ValidDomain(domain) || invalid || !b.Severity.Valid() {
		return b, DomainBlockInvalid, nil
	}
	b.Domain = domain
	if moderation.CheckDomainActionTexts(b.PublicComment, "") != nil {
		return b, DomainBlockInvalid, nil
	}
	if s.ownDomain(domain) {
		return b, DomainBlockOwnDomain, nil
	}
	if seen[domain] {
		return b, DomainBlockDuplicate, nil
	}
	seen[domain] = true
	kinds := b.Kinds()
	if len(kinds) == 0 {
		return b, DomainBlockNoAction, nil
	}
	var issue []moderation.DomainActionKind
	for _, k := range kinds {
		if !coveredBy(covered, domain, k) {
			issue = append(issue, k)
		}
	}
	if len(issue) == 0 {
		return b, DomainBlockAlreadyApplied, nil
	}
	return b, DomainBlockImportable, issue
}

// coveredBy reports whether an action of kind k in effect covers domain, on it or a parent.
func coveredBy(covered map[string][]moderation.DomainActionKind, domain string, k moderation.DomainActionKind) bool {
	for _, d := range moderation.DomainAndParents(domain) {
		if slices.Contains(covered[d], k) {
			return true
		}
	}
	return false
}

// DomainBlockImportInput is the rows of a domain block CSV that a moderator keeps from the
// preview, to issue at once.
type DomainBlockImportInput struct {
	Blocks []moderation.DomainBlock
	// PrivateNote is the private note of every action issued; "" for none.
	PrivateNote string
	// IssuedByUserID is the moderator; "" for the CLI.
	IssuedByUserID string
}

// DomainBlockImportSkipped is a block of an import that issued nothing, and why.
type DomainBlockImportSkipped struct {
	Domain string // as given
	Reason DomainBlockImportStatus
}

// DomainBlockImportResult is what an import did: the actions it issued, in the order of the
// blocks, and the blocks it skipped.
type DomainBlockImportResult struct {
	Issued  []moderation.DomainAction
	Skipped []DomainBlockImportSkipped
}

// ImportDomainBlocks issues, for each block, the actions that PreviewDomainBlocks names
// (DomainBlockPreviewRow.Issue), until they are lifted, each with the block's public comment
// and obfuscation and the input's private note, and audits each like IssueDomainAction. A block
// that would issue nothing is skipped with the status that says why. The actions are written a
// few dozen at a time, so a failure part way keeps those before it; importing the same blocks
// again skips them as already applied. It fails with moderation.ErrTooManyDomainBlocks for
// more than moderation.MaxDomainBlockImport blocks, and with an error wrapping
// ErrInvalidAction for an invalid private note.
func (s *Service) ImportDomainBlocks(ctx context.Context, in DomainBlockImportInput) (DomainBlockImportResult, error) {
	var res DomainBlockImportResult
	if len(in.Blocks) > moderation.MaxDomainBlockImport {
		return res, moderation.ErrTooManyDomainBlocks
	}
	note := strings.TrimSpace(in.PrivateNote)
	if err := moderation.CheckDomainActionTexts("", note); err != nil {
		return res, err
	}
	covered, err := s.domainCoverage(ctx)
	if err != nil {
		return res, err
	}
	type pending struct {
		block moderation.DomainBlock
		kind  moderation.DomainActionKind
	}
	var todo []pending
	seen := make(map[string]bool, len(in.Blocks))
	for _, b := range in.Blocks {
		planned, status, issue := s.planDomainBlock(b, false, seen, covered)
		if status != DomainBlockImportable {
			res.Skipped = append(res.Skipped, DomainBlockImportSkipped{Domain: b.Domain, Reason: status})
			continue
		}
		for _, k := range issue {
			todo = append(todo, pending{block: planned, kind: k})
		}
	}
	for chunk := range slices.Chunk(todo, domainBlockImportChunk) {
		now := kernel.Normalize(s.d.Clock.Now())
		actions := make([]moderation.DomainAction, len(chunk))
		auditIDs := make([]string, len(chunk))
		for i, p := range chunk {
			actions[i] = moderation.DomainAction{
				ID: s.d.IDs.NewID(), Domain: p.block.Domain, Kind: p.kind, PublicComment: p.block.PublicComment,
				PrivateNote: note, Obfuscate: p.block.Obfuscate, IssuedByUserID: in.IssuedByUserID, IssuedAt: now,
			}
			if err := actions[i].Validate(); err != nil {
				return res, err
			}
			auditIDs[i] = s.d.IDs.NewID()
		}
		err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			for i, a := range actions {
				if err := s.insertDomainAction(ctx, tx, a, auditIDs[i]); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return res, err
		}
		res.Issued = append(res.Issued, actions...)
	}
	return res, nil
}
