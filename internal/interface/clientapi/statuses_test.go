// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

const sampleStatusID = "01890a5d-ac96-774b-bcce-b302099a9001"

// fakeStatuses implements the Statuses, StatusReader, and Interactions ports. A nil function
// answers ErrNotFound, so that a Server without statuses serves only 404s.
type fakeStatuses struct {
	create   func(appcontent.CreateInput) (appcontent.Post, error)
	replay   func(accountID, key string) (appcontent.Post, error)
	edit     func(appcontent.EditInput) (appcontent.Post, error)
	del      func(accountID, id string) error
	view     func(appcontent.Viewer, string) (appcontent.StatusView, error)
	interact func(op, accountID, id string) error
	// reactions answers Reactions; nil answers like View, with no reactions.
	reactions func(v appcontent.Viewer, id, emoji, before string, limit int) (appcontent.ReactedPage, error)
}

var errNotFound = fmt.Errorf("fake: %w", appcontent.ErrNotFound)

func (f *fakeStatuses) Create(_ context.Context, in appcontent.CreateInput) (appcontent.Post, error) {
	if f.create == nil {
		return appcontent.Post{}, errNotFound
	}
	return f.create(in)
}

func (f *fakeStatuses) Replay(_ context.Context, accountID, key string) (appcontent.Post, error) {
	if f.replay == nil {
		return appcontent.Post{}, errNotFound
	}
	return f.replay(accountID, key)
}

func (f *fakeStatuses) Edit(_ context.Context, in appcontent.EditInput) (appcontent.Post, error) {
	if f.edit == nil {
		return appcontent.Post{}, errNotFound
	}
	return f.edit(in)
}

func (f *fakeStatuses) SetQuotePolicy(_ context.Context, accountID, id string, p content.QuotePolicy) (content.StatusQuotePolicy, error) {
	return content.StatusQuotePolicy{StatusID: id, Policy: p}, f.do("quote_policy:"+string(p), accountID, id)
}

func (f *fakeStatuses) Quotes(_ context.Context, accountID, id, _ string, _ int) (appcontent.QuotesPage, error) {
	view, err := f.View(context.Background(), appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: accountID}, sampleStatusID)
	if err != nil {
		return appcontent.QuotesPage{}, err
	}
	return appcontent.QuotesPage{Statuses: []appcontent.StatusView{view}}, f.do("quotes", accountID, id)
}

func (f *fakeStatuses) RevokeQuote(_ context.Context, accountID, quoted, quoting string) error {
	return f.do("revoke_quote:"+quoting, accountID, quoted)
}

func (f *fakeStatuses) Delete(_ context.Context, accountID, id string) (content.Deletion, error) {
	if f.del == nil {
		return content.Deletion{}, errNotFound
	}
	return content.Deletion{}, f.del(accountID, id)
}

func (f *fakeStatuses) View(_ context.Context, v appcontent.Viewer, id string) (appcontent.StatusView, error) {
	if f.view == nil {
		return appcontent.StatusView{}, errNotFound
	}
	return f.view(v, id)
}

func (f *fakeStatuses) Thread(_ context.Context, v appcontent.Viewer, id string) (appcontent.Thread, error) {
	view, err := f.View(context.Background(), v, id)
	if err != nil {
		return appcontent.Thread{}, err
	}
	return appcontent.Thread{Ancestors: []appcontent.StatusView{view}}, nil
}

func (f *fakeStatuses) History(_ context.Context, v appcontent.Viewer, id string) ([]appcontent.RevisionView, error) {
	view, err := f.View(context.Background(), v, id)
	if err != nil {
		return nil, err
	}
	return []appcontent.RevisionView{{Revision: view.Revision, Attachments: view.Attachments}}, nil
}

func (f *fakeStatuses) Source(_ context.Context, accountID, id string) (content.Revision, error) {
	view, err := f.View(context.Background(), appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: accountID}, id)
	if err != nil {
		return content.Revision{}, err
	}
	return view.Revision, nil
}

func (f *fakeStatuses) Bookmarks(_ context.Context, accountID, _ string, limit int) (appcontent.BookmarkPage, error) {
	view, err := f.View(context.Background(), appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: accountID}, sampleStatusID)
	if errors.Is(err, appcontent.ErrNotFound) {
		return appcontent.BookmarkPage{}, nil
	}
	if err != nil {
		return appcontent.BookmarkPage{}, err
	}
	return appcontent.BookmarkPage{Statuses: []appcontent.StatusView{view}, Next: fmt.Sprintf("next-%d", limit)}, nil
}

