// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/federation"
)

// rememberingKV is a KeyValueStore that remembers a failed fetch of every URI, so that a fetch
// that passes the federation mode ends without a request.
type rememberingKV struct{}

func (rememberingKV) Get(context.Context, string, string) ([]byte, error) {
	return []byte("not_found"), nil
}
func (rememberingKV) Set(context.Context, string, string, []byte, time.Duration) error { return nil }
func (rememberingKV) Delete(context.Context, string, string) error                     { return nil }
func (rememberingKV) IncrBy(context.Context, string, string, int64, time.Duration) (int64, error) {
	return 0, nil
}
func (rememberingKV) DeleteExpired(context.Context, int) (int64, error) { return 0, nil }

type availableInstances struct{}

func (availableInstances) InstanceState(context.Context, string) (federation.InstanceState, error) {
	return federation.InstanceAvailable, nil
}

// TestFetcherMode checks the fetches that each federation mode makes (ADR-0076, 外の取得): none
// when it is off, actors only when it is outbound, and everything otherwise. A fetch that the
// mode makes reaches the failure memory, which answers not found here.
func TestFetcherMode(t *testing.T) {
	const uri = "https://remote.example/users/carol"
	cases := []struct {
		mode          federation.Mode
		content, acts bool // whether Fetch and FetchActor are made
	}{
		{federation.ModeOff, false, false},
		{federation.ModeInbound, true, true},
		{federation.ModeOutbound, false, true},
		{federation.ModeBoth, true, true},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			f := NewFetcher(FetcherDeps{
				KV: rememberingKV{}, Domains: NoDomainRestrictions{}, Instances: availableInstances{},
				Host: "my.example", Modes: FixedMode(tc.mode),
			})
			check := func(name string, err error, made bool) {
				t.Helper()
				refused := errors.Is(err, ErrFederationRestricted)
				if refused && !errors.Is(err, ErrFetchRefused) {
					t.Errorf("%s = %v, want a refusal that wraps ErrFetchRefused", name, err)
				}
				if made && !errors.Is(err, ErrFetchNotFound) || !made && !refused {
					t.Errorf("%s = %v, made = %v", name, err, made)
				}
			}
			_, err := f.Fetch(t.Context(), uri)
			check("Fetch", err, tc.content)
			_, err = f.FetchActor(t.Context(), uri)
			check("FetchActor", err, tc.acts)
		})
	}
}

type followsAnyone bool

func (f followsAnyone) FollowsAnyone(context.Context, string) (bool, error) { return bool(f), nil }

// TestMediaGate checks which remote media files the media proxy fetches in each mode: every one
// when the mode takes content in, only the avatars and headers of the remote followers of local
// accounts when it is outbound, none when it is off, and always the images of link cards.
func TestMediaGate(t *testing.T) {
	cases := []struct {
		mode federation.Mode
		// attachment of a follower, avatar of a follower, avatar of another account
		want [3]bool
	}{
		{federation.ModeOff, [3]bool{false, false, false}},
		{federation.ModeInbound, [3]bool{true, true, true}},
		{federation.ModeOutbound, [3]bool{false, true, false}},
		{federation.ModeBoth, [3]bool{true, true, true}},
	}
	for _, tc := range cases {
		follower := MediaGate{Modes: FixedMode(tc.mode), Followers: followsAnyone(true)}
		other := MediaGate{Modes: FixedMode(tc.mode), Followers: followsAnyone(false)}
		got := [3]bool{}
		var err error
		if got[0], err = follower.ProxiesMedia(t.Context(), "acct", false); err != nil {
			t.Fatal(err)
		}
		if got[1], err = follower.ProxiesMedia(t.Context(), "acct", true); err != nil {
			t.Fatal(err)
		}
		if got[2], err = other.ProxiesMedia(t.Context(), "acct", true); err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s: ProxiesMedia = %v, want %v", tc.mode, got, tc.want)
		}
		if ok, err := other.ProxiesMedia(t.Context(), "", false); err != nil || !ok {
			t.Errorf("%s: ProxiesMedia of a link card = %v, %v; want true", tc.mode, ok, err)
		}
	}
}

