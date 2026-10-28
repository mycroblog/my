// SPDX-License-Identifier: AGPL-3.0-only
// MediaViewer stories. The viewer is a controlled modal dialog, so each story holds the index in state and opens it at
// once; the page's direction (the locale toolbar) mirrors the pager and the corner buttons.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { MediaViewer, type MediaViewerLabels, type MediaViewerProps } from "./MediaViewer";

/** A placeholder picture as an SVG data URL, so stories load no network images. */
function picture(from: string, to: string): string {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 320 180"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="${from}"/><stop offset="1" stop-color="${to}"/></linearGradient></defs><rect width="320" height="180" fill="url(#g)"/><circle cx="236" cy="58" r="26" fill="#ffd7a5"/><ellipse cx="120" cy="205" rx="230" ry="80" fill="#705e8c"/></svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
}

const labels: MediaViewerLabels = {
  title: (position, count) => `Image ${String(position)} of ${String(count)}`,
  close: "Close",
  position: (position, count) => `${String(position)} / ${String(count)}`,
  altText: "Description",
  previous: "Previous image",
  next: "Next image",
};

const sunset = {
  id: "1",
  sources: { still: [{ url: picture("#9f9fd2", "#fbbd92") }] },
  alt: "Dusk on the way home: pink clouds over a hill and a setting sun.",
  lang: "en",
  width: 800,
  height: 450,
};
const meadow = {
  id: "2",
  sources: { still: [{ url: picture("#b4d4c6", "#ffc5aa") }] },
  alt: null,
  width: 800,
  height: 450,
};
const animation = {
  id: "3",
  // The moving rendition is the picture's own and plays without a reduced-motion preference; the still one stands in otherwise.
  sources: { still: [{ url: picture("#8fb4d4", "#e3dcbf") }], animated: [{ url: picture("#8fb4d4", "#e3dcbf") }] },
  alt: "A loop of the same hill under moving clouds.",
  lang: "en",
  width: 800,
  height: 450,
};

function Controlled({ index: initial, ...props }: MediaViewerProps) {
  const [index, setIndex] = useState(initial);
  const [isOpen, setIsOpen] = useState(true);
  return <MediaViewer {...props} index={index} isOpen={isOpen} onIndexChange={setIndex} onOpenChange={setIsOpen} />;
}

const meta = {
  title: "Timeline/MediaViewer",
  component: MediaViewer,
  parameters: { layout: "fullscreen" },
  args: {
    items: [sunset],
    index: 0,
    isOpen: true,
    labels,
    noDescriptionLabel: "No description",
    onIndexChange: fn(),
    onOpenChange: fn(),
  },
  render: (args) => <Controlled {...args} />,
  decorators: [
    (Story) => (
      <div className="min-h-136">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof MediaViewer>;

export default meta;
type Story = StoryObj<typeof meta>;

/** One picture: no pager, a × over its top corner and a note button over the bottom one. */
export const Single: Story = {};

/** Several pictures: the pager sits under the frame, and the arrow keys step too. */
export const Multiple: Story = {
  args: { items: [sunset, meadow, animation], index: 1 },
  play: async ({ canvasElement }) => {
    const dialog = await within(canvasElement.ownerDocument.body).findByRole("dialog", { name: "Image 2 of 3" });
    await userEvent.click(within(dialog).getByRole("button", { name: "Next image" }));
    await expect(
      within(canvasElement.ownerDocument.body).getByRole("dialog", { name: "Image 3 of 3" }),
    ).toBeInTheDocument();
  },
};

/** A picture with a description has the note button, which lays the text over the whole picture and takes it away again. */
export const WithDescription: Story = {
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    const toggle = await body.findByRole("button", { name: "Description" });
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await userEvent.click(toggle);
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await waitFor(() => expect(body.getByText(sunset.alt)).toBeVisible());
  },
};

/** Without a description there is no note button; the picture is named by the fallback text for screen readers. */
export const WithoutDescription: Story = {
  args: { items: [meadow] },
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await body.findByRole("img", { name: "No description" });
    await expect(body.queryByRole("button", { name: "Description" })).not.toBeInTheDocument();
  },
};

/** An animated picture: its moving rendition replaces the still one unless the system asks for reduced motion. */
export const Animated: Story = {
  args: { items: [animation] },
};
