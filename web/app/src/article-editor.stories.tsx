// SPDX-License-Identifier: AGPL-3.0-only

// The article editor (#3091), a screen of its own outside the shell, on the fake server as yuki reads it: a story per
// state, from a new article to the reader's drafts of articles (one scheduled) and an edit of a published one (as sam,
// whose article it is); the preview the server could not render, and a reader the server keeps from writing new
// articles. Each reads at the toolbar's viewport (a phone writes or previews behind Write | Preview, a PC shows both
// side by side) and locale. What follows a press is in the editor's states (article-editor-states.stories.tsx).

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const draft = "article-draft:yuki-article-trail";

const meta = {
  title: "Article editor",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/articles/new" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** A new article: nothing written, the limits at nothing used, the preview's placeholder. */
export const Empty: Story = {};
/** A draft of an article reopened: its title, its body and its hashtags, and the preview of its body. */
export const Draft: Story = { parameters: screen({ at: draft }) };
/** A scheduled article: one line over the row says when it is published, and Schedule takes Publish's place. */
export const Scheduled: Story = { parameters: screen({ at: "article-draft:yuki-article-scheduled" }) };
/** The reader's published article: Edit article and Save, its visibility kept and said over the row. */
export const Edit: Story = { parameters: screen({ at: "article-edit:sam-article", viewer: "sam" }) };
/** The server could not render the body: the preview says so, with Try again. */
export const PreviewFailed: Story = {
  parameters: screen({ at: draft, server: { articles: { preview: "fails" } } }),
};
/** The server keeps new articles to a role the reader does not have: no form, the one sentence why. */
export const NotAllowed: Story = {
  parameters: screen({ at: "/articles/new", server: { articles: { authoring: "permission" } } }),
};
