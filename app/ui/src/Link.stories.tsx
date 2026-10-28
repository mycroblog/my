// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { spacing } from "./storySpacing";
import { Badge } from "./Badge";
import { HomeIcon, NotificationsIcon } from "./Icons";
import { Link } from "./Link";

const meta = {
  title: "Forms/Link",
  component: Link,
  args: { href: "#", children: "Read the rules" },
} satisfies Meta<typeof Link>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Inside running text the link is underlined, so color is not its only cue. */
export const Inline: Story = {
  render: (args) => (
    <p className="max-w-prose">
      By signing up you agree to the server rules. <Link {...args} variant="inline" /> before you post.
    </p>
  ),
};

export const Standalone: Story = { args: { variant: "standalone", children: "Show more replies" } };

/**
 * An item of the app's navigation sizes its icon at `size-6`, also inside an element of its own (here
 * the pill that holds a badge).
 */
export const Navigation: Story = {
  render: () => (
    <nav aria-label="Main">
      <ul className="flex gap-2 pt-2">
        <li>
          <Link
            href="#home"
            variant="navigation"
            aria-current="page"
            className="flex-col gap-1 px-2 text-xs text-fg-muted"
          >
            <HomeIcon />
            Home
          </Link>
        </li>
        <li>
          <Link
            href="#notifications"
            variant="navigation"
            aria-label="Notifications, 3 unread"
            className="flex-col gap-1 px-2 text-xs text-fg-muted"
          >
            <span className="relative px-2">
              <NotificationsIcon />
              <Badge count={3} label="3 unread" className="absolute -top-1.5 -inset-e-1" />
            </span>
            Notifications
          </Link>
        </li>
      </ul>
    </nav>
  ),
  play: async ({ canvas }) => {
    for (const link of canvas.getAllByRole("link")) {
      const icon = link.querySelector("svg")?.getBoundingClientRect();
      await expect(icon?.width).toBeCloseTo(spacing(6), 1);
      await expect(icon?.height).toBeCloseTo(spacing(6), 1);
    }
  },
};

export const Disabled: Story = { args: { variant: "standalone", isDisabled: true, children: "Show more replies" } };
