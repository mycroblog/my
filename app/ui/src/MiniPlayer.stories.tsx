// SPDX-License-Identifier: AGPL-3.0-only
// MiniPlayer stories: the card of a call with the stacked speakers (one talking), the room's link and who is
// talking, a listener's card with only leaving, a call that is reconnecting, a notice, and a long name cut short.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect } from "storybook/test";
import { CallDockButton } from "./CallDock";
import { LeaveIcon, MicrophoneIcon } from "./Icons";
import { Link } from "./Link";
import { MiniPlayer } from "./MiniPlayer";
import { InlineError } from "./Text";

function Microphone() {
  const [on, setOn] = useState(true);
  return <CallDockButton label="Microphone" size="sm" icon={<MicrophoneIcon filled={on} />} isSelected={on} onChange={setOn} />;
}

const leave = <CallDockButton label="Leave" size="sm" variant="danger-soft" icon={<LeaveIcon />} />;

const meta = {
  title: "Voice/MiniPlayer",
  component: MiniPlayer,
  args: {
    label: "Voice chat call",
    avatars: [
      { id: "1", name: "Haruka", speaking: true },
      { id: "2", name: "Sota" },
      { id: "3", name: "Mio" },
      { id: "4", name: "Kenji" },
    ],
    title: (
      <Link href="#room" variant="standalone">
        <span>Friday night radio</span>
      </Link>
    ),
    status: "Haruka is talking",
    children: (
      <>
        <Microphone />
        {leave}
      </>
    ),
  },
} satisfies Meta<typeof MiniPlayer>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A speaker's card: the microphone and leaving. The stack shows the first three speakers. */
export const Speaker: Story = {
  play: async ({ canvas }) => {
    const card = canvas.getByRole("region", { name: "Voice chat call" });
    await expect(card).toContainElement(canvas.getByRole("link", { name: "Friday night radio" }));
    await expect(canvas.getByRole("button", { name: "Microphone", pressed: true })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Leave" })).toBeVisible();
    await expect(card).toHaveTextContent("Haruka is talking");
  },
};

/** A listener has no microphone: only leaving. */
export const Listener: Story = {
  args: { children: leave },
};

/** While the call is not connected, the status line says so, and nobody is ringed. */
export const Reconnecting: Story = {
  args: {
    avatars: [
      { id: "1", name: "Haruka" },
      { id: "2", name: "Sota" },
    ],
    status: "Reconnecting…",
  },
};

/** What failed shows on a row of its own. */
export const WithNotice: Story = {
  args: { notice: <InlineError alert>No microphone was found. Connect one, then try again.</InlineError> },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("alert")).toHaveTextContent("No microphone was found.");
  },
};

/** A long name and a long status are cut short, and the controls stay on the card. */
export const LongName: Story = {
  args: {
    title: (
      <Link href="#room" variant="standalone">
        <span>Friday night radio with a name long enough to be cut short on every screen</span>
      </Link>
    ),
    status: "Haruka with a display name long enough to be cut short is talking",
  },
};
