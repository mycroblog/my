// SPDX-License-Identifier: AGPL-3.0-only
// OfflineBanner stories. The play checks that the status region is in the page, empty, while online, so that going
// offline is announced, and that the bar goes away again when the connection comes back.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect } from "storybook/test";
import { Button } from "./Button";
import { ExternalLink } from "./ExternalLink";
import { OfflineBanner } from "./OfflineBanner";

const meta = {
  title: "Feedback/OfflineBanner",
  component: OfflineBanner,
  args: { isOffline: true, children: "You're offline. Posts you send are kept and sent when you're back online." },
} satisfies Meta<typeof OfflineBanner>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Offline: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("status")).toHaveTextContent("You're offline.");
  },
};

/** With an action, a link to the server's status page, which follows the message outside the live region. */
export const WithAction: Story = {
  args: {
    action: (
      <ExternalLink href="https://status.example/" variant="standalone">
        Check the server's status
      </ExternalLink>
    ),
  },
  play: async ({ canvas }) => {
    const status = canvas.getByRole("status");
    await expect(status).toHaveTextContent("You're offline.");
    await expect(status).not.toContainElement(canvas.getByRole("link", { name: "Check the server's status" }));
  },
};

/** Online, the region is empty and takes no space; an empty frame has no screenshot, and InAPage shows it in a page. */
export const Online: Story = {
  tags: ["no-screenshot"],
  args: { isOffline: false },
  play: async ({ canvas }) => {
    const status = canvas.getByRole("status");
    await expect(status).toBeEmptyDOMElement();
    await expect(status.getBoundingClientRect().height).toBe(0);
  },
};

function ConnectionDemo() {
  const [offline, setOffline] = useState(false);
  return (
    <div className="flex flex-col">
      <OfflineBanner isOffline={offline}>You're offline.</OfflineBanner>
      <div className="flex flex-col items-start gap-4 px-4 py-3">
        <Button
          onPress={() => {
            setOffline((value) => !value);
          }}
        >
          Toggle the connection
        </Button>
        {["first", "second", "third"].map((key) => (
          <p key={key}>The page goes on below the bar, which stays at the top edge while the page scrolls.</p>
        ))}
      </div>
    </div>
  );
}

/** The bar sits at the top of the page above its content, and the same region carries every change. */
export const InAPage: Story = {
  render: () => <ConnectionDemo />,
  play: async ({ canvas, userEvent }) => {
    const status = canvas.getByRole("status");
    await expect(status).toBeEmptyDOMElement();
    // From the keyboard, so that no pointer hovers the button while the screenshots resize the frame.
    const toggle = canvas.getByRole("button", { name: "Toggle the connection" });
    toggle.focus();
    await userEvent.keyboard("{Enter}");
    await expect(status).toHaveTextContent("You're offline.");
    // The bar around the live region sticks.
    await expect(getComputedStyle(status.parentElement ?? status).position).toBe("sticky");
    await userEvent.keyboard("{Enter}");
    await expect(status).toBeEmptyDOMElement();
    await expect(toggle).toHaveFocus();
    // The screenshot shows the bar.
    await userEvent.keyboard("{Enter}");
  },
};
