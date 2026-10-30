// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/rivo/uniseg"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// The server-wide announcements (FR-726, data-model §4.9). The holders of manage_announcements
// write an announcement, a plain-text title and a body in Markdown, as a draft, publish it for every local user within an
// optional window, revise it, and withdraw it for good; each change records an event and its
// audit log entry in one transaction (ADR-0037). Every local user sees the published
// announcements of the moment, rendered to sanitized HTML, and marks the ones they have read.
// A publication, a revision of a published announcement, and a withdrawal signal the streams
// once they commit (ADR-0050). A publication, and a revision that moves the start of the window
// of a published announcement that has not shown yet, emit AnnouncementScheduled, from which the
// notification module schedules the announcement notifications at ShowsFrom (ADR-0125 §2).

// AnnouncementRepository is the persistence port of the announcements. It is part of Repository.
type AnnouncementRepository interface {
	InsertAnnouncement(ctx context.Context, tx uow.Tx, id string, createdAt time.Time) error
	InsertAnnouncementRevision(ctx context.Context, tx uow.Tx, r instance.AnnouncementRevision) error
	InsertAnnouncementStateChange(ctx context.Context, tx uow.Tx, c instance.AnnouncementStateChange) error
	// AnnouncementHead returns the announcement_heads row of the announcement.
	AnnouncementHead(ctx context.Context, tx uow.Tx, announcementID string) (instance.AnnouncementHead, error)
	UpsertAnnouncementHead(ctx context.Context, tx uow.Tx, h instance.AnnouncementHead) error
	DeleteAllAnnouncementHeads(ctx context.Context, tx uow.Tx) error
	// ListAnnouncementRevisionIDs returns the announcement and ID of every revision, oldest
	// first, as heads with only those two fields, for rebuilding.
	ListAnnouncementRevisionIDs(ctx context.Context, tx uow.Tx) ([]instance.AnnouncementHead, error)
	// ListAnnouncementStateChanges returns every state change, oldest first, for rebuilding.
	ListAnnouncementStateChanges(ctx context.Context, tx uow.Tx) ([]instance.AnnouncementStateChange, error)
	// Announcement returns the announcement with its current revision and state.
	Announcement(ctx context.Context, tx uow.Tx, id string) (instance.Announcement, error)
	// InsertAnnouncementRead records that the user read the announcement; a second record
	// keeps the first.
	InsertAnnouncementRead(ctx context.Context, tx uow.Tx, userID, announcementID string, at time.Time) error
	// DeleteAnnouncementReadsOfUser deletes every read of the user.
	DeleteAnnouncementReadsOfUser(ctx context.Context, tx uow.Tx, userID string) error

	// FindAnnouncement returns the announcement with its current revision and state.
	FindAnnouncement(ctx context.Context, id string) (instance.Announcement, error)
	// ListAnnouncements returns every announcement, newest first; only the published ones when
	// publishedOnly is set.
	ListAnnouncements(ctx context.Context, publishedOnly bool) ([]instance.Announcement, error)
	// ListReadAnnouncementIDs returns which of announcementIDs the user has read.
	ListReadAnnouncementIDs(ctx context.Context, userID string, announcementIDs []string) ([]string, error)
}

// Event types of the announcements, for uow.Projectors.
const (
	EventAnnouncementRevised      = "instance.announcement_revised"
	EventAnnouncementStateChanged = "instance.announcement_state_changed"
	// EventAnnouncementShown is the event type of AnnouncementShown.
	EventAnnouncementShown = "instance.announcement_shown"
	// EventAnnouncementScheduled is the event type of AnnouncementScheduled.
	EventAnnouncementScheduled = "instance.announcement_scheduled"
)

// AnnouncementRevised is emitted when an announcement revision is inserted.
type AnnouncementRevised struct {
	Revision instance.AnnouncementRevision
}

// EventType implements uow.Event.
func (AnnouncementRevised) EventType() string { return EventAnnouncementRevised }

