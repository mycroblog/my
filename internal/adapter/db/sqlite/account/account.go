// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/accountgen"
	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// Repository implements account.Repository on SQLite.
type Repository struct {
	db *moduleDB
}

var _ appaccount.Repository = (*Repository)(nil)

// NewRepository returns the account module's repository on d.
func NewRepository(d *sqlite.DB) *Repository {
	return &Repository{db: &moduleDB{d}}
}

// InsertAccount implements account.Repository.
func (r *Repository) InsertAccount(ctx context.Context, tx uow.Tx, a account.Account) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAccount(ctx, accountgen.InsertAccountParams{
		ID: a.ID, Uri: a.URI, Domain: a.Domain, ActorType: string(a.ActorType), CreatedAt: sqlite.ToMillis(a.CreatedAt),
	})
	if sqlite.IsUniqueViolation(err, "accounts.domain, accounts.actor_type") {
		return fmt.Errorf("insert account %s: %w", a.ID, appaccount.ErrInstanceActorExists)
	}
	if sqlite.IsUniqueViolation(err, "accounts.uri") {
		// A concurrent transaction stored the same remote actor; a new attempt finds it.
		return uow.MarkRetryable(sqlite.Wrap(err, "insert account"))
	}
	return sqlite.Wrap(err, "insert account")
}

// InsertLocalAccount implements account.Repository.
func (r *Repository) InsertLocalAccount(ctx context.Context, tx uow.Tx, la account.LocalAccount) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertLocalAccount(ctx, accountgen.InsertLocalAccountParams{
		AccountID: la.AccountID, UserID: la.UserID, Username: la.Username, UsernameNormalized: la.UsernameNormalized,
	})
	if sqlite.IsUniqueViolation(err, "local_accounts.username_normalized") {
		return fmt.Errorf("insert local account %q: %w", la.Username, appaccount.ErrUsernameTaken)
	}
	return sqlite.Wrap(err, "insert local account")
}

