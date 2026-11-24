// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
)

// A revision is read with all its parts in one query, each part a JSON array of its rows
// (queries/sqlite/content/revision_parts.sql): one revision by findRevision, the current
// revisions of many statuses by currentRevisions; revisionOf builds the revision from either row.

// FindCurrentRevisions implements content.Repository: the current revisions with their parts in
// one query.
func (r *StatusRepository) FindCurrentRevisions(ctx context.Context, statusIDs []string) (map[string]content.Revision, error) {
	return currentRevisions(ctx, r.db.reader(), statusIDs)
}

// FindCurrentRevision implements content.Repository.
func (r *StatusRepository) FindCurrentRevision(ctx context.Context, statusID string) (content.Revision, error) {
	revs, err := currentRevisions(ctx, r.db.reader(), []string{statusID})
	if err != nil {
		return content.Revision{}, err
	}
	rev, ok := revs[statusID]
	if !ok {
		return content.Revision{}, fmt.Errorf("head of %s: %w", statusID, appcontent.ErrNotFound)
	}
	return rev, nil
}

// findRevision returns a revision with its parts.
func findRevision(ctx context.Context, q *contentgen.Queries, id string) (content.Revision, error) {
	row, err := q.GetStatusRevision(ctx, id)
	if err != nil {
		return content.Revision{}, sqlite.NotFound(err, appcontent.ErrNotFound, "status revision "+id)
	}
	return revisionOf(row)
}

// currentRevisions returns the current revisions of statusIDs read with q, by status ID, with one
// query whatever the number of statuses. A status without a head is left out.
func currentRevisions(ctx context.Context, q *contentgen.Queries, statusIDs []string) (map[string]content.Revision, error) {
	out := make(map[string]content.Revision, len(statusIDs))
	if len(statusIDs) == 0 {
		return out, nil
	}
	ids, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCurrentStatusRevisions(ctx, ids)
	if err != nil {
		return nil, sqlite.Wrap(err, "list current status revisions")
	}
	for _, row := range rows {
		rev, err := revisionOf(contentgen.GetStatusRevisionRow(row))
		if err != nil {
			return nil, err
		}
		out[row.StatusID] = rev
	}
	return out, nil
}

// The rows of the parts of a revision, as the JSON objects of revision_parts.sql name them.
type (
	bodyPart struct {
		Language    string `json:"language"`
		ContentHTML string `json:"content_html"`
	}
	sourcePart struct {
		Text   string `json:"source_text"`
		Format string `json:"source_format"`
	}
	attachmentPart struct {
		Position    int64   `json:"position"`
		MediaID     string  `json:"media_id"`
		Description *string `json:"description"`
	}
	pollPart struct {
		ID         string           `json:"id"`
		Multiple   int64            `json:"multiple"`
		HideTotals int64            `json:"hide_totals"`
		ClosesAt   *int64           `json:"closes_at"`
		Options    []pollOptionPart `json:"options"`
	}
	pollOptionPart struct {
		Position int64  `json:"position"`
		Title    string `json:"title"`
	}
	remotePart struct {
		ActivityURI        *string `json:"activity_uri"`
		HTMLPolicyVersion  int64   `json:"html_policy_version"`
		OmittedAttachments int64   `json:"omitted_attachment_count"`
	}
	articlePart struct {
		Title         string `json:"title"`
		Summary       string `json:"summary"`
		SummaryOrigin string `json:"summary_origin"`
	}
	articleCoverPart struct {
		MediaID     string `json:"media_id"`
		Description string `json:"description"`
	}
	articleImagePart struct {
		Position    int64  `json:"position"`
		MediaID     string `json:"media_id"`
		Description string `json:"description"`
	}
)

// revisionOf builds a revision from its row with its parts; a part without rows stays nil.
func revisionOf(row contentgen.GetStatusRevisionRow) (content.Revision, error) {
	var (
		bodies                 []bodyPart
		sources                []sourcePart
		attachments            []attachmentPart
		mentions, tags, emojis []string
		polls                  []pollPart
		remotes                []remotePart
		articles               []articlePart
		covers                 []articleCoverPart
		images                 []articleImagePart
	)
	for _, part := range []struct {
		name string
		list string
		into any
	}{
		{"bodies", row.Bodies, &bodies}, {"sources", row.Sources, &sources}, {"attachments", row.Attachments, &attachments},
		{"mentions", row.Mentions, &mentions}, {"tags", row.Tags, &tags}, {"emoji", row.Emojis, &emojis},
		{"polls", row.Polls, &polls}, {"remote sources", row.Remotes, &remotes}, {"articles", row.Articles, &articles},
		{"article covers", row.ArticleCovers, &covers}, {"article images", row.ArticleImages, &images},
	} {
		if err := json.Unmarshal([]byte(part.list), part.into); err != nil {
			return content.Revision{}, fmt.Errorf("sqlite: the %s of status revision %s: %w", part.name, row.ID, err)
		}
	}
	rev := revisionFrom(contentgen.StatusRevision{
		ID: row.ID, StatusID: row.StatusID, Seq: row.Seq, SpoilerText: row.SpoilerText, Sensitive: row.Sensitive,
		Language: row.Language, RevisedAt: row.RevisedAt,
	})
	for _, b := range bodies {
		rev.Bodies = append(rev.Bodies, content.Body{Language: b.Language, ContentHTML: b.ContentHTML})
	}
	for _, src := range sources {
		rev.Source = &content.Source{Text: src.Text, Format: content.SourceFormat(src.Format)}
	}
	for _, a := range attachments {
		rev.Attachments = append(rev.Attachments, content.Attachment{
			Position: int(a.Position), MediaID: a.MediaID, Description: deref(a.Description),
		})
	}
	rev.MentionedAccountIDs, _ = nonEmpty(mentions, nil)
	rev.TagIDs, _ = nonEmpty(tags, nil)
	rev.EmojiIDs, _ = nonEmpty(emojis, nil)
	for _, p := range polls {
		poll := &content.Poll{ID: p.ID, Multiple: p.Multiple != 0, HideTotals: p.HideTotals != 0}
		if p.ClosesAt != nil {
			at := sqlite.FromMillis(*p.ClosesAt)
			poll.ClosesAt = &at
		}
		for _, o := range p.Options {
			poll.Options = append(poll.Options, content.PollOption{Position: int(o.Position), Title: o.Title})
		}
		rev.Poll = poll
	}
	for _, rm := range remotes {
		rev.Remote = &content.RemoteRevisionSource{
			ActivityURI: deref(rm.ActivityURI), HTMLPolicyVersion: int(rm.HTMLPolicyVersion), OmittedAttachments: int(rm.OmittedAttachments),
		}
	}
	for _, ar := range articles {
		a := &content.ArticleRevision{Title: ar.Title, Summary: ar.Summary, SummaryOrigin: content.SummaryOrigin(ar.SummaryOrigin)}
		for _, c := range covers {
			a.Cover = &content.ArticleCover{MediaID: c.MediaID, Description: c.Description}
		}
		for _, img := range images {
			a.Images = append(a.Images, content.ArticleImage{Position: int(img.Position), MediaID: img.MediaID, Description: img.Description})
		}
		rev.Article = a
	}
	return rev, nil
}

// deref returns the string s points at, "" for a NULL.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
