// SPDX-License-Identifier: AGPL-3.0-only

// An event's description (ADR-0161 §5, FR-951): plain text of several lines with two URLs, each a link that says it
// leaves the site, one long enough to break; and text alone. The texts are samples in English.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { EventDescription } from "./event-description";

const rem = (px: number) => `${px / rootPx}rem`;

/** The column of a screen: as wide as the app's main column at most, the frame's edge around it. */
function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
}

const meta = { title: "Event description", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** Lines as written, with a URL in the middle of one and a long one on a line of its own. */
export const WithLinks: Story = {
  render: () => (
    <Column>
      <EventDescription
        parts={[
          { text: "Up the north ridge for the morning light. The route is on " },
          {
            link: {
              href: "https://maps.example/north-ridge",
              text: "https://maps.example/north-ridge",
              host: "maps.example",
            },
          },
          { text: ".\nPhotos from last time:\n" },
          {
            link: {
              href: "https://photos.example/albums/2026/the-north-ridge-in-the-morning-light-and-the-long-way-down",
              text: "https://photos.example/albums/2026/the-north-ridge-in-the-morning-light-and-the-long-way-down",
              host: "photos.example",
            },
          },
        ]}
      />
    </Column>
  ),
};

/** Text alone. */
export const Plain: Story = {
  render: () => (
    <Column>
      <EventDescription parts={[{ text: "Bring a jar of starter if you have one.\nWe bake two loaves each." }]} />
    </Column>
  ),
};
