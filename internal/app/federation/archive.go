// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/federation"
)

// The documents of a local account's data export (FR-008): its actor and its posts as an
// ActivityStreams outbox, in the same form that other servers receive them, so that the archive
// can be read by the tools that read Mastodon's.

// ArchiveActor writes the actor document of a local account to w.
func (o *Objects) ArchiveActor(ctx context.Context, accountID string, w io.Writer) error {
	a, err := o.d.Accounts.FindByID(ctx, accountID)
	if err != nil {
		return err
	}
	images, err := LocalActorImages(ctx, o.d.Images, a)
	if err != nil {
		return err
	}
	if images.Emojis, err = LocalActorEmojis(ctx, o.d.Reader.EmojiTags, o.d.Host, a); err != nil {
		return err
	}
	if images.AlsoKnownAs, err = LocalActorAliases(ctx, o.d.Aliases, a); err != nil {
		return err
	}
	if images.MovedTo, err = LocalActorMovedTo(ctx, o.d.Moves, a); err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(LocalActorDocument(o.d.Host, o.d.AccountDomain, a, "", images))
}

// outboxArchive is the head of the outbox document of an archive; orderedItems and totalItems
// follow it as the items are written.
type outboxArchive struct {
	Context []any          `json:"@context"`
	ID      federation.IRI `json:"id"`
	Type    string         `json:"type"`
}

// ownBoardPosts pages the posts of a local account on the boards of the events (ADR-0161 §17),
// as *appcontent.Reader does.
type ownBoardPosts interface {
	OwnBoardPosts(ctx context.Context, viewer appcontent.Viewer, before string, limit int) (appcontent.BoardPage, error)
}

// ArchiveOutbox writes to w the outbox of a local account as its owner sees it: an
// OrderedCollection of the Create of every status of the account that is not deleted, of every
// visibility, newest first, each with its Note addressed as it was delivered, and then the
// account's posts on the boards of the events, newest first, which are never delivered (FR-008).
// Boosts are left out.
func (o *Objects) ArchiveOutbox(ctx context.Context, accountID string, w io.Writer) error {
	a, err := o.d.Accounts.FindByID(ctx, accountID)
	if err != nil {
		return err
	}
	actor := federation.IRI(a.Account.URI)
	bw := bufio.NewWriter(w)
	head, err := json.Marshal(outboxArchive{Context: federation.LDContext(), ID: actor + "/" + CollectionOutbox, Type: "OrderedCollection"})
	if err != nil {
		return err
	}
	// The head ends with "}"; the items go into the same object.
	if _, err := bw.Write(head[:len(head)-1]); err != nil {
		return err
	}
	if _, err := bw.WriteString(`,"orderedItems":[`); err != nil {
		return err
	}
	owner := appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: accountID}
	total, before := 0, ""
	write := func(views []appcontent.StatusView) error {
		parts, err := o.pageParts(ctx, views)
		if err != nil {
			return err
		}
		for _, v := range views {
			note := statusNote(v.Status, v.Revision, v.Author, v.Status.Visibility, parts[v.Status.ID], updatedOf(v.Revision), o.d.Host, o.d.AccountDomain)
			item, err := marshalNoEscape(createOf(v.Status, actor, note).Embedded())
			if err != nil {
				return err
			}
			if total > 0 {
				if err := bw.WriteByte(','); err != nil {
					return err
				}
			}
			if _, err := bw.Write(item); err != nil {
				return err
			}
			total++
		}
		return nil
	}
	for {
		page, err := o.d.Timeline.Profile(ctx, owner, accountID, apptimeline.ProfileFilter{ExcludeBoosts: true},
			apptimeline.Query{Before: before, Limit: apptimeline.MaxLimit})
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(page.Entries))
		for _, e := range page.Entries {
			ids = append(ids, e.StatusID)
		}
		views, err := o.d.Reader.Views(ctx, owner, ids)
		if err != nil {
			return err
		}
		if err := write(views); err != nil {
			return err
		}
		if page.Next == "" {
			break
		}
		before = page.Next
	}
	if boards, ok := o.d.Reader.(ownBoardPosts); ok {
		for before = ""; ; {
			page, err := boards.OwnBoardPosts(ctx, owner, before, apptimeline.MaxLimit)
			if err != nil {
				return err
			}
			if err := write(page.Statuses); err != nil {
				return err
			}
			if page.Next == "" {
				break
			}
			before = page.Next
		}
	}
	if _, err := fmt.Fprintf(bw, `],"totalItems":%d}`+"\n", total); err != nil {
		return err
	}
	return bw.Flush()
}

// marshalNoEscape encodes v as JSON without escaping the HTML of the contents.
func marshalNoEscape(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
