// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/stream"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/timeline"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

const (
	conversationID       = "01890a5d-ac96-774b-bcce-b302099a8400"
	mergedConversationID = "01890a5d-ac96-774b-bcce-b302099a8401"
	unknownAccountID     = "01890a5d-ac96-774b-bcce-b302099a8402"
)

// fakeConversations answers with one conversation of alice, whose last post is postID; the
// merged ID names it too. It records the viewers and the commands.
type fakeConversations struct {
	viewers  []appcontent.Viewer
	queries  []apptimeline.Query
	commands []string
}

func (f *fakeConversations) conversation() apptimeline.Conversation {
	return apptimeline.Conversation{
		ID:         conversationID,
		LastEntry:  apptimeline.Entry{ID: postID, Kind: timeline.KindPost, StatusID: postID, AuthorAccountID: aliceAccountID, OriginalAuthorAccountID: aliceAccountID},
		AccountIDs: []string{bobAccountID, unknownAccountID},
		Unread:     true,
	}
}

func (f *fakeConversations) find(viewer appcontent.Viewer, id string) (apptimeline.Conversation, error) {
	f.viewers = append(f.viewers, viewer)
	if viewer.AccountID != aliceAccountID || (id != conversationID && id != mergedConversationID) {
		return apptimeline.Conversation{}, apptimeline.ErrNotFound
	}
	return f.conversation(), nil
}

func (f *fakeConversations) Conversations(_ context.Context, viewer appcontent.Viewer, q apptimeline.Query) (apptimeline.ConversationPage, error) {
	f.viewers, f.queries = append(f.viewers, viewer), append(f.queries, q)
	return apptimeline.ConversationPage{Conversations: []apptimeline.Conversation{f.conversation()}, Next: postID}, nil
}

func (f *fakeConversations) Conversation(_ context.Context, viewer appcontent.Viewer, id string) (apptimeline.Conversation, error) {
	return f.find(viewer, id)
}

func (f *fakeConversations) ReadConversation(_ context.Context, viewer appcontent.Viewer, id string) (apptimeline.Conversation, error) {
	c, err := f.find(viewer, id)
	if err == nil {
		f.commands = append(f.commands, "read "+id)
		c.Unread = false
	}
	return c, err
}

func (f *fakeConversations) HideConversation(_ context.Context, viewer appcontent.Viewer, id string) error {
	_, err := f.find(viewer, id)
	if err == nil {
		f.commands = append(f.commands, "hide "+id)
	}
	return err
}

func (f *fakeConversations) StreamConversation(_ context.Context, viewer appcontent.Viewer, id string) (apptimeline.Conversation, bool, error) {
	c, err := f.find(viewer, id)
	return c, err == nil, nil
}

func newConversationsEnv(t *testing.T, tweaks ...func(*clientapi.Options)) (*authEnv, *fakeConversations) {
	t.Helper()
	te := newTimelineEnv(t)
	f := &fakeConversations{}
	views := te.views
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.TimelineReader, o.Conversations = views, f
		o.Accounts.(fakeAccounts)[bobAccountID] = bobActor
		for _, tweak := range tweaks {
			tweak(o)
		}
	})
	e.auth.tokens["read-write"] = bearer("read write", aliceAccountID)
	e.auth.tokens["no-direct"] = bearer("read:statuses read:notifications", aliceAccountID)
	return e, f
}

