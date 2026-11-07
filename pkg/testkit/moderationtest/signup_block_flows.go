// SPDX-License-Identifier: AGPL-3.0-only

package moderationtest

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

// SignupBlockFlows verifies the sign-up block list of a database adapter's moderation repository
// (FR-719, data-model §4.3) through the moderation use cases: blocks are stored in the canonical
// form of their kind and invalid ones are refused, a sign-up is screened against the blocks in
// effect only (neither lifted nor expired) by the domain of its email address and its parents, the
// hash of its canonical address, and the IP range of its client, the strongest severity wins, a
// lift is recorded once, a ban can add the block of the banned user's email address in its
// transaction, the blocks list newest first with their lifts and filters, every addition and lift
// is audited once, and the purge of the moderation records erases a block with its lift
// moderation.SignupBlockRetention after it ends (the earlier of its lift and expiry), keeps one in
// effect and one made by the ban of a held account, and is safe to repeat; an import of a list adds
// and audits a block per new value across transactions and skips the invalid, repeated, and already
// blocked ones. These are the flows of ADR-0153 §1, which internal/flowtest/moderation runs once on
// SQLite; SignupBlockContract checks the repository's methods on every adapter.
func SignupBlockFlows(t *testing.T, newHarness ModerationFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *signupBlockEnv)
	}{
		{"StoresCanonicalValues", signupBlocksCanonical},
		{"ScreensSignUpsWithTheBlocksInEffect", signupBlocksScreen},
		{"LiftsOnceAndAudits", signupBlocksLiftAndAudit},
		{"BanBlocksTheEmailAddress", signupBlocksFromBan},
		{"ListsWithFiltersAndPages", signupBlocksList},
		{"PurgesAfterTheyEnd", signupBlocksPurge},
		{"ImportsListsAndSkipsWhatItCannotAdd", signupBlocksImport},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newSignupBlockEnv(t, newHarness)) })
	}
}

type signupBlockEnv struct {
	h      ModerationHarness
	clock  *testkit.Clock
	ids    *testkit.IDGenerator
	svc    *appmoderation.Service
	emails stubEmails
	// local is a local account of the user localUser; remote is a remote account.
	local, localUser, remote string
}

// stubEmails stands in for the identity module: the primary email address of each user.
type stubEmails map[string]string

// PrimaryEmailAddress implements appmoderation.Emails.
func (s stubEmails) PrimaryEmailAddress(_ context.Context, userID string) (string, error) {
	return s[userID], nil
}

func newSignupBlockEnv(t *testing.T, newHarness ModerationFactory) *signupBlockEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	appmoderation.RegisterAuditProjector(projectors, h.Moderation)
	clock := testkit.NewClock(time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC))
	ids := testkit.NewIDGenerator(clock)
	actors := &identitytest.StubActors{Actors: map[string]appaccount.Actor{}}
	e := &signupBlockEnv{h: h, clock: clock, ids: ids, emails: stubEmails{}, local: ids.NewID(), localUser: ids.NewID(), remote: ids.NewID()}
	actors.Set(appaccount.Actor{
		Account: account.Account{ID: e.local, Domain: account.LocalDomain},
		Local:   &account.LocalAccount{AccountID: e.local, UserID: e.localUser, Username: "spammer"},
	})
	actors.Set(appaccount.Actor{Account: account.Account{ID: e.remote, Domain: "remote.example", URI: "https://remote.example/users/bob"}})
	e.emails[e.localUser] = "Spam.Mer+tag@Mail.Example"
	e.svc = appmoderation.NewService(appmoderation.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Moderation, Accounts: actors, Clock: clock, IDs: ids,
		Lifecycles: &stubLifecycles{due: map[string]time.Time{}}, Emails: e.emails,
	})
	return e
}

func (e *signupBlockEnv) add(t *testing.T, in appmoderation.SignupBlockInput) moderation.SignupBlock {
	t.Helper()
	if in.Severity == "" {
		in.Severity = moderation.SignupReject
	}
	b, err := e.svc.AddSignupBlock(t.Context(), in)
	if err != nil {
		t.Fatalf("AddSignupBlock(%+v) = %v", in, err)
	}
	return b
}

