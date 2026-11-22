// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"bufio"
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/mycroblog/my/api"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// featureID is the form of every identifier of Instance.features.
var featureID = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// TestFeatureRegistry checks the registry of Instance.features against the record of every
// identifier issued (features.txt): each identifier is snake_case, unique, has a one-line meaning,
// and is in the record and in the description of the OpenAPI document; each recorded one is still
// in the registry, so that a removal or a rename is refused unless the record is edited too.
func TestFeatureRegistry(t *testing.T) {
	recorded := readFeatureRecord(t)
	inRegistry := map[string]bool{}
	for _, f := range clientapi.Features() {
		if !featureID.MatchString(f.ID) {
			t.Errorf("feature %q: not of the form %s", f.ID, featureID)
		}
		if inRegistry[f.ID] {
			t.Errorf("feature %q: registered twice", f.ID)
		}
		inRegistry[f.ID] = true
		if strings.TrimSpace(f.Meaning) == "" || strings.Contains(f.Meaning, "\n") {
			t.Errorf("feature %q: the meaning is not one line", f.ID)
		}
		if !recorded[f.ID] {
			t.Errorf("feature %q: missing from features.txt; append it there", f.ID)
		}
		if !bytes.Contains(api.Spec, []byte("- `"+f.ID+"`: ")) {
			t.Errorf("feature %q: the description of Instance.features does not list it", f.ID)
		}
	}
	for id := range recorded {
		if !inRegistry[id] {
			t.Errorf("feature %q: issued (features.txt) but no longer registered; an identifier is never removed or reused", id)
		}
	}
}

// readFeatureRecord reads features.txt, refusing a malformed or repeated line.
func readFeatureRecord(t *testing.T) map[string]bool {
	t.Helper()
	data, err := os.ReadFile("features.txt")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !featureID.MatchString(line) {
			t.Errorf("features.txt:%d: %q is not of the form %s", n, line, featureID)
		}
		if out[line] {
			t.Errorf("features.txt:%d: %q is recorded twice", n, line)
		}
		out[line] = true
	}
	return out
}
