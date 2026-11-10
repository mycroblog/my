// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/content"
)

// TestDraftArticleValidate checks what the draft of an article keeps (ADR-0160 §2): an empty title
// while the author writes, and none of what an article lacks.
func TestDraftArticleValidate(t *testing.T) {
	t.Parallel()
	article := func(change func(c *content.DraftContent)) content.DraftContent {
		c := content.DraftContent{
			Text: "# Body", Visibility: content.VisibilityPublic,
			Article: &content.DraftArticle{Tags: []string{"cats"}, Images: []string{"a", "b"}, Cover: &content.ArticleCover{MediaID: "c"}},
		}
		if change != nil {
			change(&c)
		}
		return c
	}
	if err := article(nil).Validate(); err != nil {
		t.Errorf("Validate of an untitled article = %v, want nil", err)
	}
	for name, change := range map[string]func(c *content.DraftContent){
		"a content warning": func(c *content.DraftContent) { c.SpoilerText = "cw" },
		"media":             func(c *content.DraftContent) { c.Attachments = []content.DraftAttachment{{MediaID: "m"}} },
		"a poll": func(c *content.DraftContent) {
			c.Poll = &content.DraftPoll{Options: []string{"a", "b"}, Duration: time.Hour}
		},
		"a reply":            func(c *content.DraftContent) { c.InReplyToID = "s" },
		"a quote":            func(c *content.DraftContent) { c.QuotedStatusID = "s" },
		"direct":             func(c *content.DraftContent) { c.Visibility = content.VisibilityDirect },
		"a long title":       func(c *content.DraftContent) { c.Article.Title = strings.Repeat("a", content.MaxArticleTitle+1) },
		"a long summary":     func(c *content.DraftContent) { c.Article.Summary = strings.Repeat("a", content.MaxArticleSummary+1) },
		"eleven hashtags":    func(c *content.DraftContent) { c.Article.Tags = make([]string, content.MaxArticleTags+1) },
		"an image twice":     func(c *content.DraftContent) { c.Article.Images = []string{"a", "a"} },
		"a cover without ID": func(c *content.DraftContent) { c.Article.Cover = &content.ArticleCover{} },
	} {
		if err := article(change).Validate(); !errors.Is(err, content.ErrInvalidDraft) {
			t.Errorf("Validate with %s = %v, want ErrInvalidDraft", name, err)
		}
	}
}
