// SPDX-License-Identifier: AGPL-3.0-only

// The choice from a list and the number of a settings screen (#3198, ADR-0110 §5): what each is to assistive
// technology, what it reports to its owner and when, and what a form posts of it. What a setting's row gives the
// field inside it (its name, its description, its disabled state) is the row's test (settings.dom.test.tsx).

import { expect, test } from "bun:test";
import { isInaccessible, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NumberField, Select, type NumberFieldProps } from "./field";

const zones = [
  { value: "UTC", label: "UTC" },
  {
    label: "Asia",
    options: [
      { value: "Asia/Tokyo", label: "Tokyo (UTC+9)" },
      { value: "Asia/Seoul", label: "Seoul (UTC+9)" },
    ],
  },
  { label: "Europe", options: [{ value: "Europe/Lisbon", label: "Lisbon (UTC+0)" }] },
];

/** The words of the option the select is at: the one option it says is selected. */
const chosen = (select: HTMLSelectElement) =>
  within(select)
    .getAllByRole("option", { selected: true })
    .map((option) => option.textContent)
    .join(" + ");

/** The words the field draws for the eye alone, over the platform's select. */
const drawn = (words: string) => screen.queryAllByText(words).filter((element) => isInaccessible(element));

test("a select is the platform's select named by its label, with its options, plain and in groups", () => {
  render(<Select label="Time zone" options={zones} />);

  const select = screen.getByRole<HTMLSelectElement>("combobox", { name: "Time zone" });
  expect(
    within(select)
      .getAllByRole<HTMLOptionElement>("option")
      .map((option) => [option.value, option.textContent]),
  ).toEqual([
    ["UTC", "UTC"],
    ["Asia/Tokyo", "Tokyo (UTC+9)"],
    ["Asia/Seoul", "Seoul (UTC+9)"],
    ["Europe/Lisbon", "Lisbon (UTC+0)"],
  ]);
  const asia = within(select).getByRole("group", { name: "Asia" });
  expect(
    within(asia)
      .getAllByRole("option")
      .map((option) => option.textContent),
  ).toEqual(["Tokyo (UTC+9)", "Seoul (UTC+9)"]);
  // With no value given it is at its first option, as the platform's select is.
  expect(chosen(select)).toBe("UTC");
  expect(drawn("UTC")).toHaveLength(1);
});

test("choosing an option reports its value once, and the select and the field drawn over it show it", async () => {
  const user = userEvent.setup();
  const changes: string[] = [];
  render(
    <Select label="Time zone" options={zones} defaultValue="Asia/Seoul" onChange={(value) => changes.push(value)} />,
  );
  const select = screen.getByRole<HTMLSelectElement>("combobox", { name: "Time zone" });
  expect(chosen(select)).toBe("Seoul (UTC+9)");
  expect(drawn("Seoul (UTC+9)")).toHaveLength(1);

  await user.selectOptions(select, "Lisbon (UTC+0)");

  expect(changes).toEqual(["Europe/Lisbon"]);
  expect(chosen(select)).toBe("Lisbon (UTC+0)");
  expect(drawn("Lisbon (UTC+0)")).toHaveLength(1);
  expect(drawn("Seoul (UTC+9)")).toHaveLength(0);
});

test("a select whose owner keeps its value asks for the change and stays as the owner says", async () => {
  const user = userEvent.setup();
  const changes: string[] = [];
  const at = (value: string) => (
    <Select label="Time zone" options={zones} value={value} onChange={(next) => changes.push(next)} />
  );
  const { rerender } = render(at("UTC"));
  const select = screen.getByRole<HTMLSelectElement>("combobox", { name: "Time zone" });

  await user.selectOptions(select, "Tokyo (UTC+9)");

  expect(changes).toEqual(["Asia/Tokyo"]);
  expect(chosen(select)).toBe("UTC");
  expect(drawn("UTC")).toHaveLength(1);

  rerender(at("Asia/Tokyo"));

  expect(chosen(select)).toBe("Tokyo (UTC+9)");
  expect(drawn("Tokyo (UTC+9)")).toHaveLength(1);
});

test("a select is named and described by the elements it is told of", () => {
  render(
    <>
      <span id="languages">Languages</span>
      <span id="languages-about">Posts in other languages stay out of the timelines</span>
      <Select labelledBy="languages" describedBy="languages-about" options={zones} />
    </>,
  );

  expect(
    screen.getByRole("combobox", {
      name: "Languages",
      description: "Posts in other languages stay out of the timelines",
    }),
  ).not.toBeNull();
});

test("a select in a form posts the value it is at under its name", async () => {
  const user = userEvent.setup();
  render(
    <form aria-label="Filter">
      <Select label="Time zone" name="time_zone" options={zones} defaultValue="Asia/Tokyo" />
    </form>,
  );
  const posted = () => [...new FormData(screen.getByRole<HTMLFormElement>("form", { name: "Filter" })).entries()];
  expect(posted()).toEqual([["time_zone", "Asia/Tokyo"]]);

  await user.selectOptions(screen.getByRole("combobox", { name: "Time zone" }), "UTC");

  expect(posted()).toEqual([["time_zone", "UTC"]]);
});

test("a disabled select is disabled, and reports no choice", async () => {
  const user = userEvent.setup();
  const changes: string[] = [];
  render(<Select label="Time zone" options={zones} isDisabled onChange={(value) => changes.push(value)} />);
  const select = screen.getByRole<HTMLSelectElement>("combobox", { name: "Time zone" });
  expect(select.disabled).toBe(true);

  await user.selectOptions(select, "Tokyo (UTC+9)");

  expect(changes).toEqual([]);
  expect(chosen(select)).toBe("UTC");
});

