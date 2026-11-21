// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// The board of an event (ADR-0161 §17): its posts are statuses of this module with a
// status_gathering_boards row, which never leave their event.
//
//   - They are never announced as StatusCreated, StatusRevised, StatusDeleted, or
//     StatusesDeleted, which the timelines, the search, the webhooks, the streams, and the
//     federation subscribe to, but as BoardStatusPosted, BoardStatusRevised, and
//     BoardStatusesDeleted, which only this module's projections and the notifications of their
//     mentions follow; so nothing sends them out, lists them, or indexes them.
//   - Every read decides about them through content.CanView, which shows a board post in
//     content.ContextBoard alone, to a local viewer who reads the board. The audience judges
//     a board post in ContextBoard only where a use case admits board posts (audience.onBoard):
//     the board's list, a post read or acted on by its ID (View, a favourite, a reaction, a vote),
//     and the notifications of a viewer. Every other read, the lists of statuses above all, judges
//     it in ContextStatus, which denies it, so a new read leaves board posts out unless it admits
//     them.
//   - A reply on a board replies to a post of the same board, and an ordinary post never replies
//     to a board post nor quotes one; boosts, bookmarks, and pins of a board post are refused as
//     those of a status the account cannot see.
//
// Who reads and who writes is decided by the gathering module (gathering.Board), which this
// module asks through Boards; the gathering module never reads this module.

var (
	// ErrNotBoardReader reports an account that sees an event but does not read its board: an
	// invitee who has not answered, or one who answered not going.
	ErrNotBoardReader = errors.New("content: the account does not read the board of the event")
	// ErrNotBoardWriter reports an account that reads the board of an event but whom the event's
	// board_writers do not let write on it.
	ErrNotBoardWriter = errors.New("content: the account may not write on the board of the event")
	// ErrBoardClosed reports a board that is read-only: its event was cancelled, or ended more
	// than gathering.BoardWritableAfterEnd ago.
	ErrBoardClosed = errors.New("content: the board of the event is read-only")
)

// BoardAccess is what a viewer may do with the board of an event, as the gathering module decides
// it (gathering.Board).
type BoardAccess struct {
	// Sees reports that the viewer sees the event.
	Sees bool
	// Reads reports that the viewer reads the board.
	Reads bool
	// Open reports that the board takes new posts and edits, whoever writes.
	Open bool
	// Writes reports that the viewer may post on the board and edit its posts now.
	Writes bool
}

// Boards tells who reads and who writes on the board of an event (gathering.Board). The
// composition root implements it on the gathering module.
type Boards interface {
	// Board returns what accountID may do with the board of the event gatheringID: the zero
	// BoardAccess for an unknown event.
	Board(ctx context.Context, accountID, gatheringID string) (BoardAccess, error)
	// BoardReaders returns which of accountIDs read the board of the event gatheringID.
	BoardReaders(ctx context.Context, gatheringID string, accountIDs []string) (map[string]bool, error)
	// BoardsToPurge returns the events of gatheringIDs whose boards' posts go with their records
	// now: ended or cancelled longer than event_records_retention_days ago and pointed at by no
	// report, or unknown (ADR-0161 §14, §17).
	BoardsToPurge(ctx context.Context, gatheringIDs []string) ([]string, error)
}

