// SPDX-License-Identifier: AGPL-3.0-only

// Command securitygen generates the security metadata of the client API from the security
// requirements of api/openapi.yaml: for each operation, under the http.ServeMux pattern that
// the generated code registers it with, whether it accepts anonymous requests and the scopes
// an access token needs. The clientapi Server enforces it before the generated code parses the
// request, so the spec is the only place where an operation's scopes are written
// (scripts/generate.sh runs it).
//
//	go run ./internal/interface/clientapi/internal/securitygen -o security.gen.go api/openapi.yaml
//
// It accepts these forms of an operation's security, and fails on any other:
//
//	security: []                          anonymous, or any access token
//	security: [{oauth2: [s, ...]}]        an access token with every scope s
//	security: [{}, {oauth2: [s, ...]}]    anonymous, or an access token with every scope s
//
// An operation that takes an access token names at least one scope, and every scope is one
// that the oauth2 security scheme declares. An admin operation (admin:read or admin:write)
// names the permission of the user's roles that its handler checks with x-permission, which
// lets a token of that permission's scope call it (ADR-0090 §17).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/mycroblog/my/internal/domain/identity"
)

// schemeName is the security scheme of the client API.
const schemeName = "oauth2"

// access is the generated metadata of one operation.
type access struct {
	operation string
	anonymous bool
	scopes    []string
	// firstPartyOnly is the operation's x-first-party-only: only a first-party client's token
	// may call it.
	firstPartyOnly bool
	// deprecation is the operation's x-sunset, set for a deprecated operation.
	deprecation *deprecation
	// permission is the operation's x-permission: the permission of the user's roles that it
	// needs.
	permission string
}

// deprecation is the schedule of a deprecated operation: the Deprecation header carries
// deprecated, the Sunset header carries sunset (ADR-0090 section 8).
type deprecation struct {
	deprecated time.Time
	sunset     time.Time
}

// sunsetExtension declares when a deprecated operation was deprecated and when it may go:
//
//	x-sunset: {deprecated: "2026-11-01", sunset: "2027-02-01"}   # beside deprecated: true
//
// Both are dates (UTC midnight); sunset is not before deprecated. An operation with
// An operation marked deprecated has x-sunset, and no other operation has it.
const sunsetExtension = "x-sunset"

// parseDeprecation reads the value of x-sunset.
func parseDeprecation(raw any) (*deprecation, error) {
	b, ok := raw.(json.RawMessage)
	if !ok {
		var err error
		if b, err = json.Marshal(raw); err != nil {
			return nil, fmt.Errorf("%s must be an object", sunsetExtension)
		}
	}
	var v struct {
		Deprecated string `json:"deprecated"`
		Sunset     string `json:"sunset"`
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("%s must be an object with deprecated and sunset dates: %w", sunsetExtension, err)
	}
	d, err := time.Parse(time.DateOnly, v.Deprecated)
	if err != nil {
		return nil, fmt.Errorf("%s.deprecated must be a date like 2026-11-01", sunsetExtension)
	}
	s, err := time.Parse(time.DateOnly, v.Sunset)
	if err != nil {
		return nil, fmt.Errorf("%s.sunset must be a date like 2027-02-01", sunsetExtension)
	}
	if s.Before(d) {
		return nil, fmt.Errorf("%s.sunset is before %s.deprecated", sunsetExtension, sunsetExtension)
	}
	return &deprecation{deprecated: d, sunset: s}, nil
}

// permissionExtension names the permission of the user's roles that an admin operation needs:
//
//	x-permission: manage_invites
const permissionExtension = "x-permission"

// parsePermission reads the value of x-permission, which names a permission.
func parsePermission(raw any) (string, error) {
	var v string
	switch r := raw.(type) {
	case string:
		v = r
	case json.RawMessage:
		if err := json.Unmarshal(r, &v); err != nil {
			return "", fmt.Errorf("%s must be a string", permissionExtension)
		}
	default:
		return "", fmt.Errorf("%s must be a string", permissionExtension)
	}
	if !slices.Contains(identity.AllPermissions(), identity.Permission(v)) {
		return "", fmt.Errorf("%s %q is not a permission", permissionExtension, v)
	}
	return v, nil
}

// firstPartyOnlyExtension marks an operation that takes only the token of a first-party client.
const firstPartyOnlyExtension = "x-first-party-only"

// firstPartyOnly reads the value of x-first-party-only, which must be a boolean.
func firstPartyOnly(raw any) (bool, error) {
	var b []byte
	switch v := raw.(type) {
	case json.RawMessage:
		b = v
	case bool:
		return v, nil
	default:
		return false, fmt.Errorf("%s must be a boolean", firstPartyOnlyExtension)
	}
	var v bool
	if err := json.Unmarshal(b, &v); err != nil {
		return false, fmt.Errorf("%s must be a boolean", firstPartyOnlyExtension)
	}
	return v, nil
}

func main() {
	out := flag.String("o", "", "output file")
	flag.Parse()
	if *out == "" || flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: securitygen -o OUT SPEC")
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *out); err != nil {
		fmt.Fprintln(os.Stderr, "securitygen:", err)
		os.Exit(1)
	}
}

func run(specPath, out string) error {
	doc, err := openapi3.NewLoader().LoadFromFile(specPath)
	if err != nil {
		return err
	}
	if err := doc.Validate(context.Background()); err != nil {
		return err
	}
	ops, err := operations(doc)
	if err != nil {
		return err
	}
	src, err := render(ops)
	if err != nil {
		return err
	}
	return os.WriteFile(out, src, 0o600)
}

