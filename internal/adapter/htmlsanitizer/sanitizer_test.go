// SPDX-License-Identifier: AGPL-3.0-only

package htmlsanitizer_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/adapter/htmlsanitizer"
	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit/contenttest"
)

var update = flag.Bool("update", false, "rewrite the golden files from the current output")

func TestContract(t *testing.T) {
	contenttest.HTMLSanitizerContract(t, func(*testing.T) kernel.HTMLSanitizer { return htmlsanitizer.New() })
}

// TestGolden sanitizes the content HTML that Mastodon, Misskey, GoToSocial, and Akkoma send
// (testdata/<implementation>/<name>.html) and compares it with <name>.golden.html.
// Run go test with -update to rewrite the golden files after a policy change, together with
// an increase of PolicyVersion.
func TestGolden(t *testing.T) {
	s := htmlsanitizer.New()
	inputs, err := filepath.Glob(filepath.Join("testdata", "*", "*.html"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, in := range inputs {
		if strings.HasSuffix(in, ".golden.html") {
			continue
		}
		seen[filepath.Base(filepath.Dir(in))] = true
		t.Run(strings.TrimSuffix(in, ".html"), func(t *testing.T) {
			src, err := os.ReadFile(in) //nolint:gosec // G304: a fixture path from the glob above
			if err != nil {
				t.Fatal(err)
			}
			got := s.SanitizeHTML(strings.TrimSuffix(string(src), "\n")) + "\n"
			contenttest.CheckSanitizedHTML(t, got)
			golden := strings.TrimSuffix(in, ".html") + ".golden.html"
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o600); err != nil { //nolint:gosec // G703: a fixture path derived from the glob above
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden) //nolint:gosec // G304: a fixture path derived from the glob above
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("SanitizeHTML(%s)\n got %s\nwant %s", in, got, want)
			}
			if again := s.SanitizeHTML(strings.TrimSuffix(got, "\n")) + "\n"; again != got {
				t.Errorf("sanitizing the output again gave %s", again)
			}
		})
	}
	for _, impl := range []string{"mastodon", "misskey", "gotosocial", "akkoma"} {
		if !seen[impl] {
			t.Errorf("no fixture for %s", impl)
		}
	}
}

// FuzzSanitizeHTML checks that whatever the input, the output is allowlisted markup and a
// fixed point.
func FuzzSanitizeHTML(f *testing.F) {
	for _, s := range []string{
		`<a href="javascript:alert(1)">x</a>`, `<svg onload=alert(1)>`, `<h1><div>a<p>b</p></div></h1>`,
		`<math><semantics><annotation encoding="application/x-tex">x</annotation></semantics></math>`,
		`<a href="https://a.example/"><a href="https://b.example/">x</a></a>`, `<ol start=1 reversed><li value=2>`,
		`<p class="h-card mention x" lang="ja" translate="no">`, "<table><tr><td>a<td>b</table>",
	} {
		f.Add(s)
	}
	s := htmlsanitizer.New()
	f.Fuzz(func(t *testing.T, in string) {
		out := s.SanitizeHTML(in)
		contenttest.CheckSanitizedHTML(t, out)
		if again := s.SanitizeHTML(out); again != out {
			t.Fatalf("SanitizeHTML(%q) = %q, and sanitizing that gave %q", in, out, again)
		}
	})
}

// FuzzLocalHTML checks that the HTML of any local status passes the sanitizer unchanged
// (federation.md §5: local HTML uses a subset of the allowlist).
func FuzzLocalHTML(f *testing.F) {
	for _, text := range contenttest.LocalStatusTexts {
		f.Add(text)
	}
	s := htmlsanitizer.New()
	f.Fuzz(func(t *testing.T, text string) {
		if !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
			t.Skip("content.Service refuses this text")
		}
		h := content.ParseText(text).HTML(content.RenderOptions{Host: "social.example", Mentions: contenttest.LocalStatusMentions})
		if got := s.SanitizeHTML(h); got != h {
			t.Fatalf("SanitizeHTML changed the HTML of %q\n got %q\nwant %q", text, got, h)
		}
	})
}
