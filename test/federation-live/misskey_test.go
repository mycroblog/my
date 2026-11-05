// SPDX-License-Identifier: AGPL-3.0-only

//go:build federationlive

package federationlive

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// misskeyNoEdits is why TestEdit cannot edit on Misskey: it has no API to edit a note, and its inbox
// applies an Update to actors and polls (Question) only, answering 202 to the others.
const misskeyNoEdits = "Misskey has no note editing; its inbox skips the Update of a Note"

// misskeyReaction is the reaction that stands for a favourite. Misskey sends it as a Like with the
// emoji in content and _misskey_reaction, and shows a Like without one as its default reaction.
const misskeyReaction = "👍"

// misskeyAPI drives a Misskey user through the Misskey API (POST /api/<endpoint> with the user's
// token as "i"), converted to the terms of the Mastodon client API that the scenarios use:
// Misskey's home / followers / specified are unlisted / private / direct, a renote is a boost, and
// a reaction is a favourite.
type misskeyAPI struct {
	host, token string
	http        *http.Client
}

// misskeyNotFound matches the error codes of a missing object, which Misskey answers with a 400.
var misskeyNotFound = regexp.MustCompile(`"code":"NO_SUCH_(NOTE|USER|OBJECT)"`)

func (m *misskeyAPI) call(ctx context.Context, endpoint string, body map[string]any, out any) error {
	if body == nil {
		body = map[string]any{}
	}
	body["i"] = m.token
	err := call(ctx, m.http, "", http.MethodPost, "https://"+m.host+"/api/"+endpoint, body, out)
	var ae *apiError
	if errors.As(err, &ae) && ae.status == http.StatusBadRequest && misskeyNotFound.MatchString(ae.body) {
		// The scenarios look for a 404 (isNotFound).
		ae.status = http.StatusNotFound
	}
	return err
}

// mkUser is the part of a Misskey user (UserLite) that the scenarios read.
type mkUser struct {
	ID       string  `json:"id"`
	Username string  `json:"username"`
	Host     *string `json:"host"`
	URI      *string `json:"uri"`
}

func (m *misskeyAPI) toAccount(u mkUser) account {
	a := account{ID: u.ID, Username: u.Username, Acct: u.Username}
	if u.Host != nil {
		a.Acct += "@" + *u.Host
	}
	if u.URI != nil {
		a.URI = *u.URI
	} else {
		// A local user has no uri in the API; its ActivityPub id is /users/{id}.
		a.URI = "https://" + m.host + "/users/" + u.ID
	}
	return a
}

// mkNote is the part of a Misskey note that the scenarios read.
type mkNote struct {
	ID          string         `json:"id"`
	URI         *string        `json:"uri"`
	Text        *string        `json:"text"`
	Visibility  string         `json:"visibility"`
	ReplyID     *string        `json:"replyId"`
	RenoteCount int            `json:"renoteCount"`
	Reactions   map[string]int `json:"reactions"`
	// IsHidden is set when the user may not see the note: Misskey answers with the note emptied.
	IsHidden bool `json:"isHidden"`
}

// misskeyVisibilities maps the visibilities of the Mastodon client API to Misskey's.
var misskeyVisibilities = map[string]string{
	"public": "public", "unlisted": "home", "private": "followers", "direct": "specified",
}

func (m *misskeyAPI) toStatus(n mkNote) status {
	s := status{ID: n.ID, ReblogsCount: n.RenoteCount}
	if n.URI != nil {
		s.URI = *n.URI
	} else {
		// A local note has no uri in the API; its ActivityPub id is /notes/{id}.
		s.URI = "https://" + m.host + "/notes/" + n.ID
	}
	if n.Text != nil {
		s.Content = *n.Text
	}
	for mastodon, misskey := range misskeyVisibilities {
		if n.Visibility == misskey {
			s.Visibility = mastodon
		}
	}
	if n.ReplyID != nil {
		s.InReplyToID = *n.ReplyID
	}
	for emoji, c := range n.Reactions {
		s.FavouritesCount += c
		s.Reactions = append(s.Reactions, statusReaction{Name: emoji, Count: c})
	}
	return s
}

