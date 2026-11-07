// SPDX-License-Identifier: AGPL-3.0-only

package elasticsearch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// The keys of the mapping's _meta: the version of the mapping and the hash of the settings that
// the index was created with, so that a later start can tell what it finds.
const (
	metaVersion  = "my_mapping_version"
	metaSettings = "my_settings_hash"
)

// The analyzers that the mapping refers to: the settings of an index define them, whether they are
// the built-in ones or the operator's (Options.Settings). AnalyzerText analyzes the text of the
// documents, and AnalyzerTextSearch, when the settings define it, the words of the queries.
const (
	AnalyzerText       = "my_text"
	AnalyzerTextSearch = "my_text_search"
)

// mapping is one version of the fields of the index, which my owns.
type mapping struct {
	// Version goes up by one with each change of Properties.
	Version int
	// InPlaceSince is the oldest version whose index takes this one in place, with the put mapping
	// API: a change that only adds fields. An older index cannot be changed in place (a field of
	// another type or analysis), so the alias moves to a new empty index of this version and the
	// documents come back with my search rebuild.
	InPlaceSince int
	// Properties are the fields of the documents. The text fields name AnalyzerText; mappings adds
	// AnalyzerTextSearch when the settings define it.
	Properties map[string]any
}

// currentMapping is the mapping of this build. Nothing is scored, so no field keeps norms;
// nothing is read back, so _source is off.
var currentMapping = mapping{
	Version:      1,
	InPlaceSince: 1,
	Properties: map[string]any{
		"status_id":  map[string]any{"type": "keyword"},
		"author_id":  map[string]any{"type": "keyword", "doc_values": false},
		"visibility": map[string]any{"type": "keyword", "doc_values": false},
		"text":       map[string]any{"type": "text", "analyzer": AnalyzerText, "norms": false},
	},
}

// DefaultSettings are the settings of an index when the operator gives none: one primary shard
// (the default of Elasticsearch), a replica once the cluster has a second node, and AnalyzerText,
// which is the
// standard tokenizer (Unicode word boundaries: a token of each kanji and each hiragana, of each
// run of katakana, and of each word of the scripts written with spaces), cjk_width (full-width
// Latin to ASCII, half-width katakana to full-width), and lowercase, all built into Elasticsearch.
// A query matches each word as a phrase of these tokens, so Japanese is found wherever the
// characters of a word stand together, without a plugin.
const DefaultSettings = `{"auto_expand_replicas":"0-1","analysis":{"analyzer":{"my_text":{"type":"custom","tokenizer":"standard","filter":["cjk_width","lowercase"]}}}}`

// settings are the settings of the indexes that an Index creates.
type settings struct {
	value map[string]any
	// hash identifies value whatever the order of its keys or its spacing.
	hash string
	// searchAnalyzer reports that value defines AnalyzerTextSearch.
	searchAnalyzer bool
}

// parseSettings reads the settings of an index, a JSON object.
func parseSettings(raw []byte) (settings, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v map[string]any
	if err := dec.Decode(&v); err != nil || v == nil || dec.More() {
		return settings{}, errors.New("elasticsearch: the index settings must be one JSON object")
	}
	canonical, err := json.Marshal(v) // json.Marshal sorts the keys of maps.
	if err != nil {
		return settings{}, fmt.Errorf("elasticsearch: the index settings: %w", err)
	}
	sum := sha256.Sum256(canonical)
	flat := map[string]bool{}
	flatten("", v, flat)
	return settings{value: v, hash: hex.EncodeToString(sum[:8]), searchAnalyzer: flat["analysis.analyzer."+AnalyzerTextSearch]}, nil
}

// flatten records the dotted path of every object of v, without a leading "index.", which
// Elasticsearch accepts with and without.
func flatten(prefix string, v map[string]any, out map[string]bool) {
	for k, child := range v {
		path := strings.TrimPrefix(prefix+k, "index.")
		out[path] = true
		for p := path; strings.Contains(p, "."); {
			p = p[:strings.LastIndex(p, ".")]
			out[p] = true
		}
		if m, ok := child.(map[string]any); ok {
			flatten(path+".", m, out)
		}
	}
}

