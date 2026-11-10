// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

var _ appcontent.ApprovalRepository = (*StatusRepository)(nil)

// readIn returns the queries of tx, or those outside any transaction when tx is nil.
func (r *StatusRepository) readIn(tx uow.Tx) (*contentgen.Queries, error) {
	if tx == nil {
		return r.db.reader(), nil
	}
	return r.db.queries(tx)
}

// InsertInteractionRequest implements content.ApprovalRepository.
func (r *StatusRepository) InsertInteractionRequest(ctx context.Context, tx uow.Tx, req content.InteractionRequest) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertInteractionRequest(ctx, contentgen.InsertInteractionRequestParams{
		ID: req.ID, Kind: string(req.Kind), Direction: string(req.Direction), TargetUri: req.TargetURI,
		TargetAccountID: sqlite.ToNullString(req.TargetAccountID), InteractingUri: req.InteractingURI,
		InteractingAccountID: req.InteractingAccountID, ActivityUri: req.ActivityURI, Visibility: string(req.Visibility),
		CreatedAt: sqlite.ToMillis(req.CreatedAt),
	})
	return n > 0, sqlite.Wrap(err, "insert interaction request")
}

// FindInteractionRequest implements content.ApprovalRepository.
func (r *StatusRepository) FindInteractionRequest(ctx context.Context, tx uow.Tx, id string) (content.InteractionRequest, error) {
	q, err := r.readIn(tx)
	if err != nil {
		return content.InteractionRequest{}, err
	}
	row, err := q.GetInteractionRequest(ctx, id)
	if err != nil {
		return content.InteractionRequest{}, sqlite.NotFound(err, appcontent.ErrNotFound, "interaction request "+id)
	}
	return interactionRequestFrom(row), nil
}

// FindInteractionRequestFor implements content.ApprovalRepository.
func (r *StatusRepository) FindInteractionRequestFor(ctx context.Context, tx uow.Tx, kind content.InteractionKind, interactingURI, targetURI string) (content.InteractionRequest, error) {
	q, err := r.readIn(tx)
	if err != nil {
		return content.InteractionRequest{}, err
	}
	row, err := q.GetInteractionRequestFor(ctx, contentgen.GetInteractionRequestForParams{Kind: string(kind), InteractingUri: interactingURI, TargetUri: targetURI})
	if err != nil {
		return content.InteractionRequest{}, sqlite.NotFound(err, appcontent.ErrNotFound, "interaction request of "+interactingURI)
	}
	return interactionRequestFrom(row), nil
}

// ListInteractionRequestsAfter implements content.ApprovalRepository.
func (r *StatusRepository) ListInteractionRequestsAfter(ctx context.Context, tx uow.Tx, after string, limit int) ([]content.InteractionRequest, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListInteractionRequestsAfter(ctx, contentgen.ListInteractionRequestsAfterParams{After: after, PageLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list interaction requests")
	}
	return interactionRequestsFrom(rows), nil
}

// InsertInteractionEvent implements content.ApprovalRepository.
func (r *StatusRepository) InsertInteractionEvent(ctx context.Context, tx uow.Tx, e content.InteractionEvent) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertInteractionRequestEvent(ctx, contentgen.InsertInteractionRequestEventParams{
		ID: e.ID, RequestID: e.RequestID, Kind: string(e.Kind), AuthorizationUri: sqlite.ToNullString(e.AuthorizationURI), OccurredAt: sqlite.ToMillis(e.OccurredAt),
	}), "insert interaction request event")
}

// ListInteractionEvents implements content.ApprovalRepository.
func (r *StatusRepository) ListInteractionEvents(ctx context.Context, tx uow.Tx, requestID string) ([]content.InteractionEvent, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListInteractionRequestEvents(ctx, requestID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list interaction request events")
	}
	out := make([]content.InteractionEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, content.InteractionEvent{
			ID: row.ID, RequestID: row.RequestID, Kind: content.InteractionEventKind(row.Kind),
			AuthorizationURI: row.AuthorizationUri.String, OccurredAt: sqlite.FromMillis(row.OccurredAt),
		})
	}
	return out, nil
}

