// SPDX-License-Identifier: AGPL-3.0-only

// The parts of the supporters' screens (#3244), as assistive technology gets them: an amount with its period, one
// text; the plans, a list named by its label, each plan an item named by its heading, the reader's marked current; a
// single choice of a few segments, a radio group a press and the arrow keys change, its line about it describing it;
// and a subscription's state, its sentence and its action. How they stand at a width is their stories' (Supporters).

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
