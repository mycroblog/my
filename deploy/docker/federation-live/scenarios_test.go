// SPDX-License-Identifier: AGPL-3.0-only

//go:build federationlive

package federationlive

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

const localHandle = localUser + "@" + localHost

// TestPeerResolvesLocal: each peer finds alice@my.test through WebFinger and fetches her actor
// (Mastodon in secure mode signs the fetch, GoToSocial always does).
func TestPeerResolvesLocal(t *testing.T) {
	e := stack(t)
	for _, p := range e.peers {
		t.Run(p.name, func(t *testing.T) {
			a, err := p.resolve(t.Context(), localHandle)
			if err != nil {
				t.Fatal(err)
			}
			if a.Username != localUser || a.Acct != localHandle {
				t.Errorf("%s resolved %s as %+v", p.host, localHandle, a)
			}
		})
	}
}

// TestLocalResolvesPeer: this server finds each peer user through WebFinger and fetches the actor
// with a GET signed by its instance actor (federation.md §2, §6), which Mastodon's secure mode and
// GoToSocial require.
func TestLocalResolvesPeer(t *testing.T) {
	e := stack(t)
	for _, p := range e.peers {
		t.Run(p.name, func(t *testing.T) {
			a, err := e.local.resolve(t.Context(), p.handle())
			if err != nil {
				t.Fatal(err)
			}
			if a.Username != p.user || a.Acct != p.handle() || !strings.HasPrefix(a.URI, "https://"+p.host+"/") {
				t.Errorf("%s resolved %s as %+v", localHost, p.handle(), a)
			}
		})
	}
}

// TestFollowFromPeer: a peer user follows alice. The signed Follow reaches the inbox of this
// server, which verifies and records it (federation.md §3.1); then this server accepts it and the
// peer shows the follow.
func TestFollowFromPeer(t *testing.T) {
	e := stack(t)
	for _, p := range e.peers {
		t.Run(p.name, func(t *testing.T) {
			ctx := t.Context()
			actor, err := e.local.resolve(ctx, p.handle())
			if err != nil {
				t.Fatal(err)
			}
			alice, err := p.resolve(ctx, localHandle)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.follow(ctx, alice.ID); err != nil {
				t.Fatal(err)
			}

			t.Run("delivered to the inbox", func(t *testing.T) {
				eventually(t, deliveryTimeout, "the Follow of "+actor.URI+" in received_activities", func() (bool, string) {
					got := e.receivedActivities(t, actor.URI)
					for _, a := range got {
						if a.Type == "Follow" {
							if a.Trust != "signed" {
								t.Errorf("Follow %s: trust = %s, want signed", a.URI, a.Trust)
							}
							t.Logf("Follow %s signed with %s", a.URI, a.SignatureScheme)
							return true, ""
						}
					}
					return false, fmt.Sprintf("%+v", got)
				})
			})

			t.Run("accepted", func(t *testing.T) {
				eventually(t, deliveryTimeout, p.host+" follows "+localHandle, func() (bool, string) {
					r, err := p.relationship(ctx, alice.ID)
					return err == nil && r.Following, fmt.Sprintf("%+v %v", r, err)
				})
				eventually(t, deliveryTimeout, localHost+" is followed by "+p.handle(), func() (bool, string) {
					r, err := e.local.relationship(ctx, actor.ID)
					return err == nil && r.FollowedBy, fmt.Sprintf("%+v %v", r, err)
				})
			})
		})
	}
}

// TestFollowFromLocal: alice follows a peer user, the peer accepts, and both sides show the
// follow.
func TestFollowFromLocal(t *testing.T) {
	e := stack(t)
	for _, p := range e.peers {
		t.Run(p.name, func(t *testing.T) {
			ctx := t.Context()
			followLocalToPeer(t, e, p)
			alice, err := p.resolve(ctx, localHandle)
			if err != nil {
				t.Fatal(err)
			}
			eventually(t, deliveryTimeout, p.handle()+" is followed by "+localHandle, func() (bool, string) {
				r, err := p.relationship(ctx, alice.ID)
				return err == nil && r.FollowedBy, fmt.Sprintf("%+v %v", r, err)
			})
		})
	}
}