// ListPendingInteractionRequests implements content.ApprovalRepository.
func (r *StatusRepository) ListPendingInteractionRequests(ctx context.Context, accountID string, kinds []content.InteractionKind, before string, limit int) ([]content.InteractionRequest, error) {
	ks := make([]string, len(kinds))
	for i, k := range kinds {
		ks[i] = string(k)
	}
	list, err := sqlite.JSONList(ks)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListPendingInteractionRequests(ctx, contentgen.ListPendingInteractionRequestsParams{
		AccountID: sqlite.ToNullString(accountID), Kinds: list, Before: before, PageLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list pending interaction requests")
	}
	return interactionRequestsFrom(rows), nil
}

// ListPendingInteractionRequestsTo implements content.ApprovalRepository.
func (r *StatusRepository) ListPendingInteractionRequestsTo(ctx context.Context, targetURI string) ([]content.InteractionRequest, error) {
	rows, err := r.db.reader().ListPendingInteractionRequestsTo(ctx, targetURI)
	if err != nil {
		return nil, sqlite.Wrap(err, "list pending interaction requests of a status")
	}
	return interactionRequestsFrom(rows), nil
}

// ListPendingInteractionRequestsBy implements content.ApprovalRepository.
func (r *StatusRepository) ListPendingInteractionRequestsBy(ctx context.Context, tx uow.Tx, kind content.InteractionKind, interactingURI string) ([]content.InteractionRequest, error) {
	q, err := r.readIn(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListPendingInteractionRequestsBy(ctx, contentgen.ListPendingInteractionRequestsByParams{Kind: string(kind), InteractingUri: interactingURI})
	if err != nil {
		return nil, sqlite.Wrap(err, "list pending interaction requests of an interaction")
	}
	return interactionRequestsFrom(rows), nil
}

// ListOwnInteractions implements content.ApprovalRepository.
func (r *StatusRepository) ListOwnInteractions(ctx context.Context, accountID string, uris []string) ([]appcontent.OwnInteraction, error) {
	if len(uris) == 0 {
		return nil, nil
	}
	list, err := sqlite.JSONList(uris)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListOwnInteractionStates(ctx, contentgen.ListOwnInteractionStatesParams{AccountID: accountID, Uris: list})
	if err != nil {
		return nil, sqlite.Wrap(err, "list own interaction states")
	}
	out := make([]appcontent.OwnInteraction, 0, len(rows))
	for _, row := range rows {
		out = append(out, appcontent.OwnInteraction{
			RequestID: row.ID, Kind: content.InteractionKind(row.Kind), TargetURI: row.TargetUri, InteractingURI: row.InteractingUri,
			State: content.InteractionRequestState(row.State),
		})
	}
	return out, nil
}

// ReplyAuthorizationsOf implements content.ApprovalRepository.
func (r *StatusRepository) ReplyAuthorizationsOf(ctx context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListReplyAuthorizationsOf(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list reply authorizations")
	}
	for _, row := range rows {
		out[row.ID] = row.AuthorizationUri
	}
	return out, nil
}

// ListOwnInteractionsOf implements content.ApprovalRepository.
func (r *StatusRepository) ListOwnInteractionsOf(ctx context.Context, accountIDs, uris []string) (map[string][]appcontent.OwnInteraction, error) {
	out := map[string][]appcontent.OwnInteraction{}
	if len(accountIDs) == 0 || len(uris) == 0 {
		return out, nil
	}
	accounts, err := sqlite.JSONList(accountIDs)
	if err != nil {
		return nil, err
	}
	list, err := sqlite.JSONList(uris)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListOwnInteractionStatesOfAccounts(ctx, contentgen.ListOwnInteractionStatesOfAccountsParams{AccountIds: accounts, Uris: list})
	if err != nil {
		return nil, sqlite.Wrap(err, "list own interaction states of accounts")
	}
	for _, row := range rows {
		out[row.InteractingAccountID] = append(out[row.InteractingAccountID], appcontent.OwnInteraction{
			RequestID: row.ID, Kind: content.InteractionKind(row.Kind), TargetURI: row.TargetUri, InteractingURI: row.InteractingUri,
			State: content.InteractionRequestState(row.State),
		})
	}
	return out, nil
}

// IsHeldReply implements content.Repository.
func (r *StatusRepository) IsHeldReply(ctx context.Context, tx uow.Tx, statusID string) (bool, error) {
	held, err := r.HeldRepliesAmong(ctx, tx, []string{statusID})
	return held[statusID], err
}

// HeldRepliesAmong implements content.Repository.
func (r *StatusRepository) HeldRepliesAmong(ctx context.Context, tx uow.Tx, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	q, err := r.readIn(tx)
	if err != nil {
		return nil, err
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := q.HeldRepliesAmong(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list held replies")
	}
	for _, id := range rows {
		out[id] = true
	}
	return out, nil
}

// ListHeldReplyIDs implements content.Repository.
func (r *StatusRepository) ListHeldReplyIDs(ctx context.Context, tx uow.Tx) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListHeldReplyIDs(ctx)
	return ids, sqlite.Wrap(err, "list held replies")
}

// InsertInteractionAuthorization implements content.ApprovalRepository.
func (r *StatusRepository) InsertInteractionAuthorization(ctx context.Context, tx uow.Tx, a content.InteractionAuthorization) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertInteractionAuthorization(ctx, contentgen.InsertInteractionAuthorizationParams{
		ID: a.ID, Kind: string(a.Kind), InteractingUri: a.InteractingURI, InteractingAccountID: a.InteractingAccountID,
		TargetStatusID: a.TargetStatusID, CreatedAt: sqlite.ToMillis(a.CreatedAt),
	})
	return n > 0, sqlite.Wrap(err, "insert interaction authorization")
}

// FindInteractionAuthorization implements content.ApprovalRepository.
func (r *StatusRepository) FindInteractionAuthorization(ctx context.Context, id string) (content.InteractionAuthorization, error) {
	row, err := r.db.reader().GetInteractionAuthorization(ctx, id)
	if err != nil {
		return content.InteractionAuthorization{}, sqlite.NotFound(err, appcontent.ErrNotFound, "interaction authorization "+id)
	}
	return interactionAuthorizationFrom(contentgen.GetInteractionAuthorizationForRow(row)), nil
}

// FindInteractionAuthorizationFor implements content.ApprovalRepository.
func (r *StatusRepository) FindInteractionAuthorizationFor(ctx context.Context, tx uow.Tx, kind content.InteractionKind, interactingURI, targetStatusID string) (content.InteractionAuthorization, error) {
	q, err := r.readIn(tx)
	if err != nil {
		return content.InteractionAuthorization{}, err
	}
	row, err := q.GetInteractionAuthorizationFor(ctx, contentgen.GetInteractionAuthorizationForParams{
		Kind: string(kind), InteractingUri: interactingURI, TargetStatusID: targetStatusID,
	})
	if err != nil {
		return content.InteractionAuthorization{}, sqlite.NotFound(err, appcontent.ErrNotFound, "interaction authorization of "+interactingURI)
	}
	return interactionAuthorizationFrom(row), nil
}

// InsertInteractionAuthorizationRevocation implements content.ApprovalRepository.
func (r *StatusRepository) InsertInteractionAuthorizationRevocation(ctx context.Context, tx uow.Tx, authorizationID string, at time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertInteractionAuthorizationRevocation(ctx, contentgen.InsertInteractionAuthorizationRevocationParams{AuthorizationID: authorizationID, RevokedAt: sqlite.ToMillis(at)})
	return n > 0, sqlite.Wrap(err, "insert interaction authorization revocation")
}

// InsertQuoteDetachment implements content.ApprovalRepository.
func (r *StatusRepository) InsertQuoteDetachment(ctx context.Context, tx uow.Tx, statusID string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertStatusQuoteDetachment(ctx, contentgen.InsertStatusQuoteDetachmentParams{StatusID: statusID, DetachedAt: sqlite.ToMillis(at)}), "insert status quote detachment")
}

