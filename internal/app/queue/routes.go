// SPDX-License-Identifier: AGPL-3.0-only

package queue

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// WakeRoute sends the signals of the job types that Pattern matches, a type or a prefix such as
// federation.*, through Waker (ADR-0139).
type WakeRoute struct {
	Pattern string
	Waker   Waker
}

// WakeRouter is a Waker that sends each signal through the route of its job's type, or through
// the default Waker when no route matches (ADR-0139 §2). The type is used only here: what leaves
// the process is the signal the chosen Waker sends, which names the job and nothing else.
//
// A failed send is not tried on another queue, so that a type held back on its queue does not
// overflow to the others; the WakeSweeper sends it again.
type WakeRouter struct {
	fallback Waker
	routes   []WakeRoute
}

// NewWakeRouter returns the router of routes over the default Waker fallback.
func NewWakeRouter(fallback Waker, routes []WakeRoute) *WakeRouter {
	return &WakeRouter{fallback: fallback, routes: slices.Clone(routes)}
}

// routeOf returns the index of the route of jobType, or -1 for the default Waker. A name wins over
// a prefix, and a longer prefix over a shorter one.
func (r *WakeRouter) routeOf(jobType string) int {
	best, bestLength := -1, -1
	for i, route := range r.routes {
		if !matchesJobType(route.Pattern, jobType) {
			continue
		}
		if !strings.HasSuffix(route.Pattern, "*") {
			return i
		}
		if len(route.Pattern) > bestLength {
			best, bestLength = i, len(route.Pattern)
		}
	}
	return best
}

// Wake implements Waker. Every route gets its signals even when another route fails.
func (r *WakeRouter) Wake(ctx context.Context, wakes []Wake) error {
	byRoute := make(map[int][]Wake, len(r.routes)+1)
	var order []int
	for _, w := range wakes {
		i := r.routeOf(w.Type)
		if _, seen := byRoute[i]; !seen {
			order = append(order, i)
		}
		byRoute[i] = append(byRoute[i], w)
	}
	var errs []error
	for _, i := range order {
		waker := r.fallback
		if i >= 0 {
			waker = r.routes[i].Waker
		}
		if err := waker.Wake(ctx, byRoute[i]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// TypesOfRoutes returns, per route with at least one type, the registered job types that it
// receives, each sorted, for the sweep of the route. A pattern that matches no registered type is
// an error, to catch a typo or a type that a release renamed at startup.
func (r *WakeRouter) TypesOfRoutes(registered []string) ([][]string, error) {
	var unknown []string
	for _, route := range r.routes {
		if !slices.ContainsFunc(registered, func(t string) bool { return matchesJobType(route.Pattern, t) }) {
			unknown = append(unknown, route.Pattern)
		}
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("queue: no job type matches %s", strings.Join(unknown, ", "))
	}
	groups := make([][]string, len(r.routes))
	for _, t := range slices.Sorted(slices.Values(registered)) {
		if i := r.routeOf(t); i >= 0 {
			groups[i] = append(groups[i], t)
		}
	}
	return slices.DeleteFunc(groups, func(g []string) bool { return len(g) == 0 }), nil
}
