// SPDX-License-Identifier: AGPL-3.0-only

package testkit_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/fs"
	"net/url"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/pkg/testkit"
)

var update = flag.Bool("update", false, "rewrite the golden files under testdata/golden")

// normalizeNow is the "now" of the normalization: later than every fixture's timestamps.
var normalizeNow = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

func federationFixtures(t *testing.T) []testkit.FederationFixture {
	t.Helper()
	fixtures, err := testkit.FederationFixtures()
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no federation fixtures")
	}
	return fixtures
}

// Every fixture is normalized as the inbox does (federation.md §4, hosts canonicalized) and the result is compared with
// testdata/golden/normalize/<implementation>/<version>/<name>.json.
func TestFederationFixturesNormalize(t *testing.T) {
	for _, f := range federationFixtures(t) {
		t.Run(f.Path(), func(t *testing.T) {
			var result any
			doc, err := appfederation.Normalize(f.JSON, normalizeNow)
			if err != nil {
				result = map[string]any{"error": err.Error()}
			} else {
				result = dump(reflect.ValueOf(doc))
			}
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false) // keep the HTML of the bodies readable
			enc.SetIndent("", "  ")
			if err := enc.Encode(result); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", "golden", "normalize", filepath.FromSlash(f.Path())+".json")
			testkit.CompareGolden(t, path, buf.Bytes(), *update)
		})
	}
}

// Every fixture has a row in the README.md of its implementation that records its source.
func TestFederationFixturesProvenance(t *testing.T) {
	fsys := testkit.FederationFixturesFS()
	for _, f := range federationFixtures(t) {
		readme, err := fs.ReadFile(fsys, f.Implementation+"/README.md")
		if err != nil {
			t.Errorf("%s: %v", f.Path(), err)
			continue
		}
		row := "| `" + f.Version + "/" + f.Name + ".json` |"
		if !bytes.Contains(readme, []byte(row)) {
			t.Errorf("%s/README.md has no row starting with %s", f.Implementation, row)
		}
	}
}

// namespaceHosts serve JSON-LD contexts and vocabularies; any other host must be an example
// host (RFC 2606, RFC 6761).
var namespaceHosts = []string{
	"www.w3.org", "w3id.org", "purl.archive.org", "schema.org", "ostatus.org", "joinmastodon.org",
	"misskey-hub.net", "gotosocial.org", "litepub.social", "joinsharkey.org", "fedibird.com", "smithereen.software",
}

var urlPattern = regexp.MustCompile(`[a-z][a-z0-9+.-]*://[^\s"'<>\\]+`)

func exampleHost(h string) bool {
	for _, d := range []string{"example", "example.com", "example.org", "example.net"} {
		if h == d || strings.HasSuffix(h, "."+d) {
			return true
		}
	}
	return false
}

// The fixtures carry example hosts and the example key only, never those of real servers.
func TestFederationFixturesAnonymized(t *testing.T) {
	for _, f := range federationFixtures(t) {
		for _, raw := range urlPattern.FindAllString(string(f.JSON), -1) {
			u, err := url.Parse(raw)
			if err != nil {
				t.Errorf("%s: unparsable URL %q", f.Path(), raw)
				continue
			}
			h := u.Hostname()
			if !exampleHost(h) && !slices.Contains(namespaceHosts, h) {
				t.Errorf("%s: URL %q has a host that is not an example host", f.Path(), raw)
			}
		}
		var doc any
		if err := json.Unmarshal(f.JSON, &doc); err != nil {
			t.Errorf("%s: %v", f.Path(), err)
			continue
		}
		walkKeys(doc, func(key string, v any) {
			if key == "publicKeyPem" && v != testkit.ExamplePublicKeyPEM {
				t.Errorf("%s: publicKeyPem is not testkit.ExamplePublicKeyPEM", f.Path())
			}
		})
	}
}

func walkKeys(v any, fn func(key string, v any)) {
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			fn(k, x)
			walkKeys(x, fn)
		}
	case []any:
		for _, x := range v {
			walkKeys(x, fn)
		}
	}
}

var (
	refType  = reflect.TypeFor[federation.Ref]()
	timeType = reflect.TypeFor[time.Time]()
)

// dump turns a normalized document into plain JSON values for the golden files. Zero values
// are left out; a Ref shows its IRI, the embedded document (if any), and whether that document
// is trusted.
func dump(v reflect.Value) any {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return dump(v.Elem())
	}
	switch v.Type() {
	case refType:
		r := v.Interface().(federation.Ref) //nolint:forcetypeassert // the type is checked above
		m := map[string]any{"ID": r.ID}
		if d := r.Untrusted(); d != nil {
			m["Embedded"] = dump(reflect.ValueOf(d))
			m["Trusted"] = r.Trusted() != nil
		}
		return m
	case timeType:
		return v.Interface().(time.Time).Format(time.RFC3339Nano) //nolint:forcetypeassert // the type is checked above
	}
	switch v.Kind() {
	case reflect.Struct:
		m := map[string]any{}
		for i := range v.NumField() {
			field := v.Type().Field(i)
			if !field.IsExported() || v.Field(i).IsZero() {
				continue
			}
			m[field.Name] = dump(v.Field(i))
		}
		return m
	case reflect.Slice, reflect.Array:
		out := make([]any, v.Len())
		for i := range v.Len() {
			out[i] = dump(v.Index(i))
		}
		return out
	default:
		return v.Interface()
	}
}
