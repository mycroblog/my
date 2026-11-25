// SPDX-License-Identifier: AGPL-3.0-only

// What a post shows of its status and asks of its screen: its author, its time and where they lead, its context line,
// its body behind its content warning, its reactions, its translation's line, its link card, its media, and its
// actions, each with its name, its state and what a press asks for. What a menu's keys and a fold's button do is the
// design system's; the poll is in post-poll.dom.test.tsx.

import { describe, expect, spyOn, test } from "bun:test";
import { MenuGroup, MenuItem } from "@my/design-system";
import { cleanup, isInaccessible, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Post, type PostAction, type PostProps, type TranslationFailure } from "./post";
import type { PostAccount, PostStatus, PostTranslation } from "./status";
import { asked, inLocale, inOrder, ken, status, type Part } from "./test/post";

/**
 * The post of `props` for a reader of `locale`, alone in the document: its article, and what its actions asked of the
 * screen.
 */
function post(props: Partial<PostProps> = {}, locale = "en") {
  cleanup();
  const actions: PostAction[] = [];
  inLocale(<Post status={status()} onAction={(action) => actions.push(action)} {...props} />, locale);
  const [article] = screen.getAllByRole("article");
  if (article === undefined) throw new Error("test: the post drew no article");
  return { article, part: within(article), actions };
}

/** The items of the menu that is open, by their words. */
const items = () => within(screen.getByRole("menu")).getAllByRole("menuitem");
const words = (elements: Element[]) => elements.map((element) => element.textContent);

describe("the author's line", () => {
  test("the post is an article named by its author, whose name leads to the profile and whose time to the post's page", () => {
    const shown = status();
    const { article, part } = post({ status: shown });

    expect(screen.getByRole("article", { name: "Lena Novak" }) === article).toBe(true);
    expect(part.getByRole("link", { name: "Lena Novak" }).getAttribute("href")).toBe("/@lena@toot.example.eu");
    const time = part.getByRole("link", { name: "3m ago" });
    expect(time.getAttribute("href")).toBe("/@lena@toot.example.eu/status/10");
    expect(within(time).getByRole<HTMLTimeElement>("time").dateTime).toBe(shown.created_at);
  });

  test("it says who may see the post, and the handle reads left to right under the name", () => {
    const { part } = post();

    expect(part.getByRole("img", { name: "Public" })).not.toBeNull();
    const handle = part.getByText("@lena@toot.example.eu");
    expect(handle.dir).toBe("ltr");
    expect(
      inOrder(part.getByRole("link", { name: "Lena Novak" }), part.getByRole("link", { name: "3m ago" }), handle),
    ).toBe(true);
  });

  test("a local account's handle names its domain too, a name keeps its custom emoji, and the avatar is decoration", () => {
    const { part } = post({ status: status({ account: ken }) });

    expect(part.getByText("@kenji@my.example").dir).toBe("ltr");
    const name = part.getByRole("link", { name: /^Kenji\s*:ramen:$/ });
    expect(within(name).getByRole("img", { name: ":ramen:" }).getAttribute("src")).toBe("https://my.example/ramen.png");
    const avatar = part
      .getAllByRole("presentation", { hidden: true })
      .find((image) => image.getAttribute("src") === "https://my.example/kenji.webp");
    expect(avatar !== undefined && isInaccessible(avatar)).toBe(true);
  });

  for (const [visibility, en] of [
    ["public", "Public"],
    ["local", "Neighbors"],
    ["unlisted", "Quiet public"],
    ["private", "Followers only"],
    ["direct", "Direct"],
  ] as const)
    test(`a ${visibility} post says ${en}`, () => {
      const { part } = post({ status: status({ visibility }) });

      expect(part.getByRole("img", { name: en })).not.toBeNull();
    });

  test("who may see it reads in the reader's locale", () => {
    const { part } = post({ status: status({ visibility: "private" }) }, "ja");

    expect(part.getByRole("img", { name: "フォロワーのみ" })).not.toBeNull();
  });

  test("without its stamp, the author's line has no visibility and no time", () => {
    const { part } = post({ stamp: false });

    expect(part.queryByRole("img", { name: "Public" })).toBeNull();
    expect(part.queryByRole("time")).toBeNull();
    expect(part.getByRole("link", { name: "Lena Novak" })).not.toBeNull();
  });

  test("where the reader's own posts are told apart, the reader's own says so, and another's does not", () => {
    expect(post({ self: "You", own: true }).part.getByText(/^You/)).not.toBeNull();

    expect(post({ self: "You" }).part.queryAllByText(/^You/)).toHaveLength(0);
  });

  test("under a right-to-left locale the handle still reads left to right", () => {
    const { part } = post({}, "ar-XB");

    expect(part.getByText("@lena@toot.example.eu").dir).toBe("ltr");
  });
});

