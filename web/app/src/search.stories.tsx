// SPDX-License-Identifier: AGPL-3.0-only

// Search (FR-305–FR-307, FR-309) and finding people (FR-308), on the fake server (app.samples.tsx): the search before
// anything is written; its results on a server without a search engine (people and hashtags alone, no posts offered)
// and on one with it (posts too); a remote account looked up by its handle and one that is not there; nothing found;
// each state every screen is shown in; the screen of the suggested accounts and its states, a one-person server's;
// and the home of a reader who follows no one yet, which offers the suggestions and leads to all of them.

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

PostsNotOffered.play = holds(
  "people found, and no post",
  (main) => main.querySelector('a[href^="/@sam"]') !== null && main.querySelector("article") === null,
);
Results.play = holds(
  "people, hashtags and posts",
  (main) =>
    main.querySelector('a[href^="/@kobo@bread.example"]') !== null &&
    main.querySelector('a[href^="/tags/baking"]') !== null &&
    main.querySelector("article") !== null,
);
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
/** One failure: one line with one Try again, at the column's start. */
Failed.play = holds("one failure", (main) => main.querySelectorAll('[role="alert"]').length === 1);
RemoteLookup.play = holds(
  "the account looked up",
  (main) => main.querySelector('a[href^="/@kiri@far.example"]') !== null,
);
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
