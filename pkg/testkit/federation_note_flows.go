// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"reflect"
	"slices"
	"sync"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/federation"
)

// contractAccountDomain is the domain of local handles in the federation flows, other than
// the serving host, so that mentions show which one they use.
const contractAccountDomain = "accounts.example"

// fedParts stands in for the content module's Reader.Parts: the mentioned accounts that exist,
// and the hashtags and processed media that a test registers per tag ID and revision ID.
type fedParts struct {
	mu       sync.Mutex
	accounts *fedAccounts
	tags     map[string]content.Tag
	media    map[string][]appcontent.MediaView
}

var _ appfederation.NoteParts = (*fedParts)(nil)

func (p *fedParts) Parts(ctx context.Context, rev appcontent.Revision) (appcontent.RevisionParts, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := appcontent.RevisionParts{Attachments: p.media[rev.ID]}
	for _, id := range rev.MentionedAccountIDs {
		if a, err := p.accounts.FindByID(ctx, id); err == nil {
			out.Mentions = append(out.Mentions, a)
		}
	}
	for _, id := range rev.TagIDs {
		out.Tags = append(out.Tags, p.tags[id])
	}
	return out, nil
}

// handle returns the handle of an account in the fake account module.
func (e *fedEnv) handle(id string) string {
	e.accounts.mu.Lock()
	defer e.accounts.mu.Unlock()
	h := e.accounts.actors[id].Handle
	domain := h.Domain
	if domain == "" {
		domain = contractAccountDomain
	}
	return "@" + h.Username + "@" + domain
}

// note returns the object of the stored document of the only activity of type typ about s.
func (e *fedEnv) note(t *testing.T, s content.Status, typ federation.ActivityType) map[string]any {
	t.Helper()
	doc := e.document(t, e.activity(t, federation.OutboundStatus, s.ID, typ).ID)
	note, ok := doc["object"].(map[string]any)
	if !ok || note["type"] != "Note" {
		t.Fatalf("%s document = %v, want a Note as its object", typ, doc)
	}
	return note
}

// tagsOf returns the type, href, and name of each entry of a Note's tag.
func tagsOf(note map[string]any) [][3]string {
	out := [][3]string{}
	list, _ := note["tag"].([]any)
	for _, x := range list {
		m, _ := x.(map[string]any)
		typ, _ := m["type"].(string)
		href, _ := m["href"].(string)
		name, _ := m["name"].(string)
		out = append(out, [3]string{typ, href, name})
	}
	return out
}