// AnnouncementStateChanged is emitted when an announcement is published or withdrawn.
type AnnouncementStateChanged struct {
	Change instance.AnnouncementStateChange
}

// EventType implements uow.Event.
func (AnnouncementStateChanged) EventType() string { return EventAnnouncementStateChanged }

// AnnouncementShown is emitted when a change concerns the readers: an announcement was
// published, revised while published, or withdrawn after it was published.
type AnnouncementShown struct {
	AnnouncementID string
}

// EventType implements uow.Event.
func (AnnouncementShown) EventType() string { return EventAnnouncementShown }

// AnnouncementScheduled is emitted when a published announcement gets the time at which it
// starts to show to the users (instance.Announcement.ShowsFrom): at its publication, and when a
// revision moves the start of its window before it has shown. Its users are notified then.
type AnnouncementScheduled struct {
	AnnouncementID string
	ShowsFrom      time.Time
}

// EventType implements uow.Event.
func (AnnouncementScheduled) EventType() string { return EventAnnouncementScheduled }

func registerAnnouncementProjectors(r *uow.Projectors, repo Repository) {
	apply := func(ctx context.Context, tx uow.Tx, id, revisionID string, change *instance.AnnouncementStateChange) error {
		head, err := repo.AnnouncementHead(ctx, tx, id)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		head.AnnouncementID = id
		return repo.UpsertAnnouncementHead(ctx, tx, head.Apply(revisionID, change))
	}
	r.Register(EventAnnouncementRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		rev := e.(AnnouncementRevised).Revision
		return apply(ctx, tx, rev.AnnouncementID, rev.ID, nil)
	})
	r.Register(EventAnnouncementStateChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		c := e.(AnnouncementStateChanged).Change
		return apply(ctx, tx, c.AnnouncementID, "", &c)
	})
}

// RegisterCommitHooks signals to pub, once the command commits, the announcements whose change
// concerns the readers (FR-207, FR-726). The signal carries only the ID; each stream reads
// whether its viewer sees the announcement.
func RegisterCommitHooks(r *uow.Projectors, pub stream.Publisher) {
	r.OnCommit(EventAnnouncementShown, func(_ context.Context, e uow.Event) {
		pub.Publish(stream.Event{Kind: stream.KindAnnouncement, AnnouncementID: e.(AnnouncementShown).AnnouncementID})
	})
}

func rebuildAnnouncementHeads(ctx context.Context, tx uow.Tx, repo Repository) error {
	if err := repo.DeleteAllAnnouncementHeads(ctx, tx); err != nil {
		return err
	}
	revs, err := repo.ListAnnouncementRevisionIDs(ctx, tx)
	if err != nil {
		return err
	}
	changes, err := repo.ListAnnouncementStateChanges(ctx, tx)
	if err != nil {
		return err
	}
	heads := map[string]instance.AnnouncementHead{}
	var order []string
	head := func(id string) instance.AnnouncementHead {
		h, ok := heads[id]
		if !ok {
			order = append(order, id)
			h.AnnouncementID = id
		}
		return h
	}
	for _, r := range revs {
		heads[r.AnnouncementID] = head(r.AnnouncementID).Apply(r.RevisionID, nil)
	}
	for _, c := range changes {
		heads[c.AnnouncementID] = head(c.AnnouncementID).Apply("", &c)
	}
	for _, id := range order {
		if heads[id].RevisionID == "" {
			continue // a state change without a revision: nothing to show
		}
		if err := repo.UpsertAnnouncementHead(ctx, tx, heads[id]); err != nil {
			return err
		}
	}
	return nil
}

// AnnouncementInput describes the title, the body, and the window of an announcement.
type AnnouncementInput struct {
	// Title is the heading in plain text; white space around it is trimmed.
	Title string
	// Body is the announcement in Markdown.
	Body string
	// StartsAt and EndsAt bound when the announcement shows once published; the zero time leaves
	// that side open.
	StartsAt, EndsAt time.Time
}