// UpsertInteractionState implements content.ApprovalRepository.
func (r *StatusRepository) UpsertInteractionState(ctx context.Context, tx uow.Tx, s content.InteractionState) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertInteractionRequestState(ctx, contentgen.UpsertInteractionRequestStateParams{
		RequestID: s.RequestID, Kind: string(s.Kind), TargetAccountID: sqlite.ToNullString(s.TargetAccountID),
		State: string(s.State), AuthorizationUri: sqlite.ToNullString(s.AuthorizationURI),
	}), "upsert interaction request state")
}

// FindInteractionState implements content.ApprovalRepository.
func (r *StatusRepository) FindInteractionState(ctx context.Context, tx uow.Tx, requestID string) (content.InteractionState, error) {
	q, err := r.readIn(tx)
	if err != nil {
		return content.InteractionState{}, err
	}
	row, err := q.GetInteractionRequestState(ctx, requestID)
	if err != nil {
		return content.InteractionState{}, sqlite.NotFound(err, appcontent.ErrNotFound, "interaction request state "+requestID)
	}
	return interactionStateFrom(row), nil
}

// ListInteractionStates implements content.ApprovalRepository.
func (r *StatusRepository) ListInteractionStates(ctx context.Context) ([]content.InteractionState, error) {
	rows, err := r.db.reader().ListAllInteractionRequestStates(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list interaction request states")
	}
	out := make([]content.InteractionState, 0, len(rows))
	for _, row := range rows {
		out = append(out, interactionStateFrom(row))
	}
	return out, nil
}

