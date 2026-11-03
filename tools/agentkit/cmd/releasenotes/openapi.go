// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// operation is an operation of api/openapi.yaml.
type operation struct {
	id, method, path, description string
	deprecated                    bool
	deprecation, sunset           string
	params                        []string
}

func (o operation) key() string { return o.method + " " + o.path }

func (o operation) label() string { return fmt.Sprintf("`%s` (`%s`)", o.id, o.key()) }

// apiSpec is the part of api/openapi.yaml that the notes compare.
type apiSpec struct {
	operations map[string]operation // by key
	schemas    map[string]bool
	fields     map[string]string // what each field is, by "Schema.field.nested"
	scopes     map[string]bool
}

func parseOpenAPI(path string) (apiSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return apiSpec{}, err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return apiSpec{}, err
	}
	paths := asMap(doc["paths"])
	if len(paths) == 0 {
		return apiSpec{}, errors.New("no paths")
	}
	components := asMap(doc["components"])
	spec := apiSpec{
		operations: map[string]operation{},
		schemas:    map[string]bool{},
		fields:     map[string]string{},
		scopes:     map[string]bool{},
	}
	parameters := asMap(components["parameters"])
	for path, value := range paths {
		item := asMap(value)
		for _, method := range []string{"get", "put", "post", "delete", "patch", "options", "head"} {
			if op, ok := item[method]; ok {
				o := parseOperation(asMap(op), parameters)
				o.method, o.path = strings.ToUpper(method), path
				o.params = append(o.params, paramNames(asList(item["parameters"]), parameters)...)
				spec.operations[o.key()] = o
			}
		}
	}
	for name, schema := range asMap(components["schemas"]) {
		spec.schemas[name] = true
		flatten(asMap(schema), name, spec.fields)
	}
	for _, scheme := range asMap(components["securitySchemes"]) {
		for _, flow := range asMap(asMap(scheme)["flows"]) {
			for scope := range asMap(asMap(flow)["scopes"]) {
				spec.scopes[scope] = true
			}
		}
	}
	return spec, nil
}

func parseOperation(op, parameters map[string]any) operation {
	o := operation{id: text(op["operationId"]), description: text(op["description"])}
	o.deprecated, _ = op["deprecated"].(bool)
	sunset := asMap(op["x-sunset"])
	o.deprecation, o.sunset = text(sunset["deprecated"]), text(sunset["sunset"])
	o.params = paramNames(asList(op["parameters"]), parameters)
	return o
}

// paramNames are the query and header parameters; a path's parameters are part of the path.
func paramNames(list []any, parameters map[string]any) []string {
	var names []string
	for _, p := range list {
		param := asMap(p)
		if ref := text(param["$ref"]); ref != "" {
			param = asMap(parameters[lastSegment(ref)])
		}
		if in := text(param["in"]); in == "query" || in == "header" {
			names = append(names, text(param["name"]))
		}
	}
	return names
}

// flatten records what each property of schema is, under prefix, and goes on into nested objects, composed schemas,
// and array items. schema may be absent.
func flatten(schema map[string]any, prefix string, fields map[string]string) {
	if schema == nil {
		return
	}
	required := asList(schema["required"])
	for name, value := range asMap(schema["properties"]) {
		prop := asMap(value)
		field := prefix + "." + name
		fields[field] = describe(prop)
		if slices.Contains(required, any(name)) {
			fields[field] += ", required"
		}
		flatten(prop, field, fields)
	}
	flatten(asMap(schema["items"]), prefix, fields)
	for _, key := range []string{"allOf", "oneOf", "anyOf"} {
		for _, sub := range asList(schema[key]) {
			flatten(asMap(sub), prefix, fields)
		}
	}
}

// describe is the type of a property: enough to tell that a field changed.
func describe(prop map[string]any) string {
	if ref := text(prop["$ref"]); ref != "" {
		return lastSegment(ref)
	}
	var kind string
	switch t := prop["type"].(type) {
	case string:
		kind = t
	case []any:
		kind = joinScalars(t, " or ")
	}
	if kind == "" {
		kind = "object"
	}
	if format := text(prop["format"]); format != "" {
		kind += " (" + format + ")"
	}
	if kind == "array" {
		kind += " of " + describe(asMap(prop["items"]))
	}
	if enum := asList(prop["enum"]); len(enum) > 0 {
		kind += " enum " + joinScalars(enum, " / ")
	}
	return kind
}

func joinScalars(list []any, sep string) string {
	parts := make([]string, 0, len(list))
	for _, v := range list {
		parts = append(parts, text(v))
	}
	return strings.Join(parts, sep)
}

func lastSegment(ref string) string { return ref[strings.LastIndexByte(ref, '/')+1:] }

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

// text is a scalar as written; YAML reads an unquoted date as a time.
func text(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case time.Time:
		return t.Format(time.DateOnly)
	default:
		return fmt.Sprint(t)
	}
}
