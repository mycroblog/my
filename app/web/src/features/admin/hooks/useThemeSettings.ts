// SPDX-License-Identifier: AGPL-3.0-only
// The theme screen (ADR-0059, FR-605): the overrides of the web client's design tokens and the custom CSS that
// every visitor's web client and the server-rendered pages load. The form shows the contrast of the color pairs
// as the administrator types, with the same pairs and formulas as the server (@my/ui themeContrast), and a
// preview of the overrides; the server decides whether the theme saves and lists every reason it does not.
import { type ThemeColorValue, themeContrast, type ThemeToken, themeTokens } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type Translate, useT } from "../../../lib/i18n";
import {
  cacheSavedTheme,
  customCssLimit,
  setThemeMutation,
  type ThemeError,
  themeErrorsOf,
  themeQueryOptions,
  type ThemeTokenValue,
} from "../api/theme";

const hexColor = /^#[0-9a-f]{6}$/i;
const hexAlphaColor = /^#[0-9a-f]{6}(?:[0-9a-f]{2})?$/i;

type Scheme = "light" | "dark";

/** One input of the form: what it holds now and how to change it. */
type Input = { value: string; onChange: (value: string) => void };

/** A color token: an input per theme, each empty for the default. */
export type ColorField = { token: string; defaults: ThemeColorValue; light: Input; dark: Input; alpha: boolean };

/** A token of another kind: one input, empty for the default. */
export type ValueField = { token: ThemeToken; input: Input };

/** The draft key of a token's input: the token for a value, `token:scheme` for a color. */
const draftKey = (token: string, scheme?: Scheme) => (scheme === undefined ? token : `${token}:${scheme}`);

/** The saved theme's inputs by draft key. */
function savedInputs(tokens: readonly ThemeTokenValue[]): Map<string, string> {
  const out = new Map<string, string>();
  for (const v of tokens) {
    if (v.light !== undefined) out.set(draftKey(v.token, "light"), v.light);
    if (v.dark !== undefined) out.set(draftKey(v.token, "dark"), v.dark);
    if (v.value !== undefined) out.set(draftKey(v.token), v.value);
  }
  return out;
}

/** The number of bytes of `text` in UTF-8, as the server counts the custom CSS. */
function byteLength(text: string): number {
  return new TextEncoder().encode(text).length;
}

/** The localized sentence of one reason the server gave for not saving the theme. */
function describeThemeError(t: Translate, e: ThemeError): string {
  const token = e.token ?? "";
  switch (e.code) {
    case "unknown_token":
      return t("client:admin.theme.error.unknown_token", { token });
    case "duplicate_token":
      return t("client:admin.theme.error.duplicate_token", { token });
    case "invalid_value":
      if (e.scheme === "light") return t("client:admin.theme.error.invalid_light", { token });
      if (e.scheme === "dark") return t("client:admin.theme.error.invalid_dark", { token });
      return t("client:admin.theme.error.invalid_value", { token });
    case "out_of_range":
      return t("client:admin.theme.error.out_of_range", { token, min: String(e.min ?? ""), max: String(e.max ?? "") });
    case "low_contrast":
      return contrastSentence(t, token, e.background ?? "", e.scheme ?? "light", e.ratio ?? 0, e.minimum ?? 0);
    case "css_too_long":
      return t("client:admin.theme.error.css_too_long", { max: String(e.max ?? customCssLimit) });
    case "css_invalid":
      return t("client:admin.theme.error.css_invalid");
  }
}

function contrastSentence(
  t: Translate,
  fg: string,
  bg: string,
  scheme: Scheme,
  ratio: number,
  minimum: number,
): string {
  const args = { fg, bg, ratio: ratio.toFixed(2), minimum: String(minimum) };
  return scheme === "light"
    ? t("client:admin.theme.contrast.failing_light", args)
    : t("client:admin.theme.contrast.failing_dark", args);
}