// operations returns the access of every operation of doc by its route pattern.
func operations(doc *openapi3.T) (map[string]access, error) {
	declared, err := declaredScopes(doc)
	if err != nil {
		return nil, err
	}
	if len(doc.Servers) != 1 {
		return nil, errors.New("the spec has exactly one server, whose URL is the base path")
	}
	base := doc.Servers[0].URL
	ops := map[string]access{}
	var errs []error
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			reqs := doc.Security
			if op.Security != nil {
				reqs = *op.Security
			}
			a, err := describe(reqs, declared)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s %s (%s): %w", method, path, op.OperationID, err))
				continue
			}
			a.operation = op.OperationID
			if raw, ok := op.Extensions[firstPartyOnlyExtension]; ok {
				if a.firstPartyOnly, err = firstPartyOnly(raw); err != nil {
					errs = append(errs, fmt.Errorf("%s %s (%s): %w", method, path, op.OperationID, err))
					continue
				}
			}
			if raw, ok := op.Extensions[permissionExtension]; ok {
				if a.permission, err = parsePermission(raw); err != nil {
					errs = append(errs, fmt.Errorf("%s %s (%s): %w", method, path, op.OperationID, err))
					continue
				}
				if !slices.ContainsFunc(a.scopes, func(s string) bool { return s == "admin:read" || s == "admin:write" }) {
					errs = append(errs, fmt.Errorf("%s %s (%s): %s needs the scope admin:read or admin:write", method, path, op.OperationID, permissionExtension))
					continue
				}
			}
			rawSunset, hasSunset := op.Extensions[sunsetExtension]
			switch {
			case op.Deprecated && !hasSunset:
				errs = append(errs, fmt.Errorf("%s %s (%s): a deprecated operation declares %s", method, path, op.OperationID, sunsetExtension))
				continue
			case !op.Deprecated && hasSunset:
				errs = append(errs, fmt.Errorf("%s %s (%s): %s needs deprecated: true", method, path, op.OperationID, sunsetExtension))
				continue
			case hasSunset:
				if a.deprecation, err = parseDeprecation(rawSunset); err != nil {
					errs = append(errs, fmt.Errorf("%s %s (%s): %w", method, path, op.OperationID, err))
					continue
				}
			}
			ops[strings.ToUpper(method)+" "+base+path] = a
		}
	}
	return ops, errors.Join(errs...)
}

// declaredScopes returns the scopes of the oauth2 scheme's flow.
func declaredScopes(doc *openapi3.T) (map[string]bool, error) {
	var ref *openapi3.SecuritySchemeRef
	if doc.Components != nil {
		ref = doc.Components.SecuritySchemes[schemeName]
	}
	if ref == nil || ref.Value == nil || ref.Value.Type != "oauth2" || ref.Value.Flows == nil || ref.Value.Flows.AuthorizationCode == nil {
		return nil, fmt.Errorf("the spec has no oauth2 security scheme %q with an authorizationCode flow", schemeName)
	}
	declared := map[string]bool{}
	for s := range ref.Value.Flows.AuthorizationCode.Scopes {
		declared[s] = true
	}
	return declared, nil
}

// describe turns the security requirements of an operation into its access.
func describe(reqs openapi3.SecurityRequirements, declared map[string]bool) (access, error) {
	var a access
	if len(reqs) == 0 {
		return access{anonymous: true}, nil
	}
	var token openapi3.SecurityRequirement
	for _, req := range reqs {
		switch {
		case len(req) == 0:
			if a.anonymous {
				return a, errors.New("security lists {} twice")
			}
			a.anonymous = true
		case token != nil:
			return a, errors.New("security lists more than one requirement besides {}")
		default:
			token = req
		}
	}
	if token == nil {
		return a, errors.New("security is only {}; write security: [] for an anonymous operation")
	}
	scopes, ok := token[schemeName]
	if !ok || len(token) != 1 {
		return a, fmt.Errorf("a requirement names only the %s scheme", schemeName)
	}
	if len(scopes) == 0 {
		return a, errors.New("an operation that takes an access token names its scopes")
	}
	for _, s := range scopes {
		if !declared[s] {
			return a, fmt.Errorf("scope %q is not declared by the %s scheme", s, schemeName)
		}
	}
	a.scopes = slices.Compact(slices.Sorted(slices.Values(scopes)))
	return a, nil
}

func render(ops map[string]access) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by securitygen from api/openapi.yaml. DO NOT EDIT.\n\n")
	b.WriteString("package clientapi\n\n")
	b.WriteString("// operationSecurity is the security of each operation of api/openapi.yaml, by the\n")
	b.WriteString("// http.ServeMux pattern that the generated code registers it with.\n")
	b.WriteString("var operationSecurity = map[string]operationAccess{\n")
	for _, pattern := range slices.Sorted(maps.Keys(ops)) {
		a := ops[pattern]
		fields := []string{"Operation: " + strconv.Quote(a.operation)}
		if a.anonymous {
			fields = append(fields, "Anonymous: true")
		}
		if a.firstPartyOnly {
			fields = append(fields, "FirstPartyOnly: true")
		}
		if a.permission != "" {
			fields = append(fields, "Permission: "+strconv.Quote(a.permission))
		}
		if a.deprecation != nil {
			fields = append(fields, fmt.Sprintf("Deprecated: %d, Sunset: %d", a.deprecation.deprecated.Unix(), a.deprecation.sunset.Unix()))
		}
		if len(a.scopes) > 0 {
			quoted := make([]string, len(a.scopes))
			for i, s := range a.scopes {
				quoted[i] = strconv.Quote(s)
			}
			fields = append(fields, "Scopes: []string{"+strings.Join(quoted, ", ")+"}")
		}
		fmt.Fprintf(&b, "\t%s: {%s},\n", strconv.Quote(pattern), strings.Join(fields, ", "))
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}
