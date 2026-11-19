// SPDX-License-Identifier: AGPL-3.0-only

// The parts of the supporters' screens (#3244), as assistive technology gets them: an amount with its period, one
// text; the plans, a list named by its label, each plan an item named by its heading, read from its name to its
// action, each action named by its plan, the reader's marked current where the action would be; a single choice of a
// few segments, a radio group a press and the arrow keys change, its line about it describing it, and when disabled
// out of reach; and a subscription's state, in each kind its sentence, its line and its action, its sign hidden. How
// they stand at a width is their stories' (Supporters).

import { expect, test } from "bun:test";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { Button } from "./button";
import { SegmentChoice } from "./choice";
import { Money, PlanCard, PlanList, SubscriptionState } from "./supporter";

test("an amount and its period read as one text, a space apart", () => {
  const { container } = render(<Money amount="¥1,000" per="/ month" />);

  expect(container.textContent).toBe("¥1,000 / month");
});

test("the plans are a list, each plan an item named by its heading, the reader's marked current", () => {
  render(
    <PlanList label="Plans">
      <PlanCard name="Supporter" price={<Money amount="¥300" per="/ month" />} perks={["A badge"]} />
      <PlanCard
        name="Patron"
        current="Your plan"
        price={<Money amount="¥1,000" per="/ month" />}
        includes="Everything in Supporter, plus:"
        perks={["Create 5 custom emoji a month", "Start voice chat rooms"]}
      />
    </PlanList>,
  );

  const list = screen.getByRole("list", { name: "Plans" });
  const plans = within(list).getAllByRole("listitem", { name: /^(Supporter|Patron)$/ });
  expect(plans).toHaveLength(2);
  expect(plans.map((plan) => plan.getAttribute("aria-current"))).toEqual([null, "true"]);
  expect(screen.getByRole("heading", { name: "Patron" })).toBeTruthy();
  const patron = plans[1] as HTMLElement;
  expect(patron.textContent).toContain("Your plan");
  expect(patron.textContent).toContain("Everything in Supporter, plus:");
  expect(
    within(patron)
      .getAllByRole("listitem")
      .map((perk) => perk.textContent),
  ).toEqual(["Create 5 custom emoji a month", "Start voice chat rooms"]);
});

test("a single choice of segments is a radio group a press and the arrow keys change, described by its line", async () => {
  const user = userEvent.setup();
  const chosen: string[] = [];
  render(
    <SegmentChoice
      label="Billing period"
      options={[
        { value: "month", label: "Monthly" },
        { value: "year", label: "Yearly", note: "Save 10%" },
      ]}
      value="month"
      onChange={(value) => chosen.push(value)}
      about="Switch at any time."
    />,
  );

  const group = screen.getByRole("radiogroup", { name: "Billing period", description: "Switch at any time." });
  expect(
    within(group)
      .getAllByRole("radio")
      .map((radio) => radio.closest("label")?.textContent),
  ).toEqual(["Monthly", "YearlySave 10%"]);
  expect(screen.getByRole("radio", { name: "Monthly" })).toHaveProperty("checked", true);

  await user.click(screen.getByRole("radio", { name: "Yearly Save 10%" }));
  expect(chosen).toEqual(["year"]);
});

/** The words of `element` a screen reader reads, in its order: each text outside what is hidden from it. */
function readOut(element: Element): string[] {
  const words: string[] = [];
  const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
  for (let node = walker.nextNode(); node !== null; node = walker.nextNode()) {
    const text = (node.textContent ?? "").trim();
    if (text !== "" && node.parentElement?.closest('[aria-hidden="true"]') == null) words.push(text);
  }
  return words;
}

test("a plan reads its name, a heading of the third level, then its price, what it is for, its perks, and last its action or the reader's mark", () => {
  render(
    <PlanList label="Plans">
      <PlanCard
        name="Supporter"
        price={<Money amount="¥300" per="/ month" />}
        priceNote="Or ¥3,240 a year, and save 10%"
        description="Keeps the server running."
        perks={["A supporter badge on your profile"]}
        action={<button type="button">Switch to Supporter</button>}
      />
      <PlanCard
        name="Patron"
        current="Your plan"
        price={<Money amount="¥1,000" per="/ month" />}
        priceNote="Or ¥10,800 a year, and save 10%"
        description="Covers a share of the storage."
        includes="Everything in Supporter, plus:"
        perks={["Create 5 custom emoji a month", "Start voice chat rooms"]}
      />
    </PlanList>,
  );

  expect(screen.getAllByRole("heading", { level: 3 }).map((heading) => heading.textContent)).toEqual([
    "Supporter",
    "Patron",
  ]);
  const [supporter, patron] = within(screen.getByRole("list", { name: "Plans" })).getAllByRole("listitem", {
    name: /^(Supporter|Patron)$/,
  });
  expect(readOut(supporter as HTMLElement)).toEqual([
    "Supporter",
    "¥300",
    "/ month",
    "Or ¥3,240 a year, and save 10%",
    "Keeps the server running.",
    "A supporter badge on your profile",
    "Switch to Supporter",
  ]);
  expect(readOut(patron as HTMLElement)).toEqual([
    "Patron",
    "¥1,000",
    "/ month",
    "Or ¥10,800 a year, and save 10%",
    "Covers a share of the storage.",
    "Everything in Supporter, plus:",
    "Create 5 custom emoji a month",
    "Start voice chat rooms",
    "Your plan",
  ]);
  expect(within(patron as HTMLElement).queryAllByRole("button")).toHaveLength(0);
});