func (e *signupBlockEnv) screen(t *testing.T, email, ip string, want moderation.SignupBlockSeverity) {
	t.Helper()
	got, err := e.svc.ScreenSignup(t.Context(), email, ip)
	if err != nil || got != want {
		t.Errorf("ScreenSignup(%q, %q) = %q, %v; want %q", email, ip, got, err, want)
	}
}

func signupBlocksCanonical(t *testing.T, e *signupBlockEnv) {
	hash := moderation.EmailHash("alice@example.com")
	for _, c := range []struct {
		kind       moderation.SignupBlockKind
		value, out string
	}{
		{moderation.SignupBlockEmailDomain, "@Spam.Example.", "spam.example"},
		{moderation.SignupBlockEmailHash, "Alice+x@example.com", hash},
		{moderation.SignupBlockEmailHash, strings.ToUpper(hash), hash},
		{moderation.SignupBlockIPRange, "192.0.2.77/24", "192.0.2.0/24"},
		{moderation.SignupBlockIPRange, "2001:db8::1", "2001:db8::1/128"},
		{moderation.SignupBlockIPRange, "::ffff:198.51.100.0/120", "198.51.100.0/24"},
	} {
		b := e.add(t, appmoderation.SignupBlockInput{Kind: c.kind, Value: c.value})
		if b.Value != c.out {
			t.Errorf("AddSignupBlock(%s %q).Value = %q, want %q", c.kind, c.value, b.Value, c.out)
		}
		v, err := e.svc.SignupBlock(t.Context(), b.ID)
		if err != nil || !reflect.DeepEqual(v.Block, b) || v.State != moderation.ActionActive || v.Lift != nil {
			t.Errorf("SignupBlock(%s) = %+v, %v; want %+v, active", b.ID, v, err, b)
		}
	}
	for _, in := range []appmoderation.SignupBlockInput{
		{Kind: "email", Value: "spam.example", Severity: moderation.SignupReject},
		{Kind: moderation.SignupBlockEmailDomain, Value: "spam.example", Severity: "maybe"},
		{Kind: moderation.SignupBlockEmailDomain, Value: "", Severity: moderation.SignupReject},
		{Kind: moderation.SignupBlockEmailDomain, Value: "192.0.2.1", Severity: moderation.SignupReject},
		{Kind: moderation.SignupBlockEmailHash, Value: "abc", Severity: moderation.SignupReject},
		{Kind: moderation.SignupBlockEmailHash, Value: "@", Severity: moderation.SignupReject},
		{Kind: moderation.SignupBlockIPRange, Value: "192.0.2.0/33", Severity: moderation.SignupReject},
		{Kind: moderation.SignupBlockIPRange, Value: "spam.example", Severity: moderation.SignupReject},
		{Kind: moderation.SignupBlockIPRange, Value: "192.0.2.0/24", Severity: moderation.SignupReject, Duration: -time.Hour},
		{Kind: moderation.SignupBlockIPRange, Value: "192.0.2.0/24", Severity: moderation.SignupReject, Comment: strings.Repeat("x", moderation.MaxSignupBlockComment+1)},
	} {
		if _, err := e.svc.AddSignupBlock(t.Context(), in); !errors.Is(err, appmoderation.ErrInvalidSignupBlock) {
			t.Errorf("AddSignupBlock(%+v) = %v, want ErrInvalidSignupBlock", in, err)
		}
	}
}

