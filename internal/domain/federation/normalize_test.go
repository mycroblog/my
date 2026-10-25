// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/federation"
)

var now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func normalize(t *testing.T, doc string) *federation.Document {
	t.Helper()
	d, err := federation.Normalize([]byte(doc), now, lowerHost)
	if err != nil {
		t.Fatalf("Normalize: %v\n%s", err, doc)
	}
	return d
}

func object(t *testing.T, doc string) *federation.Object {
	t.Helper()
	d := normalize(t, doc)
	if d.Object == nil {
		t.Fatalf("Normalize returned %+v, want an object", d)
	}
	return d.Object
}

func activity(t *testing.T, doc string) *federation.Activity {
	t.Helper()
	d := normalize(t, doc)
	if d.Activity == nil {
		t.Fatalf("Normalize returned %+v, want an activity", d)
	}
	return d.Activity
}

func equal[T any](t *testing.T, what string, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", what, got, want)
	}
}

const ctx = `"@context": ["https://www.w3.org/ns/activitystreams", {"toot": "http://joinmastodon.org/ns#"}]`

// Properties that may be arrays are read the same whether single, an array, or null.
func TestNormalizeSingleAndArray(t *testing.T) {
	cases := []struct {
		name, doc string
		to, cc    []federation.IRI
		tags      []string
		typ       federation.ObjectType
	}{
		{"single values", `{"type":"Note","id":"https://a.example/n/1","to":"https://www.w3.org/ns/activitystreams#Public","cc":"https://a.example/u/alice/followers","tag":{"type":"Hashtag","name":"#go"}}`,
			[]federation.IRI{federation.PublicIRI}, []federation.IRI{"https://a.example/u/alice/followers"}, []string{"go"}, federation.ObjectNote},
		{"arrays", `{"type":["Note"],"id":"https://a.example/n/1","to":["https://www.w3.org/ns/activitystreams#Public"],"cc":["https://a.example/u/alice/followers","https://a.example/u/alice/followers"],"tag":[{"type":"Hashtag","name":"#go"}]}`,
			[]federation.IRI{federation.PublicIRI}, []federation.IRI{"https://a.example/u/alice/followers"}, []string{"go"}, federation.ObjectNote},
		{"nulls", `{"type":"Note","id":"https://a.example/n/1","to":null,"cc":null,"tag":null}`, nil, nil, nil, federation.ObjectNote},
		{"type array picks the first known type", `{"type":["Hashtag","Question","Note"],"id":"https://a.example/n/1"}`, nil, nil, nil, federation.ObjectQuestion},
		{"recipient as a Link object", `{"type":"Note","id":"https://a.example/n/1","to":[{"type":"Link","href":"https://b.example/u/bob"}]}`,
			[]federation.IRI{"https://b.example/u/bob"}, nil, nil, federation.ObjectNote},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := object(t, c.doc)
			equal(t, "To", o.Audience.To, c.to)
			equal(t, "Cc", o.Audience.Cc, c.cc)
			equal(t, "Hashtags", o.Hashtags, c.tags)
			equal(t, "Type", o.Type, c.typ)
		})
	}
}

// object, attributedTo, actor, and inReplyTo may be an IRI, a Link, or an embedded object.
func TestNormalizeReferences(t *testing.T) {
	cases := []struct {
		name, ref string
		want      federation.IRI
		embedded  bool
		trusted   bool
	}{
		{"IRI", `"https://a.example/n/1"`, "https://a.example/n/1", false, false},
		{"IRI with upper-case host", `"HTTPS://A.Example/n/1"`, "https://a.example/n/1", false, false},
		{"Link", `{"type":"Link","href":"https://a.example/n/1"}`, "https://a.example/n/1", false, false},
		{"untyped object with id", `{"id":"https://a.example/n/1"}`, "https://a.example/n/1", false, false},
		{"embedded, same origin", `{"type":"Note","id":"https://a.example/n/1","content":"hi"}`, "https://a.example/n/1", true, true},
		{"embedded, other origin", `{"type":"Note","id":"https://evil.example/n/1","content":"hi"}`, "https://evil.example/n/1", true, false},
		{"embedded without id", `{"type":"Note","content":"hi"}`, "", true, true},
		{"not an IRI", `"not a url"`, "", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := activity(t, `{`+ctx+`,"type":"Announce","id":"https://a.example/act/1","actor":{"type":"Person","id":"https://a.example/u/alice"},"object":`+c.ref+`}`)
			equal(t, "Actor", a.Actor, federation.IRI("https://a.example/u/alice"))
			if c.want == "" && !c.embedded {
				equal(t, "Objects", len(a.Objects), 0)
				return
			}
			r := a.Object()
			equal(t, "Object().ID", r.ID, c.want)
			equal(t, "embedded", r.Untrusted() != nil, c.embedded)
			equal(t, "trusted", r.Trusted() != nil, c.trusted)
			if c.trusted && r.Trusted().Object.Bodies[0].HTML != "hi" {
				t.Errorf("embedded body = %+v", r.Trusted().Object.Bodies)
			}
		})
	}

	o := object(t, `{"type":"Note","id":"https://a.example/n/2","attributedTo":[{"type":"Person","id":"https://a.example/u/alice"}],"inReplyTo":{"type":"Link","href":"https://b.example/n/9"}}`)
	equal(t, "AttributedTo", o.AttributedTo, federation.IRI("https://a.example/u/alice"))
	equal(t, "InReplyTo", o.InReplyTo, federation.IRI("https://b.example/n/9"))

	// Flag carries several objects; Undo embeds an activity.
	f := activity(t, `{"type":"Flag","actor":"https://a.example/actor","object":["https://me.example/u/bob","https://me.example/s/1"],"content":"<p>spam &amp; abuse</p>"}`)
	equal(t, "Flag objects", []federation.IRI{f.Objects[0].ID, f.Objects[1].ID}, []federation.IRI{"https://me.example/u/bob", "https://me.example/s/1"})
	equal(t, "Flag content", f.Content, "spam & abuse")
	u := activity(t, `{"type":"Undo","id":"https://a.example/u/alice#undo","actor":"https://a.example/u/alice","object":{"type":"Follow","id":"https://a.example/f/1","actor":"https://a.example/u/alice","object":"https://me.example/u/bob"}}`)
	follow := u.Object().Trusted().Activity
	equal(t, "embedded Follow", [3]any{follow.Type, follow.Actor, follow.Object().ID}, [3]any{federation.ActivityFollow, federation.IRI("https://a.example/u/alice"), federation.IRI("https://me.example/u/bob")})
}