// TestReceives checks which received activities each mode hands to their handlers (ADR-0076,
// 入口ごとの振る舞い, inbox の非同期の部分).
func TestReceives(t *testing.T) {
	const host = "my.example"
	const carol = "https://remote.example/users/carol"
	n := 0
	normalize := func(doc map[string]any) *federation.Activity {
		t.Helper()
		n++
		doc["@context"] = "https://www.w3.org/ns/activitystreams"
		doc["id"] = carol + "/activities/" + strconv.Itoa(n)
		doc["actor"] = carol
		raw, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		d, err := Normalize(raw, time.Now())
		if err != nil || d.Activity == nil {
			t.Fatalf("Normalize(%s) = %v, %v", raw, d, err)
		}
		return d.Activity
	}
	act := func(typ federation.ActivityType, object string) *federation.Activity {
		return normalize(map[string]any{"type": string(typ), "object": object})
	}
	undo := func(of federation.ActivityType) *federation.Activity {
		return normalize(map[string]any{"type": "Undo", "object": map[string]any{
			"id": carol + "/undone", "type": string(of), "actor": carol, "object": "https://my.example/ap/actors/alice",
		}})
	}
	const local, remote = "https://my.example/ap/objects/1", "https://remote.example/notes/1"
	cases := []struct {
		name string
		act  *federation.Activity
		// off inbound outbound both
		want string
	}{
		{"Create", act(federation.ActivityCreate, remote), "x o x o"},
		{"Update of a post", act(federation.ActivityUpdate, remote), "x o x o"},
		{"Delete of a post", act(federation.ActivityDelete, remote), "x o x o"},
		{"Update of the actor itself", act(federation.ActivityUpdate, carol), "x o o o"},
		{"Delete of the actor itself", act(federation.ActivityDelete, carol), "x o o o"},
		{"Follow", act(federation.ActivityFollow, "https://my.example/ap/actors/alice"), "x o o o"},
		{"Accept", act(federation.ActivityAccept, "https://my.example/follows/1"), "x o o o"},
		{"Reject", act(federation.ActivityReject, "https://my.example/follows/1"), "x o o o"},
		{"Undo Follow", undo(federation.ActivityFollow), "x o o o"},
		{"Undo Accept", undo(federation.ActivityAccept), "x o x o"},
		{"Undo Block", undo(federation.ActivityBlock), "x o o o"},
		{"Undo Like", undo(federation.ActivityLike), "x o x o"},
		{"Undo of an IRI", act(federation.ActivityUndo, "https://remote.example/likes/1"), "x o o o"},
		{"Block", act(federation.ActivityBlock, "https://my.example/ap/actors/alice"), "x o o o"},
		{"Flag", act(federation.ActivityFlag, "https://my.example/ap/actors/alice"), "x o o o"},
		{"Like of a local post", act(federation.ActivityLike, local), "x x x o"},
		{"EmojiReact of a local post", act(federation.ActivityEmojiReact, local), "x x x o"},
		{"Announce of a local post", act(federation.ActivityAnnounce, local), "x x x o"},
		{"Announce of a remote post", act(federation.ActivityAnnounce, remote), "x o x o"},
		{"QuoteRequest", act(federation.ActivityQuoteRequest, local), "x x o o"},
		{"Move", act(federation.ActivityMove, carol), "x o x o"},
		{"Add", act(federation.ActivityAdd, remote), "x o x o"},
	}
	for _, tc := range cases {
		for i, m := range federation.Modes() {
			if got, want := receives(m, tc.act, host), tc.want[2*i] == 'o'; got != want {
				t.Errorf("%s: receives(%s) = %v, want %v", tc.name, m, got, want)
			}
		}
	}
}

// TestMoveSends checks the federation modes that send the move of a local account (ADR-0089,
// federation.md §14.1): its Move and the Update of the actor with movedTo are retractions, sent
// in inbound too, and nothing goes out when the mode is off. The Update of an actor that has not
// moved is the profile's, which inbound does not send. The Move goes to the followers only.
func TestMoveSends(t *testing.T) {
	const target = "https://new.example/users/alice"
	move := subject{act: federation.OutboundActivity{ObjectKind: federation.OutboundMove, Type: federation.ActivityMove}, movedTo: target}
	moved := subject{act: federation.OutboundActivity{ObjectKind: federation.OutboundAccount, Type: federation.ActivityUpdate}, movedTo: target}
	profile := subject{act: federation.OutboundActivity{ObjectKind: federation.OutboundAccount, Type: federation.ActivityUpdate}}
	for _, tc := range []struct {
		name string
		s    subject
		// off inbound outbound both
		want string
	}{
		{"Move", move, "x o o o"},
		{"Update with movedTo", moved, "x o o o"},
		{"Update of the profile", profile, "x x o o"},
	} {
		for i, m := range federation.Modes() {
			if got, want := federation.Sends(m, tc.s.act, tc.s.retracting()), tc.want[2*i] == 'o'; got != want {
				t.Errorf("%s: Sends(%s) = %v, want %v", tc.name, m, got, want)
			}
		}
	}
	if got := move.phases(); len(got) != 1 || got[0] != phaseFollowers {
		t.Errorf("phases of a Move = %v, want the followers", got)
	}
}