func decodeConversation(t *testing.T, res *http.Response) clientapi.Conversation {
	t.Helper()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var c clientapi.Conversation
	if err := json.NewDecoder(res.Body).Decode(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

func checkConversation(t *testing.T, what string, c clientapi.Conversation, unread bool) {
	t.Helper()
	if c.Id != conversationID || c.LastStatus.Id != postID || c.Unread != unread {
		t.Errorf("%s = {%s last %s unread %v}, want {%s last %s unread %v}", what, c.Id, c.LastStatus.Id, c.Unread, conversationID, postID, unread)
	}
	// An account that is gone is left out.
	if len(c.Accounts) != 1 || c.Accounts[0].Id != bobAccountID {
		t.Errorf("%s accounts = %+v, want bob", what, c.Accounts)
	}
}

func TestListConversations(t *testing.T) {
	e, f := newConversationsEnv(t)
	res := e.get(t, "/conversations?limit=5", map[string]string{"Authorization": "Bearer read"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	var page clientapi.ConversationPage
	if err := json.NewDecoder(res.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.NextCursor == nil || *page.NextCursor != postID {
		t.Fatalf("page = %+v, want one conversation and the next cursor", page)
	}
	checkConversation(t, "item", page.Items[0], true)
	if len(f.queries) != 1 || f.queries[0].Limit != 5 || len(f.queries[0].Languages) != 0 {
		t.Errorf("queries = %+v, want limit 5 and no languages", f.queries)
	}
	if len(f.viewers) != 1 || f.viewers[0].AccountID != aliceAccountID || f.viewers[0].DirectDenied {
		t.Errorf("viewers = %+v, want alice with read:direct", f.viewers)
	}
	// read:statuses alone does not read the direct messages.
	if res := e.get(t, "/conversations", map[string]string{"Authorization": "Bearer read-statuses"}); res.StatusCode != http.StatusForbidden {
		t.Errorf("without read:direct = %d, want 403", res.StatusCode)
	}
	if res := e.get(t, "/conversations?cursor=nope", map[string]string{"Authorization": "Bearer read"}); res.StatusCode != http.StatusBadRequest {
		t.Errorf("a broken cursor = %d, want 400", res.StatusCode)
	}
}

func TestGetConversation(t *testing.T) {
	e, _ := newConversationsEnv(t)
	// The merged conversation's ID answers with the conversation it was merged into.
	for _, id := range []string{conversationID, mergedConversationID} {
		c := decodeConversation(t, e.get(t, "/conversations/"+id, map[string]string{"Authorization": "Bearer read"}))
		checkConversation(t, "GET "+id, c, true)
	}
	for _, tc := range []struct{ token, id string }{
		{"bob", conversationID},
		{"read", "01890a5d-ac96-774b-bcce-b302099a8499"},
		{"read", "not-an-id"},
	} {
		res := e.get(t, "/conversations/"+tc.id, map[string]string{"Authorization": "Bearer " + tc.token, "Accept-Language": "en"})
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s as %s = %d, want 404", tc.id, tc.token, res.StatusCode)
		}
	}
}

func TestMarkAndHideConversations(t *testing.T) {
	e, f := newConversationsEnv(t)
	c := decodeConversation(t, e.send(t, http.MethodPost, "/conversations/"+mergedConversationID+"/read", "read-write", ""))
	checkConversation(t, "read", c, false)
	if res := e.send(t, http.MethodPost, "/conversations/"+conversationID+"/hide", "read-write", ""); res.StatusCode != http.StatusNoContent {
		t.Errorf("hide = %d, want 204", res.StatusCode)
	}
	if want := []string{"read " + mergedConversationID, "hide " + conversationID}; len(f.commands) != 2 || f.commands[0] != want[0] || f.commands[1] != want[1] {
		t.Errorf("commands = %v, want %v", f.commands, want)
	}
	for _, path := range []string{"/read", "/hide"} {
		if res := e.send(t, http.MethodPost, "/conversations/01890a5d-ac96-774b-bcce-b302099a8499"+path, "read-write", ""); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s of an unknown conversation = %d, want 404", path, res.StatusCode)
		}
		// write:statuses alone does not reach the direct messages.
		if res := e.send(t, http.MethodPost, "/conversations/"+conversationID+path, "write", ""); res.StatusCode != http.StatusForbidden {
			t.Errorf("%s without read:direct = %d, want 403", path, res.StatusCode)
		}
	}
}

func TestStreamingConversations(t *testing.T) {
	hub := stream.NewHub(stream.HubOptions{})
	e, _ := newConversationsEnv(t, func(o *clientapi.Options) { o.Streams, o.StreamHeartbeat = hub, 100*time.Millisecond })
	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)

	_, _, r := openStream(t.Context(), t, srv, "read")
	_, _, denied := openStream(t.Context(), t, srv, "no-direct")
	for _, s := range []*bufio.Reader{r, denied} {
		if line := readLine(t, s); line != ": connected" {
			t.Fatalf("first line = %q, want the connected comment", line)
		}
	}
	// The signal of another conversation's party, and of a conversation the list leaves out,
	// send nothing; the next one sends the conversation.
	hub.Publish(stream.Event{Kind: stream.KindConversation, AccountID: bobAccountID, ConversationID: conversationID})
	hub.Publish(stream.Event{Kind: stream.KindConversation, AccountID: aliceAccountID, ConversationID: "01890a5d-ac96-774b-bcce-b302099a8499"})
	hub.Publish(stream.Event{Kind: stream.KindConversation, AccountID: aliceAccountID, ConversationID: mergedConversationID})
	if line := readLine(t, r); line != "event: conversation" {
		t.Fatalf("line = %q, want a conversation event", line)
	}
	var c clientapi.Conversation
	if err := json.Unmarshal([]byte(strings.TrimPrefix(readLine(t, r), "data: ")), &c); err != nil {
		t.Fatal(err)
	}
	checkConversation(t, "streamed", c, true)
	// A token without read:direct receives no conversation: the heartbeat comes next.
	if line := readLine(t, denied); line != ": heartbeat" {
		t.Errorf("a stream without read:direct got %q, want only the heartbeat", line)
	}
}
