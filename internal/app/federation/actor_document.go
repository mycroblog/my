// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"crypto/ed25519"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/federation"
)

// ProfileImages returns the images that accounts show as their own, keyed by ref, leaving out
// the files that cannot be shown; (*appcontent.MediaService).Images is one.
type ProfileImages func(ctx context.Context, refs []appcontent.ImageRef) (map[appcontent.ImageRef]appcontent.Image, error)

// ActorImages are the avatar and header of an actor's document, nil when it has none, the
// Emoji tags of the custom emoji of its profile (LocalActorEmojis), and its aliases
// (LocalActorAliases).
type ActorImages struct {
	Icon, Image *federation.ImageDocument
	Emojis      []federation.TagDocument
	AlsoKnownAs []federation.IRI
}

// AliasReader reads the aliases of a local account (ADR-0089); *appaccount.Service implements
// it.
type AliasReader interface {
	Aliases(ctx context.Context, accountID string) ([]appaccount.Alias, error)
}

// LocalActorAliases returns the alsoKnownAs of a local actor: the actor ids of its aliases. A
// nil aliases reads none.
func LocalActorAliases(ctx context.Context, aliases AliasReader, a appaccount.Actor) ([]federation.IRI, error) {
	if aliases == nil || a.Local == nil {
		return nil, nil
	}
	list, err := aliases.Aliases(ctx, a.Account.ID)
	if err != nil {
		return nil, fmt.Errorf("federation: the aliases of %s: %w", a.Account.ID, err)
	}
	out := make([]federation.IRI, 0, len(list))
	for _, al := range list {
		out = append(out, federation.IRI(al.URI))
	}
	return out, nil
}

// EmojiTagReader returns the Emoji tags of the custom emoji with an ID in ids that can be shown;
// (*appcontent.MediaService).EmojiTags is one.
type EmojiTagReader func(ctx context.Context, ids []string) ([]appcontent.EmojiTag, error)

// EmojiTagOf returns the Emoji tag of a custom emoji (ADR-0051 §3): a local emoji's id is derived
// from its ID on host, and a remote one has its recorded id, or none.
func EmojiTagOf(e appcontent.EmojiTag, host string) federation.TagDocument {
	id := federation.IRI(e.URI)
	if e.Local {
		id = federation.IRI(appcontent.CustomEmojiURI(host, e.ID))
	}
	return federation.EmojiTag(id, e.Shortcode, e.Updated, e.MediaType, e.URL)
}

// LocalActorEmojis returns the Emoji tags of the custom emoji of a local actor's profile (its
// display name, note, and fields, ADR-0051 §3), with the ids of host. A nil emojis reads none.
func LocalActorEmojis(ctx context.Context, emojis EmojiTagReader, host string, a appaccount.Actor) ([]federation.TagDocument, error) {
	if emojis == nil || a.Profile == nil || len(a.Profile.EmojiIDs) == 0 {
		return nil, nil
	}
	tags, err := emojis(ctx, a.Profile.EmojiIDs)
	if err != nil {
		return nil, fmt.Errorf("federation: the custom emoji of %s: %w", a.Account.ID, err)
	}
	out := make([]federation.TagDocument, 0, len(tags))
	for _, t := range tags {
		out = append(out, EmojiTagOf(t, host))
	}
	return out, nil
}

// LocalActorImages reads the avatar and header of a local actor through images, as their large
// variants: moving for an animation, which the receiving servers make still versions of
// (ADR-0054 §5). A nil images reads none.
func LocalActorImages(ctx context.Context, images ProfileImages, a appaccount.Actor) (ActorImages, error) {
	p := a.Profile
	if images == nil || p == nil || p.AvatarMediaID == "" && p.HeaderMediaID == "" {
		return ActorImages{}, nil
	}
	avatar := appcontent.ImageRef{AccountID: a.Account.ID, MediaID: p.AvatarMediaID}
	header := appcontent.ImageRef{AccountID: a.Account.ID, MediaID: p.HeaderMediaID}
	found, err := images(ctx, []appcontent.ImageRef{avatar, header})
	if err != nil {
		return ActorImages{}, fmt.Errorf("federation: the images of %s: %w", a.Account.ID, err)
	}
	doc := func(ref appcontent.ImageRef) *federation.ImageDocument {
		img, ok := found[ref]
		if !ok || ref.MediaID == "" {
			return nil
		}
		v := img.Primary()
		return &federation.ImageDocument{Type: federation.ImageType, MediaType: v.ContentType, URL: v.URL}
	}
	return ActorImages{Icon: doc(avatar), Image: doc(header)}, nil
}

