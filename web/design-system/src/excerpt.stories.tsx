// SPDX-License-Identifier: AGPL-3.0-only

// The first words of something a line is about: on two lines by default, on one or three; muted, or in the text's own
// colour; and a post's body whose blocks all run on. The words are samples in English.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { ContentBody } from "./content";
import { Excerpt } from "./list";

const rem = (px: number) => `${px / rootPx}rem`;

const text =
  "I uploaded the photos from the ridge to the shared album. The light on the third one is unreal, have a look when you get a minute and tell me which ones you want printed.";

function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(360), padding: rem(16) }}>{children}</div>;
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Excerpt", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** Two lines, muted: the default. */
export const Default: Story = {
  render: () => (
    <Column>
      <Excerpt>{text}</Excerpt>
    </Column>
  ),
};

/** One line, then an ellipsis. */
export const OneLine: Story = {
  render: () => (
    <Column>
      <Excerpt lines={1}>{text}</Excerpt>
    </Column>
  ),
};

/** Three lines, in the text's own colour. */
export const Text: Story = {
  render: () => (
    <Column>
      <Excerpt lines={3} tone="text">
        {text}
      </Excerpt>
    </Column>
  ),
};

/** A post's body: its paragraphs run on as one text, in the excerpt's tone. */
export const Body: Story = {
  render: () => (
    <Column>
      <Excerpt body>
        <ContentBody>
          <p>I uploaded the photos from the ridge.</p>
          <p>The light on the third one is unreal, have a look when you get a minute.</p>
        </ContentBody>
      </Excerpt>
    </Column>
  ),
};
