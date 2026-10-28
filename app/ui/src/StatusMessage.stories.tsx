// SPDX-License-Identifier: AGPL-3.0-only
// StatusMessage stories. The play function checks that the region is in the page, empty, before the message,
// so that a screen reader announces the message when it arrives.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect } from "storybook/test";
import { Button } from "./Button";
import { StatusMessage } from "./StatusMessage";

const meta = {
  title: "Feedback/StatusMessage",
  component: StatusMessage,
  args: { children: "example.com is suspended until 24 October 2026." },
} satisfies Meta<typeof StatusMessage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Success: Story = {};

export const Neutral: Story = { args: { tone: "neutral", children: "No deliveries are queued for example.com." } };

export const LongMessage: Story = {
  args: {
    children:
      "The suspension of @someone-with-a-long-name@very-long-subdomain.example.social is lifted. Follows that it ended stay ended.",
  },
};

function Demo() {
  const [message, setMessage] = useState<string | null>(null);
  return (
    <div className="flex flex-col items-start gap-4">
      <Button
        onPress={() => {
          setMessage("example.com is suspended.");
        }}
      >
        Suspend
      </Button>
      <StatusMessage>{message}</StatusMessage>
    </div>
  );
}

/** The empty region is in the page first; the message fills it after the action. */
export const AfterAnAction: Story = {
  render: () => <Demo />,
  play: async ({ canvas, userEvent }) => {
    const status = canvas.getByRole("status");
    await expect(status).toBeEmptyDOMElement();
    await userEvent.click(canvas.getByRole("button", { name: "Suspend" }));
    await expect(status).toHaveTextContent("example.com is suspended.");
  },
};
