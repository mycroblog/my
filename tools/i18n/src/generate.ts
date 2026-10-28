// SPDX-License-Identifier: AGPL-3.0-only
// Computes the generated files from the source catalogs, without writing anything. They come in two parts
// (ADR-0081): the files Go reads, which are committed,
//   locales/{en-XA,ar-XB}/<namespace>/   pseudo-locales of the namespaces Go formats (FormatJS AST transforms),
//                                        one file per area as in the source locale
//   internal/i18n/messages_gen.go        Go typed accessors
//   internal/i18n/testdata/golden.json   FormatJS outputs the Go parity tests compare against
// and the files only the TS clients read, which are not:
//   locales/{en-XA,ar-XB}/client*/            pseudo-locales of the client namespaces
//   tools/i18n/src/runtime/messages.gen.ts   TS typed accessors
// gen.ts writes the result; check.ts compares the committed part with the committed files.
import { compile } from "@formatjs/cli-lib";
import type { MessageFormatElement } from "@formatjs/icu-messageformat-parser";
import { printAST } from "@formatjs/icu-messageformat-parser/printer.js";
import { IntlMessageFormat } from "intl-messageformat";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import {
  type Catalog,
  catalogFile,
  formattingLocale,
  goPackageDir,
  goTestdataDir,
  isClientNamespace,
  isPseudo,
  type Namespace,
  SOURCE,
  toJSON,
} from "./catalog";
import { type ArgType, extractArgs, parseMessage } from "./icu";
import { createTranslator } from "./runtime/translator";

export const runtimeDir = join(import.meta.dir, "runtime");

/** Pseudo-locale -> FormatJS pseudo-locale transform. ar-XB uses FormatJS's RTL "en-XB". */
const PSEUDO_TRANSFORMS = { "en-XA": "en-XA", "ar-XB": "en-XB" } as const;

export type Message = {
  ns: string;
  key: string;
  id: string;
  source: string;
  description: string;
  args: [string, ArgType][];
};

/** Parses every source-locale message; throws on ICU errors (check.ts reports them first). */
export function sourceMessages(catalog: Catalog): Message[] {
  const messages: Message[] = [];
  for (const [ns, entries] of Object.entries(catalog[SOURCE] ?? {}).sort(([a], [b]) => a.localeCompare(b))) {
    for (const [key, entry] of Object.entries(entries).sort(([a], [b]) => a.localeCompare(b))) {
      const args = [...extractArgs(parseMessage(entry.defaultMessage))].sort(([a], [b]) => a.localeCompare(b));
      messages.push({
        ns,
        key,
        id: `${ns}:${key}`,
        source: entry.defaultMessage,
        description: entry.description ?? "",
        args,
      });
    }
  }
  return messages;
}

/** The part of the generated files a namespace's pseudo-locales belong to. */
export const pseudoPart = (ns: string): Part => (isClientNamespace(ns) ? "ts" : "go");

async function pseudoCatalogs(catalog: Catalog, part: Part): Promise<Catalog> {
  const out: Catalog = {};
  for (const ns of Object.keys(catalog[SOURCE])
    .filter((ns) => pseudoPart(ns) === part)
    .sort()) {
    const files = [...new Set(Object.keys(catalog[SOURCE][ns]).map((key) => catalogFile(SOURCE, ns, key)))];
    for (const [locale, transform] of Object.entries(PSEUDO_TRANSFORMS)) {
      // FormatJS pseudo-localizes the parsed AST, so arguments, `#` and plural keywords
      // survive; print it back to ICU source.
      const compiled: Record<string, MessageFormatElement[]> = JSON.parse(
        await compile(files, { pseudoLocale: transform, ast: true }),
      );
      const entries: Namespace = {};
      for (const key of Object.keys(catalog[SOURCE][ns]).sort()) {
        entries[key] = {
          defaultMessage: printAST(compiled[key]),
          description: "Generated from en by tools/i18n; do not edit.",
        };
      }
      (out[locale] ??= {})[ns] = entries;
    }
  }
  return out;
}

// ---- Typed accessors ----

