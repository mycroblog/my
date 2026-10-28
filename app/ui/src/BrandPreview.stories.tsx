// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import { BrandPreview, BrandPreviewList } from "./BrandPreview";

/** A stand-in icon, inline so the story does not depend on the network (a drawn PNG in the app). */
const icon = `data:image/svg+xml,${encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" fill="#f6efe2"/><circle cx="32" cy="32" r="20" fill="#2f6f5e"/><circle cx="32" cy="32" r="8" fill="#f6efe2"/></svg>',
)}`;
/** A stand-in logo (4:1). */
const logo = `data:image/svg+xml,${encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 160 40"><rect width="40" height="40" rx="10" fill="#2f6f5e"/><path d="M52 10h8v20h-8zM66 10h22v6H74v3h12v5H74v6h-8z" fill="#2f6f5e"/></svg>',
)}`;
/** A stand-in link preview: the logo on an opaque ground. */
const preview = `data:image/svg+xml,${encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1200 630"><rect width="1200" height="630" fill="#fffaf2"/><rect x="420" y="240" width="150" height="150" rx="36" fill="#2f6f5e"/><path d="M610 270h30v90h-30zM660 270h80v24h-50v12h44v20h-44v24h-30z" fill="#2f6f5e"/></svg>',
)}`;

const meta = {
  title: "Forms/BrandPreview",
  component: BrandPreview,
  args: { place: "image", src: icon, caption: "The icon" },
} satisfies Meta<typeof BrandPreview>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Waits until every image has decoded, so the screenshot shows it. */
async function imagesLoaded(root: HTMLElement) {
  await waitFor(() => {
    for (const img of root.querySelectorAll("img")) {
      if (!img.complete || img.naturalWidth === 0) throw new Error("not loaded");
    }
  });
}

/** The previews of an icon: the image, a browser tab, the home screen, and the maskable app icon. */
export const Icon: Story = {
  render: () => (
    <BrandPreviewList>
      <BrandPreview place="image" src={icon} caption="SVG" />
      <BrandPreview place="tab" src={icon} caption="Browser tab" />
      <BrandPreview place="homeScreen" src={icon} caption="Home screen" />
      <BrandPreview place="maskable" src={icon} caption="App icon" />
    </BrandPreviewList>
  ),
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.getByText("Home screen")).toBeVisible();
    // The images are decorative: the captions say what they show.
    await expect(canvas.queryByRole("img")).toBeNull();
    await imagesLoaded(canvasElement);
  },
};

/** The previews of a logo: the image, an email's header on a light and a dark ground, and a link preview. */
export const Logo: Story = {
  render: () => (
    <BrandPreviewList>
      <BrandPreview place="image" src={logo} caption="SVG" />
      <BrandPreview place="mail" src={logo} caption="Email" />
      <BrandPreview place="darkMail" src={logo} caption="Email, dark" />
      <BrandPreview place="linkPreview" src={preview} caption="Link preview" />
    </BrandPreviewList>
  ),
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.getByText("Link preview")).toBeVisible();
    await imagesLoaded(canvasElement);
  },
};
