// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The reads in a transaction (the ...Tx reads of the repositories and the use cases over them,
// ADR-0060): run in the transaction of an event by a projector, they see what the transaction
// wrote, and they answer as the reads outside transactions do once it has committed.

// inTransactionOf registers on projectors a projector of eventType that runs check in the
// transaction of each such event, and returns how many times it ran.
func inTransactionOf(projectors *uow.Projectors, eventType string, check func(ctx context.Context, tx uow.Tx, ev uow.Event)) *int {
	n := new(int)
	projectors.Register(eventType, func(ctx context.Context, tx uow.Tx, ev uow.Event) error {
		*n++
		check(ctx, tx, ev)
		return nil
	})
	return n
}

func wantRan(t *testing.T, what string, n *int) {
	t.Helper()
	if *n == 0 {
		t.Errorf("%s: the reads in the transaction did not run", what)
	}
}

// accountReadsInTransaction reads an account whose profile the transaction revises.
func accountReadsInTransaction(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.register(t, "fern")
	var inTx appaccount.Actor
	ran := inTransactionOf(e.projectors, appaccount.EventProfileRevised, func(ctx context.Context, tx uow.Tx, ev uow.Event) {
		id := ev.(appaccount.ProfileRevised).Revision.AccountID
		a, err := e.accounts.FindByIDTx(ctx, tx, id)
		if err != nil || a.Profile == nil || a.Profile.DisplayName != "Fern" || a.Local == nil || a.Handle == nil || len(a.Keys) == 0 {
			t.Errorf("FindByIDTx = %+v, %v; want the revised profile", a, err)
		}
		inTx = a
		if la, err := e.accounts.LocalAccountOfUserTx(ctx, tx, reg.User.ID); err != nil || la.AccountID != id {
			t.Errorf("LocalAccountOfUserTx = %+v, %v", la, err)
		}
		if urls, err := e.accounts.RemoteProfileURLsTx(ctx, tx, []string{id}); err != nil || len(urls) != 0 {
			t.Errorf("RemoteProfileURLsTx(local) = %v, %v; want none", urls, err)
		}
	})
	name := "Fern"
	e.clock.Advance(time.Second)
	if _, err := e.accounts.UpdateProfile(ctx, reg.AccountID, appaccount.ProfileUpdate{DisplayName: &name}); err != nil {
		t.Fatalf("UpdateProfile = %v", err)
	}
	wantRan(t, "UpdateProfile", ran)
	stored, err := e.accounts.FindByID(ctx, reg.AccountID)
	if err != nil || stored.Profile == nil || inTx.Profile == nil || stored.Profile.ID != inTx.Profile.ID || stored.Lifecycle != inTx.Lifecycle {
		t.Errorf("FindByID after the commit = %+v, %v; want what FindByIDTx read: %+v", stored, err, inTx)
	}
}

// registrationReadsInTransaction reads a registration that the transaction submits.
func registrationReadsInTransaction(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	r := e.registration(identity.RegistrationApproval, true)
	var inTx appidentity.RegistrationEntry
	ran := inTransactionOf(e.projectors, appidentity.EventRegistrationPending, func(ctx context.Context, tx uow.Tx, ev uow.Event) {
		var err error
		inTx, err = r.RegistrationTx(ctx, tx, ev.(appidentity.RegistrationPending).UserID)
		if err != nil || inTx.Username != "alice" || inTx.AccountID == "" || inTx.Email != "alice@example.com" ||
			inTx.State.State != identity.RegistrationPending {
			t.Errorf("RegistrationTx = %+v, %v; want alice's pending registration", inTx, err)
		}
	})
	in := signUpInput("alice")
	in.Reason = "ducks"
	alice := r.signUp(t, in)
	wantRan(t, "SignUp", ran)
	stored, err := r.Registration(ctx, alice.User.ID)
	if err != nil || stored.Application.ID != inTx.Application.ID || stored.Email != inTx.Email || stored.State != inTx.State {
		t.Errorf("Registration after the commit = %+v, %v; want %+v", stored, err, inTx)
	}
}