// followLocalToPeer makes alice follow the peer user and waits for the Accept, and returns the
// peer user's account on this server.
func followLocalToPeer(t *testing.T, e *env, p *peerClient) account {
	t.Helper()
	ctx := t.Context()
	actor, err := e.local.resolve(ctx, p.handle())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.local.follow(ctx, actor.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, deliveryTimeout, localHandle+" follows "+p.handle(), func() (bool, string) {
		r, err := e.local.relationship(ctx, actor.ID)
		return err == nil && r.Following, fmt.Sprintf("%+v %v", r, err)
	})
	return actor
}

// followPeerToLocal makes the peer user follow alice and waits until the peer shows the follow.
func followPeerToLocal(t *testing.T, p *peerClient) {
	t.Helper()
	ctx := t.Context()
	alice, err := p.resolve(ctx, localHandle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.follow(ctx, alice.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, deliveryTimeout, p.handle()+" follows "+localHandle, func() (bool, string) {
		r, err := p.relationship(ctx, alice.ID)
		return err == nil && r.Following, fmt.Sprintf("%+v %v", r, err)
	})
}

// mutual makes alice and the peer user follow each other and returns the peer user's account on
// this server and alice's account on the peer.
func mutual(t *testing.T, e *env, p *peerClient) (account, account) {
	t.Helper()
	actor := followLocalToPeer(t, e, p)
	followPeerToLocal(t, p)
	alice, err := p.resolve(t.Context(), localHandle)
	if err != nil {
		t.Fatal(err)
	}
	return actor, alice
}

// localSees waits until a status reaches alice's home timeline and returns it.
func localSees(t *testing.T, e *env, uri string) status {
	t.Helper()
	var found status
	eventually(t, deliveryTimeout, uri+" in the home timeline of "+localHandle, func() (bool, string) {
		items, err := e.local.home(t.Context())
		s, ok := findStatus(items, uri)
		found = s
		return ok, fmt.Sprintf("%d statuses %v", len(items), err)
	})
	return found
}

// peerSees waits until a status reaches the peer user's home timeline and returns it.
func peerSees(t *testing.T, p *peerClient, uri string) status {
	t.Helper()
	var found status
	eventually(t, deliveryTimeout, uri+" in the home timeline of "+p.handle(), func() (bool, string) {
		items, err := p.home(t.Context())
		s, ok := findStatus(items, uri)
		found = s
		return ok, fmt.Sprintf("%d statuses %v", len(items), err)
	})
	return found
}

// never checks that check stays false for a while, after the delivery that would have made it true
// has had the time to land (the caller waits for a later delivery to the same server first).
func never(t *testing.T, what string, check func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(quietPeriod)
	for {
		if happened, state := check(); happened {
			t.Errorf("%s: %s", what, state)
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(2 * time.Second)
	}
}

// TestPostFromPeer: a public post of a followed peer user reaches alice's home timeline.
func TestPostFromPeer(t *testing.T) {
	e := stack(t)
	for _, p := range e.peers {
		t.Run(p.name, func(t *testing.T) {
			followLocalToPeer(t, e, p)
			posted, err := p.post(t.Context(), unique("hello from "+p.name), "public")
			if err != nil {
				t.Fatal(err)
			}
			localSees(t, e, posted.URI)
		})
	}
}

// TestPostFromLocal: a public post of alice reaches the home timeline of a peer user who follows
// her.
func TestPostFromLocal(t *testing.T) {
	e := stack(t)
	for _, p := range e.peers {
		t.Run(p.name, func(t *testing.T) {
			followPeerToLocal(t, p)
			posted, err := e.local.post(t.Context(), unique("hello to "+p.name), "public")
			if err != nil {
				t.Fatal(err)
			}
			peerSees(t, p, posted.URI)
		})
	}
}

// TestUnfollow: an unfollow in either direction (Undo Follow) ends the follow on the other side.
func TestUnfollow(t *testing.T) {
	e := stack(t)
	for _, p := range e.peers {
		t.Run(p.name, func(t *testing.T) {
			ctx := t.Context()
			actor, alice := mutual(t, e, p)

			t.Run("from local", func(t *testing.T) {
				if err := e.local.unfollow(ctx, actor.ID); err != nil {
					t.Fatal(err)
				}
				eventually(t, deliveryTimeout, p.handle()+" is no longer followed by "+localHandle, func() (bool, string) {
					r, err := p.relationship(ctx, alice.ID)
					return err == nil && !r.FollowedBy, fmt.Sprintf("%+v %v", r, err)
				})
			})
			t.Run("from peer", func(t *testing.T) {
				if err := p.unfollow(ctx, alice.ID); err != nil {
					t.Fatal(err)
				}
				eventually(t, deliveryTimeout, localHandle+" is no longer followed by "+p.handle(), func() (bool, string) {
					r, err := e.local.relationship(ctx, actor.ID)
					return err == nil && !r.FollowedBy, fmt.Sprintf("%+v %v", r, err)
				})
			})
		})
	}
}

// TestFollowApproval: a follow of a locked account waits for its approval, and the approval
// (Accept) reaches the follower, in both directions. It starts from TestUnfollow's state, where
// neither follows the other.
func TestFollowApproval(t *testing.T) {
	e := stack(t)
	for _, p := range e.peers {
		t.Run(p.name, func(t *testing.T) {
			ctx := t.Context()
			actor, err := e.local.resolve(ctx, p.handle())
			if err != nil {
				t.Fatal(err)
			}
			alice, err := p.resolve(ctx, localHandle)
			if err != nil {
				t.Fatal(err)
			}

			t.Run("peer approves", func(t *testing.T) {
				if err := p.setLocked(ctx, true); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = p.setLocked(context.Background(), false) })
				if r, err := e.local.relationship(ctx, actor.ID); err != nil || r.Following {
					t.Fatalf("%s already follows %s: %+v %v", localHandle, p.handle(), r, err)
				}
				if _, err := e.local.follow(ctx, actor.ID); err != nil {
					t.Fatal(err)
				}
				eventually(t, deliveryTimeout, "the follow request of "+localHandle+" on "+p.host, func() (bool, string) {
					as, err := p.followRequests(ctx)
					for _, a := range as {
						if a.ID == alice.ID {
							return true, ""
						}
					}
					return false, fmt.Sprintf("%+v %v", as, err)
				})
				if r, err := e.local.relationship(ctx, actor.ID); err != nil || r.Following || !r.Requested {
					t.Errorf("before the approval: %+v %v", r, err)
				}
				if err := p.authorize(ctx, alice.ID); err != nil {
					t.Fatal(err)
				}
				eventually(t, deliveryTimeout, localHandle+" follows "+p.handle(), func() (bool, string) {
					r, err := e.local.relationship(ctx, actor.ID)
					return err == nil && r.Following, fmt.Sprintf("%+v %v", r, err)
				})
			})

			t.Run("local approves", func(t *testing.T) {
				if err := e.local.setLocked(ctx, true); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = e.local.setLocked(context.Background(), false) })
				// alice has no follower on the peer now (TestUnfollow), so the peer does not get the
				// Update of her actor and still sees her unlocked: the approval is this server's.
				if r, err := p.relationship(ctx, alice.ID); err != nil || r.Following {
					t.Fatalf("%s already follows %s: %+v %v", p.handle(), localHandle, r, err)
				}
				if _, err := p.follow(ctx, alice.ID); err != nil {
					t.Fatal(err)
				}
				eventually(t, deliveryTimeout, "the follow request of "+p.handle()+" on "+localHost, func() (bool, string) {
					as, err := e.local.followRequests(ctx)
					for _, a := range as {
						if a.ID == actor.ID {
							return true, ""
						}
					}
					return false, fmt.Sprintf("%+v %v", as, err)
				})
				if r, err := p.relationship(ctx, alice.ID); err != nil || r.Following {
					t.Errorf("before the approval: %+v %v", r, err)
				}
				if err := e.local.authorize(ctx, actor.ID); err != nil {
					t.Fatal(err)
				}
				eventually(t, deliveryTimeout, p.handle()+" follows "+localHandle, func() (bool, string) {
					r, err := p.relationship(ctx, alice.ID)
					return err == nil && r.Following, fmt.Sprintf("%+v %v", r, err)
				})
			})
		})
	}
}

