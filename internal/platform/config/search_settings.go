// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// searchSettingsFiles names, for each search engine, the key of the operator's settings file of
// the engine, where Load keeps its content, and what the content must be. The operator tunes the
// engine (analysis, ranking, shards) in that file; the server owns only its fields.
var searchSettingsFiles = map[string]struct {
	key   string
	path  func(*Config) string
	keep  func(*Config, []byte)
	check func([]byte) error
}{
	"elasticsearch": {
		key:   "search_elasticsearch_index_settings_file",
		path:  func(c *Config) string { return c.SearchElasticsearchIndexSettingsFile },
		keep:  func(c *Config, b []byte) { c.SearchElasticsearchIndexSettings = b },
		check: checkIndexSettings,
	},
	"opensearch": {
		key:   "search_opensearch_index_settings_file",
		path:  func(c *Config) string { return c.SearchOpenSearchIndexSettingsFile },
		keep:  func(c *Config, b []byte) { c.SearchOpenSearchIndexSettings = b },
		check: checkIndexSettings,
	},
	"meilisearch": {
		key:   "search_meilisearch_settings_file",
		path:  func(c *Config) string { return c.SearchMeilisearchSettingsFile },
		keep:  func(c *Config, b []byte) { c.SearchMeilisearchSettings = b },
		check: checkMeilisearchSettings,
	},
}

// loadSearchSettings reads the settings file of the engine that search names, when it has one,
// and keeps its content in cfg once it passes the engine's check, so that the adapter gets what
// the configuration checked. The files of the other engines are not read. The content reaches no
// log and no error: only the path does.
func loadSearchSettings(cfg *Config, src Source) []Issue {
	f, ok := searchSettingsFiles[cfg.Search]
	if !ok || f.path(cfg) == "" {
		return nil
	}
	path := f.path(cfg)
	data, err := src.ReadFile(path)
	if err != nil {
		var pe *os.PathError
		if errors.As(err, &pe) {
			err = pe.Err
		}
		return []Issue{{Key: f.key, Message: fmt.Sprintf("cannot read %s: %v", path, err)}}
	}
	if err := f.check(data); err != nil {
		return []Issue{{Key: f.key, Message: path + ": " + err.Error()}}
	}
	f.keep(cfg, data)
	return nil
}

// checkIndexSettings accepts one JSON object, the settings of an index of Elasticsearch or
// OpenSearch, without mappings, which the server owns. The error never quotes the file.
func checkIndexSettings(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	var v map[string]json.RawMessage
	if err := dec.Decode(&v); err != nil || v == nil || dec.More() {
		return errors.New("must hold one JSON object, the settings of an index")
	}
	if _, ok := v["mappings"]; ok {
		return errors.New("must hold the settings only, without mappings: the server owns the mapping")
	}
	return nil
}