// Collections of a local actor, the last path segment under {actor}/ (ADR-0018).
const (
	CollectionInbox        = "inbox"
	CollectionOutbox       = "outbox"
	CollectionFollowers    = "followers"
	CollectionFollowing    = "following"
	CollectionFeatured     = "featured"
	CollectionFeaturedTags = "featured-tags"
)

// SharedInboxURI returns the shared inbox of the instance on host.
func SharedInboxURI(host string) string { return "https://" + host + "/ap/inbox" }

// ProfileURL returns the human-readable profile URL of a local username on host.
func ProfileURL(host, username string) string { return "https://" + host + "/@" + username }

// LocalActorDocument returns the document of a local actor served on host: the answer to a GET
// of its id (federation.md §10) and the object of the Update its profile revisions send
// (§7.1). The instance actor's document is the minimal one that signs the instance's requests;
// its preferredUsername is instanceUsername. images are the actor's avatar and header
// (LocalActorImages), the Emoji tags of its profile (LocalActorEmojis), and its aliases
// (LocalActorAliases).
func LocalActorDocument(host string, a appaccount.Actor, instanceUsername string, images ActorImages) federation.ActorDocument {
	id := federation.IRI(a.Account.URI)
	doc := federation.ActorDocument{
		Context:    federation.ActorLDContext(),
		ID:         id,
		Type:       federation.ActorType(a.ActivityStreamsType()),
		Following:  id + "/" + CollectionFollowing,
		Followers:  id + "/" + CollectionFollowers,
		Inbox:      id + "/" + CollectionInbox,
		Outbox:     id + "/" + CollectionOutbox,
		Published:  federation.FormatTime(a.Account.CreatedAt),
		Tag:        []any{},
		Attachment: []federation.PropertyValue{},
		Endpoints:  map[string]federation.IRI{"sharedInbox": federation.IRI(SharedInboxURI(host))},
	}
	if k, ok := a.PublishedKey(); ok {
		doc.PublicKey = &federation.PublicKeyDocument{ID: k.KeyID, Owner: id, PublicKeyPEM: k.PublicKeyPEM}
	}
	if m, ok := assertionMethod(a); ok {
		doc.AssertionMethod = []federation.MultikeyDocument{m}
	}
	if a.IsInstanceActor() {
		// The instance actor signs the instance's own requests and is followed by no one.
		doc.PreferredUsername = instanceUsername
		doc.ManuallyApprovesFollowers = true
		return doc
	}
	doc.Featured = id + "/" + CollectionFeatured
	doc.FeaturedTags = id + "/" + CollectionFeaturedTags
	if a.Local != nil {
		doc.PreferredUsername = a.Local.Username
		doc.URL = ProfileURL(host, a.Local.Username)
		if len(images.AlsoKnownAs) > 0 {
			doc.AlsoKnownAs = images.AlsoKnownAs
		}
	}
	if p := a.Profile; p != nil {
		doc.Name, doc.Summary = &p.DisplayName, &p.NoteHTML
		doc.ManuallyApprovesFollowers = p.Locked
		doc.Discoverable, doc.Indexable = &p.Discoverable, &p.Indexable
		doc.Icon, doc.Image = images.Icon, images.Image
		for _, f := range p.Fields {
			doc.Attachment = append(doc.Attachment, federation.PropertyValue{Type: "PropertyValue", Name: f.Name, Value: f.ValueHTML})
		}
		for _, t := range images.Emojis {
			doc.Tag = append(doc.Tag, t)
		}
	}
	return doc
}

// assertionMethod returns the Multikey of a's Ed25519 key (FEP-521a), which verifies the
// integrity proofs of its activities (FEP-8b32), false when it has none.
func assertionMethod(a appaccount.Actor) (federation.MultikeyDocument, bool) {
	k, ok := a.AssertionKey()
	if !ok {
		return federation.MultikeyDocument{}, false
	}
	pub, err := appaccount.ParsePublicKeyPEM(k.PublicKeyPEM)
	if err != nil {
		return federation.MultikeyDocument{}, false
	}
	ed, ok := pub.(ed25519.PublicKey)
	if !ok {
		return federation.MultikeyDocument{}, false
	}
	return federation.MultikeyDocument{
		ID: k.KeyID, Type: federation.MultikeyType, Controller: federation.IRI(a.Account.URI),
		PublicKeyMultibase: federation.EncodeEd25519Multikey(ed),
	}, true
}
