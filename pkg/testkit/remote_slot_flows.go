// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"encoding/json"
	"errors"
	"maps"
	"testing"
	"time"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/federation"
)

// remoteSlotSwitchOrder switches the Like slot of two local accounts on a remote post between
// favourites and reactions, and runs the fan-outs and deliveries they enqueue through a queue
// that keeps only the order the ordering keys ask for (ADR-0004), picking the latest job it may
// and failing the first delivery twice, to an inbox that keeps one reaction per account and post
// (Misskey): whatever the queue does within those rules, the inbox ends with what each sender
// has in the slot, since the Undo of what was in it reaches the inbox before the next Like, also
// while the earlier delivery retries.
func remoteSlotSwitchOrder(t *testing.T, newHarness RemoteInteractionFactory) {
	const alice = "https://remote.example/users/alice"
	cases := []struct {
		name string
		// steps fill the slot: "favourite", "unreact", or the emoji to react with.
		steps []string
	}{
		{"FavouriteToReaction", []string{"favourite", "👍"}},
		{"ReactionToReaction", []string{"👍", "🎉"}},
		{"ReactionToFavourite", []string{"👍", "favourite"}},
		{"BackAndForth", []string{"favourite", "👍", "🎉", "favourite", "👍"}},
		{"Emptied", []string{"👍", "favourite", "🎉", "unreact"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newRemoteInteractionEnv(t, newHarness)
			ctx := t.Context()
			aliceID := e.actor(t, alice)
			fans := []string{e.user(t, "fan"), e.user(t, "other")}
			for _, fan := range fans {
				e.follow(t, fan, aliceID)
			}
			id := alice + "/notes/1"
			e.expect(t, remoteActivity(t, "Create", id+"/activity", alice, remoteNote(id, alice, remoteNow.Add(-time.Hour), nil)), appfederation.InboxHandled)
			st := e.post(t, id).Status
			// Drop the jobs of the follows.
			for ok := true; ok; _, ok = e.jobs.pop() {
				continue
			}
			for _, step := range tc.steps {
				e.clock.Advance(time.Minute)
				for _, fan := range fans {
					var err error
					switch step {
					case "favourite":
						_, err = e.interactions.Favourite(ctx, fan, st.ID)
					case "unreact":
						err = e.interactions.Unreact(ctx, fan, st.ID)
					default:
						_, err = e.interactions.React(ctx, fan, st.ID, step)
					}
					if err != nil {
						t.Fatalf("%s %s = %v", fan, step, err)
					}
				}
			}

			inbox := e.runSlotJobs(t)
			for _, fan := range fans {
				actor := e.localIRI(fan)
				want := map[string]string{}
				if e.favourited(t, fan, st.ID) {
					want[actor] = ""
				}
				for _, r := range e.reactionsOf(t, fan, st.ID) {
					want[actor] = r
				}
				got := map[string]string{}
				if s, ok := inbox[actor]; ok {
					got[actor] = s.reaction
				}
				if !maps.Equal(got, want) {
					t.Errorf("the inbox holds %q, want %q", got, want)
				}
			}
		})
	}
}

// slotEntry is the Like that an inbox of one reaction per account and post holds for an actor.
type slotEntry struct {
	likeID   federation.IRI
	reaction string
}

// runSlotJobs runs the pending fan-outs of the Like slot and the deliveries they enqueue until
// none is left, as a queue that keeps only the order of the ordering keys: it runs the latest
// job that no earlier unfinished job of its key holds back, and the first delivery fails twice
// before it succeeds. The deliveries reach an inbox that keeps one Like per actor, refuses a Like
// while it holds one, and drops the held Like on its Undo, as Misskey does. It returns what the
// inbox holds, by actor.
func (e *remoteInteractionEnv) runSlotJobs(t *testing.T) map[string]slotEntry {
	t.Helper()
	ctx := t.Context()
	fanOut, _, ok := e.handlers.Lookup(appfederation.JobFanOut)
	if !ok {
		t.Fatal("no handler for " + appfederation.JobFanOut)
	}
	type entry struct {
		job  queue.Job
		done bool
	}
	var jobs []*entry
	failing := ""
	inbox := map[string]slotEntry{}
	deliver := func(j queue.Job) queue.Result {
		if j.OrderingKey == "" {
			t.Errorf("delivery %s has no ordering key", j.Payload)
		}
		if failing == "" {
			failing = j.ID
		}
		if failing == j.ID && j.Attempt <= 2 {
			return queue.Retry(0, errors.New("the inbox is unavailable"))
		}
		var p struct {
			DeliveryID string `json:"delivery_id"`
		}
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			t.Fatal(err)
		}
		d, err := e.ih.Federation.FindDelivery(ctx, p.DeliveryID)
		if err != nil {
			t.Fatalf("FindDelivery: %v", err)
		}
		raw, err := e.ih.Federation.FindOutboundDocument(ctx, d.OutboundActivityID)
		if err != nil {
			t.Fatalf("FindOutboundDocument: %v", err)
		}
		doc, err := appfederation.Normalize(raw, e.clock.Now())
		if err != nil || doc.Activity == nil {
			t.Fatalf("Normalize(%s) = %+v, %v", raw, doc, err)
		}
		a := doc.Activity
		actor := string(a.Actor)
		held, holds := inbox[actor]
		switch a.Type {
		case federation.ActivityLike:
			if !holds {
				inbox[actor] = slotEntry{likeID: a.ID, reaction: a.Reaction}
			}
		case federation.ActivityUndo:
			embedded := a.Object().Untrusted()
			if embedded == nil || embedded.Activity == nil {
				t.Fatalf("Undo %s does not embed the Like", raw)
			}
			if holds && held.likeID == embedded.Activity.ID {
				delete(inbox, actor)
			}
		default:
			t.Fatalf("delivered %s, want a Like or an Undo", a.Type)
		}
		return queue.Done()
	}
	for range 1000 {
		for nj, ok := e.jobs.pop(); ok; nj, ok = e.jobs.pop() {
			j, err := nj.Build(e.ids.NewID(), e.clock.Now())
			if err != nil {
				t.Fatal(err)
			}
			jobs = append(jobs, &entry{job: j})
		}
		// The latest job that no earlier unfinished job of its key holds back.
		var next *entry
		for i, c := range jobs {
			if c.done {
				continue
			}
			held := false
			for _, before := range jobs[:i] {
				if !before.done && c.job.OrderingKey != "" && before.job.OrderingKey == c.job.OrderingKey {
					held = true
					break
				}
			}
			if !held {
				next = c
			}
		}
		if next == nil {
			return inbox
		}
		next.job.Attempt++
		var r queue.Result
		switch next.job.Type {
		case appfederation.JobFanOut:
			if next.job.OrderingKey == "" {
				t.Errorf("fan-out %s has no ordering key", next.job.Payload)
			}
			r = fanOut(ctx, next.job)
		case appfederation.JobDeliver:
			r = deliver(next.job)
		default:
			t.Fatalf("unexpected job %s", next.job.Type)
		}
		if r.IsFail() {
			t.Fatalf("%s %s = %v", next.job.Type, next.job.Payload, r.Err())
		}
		next.done = r.IsDone()
	}
	t.Fatal("the jobs of the Like slot did not finish")
	return nil
}
