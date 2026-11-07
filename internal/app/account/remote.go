// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// The account module's types that other modules name in their ports.
type (
	Account    = account.Account
	// ReceivedProfile is what the decision hook inbound.actor reviews of a remote profile.
	ReceivedProfile = account.ReceivedProfile
	Key        = account.Key
	RemoteHead = account.RemoteHead
	Move       = account.Move
	MoveState  = account.MoveState
	Alias      = account.Alias
)

// MoveInitiatedBySelf is the initiator of a local account's own move; see
// account.MoveInitiatedBySelf.
const MoveInitiatedBySelf = account.MoveInitiatedBySelf

// ErrMoveTooSoon reports a move of an account to another target too soon after its newest
// move; see account.ErrMoveTooSoon.
var ErrMoveTooSoon = account.ErrMoveTooSoon

// Handle check outcomes, for HandleCheck.Outcome.
const (
	HandleVerified = account.HandleVerified
	HandleMismatch = account.HandleMismatch
)

// RemoteActorInput is a fetched remote actor document, as the federation module read it
// (federation.md §6.2). The URLs and key IDs are canonical IRIs; the HTML and the display name
// are as received, and UpsertRemoteActor sanitizes them.
type RemoteActorInput struct {
	// URI is the actor's id; Domain is its canonical host.
	URI    string
	Domain string
	// Type is the AS2 actor type (Person, Service, Application, Group, Organization).
	Type              string
	PreferredUsername string
	Name              string
	SummaryHTML       string
	ProfileURL        string
	InboxURL          string
	SharedInboxURL    string
	OutboxURL         string
	FollowersURL      string
	FollowingURL      string
	FeaturedURL       string
	AvatarURL         string
	HeaderURL         string
	Locked            bool
	Bot               bool
	Discoverable      bool
	Indexable         bool
	Fields            []RemoteFieldInput
	// Emojis are the custom emoji of the display name, note, and fields (the actor's Emoji tags).
	Emojis []RemoteEmojiInput
	// Keys are the public keys the document declares for the actor. A stored key of the actor
	// that is missing from them is revoked.
	Keys []RemoteKeyInput
	// Document is the fetched document, whose digest tells a changed snapshot apart.
	Document []byte
	// Handle is the result of the WebFinger check; its zero value keeps the stored handle.
	Handle HandleCheck
	// AccountID is the ID the account gets when the actor is new, "" for a fresh one. A review of
	// the profile made before the account exists names the account by it.
	AccountID string
	// KeepProfile keeps the reviewed part of the stored profile (the display name, the note, the
	// fields, the avatar and the header, and the custom emoji they use) of a known actor, because
	// the decision hook inbound.actor dropped the received one (ADR-0147 §2). Everything else of
	// the document is still stored: the snapshot with the inbox and the other endpoints, the keys,
	// the handle, and the flags of the profile (locked, bot, discoverable, indexable), which the hook
	// does not review and which say how to follow and whether to index the actor. It does nothing
	// for an actor stored for the first time, whose profile is always stored.
	KeepProfile bool
}

// ProfileReview is how a fetched remote profile stands to the stored one, for the decision hook
// inbound.actor (ADR-0147 §2, §6).
type ProfileReview struct {
	// Profile is what the hook reviews, and Digest its digest, which names the subject.
	Profile ReceivedProfile
	Digest  []byte
	// AccountID is the stored account of the actor, "" for an actor not stored yet.
	AccountID string
	// Changed says that Profile differs from the stored profile, always true for an actor not
	// stored yet: an actor fetched again with the same profile is not reviewed again.
	Changed bool
}

