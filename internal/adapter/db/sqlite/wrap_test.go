// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
)

var errQuery = errors.New("disk I/O error")

//go:noinline
func listSomething() error { return Wrap(errQuery, "list something") }

//go:noinline
func findSomething() error { return NotFound(errQuery, errors.New("not found"), "find something") }

// A wrapped database error carries the stack of the repository method that wrapped it, so that a
// report of it starts there and two methods' errors do not share their first frames.
func TestWrapRecordsTheStackOfTheCaller(t *testing.T) {
	for want, err := range map[string]error{"sqlite.listSomething": listSomething(), "sqlite.findSomething": findSomething()} {
		if !errors.Is(err, errQuery) || !strings.HasPrefix(err.Error(), "sqlite: ") {
			t.Errorf("%s: error %v, want the wrapped query error", want, err)
		}
		stack := kernel.NewErrorEvent(err, "x").Stack
		lines := strings.SplitN(stack, "\n", 3)
		if len(lines) < 2 || !strings.Contains(lines[1], want+"(") {
			t.Errorf("%s: the stack does not start with it:\n%s", want, stack)
		}
	}
	if err := NotFound(sql.ErrNoRows, errQuery, "find something"); !errors.Is(err, errQuery) {
		t.Errorf("NotFound(ErrNoRows) = %v, want the not-found error", err)
	}
}