// normalize returns in with its title trimmed, or an error wrapping instance.ErrInvalidValue
// for a bad title, body, or window.
func (in AnnouncementInput) normalize() (AnnouncementInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	return in, instance.ValidateAnnouncement(in.Title, in.Body, in.StartsAt, in.EndsAt, uniseg.GraphemeClusterCount)
}

// CreateAnnouncementInput describes a new announcement.
type CreateAnnouncementInput struct {
	AnnouncementInput
	// Publish publishes it at once instead of keeping it a draft.
	Publish bool
	// CreatedByUserID is the user writing it, or "" for the CLI.
	CreatedByUserID string
}

// CreateAnnouncement records a new announcement, a draft unless in.Publish is set. It fails
// with an error wrapping instance.ErrInvalidValue for a bad title, body, or window.
func (s *Service) CreateAnnouncement(ctx context.Context, in CreateAnnouncementInput) (instance.Announcement, error) {
	var err error
	if in.AnnouncementInput, err = in.normalize(); err != nil {
		return instance.Announcement{}, err
	}
	now := s.d.Clock.Now()
	id, revID, changeID := s.d.IDs.NewID(), s.d.IDs.NewID(), s.d.IDs.NewID()
	label := instance.AnnouncementLabel(in.Title, in.Body)
	created := s.audit(appmoderation.AuditAnnouncementCreated, appmoderation.AuditTargetAnnouncement, id, label, revID, in.CreatedByUserID, now)
	published := s.audit(appmoderation.AuditAnnouncementPublished, appmoderation.AuditTargetAnnouncement, id, label, changeID, in.CreatedByUserID, now)
	rev := instance.AnnouncementRevision{
		ID: revID, AnnouncementID: id, Title: in.Title, Body: in.Body, StartsAt: in.StartsAt, EndsAt: in.EndsAt,
		RevisedByUserID: in.CreatedByUserID, RevisedAt: now,
	}
	var out instance.Announcement
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.InsertAnnouncement(ctx, tx, id, now); err != nil {
			return err
		}
		if err := s.d.Repository.InsertAnnouncementRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := tx.Emit(ctx, AnnouncementRevised{Revision: rev}); err != nil {
			return err
		}
		if err := tx.Emit(ctx, created); err != nil {
			return err
		}
		if in.Publish {
			if err := s.changeState(ctx, tx, instance.AnnouncementStateChange{
				ID: changeID, AnnouncementID: id, State: instance.AnnouncementPublished, ChangedByUserID: in.CreatedByUserID, ChangedAt: now,
			}, published); err != nil {
				return err
			}
		}
		var err error
		if out, err = s.d.Repository.Announcement(ctx, tx, id); err != nil {
			return err
		}
		return scheduleIfPublished(ctx, tx, out)
	})
	return out, err
}

// scheduleIfPublished emits AnnouncementScheduled for a published announcement.
func scheduleIfPublished(ctx context.Context, tx uow.Tx, a instance.Announcement) error {
	if a.State != instance.AnnouncementPublished {
		return nil
	}
	return tx.Emit(ctx, AnnouncementScheduled{AnnouncementID: a.ID, ShowsFrom: a.ShowsFrom()})
}

// UpdateAnnouncementInput describes a new title, body, and window of an announcement.
type UpdateAnnouncementInput struct {
	AnnouncementInput
	AnnouncementID string
	// RevisedByUserID is the user making the change, or "" for the CLI.
	RevisedByUserID string
}