// InsertProfileRevision implements account.Repository.
func (r *Repository) InsertProfileRevision(ctx context.Context, tx uow.Tx, rev account.ProfileRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertProfileRevision(ctx, accountgen.InsertProfileRevisionParams{
		ID:            rev.ID,
		AccountID:     rev.AccountID,
		Seq:           int64(rev.Seq),
		DisplayName:   rev.DisplayName,
		NoteHtml:      rev.NoteHTML,
		Locked:        sqlite.BoolToInt(rev.Locked),
		Bot:           sqlite.BoolToInt(rev.Bot),
		Discoverable:  sqlite.BoolToInt(rev.Discoverable),
		Indexable:     sqlite.BoolToInt(rev.Indexable),
		AiOptOut:      sqlite.BoolToInt(rev.AIOptOut),
		AvatarMediaID: sqlite.ToNullString(rev.AvatarMediaID),
		HeaderMediaID: sqlite.ToNullString(rev.HeaderMediaID),
		RevisedAt:     sqlite.ToMillis(rev.RevisedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert profile revision")
	}
	for _, f := range rev.Fields {
		err := q.InsertProfileField(ctx, accountgen.InsertProfileFieldParams{
			RevisionID: rev.ID, Position: int64(f.Position), Name: f.Name, ValueHtml: f.ValueHTML,
		})
		if err != nil {
			return sqlite.Wrap(err, "insert profile field")
		}
	}
	for _, id := range rev.EmojiIDs {
		err := q.InsertProfileRevisionEmoji(ctx, accountgen.InsertProfileRevisionEmojiParams{RevisionID: rev.ID, EmojiID: id})
		if err != nil {
			return sqlite.Wrap(err, "insert profile revision emoji")
		}
	}
	if rev.Remote != nil {
		err := q.InsertRemoteProfileRevision(ctx, accountgen.InsertRemoteProfileRevisionParams{
			RevisionID: rev.ID, SnapshotID: rev.Remote.SnapshotID, HtmlPolicyVersion: int64(rev.Remote.HTMLPolicyVersion),
			ProfileDigest: rev.Remote.ProfileDigest,
		})
		if err != nil {
			return sqlite.Wrap(err, "insert remote profile revision")
		}
	}
	if rev.Source == nil {
		return nil
	}
	if err := q.InsertProfileRevisionSource(ctx, accountgen.InsertProfileRevisionSourceParams{
		RevisionID: rev.ID, NoteSource: rev.Source.Note,
	}); err != nil {
		return sqlite.Wrap(err, "insert profile revision source")
	}
	for i, v := range rev.Source.FieldValues {
		if i >= len(rev.Fields) {
			return fmt.Errorf("profile revision %s: %d field sources for %d fields", rev.ID, len(rev.Source.FieldValues), len(rev.Fields))
		}
		err := q.InsertProfileFieldSource(ctx, accountgen.InsertProfileFieldSourceParams{
			RevisionID: rev.ID, Position: int64(rev.Fields[i].Position), ValueSource: v,
		})
		if err != nil {
			return sqlite.Wrap(err, "insert profile field source")
		}
	}
	return nil
}

// InsertKey implements account.Repository.
func (r *Repository) InsertKey(ctx context.Context, tx uow.Tx, k account.Key, sealedPrivateKey []byte) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAccountKey(ctx, accountgen.InsertAccountKeyParams{
		ID: k.ID, AccountID: k.AccountID, KeyID: k.KeyID, Algorithm: string(k.Algorithm),
		PublicKeyPem: k.PublicKeyPEM, Fingerprint: k.Fingerprint, CreatedAt: sqlite.ToMillis(k.CreatedAt),
	})
	if err != nil {
		if sqlite.IsUniqueViolation(err, "account_keys.key_id, account_keys.fingerprint") {
			// A concurrent transaction stored the same key; a new attempt finds it.
			return uow.MarkRetryable(sqlite.Wrap(err, "insert account key"))
		}
		return sqlite.Wrap(err, "insert account key")
	}
	if sealedPrivateKey == nil {
		return nil
	}
	err = q.InsertLocalAccountPrivateKey(ctx, accountgen.InsertLocalAccountPrivateKeyParams{
		AccountKeyID: k.ID, PrivateKeyEnc: sealedPrivateKey,
	})
	return sqlite.Wrap(err, "insert private key")
}

// InsertLifecycleEvent implements account.Repository.
func (r *Repository) InsertLifecycleEvent(ctx context.Context, tx uow.Tx, e account.LifecycleEvent) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertLifecycleEvent(ctx, accountgen.InsertLifecycleEventParams{
		ID:                 e.ID,
		AccountID:          e.AccountID,
		Kind:               string(e.Kind),
		InitiatedBy:        string(e.InitiatedBy),
		ModerationActionID: sqlite.ToNullString(e.ModerationActionID),
		DeletionDueAt:      sqlite.ToNullMillis(e.DeletionDueAt),
		OccurredAt:         sqlite.ToMillis(e.OccurredAt),
	})
	return sqlite.Wrap(err, "insert lifecycle event")
}

// UpsertHandle implements account.Repository.
func (r *Repository) UpsertHandle(ctx context.Context, tx uow.Tx, h account.Handle) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertAccountHandle(ctx, accountgen.UpsertAccountHandleParams(h)), "upsert account handle")
}

// DeleteAllHandles implements account.Repository.
func (r *Repository) DeleteAllHandles(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteAllAccountHandles(ctx), "delete account handles")
}

// ProfileHead implements account.Repository.
func (r *Repository) ProfileHead(ctx context.Context, tx uow.Tx, accountID string) (account.ProfileHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.ProfileHead{}, err
	}
	row, err := q.GetProfileHead(ctx, accountID)
	if err != nil {
		return account.ProfileHead{}, sqlite.NotFound(err, appaccount.ErrNotFound, "profile head of "+accountID)
	}
	return account.ProfileHead{AccountID: row.AccountID, RevisionID: row.RevisionID, Seq: int(row.Seq)}, nil
}

// UpsertProfileHead implements account.Repository.
func (r *Repository) UpsertProfileHead(ctx context.Context, tx uow.Tx, h account.ProfileHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertProfileHead(ctx, accountgen.UpsertProfileHeadParams{AccountID: h.AccountID, RevisionID: h.RevisionID})
	return sqlite.Wrap(err, "upsert profile head")
}