// ReviewProfile returns what the hook inbound.actor reviews of a fetched actor, outside any
// transaction. The profile is sanitized as UpsertRemoteActor stores it. A local actor has nothing
// to review, and UpsertRemoteActor refuses it.
func (s *Service) ReviewProfile(ctx context.Context, in RemoteActorInput) (ProfileReview, error) {
	if s.d.Sanitizer == nil {
		return ProfileReview{}, errors.New("account: remote actors need an HTML sanitizer")
	}
	p := s.receivedProfile(in)
	r := ProfileReview{Profile: p, Digest: p.Digest(), Changed: true}
	a, err := s.d.Repository.FindAccountByURI(ctx, in.URI)
	switch {
	case errors.Is(err, ErrNotFound):
		return r, nil
	case err != nil:
		return ProfileReview{}, err
	case a.IsLocal():
		return ProfileReview{}, nil
	}
	r.AccountID = a.ID
	switch stored, err := s.d.Repository.FindRemoteProfileDigest(ctx, a.ID); {
	case errors.Is(err, ErrNotFound):
	case err != nil:
		return ProfileReview{}, err
	default:
		r.Changed = !bytes.Equal(stored, r.Digest)
	}
	return r, nil
}

// receivedProfile is the reviewed part of the profile of in, sanitized as it is stored.
func (s *Service) receivedProfile(in RemoteActorInput) account.ReceivedProfile {
	san := s.d.Sanitizer
	p := account.ReceivedProfile{
		DisplayName: strings.TrimSpace(san.PlainText(in.Name)),
		NoteHTML:    sanitizeOptional(san.SanitizeHTML, in.SummaryHTML),
		AvatarURL:   in.AvatarURL, HeaderURL: in.HeaderURL,
	}
	for _, f := range in.Fields {
		p.Fields = append(p.Fields, account.ReceivedProfileField{
			Name: strings.TrimSpace(san.PlainText(f.Name)), ValueHTML: sanitizeOptional(san.SanitizeHTML, f.ValueHTML),
		})
	}
	return p
}

// RemoteFieldInput is a PropertyValue of a remote profile, with its value as received.
type RemoteFieldInput struct {
	Name      string
	ValueHTML string
}

// RemoteEmojiInput is a custom emoji that a remote profile uses.
type RemoteEmojiInput struct {
	URI       string // the id of the Emoji object, "" when it has none
	Shortcode string // without colons
	ImageURL  string
}

// RemoteKeyInput is a public key of a remote actor: *rsa.PublicKey or ed25519.PublicKey.
type RemoteKeyInput struct {
	KeyID     string
	PublicKey crypto.PublicKey
}

// HandleCheck is the result of checking a remote actor's handle with WebFinger. The zero value
// means that the check did not run or did not get an answer.
type HandleCheck struct {
	Outcome  account.HandleOutcome
	Username string
	Domain   string
}

// Event types of the remote actors, for uow.Projectors.
const (
	EventRemoteActorFetched = "account.remote_actor_fetched"
	EventHandleChecked      = "account.handle_checked"
)

// RemoteActorFetched is emitted when a remote actor's document is stored: Head is its new head.
type RemoteActorFetched struct {
	Head account.RemoteHead
}

// EventType implements uow.Event.
func (RemoteActorFetched) EventType() string { return EventRemoteActorFetched }

// HandleChecked is emitted when a handle check with a new result is recorded.
type HandleChecked struct {
	Verification account.HandleVerification
}

// EventType implements uow.Event.
func (HandleChecked) EventType() string { return EventHandleChecked }

// ActorTypeOf maps an AS2 actor type to the stored one; unknown types are persons.
func ActorTypeOf(as2Type string) account.ActorType {
	switch as2Type {
	case "Service":
		return account.ActorService
	case "Application":
		return account.ActorApplication
	case "Group":
		return account.ActorGroup
	case "Organization":
		return account.ActorOrganization
	}
	return account.ActorPerson
}

