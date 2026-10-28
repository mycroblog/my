// SPDX-License-Identifier: AGPL-3.0-only
// The design token sheet for Storybook (CSF 3). Like every story, it is also an axe and a visual
// snapshot test (vitest.config.ts).
// Swatches read the semantic colors with var() (theme.css emits them all) instead of building
// class names, which web-client/README.md D9 forbids.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { colorPairs, decorativeColors, surfaces } from "./tokens";

export default {
  title: "Foundations/Tokens",
} satisfies Meta;

type Story = StoryObj;

/** The text color of a UI pair's label: the text pair checked on the same background, else fg. */
function labelColor(bg: string): string {
  return colorPairs.find((pair) => pair.bg === bg && pair.use === "text")?.fg ?? "fg";
}

function ColorPairs() {
  return (
    <ul className="grid grid-cols-[repeat(auto-fill,minmax(16rem,1fr))] gap-2">
      {colorPairs.map(({ fg, bg, use }) => (
        <li
          key={`${fg}/${bg}`}
          className="flex items-center gap-3 rounded-md border border-border p-3"
          // A UI pair only needs 3:1, so its label is in the text color checked on the same background (fg on most,
          // toast-fg on the dark toast pill); the swatch shows the pair.
          style={{
            backgroundColor: `var(--color-${bg})`,
            color: `var(--color-${use === "text" ? fg : labelColor(bg)})`,
          }}
        >
          {use === "text" ? (
            <span className="text-lg font-semibold">Aa あ ع</span>
          ) : (
            <span className="size-6 rounded-sm border-2" style={{ borderColor: `var(--color-${fg})` }} />
          )}
          <code className="font-mono text-sm">
            {fg} on {bg}
          </code>
        </li>
      ))}
    </ul>
  );
}

export const Colors: Story = { render: () => <ColorPairs /> };

function DecorativeColors() {
  return (
    <ul className="flex flex-wrap gap-4">
      {decorativeColors.map((name) => (
        <li key={name} className="flex flex-col items-center gap-1">
          <span
            className="size-16 rounded-md border border-border"
            style={{ backgroundColor: `var(--color-${name})` }}
          />
          <code className="font-mono text-sm text-fg-muted">{name}</code>
        </li>
      ))}
    </ul>
  );
}

export const Decorative: Story = { render: () => <DecorativeColors /> };

/** One line per script the font stack covers; lang and dir pick the right glyphs and direction. */
const typeSamples = [
  { lang: "en", dir: "ltr", text: "The quick brown fox jumps over the lazy dog." },
  { lang: "ja", dir: "ltr", text: "いろはにほへと ちりぬるを 色は匂へど散りぬるを" },
  { lang: "ar", dir: "rtl", text: "نص حكيم له سر قاطع وذو شأن عظيم" },
] as const;

function TypeScale() {
  return (
    <div className="flex flex-col gap-6">
      {(["text-3xl", "text-2xl", "text-xl", "text-lg", "text-base", "text-sm", "text-xs"] as const).map((size) => (
        <section key={size} className="flex flex-col gap-1">
          <code className="font-mono text-xs text-fg-muted">{size}</code>
          {typeSamples.map(({ lang, dir, text }) => (
            <p key={lang} lang={lang} dir={dir} className={size}>
              {text}
            </p>
          ))}
        </section>
      ))}
    </div>
  );
}

export const Typography: Story = { render: () => <TypeScale /> };

function Shapes() {
  return (
    <div className="flex flex-col gap-6">
      <ul className="flex flex-wrap gap-4">
        {(
          [
            ["rounded-sm shadow-sm", "flex size-24 items-end rounded-sm bg-surface-raised p-2 shadow-sm"],
            ["rounded-md shadow-md", "flex size-24 items-end rounded-md bg-surface-raised p-2 shadow-md"],
            ["rounded-lg shadow-lg", "flex size-24 items-end rounded-lg bg-surface-raised p-2 shadow-lg"],
            ["rounded-xl shadow-card", "flex size-24 items-end rounded-xl bg-surface-raised p-2 shadow-card"],
            ["rounded-xl shadow-lg", "flex size-24 items-end rounded-xl bg-surface-raised p-2 shadow-lg"],
            ["rounded-2xl shadow-lg", "flex size-24 items-end rounded-2xl bg-surface-raised p-2 shadow-lg"],
            ["rounded-avatar", "flex size-24 items-end rounded-avatar bg-soft p-2"],
          ] as const
        ).map(([name, classes]) => (
          <li key={name} className={classes}>
            <code className="font-mono text-xs">{name}</code>
          </li>
        ))}
      </ul>
      <div className="flex flex-wrap gap-4">
        {surfaces.map((name) => (
          <div
            key={name}
            className="focus-ring rounded-md border border-border-strong p-3"
            style={{ backgroundColor: `var(--color-${name})` }}
          >
            <code className="font-mono text-sm">focus-ring on {name}</code>
          </div>
        ))}
      </div>
    </div>
  );
}

export const ShapesAndFocus: Story = { name: "Radius, shadow and focus", render: () => <Shapes /> };
