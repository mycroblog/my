// SPDX-License-Identifier: AGPL-3.0-only

package gh

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// This file is the one place that knows how the repository records an issue's type, its priority, its areas, and
// what it waits for: as GitHub's issue type, the organization's Priority and Area issue fields, and the issues
// that block it, never as labels. Everything else asks through these functions. The ids of the types, the fields
// and their options are read by name, once a process.

// typeNames maps the types an issue has, as the command line spells them, to the organization's issue types.
var typeNames = map[string]string{"task": "Task", "bug": "Bug", "spike": "Spike", "adr": "ADR", "epic": "Epic"}

// TypeName is the organization's issue type of typ (one of IssueTypes).
func TypeName(typ string) string { return typeNames[typ] }

// IssueTypes are the types an issue has.
var IssueTypes = []string{"task", "bug", "spike", "adr", "epic"}

// BlockerTypes are the types of the issue `issue block` creates: a decision of the decider, or an investigation.
var BlockerTypes = []string{"adr", "spike"}

// Priorities are the priorities an issue has; the option of the Priority field is the upper-case name.
var Priorities = []string{"p0", "p1", "p2"}

const (
	priorityField = "Priority" // single-select
	areaField     = "Area"     // multi-select
)

// Blocker is an issue that blocks another (GitHub's issue dependencies).
type Blocker struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Type   string `json:"type"`
}

// Waits is the line that says an issue waits for the open blocker b, and for what kind of issue.
func (b Blocker) Waits() string {
	switch b.Type {
	case typeNames["adr"]:
		return fmt.Sprintf("waits for the decision #%d: %s", b.Number, b.Title)
	case typeNames["spike"]:
		return fmt.Sprintf("waits for the investigation #%d: %s", b.Number, b.Title)
	}
	return fmt.Sprintf("waits for #%d, which is open", b.Number)
}

// OpenBlockers are the open issues that block the issue.
func (i *Issue) OpenBlockers() []Blocker {
	var open []Blocker
	for _, b := range i.BlockedBy {
		if b.State == "OPEN" {
			open = append(open, b)
		}
	}
	return open
}

// NeedsDecision reports whether the issue waits for the decider: an open issue of type ADR blocks it.
func NeedsDecision(i *Issue) bool {
	for _, b := range i.OpenBlockers() {
		if b.Type == typeNames["adr"] {
			return true
		}
	}
	return false
}

// Unclaimable is why nobody takes an issue of this type as work, or "".
func (i *Issue) Unclaimable() string {
	switch i.Type {
	case typeNames["adr"]:
		return "an ADR issue is decided by the decider; draft the ADR in a pull request and link it"
	case typeNames["epic"]:
		return "an epic is done through its sub-issues"
	}
	return ""
}

// IsEpic reports whether the issue is an epic.
func (i *Issue) IsEpic() bool { return i.Type == typeNames["epic"] }

