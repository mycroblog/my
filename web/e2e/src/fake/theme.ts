// SPDX-License-Identifier: AGPL-3.0-only

// The fake's instance theme as an administrator edits it (FR-605, ADR-0059): a draft and the set in effect, each a
// set of token overrides as the server keeps them; saving the draft whole, checked as the server checks it (a token
// it does not know, a value of the wrong shape or out of its bounds, a contrast pair under its minimum in either
// scheme, each a reason of `/api/v1/problems/invalid-theme`); the draft's stylesheet as the server writes it; and
// publishing it or restoring the set published before the one in effect, each a new revision.

import { color, contrast, contrastPairs, type ColorName } from "@my/design-tokens";
import { themeTokens } from "@my/design-tokens/theme";
import { idAt, minute, problem } from "./common";
import type { Handlers, Refusal, Schema } from "./spec";

type ThemeSet = Schema<"ThemeSet">;
type TokenValue = Schema<"ThemeTokenValue">;
type ThemeError = Schema<"ThemeError">;

/** What the fake's theme holds when it starts: nothing (by default), a draft, the draft published, or stale overrides. */
export type FakeThemeOptions = {
  /** A draft of a darker accent text and rounder controls is saved. */
  draft?: boolean;
  /** The theme in effect is that draft, published after one before it (so that it can be restored). */
  published?: boolean;
  /** The draft holds overrides an upgrade left without effect. */
  stale?: boolean;
};

/** What the theme's handlers take from the fake. */
export type ThemeContext = {
  readonly started: number;
  readonly options: FakeThemeOptions | undefined;
  readonly permissions: () => readonly Schema<"Permission">[];
  readonly origin: string;
};

/** The fake's draft as it starts: a darker accent text in light and rounder controls. */
const startTokens: readonly TokenValue[] = [
  { token: "color-accent-text", light: "#8f3434", dark: "#ff8a80" },
  { token: "radius-control", value: "0.75rem" },
];

const tokens = themeTokens();
const byName = new Map(tokens.map((t) => [t.name, t]));

/** A length in rem, as the server reads `0`, `<n>rem` and `<n>px`; none for another text. */
const remOf = (value: string): number | undefined => {
  if (value === "0") return 0;
  const m = /^(\d+(?:\.\d+)?)(rem|px)$/.exec(value);
  if (m === null) return undefined;
  return m[2] === "px" ? Number(m[1]) / 16 : Number(m[1]);
};

/** A duration in ms, as the server reads `<n>ms` and `<n>s`; none for another text. */
const msOf = (value: string): number | undefined => {
  const m = /^(\d+(?:\.\d+)?)(ms|s)$/.exec(value);
  if (m === null) return undefined;
  return m[2] === "s" ? Number(m[1]) * 1000 : Number(m[1]);
};