func signupBlocksScreen(t *testing.T, e *signupBlockEnv) {
	e.screen(t, "alice@spam.example", "192.0.2.1", "")
	e.add(t, appmoderation.SignupBlockInput{Kind: moderation.SignupBlockEmailDomain, Value: "spam.example", Severity: moderation.SignupRequireApproval})
	e.add(t, appmoderation.SignupBlockInput{Kind: moderation.SignupBlockEmailHash, Value: "carol@good.example"})
	e.add(t, appmoderation.SignupBlockInput{Kind: moderation.SignupBlockIPRange, Value: "192.0.2.0/24", Duration: time.Hour})
	e.add(t, appmoderation.SignupBlockInput{Kind: moderation.SignupBlockIPRange, Value: "2001:db8::/32", Severity: moderation.SignupRequireApproval})

	// The domain covers its subdomains but not look-alikes; the hash ignores case and +tags.
	e.screen(t, "alice@spam.example", "", moderation.SignupRequireApproval)
	e.screen(t, "alice@mx.Spam.Example", "", moderation.SignupRequireApproval)
	e.screen(t, "alice@notspam.example", "", "")
	e.screen(t, "Carol+again@GOOD.example", "", moderation.SignupReject)
	e.screen(t, "carol2@good.example", "", "")
	// The client address matches by range; IPv4-mapped addresses count as IPv4; an unknown or
	// unparsable address matches no range.
	e.screen(t, "dave@good.example", "192.0.2.200", moderation.SignupReject)
	e.screen(t, "dave@good.example", "::ffff:192.0.2.9", moderation.SignupReject)
	e.screen(t, "dave@good.example", "192.0.3.1", "")
	e.screen(t, "dave@good.example", "2001:db8:1::5", moderation.SignupRequireApproval)
	e.screen(t, "dave@good.example", "not an address", "")
	// The strongest severity wins.
	e.screen(t, "dave@spam.example", "192.0.2.1", moderation.SignupReject)

	// An expired block no longer matches.
	e.clock.Advance(time.Hour)
	e.screen(t, "dave@good.example", "192.0.2.200", "")
	e.screen(t, "dave@spam.example", "192.0.2.1", moderation.SignupRequireApproval)
}

func signupBlocksLiftAndAudit(t *testing.T, e *signupBlockEnv) {
	ctx := t.Context()
	b := e.add(t, appmoderation.SignupBlockInput{Kind: moderation.SignupBlockEmailDomain, Value: "spam.example", Comment: "  waves  ", CreatedByUserID: testkit.NamedID("mod-1")})
	if b.Comment != "waves" || b.CreatedByUserID != testkit.NamedID("mod-1") {
		t.Errorf("AddSignupBlock = %+v, want the trimmed comment and the moderator", b)
	}
	e.screen(t, "a@spam.example", "", moderation.SignupReject)
	e.clock.Advance(time.Second)
	if err := e.svc.LiftSignupBlock(ctx, b.ID, ""); err != nil {
		t.Fatalf("LiftSignupBlock = %v", err)
	}
	liftedAt := e.clock.Now()
	e.screen(t, "a@spam.example", "", "")
	// Lifting again changes nothing and records nothing.
	e.clock.Advance(time.Second)
	if err := e.svc.LiftSignupBlock(ctx, b.ID, testkit.NamedID("mod-1")); err != nil {
		t.Errorf("LiftSignupBlock again = %v, want nil", err)
	}
	v, err := e.svc.SignupBlock(ctx, b.ID)
	if err != nil || v.State != moderation.ActionLifted || v.Lift == nil || !v.Lift.LiftedAt.Equal(liftedAt) || v.Lift.LiftedByUserID != "" {
		t.Errorf("SignupBlock after the lifts = %+v, %v; want lifted once by the CLI at %v", v, err, liftedAt)
	}
	if err := e.svc.LiftSignupBlock(ctx, e.ids.NewID(), ""); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("LiftSignupBlock(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.SignupBlock(ctx, e.ids.NewID()); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("SignupBlock(unknown) = %v, want ErrNotFound", err)
	}
	got, err := e.svc.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetSignupBlock, TargetID: b.ID}, 10)
	if err != nil {
		t.Fatal(err)
	}
	checkAudit(t, got, []auditWant{
		{moderation.AuditActorCLI, "", moderation.AuditSignupBlockLifted, moderation.AuditTargetSignupBlock, b.ID, "email_domain spam.example", b.ID, liftedAt},
		{moderation.AuditActorUser, testkit.NamedID("mod-1"), moderation.AuditSignupBlockAdded, moderation.AuditTargetSignupBlock, b.ID, "email_domain spam.example", b.ID, b.CreatedAt},
	})
}

