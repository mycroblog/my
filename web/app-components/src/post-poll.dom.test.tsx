// SPDX-License-Identifier: AGPL-3.0-only

// A post's poll, from the status: open to the reader's vote or shown as results, what its last line says, and the
// vote it asks the screen to cast. What the options and the vote button are to assistive technology is the design
// system's (poll.dom.test.tsx).

import { expect, test } from "bun:test";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Post, type PostAction, type PostProps } from "./post";
import type { PostPoll } from "./status";
import { inLocale, status } from "./test/post";

const poll = (over: Partial<PostPoll> = {}): PostPoll => ({
  id: "p",
  expires_at: new Date(Date.now() + 2 * 86_400_000 + 3_600_000).toISOString(),
  expired: false,
  multiple: false,
  votes_count: 4,
  voters_count: 4,
  options: [
    { title: "Sourdough", votes_count: 3 },
    { title: "Rye", votes_count: 1 },
  ],
  voted: false,
  own_votes: [],
  ...over,
});

/**
 * The poll of a post whose status carries `over`, alone in the document, for a reader of `locale`: its group, its
 * options' words, its last line's, and what the post asked of the screen.
 */
function shown(over: Partial<PostPoll> = {}, props: Partial<PostProps> = {}, locale = "en") {
  cleanup();
  const actions: PostAction[] = [];
  inLocale(
    <Post status={status({ poll: poll(over) })} onAction={(action) => actions.push(action)} {...props} />,
    locale,
  );
  const group = screen.getByRole("group", { name: locale === "ja" ? "投票" : "Poll" });
  const part = within(group);
  return {
    part,
    actions,
    /** Each result's words, as a reader gets them. */
    results: () => part.queryAllByRole("listitem").map((item) => item.textContent.replace(/\s+/g, " ").trim()),
    /** The words of the poll outside its options: its last line and its vote. */
    words: () => group.textContent,
    inputs: () => [
      ...part.queryAllByRole<HTMLInputElement>("radio"),
      ...part.queryAllByRole<HTMLInputElement>("checkbox"),
    ],
    vote: () => part.queryByRole<HTMLButtonElement>("button", { name: locale === "ja" ? "投票する" : "Vote" }),
  };
}

test("a post without a poll shows none", () => {
  inLocale(<Post status={status()} />);

  expect(screen.queryAllByRole("group", { name: "Poll" })).toHaveLength(0);
});

test("open to the reader with one choice: a radio per option, Vote waiting for a choice, who voted and when it ends", () => {
  const { part, words, vote } = shown();
  const options = within(part.getByRole("radiogroup", { name: "Options" }));

  expect(options.getAllByRole("radio")).toHaveLength(2);
  expect(options.getByRole("radio", { name: "Sourdough", checked: false })).not.toBeNull();
  expect(options.getByRole("radio", { name: "Rye", checked: false })).not.toBeNull();
  expect(vote()?.disabled).toBe(true);
  expect(words()).toMatch(/4 people voted\s*·\s*Ends in 2d/);
  expect(words()).not.toContain("%");
});

test("several choices are a checkbox per option, and the line says several may be chosen", () => {
  const { part, words } = shown({ multiple: true });
  const options = within(part.getByRole("group", { name: "Options" }));

  expect(options.getAllByRole("checkbox")).toHaveLength(2);
  expect(part.queryAllByRole("radio")).toHaveLength(0);
  expect(words()).toMatch(/Several choices allowed\s*·\s*4 people voted\s*·\s*Ends in 2d/);
});

test("a choice enables Vote, which asks the screen to cast it", async () => {
  const user = userEvent.setup();
  const { part, actions, vote } = shown();

  await user.click(part.getByRole("radio", { name: "Rye" }));
  expect(vote()?.disabled).toBe(false);
  await user.click(part.getByRole("button", { name: "Vote" }));

  expect(actions).toEqual([{ type: "vote", id: "10", poll: "p", choices: [1] }]);
});

