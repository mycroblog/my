// SPDX-License-Identifier: AGPL-3.0-only

// The states of the article editor that follow what the writer writes or presses (#3091), on the fake server as yuki
// writes: a draft whose title, or whose body, is over its limit, which the server is seeded with; and those the screen
// has no prop for, which a play presses: the preview behind Write | Preview (under `desktop`, where one of them shows),
// the schedule's form, and the audience menu (the composer's: who can see it, never direct, and who can reply, boost
// and quote it, with approval) opened from the row.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { scenario, type FakeSeed } from "@my/e2e/fake";
import { userEvent, waitFor, within } from "storybook/test";
import { screen, screenMeta } from "./app.samples";

const meta = {
  title: "Article editor states",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/articles/new" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The editor of the reader's draft of an article `key`, on a server that holds it besides the scenario. */
const draftOf = (key: string, title: string, text: string) =>
  screen({
    at: `article-draft:${key}`,
    server: {
      seed: {
        ...scenario,
        drafts: [
          ...scenario.drafts,
          { key, author: "yuki", text, language: "en", article: { title, summary: "", tags: [] } },
        ],
      } satisfies FakeSeed,
    },
  });

/** A title one character over its 200: the field is invalid and says by how much. */
export const TitleOver: Story = {
  parameters: draftOf("yuki-article-title-over", "A starter in a jar. ".repeat(11).slice(0, 201), "Flour and water."),
};

/** A body whose HTML is over the 100,000 bytes (102,000 characters): the limits' line says by how much. */
export const BodyOver: Story = {
  parameters: draftOf("yuki-article-body-over", "A starter in a jar", "flour ".repeat(17_000)),
};

/** The editor in the canvas, once the screen shows its state. */
async function editor(context: StoryContext) {
  await screenMeta.play(context);
  return within(context.canvasElement);
}

/**
 * The tool of the row at `place` from its end, in any locale: the row ends with the audience (5), the sensitive mark,
 * the schedule (3), the language and Publish.
 */
async function tool(context: StoryContext, place: 3 | 5): Promise<HTMLElement> {
  const found = (await (await editor(context)).findAllByRole("button")).at(-place);
  if (found === undefined) throw new Error("the row has no such tool");
  return found;
}

/** Waits until what a tool opened (a menu, or the schedule's form) has risen into place: a sheet on a phone. */
async function risen(context: StoryContext, selector: string) {
  await waitFor(
    () => {
      const opened = context.canvasElement.ownerDocument.querySelector(selector);
      if (opened === null) throw new Error(`${selector} is not open yet`);
      for (let el: Element | null = opened; el !== null; el = el.parentElement) {
        const style = getComputedStyle(el);
        if (style.opacity !== "1" || !["none", "matrix(1, 0, 0, 1, 0, 0)"].includes(style.transform))
          throw new Error(`${selector} is still rising`);
      }
    },
    { timeout: 5000 },
  );
}

/**
 * A draft previewed from Write | Preview, which shows under `desktop` alone: from it, the preview stands beside what
 * is written, and the play presses nothing.
 */
export const Preview: Story = {
  parameters: screen({ at: "article-draft:yuki-article-trail" }),
  play: async (context) => {
    const canvas = await editor(context);
    // Preview is the second of the switch's two, in any locale.
    const preview = canvas.queryAllByRole("radio")[1];
    if (preview === undefined) return;
    await userEvent.click(preview);
    await waitFor(() => {
      if (preview.getAttribute("aria-checked") !== "true") throw new Error("the preview is not chosen yet");
    });
  },
};

/**
 * The schedule's form opened from the row's tool: a sheet headed by its name and close on a phone, from `tablet` a
 * popover over the tool.
 */
export const ScheduleForm: Story = {
  play: async (context) => {
    await userEvent.click(await tool(context, 3));
    await risen(context, "[data-popup]");
  },
};

/** Opens the audience menu from the row's tool, and returns the queries of the page once it has risen. */
async function audienceMenu(context: StoryContext) {
  await userEvent.click(await tool(context, 5));
  await risen(context, '[role="menu"]');
  return within(context.canvasElement.ownerDocument.body);
}

/**
 * The composer's audience menu, opened from the row's tool: who can see the article, and who can reply, boost and
 * quote it.
 */
export const AudienceMenu: Story = {
  play: async (context) => {
    await audienceMenu(context);
  },
};

/** The audience menu's visibilities: the article's four, from the widest, and no Direct. */
export const AudienceVisibilities: Story = {
  play: async (context) => {
    const page = await audienceMenu(context);
    // Who can see it is the first of the menu's two.
    const [visibility] = page.getAllByRole("menuitem");
    if (visibility === undefined) throw new Error("the menu offers nothing");
    await userEvent.click(visibility);
    await page.findAllByRole("menuitemradio");
  },
};

/** Who can reply, set to followers: the audiences, and under them whether others may ask for approval. */
export const AudienceReply: Story = {
  play: async (context) => {
    const page = await audienceMenu(context);
    // Who can reply, boost and quote is the second of the menu's two; who can reply, the first of its three.
    const permissions = page.getAllByRole("menuitem")[1];
    if (permissions === undefined) throw new Error("the menu has no permissions");
    await userEvent.click(permissions);
    const [reply] = await waitFor(() => {
      const items = page.getAllByRole("menuitem");
      if (items.length !== 3) throw new Error("the permissions are not open yet");
      return items;
    });
    if (reply === undefined) throw new Error("the permissions offer nothing");
    await userEvent.click(reply);
    // Followers is the second audience, after anyone.
    const followers = (await page.findAllByRole("menuitemradio"))[1];
    if (followers === undefined) throw new Error("the audiences have no second one");
    await userEvent.click(followers);
    await page.findByRole("menuitemcheckbox");
  },
};
