// SPDX-License-Identifier: AGPL-3.0-only
// CallBar stories: the bar of a call with the room's link, the call's state and its controls, a long room name that
// is cut short, and the region's name.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Button } from "./Button";
import { CallBar } from "./CallBar";
import { Link } from "./Link";
import { Switch } from "./Switch";
import { InlineError } from "./Text";

const meta = {
  title: "Voice/CallBar",
  component: CallBar,
  args: {
    label: "Voice chat call",
    title: (
      <Link href="#room" variant="standalone">
        <span>Friday night radio</span>
      </Link>
    ),
    status: "Connected",
    children: (
      <>
        <Switch defaultSelected>Microphone</Switch>
        <Button variant="danger">Leave</Button>
      </>
    ),
  },
} satisfies Meta<typeof CallBar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Speaker: Story = {
  play: async ({ canvas }) => {
    const bar = canvas.getByRole("region", { name: "Voice chat call" });
    await expect(bar).toContainElement(canvas.getByRole("link", { name: "Friday night radio" }));
    await expect(canvas.getByRole("switch", { name: "Microphone" })).toBeChecked();
  },
};

/** A listener has no microphone: only leaving. */
export const Listener: Story = {
  args: {
    status: "Reconnecting…",
    children: <Button variant="danger">Leave</Button>,
  },
};

/** What failed shows on a row of its own. */
export const WithNotice: Story = {
  args: { notice: <InlineError alert>No microphone was found. Connect one, then try again.</InlineError> },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("alert")).toHaveTextContent("No microphone was found.");
  },
};

/** A long name is cut short, and the controls stay on the bar. */
export const LongName: Story = {
  args: {
    title: (
      <Link href="#room" variant="standalone">
        <span>Friday night radio with a name long enough to be cut short on every screen</span>
      </Link>
    ),
  },
};
