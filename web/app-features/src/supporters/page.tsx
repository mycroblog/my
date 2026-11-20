// SPDX-License-Identifier: AGPL-3.0-only

// The supporters' page, at /about/supporters, where the rail's funding meter and the menu lead (ADR-0070 §13, ADR-0072
// §3, §6, §8, §14, ADR-0162; FR-911, FR-914, FR-918): the plans where the program is on, under the choice of the
// period, monthly or yearly, each offered to subscribe to (to a local reader whose address is confirmed, after a
// question that says what subscribing does, then the provider's page, left for in the same tab), or, for a member, to
// switch to (after a question that says whether the change applies now, the difference charged, or at the period's
// end), their own plan marked, and why the reader is not offered to subscribe in one sentence where they are not; the
// way to tip the server once, where it takes tips, with what a tip is; and the supporters who chose to be listed,
// under their plans, where the instance shows the list to the reader. In the native shell nothing of a purchase shows
// (ADR-0072 §14), and the list alone does. What came of a change is said under the plans, in the page's one polite
// live region, which takes the focus once the question has left.

import { AccountLine, supporterSettingsPath, supporterTipPath } from "@my/app-components";
import {
  planChangeTiming,
  planPrice,
  supporterActions,
  supporterPlansQuery,
  supporterRefusal,
  supportersQuery,
  isSupporterMember,
  moneyLess,
  moneySign,
  loadSupporterAccess,
  useInstance,
  useOwnSubscription,
  useSupporterAccess,
  useSupporterPlans,
  useSupporters,
  useSupporterWrites,
  type AppData,
  type SupporterInterval,
  type SupporterListPlan,
  type SupporterOffer,
  type SupporterPlan,
  type SupporterSubscription,
} from "@my/app-data";
import {
  Box,
  Button,
  FeedNotice,
  GroupHeading,
  LinkButton,
  ModalPresence,
  RowItem,
  RowList,
  SettingsResult,
  ShowMore,
  Stack,
  TextLink,
  useAddedFocus,
  useLocale,
  type AddedFocus,
} from "@my/design-system";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { barSaid, barSentence, sentences, useMoney } from "../supporter-words";
import { Retry, useDay, useSupporterStrings, type T } from "./parts";
import { ChargeConfirm, PlanChoice } from "./plans";

/** What the page's loader waits for: who may subscribe and tip, the plans, and the list's first page. */
export async function loadSupporters(data: AppData): Promise<void> {
  await loadSupporterAccess(data);
  await Promise.all([
    data.queryClient.query({ ...supporterPlansQuery(data), staleTime: Infinity }).catch(() => undefined),
    data.queryClient.infiniteQuery({ ...supportersQuery(data), staleTime: Infinity }).catch(() => undefined),
  ]);
}

type Props = {
  /** Whether the reader has a session: a visitor sees the plans and the list where the instance shows it to them. */
  signedIn: boolean;
  /** The sign-in, which comes back to this page. */
  signInHref: string;
  /** Leaves the app for a page that loads anew (the provider's checkout). */
  leave: (href: string) => void;
};

/** The page: the plans, the tip, and the supporters list, each where the server has it for this reader. */
export function SupportersScreen({ signedIn, signInHref, leave }: Props): ReactNode {
  const t = useSupporterStrings();
  const access = useSupporterAccess(signedIn);
  const visibility = useInstance()?.supporter_list_visibility;
  if (access.program === undefined)
    return access.failed ? (
      <FeedNotice alert lead action={<Retry onPress={access.retry} t={t} />}>
        {t("supporters.failed")}
      </FeedNotice>
    ) : (
      <FeedNotice lead>{t("supporters.loading")}</FeedNotice>
    );
  const plans = access.program && !access.app;
  const tips = access.tips === true && !access.app;
  const list = access.program && visibility !== undefined && visibility !== "off";
  if (!plans && !tips && !list) return <FeedNotice lead>{t("supporters.none")}</FeedNotice>;
  return (
    <Stack gap="entry-room">
      {plans && <Plans signedIn={signedIn} signInHref={signInHref} leave={leave} t={t} />}
      {/* One line leads to the sign-in, the first that asks for an account; the others say what they need. */}
      {tips && <Tip signedIn={signedIn} signInHref={plans ? undefined : signInHref} t={t} />}
      {list && (
        <Supporters
          visible={visibility === "everyone" || signedIn}
          signInHref={plans || tips ? undefined : signInHref}
          t={t}
        />
      )}
    </Stack>
  );
}

