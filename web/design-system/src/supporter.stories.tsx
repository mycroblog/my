// SPDX-License-Identifier: AGPL-3.0-only

// The parts of the supporters' screens (ADR-0072, ADR-0162, FR-910–FR-919), each in every state, and the supporter's
// screen they make together: an amount of money, with and without its period, at a body's size and at a price's; the
// choice of the period, monthly or yearly with what yearly saves; the plans side by side for a reader with no plan,
// paid monthly, and for one with a plan, paid yearly, their plan marked; one plan alone, with a long name and
// description; the subscription's state, renewing, ending after a cancel, a change scheduled and its payment failed;
// and the tip's amounts, one, three and six of them, none chosen, one chosen, and closed. The names and the amounts
// are samples in English, formatted as the app would.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { Button } from "./button";
import { SegmentChoice, type SegmentOption } from "./choice";
import { Row, Stack } from "./layout";
import { GroupHeading } from "./list";
import { Money, PlanCard, PlanList, SubscriptionState } from "./supporter";

const rem = (px: number) => `${px / rootPx}rem`;
const none = () => undefined;

/** The column of a screen: as wide as the app's main column at most, the frame's edge around it. */
function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
}

type Period = "month" | "year";

const periods: readonly SegmentOption<Period>[] = [
  { value: "month", label: "Monthly" },
  { value: "year", label: "Yearly", note: "Save 10%" },
];

/** The choice of the period, held here as the screen would hold it. */
function PeriodChoice({ value, onChange }: { value: Period; onChange: (period: Period) => void }) {
  return <SegmentChoice label="Billing period" options={periods} value={value} onChange={onChange} />;
}

type Plan = {
  name: string;
  description: string;
  month: string;
  year: string;
  yearPerMonth: string;
  includes?: string;
  perks: readonly string[];
};

const plans: readonly Plan[] = [
  {
    name: "Supporter",
    description: "Keeps the server running.",
    month: "¥300",
    year: "¥3,240",
    yearPerMonth: "¥270",
    perks: ["A supporter badge on your profile", "Listed among the supporters, if you choose"],
  },
  {
    name: "Patron",
    description: "Covers a share of the storage, and of the moderators' tools.",
    month: "¥1,000",
    year: "¥10,800",
    yearPerMonth: "¥900",
    includes: "Everything in Supporter, plus:",
    perks: ["Create 5 custom emoji a month", "Start voice chat rooms"],
  },
  {
    name: "Benefactor",
    description: "For those who want the server to grow.\nThank you.",
    month: "¥3,000",
    year: "¥32,400",
    yearPerMonth: "¥2,700",
    includes: "Everything in Patron, plus:",
    perks: ["Create 20 custom emoji a month", "Read the server's finance totals", "Write articles"],
  },
];

/** The plans for the period chosen; `current`, the reader's plan, by name. */
function Plans({ period, current }: { period: Period; current?: string }) {
  return (
    <PlanList label="Plans">
      {plans.map((plan) => (
        <PlanCard
          key={plan.name}
          name={plan.name}
          current={plan.name === current ? "Your plan" : undefined}
          price={
            <Money
              size="headline"
              amount={period === "month" ? plan.month : plan.year}
              per={period === "month" ? "/ month" : "/ year"}
            />
          }
          priceNote={period === "month" ? `Or ${plan.year} a year` : `${plan.yearPerMonth} a month, billed yearly`}
          description={plan.description}
          includes={plan.includes}
          perks={plan.perks}
          action={
            plan.name === current ? undefined : (
              <Button kind="primary" size="target" onPress={none}>
                {current === undefined ? "Choose" : "Switch"}
              </Button>
            )
          }
        />
      ))}
    </PlanList>
  );
}

/** The period's choice over the plans, which follow it. */
function PlansChosen({ initial, current }: { initial: Period; current?: string }) {
  const [period, setPeriod] = useState<Period>(initial);
  return (
    <Stack gap="screen-row-gap">
      <PeriodChoice value={period} onChange={setPeriod} />
      <Plans period={period} {...(current === undefined ? {} : { current })} />
    </Stack>
  );
}

const yen = (amounts: readonly string[]) =>
  amounts.map((amount): SegmentOption<string> => ({ value: amount, label: amount }));

/** The tip's amounts, held here as the screen would hold them. */
function TipAmounts({
  amounts,
  initial = null,
  isDisabled,
  about,
}: {
  amounts: readonly string[];
  initial?: string | null;
  isDisabled?: boolean;
  about?: string;
}) {
  const [value, setValue] = useState<string | null>(initial);
  return (
    <SegmentChoice
      label="Amount"
      options={yen(amounts)}
      value={value}
      onChange={setValue}
      isDisabled={isDisabled}
      about={about}
    />
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Supporters", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/**
 * The supporter's screen, composed of the parts: the reader's subscription and its state; the period's choice over
 * the plans, theirs marked; and a tip, its amounts and its action.
 */
export const SupporterScreen: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-room">
        <Stack gap="entry-line">
          <GroupHeading>Your subscription</GroupHeading>
          <SubscriptionState state="renewing">Renews on Dec 1, 2026</SubscriptionState>
        </Stack>
        <Stack gap="entry-line">
          <GroupHeading>Plans</GroupHeading>
          <PlansChosen initial="year" current="Patron" />
        </Stack>
        <Stack gap="entry-line">
          <GroupHeading>Tip the server once</GroupHeading>
          <Stack gap="screen-row-gap">
            <TipAmounts
              amounts={["¥300", "¥500", "¥1,000", "¥3,000"]}
              initial="¥500"
              about="A thank-you to the server, not a donation: no perks and no role."
            />
            <Row>
              <Button kind="primary" size="target" onPress={none}>
                Tip ¥500
              </Button>
            </Row>
          </Stack>
        </Stack>
      </Stack>
    </Column>
  ),
};

