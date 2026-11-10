// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/contentgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// ReplaceInteractionPolicy implements content.InteractionPolicyRepository.
func (r *StatusRepository) ReplaceInteractionPolicy(ctx context.Context, tx uow.Tx, statusID string, kinds []content.InteractionKind, rules content.InteractionPolicy) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if len(kinds) > 0 {
		names := make([]string, len(kinds))
		for i, k := range kinds {
			names[i] = string(k)
		}
		if err := q.DeleteStatusInteractionPolicyKinds(ctx, contentgen.DeleteStatusInteractionPolicyKindsParams{StatusID: statusID, Kinds: names}); err != nil {
			return postgres.Wrap(err, "delete status interaction policy")
		}
	}
	if len(rules) == 0 {
		return nil
	}
	arg := contentgen.InsertStatusInteractionPolicyParams{StatusID: statusID}
	for _, rule := range rules {
		arg.Kinds = append(arg.Kinds, string(rule.Kind))
		arg.Approvals = append(arg.Approvals, string(rule.Approval))
		arg.Audiences = append(arg.Audiences, string(rule.Audience))
		arg.ActorUris = append(arg.ActorUris, rule.ActorURI)
	}
	return postgres.Wrap(q.InsertStatusInteractionPolicy(ctx, arg), "insert status interaction policy")
}

// FindInteractionPolicies implements content.InteractionPolicyRepository.
func (r *StatusRepository) FindInteractionPolicies(ctx context.Context, statusIDs []string) (map[string]content.InteractionPolicy, error) {
	out := map[string]content.InteractionPolicy{}
	if len(statusIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.reader().ListStatusInteractionPolicies(ctx, statusIDs)
	if err != nil {
		return nil, postgres.Wrap(err, "list status interaction policies")
	}
	for _, row := range rows {
		out[row.StatusID] = append(out[row.StatusID], content.InteractionRule{
			Kind: content.InteractionKind(row.Kind), Approval: content.InteractionApproval(row.Approval),
			Audience: content.InteractionAudience(row.Audience), ActorURI: row.ActorUri,
		})
	}
	return out, nil
}

// FindInteractionPolicy implements content.InteractionPolicyRepository.
func (r *StatusRepository) FindInteractionPolicy(ctx context.Context, statusID string) (content.InteractionPolicy, error) {
	m, err := r.FindInteractionPolicies(ctx, []string{statusID})
	return m[statusID], err
}

// InsertHeldReply implements content.Repository.
func (r *StatusRepository) InsertHeldReply(ctx context.Context, tx uow.Tx, h content.HeldReply) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertHeldReply(ctx, contentgen.InsertHeldReplyParams{StatusID: h.StatusID, Reason: string(h.Reason), HeldAt: h.HeldAt}), "insert held reply")
}

// IsHeldReply implements content.Repository.
func (r *StatusRepository) IsHeldReply(ctx context.Context, tx uow.Tx, statusID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	held, err := q.HeldReplyExists(ctx, statusID)
	return held, postgres.Wrap(err, "check held reply")
}