func (m *misskeyAPI) statuses(ns []mkNote) []status {
	out := make([]status, 0, len(ns))
	for _, n := range ns {
		out = append(out, m.toStatus(n))
	}
	return out
}

// resolve looks the handle up with users/show, which fetches a remote user through WebFinger.
func (m *misskeyAPI) resolve(ctx context.Context, handle string) (account, error) {
	username, host, _ := strings.Cut(handle, "@")
	body := map[string]any{"username": username, "host": nil}
	if host != "" && host != m.host {
		body["host"] = host
	}
	var u mkUser
	if err := m.call(ctx, "users/show", body, &u); err != nil {
		return account{}, err
	}
	return m.toAccount(u), nil
}

// resolveStatus looks the URI up with ap/show, which fetches it with a signed GET if it is new.
func (m *misskeyAPI) resolveStatus(ctx context.Context, uri string) (status, error) {
	var res struct {
		Type   string `json:"type"`
		Object mkNote `json:"object"`
	}
	if err := m.call(ctx, "ap/show", map[string]any{"uri": uri}, &res); err != nil {
		return status{}, err
	}
	if res.Type != "Note" {
		return status{}, fmt.Errorf("%s resolved %s as a %s", m.host, uri, res.Type)
	}
	if res.Object.IsHidden {
		return status{}, fmt.Errorf("%s hides %s from the user", m.host, uri)
	}
	return m.toStatus(res.Object), nil
}

// follow follows the user; following a user who is followed already is not an error, as in the
// Mastodon client API.
func (m *misskeyAPI) follow(ctx context.Context, id string) (relationship, error) {
	if err := m.call(ctx, "following/create", map[string]any{"userId": id}, nil); err != nil && !misskeyError(err, "ALREADY_FOLLOWING") {
		return relationship{}, err
	}
	return m.relationship(ctx, id)
}

// unfollow ends the follow; unfollowing a user who is not followed is not an error, as in the
// Mastodon client API.
func (m *misskeyAPI) unfollow(ctx context.Context, id string) error {
	if err := m.call(ctx, "following/delete", map[string]any{"userId": id}, nil); err != nil && !misskeyError(err, "NOT_FOLLOWING") {
		return err
	}
	return nil
}

// misskeyError reports whether err is a Misskey API error with the code.
func misskeyError(err error, code string) bool {
	var ae *apiError
	return errors.As(err, &ae) && strings.Contains(ae.body, `"code":"`+code+`"`)
}

func (m *misskeyAPI) relationship(ctx context.Context, id string) (relationship, error) {
	var rs []struct {
		IsFollowing                    bool `json:"isFollowing"`
		IsFollowed                     bool `json:"isFollowed"`
		HasPendingFollowRequestFromYou bool `json:"hasPendingFollowRequestFromYou"`
	}
	if err := m.call(ctx, "users/relation", map[string]any{"userId": []string{id}}, &rs); err != nil {
		return relationship{}, err
	}
	if len(rs) != 1 {
		return relationship{}, fmt.Errorf("%d relationships with %s", len(rs), id)
	}
	r := rs[0]
	return relationship{ID: id, Following: r.IsFollowing, Requested: r.HasPendingFollowRequestFromYou, FollowedBy: r.IsFollowed}, nil
}

func (m *misskeyAPI) setLocked(ctx context.Context, locked bool) error {
	return m.call(ctx, "i/update", map[string]any{"isLocked": locked}, nil)
}

func (m *misskeyAPI) followRequests(ctx context.Context) ([]account, error) {
	var rs []struct {
		Follower mkUser `json:"follower"`
	}
	if err := m.call(ctx, "following/requests/list", map[string]any{"limit": 100}, &rs); err != nil {
		return nil, err
	}
	out := make([]account, 0, len(rs))
	for _, r := range rs {
		out = append(out, m.toAccount(r.Follower))
	}
	return out, nil
}

func (m *misskeyAPI) authorize(ctx context.Context, id string) error {
	return m.call(ctx, "following/requests/accept", map[string]any{"userId": id}, nil)
}

// mentionPattern matches the remote mentions of a text (@user@host).
var mentionPattern = regexp.MustCompile(`@([A-Za-z0-9_]+)@([A-Za-z0-9.-]+)`)