// TestReply: replies in both directions keep the thread: each side shows the reply among the
// descendants of the status it replies to, and the reply to alice notifies her.
func TestReply(t *testing.T) {
	e := stack(t)
	for _, p := range e.peers {
		t.Run(p.name, func(t *testing.T) {
			ctx := t.Context()
			mutual(t, e, p)

			t.Run("from peer", func(t *testing.T) {
				original, err := e.local.post(ctx, unique("reply to me, "+p.name), "public")
				if err != nil {
					t.Fatal(err)
				}
				onPeer, err := p.resolveStatus(ctx, original.URI)
				if err != nil {
					t.Fatal(err)
				}
				reply, err := p.reply(ctx, unique("@"+localHandle+" a reply from "+p.name), "public", onPeer.ID)
				if err != nil {
					t.Fatal(err)
				}
				eventually(t, deliveryTimeout, reply.URI+" in the thread of "+original.URI, func() (bool, string) {
					th, err := e.local.context(ctx, original.ID)
					s, ok := findStatus(th.Descendants, reply.URI)
					return ok && s.InReplyToID == original.ID, fmt.Sprintf("%+v %v", th.Descendants, err)
				})
				eventually(t, deliveryTimeout, "the mention notification of "+reply.URI, func() (bool, string) {
					ns, err := e.local.notifications(ctx)
					for _, n := range ns {
						if n.Type == "mention" && n.Status != nil && n.Status.URI == reply.URI {
							return true, ""
						}
					}
					return false, fmt.Sprintf("%d notifications %v", len(ns), err)
				})
			})

			t.Run("from local", func(t *testing.T) {
				original, err := p.post(ctx, unique("reply to me, "+localHost), "public")
				if err != nil {
					t.Fatal(err)
				}
				onLocal := localSees(t, e, original.URI)
				reply, err := e.local.reply(ctx, unique("@"+p.handle()+" a reply from "+localHost), "public", onLocal.ID)
				if err != nil {
					t.Fatal(err)
				}
				eventually(t, deliveryTimeout, reply.URI+" in the thread of "+original.URI, func() (bool, string) {
					th, err := p.context(ctx, original.ID)
					s, ok := findStatus(th.Descendants, reply.URI)
					return ok && s.InReplyToID == original.ID, fmt.Sprintf("%+v %v", th.Descendants, err)
				})
			})
		})
	}
}