/** A question over the plans: subscribing to one at a period, or changing to it, now or at the period's end. */
type Asking =
  | { readonly kind: "checkout"; readonly plan: SupporterPlan; readonly interval: SupporterInterval }
  | {
      readonly kind: "change";
      readonly plan: SupporterPlan;
      readonly interval: SupporterInterval;
      readonly now: boolean;
    };

/** What the page says came of the last change, each a new element of its live region. */
type Said = { readonly text: string; readonly n: number };

/**
 * Runs `left`, as it was when the dialog opened, once the dialog it is given to is out of the document: the cleanup of
 * an effect of the dialog's own, which React runs after it took the dialog's nodes out.
 */
function Gone({ left }: { left: () => void }): null {
  const [run] = useState(() => left);
  useEffect(() => run, [run]);
  return null;
}

/** The plans, and what the reader may do with them. */
function Plans({ signedIn, signInHref, leave, t }: Props & { t: T }) {
  const access = useSupporterAccess(signedIn);
  const read = useSupporterPlans(true);
  const own = useOwnSubscription(signedIn);
  const writes = useSupporterWrites();
  const money = useMoney();
  const commerceUrl = useInstance()?.commerce_disclosure_url;
  const [asking, setAsking] = useState<Asking>();
  const [said, setSaid] = useState<Said>();
  const resultRef = useRef<HTMLParagraphElement>(null);
  const focusNext = useRef(false);
  const subscription = own.subscription;
  const member = isSupporterMember(subscription);
  const actions = supporterActions(subscription);
  return (
    <Stack as="section" aria-label={t("supporters.plans")} gap="screen-row-gap">
      <GroupHeading>{t("supporters.plans")}</GroupHeading>
      <Offer subscribe={access.subscribe} subscription={subscription} signInHref={signInHref} t={t} />
      {read.plans === undefined ? (
        read.status === "error" ? (
          <FeedNotice alert start action={<Retry onPress={read.refetch} t={t} />}>
            {t("supporters.plans.failed")}
          </FeedNotice>
        ) : (
          <FeedNotice start>{t("supporters.plans.loading")}</FeedNotice>
        )
      ) : read.plans.length === 0 ? (
        <FeedNotice start>{t("supporters.plans.none")}</FeedNotice>
      ) : (
        <PlanChoice
          plans={read.plans}
          current={
            member && subscription !== undefined
              ? { planId: subscription.plan_id, interval: subscription.interval }
              : undefined
          }
          initial={member && subscription !== undefined ? subscription.interval : "month"}
          action={(plan, interval) => {
            if (access.subscribe.may === true)
              return (
                <Button
                  kind="primary"
                  size="target"
                  onPress={() => {
                    setAsking({ kind: "checkout", plan, interval });
                  }}
                >
                  {t("supporters.subscribe")}
                </Button>
              );
            if (subscription === undefined || !actions.change) return undefined;
            const timing = planChangeTiming(subscription, plan, interval);
            if (timing === "same") return undefined;
            return (
              <Button
                kind="primary"
                size="target"
                onPress={() => {
                  setAsking({ kind: "change", plan, interval, now: timing === "now" });
                }}
              >
                {t("supporters.switch")}
              </Button>
            );
          }}
        />
      )}
      <SettingsResult ref={resultRef}>{said !== undefined && <span key={said.n}>{said.text}</span>}</SettingsResult>
      <ModalPresence>
        {asking?.kind === "checkout" && (
          <ChargeConfirm
            title={t("supporters.checkout.title", { plan: asking.plan.name, interval: asking.interval })}
            confirm={t("supporters.checkout.confirm")}
            cancel={t("supporters.checkout.cancel")}
            commerceUrl={commerceUrl}
            run={async () => {
              const checkout = await writes.checkout(asking.plan.id, asking.interval);
              leave(checkout.redirectUrl);
              // The page leaves for the provider's checkout: the question spins until it has.
              await new Promise<never>(() => undefined);
            }}
            onClose={() => {
              setAsking(undefined);
            }}
          >
            {t("supporters.checkout.text", {
              price: money(planPrice(asking.plan, asking.interval)),
              interval: asking.interval,
            })}
          </ChargeConfirm>
        )}
        {asking?.kind === "change" && subscription !== undefined && (
          <ChangeAsk
            asking={asking}
            subscription={subscription}
            plans={read.plans ?? []}
            commerceUrl={commerceUrl}
            done={(text) => {
              focusNext.current = true;
              setSaid((before) => ({ text, n: (before?.n ?? 0) + 1 }));
            }}
            onClose={() => {
              setAsking(undefined);
            }}
            onLeft={() => {
              if (!focusNext.current) return;
              focusNext.current = false;
              resultRef.current?.focus();
            }}
            t={t}
          />
        )}
      </ModalPresence>
    </Stack>
  );
}