describe("the edited mark", () => {
  test("an edited post says so after its time, a button named for what it opens, which asks for the history", async () => {
    const user = userEvent.setup();
    const { part, actions } = post({ status: status({ edited_at: new Date().toISOString() }) });
    const mark = part.getByRole("button", { name: "Edited, show history" });

    expect(mark.textContent).toBe("edited");
    expect(inOrder(part.getByRole("link", { name: "3m ago" }), mark)).toBe(true);
    await user.click(mark);

    expect(actions).toEqual([{ type: "history", id: "10" }]);
  });

  test("a post never edited has no mark", () => {
    const { part } = post();

    expect(part.queryByRole("button", { name: /Edited/ })).toBeNull();
    expect(part.queryByText("edited")).toBeNull();
  });

  test("it reads in the reader's locale", () => {
    const { part } = post({ status: status({ edited_at: new Date().toISOString() }) }, "ja");

    expect(part.getByRole("button", { name: "編集済み、履歴を表示" }).textContent).toBe("編集済み");
  });
});

describe("the screen's own lines", () => {
  test("a context line comes first, details before the actions and what follows after them, all inside the post", () => {
    const { part } = post({
      context: <p>Pinned</p>,
      details: <p>Nov 14, 2026</p>,
      after: <p>Show 3 more replies</p>,
    });

    expect(
      inOrder(
        part.getByText("Pinned"),
        part.getByRole("link", { name: "Lena Novak" }),
        part.getByText("Quiet morning."),
        part.getByText("Nov 14, 2026"),
        part.getByRole("button", { name: /^Reply/ }),
        part.getByText("Show 3 more replies"),
      ),
    ).toBe(true);
  });
});

describe("the context line", () => {
  const boost = () => status({ id: "20", account: ken, boost: status() });

  test("a boost says who boosted it, and shows the boosted post", () => {
    const { article, part } = post({ status: boost() });

    expect(part.getByText(/boosted$/).textContent).toMatch(/^Kenji\s*boosted$/);
    expect(within(part.getByText(/boosted$/)).getByRole("img", { name: ":ramen:" })).not.toBeNull();
    expect(screen.getByRole("article", { name: "Lena Novak" }) === article).toBe(true);
    expect(part.getByText("Quiet morning.")).not.toBeNull();
    expect(inOrder(part.getByText(/boosted$/), part.getByRole("link", { name: "Lena Novak" }))).toBe(true);
  });

  test("the boost's line reads in the reader's locale, the booster's name isolated", () => {
    const { part } = post({ status: boost() }, "ja");

    const line = part.getByText(/さんがブースト$/);
    expect(line.textContent).toMatch(/^Kenji\s*さんがブースト$/);
    expect(within(line).getByText(/Kenji/).tagName).toBe("BDI");
  });

  test("what the reader asks of a boost is asked of the boosted post", async () => {
    const user = userEvent.setup();
    const { part, actions } = post({ status: boost() });

    await user.click(part.getByRole("button", { name: /^Favourite/ }));

    expect(actions).toEqual([{ type: "favourite", id: "10" }]);
  });

  /** A reply to mika, whom it mentions: of another server (mstdn.example.jp) unless `acct` says otherwise. */
  const reply = (acct = "mika@mstdn.example.jp") =>
    status({
      in_reply_to_account_id: "3",
      mentions: [{ id: "3", username: "mika", acct, url: "https://my.example/@mika" }],
    });

  test("a reply says whom it replies to, an account of another server with its server, the handle left to right", () => {
    const { part } = post({ status: reply() });

    const handle = part.getByText("@mika@mstdn.example.jp");
    expect(handle.dir).toBe("ltr");
    expect(handle.parentElement?.textContent).toBe("Replying to @mika@mstdn.example.jp");
  });

  test("a reply to an account of this server names it without a server", () => {
    const { part } = post({ status: reply("mika") });

    const handle = part.getByText("@mika");
    expect(handle.dir).toBe("ltr");
    expect(handle.parentElement?.textContent).toBe("Replying to @mika");
  });

  test("the reply's line reads in the reader's locale", () => {
    const { part } = post({ status: reply() }, "ja");

    expect(part.getByText("@mika@mstdn.example.jp").parentElement?.textContent).toBe(
      "@mika@mstdn.example.jpさんへの返信",
    );
  });

  test("the reply's line is left out where the screen says it already", () => {
    const { part } = post({ status: reply(), replyingTo: false });

    expect(part.queryByText(/Replying to/)).toBeNull();
    expect(part.getByText("Quiet morning.")).not.toBeNull();
  });

  test("a post that is neither has no such line, nor has a reply to someone it does not mention", () => {
    expect(post().part.queryByText(/Replying to|boosted/)).toBeNull();

    const { part } = post({ status: status({ in_reply_to_account_id: "9" }) });
    expect(part.queryAllByText(/Replying to/)).toEqual([]);
  });
});