// TestBoost: boosts and their undo in both directions (Announce, Undo Announce) change the
// count on the author's side.
func TestBoost(t *testing.T) {
	interactionScenario(t, "boost", func(s status) int { return s.BoostsCount + s.ReblogsCount })
}

// TestFavourite: favourites and their undo in both directions (Like, Undo Like) change the count
// on the author's side. A Misskey favourite is a reaction (a Like with _misskey_reaction): this
// server counts it among the reactions with its emoji, not among the favourites, and notifies it
// as a reaction (ADR-0051).
func TestFavourite(t *testing.T) {
	interactionScenario(t, "favourite", func(s status) int { return s.FavouritesCount })
}

func interactionScenario(t *testing.T, action string, count func(status) int) {
	t.Helper()
	e := stack(t)
	for _, p := range e.peers {
		t.Run(p.name, func(t *testing.T) {
			ctx := t.Context()
			mutual(t, e, p)

			t.Run("from peer", func(t *testing.T) {
				original, err := e.local.post(ctx, unique(action+" me, "+p.name), "public")
				if err != nil {
					t.Fatal(err)
				}
				onPeer, err := p.resolveStatus(ctx, original.URI)
				if err != nil {
					t.Fatal(err)
				}
				// A peer whose favourite is an emoji reaction (Misskey) makes a reaction on this
				// server, and no favourite (ADR-0051).
				what, got := action, count
				emoji := ""
				if action == "favourite" && p.favouriteReaction != "" {
					emoji = p.favouriteReaction
					what = emoji + " reaction"
					got = func(s status) int { return s.reactionCount(emoji) }
				}
				for _, undo := range []bool{false, true} {
					if err := p.interact(ctx, onPeer.ID, action, undo); err != nil {
						t.Fatal(err)
					}
					want := map[bool]int{false: 1, true: 0}[undo]
					eventually(t, deliveryTimeout, fmt.Sprintf("the %s count of %s on %s is %d", what, original.URI, localHost, want), func() (bool, string) {
						s, err := e.local.status(ctx, original.ID)
						ok := err == nil && got(s) == want && (emoji == "" || s.FavouritesCount == 0)
						return ok, fmt.Sprintf("%d (favourites %d, reactions %+v) %v", got(s), s.FavouritesCount, s.Reactions, err)
					})
					if emoji != "" && !undo {
						eventually(t, deliveryTimeout, "the "+what+" notification of "+original.URI, func() (bool, string) {
							ns, err := e.local.notifications(ctx)
							for _, n := range ns {
								if n.Type == "reaction" && n.Status != nil && n.Status.URI == original.URI &&
									n.Reaction != nil && n.Reaction.Name == emoji {
									return true, ""
								}
							}
							return false, fmt.Sprintf("%d notifications %v", len(ns), err)
						})
					}
				}
			})

			t.Run("from local", func(t *testing.T) {
				original, err := p.post(ctx, unique(action+" me, "+localHost), "public")
				if err != nil {
					t.Fatal(err)
				}
				onLocal := localSees(t, e, original.URI)
				for _, undo := range []bool{false, true} {
					if err := e.local.interact(ctx, onLocal.ID, action, undo); err != nil {
						t.Fatal(err)
					}
					want := map[bool]int{false: 1, true: 0}[undo]
					eventually(t, deliveryTimeout, fmt.Sprintf("the %s count of %s on %s is %d", action, original.URI, p.host, want), func() (bool, string) {
						s, err := p.status(ctx, original.ID)
						return err == nil && count(s) == want, fmt.Sprintf("%d %v", count(s), err)
					})
				}
			})
		})
	}
}

