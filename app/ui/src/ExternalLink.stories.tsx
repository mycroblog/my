// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { ExternalLink, ExternalLinkContext } from "./ExternalLink";

const meta = {
  title: "Forms/ExternalLink",
  component: ExternalLink,
  args: {
    href: "https://remote.example/@carol",
    variant: "standalone",
    children: "Open the original profile on remote.example",
  },
  // The app gives the "opens in a new tab" text once, at its root.
  decorators: [
    (Story) => (
      <ExternalLinkContext value={{ newTabLabel: "(opens in a new tab)", isAppLink: () => false }}>
        <Story />
      </ExternalLinkContext>
    ),
  ],
} satisfies Meta<typeof ExternalLink>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * A link that leaves the app opens in a new tab without the opener or the referrer, ends with the external-link
 * icon (which mirrors in right-to-left text), and says to assistive technology that it opens in a new tab.
 */
export const Standalone: Story = {
  play: async ({ canvas }) => {
    const link = canvas.getByRole("link", { name: "Open the original profile on remote.example (opens in a new tab)" });
    await expect(link).toHaveAttribute("target", "_blank");
    await expect(link).toHaveAttribute("rel", "noopener noreferrer");
  },
};

/** Inside running text, with the rel of user content kept. */
export const Inline: Story = {
  args: { variant: "inline", rel: "nofollow ugc", children: "the rules of remote.example" },
  render: (args) => (
    <p className="max-w-prose">
      Before you follow Carol, read <ExternalLink {...args} />.
    </p>
  ),
  play: async ({ canvas }) => {
    await expect(
      canvas.getByRole("link", { name: "the rules of remote.example (opens in a new tab)" }),
    ).toHaveAttribute("rel", "noopener noreferrer nofollow ugc");
  },
};

/** A name given by `aria-label` (here, a name that says more than the host it shows) ends with the same text. */
export const Labelled: Story = {
  args: {
    "aria-label": "Open the original post on remote.example",
    className: "text-sm font-normal text-fg-muted",
    children: <span dir="ltr">remote.example</span>,
  },
  play: async ({ canvas }) => {
    await expect(
      canvas.getByRole("link", { name: "Open the original post on remote.example (opens in a new tab)" }),
    ).toBeVisible();
  },
};
