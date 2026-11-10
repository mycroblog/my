// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"slices"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	domcontent "github.com/mycroblog/my/internal/domain/content"
)

// TestProfileListsTheArticles checks the profile's list of articles on the real database
// (ADR-0160 §11): GET /accounts/{id}/statuses?articles=true pages through the account's articles
// alone, newest first, each without its body, and leaves out those the viewer may not see and
// the deleted ones.
func TestProfileListsTheArticles(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	article := func(title string, v domcontent.Visibility) string {
		t.Helper()
		p, err := e.app.Content.CreateArticle(t.Context(), appcontent.CreateArticleInput{
			AuthorAccountID: alice.accountID, Visibility: v,
			ArticleInput: appcontent.ArticleInput{Title: title, Text: "The body of " + title, Language: "en"},
		})
		if err != nil {
			t.Fatalf("CreateArticle(%s): %v", title, err)
		}
		return p.Status.ID
	}
	first := article("First", domcontent.VisibilityPublic)
	e.post(t, alice, "a post, not an article", domcontent.VisibilityPublic, "en")
	followers := article("For followers", domcontent.VisibilityPrivate)
	deleted := article("Deleted", domcontent.VisibilityPublic)
	if _, err := e.app.Content.Delete(t.Context(), alice.accountID, deleted); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	third := article("Third", domcontent.VisibilityUnlisted)
	e.post(t, bob, "bob's post", domcontent.VisibilityPublic, "en")

	path := "/accounts/" + alice.accountID + "/statuses?articles=true"
	if got, want := slices.Concat(e.all(t, &alice, path, 1, nil)...), []string{third, followers, first}; !slices.Equal(got, want) {
		t.Errorf("alice's own articles = %v, want %v", got, want)
	}
	if got, want := slices.Concat(e.all(t, nil, path, 2, nil)...), []string{third, first}; !slices.Equal(got, want) {
		t.Errorf("a visitor's articles = %v, want %v (no followers-only article)", got, want)
	}
	page := e.page(t, &bob, path)
	if len(page.Items) != 2 {
		t.Fatalf("bob's page = %d items, want 2", len(page.Items))
	}
	for _, s := range page.Items {
		if s.Article == nil || s.Article.Title == "" || s.Content != "" {
			t.Errorf("article %s = %+v with content %q, want its title without the body", s.Id, s.Article, s.Content)
		}
	}
}
