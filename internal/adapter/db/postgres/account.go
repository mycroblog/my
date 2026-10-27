// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// AccountRepository implements account.Repository on PostgreSQL.
type AccountRepository struct {
	db *DB
}

var _ appaccount.Repository = (*AccountRepository)(nil)

// AccountRepository returns the account module's repository on d.
func (d *DB) AccountRepository() *AccountRepository { return &AccountRepository{db: d} }

// InsertAccount implements account.Repository.
func (r *AccountRepository) InsertAccount(ctx context.Context, tx uow.Tx, a account.Account) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAccount(ctx, postgresgen.InsertAccountParams{
		ID: a.ID, Uri: a.URI, Domain: a.Domain, ActorType: string(a.ActorType), CreatedAt: a.CreatedAt,
	})
	if isUniqueViolation(err, "accounts_instance_actor") {
		return fmt.Errorf("insert account %s: %w", a.ID, appaccount.ErrInstanceActorExists)
	}
	if isUniqueViolation(err, "accounts_uri") {
		// A concurrent transaction stored the same remote actor; a new attempt finds it.
		return uow.MarkRetryable(wrap(err, "insert account"))
	}
	return wrap(err, "insert account")
}

// InsertLocalAccount implements account.Repository.
func (r *AccountRepository) InsertLocalAccount(ctx context.Context, tx uow.Tx, la account.LocalAccount) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertLocalAccount(ctx, postgresgen.InsertLocalAccountParams{
		AccountID: la.AccountID, UserID: la.UserID, Username: la.Username, UsernameNormalized: la.UsernameNormalized,
	})
	if isUniqueViolation(err, "local_accounts_username_normalized") {
		return fmt.Errorf("insert local account %q: %w", la.Username, appaccount.ErrUsernameTaken)
	}
	return wrap(err, "insert local account")
}

// InsertProfileRevision implements account.Repository.
func (r *AccountRepository) InsertProfileRevision(ctx context.Context, tx uow.Tx, rev account.ProfileRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	seq, err := toInt32(rev.Seq, "profile revision seq")
	if err != nil {
		return err
	}
	err = q.InsertProfileRevision(ctx, postgresgen.InsertProfileRevisionParams{
		ID:            rev.ID,
		AccountID:     rev.AccountID,
		Seq:           seq,
		DisplayName:   rev.DisplayName,
		NoteHtml:      rev.NoteHTML,
		Locked:        rev.Locked,
		Bot:           rev.Bot,
		Discoverable:  rev.Discoverable,
		Indexable:     rev.Indexable,
		AiOptOut:      rev.AIOptOut,
		AvatarMediaID: toNullString(rev.AvatarMediaID),
		HeaderMediaID: toNullString(rev.HeaderMediaID),
		RevisedAt:     rev.RevisedAt,
	})
	if isUniqueViolation(err, "account_profile_revisions_account_id_seq") {
		// A concurrent revision took this seq after the command read the head at READ COMMITTED.
		// A new attempt reads the new head and takes the next seq.
		return uow.MarkRetryable(wrap(err, "insert profile revision"))
	}
	if err != nil {
		return wrap(err, "insert profile revision")
	}
	for _, f := range rev.Fields {
		pos, err := toInt32(f.Position, "profile field position")
		if err != nil {
			return err
		}
		err = q.InsertProfileField(ctx, postgresgen.InsertProfileFieldParams{
			RevisionID: rev.ID, Position: pos, Name: f.Name, ValueHtml: f.ValueHTML,
		})
		if err != nil {
			return wrap(err, "insert profile field")
		}
	}
	for _, id := range rev.EmojiIDs {
		err := q.InsertProfileRevisionEmoji(ctx, postgresgen.InsertProfileRevisionEmojiParams{RevisionID: rev.ID, EmojiID: id})
		if err != nil {
			return wrap(err, "insert profile revision emoji")
		}
	}
	if rev.Remote != nil {
		version, err := toInt32(rev.Remote.HTMLPolicyVersion, "HTML policy version")
		if err != nil {
			return err
		}
		err = q.InsertRemoteProfileRevision(ctx, postgresgen.InsertRemoteProfileRevisionParams{
			RevisionID: rev.ID, SnapshotID: rev.Remote.SnapshotID, HtmlPolicyVersion: version,
		})
		if err != nil {
			return wrap(err, "insert remote profile revision")
		}
	}
	if rev.Source == nil {
		return nil
	}
	if err := q.InsertProfileRevisionSource(ctx, postgresgen.InsertProfileRevisionSourceParams{
		RevisionID: rev.ID, NoteSource: rev.Source.Note,
	}); err != nil {
		return wrap(err, "insert profile revision source")
	}
	for i, v := range rev.Source.FieldValues {
		if i >= len(rev.Fields) {
			return fmt.Errorf("profile revision %s: %d field sources for %d fields", rev.ID, len(rev.Source.FieldValues), len(rev.Fields))
		}
		pos, err := toInt32(rev.Fields[i].Position, "profile field position")
		if err != nil {
			return err
		}
		err = q.InsertProfileFieldSource(ctx, postgresgen.InsertProfileFieldSourceParams{
			RevisionID: rev.ID, Position: pos, ValueSource: v,
		})
		if err != nil {
			return wrap(err, "insert profile field source")
		}
	}
	return nil
}