// TestReaction: an emoji reaction from this server (a Like with content and _misskey_reaction,
// ADR-0051 §8) is a reaction with the emoji on a peer whose favourite is a reaction (Misskey), and
// a favourite on the others (Mastodon, GoToSocial); its Undo takes it back. Changing the emoji
// sends the Undo of the first one before the new Like, so the peer keeps one.
func TestReaction(t *testing.T) {
	e := stack(t)
	for _, p := range e.peers {
		t.Run(p.name, func(t *testing.T) {
			ctx := t.Context()
			mutual(t, e, p)
			original, err := p.post(ctx, unique("react to me, "+localHost), "public")
			if err != nil {
				t.Fatal(err)
			}
			onLocal := localSees(t, e, original.URI)
			for _, emoji := range []string{"👍", "🎉", ""} {
				if err := e.local.react(ctx, onLocal.ID, emoji); err != nil {
					t.Fatal(err)
				}
				want := map[bool]int{true: 0, false: 1}[emoji == ""]
				eventually(t, deliveryTimeout, fmt.Sprintf("the %q reaction to %s on %s", emoji, original.URI, p.host), func() (bool, string) {
					s, err := p.status(ctx, original.ID)
					ok := err == nil && s.FavouritesCount == want
					if p.favouriteReaction != "" && emoji != "" {
						ok = ok && s.reactionCount(emoji) == 1
					}
					return ok, fmt.Sprintf("favourites %d, reactions %+v, %v", s.FavouritesCount, s.Reactions, err)
				})
			}
		})
	}
}

// TestEdit: an edited post (Update) shows the new text on the other side. A peer that does not
// edit (noEdits) gets the Update and keeps the text.
func TestEdit(t *testing.T) {
	e := stack(t)
	for _, p := range e.peers {
		t.Run(p.name, func(t *testing.T) {
			ctx := t.Context()
			mutual(t, e, p)
			if p.noEdits != "" {
				editIgnored(t, e, p)
				return
			}

			t.Run("from peer", func(t *testing.T) {
				original, err := p.post(ctx, unique("before the edit on "+p.name), "public")
				if err != nil {
					t.Fatal(err)
				}
				onLocal := localSees(t, e, original.URI)
				text := unique("after the edit on " + p.name)
				if _, err := p.edit(ctx, original.ID, text); err != nil {
					t.Fatal(err)
				}
				eventually(t, deliveryTimeout, "the edit of "+original.URI+" on "+localHost, func() (bool, string) {
					s, err := e.local.status(ctx, onLocal.ID)
					return err == nil && strings.Contains(s.Content, text), fmt.Sprintf("%q %v", s.Content, err)
				})
			})

			t.Run("from local", func(t *testing.T) {
				original, err := e.local.post(ctx, unique("before the edit on "+localHost), "public")
				if err != nil {
					t.Fatal(err)
				}
				onPeer := peerSees(t, p, original.URI)
				text := unique("after the edit on " + localHost)
				if _, err := e.local.edit(ctx, original.ID, text); err != nil {
					t.Fatal(err)
				}
				eventually(t, deliveryTimeout, "the edit of "+original.URI+" on "+p.host, func() (bool, string) {
					s, err := p.status(ctx, onPeer.ID)
					return err == nil && strings.Contains(s.Content, text), fmt.Sprintf("%q %v", s.Content, err)
				})
			})
		})
	}
}