// Priority is the issue's priority (P0, P1, P2), or "".
func (i *Issue) Priority() string {
	if v := i.Fields[priorityField]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// Areas are the issue's areas.
func (i *Issue) Areas() []string { return i.Fields[areaField] }

// EpicFilter is the arguments of `gh issue list` that select the epics.
func EpicFilter() string { return "--search type:" + typeNames["epic"] }

// nativeSelection is the GraphQL selection of an issue's id, type and field values.
const nativeSelection = `id issueType { name } issueFieldValues(first: 50) { nodes { ` +
	`... on IssueFieldSingleSelectValue { name field { ... on IssueFieldSingleSelect { name } } } ` +
	`... on IssueFieldMultiSelectValue { options { name } field { ... on IssueFieldMultiSelect { name } } } } }`

// native is an issue as nativeSelection returns it, with the issues that block it when the query asks for them.
type native struct {
	ID        string `json:"id"`
	IssueType *struct {
		Name string `json:"name"`
	} `json:"issueType"`
	IssueFieldValues struct {
		Nodes []struct {
			Name    string `json:"name"` // the option of a single-select field
			Options []struct {
				Name string `json:"name"`
			} `json:"options"` // those of a multi-select field
			Field struct {
				Name string `json:"name"`
			} `json:"field"`
		} `json:"nodes"`
	} `json:"issueFieldValues"`
	BlockedBy struct {
		Nodes []struct {
			Number    int    `json:"number"`
			Title     string `json:"title"`
			State     string `json:"state"`
			IssueType *struct {
				Name string `json:"name"`
			} `json:"issueType"`
		} `json:"nodes"`
	} `json:"blockedBy"`
}

// apply copies the id, type, field values and blockers to i.
func (n *native) apply(i *Issue) {
	i.ID, i.Type, i.Fields, i.BlockedBy = n.ID, "", nil, nil
	if n.IssueType != nil {
		i.Type = n.IssueType.Name
	}
	for _, v := range n.IssueFieldValues.Nodes {
		if v.Field.Name == "" {
			continue
		}
		if i.Fields == nil {
			i.Fields = map[string][]string{}
		}
		if v.Name != "" {
			i.Fields[v.Field.Name] = append(i.Fields[v.Field.Name], v.Name)
		}
		for _, o := range v.Options {
			i.Fields[v.Field.Name] = append(i.Fields[v.Field.Name], o.Name)
		}
	}
	for _, b := range n.BlockedBy.Nodes {
		bl := Blocker{Number: b.Number, Title: b.Title, State: b.State}
		if b.IssueType != nil {
			bl.Type = b.IssueType.Name
		}
		i.BlockedBy = append(i.BlockedBy, bl)
	}
}

const nativeQuery = `query($owner: String!, $name: String!, $number: Int!) { repository(owner: $owner, name: $name) { issueOrPullRequest(number: $number) { ... on Issue { ` +
	nativeSelection + ` blockedBy(first: 100) { nodes { number title state issueType { name } } } } } } }`

// readNative fetches the id, type, field values and blockers of issue n into i; a pull request, which
// `gh issue view` reads too, has none.
func readNative(n int, i *Issue) error {
	owner, name, _ := strings.Cut(Repo(), "/")
	var resp struct {
		Data struct {
			Repository struct {
				Issue *native `json:"issueOrPullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := JSON(&resp, "api", "graphql", "-f", "query="+nativeQuery, "-F", "owner="+owner, "-F", "name="+name, "-F", "number="+strconv.Itoa(n)); err != nil {
		return err
	}
	if resp.Data.Repository.Issue == nil {
		return fmt.Errorf("GitHub returned no issue #%d of %s", n, Repo())
	}
	resp.Data.Repository.Issue.apply(i)
	return nil
}

// SetType records the type (one of IssueTypes) of issue n.
func SetType(n int, typ string) error {
	want := typeNames[typ]
	o, err := organization()
	if err != nil {
		return err
	}
	id, ok := o.types[want]
	if !ok {
		return fmt.Errorf("the organization %s has no issue type %q (it has %s): add it in the organization's issue types settings", o.login, want, names(o.types))
	}
	issue, err := nodeID(n)
	if err != nil {
		return err
	}
	const q = `mutation($issue: ID!, $type: ID!) { updateIssueIssueType(input: {issueId: $issue, issueTypeId: $type}) { issue { number } } }`
	if _, err := Run("api", "graphql", "-f", "query="+q, "-f", "issue="+issue, "-f", "type="+id); err != nil {
		return fmt.Errorf("%w (to do it by hand: set the type %s on the page of #%d)", err, want, n)
	}
	return nil
}

// SetPriority records the priority (one of Priorities) of issue n.
func SetPriority(n int, priority string) error {
	return setField(n, priorityField, false, strings.ToUpper(priority))
}

// Area is the name of an area as the Area field spells it: without the prefix the labels had.
func Area(a string) string { return strings.TrimPrefix(a, "area:") }

// CheckAreas fails when one of areas is not an option of the Area field, with the areas there are.
func CheckAreas(areas []string) error {
	_, _, err := fieldOptions(areaField, true, areas)
	return err
}

// SetAreas records the areas of issue n.
func SetAreas(n int, areas []string) error { return setField(n, areaField, true, areas...) }

// fieldOptions resolves the issue field and its options, all by name, to the field's id and the options' ids.
func fieldOptions(field string, multi bool, options []string) (string, []string, error) {
	o, err := organization()
	if err != nil {
		return "", nil, err
	}
	kind := "single-select"
	if multi {
		kind = "multi-select"
	}
	f, ok := o.fields[field]
	if !ok || f.multi != multi {
		return "", nil, fmt.Errorf("the organization %s has no %s issue field %q (its select fields: %s): add it in the organization's issue fields settings", o.login, kind, field, names(o.fields))
	}
	ids := make([]string, 0, len(options))
	for _, opt := range options {
		id, ok := f.options[opt]
		if !ok {
			return "", nil, fmt.Errorf("the issue field %q of the organization %s has no option %q (it has %s): pass one of those, or add it in the organization's issue fields settings",
				field, o.login, opt, names(f.options))
		}
		ids = append(ids, id)
	}
	return f.id, ids, nil
}

// setField sets the select issue field of issue n to the options, all by name.
func setField(n int, field string, multi bool, options ...string) error {
	id, opts, err := fieldOptions(field, multi, options)
	if err != nil {
		return err
	}
	issue, err := nodeID(n)
	if err != nil {
		return err
	}
	q := `mutation($issue: ID!, $field: ID!, $option: ID!) { setIssueFieldValue(input: {issueId: $issue, issueFields: [{fieldId: $field, singleSelectOptionId: $option}]}) { issue { number } } }`
	args := []string{"-f", "option=" + opts[0]}
	if multi {
		q = `mutation($issue: ID!, $field: ID!, $options: [ID!]!) { setIssueFieldValue(input: {issueId: $issue, issueFields: [{fieldId: $field, multiSelectOptionIds: $options}]}) { issue { number } } }`
		args = nil
		for _, o := range opts {
			args = append(args, "-f", "options[]="+o)
		}
	}
	args = append([]string{"api", "graphql", "-f", "query=" + q, "-f", "issue=" + issue, "-f", "field=" + id}, args...)
	if _, err := Run(args...); err != nil {
		return fmt.Errorf("%w (to do it by hand: set %s to %s on the page of #%d)", err, field, strings.Join(options, ", "), n)
	}
	return nil
}

// names is the sorted, quoted keys of m, for a message.
func names[V any](m map[string]V) string {
	if len(m) == 0 {
		return "none"
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, strconv.Quote(k))
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

type issueField struct {
	id      string
	multi   bool
	options map[string]string // option name → id
}

// org is the issue types and select issue fields of the repository's organization, by name.
type org struct {
	login  string
	types  map[string]string // type name → id
	fields map[string]issueField
}

// The answers kept for the process: the organization's settings of Repo, and the node ids of issues.
var (
	orgOf   = map[string]*org{}
	nodeIDs = map[string]string{}
)

// Forget drops what the process remembers of GitHub; tests call it between fakes.
func Forget() {
	orgOf = map[string]*org{}
	nodeIDs = map[string]string{}
}

const orgQuery = `query($login: String!) { organization(login: $login) { issueTypes(first: 100) { nodes { id name isEnabled } } issueFields(first: 100) { nodes { __typename ... on IssueFieldSingleSelect { id name options { id name } } ... on IssueFieldMultiSelect { id name options { id name } } } } } }`

// organization reads the issue types and fields of the organization that owns Repo, once.
func organization() (*org, error) {
	login, _, _ := strings.Cut(Repo(), "/")
	if o := orgOf[login]; o != nil {
		return o, nil
	}
	type named struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	var resp struct {
		Data struct {
			Organization *struct {
				IssueTypes struct {
					Nodes []struct {
						named
						IsEnabled bool `json:"isEnabled"`
					} `json:"nodes"`
				} `json:"issueTypes"`
				IssueFields struct {
					Nodes []struct {
						named
						Kind    string  `json:"__typename"`
						Options []named `json:"options"`
					} `json:"nodes"`
				} `json:"issueFields"`
			} `json:"organization"`
		} `json:"data"`
	}
	if err := JSON(&resp, "api", "graphql", "-f", "query="+orgQuery, "-f", "login="+login); err != nil {
		return nil, fmt.Errorf("could not read the issue types and issue fields of the organization %s: %w", login, err)
	}
	if resp.Data.Organization == nil {
		return nil, fmt.Errorf("%s is not an organization: issue types and issue fields are an organization's settings", login)
	}
	o := &org{login: login, types: map[string]string{}, fields: map[string]issueField{}}
	for _, t := range resp.Data.Organization.IssueTypes.Nodes {
		if t.IsEnabled {
			o.types[t.Name] = t.ID
		}
	}
	for _, f := range resp.Data.Organization.IssueFields.Nodes {
		if f.ID == "" {
			continue // a field of another kind than a select
		}
		fl := issueField{id: f.ID, multi: f.Kind == "IssueFieldMultiSelect", options: map[string]string{}}
		for _, op := range f.Options {
			fl.options[op.Name] = op.ID
		}
		o.fields[f.Name] = fl
	}
	orgOf[login] = o
	return o, nil
}

// nodeID is the GraphQL id of issue n.
func nodeID(n int) (string, error) {
	key := Repo() + "#" + strconv.Itoa(n)
	if id := nodeIDs[key]; id != "" {
		return id, nil
	}
	out, err := Run("api", fmt.Sprintf("repos/%s/issues/%d", Repo(), n), "--jq", ".node_id")
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", fmt.Errorf("GitHub returned no id for #%d of %s", n, Repo())
	}
	nodeIDs[key] = id
	return id, nil
}