// BoardRepository is the part of Repository that holds the posts of the boards
// (status_gathering_boards).
type BoardRepository interface {
	// InsertStatusBoard records that a status is a post of the board of an event.
	InsertStatusBoard(ctx context.Context, tx uow.Tx, statusID, gatheringID string) error
	// StatusBoardsAmong returns the event of each of ids that is a board post, by status. One
	// query; it reads in tx, or outside any transaction when tx is nil.
	StatusBoardsAmong(ctx context.Context, tx uow.Tx, ids []string) (map[string]string, error)
	// ListLiveBoardStatusIDs returns up to limit posts of the board of an event that are not
	// deleted, newest first, whose ID is below before ("" for the first page).
	ListLiveBoardStatusIDs(ctx context.Context, gatheringID, before string, limit int) ([]string, error)
	// ListLiveBoardStatusIDsOfAuthor returns up to limit board posts of an author that are not
	// deleted, newest first, whose ID is below before ("" for the first page).
	ListLiveBoardStatusIDsOfAuthor(ctx context.Context, authorAccountID, before string, limit int) ([]string, error)
	// ListLiveBoardStatusesOf returns up to limit posts that are not deleted of the boards of
	// gatheringIDs, in ID order, in tx.
	ListLiveBoardStatusesOf(ctx context.Context, tx uow.Tx, gatheringIDs []string, limit int) ([]content.Status, error)
	// ListBoardStatusIDs returns every board post, deleted or not, in ID order, in tx.
	ListBoardStatusIDs(ctx context.Context, tx uow.Tx) ([]string, error)
	// ListBoardGatheringIDs returns up to limit events whose boards have posts that are not
	// deleted, in ID order, after the event after ("" for the first).
	ListBoardGatheringIDs(ctx context.Context, after string, limit int) ([]string, error)
}

// Domain events of the posts of the boards. A command emits exactly one of them in its
// transaction in place of StatusCreated, StatusRevised, or StatusDeleted.
const (
	// EventBoardStatusPosted follows a new post on the board of an event, with its first
	// revision; the gathering module notifies the board's readers of it.
	EventBoardStatusPosted = "content.board_status_posted"
	// EventBoardStatusRevised follows an edit of a board post.
	EventBoardStatusRevised = "content.board_status_revised"
	// EventBoardStatusesDeleted follows the deletion of board posts: by their author, or with the
	// records of their event.
	EventBoardStatusesDeleted = "content.board_statuses_deleted"
)

// BoardStatusPosted is emitted when a post of the board of an event and its first revision are
// inserted.
type BoardStatusPosted struct {
	Status   content.Status
	Revision content.Revision
	// GatheringID is the event whose board the post is on.
	GatheringID string
	// ConversationID is the conversation that the post belongs to (ADR-0154 §2), that of the post
	// it replies to.
	ConversationID string
	// MentionedReaderIDs are the accounts that the revision mentions and that read the board,
	// the author left out: those whom the mention notifies.
	MentionedReaderIDs []string
}

// EventType implements uow.Event.
func (BoardStatusPosted) EventType() string { return EventBoardStatusPosted }

// BoardStatusRevised is emitted when a later revision of a board post is inserted.
type BoardStatusRevised struct {
	Status      content.Status
	Revision    content.Revision
	GatheringID string
	// MentionedReaderIDs are those of BoardStatusPosted for the new revision.
	MentionedReaderIDs []string
}

// EventType implements uow.Event.
func (BoardStatusRevised) EventType() string { return EventBoardStatusRevised }

// BoardStatusesDeleted is emitted when the deletions of board posts are inserted, one or many
// together.
type BoardStatusesDeleted struct {
	Deletions []StatusDeleted
}

// EventType implements uow.Event.
func (BoardStatusesDeleted) EventType() string { return EventBoardStatusesDeleted }

// registerBoardProjectors registers the projections of the board posts, the same as those of the
// other statuses.
func registerBoardProjectors(r *uow.Projectors, repo Repository) {
	r.Register(EventBoardStatusPosted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(BoardStatusPosted)
		if err := repo.LockStatusKeys(ctx, tx, statusLockKeys(ev.Status, false)); err != nil {
			return err
		}
		return projectCreated(ctx, tx, repo, ev.Status, ev.Revision, false, false)
	})
	r.Register(EventBoardStatusRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		rev := e.(BoardStatusRevised).Revision
		if err := repo.LockStatusKeys(ctx, tx, statusLockKeys(content.Status{ID: rev.StatusID}, true)); err != nil {
			return err
		}
		deleted, err := repo.IsDeleted(ctx, tx, rev.StatusID)
		if err != nil || deleted {
			return err
		}
		return projectRevision(ctx, tx, repo, rev)
	})
	r.Register(EventBoardStatusesDeleted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ds := e.(BoardStatusesDeleted).Deletions
		var keys []string
		for _, d := range ds {
			for _, k := range statusLockKeys(d.Status, true) {
				if !slices.Contains(keys, k) {
					keys = append(keys, k)
				}
			}
		}
		slices.Sort(keys)
		if err := repo.LockStatusKeys(ctx, tx, keys); err != nil {
			return err
		}
		return projectDeletedMany(ctx, tx, repo, ds)
	})
}

