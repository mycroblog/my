// SPDX-License-Identifier: AGPL-3.0-only

package meilisearch

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// The index settings that the adapter owns, since the port needs them as they are: what is
// searched, shown, filtered, and sorted on. A settings file that sets one is refused.
var ownedSettings = []string{"searchableAttributes", "displayedAttributes", "filterableAttributes", "sortableAttributes"}

// resetSettings are the tuning settings that go back to Meilisearch's default when the settings
// file does not set them, so that a key removed from the file does not stay in the index.
var resetSettings = []string{
	"stopWords", "synonyms", "dictionary", "separatorTokens", "nonSeparatorTokens", "proximityPrecision",
	"pagination", "faceting", "distinctAttribute", "searchCutoffMs", "prefixSearch",
}

// settingsVersion changes when the settings that the adapter owns or its defaults change, so
// that a new version sends them again over an unchanged file.
const settingsVersion = "1"

// defaultSettings returns the tuning settings of the adapter that apply when the settings file
// does not set them, as JSON.
func defaultSettings() map[string]json.RawMessage {
	locales := []map[string]any{}
	for _, l := range localized {
		locales = append(locales, map[string]any{"attributePatterns": []string{l.field}, "locales": []string{l.locale}})
	}
	return map[string]json.RawMessage{
		// The results are in the order of the IDs, newest first, whatever their relevance: sort
		// comes first, and the IDs are unique.
		"rankingRules": mustJSON([]string{"sort", "words", "typo", "proximity", "attribute", "exactness"}),
		// Every word of a query is a word of the document, as the port says, not a near one.
		"typoTolerance":       mustJSON(map[string]any{"enabled": false}),
		"localizedAttributes": mustJSON(locales),
	}
}

// ownedValues returns the values of the settings that the adapter owns.
func ownedValues() map[string]json.RawMessage {
	searchable := []string{fieldText}
	for _, l := range localized {
		searchable = append(searchable, l.field)
	}
	return map[string]json.RawMessage{
		"searchableAttributes": mustJSON(searchable),
		// Meilisearch reports where a query matched in the displayed fields only, which the
		// search reads to tell a query with no word.
		"displayedAttributes":  mustJSON([]string{"*"}),
		"filterableAttributes": mustJSON([]string{fieldStatusID, fieldAuthorID, fieldVisibility}),
		"sortableAttributes":   mustJSON([]string{fieldStatusID}),
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// indexSettings is what the adapter sends as the settings of the index, and its hash.
type indexSettings struct {
	body map[string]json.RawMessage
	hash string
}

// parseSettings returns the settings of the index from the settings file, nil when there is
// none. Its errors never quote the file.
func parseSettings(file []byte) (indexSettings, error) {
	body := map[string]json.RawMessage{}
	for _, k := range resetSettings {
		body[k] = json.RawMessage("null")
	}
	for k, v := range defaultSettings() {
		body[k] = v
	}
	if file != nil {
		tuned, err := decodeSettingsFile(file)
		if err != nil {
			return indexSettings{}, err
		}
		for k, v := range tuned {
			body[k] = v
		}
	}
	for k, v := range ownedValues() {
		body[k] = v
	}
	b, err := json.Marshal(body) // with the keys sorted
	if err != nil {
		return indexSettings{}, fmt.Errorf("meilisearch: the settings: %w", err)
	}
	sum := sha256.Sum256(append([]byte(settingsVersion+"\n"), b...))
	return indexSettings{body: body, hash: hex.EncodeToString(sum[:])}, nil
}

// decodeSettingsFile checks a settings file: a JSON object that sets none of the settings the
// adapter owns, whose rankingRules, when it has them, start with sort. Meilisearch checks the
// rest when the settings are sent.
func decodeSettingsFile(file []byte) (map[string]json.RawMessage, error) {
	var tuned map[string]json.RawMessage
	d := json.NewDecoder(bytes.NewReader(file))
	if err := d.Decode(&tuned); err != nil {
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			return nil, fmt.Errorf("meilisearch: the settings file is not JSON (at byte %d)", syntax.Offset)
		}
		return nil, errors.New("meilisearch: the settings file is not a JSON object")
	}
	if tuned == nil || d.More() {
		return nil, errors.New("meilisearch: the settings file is not one JSON object")
	}
	var owned []string
	for _, k := range ownedSettings {
		if _, ok := tuned[k]; ok {
			owned = append(owned, k)
		}
	}
	if len(owned) > 0 {
		return nil, fmt.Errorf("meilisearch: the settings file sets %s, which the server sets itself", strings.Join(owned, ", "))
	}
	if raw, ok := tuned["rankingRules"]; ok {
		var rules []string
		if err := json.Unmarshal(raw, &rules); err != nil || len(rules) == 0 || rules[0] != "sort" {
			return nil, errors.New(`meilisearch: the rankingRules of the settings file must be a list that starts with "sort", which keeps the results newest first`)
		}
		if slices.Contains(rules[1:], "sort") {
			return nil, errors.New(`meilisearch: the rankingRules of the settings file have "sort" twice`)
		}
	}
	return tuned, nil
}

// CheckSettings returns why b, the content of the operator's settings file, cannot be the
// settings of the index, nil when it can. The error never quotes the file.
func CheckSettings(b []byte) error {
	_, err := decodeSettingsFile(b)
	return err
}
