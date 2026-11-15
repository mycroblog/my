// SPDX-License-Identifier: AGPL-3.0-only

// The hashtags of an article (#3091) played: what is typed becomes a hashtag at Enter, a comma, a space or leaving the
// field, without its `#`; Backspace in the empty field takes the last one back into it; once full, it takes no more.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { TagInput } from "./article-editor";

/** The hashtags with their state, full at `max`. */
function Tags({ initial, max = 10 }: { initial: readonly string[]; max?: number }) {
  const [items, setItems] = useState<readonly string[]>(initial);
  return (
    <div>
      <TagInput
        label="Hashtags"
        items={items}
        removeLabel={(tag) => `Remove #${tag}`}
        placeholder={items.length >= max ? "No more hashtags" : "Add a hashtag"}
        full={items.length >= max}
        onAdd={(tag) => {
          setItems((was) => [...was, tag]);
        }}
        onRemove={(tag) => {
          setItems((was) => was.filter((t) => t !== tag));
        }}
      />
      <button type="button">After</button>
    </div>
  );
}

const meta = { title: "Tag input", globals: { locale: "en" } } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The buttons in `root` that remove a hashtag, one for each. */
const removes = (root: HTMLElement) => within(root).queryAllByRole("button", { name: /^Remove #/ });

/** Enter, a comma, a space and leaving the field each add what was typed, without its `#`. */
export const Adds: Story = {
  render: () => <Tags initial={[]} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const field = canvas.getByRole("textbox", { name: "Hashtags" });
    await userEvent.type(field, "bread{Enter}");
    await userEvent.type(field, "#rye,");
    await userEvent.type(field, "spelt ");
    await userEvent.type(field, "oats");
    await userEvent.click(canvas.getByRole("button", { name: "After" }));
    await waitFor(() => expect(removes(canvasElement)).toHaveLength(4));
    for (const tag of ["bread", "rye", "spelt", "oats"])
      await expect(canvas.getByRole("button", { name: `Remove #${tag}` })).toBeInTheDocument();
    await expect(field).toHaveValue("");
  },
};

/** Backspace in the empty field takes the last hashtag back into it, to change. */
export const BackspaceTakesTheLastBack: Story = {
  render: () => <Tags initial={["bread", "rye"]} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const field = canvas.getByRole("textbox", { name: "Hashtags" });
    await userEvent.click(field);
    await userEvent.keyboard("{Backspace}");
    await waitFor(() => expect(canvas.queryByRole("button", { name: "Remove #rye" })).toBeNull());
    await expect(canvas.getByRole("button", { name: "Remove #bread" })).toBeInTheDocument();
    await expect(field).toHaveValue("rye");
    await expect(removes(canvasElement)).toHaveLength(1);
  },
};

/** Full, the field takes no more. */
export const Full: Story = {
  render: () => <Tags initial={["a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10"]} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const field = canvas.getByRole("textbox", { name: "Hashtags" });
    await userEvent.type(field, "eleven{Enter}");
    await expect(canvas.queryByRole("button", { name: "Remove #eleven" })).toBeNull();
    await expect(removes(canvasElement)).toHaveLength(10);
  },
};
