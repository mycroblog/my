// SPDX-License-Identifier: AGPL-3.0-only

// The parts of the supporters' screens (ADR-0072, ADR-0162, FR-910–FR-919): an amount of money with the period it is
// paid for; the server's plans side by side, each its name, its price, its description and its perks, a higher plan
// saying that it holds the lower one's, the reader's plan marked; and the state of the reader's subscription, a line
// of its glyph and a sentence. A choice of the period (monthly, yearly) or of a tip's amount is a `SegmentChoice`
// (choice.tsx).
//
// The app formats every amount (`Intl.NumberFormat` in the plan's currency, ADR-0067 §10) and every date, and gives
// every word: these parts lay them out. A plan's name and description are the operator's, in no locale (`dir="auto"`).
// Everything is in the text's color on the page's surface, with no hue: a plan stands on the raised surface; the
// reader's plan is marked by a glyph and its words, never a tile; a state the reader must act on (a failed payment)
// is in the strong weight, the others in the body's.

import { useId, type ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { raisedSurface } from "./button";
import { Icon, type IconName } from "./icons";
import { Box, Column, Row, Stack } from "./layout";

const money = tv({
  slots: {
    amount: "tabular-nums whitespace-nowrap",
    per: "font-normal text-muted-text whitespace-nowrap",
  },
  variants: {
    size: {
      body: { amount: "text-body", per: "text-body" },
      // A plan's price, the largest figure of its card.
      headline: { amount: "text-headline", per: "text-body" },
    },
  },
});

/**
 * An amount of money, as the app formats it in its currency ("¥500", "US$4.50"), then, muted, the period it is paid
 * for ("/ month"), the two never parted by a line's end.
 */
export function Money({
  amount,
  per,
  size = "body",
}: {
  amount: string;
  /** The period, as the app words it ("/ month"); none for an amount paid once. */
  per?: string | undefined;
  size?: "body" | "headline";
}) {
  const s = money({ size });
  return (
    <span>
      <span className={s.amount()}>{amount}</span>
      {per !== undefined && (
        <>
          {" "}
          <span className={s.per()}>{per}</span>
        </>
      )}
    </span>
  );
}

/**
 * The server's plans side by side, named `label` ("Plans"), in the order of their rank, the lowest first: a list of
 * `PlanCard`s, as many alike to a row as fit, each at least a narrow column wide, so that a phone stacks them and a
 * tablet's column holds three, the cards of a row as tall as the tallest. Put it on the page's surface, not inside a
 * raised section, since each plan is a raised surface.
 */
export function PlanList({ label, children }: { label: string; children: ReactNode }) {
  return (
    <ul
      aria-label={label}
      className="m-0 grid list-none grid-cols-[repeat(auto-fit,minmax(min(100%,11rem),1fr))] gap-entry-caption p-0"
    >
      {children}
    </ul>
  );
}

const plan = tv({
  slots: {
    root: "box-border rounded-overlay bg-surface-raised p-shell-edge text-text",
    name: "text-title [word-break:auto-phrase]",
    // As tall as the action it stands in the place of, so that the cards of a row keep their prices and their feet in
    // line.
    current: "min-h-icon-button-touch text-body font-label",
    note: "text-caption text-muted-text [word-break:auto-phrase]",
    description: "text-body text-muted-text whitespace-pre-line [overflow-wrap:anywhere]",
    includes: "text-caption font-label [word-break:auto-phrase]",
    perk: "text-body [word-break:auto-phrase]",
    glyph: "text-muted-text",
  },
});

export type PlanCardProps = {
  /** The plan's name, the operator's. */
  name: string;
  /** The words that mark the reader's own plan ("Your plan"); none on the others. */
  current?: string | undefined;
  /** Its price for the period chosen (a `Money` at `headline`). */
  price: ReactNode;
  /**
   * A line under the price: the other period's, or what the yearly price saves ("¥450 / month, billed yearly · Save
   * 10%").
   */
  priceNote?: string | undefined;
  /** The plan's description, the operator's plain text, its line breaks kept. */
  description?: string | undefined;
  /** For a plan above the lowest, that it holds the perks of the one under it ("Everything in Supporter, plus:"). */
  includes?: string | undefined;
  /** The perks of its own, a line each, as the app words them ("Create 10 custom emoji a month"). */
  perks: readonly string[];
  /** What the reader may do with it (a `Button` of size `target`: Subscribe, Switch to this plan), at the card's foot. */
  action?: ReactNode;
};

/**
 * A plan, a raised card in a `PlanList`: its name, a heading; its price for the period chosen, and a line under it;
 * its description; that it holds the perks of the plan under it, then its own, a check before each; and at its foot
 * its action, the actions of a row of cards on one line, or, on the reader's plan, its mark, a glyph and its words,
 * in the place of an action, so that the prices of a row stay in line too.
 */
export function PlanCard({ name, current, price, priceNote, description, includes, perks, action }: PlanCardProps) {
  const s = plan();
  const id = useId();
  return (
    <Column
      as={<li aria-labelledby={id} aria-current={current === undefined ? undefined : true} {...raisedSurface} />}
      gap="screen-row-gap"
      className={s.root()}
    >
      <h3 id={id} className={s.name()} dir="auto">
        {name}
      </h3>
      <Stack gap="hair">
        {price}
        {priceNote !== undefined && <span className={s.note()}>{priceNote}</span>}
      </Stack>
      {description !== undefined && description !== "" && (
        <p className={s.description()} dir="auto">
          {description}
        </p>
      )}
      {(includes !== undefined || perks.length > 0) && (
        <Stack gap="entry-line">
          {includes !== undefined && <p className={s.includes()}>{includes}</p>}
          {perks.length > 0 && (
            <Stack as="ul" gap="entry-line" plain>
              {perks.map((perk) => (
                <Row key={perk} as="li" align="start" gap="entry-caption" className={s.perk()}>
                  <Row as="span" inline line rigid align="center" className={s.glyph()}>
                    <Icon name="chosen" place="caption" />
                  </Row>
                  <Box as="span" grow>
                    {perk}
                  </Box>
                </Row>
              ))}
            </Stack>
          )}
        </Stack>
      )}
      {(action !== undefined || current !== undefined) && (
        <Stack push="bottom" gap="entry-line">
          {current !== undefined && (
            <Row align="center" justify="center" gap="entry-caption" className={s.current()}>
              <Icon name="accepted" place="action" />
              <span>{current}</span>
            </Row>
          )}
          {action}
        </Stack>
      )}
    </Column>
  );
}

/**
 * The states of a subscription a line says (ADR-0072 §7, §8): active and renewing; ending at its period's end after a
 * cancel; a change of plan or of period scheduled for its period's end; its payment failed, in its grace.
 */
export type SubscriptionStateKind = "renewing" | "ending" | "changing" | "failed";

const stateIcon: Record<SubscriptionStateKind, IconName> = {
  renewing: "update",
  ending: "supportEnded",
  changing: "schedule",
  failed: "paymentFailed",
};

const state = tv({
  slots: {
    glyph: "text-text",
    sentence: "text-body text-text [word-break:auto-phrase]",
    detail: "text-caption text-muted-text [word-break:auto-phrase]",
  },
  variants: {
    act: { true: { sentence: "font-strong" }, false: {} },
  },
});

/**
 * The state of the reader's subscription, a line: the state's glyph, then the sentence the app words with its date
 * ("Renews on Dec 1, 2026", "Ends on Dec 1, 2026", "Changes to Patron, yearly, on Dec 1, 2026", "Your payment failed:
 * your perks end on Nov 26 unless it goes through"), in the strong weight where the reader must act (a failed payment);
 * a line under it, muted (the plan and period, the price kept from before); and the one thing to do about it at the
 * line's end (Resume, Keep my plan, Update the card), under it where the line has no room.
 */
export function SubscriptionState({
  state: kind,
  children,
  detail,
  action,
}: {
  state: SubscriptionStateKind;
  /** The sentence, as the app words it with its date. */
  children: string;
  detail?: string | undefined;
  /** What the reader may do about it (a `Button` of size `target`). */
  action?: ReactNode;
}) {
  const s = state({ act: kind === "failed" });
  return (
    <Row align="start" gap="entry-caption">
      <Row as="span" inline line rigid align="center" className={s.glyph()}>
        <Icon name={stateIcon[kind]} place="action" />
      </Row>
      {/* The action wraps under the words, from where they start, not under the glyph. */}
      <Row grow wrap align="center" gapX="entry-avatar" gap="entry-line">
        <Stack grow="names" gap="hair">
          <p className={s.sentence()}>{children}</p>
          {detail !== undefined && <p className={s.detail()}>{detail}</p>}
        </Stack>
        {action !== undefined && (
          <Row inline rigid>
            {action}
          </Row>
        )}
      </Row>
    </Row>
  );
}