// reply posts a note. A specified (direct) note needs the IDs of the users it is for: the users
// that the text mentions.
func (m *misskeyAPI) reply(ctx context.Context, text, visibility, inReplyTo string) (status, error) {
	v, ok := misskeyVisibilities[visibility]
	if !ok {
		return status{}, fmt.Errorf("misskey has no visibility %q", visibility)
	}
	body := map[string]any{"text": text, "visibility": v}
	if inReplyTo != "" {
		body["replyId"] = inReplyTo
	}
	if v == "specified" {
		ids := []string{}
		for _, mention := range mentionPattern.FindAllStringSubmatch(text, -1) {
			a, err := m.resolve(ctx, mention[1]+"@"+mention[2])
			if err != nil {
				return status{}, err
			}
			ids = append(ids, a.ID)
		}
		body["visibleUserIds"] = ids
	}
	var res struct {
		CreatedNote mkNote `json:"createdNote"`
	}
	if err := m.call(ctx, "notes/create", body, &res); err != nil {
		return status{}, err
	}
	return m.toStatus(res.CreatedNote), nil
}

func (m *misskeyAPI) status(ctx context.Context, id string) (status, error) {
	var n mkNote
	if err := m.call(ctx, "notes/show", map[string]any{"noteId": id}, &n); err != nil {
		return status{}, err
	}
	s := m.toStatus(n)
	// renoteCount only ever grows (deleting a renote does not decrement it), so the boosts are
	// the renotes that exist.
	var renotes []mkNote
	if err := m.call(ctx, "notes/renotes", map[string]any{"noteId": id, "limit": 100}, &renotes); err != nil {
		return status{}, err
	}
	s.ReblogsCount = len(renotes)
	return s, nil
}

// context returns the replies to the note (notes/children); Misskey has no ancestors endpoint that
// the scenarios need.
func (m *misskeyAPI) context(ctx context.Context, id string) (thread, error) {
	var ns []mkNote
	if err := m.call(ctx, "notes/children", map[string]any{"noteId": id, "limit": 100}, &ns); err != nil {
		return thread{}, err
	}
	return thread{Descendants: m.statuses(ns)}, nil
}

func (m *misskeyAPI) edit(context.Context, string, string) (status, error) {
	return status{}, errors.New(misskeyNoEdits)
}

func (m *misskeyAPI) delete(ctx context.Context, id string) error {
	return m.call(ctx, "notes/delete", map[string]any{"noteId": id}, nil)
}

// interact renotes (boost) or reacts with misskeyReaction (favourite), or undoes it.
func (m *misskeyAPI) interact(ctx context.Context, id, action string, undo bool) error {
	switch {
	case action == "boost" && !undo:
		return m.call(ctx, "notes/create", map[string]any{"renoteId": id}, nil)
	case action == "boost":
		return m.call(ctx, "notes/unrenote", map[string]any{"noteId": id}, nil)
	case action == "favourite" && !undo:
		return m.call(ctx, "notes/reactions/create", map[string]any{"noteId": id, "reaction": misskeyReaction}, nil)
	case action == "favourite":
		return m.call(ctx, "notes/reactions/delete", map[string]any{"noteId": id}, nil)
	}
	return fmt.Errorf("unknown action %q", action)
}

func (m *misskeyAPI) home(ctx context.Context) ([]status, error) {
	var ns []mkNote
	if err := m.call(ctx, "notes/timeline", map[string]any{"limit": 100}, &ns); err != nil {
		return nil, err
	}
	return m.statuses(ns), nil
}

// notifications returns the user's notifications (a reply is a notification of type "reply",
// not "mention").
func (m *misskeyAPI) notifications(ctx context.Context) ([]notification, error) {
	var ns []struct {
		Type string  `json:"type"`
		User *mkUser `json:"user"`
		Note *mkNote `json:"note"`
	}
	if err := m.call(ctx, "i/notifications", map[string]any{"limit": 100}, &ns); err != nil {
		return nil, err
	}
	out := make([]notification, 0, len(ns))
	for _, n := range ns {
		o := notification{Type: n.Type}
		if n.User != nil {
			o.Account = m.toAccount(*n.User)
		}
		if n.Note != nil {
			s := m.toStatus(*n.Note)
			o.Status = &s
		}
		out = append(out, o)
	}
	return out, nil
}