// editIgnored is TestEdit for a peer that does not edit: the Update of alice's edit is delivered to
// the peer's shared inbox (/inbox, as Misskey names it) and accepted, and the peer still shows the
// text before the edit.
func editIgnored(t *testing.T, e *env, p *peerClient) {
	t.Helper()
	t.Run("from peer", func(t *testing.T) { t.Skip(p.noEdits) })
	t.Run("from local", func(t *testing.T) {
		ctx := t.Context()
		before := unique("before the edit on " + localHost)
		original, err := e.local.post(ctx, before, "public")
		if err != nil {
			t.Fatal(err)
		}
		onPeer := peerSees(t, p, original.URI)
		inbox := "https://" + p.host + "/inbox"
		succeeded := func() int {
			n := 0
			for _, a := range e.deliveryAttempts(t, inbox) {
				if a.Outcome == "succeeded" {
					n++
				}
			}
			return n
		}
		delivered := succeeded()
		if _, err := e.local.edit(ctx, original.ID, unique("after the edit on "+localHost)); err != nil {
			t.Fatal(err)
		}
		eventually(t, deliveryTimeout, "the Update of "+original.URI+" delivered to "+inbox, func() (bool, string) {
			n := succeeded()
			return n > delivered, fmt.Sprintf("%d succeeded deliveries, %d before the edit", n, delivered)
		})
		s, err := p.status(ctx, onPeer.ID)
		if err != nil || s.Content != before {
			t.Errorf("after the edit %s shows %q %v, want the text before the edit %q", p.host, s.Content, err, before)
		}
	})
}

// TestDelete: a deleted post (Delete) disappears on the other side.
func TestDelete(t *testing.T) {
	e := stack(t)
	for _, p := range e.peers {
		t.Run(p.name, func(t *testing.T) {
			ctx := t.Context()
			mutual(t, e, p)

			t.Run("from peer", func(t *testing.T) {
				original, err := p.post(ctx, unique("to be deleted on "+p.name), "public")
				if err != nil {
					t.Fatal(err)
				}
				onLocal := localSees(t, e, original.URI)
				if err := p.delete(ctx, original.ID); err != nil {
					t.Fatal(err)
				}
				eventually(t, deliveryTimeout, original.URI+" is gone from "+localHost, func() (bool, string) {
					_, err := e.local.status(ctx, onLocal.ID)
					return isNotFound(err), fmt.Sprintf("%v", err)
				})
			})

			t.Run("from local", func(t *testing.T) {
				original, err := e.local.post(ctx, unique("to be deleted on "+localHost), "public")
				if err != nil {
					t.Fatal(err)
				}
				onPeer := peerSees(t, p, original.URI)
				if err := e.local.delete(ctx, original.ID); err != nil {
					t.Fatal(err)
				}
				eventually(t, deliveryTimeout, original.URI+" is gone from "+p.host, func() (bool, string) {
					_, err := p.status(ctx, onPeer.ID)
					return isNotFound(err), fmt.Sprintf("%v", err)
				})
			})
		})
	}
}

