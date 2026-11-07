// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"encoding/hex"

	appaccount "github.com/mycroblog/my/internal/app/account"
	apphook "github.com/mycroblog/my/internal/app/hook"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// ActorReview is the hook inbound.actor where a remote actor is taken in (ADR-0147 §2): the first
// fetch, an Update of the actor, and a fetch again. The plugin is asked outside the transaction,
// only when the profile it reviews differs from the stored one, and the transaction that stores the
// actor acts on the verdict. The zero value asks nothing.
type ActorReview struct {
	Gate apphook.Gate
	// Reports files the report of a flag decision against the actor, with the instance actor as its
	// reporter, in the transaction that stores the actor (ADR-0147 §7); nil files none.
	Reports HookReports
}

// actorVerdict is what the transaction that stores an actor acts on.
type actorVerdict struct {
	hookVerdict
	// file says that the transaction files the report of the decision.
	file bool
}

// ActorSubjectKey is the subject key of inbound.actor: the actor's id, "#", and the digest of the
// profile reviewed in hex, so that each profile is one subject (ADR-0147 §6).
func ActorSubjectKey(actorURI string, digest []byte) string {
	return actorURI + "#" + hex.EncodeToString(digest)
}

// reviewActor asks inbound.actor about the profile of in, and sets on in what storing it takes: the
// ID of the account a new actor gets, which the decision names, and KeepProfile for a drop of a
// known actor. A drop of an actor not stored yet is taken for a flag, since follows and mentions
// need its account. A plugin failure under the retry behaviour comes back as apphook.ErrRetryLater:
// a job returns it and runs again, and a fetch outside a job fails with it.
func (r *RemoteActors) reviewActor(ctx context.Context, in *appaccount.RemoteActorInput) (actorVerdict, error) {
	none := actorVerdict{hookVerdict: allowed}
	gate := r.d.Review.Gate
	if gate == nil {
		return none, nil
	}
	if active, err := gate.Active(ctx, apphook.InboundActor); err != nil || !active {
		return none, err
	}
	review, err := r.d.Accounts.ReviewProfile(ctx, *in)
	if err != nil || !review.Changed {
		return none, err
	}
	accountID, isNew := review.AccountID, review.AccountID == ""
	if isNew {
		accountID = r.d.IDs.NewID()
	}
	p := review.Profile
	subject := apphook.ActorSubject{
		ActorURI: in.URI, Domain: in.Domain, Username: in.PreferredUsername, New: isNew,
		DisplayName: p.DisplayName, NoteHTML: p.NoteHTML, AvatarURL: p.AvatarURL, HeaderURL: p.HeaderURL,
	}
	for _, f := range p.Fields {
		subject.Fields = append(subject.Fields, apphook.ActorField{Name: f.Name, ValueHTML: f.ValueHTML})
	}
	res, err := gate.Decide(ctx, apphook.Call{
		Hook: apphook.InboundActor, SubjectKey: ActorSubjectKey(in.URI, review.Digest), AccountID: accountID, Subject: subject,
	})
	if err != nil {
		return actorVerdict{}, err
	}
	if isNew {
		in.AccountID = accountID
	}
	v := actorVerdict{hookVerdict: hookVerdict{Result: res}}
	switch {
	case res.Effect == apphook.Drop && !isNew:
		in.KeepProfile = true
		return v, nil
	case res.Effect == apphook.Drop:
		v.Effect = apphook.Flag
	case res.Effect != apphook.Flag:
		return v, nil
	}
	if r.d.Review.Reports == nil {
		return v, nil
	}
	instance, err := r.d.Accounts.EnsureInstanceActor(ctx)
	if err != nil {
		return actorVerdict{}, err
	}
	v.reporterAccountID, v.file = instance.ID, true
	return v, nil
}

// applyActorVerdict files inside tx the report of a flag on the stored actor, and records that the
// decision was applied (ADR-0147 §7, §11). A decision that has its report files none again, so a
// job that runs again neither files nor audits twice.
func (r *RemoteActors) applyActorVerdict(ctx context.Context, tx uow.Tx, v actorVerdict, accountID string) error {
	if !v.file {
		return nil
	}
	res := v.Result
	filed, err := r.d.Review.Reports.FileHookReport(ctx, tx, appmoderation.HookReportInput{
		ReporterAccountID: v.reporterAccountID, TargetAccountID: accountID,
		DecisionID: res.DecisionID, Hook: string(res.Hook), ReasonCode: string(res.Reason), Comment: res.Note,
	})
	if err != nil || !filed {
		return err
	}
	return r.d.Review.Gate.Applied(ctx, tx, res)
}

// RefetchDroppedProfile is the override effect of inbound.actor (ADR-0147 §9): cancelling a drop
// enqueues a fetch of the actor again, in the transaction of the cancellation. The fetch finds the
// profile's decision cancelled, which reads as allow, and stores the profile without calling the
// plugin; a profile that changed since is a new subject and is asked about. The stored document is
// not used, since the actor's server serves its present profile.
func RefetchDroppedProfile(outbox queue.Outbox) apphook.OverrideEffect {
	return func(ctx context.Context, tx uow.Tx, r apphook.Record) error {
		if r.Decision.Outcome != apphook.Drop || r.Decision.AccountID == "" {
			return nil
		}
		return outbox.EnqueueTx(ctx, tx, refreshJob(r.Decision.AccountID))
	}
}