func TestNormalizePublicSpellings(t *testing.T) {
	for _, p := range []string{"https://www.w3.org/ns/activitystreams#Public", "as:Public", "Public"} {
		t.Run(p, func(t *testing.T) {
			o := object(t, fmt.Sprintf(`{"type":"Note","id":"https://a.example/n/1","to":[%q],"cc":[%q]}`, p, p))
			equal(t, "To", o.Audience.To, []federation.IRI{federation.PublicIRI})
			equal(t, "Cc", o.Audience.Cc, []federation.IRI{federation.PublicIRI})
		})
	}
}

func TestAudienceVisibility(t *testing.T) {
	const followers = federation.IRI("https://a.example/u/alice/followers")
	bob := federation.IRI("https://b.example/u/bob")
	cases := []struct {
		name      string
		to, cc    []federation.IRI
		followers federation.IRI
		want      federation.Visibility
	}{
		{"Public in to", []federation.IRI{federation.PublicIRI}, []federation.IRI{followers}, followers, federation.VisibilityPublic},
		{"Public in cc", []federation.IRI{followers}, []federation.IRI{federation.PublicIRI}, followers, federation.VisibilityUnlisted},
		{"followers in to", []federation.IRI{followers}, nil, followers, federation.VisibilityPrivate},
		{"followers in cc", []federation.IRI{bob}, []federation.IRI{followers}, followers, federation.VisibilityPrivate},
		{"mentions only", []federation.IRI{bob}, nil, followers, federation.VisibilityDirect},
		{"unknown followers collection", []federation.IRI{followers}, nil, "", federation.VisibilityDirect},
		{"nobody", nil, nil, followers, federation.VisibilityDirect},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := federation.Audience{To: c.to, Cc: c.cc}
			equal(t, "Visibility", a.Visibility(c.followers), c.want)
		})
	}
}

func TestMentionedIRIs(t *testing.T) {
	o := object(t, `{"type":"Note","id":"https://a.example/n/1",
		"to":["https://me.example/u/carol","https://b.example/u/dave"],"cc":["https://a.example/u/alice/followers","as:Public"],
		"tag":[{"type":"Mention","href":"https://b.example/u/bob","name":"@bob@b.example"},{"type":"Mention","href":"https://me.example/u/carol"}]}`)
	isLocal := func(i federation.IRI) bool { return i.Host() == "me.example" }
	equal(t, "MentionedIRIs", o.MentionedIRIs("https://a.example/u/alice/followers", isLocal),
		[]federation.IRI{"https://b.example/u/bob", "https://me.example/u/carol"})
	o2 := object(t, `{"type":"Note","id":"https://a.example/n/2","to":["https://me.example/u/carol"]}`)
	equal(t, "addressed local actor", o2.MentionedIRIs("", isLocal), []federation.IRI{"https://me.example/u/carol"})
}

func TestNormalizeBodies(t *testing.T) {
	cases := []struct {
		name, props string
		want        []federation.Body
	}{
		{"content only", `"content":"<p>hi</p>"`, []federation.Body{{Language: "und", HTML: "<p>hi</p>"}}},
		{"contentMap keeps document order", `"content":"<p>hi</p>","contentMap":{"ja":"<p>やあ</p>","en":"<p>hi</p>"}`,
			[]federation.Body{{Language: "ja", HTML: "<p>やあ</p>"}, {Language: "en", HTML: "<p>hi</p>"}}},
		{"language tags are canonicalized", `"contentMap":{"zh_hant_tw":"a","EN-us":"b"}`,
			[]federation.Body{{Language: "zh-Hant-TW", HTML: "a"}, {Language: "en-US", HTML: "b"}}},
		{"malformed tag becomes und", `"contentMap":{"!!":"a"}`, []federation.Body{{Language: "und", HTML: "a"}}},
		{"duplicate language keeps the first", `"contentMap":{"en":"a","EN":"b"}`, []federation.Body{{Language: "en", HTML: "a"}}},
		{"empty contentMap falls back to content", `"content":"x","contentMap":{}`, []federation.Body{{Language: "und", HTML: "x"}}},
		{"no body", `"content":null`, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := object(t, `{"type":"Note","id":"https://a.example/n/1",`+c.props+`}`)
			equal(t, "Bodies", o.Bodies, c.want)
		})
	}
}