/** The plans for a reader with no plan, monthly: each its price a month, what a year costs, and its action. */
export const PlansMonthly: Story = {
  render: () => (
    <Column>
      <PlansChosen initial="month" />
    </Column>
  ),
};

/**
 * The plans for a reader who has one, yearly: each its price a year and what that is a month, the saving said once,
 * in the Yearly segment; the reader's plan marked, neutral, in the place of an action, the others' a switch.
 */
export const PlansYearlyWithYours: Story = {
  render: () => (
    <Column>
      <PlansChosen initial="year" current="Patron" />
    </Column>
  ),
};

/** One plan alone, as wide as the column; a long name and a description of lines that wrap; no perks of its own. */
export const PlanAlone: Story = {
  render: () => (
    <Column>
      <PlanList label="Plans">
        <PlanCard
          name="The supporters of a small server by the sea, who keep its lights on"
          price={<Money size="headline" amount="US$4.50" per="/ month" />}
          priceNote="Or US$48.60 a year"
          description={
            "Every month the server pays for its machine, its storage and the backups of both.\nWhat is left goes to the next year's domain."
          }
          perks={[]}
          action={
            <Button kind="primary" size="target" onPress={none}>
              Choose
            </Button>
          }
        />
      </PlanList>
    </Column>
  ),
};

/**
 * The subscription's state, one line each: renewing on a date; ending on a date after a cancel, with Resume; a change
 * of plan scheduled for the period's end, with Keep my plan; its payment failed, in its grace, with Update the card.
 */
export const SubscriptionStates: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-room">
        <SubscriptionState state="renewing" detail="Patron, monthly · ¥1,000 / month">
          Renews on Dec 1, 2026
        </SubscriptionState>
        <SubscriptionState
          state="ending"
          detail="Patron, monthly · Your perks stay until then."
          action={
            <Button size="target" onPress={none}>
              Resume
            </Button>
          }
        >
          Ends on Dec 1, 2026
        </SubscriptionState>
        <SubscriptionState
          state="changing"
          detail="Patron, yearly, until then · ¥1,000 / month after"
          action={
            <Button size="target" onPress={none}>
              Keep my plan
            </Button>
          }
        >
          Changes to Patron, monthly, on Dec 1, 2026
        </SubscriptionState>
        <SubscriptionState
          state="failed"
          detail="The card ending in 4242 was declined on Nov 19."
          action={
            <Button size="target" kind="primary" onPress={none}>
              Update the card
            </Button>
          }
        >
          Your payment failed: your perks end on Nov 26, 2026 unless it goes through
        </SubscriptionState>
      </Stack>
    </Column>
  ),
};

/**
 * The tip's amounts: one; three, none chosen yet; six, which a phone puts three to a row, one chosen; and closed
 * while a tip is under way, the chosen one kept, the note saying why.
 */
export const TipAmountChoice: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-room">
        <TipAmounts amounts={["¥500"]} initial="¥500" />
        <TipAmounts amounts={["¥300", "¥1,000", "¥5,000"]} />
        <TipAmounts amounts={["¥100", "¥300", "¥500", "¥1,000", "¥10,000", "¥100,000"]} initial="¥1,000" />
        <TipAmounts
          amounts={["¥300", "¥1,000", "¥5,000"]}
          initial="¥1,000"
          isDisabled
          about="A tip is under way in another tab. Finish or cancel it there first."
        />
      </Stack>
    </Column>
  ),
};

/** The period's choice alone: monthly chosen, and yearly, with what it saves under its word. */
export const PeriodChoices: Story = {
  render: () => {
    function Held({ initial }: { initial: Period }) {
      const [period, setPeriod] = useState<Period>(initial);
      return <PeriodChoice value={period} onChange={setPeriod} />;
    }
    return (
      <Column>
        <Stack gap="entry-room">
          <Held initial="month" />
          <Held initial="year" />
        </Stack>
      </Column>
    );
  },
};

/** An amount: alone, with its period, and as a plan's price; in yen and in dollars. */
export const Amounts: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-line">
        <Money amount="¥500" />
        <Money amount="¥1,000" per="/ month" />
        <Money amount="US$48.60" per="/ year" />
        <Money size="headline" amount="¥10,800" per="/ year" />
      </Stack>
    </Column>
  ),
};
