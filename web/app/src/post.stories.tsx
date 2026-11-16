// SPDX-License-Identifier: AGPL-3.0-only

// A post's page, a story per state: a reply in its thread, a post alone with no replies, a post with reactions and who
// reacted under it, a thread folded (both folds), its posts in another language offered for translation by a provider
// that answers, and by one that fails, and each state every screen is shown in (app.samples.tsx); and an article's
// page, at its width and at the minimum.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";

const thread = "yuki-ridge-reply";

const meta = {
  title: "Post",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: thread }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Thread: Story = {};
export const Alone: Story = { parameters: screen({ at: "mio-hello" }) };
/** A post with reactions: a row per emoji under it, with the faces of who reacted, each leading to its list. */
export const WhoReacted: Story = { parameters: screen({ at: "yuki-ridge" }) };
/** With both folds: the earlier posts and the replies the page does not show at first. */
export const Folded: Story = { parameters: screen({ at: thread, state: "folded" }) };
export const Empty: Story = { parameters: screen({ at: thread, state: "empty" }) };
export const Loading: Story = { parameters: screen({ at: thread, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at: thread, state: "error" }) };
export const LongText: Story = { parameters: screen({ at: thread, state: "long" }) };
/**
 * With a translation provider on the server: the posts in another language than the reader's offer Translate (in en,
 * the Japanese ones; in ja, the English ones), which the provider answers.
 */
export const Translatable: Story = {
  parameters: screen({ at: thread, server: { translation: { provider: "answers" } } }),
};
/** With a provider on the server that fails: Translate tells it, with Try again. */
export const TranslationFailed: Story = {
  parameters: screen({ at: thread, server: { translation: { provider: "failed" } } }),
};
export const RightToLeft: Story = { globals: rightToLeft };
export const MinimumWidth: Story = { globals: minimumWidth };
/**
 * A post whose author approves replies and boosts by hand (ADR-0155): the reader's boost of it says it waits, and the
 * reader's replies under it, which the reader alone sees, say one waits and the other was not approved.
 */
export const WaitingForApproval: Story = { parameters: screen({ at: "layla-asks", server: { approvals: true } }) };
/** An article's page (ADR-0160): its head, its text in full for long reading, its actions, and its replies under it. */
export const Article: Story = { parameters: screen({ at: "sam-article" }) };
export const ArticleMinimumWidth: Story = { parameters: screen({ at: "sam-article" }), globals: minimumWidth };

// The flows of a translation, as the reader of English presses them on Yuki's reply in Japanese, each a story whose
// play presses and checks it: the server's provider and back to the original, the provider down, and the browser's
// translator (stubbed: a headless browser has none) taking over a refusal, at once or on the next press after a
// download. They read in en, whatever the toolbar's locale.

const reply = "北側の尾根道です";

/**
 * Yuki's reply in the canvas, once the screen shows it: its queries, and its words as the reader sees them (the
 * joiners that hold a line's last word, and the spaces that do not break, read as nothing and a space).
 */
async function replyIn({ canvasElement, parameters }: StoryContext, text = reply) {
  await screenMeta.play({ canvasElement, parameters } as StoryContext);
  const article = [...canvasElement.querySelectorAll("article")].find((a) => a.textContent.includes(text));
  if (article === undefined) throw new Error(`the story shows no reply with "${text}"`);
  const words = () => article.textContent.replaceAll("⁠", "").replaceAll(" ", " ");
  return { post: within(article), words };
}

/** The browser's Translator API, answering `availability` for every pair; its model downloads in halves. */
const browserTranslator = (availability: "available" | "downloadable") => () => {
  Object.defineProperty(window, "Translator", {
    configurable: true,
    value: {
      availability: () => Promise.resolve(availability),
      create: ({ targetLanguage, monitor }: { targetLanguage: string; monitor?: (m: EventTarget) => void }) => {
        const target = new EventTarget();
        monitor?.(target);
        if (availability === "downloadable")
          for (const loaded of [0.5, 1]) target.dispatchEvent(Object.assign(new Event("downloadprogress"), { loaded }));
        return Promise.resolve({
          translate: (text: string) => Promise.resolve(`[${targetLanguage}] ${text}`),
          destroy: () => undefined,
        });
      },
    },
  });
  return () => {
    Reflect.deleteProperty(window, "Translator");
  };
};

const translatedReply = /\[en\][^]*北側の尾根道です/;
const english = { locale: "en" };

/** Translate through the server's provider shows the translation and who made it; Show original goes back. */
export const TranslateAndBack: Story = {
  globals: english,
  parameters: screen({ at: thread, server: { translation: { provider: "answers" } } }),
  play: async (context) => {
    const { post, words } = await replyIn(context);
    await userEvent.click(post.getByRole("button", { name: "Translate" }));
    await waitFor(() => expect(words()).toMatch(translatedReply));
    await expect(words()).toContain("Post translated from Japanese by Demo Translate · Show original");
    await userEvent.click(post.getByRole("button", { name: "Show original" }));
    await waitFor(() => expect(words()).not.toContain("[en]"));
    await expect(words()).toContain(reply);
    await expect(post.getByRole("button", { name: "Translate" })).toBeVisible();
  },
};