func (f *fakeStatuses) Reactions(_ context.Context, v appcontent.Viewer, id, emoji, before string, limit int) (appcontent.ReactedPage, error) {
	if f.reactions != nil {
		return f.reactions(v, id, emoji, before, limit)
	}
	_, err := f.View(context.Background(), v, id)
	return appcontent.ReactedPage{}, err
}

func (f *fakeStatuses) ReactionEmoji(_ context.Context, e appcontent.ReactionEmoji) (appcontent.EmojiView, error) {
	return appcontent.EmojiView{Emoji: e, Custom: &sampleCustomEmoji}, nil
}

// sampleCustomEmoji is a custom emoji of another server.
var sampleCustomEmoji = appcontent.ReactionCustomEmoji{
	ID: "01890a5d-ac96-774b-bcce-b302099a9200", Shortcode: "blobcat", Domain: "misskey.example",
	Image: &appcontent.CustomEmojiView{
		ID: "01890a5d-ac96-774b-bcce-b302099a9200", Shortcode: "blobcat",
		URL: "https://social.example/media/proxy/blobcat", StaticURL: "https://social.example/media/proxy/blobcat",
	},
}

func (f *fakeStatuses) do(op, accountID, id string) error {
	if f.interact == nil {
		return errNotFound
	}
	return f.interact(op, accountID, id)
}

func (f *fakeStatuses) Boost(_ context.Context, a, id string) (content.Boost, error) {
	return content.Boost{}, f.do("boost", a, id)
}
func (f *fakeStatuses) Unboost(_ context.Context, a, id string) error { return f.do("unboost", a, id) }
func (f *fakeStatuses) Favourite(_ context.Context, a, id string) (content.Favourite, error) {
	return content.Favourite{}, f.do("favourite", a, id)
}
func (f *fakeStatuses) Unfavourite(_ context.Context, a, id string) error {
	return f.do("unfavourite", a, id)
}
func (f *fakeStatuses) React(_ context.Context, a, id, emoji string) (content.Reaction, error) {
	return content.Reaction{}, f.do("react:"+emoji, a, id)
}
func (f *fakeStatuses) Unreact(_ context.Context, a, id string) error {
	return f.do("unreact", a, id)
}
func (f *fakeStatuses) Bookmark(_ context.Context, a, id string) error {
	return f.do("bookmark", a, id)
}
func (f *fakeStatuses) Unbookmark(_ context.Context, a, id string) error {
	return f.do("unbookmark", a, id)
}
func (f *fakeStatuses) Pin(_ context.Context, a, id string) error   { return f.do("pin", a, id) }
func (f *fakeStatuses) Unpin(_ context.Context, a, id string) error { return f.do("unpin", a, id) }
func (f *fakeStatuses) VotePoll(_ context.Context, a, id, pollID string, choices []int) ([]content.PollVote, error) {
	return nil, f.do(fmt.Sprintf("vote:%s:%v", pollID, choices), a, id)
}

// sampleView is a status of alice with every optional part: a reply, an edit, media, a
// mention, a tag, and the viewer's relation.
func sampleView(viewer appcontent.Viewer) appcontent.StatusView {
	published := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	alice := appaccount.Actor{
		Account: account.Account{ID: aliceAccountID, URI: "https://social.example/ap/actors/" + aliceAccountID, CreatedAt: createdAt},
		Local:   &account.LocalAccount{AccountID: aliceAccountID, Username: "alice"},
	}
	v := appcontent.StatusView{
		Status: content.Status{
			ID: sampleStatusID, URI: "https://social.example/ap/statuses/" + sampleStatusID,
			AuthorAccountID: aliceAccountID, IsLocal: true, Visibility: content.VisibilityPublic, PublishedAt: published,
		},
		Revision: content.Revision{
			ID: testkit.NamedID("rev-2"), StatusID: sampleStatusID, Seq: 2, SpoilerText: "cw", Sensitive: true, Language: "en", RevisedAt: published.Add(time.Hour),
			Bodies: []content.Body{{Language: "en", ContentHTML: "<p>hi</p>"}},
			Source: &content.Source{Text: "hi", Format: content.SourceTextPlain},
		},
		Author:    alice,
		InReplyTo: &content.Status{ID: "01890a5d-ac96-774b-bcce-b302099a9000", AuthorAccountID: aliceAccountID},
		Attachments: []appcontent.MediaView{{
			Media: appcontent.Media{
				File: content.MediaFile{ID: "01890a5d-ac96-774b-bcce-b302099a9100"},
				Result: &content.MediaProcessingResult{
					Outcome: content.MediaReady, MediaType: content.MediaTypeImage, Blurhash: "LEHV6nWB2yk8", Width: 640, Height: 480,
				},
				Focus: &content.MediaFocus{X: 0.5, Y: -0.25},
			},
			Description: "a cat",
			Image: appcontent.Image{Still: appcontent.ImageVariants{
				Small:  appcontent.ImageVariant{URL: "https://social.example/media/r/small.webp", ContentType: "image/webp", Size: 512, Width: 415, Height: 311},
				Medium: appcontent.ImageVariant{URL: "https://social.example/media/r/medium.webp", ContentType: "image/webp", Size: 1024, Width: 640, Height: 480},
				Large:  appcontent.ImageVariant{URL: "https://social.example/media/r/large.jpg", ContentType: "image/jpeg", Size: 2048, Width: 640, Height: 480},
			}},
		}},
		Mentions: []appaccount.Actor{alice},
		Tags:     []content.Tag{{Name: "GoLang", NameNormalized: "golang"}},
		Counters: content.Counters{StatusID: sampleStatusID, RepliesCount: 1, BoostsCount: 2, FavouritesCount: 3},
		Reactions: []appcontent.ReactionView{
			{EmojiView: appcontent.EmojiView{Emoji: content.ReactionEmoji{Unicode: "👍"}}, Count: 4},
			{EmojiView: appcontent.EmojiView{Emoji: content.ReactionEmoji{CustomEmojiID: sampleCustomEmoji.ID}, Custom: &sampleCustomEmoji}, Count: 1},
		},
	}
	if viewer.Kind == appcontent.ViewerLocal {
		v.Relation = &appcontent.ViewerRelation{Favourited: true, Author: viewer.AccountID == aliceAccountID}
		v.Reactions[0].Me = true
	}
	return v
}

