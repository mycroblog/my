// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { ButtonLink } from "./Button";
import { Steps } from "./Steps";

const meta = {
  title: "Display/Steps",
  component: Steps,
  args: {
    steps: [
      { key: "apply", title: "Apply, saying why you want to join" },
      { key: "review", title: "A moderator reviews it", detail: "Usually within a day or two." },
      { key: "email", title: "You get an email once it is approved" },
    ],
    action: (
      <ButtonLink href="#sign-up" variant="primary">
        Apply for an account
      </ButtonLink>
    ),
  },
} satisfies Meta<typeof Steps>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getAllByRole("listitem")).toHaveLength(3);
  },
};

/** Without an action: steps to follow elsewhere. */
export const WithoutAction: Story = { args: { action: undefined } };

/** Long words wrap instead of scrolling the page. */
export const LongText: Story = {
  args: {
    steps: [
      { key: "a", title: "Copy the handle @averyveryverylongusernamewithoutanyspaces@a-very-long-subdomain.example" },
      {
        key: "b",
        title: "Search for it on your own server",
        detail: "Mastodon, Misskey and other servers can follow it.",
      },
    ],
  },
};
