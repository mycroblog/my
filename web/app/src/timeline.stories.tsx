// SPDX-License-Identifier: AGPL-3.0-only

// The home timeline, the app's first screen, a story per state every screen is shown in (app.samples.tsx), fed new
// posts and reconnections from the Stream panel; a first page that fails once and loads on its retry; and a keyword
// filter of the reader's in effect, which folds the posts that hold its word.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const meta = {
  title: "Timeline",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/" }, "home") },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Normal: Story = {};
export const Empty: Story = { parameters: screen({ at: "/", state: "empty" }, "home") };
export const Loading: Story = { parameters: screen({ at: "/", state: "loading" }, "home") };
export const Failed: Story = { parameters: screen({ at: "/", state: "error" }, "home") };
export const LongText: Story = { parameters: screen({ at: "/", state: "long" }, "home") };
export const FailsFirst: Story = { parameters: screen({ at: "/", failFirst: true }, "home") };
/** A filter of the reader's that folds on the home timeline ("bread"): the posts that hold its word, a boost too. */
export const Filtered: Story = {
  parameters: screen(
    {
      at: "/",
      server: {
        settings: {
          filters: [
            {
              title: "Baking",
              action: "warn",
              contexts: ["home"],
              keywords: [{ keyword: "bread", whole_word: false }],
            },
          ],
        },
      },
    },
    "home",
  ),
};
