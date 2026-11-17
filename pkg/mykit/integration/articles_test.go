// SPDX-License-Identifier: MIT

package integration_test

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform/platformtest"
	"github.com/mycroblog/my/pkg/testkit"

	"github.com/mycroblog/my/pkg/mykit"
)

// readyImage uploads an image with api and waits until the server has processed it.
func readyImage(t *testing.T, api *mykit.API) mykit.MediaAttachment {
	t.Helper()
	media, err := api.UploadMedia(t.Context(), mykit.MediaUploadInput{
		File: mykit.MediaFile{Name: "square.png", ContentType: "image/png", Body: bytes.NewReader(pngImage(t))},
	})
	if err != nil {
		t.Fatalf("UploadMedia: %v", err)
	}
	processed := testkit.Eventually(t, 30*time.Second, func() bool {
		if media.State != mykit.MediaAttachmentStateProcessing {
			return true
		}
		resp, err := api.GetMediaWithResponse(t.Context(), media.Id)
		if err != nil || resp.JSON200 == nil {
			t.Fatalf("GetMedia = %v", err)
		}
		media = resp.JSON200
		return media.State != mykit.MediaAttachmentStateProcessing
	})
	if !processed {
		t.Fatalf("media %s still processing after 30 seconds", media.Id)
	}
	if media.State != mykit.MediaAttachmentStateReady {
		t.Fatalf("media state = %s, want ready", media.State)
	}
	return *media
}

