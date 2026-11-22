// SPDX-License-Identifier: AGPL-3.0-only

// The media of an entry: Default, driven by the Controls (how many pictures, their kinds, and sensitive), and a story
// per state: one to four images, a video and a GIF with their marks, a sound and a file, the previews of a quoted post,
// and sensitive media of each kind under its line, in a column of a post's width; and how a picture and a covered one
// look at rest, hovered and pressed.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { MediaGrid, type MediaItem } from "./media";
import { pictures } from "./media.samples";
import { Preview, type PreviewState } from "./preview";

const rem = (px: number) => `${px / rootPx}rem`;

/** Each picture's blurhash, as the server would give it: its sky, its light and its ridges, top to bottom. */
const blurhashes = {
  dawn: "IuPrad~U8_",
  sea: "IxHWWH%h01",
  forest: "I#K2tg.80K",
  night: "IE68O8WBDz",
};

const image = (id: string, picture: keyof typeof pictures, alt: string): MediaItem => ({
  id,
  src: pictures[picture],
  alt,
  blurhash: blurhashes[picture],
});

const samples: MediaItem[] = [
  image("a", "dawn", "Low hills at dawn under a peach sky"),
  image("b", "sea", ""),
  image("c", "forest", "Green hills under a pale sky"),
  image("d", "night", "A night sky over dark hills"),
];
const video: MediaItem = {
  id: "v",
  src: pictures.night,
  alt: "Waves at dusk",
  kind: "video",
  duration: 92,
  blurhash: blurhashes.night,
};
const gif: MediaItem = { id: "g", src: pictures.forest, alt: "", kind: "gifv", blurhash: blurhashes.forest };
const audio: MediaItem = {
  id: "s",
  src: "",
  alt: "Field recording: steady rain on a tin roof, with distant thunder",
  kind: "audio",
  duration: 194,
};
const file: MediaItem = { id: "f", src: "", alt: "", kind: "unknown", href: "https://files.example.org/slides.key" };

/** A column of a post's width. */
const Column = ({ children }: { children: ReactNode }) => (
  <div style={{ maxWidth: rem(568), display: "grid", gap: rem(28) }}>{children}</div>
);

type Args = { count: number; kinds: "images" | "video" | "gif"; sensitive: boolean };

const itemsOf = ({ count, kinds }: Args) => {
  const first = kinds === "video" ? video : kinds === "gif" ? gif : samples[0];
  return [first, ...samples.slice(1)].filter((m) => m !== undefined).slice(0, count);
};

// Tagged `themes`: the check of every story shows it in dark too.
const meta = {
  title: "Media",
  tags: ["themes"],
  args: { count: 1, kinds: "images", sensitive: false },
  argTypes: {
    count: { control: { type: "range", min: 1, max: 4 }, description: "How many pictures" },
    kinds: { control: "select", options: ["images", "video", "gif"], description: "What the first picture is" },
    sensitive: { control: "boolean", description: "Covered until shown" },
  },
  render: (args) => (
    <Column>
      <MediaGrid key={JSON.stringify(args)} items={itemsOf(args)} sensitive={args.sensitive} onOpen={() => undefined} />
    </Column>
  ),
} satisfies Meta<Args>;
export default meta;

type Story = StoryObj<typeof meta>;

const fixed = { parameters: { controls: { disable: true } } };
const open = () => undefined;

export const Default: Story = {};
/** One, two, three and four images. */
export const Images: Story = {
  ...fixed,
  render: () => (
    <Column>
      {[1, 2, 3, 4].map((n) => (
        <MediaGrid key={n} items={samples.slice(0, n)} onOpen={open} />
      ))}
    </Column>
  ),
};
/** A video with its play disc, its length and its way to open whole; a GIF with its mark; and both beside an image. */
export const VideoAndGif: Story = {
  ...fixed,
  render: () => (
    <Column>
      <MediaGrid items={[video]} onOpen={open} />
      <MediaGrid items={[gif]} onOpen={open} />
      <MediaGrid items={[samples[0] ?? video, gif, video]} onOpen={open} />
    </Column>
  ),
};
/** A sound and a file of a kind the client cannot show. */
export const AudioAndFile: Story = {
  ...fixed,
  render: () => (
    <Column>
      <MediaGrid items={[audio, file]} />
    </Column>
  ),
};
/** A quoted post's media: small tiles, sensitive ones blurred. */
export const Previews: Story = {
  ...fixed,
  render: () => (
    <Column>
      <MediaGrid items={[...samples.slice(0, 2), audio]} size="preview" />
      <MediaGrid items={samples.slice(0, 2)} size="preview" sensitive />
    </Column>
  ),
};
/**
 * Sensitive media under its line, named by kind and count: one image, two, a video, a sound (folded), and several
 * kinds.
 */
export const Sensitive: Story = {
  ...fixed,
  render: () => (
    <Column>
      <MediaGrid items={samples.slice(0, 1)} sensitive onOpen={open} />
      <MediaGrid items={samples.slice(0, 2)} sensitive onOpen={open} />
      <MediaGrid items={[video]} sensitive onOpen={open} />
      <MediaGrid items={[audio]} sensitive />
      <MediaGrid items={[samples[0] ?? video, gif, audio]} sensitive onOpen={open} />
    </Column>
  ),
};

const pictureSteps = [
  { label: "At rest", state: undefined },
  { label: "Hovered", state: "hover" },
  { label: "Pressed", state: "pressed" },
] as const satisfies readonly { label: string; state: PreviewState | undefined }[];

/**
 * Pictures at rest, hovered and pressed, a column each, three across where they fit: an image that opens, and apart
 * from it, sensitive media under its line.
 */
function PictureSteps() {
  return (
    <div style={{ display: "grid", gap: rem(28), gridTemplateColumns: `repeat(auto-fit, minmax(${rem(240)}, 1fr))` }}>
      {pictureSteps.map(({ label, state }) => (
        <Preview key={label} state={state}>
          <div style={{ display: "grid", gap: rem(28), alignContent: "start" }}>
            <div style={{ display: "grid", gap: rem(4) }}>
              <p className="m-0 text-caption font-label text-text">{label}</p>
              <MediaGrid items={samples.slice(0, 1)} onOpen={open} />
            </div>
            <MediaGrid items={samples.slice(2, 3)} sensitive onOpen={open} />
          </div>
        </Preview>
      ))}
    </div>
  );
}

/** How a picture answers a hover and a press: the press tokens' shade over it, 10% and 20%; it does not move. */
export const PicturePress: Story = { ...fixed, render: () => <PictureSteps /> };
