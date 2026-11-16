// SPDX-License-Identifier: AGPL-3.0-only

// Search (FR-305–FR-307, FR-309) and finding people (FR-308), on the fake server (app.samples.tsx): the search before
// anything is written; its results on a server without a search engine (people and hashtags alone, no posts offered)
// and on one with it (posts too); a remote account looked up by its handle and one that is not there; nothing found;
// each state every screen is shown in; the screen of the suggested accounts and its states, a one-person server's;
// and the home of a reader who follows no one yet, which offers the suggestions.
//
// Two choices of the decider's are open as live options over every story (@my/devtools/options): `search-results`,
// the kinds of results in sections one under another or in tabs; and `find-people`, finding people reached from the
// search before anything is written, from a place of the menu, or both.

import type { DiscoveryLayout } from "@my/app-features";
import { DiscoveryLayoutProvider } from "@my/app-features";
import { Options } from "@my/devtools/options";
import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { waitFor } from "storybook/test";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";

/** The app in each variant of the open choices of finding. */
function Variants({ app }: { app: () => React.ReactNode }) {
  const people = (results: DiscoveryLayout["results"]) => {
    const as = (where: DiscoveryLayout["people"]) => (
      <DiscoveryLayoutProvider value={{ results, people: where }}>{app()}</DiscoveryLayoutProvider>
    );
    return <Options id="find-people" variants={{ both: as("both"), search: as("search"), menu: as("menu") }} />;
  };
  return <Options id="search-results" variants={{ sections: people("sections"), tabs: people("tabs") }} />;
}

const posts = { search: { posts: true } };

const meta = {
  title: "Search",
  ...screenMeta,
  render: (args: unknown, context: StoryContext) => <Variants app={() => screenMeta.render(args, context)} />,
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
/** People, hashtags and posts on a server with a search engine. */
export const Results: Story = { parameters: screen({ at: "/search?q=bread", server: posts }) };
export const ResultsJapanese: Story = {
  parameters: screen({ at: "/search?q=bread", server: posts }),
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
Results.play = holds("posts found", (main) => main.querySelector("article") !== null);
/** The kinds in tabs, open on the first that found something, each named with how many. */
export const ResultsInTabs: Story = {
  parameters: screen({ at: "/search?q=bread", server: posts }),
  render: (args: unknown, context: StoryContext) => (
    <DiscoveryLayoutProvider value={{ results: "tabs", people: "both" }}>
      {screenMeta.render(args, context)}
    </DiscoveryLayoutProvider>
  ),
  play: holds("tabs named with how many, the first open", (main) => {
    const tabs = [...main.querySelectorAll('[role="tab"]')];
    return (
      tabs.length > 0 &&
      tabs.every((tab) => /\d/u.test(tab.textContent)) &&
      tabs[0]?.getAttribute("aria-selected") === "true"
    );
  }),
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