describe("the body", () => {
  const soup = {
    id: "m",
    type: "image",
    description: "A bowl of soup",
    preview_url: "https://m.example/s.webp",
  } as const;

  test("a content warning folds the text and the images under Show, which the warning describes", async () => {
    const user = userEvent.setup();
    const { part } = post({ status: status({ spoiler_text: "Food, close up", media_attachments: [soup] }) });
    const show = part.getByRole("button", { name: "Show", description: /Food, close up/, expanded: false });

    expect(isInaccessible(part.getByText("Food, close up"))).toBe(false);
    expect(part.queryByText("Quiet morning.") === null || isInaccessible(part.getByText("Quiet morning."))).toBe(true);
    expect(part.queryByRole("img", { name: "A bowl of soup" })).toBeNull();

    await user.click(show);

    expect(isInaccessible(part.getByText("Quiet morning."))).toBe(false);
    expect(part.getByRole("img", { name: "A bowl of soup" })).not.toBeNull();
    expect(part.getByRole("button", { name: "Hide", expanded: true })).not.toBeNull();
  });

  test("the images show with their descriptions, the small file first", () => {
    const { part } = post({
      status: status({
        media_attachments: [
          {
            id: "a",
            type: "image",
            description: "A ridge",
            preview_url: "https://m.example/a-small.webp",
            url: "https://m.example/a.webp",
          },
          { id: "b", type: "image", url: "https://m.example/b.webp" },
        ],
      }),
    });

    expect(part.getByRole("img", { name: "A ridge" }).getAttribute("src")).toBe("https://m.example/a-small.webp");
    expect(asked("https://m.example/b.webp")).toHaveLength(1);
    expect(part.getByRole("button", { name: "Open image: A ridge" })).not.toBeNull();
    expect(part.getByRole("button", { name: "Open image" })).not.toBeNull();
  });

  test("a quoted post is embedded with its author, its handle, its time, its text in its language and its images", () => {
    const quoted = status({
      id: "30",
      account: ken,
      language: "ja",
      content: "<p>小さなサーバー</p>",
      media_attachments: [{ id: "q", type: "image", description: "森", preview_url: "https://m.example/q.webp" }],
    });
    const { part } = post({ status: status({ quote: { state: "accepted", quoted_status: quoted } }) });

    const text = part.getByText("小さなサーバー");
    expect(text.closest("[lang]")?.getAttribute("lang")).toBe("ja");
    expect(part.getByText("@kenji@my.example").dir).toBe("ltr");
    expect(part.getByRole("img", { name: "森" }).getAttribute("src")).toBe("https://m.example/q.webp");
    expect(part.getAllByRole("time")).toHaveLength(2);
    // The quoting post's own text comes first.
    expect(inOrder(part.getByText("Quiet morning."), text, part.getByRole("button", { name: /^Reply/ }))).toBe(true);
  });

  test("a quote of a post that is gone embeds nothing of it", () => {
    const { part } = post({ status: status({ quote: { state: "deleted" } }) });

    expect(part.getAllByRole("time")).toHaveLength(1);
    expect(part.queryByText("@kenji@my.example")).toBeNull();
  });
});

describe("the reactions", () => {
  const reacted = () =>
    status({
      reactions: [
        { name: "🎉", count: 5 },
        {
          name: "blobcat",
          shortcode: "blobcat",
          emoji: { shortcode: "blobcat", url: "https://m.example/b.gif", static_url: "https://m.example/b.png" },
          count: 8,
          me: true,
        },
      ],
    });

  test("a button per emoji in a named group, named by the emoji and its count, the reader's own pressed", () => {
    const { part } = post({ status: reacted() });
    const group = within(part.getByRole("group", { name: "Reactions" }));

    expect(group.getByRole("button", { name: "🎉, 5", pressed: false })).not.toBeNull();
    expect(group.getByRole("button", { name: ":blobcat:, 8, including yours", pressed: true })).not.toBeNull();
    expect(group.getAllByRole("button")).toHaveLength(2);
  });

  test("a press on one asks to react with it, or to take the reader's own back", async () => {
    const user = userEvent.setup();
    const { part, actions } = post({ status: reacted() });

    await user.click(part.getByRole("button", { name: "🎉, 5" }));
    await user.click(part.getByRole("button", { name: /^:blobcat:/ }));

    expect(actions).toEqual([
      { type: "reaction", id: "10", name: "🎉" },
      { type: "reaction", id: "10", name: "blobcat" },
    ]);
  });

  test("they read in the reader's locale", () => {
    const { part } = post({ status: reacted() }, "ja");

    expect(part.getByRole("button", { name: "🎉、5件" })).not.toBeNull();
  });

  test("a post nobody reacted to has no group of them", () => {
    expect(post().part.queryByRole("group")).toBeNull();
  });
});