/** The provider down: the line says so with Try again, which asks it again and says so again. */
export const TranslateFails: Story = {
  globals: english,
  parameters: screen({ at: thread, server: { translation: { provider: "failed" } } }),
  play: async (context) => {
    const { post, words } = await replyIn(context);
    await userEvent.click(post.getByRole("button", { name: "Translate" }));
    await waitFor(() => expect(words()).toContain("This server's translation service is not answering. Try again"));
    await userEvent.click(post.getByRole("button", { name: "Try again" }));
    await waitFor(() => expect(post.getByRole("button", { name: "Try again" })).toBeVisible());
    await expect(words()).not.toContain("[en]");
  },
};

/** No provider may receive the post, and the browser translates it at once, needing no download. */
export const TranslateInBrowser: Story = {
  globals: english,
  parameters: screen({ at: thread, server: { translation: { provider: "unavailable", browser: true } } }),
  beforeEach: browserTranslator("available"),
  play: async (context) => {
    const { post, words } = await replyIn(context);
    await userEvent.click(post.getByRole("button", { name: "Translate" }));
    await waitFor(() => expect(words()).toMatch(translatedReply));
    await expect(post.getByRole("button", { name: "Show original" })).toBeVisible();
    await expect(words()).not.toContain("Demo Translate");
    await expect(words()).not.toContain("cannot be sent");
  },
};

/**
 * Over the server's limit, with a browser whose model must download first: the line offers the browser, whose press
 * downloads the model and translates. A reader of Japanese presses it on Sam's reply in English: the page keeps what
 * the browser said of a pair for the session, and the stories before translated Japanese into English.
 */
export const TranslateInBrowserNext: Story = {
  globals: { locale: "ja" },
  parameters: screen({ at: thread, server: { translation: { provider: "limit", browser: true } } }),
  beforeEach: browserTranslator("downloadable"),
  play: async (context) => {
    const { post, words } = await replyIn(context, "That light!");
    await userEvent.click(post.getByRole("button", { name: "翻訳する" }));
    await waitFor(() =>
      expect(words()).toContain("このサーバーで今月翻訳できる量の上限に達しました。このブラウザで翻訳する"),
    );
    await expect(words()).not.toContain("来月");
    await userEvent.click(post.getByRole("button", { name: "このブラウザで翻訳する" }));
    await waitFor(() => expect(words()).toMatch(/\[ja\][^]*That light!/));
  },
};

/**
 * Sam's article on its page, read in en: its head, its cover, its whole text from the post read alone (the headings,
 * the list, the quote, the table, the link for the image the server did not keep, the code, the rule), and Yuki's reply
 * under it without the line of whom it replies to.
 */
export const ArticleRead: Story = {
  globals: english,
  parameters: screen({ at: "sam-article" }),
  play: async (context) => {
    await screenMeta.play(context);
    const main = within(within(context.canvasElement).getByRole("main"));
    await waitFor(() =>
      expect(main.getByRole("heading", { level: 1, name: "A sourdough starter in seven days" })).toBeVisible(),
    );
    const sam = main.getByRole("article", { name: "Sam Rivera" });
    const page = within(sam);
    await expect(page.getByText(/^What I fed it, when it rose, and the three mistakes/)).toBeVisible();
    await expect(page.getByRole("img", { name: "A round loaf on a wooden board" })).toBeVisible();
    await expect(page.getAllByRole("heading", { level: 2 }).map((h) => h.textContent)).toEqual([
      "The first three days",
      "What I fed it",
    ]);
    await expect(page.getByRole("heading", { level: 3, name: "The mistakes" })).toBeVisible();
    await expect(page.getByText("old cheese", { selector: "strong" })).toBeVisible();
    await expect(page.getAllByRole("listitem").map((li) => li.textContent)).toEqual([
      "Day 1: 50 g flour, 50 g water",
      "Day 2: the same, at the same hour",
      "Day 3: half of it out, then the same",
    ]);
    await expect(page.getByText("A starter is ready when it doubles within six hours of a feed.")).toBeVisible();
    const table = within(page.getByRole("table"));
    await expect(table.getAllByRole("columnheader").map((th) => th.textContent)).toEqual(["Day", "Flour", "Water"]);
    await expect(table.getAllByRole("row")).toHaveLength(4);
    await expect(page.getByRole("link", { name: "the shop on a map" })).toBeVisible();
    await expect(page.queryByRole("img", { name: "the shop on a map" })).toBeNull();
    await expect(page.getByText(/feed: 1 starter : 1 flour : 1 water/)).toBeVisible();
    await expect(page.getByRole("separator")).toBeInTheDocument();
    await expect(page.getByText("Next week: the first loaf.")).toBeVisible();
    const yuki = within(main.getByRole("article", { name: "Yuki 雪" }));
    await expect(yuki.getByText(/三日目のにおい/)).toBeVisible();
    await expect(yuki.queryByText(/Replying to/)).toBeNull();
  },
};

/** Over the server's limit in a browser without a translator: the line says when translation comes back. */
export const TranslationLimit: Story = {
  globals: english,
  parameters: screen({ at: thread, server: { translation: { provider: "limit", browser: true } } }),
  play: async (context) => {
    const { post, words } = await replyIn(context);
    await userEvent.click(post.getByRole("button", { name: "Translate" }));
    await waitFor(() =>
      expect(words()).toContain(
        "This server's monthly translation limit is reached. Translation is available again next month.",
      ),
    );
    await expect(post.queryByRole("button", { name: "Translate in this browser" })).toBeNull();
    await expect(post.queryByRole("button", { name: "Try again" })).toBeNull();
  },
};