// InsertKey implements account.Repository.
func (r *AccountRepository) InsertKey(ctx context.Context, tx uow.Tx, k account.Key, sealedPrivateKey []byte) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAccountKey(ctx, postgresgen.InsertAccountKeyParams{
		ID: k.ID, AccountID: k.AccountID, KeyID: k.KeyID, Algorithm: string(k.Algorithm),
		PublicKeyPem: k.PublicKeyPEM, Fingerprint: k.Fingerprint, CreatedAt: k.CreatedAt,
	})
	if err != nil {
		if isUniqueViolation(err, "account_keys_key_id_fingerprint") {
			// A concurrent transaction stored the same key; a new attempt finds it.
			return uow.MarkRetryable(wrap(err, "insert account key"))
		}
		return wrap(err, "insert account key")
	}
	if sealedPrivateKey == nil {
		return nil
	}
	err = q.InsertLocalAccountPrivateKey(ctx, postgresgen.InsertLocalAccountPrivateKeyParams{
		AccountKeyID: k.ID, PrivateKeyEnc: sealedPrivateKey,
	})
	return wrap(err, "insert private key")
}

// InsertLifecycleEvent implements account.Repository.
func (r *AccountRepository) InsertLifecycleEvent(ctx context.Context, tx uow.Tx, e account.LifecycleEvent) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertLifecycleEvent(ctx, postgresgen.InsertLifecycleEventParams{
		ID:                 e.ID,
		AccountID:          e.AccountID,
		Kind:               string(e.Kind),
		InitiatedBy:        string(e.InitiatedBy),
		ModerationActionID: toNullString(e.ModerationActionID),
		DeletionDueAt:      e.DeletionDueAt,
		OccurredAt:         e.OccurredAt,
	})
	return wrap(err, "insert lifecycle event")
}

// UpsertHandle implements account.Repository.
func (r *AccountRepository) UpsertHandle(ctx context.Context, tx uow.Tx, h account.Handle) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertAccountHandle(ctx, postgresgen.UpsertAccountHandleParams(h)), "upsert account handle")
}

// DeleteAllHandles implements account.Repository.
func (r *AccountRepository) DeleteAllHandles(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteAllAccountHandles(ctx), "delete account handles")
}

// ProfileHead implements account.Repository.
func (r *AccountRepository) ProfileHead(ctx context.Context, tx uow.Tx, accountID string) (account.ProfileHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.ProfileHead{}, err
	}
	row, err := q.GetProfileHead(ctx, accountID)
	if err != nil {
		return account.ProfileHead{}, notFound(err, appaccount.ErrNotFound, "profile head of "+accountID)
	}
	return account.ProfileHead{AccountID: row.AccountID, RevisionID: row.RevisionID, Seq: int(row.Seq)}, nil
}