/**
 * Whether `subscription` renews at another price than its plan's of today at its period (an earlier price kept, ADR-0072
 * §9); false for a plan the public list leaves out, whose price now is not known.
 */
function keptPrice(subscription: SupporterSubscription, plans: readonly SupporterPlan[]): boolean {
  const plan = plans.find((p) => p.id === subscription.plan_id);
  if (plan === undefined) return false;
  const now = planPrice(plan, subscription.interval);
  return now.currency !== subscription.price.currency || moneySign(moneyLess(now, subscription.price) ?? now) !== 0;
}

/** The question before a change of the plan, and what came of it. */
function ChangeAsk({
  asking,
  subscription,
  plans,
  commerceUrl,
  done,
  onClose,
  onLeft,
  t,
}: {
  asking: Extract<Asking, { kind: "change" }>;
  subscription: SupporterSubscription;
  /** The public plans, among which the reader's plan's price now is found. */
  plans: readonly SupporterPlan[];
  commerceUrl: string | undefined;
  done: (text: string) => void;
  onClose: () => void;
  onLeft: () => void;
  t: T;
}) {
  const writes = useSupporterWrites();
  const money = useMoney();
  const { locale } = useLocale();
  const day = useDay();
  const when = asking.now ? "now" : "end";
  const date = subscription.period_ends_at === undefined ? "" : day(subscription.period_ends_at);
  const words = { plan: asking.plan.name, interval: asking.interval, when, date };
  return (
    <>
      <Gone left={onLeft} />
      <ChargeConfirm
        title={t("supporters.change.title", words)}
        confirm={t("supporters.change.confirm", { when })}
        cancel={t("supporters.change.keep")}
        commerceUrl={commerceUrl}
        run={async () => {
          try {
            await writes.change(asking.plan.id, asking.interval);
          } catch (error) {
            // A change whose answer did not reach the reader may have been made: pressed again, it is their plan.
            if (supporterRefusal(error) !== "same-plan") throw error;
            done(t("supporters.same_plan", words));
            return;
          }
          done(t("supporters.changed", words));
        }}
        onClose={onClose}
      >
        {sentences(locale, [
          t("supporters.change.text", {
            when,
            plan: subscription.plan_name,
            price: money(planPrice(asking.plan, asking.interval)),
            interval: asking.interval,
            date,
          }),
          // An earlier price the reader renews at is lost with any change, even back to the plan (ADR-0072 §8).
          ...(keptPrice(subscription, plans)
            ? [t("supporters.change.price_kept", { kept: money(subscription.price), plan: subscription.plan_name })]
            : []),
        ])}
      </ChargeConfirm>
    </>
  );
}

/**
 * What the page says of the offer above the plans: why the reader may not subscribe, in one sentence, with the way on
 * (signing in); that they support the server, with the way to their subscription; nothing while they may subscribe.
 */
function Offer({
  subscribe,
  subscription,
  signInHref,
  t,
}: {
  subscribe: SupporterOffer;
  subscription: SupporterSubscription | undefined;
  signInHref: string;
  t: T;
}) {
  if (subscribe.bar === "subscribed")
    return (
      <FeedNotice start action={<TextLink href={supporterSettingsPath}>{t("supporters.subscription")}</TextLink>}>
        {/* A subscription whose grace ended holds no plan: its payment is what to see to. */}
        {isSupporterMember(subscription)
          ? t("supporters.yours.plan", { plan: subscription?.plan_name ?? "" })
          : t("supporters.yours.lapsed", { plan: subscription?.plan_name ?? "" })}
      </FeedNotice>
    );
  if (!barSaid(subscribe.bar)) return null;
  return (
    <FeedNotice
      start
      action={
        subscribe.bar === "signed-out" ? <TextLink href={signInHref}>{t("supporters.sign_in")}</TextLink> : undefined
      }
    >
      {barSentence(subscribe.bar, "subscribe", t)}
    </FeedNotice>
  );
}

