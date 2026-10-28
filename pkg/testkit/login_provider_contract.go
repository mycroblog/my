// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"reflect"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// LoginProviderHarness is what LoginProviderRepositoryContract needs from a database adapter:
// its unit of work and the identity repository, which also erases a user's data.
type LoginProviderHarness struct {
	UnitOfWork uow.UnitOfWork
	Identity   interface {
		appidentity.Repository
		appidentity.ErasureRepository
	}
}

// LoginProviderFactory returns a harness on a new, empty database. The factory registers the
// cleanup of everything it creates with t.Cleanup.
type LoginProviderFactory func(t *testing.T) LoginProviderHarness

// LoginProviderRepositoryContract verifies the login provider repository of a database adapter
// (data-model §4.1, ADR-0106 §2): a provider of each kind keeps its settings with the claims,
// Organizations, guilds, and scopes of its current revision, its newest secret, and its state; a deleted
// provider is neither found nor listed; an external identity is stored once per provider and
// subject; it is linked to one user at a time, and a user links one account per provider; an
// unlink ends a link once and frees the identity; the projections rebuild to what the commands
// wrote; and erasing a user removes its links and the identities that only it linked.
func LoginProviderRepositoryContract(t *testing.T, newHarness LoginProviderFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *loginProviderEnv)
	}{
		{"KeepsProvidersOfEachKind", loginProviderKinds},
		{"RevisesRotatesAndDeletes", loginProviderLifecycle},
		{"LinksOneUserPerIdentityAndOneIdentityPerProvider", loginProviderLinks},
		{"RebuildsItsProjections", loginProviderRebuild},
		{"ErasesTheLinksOfAUser", loginProviderErasure},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clock := NewClock(time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC))
			tc.run(t, &loginProviderEnv{h: newHarness(t), clock: clock, ids: NewIDGenerator(clock)})
		})
	}
}

type loginProviderEnv struct {
	h     LoginProviderHarness
	clock *Clock
	ids   *IDGenerator
}

func (e *loginProviderEnv) tx(t *testing.T, f func(tx uow.Tx) error) {
	t.Helper()
	if err := e.h.UnitOfWork.Tx(t.Context(), f); err != nil {
		t.Fatalf("Tx = %v", err)
	}
}

// create sets up a provider with s, a secret, and the state enabled, and returns its ID.
func (e *loginProviderEnv) create(t *testing.T, s identity.LoginProviderSettings, by string) string {
	t.Helper()
	ctx := t.Context()
	now := e.clock.Advance(time.Second)
	id, revID, secretID, changeID := e.ids.NewID(), e.ids.NewID(), e.ids.NewID(), e.ids.NewID()
	r := e.h.Identity
	e.tx(t, func(tx uow.Tx) error {
		if err := r.InsertLoginProvider(ctx, tx, id, by, now); err != nil {
			return err
		}
		if err := r.InsertLoginProviderRevision(ctx, tx, identity.LoginProviderRevision{
			ID: revID, ProviderID: id, Settings: s, RevisedByUserID: by, RevisedAt: now,
		}); err != nil {
			return err
		}
		if err := r.InsertLoginProviderSecret(ctx, tx, identity.LoginProviderSecret{
			ID: secretID, ProviderID: id, Sealed: []byte("sealed-" + secretID), CreatedByUserID: by, CreatedAt: now,
		}); err != nil {
			return err
		}
		if err := r.InsertLoginProviderStateChange(ctx, tx, identity.LoginProviderStateChange{
			ID: changeID, ProviderID: id, State: identity.LoginProviderEnabled, ChangedByUserID: by, ChangedAt: now,
		}); err != nil {
			return err
		}
		return r.UpsertLoginProviderHead(ctx, tx, identity.LoginProviderHead{
			ProviderID: id, RevisionID: revID, State: identity.LoginProviderEnabled, SecretID: secretID,
		})
	})
	return id
}