// UpsertProfileHead implements account.Repository.
func (r *AccountRepository) UpsertProfileHead(ctx context.Context, tx uow.Tx, h account.ProfileHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertProfileHead(ctx, postgresgen.UpsertProfileHeadParams{AccountID: h.AccountID, RevisionID: h.RevisionID})
	return wrap(err, "upsert profile head")
}

// DeleteAllProfileHeads implements account.Repository.
func (r *AccountRepository) DeleteAllProfileHeads(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteAllProfileHeads(ctx), "delete profile heads")
}

// LifecycleState implements account.Repository.
func (r *AccountRepository) LifecycleState(ctx context.Context, tx uow.Tx, accountID string) (account.LifecycleState, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.LifecycleState{}, err
	}
	row, err := q.GetLifecycleStateForUpdate(ctx, accountID)
	if err != nil {
		return account.LifecycleState{}, notFound(err, appaccount.ErrNotFound, "lifecycle state of "+accountID)
	}
	return lifecycleState(row), nil
}

func lifecycleState(row postgresgen.AccountLifecycleState) account.LifecycleState {
	return account.LifecycleState{
		AccountID:     row.AccountID,
		State:         account.LifecycleStateName(row.State),
		Since:         utc(row.Since),
		DeletionDueAt: utcPtr(row.DeletionDueAt),
	}
}

// UpsertLifecycleState implements account.Repository.
func (r *AccountRepository) UpsertLifecycleState(ctx context.Context, tx uow.Tx, s account.LifecycleState) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertLifecycleState(ctx, postgresgen.UpsertLifecycleStateParams{
		AccountID: s.AccountID, State: string(s.State), Since: s.Since, DeletionDueAt: s.DeletionDueAt,
	})
	return wrap(err, "upsert lifecycle state")
}

// DeleteAllLifecycleStates implements account.Repository.
func (r *AccountRepository) DeleteAllLifecycleStates(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteAllLifecycleStates(ctx), "delete lifecycle states")
}

// ListLocalAccounts implements account.Repository.
func (r *AccountRepository) ListLocalAccounts(ctx context.Context, tx uow.Tx) ([]account.LocalAccount, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListLocalAccounts(ctx)
	if err != nil {
		return nil, wrap(err, "list local accounts")
	}
	out := make([]account.LocalAccount, len(rows))
	for i, row := range rows {
		out[i] = account.LocalAccount(row)
	}
	return out, nil
}

// ListProfileRevisionSeqs implements account.Repository.
func (r *AccountRepository) ListProfileRevisionSeqs(ctx context.Context, tx uow.Tx) ([]account.ProfileHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListProfileRevisionSeqs(ctx)
	if err != nil {
		return nil, wrap(err, "list profile revisions")
	}
	out := make([]account.ProfileHead, len(rows))
	for i, row := range rows {
		out[i] = account.ProfileHead{AccountID: row.AccountID, RevisionID: row.ID, Seq: int(row.Seq)}
	}
	return out, nil
}

// ListLifecycleEvents implements account.Repository.
func (r *AccountRepository) ListLifecycleEvents(ctx context.Context, tx uow.Tx) ([]account.LifecycleEvent, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListLifecycleEvents(ctx)
	if err != nil {
		return nil, wrap(err, "list lifecycle events")
	}
	out := make([]account.LifecycleEvent, len(rows))
	for i, row := range rows {
		out[i] = account.LifecycleEvent{
			ID:                 row.ID,
			AccountID:          row.AccountID,
			Kind:               account.LifecycleKind(row.Kind),
			InitiatedBy:        account.Initiator(row.InitiatedBy),
			ModerationActionID: fromNullString(row.ModerationActionID),
			DeletionDueAt:      utcPtr(row.DeletionDueAt),
			OccurredAt:         utc(row.OccurredAt),
		}
	}
	return out, nil
}

