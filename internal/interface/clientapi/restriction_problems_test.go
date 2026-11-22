// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// restrictedArticles refuses every article with err.
type restrictedArticles struct {
	clientapi.Articles
	err error
}

func (f restrictedArticles) CreateArticle(context.Context, appcontent.CreateArticleInput) (appcontent.Post, error) {
	return appcontent.Post{}, f.err
}

func (f restrictedArticles) EditArticle(context.Context, appcontent.EditArticleInput) (appcontent.Post, error) {
	return appcontent.Post{}, f.err
}

// restrictedBoards refuses every post on a board with err.
type restrictedBoards struct {
	clientapi.EventBoards
	err error
}

func (f restrictedBoards) PostOnBoard(context.Context, appcontent.BoardPostInput) (appcontent.Post, error) {
	return appcontent.Post{}, f.err
}

// restrictedGraph refuses every follow with err.
type restrictedGraph struct {
	fakeGraph
	err error
}

func (f restrictedGraph) Follow(context.Context, string, string) (appgraph.Relationship, error) {
	return appgraph.Relationship{}, f.err
}

func (f restrictedGraph) FollowWith(context.Context, string, string, appgraph.FollowOptions) (appgraph.Relationship, error) {
	return appgraph.Relationship{}, f.err
}

// restrictionProblem is what a refusal of a partial restriction answers: a 403 of its own type,
// titled errors.forbidden, with the sentence that names Account status (#3216).
type restrictionProblem struct {
	typ, detail string
}

func restrictionProblems(t *testing.T) (posting, interaction restrictionProblem, title string) {
	t.Helper()
	messages, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	ja := messages.Localizer("ja")
	return restrictionProblem{clientapi.ProblemTypePostingRestricted, ja.ServerErrorsStatusPostingRestricted()},
		restrictionProblem{clientapi.ProblemTypeInteractionRestricted, ja.ServerErrorsInteractionRestricted()},
		ja.ServerErrorsForbidden()
}

func checkRestrictionProblem(t *testing.T, what string, res *http.Response, want restrictionProblem, title string) {
	t.Helper()
	p := decodeProblem(t, res)
	if res.StatusCode != http.StatusForbidden || p.Type != want.typ || p.Title != title || p.Detail == nil || *p.Detail != want.detail {
		t.Errorf("%s = %d %s %q %q, want 403 %s %q %q", what, res.StatusCode, p.Type, p.Title, deref(p.Detail), want.typ, title, want.detail)
	}
}

