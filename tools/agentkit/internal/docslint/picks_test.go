// SPDX-License-Identifier: AGPL-3.0-only

package docslint

import (
	"reflect"
	"strings"
	"testing"
)

const goodPick = `{"options":"button-shape","variants":["pill","soft","tight"],"pick":"soft","reason":null,"context":"pill takes room","evaluator":"fix soft (tight crams ja)","from":"20261113-132902-86d9.2","shots":".tmp/visual/button-shape","date":"2026-11-13"}`

func TestPicksLog(t *testing.T) {
	c := knowledgeConfig()
	c.Knowledge.Picks = ".claude/picks"
	note := `{"options":"button-color","variants":["muted","neutral"],"pick":null,"reason":"plain gray reads bleak","context":null,"evaluator":"ship neutral","from":"n1","shots":".tmp/visual/button-color","date":"2026-11-13"}`
	got := lint(t, c, map[string]string{
		".claude/picks/copy.jsonl":   goodPick + "\n" + note + "\n",
		".claude/picks/walker.jsonl": goodPick + "\n" + "not json\n" + goodPick + "\n",
		".claude/picks/nobody.jsonl": goodPick + "\n",
		".claude/picks/copy.md":      "# notes\n",
	})
	var picks []string
	for _, f := range got {
		if strings.HasSuffix(f, ":"+RulePicks) {
			picks = append(picks, f)
		}
	}
	want := []string{
		".claude/picks/copy.md:0:picks",
		".claude/picks/nobody.jsonl:0:picks",
		".claude/picks/walker.jsonl:2:picks",
		".claude/picks/walker.jsonl:3:picks",
	}
	if !reflect.DeepEqual(picks, want) {
		t.Errorf("findings =\n%v\nwant\n%v", strings.Join(picks, "\n"), strings.Join(want, "\n"))
	}
}

func TestPickLine(t *testing.T) {
	for ln, want := range map[string]string{
		goodPick:   "",
		`[1, 2]`:   "not a JSON object",
		`null`:     "not a JSON object",
		`{"a": 1}`: `no field "options"`,
		strings.Replace(goodPick, `"pick":"soft"`, `"pick":"round"`, 1):                           `the pick "round" is not one of the variants`,
		strings.Replace(goodPick, `"date":"2026-11-13"`, `"date":"13/11/2026"`, 1):                `the date "13/11/2026" is not YYYY-MM-DD`,
		strings.Replace(goodPick, `["pill","soft","tight"]`, `["soft","pill"]`, 1):                "the variants are not two or more distinct names in name order",
		strings.Replace(goodPick, `["pill","soft","tight"]`, `["soft","soft"]`, 1):                "the variants are not two or more distinct names in name order",
		strings.Replace(goodPick, `"reason":null`, `"reason":" "`, 1):                             "an empty reason: null when there is none",
		strings.Replace(goodPick, `"evaluator":"fix soft (tight crams ja)"`, `"evaluator":""`, 1): "an empty evaluator",
		strings.Replace(goodPick, `"shots":".tmp/visual/button-shape"`, `"shots":"/tmp/x"`, 1):    `the shots "/tmp/x" are not a clean path from the main checkout`,
		strings.Replace(goodPick, `"pick":"soft"`, `"pick":null`, 1):                              "no pick and no reason: a record without a pick keeps the decider's note as its reason",
		strings.Replace(goodPick, `"date"`, `"extra":1,"date"`, 1):                                `an unknown field "extra"`,
		strings.Replace(goodPick, `"pick":"soft"`, `"pick":3`, 1):                                 "a field of the wrong type",
	} {
		got, _ := pickLine([]byte(ln))
		if !strings.HasPrefix(got, want) || (want == "") != (got == "") {
			t.Errorf("pickLine(%s) = %q, want %q", ln, got, want)
		}
	}
}