describe("the translate line", () => {
  const translation: PostTranslation = {
    language: "en",
    source_language: "ja",
    content: "<p>A small server.</p>",
    spoiler_text: "",
    media_attachments: [],
    provider: { name: "LibreTranslate" },
  };
  const ja = () => status({ language: "ja", content: "<p>小さなサーバー</p>" });
  /** The line that assistive technology hears change. */
  const live = (article: HTMLElement) => {
    const region = article.querySelector<HTMLElement>('[aria-live="polite"]');
    if (region === null) throw new Error("test: the post has no line that is heard to change");
    return region;
  };
  /** Its words, without the joiners that hold its last word and with plain spaces. */
  const line = (article: HTMLElement) =>
    live(article).textContent.replaceAll("\u2060", "").replaceAll("\u00a0", " ").trim();

  test("a post that may be translated offers to, and one that may not does not", async () => {
    const user = userEvent.setup();
    const { part, actions } = post({ status: ja(), translatable: true });

    await user.click(part.getByRole("button", { name: "Translate" }));
    expect(actions).toEqual([{ type: "translate", id: "10" }]);

    expect(post({ status: ja() }).part.queryByRole("button", { name: "Translate" })).toBeNull();
  });

  test("translated, it shows the translation in its language in place of the text, and says from what and by whom", async () => {
    const user = userEvent.setup();
    const { article, part, actions } = post({ status: ja(), translatable: true, translation });

    expect(part.getByText("A small server.").closest("[lang]")?.getAttribute("lang")).toBe("en");
    expect(part.queryByText("小さなサーバー")).toBeNull();
    expect(line(article)).toBe("Post translated from Japanese by LibreTranslate · Show original");
    // Names of a language and of a provider are isolated from the sentence around them.
    expect(part.getByText("Japanese").tagName).toBe("BDI");
    expect(part.getByText("LibreTranslate").tagName).toBe("BDI");
    await user.click(part.getByRole("button", { name: "Show original" }));

    expect(actions).toEqual([{ type: "original", id: "10" }]);
  });

  test("the note says what it knows: no language it could not tell, no provider it was not given", () => {
    const unknown = post({ status: ja(), translation: { ...translation, source_language: "und" } });
    expect(line(unknown.article)).toBe("Post translated by LibreTranslate · Show original");

    const { provider: _, ...nameless } = translation;
    const browser = post({ status: ja(), translation: nameless });
    expect(line(browser.article)).toBe("Post translated from Japanese · Show original");
  });

  test("the note reads in the reader's locale, and a pseudo-locale never changes the provider's name", () => {
    const toJa = { ...translation, language: "ja", source_language: "en" };
    const { article } = post({ status: status(), translation: toJa }, "ja");
    expect(line(article)).toMatch(/^LibreTranslate が投稿を英語から翻訳/);

    const pseudo = post({ status: ja(), translation }, "ar-XB");
    expect(pseudo.part.getByText("LibreTranslate").tagName).toBe("BDI");
  });

  test("on its way, the line says it is translating, offers nothing to press, and the text stays", () => {
    const { article, part } = post({ status: ja(), translatable: true, translating: true });

    expect(line(article)).toBe("Translating…");
    expect(within(live(article)).queryAllByRole("button")).toHaveLength(0);
    expect(part.getByText("小さなサーバー")).not.toBeNull();
  });

  test("while the browser's model downloads, the line shows its share, and a screen reader hears it by quarters", () => {
    /** What is shown, and what assistive technology gets, at `share` of the download. */
    const at = (share: number) => {
      const { part } = post({ status: ja(), translatable: true, translating: share });
      const lines = part.getAllByText(/^Downloading what this browser needs to translate… \d+%$/);
      const percent = (shown: boolean) =>
        /(\d+)%$/.exec(lines.find((element) => isInaccessible(element) === shown)?.textContent ?? "")?.[1];
      return [percent(true), percent(false)];
    };

    expect(at(0.4)).toEqual(["40", "25"]);
    expect([0.1, 0.26, 0.5, 0.74, 0.99, 1].map((share) => at(share)[1])).toEqual(["0", "25", "50", "50", "75", "100"]);
  });

  /** The line of a post whose translation failed with `failed`, and the buttons in it. */
  const failure = (failed: TranslationFailure, locale?: string) => {
    const shown = post({ status: ja(), translatable: true, translationFailure: failed }, locale);
    return {
      ...shown,
      said: line(shown.article),
      buttons: words(within(live(shown.article)).queryAllByRole("button")),
    };
  };

  test("over the limit with nothing else to try, the line says when translation comes back, with nothing to press", () => {
    const { said, buttons } = failure({ reason: "limit", retry: false });

    expect(said).toBe("This server's monthly translation limit is reached. Translation is available again next month.");
    expect(buttons).toEqual([]);
  });

  test("over the limit where the browser may translate, the next press goes to the browser", async () => {
    const user = userEvent.setup();
    const { said, buttons, part, actions } = failure({ reason: "limit", retry: "browser" });

    expect(said).toBe("This server's monthly translation limit is reached. Translate in this browser");
    expect(buttons).toEqual(["Translate in this browser"]);
    await user.click(part.getByRole("button", { name: "Translate in this browser" }));

    expect(actions).toEqual([{ type: "translate", id: "10" }]);
  });

  test("a visibility the provider may not receive hands the next press to the browser", () => {
    expect(failure({ reason: "visibility", retry: "browser" }).said).toBe(
      "Only some people can see this post, so it cannot be sent to this server's translation service. Translate in this browser",
    );
  });

  test("the provider down says so, and Try again asks it again", async () => {
    const user = userEvent.setup();
    const { said, buttons, part, actions } = failure({ reason: "failed", retry: "server" });

    expect(said).toBe("This server's translation service is not answering. Try again");
    expect(buttons).toEqual(["Try again"]);
    await user.click(part.getByRole("button", { name: "Try again" }));

    expect(actions).toEqual([{ type: "translate", id: "10" }]);
  });

  test("a language the provider does not translate, and a browser unable to, offer nothing to press", () => {
    const language = failure({ reason: "language", retry: false });
    expect(language.said).toBe("This server's translation service does not translate between these languages.");
    expect(language.buttons).toEqual([]);

    const browser = failure({ reason: "browser", retry: false });
    expect(browser.said).toBe("This browser cannot translate this post.");
    expect(browser.buttons).toEqual([]);
  });

  test("the refusals and the wait read in the reader's locale", () => {
    expect(failure({ reason: "limit", retry: false }, "ja").said).toBe(
      "このサーバーで今月翻訳できる量の上限に達しました。 来月からまた翻訳できます。",
    );
    expect(failure({ reason: "failed", retry: "server" }, "ja").buttons).toEqual(["もう一度試す"]);
    expect(failure({ reason: "visibility", retry: "browser" }, "ja").buttons).toEqual(["このブラウザで翻訳する"]);
    expect(line(post({ status: ja(), translatable: true, translating: true }, "ja").article)).toBe("翻訳しています…");
  });

  test("behind a content warning, the line stays in reach while the text is folded", () => {
    const warned = status({ language: "ja", spoiler_text: "ネタバレ" });
    const offered = post({ status: warned, translatable: true });
    expect(offered.part.getByRole("button", { name: "Show", expanded: false })).not.toBeNull();
    expect(offered.part.getByRole("button", { name: "Translate" })).not.toBeNull();

    const refused = post({
      status: warned,
      translatable: true,
      translationFailure: { reason: "visibility", retry: "browser" },
    });
    expect(refused.part.getByRole("button", { name: "Translate in this browser" })).not.toBeNull();
    expect(
      inOrder(
        refused.part.getByText("ネタバレ"),
        refused.part.getByRole("button", { name: "Translate in this browser" }),
        refused.part.getByRole("button", { name: "Show", expanded: false }),
      ),
    ).toBe(true);
  });

  test("translated behind a content warning: the warning, the text, the poll's options and the descriptions", async () => {
    const user = userEvent.setup();
    const whole = status({
      language: "ja",
      spoiler_text: "ネタバレ",
      content: "<p>小さなサーバー</p>",
      media_attachments: [{ id: "m", type: "image", description: "スープ", preview_url: "https://m.example/s.webp" }],
      poll: {
        id: "p",
        expires_at: new Date(Date.now() + 2 * 86_400_000).toISOString(),
        expired: false,
        multiple: false,
        votes_count: 0,
        voters_count: 0,
        options: [
          { title: "ライ麦", votes_count: 0 },
          { title: "サワー種", votes_count: 0 },
        ],
        voted: false,
        own_votes: [],
      },
    });
    const { part } = post({
      status: whole,
      translatable: true,
      translation: {
        ...translation,
        content: "<p>[en] 小さなサーバー</p>",
        spoiler_text: "[en] ネタバレ",
        media_attachments: [{ id: "m", description: "[en] スープ" }],
        poll_options: ["[en] ライ麦", "[en] サワー種"],
      },
    });

    expect(part.getByText("[en] ネタバレ")).not.toBeNull();
    expect(part.getByRole("button", { name: "Show original" })).not.toBeNull();
    await user.click(part.getByRole("button", { name: "Show", expanded: false }));

    expect(part.getByText("[en] 小さなサーバー")).not.toBeNull();
    expect(part.getAllByRole("radio").map((radio) => radio.closest("label")?.textContent)).toEqual([
      "[en] ライ麦",
      "[en] サワー種",
    ]);
    expect(part.getByRole("img", { name: "[en] スープ" })).not.toBeNull();
    for (const original of ["ネタバレ", "小さなサーバー", "ライ麦", "サワー種"])
      expect(part.queryByText(original)).toBeNull();
    expect(part.queryByRole("img", { name: "スープ" })).toBeNull();
  });
});

