// SPDX-License-Identifier: AGPL-3.0-only

// The media viewer over a page: a story per state: a wide picture with its description, 2 of 3; a tall one; a long
// description; one as long as an attachment's may be, which scrolls in its own area; a picture without one; a video; a
// GIF; one picture alone, with nothing to move to; and a description in ja.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { MediaItem } from "./media";
import { MediaViewer } from "./viewer";
import { night, tall, wide } from "./viewer.samples";

const items: MediaItem[] = [
  { id: "1", src: tall, alt: "A maple tree in autumn, its leaves orange and red against a warm sky" },
  { id: "2", src: wide, alt: "A sea under a pale blue sky, with low hills on the horizon and a pale sun above them" },
  { id: "3", src: night, alt: "" },
];
const long =
  "A crowded harbour at low tide, seen from the hill above the town. Fishing boats lie on their sides in the mud, " +
  "their ropes slack; gulls stand in rows on the sea wall. On the far side, the old lighthouse is being painted, " +
  "half white and half its old red, with a scaffold up one side.\n\nIn the foreground, a bench with a thermos and two " +
  "cups, and a dog asleep under it.";
const video: MediaItem = { id: "v", src: night, alt: "Waves at dusk, ninety seconds", kind: "video", duration: 92 };
const gif: MediaItem = { id: "g", src: wide, alt: "Gulls circling over the pier", kind: "gifv" };

/** A description as long as an attachment's may be: the long one, repeated up to 1,500 characters. */
const veryLong = `${long}\n\n${long}\n\n${long}`.slice(0, 1500);

/** The viewer open at `at` among `media`, as a post holds it open. */
function Open({ media, at = 0, locale }: { media: MediaItem[]; at?: number; locale?: string }) {
  const [index, setIndex] = useState(at);
  return <MediaViewer items={media} index={index} onIndexChange={setIndex} onClose={() => undefined} locale={locale} />;
}

const meta = {
  title: "Media viewer",
  tags: ["themes"],
  parameters: { layout: "fullscreen", controls: { disable: true } },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** A wide picture with its description, the second of three. */
export const Default: Story = { render: () => <Open media={items} at={1} /> };
/** A tall picture, the first: nothing before it. */
export const Tall: Story = { render: () => <Open media={items} at={0} /> };
/** A description that runs long. */
export const LongDescription: Story = {
  render: () => <Open media={[{ id: "l", src: wide, alt: long }, ...items]} />,
};
/** A picture without a description, the last: nothing after it. */
export const NoDescription: Story = { render: () => <Open media={items} at={2} /> };
/** A video, with the browser's controls. */
export const Video: Story = { render: () => <Open media={[video, ...items]} /> };
/** A GIF, looping. */
export const Gif: Story = { render: () => <Open media={[gif]} /> };
/** One picture alone: no position, nothing to move to. */
export const Alone: Story = { render: () => <Open media={[items[1] ?? video]} /> };
/** A description in ja. */
export const Japanese: Story = {
  render: () => (
    <Open
      media={[
        { id: "j", src: wide, alt: "淡い青空の下の海。水平線に低い丘が連なり、その上に白い太陽が出ている" },
        ...items,
      ]}
      locale="ja"
    />
  ),
};
/**
 * A description of the 1,500 characters an attachment may hold: it scrolls in its own area under the picture, which
 * keeps the rest of the screen, at 320 px too.
 */
export const VeryLongDescription: Story = {
  render: () => <Open media={[{ id: "v", src: tall, alt: veryLong }, ...items]} />,
};
