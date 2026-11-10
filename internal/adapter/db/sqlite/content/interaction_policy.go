// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"encoding/json"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
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
		list, err := sqlite.JSONList(names)
		if err != nil {
			return err
		}
		if err := q.DeleteStatusInteractionPolicyKinds(ctx, contentgen.DeleteStatusInteractionPolicyKindsParams{StatusID: statusID, Kinds: list}); err != nil {
			return sqlite.Wrap(err, "delete status interaction policy")
		}
	}
	if len(rules) == 0 {
		return nil
	}
	type row struct {
		Kind     string `json:"kind"`
		Approval string `json:"approval"`
		Audience string `json:"audience"`
		ActorURI string `json:"actor_uri"`
	}
	rows := make([]row, len(rules))
	for i, rule := range rules {
		rows[i] = row{Kind: string(rule.Kind), Approval: string(rule.Approval), Audience: string(rule.Audience), ActorURI: rule.ActorURI}
	}
	list, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertStatusInteractionPolicy(ctx, contentgen.InsertStatusInteractionPolicyParams{StatusID: statusID, Rules: string(list)}),
		"insert status interaction policy")
}

// FindInteractionPolicies implements content.InteractionPolicyRepository.
func (r *StatusRepository) FindInteractionPolicies(ctx context.Context, statusIDs []string) (map[string]content.InteractionPolicy, error) {
	out := map[string]content.InteractionPolicy{}
	if len(statusIDs) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListStatusInteractionPolicies(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list status interaction policies")
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
	return sqlite.Wrap(q.InsertHeldReply(ctx, contentgen.InsertHeldReplyParams{StatusID: h.StatusID, Reason: string(h.Reason), HeldAt: sqlite.ToMillis(h.HeldAt)}), "insert held reply")
}

// IsHeldReply implements content.Repository.
func (r *StatusRepository) IsHeldReply(ctx context.Context, tx uow.Tx, statusID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	held, err := q.HeldReplyExists(ctx, statusID)
	return held, sqlite.Wrap(err, "check held reply")
}
