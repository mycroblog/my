// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, waitFor, within } from "storybook/test";
import { MediaGrid, type MediaItem } from "./MediaGrid";

/** A placeholder picture as an SVG data URL, so stories load no network images. */
function picture(fill: string, shape: string): string {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 160 90"><rect width="160" height="90" fill="${fill}"/>${shape}</svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
}

/** The first three pictures have a blurhash; the fourth has none, and its plain cell stands in for the blur. */
const pictures: readonly MediaItem[] = [
  {
    id: "1",
    src: picture("#4f6d8f", '<circle cx="80" cy="45" r="24" fill="#f2c14e"/>'),
    alt: "A yellow sun over a blue sky",
    lang: "en",
    blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
  },
  {
    id: "2",
    src: picture("#3d7a52", '<path d="M0 90 60 30l40 40 20-20 40 40z" fill="#e8efe9"/>'),
    alt: "Snowy hills in a green valley",
    lang: "en",
    blurhash: "LGF5]+Yk^6#M@-5c,1J5@[or[Q6.",
  },
  {
    id: "3",
    src: picture("#7a3d6b", '<rect x="50" y="20" width="60" height="50" fill="#f4d6ea"/>'),
    alt: "A pink square on purple",
    lang: "en",
    blurhash: "L6PZfSi_.AyE_3t7t7R**0o#DgR4",
  },
  {
    id: "4",
    src: picture(
      "#8f5a2e",
      '<circle cx="50" cy="45" r="15" fill="#fbe3c8"/><circle cx="110" cy="45" r="15" fill="#fbe3c8"/>',
    ),
    alt: null,
  },
];

const meta = {
  title: "Timeline/MediaGrid",
  component: MediaGrid,
  args: {
    items: pictures.slice(0, 1),
    noDescriptionLabel: "No description",
    revealLabel: "Show sensitive media",
    audioLabels: { play: "Play", pause: "Pause", seek: "Position" },
    onRevealedChange: fn(),
  },
} satisfies Meta<typeof MediaGrid>;

export default meta;
type Story = StoryObj<typeof meta>;

export const One: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("img", { name: "A yellow sun over a blue sky" })).toBeVisible();
  },
};

export const Two: Story = { args: { items: pictures.slice(0, 2) } };

export const Three: Story = { args: { items: pictures.slice(0, 3) } };

/** The first three pictures have a description and carry the note badge; the fourth has none: it is named "No description" and carries no badge. */
export const Four: Story = {
  args: { items: pictures },
  play: async ({ canvas }) => {
    await expect(canvas.getAllByRole("img")).toHaveLength(4);
    await expect(canvas.getByRole("img", { name: "No description" })).toBeVisible();
  },
};

export const NoDescription: Story = { args: { items: pictures.slice(3) } };

/** A remote post with six attachments, of which the server keeps four: the mark in the corner links to the original post. */
export const More: Story = {
  args: {
    items: pictures,
    more: { text: "+2", label: "+2 more — see the original post", href: "https://remote.example/@carol/1" },
  },
  play: async ({ canvas }) => {
    const link = canvas.getByRole("link", { name: "+2 more — see the original post" });
    await expect(link).toHaveTextContent("+2");
    await expect(link).toHaveAttribute("href", "https://remote.example/@carol/1");
    await expect(link).toHaveAttribute("target", "_blank");
  },
};

/**
 * Sensitive media are not loaded until the reader presses the toggle, which keeps focus. Until then each cell shows
 * the blur of its picture (its blurhash).
 */
export const Sensitive: Story = {
  args: { items: pictures.slice(0, 2), sensitive: true },
  play: async ({ canvas, canvasElement, userEvent }) => {
    await expect(canvas.queryByRole("img")).not.toBeInTheDocument();
    await expect(canvasElement.querySelectorAll("canvas")).toHaveLength(2);
    const toggle = canvas.getByRole("button", { name: "Show sensitive media" });
    await expect(toggle).toHaveAttribute("aria-pressed", "false");
    await userEvent.tab();
    await userEvent.keyboard("{Enter}");
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
    await expect(toggle).toHaveFocus();
    await expect(canvas.getAllByRole("img")).toHaveLength(2);
    // Back to concealed, so the screenshots show the concealed state.
    await userEvent.keyboard("{Enter}");
    await expect(canvas.queryByRole("img")).not.toBeInTheDocument();
  },
};

