// SPDX-License-Identifier: AGPL-3.0-only

package docslint

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// inventoryRepo is a repository with one name of each inventory that its documents mention, and one that they do
// not ("undocumented", "unlisted", ...).
var inventoryRepo = map[string]string{
	"cmd/my/main.go": `package main

func run(args []string) int {
	switch cmd := args[0]; cmd {
	case "serve":
		return 0
	case "admin":
		return runAdmin(args[1:])
	case "-h", "":
		return 2
	}
	return 2
}
`,
	"cmd/my/admin.go": `package main

func runAdmin(args []string) int {
	switch {
	case args[0] == "relay":
		return runAdminRelay(args[1:])
	case args[0] == "secret":
		return runAdminSecret(args[1:])
	case args[0] != "create":
		return 2
	}
	return 0
}

func runAdminRelay(args []string) int {
	sub, code := dispatch("admin relay", args, []string{"add", "list"})
	_ = sub
	return code
}

func runAdminSecret(args []string) int {
	switch sub := args[0]; sub {
	case "undocumented":
		return 0
	}
	switch f := args[1]; f {
	case "flag-value":
		return 0
	}
	return 2
}

func dispatch(cmd string, args, subs []string) (string, int) { return "", 0 }
`,
	"cmd/my/admin_test.go": `package main

func runTestOnly() { dispatch("test only", nil, []string{"never"}) }
`,
	"internal/app/queue/metrics.go": `package queue

var (
	Ready = kernel.Instrument{Name: "my.queue.ready", Unit: "{job}"}
	Lost  = kernel.Instrument{Name: "my.queue.lost", Unit: "{job}"}
	Span  = kernel.SpanSpec{Name: "my.job"}
)
`,
	"internal/domain/webhook/webhook.go": `package webhook

type EventType string

const (
	EventReportCreated EventType = "report.created"
	EventStatusCreated EventType = "status.created"
)

const notAnEvent = "account.updated"
`,
	"internal/platform/adapters_no_sqlite.go": "//go:build no_sqlite\n\npackage platform\n",
	"internal/platform/adapters_no_valkey.go": "// SPDX\n\n//go:build no_valkey && !linux\n\npackage platform\n",
	"internal/platform/adapters_live_test.go": "//go:build federationlive\n\npackage platform\n",
	"tools/agentkit/go.mod":                   "module x\n",
	"tools/agentkit/x.go":                     "//go:build no_agentkit\n\npackage x\n",
	"Taskfile.yml":                            "version: \"3\"\nincludes:\n  go: ./tasks/go.yml\n  db:\n    taskfile: ./tasks/db.yml\ntasks:\n  check:\n    desc: Check\n",
	"tasks/go.yml":                            "version: \"3\"\ntasks:\n  test:\n    desc: Test\n  hidden:\n    cmds: [true]\n  helper:\n    desc: Helper\n    internal: true\n",
	"tasks/db.yml":                            "version: \"3\"\ntasks:\n  up:\n    desc: Start\n",
	"docs/operations/admin.md":                "Run `my serve`, and `my admin create`.\n\n```sh\nmy admin relay add|list <url>\n```\n",
	"docs/operations/running.md":              "| `my.queue.ready` | Gauge |\n\nThe job `my.queue.lost.count` is something else.\n",
	"docs/reference/webhooks.md":              "| `report.created` | ... |\n",
	"docs/reference/build-tags.md":            "| `no_sqlite` | ... |\n",
	"docs/reference/commands.md":              "| `check` [`CHECKS=...`] | ... |\n| `go:test` / `db:up DB=x` | ... |\n",
}

func writeRepo(t *testing.T, files map[string]string) (string, []string) {
	t.Helper()
	root := t.TempDir()
	var paths []string
	for p, c := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return root, paths
}

func messages(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.String())
	}
	return out
}

