// SPDX-License-Identifier: AGPL-3.0-only

// The article editor (#3091), a screen of its own outside the shell, on the fake server as yuki reads it: a story per
// state, from a new article to the reader's drafts of articles (one scheduled) and an edit of a published one (as sam,
// whose article it is); the preview the server could not render, a publication its review refuses, and a reader the
// server keeps from writing new articles. Each reads at the toolbar's viewport (a phone writes or previews behind
// Write | Preview, a PC shows both side by side) and locale. A refusal follows a press: a play presses Publish or Save.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { userEvent, waitFor, within } from "storybook/test";
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
/** Presses Publish (or Save), and waits until the refusal stands before it, in the editor, which keeps what is written. */
const pressRefused = async (context: StoryContext) => {
  await screenMeta.play(context);
  const canvas = within(context.canvasElement);
  // Publish ends the row, the last button of the screen, in any locale.
  const publish = (await canvas.findAllByRole("button")).at(-1);
  if (publish === undefined) throw new Error("the editor has no Publish");
  await userEvent.click(publish);
  await waitFor(() => {
    if (!publish.closest("div")?.parentElement?.querySelector('[role="alert"]'))
      throw new Error("the refusal is not shown before Publish yet");
  });
  // React Aria announces the button's busy state for 7 s through a node of the page's own, which names the button
  // by its ID; the checks of the page read the screen once it is gone.
  await waitFor(
    () => {
      if (document.querySelector('body > [data-live-announcer] [aria-labelledby], [role="img"][aria-labelledby]'))
        throw new Error("the busy state is still announced");
    },
    { timeout: 9000 },
  );
};
/** Publish pressed, and the server's review refuses the article: its words before Publish, what is written kept. */
export const PublishingError: Story = {
  parameters: screen({ at: draft, server: { articles: { publish: "refused" } } }),
  play: pressRefused,
};
/**
 * Save pressed on an edit, and the server refuses the value of the summary, which it names: what is wrong stands under
 * the summary, the problem's title alone before Save.
 */
export const SummaryRefused: Story = {
  parameters: screen({ at: "article-edit:sam-article", viewer: "sam", server: { articles: { publish: "summary" } } }),
  play: pressRefused,
};
/** The server keeps new articles to a role the reader does not have: no form, the one sentence why. */
export const NotAllowed: Story = {
  parameters: screen({ at: "/articles/new", server: { articles: { authoring: "permission" } } }),
};