// TestVisibilityAudiences: each visibility reaches exactly its audience, in both directions.
//
// alice posts with every visibility, mentioning the peer's stranger (a user who never follows
// her); the peer user follows her. public, unlisted, and private reach both, direct reaches the
// stranger only, and local reaches the follower only: the stranger can neither see it nor gets a
// notification, although the post reaches the stranger's server (ADR-0017). Then the
// peer user posts with every visibility of the Mastodon client API, mentioning alice in the
// direct one, and each reaches alice with that visibility.
func TestVisibilityAudiences(t *testing.T) {
	e := stack(t)
	for _, p := range e.peers {
		t.Run(p.name, func(t *testing.T) {
			ctx := t.Context()
			mutual(t, e, p)
			s := p.stranger
			if _, err := e.local.resolve(ctx, s.handle()); err != nil {
				t.Fatal(err)
			}

			t.Run("from local", func(t *testing.T) {
				// visibility on this server → visibility on the peer, and who sees it.
				cases := []struct {
					visibility, onPeer string
					follower, stranger bool
				}{
					{"public", "public", true, true},
					{"unlisted", "unlisted", true, true},
					{"private", "private", true, true},
					{"direct", "direct", false, true},
					{"local", "private", true, false},
				}
				posted := map[string]status{}
				for _, c := range cases {
					st, err := e.local.post(ctx, unique("@"+s.handle()+" "+c.visibility+" from "+localHost), c.visibility)
					if err != nil {
						t.Fatal(err)
					}
					posted[c.visibility] = st
				}
				for _, c := range cases {
					st := posted[c.visibility]
					if c.follower {
						got := peerSees(t, p, st.URI)
						if got.Visibility != c.onPeer {
							t.Errorf("%s: %s sees it as %s, want %s", c.visibility, p.handle(), got.Visibility, c.onPeer)
						}
					}
					if c.stranger {
						eventually(t, deliveryTimeout, "the mention of "+s.handle()+" in the "+c.visibility+" post", func() (bool, string) {
							n, ok := mentionOf(ctx, s, st.URI)
							return ok, fmt.Sprintf("%+v", n)
						})
						got, err := s.resolveStatus(ctx, st.URI)
						if err != nil {
							t.Errorf("%s: %s cannot see it: %v", c.visibility, s.handle(), err)
						} else if got.Visibility != c.onPeer {
							t.Errorf("%s: %s sees it as %s, want %s", c.visibility, s.handle(), got.Visibility, c.onPeer)
						}
					}
				}
				// Every post has reached the peer by now; the ones that must not show must stay hidden.
				for _, c := range cases {
					st := posted[c.visibility]
					if !c.follower {
						never(t, c.visibility+": in the home timeline of "+p.handle(), func() (bool, string) {
							items, _ := p.home(ctx)
							_, ok := findStatus(items, st.URI)
							return ok, st.URI
						})
					}
					if !c.stranger {
						never(t, c.visibility+": "+s.handle()+" is notified", func() (bool, string) {
							n, ok := mentionOf(ctx, s, st.URI)
							return ok, fmt.Sprintf("%+v", n)
						})
						if got, err := s.resolveStatus(ctx, st.URI); err == nil {
							t.Errorf("%s: %s can see it: %+v", c.visibility, s.handle(), got)
						}
					}
				}
			})

			t.Run("from peer", func(t *testing.T) {
				for _, visibility := range []string{"public", "unlisted", "private", "direct"} {
					text := unique(visibility + " from " + p.host)
					if visibility == "direct" {
						text = "@" + localHandle + " " + text
					}
					st, err := p.post(ctx, text, visibility)
					if err != nil {
						t.Fatal(err)
					}
					var got status
					if visibility == "direct" {
						eventually(t, deliveryTimeout, "the mention of "+localHandle+" in "+st.URI, func() (bool, string) {
							ns, err := e.local.notifications(ctx)
							for _, n := range ns {
								if n.Type == "mention" && n.Status != nil && n.Status.URI == st.URI {
									got = *n.Status
									return true, ""
								}
							}
							return false, fmt.Sprintf("%d notifications %v", len(ns), err)
						})
					} else {
						got = localSees(t, e, st.URI)
					}
					if got.Visibility != visibility {
						t.Errorf("%s: %s sees it as %s", visibility, localHandle, got.Visibility)
					}
				}
			})
		})
	}
}

// mentionOf returns the notification of the peer user about the status, if there is one.
func mentionOf(ctx context.Context, p *peerClient, uri string) (notification, bool) {
	ns, err := p.notifications(ctx)
	if err != nil {
		return notification{}, false
	}
	for _, n := range ns {
		if n.Status != nil && n.Status.URI == uri {
			return n, true
		}
	}
	return notification{}, false
}

