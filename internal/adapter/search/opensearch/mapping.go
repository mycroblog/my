// SPDX-License-Identifier: AGPL-3.0-only

package opensearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"
)

// mappingVersion is the version of mapping(). A release that changes the mapping raises it:
// Ensure then creates an index of the new version and moves the alias to it, and the operator
// rebuilds the index (my search rebuild), since the documents of the old index stay there.
const mappingVersion = 1

// The analyzers that the mapping names, which the settings of the index define: TextAnalyzer
// analyzes the text of a document, and TextSearchAnalyzer the text of a query. They are the
// contract between my and the settings file of an operator (search_opensearch_index_settings_file).
const (
	TextAnalyzer       = "my_text"
	TextSearchAnalyzer = "my_text_search"
)

// The fields of _meta of the mapping by which Ensure recognizes an index of its own.
const (
	metaVersion  = "my_mapping_version"
	metaSettings = "my_settings_hash"
)

// DefaultSettings returns the settings of the index without a settings file. Both analyzers
// split the text with the standard tokenizer, which splits the scripts written with spaces into
// words and Chinese, Japanese, and Korean text into single characters, and fold the full-width
// Latin letters and the half-width katakana (cjk_width) and the case. cjk_bigram joins each pair
// of neighbouring CJK characters: the index keeps the single characters as well, so that a
// one-character query finds them, and a query uses the pairs only, so that 東京 does not find a
// text with 東 and 京 apart. Every part is built into OpenSearch.
func DefaultSettings() json.RawMessage {
	analyzer := func(bigram string) map[string]any {
		return map[string]any{"type": "custom", "tokenizer": "standard", "filter": []string{"cjk_width", "lowercase", bigram}}
	}
	b, _ := json.Marshal(map[string]any{
		"analysis": map[string]any{
			"filter": map[string]any{
				"my_cjk_bigram":          map[string]any{"type": "cjk_bigram", "output_unigrams": false},
				"my_cjk_bigram_unigrams": map[string]any{"type": "cjk_bigram", "output_unigrams": true},
			},
			"analyzer": map[string]any{
				TextAnalyzer:       analyzer("my_cjk_bigram_unigrams"),
				TextSearchAnalyzer: analyzer("my_cjk_bigram"),
			},
		},
	})
	return b
}

