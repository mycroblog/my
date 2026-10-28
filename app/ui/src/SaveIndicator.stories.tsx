// SPDX-License-Identifier: AGPL-3.0-only
// SaveIndicator stories: each state, and a switch that saves itself, whose play function checks that the mark beside
// it shows "Saving…" right after the press and "Saved" once the save is done.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, waitFor } from "storybook/test";
import { AutosaveControl, SaveIndicator, type SaveState } from "./SaveIndicator";
import { Switch } from "./Switch";

const labels = { saving: "Saving…", saved: "Saved", failed: "Not saved" };

const meta = {
  title: "Feedback/SaveIndicator",
  component: SaveIndicator,
  args: { state: "saved", labels },
} satisfies Meta<typeof SaveIndicator>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Saving, saved and not saved, with their words, and the same in a table's cell (the words for assistive technology). */
export const States: Story = {
  render: (args) => (
    <div className="flex flex-col items-start gap-3">
      <SaveIndicator {...args} state="saving" />
      <SaveIndicator {...args} state="saved" />
      <SaveIndicator {...args} state="failed" />
      <div className="flex gap-3">
        <SaveIndicator {...args} state="saving" display="icon" />
        <SaveIndicator {...args} state="saved" display="icon" />
        <SaveIndicator {...args} state="failed" display="icon" />
      </div>
    </div>
  ),
};

/** A switch that saves itself, after a short wait. */
function AutosavedSwitch() {
  const [selected, setSelected] = useState(false);
  const [state, setState] = useState<SaveState>("idle");
  return (
    <AutosaveControl state={state} labels={labels}>
      <Switch
        isSelected={selected}
        onChange={(next) => {
          setSelected(next);
          setState("saving");
          setTimeout(() => {
            setState("saved");
          }, 600);
        }}
        description="New posts go into the timeline as they arrive."
      >
        Show new posts at once
      </Switch>
    </AutosaveControl>
  );
}

/** The mark answers the press at once, beside the switch, and says when the change is kept. */
export const AfterAChange: Story = {
  render: () => <AutosavedSwitch />,
  play: async ({ canvas, userEvent }) => {
    await expect(canvas.queryByText("Saved")).not.toBeInTheDocument();
    await userEvent.click(canvas.getByRole("switch", { name: "Show new posts at once" }));
    // It is in the page at once, and visible once its quick entrance has begun.
    await expect(canvas.getByText("Saving…")).toBeInTheDocument();
    await waitFor(() => expect(canvas.getByText("Saving…")).toBeVisible());
    await waitFor(() => expect(canvas.getByText("Saved")).toBeVisible());
    // The screenshot waits for the end of the entrance.
    await waitFor(() =>
      expect(getComputedStyle(canvas.getByText("Saved").parentElement ?? document.body).opacity).toBe("1"),
    );
  },
};