// ListHandles implements account.Repository.
func (r *AccountRepository) ListHandles(ctx context.Context) ([]account.Handle, error) {
	rows, err := r.db.reader().ListAccountHandles(ctx)
	if err != nil {
		return nil, wrap(err, "list account handles")
	}
	out := make([]account.Handle, len(rows))
	for i, row := range rows {
		out[i] = account.Handle(row)
	}
	return out, nil
}

// ListProfileHeads implements account.Repository.
func (r *AccountRepository) ListProfileHeads(ctx context.Context) ([]account.ProfileHead, error) {
	rows, err := r.db.reader().ListProfileHeads(ctx)
	if err != nil {
		return nil, wrap(err, "list profile heads")
	}
	out := make([]account.ProfileHead, len(rows))
	for i, row := range rows {
		out[i] = account.ProfileHead{AccountID: row.AccountID, RevisionID: row.RevisionID, Seq: int(row.Seq)}
	}
	return out, nil
}

// ListLifecycleStates implements account.Repository.
func (r *AccountRepository) ListLifecycleStates(ctx context.Context) ([]account.LifecycleState, error) {
	rows, err := r.db.reader().ListLifecycleStates(ctx)
	if err != nil {
		return nil, wrap(err, "list lifecycle states")
	}
	out := make([]account.LifecycleState, len(rows))
	for i, row := range rows {
		out[i] = lifecycleState(row)
	}
	return out, nil
}

func accountFrom(row postgresgen.Account) account.Account {
	return account.Account{
		ID: row.ID, URI: row.Uri, Domain: row.Domain, ActorType: account.ActorType(row.ActorType), CreatedAt: utc(row.CreatedAt),
	}
}

// Account implements account.Repository.
func (r *AccountRepository) Account(ctx context.Context, tx uow.Tx, id string) (account.Account, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.Account{}, err
	}
	row, err := q.GetAccount(ctx, id)
	if err != nil {
		return account.Account{}, notFound(err, appaccount.ErrNotFound, "account "+id)
	}
	return accountFrom(row), nil
}

// FindAccount implements account.Repository.
func (r *AccountRepository) FindAccount(ctx context.Context, id string) (account.Account, error) {
	row, err := r.db.reader().GetAccount(ctx, id)
	if err != nil {
		return account.Account{}, notFound(err, appaccount.ErrNotFound, "account "+id)
	}
	return accountFrom(row), nil
}

// FindAccountByURI implements account.Repository.
func (r *AccountRepository) FindAccountByURI(ctx context.Context, uri string) (account.Account, error) {
	row, err := r.db.reader().GetAccountByURI(ctx, uri)
	if err != nil {
		return account.Account{}, notFound(err, appaccount.ErrNotFound, "account "+uri)
	}
	return accountFrom(row), nil
}

// AnyAccountOnDomain implements account.Repository.
func (r *AccountRepository) AnyAccountOnDomain(ctx context.Context, domain string, ids []string) (bool, error) {
	if len(ids) == 0 {
		return false, nil
	}
	if ids == nil {
		ids = []string{}
	}
	found, err := r.db.reader().AnyAccountOnDomain(ctx, postgresgen.AnyAccountOnDomainParams{Domain: domain, Ids: ids})
	if err != nil {
		return false, wrap(err, "find accounts on "+domain)
	}
	return found, nil
}

// ExistingAccountIDs implements account.Repository.
func (r *AccountRepository) ExistingAccountIDs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out, err := r.db.reader().ListExistingAccountIDs(ctx, ids)
	if err != nil {
		return nil, wrap(err, "list existing accounts")
	}
	return out, nil
}

// AccountsOnDomain implements account.Repository.
func (r *AccountRepository) AccountsOnDomain(ctx context.Context, tx uow.Tx, domain string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	out, err := q.ListAccountsOnDomain(ctx, postgresgen.ListAccountsOnDomainParams{Domain: domain, Ids: ids})
	if err != nil {
		return nil, wrap(err, "list accounts on "+domain)
	}
	return out, nil
}

