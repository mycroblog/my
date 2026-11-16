// SPDX-License-Identifier: AGPL-3.0-only

// Search (FR-305–FR-307, FR-309) and finding people (FR-308), on the fake server (app.samples.tsx): the search before
// anything is written; its results on a server without a search engine (people and hashtags alone, no posts offered)
// and on one with it (posts too); a remote account looked up by its handle, while it is, and one that is not there;
// nothing found; each state every screen is shown in; the screen of the suggested accounts and its states, a one-person
// server's; and the home of a reader who follows no one yet, which offers the suggestions and leads to all of them.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";

const posts = { search: { posts: true } };

const meta = {
  title: "Search",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/search" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** Before anything is written: what the field finds, over the suggested people. */
export const Start: Story = {};
/** Before anything is written, on a server that suggests no one: the field alone, nothing of finding people. */
export const StartNoSuggestions: Story = { parameters: screen({ at: "/search", state: "empty" }) };
/** People and hashtags on a server without a search engine: nothing of posts shows. */
export const PostsNotOffered: Story = { parameters: screen({ at: "/search?q=sa" }) };
/**
 * People, hashtags and posts at once on a server with a search engine: "bak" finds Bakery Kobo, #baking and the posts
 * that hold it.
 */
const every = "/search?q=bak";
export const Results: Story = { parameters: screen({ at: every, server: posts }) };
export const ResultsJapanese: Story = {
  parameters: screen({ at: every, server: posts }),
  globals: { locale: "ja" },
};
/** An account of another server named by its handle, looked up there and put first. */
export const RemoteLookup: Story = { parameters: screen({ at: "/search?q=%40kiri%40far.example" }) };
/** A handle no server has. */
export const RemoteNotFound: Story = { parameters: screen({ at: "/search?q=%40nobody%40far.example" }) };
export const NothingFound: Story = { parameters: screen({ at: "/search?q=zzqx", server: posts }) };
export const Loading: Story = { parameters: screen({ at: "/search?q=sa", server: posts, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at: "/search?q=sa", server: posts, state: "error" }) };
export const LongText: Story = { parameters: screen({ at: "/search?q=a", server: posts, state: "long" }) };
export const RightToLeft: Story = { parameters: screen({ at: "/search?q=sa", server: posts }), globals: rightToLeft };
export const MinimumWidth: Story = { parameters: screen({ at: "/search?q=sa", server: posts }), globals: minimumWidth };

/** The screen of the suggested accounts, each with its follow. */
export const People: Story = { parameters: screen({ at: "/people" }) };
/** Finding people reached by its address on a server that suggests no one: it says so plainly. */
export const PeopleEmpty: Story = { parameters: screen({ at: "/people", state: "empty" }) };
export const PeopleLoading: Story = { parameters: screen({ at: "/people", state: "loading" }) };
export const PeopleFailed: Story = { parameters: screen({ at: "/people", state: "error" }) };
export const PeopleMinimumWidth: Story = { parameters: screen({ at: "/people" }), globals: minimumWidth };
/** A one-person server suggests no one, and finding people is no place of its menu. */
export const PeopleSingleUser: Story = {
  parameters: screen({ at: "/people", viewer: "hana", server: { singleUser: true } }),
};
/** The home of a reader who follows no one yet, as right after signing up: the suggested people to follow. */
export const NewAccountHome: Story = { parameters: screen({ at: "/", viewer: "quickdeals" }) };
/** The same home on a server that suggests no one: empty as every timeline is, nothing of finding people. */
export const NewAccountHomeNoSuggestions: Story = {
  parameters: screen({ at: "/", viewer: "quickdeals", state: "empty" }),
};

/** Waits until the screen shows its state, then that `check` holds of its main region. */
const holds =
  (what: string, check: (main: HTMLElement) => boolean) =>
  async (context: StoryContext): Promise<void> => {
    await screenMeta.play(context);
    const main = context.canvasElement.querySelector("main");
    await waitFor(() => {
      if (main === null || !check(main)) throw new Error(`the screen does not show ${what}`);
    });
  };

/** The new account's Find more people followed: the screen of all the suggested people, named Find people. */
export const NewAccountFindsMore: Story = {
  globals: { locale: "en" },
  parameters: screen({ at: "/", viewer: "quickdeals" }),
  play: async (context) => {
    await screenMeta.play(context);
    const main = within(context.canvasElement.querySelector("main") ?? context.canvasElement);
    await userEvent.click(await main.findByRole("link", { name: "Find more people" }));
    await waitFor(() => expect(main.getByRole("list", { name: "Find people" })).toBeVisible());
  },
};
People.play = holds("the suggested people", (main) => main.querySelector('a[href^="/@"]') !== null);
NewAccountHome.play = holds("the suggested people", (main) => main.querySelector('a[href^="/@"]') !== null);
PeopleSingleUser.play = holds("no one", (main) => main.querySelector('a[href^="/@"]') === null);
/** Nothing of finding people: no heading over suggestions, no account to follow, no link to all of them. */
const noFinding = (main: HTMLElement) =>
  main.querySelector("section") === null &&
  main.querySelector('a[href^="/@"]') === null &&
  main.querySelector('a[href="/people"]') === null;
StartNoSuggestions.play = holds("nothing of finding people", noFinding);
NewAccountHomeNoSuggestions.play = holds("nothing of finding people", noFinding);

/** The words a play reads in the story's language, for the locales whose copy it knows (en, ja); else undefined. */
const copy = {
  en: {
    people: "People",
    hashtags: "Hashtags",
    posts: "Posts",
    searching: "Searching…",
    retry: "Try again",
    hintWithoutPosts: "People by name or handle, and hashtags",
    hintWithPosts: "People by name or handle, hashtags, and posts by their words",
    none: "Nothing was found for “zzqx”. Try other words.",
    remoteNone: "No account was found at @nobody@far.example. Check the handle.",
    lookingUp: "Looking up @kiri@far.example on its server…",
    tagPosts: /^\d+ posts?$/u,
  },
  ja: {
    people: "人",
    hashtags: "ハッシュタグ",
    posts: "投稿",
    searching: "検索しています…",
    retry: "もう一度試す",
    hintWithoutPosts: "名前やハンドルで人を、ハッシュタグを探せます",
    hintWithPosts: "名前やハンドルで人を、ハッシュタグを、言葉で投稿を探せます",
    none: "「zzqx」に一致するものは見つかりませんでした。別の言葉で試してください。",
    remoteNone: "@nobody@far.example にアカウントは見つかりませんでした。ハンドルを確かめてください。",
    lookingUp: "@kiri@far.example をそのサーバーに問い合わせています…",
    tagPosts: /^\d+ 件の投稿$/u,
  },
};
type Copy = (typeof copy)["en"];

/** The text of `el` as read, the marks of bidi isolation left out. */
const textOf = (el: Element | null | undefined) => (el?.textContent ?? "").replace(/[⁨⁩]/gu, "").trim();

/**
 * Waits until the screen shows its state, then until `check` passes on its main region, given the copy of the story's
 * language where a play knows it (en, ja).
 */
const plays =
  (check: (main: HTMLElement, w: Copy | undefined) => Promise<void>) =>
  async (context: StoryContext): Promise<void> => {
    await screenMeta.play(context);
    const locale = String(context.globals["locale"] ?? "en");
    const w = locale === "en" || locale === "ja" ? copy[locale] : undefined;
    await waitFor(async () => {
      const main = context.canvasElement.querySelector("main");
      if (main === null) throw new Error("the screen shows no main region");
      await check(main, w);
    });
  };

/** The rows of the accounts in `main`: one per account, whatever links a row holds. */
const accountRows = (main: HTMLElement) =>
  [...main.querySelectorAll("li")].filter((li) => li.querySelector('a[href^="/@"]') !== null);

/** The names of the lists and feeds of `main`, in the order they stand. */
const kindsOf = (main: HTMLElement) =>
  [...within(main).queryAllByRole("list"), ...within(main).queryAllByRole("feed")]
    .toSorted((a, b) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1))
    .map(
      (el) =>
        el.getAttribute("aria-label") ??
        textOf(main.ownerDocument.getElementById(el.getAttribute("aria-labelledby") ?? "")),
    );

