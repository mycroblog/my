// SPDX-License-Identifier: AGPL-3.0-only

package gh

import (
	"fmt"
	"strconv"
)

// This file is the one place that knows how the repository records an issue's type, its priority, and whether
// it is ready: as labels. Everything else asks through these functions.

const (
	typePrefix     = "type:"
	priorityPrefix = "priority:"
	readyLabel     = "status:ready"
)

// IssueTypes are the types an issue has.
var IssueTypes = []string{"task", "bug", "spike", "adr", "epic"}

// Priorities are the priorities an issue has.
var Priorities = []string{"p0", "p1", "p2"}

// ReadyName is how a message names the ready mark.
const ReadyName = "labeled " + readyLabel

// IsEpic reports whether the issue is an epic.
func (i *Issue) IsEpic() bool { return i.HasLabel(typePrefix + "epic") }

// IsReady reports whether the issue is marked ready to take.
func (i *Issue) IsReady() bool { return i.HasLabel(readyLabel) }

// EpicFilter is the arguments of `gh issue list` that select the epics.
func EpicFilter() string { return "--label " + typePrefix + "epic" }

// SetType records the type (one of IssueTypes) of issue n.
func SetType(n int, typ string) error { return addLabel(n, typePrefix+typ) }

// SetPriority records the priority (one of Priorities) of issue n.
func SetPriority(n int, priority string) error { return addLabel(n, priorityPrefix+priority) }

// MarkReady marks issue n ready to take.
func MarkReady(n int) error { return addLabel(n, readyLabel) }

func addLabel(n int, label string) error {
	if _, err := Run("issue", "edit", strconv.Itoa(n), "-R", Repo(), "--add-label", label); err != nil {
		return fmt.Errorf("%w (to do it by hand: gh issue edit %d -R %s --add-label %s)", err, n, Repo(), label)
	}
	return nil
}