// boardOf returns the event whose board st is a post of, "" for an ordinary status: one query
// for a local status, none for a remote one, which is never a board post. It reads in tx, or
// outside any transaction when tx is nil.
func boardOf(ctx context.Context, repo BoardRepository, tx uow.Tx, st content.Status) (string, error) {
	if !st.IsLocal {
		return "", nil
	}
	boards, err := repo.StatusBoardsAmong(ctx, tx, []string{st.ID})
	if err != nil {
		return "", err
	}
	return boards[st.ID], nil
}

// offBoardFacts returns the board facts of st for a read that does not admit board posts: nil
// for an ordinary status, and for a board post the facts of a viewer who does not read the
// board, which content.CanView denies in every context.
func offBoardFacts(ctx context.Context, repo BoardRepository, st content.Status) (*content.BoardFacts, error) {
	gid, err := boardOf(ctx, repo, nil, st)
	if err != nil || gid == "" {
		return nil, err
	}
	return &content.BoardFacts{}, nil
}

// OnBoard reports whether st is a post of the board of an event, which never leaves its event:
// for the modules that judge statuses for remote viewers themselves (the ActivityPub objects).
func (s *Service) OnBoard(ctx context.Context, st Status) (bool, error) {
	gid, err := boardOf(ctx, s.d.Repository, nil, st)
	return gid != "", err
}

