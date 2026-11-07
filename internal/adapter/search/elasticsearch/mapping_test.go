// SPDX-License-Identifier: AGPL-3.0-only

package elasticsearch

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// cluster is a stand-in of the alias and index APIs: the alias points at alias, whose mapping has
// version, and the indexes in exists are there.
type cluster struct {
	alias   string
	version int
	hash    string
	exists  map[string]bool
	// refused is the error type of an index creation, empty to accept it.
	refused string
	// aliasRefused is the error type of an aliases request, empty to accept it.
	aliasRefused string
}

func (c *cluster) reply(r request) (int, string) {
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.Path, "/proxy/_alias/"):
		if c.alias == "" {
			return http.StatusNotFound, `{"error":"alias [my-statuses] missing","status":404}`
		}
		return http.StatusOK, `{"` + c.alias + `":{"aliases":{"my-statuses":{}}}}`
	case r.Method == http.MethodGet && strings.HasSuffix(r.Path, "/_mapping"):
		return http.StatusOK, `{"` + c.alias + `":{"mappings":{"_meta":{"my_mapping_version":` + strconv.Itoa(c.version) + `,"my_settings_hash":"` + c.hash + `"}}}}`
	case r.Method == http.MethodPut && strings.Count(r.Path, "/") == 2:
		name := strings.TrimPrefix(r.Path, "/proxy/")
		if c.refused != "" {
			return http.StatusBadRequest, `{"error":{"type":"` + c.refused + `","reason":"analyzer [my_text] has not been configured in mappings"},"status":400}`
		}
		if c.exists[name] {
			return http.StatusBadRequest, `{"error":{"type":"resource_already_exists_exception"},"status":400}`
		}
		c.exists[name] = true
		return http.StatusOK, `{"acknowledged":true}`
	case r.Path == "/proxy/_aliases":
		if c.aliasRefused != "" {
			return http.StatusBadRequest, `{"error":{"type":"` + c.aliasRefused + `"},"status":400}`
		}
		for _, a := range r.Body["actions"].([]any) {
			if add, ok := a.(map[string]any)["add"].(map[string]any); ok {
				c.alias = add["index"].(string)
			}
		}
		return http.StatusOK, `{"acknowledged":true}`
	}
	return http.StatusOK, `{"acknowledged":true}`
}

func newCluster(t *testing.T, c *cluster, settings string) (*Index, *standIn) {
	t.Helper()
	if c.exists == nil {
		c.exists = map[string]bool{}
	}
	o := Options{}
	if settings != "" {
		o.Settings = []byte(settings)
	}
	x, s := newStandIn(t, o)
	s.reply = c.reply
	return x, s
}

func paths(s *standIn) string {
	var out []string
	for _, r := range s.requests {
		out = append(out, r.Method+" "+strings.TrimPrefix(r.Path, "/proxy"))
	}
	return strings.Join(out, ", ")
}

// defaultHash is the hash of DefaultSettings.
func defaultHash(t *testing.T) string {
	t.Helper()
	st, err := parseSettings([]byte(DefaultSettings))
	if err != nil {
		t.Fatal(err)
	}
	return st.hash
}

func TestEnsureCreates(t *testing.T) {
	c := &cluster{}
	x, s := newCluster(t, c, "")
	if err := x.ensure(t.Context(), currentMapping); err != nil {
		t.Fatal(err)
	}
	name := "my-statuses-v1-" + defaultHash(t)
	want := "GET /_alias/my-statuses, PUT /" + name + ", POST /_aliases"
	if got := paths(s); got != want {
		t.Errorf("ensure sent %s, want %s", got, want)
	}
	create := s.requests[1].Body
	if got, want := mustJSON(create["mappings"].(map[string]any)["_meta"]), `{"my_mapping_version":1,"my_settings_hash":"`+defaultHash(t)+`"}`; got != want {
		t.Errorf("the index was created with the _meta %s, want %s", got, want)
	}
	var def any
	_ = json.Unmarshal([]byte(DefaultSettings), &def)
	if got := mustJSON(create["settings"]); got != mustJSON(def) {
		t.Errorf("the index was created with the settings %s, want the default", got)
	}
	text := create["mappings"].(map[string]any)["properties"].(map[string]any)["text"].(map[string]any)
	if text["analyzer"] != AnalyzerText || text["search_analyzer"] != nil {
		t.Errorf("the text field is %v, want the analyzer %s and no search analyzer", text, AnalyzerText)
	}
	if c.alias != name {
		t.Errorf("the alias points at %q", c.alias)
	}
}

func TestEnsureCreatesWithTheOperatorsSettings(t *testing.T) {
	const custom = `{"number_of_shards": 2, "analysis": {"analyzer": {"my_text": {"type": "standard"}, "my_text_search": {"type": "simple"}}}}`
	c := &cluster{}
	x, s := newCluster(t, c, custom)
	if err := x.ensure(t.Context(), currentMapping); err != nil {
		t.Fatal(err)
	}
	create := s.requests[1].Body
	if got := mustJSON(create["settings"]); got != `{"analysis":{"analyzer":{"my_text":{"type":"standard"},"my_text_search":{"type":"simple"}}},"number_of_shards":2}` {
		t.Errorf("the index was created with the settings %s", got)
	}
	text := create["mappings"].(map[string]any)["properties"].(map[string]any)["text"].(map[string]any)
	if text["search_analyzer"] != AnalyzerTextSearch {
		t.Errorf("the text field is %v, want the search analyzer %s", text, AnalyzerTextSearch)
	}
	if currentMapping.Properties["text"].(map[string]any)["search_analyzer"] != nil {
		t.Error("the mapping of the build was changed")
	}
}

