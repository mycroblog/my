// SPDX-License-Identifier: AGPL-3.0-only

package timeline_test

import (
	"context"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// fakeTags names the tags of the hashtag branch.
type fakeTags struct{ byID map[string]appcontent.Tag }

func (f fakeTags) EnsureTagTx(context.Context, uow.Tx, string) (appcontent.Tag, error) {
	return appcontent.Tag{}, appcontent.ErrInvalidHashtag
}

func (f fakeTags) TagByName(context.Context, string) (appcontent.Tag, error) {
	return appcontent.Tag{}, appcontent.ErrNotFound
}

func (f fakeTags) TagsByID(_ context.Context, ids []string) (map[string]appcontent.Tag, error) {
	out := map[string]appcontent.Tag{}
	for _, id := range ids {
		if t, ok := f.byID[id]; ok {
			out[id] = t
		}
	}
	return out, nil
}

func tagged(e apptimeline.Entry, tagIDs ...string) apptimeline.Entry {
	e.FollowedTagIDs = tagIDs
	return e
}

// The hashtag branch merges with the follows by ID, each entry once, and CanView judges its
// entries as on the hashtag timeline: an author kept from discovery or an unlisted post that the
// SQL let through does not show, while a public post of an author the viewer does not follow
// does, with the names of its followed tags.
func TestServiceHomeMergesFollowedHashtags(t *testing.T) {
	cats, dogs := testID(801), testID(802)
	silencedID := testID(906)
	repo := &leakyRepository{
		rows: []apptimeline.Entry{
			post(testID(7), followeeID, appcontent.VisibilityPublic),
			post(testID(4), followeeID, appcontent.VisibilityPrivate),
		},
		tagged: []apptimeline.Entry{
			// The same entry as a follow: the follow wins, without the tags.
			tagged(post(testID(7), followeeID, appcontent.VisibilityPublic), cats),
			tagged(post(testID(6), authorID, appcontent.VisibilityPublic), dogs, cats),
			tagged(post(testID(5), authorID, appcontent.VisibilityUnlisted), cats),
			tagged(post(testID(3), silencedID, appcontent.VisibilityPublic), cats),
			tagged(post(testID(2), authorID, appcontent.VisibilityLocal), cats),
		},
		facts: apptimeline.Facts{
			Followed:            map[string]bool{followeeID: true},
			AccountRestrictions: map[string][]timeline.Restriction{silencedID: {timeline.RestrictPublicDiscovery}},
		},
	}
	svc := apptimeline.NewService(apptimeline.Deps{
		Repository: repo, Clock: fixedClock{time.Now()},
		Tags: fakeTags{byID: map[string]appcontent.Tag{cats: {ID: cats, Name: "Cats"}, dogs: {ID: dogs, Name: "dogs"}}},
	})
	viewer := appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: viewerID}
	p, err := svc.Home(t.Context(), viewer, apptimeline.Query{Limit: 10})
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if got, want := ids(p), []string{testID(7), testID(6), testID(4), testID(2)}; !slices.Equal(got, want) {
		t.Fatalf("Home = %v, want %v", got, want)
	}
	for _, e := range p.Entries {
		var want []string
		switch e.ID {
		case testID(6):
			want = []string{"Cats", "dogs"}
		case testID(2):
			want = []string{"Cats"}
		}
		if !slices.Equal(e.FollowedTags, want) {
			t.Errorf("FollowedTags of %s = %v, want %v", e.ID, e.FollowedTags, want)
		}
	}
}
