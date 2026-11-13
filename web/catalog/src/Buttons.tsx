// SPDX-License-Identifier: AGPL-3.0-only

// The button of @my/design-system on the catalog page: each kind in every state, with its label's contrast, at the
// sizes of a timeline and a form, in light and dark.

import { Button, type ButtonKind, type ButtonSize } from "@my/design-system";
import { Preview } from "@my/design-system/preview";
import { color, contrast, mix, press, rootPx, text, type ColorName, type Scheme } from "@my/design-tokens";

const rem = (px: number) => `${px / rootPx}rem`;

type State = "rest" | "hover" | "pressed" | "focus-visible" | "disabled" | "busy";
type Lang = "en" | "ja";

const kinds: readonly ButtonKind[] = ["primary", "secondary", "quiet"];
const states: readonly State[] = ["rest", "hover", "pressed", "focus-visible", "disabled", "busy"];
const labels: Record<ButtonKind, Record<Lang, string>> = {
  primary: { en: "Post", ja: "投稿する" },
  secondary: { en: "Save draft", ja: "下書き保存" },
  quiet: { en: "Cancel", ja: "キャンセル" },
};

// The label and its surface at rest, hovered and pressed, as the design system's styles draw them.
const surfaces: Record<ButtonKind, { fg: ColorName; bg: (scheme: Scheme) => [string, string, string] }> = {
  primary: {
    fg: "on-accent",
    bg: (s) => [
      color.accent[s],
      mix(press.shade, color.accent[s], press.hover),
      mix(press.shade, color.accent[s], press.pressed),
    ],
  },
  secondary: {
    fg: "text",
    bg: (s) => [color["surface-raised"][s], color["surface-raised-hover"][s], color["surface-raised-pressed"][s]],
  },
  quiet: {
    fg: "text",
    bg: (s) => [color.surface[s], color["surface-hover"][s], color["surface-pressed"][s]],
  },
};

const labelContrast = (kind: ButtonKind, scheme: Scheme) => {
  const { fg, bg } = surfaces[kind];
  const [rest, hover, pressed] = bg(scheme).map((b) => contrast(color[fg][scheme], b).toFixed(1));
  return `rest ${rest} · hover ${hover} · pressed ${pressed}`;
};

function Specimen({
  kind,
  size,
  state = "rest",
  lang,
}: {
  kind: ButtonKind;
  size: ButtonSize;
  state?: State;
  lang: Lang;
}) {
  const shown = state === "hover" || state === "pressed" || state === "focus-visible" ? state : undefined;
  return (
    <Preview state={shown}>
      <Button kind={kind} size={size} isDisabled={state === "disabled"} busy={state === "busy"} lang={lang}>
        {labels[kind][lang]}
      </Button>
    </Preview>
  );
}

const caption = {
  color: "var(--color-muted-text)",
  fontSize: `${text.caption.size}rem`,
  lineHeight: text.caption.lineHeight,
} as const;
const group = { marginBottom: rem(24) } as const;
const heading = { ...caption, margin: `0 0 ${rem(8)}` } as const;
const line = { alignItems: "center", display: "flex", flexWrap: "wrap", gap: rem(8) } as const;

// The buttons stay one group: beside the field where they fit, below it where they do not.
function Row({ size, lang, field }: { size: ButtonSize; lang: Lang; field?: boolean }) {
  const name = lang === "en" ? "Display name" : "表示名";
  return (
    <div style={{ ...line, gap: rem(12), marginBottom: rem(12) }} lang={lang}>
      {field && (
        <input
          aria-label={name}
          placeholder={name}
          style={{
            background: "var(--color-surface)",
            border: "1px solid var(--color-muted-text)",
            borderRadius: "var(--radius-control)",
            boxSizing: "border-box",
            color: "var(--color-text)",
            flex: `1 1 ${rem(192)}`,
            font: "inherit",
            height: "var(--control-form-height)",
            minWidth: 0,
            padding: "0 var(--control-form-padding-inline)",
          }}
        />
      )}
      <div style={{ ...line, flex: "0 0 auto" }}>
        {kinds.map((kind) => (
          <Specimen key={kind} kind={kind} size={size} lang={lang} />
        ))}
      </div>
    </div>
  );
}

function Panel({ scheme }: { scheme: Scheme }) {
  return (
    <div
      style={{
        background: "var(--color-surface)",
        border: "1px solid #ccc",
        borderRadius: 8,
        color: "var(--color-text)",
        colorScheme: scheme,
        fontSize: `${text.body.size}rem`,
        lineHeight: text.body.lineHeight,
        padding: rem(16),
      }}
    >
      <div style={{ ...caption, marginBottom: rem(16) }}>{scheme}</div>
      <div style={group}>
        <h4 style={heading}>States · form size</h4>
        {kinds.map((kind) => (
          <div key={kind} style={{ marginBottom: rem(16) }}>
            <div style={{ ...caption, marginBottom: rem(8) }}>
              {kind} · label contrast {labelContrast(kind, scheme)}
            </div>
            <div style={{ display: "flex", flexWrap: "wrap", gap: `${rem(8)} ${rem(16)}` }}>
              {states.map((state) => (
                <div key={state} style={{ display: "grid", gap: rem(4), justifyItems: "start" }}>
                  <span style={caption}>{state}</span>
                  <Specimen kind={kind} size="form" state={state} lang="en" />
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
      <div style={group}>
        <h4 style={heading}>Timeline size</h4>
        <Row size="timeline" lang="en" />
        <Row size="timeline" lang="ja" />
      </div>
      <div>
        <h4 style={heading}>Form size, next to a text field</h4>
        <Row size="form" lang="en" field />
        <Row size="form" lang="ja" field />
      </div>
    </div>
  );
}

export function Buttons() {
  return (
    <div style={{ display: "grid", gap: rem(16) }}>
      <Panel scheme="light" />
      <Panel scheme="dark" />
    </div>
  );
}
