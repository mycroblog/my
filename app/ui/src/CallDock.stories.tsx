// SPDX-License-Identifier: AGPL-3.0-only
// CallDock stories: the controls of the host (the microphone on, reactions, invite, the raised hands with their
// count, leaving), of a listener with a raised hand, the arrow keys of the toolbar, and a notice under it.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, waitFor } from "storybook/test";
import { CallDock, CallDockButton } from "./CallDock";
import { EmojiIcon, InviteIcon, LeaveIcon, MicrophoneIcon, RaiseHandIcon } from "./Icons";
import { InlineError } from "./Text";

const onLeave = fn();

function Microphone({ initial }: { initial: boolean }) {
  const [on, setOn] = useState(initial);
  return (
    <CallDockButton
      label="Microphone"
      icon={<MicrophoneIcon filled={on} />}
      showLabel
      isSelected={on}
      onChange={setOn}
    />
  );
}

function RaiseHand({ initial }: { initial: boolean }) {
  const [raised, setRaised] = useState(initial);
  return (
    <CallDockButton
      label="Raise hand"
      icon={<RaiseHandIcon filled={raised} />}
      isSelected={raised}
      onChange={setRaised}
      selectedTone="hand"
    />
  );
}

const meta = {
  title: "Voice/CallDock",
  component: CallDock,
  args: {
    label: "Call controls",
    children: (
      <>
        <Microphone initial />
        <CallDockButton label="Reactions" icon={<EmojiIcon />} />
        <CallDockButton label="Invite" icon={<InviteIcon />} />
        <CallDockButton label="Hands" icon={<RaiseHandIcon />} showLabel count={{ value: 2, label: "2 raised hands" }} />
      </>
    ),
    end: <CallDockButton label="Leave" icon={<LeaveIcon />} showLabel variant="danger" onPress={onLeave} />,
  },
} satisfies Meta<typeof CallDock>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The host's controls: the microphone is on (coral, pressed), and two people wait with a raised hand. */
export const Host: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("toolbar", { name: "Call controls" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Microphone", pressed: true })).toBeVisible();
    // The count describes the button, whose name stays the same.
    await expect(canvas.getByRole("button", { name: "Hands" })).toHaveAccessibleDescription("2 raised hands");
    // Every control is at least 44 px square.
    for (const button of canvas.getAllByRole("button")) {
      const box = button.getBoundingClientRect();
      await expect(box.height).toBeGreaterThanOrEqual(44);
      await expect(box.width).toBeGreaterThanOrEqual(44);
    }
  },
};

/** A listener: a raised hand (mango, pressed), reactions, and leaving. */
export const Listener: Story = {
  args: {
    children: (
      <>
        <RaiseHand initial />
        <CallDockButton label="Reactions" icon={<EmojiIcon />} />
      </>
    ),
  },
  play: async ({ canvas, userEvent }) => {
    const hand = canvas.getByRole("button", { name: "Raise hand", pressed: true });
    await userEvent.click(hand);
    await expect(hand).toHaveAttribute("aria-pressed", "false");
  },
};

/** The arrow keys move along the line (mirrored in right-to-left text), and Space and Enter press. */
export const Keyboard: Story = {
  play: async ({ canvas, userEvent }) => {
    onLeave.mockClear();
    const [next, previous] = document.documentElement.dir === "rtl" ? ["{ArrowLeft}", "{ArrowRight}"] : ["{ArrowRight}", "{ArrowLeft}"];
    await userEvent.tab();
    const microphone = canvas.getByRole("button", { name: "Microphone" });
    await expect(microphone).toHaveFocus();
    await userEvent.keyboard(next);
    const reactions = canvas.getByRole("button", { name: "Reactions" });
    await expect(reactions).toHaveFocus();
    // An icon-only control shows its name as a tooltip on focus.
    await waitFor(() => expect(canvas.getByRole("tooltip")).toHaveTextContent("Reactions"));
    await userEvent.keyboard(previous);
    await expect(microphone).toHaveFocus();
    await userEvent.keyboard(" ");
    await expect(microphone).toHaveAttribute("aria-pressed", "false");
    // Leaving, at the end of the line, comes last.
    await userEvent.keyboard(next.repeat(4));
    await expect(canvas.getByRole("button", { name: "Leave" })).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(onLeave).toHaveBeenCalledOnce();
  },
};

/** What failed shows under the toolbar. */
export const WithNotice: Story = {
  args: { notice: <InlineError alert>No microphone was found. Connect one, then try again.</InlineError> },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("alert")).toHaveTextContent("No microphone was found.");
  },
};
