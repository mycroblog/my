// SPDX-License-Identifier: AGPL-3.0-only

package activitypub_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The documents under testdata/peers are what Mastodon, GoToSocial, and Misskey themselves
// serve, with hosts, names, and texts replaced by example values. The Mastodon and Misskey
// documents were fetched from running servers (Mastodon 4.8, Misskey 2025.4); the GoToSocial
// actor follows its documented shape, since GoToSocial serves actors only to signed requests.
// The create-note.json documents are the Create of a public Note with a content warning, a
// mention, a hashtag, and an image, written after each implementation's serializer
// (Mastodon's ActivityPub::NoteSerializer, GoToSocial's internal/typeutils/internaltoas.go,
// Misskey's ApRendererService.renderNote). They show the shape each implementation publishes and
// therefore parses; the tests below check that our documents have the same shape where they
// carry the same properties.

var peers = []string{"mastodon", "gotosocial", "misskey"}

// kind is the JSON type of a value.
func kind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case []any:
		return "array"
	default:
		return "object"
	}
}

// kinds flattens the objects of a document into dotted paths and their JSON types. The
// elements of arrays are described by the first element, as "path[]".
func kinds(prefix string, v any, out map[string]string) map[string]string {
	if out == nil {
		out = map[string]string{}
	}
	if prefix != "" {
		out[prefix] = kind(v)
	}
	switch v := v.(type) {
	case map[string]any:
		for k, e := range v {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			kinds(p, e, out)
		}
	case []any:
		if len(v) > 0 {
			kinds(prefix+"[]", v[0], out)
		}
	}
	return out
}

func readPeer(t *testing.T, peer, name string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(readFile(t, "peers/"+peer+"/"+name)), &doc); err != nil {
		t.Fatalf("peers/%s/%s: %v", peer, name, err)
	}
	return doc
}

// checkShape fails for every path that both documents have with different JSON types, and for
// every required path that ours lacks. "@context" is exempt: its array elements differ in
// kind (strings, then an object) in every implementation. Where theirs has a single value and
// ours an array of such values, the shapes agree: JSON-LD compaction writes a one-element array
// as its element (GoToSocial's "to").
func checkShape(t *testing.T, peer string, ours, theirs map[string]any, required []string) {
	t.Helper()
	got, want := kinds("", ours, nil), kinds("", theirs, nil)
	for path, k := range want {
		if strings.HasPrefix(path, "@context") {
			continue
		}
		if g, ok := got[path]; ok && g != k && k != "null" && (g != "array" || got[path+"[]"] != k) {
			t.Errorf("%s: %s is a %s, %s publishes a %s", peer, path, g, peer, k)
		}
	}
	for _, path := range required {
		if _, ok := got[path]; !ok {
			t.Errorf("%s: our document lacks %s", peer, path)
		}
	}
}

// actorRequired lists the actor properties each implementation reads when it imports a
// remote account (Mastodon: ActivityPub::ProcessAccountService; GoToSocial:
// internal/ap/extract.go; Misskey: ApPersonService.validateActor and createPerson).
var actorRequired = map[string][]string{
	"mastodon": {
		"@context", "id", "type", "preferredUsername", "name", "summary", "url", "inbox", "outbox",
		"followers", "following", "featured", "featuredTags", "endpoints.sharedInbox",
		"manuallyApprovesFollowers", "discoverable", "indexable", "published", "attachment",
		"attachment[].type", "attachment[].name", "attachment[].value", "tag",
		"publicKey.id", "publicKey.owner", "publicKey.publicKeyPem", "icon.url", "image.url",
	},
	"gotosocial": {
		"@context", "id", "type", "preferredUsername", "name", "summary", "url", "inbox", "outbox",
		"followers", "following", "featured", "manuallyApprovesFollowers", "discoverable",
		"publicKey.id", "publicKey.owner", "publicKey.publicKeyPem",
	},
	"misskey": {
		"@context", "id", "type", "preferredUsername", "name", "summary", "url", "inbox", "outbox",
		"followers", "following", "featured", "endpoints.sharedInbox", "manuallyApprovesFollowers",
		"discoverable", "attachment[].name", "attachment[].value",
		"publicKey.id", "publicKey.owner", "publicKey.publicKeyPem",
	},
}

func TestActorMatchesPeers(t *testing.T) {
	s := newServer(t, accountDomain)
	ours := decode(t, s.get(t, "/ap/actors/"+aliceID, acceptMastodon).Body.Bytes())
	for _, peer := range peers {
		checkShape(t, peer, ours, readPeer(t, peer, "actor.json"), actorRequired[peer])
	}
}