// UpdateAnnouncement records a revision of the announcement with a new title, body, and
// window; a published announcement stays published. A revision after the announcement has
// started to show notifies nobody again; one that moves the start of the window before then
// schedules the notifications at the new time. It fails with an error wrapping ErrNotFound for
// an unknown announcement, instance.ErrAnnouncementWithdrawn for a withdrawn one, and
// instance.ErrInvalidValue for a bad title, body, or window.
func (s *Service) UpdateAnnouncement(ctx context.Context, in UpdateAnnouncementInput) (instance.Announcement, error) {
	var err error
	if in.AnnouncementInput, err = in.normalize(); err != nil {
		return instance.Announcement{}, err
	}
	now := s.d.Clock.Now()
	revID := s.d.IDs.NewID()
	audit := s.audit(appmoderation.AuditAnnouncementUpdated, appmoderation.AuditTargetAnnouncement, in.AnnouncementID,
		instance.AnnouncementLabel(in.Title, in.Body), revID, in.RevisedByUserID, now)
	rev := instance.AnnouncementRevision{
		ID: revID, AnnouncementID: in.AnnouncementID, Title: in.Title, Body: in.Body, StartsAt: in.StartsAt, EndsAt: in.EndsAt,
		RevisedByUserID: in.RevisedByUserID, RevisedAt: now,
	}
	var out instance.Announcement
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		cur, err := s.d.Repository.Announcement(ctx, tx, in.AnnouncementID)
		if err != nil {
			return err
		}
		if cur.State == instance.AnnouncementWithdrawn {
			return fmt.Errorf("revise announcement %s: %w", in.AnnouncementID, instance.ErrAnnouncementWithdrawn)
		}
		if err := s.d.Repository.InsertAnnouncementRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := tx.Emit(ctx, AnnouncementRevised{Revision: rev}); err != nil {
			return err
		}
		if err := tx.Emit(ctx, audit); err != nil {
			return err
		}
		if cur.State == instance.AnnouncementPublished {
			if err := tx.Emit(ctx, AnnouncementShown{AnnouncementID: in.AnnouncementID}); err != nil {
				return err
			}
		}
		if out, err = s.d.Repository.Announcement(ctx, tx, in.AnnouncementID); err != nil {
			return err
		}
		if cur.State == instance.AnnouncementPublished && now.Before(cur.ShowsFrom()) && !out.ShowsFrom().Equal(cur.ShowsFrom()) {
			return scheduleIfPublished(ctx, tx, out)
		}
		return nil
	})
	return out, err
}

// PublishAnnouncement publishes a draft for every local user, within its window. Publishing a
// published announcement changes nothing. It fails with an error wrapping ErrNotFound for an
// unknown announcement and instance.ErrAnnouncementWithdrawn for a withdrawn one.
func (s *Service) PublishAnnouncement(ctx context.Context, id, byUserID string) (instance.Announcement, error) {
	return s.setAnnouncementState(ctx, id, byUserID, instance.AnnouncementPublished, appmoderation.AuditAnnouncementPublished)
}

// WithdrawAnnouncement withdraws an announcement for good: no user sees it again, and it can no
// longer change. Withdrawing a withdrawn announcement changes nothing. It fails with an error
// wrapping ErrNotFound for an unknown announcement.
func (s *Service) WithdrawAnnouncement(ctx context.Context, id, byUserID string) (instance.Announcement, error) {
	return s.setAnnouncementState(ctx, id, byUserID, instance.AnnouncementWithdrawn, appmoderation.AuditAnnouncementWithdrawn)
}

func (s *Service) setAnnouncementState(ctx context.Context, id, byUserID string, state instance.AnnouncementState, action appmoderation.AuditAction) (instance.Announcement, error) {
	now := s.d.Clock.Now()
	change := instance.AnnouncementStateChange{
		ID: s.d.IDs.NewID(), AnnouncementID: id, State: state, ChangedByUserID: byUserID, ChangedAt: now,
	}
	audit := s.audit(action, appmoderation.AuditTargetAnnouncement, id, "", change.ID, byUserID, now)
	var out instance.Announcement
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		cur, err := s.d.Repository.Announcement(ctx, tx, id)
		if err != nil {
			return err
		}
		switch cur.State {
		case state:
			out = cur
			return nil // nothing changes
		case instance.AnnouncementWithdrawn:
			return fmt.Errorf("change announcement %s: %w", id, instance.ErrAnnouncementWithdrawn)
		}
		if err := s.changeState(ctx, tx, change, withLabel(audit, instance.AnnouncementLabel(cur.Revision.Title, cur.Revision.Body))); err != nil {
			return err
		}
		if out, err = s.d.Repository.Announcement(ctx, tx, id); err != nil {
			return err
		}
		return scheduleIfPublished(ctx, tx, out)
	})
	return out, err
}

