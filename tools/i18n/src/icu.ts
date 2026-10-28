// SPDX-License-Identifier: AGPL-3.0-only
// Argument extraction and subset checks on ICU MessageFormat ASTs (FormatJS parser).
import { type MessageFormatElement, parse, TYPE } from "@formatjs/icu-messageformat-parser";

/** The type an argument must have, derived from how the message uses it. */
export type ArgType =
  | { kind: "string" }
  | { kind: "number"; integer: boolean } // plural / selectordinal (integer) or {x, number}
  | { kind: "date" } // {x, date} / {x, time}
  | { kind: "select"; options: string[] } // literal options except `other`
  | { kind: "tag" }; // <b>…</b> rich-text callback

export type Args = Map<string, ArgType>;

export function parseMessage(source: string): MessageFormatElement[] {
  return parse(source, { requiresOtherClause: true, shouldParseSkeletons: true });
}

/** Collects every argument used in `ast`. Throws on conflicting usages. */
export function extractArgs(ast: MessageFormatElement[], args: Args = new Map()): Args {
  const put = (name: string, t: ArgType) => {
    const prev = args.get(name);
    if (!prev) return void args.set(name, t);
    if (prev.kind === "select" && t.kind === "select") {
      prev.options = [...new Set([...prev.options, ...t.options])].sort();
      return;
    }
    if (prev.kind === "number" && t.kind === "number") {
      prev.integer &&= t.integer;
      return;
    }
    // A plain {x} next to a typed usage adopts the typed one.
    if (t.kind === "string") return;
    if (prev.kind === "string") return void args.set(name, t);
    if (prev.kind !== t.kind) throw new Error(`argument {${name}} is used as both ${prev.kind} and ${t.kind}`);
  };
  for (const el of ast) {
    switch (el.type) {
      case TYPE.argument:
        put(el.value, { kind: "string" });
        break;
      case TYPE.number:
        put(el.value, { kind: "number", integer: false });
        break;
      case TYPE.date:
      case TYPE.time:
        put(el.value, { kind: "date" });
        break;
      case TYPE.plural:
        put(el.value, { kind: "number", integer: true });
        for (const o of Object.values(el.options)) extractArgs(o.value, args);
        break;
      case TYPE.select:
        put(el.value, {
          kind: "select",
          options: Object.keys(el.options)
            .filter((k) => k !== "other")
            .sort(),
        });
        for (const o of Object.values(el.options)) extractArgs(o.value, args);
        break;
      case TYPE.tag:
        put(el.value, { kind: "tag" });
        extractArgs(el.children, args);
        break;
    }
  }
  return args;
}

/**
 * Lists constructs outside the subset that Go and FormatJS format identically:
 * rich-text tags, number skeletons and styles other than integer / percent, and
 * date / time arguments (pass strings formatted by the caller instead).
 */
export function outsideSharedSubset(ast: MessageFormatElement[], out: string[] = []): string[] {
  for (const el of ast) {
    switch (el.type) {
      case TYPE.tag:
        out.push(`tag <${el.value}>`);
        outsideSharedSubset(el.children, out);
        break;
      case TYPE.number:
        if (el.style != null && el.style !== "integer" && el.style !== "percent") {
          out.push(`{${el.value}, number, ${typeof el.style === "string" ? el.style : "::skeleton"}}`);
        }
        break;
      case TYPE.date:
      case TYPE.time:
        out.push(`{${el.value}, ${el.type === TYPE.date ? "date" : "time"}}`);
        break;
      case TYPE.plural:
      case TYPE.select:
        for (const o of Object.values(el.options)) outsideSharedSubset(o.value, out);
        break;
    }
  }
  return out;
}

export type PluralUse = { type: "cardinal" | "ordinal"; key: string };

/** Plural / selectordinal categories used anywhere in the message, for CLDR validation. */
export function pluralCategories(ast: MessageFormatElement[], out: PluralUse[] = []): PluralUse[] {
  for (const el of ast) {
    if (el.type === TYPE.plural) {
      for (const [k, o] of Object.entries(el.options)) {
        if (!k.startsWith("=")) out.push({ type: el.pluralType === "ordinal" ? "ordinal" : "cardinal", key: k });
        pluralCategories(o.value, out);
      }
    } else if (el.type === TYPE.select) {
      for (const o of Object.values(el.options)) pluralCategories(o.value, out);
    } else if (el.type === TYPE.tag) {
      pluralCategories(el.children, out);
    }
  }
  return out;
}

/** The literal text of the message, every branch joined, without arguments or ICU syntax. */
export function literalText(ast: MessageFormatElement[]): string {
  let out = "";
  for (const el of ast) {
    if (el.type === TYPE.literal) out += el.value;
    else if (el.type === TYPE.plural || el.type === TYPE.select) {
      for (const o of Object.values(el.options)) out += `\n${literalText(o.value)}`;
    } else if (el.type === TYPE.tag) out += literalText(el.children);
  }
  return out;
}