test("several choices are cast together, in the options' order", async () => {
  const user = userEvent.setup();
  const { part, actions } = shown({ multiple: true });

  await user.click(part.getByRole("checkbox", { name: "Rye" }));
  await user.click(part.getByRole("checkbox", { name: "Sourdough" }));
  await user.click(part.getByRole("button", { name: "Vote" }));

  expect(actions).toEqual([{ type: "vote", id: "10", poll: "p", choices: [0, 1] }]);
});

test("in flight, the reader's choices stay held and Vote is busy, with nothing to change and no shares yet", () => {
  const { part, inputs, words } = shown({ multiple: true }, { voting: [1] });

  expect(inputs().map((input) => [input.disabled, input.checked])).toEqual([
    [true, false],
    [true, true],
  ]);
  expect(part.getByRole("button", { name: "Vote", busy: true })).not.toBeNull();
  expect(words()).not.toContain("%");
});

test("after the reader's vote, each option's share, the reader's choice named, and no Vote", () => {
  const { results, inputs, vote, words } = shown({ voted: true, own_votes: [0] });

  expect(results()).toEqual(["Sourdough (your vote)75%", "Rye25%"]);
  expect(inputs()).toHaveLength(0);
  expect(vote()).toBeNull();
  expect(words()).toMatch(/4 people voted\s*·\s*Ends in 2d/);
});

test("several choices voted are each named", () => {
  const { results } = shown({ multiple: true, voted: true, own_votes: [0, 1] });

  expect(results()).toEqual(["Sourdough (your vote)75%", "Rye (your vote)25%"]);
});

test("ended, it shows the results without a vote, and says it ended", () => {
  const { results, inputs, vote, words } = shown({
    expired: true,
    expires_at: new Date(Date.now() - 3_600_000).toISOString(),
  });

  expect(results()).toEqual(["Sourdough75%", "Rye25%"]);
  expect(inputs()).toHaveLength(0);
  expect(vote()).toBeNull();
  expect(words()).toMatch(/4 people voted\s*·\s*Ended$/);
  expect(words()).not.toContain("Ends in");
});

test("the reader's own poll shows its results, never a vote", () => {
  const { results, inputs, vote } = shown({}, { own: true });

  expect(results()).toEqual(["Sourdough75%", "Rye25%"]);
  expect(inputs()).toHaveLength(0);
  expect(vote()).toBeNull();
});

test("counts hidden until it ends: no share of any option, and the line says when they show", () => {
  const hidden = { options: [{ title: "Sourdough" }, { title: "Rye" }] };
  const voted = shown({ ...hidden, voted: true, own_votes: [1] });

  expect(voted.results()).toEqual(["Sourdough", "Rye (your vote)"]);
  expect(voted.words()).toContain("Counts show when it ends");
  expect(voted.words()).not.toContain("%");

  expect(shown(hidden, { own: true }).words()).toContain("Counts show when it ends");
});

test("a poll whose voters are unknown counts its votes instead", () => {
  const { words } = shown({ voters_count: undefined });

  expect(words()).toContain("4 votes");
  expect(words()).not.toContain("people voted");
});

test("it reads in the reader's locale: the group, the options, the line and Vote", () => {
  const open = shown({ multiple: true, voters_count: undefined }, {}, "ja");
  expect(open.part.getByRole("group", { name: "選択肢" })).not.toBeNull();
  expect(open.words()).toMatch(/複数選べます\s*·\s*4票\s*·\s*2日後に締切/);
  expect(open.vote()).not.toBeNull();

  const ended = shown({ expired: true, voted: true, own_votes: [0] }, {}, "ja");
  expect(ended.words()).toMatch(/4人が投票\s*·\s*締切済み$/);
  expect(ended.results()[0]).toMatch(/^Sourdough.*自分の票.*75%$/);

  const hidden = shown({ options: [{ title: "A" }, { title: "B" }] }, { own: true }, "ja");
  expect(hidden.words()).toContain("票数は締切後に表示");
});