func fedBuildsTheNote(t *testing.T, e *fedEnv) {
	a := e.audience()
	tag := content.Tag{ID: e.ids.NewID(), Name: "Fediverse", NameNormalized: "fediverse"}
	e.parts.tags[tag.ID] = tag
	media := appcontent.MediaView{
		Media: appcontent.Media{
			Result: &content.MediaProcessingResult{
				Outcome: content.MediaReady, MediaType: content.MediaTypeImage, Width: 1280, Height: 960, Blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
			},
			Focus: &content.MediaFocus{X: 0.5, Y: -0.25},
		},
		Description: "a cat",
		Image: appcontent.Image{
			Still: appcontent.ImageVariants{
				Small:  appcontent.ImageVariant{URL: "https://files.social.example/media/r/small.webp", ContentType: "image/webp", Width: 160, Height: 120},
				Medium: appcontent.ImageVariant{URL: "https://files.social.example/media/r/medium.webp", ContentType: "image/webp", Width: 320, Height: 240},
				Large:  appcontent.ImageVariant{URL: "https://files.social.example/media/r/large.png", ContentType: "image/png", Width: 640, Height: 480},
			},
			Animated: &appcontent.ImageVariants{
				Small:  appcontent.ImageVariant{URL: "https://files.social.example/media/r/small-animated.webp", ContentType: "image/webp", Width: 160, Height: 120},
				Medium: appcontent.ImageVariant{URL: "https://files.social.example/media/r/medium-animated.webp", ContentType: "image/webp", Width: 320, Height: 240},
				Large:  appcontent.ImageVariant{URL: "https://files.social.example/media/r/large-animated.gif", ContentType: "image/gif", Width: 640, Height: 480},
			},
		},
	}
	s := e.postWith(t, a.author, content.VisibilityPublic, func(r *content.Revision) {
		r.Language, r.SpoilerText = "ja", "spoiler"
		r.Bodies = []content.Body{{Language: "en", ContentHTML: "<p>hello</p>"}, {Language: "ja", ContentHTML: "<p>こんにちは</p>"}}
		r.MentionedAccountIDs, r.TagIDs = []string{a.mentioned, a.localMentioned}, []string{tag.ID}
		r.Attachments = []content.Attachment{{MediaID: NamedID("m1"), Description: "a cat"}}
		e.parts.media[r.ID] = []appcontent.MediaView{media}
	})
	e.drain(t)

	note := e.note(t, s, federation.ActivityCreate)
	if note["content"] != "<p>こんにちは</p>" ||
		!reflect.DeepEqual(note["contentMap"], map[string]any{"en": "<p>hello</p>", "ja": "<p>こんにちは</p>"}) {
		t.Errorf("content = %v, contentMap = %v; want the revision's language and every language", note["content"], note["contentMap"])
	}
	// A local status's url is derived from its author's current username (ADR-0018).
	e.accounts.mu.Lock()
	wantURL := content.StatusURL(contractHost, e.accounts.actors[a.author].Local.Username, s.ID)
	e.accounts.mu.Unlock()
	if note["summary"] != "spoiler" || note["sensitive"] != true || note["published"] != federation.FormatTime(s.PublishedAt) || note["url"] != wantURL {
		t.Errorf("summary = %v, sensitive = %v, published = %v, url = %v", note["summary"], note["sensitive"], note["published"], note["url"])
	}
	if _, ok := note["updated"]; ok {
		t.Errorf("the Note of a Create has updated %v", note["updated"])
	}
	wantAttachment := []any{map[string]any{
		// An animation is sent as its large moving variant (ADR-0005 §8).
		"type": "Document", "mediaType": "image/gif", "url": media.Image.Animated.Large.URL, "name": "a cat",
		"blurhash": "LEHV6nWB2yk8pyo0adR*.7kCMdnj", "focalPoint": []any{0.5, -0.25}, "width": 640.0, "height": 480.0,
	}}
	if !reflect.DeepEqual(note["attachment"], wantAttachment) {
		t.Errorf("attachment = %v, want %v", note["attachment"], wantAttachment)
	}
	wantTags := [][3]string{
		{"Mention", e.uri(a.mentioned), e.handle(a.mentioned)},
		{"Mention", e.uri(a.localMentioned), e.handle(a.localMentioned)},
		{"Hashtag", "https://" + contractHost + "/tags/fediverse", "#Fediverse"},
	}
	if got := tagsOf(note); !slices.Equal(got, wantTags) {
		t.Errorf("tag = %v, want %v", got, wantTags)
	}

	e.reviseWith(t, s, func(r *content.Revision) {
		r.Bodies = []content.Body{{Language: "en", ContentHTML: "<p>edited</p>"}}
	})
	e.drain(t)
	update := e.note(t, s, federation.ActivityUpdate)
	if update["content"] != "<p>edited</p>" || !reflect.DeepEqual(update["contentMap"], map[string]any{"en": "<p>edited</p>"}) ||
		update["sensitive"] != false || update["summary"] != nil || update["updated"] != federation.FormatTime(e.clock.Now()) {
		t.Errorf("Update Note = %v, want the body of the new revision", update)
	}
	if got := update["attachment"]; !reflect.DeepEqual(got, []any{}) || len(tagsOf(update)) != 0 {
		t.Errorf("Update attachment = %v, tag = %v; want both empty", got, update["tag"])
	}
}