/** People and hashtags, and nothing of posts: no kind named Posts, a hint that names none. */
PostsNotOffered.play = plays(async (main, w) => {
  await expect(main.querySelector('a[href^="/@sam"]')).not.toBeNull();
  await expect(main.querySelector("article")).toBeNull();
  if (w === undefined) return;
  await expect(kindsOf(main)).not.toContain(w.posts);
  await expect(within(main).getByRole("searchbox")).toHaveAccessibleDescription(w.hintWithoutPosts);
});

/** People, hashtags and posts, in that order, each hashtag a link to its timeline with how many posts carry it. */
const inSections = async (main: HTMLElement, w: Copy | undefined) => {
  await expect(main.querySelector('a[href^="/@kobo@bread.example"]')).not.toBeNull();
  await expect(main.querySelector("article")).not.toBeNull();
  const tag = main.querySelector<HTMLElement>('a[href="/tags/baking"]');
  await expect(tag).not.toBeNull();
  if (w === undefined || tag === null) return;
  await expect(kindsOf(main)).toEqual([w.people, w.hashtags, w.posts]);
  await expect(within(tag).getByText(w.tagPosts)).toBeVisible();
  await expect(within(main).getByRole("searchbox")).toHaveAccessibleDescription(w.hintWithPosts);
};
Results.play = plays(inSections);
ResultsJapanese.play = plays(inSections);

