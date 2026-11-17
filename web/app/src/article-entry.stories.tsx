// SPDX-License-Identifier: AGPL-3.0-only

// Where the article editor is opened from (#3091): on a PC, New article under New post in the main navigation; on a
// phone, the place New article that starts the Menu's sheet, and the menu of other things to write of the floating
// New post. The home's and the shell's stories show them; this one shows the home of a reader the server keeps from
// writing new articles, which offers none of them.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const meta = {
  title: "Article entry",
  ...screenMeta,
  parameters: {
    ...screenMeta.parameters,
    ...screen({ at: "/", server: { articles: { authoring: "permission" } } }),
  },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** No New article in the navigation, no menu on the floating New post, no New article in the Menu's sheet. */
export const NotAllowed: Story = {};