const words = (s: string) => s.split(/[._\-:]/).filter(Boolean);
export const goName = (id: string): string =>
  words(id)
    .map((w) => w[0].toUpperCase() + w.slice(1))
    .join("");
const GO_KEYWORDS = new Set(
  "break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var".split(
    " ",
  ),
);
const goParam = (name: string) => {
  const n = goName(name);
  const p = n[0].toLowerCase() + n.slice(1);
  return GO_KEYWORDS.has(p) ? `${p}Arg` : p;
};
const goType = (t: ArgType): string => {
  switch (t.kind) {
    case "string":
    case "select":
      return "string";
    case "number":
      return t.integer ? "int" : "float64";
    case "date":
      return "time.Time";
    case "tag":
      throw new Error("rich-text tags are client-only and have no Go accessor");
  }
};
const tsType = (t: ArgType): string => {
  switch (t.kind) {
    case "string":
      return "string";
    case "number":
      return "number";
    case "date":
      return "Date | number";
    case "select":
      // Literal options for autocompletion; `other` accepts any string.
      return [...t.options.map((o) => JSON.stringify(o)), "(string & {})"].join(" | ");
    case "tag":
      return "(chunks: string[]) => string";
  }
};
const comment = (s: string) => s.replaceAll("\n", " ");

function goAccessors(messages: Message[]): string {
  const server = messages.filter((m) => !m.ns.startsWith("client"));
  const needsTime = server.some((m) => m.args.some(([, t]) => t.kind === "date"));
  return [
    "// SPDX-License-Identifier: AGPL-3.0-only",
    "",
    "// Code generated by tools/i18n (scripts/task.sh i18n:gen); DO NOT EDIT.",
    "",
    "package i18n",
    "",
    ...(needsTime ? ['import "time"', ""] : []),
    ...server.flatMap(({ id, source, description, args }) => {
      const name = goName(id);
      // A message with two or more arguments takes them by name, in a struct of its own: positional
      // parameters of the same type (mostly string) would let a caller swap them without a compile error.
      // The blank field makes an unkeyed literal a compile error outside the package, and the
      // //exhaustruct:enforce directive has golangci-lint (exhaustruct_v5) report a field left out.
      const named = args.length >= 2;
      const argsType = `${name}Args`;
      const params = named ? `args ${argsType}` : args.map(([n, t]) => `${goParam(n)} ${goType(t)}`).join(", ");
      const value = (n: string) => (named ? `args.${goName(n)}` : goParam(n));
      const values = args.length
        ? `map[string]any{${args.map(([n]) => `${JSON.stringify(n)}: ${value(n)}`).join(", ")}}`
        : "nil";
      const fieldWidth = Math.max(0, ...args.map(([n]) => goName(n).length));
      return [
        ...(named
          ? [
              `// ${argsType} holds the arguments of ${name}, passed by name.`,
              "//",
              "//exhaustruct:enforce",
              `type ${argsType} struct {`,
              `\t${"_".padEnd(fieldWidth)} struct{} // forces keyed fields`,
              ...args.map(([n, t]) => `\t${goName(n).padEnd(fieldWidth)} ${goType(t)}`),
              "}",
              "",
            ]
          : []),
        `// ${name} formats ${JSON.stringify(id)}.${description ? ` ${comment(description)}` : ""}`,
        "//",
        `//\t${SOURCE}: ${comment(source)}`,
        `func (l *Localizer) ${name}(${params}) string {`,
        `\treturn l.format(${JSON.stringify(id)}, ${values})`,
        "}",
        "",
      ];
    }),
  ].join("\n");
}

function tsAccessors(messages: Message[]): string {
  return [
    "// SPDX-License-Identifier: AGPL-3.0-only",
    "// Code generated by tools/i18n (scripts/task.sh i18n:gen); DO NOT EDIT.",
    "",
    "export interface MessageArgs {",
    ...messages.map(({ id, source, args }) =>
      [
        `  /** ${comment(source).replaceAll("*/", "*\\/")} */`,
        `  ${JSON.stringify(id)}: {${args.length ? ` ${args.map(([n, t]) => `${n}: ${tsType(t)}`).join("; ")} ` : ""}};`,
      ].join("\n"),
    ),
    "}",
    "",
    "export type MessageKey = keyof MessageArgs;",
    "",
  ].join("\n");
}