func signupBlocksFromBan(t *testing.T, e *signupBlockEnv) {
	ctx := t.Context()
	ban := func(target string, kind moderation.ActionKind) (moderation.Action, error) {
		return e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
			TargetAccountID: target, Kind: kind, ReasonCategory: moderation.CategorySpam, IssuedByUserID: testkit.NamedID("mod-1"), BlockEmail: true,
		})
	}
	// Only a ban of a local account with an email address blocks it; a refusal writes nothing.
	if _, err := ban(e.local, moderation.ActionSuspend); !errors.Is(err, appmoderation.ErrCannotBlockEmail) {
		t.Errorf("suspend with BlockEmail = %v, want ErrCannotBlockEmail", err)
	}
	if _, err := ban(e.remote, moderation.ActionBan); !errors.Is(err, appmoderation.ErrUnsupportedAction) {
		t.Errorf("ban of a remote account = %v, want ErrUnsupportedAction", err)
	}
	address := e.emails[e.localUser]
	delete(e.emails, e.localUser)
	if _, err := ban(e.local, moderation.ActionBan); !errors.Is(err, appmoderation.ErrCannotBlockEmail) {
		t.Errorf("ban of a user without an email address = %v, want ErrCannotBlockEmail", err)
	}
	if vs, err := e.svc.AccountActions(ctx, appmoderation.AccountActionFilter{AccountID: e.local}, 10); err != nil || len(vs) != 0 {
		t.Errorf("actions after the refusals = %+v, %v; want none", vs, err)
	}
	e.emails[e.localUser] = address

	a, err := ban(e.local, moderation.ActionBan)
	if err != nil {
		t.Fatalf("ban with BlockEmail = %v", err)
	}
	blocks, err := e.svc.SignupBlocks(ctx, appmoderation.SignupBlockFilter{}, 10)
	if err != nil || len(blocks) != 1 {
		t.Fatalf("SignupBlocks = %+v, %v; want the ban's block", blocks, err)
	}
	b := blocks[0].Block
	want := moderation.SignupBlock{
		ID: b.ID, Kind: moderation.SignupBlockEmailHash, Value: moderation.EmailHash("spam.mer@mail.example"), Severity: moderation.SignupReject,
		ModerationActionID: a.ID, CreatedByUserID: testkit.NamedID("mod-1"), CreatedAt: a.IssuedAt,
	}
	if !reflect.DeepEqual(b, want) {
		t.Errorf("ban's block = %+v, want %+v", b, want)
	}
	e.screen(t, "spam.mer+new@mail.example", "", moderation.SignupReject)
	got, err := e.svc.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetSignupBlock}, 10)
	if err != nil {
		t.Fatal(err)
	}
	checkAudit(t, got, []auditWant{
		{moderation.AuditActorUser, testkit.NamedID("mod-1"), moderation.AuditSignupBlockAdded, moderation.AuditTargetSignupBlock, b.ID, "email_hash " + b.Value, b.ID, a.IssuedAt},
	})
}

func signupBlocksList(t *testing.T, e *signupBlockEnv) {
	ctx := t.Context()
	var all []moderation.SignupBlock
	for _, in := range []appmoderation.SignupBlockInput{
		{Kind: moderation.SignupBlockEmailDomain, Value: "a.example"},
		{Kind: moderation.SignupBlockIPRange, Value: "192.0.2.0/24", Duration: time.Minute},
		{Kind: moderation.SignupBlockEmailDomain, Value: "b.example", Severity: moderation.SignupRequireApproval},
		{Kind: moderation.SignupBlockEmailHash, Value: "x@c.example"},
	} {
		e.clock.Advance(time.Second)
		all = append(all, e.add(t, in))
	}
	if err := e.svc.LiftSignupBlock(ctx, all[2].ID, ""); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Hour)
	ids := func(vs []appmoderation.SignupBlockView) []string {
		out := []string{}
		for _, v := range vs {
			out = append(out, v.Block.ID)
		}
		return out
	}
	check := func(name string, f appmoderation.SignupBlockFilter, limit int, want ...moderation.SignupBlock) []appmoderation.SignupBlockView {
		t.Helper()
		vs, err := e.svc.SignupBlocks(ctx, f, limit)
		w := []string{}
		for _, b := range want {
			w = append(w, b.ID)
		}
		if err != nil || !reflect.DeepEqual(ids(vs), w) {
			t.Errorf("%s: SignupBlocks = %v, %v; want %v", name, ids(vs), err, w)
		}
		return vs
	}
	vs := check("all", appmoderation.SignupBlockFilter{}, 10, all[3], all[2], all[1], all[0])
	if len(vs) == 4 {
		for i, state := range []moderation.ActionState{moderation.ActionActive, moderation.ActionLifted, moderation.ActionExpired, moderation.ActionActive} {
			if vs[i].State != state {
				t.Errorf("state of %s = %s, want %s", vs[i].Block.ID, vs[i].State, state)
			}
		}
	}
	check("active", appmoderation.SignupBlockFilter{Active: true}, 10, all[3], all[0])
	check("email_domain", appmoderation.SignupBlockFilter{Kind: moderation.SignupBlockEmailDomain}, 10, all[2], all[0])
	check("page", appmoderation.SignupBlockFilter{Before: all[2].ID}, 1, all[1])
	if _, err := e.svc.SignupBlocks(ctx, appmoderation.SignupBlockFilter{Kind: "email"}, 10); !errors.Is(err, appmoderation.ErrInvalidQuery) {
		t.Errorf("SignupBlocks(unknown kind) = %v, want ErrInvalidQuery", err)
	}
}

