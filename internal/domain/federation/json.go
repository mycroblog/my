// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// member is one property of a decoded JSON object.
type member struct {
	key   string
	value any
}

// object is a decoded JSON object that keeps its properties in document order, so "the first
// language of contentMap" is well defined. A value is one of nil, bool, json.Number, string,
// []any, or object. Every string, keys included, is storable text (see storable).
type object []member

// get returns the value of the first property named key, or nil.
func (o object) get(key string) any {
	for _, m := range o {
		if m.key == key {
			return m.value
		}
	}
	return nil
}

// has reports whether o has a property named key, even with a null value.
func (o object) has(key string) bool {
	for _, m := range o {
		if m.key == key {
			return true
		}
	}
	return false
}

// decode parses one JSON value and rejects nesting deeper than maxDepth and trailing data. Every
// string it returns, keys included, is storable text (see storable).
func decode(raw []byte, maxDepth int) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	v, err := decodeValue(dec, 0, maxDepth)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: trailing data after the document", ErrMalformed)
	}
	return v, nil
}

func decodeValue(dec *json.Decoder, depth, maxDepth int) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	if s, ok := tok.(string); ok {
		return storable(s), nil
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	if depth+1 > maxDepth {
		return nil, fmt.Errorf("%w: nesting deeper than %d", ErrLimitExceeded, maxDepth)
	}
	switch delim {
	case '{':
		obj := object{}
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
			}
			key, _ := keyTok.(string)
			key = storable(key)
			v, err := decodeValue(dec, depth+1, maxDepth)
			if err != nil {
				return nil, err
			}
			obj = append(obj, member{key: key, value: v})
		}
		if _, err := dec.Token(); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
		}
		return obj, nil
	case '[':
		arr := []any{}
		for dec.More() {
			v, err := decodeValue(dec, depth+1, maxDepth)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		if _, err := dec.Token(); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
		}
		return arr, nil
	}
	return nil, fmt.Errorf("%w: unexpected %v", ErrMalformed, delim)
}

// storable returns s as text that every dialect stores as it is: U+0000, which
// a JSON string can carry as \u0000 but PostgreSQL text cannot store, becomes U+FFFD. The JSON
// decoder already reads invalid UTF-8 and unpaired surrogates as U+FFFD, so a decoded string
// has no other value that a text column refuses. Replacing the NUL, as the decoder does with
// invalid UTF-8, rather than removing it never joins the text around it into something new
// (java\u0000script: does not become javascript:).
func storable(s string) string {
	return strings.ReplaceAll(s, "\x00", "\uFFFD")
}
