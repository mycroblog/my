// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"reflect"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/platform"
)

// TestNotificationsAreWired checks that the composition root registers the notification
// projectors once, on the graph and content events: a follow, a mention, and a favourite each
// notify the other account once, and the unread count follows.
func TestNotificationsAreWired(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	start(t, cfg, platform.ServeOptions{}, func(a *platform.App) {
		ctx := t.Context()
		var accounts []string
		for _, name := range []string{"alice", "bob"} {
			reg, err := a.Identity.RegisterLocalUser(ctx, identity.RegisterInput{
				Username: name, Password: "correct horse battery staple", DisplayName: name, Locale: "en",
			})
			if err != nil {
				t.Fatalf("RegisterLocalUser(%s): %v", name, err)
			}
			accounts = append(accounts, reg.AccountID)
		}
		alice, bob := accounts[0], accounts[1]
		if _, err := a.Graph.Follow(ctx, bob, alice); err != nil {
			t.Fatalf("Follow: %v", err)
		}
		p, err := a.Content.Create(ctx, appcontent.CreateInput{
			AuthorAccountID: alice, TextInput: appcontent.TextInput{Text: "hi @bob"}, Visibility: domcontent.VisibilityPublic,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		for range 2 {
			if _, err := a.Interactions.Favourite(ctx, bob, p.Status.ID); err != nil {
				t.Fatalf("Favourite: %v", err)
			}
		}
		types := func(accountID string) []notification.Type {
			ns, err := a.Notifications.List(ctx, accountID, appnotification.Query{})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			out := []notification.Type{}
			for _, n := range ns {
				out = append(out, n.Type)
			}
			return out
		}
		if got, want := types(alice), []notification.Type{notification.TypeFavourite, notification.TypeFollow}; !reflect.DeepEqual(got, want) {
			t.Errorf("notifications of alice = %v, want %v", got, want)
		}
		if got, want := types(bob), []notification.Type{notification.TypeMention}; !reflect.DeepEqual(got, want) {
			t.Errorf("notifications of bob = %v, want %v", got, want)
		}
		if n, err := a.Notifications.UnreadCount(ctx, alice, false); err != nil || n != 2 {
			t.Errorf("UnreadCount = %d, %v; want 2", n, err)
		}
	})
}

// TestSilenceLimitsNotifications checks that the composition root hands the moderation policy to
// the notification projectors: a limited account's mention notifies only the mentioned accounts
// that follow it.
func TestSilenceLimitsNotifications(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	start(t, cfg, platform.ServeOptions{}, func(a *platform.App) {
		ctx := t.Context()
		var accounts []string
		for _, name := range []string{"alice", "bob", "carol"} {
			reg, err := a.Identity.RegisterLocalUser(ctx, identity.RegisterInput{
				Username: name, Password: "correct horse battery staple", DisplayName: name, Locale: "en",
			})
			if err != nil {
				t.Fatalf("RegisterLocalUser(%s): %v", name, err)
			}
			accounts = append(accounts, reg.AccountID)
		}
		alice, bob, carol := accounts[0], accounts[1], accounts[2]
		if _, err := a.Graph.Follow(ctx, bob, alice); err != nil {
			t.Fatalf("Follow: %v", err)
		}
		if _, err := a.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
			TargetAccountID: alice, Kind: moderation.ActionLimit, ReasonCategory: moderation.CategorySpam,
		}); err != nil {
			t.Fatalf("IssueAccountAction(limit): %v", err)
		}
		if _, err := a.Content.Create(ctx, appcontent.CreateInput{
			AuthorAccountID: alice, TextInput: appcontent.TextInput{Text: "hi @bob @carol"}, Visibility: domcontent.VisibilityPrivate,
		}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		for _, c := range []struct {
			account string
			want    int
		}{{bob, 1}, {carol, 0}} {
			ns, err := a.Notifications.List(ctx, c.account, appnotification.Query{Types: []notification.Type{notification.TypeMention}})
			if err != nil || len(ns) != c.want {
				t.Errorf("mentions of %s = %+v, %v; want %d", c.account, ns, err, c.want)
			}
		}
	})
}
