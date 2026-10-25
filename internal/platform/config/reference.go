// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"fmt"
	"strings"
)

// ReferencePath is the generated reference, relative to the repository root.
const ReferencePath = "docs/reference/configuration.md"

// Reference renders the configuration reference from the schema as Markdown.
func Reference() string {
	var b strings.Builder
	b.WriteString(`<!-- Code generated from internal/platform/config/schema.go by "scripts/task.sh go:config-docs". DO NOT EDIT. -->

# Configuration reference

The server reads each setting from these sources, highest precedence first (ADR-0010):

1. The environment variable ` + "`" + EnvPrefix + "<KEY>`" + ` (the key in upper case).
2. The TOML file named by ` + "`" + EnvConfigFile + "`" + ` (or ` + "`my config check -config <path>`" + `), with the keys below at the top level.
3. The default.

Secrets are read only from the environment: either the variable itself or ` + "`<NAME>_FILE`" + `, the path of a file that holds the value
(trailing newlines are removed), for Docker and Kubernetes secrets (ADR-0019 §6). Setting a secret in the TOML file is an error.

Adapters are always selected by an explicit key; nothing is inferred from a URL scheme.
Values marked "not in v0" are decided in an ADR but not shipped in v0, and are rejected.
A binary built with a build tag that leaves an adapter out rejects the values that select it ([build tags](build-tags.md)).

` + "`my config check`" + ` validates the configuration and lists every problem at once; the server refuses to start on any of them.
The problems are described in the language of the process locale (` + "`LANGUAGE`, `LC_ALL`, `LC_MESSAGES`, `LANG`" + `; English for ` + "`C`" + ` or an unsupported language),
followed by the technical detail in English.

A setting that a release renamed or removed is one of those problems, in the config file and in the environment alike:
the error names the new key, or tells why the setting was removed ([renamed and removed settings](#renamed-and-removed-settings)).
An unknown key in the config file is an error too. An environment variable that starts with ` + "`" + EnvPrefix + "`" + ` but sets nothing
is ignored with a warning, in the log of ` + "`my serve`" + ` and in the output of ` + "`my config check`" + `, because the platform may add such variables:
Kubernetes adds ` + "`" + EnvPrefix + "SERVICE_HOST`, `" + EnvPrefix + "PORT`" + ` and the like for a Service named ` + "`my`" + `, and those are ignored without a warning.

In the ` + "`production`" + ` profile, ` + "`my config check`" + ` then checks that the valid settings suit a production instance:
errors, such as a ` + "`host`" + ` that other servers cannot reach or a ` + "`kek`" + ` that is not random, make it exit 1, and warnings, such as ` + "`mail = none`" + `, do not.
` + "`my serve`" + ` logs both at start without refusing to start. The list of checks is in the [production checklist](../operations/production-checklist.md).

## Configuration and instance settings

The settings on this page configure the deployment: what a process runs, which adapters it uses, where they are, and its secrets.
These values are fixed for the life of a process and change only with a restart.
The policies of the instance that administrators change while it runs (registration mode, maximum post length,
default locale, which timelines are enabled) and its localized texts (name, descriptions, terms, privacy policy)
are instance settings instead: they are stored in the database with every change kept as a revision (data-model §4.9),
and are changed with ` + "`my instance set`" + ` ([instance settings](../operations/instance-settings.md)).

The two never share a key, so neither overrides the other: no environment variable or config file key sets an
instance setting, and the database never holds a setting from this page. A test keeps the two key sets disjoint.
An instance setting that has never been set reads as its default, which is defined in code.

## Summary

| Key | Environment variable | Default | Required | Secret |
| --- | --- | --- | --- | --- |
`)
	for i := range schema {
		f := &schema[i]
		fmt.Fprintf(&b, "| [`%s`](#%s) | `%s` | %s | %s | %s |\n",
			f.key, f.key, f.env(), defaultCell(f), requiredCell(f), yesNo(f.secret))
	}
	b.WriteString("\n## Settings\n")
	for i := range schema {
		f := &schema[i]
		fmt.Fprintf(&b, "\n### `%s`\n\n%s\n\n", f.key, f.doc)
		fmt.Fprintf(&b, "- Environment variable: `%s`", f.env())
		if f.secret {
			fmt.Fprintf(&b, " or `%s_FILE`", f.env())
		}
		fmt.Fprintf(&b, "\n- Default: %s\n- Required: %s\n", defaultCell(f), requiredCell(f))
		if len(f.choices) > 0 {
			b.WriteString("\n| Value | In v0 | Description |\n| --- | --- | --- |\n")
			for _, c := range f.choices {
				inV0 := "yes"
				if !c.inV0 {
					inV0 = "not in v0"
				}
				fmt.Fprintf(&b, "| `%s` | %s | %s |\n", c.value, inV0, c.doc)
			}
		}
	}
	b.WriteString("\n## Renamed and removed settings\n\n" +
		"Using one of these keys, in the config file or as its environment variable (or `<NAME>_FILE`), stops the server with an error.\n\n")
	if len(retiredKeys) == 0 {
		b.WriteString("None.\n")
		return b.String()
	}
	b.WriteString("| Key | Environment variable | Replacement or reason |\n| --- | --- | --- |\n")
	for i := range retiredKeys {
		r := &retiredKeys[i]
		replaced := r.removed
		if r.renamedTo != "" {
			replaced = fmt.Sprintf("[`%s`](#%s)", r.renamedTo, r.renamedTo)
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | %s |\n", r.key, r.env(), replaced)
	}
	return b.String()
}

func defaultCell(f *field) string {
	switch {
	case f.def != "":
		return "`" + f.def + "`"
	case f.defDoc != "":
		return f.defDoc
	default:
		return "none"
	}
}

func requiredCell(f *field) string {
	if f.requiredWhen != "" {
		return f.requiredWhen
	}
	return yesNo(f.required)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