/** A picture without a blurhash (the fourth) keeps the plain surface of its cell while concealed. */
export const SensitiveWithoutBlurhash: Story = {
  args: { items: pictures.slice(2, 4), sensitive: true },
  play: async ({ canvasElement }) => {
    await expect(canvasElement.querySelectorAll("canvas")).toHaveLength(1);
  },
};

export const SensitiveRevealed: Story = {
  args: { items: pictures.slice(0, 3), sensitive: true, defaultRevealed: true },
};

/**
 * Pressing a picture opens it at its original size in the viewer (a modal dialog), whose buttons and arrow keys step
 * through the post's pictures, going round at either end. Esc closes it and returns focus to the picture; the story
 * opens it again on the second picture for the screenshots.
 */
export const Viewer: Story = {
  args: {
    items: pictures.slice(0, 2).map((item) => ({
      ...item,
      original: { sources: { still: [{ url: item.src ?? "" }] }, width: 1600, height: 900 },
    })),
    viewerLabels: {
      title: (position, count) => `Image ${String(position)} of ${String(count)}`,
      close: "Close",
      position: (position: number, count: number) => `${String(position)} / ${String(count)}`,
      altText: "Description",
      previous: "Previous image",
      next: "Next image",
    },
  },
  // The viewer covers the story frame, so give it room.
  decorators: [
    (Story) => (
      <div className="min-h-136">
        <Story />
      </div>
    ),
  ],
  play: async ({ canvas, userEvent }) => {
    const first = canvas.getByRole("button", { name: "A yellow sun over a blue sky" });
    await userEvent.click(first);
    const dialog = await canvas.findByRole("dialog", { name: "Image 1 of 2" });
    // Visible once the panel has faded in.
    await waitFor(() =>
      expect(within(dialog).getByRole("img", { name: "A yellow sun over a blue sky" })).toBeVisible(),
    );
    // The arrow keys follow the screen: the next picture is to the right in left-to-right text, to the left otherwise.
    const forward = document.documentElement.dir === "rtl" ? "{ArrowLeft}" : "{ArrowRight}";
    await userEvent.keyboard(forward);
    await expect(canvas.getByRole("dialog", { name: "Image 2 of 2" })).toBeInTheDocument();
    await userEvent.keyboard(forward);
    await expect(canvas.getByRole("dialog", { name: "Image 1 of 2" })).toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole("button", { name: "Previous image" }));
    await expect(canvas.getByRole("dialog", { name: "Image 2 of 2" })).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(canvas.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(first).toHaveFocus());

    await userEvent.click(canvas.getByRole("button", { name: "Snowy hills in a green valley" }));
    await expect(await canvas.findByRole("dialog", { name: "Image 2 of 2" })).toBeInTheDocument();
  },
};
/** A one-second VP8 video of one blue frame and a one-second silent Opus file, so stories load no network media. */
const videoFile =
  "data:video/webm;base64,GkXfo59ChoEBQveBAULygQRC84EIQoKEd2VibUKHgQJChYECGFOAZwEAAAAAAAIpEU2bdLpNu4tTq4QVSalmU6yBoU27i1OrhBZUrmtTrIHYTbuMU6uEElTDZ1OsggEfTbuMU6uEHFO7a1OsggIT7AEAAAAAAABZAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAVSalmsirXsYMPQkBNgI1MYXZmNTguNDUuMTAwV0GNTGF2ZjU4LjQ1LjEwMESJiECPQAAAAAAAFlSua8KuAQAAAAAAADnXgQFzxYiPgJbYbBAl7pyBACK1nIN1bmSGhVZfVlA4g4EBI+ODhDuaygDgAQAAAAAAAAawgaC6gVoSVMNnQJlzcwEAAAAAAAAnY8CAZ8gBAAAAAAAAGkWjh0VOQ09ERVJEh41MYXZmNTguNDUuMTAwc3MBAAAAAAAAXmPAi2PFiI+AlthsECXuZ8gBAAAAAAAAIUWjh0VOQ09ERVJEh5RMYXZjNTguOTEuMTAwIGxpYnZweGfIokWjiERVUkFUSU9ORIeUMDA6MDA6MDEuMDAwMDAwMDAwAAAfQ7Z10OeBAKPLgQAAgLAFAJ0BKqAAWgABhwiFhYiZhIgIAgJ1qgIGeZak569GXiOevRl4jnr0ZeI569GXiOevRl4jlAD+7gpnySPVzHQfit9QM0gAHFO7a5G7j7OBALeK94EB8YIBvvCBAw==";
