// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import { Avatar } from "./Avatar";

/** A stand-in profile picture, inline so the story does not depend on the network. */
const photo = `data:image/svg+xml,${encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" fill="#3b6fd8"/><circle cx="32" cy="26" r="12" fill="#f4d7b5"/><path d="M10 64c0-14 10-22 22-22s22 8 22 22z" fill="#1e3a75"/></svg>',
)}`;
/** Bytes no browser can decode, so the image fails to load at once. */
const broken = "data:image/png;base64,AAAA";

const meta = {
  title: "Display/Avatar",
  component: Avatar,
  args: { name: "Alice Example", decorative: false, src: photo },
} satisfies Meta<typeof Avatar>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Waits until every image in the story has decoded. */
async function imagesLoaded(root: HTMLElement) {
  await waitFor(() => {
    for (const img of root.querySelectorAll("img")) {
      if (!img.complete || img.naturalWidth === 0) throw new Error(`${img.alt || "an image"} has not loaded`);
    }
  });
}

/** A standalone avatar is an image named after the account. */
export const Standalone: Story = {
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.getByRole("img", { name: "Alice Example" })).toBeVisible();
    await imagesLoaded(canvasElement);
  },
};

/** Next to the visible name, the avatar is decorative (`alt=""`), so the name is read once. */
export const NextToName: Story = {
  args: { decorative: true },
  render: (args) => (
    <p className="flex items-center gap-2">
      <Avatar {...args} />
      <span dir="auto" className="font-semibold">
        {args.name}
      </span>
    </p>
  ),
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.queryByRole("img")).toBeNull();
    await expect(canvasElement.querySelector("img")).toHaveAttribute("alt", "");
    await imagesLoaded(canvasElement);
  },
};

/** When the image fails to load, the first letter of the name takes its place and keeps the name. */
export const LoadFailure: Story = {
  args: { name: "Bob Example", src: broken },
  play: async ({ canvas, canvasElement }) => {
    await waitFor(() => {
      if (canvasElement.querySelector("img") !== null) throw new Error("the broken image is still shown");
    });
    await expect(canvas.getByRole("img", { name: "Bob Example" })).toHaveTextContent("B");
  },
};

/** Without an image, the first letter shows; one grapheme in any script. */
export const Initials: Story = {
  render: () => (
    <ul className="flex flex-wrap items-center gap-3">
      {["Alice Example", "山田 花子", "ليلى", "émile", "👩‍🚀 Astro"].map((name) => (
        <li key={name}>
          <Avatar name={name} decorative={false} />
        </li>
      ))}
    </ul>
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("img", { name: "山田 花子" })).toHaveTextContent("山");
    await expect(canvas.getByRole("img", { name: "émile" })).toHaveTextContent("É");
    await expect(canvas.getByRole("img", { name: "👩‍🚀 Astro" })).toHaveTextContent("👩‍🚀");
  },
};

/** A blank display name falls back to a silhouette; the handle beside it carries the meaning. */
export const Placeholder: Story = {
  args: { name: " ", decorative: true, src: undefined },
  render: (args) => (
    <p className="flex items-center gap-2">
      <Avatar {...args} />
      <span dir="auto">@anonymous</span>
    </p>
  ),
  play: async ({ canvasElement }) => {
    await expect(canvasElement.querySelector("svg")).toBeInTheDocument();
  },
};

export const Sizes: Story = {
  render: (args) => (
    <ul className="flex flex-wrap items-end gap-3">
      {(["xs", "sm", "md", "lg", "xl"] as const).map((size) => (
        <li key={size} className="flex flex-col items-center gap-2">
          <Avatar {...args} size={size} />
          <Avatar name={args.name} decorative={false} size={size} />
          <code className="font-mono text-xs text-fg-muted">{size}</code>
        </li>
      ))}
    </ul>
  ),
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.getAllByRole("img", { name: "Alice Example" })).toHaveLength(10);
    await imagesLoaded(canvasElement);
  },
};

/** Without an image the initial sits on one of five pastel tints, picked from the seed: the same seed is always the same color. */
export const Tints: Story = {
  render: () => (
    <ul className="flex flex-wrap items-center gap-3">
      {["1", "2", "3", "4", "5", "6", "7", "8"].map((seed) => (
        <li key={seed}>
          <Avatar name="Alice Example" seed={seed} decorative={false} />
        </li>
      ))}
    </ul>
  ),
  play: async ({ canvas }) => {
    const avatars = canvas.getAllByRole("img", { name: "Alice Example" });
    const tintOf = (el: HTMLElement) => [...el.classList].find((c) => c.startsWith("bg-avatar-"));
    await expect(new Set(avatars.map(tintOf)).size).toBeGreaterThan(1);
    await expect(tintOf(avatars[0] as HTMLElement)).toBeDefined();
  },
};