// DeleteAllProfileHeads implements account.Repository.
func (r *Repository) DeleteAllProfileHeads(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteAllProfileHeads(ctx), "delete profile heads")
}

// LifecycleState implements account.Repository.
func (r *Repository) LifecycleState(ctx context.Context, tx uow.Tx, accountID string) (account.LifecycleState, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.LifecycleState{}, err
	}
	row, err := q.GetLifecycleState(ctx, accountID)
	if err != nil {
		return account.LifecycleState{}, sqlite.NotFound(err, appaccount.ErrNotFound, "lifecycle state of "+accountID)
	}
	return lifecycleState(row), nil
}

func lifecycleState(row accountgen.AccountLifecycleState) account.LifecycleState {
	return account.LifecycleState{
		AccountID:     row.AccountID,
		State:         account.LifecycleStateName(row.State),
		Since:         sqlite.FromMillis(row.Since),
		DeletionDueAt: sqlite.FromNullMillis(row.DeletionDueAt),
	}
}

// NewestRemoteDeletion implements account.Repository.
func (r *Repository) NewestRemoteDeletion(ctx context.Context, tx uow.Tx, accountID string) (account.Initiator, error) {
	var by string
	var err error
	if tx == nil {
		by, err = r.db.reader().FindNewestRemoteDeletion(ctx, accountID)
	} else {
		q, qerr := r.db.queries(tx)
		if qerr != nil {
			return "", qerr
		}
		by, err = q.FindNewestRemoteDeletion(ctx, accountID)
	}
	if err != nil {
		return "", sqlite.NotFound(err, appaccount.ErrNotFound, "remote deletion of "+accountID)
	}
	return account.Initiator(by), nil
}

// UpsertLifecycleState implements account.Repository.
func (r *Repository) UpsertLifecycleState(ctx context.Context, tx uow.Tx, s account.LifecycleState) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertLifecycleState(ctx, accountgen.UpsertLifecycleStateParams{
		AccountID: s.AccountID, State: string(s.State), Since: sqlite.ToMillis(s.Since), DeletionDueAt: sqlite.ToNullMillis(s.DeletionDueAt),
	})
	return sqlite.Wrap(err, "upsert lifecycle state")
}

// DeleteAllLifecycleStates implements account.Repository.
func (r *Repository) DeleteAllLifecycleStates(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteAllLifecycleStates(ctx), "delete lifecycle states")
}

// ListLocalAccounts implements account.Repository.
func (r *Repository) ListLocalAccounts(ctx context.Context, tx uow.Tx) ([]account.LocalAccount, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListLocalAccounts(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list local accounts")
	}
	out := make([]account.LocalAccount, len(rows))
	for i, row := range rows {
		out[i] = account.LocalAccount(row)
	}
	return out, nil
}

// ListProfileRevisionSeqs implements account.Repository.
func (r *Repository) ListProfileRevisionSeqs(ctx context.Context, tx uow.Tx) ([]account.ProfileHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListProfileRevisionSeqs(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list profile revisions")
	}
	out := make([]account.ProfileHead, len(rows))
	for i, row := range rows {
		out[i] = account.ProfileHead{AccountID: row.AccountID, RevisionID: row.ID, Seq: int(row.Seq)}
	}
	return out, nil
}

// ListLifecycleEvents implements account.Repository.
func (r *Repository) ListLifecycleEvents(ctx context.Context, tx uow.Tx) ([]account.LifecycleEvent, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListLifecycleEvents(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list lifecycle events")
	}
	out := make([]account.LifecycleEvent, len(rows))
	for i, row := range rows {
		out[i] = account.LifecycleEvent{
			ID:                 row.ID,
			AccountID:          row.AccountID,
			Kind:               account.LifecycleKind(row.Kind),
			InitiatedBy:        account.Initiator(row.InitiatedBy),
			ModerationActionID: row.ModerationActionID.String,
			DeletionDueAt:      sqlite.FromNullMillis(row.DeletionDueAt),
			OccurredAt:         sqlite.FromMillis(row.OccurredAt),
		}
	}
	return out, nil
}

// ListHandles implements account.Repository.
func (r *Repository) ListHandles(ctx context.Context) ([]account.Handle, error) {
	rows, err := r.db.reader().ListAccountHandles(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list account handles")
	}
	out := make([]account.Handle, len(rows))
	for i, row := range rows {
		out[i] = account.Handle(row)
	}
	return out, nil
}

