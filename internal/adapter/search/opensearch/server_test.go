// SPDX-License-Identifier: AGPL-3.0-only

package opensearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/pkg/testkit/searchtest"
)

// The environment of the tests on a real engine: they run with TEST_SEARCH=container
// (scripts/task.sh go:test-search), which starts an OpenSearch container, or against the cluster
// at TEST_OPENSEARCH_URL (security off), and skip otherwise. With TEST_SEARCH=container, a Docker
// that cannot start the container fails them.
const (
	envSearch = "TEST_SEARCH"
	envURL    = "TEST_OPENSEARCH_URL"
	envImage  = "TEST_OPENSEARCH_IMAGE"
	// envSettings names one more settings file for the contract, such as one with the kuromoji
	// analyzers on an image that has the plugin.
	envSettings = "TEST_OPENSEARCH_SETTINGS_FILE"
)

// defaultImage is the container image of the tests, pinned by tag and digest.
const defaultImage = "opensearchproject/opensearch:3.9.0@sha256:3b55f6b6249cfdb92b52ee59adc8a266203ba9ef9f5d82544fad6f1af2f3609e"

// testServer returns the URL of an OpenSearch cluster for t, or skips t.
func testServer(t *testing.T) string {
	t.Helper()
	if u := os.Getenv(envURL); u != "" {
		return u
	}
	if os.Getenv(envSearch) != "container" {
		t.Skipf("the OpenSearch tests run with %s=container (scripts/task.sh go:test-search) or %s", envSearch, envURL)
	}
	image := os.Getenv(envImage)
	if image == "" {
		image = defaultImage
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	c, err := testcontainers.Run(ctx, image,
		testcontainers.WithExposedPorts("9200/tcp"),
		testcontainers.WithEnv(map[string]string{
			"discovery.type":              "single-node",
			"DISABLE_SECURITY_PLUGIN":     "true",
			"DISABLE_INSTALL_DEMO_CONFIG": "true",
			"OPENSEARCH_JAVA_OPTS":        "-Xms512m -Xmx512m",
		}),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/_cluster/health?wait_for_status=yellow&timeout=5s").
			WithPort("9200/tcp").WithStartupTimeout(3*time.Minute)),
	)
	if c != nil {
		t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })
	}
	if err != nil {
		t.Fatalf("start the container %s: %v", image, err)
	}
	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}
	port, err := c.MappedPort(ctx, "9200/tcp")
	if err != nil {
		t.Fatalf("container port: %v", err)
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(int(port.Num())))
}

// refreshing is the Index of the contract: every write refreshes the index, since the contract
// wants a write in the next search and the engine shows it within a second.
type refreshing struct{ x *Index }

func (r refreshing) after(ctx context.Context, err error) error {
	if err != nil {
		return err
	}
	return r.x.refresh(ctx)
}

func (r refreshing) Replace(ctx context.Context, d search.Document) error {
	return r.after(ctx, r.x.Replace(ctx, d))
}

func (r refreshing) Remove(ctx context.Context, id string) error {
	return r.after(ctx, r.x.Remove(ctx, id))
}

func (r refreshing) RemoveByAuthor(ctx context.Context, id string) error {
	return r.after(ctx, r.x.RemoveByAuthor(ctx, id))
}

func (r refreshing) RemoveByVisibility(ctx context.Context, v search.Visibility) error {
	return r.after(ctx, r.x.RemoveByVisibility(ctx, v))
}

func (r refreshing) Clear(ctx context.Context) error { return r.after(ctx, r.x.Clear(ctx)) }

func (r refreshing) Search(ctx context.Context, q search.Query) (search.Page, error) {
	return r.x.Search(ctx, q)
}

var indexSeq atomic.Int64

func newServerIndex(t *testing.T, base, name string) *Index {
	t.Helper()
	return newServerIndexWith(t, base, name, nil)
}

