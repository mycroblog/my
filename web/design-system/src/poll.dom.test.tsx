// SPDX-License-Identifier: AGPL-3.0-only

// What a poll is to a reader and to assistive technology: a named group, its options as radios or checkboxes while
// the reader may vote, as results with their shares once they may not, and its last line with the vote.

import { expect, test } from "bun:test";
import { isInaccessible, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { PollBox, PollChoices, PollFoot, PollMark, PollResults, type PollResultItem } from "./poll";

const none = () => undefined;

const options = [
  { key: "a", label: "Sourdough" },
  { key: "b", label: "Rye" },
  { key: "c", label: <bdi>ライ麦</bdi> },
];

/** The options as a caller holds them, and each choice it was told. */
function Choices({ multiple, told }: { multiple: boolean; told: number[][] }) {
  const [chosen, setChosen] = useState<number[]>([]);
  return (
    <PollChoices
      label="Options"
      multiple={multiple}
      items={options}
      chosen={chosen}
      onChange={(next) => {
        told.push(next);
        setChosen(next);
      }}
    />
  );
}

test("the poll is a group named by its label, around what it holds", () => {
  render(
    <PollBox label="Poll">
      <PollChoices label="Options" multiple={false} items={options} chosen={[]} onChange={none} />
      <PollFoot parts={["4 people voted"]} />
    </PollBox>,
  );
  const poll = screen.getByRole("group", { name: "Poll" });

  expect(within(poll).getByRole("radiogroup", { name: "Options" })).not.toBeNull();
  expect(within(poll).getByText("4 people voted")).not.toBeNull();
});

test("with one choice the options are a named group of radios, and choosing one tells the caller it alone", async () => {
  const user = userEvent.setup();
  const told: number[][] = [];
  render(<Choices multiple={false} told={told} />);
  const group = within(screen.getByRole("radiogroup", { name: "Options" }));

  expect(group.getAllByRole("radio").map((radio) => (radio as HTMLInputElement).checked)).toEqual([
    false,
    false,
    false,
  ]);
  await user.click(group.getByRole("radio", { name: "Rye" }));
  await user.click(group.getByRole("radio", { name: "ライ麦" }));

  expect(told).toEqual([[1], [2]]);
  expect(group.getByRole("radio", { name: "ライ麦", checked: true })).not.toBeNull();
  expect(group.getByRole("radio", { name: "Rye", checked: false })).not.toBeNull();
  expect(screen.queryByRole("checkbox")).toBeNull();
});

test("with several choices the options are a named group of checkboxes, each press adding or taking away its own", async () => {
  const user = userEvent.setup();
  const told: number[][] = [];
  render(<Choices multiple told={told} />);
  const group = within(screen.getByRole("group", { name: "Options" }));

  await user.click(group.getByRole("checkbox", { name: "ライ麦" }));
  await user.click(group.getByRole("checkbox", { name: "Sourdough" }));
  await user.click(group.getByRole("checkbox", { name: "ライ麦" }));

  expect(told.map((chosen) => [...chosen].sort())).toEqual([[2], [0, 2], [0]]);
  expect(group.getByRole("checkbox", { name: "Sourdough", checked: true })).not.toBeNull();
  expect(group.getByRole("checkbox", { name: "ライ麦", checked: false })).not.toBeNull();
  expect(screen.queryByRole("radio")).toBeNull();
});

test("an option is chosen from the keyboard: Tab reaches the options, and Space chooses", async () => {
  const user = userEvent.setup();
  const told: number[][] = [];
  render(<Choices multiple told={told} />);

  await user.tab();
  expect(document.activeElement === screen.getByRole("checkbox", { name: "Sourdough" })).toBe(true);
  await user.tab();
  await user.keyboard(" ");

  expect(told).toEqual([[1]]);
});

test("a press anywhere on an option's row chooses it", async () => {
  const user = userEvent.setup();
  const told: number[][] = [];
  render(<Choices multiple={false} told={told} />);

  await user.click(screen.getByText("Rye"));

  expect(told).toEqual([[1]]);
});

test("while a vote is in flight the options are held: what was chosen stays, and nothing changes", async () => {
  const user = userEvent.setup();
  const told: number[][] = [];
  render(
    <PollChoices label="Options" multiple items={options} chosen={[1]} onChange={(next) => told.push(next)} disabled />,
  );
  const boxes = screen.getAllByRole<HTMLInputElement>("checkbox");

  expect(boxes.map((box) => box.disabled)).toEqual([true, true, true]);
  expect(boxes.map((box) => box.checked)).toEqual([false, true, false]);
  await user.click(screen.getByText("Sourdough"));

  expect(told).toEqual([]);
});

const results: PollResultItem[] = [
  { key: "a", label: "Sourdough", share: { value: 0.75, text: "75%" }, mine: true, leading: true },
  { key: "b", label: "Rye", share: { value: 0.25, text: "25%" }, mine: false, leading: false },
];

const words = (element: Element) => element.textContent.replace(/\s+/g, " ").trim();

test("as results the options are a list, each with its share as text, the reader's choice named", () => {
  render(<PollResults multiple={false} items={results} mineLabel="your vote" />);
  const [first, second] = within(screen.getByRole("list")).getAllByRole("listitem");
  if (first === undefined || second === undefined) throw new Error("the list has no two options");

  expect(within(first).getByText("Sourdough", { exact: false })).not.toBeNull();
  expect(within(first).getByText(/your vote/)).not.toBeNull();
  expect(isInaccessible(within(first).getByText(/your vote/))).toBe(false);
  expect(within(first).getByText("75%")).not.toBeNull();
  expect(words(second)).toBe("Rye25%");
  // Nothing is left to choose.
  expect(screen.queryByRole("radio")).toBeNull();
  expect(screen.queryByRole("checkbox")).toBeNull();
});

test("several choices of the reader's are each named", () => {
  render(<PollResults multiple items={results.map((item) => ({ ...item, mine: true }))} mineLabel="your vote" />);

  expect(screen.getAllByText(/your vote/)).toHaveLength(2);
});

test("with the shares hidden until the poll ends, an option shows none", () => {
  render(
    <PollResults
      multiple={false}
      items={[
        { key: "a", label: "Sourdough", mine: false, leading: false },
        { key: "b", label: "Rye", mine: true, leading: false },
      ]}
      mineLabel="your vote"
    />,
  );
  const [first, second] = screen.getAllByRole("listitem");
  if (first === undefined || second === undefined) throw new Error("the list has no two options");

  expect(words(first)).toBe("Sourdough");
  expect(words(second)).toMatch(/^Rye \(?your vote\)?$/);
  expect(screen.queryByText(/%/)).toBeNull();
});

test("the last line shows each of its parts, and no vote where the reader has none", () => {
  render(<PollFoot parts={["Several choices allowed", "4 people voted", "Ends in 2d"]} />);

  for (const part of ["Several choices allowed", "4 people voted", "Ends in 2d"])
    expect(screen.getByText(part)).not.toBeNull();
  expect(screen.queryByRole("button")).toBeNull();
});

test("the vote waits for a choice: disabled until there is one, then a press casts it", async () => {
  const user = userEvent.setup();
  let cast = 0;
  const foot = (disabled: boolean) => (
    <PollFoot parts={["4 people voted"]} vote={{ label: "Vote", disabled, busy: false, onPress: () => cast++ }} />
  );
  const { rerender } = render(foot(true));
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Vote" }).disabled).toBe(true);

  rerender(foot(false));
  await user.click(screen.getByRole("button", { name: "Vote" }));

  expect(cast).toBe(1);
});

test("while the vote is in flight its button is busy and a press casts nothing more", async () => {
  const user = userEvent.setup();
  let cast = 0;
  render(
    <PollFoot
      parts={["4 people voted"]}
      vote={{ label: "Vote", disabled: false, busy: true, onPress: () => cast++ }}
    />,
  );

  const vote = screen.getByRole("button", { name: "Vote", busy: true });
  await user.click(vote);
  vote.focus();
  await user.keyboard("{Enter}");

  expect(cast).toBe(0);
});

test("the mark an option is answered with is decoration", () => {
  const { container } = render(
    <>
      <PollMark multiple={false} />
      <PollMark multiple on />
      <PollMark multiple={false} plain />
    </>,
  );

  expect(Array.from(container.children).map((mark) => isInaccessible(mark))).toEqual([true, true, true]);
});
