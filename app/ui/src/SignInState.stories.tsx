// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { SignInState } from "./SignInState";

const meta = {
  title: "Display/SignInState",
  component: SignInState,
  args: {
    heading: "You are not signed in",
    description: "Sign in to read the posts of the people you follow.",
    signInHref: "#login",
    signInLabel: "Sign in",
  },
} satisfies Meta<typeof SignInState>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The heading says the fact, the description what signing in shows, and the link leads on. */
export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("heading", { level: 2, name: "You are not signed in" })).toBeVisible();
    await expect(canvas.getByRole("link", { name: "Sign in" })).toHaveAttribute("href", "#login");
  },
};