// TestReportForwarding: a report of a peer user that alice asks to forward reaches the moderators'
// list of the peer as a Flag of this server's instance actor, with the comment; one that she does
// not ask to forward does not (FR-709, federation.md §11).
func TestReportForwarding(t *testing.T) {
	e := stack(t)
	for _, p := range e.peers {
		t.Run(p.name, func(t *testing.T) {
			ctx := t.Context()
			if _, why, _ := peerReportComments(ctx, e, p); why != "" {
				t.Skip(why)
			}
			actor := followLocalToPeer(t, e, p)
			posted, err := p.post(ctx, unique("report me on "+p.name), "public")
			if err != nil {
				t.Fatal(err)
			}
			seen := localSees(t, e, posted.URI)

			kept := unique("kept on " + localHost)
			if forwarded, err := e.local.report(ctx, actor.ID, []string{seen.ID}, kept, false); err != nil || forwarded {
				t.Fatalf("report without forwarding = %v, %v", forwarded, err)
			}
			sent := unique("forwarded from " + localHost)
			if forwarded, err := e.local.report(ctx, actor.ID, []string{seen.ID}, sent, true); err != nil || !forwarded {
				t.Fatalf("report with forwarding = %v, %v", forwarded, err)
			}
			has := func(comments []string, text string) bool {
				return slices.ContainsFunc(comments, func(c string) bool { return strings.Contains(c, text) })
			}
			eventually(t, deliveryTimeout, "the forwarded report in the reports of "+p.host, func() (bool, string) {
				comments, _, err := peerReportComments(ctx, e, p)
				return err == nil && has(comments, sent), fmt.Sprintf("%q %v", comments, err)
			})
			comments, _, err := peerReportComments(ctx, e, p)
			if err != nil {
				t.Fatal(err)
			}
			if has(comments, kept) {
				t.Errorf("reports of %s = %q, want no report that alice did not forward", p.host, comments)
			}
		})
	}
}

// TestSignatureProfile checks the rows of ADR-0006 that only a real server shows:
//
//   - every peer signs its deliveries with draft-cavage first, and this server verifies them;
//   - a Mastodon delivery that the inbox refuses with 401 comes back signed with RFC 9421, which
//     this server verifies (the proxy refuses eve's draft-cavage POSTs, Caddyfile);
//   - this server's delivery that the inbox refuses with 401 is resent with RFC 9421 and
//     Mastodon accepts it (double-knocking; the proxy refuses draft-cavage POSTs to eve's inbox);
//   - an unsigned POST to the inbox is a 401 with Accept-Signature.
//
// It runs last: from here on this server remembers to sign for mastodon.test with RFC 9421 first.
func TestSignatureProfile(t *testing.T) {
	e := stack(t)

	t.Run("draft-cavage from every peer", func(t *testing.T) {
		for _, p := range e.peers {
			actor, err := e.local.resolve(t.Context(), p.handle())
			if err != nil {
				t.Fatal(err)
			}
			got := e.receivedActivities(t, actor.URI)
			if len(got) == 0 {
				t.Errorf("nothing received from %s", actor.URI)
			}
			for _, a := range got {
				if a.Trust != "signed" || a.SignatureScheme != "cavage" {
					t.Errorf("%s %s from %s: %s / %s, want signed / cavage", a.Type, a.URI, p.host, a.Trust, a.SignatureScheme)
				}
			}
		}
	})

	m := e.peers[0].signer
	t.Run("RFC 9421 both ways with mastodon", func(t *testing.T) {
		ctx := t.Context()
		actor, err := e.local.resolve(ctx, m.handle())
		if err != nil {
			t.Fatal(err)
		}
		// eve's Follow reaches this server with RFC 9421; this server's Accept reaches eve's
		// inbox with RFC 9421 after the 401.
		followPeerToLocal(t, m)
		got := e.receivedActivities(t, actor.URI)
		if len(got) == 0 {
			t.Fatalf("nothing received from %s", actor.URI)
		}
		for _, a := range got {
			if a.Trust != "signed" || a.SignatureScheme != "rfc9421" {
				t.Errorf("%s %s: %s / %s, want signed / rfc9421", a.Type, a.URI, a.Trust, a.SignatureScheme)
			}
		}
		inbox := "https://" + mastodonHost + os.Getenv("FEDERATION_LIVE_SIGNER_INBOX")
		attempts := e.deliveryAttempts(t, inbox)
		ok := false
		for _, a := range attempts {
			ok = ok || (a.Outcome == "succeeded" && a.SignatureScheme == "rfc9421")
		}
		if !ok {
			t.Errorf("no delivery to %s succeeded with rfc9421: %+v", inbox, attempts)
		}
	})

	t.Run("Accept-Signature on an unsigned POST", func(t *testing.T) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://"+localHost+"/ap/inbox",
			strings.NewReader(`{"type":"Follow"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/activity+json")
		resp, err := e.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("Accept-Signature") == "" {
			t.Errorf("unsigned POST = %d, Accept-Signature %q; want 401 with Accept-Signature",
				resp.StatusCode, resp.Header.Get("Accept-Signature"))
		}
	})
}