func TestNormalizeSpoilerAndSensitive(t *testing.T) {
	o := object(t, `{"type":"Note","id":"https://a.example/n/1","summary":"<p>spoiler &lt;b&gt;</p>","sensitive":true}`)
	equal(t, "SpoilerText", o.SpoilerText, "spoiler <b>")
	equal(t, "Sensitive", o.Sensitive, true)
	o = object(t, `{"type":"Note","id":"https://a.example/n/1","summaryMap":{"ja":"ネタバレ"}}`)
	equal(t, "SpoilerText from summaryMap", o.SpoilerText, "ネタバレ")
}

func TestNormalizeConvertedTypes(t *testing.T) {
	for _, typ := range []string{"Article", "Page", "Event", "Image", "Video", "Audio"} {
		t.Run(typ, func(t *testing.T) {
			o := object(t, `{"type":"`+typ+`","id":"https://a.example/p/1","name":"Title <&>","summary":"<p>Line 1<br>Line 2</p>",
				"content":"<h1>long article</h1>","url":"https://a.example/p/1.html","contentMap":{"ja":"<h1>長い記事</h1>"}}`)
			equal(t, "Converted", o.Converted, true)
			equal(t, "Type", string(o.Type), typ)
			equal(t, "SpoilerText", o.SpoilerText, "")
			equal(t, "Bodies", o.Bodies, []federation.Body{{Language: "ja",
				HTML: `<p><strong>Title &lt;&amp;&gt;</strong></p><p>Line 1<br>Line 2</p><p><a href="https://a.example/p/1.html">https://a.example/p/1.html</a></p>`}})
		})
	}
	o := object(t, `{"type":"Page","id":"https://a.example/p/2","name":"T"}`)
	equal(t, "link falls back to id", o.Bodies[0].HTML, `<p><strong>T</strong></p><p><a href="https://a.example/p/2">https://a.example/p/2</a></p>`)
	equal(t, "Note is not converted", object(t, `{"type":"Note","id":"https://a.example/n/1"}`).Converted, false)
	o = object(t, `{"type":"Page","id":"https://a.example/p/3","name":"T","url":"javascript://a.example/%0Aalert(1)"}`)
	equal(t, "a url that is not http(s) falls back to id", o.Bodies[0].HTML,
		`<p><strong>T</strong></p><p><a href="https://a.example/p/3">https://a.example/p/3</a></p>`)
	o = object(t, `{"type":"Page","id":"ftp://a.example/p/4","name":"T","url":"data://a.example/x"}`)
	equal(t, "no link without an http(s) URL", o.Bodies[0].HTML, `<p><strong>T</strong></p>`)
	o = object(t, `{"type":"Page","id":"https://a.example/p/5","name":"T","url":"https://a.example/p?q=\"><script>&'"}`)
	equal(t, "the link is escaped for the attribute", o.Bodies[0].HTML,
		`<p><strong>T</strong></p><p><a href="https://a.example/p?q=&#34;&gt;&lt;script&gt;&amp;&#39;">https://a.example/p?q=&#34;&gt;&lt;script&gt;&amp;&#39;</a></p>`)
}

func TestNormalizeTags(t *testing.T) {
	o := object(t, `{"type":"Note","id":"https://a.example/n/1","tag":[
		{"type":"Mention","href":"https://b.example/u/bob","name":"@bob@b.example"},
		{"type":"Hashtag","href":"https://a.example/tags/go","name":"#Go"},
		{"type":"Hashtag","name":"#Go"},
		{"type":"Emoji","id":"https://a.example/emoji/1","name":":blob:","icon":{"type":"Image","url":"https://a.example/e/blob.png"}},
		{"type":"Emoji","name":":broken:"},
		{"type":"Emoji","name":":blob:","icon":{"type":"Image","url":"https://a.example/e/blob2.png"}},
		{"type":"Emoji","name":":`+strings.Repeat("a", federation.MaxShortcode+1)+`:","icon":{"type":"Image","url":"https://a.example/e/long.png"}},
		{"type":"Unknown","name":"x"}, "https://a.example/not-an-object"]}`)
	equal(t, "Mentions", o.Mentions, []federation.Mention{{Href: "https://b.example/u/bob", Name: "@bob@b.example"}})
	equal(t, "Hashtags", o.Hashtags, []string{"Go"})
	equal(t, "Emojis", o.Emojis, []federation.Emoji{{ID: "https://a.example/emoji/1", Shortcode: "blob", IconURL: "https://a.example/e/blob.png"}})
}

func TestNormalizeAttachmentsAndURL(t *testing.T) {
	o := object(t, `{"type":"Note","id":"https://a.example/n/1",
		"url":[{"type":"Link","mediaType":"application/activity+json","href":"https://a.example/n/1.json"},{"type":"Link","mediaType":"text/html","href":"https://a.example/@alice/1"}],
		"attachment":[
		{"type":"Document","mediaType":"image/png","url":"https://a.example/m/1.png","name":"alt","blurhash":"UABC","focalPoint":[0.5,-0.25],"width":640,"height":480},
		{"type":"Link","href":"https://a.example/link"},
		{"type":"PropertyValue","name":"x","value":"y"},
		{"type":"Image","url":[{"type":"Link","href":"https://a.example/m/2.jpg","mediaType":"image/jpeg"}]},
		{"type":"Video","url":"https://a.example/m/3.mp4"},
		{"type":"Audio","url":"https://a.example/m/4.mp3"},
		{"type":"Document","url":"https://a.example/m/5.png"}]}`)
	equal(t, "URL", o.URL, federation.IRI("https://a.example/@alice/1"))
	equal(t, "len(Attachments)", len(o.Attachments), federation.MaxAttachments)
	equal(t, "Attachments[0]", o.Attachments[0], federation.Attachment{Type: "Document", URL: "https://a.example/m/1.png", MediaType: "image/png",
		Name: "alt", Blurhash: "UABC", FocalPoint: &[2]float64{0.5, -0.25}, Width: 640, Height: 480})
	equal(t, "Attachments[1]", o.Attachments[1], federation.Attachment{Type: "Image", URL: "https://a.example/m/2.jpg", MediaType: "image/jpeg"})
	equal(t, "Attachments[3].URL", o.Attachments[3].URL, federation.IRI("https://a.example/m/4.mp3"))
	equal(t, "string url", object(t, `{"type":"Note","id":"https://a.example/n/1","url":"https://a.example/@a/1"}`).URL, federation.IRI("https://a.example/@a/1"))
}

