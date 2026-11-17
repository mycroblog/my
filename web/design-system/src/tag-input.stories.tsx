// SPDX-License-Identifier: AGPL-3.0-only

// The hashtags of an article (#3091): none yet, some, and full, where the field takes no more.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
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
    </div>
  );
}

const meta = { title: "Tag input" } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** None yet. */
export const Empty: Story = { render: () => <Tags initial={[]} /> };

/** Some hashtags, each with its remove. */
export const WithHashtags: Story = { render: () => <Tags initial={["bread", "rye"]} /> };

/** Full, the field takes no more. */
export const Full: Story = {
  render: () => <Tags initial={["a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10"]} />,
};