// ListProfileHeads implements account.Repository.
func (r *Repository) ListProfileHeads(ctx context.Context) ([]account.ProfileHead, error) {
	rows, err := r.db.reader().ListProfileHeads(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list profile heads")
	}
	out := make([]account.ProfileHead, len(rows))
	for i, row := range rows {
		out[i] = account.ProfileHead{AccountID: row.AccountID, RevisionID: row.RevisionID, Seq: int(row.Seq)}
	}
	return out, nil
}

// ListLifecycleStates implements account.Repository.
func (r *Repository) ListLifecycleStates(ctx context.Context) ([]account.LifecycleState, error) {
	rows, err := r.db.reader().ListLifecycleStates(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list lifecycle states")
	}
	out := make([]account.LifecycleState, len(rows))
	for i, row := range rows {
		out[i] = lifecycleState(row)
	}
	return out, nil
}

func accountFrom(row accountgen.Account) account.Account {
	return account.Account{
		ID: row.ID, URI: row.Uri, Domain: row.Domain, ActorType: account.ActorType(row.ActorType), CreatedAt: sqlite.FromMillis(row.CreatedAt),
	}
}

// Account implements account.Repository.
func (r *Repository) Account(ctx context.Context, tx uow.Tx, id string) (account.Account, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.Account{}, err
	}
	row, err := q.GetAccount(ctx, id)
	if err != nil {
		return account.Account{}, sqlite.NotFound(err, appaccount.ErrNotFound, "account "+id)
	}
	return accountFrom(row), nil
}

// FindAccount implements account.Repository.
func (r *Repository) FindAccount(ctx context.Context, id string) (account.Account, error) {
	row, err := r.db.reader().GetAccount(ctx, id)
	if err != nil {
		return account.Account{}, sqlite.NotFound(err, appaccount.ErrNotFound, "account "+id)
	}
	return accountFrom(row), nil
}

// FindAccountByURI implements account.Repository.
func (r *Repository) FindAccountByURI(ctx context.Context, uri string) (account.Account, error) {
	row, err := r.db.reader().GetAccountByURI(ctx, uri)
	if err != nil {
		return account.Account{}, sqlite.NotFound(err, appaccount.ErrNotFound, "account "+uri)
	}
	return accountFrom(row), nil
}

// AnyAccountOnDomain implements account.Repository.
func (r *Repository) AnyAccountOnDomain(ctx context.Context, domain string, ids []string) (bool, error) {
	if len(ids) == 0 {
		return false, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return false, err
	}
	found, err := r.db.reader().AnyAccountOnDomain(ctx, accountgen.AnyAccountOnDomainParams{Domain: domain, Ids: list})
	if err != nil {
		return false, sqlite.Wrap(err, "find accounts on "+domain)
	}
	return found, nil
}

// ExistingAccountIDs implements account.Repository.
func (r *Repository) ExistingAccountIDs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	out, err := r.db.reader().ListExistingAccountIDs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list existing accounts")
	}
	return out, nil
}

// AccountsOnDomain implements account.Repository.
func (r *Repository) AccountsOnDomain(ctx context.Context, tx uow.Tx, domain string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	out, err := q.ListAccountsOnDomain(ctx, accountgen.ListAccountsOnDomainParams{Domain: domain, Ids: list})
	if err != nil {
		return nil, sqlite.Wrap(err, "list accounts on "+domain)
	}
	return out, nil
}