// setState records a state change of a provider and moves its head.
func (e *loginProviderEnv) setState(t *testing.T, id string, state identity.LoginProviderState) {
	t.Helper()
	ctx := t.Context()
	r := e.h.Identity
	e.tx(t, func(tx uow.Tx) error {
		h, err := r.LoginProviderHeadForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		c := identity.LoginProviderStateChange{ID: e.ids.NewID(), ProviderID: id, State: state, ChangedAt: e.clock.Advance(time.Second)}
		if err := r.InsertLoginProviderStateChange(ctx, tx, c); err != nil {
			return err
		}
		h.State = state
		return r.UpsertLoginProviderHead(ctx, tx, h)
	})
}

func (e *loginProviderEnv) find(t *testing.T, id string) identity.LoginProvider {
	t.Helper()
	p, err := e.h.Identity.FindLoginProvider(t.Context(), id)
	if err != nil {
		t.Fatalf("FindLoginProvider(%s) = %v", id, err)
	}
	return p
}

func oidcSettings() identity.LoginProviderSettings {
	s := identity.NewLoginProviderSettings(identity.LoginProviderOIDC)
	s.DisplayName = "Example Workspace"
	s.Issuer = "https://accounts.google.com"
	s.ClientID = "client.apps.googleusercontent.com"
	s.Scopes = []string{"email", "profile"}
	s.Claims = []identity.ClaimCondition{
		{Claim: "groups", Values: []string{"staff"}},
		{Claim: "hd", Values: []string{"example.com", "example.org"}},
	}
	return s
}

func githubSettings() identity.LoginProviderSettings {
	s := identity.NewLoginProviderSettings(identity.LoginProviderGitHub)
	s.DisplayName = "GitHub"
	s.ClientID = "Iv1.0123456789abcdef"
	s.AllowSignup = true
	s.Organizations = []string{"acme", "mycroblog"}
	return s
}

func discordSettings() identity.LoginProviderSettings {
	s := identity.NewLoginProviderSettings(identity.LoginProviderDiscord)
	s.DisplayName = "Discord"
	s.ClientID = "1234567890123456789"
	s.Guilds = []string{"1000000000000000001", "2000000000000000002"}
	return s
}

func mastodonSettings() identity.LoginProviderSettings {
	s := identity.NewLoginProviderSettings(identity.LoginProviderMastodon)
	s.DisplayName = "mastodon.social"
	s.Issuer = "https://mastodon.social"
	s.ClientID = "mastodon-client"
	s.AllowLink = false
	return s
}

// loginProviderKinds stores a provider of each kind and reads its settings back as written,
// newest first in the list, and its secret.
func loginProviderKinds(t *testing.T, e *loginProviderEnv) {
	ctx := t.Context()
	admin := e.ids.NewID()
	settings := []identity.LoginProviderSettings{oidcSettings(), githubSettings(), mastodonSettings(), discordSettings()}
	ids := make([]string, len(settings))
	for i, s := range settings {
		by := admin
		if i == 2 {
			by = "" // the CLI
		}
		ids[i] = e.create(t, s, by)
	}
	for i, id := range ids {
		p := e.find(t, id)
		if !reflect.DeepEqual(p.Settings, settings[i]) {
			t.Errorf("settings of %s = %+v\nwant %+v", settings[i].Kind, p.Settings, settings[i])
		}
		wantBy := admin
		if i == 2 {
			wantBy = ""
		}
		if p.ID != id || p.CreatedByUserID != wantBy || p.State != identity.LoginProviderEnabled || p.SecretID == "" ||
			p.CreatedAt.IsZero() || !p.RevisedAt.Equal(p.CreatedAt) || !p.StateChangedAt.Equal(p.CreatedAt) {
			t.Errorf("provider %s = %+v", settings[i].Kind, p)
		}
		secret, err := e.h.Identity.FindLoginProviderSecret(ctx, id)
		if err != nil || secret.ID != p.SecretID || string(secret.Sealed) != "sealed-"+p.SecretID || secret.CreatedByUserID != wantBy {
			t.Errorf("FindLoginProviderSecret(%s) = %+v, %v", settings[i].Kind, secret, err)
		}
	}
	list, err := e.h.Identity.ListLoginProviders(ctx)
	if err != nil {
		t.Fatalf("ListLoginProviders = %v", err)
	}
	got := make([]string, len(list))
	for i, p := range list {
		got[i] = p.ID
		if !reflect.DeepEqual(p, e.find(t, p.ID)) {
			t.Errorf("listed %+v, found %+v", p, e.find(t, p.ID))
		}
	}
	if want := []string{ids[3], ids[2], ids[1], ids[0]}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListLoginProviders = %v, want newest first %v", got, want)
	}
	unknown := e.ids.NewID()
	if _, err := e.h.Identity.FindLoginProvider(ctx, unknown); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindLoginProvider(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := e.h.Identity.FindLoginProviderSecret(ctx, unknown); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindLoginProviderSecret(unknown) = %v, want ErrNotFound", err)
	}
	e.tx(t, func(tx uow.Tx) error {
		if _, err := e.h.Identity.LoginProviderHeadForUpdate(ctx, tx, unknown); !errors.Is(err, appidentity.ErrNotFound) {
			t.Errorf("LoginProviderHeadForUpdate(unknown) = %v, want ErrNotFound", err)
		}
		return nil
	})
}