func newStatusEnv(t *testing.T, f *fakeStatuses) *authEnv {
	t.Helper()
	return newAuthEnv(t, func(o *clientapi.Options) {
		o.Statuses, o.StatusReader, o.Interactions = f, f, f
	})
}

func decodeStatus(t *testing.T, res *http.Response) clientapi.Status {
	t.Helper()
	var s clientapi.Status
	if err := json.NewDecoder(res.Body).Decode(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestGetStatus(t *testing.T) {
	f := &fakeStatuses{view: func(v appcontent.Viewer, id string) (appcontent.StatusView, error) {
		if id != sampleStatusID {
			return appcontent.StatusView{}, errNotFound
		}
		return sampleView(v), nil
	}}
	e := newStatusEnv(t, f)

	res := e.send(t, http.MethodGet, "/statuses/"+sampleStatusID, "", "")
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "public, max-age=60" {
		t.Fatalf("anonymous GET = %d, Cache-Control %q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	s := decodeStatus(t, res)
	if s.Id != sampleStatusID || s.Account.Username != "alice" || s.Content != "<p>hi</p>" || s.SpoilerText != "cw" || !s.Sensitive ||
		s.Visibility != clientapi.VisibilityPublic || s.Language != "en" || s.EditedAt == nil || s.InReplyToId == nil ||
		s.RepliesCount != 1 || s.BoostsCount != 2 || s.FavouritesCount != 3 {
		t.Errorf("status = %+v", s)
	}
	if s.Favourited != nil || s.Boosted != nil || s.Bookmarked != nil || s.Pinned != nil {
		t.Errorf("an anonymous status has the viewer's relation: %+v", s)
	}
	if r := s.Reactions; len(r) != 2 || r[0].Name != "👍" || r[0].Count != 4 || r[0].Me != nil || r[0].Shortcode != nil || r[0].Emoji != nil ||
		r[1].Name != "blobcat@misskey.example" || *r[1].Shortcode != "blobcat" || *r[1].Domain != "misskey.example" ||
		r[1].Emoji == nil || r[1].Emoji.Url != sampleCustomEmoji.Image.URL || r[1].Count != 1 {
		t.Errorf("anonymous reactions = %+v", r)
	}
	if len(s.MediaAttachments) != 1 || s.MediaAttachments[0].Type != clientapi.MediaAttachmentTypeImage || s.MediaAttachments[0].Meta.Width == nil || *s.MediaAttachments[0].Meta.Width != 640 ||
		*s.MediaAttachments[0].Description != "a cat" || s.MediaAttachments[0].Meta.Focus.Y != -0.25 || s.MediaAttachments[0].Variants == nil ||
		s.MediaAttachments[0].Variants.Large.Url != "https://social.example/media/r/large.jpg" ||
		s.MediaAttachments[0].Variants.Small.Url != "https://social.example/media/r/small.webp" ||
		deref(s.MediaAttachments[0].Url) != "https://social.example/media/r/large.jpg" ||
		deref(s.MediaAttachments[0].PreviewUrl) != "https://social.example/media/r/small.webp" {
		t.Errorf("media = %+v", s.MediaAttachments)
	}
	if len(s.Tags) != 1 || s.Tags[0].Url != "https://social.example/tags/golang" || len(s.Mentions) != 1 || s.Mentions[0].Acct != "alice" {
		t.Errorf("tags = %+v, mentions = %+v", s.Tags, s.Mentions)
	}

	res = e.send(t, http.MethodGet, "/statuses/"+sampleStatusID, "read", "")
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("GET with a token = %d, Cache-Control %q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	if s := decodeStatus(t, res); s.Favourited == nil || !*s.Favourited || s.Pinned == nil || *s.Pinned {
		t.Errorf("the author's status = %+v, want favourited and pinned false", s)
	} else if r := s.Reactions; r[0].Me == nil || !*r[0].Me || r[1].Me == nil || *r[1].Me {
		t.Errorf("the viewer's reactions = %+v, want the first only", r)
	}
	res = e.send(t, http.MethodGet, "/statuses/"+sampleStatusID, "bob", "")
	if s := decodeStatus(t, res); s.Favourited == nil || s.Pinned != nil {
		t.Errorf("another viewer's status = %+v, want no pinned", s)
	}

	res = e.send(t, http.MethodGet, "/statuses/01890a5d-ac96-774b-bcce-b302099a9999", "", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusNotFound || p.Status != http.StatusNotFound || res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("missing status = %d %+v", res.StatusCode, p)
	}

	for _, path := range []string{"/context", "/history"} {
		if res := e.send(t, http.MethodGet, "/statuses/"+sampleStatusID+path, "", ""); res.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d", path, res.StatusCode)
		}
	}
	if res := e.send(t, http.MethodGet, "/statuses/"+sampleStatusID+"/source", "read", ""); res.StatusCode != http.StatusOK {
		t.Errorf("GET source = %d", res.StatusCode)
	}
	res = e.send(t, http.MethodGet, "/bookmarks?limit=5", "read", "")
	var page clientapi.StatusPage
	if err := json.NewDecoder(res.Body).Decode(&page); err != nil || res.StatusCode != http.StatusOK || len(page.Items) != 1 || *page.NextCursor != "next-5" {
		t.Errorf("bookmarks = %d %+v, %v", res.StatusCode, page, err)
	}
}

// A status presents its quote with the quoted status one level deep, or only the state when the
// quoted status is not shown (ADR-0069 §5).
func TestGetStatusPresentsTheQuote(t *testing.T) {
	const quotedID, pendingID = "01890a5d-ac96-774b-bcce-b302099a9002", "01890a5d-ac96-774b-bcce-b302099a9003"
	f := &fakeStatuses{view: func(v appcontent.Viewer, id string) (appcontent.StatusView, error) {
		view := sampleView(v)
		switch id {
		case sampleStatusID:
			quoted := sampleView(v)
			quoted.Status.ID = quotedID
			view.Quote = &appcontent.QuoteView{State: appcontent.QuoteViewLegacy, Status: &quoted}
		case pendingID:
			view.Status.ID = pendingID
			view.Quote = &appcontent.QuoteView{State: appcontent.QuoteViewPending}
		default:
			return appcontent.StatusView{}, errNotFound
		}
		return view, nil
	}}
	e := newStatusEnv(t, f)

	s := decodeStatus(t, e.send(t, http.MethodGet, "/statuses/"+sampleStatusID, "", ""))
	if q := s.Quote; q == nil || q.State != clientapi.QuoteStateLegacy || q.QuotedStatus == nil || q.QuotedStatus.Id != quotedID ||
		q.QuotedStatus.Account.Username != "alice" || q.QuotedStatus.Quote != nil {
		t.Errorf("quote = %+v, want legacy with the quoted status and no quote of its own", q)
	}
	s = decodeStatus(t, e.send(t, http.MethodGet, "/statuses/"+pendingID, "", ""))
	if q := s.Quote; q == nil || q.State != clientapi.QuoteStatePending || q.QuotedStatus != nil {
		t.Errorf("quote = %+v, want pending without the quoted status", q)
	}
}

func TestCreateStatus(t *testing.T) {
	var got appcontent.CreateInput
	f := &fakeStatuses{
		create: func(in appcontent.CreateInput) (appcontent.Post, error) {
			got = in
			return appcontent.Post{Status: content.Status{ID: sampleStatusID}}, nil
		},
		view: func(v appcontent.Viewer, _ string) (appcontent.StatusView, error) { return sampleView(v), nil },
	}
	e := newStatusEnv(t, f)
	res := e.send(t, http.MethodPost, "/statuses", "write", `{"text":"hi","visibility":"local","spoiler_text":"cw","sensitive":true,
		"language":"en","in_reply_to_id":"`+sampleStatusID+`","media":[{"id":"`+sampleMediaID+`","description":"a cat"},{"id":"`+otherMediaID+`"}]}`)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("POST = %d", res.StatusCode)
	}
	want := appcontent.CreateInput{
		TextInput: appcontent.TextInput{
			Text: "hi", SpoilerText: "cw", Sensitive: true, Language: "en",
			Media: []appcontent.MediaInput{{MediaID: sampleMediaID, Description: "a cat"}, {MediaID: otherMediaID}},
		},
		AuthorAccountID: aliceAccountID, Visibility: content.VisibilityLocal, InReplyToID: sampleStatusID, ClientID: "client-1",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("CreateInput = %+v, want %+v", got, want)
	}

	// A poll ends expires_in seconds after the post (FR-112).
	res = e.send(t, http.MethodPost, "/statuses", "write", `{"text":"which?","visibility":"public",
		"poll":{"options":["a","b"],"expires_in":3600,"multiple":true,"hide_totals":true}}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST with a poll = %d", res.StatusCode)
	}
	wantPoll := content.PollInput{Options: []string{"a", "b"}, Duration: time.Hour, Multiple: true, HideTotals: true}
	if got.Poll == nil || fmt.Sprint(*got.Poll) != fmt.Sprint(wantPoll) {
		t.Errorf("poll = %+v, want %+v", got.Poll, wantPoll)
	}

	req := httptest.NewRequest(http.MethodPost, clientapi.BasePath+"/statuses", strings.NewReader(`{"text":"hi","visibility":"public"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer write")
	req.Header.Set("Idempotency-Key", "key-1")
	if res := e.spec.Serve(t, e.h, req); res.StatusCode != http.StatusOK || got.IdempotencyKey != "key-1" {
		t.Errorf("POST with Idempotency-Key = %d, key %q", res.StatusCode, got.IdempotencyKey)
	}
}

// TestStatusProblems checks the problem that each failure of a use case becomes.
func TestStatusProblems(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		typ    string
		detail string
	}{
		{&appcontent.TextTooLongError{Length: 501, Max: 500}, http.StatusUnprocessableEntity, "", "本文と注意書きは合わせて 500 文字以内にしてください。"},
		{appcontent.ErrEmptyText, http.StatusUnprocessableEntity, "", "投稿の本文がありません。"},
		{fmt.Errorf("x: %w", appcontent.ErrMediaNotFound), http.StatusUnprocessableEntity, "", "添付したファイルがあなたのアップロードの中に見つかりません。"},
		{fmt.Errorf("x: %w", appcontent.ErrMediaNotReady), http.StatusUnprocessableEntity, "", "添付したファイルは処理中か、処理できませんでした。"},
		{appcontent.ErrTooManyMedia, http.StatusUnprocessableEntity, "", "1 件の投稿に添付できるファイルは重複なしで 4 個までです。"},
		{appcontent.ErrReplyTargetNotFound, http.StatusUnprocessableEntity, "", "返信先の投稿が見つかりません。"},
		{fmt.Errorf("x: %w", content.ErrInvalidPoll), http.StatusUnprocessableEntity, "", "投票の選択肢は互いに異なる 2〜4 個（それぞれ 50 文字以内）にし、期間は 5 分〜30 日にしてください。"},
		{appcontent.ErrPollWithMedia, http.StatusUnprocessableEntity, "", "投票のある投稿にはファイルを添付できません。"},
		{appcontent.ErrPollNotEditable, http.StatusUnprocessableEntity, "", "投票のある投稿は編集できません。"},
		{content.ErrInvalidVisibility, http.StatusUnprocessableEntity, "", ""},
		{fmt.Errorf("x: %w", content.ErrInvalidIdempotencyKey), http.StatusUnprocessableEntity, "", "Idempotency-Key の値が正しくありません。"},
		{appcontent.ErrPostingRestricted, http.StatusForbidden, "", "現在、あなたのアカウントは投稿できません。"},
		{fmt.Errorf("x: %w", appcontent.ErrInteractionRestricted), http.StatusForbidden, "", "現在、あなたのアカウントはフォロー・お気に入り・ブースト・返信ができません。"},
		{appcontent.ErrAlreadyDeleted, http.StatusConflict, "", "この Idempotency-Key で作成された投稿は削除されています。"},
		{appcontent.ErrAccountUnavailable, http.StatusForbidden, clientapi.ProblemTypeAccountUnavailable, ""},
		{errors.New("boom"), http.StatusInternalServerError, "", ""},
	} {
		e := newStatusEnv(t, &fakeStatuses{create: func(appcontent.CreateInput) (appcontent.Post, error) { return appcontent.Post{}, tc.err }})
		res := e.send(t, http.MethodPost, "/statuses", "write", `{"text":"hi","visibility":"public"}`)
		p := decodeProblem(t, res)
		typ := clientapi.ProblemTypeBlank
		if tc.typ != "" {
			typ = tc.typ
		}
		if res.StatusCode != tc.status || p.Type != typ || (tc.detail != "" && (p.Detail == nil || *p.Detail != tc.detail)) ||
			res.Header.Get("Content-Language") != "ja" {
			t.Errorf("%v: %d %+v, want %d %s %q", tc.err, res.StatusCode, p, tc.status, typ, tc.detail)
		}
	}

	interactions := []struct {
		method, path string
		err          error
		detail       string
	}{
		{http.MethodPost, "/boost", content.ErrNotBoostable, "この投稿はブーストできません。"},
		{http.MethodPost, "/pin", content.ErrNotPinnable, "固定できるのは、ダイレクト以外の自分の投稿だけです。"},
		{http.MethodPost, "/pin", content.ErrPinLimit, "固定できる投稿は 5 件までです。"},
		{http.MethodPost, "/favourite", errNotFound, ""},
		{http.MethodDelete, "/bookmark", errNotFound, ""},
		{http.MethodPost, "/favourite", appcontent.ErrInteractionRestricted, "現在、あなたのアカウントはフォロー・お気に入り・ブースト・返信ができません。"},
		{http.MethodPost, "/boost", appcontent.ErrPostingRestricted, "現在、あなたのアカウントは投稿できません。"},
	}
	for _, tc := range interactions {
		e := newStatusEnv(t, &fakeStatuses{interact: func(string, string, string) error { return tc.err }})
		res := e.send(t, tc.method, "/statuses/"+sampleStatusID+tc.path, "write", "")
		p := decodeProblem(t, res)
		want := http.StatusUnprocessableEntity
		switch {
		case errors.Is(tc.err, appcontent.ErrNotFound):
			want = http.StatusNotFound
		case errors.Is(tc.err, appcontent.ErrInteractionRestricted), errors.Is(tc.err, appcontent.ErrPostingRestricted):
			want = http.StatusForbidden
		}
		if res.StatusCode != want || (tc.detail != "" && (p.Detail == nil || *p.Detail != tc.detail)) {
			t.Errorf("%s %s with %v = %d %+v", tc.method, tc.path, tc.err, res.StatusCode, p)
		}
	}
}

// TestEditAndDeleteOfAnotherAccountsStatus checks that changing another account's status is a
// 403 when the viewer may see it and a 404, like a missing status, when not.
func TestEditAndDeleteOfAnotherAccountsStatus(t *testing.T) {
	visible := true
	f := &fakeStatuses{
		edit: func(appcontent.EditInput) (appcontent.Post, error) { return appcontent.Post{}, appcontent.ErrNotAuthor },
		del:  func(string, string) error { return appcontent.ErrNotAuthor },
		view: func(v appcontent.Viewer, _ string) (appcontent.StatusView, error) {
			if !visible {
				return appcontent.StatusView{}, errNotFound
			}
			return sampleView(v), nil
		},
	}
	e := newStatusEnv(t, f)
	for _, tc := range []struct {
		visible bool
		want    int
	}{{true, http.StatusForbidden}, {false, http.StatusNotFound}} {
		visible = tc.visible
		if res := e.send(t, http.MethodPut, "/statuses/"+sampleStatusID, "write", `{"text":"mine"}`); res.StatusCode != tc.want {
			t.Errorf("PUT another account's status (visible %v) = %d, want %d", tc.visible, res.StatusCode, tc.want)
		}
		if res := e.send(t, http.MethodDelete, "/statuses/"+sampleStatusID, "write", ""); res.StatusCode != tc.want {
			t.Errorf("DELETE another account's status (visible %v) = %d, want %d", tc.visible, res.StatusCode, tc.want)
		}
	}
	f.del = func(string, string) error { return nil }
	if res := e.send(t, http.MethodDelete, "/statuses/"+sampleStatusID, "write", ""); res.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE own status = %d, want 204", res.StatusCode)
	}
}

// TestInteractionsReturnTheStatus checks that every interaction runs as the viewer and answers
// with the status afterwards.
func TestInteractionsReturnTheStatus(t *testing.T) {
	var ops []string
	f := &fakeStatuses{
		interact: func(op, accountID, id string) error {
			if accountID != aliceAccountID || id != sampleStatusID {
				return fmt.Errorf("unexpected %s by %s of %s", op, accountID, id)
			}
			ops = append(ops, op)
			return nil
		},
		view: func(v appcontent.Viewer, _ string) (appcontent.StatusView, error) { return sampleView(v), nil },
	}
	e := newStatusEnv(t, f)
	for _, path := range []string{"boost", "favourite", "bookmark", "pin"} {
		for _, method := range []string{http.MethodPost, http.MethodDelete} {
			res := e.send(t, method, "/statuses/"+sampleStatusID+"/"+path, "write", "")
			if res.StatusCode != http.StatusOK || decodeStatus(t, res).Id != sampleStatusID {
				t.Errorf("%s %s = %d", method, path, res.StatusCode)
			}
		}
	}
	if want := "boost unboost favourite unfavourite bookmark unbookmark pin unpin"; strings.Join(ops, " ") != want {
		t.Errorf("operations = %v, want %s", ops, want)
	}
}

// The quote operations (ADR-0069 §5): a quote and its policy on posting, the policy of a status,
// the list of its quotes, and the revocation of an approval.
func TestQuoteOperations(t *testing.T) {
	var ops []string
	var created appcontent.CreateInput
	f := &fakeStatuses{
		interact: func(op, accountID, id string) error {
			if accountID != aliceAccountID || id != sampleStatusID {
				return fmt.Errorf("unexpected %s by %s of %s", op, accountID, id)
			}
			ops = append(ops, op)
			return nil
		},
		create: func(in appcontent.CreateInput) (appcontent.Post, error) {
			created = in
			return appcontent.Post{Status: content.Status{ID: sampleStatusID}}, nil
		},
		view: func(v appcontent.Viewer, _ string) (appcontent.StatusView, error) {
			view := sampleView(v)
			view.Quotable, view.QuotePolicy = true, content.QuotePolicyFollowers
			return view, nil
		},
	}
	e := newStatusEnv(t, f)

	res := e.send(t, http.MethodPost, "/statuses", "write", `{"text":"look","visibility":"public","quoted_status_id":"`+sampleStatusID+`","quote_policy":"nobody"}`)
	if res.StatusCode != http.StatusOK || created.QuotedStatusID != sampleStatusID || created.QuotePolicy != content.QuotePolicyNobody {
		t.Errorf("create = %d with %+v, want the quote and its policy", res.StatusCode, created)
	}
	if s := decodeStatus(t, res); s.Quotable == nil || !*s.Quotable || s.QuotePolicy == nil || *s.QuotePolicy != clientapi.QuotePolicyFollowers {
		t.Errorf("status = quotable %v, policy %v; want true and followers", s.Quotable, s.QuotePolicy)
	}
	if s := decodeStatus(t, e.send(t, http.MethodGet, "/statuses/"+sampleStatusID, "", "")); s.Quotable != nil {
		t.Errorf("an anonymous status says whether it is quotable: %v", *s.Quotable)
	}
	res = e.send(t, http.MethodPut, "/statuses/"+sampleStatusID+"/quote_policy", "write", `{"quote_policy":"followers"}`)
	if res.StatusCode != http.StatusOK || decodeStatus(t, res).Id != sampleStatusID {
		t.Errorf("set quote policy = %d", res.StatusCode)
	}
	res = e.send(t, http.MethodGet, "/statuses/"+sampleStatusID+"/quotes", "read", "")
	var page clientapi.StatusPage
	if err := json.NewDecoder(res.Body).Decode(&page); err != nil || res.StatusCode != http.StatusOK || len(page.Items) != 1 {
		t.Errorf("quotes = %d %+v, %v", res.StatusCode, page, err)
	}
	const quoting = "01890a5d-ac96-774b-bcce-b302099a9004"
	if res := e.send(t, http.MethodPost, "/statuses/"+sampleStatusID+"/quotes/"+quoting+"/revoke", "write", ""); res.StatusCode != http.StatusNoContent {
		t.Errorf("revoke = %d, want 204", res.StatusCode)
	}
	if want := "quote_policy:followers quotes revoke_quote:" + quoting; strings.Join(ops, " ") != want {
		t.Errorf("operations = %v, want %s", ops, want)
	}
}

func TestPostLimit(t *testing.T) {
	f := &fakeStatuses{
		create: func(appcontent.CreateInput) (appcontent.Post, error) {
			return appcontent.Post{Status: content.Status{ID: sampleStatusID}}, nil
		},
		view: func(v appcontent.Viewer, _ string) (appcontent.StatusView, error) { return sampleView(v), nil },
	}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Statuses, o.StatusReader, o.Interactions = f, f, f
		o.AccountLimit = kernel.Limit{Name: "api.account", Max: 1000, Window: time.Minute}
	})
	for i := range clientapi.PostLimit.Max {
		if res := e.send(t, http.MethodPost, "/statuses", "write", `{"text":"hi","visibility":"public"}`); res.StatusCode != http.StatusOK {
			t.Fatalf("post %d = %d", i, res.StatusCode)
		}
	}
	res := e.send(t, http.MethodPost, "/statuses", "write", `{"text":"hi","visibility":"public"}`)
	if res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") == "" {
		t.Errorf("post over the limit = %d, Retry-After %q", res.StatusCode, res.Header.Get("Retry-After"))
	}
}

