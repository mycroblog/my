// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
)

// dataSize is the size of a seed (doc.go, The data).
type dataSize struct {
	Accounts, Follows, Statuses int
}

var dataSizes = map[string]dataSize{
	"small": {Accounts: 32, Follows: 8, Statuses: 10},
	"large": {Accounts: 200, Follows: 50, Statuses: 100},
}

// seedWorkers is how many people the seed acts for at once.
const seedWorkers = 8

// world is what the seed made and the load acts on.
type world struct {
	people   []*person
	statuses []string // the IDs of the posts, in the order they were made
	info     DataInfo
}

// seed makes the people of size on s, signs them in, and has them follow, post and favourite.
func seed(ctx context.Context, s *server, a *api, size dataSize, logf func(string, ...any)) (*world, error) {
	w := &world{}
	for i := range size.Accounts {
		w.people = append(w.people, &person{
			Username: fmt.Sprintf("bench%03d", i), Email: fmt.Sprintf("bench%03d@bench.test", i),
			Password: "bench-password-" + randomHex(8), From: fmt.Sprintf("10.%d.%d.%d", 77+i/65536, i/256%256, i%256+1),
		})
	}
	logf("making %d people", len(w.people))
	// The server keeps one admin: one apart, whom the load never acts as, so that every person of
	// the load is an ordinary user.
	if _, err := s.my(ctx, "bench-password-"+randomHex(8)+"\n", "admin", "create", "-username", "benchadmin", "-email", "admin@bench.test"); err != nil {
		return nil, err
	}
	// One at a time: each `my` process opens the database and hashes a password.
	for _, p := range w.people {
		if _, err := s.my(ctx, p.Password+"\n", "admin", "create", "-username", p.Username, "-email", p.Email); err != nil {
			return nil, err
		}
		if _, err := s.my(ctx, "", "admin", "role", "revoke", p.Username, "admin"); err != nil {
			return nil, err
		}
	}
	logf("signing in")
	err := each(ctx, w.people, func(_ int, p *person) error {
		if err := signIn(ctx, s.web, p); err != nil {
			return err
		}
		var me struct {
			ID string `json:"id"`
		}
		if err := a.must(ctx, p, "GET", "/accounts/verify_credentials", nil, &me, nil); err != nil {
			return err
		}
		p.ID = me.ID
		return nil
	})
	if err != nil {
		return nil, err
	}
	w.info.Accounts = len(w.people)

	logf("following")
	var follows atomic.Int64
	err = each(ctx, w.people, func(i int, p *person) error {
		for k := 1; k <= size.Follows && k < len(w.people); k++ {
			to := w.people[(i+k)%len(w.people)]
			if err := a.must(ctx, p, "POST", "/accounts/"+to.ID+"/follow", map[string]any{}, nil, nil); err != nil {
				return err
			}
			follows.Add(1)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	w.info.Follows = int(follows.Load())

	logf("posting %d posts", size.Accounts*size.Statuses)
	var mu sync.Mutex
	for round := range size.Statuses {
		// A round has each person post once, so that a reply has posts of others to answer.
		mu.Lock()
		earlier := append([]string(nil), w.statuses...)
		mu.Unlock()
		err = each(ctx, w.people, func(i int, p *person) error {
			n := round*len(w.people) + i
			r := rand.New(rand.NewPCG(uint64(n), 1)) //nolint:gosec // G404: the seed's choices repeat from run to run, on purpose
			body := map[string]any{"text": fmt.Sprintf("bench post %d of %s", round, p.Username), "visibility": "public", "language": "en"}
			if n%5 == 0 {
				body["text"] = body["text"].(string) + " #bench"
			}
			if n%10 == 0 {
				body["text"] = body["text"].(string) + " @" + w.people[r.IntN(len(w.people))].Username
			}
			if n%4 == 0 && len(earlier) > 0 {
				body["in_reply_to_id"] = earlier[r.IntN(len(earlier))]
			}
			var st struct {
				ID string `json:"id"`
			}
			key := map[string]string{"Idempotency-Key": fmt.Sprintf("bench-seed-%d", n)}
			if err := a.must(ctx, p, "POST", "/statuses", body, &st, key); err != nil {
				return err
			}
			mu.Lock()
			w.statuses = append(w.statuses, st.ID)
			mu.Unlock()
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	w.info.Statuses = len(w.statuses)

	logf("favouriting")
	var favourites atomic.Int64
	err = each(ctx, w.people, func(i int, p *person) error {
		r := rand.New(rand.NewPCG(uint64(i), 2)) //nolint:gosec // G404: the seed's choices repeat from run to run, on purpose
		for range size.Statuses / 2 {
			id := w.statuses[r.IntN(len(w.statuses))]
			if err := a.must(ctx, p, "POST", "/statuses/"+id+"/favourite", map[string]any{}, nil, nil); err != nil {
				return err
			}
			favourites.Add(1)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	w.info.Favourites = int(favourites.Load())
	return w, nil
}

// each runs fn for every person, seedWorkers at a time, and returns the first error.
func each(ctx context.Context, people []*person, fn func(int, *person) error) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	var next atomic.Int64
	var wg sync.WaitGroup
	for range seedWorkers {
		wg.Go(func() {
			for ctx.Err() == nil {
				i := int(next.Add(1) - 1)
				if i >= len(people) {
					return
				}
				if err := fn(i, people[i]); err != nil {
					cancel(err)
				}
			}
		})
	}
	wg.Wait()
	return context.Cause(ctx)
}
