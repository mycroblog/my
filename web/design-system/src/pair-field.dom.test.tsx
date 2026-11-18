// SPDX-License-Identifier: AGPL-3.0-only

// The pairs a reader writes (#3200), the fields of a profile: what they are to assistive technology (a list named by
// its label, each pair a group named by its legend with two text fields named by their labels and a button that
// removes the pair), what a pair reports of what is written and of its Remove, where what is wrong goes (with one
// field, which it marks and describes alone; with the pairs as a whole, which it describes), and that nothing can be
// changed while the pairs are disabled. How a pair is laid out at each width is the story's (Pair field) and the
// layout tests'.

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { PairField, PairList, type PairFieldProps } from "./pair-field";

type Pair = { name: string; value: string };

/** What a test gives every pair of its list, or the pair at an index alone. */
type Given = Partial<Pick<PairFieldProps, "maxLength" | "isDisabled">> & {
  /** What is wrong with one field of the pair at `pair` (from 0). */
  refused?: { pair: number; of: "name" | "value"; text: string };
  /** What is wrong with the pairs as a whole. */
  whole?: string;
};

/** A list of `start` that keeps what the reader writes, and counts the presses of each pair's Remove in `removed`. */
function Pairs({ start, removed, given }: { start: readonly Pair[]; removed: number[]; given: Given }) {
  const [pairs, setPairs] = useState(start);
  const change = (at: number, c: Partial<Pair>) => {
    setPairs((was) => was.map((p, i) => (i === at ? { ...p, ...c } : p)));
  };
  return (
    <PairList id="pairs" label="Fields" error={given.whole}>
      {pairs.map((p, i) => (
        <PairField
          key={i}
          legend={`Field ${String(i + 1)}`}
          nameLabel="Label"
          name={p.name}
          onName={(name) => {
            change(i, { name });
          }}
          valueLabel="Content"
          value={p.value}
          onValue={(value) => {
            change(i, { value });
          }}
          maxLength={given.maxLength ?? 255}
          remove={{
            label: `Remove field ${String(i + 1)}`,
            onPress: () => {
              removed.push(i);
            },
          }}
          error={given.refused?.pair === i ? { of: given.refused.of, text: given.refused.text } : undefined}
          isDisabled={given.isDisabled}
        />
      ))}
    </PairList>
  );
}

const written: readonly Pair[] = [
  { name: "Photos", value: "https://yuki.photos.example" },
  { name: "Pronouns", value: "she/her" },
];

function pairs(start: readonly Pair[] = written, given: Given = {}) {
  const removed: number[] = [];
  render(<Pairs start={start} removed={removed} given={given} />);
  return { removed, list: screen.getByRole("list", { name: "Fields" }) };
}

/** The pair whose legend is `legend`. */
const pair = (legend: string) => within(screen.getByRole("group", { name: legend }));
/** The text field named `name` of the pair `legend`. */
const field = (legend: string, name: "Label" | "Content") =>
  pair(legend).getByRole<HTMLInputElement>("textbox", { name });
