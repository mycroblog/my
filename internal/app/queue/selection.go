// SPDX-License-Identifier: AGPL-3.0-only

package queue

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrNoJobTypes reports a selection that leaves a worker nothing to run.
var ErrNoJobTypes = errors.New("queue: the selection leaves no job type to run")

// SelectJobTypes returns which of the registered job types a worker runs, and which it skips
// (ADR-0138 §1), both sorted. include and except are the patterns of worker_job_types and
// worker_job_types_except, at most one of them set: a pattern is a type or a prefix such as
// federation.*. A pattern that matches no registered type is an error, to catch a typo or a type
// that a release renamed at startup.
func SelectJobTypes(registered, include, except []string) (run, skip []string, err error) {
	patterns := include
	if len(patterns) == 0 {
		patterns = except
	}
	matched := make(map[string]bool, len(registered))
	var unknown []string
	for _, p := range patterns {
		found := false
		for _, t := range registered {
			if matchesJobType(p, t) {
				matched[t], found = true, true
			}
		}
		if !found {
			unknown = append(unknown, p)
		}
	}
	if len(unknown) > 0 {
		return nil, nil, fmt.Errorf("queue: no job type matches %s", strings.Join(unknown, ", "))
	}
	for _, t := range slices.Sorted(slices.Values(registered)) {
		if matched[t] == (len(include) > 0) {
			run = append(run, t)
		} else {
			skip = append(skip, t)
		}
	}
	if len(run) == 0 {
		return nil, nil, ErrNoJobTypes
	}
	return run, skip, nil
}

func matchesJobType(pattern, jobType string) bool {
	if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
		return strings.HasPrefix(jobType, prefix)
	}
	return pattern == jobType
}
