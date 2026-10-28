// SPDX-License-Identifier: AGPL-3.0-only
// The new-post form (a dialog from lg, a sheet below; FR-101) in the states of ADR-0112 §1: empty, written with a
// content warning and a poll, too long, a reply, a quote, and a post the server refused.
import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect, userEvent, within } from "storybook/test";
import { createTranslate, type I18n } from "../../lib/i18n";
import { instanceFixture, statusPage } from "../../test/fixtures";
import { apiBase, problem } from "../../test/msw/http";
import { longPost, pictureHandlers, quotePost, variedTimeline } from "../../test/postStories";
import { answering, inLocale, narrow, renderScreen, screenLoader, settled } from "../../test/stories";

/** The story's catalog, which the screen loader loaded in the story's locale. */
function translate(context: StoryContext) {
  return createTranslate((context.loaded as { i18n: I18n }).i18n);
}

/** Clicks the first button named `name` and returns the form's dialog. */
async function openWith(context: StoryContext, name: string) {
  await settled(context);
  const [button] = await within(context.canvasElement).findAllByRole("button", { name });
  if (button === undefined) throw new Error(`no button ${name}`);
  await userEvent.click(button);
  return within(document.body).findByRole("dialog", { name: translate(context)("client:compose.title") });
}

/** Opens the form from the navigation's button (or the floating one below md) and returns its dialog. */
function openCompose(context: StoryContext) {
  return openWith(context, translate(context)("client:compose.open"));
}

const meta = {
  title: "Screens/Compose/NewPost",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/home",
      signedIn: true,
      handlers: [
        ...pictureHandlers,
        http.get(`${apiBase}/timelines/home`, () => HttpResponse.json(statusPage(variedTimeline.slice(0, 3)))),
      ],
    },
  },
  play: async (context) => {
    await expect(await openCompose(context)).toBeInTheDocument();
  },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The empty form, opened from the navigation. */
export const Empty: Story = {};

/** A post with a content warning and a poll, ready to send. */
export const Written: Story = {
  play: async (context) => {
    const { userEvent } = context;
    const dialog = within(await openCompose(context));
    await userEvent.type(dialog.getByRole("textbox", { name: "Your post" }), "Which bread next week? #baking");
    await userEvent.click(dialog.getByRole("button", { name: "Add a content warning" }));
    await userEvent.type(dialog.getByRole("textbox", { name: "Content warning" }), "Bread poll");
    await userEvent.click(dialog.getByRole("button", { name: "Add a poll" }));
    await userEvent.type(dialog.getByRole("textbox", { name: "Option 1" }), "Sourdough");
    await userEvent.type(dialog.getByRole("textbox", { name: "Option 2" }), "Rye with caraway seeds");
    await expect(dialog.getByRole("button", { name: "Post" })).toBeEnabled();
  },
};

/** A text longer than the server's limit, here 60 characters: the count says how much to cut. */
export const TooLong: Story = {
  ...answering(
    http.get(`${apiBase}/instance`, () =>
      HttpResponse.json({ ...instanceFixture, limits: { ...instanceFixture.limits, max_post_characters: 60 } }),
    ),
  ),
  play: async (context) => {
    const { userEvent } = context;
    const dialog = within(await openCompose(context));
    const text = dialog.getByRole("textbox", { name: "Your post" });
    await userEvent.type(text, "A very long story about bread, which goes on well past the limit of the server.");
    await expect(text).toHaveAttribute("aria-invalid", "true");
  },
};

/** A reply, opened from a post's page: the post replied to stands above the form. */
export const Reply: Story = {
  parameters: {
    screen: {
      path: `/@${longPost.account.acct}/status/${longPost.id}`,
      handlers: [...pictureHandlers, http.get(`${apiBase}/statuses/:id`, () => HttpResponse.json(longPost))],
    },
  },
  play: async (context) => {
    const dialog = within(await openWith(context, "Reply"));
    await expect(await dialog.findByText(/Replying to/)).toBeInTheDocument();
  },
};

/** A quote, opened from a post's page: the quoted post stands in the form. */
export const Quote: Story = {
  parameters: {
    screen: {
      path: `/@${quotePost.account.acct}/status/${quotePost.id}`,
      handlers: [
        ...pictureHandlers,
        http.get(`${apiBase}/statuses/:id`, () => HttpResponse.json({ ...quotePost, quotable: true })),
      ],
    },
  },
  play: async (context) => {
    const dialog = within(await openWith(context, "Quote"));
    await expect(await dialog.findByText(/Quoting/)).toBeInTheDocument();
  },
};

/** The server failed to take the post: the draft stays in the form, to send again. */
export const PostFailed: Story = {
  ...answering(http.post(`${apiBase}/statuses`, () => problem(503, "about:blank", "The server is busy"))),
  play: async (context) => {
    const { userEvent } = context;
    const dialog = within(await openCompose(context));
    await userEvent.type(dialog.getByRole("textbox", { name: "Your post" }), "This one will not go through.");
    await userEvent.click(dialog.getByRole("button", { name: "Post" }));
    await expect(await dialog.findByText("Could not send the post: The server is busy")).toBeInTheDocument();
  },
};

/** In Japanese. */
export const Japanese: Story = {
  globals: inLocale("ja").globals,
  play: async (context) => {
    await inLocale("ja").play(context);
    await expect(await openCompose(context)).toBeInTheDocument();
  },
};

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = {
  globals: inLocale("ar-XB").globals,
  play: async (context) => {
    await inLocale("ar-XB").play(context);
    await expect(await openCompose(context)).toBeInTheDocument();
  },
};

/** At 320 CSS px: the sheet, opened from the floating button. */
export const Narrow: Story = narrow;
