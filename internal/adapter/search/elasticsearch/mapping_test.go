// SPDX-License-Identifier: AGPL-3.0-only

package elasticsearch

import (
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
	exists  map[string]bool
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
		return http.StatusOK, `{"` + c.alias + `":{"mappings":{"_meta":{"my_mapping_version":` + strconv.Itoa(c.version) + `}}}}`
	case r.Method == http.MethodPut && strings.Count(r.Path, "/") == 2:
		name := strings.TrimPrefix(r.Path, "/proxy/")
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

func newCluster(t *testing.T, c *cluster, replicas int) (*Index, *standIn) {
	t.Helper()
	if c.exists == nil {
		c.exists = map[string]bool{}
	}
	x, s := newStandIn(t, Options{Replicas: replicas})
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

func TestEnsureCreates(t *testing.T) {
	c := &cluster{}
	x, s := newCluster(t, c, 0)
	if err := x.ensure(t.Context(), currentMapping); err != nil {
		t.Fatal(err)
	}
	want := "GET /_alias/my-statuses, PUT /my-statuses-v1, POST /_aliases, PUT /my-statuses/_settings"
	if got := paths(s); got != want {
		t.Errorf("ensure sent %s, want %s", got, want)
	}
	create := s.requests[1].Body
	if got := mustJSON(create["mappings"].(map[string]any)["_meta"]); got != `{"my_mapping_version":1}` {
		t.Errorf("the index was created with the _meta %s", got)
	}
	if got := mustJSON(create["settings"].(map[string]any)["index"].(map[string]any)["number_of_replicas"]); got != "0" {
		t.Errorf("the index was created with %s replicas, want 0", got)
	}
	if c.alias != "my-statuses-v1" {
		t.Errorf("the alias points at %q", c.alias)
	}
}

func TestEnsureTakesOverAnIndexWithoutTheAlias(t *testing.T) {
	c := &cluster{exists: map[string]bool{"my-statuses-v1": true}}
	x, _ := newCluster(t, c, -1)
	if err := x.ensure(t.Context(), currentMapping); err != nil {
		t.Fatal(err)
	}
	if c.alias != "my-statuses-v1" {
		t.Errorf("the alias points at %q", c.alias)
	}
}

func TestEnsureExplainsATakenName(t *testing.T) {
	c := &cluster{aliasRefused: "invalid_alias_name_exception"}
	x, _ := newCluster(t, c, -1)
	err := x.ensure(t.Context(), currentMapping)
	if err == nil || !strings.Contains(err.Error(), "search_elasticsearch_index") {
		t.Errorf("ensure with an index of the alias's name: %v, want an error naming the setting", err)
	}
}

func TestEnsureKeepsTheCurrentVersion(t *testing.T) {
	c := &cluster{alias: "my-statuses-v1-123", version: 1}
	x, s := newCluster(t, c, -1)
	if err := x.ensure(t.Context(), currentMapping); err != nil {
		t.Fatal(err)
	}
	if got, want := paths(s), "GET /_alias/my-statuses, GET /my-statuses-v1-123/_mapping"; got != want {
		t.Errorf("ensure sent %s, want %s (no settings without replicas)", got, want)
	}
}

func TestEnsureUpgrades(t *testing.T) {
	next := currentMapping
	next.Version, next.InPlaceSince = 3, 2

	c := &cluster{alias: "my-statuses-v2", version: 2}
	x, s := newCluster(t, c, -1)
	if err := x.ensure(t.Context(), next); err != nil {
		t.Fatal(err)
	}
	if got, want := paths(s), "GET /_alias/my-statuses, GET /my-statuses-v2/_mapping, PUT /my-statuses-v2/_mapping"; got != want {
		t.Errorf("an upgrade in place sent %s, want %s", got, want)
	}

	c = &cluster{alias: "my-statuses-v1", version: 1}
	x, s = newCluster(t, c, -1)
	if err := x.ensure(t.Context(), next); err != nil {
		t.Fatal(err)
	}
	if c.alias != "my-statuses-v3" {
		t.Errorf("the alias points at %q after an upgrade onto a new index", c.alias)
	}
	moved := s.requests[len(s.requests)-1]
	if got, want := mustJSON(moved.Body), `{"actions":[{"add":{"alias":"my-statuses","index":"my-statuses-v3","is_write_index":true}},{"remove_index":{"index":"my-statuses-v1"}}]}`; got != want {
		t.Errorf("the aliases request was %s, want %s", got, want)
	}

	c = &cluster{alias: "my-statuses-v4", version: 4}
	x, _ = newCluster(t, c, -1)
	if err := x.ensure(t.Context(), next); err == nil {
		t.Error("ensure on an index of a newer version succeeded")
	}
}

func TestClearMovesTheAlias(t *testing.T) {
	c := &cluster{alias: "my-statuses-v1", version: 1}
	x, _ := newCluster(t, c, -1)
	if err := x.Clear(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c.alias, "my-statuses-v1-") {
		t.Errorf("the alias points at %q after Clear, want a new index", c.alias)
	}
}
