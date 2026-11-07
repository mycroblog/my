// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"errors"
	"time"

	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// The decision hook local.profile (ADR-0147 §2, §6, §7): UpdateProfile asks it about an update of a
// local account's profile before the transaction, with the account and the ID of the new revision
// as the subject (hook.ProfileSubjectKey). The plugin receives the display name, the bio, and the
// fields as they would be stored, and whether the avatar or the header changes; no image. The wait
// is short (the plugin's timeout, 1 second by default) and a failure never stops an update. By
// default only accounts created recently are sent. An update that changes none of what the hook
// sees, such as one that only locks the account, is not sent. A reject refuses that update alone,
// which leaves the profile as it was and cannot be appealed: the user may change it and save again.

// ErrProfileRejected reports an update of a profile that the hook local.profile rejected while it
// enforces.
var ErrProfileRejected = errors.New("account: the profile update was rejected by an automated review")

// ProfileReview is what UpdateProfile needs to ask local.profile.
type ProfileReview struct {
	// Hooks asks the hook; nil asks nothing.
	Hooks apphook.Gate
	// MaxAccountAge returns how old an account may be and still be sent, read at each update; 0
	// sends every account. nil sends every account.
	MaxAccountAge func(ctx context.Context) (time.Duration, error)
	// Reports files the report of a flag inside the update's transaction (ADR-0147 §7), with
	// InstanceActor as its reporter; nil for either files none.
	Reports       HookReports
	InstanceActor func(ctx context.Context) (string, error)
}

// HookReports files the report of a flag decision of a hook on an account inside a transaction:
// the moderation module, which this module may not import.
type HookReports interface {
	// FileHookReport files it and reports whether it did: false when the decision has one already.
	FileHookReport(ctx context.Context, tx uow.Tx, in HookReport) (bool, error)
}

// HookReport is the report of a flag decision on an account, with no posts.
type HookReport struct {
	ReporterAccountID string
	TargetAccountID   string
	DecisionID        string
	Hook              string
	ReasonCode        string
	// Comment is the plugin's note, "" for none.
	Comment string
}

// profileVerdict is what UpdateProfile carries into its transaction: the hook's Result and, for a
// flag that files a report, its reporter.
type profileVerdict struct {
	apphook.Result
	reporterAccountID string
}

// reviewProfile asks local.profile about the update of a's profile from cur to next, which would
// store the revision revisionID. It fails with ErrProfileRejected for a reject in force.
func (s *Service) reviewProfile(ctx context.Context, a Actor, cur account.ProfileRevision, next ProfileInput, revisionID string) (profileVerdict, error) {
	r := s.d.ProfileReview
	key := apphook.ProfileSubjectKey(a.Account.ID, revisionID)
	none := profileVerdict{Result: apphook.Result{Hook: apphook.LocalProfile, SubjectKey: key, Effect: apphook.Allow}}
	if r.Hooks == nil || !r.Hooks.Configured(apphook.LocalProfile) {
		return none, nil
	}
	subject := localProfileSubject(a, next)
	if !reviewedChange(cur, subject) {
		return none, nil
	}
	if r.MaxAccountAge != nil {
		age, err := r.MaxAccountAge(ctx)
		if err != nil {
			return profileVerdict{}, err
		}
		if age > 0 && s.d.Clock.Now().Sub(a.Account.CreatedAt) > age {
			return none, nil
		}
	}
	res, err := r.Hooks.Decide(ctx, apphook.Call{Hook: apphook.LocalProfile, SubjectKey: key, AccountID: a.Account.ID, Subject: subject})
	if err != nil {
		return profileVerdict{}, err
	}
	v := profileVerdict{Result: res}
	switch {
	case res.Effect == apphook.Reject:
		return profileVerdict{}, ErrProfileRejected
	case res.DecisionID != "" && res.Effect == apphook.Flag && r.Reports != nil && r.InstanceActor != nil:
		if v.reporterAccountID, err = r.InstanceActor(ctx); err != nil {
			return profileVerdict{}, err
		}
	}
	return v, nil
}

// localProfileSubject is what local.profile sends about the profile that next would store, with
// whether its images differ from those of a's current revision.
func localProfileSubject(a Actor, next ProfileInput) apphook.LocalProfileSubject {
	out := apphook.LocalProfileSubject{
		AccountURI: a.Account.URI, AccountCreatedAt: a.Account.CreatedAt,
		DisplayName: next.DisplayName, NoteHTML: next.NoteHTML,
	}
	if a.Local != nil {
		out.Username = a.Local.Username
	}
	if next.Source != nil {
		out.Note = next.Source.Note
	}
	for i, f := range next.Fields {
		field := apphook.LocalProfileField{Name: f.Name, ValueHTML: f.ValueHTML}
		if next.Source != nil && i < len(next.Source.FieldValues) {
			field.Value = next.Source.FieldValues[i]
		}
		out.Fields = append(out.Fields, field)
	}
	cur := a.Profile
	if cur == nil {
		cur = &account.ProfileRevision{}
	}
	out.AvatarChanged = next.AvatarMediaID != cur.AvatarMediaID
	out.HeaderChanged = next.HeaderMediaID != cur.HeaderMediaID
	return out
}

// reviewedChange reports whether the update changes what local.profile sends: the display name, the
// bio, the fields, or an image.
func reviewedChange(cur account.ProfileRevision, next apphook.LocalProfileSubject) bool {
	if next.AvatarChanged || next.HeaderChanged || next.DisplayName != cur.DisplayName || next.NoteHTML != cur.NoteHTML {
		return true
	}
	var note string
	var values []string
	if cur.Source != nil {
		note, values = cur.Source.Note, cur.Source.FieldValues
	}
	if next.Note != note || len(next.Fields) != len(cur.Fields) {
		return true
	}
	return fieldsChanged(cur, values, next.Fields)
}

// fieldsChanged reports whether the fields differ from those of cur, whose typed values are values.
func fieldsChanged(cur account.ProfileRevision, values []string, next []apphook.LocalProfileField) bool {
	for i, f := range next {
		c := cur.Fields[i]
		value := ""
		if i < len(values) {
			value = values[i]
		}
		if f.Name != c.Name || f.ValueHTML != c.ValueHTML || f.Value != value {
			return true
		}
	}
	return false
}

// profileReviewed acts inside the update's transaction on a flag in force: it files the report on
// the account when a reporter is set, and records the effect in the audit log.
func (s *Service) profileReviewed(ctx context.Context, tx uow.Tx, v profileVerdict, accountID string) error {
	r := v.Result
	if r.DecisionID == "" || r.Effect != apphook.Flag || v.reporterAccountID == "" {
		return nil
	}
	filed, err := s.d.ProfileReview.Reports.FileHookReport(ctx, tx, HookReport{
		ReporterAccountID: v.reporterAccountID, TargetAccountID: accountID,
		DecisionID: r.DecisionID, Hook: string(r.Hook), ReasonCode: string(r.Reason), Comment: r.Note,
	})
	if err != nil || !filed {
		return err
	}
	return s.d.ProfileReview.Hooks.Applied(ctx, tx, r)
}
