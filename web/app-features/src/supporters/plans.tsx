// SPDX-License-Identifier: AGPL-3.0-only

// The plans of the supporters' page (ADR-0072 §3, §8, §13): side by side under the choice of the period, monthly or
// yearly (what the year saves said once, on Yearly), each its price for the period chosen, under a monthly one the
// yearly price, its description, that it holds the plan under it, and the action the page gives it (Choose, Switch,
// the primary action, which the card names by its plan), the reader's plan at the period they pay marked in its
// place; and the confirmation before an act that charges the reader, which says what
// it does: before the provider's checkout, the price now and at each renewal, when the perks start, what cancelling
// does, and the commerce disclosure where the instance has one; before a change, whether it applies now, the
// difference charged, or at the period's end. Confirmed, it spins until the act is done (for a checkout, until the app
// has left for the provider's page) and says why in its place where the server refused.

import {
  planPrice,
  refusal,
  supporterRefusal,
  yearlySaving,
  type SupporterInterval,
  type SupporterPlan,
} from "@my/app-data";
import {
  Box,
  ConfirmDialog,
  Money as Amount,
  PlanCard,
  PlanList,
  SegmentChoice,
  Stack,
  TextLink,
  type SegmentOption,
} from "@my/design-system";
import { useState, type ReactNode } from "react";
import { useMoney } from "../supporter-words";
import { useSupporterStrings, type T } from "./parts";

/** The plans by rank, the lowest first. */
const byRank = (plans: readonly SupporterPlan[]) => plans.toSorted((a, b) => a.rank - b.rank);

/** What the yearly price saves, as the choice of the period says it: what every plan saves alike, or up to the most. */
function yearlyNote(plans: readonly SupporterPlan[], t: T): string | undefined {
  const percents = plans.map((plan) => yearlySaving(plan)?.percent ?? 0);
  if (percents.length === 0 || percents.some((p) => p === 0)) return undefined;
  const least = Math.min(...percents);
  const most = Math.max(...percents);
  return t("supporters.save", { percent: most, upTo: least === most ? "no" : "yes" });
}

/**
 * The server's plans under the choice of the period: each its name, its price for the period chosen (from `initial`
 * on), under a monthly one the yearly price, its description, that it holds the plan under it, and what
 * `action` gives for it at that period; the reader's plan at the period they pay (`current`) marked in its place.
 */
export function PlanChoice({
  plans,
  current,
  initial,
  action,
}: {
  plans: readonly SupporterPlan[];
  /** The plan and period of the reader's subscription that makes them a member; none without one. */
  current?: { readonly planId: string; readonly interval: SupporterInterval } | undefined;
  initial: SupporterInterval;
  /** What the reader may do with `plan` at `interval` (a `Button` of size `target`); none for nothing. */
  action: (plan: SupporterPlan, interval: SupporterInterval) => ReactNode;
}) {
  const t = useSupporterStrings();
  const money = useMoney();
  const [period, setPeriod] = useState<SupporterInterval>(initial);
  const ranked = byRank(plans);
  const options: readonly SegmentOption<SupporterInterval>[] = [
    { value: "month", label: t("supporters.period", { interval: "month" }) },
    { value: "year", label: t("supporters.period", { interval: "year" }), note: yearlyNote(ranked, t) },
  ];
  return (
    <Stack gap="screen-row-gap">
      <SegmentChoice label={t("supporters.period.label")} options={options} value={period} onChange={setPeriod} />
      <PlanList label={t("supporters.plans")}>
        {ranked.map((plan, i) => {
          const lower = i === 0 ? undefined : ranked[i - 1];
          const yours = current?.planId === plan.id && current.interval === period;
          return (
            <PlanCard
              key={plan.id}
              name={plan.name}
              current={yours ? t("supporters.yours") : undefined}
              price={
                <Amount
                  size="headline"
                  amount={money(planPrice(plan, period))}
                  per={t("supporters.per", { interval: period })}
                />
              }
              // What the year saves is the Yearly segment's to say, once for every plan.
              priceNote={
                period === "month" ? t("supporters.price_note.month", { year: money(plan.yearly_price) }) : undefined
              }
              description={plan.description}
              includes={lower === undefined ? undefined : t("supporters.includes", { plan: lower.name })}
              perks={[]}
              action={yours ? undefined : action(plan, period)}
            />
          );
        })}
      </PlanList>
    </Stack>
  );
}

/** Why the server refused to start a checkout or to change the plan, in one sentence. */
function planRefusal(error: unknown, t: T): string {
  const kind = supporterRefusal(error);
  switch (kind) {
    case "ineligible":
    case "subscribed":
    case "plan-unavailable":
    case "closing":
    case "unavailable":
    case "state":
      return t("supporters.refused", { kind: kind.replace("-", "_") });
    case "payment-failed":
    case "payment-processing":
      // The server says what to do (another card, or wait), in the reader's language.
      return refusal(error) ?? t("supporters.refused", { kind: kind.replace("-", "_") });
    default:
      return t("supporters.refused", { kind: "other" });
  }
}

/**
 * Asks before an act that charges the reader: its question, what it does, and the commerce disclosure where the
 * instance has one (`commerceUrl`). Confirmed, `run` does it, the confirmation spinning until it is done (or, for an
 * act that leaves the app, for ever); refused, it says why in place of what it explained, and the confirmation tries
 * again.
 */
export function ChargeConfirm({
  title,
  confirm,
  cancel,
  commerceUrl,
  run,
  onClose,
  children,
}: {
  title: string;
  confirm: string;
  /** The name of the button that closes it with nothing done, for what it keeps. */
  cancel: string;
  commerceUrl?: string | undefined;
  /** Does the act; it rejects with the server's refusal. */
  run: () => Promise<void>;
  onClose: () => void;
  children: string;
}) {
  const t = useSupporterStrings();
  const [busy, setBusy] = useState(false);
  const [said, setSaid] = useState<string>();
  return (
    <ConfirmDialog
      title={title}
      cancel={cancel}
      confirm={confirm}
      busy={busy}
      onCancel={onClose}
      onConfirm={() => {
        setBusy(true);
        run().then(onClose, (error: unknown) => {
          setBusy(false);
          setSaid(planRefusal(error, t));
        });
      }}
    >
      {said === undefined ? (
        <>
          {children}
          {commerceUrl !== undefined && (
            <>
              {" "}
              <TextLink href={commerceUrl}>{t("supporters.commerce")}</TextLink>
            </>
          )}
        </>
      ) : (
        <Box as="span" role="alert">
          {said}
        </Box>
      )}
    </ConfirmDialog>
  );
}