func signupBlocksPurge(t *testing.T, e *signupBlockEnv) {
	ctx := t.Context()
	start := e.clock.Now()
	purge := func(want appmoderation.PurgedRecords) {
		t.Helper()
		got, err := e.svc.PurgeRecords(ctx)
		if err != nil || got != want {
			t.Errorf("PurgeRecords at %v = %+v, %v; want %+v", e.clock.Now().Sub(start), got, err, want)
		}
	}
	lifted := e.add(t, appmoderation.SignupBlockInput{Kind: moderation.SignupBlockEmailDomain, Value: "lifted.example"})
	if err := e.svc.LiftSignupBlock(ctx, lifted.ID, ""); err != nil {
		t.Fatal(err)
	}
	expiring := e.add(t, appmoderation.SignupBlockInput{Kind: moderation.SignupBlockEmailDomain, Value: "expiring.example", Duration: 24 * time.Hour})
	// Lifted after it expired, it ended when it expired.
	expiredFirst := e.add(t, appmoderation.SignupBlockInput{Kind: moderation.SignupBlockIPRange, Value: "192.0.2.0/24", Duration: time.Hour})
	indefinite := e.add(t, appmoderation.SignupBlockInput{Kind: moderation.SignupBlockEmailDomain, Value: "spam.example"})
	// The block of a banned account's address stays while a hold preserves the account.
	if _, err := e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.local, Kind: moderation.ActionBan, ReasonCategory: moderation.CategorySpam, BlockEmail: true,
	}); err != nil {
		t.Fatal(err)
	}
	vs, err := e.svc.SignupBlocks(ctx, appmoderation.SignupBlockFilter{Kind: moderation.SignupBlockEmailHash}, 10)
	if err != nil || len(vs) != 1 {
		t.Fatalf("SignupBlocks(email_hash) = %+v, %v; want the ban's block", vs, err)
	}
	banned := vs[0].Block
	if err := e.svc.LiftSignupBlock(ctx, banned.ID, ""); err != nil {
		t.Fatal(err)
	}
	hold, err := e.svc.PlaceRetentionHold(ctx, e.local, "court order", "")
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(48 * time.Hour)
	if err := e.svc.LiftSignupBlock(ctx, expiredFirst.ID, ""); err != nil {
		t.Fatal(err)
	}
	kept := func(bs ...moderation.SignupBlock) {
		t.Helper()
		for _, b := range bs {
			if _, err := e.svc.SignupBlock(ctx, b.ID); err != nil {
				t.Errorf("SignupBlock(%s) = %v; want it kept", b.Value, err)
			}
		}
	}
	purged := func(bs ...moderation.SignupBlock) {
		t.Helper()
		for _, b := range bs {
			if _, err := e.svc.SignupBlock(ctx, b.ID); !errors.Is(err, appmoderation.ErrNotFound) {
				t.Errorf("SignupBlock(%s) = %v; want ErrNotFound", b.Value, err)
			}
		}
	}

	// Within the retention of the earliest end, nothing goes.
	e.clock.Set(start.Add(moderation.SignupBlockRetention - time.Minute))
	purge(appmoderation.PurgedRecords{})
	kept(lifted, expiring, expiredFirst, indefinite, banned)
	// Past it, the blocks that ended then go; the one that expires later, the one in effect, and
	// the held one stay.
	e.clock.Set(start.Add(moderation.SignupBlockRetention + 2*time.Hour))
	purge(appmoderation.PurgedRecords{SignupBlocks: 2})
	purged(lifted, expiredFirst)
	kept(expiring, indefinite, banned)
	e.clock.Set(start.Add(moderation.SignupBlockRetention + 25*time.Hour))
	purge(appmoderation.PurgedRecords{SignupBlocks: 1})
	purged(expiring)
	purge(appmoderation.PurgedRecords{})
	// Released, the held block goes at the next run; the block in effect still screens.
	if _, _, err := e.svc.ReleaseRetentionHold(ctx, hold.ID, ""); err != nil {
		t.Fatal(err)
	}
	purge(appmoderation.PurgedRecords{SignupBlocks: 1})
	purged(banned)
	kept(indefinite)
	e.screen(t, "a@spam.example", "", moderation.SignupReject)
	purge(appmoderation.PurgedRecords{})
	// The lifts went with their blocks: the lift of a purged block is recorded anew (and undone
	// by failing the transaction).
	errUndo := errors.New("undo")
	err = e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		for _, b := range []moderation.SignupBlock{lifted, expiredFirst, banned} {
			if err := e.h.Moderation.InsertSignupBlockLift(ctx, tx, moderation.SignupBlockLift{BlockID: b.ID, LiftedAt: e.clock.Now()}); err != nil {
				t.Errorf("InsertSignupBlockLift(purged %s) = %v; want its lift purged", b.Value, err)
			}
		}
		return errUndo
	})
	if !errors.Is(err, errUndo) {
		t.Fatal(err)
	}
}

