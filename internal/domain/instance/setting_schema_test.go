// SPDX-License-Identifier: AGPL-3.0-only

package instance_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/domain/instance"
)

// settingKeyCheck is the CHECK of instance_setting_revisions.key, the quoted keys of its IN list in group 1.
var settingKeyCheck = regexp.MustCompile(`(?s)CREATE TABLE instance_setting_revisions \(.*?\bkey\b[^,]*?CHECK \(key IN \((.*?)\)\)`)

var quoted = regexp.MustCompile(`'([^']*)'`)

// TestSettingKeysInSchema fails when a setting definition's key is missing from the CHECK of
// instance_setting_revisions.key of a dialect, which the database would refuse only when the setting is first
// written, or when the CHECK lists a key that no definition has.
func TestSettingKeysInSchema(t *testing.T) {
	var keys []string
	for _, d := range instance.Definitions() {
		keys = append(keys, string(d.Key))
	}
	for _, dialect := range []string{"sqlite", "postgres"} {
		file := "schema/09-instance/" + dialect + ".sql"
		text, err := os.ReadFile(filepath.Join("..", "..", "..", filepath.FromSlash(file))) //nolint:gosec // fixed path inside the repository
		if err != nil {
			t.Fatal(err)
		}
		m := settingKeyCheck.FindSubmatch(text)
		if m == nil {
			t.Fatalf("%s: no CHECK (key IN (...)) on instance_setting_revisions", file)
		}
		var checked []string
		for _, q := range quoted.FindAllSubmatch(m[1], -1) {
			checked = append(checked, string(q[1]))
		}
		for _, k := range keys {
			if !slices.Contains(checked, k) {
				t.Errorf("setting %q is missing from the CHECK of instance_setting_revisions.key in %s", k, file)
			}
		}
		for _, k := range checked {
			if !slices.Contains(keys, k) {
				t.Errorf("%s: the CHECK of instance_setting_revisions.key lists %q, which no setting definition has", file, k)
			}
		}
	}
}