// ---- Parity fixtures ----

type Value = string | number;
type Cases = {
  /** Catalog messages, formatted in every GOLDEN_LOCALES locale. */
  catalog: { key: string; args: Record<string, Value> }[];
  /** Standalone messages covering the shared-subset syntax. */
  syntax: { locale: string; message: string; args: Record<string, Value> }[];
};
export const GOLDEN_LOCALES = [SOURCE, "ja", "en-XA", "ar-XB"];
/** Locales with non-trivial CLDR plural rules or number formats, for the drift test. */
const CLDR_LOCALES = ["en", "ja", "fr", "pt-PT", "ru", "pl", "cy", "ar", "he", "zh-Hant", "de", "hi"];
const CLDR_NUMBERS = [0, 1, 2, 3, 5, 11, 21, 22, 101, 1000000, 1.5];

function golden(catalog: Catalog): string {
  const cases: Cases = JSON.parse(readFileSync(join(goTestdataDir, "cases.json"), "utf8"));
  const catalogOut: Record<string, string[]> = {};
  for (const locale of GOLDEN_LOCALES) {
    const t = createTranslator(catalog, locale) as (key: string, args?: Record<string, Value>) => string;
    catalogOut[locale] = cases.catalog.map(({ key, args }) => {
      const [ns, id] = key.split(/:(.*)/s);
      if (!catalog[locale]?.[ns]?.[id]) throw new Error(`cases.json: ${key} is missing in ${locale}`);
      return t(key, args);
    });
  }
  const syntax = cases.syntax.map(({ locale, message, args }) =>
    String(new IntlMessageFormat(message, formattingLocale(locale)).format(args)),
  );
  const categories = (type: "plural" | "selectordinal", cats: string[]) =>
    `{n, ${type}, ${cats.map((c) => `${c} {${c}:#}`).join(" ")}}`;
  const cldr = CLDR_LOCALES.map((locale) => {
    const cardinal = new Intl.PluralRules(locale).resolvedOptions().pluralCategories;
    const ordinal = new Intl.PluralRules(locale, { type: "ordinal" }).resolvedOptions().pluralCategories;
    const message = `${categories("plural", cardinal)} | ${categories("selectordinal", ordinal)} | {n, number}`;
    const mf = new IntlMessageFormat(message, locale);
    return { locale, message, outputs: CLDR_NUMBERS.map((n) => ({ n, want: String(mf.format({ n })) })) };
  });
  return toJSON({ catalog: catalogOut, syntax, cldr });
}

/** The files Go reads and the repository commits ("go"), or the files only the TS clients read ("ts"). */
export type Part = "go" | "ts";

/** Returns absolute path -> expected content for every generated file of the given parts. */
export async function generate(catalog: Catalog, parts: readonly Part[] = ["go", "ts"]): Promise<Map<string, string>> {
  const source: Catalog = Object.fromEntries(Object.entries(catalog).filter(([l]) => !isPseudo(l)));
  const messages = sourceMessages(source);
  const files = new Map<string, string>();
  for (const part of parts) {
    const pseudo = await pseudoCatalogs(source, part);
    for (const [locale, namespaces] of Object.entries(pseudo)) {
      for (const [ns, entries] of Object.entries(namespaces)) {
        // One file per area, like the source catalog; the keys stay sorted inside each file.
        const areas = new Map<string, Namespace>();
        for (const [key, entry] of Object.entries(entries)) {
          const file = catalogFile(locale, ns, key);
          areas.set(file, { ...areas.get(file), [key]: entry });
        }
        for (const [file, area] of areas) files.set(file, toJSON(area));
      }
    }
    if (part === "go") {
      files.set(join(goPackageDir, "messages_gen.go"), goAccessors(messages));
      files.set(join(goTestdataDir, "golden.json"), golden({ ...source, ...pseudo }));
    } else {
      files.set(join(runtimeDir, "messages.gen.ts"), tsAccessors(messages));
    }
  }
  return files;
}
