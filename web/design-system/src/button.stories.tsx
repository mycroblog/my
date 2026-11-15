// SPDX-License-Identifier: AGPL-3.0-only

// The button: Default, driven by its props in the Controls, a story per kind and state (hover, press and keyboard
// focus shown at rest), and one sheet, Variants, of every kind in every state with its label's contrast in the
// toolbar's theme, at the sizes of a timeline and a form, beside a text field; and the text button and link in a line.

import { color, contrast, mix, press, rootPx, text, type ColorName, type Scheme } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button, TextButton, TextLink, type ButtonKind, type ButtonSize } from "./button";
import { Preview, type PreviewState } from "./preview";

const rem = (px: number) => `${px / rootPx}rem`;

type Lang = "en" | "ja";
type Shown = "none" | PreviewState;

const kinds: readonly ButtonKind[] = ["primary", "secondary", "quiet"];
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

type Args = { label: string; kind: ButtonKind; size: ButtonSize; isDisabled: boolean; busy: boolean; shown: Shown };

/** The button of the args, in the state `shown` at rest. */
const Specimen = ({ label, kind, size, isDisabled, busy, shown }: Args) => (
  <Preview state={shown === "none" ? undefined : shown}>
    <Button kind={kind} size={size} isDisabled={isDisabled} busy={busy}>
      {label}
    </Button>
  </Preview>
);

const caption = {
  color: "var(--color-muted-text)",
  fontSize: `${text.caption.size}rem`,
  lineHeight: text.caption.lineHeight,
} as const;
const heading = { ...caption, margin: `0 0 ${rem(8)}` } as const;
const line = { alignItems: "center", display: "flex", flexWrap: "wrap", gap: rem(8) } as const;

const states: readonly [string, Partial<Args>][] = [
  ["rest", {}],
  ["hover", { shown: "hover" }],
  ["pressed", { shown: "pressed" }],
  ["focus-visible", { shown: "focus-visible" }],
  ["disabled", { isDisabled: true }],
  ["busy", { busy: true }],
];

// The buttons stay one group: beside the field where they fit, below it where they do not.
function Row({ size, lang, field = false }: { size: ButtonSize; lang: Lang; field?: boolean }) {
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
          <Specimen key={kind} {...rest} kind={kind} size={size} label={labels[kind][lang]} />
        ))}
      </div>
    </div>
  );
}

const rest: Args = { label: "Post", kind: "primary", size: "form", isDisabled: false, busy: false, shown: "none" };

/** Every kind in every state with its label's contrast in `scheme`, then the sizes beside a text field. */
function VariantSheet({ scheme }: { scheme: Scheme }) {
  return (
    <div style={{ fontSize: `${text.body.size}rem`, lineHeight: text.body.lineHeight }}>
      {kinds.map((kind) => (
        <div key={kind} style={{ marginBottom: rem(16) }}>
          <div style={{ ...caption, marginBottom: rem(8) }}>
            {kind} · label contrast in {scheme} {labelContrast(kind, scheme)}
          </div>
          <div style={{ display: "flex", flexWrap: "wrap", gap: `${rem(8)} ${rem(16)}` }}>
            {states.map(([name, state]) => (
              <div key={name} style={{ display: "grid", gap: rem(4), justifyItems: "start" }}>
                <span style={caption}>{name}</span>
                <Specimen {...rest} {...state} kind={kind} label={labels[kind].en} />
              </div>
            ))}
          </div>
        </div>
      ))}
      <h4 style={heading}>Timeline size</h4>
      <Row size="timeline" lang="en" />
      <Row size="timeline" lang="ja" />
      <h4 style={heading}>Form size, next to a text field</h4>
      <Row size="form" lang="en" field />
      <Row size="form" lang="ja" field />
    </div>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = {
  title: "Button",
  tags: ["themes"],
  args: rest,
  argTypes: {
    label: { control: "text" },
    kind: { control: "select", options: kinds },
    size: { control: "select", options: ["timeline", "form"] },
    isDisabled: { control: "boolean" },
    busy: { control: "boolean" },
    shown: {
      control: "select",
      options: ["none", "hover", "pressed", "focus-visible"],
      description: "A state shown at rest, as the pointer or the keyboard would put it",
    },
  },
  render: (args) => <Specimen {...args} />,
} satisfies Meta<Args>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Primary: Story = { args: { kind: "primary", label: labels.primary.en } };
export const Secondary: Story = { args: { kind: "secondary", label: labels.secondary.en } };
export const Quiet: Story = { args: { kind: "quiet", label: labels.quiet.en } };
export const Disabled: Story = { args: { isDisabled: true } };
export const Busy: Story = { args: { busy: true } };
export const Hovered: Story = { args: { shown: "hover" } };
export const Pressed: Story = { args: { shown: "pressed" } };
export const FocusVisible: Story = { args: { shown: "focus-visible" } };
export const Japanese: Story = { args: { label: labels.primary.ja } };
export const Variants: Story = {
  render: (_, { globals }) => <VariantSheet scheme={globals["theme"] === "dark" ? "dark" : "light"} />,
};

// The text button and the text link inside a line of muted text, as a feed's notice holds them: they read alike.
export const InText: Story = {
  render: () => (
    <p className="m-0 text-caption text-muted-text">
      Sign in to see this screen. · <TextLink href="#sign-in">Sign in</TextLink> ·{" "}
      <TextButton onPress={() => undefined}>Try again</TextButton>
    </p>
  ),
};