// ListLocalAccountIDs implements account.Repository.
func (r *AccountRepository) ListLocalAccountIDs(ctx context.Context, after string, limit int) ([]string, error) {
	pageLimit, err := toInt32(limit, "page limit")
	if err != nil {
		return nil, err
	}
	out, err := r.db.reader().ListLocalAccountIDsAfter(ctx, postgresgen.ListLocalAccountIDsAfterParams{After: toNullString(after), PageLimit: pageLimit})
	if err != nil {
		return nil, wrap(err, "list local account ids")
	}
	return out, nil
}

// FindInstanceActor implements account.Repository.
func (r *AccountRepository) FindInstanceActor(ctx context.Context) (account.Account, error) {
	row, err := r.db.reader().GetInstanceActor(ctx)
	if err != nil {
		return account.Account{}, notFound(err, appaccount.ErrNotFound, "instance actor")
	}
	return accountFrom(row), nil
}

// FindLocalAccount implements account.Repository.
func (r *AccountRepository) FindLocalAccount(ctx context.Context, accountID string) (account.LocalAccount, error) {
	row, err := r.db.reader().GetLocalAccount(ctx, accountID)
	if err != nil {
		return account.LocalAccount{}, notFound(err, appaccount.ErrNotFound, "local account "+accountID)
	}
	return account.LocalAccount(row), nil
}

// FindLocalAccountByUser implements account.Repository.
func (r *AccountRepository) FindLocalAccountByUser(ctx context.Context, userID string) (account.LocalAccount, error) {
	row, err := r.db.reader().GetLocalAccountByUserID(ctx, userID)
	if err != nil {
		return account.LocalAccount{}, notFound(err, appaccount.ErrNotFound, "local account of user "+userID)
	}
	return account.LocalAccount(row), nil
}

// FindHandle implements account.Repository.
func (r *AccountRepository) FindHandle(ctx context.Context, usernameNormalized, domain string) (account.Handle, error) {
	row, err := r.db.reader().GetAccountHandleByName(ctx, postgresgen.GetAccountHandleByNameParams{
		UsernameNormalized: usernameNormalized, Domain: domain,
	})
	if err != nil {
		return account.Handle{}, notFound(err, appaccount.ErrNotFound, fmt.Sprintf("handle %s@%s", usernameNormalized, domain))
	}
	return account.Handle(row), nil
}

// FindHandleOf implements account.Repository.
func (r *AccountRepository) FindHandleOf(ctx context.Context, accountID string) (account.Handle, error) {
	row, err := r.db.reader().GetAccountHandle(ctx, accountID)
	if err != nil {
		return account.Handle{}, notFound(err, appaccount.ErrNotFound, "handle of "+accountID)
	}
	return account.Handle(row), nil
}

// FindCurrentProfile implements account.Repository.
func (r *AccountRepository) FindCurrentProfile(ctx context.Context, accountID string) (account.ProfileRevision, error) {
	return currentProfile(ctx, r.db.reader(), accountID)
}

// CurrentProfile implements account.Repository.
func (r *AccountRepository) CurrentProfile(ctx context.Context, tx uow.Tx, accountID string) (account.ProfileRevision, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.ProfileRevision{}, err
	}
	return currentProfile(ctx, q, accountID)
}

