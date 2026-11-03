// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"os"
	"regexp"
	"strings"
)

// setting is a row of the table in the generated configuration reference.
type setting struct{ defaultValue, required string }

var settingKey = regexp.MustCompile("^\\[`([a-z0-9_]+)`\\]\\(#[^)]*\\)$")

// parseSettings reads the table of docs/reference/configuration.md, whose columns are the key, the environment
// variable, the default, whether it is required, and whether it is a secret. The reference is generated from the
// schema and a check keeps it current, so it says what the schema says, in every release.
func parseSettings(path string) (map[string]setting, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	settings := map[string]setting{}
	for line := range strings.SplitSeq(string(data), "\n") {
		if !strings.HasPrefix(line, "| [") || !strings.HasSuffix(line, " |") {
			continue
		}
		cells := strings.Split(strings.TrimSuffix(strings.TrimPrefix(line, "| "), " |"), " | ")
		if len(cells) < 5 {
			continue
		}
		key := settingKey.FindStringSubmatch(cells[0])
		if key == nil {
			continue
		}
		settings[key[1]] = setting{
			defaultValue: strings.Join(cells[2:len(cells)-2], " | "),
			required:     cells[len(cells)-2],
		}
	}
	if len(settings) == 0 {
		return nil, errors.New("no table of settings")
	}
	return settings, nil
}