// changeState records a publication or a withdrawal with its audit entry, and signals the
// readers.
func (s *Service) changeState(ctx context.Context, tx uow.Tx, c instance.AnnouncementStateChange, audit appmoderation.AuditRecorded) error {
	if err := s.d.Repository.InsertAnnouncementStateChange(ctx, tx, c); err != nil {
		return err
	}
	if err := tx.Emit(ctx, AnnouncementStateChanged{Change: c}); err != nil {
		return err
	}
	if err := tx.Emit(ctx, audit); err != nil {
		return err
	}
	return tx.Emit(ctx, AnnouncementShown{AnnouncementID: c.AnnouncementID})
}

// Announcement returns an announcement in any state, for the administrators. It fails with an
// error wrapping ErrNotFound for an unknown one.
func (s *Service) Announcement(ctx context.Context, id string) (instance.Announcement, error) {
	return s.d.Repository.FindAnnouncement(ctx, id)
}

// Announcements returns every announcement in any state, newest first, for the administrators.
func (s *Service) Announcements(ctx context.Context) ([]instance.Announcement, error) {
	return s.d.Repository.ListAnnouncements(ctx, false)
}

// RenderAnnouncement returns the body of an announcement as an HTML fragment, sanitized with the
// document allowlist (ADR-0019); "" without a renderer.
func (s *Service) RenderAnnouncement(body string) string {
	if s.d.Documents == nil {
		return ""
	}
	return s.d.Documents.RenderDocument(body)
}

// ReaderAnnouncement is an announcement as a local user sees it.
type ReaderAnnouncement struct {
	instance.Announcement
	// HTML is the body rendered to sanitized HTML.
	HTML string
	// Read reports whether the user has marked it read.
	Read bool
	// Ended reports whether its window has ended (only the list with the past announcements
	// has such).
	Ended bool
}

// ShownAnnouncements returns the announcements that the users see now, those that started to
// show last first, with whether the user userID has read each.
func (s *Service) ShownAnnouncements(ctx context.Context, userID string) ([]ReaderAnnouncement, error) {
	all, err := s.d.Repository.ListAnnouncements(ctx, true)
	if err != nil {
		return nil, err
	}
	now := s.d.Clock.Now()
	all = slices.DeleteFunc(all, func(a instance.Announcement) bool { return !a.ShowsAt(now) })
	slices.SortStableFunc(all, func(a, b instance.Announcement) int {
		return cmp.Or(b.ShowsFrom().Compare(a.ShowsFrom()), cmp.Compare(b.ID, a.ID))
	})
	return s.forReader(ctx, userID, all)
}

// ShownAnnouncementIDs returns the IDs of the announcements that the users see now, whose
// notifications stay listed: the notification module hides those of the withdrawn and the
// ended announcements when it reads them (ADR-0125 §2).
func (s *Service) ShownAnnouncementIDs(ctx context.Context) ([]string, error) {
	all, err := s.d.Repository.ListAnnouncements(ctx, true)
	if err != nil {
		return nil, err
	}
	now := s.d.Clock.Now()
	ids := make([]string, 0, len(all))
	for _, a := range all {
		if a.ShowsAt(now) {
			ids = append(ids, a.ID)
		}
	}
	return ids, nil
}

// AnnouncementCursor is the position of the last announcement of a page of PastAnnouncements:
// its ShowsFrom and its ID. The zero value starts at the newest.
type AnnouncementCursor struct {
	ShowsFrom time.Time
	ID        string
}

