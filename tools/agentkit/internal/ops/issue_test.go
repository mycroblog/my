// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"errors"
	"reflect"
	"testing"
)

func TestOpenDependencies(t *testing.T) {
	orig := itemState
	t.Cleanup(func() { itemState = orig })
	states := map[string]string{"issue 1": "OPEN", "issue 2": "CLOSED", "pr 4": "OPEN", "pr 5": "MERGED"}
	itemState = func(kind, d string) (string, error) {
		if st, ok := states[kind+" "+d]; ok {
			return st, nil
		}
		return "", errors.New("not found")
	}
	got, err := openDependencies([]string{"1", "2", "4", "5"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"waits for #1, which is open", "waits for PR #4, which is open"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	// A lookup that fails for both kinds is not "no dependency".
	if got, err := openDependencies([]string{"2", "9"}); err == nil {
		t.Errorf("an unreadable dependency must be an error, got %q", got)
	}
}