// loginProviderLifecycle revises a provider (the new revision's claims replace the old ones),
// rotates its secret, disables and enables it, and deletes it, after which it is neither found
// nor listed while its head stays.
func loginProviderLifecycle(t *testing.T, e *loginProviderEnv) {
	ctx := t.Context()
	r := e.h.Identity
	id := e.create(t, oidcSettings(), "")
	other := e.create(t, githubSettings(), "")
	admin := e.ids.NewID()

	next := oidcSettings()
	next.DisplayName = "Renamed"
	next.Scopes = nil
	next.Claims = []identity.ClaimCondition{{Claim: "hd", Values: []string{"example.net"}}}
	next.AllowSignup = true
	revisedAt := e.clock.Advance(time.Minute)
	revID, secretID := e.ids.NewID(), e.ids.NewID()
	e.tx(t, func(tx uow.Tx) error {
		h, err := r.LoginProviderHeadForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := r.InsertLoginProviderRevision(ctx, tx, identity.LoginProviderRevision{
			ID: revID, ProviderID: id, Settings: next, RevisedByUserID: admin, RevisedAt: revisedAt,
		}); err != nil {
			return err
		}
		if err := r.InsertLoginProviderSecret(ctx, tx, identity.LoginProviderSecret{
			ID: secretID, ProviderID: id, Sealed: []byte("rotated"), CreatedByUserID: admin, CreatedAt: revisedAt,
		}); err != nil {
			return err
		}
		h.RevisionID, h.SecretID = revID, secretID
		return r.UpsertLoginProviderHead(ctx, tx, h)
	})
	p := e.find(t, id)
	if !reflect.DeepEqual(p.Settings, next) || p.RevisionID != revID || !p.RevisedAt.Equal(revisedAt) || p.SecretID != secretID {
		t.Errorf("revised provider = %+v, want the settings %+v of revision %s and secret %s", p, next, revID, secretID)
	}
	if s, err := r.FindLoginProviderSecret(ctx, id); err != nil || s.ID != secretID || string(s.Sealed) != "rotated" || s.CreatedByUserID != admin {
		t.Errorf("FindLoginProviderSecret after a rotation = %+v, %v", s, err)
	}
	if !reflect.DeepEqual(e.find(t, other).Settings, githubSettings()) {
		t.Errorf("the other provider changed: %+v", e.find(t, other).Settings)
	}

	e.setState(t, id, identity.LoginProviderDisabled)
	if p := e.find(t, id); p.State != identity.LoginProviderDisabled || !p.StateChangedAt.Equal(e.clock.Now()) {
		t.Errorf("disabled provider = %+v", p)
	}
	e.setState(t, id, identity.LoginProviderEnabled)
	if p := e.find(t, id); p.State != identity.LoginProviderEnabled {
		t.Errorf("enabled provider = %+v", p)
	}
	e.setState(t, id, identity.LoginProviderDeleted)
	if _, err := r.FindLoginProvider(ctx, id); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindLoginProvider(deleted) = %v, want ErrNotFound", err)
	}
	if _, err := r.FindLoginProviderSecret(ctx, id); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindLoginProviderSecret(deleted) = %v, want ErrNotFound", err)
	}
	list, err := r.ListLoginProviders(ctx)
	if err != nil || len(list) != 1 || list[0].ID != other {
		t.Errorf("ListLoginProviders after the deletion = %+v, %v; want only %s", list, err, other)
	}
	e.tx(t, func(tx uow.Tx) error {
		h, err := r.LoginProviderHeadForUpdate(ctx, tx, id)
		if err != nil || h.State != identity.LoginProviderDeleted || h.RevisionID != revID {
			t.Errorf("LoginProviderHeadForUpdate(deleted) = %+v, %v", h, err)
		}
		return nil
	})
}

