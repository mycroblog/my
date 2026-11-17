// SPDX-License-Identifier: AGPL-3.0-only

// A row of a list that is pressed as a whole: its parts one under another or side by side, at rest, with the text
// of the caption muted, and with a text that wraps. The words are samples in English.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { Icon } from "./icons";
import { PressRow } from "./list";

const rem = (px: number) => `${px / rootPx}rem`;
const none = () => undefined;

function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Press row", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** Its parts one under another, the surface reaching past the column at its start. */
export const Stacked: Story = {
  render: () => (
    <Column>
      <PressRow bleed="start" gap="entry-line" onPress={none}>
        <span>The first part, which wraps when it is longer than the line the row has to show it on.</span>
        <span>The second part.</span>
      </PressRow>
    </Column>
  ),
};

/** Its parts side by side, a glyph and a caption's text, muted, the surface reaching past the column at both ends. */
export const Aligned: Story = {
  render: () => (
    <Column>
      <PressRow flow="row" bleed="inline" gap="entry-caption" text="caption" tone="muted" onPress={none}>
        <Icon name="note" place="caption" />
        <span>A line that opens what it names, with a text long enough to wrap onto a second line of the row.</span>
      </PressRow>
    </Column>
  ),
};
