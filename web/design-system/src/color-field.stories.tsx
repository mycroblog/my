// SPDX-License-Identifier: AGPL-3.0-only

// The field of a color token in the theme editor (#3258, ADR-0059), with the default theme's colors and the pairs the
// server holds them to (`contrastPairs` of @my/design-tokens): a token whose pairs all meet AA; one whose light color
// is the brand's coral, which carries no text on the light surface, so that scheme's field is invalid; what was typed
// that is no color; and the surface's colors section of the editor, each field changing the pairs of the others. The
// tokens' names and what they color are samples in English, as the administration screen would write them.

import { color, contrastPairs, rootPx, type ColorName } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { ColorField, type ColorPair, type SchemeColors } from "./color-field";
import { SettingsSection } from "./settings";

const rem = (px: number) => `${px / rootPx}rem`;

/** The column of a screen: as wide as the app's main column at most, the frame's edge around it. */
function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
}

/** The tokens' names and what they color, as the editor would say them. */
const tokens: Partial<Record<ColorName, { label: string; about: string }>> = {
  surface: { label: "Surface", about: "The page behind everything." },
  text: { label: "Text", about: "Most words: posts, names, labels." },
  "muted-text": { label: "Muted text", about: "Times, counts and the lines under a name." },
  "accent-text": { label: "Accent text", about: "Links and the focus ring." },
  "surface-raised": { label: "Raised surface", about: "Sections, buttons and fields." },
};

/** Every token's name, as the editor would say it in a pair. */
const names: Record<ColorName, string> = {
  surface: "Surface",
  text: "Text",
  "muted-text": "Muted text",
  accent: "Accent",
  "on-accent": "Text on accent",
  "accent-text": "Accent text",
  "surface-raised": "Raised surface",
  "surface-raised-hover": "Raised surface, hovered",
  "surface-raised-pressed": "Raised surface, pressed",
  "surface-hover": "Surface, hovered",
  "surface-pressed": "Surface, pressed",
  "avatar-backing": "Avatar backing",
  "quote-bar": "Quote bar",
};

type Theme = Partial<Record<ColorName, SchemeColors>>;

const defaults: Theme = Object.fromEntries(Object.keys(tokens).map((name) => [name, { ...color[name as ColorName] }]));

/** The pairs `name` makes, from its side, against the theme's other colors. */
const pairsOf = (name: ColorName, theme: Theme): ColorPair[] =>
  contrastPairs.flatMap((pair) => {
    const other = pair.fg === name ? pair.bg : pair.bg === name ? pair.fg : undefined;
    if (other === undefined) return [];
    const otherLabel = names[other].toLowerCase();
    return [
      {
        label: pair.fg === name ? `On ${otherLabel}` : `Under ${otherLabel}`,
        other: theme[other] ?? color[other],
        role: pair.fg === name ? ("text" as const) : ("background" as const),
        minimum: pair.minimum,
      },
    ];
  });

/** The field of `name`, starting from `start`, holding its value as the editor would. */
function Token({ name, start }: { name: ColorName; start?: SchemeColors }) {
  const [value, setValue] = useState<SchemeColors>(start ?? color[name]);
  const token = tokens[name] ?? { label: name, about: "" };
  return (
    <ColorField
      label={token.label}
      about={token.about}
      value={value}
      onChange={setValue}
      pairs={pairsOf(name, { ...defaults, [name]: value })}
    />
  );
}

/** Types `text` into the first field of a color inside and leaves it, as the reader would, once it is there. */
function TypedIn({ text, children }: { text: string; children: ReactNode }) {
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const input = root.current?.querySelector<HTMLInputElement>('input[type="text"]');
    if (input === null || input === undefined) return;
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set?.call(input, text);
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
  }, [text]);
  return <div ref={root}>{children}</div>;
}

const meta = { title: "Color field", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** Accent text as the default theme has it: each pair, in light and in dark, meets AA. */
export const Meets: Story = {
  render: () => (
    <Column>
      <Token name="accent-text" />
    </Column>
  ),
};

/** Accent text in the brand's coral in light: under 4.5:1 on the surface, so the light field is invalid. */
export const Below: Story = {
  render: () => (
    <Column>
      <Token name="accent-text" start={{ light: color.accent.light, dark: color["accent-text"].dark }} />
    </Column>
  ),
};

/** A light color typed that is no color (six digits but one): the field says how to write one, and the pairs keep the last color. */
export const Misformed: Story = {
  render: () => (
    <Column>
      <TypedIn text={color.accent.light.slice(0, -1)}>
        <Token name="accent-text" />
      </TypedIn>
    </Column>
  ),
};

/**
 * The editor's section of the surface and its text, inside a raised section: a change to the surface changes the
 * pairs of the text over it.
 */
export const InTheEditor: Story = {
  render: () => {
    function Editor() {
      const [theme, setTheme] = useState<Theme>(defaults);
      const names: ColorName[] = ["surface", "text", "muted-text"];
      return (
        <SettingsSection heading="Surface and text" about="Each pair must meet WCAG AA in both schemes to save.">
          {names.map((name) => (
            <ColorField
              key={name}
              label={tokens[name]?.label ?? name}
              about={tokens[name]?.about}
              value={theme[name] ?? color[name]}
              onChange={(value) => {
                setTheme({ ...theme, [name]: value });
              }}
              pairs={pairsOf(name, theme)}
            />
          ))}
        </SettingsSection>
      );
    }
    return (
      <Column>
        <Editor />
      </Column>
    );
  },
};