// UpsertRemoteActor stores a fetched remote actor inside tx (federation.md §6.2): the account
// and its remote subtype the first time, a snapshot when the document changed, its head, a
// profile revision when the sanitized profile changed, its keys (a key whose fingerprint changed
// under the same key ID replaces the old one, which is revoked, and so is a key the document no
// longer declares), and the handle check when its result changed. It fails with ErrActorDeleted
// for an actor that is deleted, whose data the erasure removes for good, and with ErrNotRemote
// when the URI is a local account's.
func (s *Service) UpsertRemoteActor(ctx context.Context, tx uow.Tx, in RemoteActorInput) (account.Account, error) {
	if s.d.Sanitizer == nil {
		return account.Account{}, errors.New("account: remote actors need an HTML sanitizer")
	}
	if in.URI == "" || in.Domain == "" || in.InboxURL == "" {
		return account.Account{}, errors.New("account: a remote actor needs an id, a domain, and an inbox")
	}
	repo := s.d.Repository
	now := s.d.Clock.Now()
	a, err := repo.AccountByURI(ctx, tx, in.URI)
	switch {
	case errors.Is(err, ErrNotFound):
		id := in.AccountID
		if id == "" {
			id = s.d.IDs.NewID()
		}
		a = account.Account{
			ID: id, URI: in.URI, Domain: account.NormalizeDomain(in.Domain),
			ActorType: ActorTypeOf(in.Type), CreatedAt: now,
		}
		if err := repo.InsertAccount(ctx, tx, a); err != nil {
			return account.Account{}, err
		}
		if _, err := repo.InsertRemoteAccount(ctx, tx, account.RemoteAccount{AccountID: a.ID, FirstSeenAt: now}); err != nil {
			return account.Account{}, err
		}
	case err != nil:
		return account.Account{}, err
	case a.IsLocal():
		return account.Account{}, fmt.Errorf("%s: %w", in.URI, ErrNotRemote)
	default:
		switch st, err := repo.LifecycleState(ctx, tx, a.ID); {
		case errors.Is(err, ErrNotFound):
		case err != nil:
			return account.Account{}, err
		case st.State == account.StateDeleted:
			return account.Account{}, fmt.Errorf("%s: %w", in.URI, ErrActorDeleted)
		}
	}

	snap, err := s.recordSnapshot(ctx, tx, a.ID, in, now)
	if err != nil {
		return account.Account{}, err
	}
	head := account.RemoteHead{
		AccountID: a.ID, SnapshotID: snap.ID, InboxURL: snap.InboxURL, SharedInboxURL: snap.SharedInboxURL, LastFetchedAt: now,
	}
	if err := tx.Emit(ctx, RemoteActorFetched{Head: head}); err != nil {
		return account.Account{}, err
	}
	if err := s.reviseRemoteProfile(ctx, tx, a.ID, snap.ID, in, now); err != nil {
		return account.Account{}, err
	}
	if err := s.rotateRemoteKeys(ctx, tx, a, in.Keys, now); err != nil {
		return account.Account{}, err
	}
	if err := s.recordHandleCheck(ctx, tx, a.ID, in.Handle, now); err != nil {
		return account.Account{}, err
	}
	return a, nil
}

func (s *Service) recordSnapshot(ctx context.Context, tx uow.Tx, accountID string, in RemoteActorInput, now time.Time) (account.RemoteSnapshot, error) {
	sum := sha256.Sum256(in.Document)
	last, err := s.d.Repository.LatestRemoteSnapshot(ctx, tx, accountID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return account.RemoteSnapshot{}, err
	}
	if last.SameDocument(sum[:]) {
		return last, nil
	}
	snap := account.RemoteSnapshot{
		ID: s.d.IDs.NewID(), AccountID: accountID, PreferredUsername: in.PreferredUsername,
		ProfileURL: in.ProfileURL, InboxURL: in.InboxURL, SharedInboxURL: in.SharedInboxURL,
		OutboxURL: in.OutboxURL, FollowersURL: in.FollowersURL, FollowingURL: in.FollowingURL, FeaturedURL: in.FeaturedURL,
		DocumentDigest: sum[:], FetchedAt: now,
	}
	return snap, s.d.Repository.InsertRemoteSnapshot(ctx, tx, snap)
}

