// SPDX-License-Identifier: AGPL-3.0-only

package i18n

import (
	"encoding/json"
	"io/fs"
	"os"
	"testing"
)

// testdata/golden.json holds FormatJS (intl-messageformat) outputs for the inputs in
// testdata/cases.json and for CLDR probes; tools/i18n regenerates it and `scripts/task.sh i18n:check`
// fails when it is stale. These tests require the Go engine to produce identical strings,
// so the server and a FormatJS client format shared messages the same way.

type parityCases struct {
	Catalog []struct {
		Key  string         `json:"key"`
		Args map[string]any `json:"args"`
	} `json:"catalog"`
	Syntax []struct {
		Locale  string         `json:"locale"`
		Message string         `json:"message"`
		Args    map[string]any `json:"args"`
	} `json:"syntax"`
}

type parityGolden struct {
	Catalog map[string][]string `json:"catalog"`
	Syntax  []string            `json:"syntax"`
	CLDR    []struct {
		Locale  string `json:"locale"`
		Message string `json:"message"`
		Outputs []struct {
			N    float64 `json:"n"`
			Want string  `json:"want"`
		} `json:"outputs"`
	} `json:"cldr"`
}

func readJSON(t *testing.T, name string, v any) {
	t.Helper()
	b, err := fs.ReadFile(os.DirFS("testdata"), name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

// intArgs converts whole JSON numbers to int, as the generated accessors pass plural counts.
func intArgs(args map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	for k, v := range args {
		if f, ok := v.(float64); ok && f == float64(int(f)) {
			v = int(f)
		}
		out[k] = v
	}
	return out
}

func formatOnce(t *testing.T, locale, message string, args map[string]any) (string, error) {
	t.Helper()
	c, err := newCompiler(formattingLocale(locale))
	if err != nil {
		return "", err
	}
	m, err := c.compile(message)
	if err != nil {
		return "", err
	}
	return m.format(args)
}

func TestParityCatalogWithFormatJS(t *testing.T) {
	var cases parityCases
	var golden parityGolden
	readJSON(t, "cases.json", &cases)
	readJSON(t, "golden.json", &golden)
	b, err := Load(WithPseudoLocales())
	if err != nil {
		t.Fatal(err)
	}
	if len(golden.Catalog) == 0 {
		t.Fatal("golden.json has no catalog outputs")
	}
	for locale, want := range golden.Catalog {
		l := b.Localizer(locale)
		if len(want) != len(cases.Catalog) {
			t.Fatalf("%s: golden has %d outputs for %d cases; run scripts/task.sh i18n:gen", locale, len(want), len(cases.Catalog))
		}
		for i, c := range cases.Catalog {
			if _, ok := b.messages[locale][c.Key]; !ok {
				t.Errorf("%s: %s is missing from the catalog", locale, c.Key)
				continue
			}
			if got := l.format(c.Key, intArgs(c.Args)); got != want[i] {
				t.Errorf("%s %s %v:\n got  %q\n want %q", locale, c.Key, c.Args, got, want[i])
			}
		}
	}
}

func TestParitySyntaxWithFormatJS(t *testing.T) {
	var cases parityCases
	var golden parityGolden
	readJSON(t, "cases.json", &cases)
	readJSON(t, "golden.json", &golden)
	if len(golden.Syntax) != len(cases.Syntax) {
		t.Fatalf("golden has %d syntax outputs for %d cases; run scripts/task.sh i18n:gen", len(golden.Syntax), len(cases.Syntax))
	}
	for i, c := range cases.Syntax {
		got, err := formatOnce(t, c.Locale, c.Message, intArgs(c.Args))
		if err != nil || got != golden.Syntax[i] {
			t.Errorf("%s %q %v:\n got  %q (err %v)\n want %q", c.Locale, c.Message, c.Args, got, err, golden.Syntax[i])
		}
	}
}

// TestCLDRDrift detects plural-rule and number-format differences between the CLDR data
// of the Go engine and of the JavaScript Intl implementation that generated golden.json.
func TestCLDRDrift(t *testing.T) {
	var golden parityGolden
	readJSON(t, "golden.json", &golden)
	if len(golden.CLDR) == 0 {
		t.Fatal("golden.json has no CLDR probes")
	}
	for _, probe := range golden.CLDR {
		for _, o := range probe.Outputs {
			args := intArgs(map[string]any{"n": o.N})
			got, err := formatOnce(t, probe.Locale, probe.Message, args)
			if err != nil || got != o.Want {
				t.Errorf("%s n=%v:\n got  %q (err %v)\n want %q", probe.Locale, o.N, got, err, o.Want)
			}
		}
	}
}

// TestSharedSubsetLimits pins where the Go engine diverges from FormatJS. tools/i18n
// rejects these constructs in every catalog; if one starts to match, the
// shared subset can grow.
func TestSharedSubsetLimits(t *testing.T) {
	tests := []struct {
		msg  string
		args map[string]any
		want string // Go output; "" means an error. FormatJS output in the comment.
	}{
		{"<b>{name}</b> joined", map[string]any{"name": "A"}, "<b>A</b> joined"}, // calls the b() callback
		{"{n, number, ::percent}", map[string]any{"n": 0.25}, ""},                // 25%
		{"{n, number, ::compact-short}", map[string]any{"n": 12345}, ""},         // 12K
		{"{n, number, ::currency/JPY}", map[string]any{"n": 1234}, ""},           // ¥1,234
	}
	for _, tt := range tests {
		got, err := formatOnce(t, "en", tt.msg, tt.args)
		if err != nil {
			got = ""
		}
		if got != tt.want {
			t.Errorf("%q: got %q (err %v), want %q", tt.msg, got, err, tt.want)
		}
	}
}