// link stores the external identity (provider, subject) and links it to user, returning the
// stored identity and the error of the link.
func (e *loginProviderEnv) link(t *testing.T, provider, subject, user string) (identity.ExternalIdentity, string, error) {
	t.Helper()
	ctx := t.Context()
	var x identity.ExternalIdentity
	linkID := e.ids.NewID()
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		now := e.clock.Advance(time.Second)
		x, err = e.h.Identity.EnsureExternalIdentity(ctx, tx, identity.ExternalIdentity{
			ID: e.ids.NewID(), ProviderID: provider, Subject: subject, CreatedAt: now,
		})
		if err != nil {
			t.Fatalf("EnsureExternalIdentity = %v", err)
		}
		return e.h.Identity.InsertExternalIdentityLink(ctx, tx, x, identity.ExternalIdentityLink{
			ID: linkID, ExternalIdentityID: x.ID, UserID: user, LinkedAt: now,
		})
	})
	return x, linkID, err
}

func (e *loginProviderEnv) unlink(t *testing.T, linkID string) {
	t.Helper()
	e.tx(t, func(tx uow.Tx) error {
		return e.h.Identity.InsertExternalIdentityUnlink(t.Context(), tx, identity.ExternalIdentityUnlink{
			LinkID: linkID, UnlinkedAt: e.clock.Advance(time.Second),
		})
	})
}

// linkedUser returns the user that the account subject at provider is linked to, "" for none.
func (e *loginProviderEnv) linkedUser(t *testing.T, provider, subject string) string {
	t.Helper()
	l, err := e.h.Identity.FindLinkedExternalIdentity(t.Context(), provider, subject)
	if errors.Is(err, appidentity.ErrNotFound) {
		return ""
	}
	if err != nil {
		t.Fatalf("FindLinkedExternalIdentity = %v", err)
	}
	if l.ProviderID != provider || l.Subject != subject {
		t.Errorf("FindLinkedExternalIdentity(%s, %s) = %+v", provider, subject, l)
	}
	return l.UserID
}

func (e *loginProviderEnv) linkedSubjects(t *testing.T, user string) []string {
	t.Helper()
	links, err := e.h.Identity.ListLinkedExternalIdentities(t.Context(), user)
	if err != nil {
		t.Fatalf("ListLinkedExternalIdentities = %v", err)
	}
	out := []string{}
	for _, l := range links {
		if l.UserID != user {
			t.Errorf("link %+v of another user than %s", l, user)
		}
		out = append(out, l.Subject)
	}
	return out
}

