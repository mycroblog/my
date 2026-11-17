// SPDX-License-Identifier: AGPL-3.0-only

// Search (FR-305–FR-307, FR-309) and finding people (FR-308), on the fake server (app.samples.tsx): the search before
// anything is written; its results on a server without a search engine (people and hashtags alone, no posts offered)
// and on one with it (posts too); a remote account looked up by its handle, while it is, and one that is not there;
// nothing found; each state every screen is shown in; the screen of the suggested accounts and its states, a one-person
// server's; and the home of a reader who follows no one yet, which offers the suggestions and leads to all of them.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

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
export const ResultsJapanese: Story = { parameters: screen({ at: every, server: posts }) };
/** An account of another server named by its handle, looked up there and put first. */
export const RemoteLookup: Story = { parameters: screen({ at: "/search?q=%40kiri%40far.example" }) };
/** A handle no server has. */
export const RemoteNotFound: Story = { parameters: screen({ at: "/search?q=%40nobody%40far.example" }) };
export const NothingFound: Story = { parameters: screen({ at: "/search?q=zzqx", server: posts }) };
export const Loading: Story = { parameters: screen({ at: "/search?q=sa", server: posts, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at: "/search?q=sa", server: posts, state: "error" }) };
export const LongText: Story = { parameters: screen({ at: "/search?q=a", server: posts, state: "long" }) };
export const RightToLeft: Story = { parameters: screen({ at: "/search?q=sa", server: posts }) };
export const MinimumWidth: Story = { parameters: screen({ at: "/search?q=sa", server: posts }) };

/** The screen of the suggested accounts, each with its follow. */
export const People: Story = { parameters: screen({ at: "/people" }) };
/** Finding people reached by its address on a server that suggests no one: it says so plainly. */
export const PeopleEmpty: Story = { parameters: screen({ at: "/people", state: "empty" }) };
export const PeopleLoading: Story = { parameters: screen({ at: "/people", state: "loading" }) };
export const PeopleFailed: Story = { parameters: screen({ at: "/people", state: "error" }) };
export const PeopleMinimumWidth: Story = { parameters: screen({ at: "/people" }) };
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

/** The new account's Find more people followed: the screen of all the suggested people, named Find people. */
export const NewAccountFindsMore: Story = { parameters: screen({ at: "/people", viewer: "quickdeals" }) };
/**
 * Before anything is written, for a reader who follows no one, whom the fake suggests more than five: five of them,
 * then the way to the rest, finding people.
 */
export const StartMoreThanFive: Story = { parameters: screen({ at: "/search", viewer: "quickdeals" }) };
/** While a handle is looked up on its server: one line that names the handle, and the field busy. */
export const LookingUp: Story = { parameters: screen({ at: "/search?q=%40kiri%40far.example", state: "loading" }) };
