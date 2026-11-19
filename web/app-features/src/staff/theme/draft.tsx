// SPDX-License-Identifier: AGPL-3.0-only

// The draft of the instance theme, at /admin/theme/draft (FR-605, ADR-0059): a form saved as a whole, its sections the
// tokens of one concern each (tokens.ts), started from the draft, else from the theme in effect. A color is its field
// for light and dark with the contrast of each pair it makes, against the colors the form holds now; the others are
// typed as the stylesheet writes them, empty for the default. A pair under its minimum (WCAG AA) is no draft: Save
// refuses it before sending, the focus on the first color at fault, as the server refuses it too. What the server
// refuses of a value stands under its field, in the words of its rule; a token is saved only where it differs from its
// default, so that the draft lists what the theme changes.

import {
  themeErrorsOf,
  useAdminTheme,
  useAdminThemeWrites,
  type AdminTheme,
  type ThemeError,
  type ThemeTokenValue,
} from "@my/app-data";
import { contrast } from "@my/design-tokens";
import {
  BackButton,
  Button,
  ColorField,
  focusRefusedField,
  FormAlert,
  ModalPresence,
  SaveState,
  SettingsScreen,
  SettingsSection,
  SubmitBar,
  TextField,
  type SchemeColors,
} from "@my/design-system";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { useStaffStrings, type T } from "../parts";
import { LeaveDialog, ScreenNotice, type AdminSubProps } from "../settings/parts";
import { pairsOf, themeSections, themeTokensByName, tokenAbouts, tokenNames } from "./tokens";

/** The address of the theme. */
const adminThemePath = "/admin/theme";

/** What the form holds: each color's value in each scheme, and each other token's text, empty for its default. */
type Values = {
  readonly colors: Readonly<Record<string, SchemeColors>>;
  readonly texts: Readonly<Record<string, string>>;
};

/** The form's values of the set `tokens` (the draft's, or the theme's in effect), the defaults where it has none. */
function valuesOf(tokens: readonly ThemeTokenValue[]): Values {
  const colors: Record<string, SchemeColors> = {};
  const texts: Record<string, string> = {};
  for (const token of themeTokensByName.values()) {
    const set = tokens.find((v) => v.token === token.name);
    if (token.kind === "color")
      colors[token.name] = { light: set?.light ?? token.light ?? "", dark: set?.dark ?? token.dark ?? "" };
    else texts[token.name] = set?.value ?? "";
  }
  return { colors, texts };
}

/** The default of the color `token` in each scheme. */
const defaultColor = (token: string): SchemeColors => {
  const t = themeTokensByName.get(token);
  return { light: t?.light ?? "", dark: t?.dark ?? "" };
};

/** The overrides of `values`, in the order of the token list: what differs from each token's default. */
function overridesOf(values: Values): ThemeTokenValue[] {
  return [...themeTokensByName.values()].flatMap((token): ThemeTokenValue[] => {
    if (token.kind === "color") {
      const c = values.colors[token.name];
      if (c === undefined || (c.light === token.light && c.dark === token.dark)) return [];
      return [{ token: token.name, light: c.light, dark: c.dark }];
    }
    const text = values.texts[token.name]?.trim() ?? "";
    return text === "" || text === token.value ? [] : [{ token: token.name, value: text }];
  });
}

/** Whether two sets of overrides are the same. */
const sameOverrides = (a: readonly ThemeTokenValue[], b: readonly ThemeTokenValue[]) =>
  JSON.stringify(a) === JSON.stringify(b);

/** A ratio cut to two decimals, as the color field shows it, so that the form and the field agree on a verdict. */
const cut = (ratio: number) => Math.floor(ratio * 100) / 100;

/** The first color of the form, in its order, whose pair in a scheme is under its minimum: its token and the scheme. */
function firstLowContrast(values: Values): { token: string; scheme: "light" | "dark" } | undefined {
  for (const section of themeSections)
    for (const token of section.tokens) {
      const own = values.colors[token];
      if (own === undefined) continue;
      for (const pair of pairsOf(token)) {
        const other = values.colors[pair.other];
        if (other === undefined) continue;
        for (const scheme of ["light", "dark"] as const)
          if (cut(contrast(own[scheme], other[scheme])) < pair.minimum) return { token, scheme };
      }
    }
  return undefined;
}

/** The words for what the server refused of the value of a token, by its rule; none for a reason of no field. */
function faultOf(t: T, error: ThemeError): string | undefined {
  const kind = themeTokensByName.get(error.token ?? "")?.kind;
  if (kind === undefined || kind === "color") return undefined;
  const words = kind === "font-family" ? "font_family" : kind;
  if (error.code === "invalid_value") return t("staff_theme.error.invalid", { kind: words });
  if (error.code === "out_of_range")
    return t("staff_theme.error.range", { kind: words, min: error.min ?? 0, max: error.max ?? 0 });
  return undefined;
}

/** What came of the last press of Save. */
type Sent =
  | { readonly kind: "idle" | "sending" | "saved" }
  | { readonly kind: "refused"; readonly faults: Readonly<Record<string, string>>; readonly reason: string };