// loginProviderLinks links external identities to users under the rules of
// InsertExternalIdentityLink and ends them with unlinks.
func loginProviderLinks(t *testing.T, e *loginProviderEnv) {
	gh := e.create(t, githubSettings(), "")
	md := e.create(t, mastodonSettings(), "")
	alice, bob := e.ids.NewID(), e.ids.NewID()

	x, aliceGH, err := e.link(t, gh, "1001", alice)
	if err != nil {
		t.Fatalf("link 1001 to alice = %v", err)
	}
	if x.ProviderID != gh || x.Subject != "1001" {
		t.Errorf("EnsureExternalIdentity = %+v", x)
	}
	if got := e.linkedUser(t, gh, "1001"); got != alice {
		t.Errorf("user of 1001 = %q, want alice", got)
	}
	// The same subject at another provider is another identity.
	if got := e.linkedUser(t, md, "1001"); got != "" {
		t.Errorf("user of 1001 at mastodon = %q, want none", got)
	}

	again, _, err := e.link(t, gh, "1001", bob)
	if !errors.Is(err, appidentity.ErrExternalIdentityLinked) {
		t.Errorf("link 1001 to bob = %v, want ErrExternalIdentityLinked", err)
	}
	if again.ID != x.ID {
		t.Errorf("EnsureExternalIdentity again = %s, want the stored %s", again.ID, x.ID)
	}
	if _, _, err := e.link(t, gh, "1002", alice); !errors.Is(err, appidentity.ErrLoginProviderLinked) {
		t.Errorf("link 1002 to alice = %v, want ErrLoginProviderLinked", err)
	}
	if _, _, err := e.link(t, md, "109", alice); err != nil {
		t.Errorf("link a Mastodon account to alice = %v", err)
	}
	if got, want := e.linkedSubjects(t, alice), []string{"1001", "109"}; !reflect.DeepEqual(got, want) {
		t.Errorf("links of alice = %v, want %v", got, want)
	}
	if got := e.linkedSubjects(t, bob); len(got) != 0 {
		t.Errorf("links of bob = %v, want none", got)
	}

	e.unlink(t, aliceGH)
	e.unlink(t, aliceGH) // a second unlink does nothing
	if got := e.linkedUser(t, gh, "1001"); got != "" {
		t.Errorf("user of 1001 after the unlink = %q, want none", got)
	}
	if got, want := e.linkedSubjects(t, alice), []string{"109"}; !reflect.DeepEqual(got, want) {
		t.Errorf("links of alice after the unlink = %v, want %v", got, want)
	}
	if relinked, _, err := e.link(t, gh, "1001", bob); err != nil || relinked.ID != x.ID {
		t.Errorf("link 1001 to bob after the unlink = %+v, %v; want the identity %s", relinked, err, x.ID)
	}
	if got := e.linkedUser(t, gh, "1001"); got != bob {
		t.Errorf("user of 1001 = %q, want bob", got)
	}
	if _, _, err := e.link(t, gh, "1003", alice); err != nil {
		t.Errorf("link 1003 to alice after her unlink = %v", err)
	}
	counts, err := e.h.Identity.CountLoginProviderLinks(t.Context())
	if err != nil || counts[gh] != 2 || counts[md] != 1 {
		t.Errorf("CountLoginProviderLinks = %v, %v; want 2 at github and 1 at mastodon", counts, err)
	}
}