// testSettings returns the settings file of the tests, n-grams of the built-in tokenizer.
func testSettings(t *testing.T) json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("testdata/ngram-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newServerIndexWith(t *testing.T, base, name string, settings json.RawMessage) *Index {
	t.Helper()
	x, err := New(Options{
		URL: base, Index: name, Settings: settings, HTTPClient: &http.Client{Timeout: time.Minute},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func TestSearchServerContract(t *testing.T) {
	base := testServer(t)
	cases := []struct {
		name     string
		settings json.RawMessage
	}{{"default settings", nil}, {"a settings file of n-grams", testSettings(t)}}
	if path := os.Getenv(envSettings); path != "" {
		b, err := os.ReadFile(path) //nolint:gosec // the path of the test's environment
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, struct {
			name     string
			settings json.RawMessage
		}{"the settings file of " + envSettings, b})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			searchtest.SearchIndexContract(t, func(t *testing.T) search.Index {
				x := newServerIndexWith(t, base, fmt.Sprintf("my-contract-%d", indexSeq.Add(1)), tc.settings)
				if err := x.Ensure(t.Context()); err != nil {
					t.Fatalf("Ensure: %v", err)
				}
				return refreshing{x}
			})
		})
	}
}

func TestSearchServerEnsure(t *testing.T) {
	base := testServer(t)
	ctx := t.Context()
	name := fmt.Sprintf("my-ensure-%d", indexSeq.Add(1))
	x := newServerIndex(t, base, name)

	t.Run("a write without the alias fails rather than creating an index", func(t *testing.T) {
		err := x.Replace(ctx, search.Document{StatusID: "a", AuthorID: "b", Visibility: search.VisibilityPublic, Text: "alpha"})
		if err == nil {
			t.Fatal("Replace before Ensure succeeded")
		}
		if targets, err := x.aliasTargets(ctx); err != nil || len(targets) != 0 {
			t.Fatalf("the alias after a refused write: %v, %v", targets, err)
		}
	})

	t.Run("Ensure creates the index and can be repeated", func(t *testing.T) {
		for range 2 {
			if err := x.Ensure(ctx); err != nil {
				t.Fatalf("Ensure: %v", err)
			}
		}
		targets, err := x.aliasTargets(ctx)
		if err != nil || len(targets) != 1 || targets[0] != x.indexName("") {
			t.Fatalf("the alias points at %v (%v), want %s", targets, err, x.indexName(""))
		}
	})

	t.Run("a newer version moves the alias, and an older binary leaves it", func(t *testing.T) {
		if err := x.Replace(ctx, search.Document{StatusID: "a", AuthorID: "b", Visibility: search.VisibilityPublic, Text: "alpha"}); err != nil {
			t.Fatal(err)
		}
		if err := x.refresh(ctx); err != nil {
			t.Fatal(err)
		}
		next := newServerIndex(t, base, name)
		next.version = mappingVersion + 1
		for _, y := range []*Index{next, x} {
			if err := y.Ensure(ctx); err != nil {
				t.Fatalf("Ensure of version %d: %v", y.version, err)
			}
			targets, err := y.aliasTargets(ctx)
			if err != nil || len(targets) != 1 || targets[0] != next.indexName("") {
				t.Fatalf("after Ensure of version %d the alias points at %v (%v)", y.version, targets, err)
			}
		}
		// The new index starts empty until the rebuild.
		page, err := x.Search(ctx, search.Query{Text: "alpha", Limit: 10})
		if err != nil || len(page.StatusIDs) != 0 {
			t.Fatalf("Search after the upgrade = %v, %v; want nothing", page, err)
		}
	})

	t.Run("other settings move the alias, and Clear deletes the index", func(t *testing.T) {
		other := newServerIndexWith(t, base, name, testSettings(t))
		other.version = mappingVersion + 1
		if err := other.Ensure(ctx); err != nil {
			t.Fatal(err)
		}
		before, _ := other.aliasTargets(ctx)
		if len(before) != 1 || before[0] != other.indexName("") {
			t.Fatalf("the alias points at %v, want %s", before, other.indexName(""))
		}
		if err := other.Clear(ctx); err != nil {
			t.Fatal(err)
		}
		after, _ := other.aliasTargets(ctx)
		if len(after) != 1 || after[0] == before[0] {
			t.Fatalf("after Clear the alias points at %v", after)
		}
		var e *Error
		if err := other.call(ctx, "read", http.MethodGet, before[0], nil, nil, nil); !errors.As(err, &e) || e.Status != http.StatusNotFound {
			t.Errorf("the cleared index: %v, want 404", err)
		}
	})

	t.Run("settings without the analyzers are refused with the reason", func(t *testing.T) {
		bad := newServerIndexWith(t, base, fmt.Sprintf("my-bad-%d", indexSeq.Add(1)), json.RawMessage(`{"analysis":{}}`))
		err := bad.Ensure(ctx)
		if err == nil || !strings.Contains(err.Error(), "my_text") {
			t.Fatalf("Ensure: %v", err)
		}
		t.Logf("the error at start: %v", err)
	})

	t.Run("a name that is an index is refused", func(t *testing.T) {
		taken := fmt.Sprintf("my-taken-%d", indexSeq.Add(1))
		o := newServerIndex(t, base, taken)
		if err := o.createIndex(ctx, taken); err != nil {
			t.Fatal(err)
		}
		if err := o.Ensure(ctx); err == nil {
			t.Fatal("Ensure succeeded on a name that is an index")
		}
	})
}