func TestEnsureExplainsRefusedSettings(t *testing.T) {
	c := &cluster{refused: "mapper_parsing_exception"}
	x, _ := newCluster(t, c, `{"analysis":{}}`)
	err := x.ensure(t.Context(), currentMapping)
	if err == nil || !strings.Contains(err.Error(), "my_text") || !strings.Contains(err.Error(), "has not been configured") {
		t.Errorf("ensure with refused settings: %v, want an error naming the analyzer and quoting the reason", err)
	}

	c = &cluster{alias: "my-statuses-v1-old", version: 1, hash: "old", refused: "illegal_argument_exception"}
	x, _ = newCluster(t, c, `{"analysis":{}}`)
	if err := x.ensure(t.Context(), currentMapping); err == nil {
		t.Error("ensure with refused new settings succeeded")
	}
	if c.alias != "my-statuses-v1-old" {
		t.Errorf("the alias moved to %q although the new index was refused", c.alias)
	}
}

func TestEnsureTakesOverAnIndexWithoutTheAlias(t *testing.T) {
	name := "my-statuses-v1-" + defaultHash(t)
	c := &cluster{exists: map[string]bool{name: true}}
	x, _ := newCluster(t, c, "")
	if err := x.ensure(t.Context(), currentMapping); err != nil {
		t.Fatal(err)
	}
	if c.alias != name {
		t.Errorf("the alias points at %q", c.alias)
	}
}

func TestEnsureExplainsATakenName(t *testing.T) {
	c := &cluster{aliasRefused: "invalid_alias_name_exception"}
	x, _ := newCluster(t, c, "")
	err := x.ensure(t.Context(), currentMapping)
	if err == nil || !strings.Contains(err.Error(), "search_elasticsearch_index") {
		t.Errorf("ensure with an index of the alias's name: %v, want an error naming the setting", err)
	}
}

func TestEnsureKeepsTheCurrentIndex(t *testing.T) {
	c := &cluster{alias: "my-statuses-v1-x-123", version: 1, hash: defaultHash(t)}
	x, s := newCluster(t, c, "")
	if err := x.ensure(t.Context(), currentMapping); err != nil {
		t.Fatal(err)
	}
	if got, want := paths(s), "GET /_alias/my-statuses, GET /my-statuses-v1-x-123/_mapping"; got != want {
		t.Errorf("ensure sent %s, want %s", got, want)
	}
}

func TestEnsureReplacesTheIndexOfOtherSettings(t *testing.T) {
	c := &cluster{alias: "my-statuses-v1-old", version: 1, hash: "0123456789abcdef"}
	x, s := newCluster(t, c, "")
	if err := x.ensure(t.Context(), currentMapping); err != nil {
		t.Fatal(err)
	}
	name := "my-statuses-v1-" + defaultHash(t)
	if c.alias != name {
		t.Errorf("the alias points at %q after a change of the settings, want %s", c.alias, name)
	}
	moved := s.requests[len(s.requests)-1]
	if got, want := mustJSON(moved.Body), `{"actions":[{"add":{"alias":"my-statuses","index":"`+name+`","is_write_index":true}},{"remove_index":{"index":"my-statuses-v1-old"}}]}`; got != want {
		t.Errorf("the aliases request was %s, want %s", got, want)
	}
}

func TestEnsureUpgrades(t *testing.T) {
	next := currentMapping
	next.Version, next.InPlaceSince = 3, 2
	hash := defaultHash(t)

	c := &cluster{alias: "my-statuses-v2-" + hash, version: 2, hash: hash}
	x, s := newCluster(t, c, "")
	if err := x.ensure(t.Context(), next); err != nil {
		t.Fatal(err)
	}
	if got, want := paths(s), "GET /_alias/my-statuses, GET /"+c.alias+"/_mapping, PUT /"+c.alias+"/_mapping"; got != want {
		t.Errorf("an upgrade in place sent %s, want %s", got, want)
	}
	if got := mustJSON(s.requests[2].Body["_meta"]); got != `{"my_mapping_version":3,"my_settings_hash":"`+hash+`"}` {
		t.Errorf("an upgrade in place recorded the _meta %s", got)
	}

	c = &cluster{alias: "my-statuses-v1-" + hash, version: 1, hash: hash}
	x, _ = newCluster(t, c, "")
	if err := x.ensure(t.Context(), next); err != nil {
		t.Fatal(err)
	}
	if c.alias != "my-statuses-v3-"+hash {
		t.Errorf("the alias points at %q after an upgrade onto a new index", c.alias)
	}

	c = &cluster{alias: "my-statuses-v4-" + hash, version: 4, hash: hash}
	x, _ = newCluster(t, c, "")
	if err := x.ensure(t.Context(), next); err == nil {
		t.Error("ensure on an index of a newer version succeeded")
	}
}

func TestClearMovesTheAlias(t *testing.T) {
	c := &cluster{alias: "my-statuses-v1-" + defaultHash(t), version: 1, hash: defaultHash(t)}
	x, _ := newCluster(t, c, "")
	if err := x.Clear(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c.alias, "my-statuses-v1-"+defaultHash(t)+"-") {
		t.Errorf("the alias points at %q after Clear, want a new index", c.alias)
	}
}
