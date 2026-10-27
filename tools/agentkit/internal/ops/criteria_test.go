// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"reflect"
	"testing"
)

func TestAcceptanceCriteriaKeepsCheckedState(t *testing.T) {
	body := "## Summary\n\n- [x] not a criterion\n\n## Acceptance criteria\n\n- [ ] open item\n- [x] done item\n- [X] done upper\n\n## Notes\n\n- [ ] not a criterion either\n"
	got := acceptanceCriteria(body)
	want := []criterion{{"open item", false}, {"done item", true}, {"done upper", true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