func TestNormalizeTimes(t *testing.T) {
	o := object(t, `{"type":"Note","id":"https://a.example/n/1","published":"2026-09-30T01:02:03.456+09:00","updated":"2030-01-01T00:00:00Z"}`)
	equal(t, "Published", o.Published, time.Date(2026, 9, 29, 16, 2, 3, 456000000, time.UTC))
	equal(t, "Updated (future clamps to now)", o.Updated, now)
	o = object(t, `{"type":"Note","id":"https://a.example/n/1","published":"yesterday"}`)
	equal(t, "malformed Published", o.Published.IsZero(), true)
}

func TestNormalizePoll(t *testing.T) {
	o := object(t, `{"type":"Question","id":"https://a.example/q/1","endTime":"2026-10-01T00:00:00Z","votersCount":7,
		"anyOf":[{"type":"Note","name":"A","replies":{"type":"Collection","totalItems":5}},{"type":"Note","name":"B","replies":{"totalItems":3}}]}`)
	equal(t, "Poll", *o.Poll, federation.Poll{Multiple: true, EndTime: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), VotersCount: 7,
		Options: []federation.PollOption{{Name: "A", Votes: 5}, {Name: "B", Votes: 3}}})
	o = object(t, `{"type":"Question","id":"https://a.example/q/2","oneOf":[{"name":"X"}],"endTime":"2030-01-01T00:00:00Z"}`)
	equal(t, "single choice", o.Poll.Multiple, false)
	equal(t, "EndTime (future, not clamped)", o.Poll.EndTime, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC))
	equal(t, "Note has no poll", object(t, `{"type":"Note","id":"https://a.example/n/1"}`).Poll == nil, true)
}

func TestNormalizeActor(t *testing.T) {
	d := normalize(t, `{`+ctx+`,"type":"Service","id":"https://a.example/u/alice","preferredUsername":"alice","name":"Alice :blob:",
		"summary":"<p>bio</p>","url":"https://a.example/@alice","inbox":"https://a.example/u/alice/inbox","outbox":"https://a.example/u/alice/outbox",
		"followers":"https://a.example/u/alice/followers","following":"https://a.example/u/alice/following","featured":"https://a.example/u/alice/featured",
		"endpoints":{"sharedInbox":"https://a.example/inbox"},"icon":{"type":"Image","url":"https://a.example/a.png"},"image":"https://a.example/h.png",
		"manuallyApprovesFollowers":true,"discoverable":true,"indexable":true,"published":"2020-01-01T00:00:00Z","isCat":true,
		"alsoKnownAs":["https://old.example/users/alice","https://a.example/u/alice"],"movedTo":"https://new.example/users/alice",
		"publicKey":{"id":"https://a.example/u/alice#main-key","owner":"https://a.example/u/alice","publicKeyPem":"-----BEGIN PUBLIC KEY-----"},
		"attachment":[{"type":"PropertyValue","name":"Site","value":"<a href=\"https://alice.example\">alice.example</a>"}],
		"tag":[{"type":"Emoji","name":":blob:","icon":{"url":"https://a.example/e.png"}}]}`)
	a := d.Actor
	if a == nil {
		t.Fatalf("Normalize returned %+v, want an actor", d)
	}
	equal(t, "Actor", *a, federation.Actor{
		ID: "https://a.example/u/alice", Type: federation.ActorService, PreferredUsername: "alice", Name: "Alice :blob:", Summary: "<p>bio</p>",
		URL: "https://a.example/@alice", Inbox: "https://a.example/u/alice/inbox", Outbox: "https://a.example/u/alice/outbox",
		SharedInbox: "https://a.example/inbox", Followers: "https://a.example/u/alice/followers", Following: "https://a.example/u/alice/following",
		Featured: "https://a.example/u/alice/featured", Icon: "https://a.example/a.png", Image: "https://a.example/h.png",
		ManuallyApprovesFollowers: true, Discoverable: true, Indexable: true, Published: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		AlsoKnownAs: []federation.IRI{"https://old.example/users/alice", "https://a.example/u/alice"}, MovedTo: "https://new.example/users/alice",
		PublicKeys: []federation.PublicKey{{ID: "https://a.example/u/alice#main-key", Owner: "https://a.example/u/alice", PEM: "-----BEGIN PUBLIC KEY-----"}},
		Fields:     []federation.Field{{Name: "Site", Value: `<a href="https://alice.example">alice.example</a>`}},
		Emojis:     []federation.Emoji{{Shortcode: "blob", IconURL: "https://a.example/e.png"}},
	})
	equal(t, "Bot", a.Bot(), true)
}