describe("the actions", () => {
  test("reply, boost, favourite, react and more, in that order, named with their counts, with their states", () => {
    const { part } = post({ status: status({ boosted: true }) });
    const reply = part.getByRole("button", { name: "Reply, 2" });
    const boost = part.getByRole("button", { name: "Boost, 1.2K", pressed: true, expanded: false });
    const favourite = part.getByRole("button", { name: "Favourite, 12.8K", pressed: false });
    const react = part.getByRole("button", { name: "React" });
    const more = part.getByRole("button", { name: "More", expanded: false });

    expect(inOrder(reply, boost, favourite, react, more)).toBe(true);
    // The exact counts, which the short ones round.
    expect(boost.getAttribute("aria-description")).toBe("1,234");
    expect(favourite.getAttribute("aria-description")).toBe("12,840");
  });

  test("they are named in the reader's locale, whatever the post's language", () => {
    const { part } = post({}, "ja");

    const reply = part.getByRole("button", { name: "返信, 2" });
    expect(part.getByRole("button", { name: "お気に入り, 1.3万" })).not.toBeNull();
    expect(reply.closest("[lang]")?.getAttribute("lang")).toBe("ja");
  });

  test("reply, favourite and react ask the screen for themselves", async () => {
    const user = userEvent.setup();
    const { part, actions } = post();

    await user.click(part.getByRole("button", { name: /^Reply/ }));
    await user.click(part.getByRole("button", { name: /^Favourite/ }));
    await user.click(part.getByRole("button", { name: "React" }));

    expect(actions).toEqual([
      { type: "reply", id: "10" },
      { type: "favourite", id: "10" },
      { type: "react", id: "10" },
    ]);
  });

  test("boost opens a menu of Boost and Quote, each asking for itself", async () => {
    const user = userEvent.setup();
    const { part, actions } = post();

    await user.click(part.getByRole("button", { name: /^Boost/ }));
    expect(words(items())).toEqual(["Boost", "Quote"]);
    await user.click(screen.getByRole("menuitem", { name: "Boost" }));
    await user.click(part.getByRole("button", { name: /^Boost/ }));
    await user.click(await screen.findByRole("menuitem", { name: "Quote" }));

    expect(actions).toEqual([
      { type: "boost", id: "10" },
      { type: "quote", id: "10" },
    ]);
  });

  /** Opens the post's more menu, presses each of its items from `from` on in turn, and gives the words of them all. */
  async function more(user: ReturnType<typeof userEvent.setup>, part: Part, from = 0) {
    await user.click(part.getByRole("button", { name: "More" }));
    const names = words(await screen.findAllByRole("menuitem"));
    await user.keyboard("{Escape}");
    for (const name of names.slice(from)) {
      await user.click(part.getByRole("button", { name: "More" }));
      await user.click(await screen.findByRole("menuitem", { name }));
    }
    return names;
  }

  test("another's post's more menu copies its link, opens its original, shares and bookmarks it, then the screen's groups on its author", async () => {
    const user = userEvent.setup();
    let muted = 0;
    const { part, actions } = post({
      authorMenu: (
        <MenuGroup title="@lena@toot.example.eu">
          <MenuItem id="mute" icon="mute" onAction={() => muted++}>
            Mute
          </MenuItem>
        </MenuGroup>
      ),
    });

    const names = await more(user, part);

    expect(names).toEqual(["Copy link", "Open original page", "Share", "Bookmark", "Mute"]);
    expect(actions).toEqual([
      { type: "copy-link", id: "10" },
      { type: "open-original", id: "10" },
      { type: "share", id: "10" },
      { type: "bookmark", id: "10" },
    ]);
    expect(muted).toBe(1);
  });

  test("the reader's own post's more menu also edits it, deletes and redrafts it, and last deletes it", async () => {
    const user = userEvent.setup();
    const { part, actions } = post({ own: true });

    // What every post's menu offers comes first, as on another's post.
    const names = await more(user, part, 4);

    expect(names).toEqual([
      "Copy link",
      "Open original page",
      "Share",
      "Bookmark",
      "Edit",
      "Delete and redraft…",
      "Delete…",
    ]);
    expect(actions).toEqual([
      { type: "edit", id: "10" },
      { type: "redraft", id: "10" },
      { type: "delete", id: "10" },
    ]);
  });

  test("a bookmarked post offers to take the bookmark away", async () => {
    const user = userEvent.setup();
    const { part } = post({ status: status({ bookmarked: true }) });

    await user.click(part.getByRole("button", { name: "More" }));

    expect(words(await screen.findAllByRole("menuitem"))).toContain("Remove bookmark");
  });

  test("for a reader without an account, what needs one leads to the sign-in with the post, and the rest stays the screen's", async () => {
    const user = userEvent.setup();
    const asking: string[] = [];
    const { part, actions } = post({ signIn: (pressed) => asking.push(pressed.id) });

    await user.click(part.getByRole("button", { name: /^Reply/ }));
    await user.click(part.getByRole("button", { name: /^Favourite/ }));
    await user.click(part.getByRole("button", { name: "React" }));
    expect(asking).toEqual(["10", "10", "10"]);

    await user.click(part.getByRole("button", { name: "More" }));
    await user.click(await screen.findByRole("menuitem", { name: "Copy link" }));
    expect(actions).toEqual([{ type: "copy-link", id: "10" }]);
  });
});