// PastAnnouncements returns a page of the announcements that have started to show (published,
// with ShowsFrom passed), the ended ones included and the withdrawn ones left out (ADR-0125 §3):
// those after the cursor in the order of ShowsFrom, newest first, then of ID, at most limit,
// with whether the user userID has read each and whether each has ended. more reports whether
// another page follows.
func (s *Service) PastAnnouncements(ctx context.Context, userID string, after AnnouncementCursor, limit int) (page []ReaderAnnouncement, more bool, err error) {
	all, err := s.d.Repository.ListAnnouncements(ctx, true)
	if err != nil {
		return nil, false, err
	}
	now := s.d.Clock.Now()
	all = slices.DeleteFunc(all, func(a instance.Announcement) bool {
		if !a.HasShownBy(now) {
			return true
		}
		if after.ID == "" {
			return false
		}
		// Keep only those after the cursor: older, or as old with a smaller ID.
		c := a.ShowsFrom().Compare(after.ShowsFrom)
		return c > 0 || (c == 0 && a.ID >= after.ID)
	})
	slices.SortStableFunc(all, func(a, b instance.Announcement) int {
		return cmp.Or(b.ShowsFrom().Compare(a.ShowsFrom()), cmp.Compare(b.ID, a.ID))
	})
	if len(all) > limit {
		all, more = all[:limit], true
	}
	page, err = s.forReader(ctx, userID, all)
	return page, more, err
}

// ShownAnnouncement returns the announcement id as the user userID sees it, and false when the
// users do not see it now (a draft, a withdrawn one, one outside its window, or an unknown ID).
func (s *Service) ShownAnnouncement(ctx context.Context, userID, id string) (ReaderAnnouncement, bool, error) {
	a, err := s.d.Repository.FindAnnouncement(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return ReaderAnnouncement{}, false, nil
	}
	if err != nil {
		return ReaderAnnouncement{}, false, err
	}
	if !a.ShowsAt(s.d.Clock.Now()) {
		return ReaderAnnouncement{}, false, nil
	}
	out, err := s.forReader(ctx, userID, []instance.Announcement{a})
	if err != nil {
		return ReaderAnnouncement{}, false, err
	}
	return out[0], true, nil
}

func (s *Service) forReader(ctx context.Context, userID string, list []instance.Announcement) ([]ReaderAnnouncement, error) {
	out := make([]ReaderAnnouncement, len(list))
	if len(list) == 0 {
		return out, nil
	}
	ids := make([]string, len(list))
	for i, a := range list {
		ids[i] = a.ID
	}
	read, err := s.d.Repository.ListReadAnnouncementIDs(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	now := s.d.Clock.Now()
	for i, a := range list {
		out[i] = ReaderAnnouncement{
			Announcement: a, HTML: s.RenderAnnouncement(a.Revision.Body), Read: slices.Contains(read, a.ID), Ended: a.EndedBy(now),
		}
	}
	return out, nil
}

// MarkAnnouncementRead records that the user userID read an announcement that the users see
// now; marking it again changes nothing. It fails with an error wrapping ErrNotFound for an
// announcement that the users do not see.
func (s *Service) MarkAnnouncementRead(ctx context.Context, userID, id string) (ReaderAnnouncement, error) {
	a, ok, err := s.ShownAnnouncement(ctx, userID, id)
	switch {
	case err != nil:
		return ReaderAnnouncement{}, err
	case !ok:
		return ReaderAnnouncement{}, fmt.Errorf("announcement %s: %w", id, ErrNotFound)
	case a.Read:
		return a, nil
	}
	now := s.d.Clock.Now()
	if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.d.Repository.InsertAnnouncementRead(ctx, tx, userID, id, now)
	}); err != nil {
		return ReaderAnnouncement{}, err
	}
	a.Read = true
	return a, nil
}

// EraseAccountData implements account.DataEraser: it deletes the announcement reads of the
// erased account's user (FR-008).
func (s *Service) EraseAccountData(ctx context.Context, t appaccount.ErasureTarget, _ string, _ int) (appaccount.ErasureStep, error) {
	if t.UserID == "" {
		return appaccount.ErasureStep{Done: true}, nil
	}
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.d.Repository.DeleteAnnouncementReadsOfUser(ctx, tx, t.UserID)
	})
	return appaccount.ErasureStep{Done: err == nil}, err
}
