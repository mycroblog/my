// SPDX-License-Identifier: AGPL-3.0-only

// The subscriptions to the supporter plans as a holder of `manage_supporters` reads them (ADR-0072 §10; FR-910): at
// /admin/supporters/subscriptions, the newest first, filtered by plan and state, which the address keeps, each its
// subscriber, its plan and period, its state and when its period ends, a page at a time; and one subscription at
// /admin/supporters/subscriptions/{id}: its facts, and ending it at once, without a refund, after the reader typed
// the subscriber's handle. What a press did is said in the screen's status, which takes the focus. Never an amount
// paid: that is the finance's.

import {
  loadAdminPlans,
  loadAdminSubscription,
  loadAdminSubscriptions,
  subscriptionStates,
  useAdminPlans,
  useAdminSubscription,
  useAdminSubscriptions,
  useEndSubscription,
  type AdminSubscription,
  type AppData,
  type SubscriptionQuery,
  type SubscriptionState,
} from "@my/app-data";
import {
  BackButton,
  Button,
  ConfirmDialog,
  FactList,
  FeedNotice,
  FilterBar,
  FilterFacet,
  ModalPresence,
  MoreRecords,
  RecordTable,
  Row,
  ScreenStack,
  ScreenTitle,
  Stack,
  StateMark,
  TextLink,
  type Fact,
  type FilterValue,
  type RecordColumn,
  type RecordRow,
} from "@my/design-system";
import { useRef, useState, type ReactNode } from "react";
import { staffHref, type StaffSearch, type StaffUnderProps } from "../entries";
import { moneyText } from "../../finance-totals";
import { ReadNotice, StatusRegion, Titled, useMoment, useStaffStrings, useStatus, type T } from "../parts";
import { stateWords, subscriberOf, subscriptionMark, supportersPaths } from "./parts";

/** The subscriptions an address's query asks for: of a plan and a state, each of every kind where left out. */
export function subscriptionsOf(search: StaffSearch): SubscriptionQuery {
  const plan = search["plan"];
  return {
    plan: plan === undefined || plan === "" ? undefined : plan,
    state: subscriptionStates.find((s) => s === search["state"]),
  };
}

const searchOf = (q: SubscriptionQuery): StaffSearch => ({
  ...(q.plan === undefined ? {} : { plan: q.plan }),
  ...(q.state === undefined ? {} : { state: q.state }),
});

/** A subscription's plan and period in words. */
const termsOf = (t: T, s: AdminSubscription) =>
  t("staff_supporters.subscription.terms", { plan: s.plan_name, interval: s.interval });

type Column = "plan" | "ends" | "state";