/** The way to tip the server once, with what a tip is (FR-918), or why the reader may not. */
function Tip({ signedIn, signInHref, t }: { signedIn: boolean; signInHref: string | undefined; t: T }) {
  const { tip } = useSupporterAccess(signedIn);
  const server = useInstance()?.name ?? "";
  return (
    <Stack as="section" aria-label={t("supporters.tip")} gap="screen-row-gap">
      <GroupHeading>{t("supporters.tip")}</GroupHeading>
      <Box as="p" text="body" tone="muted" breaks="phrase">
        {t("supporters.tip.about", { server })}
      </Box>
      {tip.may === true ? (
        <Box>
          <LinkButton size="target" href={supporterTipPath}>
            {t("supporters.tip.go")}
          </LinkButton>
        </Box>
      ) : (
        barSaid(tip.bar) && (
          <FeedNotice
            start
            action={
              tip.bar === "signed-out" && signInHref !== undefined ? (
                <TextLink href={signInHref}>{t("supporters.sign_in")}</TextLink>
              ) : undefined
            }
          >
            {barSentence(tip.bar, "tip", t)}
          </FeedNotice>
        )
      )}
    </Stack>
  );
}

/** The supporters who chose to be listed, under the plans they are members of, or that a visitor must sign in. */
function Supporters({ visible, signInHref, t }: { visible: boolean; signInHref: string | undefined; t: T }) {
  const list = useSupporters(undefined, visible);
  const added = useAddedFocus(list.items.length);
  let body: ReactNode;
  if (!visible)
    body = (
      <FeedNotice
        start
        action={signInHref === undefined ? undefined : <TextLink href={signInHref}>{t("supporters.sign_in")}</TextLink>}
      >
        {t("supporters.list.signed_in")}
      </FeedNotice>
    );
  else if (list.status === "pending") body = <FeedNotice start>{t("supporters.list.loading")}</FeedNotice>;
  else if (list.items.length === 0)
    body =
      list.status === "error" ? (
        <FeedNotice alert start action={<Retry onPress={list.refetch} t={t} />}>
          {t("supporters.list.failed")}
        </FeedNotice>
      ) : (
        <FeedNotice start>{t("supporters.list.empty")}</FeedNotice>
      );
  else
    body = (
      <Stack gap="entry-room">
        {(list.plans ?? []).map((plan) => (
          <PlanGroup
            key={plan.id}
            plan={plan}
            accounts={list.items.filter((e) => e.plan_id === plan.id)}
            item={(entry) => added.item(list.items.indexOf(entry))}
          />
        ))}
        <ShowMore
          more={list.hasNextPage}
          busy={list.isFetchingNextPage}
          failed={list.failedNextPage}
          onPress={added.press(list.fetchNextPage)}
        />
      </Stack>
    );
  return (
    <Stack as="section" aria-label={t("supporters.list")} gap="screen-row-gap">
      <GroupHeading>{t("supporters.list")}</GroupHeading>
      <Box as="p" text="body" tone="muted" breaks="phrase">
        {t("supporters.list.about")}
      </Box>
      {body}
    </Stack>
  );
}

/** The listed supporters of one plan, under its name; nothing for none. */
function PlanGroup<A extends { readonly account: Parameters<typeof AccountLine>[0]["account"] }>({
  plan,
  accounts,
  item,
}: {
  plan: SupporterListPlan;
  accounts: readonly A[];
  /** The ref of a supporter's row, for the list that grows under Show more. */
  item: (entry: A) => ReturnType<AddedFocus["item"]>;
}) {
  if (accounts.length === 0) return null;
  return (
    <Stack gap="entry-line">
      <GroupHeading>{plan.name}</GroupHeading>
      <RowList label={plan.name}>
        {accounts.map((a) => (
          <RowItem key={a.account.id} itemRef={item(a)}>
            <AccountLine account={a.account} />
          </RowItem>
        ))}
      </RowList>
    </Stack>
  );
}
