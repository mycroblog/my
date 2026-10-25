// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// With media_origin set, the URLs of the local media files and of the media proxy are on it in
// the client API and in ActivityPub, while the ids of ActivityPub objects stay on the host
// (ADR-0018), and the server still serves the files to anyone at /media/ (a CDN's origin pull).
func TestMediaOrigin(t *testing.T) {
	t.Parallel()
	const origin = "https://media.example"
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_MEDIA_ORIGIN": origin})
	var localID, remoteID, accountID string
	s := start(t, cfg, platform.ServeOptions{}, func(a *platform.App) {
		ctx := t.Context()
		reg, err := a.Identity.RegisterLocalUser(ctx, identity.RegisterInput{
			Username: "alice", Password: "correct horse battery staple", DisplayName: "Alice",
		})
		if err != nil {
			t.Fatalf("RegisterLocalUser: %v", err)
		}
		accountID = reg.AccountID
		m, err := a.Media.Upload(ctx, appcontent.UploadInput{OwnerAccountID: accountID, Content: bytes.NewReader(testkit.FixturePNG(64, 48, false))})
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		a.Media.Process(ctx, m.File.ID, "")
		p, err := a.Content.Create(ctx, appcontent.CreateInput{
			AuthorAccountID: accountID, TextInput: appcontent.TextInput{Text: "with a picture", Media: []appcontent.MediaInput{{MediaID: m.File.ID}}},
			Visibility: domcontent.VisibilityPublic,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		localID = p.Status.ID

		const actor = "https://remote.example/users/carol"
		var carol account.Account
		err = a.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			carol, err = a.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
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
		at := time.Now().UTC()
		err = a.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			p, err := a.Content.IngestRemoteStatus(ctx, tx, appcontent.RemoteStatusInput{
				URI: actor + "/statuses/1", URL: "https://remote.example/@carol/1",
				AuthorAccountID: carol.ID, AuthorDomain: "remote.example", Visibility: domcontent.VisibilityPublic,
				PublishedAt: at, RevisedAt: at, Bodies: []appcontent.RemoteBody{{Language: "und", HTML: "<p>hi</p>"}},
				Attachments: []appcontent.RemoteAttachmentInput{{
					Type: "Image", URL: "https://remote.example/files/cat.png", ContentType: "image/png", Description: "a cat", Width: 640, Height: 480,
				}},
			})
			remoteID = p.Status.ID
			return err
		})
		if err != nil {
			t.Fatalf("IngestRemoteStatus: %v", err)
		}
	})

	// The client API: the local file and the proxied remote file, and the proxied avatar.
	var local, remote clientapi.Status
	if resp, body := s.do(t, http.MethodGet, "/api/v1/statuses/"+localID, nil); resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(body), &local) != nil {
		t.Fatalf("GET local status = %d %s", resp.StatusCode, body)
	}
	if resp, body := s.do(t, http.MethodGet, "/api/v1/statuses/"+remoteID, nil); resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(body), &remote) != nil {
		t.Fatalf("GET remote status = %d %s", resp.StatusCode, body)
	}
	if len(local.MediaAttachments) != 1 || local.MediaAttachments[0].Url == nil || !strings.HasPrefix(*local.MediaAttachments[0].Url, origin+"/media/") {
		t.Errorf("local attachment = %+v, want a URL under %s/media/", local.MediaAttachments, origin)
	}
	if len(remote.MediaAttachments) != 1 || remote.MediaAttachments[0].Url == nil || !strings.HasPrefix(*remote.MediaAttachments[0].Url, origin+"/media/proxy/") {
		t.Errorf("remote attachment = %+v, want a URL under %s/media/proxy/", remote.MediaAttachments, origin)
	}
	if a := remote.Account; a.Avatar == nil || !strings.HasPrefix(a.Avatar.Large.Url, origin+"/media/proxy/") {
		t.Errorf("remote avatar = %+v, want a URL under %s/media/proxy/", a.Avatar, origin)
	}
	// Pages and handles are not media: they stay on the server.
	if local.Account.Url != "https://"+cfg.Host+"/@alice" {
		t.Errorf("account url = %q, want it on the host", local.Account.Url)
	}

	// ActivityPub: the attachment is on the media origin and the ids are not.
	ap := map[string]string{"Accept": "application/activity+json"}
	resp, body := s.do(t, http.MethodGet, "/ap/objects/"+localID, ap)
	var note struct {
		ID           string
		AttributedTo string
		Attachment   []struct{ URL string }
	}
	if err := json.Unmarshal([]byte(body), &note); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET Note = %d %s (%v)", resp.StatusCode, body, err)
	}
	if len(note.Attachment) != 1 || !strings.HasPrefix(note.Attachment[0].URL, origin+"/media/") {
		t.Errorf("Note attachment = %+v, want a URL under %s/media/", note.Attachment, origin)
	}
	if want := "https://" + cfg.Host + "/ap/"; !strings.HasPrefix(note.ID, want) || !strings.HasPrefix(note.AttributedTo, want) {
		t.Errorf("Note id = %q, attributedTo = %q, want them under %s", note.ID, note.AttributedTo, want)
	}
	if strings.Contains(body, "media.example/ap/") {
		t.Errorf("Note %s has an id on the media origin", body)
	}

	// The server serves the file itself, to an anonymous request, as the CDN's origin.
	path := strings.TrimPrefix(note.Attachment[0].URL, origin)
	if resp, _ := s.do(t, http.MethodGet, path, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("GET %s = %d, want 200 from the origin", path, resp.StatusCode)
	}
}
