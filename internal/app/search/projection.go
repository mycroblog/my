// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// settingSearchVisibilities is the key of the setting search_visibilities.
const settingSearchVisibilities = "search_visibilities"

// RegisterProjectors subscribes the index to the facts that change what it may hold (ADR-0149 §7):
// each enqueues its job in the transaction of the fact, through the outbox. The composition root
// calls it only with a backend, so that search = none enqueues nothing.
//
// The jobs carry no idempotency key: each comes from one fact in one transaction, and a key would
// drop the job of a newer fact while the job of an older one is running.
func (s *Service) RegisterProjectors(r *uow.Projectors) {
	syncStatus := func(ctx context.Context, tx uow.Tx, st appcontent.Status) error {
		if !mayEverQualify(st.Visibility) {
			return nil
		}
		b, err := json.Marshal(syncStatusPayload{StatusID: st.ID})
		if err != nil {
			return err
		}
		return s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{Type: JobSyncStatus, Payload: b})
	}
	syncAuthor := func(ctx context.Context, tx uow.Tx, accountID string) error {
		job, err := syncAuthorJob(accountID)
		if err != nil {
			return err
		}
		return s.d.Outbox.EnqueueTx(ctx, tx, job)
	}
	syncDomain := func(ctx context.Context, tx uow.Tx, domain string) error {
		b, err := json.Marshal(syncDomainPayload{Domain: domain})
		if err != nil {
			return err
		}
		return s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{Type: JobSyncDomain, Payload: b})
	}

	r.Register(appcontent.EventStatusCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return syncStatus(ctx, tx, e.(appcontent.StatusCreated).Status)
	})
	r.Register(appcontent.EventStatusRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return syncStatus(ctx, tx, e.(appcontent.StatusRevised).Status)
	})
	r.Register(appcontent.EventStatusDeleted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appcontent.StatusDeleted)
		if ev.ByAccountErasure() {
			return nil
		}
		return syncStatus(ctx, tx, ev.Status)
	})

	r.Register(appaccount.EventProfileRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appaccount.ProfileRevised)
		if !ev.IndexableChanged {
			return nil
		}
		return syncAuthor(ctx, tx, ev.Revision.AccountID)
	})
	r.Register(appaccount.EventLifecycleEventRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return syncAuthor(ctx, tx, e.(appaccount.LifecycleEventRecorded).Event.AccountID)
	})

	r.Register(appmoderation.EventAccountRestrictionsChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return syncAuthor(ctx, tx, e.(appmoderation.AccountRestrictionsChanged).AccountID)
	})
	r.Register(appmoderation.EventAccountRestrictionsExpired, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return syncAuthor(ctx, tx, e.(appmoderation.AccountRestrictionsExpired).AccountID)
	})
	r.Register(appmoderation.EventDomainRestrictionsChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return syncDomain(ctx, tx, e.(appmoderation.DomainRestrictionsChanged).Domain)
	})
	r.Register(appmoderation.EventDomainRestrictionsExpired, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return syncDomain(ctx, tx, e.(appmoderation.DomainRestrictionsExpired).Domain)
	})

	r.Register(appinstance.EventSettingRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		rev := e.(appinstance.SettingRevised).Revision
		if string(rev.Key) != settingSearchVisibilities {
			return nil
		}
		return s.enqueueNarrow(ctx, tx, rev.Value)
	})
}

// enqueueNarrow enqueues JobNarrow for each visibility that value leaves out. A value that narrowed
// nothing costs a job that removes nothing: the event holds only the new value, and a job that reads
// the setting needs no older one. Widening enqueues nothing that indexes older posts: a rebuild does
// that (ADR-0149 §5).
func (s *Service) enqueueNarrow(ctx context.Context, tx uow.Tx, value string) error {
	covered := strings.Split(value, ",")
	for _, v := range []Visibility{VisibilityPublic, VisibilityLocal} {
		if slices.Contains(covered, string(v)) {
			continue
		}
		b, err := json.Marshal(narrowPayload{Visibility: v})
		if err != nil {
			return err
		}
		if err := s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{Type: JobNarrow, Payload: b}); err != nil {
			return err
		}
	}
	return nil
}