/** The account looked up on its server stands first. */
RemoteLookup.play = plays(async (main) => {
  await expect(accountRows(main)[0]?.querySelector('a[href^="/@kiri@far.example"]')).not.toBeNull();
});

/** While a handle is looked up on its server: one line that names the handle, and the field busy. */
export const LookingUp: Story = {
  parameters: screen({ at: "/search?q=%40kiri%40far.example", state: "loading" }),
  play: plays(async (main, w) => {
    await expect(main.querySelectorAll('p[role="status"]')).toHaveLength(1);
    await expect(main.querySelectorAll("li")).toHaveLength(0);
    await expect(within(main).getByRole("searchbox").closest('[aria-busy="true"]')).not.toBeNull();
    if (w !== undefined) await expect(textOf(main.querySelector('p[role="status"]'))).toBe(w.lookingUp);
  }),
};

/** One line that no account is at the handle, and no failure. */
RemoteNotFound.play = plays(async (main, w) => {
  await expect(main.querySelector('a[href^="/@nobody"]')).toBeNull();
  await expect(main.querySelector('[role="alert"]')).toBeNull();
  if (w !== undefined) await expect(within(main).getByText(w.remoteNone)).toBeVisible();
});

/** One line that nothing was found, no kind, no failure. */
NothingFound.play = plays(async (main, w) => {
  await expect(main.querySelectorAll("li")).toHaveLength(0);
  await expect(main.querySelector("article")).toBeNull();
  await expect(main.querySelector('[role="alert"]')).toBeNull();
  if (w !== undefined) await expect(within(main).getByText(w.none)).toBeVisible();
});

/** One line while it searches, not one per kind. */
Loading.play = plays(async (main, w) => {
  await expect(main.querySelectorAll('p[role="status"]')).toHaveLength(1);
  await expect(main.querySelectorAll("li")).toHaveLength(0);
  await expect(main.querySelector('[role="alert"]')).toBeNull();
  if (w !== undefined) await expect(textOf(main.querySelector('p[role="status"]'))).toBe(w.searching);
});

/** One failure, never one per kind, with one Try again. */
Failed.play = plays(async (main, w) => {
  await expect(main.querySelectorAll('[role="alert"]')).toHaveLength(1);
  await expect(main.querySelectorAll("li")).toHaveLength(0);
  if (w !== undefined) await expect(within(main).getAllByRole("button", { name: w.retry })).toHaveLength(1);
});