// ListAccountIDsOnDomain implements account.Repository.
func (r *Repository) ListAccountIDsOnDomain(ctx context.Context, domain, after string, limit int) ([]string, error) {
	out, err := r.db.reader().ListAccountIDsOnDomainAfter(ctx, accountgen.ListAccountIDsOnDomainAfterParams{Domain: domain, After: after, PageLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list accounts on "+domain)
	}
	return out, nil
}

// ListLocalAccountIDs implements account.Repository.
func (r *Repository) ListLocalAccountIDs(ctx context.Context, after string, limit int) ([]string, error) {
	out, err := r.db.reader().ListLocalAccountIDsAfter(ctx, accountgen.ListLocalAccountIDsAfterParams{After: after, PageLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list local account ids")
	}
	return out, nil
}

// FindInstanceActor implements account.Repository.
func (r *Repository) FindInstanceActor(ctx context.Context) (account.Account, error) {
	row, err := r.db.reader().GetInstanceActor(ctx)
	if err != nil {
		return account.Account{}, sqlite.NotFound(err, appaccount.ErrNotFound, "instance actor")
	}
	return accountFrom(row), nil
}

// FindLocalAccount implements account.Repository.
func (r *Repository) FindLocalAccount(ctx context.Context, accountID string) (account.LocalAccount, error) {
	row, err := r.db.reader().GetLocalAccount(ctx, accountID)
	if err != nil {
		return account.LocalAccount{}, sqlite.NotFound(err, appaccount.ErrNotFound, "local account "+accountID)
	}
	return account.LocalAccount(row), nil
}

// FindLocalAccountByUser implements account.Repository.
func (r *Repository) FindLocalAccountByUser(ctx context.Context, userID string) (account.LocalAccount, error) {
	row, err := r.db.reader().GetLocalAccountByUserID(ctx, userID)
	if err != nil {
		return account.LocalAccount{}, sqlite.NotFound(err, appaccount.ErrNotFound, "local account of user "+userID)
	}
	return account.LocalAccount(row), nil
}

// FindHandle implements account.Repository.
func (r *Repository) FindHandle(ctx context.Context, usernameNormalized, domain string) (account.Handle, error) {
	row, err := r.db.reader().GetAccountHandleByName(ctx, accountgen.GetAccountHandleByNameParams{
		UsernameNormalized: usernameNormalized, Domain: domain,
	})
	if err != nil {
		return account.Handle{}, sqlite.NotFound(err, appaccount.ErrNotFound, fmt.Sprintf("handle %s@%s", usernameNormalized, domain))
	}
	return account.Handle(row), nil
}

// FindHandleOf implements account.Repository.
func (r *Repository) FindHandleOf(ctx context.Context, accountID string) (account.Handle, error) {
	row, err := r.db.reader().GetAccountHandle(ctx, accountID)
	if err != nil {
		return account.Handle{}, sqlite.NotFound(err, appaccount.ErrNotFound, "handle of "+accountID)
	}
	return account.Handle(row), nil
}

// FindCurrentProfile implements account.Repository.
func (r *Repository) FindCurrentProfile(ctx context.Context, accountID string) (account.ProfileRevision, error) {
	return currentProfile(ctx, r.db.reader(), accountID)
}

// CurrentProfile implements account.Repository.
func (r *Repository) CurrentProfile(ctx context.Context, tx uow.Tx, accountID string) (account.ProfileRevision, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.ProfileRevision{}, err
	}
	return currentProfile(ctx, q, accountID)
}

func currentProfile(ctx context.Context, q *accountgen.Queries, accountID string) (account.ProfileRevision, error) {
	head, err := q.GetProfileHead(ctx, accountID)
	if err != nil {
		return account.ProfileRevision{}, sqlite.NotFound(err, appaccount.ErrNotFound, "profile head of "+accountID)
	}
	row, err := q.GetProfileRevision(ctx, head.RevisionID)
	if err != nil {
		return account.ProfileRevision{}, sqlite.NotFound(err, appaccount.ErrNotFound, "profile revision "+head.RevisionID)
	}
	fields, err := q.ListProfileFields(ctx, row.ID)
	if err != nil {
		return account.ProfileRevision{}, sqlite.Wrap(err, "list profile fields")
	}
	emojis, err := q.ListProfileRevisionEmojis(ctx, row.ID)
	if err != nil {
		return account.ProfileRevision{}, sqlite.Wrap(err, "list profile revision emoji")
	}
	rev := profileParts(profileRow(row), fields, emojis)
	note, err := q.GetProfileRevisionSource(ctx, row.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return rev, nil
	}
	if err != nil {
		return account.ProfileRevision{}, sqlite.Wrap(err, "get profile revision source")
	}
	sources, err := q.ListProfileFieldSources(ctx, row.ID)
	if err != nil {
		return account.ProfileRevision{}, sqlite.Wrap(err, "list profile field sources")
	}
	byPosition := make(map[int]string, len(sources))
	for _, s := range sources {
		byPosition[int(s.Position)] = s.ValueSource
	}
	return profileSource(rev, note, byPosition), nil
}

// profileRow returns the revision of row, without its fields, custom emoji, and source.
func profileRow(row accountgen.AccountProfileRevision) account.ProfileRevision {
	return account.ProfileRevision{
		ID:            row.ID,
		AccountID:     row.AccountID,
		Seq:           int(row.Seq),
		DisplayName:   row.DisplayName,
		NoteHTML:      row.NoteHtml,
		Locked:        row.Locked != 0,
		Bot:           row.Bot != 0,
		Discoverable:  row.Discoverable != 0,
		Indexable:     row.Indexable != 0,
		AIOptOut:      row.AiOptOut != 0,
		AvatarMediaID: row.AvatarMediaID.String,
		HeaderMediaID: row.HeaderMediaID.String,
		RevisedAt:     sqlite.FromMillis(row.RevisedAt),
	}
}

// profileParts returns rev with the fields and the custom emoji of its revision.
func profileParts(rev account.ProfileRevision, fields []accountgen.AccountProfileField, emojis []string) account.ProfileRevision {
	for _, f := range fields {
		rev.Fields = append(rev.Fields, account.ProfileField{Position: int(f.Position), Name: f.Name, ValueHTML: f.ValueHtml})
	}
	if len(emojis) > 0 {
		rev.EmojiIDs = emojis
	}
	return rev
}

// profileSource returns rev, a local account's revision, with its source: the note and the source
// of each field by its position ("" for a field without one).
func profileSource(rev account.ProfileRevision, note string, byPosition map[int]string) account.ProfileRevision {
	rev.Source = &account.ProfileSource{Note: note, FieldValues: make([]string, len(rev.Fields))}
	for i, f := range rev.Fields {
		rev.Source.FieldValues[i] = byPosition[f.Position]
	}
	return rev
}

// FindLifecycleState implements account.Repository.
func (r *Repository) FindLifecycleState(ctx context.Context, accountID string) (account.LifecycleState, error) {
	row, err := r.db.reader().GetLifecycleState(ctx, accountID)
	if err != nil {
		return account.LifecycleState{}, sqlite.NotFound(err, appaccount.ErrNotFound, "lifecycle state of "+accountID)
	}
	return lifecycleState(row), nil
}

// ListKeys implements account.Repository.
func (r *Repository) ListKeys(ctx context.Context, accountID string) ([]account.Key, error) {
	rows, err := r.db.reader().ListAccountKeys(ctx, accountID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list account keys")
	}
	out := make([]account.Key, len(rows))
	for i, row := range rows {
		out[i] = keyFrom(row)
	}
	return out, nil
}

// FindSealedPrivateKey implements account.Repository.
func (r *Repository) FindSealedPrivateKey(ctx context.Context, accountKeyID string) ([]byte, error) {
	sealed, err := r.db.reader().GetLocalAccountPrivateKey(ctx, accountKeyID)
	if err != nil {
		return nil, sqlite.NotFound(err, appaccount.ErrNotFound, "private key of "+accountKeyID)
	}
	return sealed, nil
}

// IsMediaReferenced implements account.Repository.
func (r *Repository) IsMediaReferenced(ctx context.Context, mediaID string) (bool, error) {
	ok, err := r.db.reader().IsMediaReferencedByProfiles(ctx, sqlite.ToNullString(mediaID))
	return ok, sqlite.Wrap(err, "look up profile media")
}

// IsEmojiReferenced implements account.Repository.
func (r *Repository) IsEmojiReferenced(ctx context.Context, emojiID string) (bool, error) {
	ok, err := r.db.reader().IsEmojiReferencedByProfiles(ctx, emojiID)
	return ok, sqlite.Wrap(err, "look up profile emoji")
}

// CountLocalAccounts implements account.Repository.
func (r *Repository) CountLocalAccounts(ctx context.Context) (int, error) {
	n, err := r.db.reader().CountLocalAccounts(ctx)
	if err != nil {
		return 0, sqlite.Wrap(err, "count local accounts")
	}
	return int(n), nil
}
