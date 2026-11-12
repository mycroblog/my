// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"unicode/utf8"
)

// validText reports whether s is text that every database dialect holds and compares as it
// is: valid UTF-8 without NUL.
func validText(s string) bool { return utf8.ValidString(s) && strings.IndexByte(s, 0) < 0 }

// rejectInvalidBodyText is the strict middleware that answers a request whose JSON body holds
// a string that is not valid text (a NUL; JSON reads invalid UTF-8 as U+FFFD) with a 422 that
// names the property, before the handler looks anything up by it or writes it. The platform
// rejects such text in the request target and form bodies; this covers the client API's JSON
// bodies, so that no handler passes such a value to a lookup, where PostgreSQL fails on it
// and SQLite finds nothing.
func (s *Server) rejectInvalidBodyText(next StrictHandlerFunc, _ string) StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		v := reflect.ValueOf(request)
		if v.Kind() == reflect.Struct {
			if body := v.FieldByName("Body"); body.IsValid() {
				if field := invalidTextField(body, ""); field != "" {
					l := s.Localizer(ctx)
					s.writeProblem(w, r, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue(field))
					return nil, nil
				}
			}
		}
		return next(ctx, w, r, request)
	}
}

var readerType = reflect.TypeFor[io.Reader]()

// invalidTextField returns the JSON path (media_ids[0], poll.options[1]) under path of the
// first string in v that is not valid text, or "". It follows the exported fields of structs
// by their JSON names, pointers, interfaces, slices, arrays, and maps, and skips readers (the
// multipart bodies, which their handlers validate).
func invalidTextField(v reflect.Value, path string) string {
	if v.Type().Implements(readerType) {
		return ""
	}
	switch v.Kind() {
	case reflect.String:
		if !validText(v.String()) {
			return path
		}
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			return invalidTextField(v.Elem(), path)
		}
	case reflect.Struct:
		t := v.Type()
		for i := range t.NumField() {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" {
				continue
			}
			p := path
			if !f.Anonymous {
				if name == "" {
					name = f.Name
				}
				p = joinField(path, name)
			}
			if bad := invalidTextField(v.Field(i), p); bad != "" {
				return bad
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			if bad := invalidTextField(v.Index(i), fmt.Sprintf("%s[%d]", path, i)); bad != "" {
				return bad
			}
		}
	case reflect.Map:
		for it := v.MapRange(); it.Next(); {
			if it.Key().Kind() == reflect.String && !validText(it.Key().String()) {
				if path == "" {
					return "body"
				}
				return path
			}
			p := joinField(path, fmt.Sprint(it.Key()))
			if bad := invalidTextField(it.Value(), p); bad != "" {
				return bad
			}
		}
	}
	return ""
}

func joinField(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}