func TestWebFingerMatchesPeers(t *testing.T) {
	s := newServer(t, accountDomain)
	ours := decode(t, s.get(t, "/.well-known/webfinger?resource=acct:alice@"+accountDomain, "").Body.Bytes())
	for _, peer := range peers {
		checkShape(t, peer, ours, readPeer(t, peer, "webfinger.json"), []string{"subject", "links[].rel", "links[].type", "links[].href"})
	}
}

func TestNodeInfoMatchesPeers(t *testing.T) {
	s := newServer(t, accountDomain)
	ours := decode(t, s.get(t, "/nodeinfo/2.1", "").Body.Bytes())
	required := []string{
		"version", "software.name", "software.version", "software.repository", "protocols",
		"services.inbound", "services.outbound", "openRegistrations", "usage.users.total",
		"usage.localPosts", "metadata",
	}
	for _, peer := range peers {
		checkShape(t, peer, ours, readPeer(t, peer, "nodeinfo.json"), required)
	}
	if ours["version"] != "2.1" || !regexp.MustCompile(`^[a-z0-9-]+$`).MatchString(ours["software"].(map[string]any)["name"].(string)) {
		t.Errorf("version / software.name = %v / %v, want 2.1 and a NodeInfo name", ours["version"], ours["software"])
	}
}

// TestDiscoveryRoundTrip follows the steps a remote server takes from a handle to a verified
// actor: WebFinger, then the self link, then the check that the actor's preferredUsername on
// the WebFinger subject's domain gives back the same actor (Mastodon's
// ActivityPub::FetchRemoteAccountService and GoToSocial's dereferencing both verify this).
func TestDiscoveryRoundTrip(t *testing.T) {
	for _, tc := range []struct{ name, accountDomain, handle string }{
		{"same domain", "", "alice@" + host},
		{"separate account domain", accountDomain, "alice@" + accountDomain},
		{"instance actor", accountDomain, accountDomain + "@" + accountDomain},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newServer(t, tc.accountDomain)
			jrd := decode(t, s.get(t, "/.well-known/webfinger?resource=acct:"+tc.handle, "").Body.Bytes())
			if jrd["subject"] != "acct:"+tc.handle {
				t.Fatalf("subject = %v, want acct:%s", jrd["subject"], tc.handle)
			}
			var self string
			for _, l := range jrd["links"].([]any) {
				if l := l.(map[string]any); l["rel"] == "self" && l["type"] == "application/activity+json" {
					self = l["href"].(string)
				}
			}
			u, err := url.Parse(self)
			if err != nil || u.Host != host {
				t.Fatalf("self link = %q, want an id on %s", self, host)
			}
			rec := s.get(t, u.Path, acceptMastodon)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s = %d", self, rec.Code)
			}
			actor := decode(t, rec.Body.Bytes())
			if actor["id"] != self {
				t.Errorf("actor id = %v, want the self link %s", actor["id"], self)
			}
			domain := tc.handle[strings.LastIndexByte(tc.handle, '@')+1:]
			again := decode(t, s.get(t, "/.well-known/webfinger?resource=acct:"+actor["preferredUsername"].(string)+"@"+domain, "").Body.Bytes())
			if again["subject"] != jrd["subject"] {
				t.Errorf("WebFinger of preferredUsername %v = %v, want %v", actor["preferredUsername"], again["subject"], jrd["subject"])
			}
			// Misskey rejects an actor whose collections or key are on another host than its id.
			key := actor["publicKey"].(map[string]any)
			for _, v := range []any{actor["inbox"], actor["outbox"], actor["followers"], actor["following"], key["id"], key["owner"]} {
				if s, _ := v.(string); !strings.HasPrefix(s, self) {
					t.Errorf("%v is not under the actor id %s", v, self)
				}
			}
			if !regexp.MustCompile(`^\w([\w-.]*\w)?$`).MatchString(actor["preferredUsername"].(string)) {
				t.Errorf("preferredUsername %v fails Misskey's validation", actor["preferredUsername"])
			}
		})
	}
}

// TestHiddenAccountsAreNeverListed guards that lookups go through one visibility rule: the
// fixtures hold accounts of every kind, and only active local ones resolve.
func TestHiddenAccountsAreNeverListed(t *testing.T) {
	s := newServer(t, accountDomain)
	var visible []string
	for id := range s.accounts.actors {
		// A deactivated actor answers with an emptied document, and is listed nowhere either.
		if id != gwenID && s.get(t, "/ap/actors/"+id, acceptMastodon).Code == http.StatusOK {
			visible = append(visible, id)
		}
	}
	slices.Sort(visible)
	want := []string{aliceID, botID, instanceID}
	if !slices.Equal(visible, want) {
		t.Errorf("visible actors = %v, want %v", visible, want)
	}
}