func fedLeavesNonFollowersOutOfLocalTags(t *testing.T, e *fedEnv) {
	a := e.audience()
	link := `<p><span class="h-card"><a href="` + e.uri(a.mentioned) + `" class="u-url mention">@<span>` + e.handle(a.mentioned) + `</span></a></span></p>`
	mentions := []string{a.mentioned, a.sharedA, a.localMentioned}
	mention := func(id string) [3]string { return [3]string{"Mention", e.uri(id), e.handle(id)} }
	post := func(v content.Visibility) content.Status {
		return e.postWith(t, a.author, v, func(r *content.Revision) {
			r.Bodies = []content.Body{{Language: "en", ContentHTML: link}}
			r.MentionedAccountIDs = mentions
		})
	}

	// ADR-0017: a local status names in tag the local accounts and the remote followers, never a
	// remote account that does not follow the author; the link in the content stays.
	local := post(content.VisibilityLocal)
	e.drain(t)
	note := e.note(t, local, federation.ActivityCreate)
	if got, want := tagsOf(note), [][3]string{mention(a.sharedA), mention(a.localMentioned)}; !slices.Equal(got, want) {
		t.Errorf("local tag = %v, want %v", got, want)
	}
	if note["content"] != link {
		t.Errorf("local content = %v, want the mention link kept: %s", note["content"], link)
	}
	if got := e.inboxes(t, e.activity(t, federation.OutboundStatus, local.ID, federation.ActivityCreate).ID); slices.Contains(got, "https://c.example/inbox") {
		t.Errorf("local inboxes = %v, want no inbox of the mentioned non-follower", got)
	}

	// Other visibilities name every mentioned account.
	public := post(content.VisibilityPublic)
	e.drain(t)
	all := [][3]string{mention(a.mentioned), mention(a.sharedA), mention(a.localMentioned)}
	if got := tagsOf(e.note(t, public, federation.ActivityCreate)); !slices.Equal(got, all) {
		t.Errorf("public tag = %v, want %v", got, all)
	}

	// The Update follows the same rule as of its time: a mention that has followed since is in.
	e.follow(a.mentioned, a.author)
	e.reviseWith(t, local, func(r *content.Revision) {
		r.Bodies = []content.Body{{Language: "en", ContentHTML: link}}
		r.MentionedAccountIDs = mentions
	})
	e.drain(t)
	if got := tagsOf(e.note(t, local, federation.ActivityUpdate)); !slices.Equal(got, all) {
		t.Errorf("Update tag after the mention followed = %v, want %v", got, all)
	}
}

func fedDeliversTheStoredDelete(t *testing.T, e *fedEnv) {
	a := e.audience()
	s := e.post(t, a.author, content.VisibilityPublic, a.mentioned)
	e.drain(t)
	created := e.inboxes(t, e.activity(t, federation.OutboundStatus, s.ID, federation.ActivityCreate).ID)

	e.delete(t, s)
	del := e.activity(t, federation.OutboundStatus, s.ID, federation.ActivityDelete)
	// The deletion's transaction stored the document, before any fan-out ran.
	stored, err := e.h.Federation.FindOutboundDocument(t.Context(), del.ID)
	if err != nil {
		t.Fatalf("the Delete has no document after the deletion: %v", err)
	}
	// The purge removes the status's rows before the fan-out runs.
	e.statuses.mu.Lock()
	delete(e.statuses.statuses, s.ID)
	for id, r := range e.statuses.revisions {
		if r.StatusID == s.ID {
			delete(e.statuses.revisions, id)
		}
	}
	e.statuses.mu.Unlock()
	e.drain(t)

	if got := e.inboxes(t, del.ID); !slices.Equal(got, created) {
		t.Errorf("Delete inboxes = %v, want those of the Create %v", got, created)
	}
	after, err := e.h.Federation.FindOutboundDocument(t.Context(), del.ID)
	if err != nil || string(after) != string(stored) {
		t.Errorf("document after the fan-out = %s, %v; want the stored %s", after, err, stored)
	}
	doc := e.document(t, del.ID)
	obj, _ := doc["object"].(map[string]any)
	actor := e.uri(a.author)
	if doc["type"] != "Delete" || doc["id"] != string(del.ActivityURI) || doc["actor"] != actor ||
		obj["type"] != "Tombstone" || obj["id"] != s.URI || doc["@context"] == nil {
		t.Errorf("Delete document = %v, want the author's Delete of a Tombstone", doc)
	}
	if !slices.Equal(strs(doc["to"]), []string{string(federation.PublicIRI)}) || !slices.Equal(strs(doc["cc"]), []string{actor + "/followers"}) {
		t.Errorf("Delete to = %v, cc = %v; want the public status's addressing", doc["to"], doc["cc"])
	}
}

