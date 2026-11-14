// SPDX-License-Identifier: AGPL-3.0-only

// The disclosure, as a post's content warning: Default, driven by its props in the Controls, and a story per state:
// folded, shown, its button hovered and focused from the keyboard (shown at rest), a long warning, one with no glyph,
// and one in ja.

import { text } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Disclosure } from "./disclosure";
import { Preview, type PreviewState } from "./preview";

type Lang = "en" | "ja";
type Args = { summary: string; glyph: boolean; defaultExpanded: boolean; shown: "none" | PreviewState; lang: Lang };

const sample = {
  en: {
    warning: "Spoilers for the season finale",
    long: "Spoilers for the season finale, and a long talk about the ending that runs past one line on a phone",
    body: "The ending was not what anyone expected, and the last scene changes how the first episode reads.",
  },
  ja: {
    warning: "最終回のネタバレ",
    long: "最終回のネタバレと、結末についての長い話を含みます。モバイルでは 1 行に収まりません",
    body: "誰も予想しなかった結末で、最後の場面を見ると 1 話の見え方が変わります。",
  },
} as const;

// Tagged `themes`: the story tests run it in dark too.
const meta = {
  title: "Disclosure",
  tags: ["themes"],
  args: { summary: sample.en.warning, glyph: true, defaultExpanded: false, shown: "none", lang: "en" },
  argTypes: {
    summary: { control: "text", description: "The warning, which shows while the content is folded" },
    glyph: { control: "boolean", description: "The content warning's glyph before the summary" },
    defaultExpanded: { control: "boolean", description: "Shown at first" },
    shown: {
      control: "select",
      options: ["none", "hover", "pressed", "focus-visible"],
      description: "A state of its button shown at rest",
    },
    lang: { control: "select", options: ["en", "ja"], description: "The language of the content and its button" },
  },
  render: ({ summary, glyph, defaultExpanded, shown, lang }) => (
    <div lang={lang}>
      <Preview state={shown === "none" ? undefined : shown}>
        <Disclosure
          key={String(defaultExpanded)}
          summary={summary}
          icon={glyph ? "hidden" : undefined}
          defaultExpanded={defaultExpanded}
          locale={lang}
        >
          <p style={{ fontSize: `${text.body.size}rem`, lineHeight: text.body.lineHeight, margin: 0 }}>
            {sample[lang].body}
          </p>
        </Disclosure>
      </Preview>
    </div>
  ),
} satisfies Meta<Args>;
export default meta;

type Story = StoryObj<typeof meta>;

/** Folded, as a post shows it at first. */
export const Default: Story = {};
export const Shown: Story = { args: { defaultExpanded: true } };
export const Hovered: Story = { args: { shown: "hover" } };
export const FocusVisible: Story = { args: { shown: "focus-visible" } };
export const LongWarning: Story = { args: { summary: sample.en.long } };
export const NoGlyph: Story = { args: { glyph: false } };
export const Japanese: Story = { args: { summary: sample.ja.warning, lang: "ja" } };
