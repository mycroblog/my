// SPDX-License-Identifier: AGPL-3.0-only

package opensearch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// mappingVersion is the version of indexDefinition. A release that changes the mapping or the
// analysis raises it: Ensure then creates the index of the new version and moves the alias to it,
// and the operator rebuilds the index (my search rebuild), since the documents of the old index
// stay there.
const mappingVersion = 1

// The analyzers of the text. Both lowercase and fold the full-width Latin letters and the
// half-width katakana (cjk_width), and split the text with the standard tokenizer, which splits
// the scripts written with spaces into words and Chinese, Japanese, and Korean text into single
// characters. cjk_bigram joins each pair of neighbouring CJK characters: the index keeps the
// single characters as well, so that a one-character query finds them, and a query uses the pairs
// only, so that 東京 does not find a text with 東 and 京 apart.
const (
	textAnalyzer       = "my_text"
	textSearchAnalyzer = "my_text_search"
)

// indexDefinition returns the settings and the mapping of the index of mappingVersion. The
// mapping is strict: a field it does not declare is refused rather than guessed.
func indexDefinition() map[string]any {
	analyzer := func(bigram string) map[string]any {
		return map[string]any{"type": "custom", "tokenizer": "standard", "filter": []string{"cjk_width", "lowercase", bigram}}
	}
	keyword := map[string]any{"type": "keyword"}
	return map[string]any{
		"settings": map[string]any{
			"analysis": map[string]any{
				"filter": map[string]any{
					"my_cjk_bigram":          map[string]any{"type": "cjk_bigram", "output_unigrams": false},
					"my_cjk_bigram_unigrams": map[string]any{"type": "cjk_bigram", "output_unigrams": true},
				},
				"analyzer": map[string]any{
					textAnalyzer:       analyzer("my_cjk_bigram_unigrams"),
					textSearchAnalyzer: analyzer("my_cjk_bigram"),
				},
			},
		},
		"mappings": map[string]any{
			"dynamic": "strict",
			"properties": map[string]any{
				"status_id":    keyword,
				"author_id":    keyword,
				"visibility":   keyword,
				"language":     keyword,
				"published_at": map[string]any{"type": "date"},
				"text":         map[string]any{"type": "text", "analyzer": textAnalyzer, "search_analyzer": textSearchAnalyzer},
			},
		},
	}
}

// versionedName returns the name of the index of a version of the mapping.
func versionedName(alias string, version int) string {
	return alias + "-v" + strconv.Itoa(version)
}

// parseVersion returns the version of the mapping that an index behind the alias has, and false
// for an index that Ensure did not name.
func parseVersion(alias, index string) (int, bool) {
	s, ok := strings.CutPrefix(index, alias+"-v")
	if !ok {
		return 0, false
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 || strconv.Itoa(v) != s {
		return 0, false
	}
	return v, true
}

// Ensure creates the index of mappingVersion and points the alias at it, unless the alias points
// at it already. It can run in several processes at once: creating an index that exists and
// adding an alias that is there are not errors. When the alias pointed at an index of an older
// version, it warns that the index needs a rebuild; when at a newer one, a binary of an older
// release is running and it leaves the alias as it is.
func (x *Index) Ensure(ctx context.Context) error {
	for attempt := 0; ; attempt++ {
		err := x.ensure(ctx)
		var e *Error
		// Another process moved the alias between the read and the update: read again.
		if attempt == 0 && errors.As(err, &e) && e.Op == "move the alias" && e.Status == http.StatusNotFound {
			continue
		}
		return err
	}
}

func (x *Index) ensure(ctx context.Context) error {
	targets, err := x.aliasTargets(ctx)
	if err != nil {
		return err
	}
	want := versionedName(x.o.Index, x.version)
	var older []string
	for _, index := range targets {
		v, ok := parseVersion(x.o.Index, index)
		switch {
		case !ok:
			return fmt.Errorf("opensearch: the alias %s points at the index %s, which my did not create; point it at nothing or choose another search_opensearch_index", x.o.Index, index)
		case v > x.version:
			x.o.Logger.WarnContext(ctx, "the search index has a newer mapping than this release of my; it is used as it is",
				"alias", x.o.Index, "index", index, "mapping_version", x.version)
			return nil
		case v < x.version:
			older = append(older, index)
		}
	}
	if len(targets) == 1 && len(older) == 0 {
		return nil
	}
	if err := x.createIndex(ctx, want); err != nil {
		return err
	}
	actions := make([]any, 0, len(older)+1)
	for _, index := range older {
		actions = append(actions, map[string]any{"remove": map[string]any{"index": index, "alias": x.o.Index}})
	}
	actions = append(actions, map[string]any{"add": map[string]any{"index": want, "alias": x.o.Index}})
	if err := x.call(ctx, "move the alias", http.MethodPost, "_aliases", nil, map[string]any{"actions": actions}, nil); err != nil {
		var e *Error
		if errors.As(err, &e) && e.Type == "invalid_alias_name_exception" {
			return fmt.Errorf("opensearch: %s is an index, not an alias: choose another search_opensearch_index, or delete that index: %w", x.o.Index, err)
		}
		return err
	}
	if len(older) > 0 {
		x.o.Logger.WarnContext(ctx, "the search index has a new mapping, which starts empty: run my search rebuild, then delete the old index",
			"alias", x.o.Index, "index", want, "old_indexes", older)
	}
	return nil
}

// aliasTargets returns the indexes that the alias points at, none when there is no alias.
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
	return targets, nil
}

// createIndex creates an index with indexDefinition; an index that exists already is left as it
// is.
func (x *Index) createIndex(ctx context.Context, name string) error {
	err := x.call(ctx, "create the index", http.MethodPut, url.PathEscape(name), nil, indexDefinition(), nil)
	var e *Error
	if errors.As(err, &e) && e.Type == "resource_already_exists_exception" {
		return nil
	}
	return err
}

// refresh makes the writes so far visible to searches and to a delete by query. The engine
// refreshes on its own within a second; deleteByQuery and the tests, whose contract wants a write
// in the next search, call it.
func (x *Index) refresh(ctx context.Context) error {
	return x.call(ctx, "refresh", http.MethodPost, x.o.Index+"/_refresh", nil, nil, nil)
}