// mentionedReaders returns the accounts of ids, the author left out, that read the board of
// gatheringID, in the order of ids.
func (s *Service) mentionedReaders(ctx context.Context, gatheringID, authorID string, ids []string) ([]string, error) {
	var others []string
	for _, id := range ids {
		if id != authorID && !slices.Contains(others, id) {
			others = append(others, id)
		}
	}
	if len(others) == 0 || s.d.Boards == nil {
		return nil, nil
	}
	reads, err := s.d.Boards.BoardReaders(ctx, gatheringID, others)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, id := range others {
		if reads[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

// boardWriter fails unless accountID may write on the board of gatheringID now: ErrNotFound for an
// event that the account does not see, ErrNotBoardReader, ErrBoardClosed, and ErrNotBoardWriter.
func (s *Service) boardWriter(ctx context.Context, accountID, gatheringID string) error {
	if s.d.Boards == nil {
		return fmt.Errorf("event %s: %w", gatheringID, ErrNotFound)
	}
	a, err := s.d.Boards.Board(ctx, accountID, gatheringID)
	if err != nil {
		return err
	}
	return boardWriteError(a, gatheringID)
}

// boardWriteError returns why a cannot write on the board of gatheringID, nil when it can.
func boardWriteError(a BoardAccess, gatheringID string) error {
	switch {
	case !a.Sees:
		return fmt.Errorf("event %s: %w", gatheringID, ErrNotFound)
	case !a.Reads:
		return fmt.Errorf("event %s: %w", gatheringID, ErrNotBoardReader)
	case !a.Open:
		return fmt.Errorf("event %s: %w", gatheringID, ErrBoardClosed)
	case !a.Writes:
		return fmt.Errorf("event %s: %w", gatheringID, ErrNotBoardWriter)
	}
	return nil
}

// BoardPostInput describes a new post on the board of an event.
type BoardPostInput struct {
	TextInput
	AuthorAccountID string
	// GatheringID is the event whose board the post is on.
	GatheringID string
	// InReplyToID is the post of the same board replied to, "" for none.
	InReplyToID string
	// Poll is the poll of the post, nil for none (FR-112); a post with a poll attaches no media.
	Poll *content.PollInput
	// ClientID is the OAuth client that posts, "" when none is recorded.
	ClientID string
	// IdempotencyKey is the client API's Idempotency-Key, "" for none, as for Create.
	IdempotencyKey string
}

// PostOnBoard posts a local status on the board of an event (ADR-0161 §17), by an account that
// may write on it now (boardWriter): with what Create checks of a post, a reply to a post of the
// same board alone (ErrReplyTargetNotFound for any other status), and no quote. The post is
// followers-only by its stored visibility, which grants nothing: the board decides who sees it
// (content.ContextBoard). It emits BoardStatusPosted, whose mentions notify only the mentioned
// accounts that read the board. Retrying with an Idempotency-Key that the author already used
// returns the status posted with it, as Create does.
func (s *Service) PostOnBoard(ctx context.Context, in BoardPostInput) (Post, error) {
	if err := content.ValidateIdempotencyKey(in.IdempotencyKey); err != nil {
		return Post{}, err
	}
	author, err := s.localActive(ctx, in.AuthorAccountID)
	if err != nil {
		return Post{}, err
	}
	if in.IdempotencyKey != "" {
		if post, err := s.replay(ctx, in.AuthorAccountID, in.IdempotencyKey); !errors.Is(err, ErrNotFound) {
			return post, err
		}
	}
	if err := s.boardWriter(ctx, in.AuthorAccountID, in.GatheringID); err != nil {
		return Post{}, err
	}
	var parent *content.Status
	if in.InReplyToID != "" {
		restrictions, err := s.postingRestrictions(ctx, in.AuthorAccountID)
		if err != nil {
			return Post{}, err
		}
		if !restrictions.CanInteract() {
			return Post{}, fmt.Errorf("account %s: %w", in.AuthorAccountID, ErrInteractionRestricted)
		}
		p, err := s.boardReplyTarget(ctx, in.AuthorAccountID, in.GatheringID, in.InReplyToID)
		if err != nil {
			return Post{}, err
		}
		parent = &p
	}
	p, err := s.prepareCreate(ctx, author, CreateInput{
		TextInput: in.TextInput, AuthorAccountID: in.AuthorAccountID, Visibility: content.VisibilityPrivate,
		Poll: in.Poll, ClientID: in.ClientID, IdempotencyKey: in.IdempotencyKey,
	})
	if err != nil {
		return Post{}, err
	}
	if parent != nil {
		p.st.InReplyToURI = parent.URI
	}
	readers, err := s.mentionedReaders(ctx, in.GatheringID, in.AuthorAccountID, p.rev.MentionedAccountIDs)
	if err != nil {
		return Post{}, err
	}
	p.board = &preparedBoard{gatheringID: in.GatheringID, mentionedReaders: readers}
	return s.commitCreate(ctx, author, p, in.IdempotencyKey, nil)
}

// preparedBoard is the board that a PreparedCreate posts on.
type preparedBoard struct {
	gatheringID      string
	mentionedReaders []string
}

// boardReplyTarget returns the post of the board of gatheringID that accountID, a writer of the
// board, replies to: a status that is missing, deleted, not on that board, or whose author and
// the account block each other fails alike with ErrReplyTargetNotFound.
func (s *Service) boardReplyTarget(ctx context.Context, accountID, gatheringID, statusID string) (content.Status, error) {
	notFound := fmt.Errorf("status %s: %w", statusID, ErrReplyTargetNotFound)
	st, err := s.d.Repository.FindStatus(ctx, statusID)
	if errors.Is(err, ErrNotFound) {
		return content.Status{}, notFound
	}
	if err != nil {
		return content.Status{}, err
	}
	switch _, err := s.d.Repository.FindCurrentRevision(ctx, statusID); {
	case errors.Is(err, ErrNotFound):
		return content.Status{}, notFound // deleted
	case err != nil:
		return content.Status{}, err
	}
	gid, err := boardOf(ctx, s.d.Repository, nil, st)
	if err != nil {
		return content.Status{}, err
	}
	if gid != gatheringID {
		return content.Status{}, notFound
	}
	if st.AuthorAccountID != accountID {
		r, err := s.d.Relationships.Relationship(ctx, accountID, st.AuthorAccountID)
		if err != nil {
			return content.Status{}, err
		}
		if r.Blocking || r.BlockedBy {
			return content.Status{}, notFound
		}
	}
	return st, nil
}

// editableOnBoard fails unless the board post st, of the board of gatheringID, may be edited
// by its author now (boardWriter).
func (s *Service) editableOnBoard(ctx context.Context, st content.Status, gatheringID string) error {
	return s.boardWriter(ctx, st.AuthorAccountID, gatheringID)
}

// BoardPage is a page of the posts of a board, newest first.
type BoardPage struct {
	Statuses []StatusView
	// Next is the cursor of the next page, "" for the last page.
	Next string
}

// Board returns a page of the posts of the board of the event gatheringID as viewer, a local
// account, sees them, newest first: up to limit posts older than the cursor before ("" for the
// newest). It fails with ErrNotFound for an event that the viewer does not see and with
// ErrNotBoardReader for a viewer who does not read its board. A post that the viewer may not see
// (its author blocks the viewer) is left out, so a page may be shorter than limit and still have
// a next page.
func (r *Reader) Board(ctx context.Context, viewer Viewer, gatheringID, before string, limit int) (BoardPage, error) {
	if viewer.Kind != ViewerLocal || viewer.AccountID == "" || r.d.Boards == nil {
		return BoardPage{}, fmt.Errorf("event %s: %w", gatheringID, ErrNotFound)
	}
	a, err := r.d.Boards.Board(ctx, viewer.AccountID, gatheringID)
	if err != nil {
		return BoardPage{}, err
	}
	switch {
	case !a.Sees:
		return BoardPage{}, fmt.Errorf("event %s: %w", gatheringID, ErrNotFound)
	case !a.Reads:
		return BoardPage{}, fmt.Errorf("event %s: %w", gatheringID, ErrNotBoardReader)
	}
	ids, err := r.d.Statuses.ListLiveBoardStatusIDs(ctx, gatheringID, before, limit+1)
	if err != nil {
		return BoardPage{}, err
	}
	var out BoardPage
	if len(ids) > limit {
		ids = ids[:limit]
		out.Next = ids[limit-1]
	}
	board := *r
	board.aud = board.aud.onBoard()
	board.aud.known = &knownBoard{gatheringID: gatheringID, viewerID: viewer.AccountID, access: a}
	if out.Statuses, err = board.Views(ctx, viewer, ids); err != nil {
		return BoardPage{}, err
	}
	return out, nil
}

// ViewsWithBoards is Views admitting the posts of the boards that viewer reads, for a list that
// points at the statuses one by one for its viewer: the viewer's notifications.
func (r *Reader) ViewsWithBoards(ctx context.Context, viewer Viewer, ids []string) ([]StatusView, error) {
	return r.onBoard().Views(ctx, viewer, ids)
}

// OwnBoardPosts returns a page of the board posts of viewer, a local account, that are not
// deleted, newest first, as Board pages them, whether or not the viewer still reads their boards:
// the board posts of the viewer's data export (FR-008).
func (r *Reader) OwnBoardPosts(ctx context.Context, viewer Viewer, before string, limit int) (BoardPage, error) {
	if viewer.Kind != ViewerLocal || viewer.AccountID == "" {
		return BoardPage{}, nil
	}
	ids, err := r.d.Statuses.ListLiveBoardStatusIDsOfAuthor(ctx, viewer.AccountID, before, limit+1)
	if err != nil {
		return BoardPage{}, err
	}
	var out BoardPage
	if len(ids) > limit {
		ids = ids[:limit]
		out.Next = ids[limit-1]
	}
	own := *r
	own.aud = own.aud.onBoard()
	own.aud.ownPosts = true
	if out.Statuses, err = own.Views(ctx, viewer, ids); err != nil {
		return BoardPage{}, err
	}
	return out, nil
}

// onBoard returns the Reader whose single reads admit the posts of the boards that the viewer
// reads (audience.onBoard).
func (r *Reader) onBoard() *Reader {
	p := *r
	p.aud = p.aud.onBoard()
	return &p
}