test("plans side by side each with a Choose tell their actions apart by name, each beginning with the word shown", () => {
  render(
    <PlanList label="Plans">
      {["Supporter", "Patron", "Benefactor"].map((name) => (
        <PlanCard
          key={name}
          name={name}
          price={<Money amount="¥300" per="/ month" />}
          perks={[]}
          action={<Button onPress={() => undefined}>Choose</Button>}
        />
      ))}
    </PlanList>,
  );

  // One button a plan, named by the word it shows first and then by its plan, so a list of buttons tells them apart.
  expect(
    ["Supporter", "Patron", "Benefactor"].map(
      (name) => screen.queryAllByRole("button", { name: new RegExp(`^Choose\\b.*\\b${name}\\b`) }).length,
    ),
  ).toEqual([1, 1, 1]);
});

test("the segments are reached by Tab on the chosen one, and the arrow keys choose the next", async () => {
  const user = userEvent.setup();
  const chosen: string[] = [];
  function Held() {
    const [value, setValue] = useState("month");
    return (
      <SegmentChoice
        label="Period"
        options={[
          { value: "week", label: "Week" },
          { value: "month", label: "Month" },
          { value: "year", label: "Year" },
        ]}
        value={value}
        onChange={(next) => {
          chosen.push(next);
          setValue(next);
        }}
      />
    );
  }
  render(
    <>
      <button type="button">Before</button>
      <Held />
    </>,
  );

  await user.click(screen.getByRole("button", { name: "Before" }));
  await user.tab();
  expect(document.activeElement === screen.getByRole("radio", { name: "Month" })).toBe(true);
  await user.keyboard("{ArrowRight}{ArrowLeft}{ArrowLeft}");
  expect(chosen).toEqual(["year", "month", "week"]);
  expect(screen.getByRole("radio", { name: "Week" })).toHaveProperty("checked", true);
});

test("a disabled choice takes no press and no focus, and keeps its chosen segment", async () => {
  const user = userEvent.setup();
  const chosen: string[] = [];
  render(
    <>
      <button type="button">Before</button>
      <SegmentChoice
        label="Amount"
        options={[
          { value: "300", label: "¥300" },
          { value: "1000", label: "¥1,000" },
        ]}
        value="1000"
        onChange={(value) => chosen.push(value)}
        isDisabled
        about="A tip is under way in another tab."
      />
      <button type="button">After</button>
    </>,
  );

  await user.click(screen.getByRole("radio", { name: "¥300" }));
  await user.click(screen.getByRole("button", { name: "Before" }));
  await user.tab();
  expect(document.activeElement === screen.getByRole("button", { name: "After" })).toBe(true);
  expect(chosen).toEqual([]);
  expect(screen.getByRole("radio", { name: "¥1,000" })).toHaveProperty("checked", true);
  expect(
    screen.getByRole("radiogroup", { name: "Amount", description: "A tip is under way in another tab." }),
  ).toBeTruthy();
});

test("each state of a subscription reads as its words alone, its sign hidden, its action after them", () => {
  for (const state of ["renewing", "ending", "changing", "failed"] as const) {
    cleanup();
    const { container } = render(
      <SubscriptionState
        state={state}
        detail="Patron, monthly · ¥1,000 / month"
        action={<button type="button">Do it</button>}
      >
        {`The sentence of ${state}`}
      </SubscriptionState>,
    );
    expect(readOut(container)).toEqual([`The sentence of ${state}`, "Patron, monthly · ¥1,000 / month", "Do it"]);
  }

  cleanup();
  const { container } = render(<SubscriptionState state="renewing">Renews on Dec 1, 2026</SubscriptionState>);
  expect(readOut(container)).toEqual(["Renews on Dec 1, 2026"]);
  expect(screen.queryAllByRole("button")).toHaveLength(0);
});

test("no segment may be chosen while the choice is disabled", () => {
  render(
    <SegmentChoice
      label="Amount"
      options={[
        { value: "300", label: "¥300" },
        { value: "500", label: "¥500" },
      ]}
      value="500"
      onChange={() => undefined}
      isDisabled
    />,
  );

  expect(screen.getAllByRole("radio").map((radio) => (radio as HTMLInputElement).disabled)).toEqual([true, true]);
});

test("a subscription's state is its sentence, a line under it, and the one thing to do about it", () => {
  render(
    <SubscriptionState
      state="ending"
      detail="Your perks stay until then."
      action={<button type="button">Resume</button>}
    >
      Ends on Dec 1, 2026
    </SubscriptionState>,
  );

  expect(screen.getByText("Ends on Dec 1, 2026")).toBeTruthy();
  expect(screen.getByText("Your perks stay until then.")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Resume" })).toBeTruthy();
});

test("the reader's plan shows its mark and no action, whatever action is given", () => {
  render(
    <PlanList label="Plans">
      <PlanCard
        name="Patron"
        current="Your plan"
        price={<Money amount="¥1,000" per="/ month" />}
        perks={[]}
        action={<button type="button">Switch</button>}
      />
    </PlanList>,
  );

  const plan = screen.getByRole("listitem", { name: "Patron" });
  expect(plan.textContent).toContain("Your plan");
  expect(within(plan).queryByRole("button")).toBeNull();
});