/** The form of the draft, started from `theme`'s draft, else from the set in effect. */
function DraftForm({ theme, go, onUnsaved, leaving }: AdminSubProps & { theme: AdminTheme }): ReactNode {
  const { t } = useStaffStrings();
  const id = useId();
  const writes = useAdminThemeWrites();
  const [start, setStart] = useState(() => valuesOf(theme.draft?.tokens ?? theme.published?.set.tokens ?? []));
  const [values, setValues] = useState(start);
  const [sent, setSent] = useState<Sent>({ kind: "idle" });
  const focusNext = useRef<string | undefined>(undefined);
  const busy = sent.kind === "sending";
  const overrides = overridesOf(values);
  const unsaved = !sameOverrides(overrides, overridesOf(start));
  const fieldId = (token: string) => `${id}-${token}`;

  useEffect(() => {
    const next = focusNext.current;
    if (next === undefined) return;
    focusNext.current = undefined;
    focusRefusedField(next);
  });
  useEffect(() => {
    onUnsaved?.(unsaved && !busy);
  }, [onUnsaved, unsaved, busy]);
  useEffect(
    () => () => {
      onUnsaved?.(false);
    },
    [onUnsaved],
  );

  const change = (next: Values) => {
    setValues(next);
    // What the server said of the values goes at any change; the saved state, once the form holds a change again.
    setSent((was) => (was.kind === "refused" || was.kind === "saved" ? { kind: "idle" } : was));
  };

  const save = () => {
    if (busy) return;
    const low = firstLowContrast(values);
    if (low !== undefined) {
      setSent({ kind: "refused", faults: {}, reason: t("staff_settings.save.refused") });
      focusNext.current = `${fieldId(low.token)}-${low.scheme}`;
      return;
    }
    setSent({ kind: "sending" });
    writes.saveDraft(overrides).then(
      (saved) => {
        const now = valuesOf(saved.draft?.tokens ?? []);
        setStart(now);
        setValues(now);
        setSent({ kind: "saved" });
      },
      (error: unknown) => {
        const errors = themeErrorsOf(error) ?? [];
        const faults: Record<string, string> = {};
        for (const e of errors) {
          const words = faultOf(t, e);
          if (words !== undefined && e.token !== undefined && faults[e.token] === undefined) faults[e.token] = words;
        }
        const contrastError = errors.find((e) => e.code === "low_contrast");
        const first = themeSections.flatMap((s) => s.tokens).find((token) => faults[token] !== undefined);
        setSent({
          kind: "refused",
          faults,
          reason:
            first !== undefined
              ? t("staff_settings.save.refused")
              : contrastError !== undefined
                ? t("staff_theme.error.contrast", {
                    token: t(tokenNames[contrastError.token ?? ""] ?? "staff_theme.token.unknown"),
                    background: t(tokenNames[contrastError.background ?? ""] ?? "staff_theme.token.unknown"),
                    scheme: contrastError.scheme ?? "light",
                    ratio: contrastError.ratio ?? 0,
                    minimum: contrastError.minimum ?? 0,
                  })
                : t("staff_settings.save.failed"),
        });
        if (first !== undefined) focusNext.current = fieldId(first);
      },
    );
  };

  const faults = sent.kind === "refused" ? sent.faults : {};
  const field = (token: string): ReactNode => {
    const definition = themeTokensByName.get(token);
    const name = tokenNames[token];
    if (definition === undefined || name === undefined) return null;
    const about = tokenAbouts[token];
    if (definition.kind === "color") {
      const value = values.colors[token];
      if (value === undefined) return null;
      return (
        <ColorField
          key={token}
          id={fieldId(token)}
          label={t(name)}
          about={about === undefined ? undefined : t(about)}
          value={value}
          onChange={(next) => {
            change({ ...values, colors: { ...values.colors, [token]: next } });
          }}
          pairs={pairsOf(token).map((pair) => ({
            label: t("staff_theme.pair", {
              role: pair.role,
              name: t(tokenNames[pair.other] ?? "staff_theme.token.unknown"),
            }),
            other: values.colors[pair.other] ?? defaultColor(pair.other),
            role: pair.role,
            minimum: pair.minimum,
          }))}
        />
      );
    }
    return (
      <TextField
        key={token}
        id={fieldId(token)}
        name={token}
        label={t(name)}
        row
        hint={about === undefined ? t("staff_theme.default", { value: definition.value ?? "" }) : t(about)}
        defaultValue={values.texts[token] ?? ""}
        autoCapitalize="none"
        error={faults[token]}
        onChange={(text) => {
          change({ ...values, texts: { ...values.texts, [token]: text } });
        }}
      />
    );
  };

  return (
    <>
      <SettingsScreen
        sub
        title={t("staff_theme.draft.title")}
        back={
          <BackButton
            onPress={() => {
              go(adminThemePath);
            }}
          >
            {t("staff.entry.theme")}
          </BackButton>
        }
        foot={
          <SubmitBar
            result={
              sent.kind === "saved" ? (
                <SaveState state="saved" />
              ) : sent.kind === "refused" ? (
                <FormAlert>{sent.reason}</FormAlert>
              ) : undefined
            }
          >
            <Button kind="primary" busy={busy} onPress={save}>
              {t("staff_settings.save")}
            </Button>
          </SubmitBar>
        }
      >
        {themeSections.map((section) => (
          <SettingsSection key={section.id} heading={t(section.heading)}>
            {section.tokens.map(field)}
          </SettingsSection>
        ))}
      </SettingsScreen>
      <ModalPresence>{leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />}</ModalPresence>
    </>
  );
}

/** The draft of the theme: its form once the theme is read, until then that it loads or failed. */
export function ThemeDraftScreen(sub: AdminSubProps): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminTheme();
  if (read.theme === undefined)
    return (
      <SettingsScreen
        sub
        title={t("staff_theme.draft.title")}
        back={
          <BackButton
            onPress={() => {
              sub.go(adminThemePath);
            }}
          >
            {t("staff.entry.theme")}
          </BackButton>
        }
        notice={
          <ScreenNotice
            status={read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{ loading: t("staff_theme.loading"), failed: t("staff_theme.failed") }}
          />
        }
      />
    );
  return <DraftForm theme={read.theme} {...sub} />;
}

/** The title of the draft's screen. */
export function useThemeDraftTitle(): string {
  return useStaffStrings().t("staff_theme.draft.title");
}