/** A number field of 1 to 365 days with a button after it, and what it reported, in order. */
function days(props: Partial<NumberFieldProps> = {}) {
  const reported: (number | null)[] = [];
  render(
    <>
      <NumberField
        label="Ends after"
        min={1}
        max={365}
        unit="days"
        onChange={(value) => reported.push(value)}
        {...props}
      />
      <button type="button">After</button>
    </>,
  );
  return { reported, field: screen.getByRole<HTMLInputElement>("textbox", { name: "Ends after" }) };
}

test("a number field is a text field named by its label, at the number it is given, its unit beside it", () => {
  const { field, reported } = days({ defaultValue: 30 });

  expect(field.value).toBe("30");
  expect(screen.getByText("days")).not.toBeNull();
  expect(reported).toEqual([]);
});

test("a number typed in reports nothing key by key, and once when the focus leaves the field", async () => {
  const user = userEvent.setup();
  const { field, reported } = days();

  await user.click(field);
  await user.keyboard("12");
  expect(field.value).toBe("12");
  expect(reported).toEqual([]);
  await user.tab();

  expect(reported).toEqual([12]);
  // Coming back and leaving with nothing changed reports nothing more.
  await user.tab({ shift: true });
  await user.tab();
  expect(reported).toEqual([12]);
});

test("Enter in a number field reports the number once, and leaving the field after it nothing more", async () => {
  const user = userEvent.setup();
  const { field, reported } = days({ defaultValue: 30 });

  await user.clear(field);
  await user.keyboard("7{Enter}");

  expect(reported).toEqual([7]);
  expect(field.value).toBe("7");
  await user.tab();
  expect(reported).toEqual([7]);
});

for (const [typed, bound] of [
  ["400", 365],
  ["0", 1],
] as const)
  test(`${typed} entered where 1 to 365 is taken becomes ${String(bound)}, the nearest the field takes`, async () => {
    const user = userEvent.setup();
    const { field, reported } = days({ defaultValue: 30 });

    await user.clear(field);
    await user.keyboard(typed);
    await user.tab();

    expect(field.value).toBe(String(bound));
    expect(reported).toEqual([bound]);
  });

test("a number field given a number outside what it takes shows the nearest it takes", () => {
  expect(days({ defaultValue: 400 }).field.value).toBe("365");
});

test("a number field takes whole numbers: no letter and no point is written", async () => {
  const user = userEvent.setup();
  const { field, reported } = days();

  await user.click(field);
  await user.keyboard("1a2");
  expect(field.value).toBe("12");
  await user.keyboard(".5");
  expect(field.value).toBe("125");
  await user.tab();

  expect(reported).toEqual([125]);
});

test("a number field emptied reports no number", async () => {
  const user = userEvent.setup();
  const { field, reported } = days({ defaultValue: 30 });

  await user.clear(field);
  await user.tab();

  expect(reported).toEqual([null]);
  expect(field.value).toBe("");
});

test("the arrow keys step a number field by one, Home and End go to the least and the most it takes", async () => {
  const user = userEvent.setup();
  const { field, reported } = days({ defaultValue: 30 });

  await user.click(field);
  await user.keyboard("{ArrowUp}");
  expect(field.value).toBe("31");
  await user.keyboard("{ArrowDown}{ArrowDown}");
  expect(field.value).toBe("29");
  await user.keyboard("{Home}");
  expect(field.value).toBe("1");
  await user.keyboard("{ArrowDown}");
  expect(field.value).toBe("1");
  await user.keyboard("{End}");
  expect(field.value).toBe("365");
  await user.keyboard("{ArrowUp}");
  expect(field.value).toBe("365");
  // What the keys left is reported as a number typed in is: once, when the focus leaves.
  expect(reported).toEqual([]);
  await user.tab();

  expect(reported).toEqual([365]);
});

test("a number field in a form posts its number under its name", async () => {
  const user = userEvent.setup();
  render(
    <form aria-label="Filter">
      <NumberField label="Ends after" name="days" defaultValue={30} min={1} />
      <button type="button">After</button>
    </form>,
  );
  const posted = () => [...new FormData(screen.getByRole<HTMLFormElement>("form", { name: "Filter" })).entries()];
  expect(posted()).toEqual([["days", "30"]]);

  await user.clear(screen.getByRole("textbox", { name: "Ends after" }));
  await user.keyboard("45");
  await user.tab();

  expect(posted()).toEqual([["days", "45"]]);
});

test("a number field whose owner keeps its number asks for the change, and shows the owner's number when it changes", async () => {
  const user = userEvent.setup();
  const reported: (number | null)[] = [];
  const at = (value: number | null) => (
    <>
      <NumberField label="Ends after" min={1} value={value} onChange={(next) => reported.push(next)} />
      <button type="button">After</button>
    </>
  );
  const { rerender } = render(at(30));
  const field = screen.getByRole<HTMLInputElement>("textbox", { name: "Ends after" });

  await user.clear(field);
  await user.keyboard("45");
  await user.tab();

  expect(reported).toEqual([45]);

  // The owner answers with a number of its own, then with none: the field shows each, and reports neither.
  rerender(at(60));
  expect(field.value).toBe("60");
  rerender(at(null));
  expect(field.value).toBe("");
  rerender(at(30));
  expect(field.value).toBe("30");
  expect(reported).toEqual([45]);
});

test("a disabled number field is disabled", () => {
  const { field } = days({ defaultValue: 30, isDisabled: true });

  expect(field.disabled).toBe(true);
});