// loginProviderRebuild empties the projections, rebuilds them, and reads what the commands
// wrote.
func loginProviderRebuild(t *testing.T, e *loginProviderEnv) {
	ctx := t.Context()
	r := e.h.Identity
	kept := e.create(t, oidcSettings(), "")
	disabled := e.create(t, githubSettings(), "")
	deleted := e.create(t, mastodonSettings(), "")
	e.setState(t, disabled, identity.LoginProviderDisabled)
	e.setState(t, deleted, identity.LoginProviderDeleted)
	alice, bob := e.ids.NewID(), e.ids.NewID()
	if _, _, err := e.link(t, kept, "sub-a", alice); err != nil {
		t.Fatal(err)
	}
	_, ended, err := e.link(t, disabled, "1", bob)
	if err != nil {
		t.Fatal(err)
	}
	e.unlink(t, ended)
	if _, _, err := e.link(t, disabled, "1", alice); err != nil {
		t.Fatal(err)
	}

	before, err := r.ListLoginProviders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	aliceBefore, bobBefore := e.linkedSubjects(t, alice), e.linkedSubjects(t, bob)
	if err := appidentity.RebuildProjections(ctx, e.h.UnitOfWork, r); err != nil {
		t.Fatalf("RebuildProjections = %v", err)
	}
	after, err := r.ListLoginProviders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) || len(after) != 2 {
		t.Errorf("providers after the rebuild = %+v\nwant %+v", after, before)
	}
	if got := e.linkedSubjects(t, alice); !reflect.DeepEqual(got, aliceBefore) {
		t.Errorf("links of alice after the rebuild = %v, want %v", got, aliceBefore)
	}
	if got := e.linkedSubjects(t, bob); !reflect.DeepEqual(got, bobBefore) {
		t.Errorf("links of bob after the rebuild = %v, want %v", got, bobBefore)
	}
	if got := e.linkedUser(t, disabled, "1"); got != alice {
		t.Errorf("user of 1 after the rebuild = %q, want alice", got)
	}
	e.tx(t, func(tx uow.Tx) error {
		h, err := r.LoginProviderHeadForUpdate(ctx, tx, deleted)
		if err != nil || h.State != identity.LoginProviderDeleted {
			t.Errorf("head of the deleted provider after the rebuild = %+v, %v", h, err)
		}
		return nil
	})
}

// loginProviderErasure erases a user's links and the identities that only the user linked; an
// identity that another user linked after the erased user stays with that user's link.
func loginProviderErasure(t *testing.T, e *loginProviderEnv) {
	ctx := t.Context()
	gh := e.create(t, githubSettings(), "")
	md := e.create(t, mastodonSettings(), "")
	alice, bob := e.ids.NewID(), e.ids.NewID()
	shared, first, err := e.link(t, gh, "1", alice)
	if err != nil {
		t.Fatal(err)
	}
	e.unlink(t, first)
	if _, _, err := e.link(t, gh, "1", bob); err != nil {
		t.Fatal(err)
	}
	own, _, err := e.link(t, md, "2", alice)
	if err != nil {
		t.Fatal(err)
	}

	e.tx(t, func(tx uow.Tx) error { return e.h.Identity.EraseUserData(ctx, tx, alice) })
	if got := e.linkedSubjects(t, alice); len(got) != 0 {
		t.Errorf("links of alice after the erasure = %v, want none", got)
	}
	if got := e.linkedUser(t, md, "2"); got != "" {
		t.Errorf("user of 2 after the erasure = %q, want none", got)
	}
	if got := e.linkedUser(t, gh, "1"); got != bob {
		t.Errorf("user of 1 after erasing alice = %q, want bob", got)
	}
	e.tx(t, func(tx uow.Tx) error {
		x, err := e.h.Identity.EnsureExternalIdentity(ctx, tx, identity.ExternalIdentity{
			ID: e.ids.NewID(), ProviderID: md, Subject: "2", CreatedAt: e.clock.Now(),
		})
		if err != nil || x.ID == own.ID {
			t.Errorf("EnsureExternalIdentity(2) after the erasure = %+v, %v; want a new identity", x, err)
		}
		x, err = e.h.Identity.EnsureExternalIdentity(ctx, tx, identity.ExternalIdentity{
			ID: e.ids.NewID(), ProviderID: gh, Subject: "1", CreatedAt: e.clock.Now(),
		})
		if err != nil || x.ID != shared.ID {
			t.Errorf("EnsureExternalIdentity(1) after the erasure = %+v, %v; want the kept %s", x, err, shared.ID)
		}
		return nil
	})
	if err := appidentity.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Identity); err != nil {
		t.Fatalf("RebuildProjections = %v", err)
	}
	if got := e.linkedUser(t, gh, "1"); got != bob {
		t.Errorf("user of 1 after the erasure and a rebuild = %q, want bob", got)
	}
}