// Dialects of Misskey and Akkoma / Pleroma.
func TestNormalizeDialects(t *testing.T) {
	t.Run("Misskey note", func(t *testing.T) {
		o := object(t, `{"@context":["https://www.w3.org/ns/activitystreams","https://w3id.org/security/v1",{"misskey":"https://misskey-hub.net/ns#","_misskey_content":"misskey:_misskey_content","isCat":"misskey:isCat"}],
			"id":"https://mk.example/notes/9abc","type":"Note","attributedTo":"https://mk.example/users/9xyz",
			"content":"<p><span>hello $[tada world]</span></p>","_misskey_content":"hello $[tada world]","source":{"content":"hello $[tada world]","mediaType":"text/x.misskeymarkdown"},
			"_misskey_summary":"mfm cw","summary":"cw","_misskey_quote":"https://b.example/n/1","quoteUrl":"https://b.example/n/1","quoteUri":"https://b.example/n/1",
			"published":"2026-09-01T00:00:00.000Z","to":["https://www.w3.org/ns/activitystreams#Public"],"cc":["https://mk.example/users/9xyz/followers"],
			"inReplyTo":null,"attachment":[],"sensitive":false,"tag":[{"type":"Hashtag","href":"https://mk.example/tags/misskey","name":"#misskey"}]}`)
		equal(t, "Bodies", o.Bodies, []federation.Body{{Language: "und", HTML: "<p><span>hello $[tada world]</span></p>"}})
		equal(t, "SpoilerText", o.SpoilerText, "cw")
		equal(t, "Hashtags", o.Hashtags, []string{"misskey"})
		equal(t, "Visibility", o.Audience.Visibility("https://mk.example/users/9xyz/followers"), federation.VisibilityPublic)
	})
	t.Run("Misskey reaction", func(t *testing.T) {
		a := activity(t, `{"type":"Like","id":"https://mk.example/likes/1","actor":"https://mk.example/users/9xyz","object":"https://me.example/s/1",
			"content":":blobcat:","_misskey_reaction":":blobcat:","tag":[{"id":"https://mk.example/emojis/blobcat","type":"Emoji","name":":blobcat:","icon":{"type":"Image","mediaType":"image/png","url":"https://mk.example/files/blobcat.png"}}]}`)
		equal(t, "Reaction", a.Reaction, ":blobcat:")
		equal(t, "Emojis", a.Emojis, []federation.Emoji{{ID: "https://mk.example/emojis/blobcat", Shortcode: "blobcat", IconURL: "https://mk.example/files/blobcat.png"}})
		equal(t, "plain Like has no reaction", activity(t, `{"type":"Like","actor":"https://a.example/u/a","object":"https://me.example/s/1"}`).Reaction, "")
	})
	t.Run("Akkoma EmojiReact", func(t *testing.T) {
		a := activity(t, `{"type":"EmojiReact","id":"https://ak.example/activities/1","actor":"https://ak.example/users/a","object":"https://me.example/s/1","content":"🍣",
			"to":"https://me.example/u/bob","cc":"https://www.w3.org/ns/activitystreams#Public","context":"https://ak.example/contexts/1"}`)
		equal(t, "Type", a.Type, federation.ActivityEmojiReact)
		equal(t, "Reaction", a.Reaction, "🍣")
		equal(t, "To", a.Audience.To, []federation.IRI{"https://me.example/u/bob"})
	})
	t.Run("Akkoma attachment with a url array", func(t *testing.T) {
		o := object(t, `{"type":"Note","id":"https://ak.example/objects/1","actor":"https://ak.example/users/a","attributedTo":"https://ak.example/users/a",
			"to":["as:Public"],"attachment":[{"type":"Document","name":"","mediaType":"image/webp","url":[{"href":"https://ak.example/media/1.webp","mediaType":"image/webp","type":"Link"}]}]}`)
		equal(t, "Attachments", o.Attachments, []federation.Attachment{{Type: "Document", URL: "https://ak.example/media/1.webp", MediaType: "image/webp"}})
		equal(t, "Visibility", o.Audience.Visibility(""), federation.VisibilityPublic)
	})
}