func currentProfile(ctx context.Context, q *postgresgen.Queries, accountID string) (account.ProfileRevision, error) {
	head, err := q.GetProfileHead(ctx, accountID)
	if err != nil {
		return account.ProfileRevision{}, notFound(err, appaccount.ErrNotFound, "profile head of "+accountID)
	}
	row, err := q.GetProfileRevision(ctx, head.RevisionID)
	if err != nil {
		return account.ProfileRevision{}, notFound(err, appaccount.ErrNotFound, "profile revision "+head.RevisionID)
	}
	fields, err := q.ListProfileFields(ctx, row.ID)
	if err != nil {
		return account.ProfileRevision{}, wrap(err, "list profile fields")
	}
	emojis, err := q.ListProfileRevisionEmojis(ctx, row.ID)
	if err != nil {
		return account.ProfileRevision{}, wrap(err, "list profile revision emoji")
	}
	rev := account.ProfileRevision{
		ID:            row.ID,
		AccountID:     row.AccountID,
		Seq:           int(row.Seq),
		DisplayName:   row.DisplayName,
		NoteHTML:      row.NoteHtml,
		Locked:        row.Locked,
		Bot:           row.Bot,
		Discoverable:  row.Discoverable,
		Indexable:     row.Indexable,
		AIOptOut:      row.AiOptOut,
		AvatarMediaID: fromNullString(row.AvatarMediaID),
		HeaderMediaID: fromNullString(row.HeaderMediaID),
		RevisedAt:     utc(row.RevisedAt),
	}
	for _, f := range fields {
		rev.Fields = append(rev.Fields, account.ProfileField{Position: int(f.Position), Name: f.Name, ValueHTML: f.ValueHtml})
	}
	if len(emojis) > 0 {
		rev.EmojiIDs = emojis
	}
	note, err := q.GetProfileRevisionSource(ctx, row.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return rev, nil
	}
	if err != nil {
		return account.ProfileRevision{}, wrap(err, "get profile revision source")
	}
	sources, err := q.ListProfileFieldSources(ctx, row.ID)
	if err != nil {
		return account.ProfileRevision{}, wrap(err, "list profile field sources")
	}
	rev.Source = &account.ProfileSource{Note: note, FieldValues: make([]string, len(rev.Fields))}
	for _, s := range sources {
		if i := slices.IndexFunc(rev.Fields, func(f account.ProfileField) bool { return f.Position == int(s.Position) }); i >= 0 {
			rev.Source.FieldValues[i] = s.ValueSource
		}
	}
	return rev, nil
}

// FindLifecycleState implements account.Repository.
func (r *AccountRepository) FindLifecycleState(ctx context.Context, accountID string) (account.LifecycleState, error) {
	row, err := r.db.reader().GetLifecycleState(ctx, accountID)
	if err != nil {
		return account.LifecycleState{}, notFound(err, appaccount.ErrNotFound, "lifecycle state of "+accountID)
	}
	return lifecycleState(row), nil
}

// ListKeys implements account.Repository.
func (r *AccountRepository) ListKeys(ctx context.Context, accountID string) ([]account.Key, error) {
	rows, err := r.db.reader().ListAccountKeys(ctx, accountID)
	if err != nil {
		return nil, wrap(err, "list account keys")
	}
	out := make([]account.Key, len(rows))
	for i, row := range rows {
		out[i] = account.Key{
			ID: row.ID, AccountID: row.AccountID, KeyID: row.KeyID, Algorithm: account.KeyAlgorithm(row.Algorithm),
			PublicKeyPEM: row.PublicKeyPem, Fingerprint: row.Fingerprint, CreatedAt: utc(row.CreatedAt),
		}
	}
	return out, nil
}

// FindSealedPrivateKey implements account.Repository.
func (r *AccountRepository) FindSealedPrivateKey(ctx context.Context, accountKeyID string) ([]byte, error) {
	sealed, err := r.db.reader().GetLocalAccountPrivateKey(ctx, accountKeyID)
	if err != nil {
		return nil, notFound(err, appaccount.ErrNotFound, "private key of "+accountKeyID)
	}
	return sealed, nil
}

// IsMediaReferenced implements account.Repository.
func (r *AccountRepository) IsMediaReferenced(ctx context.Context, mediaID string) (bool, error) {
	ok, err := r.db.reader().IsMediaReferencedByProfiles(ctx, toNullString(mediaID))
	return ok, wrap(err, "look up profile media")
}

// IsEmojiReferenced implements account.Repository.
func (r *AccountRepository) IsEmojiReferenced(ctx context.Context, emojiID string) (bool, error) {
	ok, err := r.db.reader().IsEmojiReferencedByProfiles(ctx, emojiID)
	return ok, wrap(err, "look up profile emoji")
}

// CountLocalAccounts implements account.Repository.
func (r *AccountRepository) CountLocalAccounts(ctx context.Context) (int, error) {
	n, err := r.db.reader().CountLocalAccounts(ctx)
	if err != nil {
		return 0, wrap(err, "count local accounts")
	}
	return int(n), nil
}