// parseSettings returns the settings of o as an object and the hash of their canonical JSON
// (encoding/json sorts the keys of a map), which tells two settings apart however the file is
// formatted.
func parseSettings(raw json.RawMessage) (map[string]any, string, error) {
	if raw == nil {
		raw = DefaultSettings()
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil || settings == nil {
		return nil, "", errors.New("opensearch: the index settings must be a JSON object")
	}
	if _, ok := settings["mappings"]; ok {
		return nil, "", errors.New("opensearch: the index settings hold the settings only: my owns the mapping")
	}
	canonical, err := json.Marshal(settings)
	if err != nil {
		return nil, "", fmt.Errorf("opensearch: the index settings: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return settings, hex.EncodeToString(sum[:]), nil
}

// mapping returns the mapping of mappingVersion, with the version and the hash of the settings
// in _meta. It is strict: a field it does not declare is refused rather than guessed.
func (x *Index) mapping() map[string]any {
	keyword := map[string]any{"type": "keyword"}
	return map[string]any{
		"dynamic": "strict",
		"_meta":   map[string]any{metaVersion: x.version, metaSettings: x.settingsHash},
		"properties": map[string]any{
			"status_id":    keyword,
			"author_id":    keyword,
			"visibility":   keyword,
			"language":     keyword,
			"published_at": map[string]any{"type": "date"},
			"text":         map[string]any{"type": "text", "analyzer": TextAnalyzer, "search_analyzer": TextSearchAnalyzer},
		},
	}
}

// indexName returns the name of an index of the alias with this mapping and these settings:
// Ensure names it the same in every process, and Clear adds a suffix of its own.
func (x *Index) indexName(suffix string) string {
	name := x.o.Index + "-v" + strconv.Itoa(x.version) + "-" + x.settingsHash[:12]
	if suffix != "" {
		name += "-" + suffix
	}
	return name
}

// meta is what _meta of an index says of it.
type meta struct {
	Version  int    `json:"my_mapping_version"`
	Settings string `json:"my_settings_hash"`
}

// Ensure makes the alias point at one index of this mapping and these settings, unless it does
// already. Otherwise it creates the index and moves the alias to it; the documents stay in the
// old index, so it warns that the index needs a rebuild. An index of a newer mapping means that
// a binary of an older release is running, and it leaves the alias as it is. It can run in
// several processes at once: they create the same index.
func (x *Index) Ensure(ctx context.Context) error {
	targets, err := x.aliasTargets(ctx)
	if err != nil {
		return err
	}
	current := false
	for _, index := range targets {
		m, err := x.indexMeta(ctx, index)
		if err != nil {
			return err
		}
		switch {
		case m.Version > x.version:
			x.o.Logger.WarnContext(ctx, "the search index has a newer mapping than this release of my; it is used as it is",
				"alias", x.o.Index, "index", index, "mapping_version", x.version)
			return nil
		case m.Version == x.version && m.Settings == x.settingsHash:
			current = true
		}
	}
	if current && len(targets) == 1 {
		return nil
	}
	want := x.indexName("")
	if err := x.createIndex(ctx, want); err != nil {
		return err
	}
	if err := x.moveAlias(ctx, want, false); err != nil {
		return err
	}
	if len(targets) > 0 {
		x.o.Logger.WarnContext(ctx, "the search index has a new mapping or new settings, and starts empty: run my search rebuild, then delete the old index",
			"alias", x.o.Index, "index", want, "old_indexes", targets)
	}
	return nil
}

// Clear implements search.Index: it creates an empty index with the same mapping and settings,
// and in one request moves the alias to it and deletes the indexes the alias pointed at. It is
// fast, frees the disk at once, and leaves nothing that a delete by query has not seen yet.
func (x *Index) Clear(ctx context.Context) error {
	name := x.indexName(strconv.FormatInt(time.Now().UnixNano(), 36))
	if err := x.createIndex(ctx, name); err != nil {
		return err
	}
	return x.moveAlias(ctx, name, true)
}

// moveAlias points the alias at index alone. It removes the alias from the indexes it pointed
// at, or deletes them with deleteOld. When another process moved the alias between the read and
// the update, it reads again.
func (x *Index) moveAlias(ctx context.Context, index string, deleteOld bool) error {
	for attempt := 0; ; attempt++ {
		targets, err := x.aliasTargets(ctx)
		if err != nil {
			return err
		}
		actions := []any{map[string]any{"add": map[string]any{"index": index, "alias": x.o.Index}}}
		for _, old := range targets {
			switch {
			case old == index:
			case deleteOld:
				actions = append(actions, map[string]any{"remove_index": map[string]any{"index": old}})
			default:
				actions = append(actions, map[string]any{"remove": map[string]any{"index": old, "alias": x.o.Index}})
			}
		}
		err = x.call(ctx, "move the alias", http.MethodPost, "_aliases", nil, map[string]any{"actions": actions}, nil)
		var e *Error
		switch {
		case errors.As(err, &e) && e.Type == "invalid_alias_name_exception":
			return fmt.Errorf("opensearch: %s is an index, not an alias: choose another search_opensearch_index, or delete that index: %w", x.o.Index, err)
		case attempt < 2 && errors.As(err, &e) && e.Status == http.StatusNotFound:
			continue
		}
		return err
	}
}

// aliasTargets returns the indexes that the alias points at, sorted, none when there is no alias.
func (x *Index) aliasTargets(ctx context.Context) ([]string, error) {
	var res map[string]any
	err := x.call(ctx, "read the alias", http.MethodGet, "_alias/"+url.PathEscape(x.o.Index), nil, nil, &res)
	var e *Error
	if errors.As(err, &e) && e.Status == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	targets := make([]string, 0, len(res))
	for index := range res {
		targets = append(targets, index)
	}
	slices.Sort(targets)
	return targets, nil
}

// indexMeta returns what _meta of the mapping of index says, and an error for an index that my
// did not create.
func (x *Index) indexMeta(ctx context.Context, index string) (meta, error) {
	var res map[string]struct {
		Mappings struct {
			Meta *meta `json:"_meta"`
		} `json:"mappings"`
	}
	if err := x.call(ctx, "read the mapping", http.MethodGet, url.PathEscape(index)+"/_mapping", nil, nil, &res); err != nil {
		return meta{}, err
	}
	m := res[index].Mappings.Meta
	if m == nil || m.Version < 1 || m.Settings == "" {
		return meta{}, fmt.Errorf("opensearch: the alias %s points at the index %s, which my did not create; point it at nothing or choose another search_opensearch_index", x.o.Index, index)
	}
	return *m, nil
}

// createIndex creates an index with the settings and the mapping; an index that exists already
// is left as it is. The error of a refused creation holds the engine's reason, which names the
// setting at fault and quotes no post: the settings come from the operator.
func (x *Index) createIndex(ctx context.Context, name string) error {
	err := x.call(ctx, "create the index", http.MethodPut, url.PathEscape(name), nil,
		map[string]any{"settings": x.settings, "mappings": x.mapping()}, nil)
	var e *Error
	switch {
	case errors.As(err, &e) && e.Type == "resource_already_exists_exception":
		return nil
	case errors.As(err, &e) && e.Status == http.StatusBadRequest:
		return fmt.Errorf("opensearch: create the index %s: the settings must define the analyzers %s and %s, with plugins the cluster has (%s): %w",
			name, TextAnalyzer, TextSearchAnalyzer, e.reason, err)
	}
	return err
}

// refresh makes the writes so far visible to searches and to a delete by query. The engine
// refreshes on its own within a second; deleteByQuery and the tests, whose contract wants a write
// in the next search, call it.
func (x *Index) refresh(ctx context.Context) error {
	return x.call(ctx, "refresh", http.MethodPost, x.o.Index+"/_refresh", nil, nil, nil)
}