/** The subscriptions, filtered by plan and state, a page at a time. */
export function SubscriptionListScreen({ search, onSearch, go }: StaffUnderProps): ReactNode {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const q = subscriptionsOf(search);
  const list = useAdminSubscriptions(q);
  const plans = useAdminPlans().value ?? [];
  const planValues: FilterValue[] = plans.map((p) => ({ id: p.id, label: p.name, icon: "funding" }));
  const stateValues: FilterValue[] = subscriptionStates.map((s) => ({
    id: s,
    label: stateWords(t, s),
    icon:
      subscriptionMark[s] === "active"
        ? "stateActive"
        : subscriptionMark[s] === "expired"
          ? "stateExpired"
          : "statePending",
  }));
  const filtered = q.plan !== undefined || q.state !== undefined;
  const head = (
    <>
      <ScreenTitle
        back={
          <BackButton
            onPress={() => {
              go(supportersPaths.program);
            }}
          >
            {t("staff_supporters.back.program")}
          </BackButton>
        }
      >
        {t("staff_supporters.subscriptions")}
      </ScreenTitle>
      <FilterBar
        label={t("staff_supporters.subscriptions.filters")}
        applied={filtered}
        onClear={() => {
          onSearch({});
        }}
      >
        <FilterFacet
          label={t("staff_supporters.subscriptions.plan")}
          any={t("staff_supporters.subscriptions.plan.any")}
          values={planValues}
          chosen={q.plan}
          onChoose={(plan) => {
            onSearch(searchOf({ ...q, plan }));
          }}
        />
        <FilterFacet
          label={t("staff_supporters.subscriptions.state")}
          any={t("staff_supporters.subscriptions.state.any")}
          values={stateValues}
          chosen={q.state}
          onChoose={(state) => {
            onSearch(searchOf({ ...q, state: subscriptionStates.find((s) => s === state) }));
          }}
        />
      </FilterBar>
    </>
  );
  if (list.status !== "success" && list.items.length === 0)
    return (
      <ScreenStack>
        {head}
        <ReadNotice
          status={list.status === "error" ? "error" : "pending"}
          loading={t("staff_supporters.subscriptions.loading")}
          failed={t("staff_supporters.subscriptions.failed")}
          retry={list.refetch}
        />
      </ScreenStack>
    );
  const columns: RecordColumn<Column>[] = [
    { key: "plan", label: t("staff_supporters.subscriptions.column.plan"), bare: true },
    { key: "ends", label: t("staff_supporters.subscriptions.column.ends"), kind: "fit", bare: true },
    { key: "state", label: t("staff_supporters.subscriptions.column.state"), kind: "fit", beside: true },
  ];
  const rows = list.items.map((s): RecordRow<Column> => ({
    id: s.id,
    head: subscriberOf(t, s),
    href: staffHref(supportersPaths.subscription(s.id), search),
    cells: {
      plan: termsOf(t, s),
      ends:
        s.period_ends_at === undefined
          ? t("staff_supporters.subscription.no_period")
          : t("staff_supporters.subscription.until", { at: moment(s.period_ends_at) }),
      state: <StateMark state={subscriptionMark[s.state]}>{stateWords(t, s.state)}</StateMark>,
    },
  }));
  return (
    <ScreenStack>
      {head}
      <RecordTable
        label={t("staff_supporters.subscriptions")}
        head={{ label: t("staff_supporters.subscriptions.column.subscriber") }}
        columns={columns}
        rows={rows}
        even
        empty={
          <FeedNotice>
            {t(filtered ? "staff_supporters.subscriptions.empty.filtered" : "staff_supporters.subscriptions.empty")}
          </FeedNotice>
        }
      />
      {list.failedNextPage ? <FeedNotice alert>{t("staff_supporters.subscriptions.more_failed")}</FeedNotice> : null}
      {list.hasNextPage && <MoreRecords onPress={list.fetchNextPage} busy={list.isFetchingNextPage} />}
    </ScreenStack>
  );
}

/** Reads the subscriptions of an address's query and the plans its filter names, for the route's loader. */
export async function loadSubscriptionList(data: AppData, search: StaffSearch): Promise<void> {
  await Promise.all([loadAdminSubscriptions(data, subscriptionsOf(search)), loadAdminPlans(data)]);
}

/** Reads the subscription of `id`, for the route's loader. */
export const loadSubscription = (data: AppData, id: string): Promise<void> => loadAdminSubscription(data, id);

/** The subscriber of the subscription of `id`, the title of its screen; undefined until it is read. */
export function useSubscriptionTitle(id: string): string | undefined {
  const { t } = useStaffStrings();
  const s = useAdminSubscription(id).value;
  return s === undefined ? undefined : t("staff_supporters.subscription.title", { subscriber: subscriberOf(t, s) });
}

/** Whether a subscription goes on, so that it can be ended. */
const goesOn = (state: SubscriptionState) =>
  state === "pending" || state === "active" || state === "canceling" || state === "past_due" || state === "lapsed";

