// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// The forwarding of reports to the server of the reported account (FR-709, federation.md §11,
// ADR-0058): a Flag of the instance actor, delivered to the reported account's own inbox.

// ReportReader reads the reports that Flags forward; *appmoderation.Service implements it.
type ReportReader interface {
	// FindReport fails with an error wrapping appmoderation.ErrNotFound for an unknown report.
	FindReport(ctx context.Context, id string) (appmoderation.Report, error)
}

// InstanceActorReader returns the instance actor; *appaccount.Service implements it.
type InstanceActorReader interface {
	// InstanceActor fails with appaccount.ErrNotFound before the instance actor exists.
	InstanceActor(ctx context.Context) (appaccount.Actor, error)
}

// InstanceActorTxReader returns the instance actor inside a transaction, which sends the Flags;
// *appaccount.Service implements it.
type InstanceActorTxReader interface {
	// InstanceActorTx fails with appaccount.ErrNotFound before the instance actor exists.
	InstanceActorTx(ctx context.Context, tx uow.Tx) (appaccount.Actor, error)
}

// RegisterReportProjectors subscribes the federation module to moderation.report_filed: a local
// report of a remote account whose reporter asked to forward it (Report.Forwarded) is recorded,
// in the transaction that files it, as a Flag of the instance actor about the report, with the
// fan-out that delivers it. Other reports (not forwarded, of a local account, or received in a
// Flag) are not. moderation.report_forwarded, a moderator's forward of a report after it was
// filed, records the same Flag in the transaction of the forward.
//
// The instance actor exists whenever a remote account does: it signs the fetch that stored the
// account (InstanceKey). When it is missing, filing the report fails rather than leaving a
// forwarded report unsent.
func RegisterReportProjectors(r *uow.Projectors, d RecorderDeps, instance InstanceActorTxReader) {
	rec := recorder{d: d}
	r.Register(appmoderation.EventReportFiled, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appmoderation.ReportFiled)
		if !ok || !ev.Report.Forwarded {
			return nil
		}
		ia, err := instance.InstanceActorTx(ctx, tx)
		if err != nil {
			return fmt.Errorf("the instance actor that forwards report %s: %w", ev.Report.ID, err)
		}
		return rec.record(ctx, tx, federation.OutboundActivity{
			Type: federation.ActivityFlag, ActorAccountID: ia.Account.ID,
			ObjectKind: federation.OutboundReport, ObjectID: ev.Report.ID, CreatedAt: ev.Report.FiledAt,
		}, "")
	})
	// A moderator's forward of a report after it was filed sends the same Flag, recorded in the
	// transaction of the forward. A report is forwarded once at most, so it has one Flag.
	r.Register(appmoderation.EventReportForwarded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appmoderation.ReportForwarded)
		if !ok || ev.Report.Forward == nil {
			return nil
		}
		ia, err := instance.InstanceActorTx(ctx, tx)
		if err != nil {
			return fmt.Errorf("the instance actor that forwards report %s: %w", ev.Report.ID, err)
		}
		return rec.record(ctx, tx, federation.OutboundActivity{
			Type: federation.ActivityFlag, ActorAccountID: ia.Account.ID,
			ObjectKind: federation.OutboundReport, ObjectID: ev.Report.ID, CreatedAt: ev.Report.Forward.ForwardedAt,
		}, "")
	})
}

// reportSubject loads the report of a Flag. Only a forwarded report sent by the instance actor
// has one; its only recipient is the reported account.
func (f *FanOut) reportSubject(ctx context.Context, s subject) (subject, error) {
	act := s.act
	if f.d.Reports == nil || act.Type != federation.ActivityFlag {
		return subject{}, fmt.Errorf("%w: %s of a report", federation.ErrInvalidOutbound, act.Type)
	}
	r, err := f.d.Reports.FindReport(ctx, act.ObjectID)
	switch {
	case errors.Is(err, appmoderation.ErrNotFound):
		return subject{}, fmt.Errorf("%w: report %s", errGone, act.ObjectID)
	case err != nil:
		return subject{}, err
	case !r.SentToTarget():
		return subject{}, fmt.Errorf("%w: report %s is not forwarded", federation.ErrInvalidOutbound, r.ID)
	}
	a, err := f.d.Accounts.FindByID(ctx, act.ActorAccountID)
	if err != nil {
		return subject{}, fmt.Errorf("actor %s: %w", act.ActorAccountID, err)
	}
	if !a.IsInstanceActor() {
		return subject{}, fmt.Errorf("%w: Flag of report %s by %s, which is not the instance actor", federation.ErrInvalidOutbound, r.ID, act.ActorAccountID)
	}
	s.actor = a
	s.report = r
	s.mentioned = []string{r.TargetAccountID}
	return s, nil
}

// reportedInbox returns the own inbox of the reported account, as Mastodon forwards reports.
func (f *FanOut) reportedInbox(ctx context.Context, s subject) ([]target, error) {
	inbox, err := f.d.Accounts.RemoteInbox(ctx, s.report.TargetAccountID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return nil, nil // a local account, or a remote one never fetched
	}
	if err != nil {
		return nil, err
	}
	t, ok := targetOf(federation.InboxFor(federation.IRI(inbox.InboxURL), ""))
	if !ok {
		return nil, nil
	}
	return []target{t}, nil
}

// flagDocument builds the Flag of a forwarded report: the instance actor's, about the reported
// actor and the URIs of the reported statuses that still exist. Its content is the reporter's
// comment for a report forwarded when it was filed, and the moderator's text for a later forward,
// which never sends the reporter's comment or a hook's note (ADR-0058 §6). It carries nothing of
// the reporter: not their account, address, or email.
func (f *FanOut) flagDocument(ctx context.Context, act federation.OutboundActivity, s subject) ([]byte, error) {
	reported, err := f.actorIRI(ctx, s.report.TargetAccountID)
	if err != nil {
		return nil, err
	}
	var statuses []federation.IRI
	for _, id := range s.report.StatusIDs {
		st, err := f.d.Statuses.FindStatus(ctx, id)
		if errors.Is(err, appcontent.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if st.URI != "" {
			statuses = append(statuses, federation.IRI(st.URI))
		}
	}
	return federation.NewFlagDocument(act.ActivityURI, federation.IRI(s.actor.Account.URI), reported, statuses, s.report.FlagComment()).Marshal()
}
