// SPDX-License-Identifier: AGPL-3.0-only
// ChangeNotice stories: a change that the host made, and the end of a call, with a link back. The play function
// dismisses the notice.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, waitFor } from "storybook/test";
import { Button } from "./Button";
import { ChangeNotice } from "./ChangeNotice";
import { Link } from "./Link";

const meta = {
  title: "Feedback/ChangeNotice",
  component: ChangeNotice,
  args: {
    children: "The host turned your microphone off. You can turn it on again.",
    dismissLabel: "Dismiss",
    onDismiss: fn(),
  },
} satisfies Meta<typeof ChangeNotice>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Neutral: Story = {};

export const Warning: Story = {
  args: {
    tone: "warning",
    children: "The room ended: the host ended it.",
    action: (
      <Link href="#room" variant="standalone">
        Morning radio
      </Link>
    ),
  },
};

function Demo() {
  const [shown, setShown] = useState(false);
  return (
    <div className="flex flex-col items-start gap-3">
      <Button
        onPress={() => {
          setShown(true);
        }}
      >
        Mute me
      </Button>
      {shown && (
        <ChangeNotice
          dismissLabel="Dismiss"
          onDismiss={() => {
            setShown(false);
          }}
        >
          The host turned your microphone off.
        </ChangeNotice>
      )}
    </div>
  );
}

/** The notice shows the change at once and goes when it is dismissed. */
export const ShownAndDismissed: Story = {
  render: () => <Demo />,
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Mute me" }));
    await waitFor(() => expect(canvas.getByText("The host turned your microphone off.")).toBeVisible());
    await userEvent.click(canvas.getByRole("button", { name: "Dismiss" }));
    await expect(canvas.queryByText("The host turned your microphone off.")).not.toBeInTheDocument();
  },
};