// reviseRemoteProfile records a revision when the sanitized profile differs from the current
// one (data-model §4.2: remote revisions are made only when the content changed). With
// in.KeepProfile, a known actor's revision keeps the reviewed part of the current one and takes
// only the flags of in.
func (s *Service) reviseRemoteProfile(ctx context.Context, tx uow.Tx, accountID, snapshotID string, in RemoteActorInput, now time.Time) error {
	repo := s.d.Repository
	cur, err := repo.CurrentProfile(ctx, tx, accountID)
	first := errors.Is(err, ErrNotFound)
	if err != nil && !first {
		return err
	}
	rev := account.ProfileRevision{
		AccountID: accountID, Locked: in.Locked, Bot: in.Bot, Discoverable: in.Discoverable, Indexable: in.Indexable,
	}
	var digest []byte
	if in.KeepProfile && !first {
		rev.DisplayName, rev.NoteHTML, rev.Fields = cur.DisplayName, cur.NoteHTML, cur.Fields
		rev.AvatarMediaID, rev.HeaderMediaID, rev.EmojiIDs = cur.AvatarMediaID, cur.HeaderMediaID, cur.EmojiIDs
		switch digest, err = repo.RemoteProfileDigest(ctx, tx, accountID); {
		case errors.Is(err, ErrNotFound):
			digest = []byte{} // a digest of no profile, which every received one differs from
		case err != nil:
			return err
		}
	} else {
		p := s.receivedProfile(in)
		digest = p.Digest()
		rev.DisplayName, rev.NoteHTML = p.DisplayName, p.NoteHTML
		for i, f := range p.Fields {
			rev.Fields = append(rev.Fields, account.ProfileField{Position: i, Name: f.Name, ValueHTML: f.ValueHTML})
		}
		if rev.AvatarMediaID, err = s.remoteImage(ctx, tx, accountID, in.AvatarURL, account.PropertyAvatar); err != nil {
			return err
		}
		if rev.HeaderMediaID, err = s.remoteImage(ctx, tx, accountID, in.HeaderURL, account.PropertyHeader); err != nil {
			return err
		}
		if len(in.Emojis) > 0 && s.d.RecordRemoteEmojis != nil {
			if rev.EmojiIDs, err = s.d.RecordRemoteEmojis(ctx, tx, accountID, account.NormalizeDomain(in.Domain), in.Emojis); err != nil {
				return err
			}
		}
	}
	if !first && rev.SameContent(cur) {
		return nil
	}
	rev.ID, rev.Seq, rev.RevisedAt = s.d.IDs.NewID(), cur.Seq+1, now
	rev.Remote = &account.RemoteProfileSource{SnapshotID: snapshotID, HTMLPolicyVersion: s.d.Sanitizer.PolicyVersion(), ProfileDigest: digest}
	if err := repo.InsertProfileRevision(ctx, tx, rev); err != nil {
		return err
	}
	return tx.Emit(ctx, ProfileRevised{Revision: rev, IndexableChanged: !first && cur.Indexable != rev.Indexable})
}

func sanitizeOptional(sanitize func(string) string, html string) string {
	if strings.TrimSpace(html) == "" {
		return ""
	}
	return sanitize(html)
}

func (s *Service) remoteImage(ctx context.Context, tx uow.Tx, accountID, url string, prop account.ProfileProperty) (string, error) {
	if url == "" || s.d.RecordRemoteImage == nil {
		return "", nil
	}
	id, err := s.d.RecordRemoteImage(ctx, tx, accountID, url, prop)
	if err != nil {
		return "", fmt.Errorf("account: record remote image %s: %w", url, err)
	}
	return id, nil
}

