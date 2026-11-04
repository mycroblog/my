// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The audit log (FR-704, ADR-0058, view_audit): the operations of the moderators and the
// administrators, which their use cases record. It is read-only here, as everywhere.

// AuditLog lists the audit log and tells which targets of its entries are gone.
// *appmoderation.Service implements it.
type AuditLog interface {
	AuditLog(ctx context.Context, q appmoderation.AuditQuery, limit int) ([]moderation.AuditEntry, error)
	GoneAuditTargets(ctx context.Context, entries []moderation.AuditEntry) (map[appmoderation.AuditTarget]bool, error)
}

// ListAuditLog serves GET /api/v1/admin/audit_log, newest first. The cursor is the ID of the
// last entry of the previous page.
func (s *Server) ListAuditLog(ctx context.Context, req ListAuditLogRequestObject) (ListAuditLogResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermViewAudit); err != nil {
		return nil, err
	} else if p != nil {
		return ListAuditLogdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	pg := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if pg.problem != nil {
		return ListAuditLogdefaultApplicationProblemPlusJSONResponse(*pg.problem), nil
	}
	params := req.Params
	q := appmoderation.AuditQuery{Before: pg.before}
	invalid := func(name string) (ListAuditLogResponseObject, error) {
		return ListAuditLogdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, name)), nil
	}
	if params.Action != nil {
		if q.Action = moderation.AuditAction(*params.Action); !q.Action.Valid() {
			return invalid("action")
		}
	}
	if params.TargetKind != nil {
		if q.TargetKind = moderation.AuditTargetKind(*params.TargetKind); !q.TargetKind.Valid() {
			return invalid("target_kind")
		}
	}
	if params.TargetId != nil {
		if q.TargetID = *params.TargetId; q.TargetKind == "" || q.TargetID == "" || utf8.RuneCountInString(q.TargetID) > moderation.MaxAuditTargetLabel {
			return invalid("target_id")
		}
	}
	empty := false
	if params.ActorAccountId != nil {
		a, err := s.o.Accounts.FindByID(ctx, knownID(*params.ActorAccountId))
		switch {
		case errors.Is(err, appaccount.ErrNotFound) || (err == nil && a.Local == nil):
			empty = true // only local users perform operations
		case err != nil:
			return nil, fmt.Errorf("clientapi: the actor %s of the audit log: %w", *params.ActorAccountId, err)
		default:
			q.ActorUserID = a.Local.UserID
		}
	}
	var entries []moderation.AuditEntry
	if !empty {
		var err error
		entries, err = s.o.AuditLog.AuditLog(ctx, q, pg.limit+1)
		if err != nil {
			return nil, fmt.Errorf("clientapi: list the audit log: %w", err)
		}
	}
	entries, last := trim(entries, pg.limit)
	items, err := s.newAuditEntries(ctx, entries)
	if err != nil {
		return nil, err
	}
	page := AdminAuditEntryPage{Items: items}
	if last >= 0 {
		page.NextCursor = encodeCursor(entries[last].ID)
	}
	cache := cacheNoStore
	return ListAuditLog200JSONResponse{Body: page, Headers: ListAuditLog200ResponseHeaders{CacheControl: &cache}}, nil
}

// newAuditEntries presents entries with the accounts of their actors, reading each user's
// account once, and whether their targets still exist, looked up once for all the entries. An
// actor whose account is gone is presented by kind only.
func (s *Server) newAuditEntries(ctx context.Context, entries []moderation.AuditEntry) ([]AdminAuditEntry, error) {
	gone, err := s.o.AuditLog.GoneAuditTargets(ctx, entries)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the targets of the audit log: %w", err)
	}
	accountOf := map[string]string{} // user ID -> account ID; "" when gone
	var order []appaccount.Actor
	for _, e := range entries {
		u := e.ActorUserID
		if _, seen := accountOf[u]; seen || u == "" {
			continue
		}
		accountOf[u] = ""
		if s.o.UserAccounts == nil {
			continue
		}
		la, err := s.o.UserAccounts.LocalAccountOfUser(ctx, u)
		if errors.Is(err, appaccount.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("clientapi: the account of user %s: %w", u, err)
		}
		accountOf[u] = la.AccountID
		a, err := s.o.Accounts.FindByID(ctx, la.AccountID)
		if errors.Is(err, appaccount.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("clientapi: account %s of the audit log: %w", la.AccountID, err)
		}
		order = append(order, a)
	}
	presented, err := s.newAccounts(ctx, order)
	if err != nil {
		return nil, err
	}
	accounts := make(map[string]*Account, len(presented))
	for i := range presented {
		accounts[presented[i].Id] = &presented[i]
	}
	out := make([]AdminAuditEntry, 0, len(entries))
	for _, e := range entries {
		item := AdminAuditEntry{
			Id: e.ID, ActorKind: AdminAuditActorKind(e.ActorKind), Action: AdminAuditAction(e.Action),
			TargetKind: AdminAuditTargetKind(e.TargetKind), TargetId: e.TargetID, TargetLabel: e.TargetLabel,
			TargetExists: !gone[appmoderation.AuditTarget{Kind: e.TargetKind, ID: e.TargetID}], OccurredAt: e.OccurredAt.UTC(),
		}
		if id := accountOf[e.ActorUserID]; id != "" {
			item.ActorAccountId = &id
			item.ActorAccount = accounts[id]
		}
		if e.SourceEventID != "" {
			src := e.SourceEventID
			item.SourceEventId = &src
		}
		out = append(out, item)
	}
	return out, nil
}
