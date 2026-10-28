// SPDX-License-Identifier: AGPL-3.0-only

package config

import "fmt"

// excludedValues maps a setting to its values whose adapter a build tag left out of this binary
// (NFR-PORT-4, ADR-0010), and each such value to the tag. The files with the tag's build
// constraint fill it in; Load rejects these values.
var excludedValues = map[string]map[string]string{}

// exclude records that the build tag tag leaves out the adapters of the values of key.
func exclude(tag, key string, values ...string) {
	if excludedValues[key] == nil {
		excludedValues[key] = map[string]string{}
	}
	for _, v := range values {
		excludedValues[key][v] = tag
	}
}

// ExcludedError is the error that reports that key = value selects an adapter that the build
// tag tag left out of this binary.
func ExcludedError(key, value, tag string) error {
	return fmt.Errorf("%s = %w", key, excludedError(value, tag))
}

func excludedError(value, tag string) error {
	return fmt.Errorf("%q is not in this build: the build tag %s left its adapter out (docs/reference/build-tags.md)", value, tag)
}

// testOnlyValues maps a setting to its values whose adapter only a test build has, and each such
// value to the build tag that adds it (ADR-0117 §2). The files of the builds without the tag
// fill it in; Load rejects these values.
var testOnlyValues = map[string]map[string]string{}

// testOnly records that only a build with the build tag tag has the adapters of the values of
// key.
func testOnly(tag, key string, values ...string) {
	if testOnlyValues[key] == nil {
		testOnlyValues[key] = map[string]string{}
	}
	for _, v := range values {
		testOnlyValues[key][v] = tag
	}
}

func testOnlyError(value, tag string) error {
	return fmt.Errorf("%q is only in test builds: the build tag %s adds its adapter, and released binaries never have it (docs/reference/build-tags.md)", value, tag)
}