const audioFile =
  "data:audio/ogg;base64,T2dnUwACAAAAAAAAAABf1EMQAAAAAL7MFqUBE09wdXNIZWFkAQE4AYC7AAAAAABPZ2dTAAAAAAAAAAAAAF/UQxABAAAAIiMbKgE+T3B1c1RhZ3MNAAAATGF2ZjU4LjQ1LjEwMAEAAAAdAAAAZW5jb2Rlcj1MYXZjNTguOTEuMTAwIGxpYm9wdXNPZ2dTAACAuwAAAAAAAF/UQxACAAAAWsUo5TIHBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBggL5FMVS/IIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsZPZ2dTAAS4vAAAAAAAAF/UQxADAAAAEH17vgEGCAbgsw7G";

/** A video, an audio file with a poster, an audio file without one, and a short silent video that loops like a GIF. */
const playables: readonly MediaItem[] = [
  {
    id: "v",
    src: pictures[0]?.src,
    playback: { kind: "video", url: videoFile },
    alt: "A sunny sky, filmed for a second",
    lang: "en",
    blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
  },
  {
    id: "a",
    src: pictures[1]?.src,
    playback: { kind: "audio", url: audioFile },
    alt: "A second of silence",
    lang: "en",
  },
  { id: "n", src: undefined, playback: { kind: "audio", url: audioFile }, alt: null },
  { id: "g", src: pictures[2]?.src, playback: { kind: "gifv", url: videoFile }, alt: "A pink square" },
];

/**
 * A video plays in its cell with the browser's controls under a play disc, an audio file in an AudioPlayer, and neither starts by itself; each player
 * is named by its description. These stories take no screenshots: the browser's media controls draw the time once the
 * file has loaded, which a screenshot cannot wait for.
 */
export const Video: Story = {
  args: { items: playables.slice(0, 1) },
  tags: ["no-screenshot"],
  play: async ({ canvasElement }) => {
    // A <video> has no ARIA role to query by.
    const video = canvasElement.querySelector("video");
    await expect(video).toHaveAccessibleName("A sunny sky, filmed for a second");
    await expect(video).toHaveAttribute("controls");
    await expect(video).not.toHaveAttribute("autoplay");
  },
};

/** An audio file plays in an AudioPlayer, not the browser's controls, and never starts by itself. */
export const Audio: Story = {
  args: { items: playables.slice(1, 3) },
  tags: ["no-screenshot"],
  play: async ({ canvas, canvasElement }) => {
    await expect(canvasElement.querySelector("audio[controls]")).toBeNull();
    await expect(canvas.getAllByRole("button", { name: "Play" })).toHaveLength(2);
  },
};

/** A gifv loops by itself without controls, unless animations do not play by themselves or motion is reduced. */
export const Gifv: Story = { args: { items: playables.slice(3) }, tags: ["no-screenshot"] };

export const Mixed: Story = {
  args: { items: [...pictures.slice(0, 1), ...playables.slice(0, 2), ...playables.slice(3)] },
  tags: ["no-screenshot"],
};
