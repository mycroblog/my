// SPDX-License-Identifier: AGPL-3.0-only

// The media viewer over a page: a story per state: a wide picture with its description, 2 of 3; a tall one; a long
// description; one as long as an attachment's may be, which scrolls in its own area; a picture without one; a video; a
// GIF; one picture alone, with nothing to move to; and a description in ja.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { MediaItem } from "./media";
import { modalShown } from "./preview";
import { MediaViewer } from "./viewer";

/* oxlint-disable restrict/token-colors -- a sample picture is drawn in its own colours */
const svg = (w: number, h: number, body: string) =>
  `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}" width="${w * 8}" height="${h * 8}">${body}</svg>`)}`;

const wide = svg(
  160,
  100,
  `<defs><linearGradient id="s" x2="0" y2="1"><stop offset="0" stop-color="#7fb3d5"/><stop offset="1" stop-color="#d6ecf5"/></linearGradient></defs><rect width="160" height="100" fill="url(#s)"/><circle cx="112" cy="40" r="12" fill="#fffbe8"/><path d="M0 64 L36 50 L70 60 L104 44 L160 58 V100 H0Z" fill="#3f6f86"/><path d="M0 82 Q60 74 160 80 V100 H0Z" fill="#2a4d5f"/>`,
);
const tall = svg(
  90,
  160,
  `<defs><linearGradient id="s" x2="0" y2="1"><stop offset="0" stop-color="#f2b56b"/><stop offset="1" stop-color="#f9dfb0"/></linearGradient></defs><rect width="90" height="160" fill="url(#s)"/><rect x="38" y="40" width="14" height="120" fill="#5e2d22"/><circle cx="45" cy="40" r="30" fill="#c2683f"/><circle cx="30" cy="56" r="18" fill="#94462f"/>`,
);
const night = svg(
  160,
  100,
  `<rect width="160" height="100" fill="#1d2440"/><circle cx="40" cy="30" r="10" fill="#f3efd6"/><path d="M0 70 Q50 56 100 68 T160 64 V100 H0Z" fill="#2a2b45"/>`,
);
/* oxlint-enable restrict/token-colors */

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
  // Shown at rest: the checks read it once it has come in, not halfway through its entrance.
  play: async () => {
    await modalShown();
  },
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