// The spellings of a quote (ADR-0069 §2): FEP-044f's quote comes first, then the dialects, then
// an FEP-e232 Link.
func TestNormalizeQuote(t *testing.T) {
	for _, tc := range []struct {
		name, props string
		want        *federation.Quote
	}{
		{"none", ``, nil},
		{"FEP-044f with its dialect aliases", `"quote":"https://b.example/n/1","_misskey_quote":"https://b.example/n/2","quoteUri":"https://b.example/n/2",
			"quoteAuthorization":"https://b.example/u/b/quote_authorizations/1"`,
			&federation.Quote{Target: "https://b.example/n/1", Consent: true, Authorization: "https://b.example/u/b/quote_authorizations/1"}},
		{"FEP-044f without an authorization", `"quote":"https://b.example/n/1"`, &federation.Quote{Target: "https://b.example/n/1", Consent: true}},
		{"Misskey", `"_misskey_quote":"https://b.example/n/1","quoteUrl":"https://b.example/n/1","quoteAuthorization":"https://b.example/q/1"`,
			&federation.Quote{Target: "https://b.example/n/1"}},
		{"Akkoma quoteUrl", `"quoteUrl":"https://b.example/n/1"`, &federation.Quote{Target: "https://b.example/n/1"}},
		{"quoteURL", `"quoteURL":"https://b.example/n/1"`, &federation.Quote{Target: "https://b.example/n/1"}},
		{"Fedibird quoteUri", `"quoteUri":"https://b.example/n/1"`, &federation.Quote{Target: "https://b.example/n/1"}},
		{"invalid IRI falls through", `"quote":"not an iri","quoteUrl":"https://b.example/n/1"`, &federation.Quote{Target: "https://b.example/n/1"}},
		{"FEP-e232 Link", `"tag":[{"type":"Link","mediaType":"application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"",
			"rel":"https://misskey-hub.net/ns#_misskey_quote","href":"https://b.example/n/1","name":"RE: https://b.example/n/1"}]`,
			&federation.Quote{Target: "https://b.example/n/1"}},
		{"FEP-e232 Link with FEP-044f's rel spelling", `"tag":[{"type":"Link","mediaType":"application/activity+json",
			"rel":["https://misskey-hub.net/ns/#_misskey_quote"],"href":"https://b.example/n/1"}]`, &federation.Quote{Target: "https://b.example/n/1"}},
		{"FEP-e232 Link without the quote rel", `"tag":[{"type":"Link","mediaType":"application/activity+json","href":"https://b.example/n/1"}]`, nil},
		{"Link to a web page", `"tag":[{"type":"Link","mediaType":"text/html","rel":"https://misskey-hub.net/ns#_misskey_quote","href":"https://b.example/n/1"}]`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			props := tc.props
			if props != "" {
				props = "," + props
			}
			o := object(t, `{"type":"Note","id":"https://a.example/n/1","attributedTo":"https://a.example/u/a"`+props+`}`)
			equal(t, "Quote", o.Quote, tc.want)
		})
	}
	t.Run("QuoteAuthorization", func(t *testing.T) {
		o := object(t, `{"@context":["https://www.w3.org/ns/activitystreams",{"QuoteAuthorization":"https://w3id.org/fep/044f#QuoteAuthorization",
			"gts":"https://gotosocial.org/ns#","interactingObject":{"@id":"gts:interactingObject","@type":"@id"},"interactionTarget":{"@id":"gts:interactionTarget","@type":"@id"}}],
			"type":"QuoteAuthorization","id":"https://b.example/u/b/quote_authorizations/1","attributedTo":"https://b.example/u/b",
			"interactingObject":"https://a.example/n/1","interactionTarget":"https://b.example/n/1"}`)
		equal(t, "Type", o.Type, federation.ObjectQuoteAuthorization)
		equal(t, "AttributedTo", o.AttributedTo, federation.IRI("https://b.example/u/b"))
		equal(t, "InteractingObject", o.InteractingObject, federation.IRI("https://a.example/n/1"))
		equal(t, "InteractionTarget", o.InteractionTarget, federation.IRI("https://b.example/n/1"))
	})
}

func TestNormalizeUnknown(t *testing.T) {
	a := activity(t, `{"type":"Arrive","actor":"https://a.example/u/a","object":"https://a.example/place"}`)
	equal(t, "unknown activity", [2]any{a.Type, a.RawType}, [2]any{federation.ActivityUnknown, "Arrive"})
	o := object(t, `{"type":"Place","id":"https://a.example/c"}`)
	equal(t, "unknown object", [2]any{o.Type, o.RawType}, [2]any{federation.ObjectUnknown, "Place"})
	m := activity(t, `{"type":"Move","actor":"https://a.example/u/a","object":"https://a.example/u/a","target":"https://b.example/u/a"}`)
	equal(t, "Move target", m.Target, federation.IRI("https://b.example/u/a"))
	d := normalize(t, `{"type":"Tombstone","id":"https://a.example/n/1","signature":{"type":"RsaSignature2017","signatureValue":"x"}}`)
	equal(t, "Tombstone", d.Object.Type, federation.ObjectTombstone)
}

// The replies of a post are a collection whose pages may be embedded (Mastodon) or linked.
func TestNormalizeRepliesAndCollections(t *testing.T) {
	o := object(t, `{`+ctx+`, "id": "https://a.example/notes/1", "type": "Note",
		"replies": {"id": "https://a.example/notes/1/replies", "type": "Collection",
			"first": {"type": "CollectionPage", "next": "https://a.example/notes/1/replies?page=2",
				"partOf": "https://a.example/notes/1/replies", "items": ["https://a.example/notes/2", {"id": "https://b.example/notes/3", "type": "Note"}]}}}`)
	equal(t, "replies", o.Replies.ID, federation.IRI("https://a.example/notes/1/replies"))
	replies := o.Replies.Trusted()
	if replies == nil || replies.Collection == nil || replies.Collection.Page {
		t.Fatalf("replies = %+v, want a trusted collection", replies)
	}
	first := replies.Collection.First.Trusted()
	if first == nil || first.Collection == nil || !first.Collection.Page {
		t.Fatalf("first page = %+v, want a trusted embedded page", first)
	}
	page := first.Collection
	equal(t, "next", page.Next.ID, federation.IRI("https://a.example/notes/1/replies?page=2"))
	if len(page.Items) != 2 || page.Items[0].ID != "https://a.example/notes/2" || page.Items[1].ID != "https://b.example/notes/3" {
		t.Fatalf("items = %+v", page.Items)
	}
	if page.Items[1].Trusted() != nil {
		t.Errorf("an item of another origin is trusted")
	}

	// An ordered page reads orderedItems, keeps at most MaxItems of them, and links its next page.
	items := make([]string, 0, federation.MaxItems+5)
	for i := range federation.MaxItems + 5 {
		items = append(items, fmt.Sprintf(`"https://a.example/notes/%d"`, i))
	}
	d := normalize(t, `{`+ctx+`, "id": "https://a.example/c?page=1", "type": "OrderedCollectionPage",
		"next": {"type": "Link", "href": "https://a.example/c?page=2"}, "orderedItems": [`+strings.Join(items, ",")+`]}`)
	if d.Collection == nil || !d.Collection.Page || len(d.Collection.Items) != federation.MaxItems || d.Collection.Next.ID != "https://a.example/c?page=2" {
		t.Fatalf("ordered page = %+v", d.Collection)
	}
	equal(t, "ID", d.ID(), federation.IRI("https://a.example/c?page=1"))
	// A replies link without an embedded collection.
	o = object(t, `{`+ctx+`, "id": "https://a.example/notes/1", "type": "Note", "replies": "https://a.example/notes/1/replies"}`)
	if o.Replies.ID != "https://a.example/notes/1/replies" || o.Replies.Untrusted() != nil {
		t.Errorf("replies = %+v, want the bare link", o.Replies)
	}
}