// rotateRemoteKeys stores the declared keys of a remote actor (data-model §4.2, ADR-0006): a
// key seen before stays, a new fingerprint under a key ID replaces the current key of that ID,
// which is revoked in the same transaction, and a key of the actor that the document no longer
// declares is revoked. A key ID that another account holds is left alone.
func (s *Service) rotateRemoteKeys(ctx context.Context, tx uow.Tx, a account.Account, keys []RemoteKeyInput, now time.Time) error {
	repo := s.d.Repository
	declared := map[string]bool{}
	for _, k := range keys {
		alg, ok := keyAlgorithm(k.PublicKey)
		if !ok || k.KeyID == "" {
			continue
		}
		pemBlock, fingerprint, err := encodePublicKey(k.PublicKey)
		if err != nil {
			continue
		}
		existing, err := repo.KeyByFingerprint(ctx, tx, k.KeyID, fingerprint)
		switch {
		case err == nil:
			if existing.AccountID == a.ID {
				declared[k.KeyID] = true
			}
			continue
		case !errors.Is(err, ErrNotFound):
			return err
		}
		cur, err := repo.CurrentKey(ctx, tx, k.KeyID)
		switch {
		case errors.Is(err, ErrNotFound):
		case err != nil:
			return err
		case cur.AccountID != a.ID:
			continue
		}
		declared[k.KeyID] = true
		key := account.Key{
			ID: s.d.IDs.NewID(), AccountID: a.ID, KeyID: k.KeyID, Algorithm: alg,
			PublicKeyPEM: pemBlock, Fingerprint: fingerprint, CreatedAt: now,
		}
		if err := repo.InsertKey(ctx, tx, key, nil); err != nil {
			return err
		}
		if cur.ID != "" {
			if _, err := repo.InsertKeyRevocation(ctx, tx, account.KeyRevocation{AccountKeyID: cur.ID, RevokedAt: now}); err != nil {
				return err
			}
		}
	}
	active, err := repo.ListActiveKeys(ctx, tx, a.ID)
	if err != nil {
		return err
	}
	for _, k := range active {
		if declared[k.KeyID] {
			continue
		}
		if _, err := repo.InsertKeyRevocation(ctx, tx, account.KeyRevocation{AccountKeyID: k.ID, RevokedAt: now}); err != nil {
			return err
		}
	}
	return nil
}

func keyAlgorithm(pub crypto.PublicKey) (account.KeyAlgorithm, bool) {
	switch pub.(type) {
	case *rsa.PublicKey:
		return account.KeyRSASHA256, true
	case ed25519.PublicKey:
		return account.KeyEd25519, true
	}
	return "", false
}

func (s *Service) recordHandleCheck(ctx context.Context, tx uow.Tx, accountID string, c HandleCheck, now time.Time) error {
	if c.Outcome == "" {
		return nil
	}
	v := account.HandleVerification{
		AccountID: accountID, Username: c.Username, Domain: account.NormalizeDomain(c.Domain), Outcome: c.Outcome, CheckedAt: now,
	}
	last, err := s.d.Repository.LatestHandleVerification(ctx, tx, accountID)
	switch {
	case errors.Is(err, ErrNotFound):
	case err != nil:
		return err
	case last.SameResult(v):
		return nil
	}
	v.ID = s.d.IDs.NewID()
	if err := s.d.Repository.InsertHandleVerification(ctx, tx, v); err != nil {
		return err
	}
	return tx.Emit(ctx, HandleChecked{Verification: v})
}

// SaveRemoteActor is UpsertRemoteActor in a transaction of its own; it returns the stored actor.
func (s *Service) SaveRemoteActor(ctx context.Context, in RemoteActorInput) (Actor, error) {
	var a account.Account
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		a, err = s.UpsertRemoteActor(ctx, tx, in)
		return err
	})
	if err != nil {
		return Actor{}, err
	}
	return s.actor(ctx, a)
}