// DeleteAllInteractionStates implements content.ApprovalRepository.
func (r *StatusRepository) DeleteAllInteractionStates(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteAllInteractionRequestStates(ctx), "delete interaction request states")
}

func interactionRequestFrom(row contentgen.InteractionRequest) content.InteractionRequest {
	return content.InteractionRequest{
		ID: row.ID, Kind: content.InteractionKind(row.Kind), Direction: content.InteractionDirection(row.Direction),
		TargetURI: row.TargetUri, TargetAccountID: row.TargetAccountID.String, InteractingURI: row.InteractingUri,
		InteractingAccountID: row.InteractingAccountID, ActivityURI: row.ActivityUri, Visibility: content.Visibility(row.Visibility),
		CreatedAt: sqlite.FromMillis(row.CreatedAt),
	}
}

func interactionRequestsFrom(rows []contentgen.InteractionRequest) []content.InteractionRequest {
	out := make([]content.InteractionRequest, 0, len(rows))
	for _, row := range rows {
		out = append(out, interactionRequestFrom(row))
	}
	return out
}

func interactionStateFrom(row contentgen.InteractionRequestState) content.InteractionState {
	return content.InteractionState{
		RequestID: row.RequestID, Kind: content.InteractionKind(row.Kind), TargetAccountID: row.TargetAccountID.String,
		State: content.InteractionRequestState(row.State), AuthorizationURI: row.AuthorizationUri.String,
	}
}

func interactionAuthorizationFrom(row contentgen.GetInteractionAuthorizationForRow) content.InteractionAuthorization {
	return content.InteractionAuthorization{
		ID: row.ID, Kind: content.InteractionKind(row.Kind), InteractingURI: row.InteractingUri, InteractingAccountID: row.InteractingAccountID,
		TargetStatusID: row.TargetStatusID, CreatedAt: sqlite.FromMillis(row.CreatedAt), RevokedAt: sqlite.FromNullMillis(row.RevokedAt),
	}
}