func signupBlocksImport(t *testing.T, e *signupBlockEnv) {
	ctx := t.Context()
	domain := moderation.SignupBlockEmailDomain
	e.add(t, appmoderation.SignupBlockInput{Kind: domain, Value: "old.example"})
	e.add(t, appmoderation.SignupBlockInput{Kind: domain, Value: "weak.example", Severity: moderation.SignupRequireApproval})
	lifted := e.add(t, appmoderation.SignupBlockInput{Kind: domain, Value: "lifted.example"})
	if err := e.svc.LiftSignupBlock(ctx, lifted.ID, ""); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Second)

	// More values than one transaction writes, so the import spans several.
	var text strings.Builder
	text.WriteString("# disposable domains\nnew.example\n@Old.Example\nweak.example\nlifted.example\n192.0.2.1\nNEW.example\n\n")
	const bulk = 600
	for i := range bulk {
		fmt.Fprintf(&text, "d%03d.example\n", i)
	}
	in := appmoderation.SignupBlockImportInput{
		Kind: domain, Text: text.String(), Severity: moderation.SignupReject, Comment: " disposable ", CreatedByUserID: testkit.NamedID("mod-1"),
	}
	res, err := e.svc.ImportSignupBlocks(ctx, in)
	if err != nil {
		t.Fatalf("ImportSignupBlocks = %v", err)
	}
	wantSkipped := []appmoderation.SignupBlockImportSkipped{
		{Line: 3, Value: "@Old.Example", Reason: moderation.SignupBlockSkipAlreadyBlocked},
		{Line: 6, Value: "192.0.2.1", Reason: moderation.SignupBlockSkipInvalid},
		{Line: 7, Value: "NEW.example", Reason: moderation.SignupBlockSkipDuplicate},
	}
	// new, weak (a stronger severity than the block in effect), lifted (no block in effect), and the bulk.
	if res.Added != 3+bulk || !reflect.DeepEqual(res.Skipped, wantSkipped) {
		t.Errorf("ImportSignupBlocks = %d added, skipped %+v; want %d, %+v", res.Added, res.Skipped, 3+bulk, wantSkipped)
	}
	e.screen(t, "a@new.example", "", moderation.SignupReject)
	e.screen(t, "a@mx.weak.example", "", moderation.SignupReject)
	e.screen(t, "a@lifted.example", "", moderation.SignupReject)
	e.screen(t, "a@d599.example", "", moderation.SignupReject)
	vs, err := e.svc.SignupBlocks(ctx, appmoderation.SignupBlockFilter{Kind: domain, Active: true}, 1)
	if err != nil || len(vs) != 1 || vs[0].Block.Comment != "disposable" || vs[0].Block.CreatedByUserID != testkit.NamedID("mod-1") || vs[0].Block.ExpiresAt != nil {
		t.Errorf("newest imported block = %+v, %v; want the trimmed comment, the moderator, no expiry", vs, err)
	}
	// Each block added is audited once.
	entries, err := e.svc.AuditLog(ctx, appmoderation.AuditQuery{ActorUserID: testkit.NamedID("mod-1")}, 2*bulk)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != res.Added {
		t.Errorf("audit log of the import has %d entries, want %d", len(entries), res.Added)
	}
	for _, en := range entries {
		if en.Action != moderation.AuditSignupBlockAdded || en.TargetKind != moderation.AuditTargetSignupBlock {
			t.Errorf("audit entry of the import = %+v, want signup_block_added", en)
			break
		}
	}

	// Importing the list again adds nothing: every valid value is blocked now.
	again, err := e.svc.ImportSignupBlocks(ctx, in)
	if err != nil || again.Added != 0 || len(again.Skipped) != len(wantSkipped)+3+bulk {
		t.Errorf("ImportSignupBlocks again = %d added, %d skipped, %v; want 0, %d", again.Added, len(again.Skipped), err, len(wantSkipped)+3+bulk)
	} else if s := again.Skipped[0]; s != (appmoderation.SignupBlockImportSkipped{Line: 2, Value: "new.example", Reason: moderation.SignupBlockSkipAlreadyBlocked}) {
		t.Errorf("first skipped line of the second import = %+v, want new.example already blocked", s)
	}
	// A weaker severity is covered by the stronger block in effect.
	weaker, err := e.svc.ImportSignupBlocks(ctx, appmoderation.SignupBlockImportInput{Kind: domain, Text: "new.example", Severity: moderation.SignupRequireApproval})
	if err != nil || weaker.Added != 0 || len(weaker.Skipped) != 1 || weaker.Skipped[0].Reason != moderation.SignupBlockSkipAlreadyBlocked {
		t.Errorf("ImportSignupBlocks(require_approval) = %+v, %v; want new.example already blocked", weaker, err)
	}

	// The settings of the import are checked before any value, and the list has a size limit.
	for _, bad := range []appmoderation.SignupBlockImportInput{
		{Kind: "email", Text: "a.example", Severity: moderation.SignupReject},
		{Kind: domain, Text: "a.example", Severity: "maybe"},
		{Kind: domain, Text: "a.example", Severity: moderation.SignupReject, Duration: -time.Hour},
		{Kind: domain, Text: "a.example", Severity: moderation.SignupReject, Comment: strings.Repeat("x", moderation.MaxSignupBlockComment+1)},
	} {
		if _, err := e.svc.ImportSignupBlocks(ctx, bad); !errors.Is(err, appmoderation.ErrInvalidSignupBlock) {
			t.Errorf("ImportSignupBlocks(%s, %q, %v) = %v, want ErrInvalidSignupBlock", bad.Kind, bad.Severity, bad.Duration, err)
		}
	}
	tooMany := strings.Repeat("a.example\n", moderation.MaxSignupBlockImport+1)
	if _, err := e.svc.ImportSignupBlocks(ctx, appmoderation.SignupBlockImportInput{Kind: domain, Text: tooMany, Severity: moderation.SignupReject}); !errors.Is(err, appmoderation.ErrTooManySignupBlocks) {
		t.Errorf("ImportSignupBlocks(too many) = %v, want ErrTooManySignupBlocks", err)
	}
	// Other kinds take their own forms of values.
	ips, err := e.svc.ImportSignupBlocks(ctx, appmoderation.SignupBlockImportInput{
		Kind: moderation.SignupBlockIPRange, Text: "198.51.100.7\n198.51.100.0/24\nspam.example", Severity: moderation.SignupReject,
	})
	if err != nil || ips.Added != 2 || len(ips.Skipped) != 1 || ips.Skipped[0].Line != 3 || ips.Skipped[0].Reason != moderation.SignupBlockSkipInvalid {
		t.Errorf("ImportSignupBlocks(ip_range) = %+v, %v; want 2 added and line 3 invalid", ips, err)
	}
}
