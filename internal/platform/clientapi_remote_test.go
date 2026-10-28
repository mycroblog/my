// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/htmlsanitizer"
	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestClientAPIPresentsRemoteStatuses checks the client API's view of remote content on the real
// database: the federated timeline lists the public posts, local and remote, and nothing else; a
// remote post's HTML is the sanitizer's output; its author links to the actor's profile page with
// the avatar behind the media proxy; its attachments are served through the proxy with their alt
// text, the first four of six with the number of the others; and turning the timeline off makes it
// a 404.
func TestClientAPIPresentsRemoteStatuses(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	ctx := t.Context()
	alice := e.user(t, "alice")
	const actor = "https://remote.example/users/carol"
	var carol account.Account
	err := e.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		carol, err = e.app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: actor, Domain: "remote.example", Type: "Person", PreferredUsername: "carol", Name: "Carol",
			ProfileURL: "https://remote.example/@carol", AvatarURL: "https://remote.example/avatars/carol.png",
			InboxURL: actor + "/inbox", FollowersURL: actor + "/followers",
			Document: []byte(`{"id":"` + actor + `"}`),
			Handle:   appaccount.HandleCheck{Outcome: account.HandleVerified, Username: "carol", Domain: "remote.example"},
		})
		return err
	})
	if err != nil {
		t.Fatalf("UpsertRemoteActor: %v", err)
	}

	const dirty = `<p onclick="x()">hi <a href="https://remote.example/tags/cats" class="mention hashtag" rel="tag">#<span>cats</span></a>` +
		`<script>alert(1)</script><img src="https://evil.example/x.png"><iframe src="https://evil.example"></iframe>` +
		`<a href="javascript:alert(1)">bad</a><b style="color:red">bold</b></p>`
	ingest := func(n int, v domcontent.Visibility, attachments ...appcontent.RemoteAttachmentInput) string {
		t.Helper()
		var id string
		at := time.Now().UTC().Add(time.Duration(n-10) * time.Minute)
		err := e.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			p, err := e.app.Content.IngestRemoteStatus(ctx, tx, appcontent.RemoteStatusInput{
				URI: actor + "/statuses/" + strconv.Itoa(n), URL: "https://remote.example/@carol/" + strconv.Itoa(n),
				AuthorAccountID: carol.ID, AuthorDomain: "remote.example", Visibility: v,
				PublishedAt: at, RevisedAt: at, Bodies: []appcontent.RemoteBody{{Language: "und", HTML: dirty}},
				Attachments: attachments,
			})
			id = p.Status.ID
			return err
		})
		if err != nil {
			t.Fatalf("IngestRemoteStatus: %v", err)
		}
		return id
	}
	attachments := []appcontent.RemoteAttachmentInput{{
		Type: "Image", URL: "https://remote.example/files/cat.png", ContentType: "image/png",
		Description: "a <b>cat</b>", Width: 640, Height: 480,
	}}
	for i := range 5 {
		attachments = append(attachments, appcontent.RemoteAttachmentInput{Type: "Image", URL: "https://remote.example/files/" + strconv.Itoa(i) + ".png"})
	}
	public := ingest(1, domcontent.VisibilityPublic, attachments...)
	for i, v := range []domcontent.Visibility{domcontent.VisibilityUnlisted, domcontent.VisibilityPrivate, domcontent.VisibilityDirect} {
		ingest(2+i, v)
	}
	local := e.post(t, alice, "local only", domcontent.VisibilityLocal, "en")
	pub := e.post(t, alice, "hello", domcontent.VisibilityPublic, "en")

	for _, viewer := range []*timelineUser{nil, &alice} {
		page := e.page(t, viewer, "/timelines/public")
		if got, want := itemIDs(page), []string{pub, public}; !slices.Equal(got, want) {
			t.Errorf("federated timeline = %v, want the public posts %v (not %s)", got, want, local)
		}
	}
	st := e.page(t, nil, "/timelines/public").Items[1]
	if want := htmlsanitizer.New().SanitizeHTML(dirty); st.Content != want {
		t.Errorf("content = %q, want the sanitizer's output %q", st.Content, want)
	}
	for _, bad := range []string{"<script", "<img", "<iframe", "javascript:", "onclick", "style="} {
		if strings.Contains(st.Content, bad) {
			t.Errorf("content %q keeps %s", st.Content, bad)
		}
	}
	if st.Url == nil || *st.Url != "https://remote.example/@carol/1" || st.EditedAt != nil {
		t.Errorf("status url = %v, edited_at = %v", st.Url, st.EditedAt)
	}
	proxy := "https://" + e.app.Config.Host + "/media/proxy/"
	a := st.Account
	if a.Acct != "carol@remote.example" || a.Domain != "remote.example" || a.Uri != actor || a.Url != "https://remote.example/@carol" ||
		a.Avatar == nil || !strings.HasPrefix(a.Avatar.Large.Url, proxy) || !strings.HasPrefix(a.Avatar.Small.Url, proxy) {
		t.Errorf("account = %+v, want carol's handle, profile page, and proxied avatar", a)
	}
	if len(st.MediaAttachments) != domcontent.MaxAttachments || st.OmittedMediaCount == nil || *st.OmittedMediaCount != 2 {
		t.Fatalf("media = %+v, omitted = %v, want the first four remote images and two omitted", st.MediaAttachments, st.OmittedMediaCount)
	}
	if got := e.page(t, nil, "/timelines/public").Items[0].OmittedMediaCount; got != nil {
		t.Errorf("local status omitted_media_count = %v, want none", *got)
	}
	m := st.MediaAttachments[0]
	if m.State != "ready" || m.Type != "image" || m.Url == nil || !strings.HasPrefix(*m.Url, proxy) ||
		m.RemoteUrl == nil || *m.RemoteUrl != "https://remote.example/files/cat.png" || m.Description == nil || *m.Description != "a cat" {
		t.Errorf("attachment = %+v, want the proxied image with its alt text", m)
	}

	// The operator turns the federated timeline off.
	if _, err := e.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingFederatedTimelineAccess, Value: "off"}); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	res := e.spec.Serve(t, e.h, httptest.NewRequest(http.MethodGet, clientapi.BasePath+"/timelines/public", nil))
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("disabled federated timeline = %d, want 404", res.StatusCode)
	}
}
