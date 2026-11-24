// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/accountgen"
	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// FindActorParts implements account.Repository: one query (ListActorsByIDs) reads every part of
// the actors of ids, each as the read of one account reads it.
func (r *Repository) FindActorParts(ctx context.Context, ids []string) (appaccount.ActorParts, error) {
	return actorParts(ctx, r.db.reader(), ids)
}

// FindActorPartsTx implements the reads in a transaction.
func (r *Repository) FindActorPartsTx(ctx context.Context, tx uow.Tx, ids []string) (appaccount.ActorParts, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return appaccount.ActorParts{}, err
	}
	return actorParts(ctx, db.reader(), ids)
}

func actorParts(ctx context.Context, q *accountgen.Queries, ids []string) (appaccount.ActorParts, error) {
	out := appaccount.ActorParts{
		Accounts: map[string]account.Account{}, Locals: map[string]account.LocalAccount{}, Handles: map[string]account.Handle{},
		Profiles: map[string]account.ProfileRevision{}, Keys: map[string][]account.Key{}, Lifecycle: map[string]account.LifecycleState{},
	}
	if len(ids) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return out, err
	}
	rows, err := q.ListActorsByIDs(ctx, list)
	if err != nil {
		return out, sqlite.Wrap(err, "list actors")
	}
	for _, row := range rows {
		if err := addActorRow(out, row); err != nil {
			return out, err
		}
	}
	return out, nil
}

// addActorRow puts the parts of an actor of ListActorsByIDs into out.
func addActorRow(out appaccount.ActorParts, row accountgen.ListActorsByIDsRow) error {
	id := row.ID
	out.Accounts[id] = accountFrom(accountgen.Account{ID: id, Uri: row.Uri, Domain: row.Domain, ActorType: row.ActorType, CreatedAt: row.CreatedAt})
	if row.LocalUserID.Valid {
		out.Locals[id] = account.LocalAccount{
			AccountID: id, UserID: row.LocalUserID.String, Username: row.LocalUsername.String, UsernameNormalized: row.LocalUsernameNormalized.String,
		}
	}
	if row.HandleUsername.Valid {
		out.Handles[id] = account.Handle{
			AccountID: id, Username: row.HandleUsername.String, UsernameNormalized: row.HandleUsernameNormalized.String, Domain: row.HandleDomain.String,
		}
	}
	if row.RevisionID.Valid {
		rev, err := profileFrom(accountgen.GetCurrentProfileRow{
			ID: row.RevisionID.String, AccountID: id, Seq: row.Seq.Int64, DisplayName: row.DisplayName.String, NoteHtml: row.NoteHtml.String,
			Locked: row.Locked.Int64, Bot: row.Bot.Int64, Discoverable: row.Discoverable.Int64, Indexable: row.Indexable.Int64, AiOptOut: row.AiOptOut.Int64,
			AvatarMediaID: row.AvatarMediaID, HeaderMediaID: row.HeaderMediaID, RevisedAt: row.RevisedAt.Int64,
			Feed: row.Feed.String, FollowsVisibility: row.FollowsVisibility.String, NoteSource: row.NoteSource, Fields: row.Fields, EmojiIds: row.EmojiIds,
		})
		if err != nil {
			return err
		}
		out.Profiles[id] = rev
	}
	var keys []keyJSON
	if err := json.Unmarshal([]byte(row.Keys), &keys); err != nil {
		return fmt.Errorf("decode the keys of %s: %w", id, err)
	}
	for _, k := range keys {
		fingerprint, err := hex.DecodeString(k.Fingerprint)
		if err != nil {
			return fmt.Errorf("decode the fingerprint of key %s: %w", k.ID, err)
		}
		out.Keys[id] = append(out.Keys[id], keyFrom(accountgen.AccountKey{
			ID: k.ID, AccountID: id, KeyID: k.KeyID, Algorithm: k.Algorithm, PublicKeyPem: k.PublicKeyPEM, Fingerprint: fingerprint, CreatedAt: k.CreatedAt,
		}))
	}
	if row.State.Valid {
		out.Lifecycle[id] = lifecycleState(accountgen.AccountLifecycleState{
			AccountID: id, State: row.State.String, Since: row.Since.Int64, DeletionDueAt: row.DeletionDueAt,
		})
	}
	return nil
}

// keyJSON is a key of the keys of ListActorsByIDs.
type keyJSON struct {
	ID           string `json:"id"`
	KeyID        string `json:"key_id"`
	Algorithm    string `json:"algorithm"`
	PublicKeyPEM string `json:"public_key_pem"`
	Fingerprint  string `json:"fingerprint"`
	CreatedAt    int64  `json:"created_at"`
}

// FindAccounts implements account.Repository.
func (r *Repository) FindAccounts(ctx context.Context, ids []string) (map[string]account.Account, error) {
	out := map[string]account.Account{}
	if len(ids) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListAccountsByIDs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list accounts")
	}
	for _, row := range rows {
		out[row.ID] = accountFrom(row)
	}
	return out, nil
}

// FindLocalAccountsByUsers implements account.Repository.
func (r *Repository) FindLocalAccountsByUsers(ctx context.Context, userIDs []string) (map[string]account.LocalAccount, error) {
	out := map[string]account.LocalAccount{}
	if len(userIDs) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(userIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListLocalAccountsByUsers(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list local accounts of users")
	}
	for _, row := range rows {
		out[row.UserID] = account.LocalAccount(row)
	}
	return out, nil
}

// FindMoves implements account.Repository.
func (r *Repository) FindMoves(ctx context.Context, ids []string) (map[string]account.Move, error) {
	out := map[string]account.Move{}
	if len(ids) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListAccountMovesByIDs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list account moves")
	}
	for _, row := range rows {
		out[row.ID] = moveFrom(row)
	}
	return out, nil
}

// FindLifecycleStates implements account.Repository.
func (r *Repository) FindLifecycleStates(ctx context.Context, ids []string) (map[string]account.LifecycleState, error) {
	out := map[string]account.LifecycleState{}
	if len(ids) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListLifecycleStatesByIDs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list lifecycle states")
	}
	for _, row := range rows {
		out[row.AccountID] = lifecycleState(row)
	}
	return out, nil
}

// FindAccountsByURIs implements account.Repository.
func (r *Repository) FindAccountsByURIs(ctx context.Context, uris []string) (map[string]account.Account, error) {
	out := map[string]account.Account{}
	if len(uris) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(uris)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListAccountsByURIs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list accounts by URI")
	}
	for _, row := range rows {
		out[row.Uri] = accountFrom(row)
	}
	return out, nil
}

// FindHandles implements account.Repository.
func (r *Repository) FindHandles(ctx context.Context, names []appaccount.HandleName) (map[appaccount.HandleName]account.Handle, error) {
	out := map[appaccount.HandleName]account.Handle{}
	if len(names) == 0 {
		return out, nil
	}
	type pair struct {
		U string `json:"u"`
		D string `json:"d"`
	}
	pairs := make([]pair, len(names))
	for i, n := range names {
		pairs[i] = pair{U: n.UsernameNormalized, D: n.Domain}
	}
	list, err := json.Marshal(pairs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListAccountHandlesByNames(ctx, string(list))
	if err != nil {
		return nil, sqlite.Wrap(err, "list account handles")
	}
	for _, row := range rows {
		out[appaccount.HandleName{UsernameNormalized: row.UsernameNormalized, Domain: row.Domain}] = account.Handle(row)
	}
	return out, nil
}
