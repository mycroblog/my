// SPDX-License-Identifier: AGPL-3.0-only
// The subscriptions page of the supporters screen (ADR-0072, manage_supporters;
// web-client/screens/admin-supporters-subscriptions.md): the subscriptions, newest first, as a paginated table
// narrowed to a plan and a state (from the URL), in four columns: the account (which leads to the subscription's page,
// where it is ended at once), the plan, the state and the end of the period paid for. The plans' names are the
// administrators' text, shown with dir="auto".
import { EmptyState, Link, ListToolbar, Select, SelectItem, Stack } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { DateTime, type RecordColumns, RecordTable } from "../../moderation";
import {
  type SupporterSubscription,
  type SupporterSubscriptionFilter,
  supporterSubscriptionStates,
} from "../api/supporters";
import { useSupporterPlans, useSupporterSubscriptions } from "../hooks/useSupporters";
import { SupporterPages, subscriberName, subscriptionStateLabels } from "./supporterParts";

/** The value of the "every" choice of the filters. */
const anyChoice = "any";

type SupporterSubscriptionsProps = {
  /** The subscriptions shown, from the URL (web-client/README.md S3). */
  filter: SupporterSubscriptionFilter;
  onFilterChange: (filter: SupporterSubscriptionFilter) => void;
};

export function SupporterSubscriptions({ filter, onFilterChange }: SupporterSubscriptionsProps) {
  return (
    <SupporterPages current="subscriptions">
      <Subscriptions filter={filter} onFilterChange={onFilterChange} />
    </SupporterPages>
  );
}

function Subscriptions({ filter, onFilterChange }: SupporterSubscriptionsProps) {
  const t = useT();
  const plans = useSupporterPlans();
  const list = useSupporterSubscriptions(filter);
  const columns: RecordColumns<SupporterSubscription> = [
    {
      id: "account",
      header: t("client:admin.supporters.subscription.account"),
      wrap: "anywhere",
      cell: (subscription) => (
        <Link href={`/admin/supporters/subscriptions/${encodeURIComponent(subscription.id)}`} variant="standalone">
          <span dir="ltr">{subscriberName(t, subscription)}</span>
        </Link>
      ),
    },
    {
      id: "plan",
      header: t("client:admin.supporters.subscription.plan"),
      wrap: "anywhere",
      cell: (subscription) => (
        <span dir="auto" lang="">
          {subscription.plan_name}
        </span>
      ),
    },
    {
      id: "state",
      header: t("client:admin.supporters.subscription.state"),
      cell: (subscription) => t(subscriptionStateLabels[subscription.state]),
    },
    {
      id: "period_ends_at",
      header: t("client:admin.supporters.subscription.period_ends_at"),
      cell: (subscription) =>
        subscription.period_ends_at === undefined ? (
          t("client:admin.supporters.subscription.no_period")
        ) : (
          <DateTime value={subscription.period_ends_at} />
        ),
    },
  ];
  const planChoices = [
    { id: anyChoice, name: t("client:admin.supporters.filter.any_plan") },
    ...plans.map((plan) => ({ id: plan.id, name: plan.name })),
  ];
  const states = [
    { id: anyChoice, name: t("client:admin.supporters.filter.any_state") },
    ...supporterSubscriptionStates.map((state) => ({ id: state, name: t(subscriptionStateLabels[state]) })),
  ];
  return (
    <Stack gap="4">
      <ListToolbar label={t("client:admin.list.filters")}>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.supporters.filter.plan")}
          value={filter.planId ?? anyChoice}
          onChange={(key) => {
            onFilterChange({ ...filter, planId: key === anyChoice || typeof key !== "string" ? undefined : key });
          }}
          items={planChoices}
        >
          {(item) => (
            <SelectItem id={item.id} textValue={item.name}>
              <span dir="auto">{item.name}</span>
            </SelectItem>
          )}
        </Select>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.supporters.filter.state")}
          value={filter.state ?? anyChoice}
          onChange={(key) => {
            const state = supporterSubscriptionStates.find((s) => s === key);
            onFilterChange({ ...filter, state });
          }}
          items={states}
        >
          {(item) => (
            <SelectItem id={item.id} textValue={item.name}>
              {item.name}
            </SelectItem>
          )}
        </Select>
      </ListToolbar>
      {/* Another filter is another list: it starts at its first page. */}
      <RecordTable
        key={`${filter.planId ?? anyChoice}:${filter.state ?? anyChoice}`}
        list={list}
        caption={t("client:admin.supporters.subscriptions.title")}
        columns={columns}
        getId={(subscription) => subscription.id}
        headingLevel={2}
        empty={<EmptyState headingLevel={2} heading={t("client:admin.supporters.subscriptions.empty")} />}
      />
    </Stack>
  );
}