// mappings is the mappings of an index of m created with the settings of x.
func (x *Index) mappings(m mapping) map[string]any {
	props := m.Properties
	if x.settings.searchAnalyzer {
		props = make(map[string]any, len(m.Properties))
		for name, p := range m.Properties {
			if f, ok := p.(map[string]any); ok && f["analyzer"] == AnalyzerText {
				g := make(map[string]any, len(f)+1)
				for k, v := range f {
					g[k] = v
				}
				g["search_analyzer"] = AnalyzerTextSearch
				p = g
			}
			props[name] = p
		}
	}
	return map[string]any{
		"dynamic":    "strict",
		"_source":    map[string]any{"enabled": false},
		"_meta":      x.meta(m),
		"properties": props,
	}
}

func (x *Index) meta(m mapping) map[string]any {
	return map[string]any{metaVersion: m.Version, metaSettings: x.settings.hash}
}

// indexName is the index of m that a start creates: the same name in every process with the same
// settings, so that processes starting together create one index.
func (x *Index) indexName(m mapping) string {
	return x.o.Index + "-v" + strconv.Itoa(m.Version) + "-" + x.settings.hash
}

// clearedIndexName is the index of m that Clear creates.
func (x *Index) clearedIndexName(m mapping) string {
	return x.indexName(m) + "-" + strconv.FormatInt(time.Now().UnixMilli(), 10)
}

// ensure makes the alias point at an index of m and of the settings of x: it creates one when the
// alias is missing, upgrades an older mapping in place when m allows it, moves the alias to a new
// empty index when the mapping cannot change in place or the settings differ, and refuses an index
// of a newer build, which this one would write documents it does not know to.
func (x *Index) ensure(ctx context.Context, m mapping) error {
	for range 3 {
		current, err := x.aliasIndex(ctx)
		if err != nil {
			return err
		}
		if current == "" {
			// A stopped setup may have left the index without the alias; it is empty, and is taken
			// over.
			if err := x.createDetached(ctx, x.indexName(m), m); err != nil {
				return err
			}
			if err := x.moveAlias(ctx, "", x.indexName(m)); err != nil {
				if isType(err, "invalid_alias_name_exception") {
					return fmt.Errorf("%w: an index named %s exists, so the name cannot be an alias; set another search_elasticsearch_index or delete that index", err, x.o.Index)
				}
				continue
			}
			return nil
		}
		version, hash, err := x.indexMeta(ctx, current)
		if err != nil {
			return err
		}
		switch {
		case version > m.Version:
			return fmt.Errorf("elasticsearch: the index %s has the mapping version %d, newer than the version %d of this build of my; run the newer build", current, version, m.Version)
		case version == m.Version && hash == x.settings.hash:
			return nil
		case version >= m.InPlaceSince && hash == x.settings.hash:
			if err := x.putMapping(ctx, current, m); err != nil {
				return err
			}
			x.o.Logger.InfoContext(ctx, "upgraded the mapping of the search index in place", "index", current, "from", version, "to", m.Version)
			return nil
		default:
			if err := x.replaceIndex(ctx, m, x.indexName(m)); err != nil {
				if isCreationRefused(err) {
					return err
				}
				continue
			}
			reason := "the mapping"
			if version == m.Version {
				reason = "the index settings"
			}
			x.o.Logger.WarnContext(ctx, "the search index was replaced by an empty one because "+reason+" changed; run my search rebuild to index the posts again",
				"index", x.indexName(m), "mapping_from", version, "mapping_to", m.Version)
			return nil
		}
	}
	return fmt.Errorf("elasticsearch: the alias %s kept changing while it was being set up; another process may be setting it up too", x.o.Index)
}