// A retry with the Idempotency-Key of a posted status returns that status even over the post limit,
// without posting again; a key that posted nothing is a new post, which the limit refuses.
func TestCreateStatusReplaysBeforeTheLimit(t *testing.T) {
	posted := map[string]bool{}
	var created int64
	f := &fakeStatuses{
		create: func(in appcontent.CreateInput) (appcontent.Post, error) {
			created++
			posted[in.IdempotencyKey] = true
			return appcontent.Post{Status: content.Status{ID: sampleStatusID}}, nil
		},
		replay: func(accountID, key string) (appcontent.Post, error) {
			if accountID != aliceAccountID || !posted[key] {
				return appcontent.Post{}, errNotFound
			}
			return appcontent.Post{Status: content.Status{ID: sampleStatusID}}, nil
		},
		view: func(v appcontent.Viewer, _ string) (appcontent.StatusView, error) { return sampleView(v), nil },
	}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Statuses, o.StatusReader, o.Interactions = f, f, f
		o.AccountLimit = kernel.Limit{Name: "api.account", Max: 1000, Window: time.Minute}
	})
	post := func(key string) *http.Response {
		req := httptest.NewRequest(http.MethodPost, clientapi.BasePath+"/statuses", strings.NewReader(`{"text":"hi","visibility":"public"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer write")
		req.Header.Set("Idempotency-Key", key)
		return e.spec.Serve(t, e.h, req)
	}
	for i := range clientapi.PostLimit.Max {
		if res := post(fmt.Sprintf("key-%d", i)); res.StatusCode != http.StatusOK {
			t.Fatalf("post %d = %d", i, res.StatusCode)
		}
	}
	if res := post("key-0"); res.StatusCode != http.StatusOK || decodeStatus(t, res).Id != sampleStatusID {
		t.Errorf("retry over the limit = %d, want 200 with the status", res.StatusCode)
	}
	if created != clientapi.PostLimit.Max {
		t.Errorf("created %d statuses, want %d", created, clientapi.PostLimit.Max)
	}
	if res := post("key-new"); res.StatusCode != http.StatusTooManyRequests {
		t.Errorf("new post over the limit = %d, want 429", res.StatusCode)
	}
}

type fakeThreads struct {
	opened []string
	err    error
}

func (f *fakeThreads) FetchThread(_ context.Context, statusID string) error {
	f.opened = append(f.opened, statusID)
	return f.err
}

// Opening a thread starts fetching its remote parts; a failure to start does not fail the request,
// and a status the viewer may not see starts nothing.
func TestGetStatusContextFetchesTheThread(t *testing.T) {
	f := &fakeStatuses{view: func(v appcontent.Viewer, id string) (appcontent.StatusView, error) {
		if id != sampleStatusID {
			return appcontent.StatusView{}, errNotFound
		}
		return sampleView(v), nil
	}}
	threads := &fakeThreads{}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Statuses, o.StatusReader, o.Interactions, o.Threads = f, f, f, threads
	})
	if res := e.send(t, http.MethodGet, "/statuses/"+sampleStatusID+"/context", "", ""); res.StatusCode != http.StatusOK {
		t.Fatalf("GET context = %d", res.StatusCode)
	}
	threads.err = errors.New("queue down")
	if res := e.send(t, http.MethodGet, "/statuses/"+sampleStatusID+"/context", "read", ""); res.StatusCode != http.StatusOK {
		t.Errorf("GET context while the fetch fails = %d", res.StatusCode)
	}
	if res := e.send(t, http.MethodGet, "/statuses/01890a5d-ac96-774b-bcce-b302099a9999/context", "", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET context of a missing status = %d", res.StatusCode)
	}
	if len(threads.opened) != 2 || threads.opened[0] != sampleStatusID {
		t.Errorf("threads fetched = %v, want %s twice", threads.opened, sampleStatusID)
	}
}

const (
	sampleMediaID = "01890a5d-ac96-774b-bcce-b302099a9501"
	otherMediaID  = "01890a5d-ac96-774b-bcce-b302099a9502"
)
