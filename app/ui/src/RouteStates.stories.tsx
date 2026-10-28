// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Button } from "./Button";
import { PageHeading } from "./PageHeading";
import { RouteLoading, StandalonePage } from "./RouteStates";
import { atViewport, shellParameters } from "./storyViewports";
import { Text } from "./Text";

const meta = {
  title: "Shell/RouteStates",
  component: RouteLoading,
  args: { label: "Loading" },
} satisfies Meta<typeof RouteLoading>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A page that takes a while to load: the spinner in the page's column. */
export const Loading: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("progressbar", { name: "Loading" })).toBeVisible();
  },
};

/**
 * A page without the app's layout (the layout's own error): centered in the screen, as wide as prose at most. It is as
 * tall as the screen, so the story sets a viewport, as the shell's stories do.
 */
export const Standalone: Story = {
  parameters: shellParameters,
  globals: atViewport("phone"),
  render: () => (
    <StandalonePage>
      <PageHeading>Something went wrong</PageHeading>
      <Text as="p">
        The server could not be reached. Check your connection and try again; the page loads again by itself when you
        are back online.
      </Text>
      <div>
        <Button>Try again</Button>
      </div>
    </StandalonePage>
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("main")).toContainElement(
      canvas.getByRole("heading", { level: 1, name: "Something went wrong" }),
    );
  },
};