// aliasIndex returns the index behind the alias, empty when there is no alias.
func (x *Index) aliasIndex(ctx context.Context) (string, error) {
	var res map[string]json.RawMessage
	err := x.call(ctx, http.MethodGet, x.path("_alias", x.o.Index), nil, nil, &res)
	var e *apiError
	if errors.As(err, &e) && e.Status == http.StatusNotFound {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if len(res) != 1 {
		return "", fmt.Errorf("elasticsearch: the alias %s points at %d indexes; it must point at one", x.o.Index, len(res))
	}
	for name := range res {
		return name, nil
	}
	return "", nil
}

// indexMeta returns the mapping version of index, 0 for an index without one, and the hash of its
// settings, empty for an index without one.
func (x *Index) indexMeta(ctx context.Context, index string) (int, string, error) {
	var res map[string]struct {
		Mappings struct {
			Meta map[string]any `json:"_meta"`
		} `json:"mappings"`
	}
	if err := x.call(ctx, http.MethodGet, x.path(index, "_mapping"), nil, nil, &res); err != nil {
		return 0, "", err
	}
	meta := res[index].Mappings.Meta
	v, _ := meta[metaVersion].(float64)
	h, _ := meta[metaSettings].(string)
	return int(v), h, nil
}

// createIndex creates index with m and the settings of x, without the alias. Settings that do not
// define the analyzers of the mapping, or that Elasticsearch refuses, fail here: the error says
// which.
func (x *Index) createIndex(ctx context.Context, index string, m mapping) error {
	body := map[string]any{"settings": x.settings.value, "mappings": x.mappings(m)}
	err := x.call(ctx, http.MethodPut, x.path(index), nil, body, nil)
	if isCreationRefused(err) {
		return &settingsError{err: err}
	}
	return err
}

// isCreationRefused reports the errors of creating an index whose settings or mapping
// Elasticsearch refuses, such as an analyzer that is not defined or a plugin that is not installed.
func isCreationRefused(err error) bool {
	var s *settingsError
	return errors.As(err, &s) || isType(err, "mapper_parsing_exception") || isType(err, "illegal_argument_exception") || isType(err, "settings_exception")
}

// settingsError is an index that Elasticsearch refused to create.
type settingsError struct{ err error }

func (e *settingsError) Error() string {
	reason := ""
	var a *apiError
	if errors.As(e.err, &a) && a.Reason != "" {
		reason = a.Reason
		if r := []rune(reason); len(r) > 300 {
			reason = string(r[:300]) + "…"
		}
		reason = " (" + reason + ")"
	}
	return fmt.Sprintf("%v%s: Elasticsearch refused the index settings; they must define the analyzer %s, and may define %s for the queries, with every tokenizer and filter they name installed",
		e.err, reason, AnalyzerText, AnalyzerTextSearch)
}

func (e *settingsError) Unwrap() error { return e.err }

// createDetached creates index with m and without the alias, accepting one that exists.
func (x *Index) createDetached(ctx context.Context, index string, m mapping) error {
	err := x.createIndex(ctx, index, m)
	if isType(err, "resource_already_exists_exception") {
		return nil
	}
	return err
}

// putMapping adds the fields of m to index and records its version.
func (x *Index) putMapping(ctx context.Context, index string, m mapping) error {
	mappings := x.mappings(m)
	body := map[string]any{"_meta": mappings["_meta"], "properties": mappings["properties"]}
	return x.call(ctx, http.MethodPut, x.path(index, "_mapping"), nil, body, nil)
}

// replaceIndex moves the alias to target, a new empty index of m, and deletes the index it
// pointed at, in one request of the aliases API: no write reaches a missing alias in between.
func (x *Index) replaceIndex(ctx context.Context, m mapping, target string) error {
	old, err := x.aliasIndex(ctx)
	if err != nil {
		return err
	}
	if old == target {
		return nil
	}
	if err := x.createDetached(ctx, target, m); err != nil {
		return err
	}
	if err := x.moveAlias(ctx, old, target); err != nil {
		// Another process may have moved it already.
		if now, aerr := x.aliasIndex(ctx); aerr == nil && now == target {
			return nil
		}
		return err
	}
	return nil
}

// moveAlias points the alias at target and deletes old, when it is not empty, atomically.
func (x *Index) moveAlias(ctx context.Context, old, target string) error {
	actions := []any{map[string]any{"add": map[string]any{"index": target, "alias": x.o.Index, "is_write_index": true}}}
	if old != "" {
		actions = append(actions, map[string]any{"remove_index": map[string]any{"index": old}})
	}
	return x.call(ctx, http.MethodPost, x.path("_aliases"), nil, map[string]any{"actions": actions}, nil)
}

// isType reports an error response of the API of the given error type.
func isType(err error, typ string) bool {
	var e *apiError
	return errors.As(err, &e) && e.Type == typ
}