// MarkRemoteDeleted records inside tx that a remote actor deleted itself with a signed Delete,
// which the erasure takes as it is. An actor that a 410 marked gets the event too, so that the
// erasure no longer asks its server; for one that a Delete marked already this does nothing, so
// that redelivered deletions are no-ops.
func (s *Service) MarkRemoteDeleted(ctx context.Context, tx uow.Tx, accountID string) error {
	deleted, err := s.remoteDeleted(ctx, tx, accountID)
	if err != nil {
		return err
	}
	if deleted {
		switch by, err := s.d.Repository.NewestRemoteDeletion(ctx, tx, accountID); {
		case errors.Is(err, ErrNotFound):
			return nil
		case err != nil:
			return err
		case by != account.InitiatedBySystem:
			return nil
		}
	}
	return s.recordLifecycle(ctx, tx, accountID, account.LifecycleRemoteDeleted, account.InitiatedByRemote)
}

// MarkRemoteGone records inside tx that a fetch of a remote actor answered 410 Gone. It hides the
// actor as a Delete does, but the erasure fetches the actor once more first (RecheckDeletion). An
// account that is deleted already is left as it is.
func (s *Service) MarkRemoteGone(ctx context.Context, tx uow.Tx, accountID string) error {
	deleted, err := s.remoteDeleted(ctx, tx, accountID)
	if err != nil || deleted {
		return err
	}
	return s.recordLifecycle(ctx, tx, accountID, account.LifecycleRemoteDeleted, account.InitiatedBySystem)
}

// RestoreRemoteActor takes back inside tx the mark of a 410 on an actor whose server serves it
// again. It fails with ErrActorDeleted when the actor's own Delete marked it, which is final.
func (s *Service) RestoreRemoteActor(ctx context.Context, tx uow.Tx, accountID string) error {
	deleted, err := s.remoteDeleted(ctx, tx, accountID)
	if err != nil || !deleted {
		return err
	}
	switch by, err := s.d.Repository.NewestRemoteDeletion(ctx, tx, accountID); {
	case err != nil && !errors.Is(err, ErrNotFound):
		return err
	case err != nil || by != account.InitiatedBySystem:
		return fmt.Errorf("account %s: %w", accountID, ErrActorDeleted)
	}
	return s.recordLifecycle(ctx, tx, accountID, account.LifecycleReactivated, account.InitiatedBySystem)
}

// RemoteDeletionUnconfirmed reports whether a 410 is all that marked the account deleted: no
// signed Delete of the actor followed it.
func (s *Service) RemoteDeletionUnconfirmed(ctx context.Context, accountID string) (bool, error) {
	by, err := s.d.Repository.NewestRemoteDeletion(ctx, nil, accountID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return by == account.InitiatedBySystem, err
}

// remoteDeleted reports whether the remote account is deleted, and fails with ErrNotRemote for a
// local one.
func (s *Service) remoteDeleted(ctx context.Context, tx uow.Tx, accountID string) (bool, error) {
	a, err := s.d.Repository.Account(ctx, tx, accountID)
	if err != nil {
		return false, err
	}
	if a.IsLocal() {
		return false, fmt.Errorf("account %s: %w", accountID, ErrNotRemote)
	}
	st, err := s.d.Repository.LifecycleState(ctx, tx, accountID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return st.State == account.StateDeleted, nil
}

func (s *Service) recordLifecycle(ctx context.Context, tx uow.Tx, accountID string, kind account.LifecycleKind, by account.Initiator) error {
	e := account.LifecycleEvent{ID: s.d.IDs.NewID(), AccountID: accountID, Kind: kind, InitiatedBy: by, OccurredAt: s.d.Clock.Now()}
	if err := s.d.Repository.InsertLifecycleEvent(ctx, tx, e); err != nil {
		return err
	}
	return tx.Emit(ctx, LifecycleEventRecorded{Event: e})
}

// RecordRemoteFetchFailure counts a failed fetch of a remote actor and defers its next periodic
// refresh by RemoteActorTTL. An actor that was never fetched is left as it is.
func (s *Service) RecordRemoteFetchFailure(ctx context.Context, accountID string) error {
	now := s.d.Clock.Now()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := s.d.Repository.RecordRemoteFetchFailure(ctx, tx, accountID, now)
		return err
	})
}

