// SPDX-License-Identifier: AGPL-3.0-only

package elasticsearch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// metaVersion is the key of the mapping's _meta that holds the version of the mapping, so that a
// later build can tell what it finds.
const metaVersion = "my_mapping_version"

// mapping is one version of the definition of the index.
type mapping struct {
	// Version goes up by one with each change of Settings or Properties.
	Version int
	// InPlaceSince is the oldest version whose index takes this one in place, with the put mapping
	// API: a change that only adds fields. An older index cannot be changed in place (a new
	// analyzer, or a field of another type), so the alias moves to a new empty index of this
	// version and the documents come back with my search rebuild.
	InPlaceSince int
	// Analysis is index.analysis of the settings, fixed when the index is created.
	Analysis map[string]any
	// Properties are the fields of the documents.
	Properties map[string]any
}

// currentMapping is the definition of the index of this build.
//
// The text is analyzed with the standard tokenizer (Unicode word boundaries: a token of each kanji
// and each hiragana, of each run of katakana, and of each word of the scripts written with spaces),
// cjk_width (full-width Latin to ASCII, half-width katakana to full-width), and lowercase, all
// built into Elasticsearch. Search matches each word as a phrase of these tokens, so Japanese is
// found wherever the characters of a word stand together, without a plugin. Nothing is scored, so
// no field keeps norms; nothing is read back, so _source is off.
var currentMapping = mapping{
	Version:      1,
	InPlaceSince: 1,
	Analysis: map[string]any{
		"analyzer": map[string]any{
			"my_text": map[string]any{"type": "custom", "tokenizer": "standard", "filter": []any{"cjk_width", "lowercase"}},
		},
	},
	Properties: map[string]any{
		"status_id":  map[string]any{"type": "keyword"},
		"author_id":  map[string]any{"type": "keyword", "doc_values": false},
		"visibility": map[string]any{"type": "keyword", "doc_values": false},
		"text":       map[string]any{"type": "text", "analyzer": "my_text", "norms": false},
	},
}

// mappings is the mappings of an index of m.
func (m mapping) mappings() map[string]any {
	return map[string]any{
		"dynamic":    "strict",
		"_source":    map[string]any{"enabled": false},
		"_meta":      map[string]any{metaVersion: m.Version},
		"properties": m.Properties,
	}
}

// indexName is the index of m that an upgrade creates: the same name in every process, so that
// processes starting together create one index.
func (x *Index) indexName(m mapping) string {
	return x.o.Index + "-v" + strconv.Itoa(m.Version)
}

// clearedIndexName is the index of m that Clear creates.
func (x *Index) clearedIndexName(m mapping) string {
	return x.indexName(m) + "-" + strconv.FormatInt(time.Now().UnixMilli(), 10)
}

// ensure makes the alias point at an index of m: it creates one when the alias is missing,
// upgrades an older index in place when m allows it and onto a new empty index when it does not,
// and refuses an index of a newer build, which this one would write documents it does not know to.
func (x *Index) ensure(ctx context.Context, m mapping) error {
	for range 3 {
		current, err := x.aliasIndex(ctx)
		if err != nil {
			return err
		}
		if current == "" {
			// The index has a name of its version, so processes starting together create one;
			// one that a stopped setup left without the alias is empty and taken over.
			if err := x.createDetached(ctx, x.indexName(m), m); err != nil {
				return err
			}
			if err := x.moveAlias(ctx, "", x.indexName(m)); err != nil {
				if isType(err, "invalid_alias_name_exception") {
					return x.creationError(err)
				}
				continue
			}
			return x.applySettings(ctx)
		}
		version, err := x.mappingVersion(ctx, current)
		if err != nil {
			return err
		}
		switch {
		case version == m.Version:
			return x.applySettings(ctx)
		case version > m.Version:
			return fmt.Errorf("elasticsearch: the index %s has the mapping version %d, newer than the version %d of this build of my; run the newer build", current, version, m.Version)
		case version >= m.InPlaceSince:
			if err := x.putMapping(ctx, current, m); err != nil {
				return err
			}
			x.o.Logger.InfoContext(ctx, "upgraded the mapping of the search index in place", "index", current, "from", version, "to", m.Version)
			return x.applySettings(ctx)
		default:
			err := x.replaceIndex(ctx, m, x.indexName(m))
			if err != nil {
				continue
			}
			x.o.Logger.WarnContext(ctx, "the mapping of the search index changed: it is now empty; run my search rebuild to index the posts again",
				"index", x.indexName(m), "from", version, "to", m.Version)
			return x.applySettings(ctx)
		}
	}
	return fmt.Errorf("elasticsearch: the alias %s kept changing while it was being set up; another process may be setting it up too", x.o.Index)
}

// creationError explains the error of creating the index when the name of the alias is taken.
func (x *Index) creationError(err error) error {
	if isType(err, "invalid_alias_name_exception") {
		return fmt.Errorf("%w: an index named %s exists, so the name cannot be an alias; set another search_elasticsearch_index or delete that index", err, x.o.Index)
	}
	return err
}

// aliasIndex returns the index behind the alias, empty when there is no alias.
func (x *Index) aliasIndex(ctx context.Context) (string, error) {
	var res map[string]struct {
		Aliases map[string]struct {
			IsWriteIndex *bool `json:"is_write_index"`
		} `json:"aliases"`
	}
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

// mappingVersion returns the version of the mapping of index, 0 for an index without one.
func (x *Index) mappingVersion(ctx context.Context, index string) (int, error) {
	var res map[string]struct {
		Mappings struct {
			Meta map[string]any `json:"_meta"`
		} `json:"mappings"`
	}
	if err := x.call(ctx, http.MethodGet, x.path(index, "_mapping"), nil, nil, &res); err != nil {
		return 0, err
	}
	v, ok := res[index].Mappings.Meta[metaVersion].(float64)
	if !ok {
		return 0, nil
	}
	return int(v), nil
}

// createIndex creates index with m, without the alias.
func (x *Index) createIndex(ctx context.Context, index string, m mapping) error {
	settings := map[string]any{"analysis": m.Analysis}
	if x.o.Replicas >= 0 {
		settings["number_of_replicas"] = x.o.Replicas
	}
	body := map[string]any{"settings": map[string]any{"index": settings}, "mappings": m.mappings()}
	return x.call(ctx, http.MethodPut, x.path(index), nil, body, nil)
}

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
	body := map[string]any{"_meta": map[string]any{metaVersion: m.Version}, "properties": m.Properties}
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

// applySettings sets the replicas of the index, which can change at any time, when they are
// configured.
func (x *Index) applySettings(ctx context.Context) error {
	if x.o.Replicas < 0 {
		return nil
	}
	body := map[string]any{"index": map[string]any{"number_of_replicas": x.o.Replicas}}
	return x.call(ctx, http.MethodPut, x.path(x.o.Index, "_settings"), nil, body, nil)
}

// isType reports an error response of the API of the given error type.
func isType(err error, typ string) bool {
	var e *apiError
	return errors.As(err, &e) && e.Type == typ
}