/** Every reason the set of `values` breaks a rule, as the server lists them. */
function errorsOf(values: readonly TokenValue[]): ThemeError[] {
  const errors: ThemeError[] = [];
  const seen = new Set<string>();
  for (const v of values) {
    const token = byName.get(v.token);
    if (token === undefined) {
      errors.push({ code: "unknown_token", token: v.token });
      continue;
    }
    if (seen.has(v.token)) errors.push({ code: "duplicate_token", token: v.token });
    seen.add(v.token);
    if (token.kind === "color") {
      for (const scheme of ["light", "dark"] as const)
        if (!/^#[0-9a-f]{6}$/.test(v[scheme] ?? "")) errors.push({ code: "invalid_value", token: v.token, scheme });
      continue;
    }
    const value = v.value ?? "";
    const n =
      token.kind === "length"
        ? remOf(value)
        : token.kind === "duration"
          ? msOf(value)
          : token.kind === "number"
            ? /^\d+(\.\d+)?$/.test(value)
              ? Number(value)
              : undefined
            : value.trim() === ""
              ? undefined
              : 0;
    if (n === undefined) errors.push({ code: "invalid_value", token: v.token });
    else if (token.min !== undefined && token.max !== undefined && (n < token.min || n > token.max))
      errors.push({ code: "out_of_range", token: v.token, min: token.min, max: token.max });
  }
  if (errors.length > 0) return errors;
  // The contrast of every pair, with the defaults of the colors not overridden.
  const colorAt = (name: ColorName, scheme: "light" | "dark") =>
    values.find((v) => v.token === `color-${name}`)?.[scheme] ?? color[name][scheme];
  for (const pair of contrastPairs)
    for (const scheme of ["light", "dark"] as const) {
      const ratio = Math.floor(contrast(colorAt(pair.fg, scheme), colorAt(pair.bg, scheme)) * 100) / 100;
      if (ratio < pair.minimum)
        errors.push({
          code: "low_contrast",
          token: `color-${pair.fg}`,
          background: `color-${pair.bg}`,
          scheme,
          ratio,
          minimum: pair.minimum,
        });
    }
  return errors;
}

/** The stylesheet of `set`, as the server writes it: one `:root` rule of the overrides, colors in `light-dark()`. */
function stylesheetOf(set: ThemeSet): string {
  const lines = set.tokens.map((v) =>
    v.light === undefined
      ? `  --${v.token}: ${v.value ?? ""};`
      : `  --${v.token}: light-dark(${v.light}, ${v.dark ?? ""});`,
  );
  return `:root {\n${lines.join("\n")}\n}\n`;
}

/** The refusal of a set that breaks a rule. */
const invalid = (errors: ThemeError[]): Refusal =>
  problem(422, "Invalid theme", { type: "/api/v1/problems/invalid-theme", theme_errors: errors } as Partial<
    Schema<"Problem">
  >);

/** The fake's theme: its handlers, and its stylesheet's address in effect. */
export function themeHandlers(c: ThemeContext): { handlers: Handlers } {
  const o = c.options ?? {};
  let n = 0;
  const setOf = (values: readonly TokenValue[], at: number): ThemeSet => ({
    id: idAt(at, 880_000 + n++),
    tokens: [...values],
    stale_tokens: [],
    saved_at: new Date(at).toISOString(),
  });
  let draft: ThemeSet | undefined =
    o.draft === true || o.published === true || o.stale === true
      ? {
          ...setOf(startTokens, c.started - 30 * minute),
          ...(o.stale === true
            ? { stale_tokens: [{ token: "color-link", light: "#2255aa", dark: "#88aaff", reason: "unknown_token" }] }
            : {}),
        }
      : undefined;
  /** The publications, oldest first; the last is in effect. */
  const published: Schema<"AdminThemeRevision">[] = [];
  const publish = (set: ThemeSet, at: number) => {
    const revision = idAt(at, 881_000 + published.length);
    published.push({
      revision,
      ...(set.tokens.length === 0 ? {} : { stylesheet_url: `${c.origin}/theme/${revision}.css` }),
      set,
      published_at: new Date(at).toISOString(),
    });
  };
  if (o.published === true && draft !== undefined) {
    publish(setOf([], c.started - 3 * 24 * 60 * minute), c.started - 3 * 24 * 60 * minute);
    publish(draft, c.started - 20 * minute);
  }
  const theme = (): Schema<"AdminTheme"> => {
    const inEffect = published.at(-1);
    return { ...(draft === undefined ? {} : { draft }), ...(inEffect === undefined ? {} : { published: inEffect }) };
  };
  const forbidden = () => !c.permissions().includes("manage_settings");
  const refused = () => problem(403, "Forbidden", { type: "/api/v1/problems/missing-permission" });
  return {
    handlers: {
      "GET /admin/theme": () => (forbidden() ? refused() : { status: 200, body: theme() }),
      "PUT /admin/theme/draft": ({ body }) => {
        if (forbidden()) return refused();
        const errors = errorsOf(body.tokens);
        if (errors.length > 0) return invalid(errors);
        draft = setOf(body.tokens, Date.now());
        return { status: 200, body: theme() };
      },
      "GET /admin/theme/draft/stylesheet": () => {
        if (forbidden()) return refused();
        if (draft === undefined) return problem(404, "Not Found");
        return { status: 200, type: "text/css", raw: stylesheetOf(draft) };
      },
      "POST /admin/theme/publish": () => {
        if (forbidden()) return refused();
        if (draft === undefined) return problem(404, "Not Found");
        const errors = errorsOf(draft.tokens);
        if (errors.length > 0) return invalid(errors);
        publish(draft, Date.now());
        return { status: 200, body: theme() };
      },
      "POST /admin/theme/restore": () => {
        if (forbidden()) return refused();
        const before = published.at(-2);
        if (before === undefined) return problem(404, "Not Found");
        publish(before.set, Date.now());
        return { status: 200, body: theme() };
      },
    },
  };
}