describe("the link card", () => {
  const ann: PostAccount = {
    id: "4",
    username: "ann",
    acct: "ann@blog.example",
    domain: "blog.example",
    display_name: "Ann Lee",
    emojis: [],
  };
  const card: NonNullable<PostStatus["card"]> = {
    url: "https://blog.example/posts/rye",
    title: "Baking rye at home",
    description: "What three weeks of starters taught me.",
    site_name: "Field Notes",
    image: {
      url: "https://my.example/proxy/rye.webp",
      width: 1200,
      height: 600,
      description: "A dark loaf on a board",
      blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
    },
    author: ann,
  };
  const linked = (over: Partial<PostStatus> = {}) =>
    status({ content: '<p>Read <a href="https://blog.example/posts/rye">this</a></p>', card, ...over });
  const cardOf = (part: Part) => part.queryByRole<HTMLAnchorElement>("link", { name: /Baking rye at home/ });

  test("a post with a card shows it after its text: one link in a new tab, with the site, the title, the description and its author", () => {
    const { part } = post({ status: linked() });
    const shown = cardOf(part);
    if (shown === null) throw new Error("the post shows no card");

    expect(shown.getAttribute("href")).toBe("https://blog.example/posts/rye");
    expect(shown.target).toBe("_blank");
    expect(shown.rel.split(" ")).toContain("noopener");
    expect(shown.textContent).toMatch(
      /Field Notes\s*Baking rye at home\s*What three weeks of starters taught me\..*by Ann Lee\s*\(opens in a new tab\)$/,
    );
    expect(within(shown).getByRole("img", { name: "A dark loaf on a board" }).getAttribute("src")).toBe(
      "https://my.example/proxy/rye.webp",
    );
    expect(inOrder(part.getByText("this"), shown, part.getByRole("button", { name: /^Reply/ }))).toBe(true);
  });

  test("small or large as the reader prefers, the same card", () => {
    for (const cardSize of ["small", "large"] as const) {
      const { part } = post({ status: linked(), cardSize });
      expect(cardOf(part)?.getAttribute("href")).toBe("https://blog.example/posts/rye");
      expect(within(cardOf(part) as HTMLElement).getByRole("img", { name: "A dark loaf on a board" })).not.toBeNull();
    }
  });

  test("a post with media shows no card", () => {
    const { part } = post({
      status: linked({
        media_attachments: [{ id: "m", type: "image", preview_url: "https://m.example/s.webp", description: "Soup" }],
      }),
    });

    expect(cardOf(part)).toBeNull();
    expect(part.getByRole("img", { name: "Soup" })).not.toBeNull();
  });

  test("a post that quotes another shows no card", () => {
    const quoted = status({ id: "30", account: ken, content: "<p>小さなサーバー</p>" });
    const { part } = post({ status: linked({ quote: { state: "accepted", quoted_status: quoted } }) });

    expect(cardOf(part)).toBeNull();
    expect(part.getByText("小さなサーバー")).not.toBeNull();
  });

  test("a sensitive post covers the card's image, with its line and Show outside the link", () => {
    const { part } = post({ status: linked({ sensitive: true }) });
    const shown = cardOf(part);
    const show = part.getByRole("button", { name: "Show", description: "Sensitive image", expanded: false });

    expect(asked("proxy/rye.webp")).toEqual([]);
    expect(shown?.contains(show)).toBe(false);
    expect(shown !== null && inOrder(show, shown)).toBe(true);
  });

  test("a card that cannot be drawn leaves its post drawn", () => {
    // React reports what the card threw as it drew: the test reads it, and keeps it out of the run's output.
    const reported = spyOn(console, "error").mockImplementation(() => undefined);
    try {
      // A title that is no text: the card throws as it draws.
      const broken = { ...card, title: { broken: true } as unknown as string };
      const { part } = post({ status: linked({ card: broken }) });

      expect(cardOf(part)).toBeNull();
      expect(part.getByText("this")).not.toBeNull();
      expect(part.getByRole("button", { name: /^Favourite/ })).not.toBeNull();
      // An object drawn as text: the card did throw, and its boundary caught it.
      expect(
        reported.mock.calls.flat().some((line) => /Objects are not valid as a React child/.test(String(line))),
      ).toBe(true);
    } finally {
      reported.mockRestore();
    }
  });

  test("behind a content warning, the card folds with the text", async () => {
    const user = userEvent.setup();
    const { part } = post({ status: linked({ spoiler_text: "Food" }) });

    expect(cardOf(part)).toBeNull();
    await user.click(part.getByRole("button", { name: "Show", expanded: false }));

    expect(cardOf(part)).not.toBeNull();
  });
});

