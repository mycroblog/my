// SPDX-License-Identifier: AGPL-3.0-only
// Announcer stories. The live regions are visually hidden, so the play function is the point: an
// announcement lands in the polite or the assertive region, and focus stays where it was.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import { Button } from "./Button";
import { AnnouncerProvider, useAnnouncer } from "./Announcer";

function Demo() {
  const announce = useAnnouncer();
  return (
    <div className="flex flex-wrap gap-2">
      <Button
        onPress={() => {
          announce("3 new posts");
        }}
      >
        Load new posts
      </Button>
      <Button
        onPress={() => {
          announce("Could not send the post. Check your connection.", "assertive");
        }}
      >
        Send
      </Button>
    </div>
  );
}

const meta = {
  title: "Feedback/Announcer",
  component: AnnouncerProvider,
  args: { children: <Demo /> },
} satisfies Meta<typeof AnnouncerProvider>;

export default meta;
type Story = StoryObj<typeof meta>;

export const DoesNotMoveFocus: Story = {
  play: async ({ canvas, userEvent }) => {
    const status = canvas.getByRole("status");
    const alert = canvas.getByRole("alert");
    await expect(status).toBeEmptyDOMElement();

    const load = canvas.getByRole("button", { name: "Load new posts" });
    await userEvent.click(load);
    await expect(status).toHaveTextContent("3 new posts");
    await expect(alert).toBeEmptyDOMElement();
    await expect(load).toHaveFocus();

    // The same message again is a new node, so a screen reader reads it again.
    await userEvent.keyboard("{Enter}");
    await expect(status.childElementCount).toBe(2);
    await expect(load).toHaveFocus();

    const send = canvas.getByRole("button", { name: "Send" });
    await userEvent.tab();
    await userEvent.keyboard("{Enter}");
    await expect(alert).toHaveTextContent("Could not send the post. Check your connection.");
    await expect(send).toHaveFocus();
  },
};

/** Messages leave their region after `clearAfter`, so stale ones are not found later. */
export const ClearsAfterTimeout: Story = {
  args: { clearAfter: 50 },
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Load new posts" }));
    await expect(canvas.getByRole("status")).toHaveTextContent("3 new posts");
    await waitFor(() => expect(canvas.getByRole("status")).toBeEmptyDOMElement());
  },
};