// RemoteHead returns the head of a remote actor. It fails with ErrNotFound for a local account
// and for a remote one that has never been fetched.
func (s *Service) RemoteHead(ctx context.Context, accountID string) (account.RemoteHead, error) {
	return s.d.Repository.FindRemoteHead(ctx, accountID)
}

// RemoteProfileURL returns the profile page of a remote actor, the url of its newest document:
// "" when the document has none. It fails with ErrNotFound for a local account and for a remote
// one that has never been fetched.
func (s *Service) RemoteProfileURL(ctx context.Context, accountID string) (string, error) {
	snap, err := s.d.Repository.FindLatestRemoteSnapshot(ctx, accountID)
	if err != nil {
		return "", err
	}
	return snap.ProfileURL, nil
}

// RemoteProfileURLs returns the profile pages of the remote actors in accountIDs, keyed by
// account ID, from their current snapshots in one repository call, so that a page of accounts
// is presented without a lookup per account. Local accounts, actors never fetched, and
// documents without a url are absent.
func (s *Service) RemoteProfileURLs(ctx context.Context, accountIDs []string) (map[string]string, error) {
	if len(accountIDs) == 0 {
		return map[string]string{}, nil
	}
	return s.d.Repository.ListRemoteProfileURLs(ctx, accountIDs)
}

// RemoteFollowersURL returns the followers collection of a remote actor, from its newest
// document: "" when the document has none. It fails with ErrNotFound for a local account and for
// a remote one that has never been fetched.
func (s *Service) RemoteFollowersURL(ctx context.Context, accountID string) (string, error) {
	snap, err := s.d.Repository.FindLatestRemoteSnapshot(ctx, accountID)
	if err != nil {
		return "", err
	}
	return snap.FollowersURL, nil
}

// RemoteOutboxURL returns the outbox of a remote actor, from its newest document: "" when the
// document has none. It fails with ErrNotFound for a local account and for a remote one that has
// never been fetched.
func (s *Service) RemoteOutboxURL(ctx context.Context, accountID string) (string, error) {
	snap, err := s.d.Repository.FindLatestRemoteSnapshot(ctx, accountID)
	if err != nil {
		return "", err
	}
	return snap.OutboxURL, nil
}

// RemoteFeaturedURL returns the featured collection (the pinned posts) of a remote actor, from its
// newest document: "" when the document has none. It fails with ErrNotFound for a local account
// and for a remote one that has never been fetched.
func (s *Service) RemoteFeaturedURL(ctx context.Context, accountID string) (string, error) {
	snap, err := s.d.Repository.FindLatestRemoteSnapshot(ctx, accountID)
	if err != nil {
		return "", err
	}
	return snap.FeaturedURL, nil
}

// StaleRemoteActors returns at most limit remote actors whose last fetch is RemoteActorTTL or
// more old, the least recently fetched first, starting after the head after (the zero value
// for the first page).
func (s *Service) StaleRemoteActors(ctx context.Context, after account.RemoteHead, limit int) ([]account.RemoteHead, error) {
	return s.d.Repository.ListStaleRemoteHeads(ctx, s.d.Clock.Now().Add(-account.RemoteActorTTL), after, limit)
}

// CurrentKey returns the key that verifies signatures naming keyID: the newest one without a
// revocation. It fails with ErrNotFound when there is none.
func (s *Service) CurrentKey(ctx context.Context, keyID string) (account.Key, error) {
	return s.d.Repository.FindCurrentKey(ctx, keyID)
}
