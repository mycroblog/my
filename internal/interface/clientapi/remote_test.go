// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"slices"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/timeline"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeRemoteProfiles serves the profile pages of remote accounts and records each call.
type fakeRemoteProfiles struct {
	urls  map[string]string
	calls [][]string
}

func (f *fakeRemoteProfiles) RemoteProfileURLs(_ context.Context, ids []string) (map[string]string, error) {
	f.calls = append(f.calls, ids)
	out := map[string]string{}
	for _, id := range ids {
		if u, ok := f.urls[id]; ok {
			out[id] = u
		}
	}
	return out, nil
}

const (
	remoteStatusID = "01890a5d-ac96-774b-bcce-b302099a8200"
	carolAccountID = "01890a5d-ac96-774b-bcce-b302099a8201"
	daveAccountID  = "01890a5d-ac96-774b-bcce-b302099a8202"
	remoteMediaID  = "01890a5d-ac96-774b-bcce-b302099a8203"
	proxyURL       = "https://social.example/media/proxy/" + remoteMediaID + "/full?sig=abc"
)

func TestRemoteStatusRepresentation(t *testing.T) {
	carol := appaccount.Actor{
		Account: account.Account{ID: carolAccountID, Domain: "remote.example", URI: "https://remote.example/users/carol", CreatedAt: createdAt},
		Handle:  &account.Handle{AccountID: carolAccountID, Username: "carol", Domain: "remote.example"},
		Profile: &account.ProfileRevision{DisplayName: "Carol", NoteHTML: "<p>remote</p>"},
	}
	// dave has no confirmed handle and declares no profile page.
	dave := appaccount.Actor{Account: account.Account{ID: daveAccountID, Domain: "other.example", URI: "https://other.example/ap/dave", CreatedAt: createdAt}}
	focus := content.MediaFocus{MediaID: remoteMediaID, X: 0.5, Y: -0.5}
	view := appcontent.StatusView{
		Status: content.Status{
			ID: remoteStatusID, URI: "https://remote.example/users/carol/statuses/1", URL: "https://remote.example/@carol/1",
			AuthorAccountID: carolAccountID, Visibility: content.VisibilityPublic, PublishedAt: publishedAt,
		},
		Revision: content.Revision{
			Seq: 2, Language: "und", RevisedAt: publishedAt.Add(time.Minute),
			Bodies: []content.Body{{ContentHTML: `<p>hi <a href="https://other.example/ap/dave" class="u-url mention">@dave</a></p>`}},
		},
		Author:   carol,
		Mentions: []appaccount.Actor{dave},
		Attachments: []appcontent.MediaView{{
			Media: appcontent.Media{
				File: content.MediaFile{ID: remoteMediaID, OwnerAccountID: carolAccountID, Origin: content.MediaOriginRemote},
				Remote: &content.RemoteMedia{
					MediaID: remoteMediaID, OwnerAccountID: carolAccountID, RemoteURL: "https://remote.example/files/cat.png",
					DeclaredMediaType: content.MediaTypeImage, Width: 640, Height: 480, Blurhash: "LEHV6nWB2yk8", Focus: &focus,
					Description: "the remote copy",
				},
			},
			Description: "a cat", Image: proxied(proxyURL),
		}, {
			Media: appcontent.Media{
				File:   content.MediaFile{ID: postID, OwnerAccountID: carolAccountID, Origin: content.MediaOriginRemote},
				Remote: &content.RemoteMedia{MediaID: postID, OwnerAccountID: carolAccountID, RemoteURL: "https://remote.example/files/x", DeclaredMediaType: content.MediaTypeUnknown},
			},
			Image: proxied(proxyURL),
		}},
	}
	profiles := &fakeRemoteProfiles{urls: map[string]string{carolAccountID: "https://remote.example/@carol"}}
	te := &timelineEnv{
		timelines: &fakeTimelines{page: apptimeline.Page{Entries: []apptimeline.Entry{{
			ID: remoteStatusID, Kind: timeline.KindPost, StatusID: remoteStatusID, AuthorAccountID: carolAccountID,
			OriginalAuthorAccountID: carolAccountID, AuthorDomain: "remote.example",
		}}}},
		views: &fakeViews{statuses: map[string]appcontent.StatusView{remoteStatusID: view}},
	}
	te.authEnv = newAuthEnv(t, func(o *clientapi.Options) {
		o.Timelines, o.TimelineReader, o.RemoteProfiles = te.timelines, te.views, profiles
		o.Instance = federatedTimelineForEveryone()
	})
	page := decodePage(t, te.get(t, "/timelines/public", nil))
	if len(page.Items) != 1 {
		t.Fatalf("items = %+v, want the remote status", page.Items)
	}
	st := page.Items[0]
	if st.Uri != view.Status.URI || deref(st.Url) != "https://remote.example/@carol/1" || st.Content != view.Revision.Bodies[0].ContentHTML ||
		st.EditedAt == nil || !st.EditedAt.Equal(publishedAt.Add(time.Minute)) || st.Language != "und" {
		t.Errorf("status = %+v", st)
	}
	a := st.Account
	if a.Acct != "carol@remote.example" || a.Username != "carol" || a.Domain != "remote.example" ||
		a.Uri != "https://remote.example/users/carol" || a.Url != "https://remote.example/@carol" || a.Note != "<p>remote</p>" {
		t.Errorf("account = %+v, want carol's handle and profile page", a)
	}
	if m := st.Mentions; len(m) != 1 || m[0].Id != daveAccountID || m[0].Url != "https://other.example/ap/dave" {
		t.Errorf("mentions = %+v, want dave linked to his uri", m)
	}
	// One call reads the profile pages of the author and the mentions.
	if want := [][]string{{carolAccountID, daveAccountID}}; !slices.EqualFunc(profiles.calls, want, slices.Equal) {
		t.Errorf("RemoteProfileURLs calls = %v, want %v", profiles.calls, want)
	}
	if len(st.MediaAttachments) != 2 {
		t.Fatalf("media = %+v, want both remote files", st.MediaAttachments)
	}
	m := st.MediaAttachments[0]
	if m.Id != remoteMediaID || m.State != "ready" || m.Type != "image" || deref(m.Url) != proxyURL || deref(m.PreviewUrl) != proxyURL ||
		deref(m.RemoteUrl) != "https://remote.example/files/cat.png" || m.Variants != nil || deref(m.Description) != "a cat" ||
		deref(m.Blurhash) != "LEHV6nWB2yk8" || deref(m.Meta.Width) != 640 || deref(m.Meta.Height) != 480 ||
		m.Meta.Focus != (clientapi.MediaFocus{X: 0.5, Y: -0.5}) {
		t.Errorf("remote attachment = %+v", m)
	}
	// Without alt text and declared values, the attachment says so by leaving them out.
	if m := st.MediaAttachments[1]; m.Type != "unknown" || m.Description != nil || m.Blurhash != nil || m.Meta.Width != nil {
		t.Errorf("bare remote attachment = %+v", m)
	}
}

// proxied returns the image of a remote file served at its proxy URL in every size.
func proxied(u string) appcontent.Image {
	v := appcontent.ImageVariant{URL: u}
	return appcontent.Image{Still: appcontent.ImageVariants{Small: v, Medium: v, Large: v}}
}
