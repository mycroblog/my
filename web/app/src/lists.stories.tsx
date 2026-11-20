// SPDX-License-Identifier: AGPL-3.0-only

// The reader's lists, which the menu leads to, a list's timeline and its members: a story per state every screen is
// shown in (app.samples.tsx), of each; and the index opened to make a list. What a press alone reaches in managing
// them (#3124) is the parts' own, in the design system's List management: a list refused at the limit of lists, its
// name in its field, its deletion asked, an account added to its members and refused at the limit of members, and Add
// to list on a profile.
// More of a list's members than a page holds, before and after Show more (pages.samples.ts, `more`).

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";
import { manyMembers } from "./pages.samples";

const index = "/lists";
const list = "list:0";

const meta = {
  title: "Lists",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: index }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Index: Story = {};
export const IndexEmpty: Story = { parameters: screen({ at: index, state: "empty" }) };
export const IndexLoading: Story = { parameters: screen({ at: index, state: "loading" }) };
export const IndexFailed: Story = { parameters: screen({ at: index, state: "error" }) };
export const ListTimeline: Story = { parameters: screen({ at: list }) };
export const ListEmpty: Story = { parameters: screen({ at: list, state: "empty" }) };
export const ListLoading: Story = { parameters: screen({ at: list, state: "loading" }) };
export const ListFailed: Story = { parameters: screen({ at: list, state: "error" }) };
export const ListGone: Story = { parameters: screen({ at: "/lists/gone" }) };
export const ListLongText: Story = { parameters: screen({ at: "list:2", state: "long" }) };
/** The list whose name runs long, with its own posts. */
export const ListLongName: Story = { parameters: screen({ at: "list:2" }) };

// ---- Managing the lists (ADR-0013 §11–§15) -------------------------------------------------------------------------

const create = "/lists?create=1";

/** The index opened to make a list: the name's field in place at its top, New list gone while it is open. */
export const Creating: Story = { parameters: screen({ at: create }) };

/** The members of the first list, each with Remove, under the field that adds. */
export const Members: Story = { parameters: screen({ at: "members:0" }) };
export const MembersEmpty: Story = { parameters: screen({ at: "members:0", state: "empty" }) };
export const MembersLoading: Story = { parameters: screen({ at: "members:0", state: "loading" }) };
export const MembersFailed: Story = { parameters: screen({ at: "members:0", state: "error" }) };
/** More members than a page holds: Show more under the first twenty. */
export const MembersMany: Story = { parameters: screen({ at: "members:0", server: manyMembers }) };
/** The same after Show more: the rest under the first twenty, and no Show more. */
export const MembersManyMore: Story = { parameters: screen({ at: "members:0", server: manyMembers, more: true }) };
/** The members of the list whose name runs long. */
export const MembersLongName: Story = { parameters: screen({ at: "members:2" }) };