// moderationReadsInTransaction reads a report, an action on an account and its restrictions, and
// an action on a domain and its restrictions in the transactions that record them.
func moderationReadsInTransaction(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	reports := inTransactionOf(e.projectors, appmoderation.EventReportFiled, func(ctx context.Context, tx uow.Tx, ev uow.Event) {
		id := ev.(appmoderation.ReportFiled).Report.ID
		if rec, err := e.svc.ReportRecordTx(ctx, tx, id); err != nil || rec.Report.ID != id || rec.Status.State != moderation.ReportOpen {
			t.Errorf("ReportRecordTx = %+v, %v; want the open report", rec, err)
		}
	})
	if _, _, err := e.svc.FileRemoteReport(ctx, appmoderation.RemoteReportInput{
		URI: "https://remote.example/flags/tx", ReporterAccountID: e.reports, TargetAccountID: e.local,
	}); err != nil {
		t.Fatalf("FileRemoteReport = %v", err)
	}
	wantRan(t, "FileRemoteReport", reports)

	actions := inTransactionOf(e.projectors, appmoderation.EventAccountActionIssued, func(ctx context.Context, tx uow.Tx, ev uow.Event) {
		a := ev.(appmoderation.AccountActionIssued).Action
		if v, err := e.svc.AccountActionTx(ctx, tx, a.ID); err != nil || v.Action.ID != a.ID || v.State != moderation.ActionActive {
			t.Errorf("AccountActionTx = %+v, %v; want the active action", v, err)
		}
		if rs, err := e.accounts.OwnRestrictionsTx(ctx, tx, a.TargetAccountID); err != nil || len(rs) == 0 {
			t.Errorf("OwnRestrictionsTx = %v, %v; want the restrictions of the action", rs, err)
		}
	})
	a, err := e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.remote, Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam,
	})
	if err != nil {
		t.Fatalf("IssueAccountAction = %v", err)
	}
	wantRan(t, "IssueAccountAction", actions)
	if v, err := e.svc.AccountAction(ctx, a.ID); err != nil || v.State != moderation.ActionActive {
		t.Errorf("AccountAction after the commit = %+v, %v", v, err)
	}

	domains := inTransactionOf(e.projectors, appmoderation.EventDomainActionIssued, func(ctx context.Context, tx uow.Tx, ev uow.Event) {
		d := ev.(appmoderation.DomainActionIssued).Action
		if v, err := e.svc.DomainActionTx(ctx, tx, d.ID); err != nil || v.Action.ID != d.ID || v.State != moderation.ActionActive {
			t.Errorf("DomainActionTx = %+v, %v; want the active action", v, err)
		}
		if rejected, err := e.policy.RejectsMediaTx(ctx, tx, "cdn.media.example"); err != nil || !rejected {
			t.Errorf("RejectsMediaTx = %v, %v; want true", rejected, err)
		}
		if rejected, err := e.policy.RejectsMediaTx(ctx, tx, "other.example"); err != nil || rejected {
			t.Errorf("RejectsMediaTx(another domain) = %v, %v; want false", rejected, err)
		}
	})
	e.domainAction(t, "media.example", moderation.DomainRejectMedia, 0)
	wantRan(t, "IssueDomainAction", domains)
}

// appealReadsInTransaction reads an appeal with its action in the transaction that files it.
func appealReadsInTransaction(t *testing.T, e *appealEnv) {
	ctx := t.Context()
	suspend := e.issue(t, e.local, moderation.ActionSuspend)
	ran := inTransactionOf(e.projectors, appmoderation.EventAppealFiled, func(ctx context.Context, tx uow.Tx, ev uow.Event) {
		id := ev.(appmoderation.AppealFiled).Appeal.ID
		v, err := e.svc.AppealTx(ctx, tx, id)
		if err != nil || v.Appeal.ID != id || v.Action.Action.ID != suspend.ID || v.State() != moderation.AppealPending {
			t.Errorf("AppealTx = %+v, %v; want the pending appeal of the suspension", v, err)
		}
	})
	got := e.appeal(t, e.local, suspend)
	wantRan(t, "FileAppeal", ran)
	if v, err := e.svc.Appeal(ctx, got.Appeal.ID); err != nil || v.Action.Action.ID != suspend.ID {
		t.Errorf("Appeal after the commit = %+v, %v", v, err)
	}
}

// emojiReadsInTransaction reads a custom emoji, its view, and its image in the transaction that
// registers it.
func emojiReadsInTransaction(t *testing.T, e *emojiEnv) {
	ctx := t.Context()
	var inTx appcontent.AdminCustomEmoji
	ran := inTransactionOf(e.projectors, appcontent.EventCustomEmojiAdministered, func(ctx context.Context, tx uow.Tx, ev uow.Event) {
		em := ev.(appcontent.CustomEmojiAdministered).Emoji
		var err error
		inTx, err = e.service.GetTx(ctx, tx, em.ID)
		if err != nil || inTx.Emoji.Shortcode != "blobcat" || inTx.View == nil || !inTx.Settings.Enabled {
			t.Errorf("GetTx = %+v, %v; want the new emoji with its image", inTx, err)
		}
		if views, err := e.media.CustomEmojisTx(ctx, tx, []string{em.ID}); err != nil || views[em.ID].Shortcode != "blobcat" {
			t.Errorf("CustomEmojisTx = %+v, %v", views, err)
		}
		mediaID := inTx.Image.MediaID
		if imgs, err := e.media.ImagesTx(ctx, tx, []appcontent.ImageRef{{AccountID: emojiAdmin, MediaID: mediaID}}); err != nil || len(imgs) != 1 {
			t.Errorf("ImagesTx = %+v, %v; want the image", imgs, err)
		}
		if remote, err := e.h.Media.ListRemoteMediaTx(ctx, tx, []string{mediaID}); err != nil || len(remote) != 0 {
			t.Errorf("ListRemoteMediaTx(an upload) = %+v, %v; want none", remote, err)
		}
	})
	e.create(t, "blobcat")
	wantRan(t, "Create", ran)
	stored, err := e.service.Get(ctx, inTx.Emoji.ID)
	if err != nil || stored.Image.ID != inTx.Image.ID || stored.View == nil || inTx.View == nil || *stored.View != *inTx.View {
		t.Errorf("Get after the commit = %+v, %v; want %+v", stored, err, inTx)
	}
}