func TestInventoryCollects(t *testing.T) {
	root, files := writeRepo(t, inventoryRepo)
	want := map[string][]string{
		"cli": {"my admin", "my admin create", "my admin relay", "my admin relay add", "my admin relay list",
			"my admin secret", "my admin secret undocumented", "my serve"},
		"metrics":        {"my.queue.ready", "my.queue.lost"},
		"webhook-events": {"report.created", "status.created"},
		"build-tags":     {"no_sqlite", "no_valkey"},
		"tasks":          {"check", "db:up", "go:test"},
	}
	for _, inv := range inventories {
		items, err := inv.collect(root, files)
		if err != nil {
			t.Fatalf("%s: %v", inv.name, err)
		}
		var got []string
		for _, it := range items {
			got = append(got, it.Name)
		}
		sort.Strings(got)
		w := append([]string(nil), want[inv.name]...)
		sort.Strings(w)
		if strings.Join(got, ",") != strings.Join(w, ",") {
			t.Errorf("%s: got %v, want %v", inv.name, got, w)
		}
	}
}

func TestInventory(t *testing.T) {
	root, files := writeRepo(t, inventoryRepo)
	c := &Config{}
	got, err := Inventory(root, files, c)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"the command my admin secret (cmd/my/admin.go:",
		"the command my admin secret undocumented (cmd/my/admin.go:",
		"the metric my.queue.lost (internal/app/queue/metrics.go:5)",
		"the webhook event type status.created (internal/domain/webhook/webhook.go:7)",
		"the build tag no_valkey (internal/platform/adapters_no_valkey.go:3)",
	}
	msgs := messages(got)
	if len(msgs) != len(want) {
		t.Fatalf("got %d findings, want %d:\n%s", len(msgs), len(want), strings.Join(msgs, "\n"))
	}
	for i, w := range want {
		if !strings.Contains(msgs[i], w) {
			t.Errorf("finding %d = %q, want it to contain %q", i, msgs[i], w)
		}
	}

	// Exclusions silence a name, and fail when they name a documented or unknown name, or an unknown inventory.
	c.Inventory.Exclude = map[string][]string{
		"cli":            {"my admin secret", "my admin secret undocumented", "my serve"},
		"metrics":        {"my.queue.lost", "my.gone"},
		"webhook-events": {"status.created"},
		"build-tags":     {"no_valkey"},
		"bogus":          {"x"},
	}
	got, err = Inventory(root, files, c)
	if err != nil {
		t.Fatal(err)
	}
	msgs = messages(got)
	want = []string{
		"inventory.exclude.cli lists my serve, which README.md, docs/operations/*.md, docs/reference/*.md mentions now",
		"inventory.exclude.metrics lists my.gone, which the code does not define",
		"inventory.exclude has no inventory \"bogus\"",
	}
	if len(msgs) != len(want) {
		t.Fatalf("got %d findings, want %d:\n%s", len(msgs), len(want), strings.Join(msgs, "\n"))
	}
	for i, w := range want {
		if !strings.Contains(msgs[i], w) {
			t.Errorf("finding %d = %q, want it to contain %q", i, msgs[i], w)
		}
	}
}

func TestCLIMentions(t *testing.T) {
	mentioned := cliMentions([]string{
		"Use `my job list|retry|delete` or\n\n```sh\nmy admin role grant alice moderator   # comment\n```\n",
		"`my admin supporters` runs part of it:\n\n- `status`: whether it is on.\n- `cancel <id>`: cancels.\n",
		"Words that follow a dummy serve, or a path like /my serve, are not commands.",
	})
	for _, name := range []string{"my job", "my job list", "my job retry", "my job delete", "my admin role grant",
		"my admin supporters status", "my admin supporters cancel"} {
		if !mentioned(name) {
			t.Errorf("%q is not mentioned", name)
		}
	}
	for _, name := range []string{"my job status", "my admin role status", "my serve", "my admin supporters whether"} {
		if mentioned(name) {
			t.Errorf("%q is mentioned", name)
		}
	}
}

func TestNameMentions(t *testing.T) {
	mentioned := nameMentions([]string{"| `my.queue.ready` | x |\nSee my.job.duration.\nAnd `my.db.client.connection.waits`."})
	for name, want := range map[string]bool{
		"my.queue.ready": true, "my.job.duration": true, "my.db.client.connection.waits": true,
		"my.queue": false, "db.client.connection.waits": false, "my.job": false,
	} {
		if got := mentioned(name); got != want {
			t.Errorf("mentioned(%q) = %v, want %v", name, got, want)
		}
	}
}