describe("every kind of media", () => {
  const media: PostStatus["media_attachments"] = [
    {
      id: "v",
      type: "video",
      url: "https://m.example/v.webm",
      preview_url: "https://m.example/v.webp",
      description: "Waves at dusk",
      meta: { duration: 92 },
      blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
    },
    {
      id: "g",
      type: "gifv",
      url: "https://m.example/g.mp4",
      preview_url: "https://m.example/g.webp",
      blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
    },
    { id: "s", type: "audio", url: "https://m.example/s.mp3", description: "Rain", meta: { duration: 192 } },
    { id: "f", type: "unknown", url: "https://files.example.org/slides.key" },
  ];

  test("a video, a GIF, a sound and a file each show as their kind, with their descriptions and lengths", () => {
    const { part } = post({ status: status({ media_attachments: media }) });

    expect(part.getByRole("button", { name: "Play video: Waves at dusk" })).not.toBeNull();
    expect(part.getByText("1:32")).not.toBeNull();
    expect(part.getByRole("button", { name: "Open GIF" })).not.toBeNull();
    expect(part.getByRole("button", { name: "Play audio" })).not.toBeNull();
    expect(part.getByText("Rain")).not.toBeNull();
    expect(part.getByText("0:00 of 3:12")).not.toBeNull();
    const file = part.getByRole<HTMLAnchorElement>("link", { name: /^slides\.key/ });
    expect(file.getAttribute("href")).toBe("https://files.example.org/slides.key");
    expect(file.target).toBe("_blank");
  });

  test("sensitive, one line names them all, and no file of theirs is asked for", () => {
    const { part } = post({ status: status({ sensitive: true, media_attachments: media }) });

    expect(part.getByRole("button", { name: "Show", description: "4 sensitive attachments" })).not.toBeNull();
    expect(asked("https://m.example/")).toEqual([]);
  });

  test("the sensitive line reads in the reader's locale", () => {
    const { part } = post(
      {
        status: status({
          sensitive: true,
          media_attachments: [
            { id: "a", type: "image", preview_url: "https://m.example/a.webp" },
            { id: "b", type: "image", preview_url: "https://m.example/b.webp" },
          ],
        }),
      },
      "ja",
    );

    expect(part.getByRole("button", { name: "表示", description: "センシティブな画像 2 枚" })).not.toBeNull();
  });

  test("a content warning and the sensitive flag are separate: the warning folds the line and the covered media", async () => {
    const user = userEvent.setup();
    const { part } = post({
      status: status({
        spoiler_text: "Injury",
        sensitive: true,
        media_attachments: [
          {
            id: "a",
            type: "image",
            preview_url: "https://m.example/a.webp",
            blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
          },
        ],
      }),
    });

    expect(part.getAllByRole("button", { name: "Show" })).toHaveLength(1);
    await user.click(part.getByRole("button", { name: "Show", description: /Injury/ }));

    expect(part.getByRole("button", { name: "Show", description: "Sensitive image", expanded: false })).not.toBeNull();
    expect(asked("https://m.example/a.webp")).toEqual([]);
  });
});
