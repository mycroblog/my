// SPDX-License-Identifier: AGPL-3.0-only

// The avatar: Default, driven by its props in the Controls, a story per kind of image (a photo, a bright photo whose
// edge meets the surface, a logo on transparency, none), and Sizes, every kind at the preview, post and inline sizes
// and the inline size beside a name. The images are generated ones (samples.tsx).

import { rootPx, text } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties, ReactNode } from "react";
import { Avatar, type AvatarSize } from "./avatar";
import { images } from "./samples";

const rem = (px: number) => `${px / rootPx}rem`;

type Image = keyof typeof images | "none";
type Args = { name: string; image: Image; size: AvatarSize; decorative: boolean };

const caption: CSSProperties = {
  color: "var(--color-muted-text)",
  fontSize: `${text.caption.size}rem`,
  lineHeight: text.caption.lineHeight,
  margin: 0,
};

const sizes: AvatarSize[] = ["preview", "post", "inline"];
const srcOf = (image: Image) => (image === "none" ? undefined : images[image]);

const samples: { label: string; name: string; image: Image }[] = [
  { label: "Photo", name: "Alice Chen", image: "dusk" },
  { label: "Bright photo", name: "佐藤 美香", image: "snow" },
  { label: "Logo, transparent", name: "Open Source Weekly", image: "logo" },
  { label: "No image", name: "Ravi", image: "none" },
  { label: "No image, ja", name: "たくや", image: "none" },
];

const Row = ({ label, children }: { label: string; children: ReactNode }) => (
  <div
    style={{ alignItems: "center", display: "grid", gap: rem(12), gridTemplateColumns: `${rem(96)} minmax(0, 1fr)` }}
  >
    <span style={caption}>{label}</span>
    <div style={{ alignItems: "center", display: "flex", gap: rem(16) }}>{children}</div>
  </div>
);

/** Every kind of image at the preview (64 px), post (40 px) and inline (20 px) sizes, then inline beside names. */
function SizeSheet() {
  return (
    <div style={{ display: "grid", gap: rem(12), gridTemplateColumns: "minmax(0, 1fr)" }}>
      {samples.map((s) => (
        <Row key={s.label} label={s.label}>
          {sizes.map((size) => (
            <Avatar key={size} name={s.name} src={srcOf(s.image)} size={size} />
          ))}
        </Row>
      ))}
      <Row label="Inline, by a name">
        <span style={{ ...caption, alignItems: "center", display: "flex", flexWrap: "wrap", gap: rem(6) }}>
          Replying to
          <Avatar name="Alice Chen" src={images.dusk} size="inline" decorative />
          <span style={{ color: "var(--color-text)" }}>Alice Chen</span>
          <Avatar name="たくや" size="inline" decorative />
          <span style={{ color: "var(--color-text)" }}>たくや</span>
        </span>
      </Row>
    </div>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = {
  title: "Avatar",
  tags: ["themes"],
  args: { name: "Alice Chen", image: "dusk", size: "preview", decorative: false },
  argTypes: {
    name: { control: "text", description: "Whose avatar: its name, and its initial when there is no image" },
    image: { control: "select", options: [...Object.keys(images), "none"] },
    size: { control: "select", options: sizes },
    decorative: { control: "boolean", description: "Beside the name it shows, so assistive technology skips it" },
  },
  render: ({ name, image, size, decorative }) => (
    <Avatar name={name} src={srcOf(image)} size={size} decorative={decorative} />
  ),
} satisfies Meta<Args>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Photo: Story = { args: { image: "dusk" } };
export const BrightPhoto: Story = { args: { name: "佐藤 美香", image: "snow" } };
export const Logo: Story = { args: { name: "Open Source Weekly", image: "logo" } };
export const NoImage: Story = { args: { name: "Ravi", image: "none" } };
export const NoImageJapanese: Story = { args: { name: "たくや", image: "none" } };
export const Sizes: Story = { render: () => <SizeSheet /> };
