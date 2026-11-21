// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"testing"

	"github.com/mycroblog/my/internal/domain/content"
)

// boardPost is a post of the board of an event by the local author, of visibility vis, as seen
// by a viewer who reads the board (reads) or does not: the facts of a board post carry it.
func boardPost(vis content.Visibility, reads bool) content.StatusFacts {
	return content.StatusFacts{
		AuthorAccountID: localAuthorID, AuthorIsLocal: true, Visibility: vis,
		Board: &content.BoardFacts{Reads: reads},
	}
}

// TestCanViewBoardPostsNeverLeaveTheBoard checks ADR-0161 §17: whatever its visibility, and to
// every viewer, its author and a moderator included, a post of a board is shown in no context
// but ContextBoard: no timeline, profile, search, direct message, delivery, or fetch.
func TestCanViewBoardPostsNeverLeaveTheBoard(t *testing.T) {
	for _, ctx := range content.ViewContexts() {
		if ctx == content.ContextBoard {
			continue
		}
		for _, vw := range viewers {
			for _, vis := range content.Visibilities() {
				for _, reads := range []bool{false, true} {
					if content.CanView(ctx, vw.who(localAuthorID), boardPost(vis, reads), content.AuthorRestrictions{}, vw.rel) {
						t.Errorf("%v / %s / %s / reads %v: a board post is shown outside its board", ctx, vw.name, vis, reads)
					}
				}
			}
		}
	}
}

// TestCanViewOnTheBoard checks that ContextBoard shows a board post, whatever its visibility, to a
// local viewer who reads the board, and to nobody else.
func TestCanViewOnTheBoard(t *testing.T) {
	reader := content.Viewer{Kind: content.ViewerLocal, AccountID: viewerID}
	author := content.Viewer{Kind: content.ViewerLocal, AccountID: localAuthorID}
	for _, vis := range content.Visibilities() {
		for _, tc := range []struct {
			name   string
			viewer content.Viewer
			reads  bool
			rel    content.Relation
			want   bool
		}{
			{"a local reader", reader, true, content.Relation{}, true},
			{"a local reader who follows the author", reader, true, following, true},
			{"the author, reading the board", author, true, content.Relation{}, true},
			{"the author, no longer reading the board", author, false, content.Relation{}, false},
			{"a local viewer who does not read the board", reader, false, followAndAt, false},
			{"a moderator who does not read the board", content.Viewer{Kind: content.ViewerLocal, AccountID: viewerID, Moderator: true}, false, content.Relation{}, false},
			{"a reader who blocks the author", reader, true, withBlocking(content.Relation{}), false},
			{"a reader whom the author blocks", reader, true, withBlockedBy(content.Relation{}), false},
			{"an anonymous viewer", content.Viewer{}, true, content.Relation{}, false},
			{"a remote viewer", content.Viewer{Kind: content.ViewerRemote, AccountID: remoteAuthorID}, true, following, false},
		} {
			got := content.CanView(content.ContextBoard, tc.viewer, boardPost(vis, tc.reads), content.AuthorRestrictions{}, tc.rel)
			if got != tc.want {
				t.Errorf("%s / %s: CanView(ContextBoard) = %v, want %v", tc.name, vis, got, tc.want)
			}
		}
	}
}

// TestCanViewBoardShowsNoOrdinaryStatus checks that ContextBoard admits no status that is not a
// board post, whoever looks.
func TestCanViewBoardShowsNoOrdinaryStatus(t *testing.T) {
	for _, vw := range viewers {
		for _, vis := range content.Visibilities() {
			s := content.StatusFacts{AuthorAccountID: localAuthorID, AuthorIsLocal: true, Visibility: vis}
			if content.CanView(content.ContextBoard, vw.who(localAuthorID), s, content.AuthorRestrictions{}, vw.rel) {
				t.Errorf("%s / %s: ContextBoard shows an ordinary status", vw.name, vis)
			}
		}
	}
}

// TestDeletedByEventRecords checks the origin of the deletion of a board post with its event's
// records, as status_deletions stores it.
func TestDeletedByEventRecords(t *testing.T) {
	if content.DeletedByEventRecords != "event_records" {
		t.Errorf("DeletedByEventRecords = %q, want event_records", content.DeletedByEventRecords)
	}
}