func TestNormalizeErrors(t *testing.T) {
	deep := strings.Repeat(`{"a":`, federation.MaxDepth+1) + `1` + strings.Repeat(`}`, federation.MaxDepth+1)
	deepOK := `{"type":"Note","x":` + strings.Repeat(`[`, federation.MaxDepth-1) + strings.Repeat(`]`, federation.MaxDepth-1) + `}`
	if _, err := federation.Normalize([]byte(deepOK), now, lowerHost); err != nil {
		t.Errorf("depth %d: %v, want no error", federation.MaxDepth, err)
	}
	many := func(n int, item string) string {
		return "[" + strings.TrimSuffix(strings.Repeat(item+",", n), ",") + "]"
	}
	cases := []struct {
		name, doc string
		want      error
	}{
		{"not JSON", `{`, federation.ErrMalformed},
		{"trailing data", `{"type":"Note"} {}`, federation.ErrMalformed},
		{"array", `[{"type":"Note"}]`, federation.ErrMalformed},
		{"no type", `{"id":"https://a.example/n/1"}`, federation.ErrMalformed},
		{"too deep", deep, federation.ErrLimitExceeded},
		{"too many tags", `{"type":"Note","tag":` + many(federation.MaxTags+1, `{"type":"Hashtag","name":"#a"}`) + `}`, federation.ErrLimitExceeded},
		{"too many recipients", `{"type":"Note","to":` + many(federation.MaxRecipients/2+1, `"https://a.example/u"`) + `,"cc":` + many(federation.MaxRecipients/2, `"https://a.example/u"`) + `}`, federation.ErrLimitExceeded},
		{"content too large", `{"type":"Note","content":"` + strings.Repeat("a", federation.MaxBodyBytes+1) + `"}`, federation.ErrLimitExceeded},
		{"contentMap entry too large", `{"type":"Note","contentMap":{"en":"` + strings.Repeat("a", federation.MaxBodyBytes+1) + `"}}`, federation.ErrLimitExceeded},
		{"summary too large", `{"type":"Person","summary":"` + strings.Repeat("a", federation.MaxBodyBytes+1) + `"}`, federation.ErrLimitExceeded},
		{"embedded object over a limit", `{"type":"Create","actor":"https://a.example/u","object":{"type":"Note","tag":` + many(federation.MaxTags+1, `{}`) + `}}`, federation.ErrLimitExceeded},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := federation.Normalize([]byte(c.doc), now, lowerHost); !errors.Is(err, c.want) {
				t.Errorf("Normalize = %v, want %v", err, c.want)
			}
		})
	}
	// The limits themselves are allowed.
	ok := `{"type":"Note","tag":` + many(federation.MaxTags, `{}`) + `,"to":` + many(federation.MaxRecipients, `"https://a.example/u"`) +
		`,"content":"` + strings.Repeat("a", federation.MaxBodyBytes) + `"}`
	if _, err := federation.Normalize([]byte(ok), now, lowerHost); err != nil {
		t.Errorf("Normalize at the limits = %v, want no error", err)
	}
}