/** One subscription: its facts, and End now… while it goes on. */
export function SubscriptionScreen({ id, search, go }: StaffUnderProps & { id: string }): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminSubscription(id);
  const back = staffHref(supportersPaths.subscriptions, search);
  if (read.forbidden) return <FeedNotice lead>{t("staff_supporters.forbidden")}</FeedNotice>;
  if (read.missing)
    return (
      <FeedNotice alert lead action={<TextLink href={back}>{t("staff_supporters.back.subscriptions")}</TextLink>}>
        {t("staff_supporters.subscription.missing")}
      </FeedNotice>
    );
  if (read.value === undefined)
    return (
      <ReadNotice
        status={read.status === "error" ? "error" : "pending"}
        loading={t("staff_supporters.subscription.loading")}
        failed={t("staff_supporters.subscription.failed")}
        retry={read.refetch}
      />
    );
  return (
    <SubscriptionView
      subscription={read.value}
      onBack={() => {
        go(back);
      }}
    />
  );
}

function SubscriptionView({ subscription: s, onBack }: { subscription: AdminSubscription; onBack: () => void }) {
  const { t, locale } = useStaffStrings();
  const moment = useMoment();
  const end = useEndSubscription();
  const region = useRef<HTMLDivElement>(null);
  const status = useStatus(region);
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  const subscriber = subscriberOf(t, s);
  const facts: Fact[] = [
    {
      term: t("staff_supporters.subscription.state"),
      value: <StateMark state={subscriptionMark[s.state]}>{stateWords(t, s.state)}</StateMark>,
    },
    { term: t("staff_supporters.subscription.plan"), value: termsOf(t, s) },
    {
      term: t("staff_supporters.subscription.price"),
      value: t("staff_supporters.subscription.price.value", {
        price: moneyText(locale, s.price),
        interval: s.interval,
      }),
    },
    { term: t("staff_supporters.subscription.started"), value: moment(s.started_at) },
    ...(s.period_ends_at === undefined
      ? []
      : [{ term: t("staff_supporters.subscription.period_ends"), value: moment(s.period_ends_at) }]),
    ...(s.grace_ends_at === undefined
      ? []
      : [{ term: t("staff_supporters.subscription.grace_ends"), value: moment(s.grace_ends_at) }]),
    ...(s.scheduled_interval === undefined && s.scheduled_plan_id === undefined
      ? []
      : [
          {
            term: t("staff_supporters.subscription.scheduled"),
            value: t("staff_supporters.subscription.scheduled.value"),
          },
        ]),
  ];
  return (
    <ScreenStack>
      <ScreenTitle back={<BackButton onPress={onBack}>{t("staff_supporters.back.subscriptions")}</BackButton>}>
        {subscriber}
      </ScreenTitle>
      <StatusRegion said={status.said} region={region} />
      <FactList label={subscriber} items={facts} />
      {goesOn(s.state) && (
        <Titled title={t("staff_supporters.subscription.end")}>
          <Stack gap="screen-field-gap">
            <FeedNotice start lead>
              {t("staff_supporters.subscription.end.about")}
            </FeedNotice>
            <Row>
              <Button
                onPress={() => {
                  setAsking(true);
                }}
              >
                {t("staff_supporters.subscription.end.ask")}
              </Button>
            </Row>
          </Stack>
        </Titled>
      )}
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            title={t("staff_supporters.subscription.end.confirm", { subscriber })}
            cancel={t("staff_supporters.cancel")}
            confirm={t("staff_supporters.subscription.end.do")}
            {...(s.username === undefined ? {} : { phrase: `@${s.username}` })}
            busy={busy}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={() => {
              setBusy(true);
              end(s.id).then(
                () => {
                  setBusy(false);
                  setAsking(false);
                  status.say(t("staff_supporters.subscription.done.ended"));
                },
                () => {
                  setBusy(false);
                  setAsking(false);
                  status.say(t("staff_supporters.write_failed"), true);
                },
              );
            }}
          >
            {t("staff_supporters.subscription.end.confirm.about")}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </ScreenStack>
  );
}