// Each operation that a partial restriction refuses answers the problem type of that restriction
// (#3216): posting for a post, an edit, an article, a post on a board, and a boost by an account
// that may not post; interaction for a reply, a quote, a favourite, a boost, a reaction, a vote,
// and a follow by an account that may not interact.
func TestPartialRestrictionProblems(t *testing.T) {
	posting, interaction, title := restrictionProblems(t)
	wrapped := func(err error) error { return fmt.Errorf("x: %w", err) }
	cases := []struct {
		name           string
		postingErr     error
		interactionErr error
	}{
		{"bare", appcontent.ErrPostingRestricted, appcontent.ErrInteractionRestricted},
		{"wrapped", wrapped(appcontent.ErrPostingRestricted), wrapped(appcontent.ErrInteractionRestricted)},
	}

	t.Run("statuses", func(t *testing.T) {
		for _, c := range cases {
			for _, tc := range []struct {
				name, method, path, body string
				err                      error
				want                     restrictionProblem
			}{
				{"create", http.MethodPost, "/statuses", `{"text":"hi","visibility":"public"}`, c.postingErr, posting},
				{"reply", http.MethodPost, "/statuses", `{"text":"hi","visibility":"public","in_reply_to_id":"` + sampleStatusID + `"}`, c.interactionErr, interaction},
				{"quote", http.MethodPost, "/statuses", `{"text":"hi","visibility":"public","quoted_status_id":"` + sampleStatusID + `"}`, c.interactionErr, interaction},
				{"edit", http.MethodPut, "/statuses/" + sampleStatusID, `{"text":"mine"}`, c.postingErr, posting},
			} {
				f := &fakeStatuses{
					create: func(appcontent.CreateInput) (appcontent.Post, error) { return appcontent.Post{}, tc.err },
					edit:   func(appcontent.EditInput) (appcontent.Post, error) { return appcontent.Post{}, tc.err },
					view:   func(v appcontent.Viewer, _ string) (appcontent.StatusView, error) { return sampleView(v), nil },
				}
				e := newStatusEnv(t, f)
				checkRestrictionProblem(t, c.name+" "+tc.name, e.send(t, tc.method, tc.path, "write", tc.body), tc.want, title)
			}
		}
	})

	t.Run("interactions", func(t *testing.T) {
		for _, c := range cases {
			for _, tc := range []struct {
				name, method, path, body string
				err                      error
				want                     restrictionProblem
			}{
				{"favourite", http.MethodPost, "/favourite", "", c.interactionErr, interaction},
				{"boost while posting is restricted", http.MethodPost, "/boost", "", c.postingErr, posting},
				{"boost while interacting is restricted", http.MethodPost, "/boost", "", c.interactionErr, interaction},
				{"reaction", http.MethodPut, "/reaction", `{"emoji":"👍"}`, c.interactionErr, interaction},
				{"vote", http.MethodPost, "/poll/votes", `{"poll_id":"` + samplePollID + `","choices":[0]}`, c.interactionErr, interaction},
			} {
				f := &fakeStatuses{
					view:     func(v appcontent.Viewer, _ string) (appcontent.StatusView, error) { return sampleView(v), nil },
					interact: func(string, string, string) error { return tc.err },
				}
				e := newStatusEnv(t, f)
				checkRestrictionProblem(t, c.name+" "+tc.name, e.send(t, tc.method, "/statuses/"+sampleStatusID+tc.path, "write", tc.body), tc.want, title)
			}
		}
	})

	t.Run("articles", func(t *testing.T) {
		for _, c := range cases {
			e := newAuthEnv(t, func(o *clientapi.Options) { o.Articles = restrictedArticles{err: c.postingErr} })
			res := e.send(t, http.MethodPost, "/articles", "write", `{"title":"T","text":"body","visibility":"public"}`)
			checkRestrictionProblem(t, c.name+" create article", res, posting, title)
			// A token without read:direct or read:events would first read the post, which this env
			// does not hold.
			e.auth.tokens["edit"] = bearer("write read:direct read:events", aliceAccountID)
			res = e.send(t, http.MethodPut, "/articles/"+sampleStatusID, "edit", `{"title":"T","text":"body"}`)
			checkRestrictionProblem(t, c.name+" edit article", res, posting, title)
		}
	})

	t.Run("event board", func(t *testing.T) {
		for _, c := range cases {
			for _, tc := range []struct {
				name, body string
				err        error
				want       restrictionProblem
			}{
				{"post", `{"text":"hi"}`, c.postingErr, posting},
				{"reply", `{"text":"hi","in_reply_to_id":"` + sampleStatusID + `"}`, c.interactionErr, interaction},
			} {
				e := newAuthEnv(t, func(o *clientapi.Options) {
					o.Events = &fakeEvents{}
					o.EventBoards = restrictedBoards{err: tc.err}
				})
				e.auth.tokens["events"] = bearer("read:events write:events", aliceAccountID)
				res := e.send(t, http.MethodPost, "/events/"+eventID+"/board", "events", tc.body)
				checkRestrictionProblem(t, c.name+" board "+tc.name, res, tc.want, title)
			}
		}
	})

	t.Run("follow", func(t *testing.T) {
		for _, err := range []error{appgraph.ErrInteractionRestricted, wrapped(appgraph.ErrInteractionRestricted)} {
			e := newAuthEnv(t, func(o *clientapi.Options) {
				accounts := fakeAccounts{}
				for id, a := range o.Accounts.(fakeAccounts) {
					accounts[id] = a
				}
				accounts[bobAccountID] = appaccount.Actor{
					Account:   account.Account{ID: bobAccountID, URI: "https://social.example/ap/actors/" + bobAccountID, CreatedAt: createdAt},
					Local:     &account.LocalAccount{AccountID: bobAccountID, UserID: userOf(bobAccountID), Username: "bob"},
					Lifecycle: account.InitialLifecycleState(bobAccountID),
				}
				o.Accounts = accounts
				o.Graph = restrictedGraph{err: err}
			})
			checkRestrictionProblem(t, fmt.Sprintf("follow with %v", err), e.send(t, http.MethodPost, "/accounts/"+bobAccountID+"/follow", "write", ""), interaction, title)
		}
	})
}
