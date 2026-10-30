// SPDX-License-Identifier: AGPL-3.0-only
// FileDropZone stories: an area with a field, at rest and while a file is dragged over it. The play function drags a
// file into the area with the browser's own DataTransfer, checks that the drop target shows, and drops it.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, waitFor } from "storybook/test";
import { FileDropZone } from "./FileDropZone";
import { TextArea } from "./TextArea";

const meta = {
  title: "Forms/FileDropZone",
  component: FileDropZone,
  args: {
    label: "Drop to attach",
    onDropFiles: fn(),
    children: <TextArea label="Your post" rows={4} />,
  },
} satisfies Meta<typeof FileDropZone>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("textbox", { name: "Your post" })).toBeVisible();
    await expect(canvas.queryByText("Drop to attach")).not.toBeInTheDocument();
  },
};

/** A file carried over the area: a dashed outline and the label cover it. */
export const Dragging: Story = {
  play: async ({ canvas, args }) => {
    const field = canvas.getByRole("textbox", { name: "Your post" });
    const dataTransfer = new DataTransfer();
    dataTransfer.items.add(new File(["png"], "photo.png", { type: "image/png" }));
    field.dispatchEvent(new DragEvent("dragenter", { bubbles: true, cancelable: true, dataTransfer }));
    await waitFor(() => expect(canvas.getByText("Drop to attach")).toBeVisible());
    await expect(field.closest("[data-drop-target]")).not.toBeNull();
    // Dropped and dragged again, the area takes the files and shows the target once more for the screenshot.
    field.dispatchEvent(new DragEvent("drop", { bubbles: true, cancelable: true, dataTransfer }));
    await waitFor(() => expect(args.onDropFiles).toHaveBeenCalledTimes(1));
    field.dispatchEvent(new DragEvent("dragenter", { bubbles: true, cancelable: true, dataTransfer }));
    await waitFor(() => expect(canvas.getByText("Drop to attach")).toBeVisible());
  },
};
