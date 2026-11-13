// SPDX-License-Identifier: AGPL-3.0-only

package docslint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// RulePicks is a log of the decider's picks out of its shape (ADR-0169 §3, §7).
const RulePicks = "picks"

// pickFields are the fields of one record of a pick, every one present: the options' id, their variants in name
// order, the variant the decider picked (null when the decider sent a note instead), the decider's reason (null when
// none was given), what else the role recorded about the pick (or null), the evaluator's first line with its reasons,
// the batch or note the pick came from, the folder of the shots from the main checkout, and the date.
var pickFields = []string{"options", "variants", "pick", "reason", "context", "evaluator", "from", "shots", "date"}

type pickRecord struct {
	Options   string   `json:"options"`
	Variants  []string `json:"variants"`
	Pick      *string  `json:"pick"`
	Reason    *string  `json:"reason"`
	Context   *string  `json:"context"`
	Evaluator string   `json:"evaluator"`
	From      string   `json:"from"`
	Shots     string   `json:"shots"`
	Date      string   `json:"date"`
}

var reName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// picks checks the logs of the picks, <Picks>/<role>.jsonl for a role of Roles: one record per line, each of the
// shape of pickFields, and each options id once. A log has no size limit: the roles do not read it.
func (l *linter) picks(files []string) ([]Finding, error) {
	k := l.c.Knowledge
	if k.Picks == "" {
		return nil, nil
	}
	var out []Finding
	for _, f := range files {
		if !strings.HasPrefix(f, k.Picks+"/") {
			continue
		}
		role, ok := strings.CutSuffix(strings.TrimPrefix(f, k.Picks+"/"), ".jsonl")
		if !ok || !slices.Contains(k.Roles, role) {
			out = append(out, Finding{f, 0, RulePicks, fmt.Sprintf("the logs of the picks are %s/<role>.jsonl, for a role of %s knowledge.roles: rename it, or delete it", k.Picks, File)})
			continue
		}
		src, err := os.ReadFile(filepath.Join(l.root, filepath.FromSlash(f)))
		if err != nil {
			return nil, err
		}
		out = append(out, picksShape(f, src)...)
	}
	return out, nil
}

// picksShape checks one log, line by line.
func picksShape(p string, src []byte) []Finding {
	var out []Finding
	seen := map[string]int{}
	shape := "one JSON object per line with " + strings.Join(pickFields, ", ")
	for i, ln := range bytes.Split(bytes.TrimSuffix(src, []byte("\n")), []byte("\n")) {
		msg, id := pickLine(ln)
		if msg == "" && id != "" {
			if first, dup := seen[id]; dup {
				msg = fmt.Sprintf("the options %s are recorded on line %d already: one record per options id", id, first)
			} else {
				seen[id] = i + 1
			}
		}
		if msg != "" {
			out = append(out, Finding{p, i + 1, RulePicks, msg + "; a log of picks holds " + shape})
		}
	}
	return out
}

// pickLine says what is wrong with one record, or "" with its options id.
func pickLine(ln []byte) (string, string) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(ln, &fields); err != nil || fields == nil {
		return "not a JSON object", ""
	}
	for _, f := range pickFields {
		if _, ok := fields[f]; !ok {
			return fmt.Sprintf("no field %q", f), ""
		}
	}
	if len(fields) != len(pickFields) {
		for f := range fields {
			if !slices.Contains(pickFields, f) {
				return fmt.Sprintf("an unknown field %q", f), ""
			}
		}
	}
	var r pickRecord
	if err := json.Unmarshal(ln, &r); err != nil {
		return "a field of the wrong type: " + err.Error(), ""
	}
	if !reName.MatchString(r.Options) {
		return fmt.Sprintf("the options id %q is not a lower-case name", r.Options), ""
	}
	if len(r.Variants) < 2 || !slices.IsSorted(r.Variants) || len(slices.Compact(slices.Clone(r.Variants))) != len(r.Variants) {
		return "the variants are not two or more distinct names in name order", ""
	}
	for _, v := range r.Variants {
		if !reName.MatchString(v) {
			return fmt.Sprintf("the variant %q is not a lower-case name", v), ""
		}
	}
	if r.Pick != nil && !slices.Contains(r.Variants, *r.Pick) {
		return fmt.Sprintf("the pick %q is not one of the variants", *r.Pick), ""
	}
	for name, s := range map[string]*string{"reason": r.Reason, "context": r.Context} {
		if s != nil && strings.TrimSpace(*s) == "" {
			return fmt.Sprintf("an empty %s: null when there is none", name), ""
		}
	}
	if r.Pick == nil && r.Reason == nil {
		return "no pick and no reason: a record without a pick keeps the decider's note as its reason", ""
	}
	for name, s := range map[string]string{"evaluator": r.Evaluator, "from": r.From} {
		if strings.TrimSpace(s) == "" {
			return "an empty " + name, ""
		}
	}
	if r.Shots == "" || path.IsAbs(r.Shots) || path.Clean(r.Shots) != r.Shots || strings.HasPrefix(r.Shots, "..") {
		return fmt.Sprintf("the shots %q are not a clean path from the main checkout", r.Shots), ""
	}
	if _, err := time.Parse(time.DateOnly, r.Date); err != nil {
		return fmt.Sprintf("the date %q is not YYYY-MM-DD", r.Date), ""
	}
	return "", r.Options
}
