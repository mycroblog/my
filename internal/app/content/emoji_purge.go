// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

const (
	// JobPurgeRemoteEmojis deletes the remote custom emoji that nothing refers to any more
	// (ADR-0051 §1, FR-803). Its payload is the cursor of the run (none for the first job of a
	// run).
	JobPurgeRemoteEmojis = "content.purge_remote_emojis"
	// EmojiPurgeInterval is the period of the purge's schedule.
	EmojiPurgeInterval = 24 * time.Hour
	// EmojiPurgeBatch is the number of emoji that one listing of the purge looks at;
	// emojiPurgeMaxBatches bounds the listings of one job, and a job that used them up enqueues
	// the next with its cursor.
	EmojiPurgeBatch      = 100
	emojiPurgeMaxBatches = 10
)

// EmojiPurgeRepository is the part of the content module's persistence that the purge of remote
// custom emoji uses. The database adapters' custom emoji repositories implement it and pass
// contenttest.CustomEmojiRepositoryContract; the purge on top of it is a case of
// federationtest.RetentionContract (emoji_purge_flows.go).
type EmojiPurgeRepository interface {
	// ListUnreferencedRemoteEmojis returns the IDs of up to limit remote custom emoji, in ID
	// order after the cursor after, that no status revision and no reaction refers to and that
	// are not disabled.
	ListUnreferencedRemoteEmojis(ctx context.Context, after string, limit int) ([]string, error)
	// CustomEmojiImageMediaIDs returns the media files of every image of an emoji, current or
	// not.
	CustomEmojiImageMediaIDs(ctx context.Context, emojiID string) ([]string, error)
	// PurgeRemoteEmoji deletes inside tx a remote emoji that still no status revision and no
	// reaction refers to and that is still not disabled, with its images, settings, and aliases,
	// and then of mediaIDs the remote media files that no other emoji image and no attachment
	// uses. It reports whether it deleted the emoji: false when it is gone, referred to, or
	// disabled meanwhile.
	PurgeRemoteEmoji(ctx context.Context, tx uow.Tx, emojiID string, mediaIDs []string) (bool, error)
}

// EmojiReferrer is a module that refers to custom emoji from outside the content module's status
// revisions and reactions, such as the account module's profiles. The purge keeps every emoji
// that a referrer reports as referenced.
type EmojiReferrer interface {
	IsEmojiReferenced(ctx context.Context, emojiID string) (bool, error)
}

// EmojiPurgeDeps are the ports of the purge of remote custom emoji.
type EmojiPurgeDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository EmojiPurgeRepository
	// Referrers keep the emoji that they use.
	Referrers []EmojiReferrer
	// MediaReferrers keep the media files that they use, which an emoji image may share with a
	// profile's avatar or header (one remote media file per owner and URL).
	MediaReferrers []MediaReferrer
	// Jobs enqueues the next job of a run, outside any transaction.
	Jobs queue.JobQueue
	// Batch is the number of emoji per listing; zero means EmojiPurgeBatch.
	Batch int
}

// EmojiPurge deletes the remote custom emoji that no status revision, profile revision, or
// reaction refers to any more (ADR-0051 §1), so that they do not outlive the remote statuses and
// profiles that the retention purges remove (ADR-0006, FR-803). A disabled emoji stays, referenced
// or not, with its images: its row and settings are what keep its (shortcode, domain) disabled
// when it arrives again, which recording it anew would turn back to enabled. Each emoji goes in a short
// transaction of its own with its images, settings, and aliases, and with the remote media files
// of its images that nothing else uses; the copies of the remote media cache expire on their own.
//
// The references of the content module are checked again in the deletion's statement. A
// document received while the purge runs may still refer to an emoji that the purge deletes; the
// readers skip such a reference, as they skip one to a deleted local emoji, and the emoji is
// recorded again when it arrives again.
type EmojiPurge struct{ d EmojiPurgeDeps }

// NewEmojiPurge returns the purge of remote custom emoji on d.
func NewEmojiPurge(d EmojiPurgeDeps) *EmojiPurge {
	if d.Batch <= 0 {
		d.Batch = EmojiPurgeBatch
	}
	return &EmojiPurge{d: d}
}

// RegisterJobs registers the JobPurgeRemoteEmojis handler and its daily schedule.
func (p *EmojiPurge) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobPurgeRemoteEmojis, p.handle)
	sched.Add(queue.Schedule{Name: JobPurgeRemoteEmojis, Every: EmojiPurgeInterval, Job: queue.NewJob{Type: JobPurgeRemoteEmojis}})
}

func (p *EmojiPurge) handle(ctx context.Context, j queue.Job) queue.Result {
	var pl retentionPayload
	if len(j.Payload) > 0 && string(j.Payload) != "null" {
		if err := json.Unmarshal(j.Payload, &pl); err != nil {
			return queue.Fail(fmt.Errorf("content: emoji purge payload %s: %w", j.Payload, err))
		}
	}
	next, _, err := p.Run(ctx, pl.After)
	if err != nil {
		return queue.Retry(0, err)
	}
	if next == "" || p.d.Jobs == nil {
		return queue.Done()
	}
	b, err := json.Marshal(retentionPayload{After: next})
	if err != nil {
		return queue.Fail(err)
	}
	if err := p.d.Jobs.Enqueue(ctx, queue.NewJob{Type: JobPurgeRemoteEmojis, Payload: b, IdempotencyKey: "purge_remote_emojis:" + next}); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// Run purges the unreferenced remote custom emoji after the cursor after ("" for the first), in
// a bounded number of listings. It returns how many it purged and the cursor to continue from,
// "" when it reached the end. It is safe to repeat.
func (p *EmojiPurge) Run(ctx context.Context, after string) (next string, purged int, err error) {
	for range emojiPurgeMaxBatches {
		ids, err := p.d.Repository.ListUnreferencedRemoteEmojis(ctx, after, p.d.Batch)
		if err != nil {
			return after, purged, err
		}
		for _, id := range ids {
			ok, err := p.purge(ctx, id)
			if err != nil {
				return after, purged, err
			}
			if ok {
				purged++
			}
			after = id
		}
		if len(ids) < p.d.Batch {
			return "", purged, nil
		}
	}
	return after, purged, nil
}

// purge deletes one emoji unless a referrer refers to it, and reports whether it did.
func (p *EmojiPurge) purge(ctx context.Context, emojiID string) (bool, error) {
	for _, r := range p.d.Referrers {
		referenced, err := r.IsEmojiReferenced(ctx, emojiID)
		if err != nil || referenced {
			return false, err
		}
	}
	all, err := p.d.Repository.CustomEmojiImageMediaIDs(ctx, emojiID)
	if err != nil {
		return false, err
	}
	media := make([]string, 0, len(all))
	for _, id := range all {
		referenced, err := p.mediaReferenced(ctx, id)
		if err != nil {
			return false, err
		}
		if !referenced {
			media = append(media, id)
		}
	}
	var deleted bool
	err = p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		deleted, err = p.d.Repository.PurgeRemoteEmoji(ctx, tx, emojiID, media)
		return err
	})
	return deleted, err
}

func (p *EmojiPurge) mediaReferenced(ctx context.Context, mediaID string) (bool, error) {
	for _, r := range p.d.MediaReferrers {
		referenced, err := r.IsMediaReferenced(ctx, mediaID)
		if err != nil || referenced {
			return referenced, err
		}
	}
	return false, nil
}