// TestSearchServerJapanese checks what the operator documentation says of Japanese: a run of
// characters that a text holds finds it, a single character too, and the pairs of a query keep
// characters that stand apart in the text from matching.
func TestSearchServerJapanese(t *testing.T) {
	base := testServer(t)
	x := newServerIndex(t, base, fmt.Sprintf("my-japanese-%d", indexSeq.Add(1)))
	ctx := t.Context()
	if err := x.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	docs := []string{"全文検索の本文です", "東京都の天気は晴れ", "東の空と京の町", "ｶﾀｶﾅのテキスト", "alpha"}
	for i, text := range docs {
		d := search.Document{StatusID: strconv.Itoa(i + 1), AuthorID: "a", Visibility: search.VisibilityPublic, Language: "ja", Text: text}
		if err := x.Replace(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	if err := x.refresh(ctx); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"全文検索", []string{"1"}},
		{"検索", []string{"1"}},
		{"本", []string{"1"}},
		{"全文 本文", []string{"1"}},
		{"東京", []string{"2"}},
		{"京都", []string{"2"}},
		{"天気 晴れ", []string{"2"}},
		{"東", []string{"3", "2"}},
		{"カタカナ", []string{"4"}},
		{"ＡＬＰＨＡ", []string{"5"}},
		{"検索エンジン", nil},
	} {
		page, err := x.Search(ctx, search.Query{Text: tc.query, Limit: 10})
		if err != nil {
			t.Fatalf("Search(%q): %v", tc.query, err)
		}
		if fmt.Sprint(page.StatusIDs) != fmt.Sprint(tc.want) {
			t.Errorf("Search(%q) = %v, want %v", tc.query, page.StatusIDs, tc.want)
		}
	}
}

// TestSearchServerDeleteByQueryOfUnrefreshedWrites checks that a removal by author or visibility
// removes a document written just before it, which no refresh has shown to searches yet.
func TestSearchServerDeleteByQueryOfUnrefreshedWrites(t *testing.T) {
	base := testServer(t)
	x := newServerIndex(t, base, fmt.Sprintf("my-unrefreshed-%d", indexSeq.Add(1)))
	ctx := t.Context()
	if err := x.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	for _, remove := range []func() error{
		func() error { return x.RemoveByAuthor(ctx, "a") },
		func() error { return x.RemoveByVisibility(ctx, search.VisibilityLocal) },
		func() error { return x.Clear(ctx) },
	} {
		if err := x.Replace(ctx, search.Document{StatusID: "1", AuthorID: "a", Visibility: search.VisibilityLocal, Text: "alpha"}); err != nil {
			t.Fatal(err)
		}
		if err := remove(); err != nil {
			t.Fatal(err)
		}
		if err := x.refresh(ctx); err != nil {
			t.Fatal(err)
		}
		if page, err := x.Search(ctx, search.Query{Text: "alpha", Limit: 10}); err != nil || len(page.StatusIDs) != 0 {
			t.Errorf("Search after the removal = %v, %v", page.StatusIDs, err)
		}
	}
}