// fedDeliversAutoDelete checks that the automatic deletion of an old local post (FR-118) sends its
// Delete to the inboxes of the Create, as its author's deletion does.
func fedDeliversAutoDelete(t *testing.T, e *fedEnv) {
	a := e.audience()
	s := e.post(t, a.author, content.VisibilityPublic, a.mentioned)
	e.drain(t)
	created := e.inboxes(t, e.activity(t, federation.OutboundStatus, s.ID, federation.ActivityCreate).ID)

	e.deleteBy(t, s, content.DeletedByAutoDelete)
	e.drain(t)
	del := e.activity(t, federation.OutboundStatus, s.ID, federation.ActivityDelete)
	if got := e.inboxes(t, del.ID); !slices.Equal(got, created) {
		t.Errorf("Delete inboxes = %v, want those of the Create %v", got, created)
	}
	doc := e.document(t, del.ID)
	obj, _ := doc["object"].(map[string]any)
	if doc["type"] != "Delete" || doc["actor"] != e.uri(a.author) || obj["type"] != "Tombstone" || obj["id"] != s.URI {
		t.Errorf("Delete document = %v, want the author's Delete of a Tombstone", doc)
	}
}

// fedSendsVideoAndAudio checks the attachments of video and audio files (ADR-0005 §8): the
// file as url with its media type, the poster's large variant as icon when there is one, and
// the length as duration.
func fedSendsVideoAndAudio(t *testing.T, e *fedEnv) {
	a := e.audience()
	poster := appcontent.ImageVariants{
		Small:  appcontent.ImageVariant{URL: "https://files.social.example/media/v/small.webp", ContentType: "image/webp", Width: 160, Height: 90},
		Medium: appcontent.ImageVariant{URL: "https://files.social.example/media/v/medium.webp", ContentType: "image/webp", Width: 320, Height: 180},
		Large:  appcontent.ImageVariant{URL: "https://files.social.example/media/v/large.jpg", ContentType: "image/jpeg", Width: 640, Height: 360},
	}
	video := appcontent.MediaView{
		Media: appcontent.Media{Result: &content.MediaProcessingResult{
			Outcome: content.MediaReady, MediaType: content.MediaTypeVideo, Width: 640, Height: 360, DurationMS: 12500, Blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
		}},
		Description: "a cat jumps",
		Image: appcontent.Image{Still: poster, Media: &appcontent.ImageVariant{
			URL: "https://files.social.example/media/v/large-animated.mp4", ContentType: "video/mp4", Width: 640, Height: 360,
		}},
	}
	audio := appcontent.MediaView{
		Media: appcontent.Media{Result: &content.MediaProcessingResult{Outcome: content.MediaReady, MediaType: content.MediaTypeAudio, DurationMS: 61000}},
		Image: appcontent.Image{Media: &appcontent.ImageVariant{URL: "https://files.social.example/media/a/large-animated.ogg", ContentType: "audio/ogg"}},
	}
	s := e.postWith(t, a.author, content.VisibilityPublic, func(r *content.Revision) {
		r.Attachments = []content.Attachment{{MediaID: NamedID("v1"), Description: "a cat jumps"}, {Position: 1, MediaID: NamedID("a1")}}
		e.parts.media[r.ID] = []appcontent.MediaView{video, audio}
	})
	e.drain(t)
	want := []any{
		map[string]any{
			"type": "Document", "mediaType": "video/mp4", "url": video.Image.Media.URL, "name": "a cat jumps",
			"blurhash": "LEHV6nWB2yk8pyo0adR*.7kCMdnj", "width": 640.0, "height": 360.0, "duration": "PT12.5S",
			"icon": map[string]any{"type": "Image", "mediaType": "image/jpeg", "url": poster.Large.URL},
		},
		map[string]any{"type": "Document", "mediaType": "audio/ogg", "url": audio.Image.Media.URL, "duration": "PT61S"},
	}
	if got := e.note(t, s, federation.ActivityCreate)["attachment"]; !reflect.DeepEqual(got, want) {
		t.Errorf("attachment = %v, want %v", got, want)
	}
}
