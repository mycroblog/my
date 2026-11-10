// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"context"
	"reflect"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/federation"
)

// featuredTagReader returns the same featured hashtags for every account.
type featuredTagReader []apptimeline.FeaturedTag

func (r featuredTagReader) FeaturedTags(context.Context, string) ([]apptimeline.FeaturedTag, error) {
	return r, nil
}

// TestObjectsFeaturedTags checks the Hashtags of the featured-tags collection of a local actor:
// the name with its #, and the href of the profile's page of the tag (ADR-0006 §17).
func TestObjectsFeaturedTags(t *testing.T) {
	const host = "social.example"
	// The domain types are the account module's own (depguard): the local account is set as
	// objects_test.go sets it.
	alice := appaccount.Actor{Account: appaccount.Account{ID: "0192f3a4-0000-7000-8000-000000000001"}}
	local := reflect.New(reflect.TypeOf(alice.Local).Elem())
	local.Elem().FieldByName("Username").SetString("alice")
	reflect.ValueOf(&alice).Elem().FieldByName("Local").Set(local)
	reader := featuredTagReader{
		{Tag: appcontent.Tag{Name: "Cats", NameNormalized: "cats"}, StatusesCount: 3},
		{Tag: appcontent.Tag{Name: "日本語", NameNormalized: "日本語"}},
	}
	o := appfederation.NewObjects(appfederation.ObjectsDeps{FeaturedTags: reader, Host: host})
	got, err := o.FeaturedTags(t.Context(), alice)
	want := []federation.TagDocument{
		{Type: federation.TagHashtag, Href: "https://social.example/@alice/tagged/cats", Name: "#Cats"},
		{Type: federation.TagHashtag, Href: "https://social.example/@alice/tagged/%E6%97%A5%E6%9C%AC%E8%AA%9E", Name: "#日本語"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("FeaturedTags = %+v, %v; want %+v", got, err, want)
	}

	// The instance actor features nothing, and neither does a server without the reader.
	instance := appaccount.Actor{Account: alice.Account}
	if got, err := o.FeaturedTags(t.Context(), instance); err != nil || len(got) != 0 {
		t.Errorf("FeaturedTags(instance actor) = %+v, %v; want none", got, err)
	}
	none := appfederation.NewObjects(appfederation.ObjectsDeps{Host: host})
	if got, err := none.FeaturedTags(t.Context(), alice); err != nil || got == nil || len(got) != 0 {
		t.Errorf("FeaturedTags(no reader) = %#v, %v; want an empty list", got, err)
	}
}