/** What describes `element`, part by part. */
const describedBy = (element: HTMLElement) =>
  (element.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .filter((id) => id !== "")
    .map((id) => document.getElementById(id)?.textContent ?? "");
/** The text fields of the page that say their value is not valid, each as its pair's legend and its own name. */
const invalid = () =>
  screen.getAllByRole("group").flatMap((group) =>
    within(group)
      .getAllByRole("textbox")
      .filter((input) => input.getAttribute("aria-invalid") === "true")
      .map((input) => `${group.getAttribute("aria-label") ?? ""}: ${input.getAttribute("aria-label") ?? ""}`),
  );

test("the pairs are a list named by its label, each an item with a group named by its legend: two text fields and its Remove", () => {
  const { list } = pairs();

  expect(within(list).getAllByRole("listitem")).toHaveLength(2);
  expect(
    within(list)
      .getAllByRole("group")
      .map((group) => group.getAttribute("aria-label")),
  ).toEqual(["Field 1", "Field 2"]);
  for (const [legend, name, value] of [
    ["Field 1", "Photos", "https://yuki.photos.example"],
    ["Field 2", "Pronouns", "she/her"],
  ] as const) {
    expect(
      pair(legend)
        .getAllByRole<HTMLInputElement>("textbox")
        .map((input) => [input.getAttribute("aria-label"), input.value]),
    ).toEqual([
      ["Label", name],
      ["Content", value],
    ]);
    expect(
      pair(legend)
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label")),
    ).toEqual([`Remove ${legend.toLowerCase()}`]);
  }
  // Nothing is wrong: nothing is marked, and nothing describes the list.
  expect(invalid()).toEqual([]);
  expect(describedBy(list)).toEqual([]);
});

test("a pair not written yet shows the name of each field in its place", () => {
  pairs([{ name: "", value: "" }]);

  expect([field("Field 1", "Label").placeholder, field("Field 1", "Content").placeholder]).toEqual([
    "Label",
    "Content",
  ]);
});

test("what is written in a field is reported for that field of that pair alone", async () => {
  const user = userEvent.setup();
  pairs();

  await user.type(field("Field 2", "Label"), "!");
  await user.type(field("Field 1", "Content"), "/2026");

  expect(
    ["Field 1", "Field 2"].map((legend) => [field(legend, "Label").value, field(legend, "Content").value]),
  ).toEqual([
    ["Photos", "https://yuki.photos.example/2026"],
    ["Pronouns!", "she/her"],
  ]);
});

test("a field takes no more characters than the pairs' limit", async () => {
  const user = userEvent.setup();
  pairs([{ name: "", value: "" }], { maxLength: 5 });

  await user.type(field("Field 1", "Label"), "Pronouns");
  await user.type(field("Field 1", "Content"), "she/her");

  expect([field("Field 1", "Label").value, field("Field 1", "Content").value]).toEqual(["Prono", "she/h"]);
});

test("Remove of a pair reports that pair's press, once", async () => {
  const user = userEvent.setup();
  const { removed } = pairs();

  await user.click(pair("Field 2").getByRole("button", { name: "Remove field 2" }));

  expect(removed).toEqual([1]);
});

for (const [of, name, other] of [
  ["name", "Label", "Content"],
  ["value", "Content", "Label"],
] as const)
  test(`what is wrong with a pair's ${name.toLowerCase()} marks that field alone, and describes it`, () => {
    const { list } = pairs(written, {
      refused: { pair: 1, of, text: "This value is not valid. Check what you entered." },
    });

    expect(invalid()).toEqual([`Field 2: ${name}`]);
    expect(describedBy(field("Field 2", name))).toEqual(["This value is not valid. Check what you entered."]);
    expect(describedBy(field("Field 2", other))).toEqual([]);
    // It is that field's, not the list's.
    expect(describedBy(list)).toEqual([]);
  });

test("what is wrong with the pairs as a whole describes the list, and marks no field", () => {
  const { list } = pairs(written, { whole: "You can have at most 4." });

  expect(describedBy(list)).toEqual(["You can have at most 4."]);
  expect(invalid()).toEqual([]);
});

test("disabled pairs keep what is written, and neither a field nor Remove can be used", async () => {
  const user = userEvent.setup();
  const { removed } = pairs(written, { isDisabled: true });
  const remove = pair("Field 1").getByRole<HTMLButtonElement>("button", { name: "Remove field 1" });

  expect([field("Field 1", "Label").disabled, field("Field 1", "Content").disabled, remove.disabled]).toEqual([
    true,
    true,
    true,
  ]);
  await user.type(field("Field 1", "Label"), "!");
  await user.click(remove);

  expect(field("Field 1", "Label").value).toBe("Photos");
  expect(removed).toEqual([]);
});