export function useThemeSettings() {
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const { data } = useSuspenseQuery(themeQueryOptions(api));
  const saved = savedInputs(data.tokens);
  const [drafts, setDrafts] = useState<Readonly<Record<string, string>>>({});
  const [cssDraft, setCssDraft] = useState<string | undefined>(undefined);
  const [status, setStatus] = useState<string | undefined>(undefined);

  const valueOf = (key: string) => drafts[key] ?? saved.get(key) ?? "";
  const input = (key: string): Input => ({
    value: valueOf(key),
    onChange: (value) => {
      setDrafts({ ...drafts, [key]: value });
    },
  });

  const colorFields: ColorField[] = themeTokens
    .filter((token) => token.kind === "color")
    .map((token) => ({
      token: token.name,
      defaults: { light: token.light ?? "", dark: token.dark ?? "" },
      light: input(draftKey(token.name, "light")),
      dark: input(draftKey(token.name, "dark")),
      alpha: token.alpha === true,
    }));
  const valueFields: ValueField[] = themeTokens
    .filter((token) => token.kind !== "color")
    .map((token) => ({ token, input: input(draftKey(token.name)) }));
  const customCss = cssDraft ?? data.custom_css;

  // A color with one theme filled takes the default for the other.
  const colorOverrides = new Map<string, ThemeColorValue>();
  for (const field of colorFields) {
    const light = field.light.value.trim();
    const dark = field.dark.value.trim();
    if (light === "" && dark === "") continue;
    colorOverrides.set(field.token, {
      light: light === "" ? field.defaults.light : light,
      dark: dark === "" ? field.defaults.dark : dark,
    });
  }
  const malformedColors = colorFields.flatMap((field) =>
    (["light", "dark"] as const).flatMap((scheme) => {
      const value = field[scheme].value.trim();
      if (value === "" || (field.alpha ? hexAlphaColor : hexColor).test(value)) return [];
      return [
        scheme === "light"
          ? t("client:admin.theme.error.invalid_light", { token: field.token })
          : t("client:admin.theme.error.invalid_dark", { token: field.token }),
      ];
    }),
  );
  const failingPairs = themeContrast(colorOverrides)
    .filter((c) => c.ratio !== null && c.ratio < c.minimum)
    .map((c) => ({
      key: `${c.fg}/${c.bg}/${c.scheme}`,
      text: contrastSentence(t, c.fg, c.bg, c.scheme, c.ratio ?? 0, c.minimum),
    }));

  const tokens: ThemeTokenValue[] = [
    ...[...colorOverrides].map(([token, value]) => ({ token, light: value.light, dark: value.dark })),
    ...valueFields
      .filter((field) => field.input.value.trim() !== "")
      .map((field) => ({ token: field.token.name, value: field.input.value.trim() })),
  ];

  // The preview applies the overrides as custom properties of its region, which the utilities inside read.
  const previewStyle: Record<string, string> = {};
  for (const [token, value] of colorOverrides) {
    if (hexAlphaColor.test(value.light) && hexAlphaColor.test(value.dark))
      previewStyle[`--${token}`] = `light-dark(${value.light}, ${value.dark})`;
  }
  for (const field of valueFields) {
    const value = field.input.value.trim();
    if (value !== "") previewStyle[`--${field.token.name}`] = value;
  }

  const save = useMutation({
    ...setThemeMutation(api),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: async (theme) => {
      await cacheSavedTheme(queryClient, api, theme);
      setDrafts({});
      setCssDraft(undefined);
      setStatus(t("client:admin.theme.saved"));
    },
  });
  const serverErrors = themeErrorsOf(save.error);

  return {
    colorFields,
    valueFields,
    customCss: {
      value: customCss,
      onChange: (value: string) => {
        setCssDraft(value);
      },
      error:
        byteLength(customCss) > customCssLimit
          ? t("client:admin.theme.error.css_too_long", { max: String(customCssLimit) })
          : undefined,
    },
    /** The colors that are not `#rrggbb` (`#rrggbbaa` where translucency is allowed), as sentences. */
    malformedColors,
    /** The color pairs below their minimum contrast with the current inputs, as sentences. */
    failingPairs,
    previewStyle,
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      save.mutate({ tokens, custom_css: customCss });
    },
    isSaving: save.isPending,
    /** Every reason the server gave for not saving, localized. */
    saveErrors: serverErrors?.map((e) => describeThemeError(t, e)),
    /** Why the last save failed otherwise, localized. */
    saveError: save.isError && serverErrors === undefined ? errorMessage(save.error) : undefined,
    status,
  };
}