// TestArticles writes, reads, edits, and deletes an article with mykit (ADR-0160): the preview and
// the published body are the same HTML, raw HTML and images from elsewhere never reach it, lists
// carry no body, the edit history keeps every revision, and the permission is enforced once the
// operator limits who writes articles.
func TestArticles(t *testing.T) {
	t.Parallel()
	srv := platformtest.Start(t, platformtest.Options{})
	alice, bob := srv.User(t, "alice"), srv.User(t, "bob")
	aliceAPI, bobAPI := signIn(t, srv, alice, mykit.Config{}), signIn(t, srv, bob, mykit.Config{})
	ctx := t.Context()
	img := readyImage(t, aliceAPI)

	body := "# Heading\n\nHello @bob, see ![a square](media:" + img.Id + ") and ![a dog](https://elsewhere.example/dog.png).\n\n" +
		"<script>alert(1)</script> <img src=\"https://elsewhere.example/x.png\">\n\nAbout #cats."
	preview, err := aliceAPI.PreviewArticleWithResponse(ctx, mykit.ArticlePreviewRequest{Text: body})
	if err != nil || preview.JSON200 == nil {
		t.Fatalf("previewArticle = %v", err)
	}
	p := preview.JSON200
	if !p.SummaryFromBody || !strings.HasPrefix(p.Summary, "Hello @bob") || p.Images != 1 || p.Bytes != len(p.Content) {
		t.Errorf("preview = %+v, want a summary from the body and one image", p)
	}
	for _, bad := range []string{"<script", "alert", `src="https://elsewhere.example`, "<h1"} {
		if strings.Contains(p.Content, bad) {
			t.Errorf("preview content = %q, want it without %q", p.Content, bad)
		}
	}
	if !strings.Contains(p.Content, "<img ") || !strings.Contains(p.Content, `<a href="https://elsewhere.example/dog.png"`) ||
		!strings.Contains(p.Content, `class="u-url mention"`) || !strings.Contains(p.Content, "#cats") {
		t.Errorf("preview content = %q, want the own image, the other as a link, and the mention", p.Content)
	}

	tags := mykit.ArticleTagsInput{"cats"}
	created, err := aliceAPI.CreateArticleWithResponse(ctx, &mykit.CreateArticleParams{}, mykit.CreateArticleRequest{
		Title: "My first article", Text: body, Tags: &tags, Visibility: mykit.VisibilityPublic,
		Cover: &mykit.ArticleCoverInput{Id: img.Id},
	})
	if err != nil || created.JSON201 == nil {
		t.Fatalf("createArticle = %v", err)
	}
	article := *created.JSON201
	if article.Content != p.Content {
		t.Errorf("published content = %q, want the preview's %q", article.Content, p.Content)
	}
	if a := article.Article; a == nil || a.Title != "My first article" || a.Summary != p.Summary || a.Cover == nil || a.Cover.Id != img.Id {
		t.Errorf("article = %+v, want the title, the summary of the preview, and the cover", a)
	}
	if len(article.Tags) != 1 || article.Tags[0].Name != "cats" || len(article.Mentions) != 0 {
		t.Errorf("tags = %+v, mentions = %+v; want the tag of the editor and no mention", article.Tags, article.Mentions)
	}

	// Lists carry the title, the summary, and the cover, but no body; one article has its body.
	home, err := aliceAPI.GetHomeTimelineWithResponse(ctx, &mykit.GetHomeTimelineParams{})
	if err != nil || home.JSON200 == nil || len(home.JSON200.Items) == 0 {
		t.Fatalf("home timeline = %v", err)
	}
	if listed := home.JSON200.Items[0]; listed.Id != article.Id || listed.Content != "" || listed.Article == nil || listed.Article.Title != "My first article" {
		t.Errorf("home timeline item = %+v, want the article without its body", listed)
	}
	got, err := bobAPI.GetStatusWithResponse(ctx, article.Id)
	if err != nil || got.JSON200 == nil || got.JSON200.Content != article.Content {
		t.Errorf("getStatus = %v, want the article with its body", err)
	}

	// Edits keep the history like posts.
	newTitle := "My first article, edited"
	edited, err := aliceAPI.EditArticleWithResponse(ctx, article.Id, mykit.EditArticleRequest{Title: newTitle, Text: "Just **one** line."})
	if err != nil || edited.JSON200 == nil {
		t.Fatalf("editArticle = %v", err)
	}
	if e := edited.JSON200; e.Content != "<p>Just <strong>one</strong> line.</p>" || e.Article.Title != newTitle || e.Article.Cover != nil || e.EditedAt == nil {
		t.Errorf("edited = %+v, want the new body and title without a cover", e)
	}
	history, err := aliceAPI.GetStatusHistoryWithResponse(ctx, article.Id)
	if err != nil || history.JSON200 == nil || len(history.JSON200.Items) != 2 {
		t.Fatalf("history = %v, want two revisions", err)
	}
	if h := history.JSON200.Items; h[0].Article == nil || h[0].Article.Title != "My first article" || h[1].Article.Title != newTitle {
		t.Errorf("history = %+v, want the titles of both revisions", h)
	}
	source, err := aliceAPI.GetStatusSourceWithResponse(ctx, article.Id)
	if err != nil || source.JSON200 == nil || source.JSON200.Text != "Just **one** line." || source.JSON200.Article == nil ||
		source.JSON200.Article.Title != newTitle || source.JSON200.Article.Summary != "" {
		t.Errorf("source = %v %+v, want the Markdown and the title, with no summary of the author", err, source)
	}
	var problem *mykit.Problem
	_, err = aliceAPI.EditStatusWithResponse(ctx, article.Id, mykit.EditStatusRequest{Text: "a post now"})
	if !errors.As(err, &problem) || problem.Status != http.StatusUnprocessableEntity {
		t.Errorf("editStatus of an article = %v, want a 422", err)
	}
	_, err = aliceAPI.CreateArticleWithResponse(ctx, &mykit.CreateArticleParams{}, mykit.CreateArticleRequest{
		Title: "Secret", Text: "x", Visibility: mykit.VisibilityDirect,
	})
	if !errors.As(err, &problem) || problem.Status != http.StatusUnprocessableEntity {
		t.Errorf("a direct article = %v, want a 422", err)
	}

	// The limits of the body and the hashtags.
	if _, err := srv.App.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingMaxArticleBytes, Value: "1000"}); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("A paragraph of words.\n\n", 60)
	if p, err := aliceAPI.PreviewArticleWithResponse(ctx, mykit.ArticlePreviewRequest{Text: long}); err != nil || p.JSON200.Bytes <= p.JSON200.MaxBytes || p.JSON200.MaxBytes != 1000 {
		t.Errorf("preview of a long body = %v, want it rendered over the limit of 1000 bytes", err)
	}
	_, err = aliceAPI.CreateArticleWithResponse(ctx, &mykit.CreateArticleParams{}, mykit.CreateArticleRequest{
		Title: "Long", Text: long, Visibility: mykit.VisibilityPublic,
	})
	if !errors.As(err, &problem) || problem.Status != http.StatusUnprocessableEntity {
		t.Errorf("a body over max_article_bytes = %v, want a 422", err)
	}
	tooMany := mykit.ArticleTagsInput{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10", "a11"}
	_, err = aliceAPI.CreateArticleWithResponse(ctx, &mykit.CreateArticleParams{}, mykit.CreateArticleRequest{
		Title: "Tags", Text: "x", Tags: &tooMany, Visibility: mykit.VisibilityPublic,
	})
	if !errors.As(err, &problem) || problem.Status != http.StatusUnprocessableEntity {
		t.Errorf("11 hashtags = %v, want a 422", err)
	}

	// Once only the holders of create_articles write articles, bob, who lost the admin role, may
	// not; alice keeps the permission through the admin role.
	if _, err := srv.App.Identity.RevokeRole(ctx, appidentity.RoleChangeInput{UserID: bob.UserID, RoleKey: identity.RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.App.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingArticleAuthoring, Value: string(instance.ArticleAuthoringPermission)}); err != nil {
		t.Fatal(err)
	}
	_, err = bobAPI.CreateArticleWithResponse(ctx, &mykit.CreateArticleParams{}, mykit.CreateArticleRequest{
		Title: "Bob's", Text: "x", Visibility: mykit.VisibilityPublic,
	})
	if !errors.As(err, &problem) || problem.Status != http.StatusForbidden {
		t.Errorf("an article without create_articles = %v, want a 403", err)
	}
	if _, err := aliceAPI.CreateArticleWithResponse(ctx, &mykit.CreateArticleParams{}, mykit.CreateArticleRequest{
		Title: "Alice's second", Text: "x", Visibility: mykit.VisibilityPublic,
	}); err != nil {
		t.Errorf("an article with create_articles = %v", err)
	}

	// Deletion works as for posts.
	if _, err := aliceAPI.DeleteStatusWithResponse(ctx, article.Id); err != nil {
		t.Fatalf("deleteStatus = %v", err)
	}
	_, err = bobAPI.GetStatusWithResponse(ctx, article.Id)
	if !errors.As(err, &problem) || problem.Status != http.StatusNotFound {
		t.Errorf("getStatus of the deleted article = %v, want a 404", err)
	}
}