func TestSameOrigin(t *testing.T) {
	cases := []struct {
		a, b federation.IRI
		want bool
	}{
		{"https://a.example/x", "https://a.example/y", true},
		{"https://a.example/x", "https://a.example:8443/y", false},
		{"https://a.example/x", "http://a.example/x", false},
		{"https://a.example/x", "https://b.example/x", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := federation.SameOrigin(c.a, c.b); got != c.want {
			t.Errorf("SameOrigin(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// lowerHost is the HostFunc of these tests: the IDNA mapping is the application layer's.
func lowerHost(h string) (string, error) {
	if h == "" || strings.ContainsAny(h, " *") {
		return "", errors.New("bad host")
	}
	return strings.ToLower(h), nil
}

func TestNormalizeIRI(t *testing.T) {
	cases := []struct {
		in   string
		want federation.IRI
	}{
		{" HTTPS://A.Example/Path?Q#frag ", "https://a.example/Path?Q#frag"},
		{"https://A.Example:8443/x", "https://a.example:8443/x"},
		{"https://[2001:DB8::1]:8443/x", "https://[2001:db8::1]:8443/x"},
		{"https://bad*host/x", ""},
		{"https://:443/x", ""},
		{"https://user@a.example/x", ""},
		{"/relative", ""},
		{"mailto:a@a.example", ""},
		{"", ""},
	}
	for _, c := range cases {
		got, err := federation.NormalizeIRI(c.in, lowerHost)
		if got != c.want || (err != nil) != (c.want == "") {
			t.Errorf("NormalizeIRI(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
		if err != nil && !errors.Is(err, federation.ErrInvalidIRI) {
			t.Errorf("NormalizeIRI(%q) error = %v, want ErrInvalidIRI", c.in, err)
		}
	}
}

func TestNormalizeRejectedHost(t *testing.T) {
	o := object(t, `{"type":"Note","id":"https://bad*host/n/1","attributedTo":"https://a.example/u/a","to":["https://bad*host/u/b","as:Public"]}`)
	equal(t, "ID", o.ID, federation.IRI(""))
	equal(t, "To", o.Audience.To, []federation.IRI{federation.PublicIRI})
}

func TestDomainAndParents(t *testing.T) {
	cases := map[string][]string{
		"a.b.example": {"a.b.example", "b.example", "example"},
		"example":     {"example"},
		"":            nil,
	}
	for in, want := range cases {
		equal(t, in, federation.DomainAndParents(in), want)
	}
}

// A NUL in any string of a document, keys included, is read as U+FFFD (federation.md §4), so
// that PostgreSQL text can store every value.
func TestNormalizeReplacesNUL(t *testing.T) {
	const n = `\u0000`
	docs := map[string]string{
		"post": `{` + ctx + `,"type":"Create","id":"https://a.example/a/1` + n + `","actor":"https://a.example/u/alice","content":"x` + n + `",
			"object":{"type":"Question","id":"https://a.example/n/1` + n + `","attributedTo":"https://a.example/u/alice",
			"url":"https://a.example/@alice/1` + n + `","content":"<p>a` + n + `b</p>","contentMap":{"en` + n + `":"<p>c` + n + `</p>","ja":"<p>d` + n + `</p>"},
			"summary":"cw` + n + `","to":["https://www.w3.org/ns/activitystreams#Public` + n + `","https://a.example/u/alice/followers"],
			"tag":[{"type":"Hashtag","name":"#go` + n + `"},{"type":"Mention","href":"https://a.example/u/bob` + n + `","name":"@bob` + n + `"},
				{"type":"Emoji","id":"https://a.example/e/1","name":":blob` + n + `:","icon":{"url":"https://a.example/e` + n + `.png"}}],
			"attachment":[{"type":"Document","url":"https://a.example/f` + n + `.png","mediaType":"image/png` + n + `","name":"alt` + n + `","blurhash":"U` + n + `"}],
			"oneOf":[{"type":"Note","name":"yes` + n + `"}]}}`,
		"converted": `{` + ctx + `,"type":"Article","id":"https://a.example/n/2","name":"title` + n + `","summary":"lead` + n + `","content":"<p>x</p>"}`,
		"actor": `{` + ctx + `,"type":"Person","id":"https://a.example/u/alice","preferredUsername":"ali` + n + `ce","name":"Alice` + n + `",
			"summary":"<p>bio` + n + `</p>","url":"https://a.example/@alice` + n + `","inbox":"https://a.example/u/alice/inbox",
			"icon":"https://a.example/a` + n + `.png","publicKey":{"id":"https://a.example/u/alice#key","owner":"https://a.example/u/alice","publicKeyPem":"pem` + n + `"},
			"attachment":[{"type":"PropertyValue","name":"Site` + n + `","value":"<p>v` + n + `</p>"}]}`,
	}
	for name, doc := range docs {
		if path := nulPath(reflect.ValueOf(normalize(t, doc)), "Document"); path != "" {
			t.Errorf("%s: %s has a NUL", name, path)
		}
	}
	o := normalize(t, docs["post"]).Activity.Objects[0].Untrusted().Object
	equal(t, "Bodies", o.Bodies, []federation.Body{{Language: federation.UndeterminedLang, HTML: "<p>c\uFFFD</p>"}, {Language: "ja", HTML: "<p>d\uFFFD</p>"}})
	equal(t, "SpoilerText", o.SpoilerText, "cw\uFFFD")
	equal(t, "Hashtags", o.Hashtags, []string{"go\uFFFD"})
	equal(t, "Attachment name", o.Attachments[0].Name, "alt\uFFFD")
	equal(t, "Poll option", o.Poll.Options[0].Name, "yes\uFFFD")
	equal(t, "URL", o.URL, "https://a.example/@alice/1%EF%BF%BD")
	equal(t, "PreferredUsername", normalize(t, docs["actor"]).Actor.PreferredUsername, "ali\uFFFDce")

	env, err := federation.ReadEnvelope([]byte(`{"id":"https://a.example/a/1`+n+`","type":"Cre`+n+`ate","actor":"https://a.example/u/1"}`), lowerHost)
	if err != nil || env.Type != "Cre\uFFFDate" || env.ID != "https://a.example/a/1%EF%BF%BD" {
		t.Errorf("ReadEnvelope = %+v, %v; want the NUL of the type and the id as U+FFFD", env, err)
	}
}

// nulPath returns the path of the first string under v that has a NUL, or "".
func nulPath(v reflect.Value, path string) string {
	switch v.Kind() {
	case reflect.String:
		if strings.Contains(v.String(), "\x00") {
			return path
		}
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			return nulPath(v.Elem(), path)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if p := nulPath(v.Field(i), path+"."+v.Type().Field(i).Name); p != "" {
				return p
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			if p := nulPath(v.Index(i), fmt.Sprintf("%s[%d]", path, i)); p != "" {
				return p
			}
		}
	}
	return ""
}
