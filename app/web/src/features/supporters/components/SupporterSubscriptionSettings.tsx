// SPDX-License-Identifier: AGPL-3.0-only
import {
  Box,
  Button,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  DialogTrigger,
  EmptyState,
  Heading,
  Hint,
  InlineError,
  Radio,
  RadioGroup,
  RegionError,
  Spinner,
  Stack,
  StatusMessage,
  Text,
} from "@my/ui";
import { type ReactNode, useId, useState } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useFormatMoney } from "../../costs";
import { DocumentLink } from "../../documents";
import {
  type SupporterIneligibleReason,
  type SupporterInterval,
  supporterIntervals,
  type SupporterPlan,
  type SupporterSubscription,
} from "../api/supporters";
import {
  changeTiming,
  type SupporterReturn,
  useCheckout,
  usePortal,
  useSubscriptionCommands,
  useSupporterPage,
} from "../hooks/useSupporterSubscription";

/**
 * The body of the user's page of the supporter plans (`/settings/supporter`, ADR-0072 §6, §8, §13): the user's
 * subscription and what they may do with it, or the plans with the final confirmation before the provider's hosted
 * checkout. The Tauri app shows that the page is not available there and nothing else (§14).
 */
export function SupporterSubscriptionSettings({ returned }: { returned: SupporterReturn }) {
  const t = useT();
  const page = useSupporterPage(returned);
  switch (page.status) {
    case "app":
      return <EmptyState heading={t("client:supporter.app")} />;
    case "disabled":
      return <EmptyState heading={t("client:supporter.disabled")} />;
    case "loading":
      return (
        <div aria-busy="true">
          <Spinner label={t("client:supporter.loading")} />
        </div>
      );
    case "error":
      return (
        <RegionError
          heading={t("client:supporter.failed")}
          reason={page.reason}
          retryLabel={t("client:supporter.retry")}
          onRetry={page.retry}
          isRetrying={page.isRetrying}
        />
      );
    case "ready":
      return (
        <Stack gap="8">
          <ReturnMessage returned={returned} confirming={page.confirming} subscription={page.subscription} />
          {page.subscription === undefined ? (
            <Subscribe
              plans={page.plans}
              ineligible={page.ineligible}
              commerceDisclosureUrl={page.commerceDisclosureUrl}
            />
          ) : (
            <CurrentSubscription
              subscription={page.subscription}
              plans={page.plans}
              commerceDisclosureUrl={page.commerceDisclosureUrl}
            />
          )}
        </Stack>
      );
  }
}

/** What the provider's checkout returned: the payment being confirmed, the subscription that started, or nothing charged. */
function ReturnMessage({
  returned,
  confirming,
  subscription,
}: {
  returned: SupporterReturn;
  confirming: boolean;
  subscription: SupporterSubscription | undefined;
}) {
  const t = useT();
  let message: string | undefined;
  if (returned === "checkout-canceled") message = t("client:supporter.return.canceled");
  else if (returned === "checkout-success") {
    if (confirming) message = t("client:supporter.return.confirming");
    else if (subscription !== undefined) message = t("client:supporter.return.started");
  }
  return (
    <StatusMessage tone={returned === "checkout-success" && !confirming ? "success" : "neutral"}>
      {message}
    </StatusMessage>
  );
}

/** A section of the page: an `h2` that names a `<section>`. */
function Section({ title, children }: { title: string; children: ReactNode }) {
  const headingId = useId();
  return (
    <Stack as="section" gap="4" aria-labelledby={headingId}>
      <Heading level={2} id={headingId}>
        {title}
      </Heading>
      {children}
    </Stack>
  );
}

const ineligibleMessages = {
  not_local: "client:supporter.ineligible.not_local",
  email_unverified: "client:supporter.ineligible.email_unverified",
  restricted: "client:supporter.ineligible.restricted",
} as const satisfies Record<SupporterIneligibleReason, ClientMessageKey>;

/** An amount per interval: "¥500 a month". */
function usePerInterval(): (amount: string, interval: SupporterInterval) => string {
  const t = useT();
  return (amount, interval) =>
    t(interval === "year" ? "client:supporter.per_year" : "client:supporter.per_month", { amount });
}

/** The plans, the choice of one and of the interval, and the final confirmation before the checkout. */
function Subscribe({
  plans,
  ineligible,
  commerceDisclosureUrl,
}: {
  plans: readonly SupporterPlan[];
  ineligible: SupporterIneligibleReason | undefined;
  commerceDisclosureUrl: string | undefined;
}) {
  const t = useT();
  const [planId, setPlanId] = useState<string | undefined>(undefined);
  const [interval, setInterval] = useState<SupporterInterval>("month");
  const checkout = useCheckout();
  const plan = plans.find((p) => p.id === planId) ?? plans[0];
  return (
    <Section title={t("client:supporter.plans.title")}>
      <Hint size="inherit">{t("client:supporter.plans.description")}</Hint>
      {ineligible !== undefined && (
        <Box as="p" border="lg" padding="4" role="note">
          {t(ineligibleMessages[ineligible])}
        </Box>
      )}
      {plans.length === 0 || plan === undefined ? (
        <EmptyState headingLevel={3} heading={t("client:supporter.plans.empty")} />
      ) : (
        <>
          <PlanChooser
            plans={plans}
            plan={plan}
            onPlanChange={setPlanId}
            interval={interval}
            onIntervalChange={setInterval}
          />
          {ineligible === undefined && (
            <div>
              <DialogTrigger>
                <Button variant="primary">{t("client:supporter.subscribe")}</Button>
                <CheckoutConfirmation
                  plan={plan}
                  interval={interval}
                  commerceDisclosureUrl={commerceDisclosureUrl}
                  isPending={checkout.isPending}
                  error={checkout.error}
                  onConfirm={() => {
                    checkout.start({ plan_id: plan.id, interval });
                  }}
                />
              </DialogTrigger>
            </div>
          )}
        </>
      )}
    </Section>
  );
}

/** The radio groups of the plan and of the interval, with the prices of each. */
function PlanChooser({
  plans,
  plan,
  onPlanChange,
  interval,
  onIntervalChange,
}: {
  plans: readonly SupporterPlan[];
  plan: SupporterPlan;
  onPlanChange: (id: string) => void;
  interval: SupporterInterval;
  onIntervalChange: (interval: SupporterInterval) => void;
}) {
  const t = useT();
  const format = useFormatMoney();
  const perInterval = usePerInterval();
  return (
    <Stack gap="6">
      <RadioGroup label={t("client:supporter.plans.plan")} value={plan.id} onChange={onPlanChange}>
        {plans.map((p) => (
          <Radio
            key={p.id}
            value={p.id}
            description={
              <Stack as="span" gap="1">
                <span>
                  {t("client:supporter.plans.prices", {
                    monthly: perInterval(format(p.monthly_price), "month"),
                    yearly: perInterval(format(p.yearly_price), "year"),
                  })}
                </span>
                {p.description !== "" && (
                  // The administrators' text, in a language the app does not know (web-client/README.md Y10).
                  <Text whitespace="preLine" dir="auto" lang="">
                    {p.description}
                  </Text>
                )}
              </Stack>
            }
          >
            <span dir="auto" lang="">
              {p.name}
            </span>
          </Radio>
        ))}
      </RadioGroup>
      <RadioGroup
        label={t("client:supporter.plans.interval")}
        value={interval}
        onChange={(value) => {
          const next = supporterIntervals.find((i) => i === value);
          if (next !== undefined) onIntervalChange(next);
        }}
      >
        {supporterIntervals.map((i) => (
          <Radio
            key={i}
            value={i}
            description={perInterval(format(i === "year" ? plan.yearly_price : plan.monthly_price), i)}
          >
            {t(i === "year" ? "client:supporter.interval.year" : "client:supporter.interval.month")}
          </Radio>
        ))}
      </RadioGroup>
    </Stack>
  );
}

/**
 * The final confirmation before the checkout (ADR-0072 §13): the plan, the amount of each period and that it renews
 * at the same amount, when the payments are made, when the benefits start, how to cancel and what it does, and the
 * link to the commerce disclosures.
 */
function CheckoutConfirmation({
  plan,
  interval,
  commerceDisclosureUrl,
  isPending,
  error,
  onConfirm,
}: {
  plan: SupporterPlan;
  interval: SupporterInterval;
  commerceDisclosureUrl: string | undefined;
  isPending: boolean;
  error: string | undefined;
  onConfirm: () => void;
}) {
  const t = useT();
  const format = useFormatMoney();
  const perInterval = usePerInterval();
  const amount = format(interval === "year" ? plan.yearly_price : plan.monthly_price);
  const name = `⁨${plan.name}⁩`;
  return (
    <ConfirmDialog
      title={t("client:supporter.confirm.title", { plan: name })}
      description={t("client:supporter.confirm.description")}
      confirmLabel={t("client:supporter.confirm.go")}
      cancelLabel={t("client:supporter.cancel")}
      tone="primary"
      isPending={isPending}
      {...(error !== undefined && { errorMessage: t("client:supporter.confirm.failed", { reason: error }) })}
      onConfirm={onConfirm}
    >
      <DescriptionList columns={1}>
        <DescriptionItem term={t("client:supporter.confirm.plan")}>
          <span dir="auto" lang="">
            {plan.name}
          </span>
        </DescriptionItem>
        <DescriptionItem term={t("client:supporter.confirm.amount")}>
          {t("client:supporter.confirm.amount.value", { price: perInterval(amount, interval) })}
        </DescriptionItem>
        <DescriptionItem term={t("client:supporter.confirm.payments")}>
          {t("client:supporter.confirm.payments.value", { interval })}
        </DescriptionItem>
        <DescriptionItem term={t("client:supporter.confirm.benefits")}>
          {t("client:supporter.confirm.benefits.value")}
        </DescriptionItem>
        <DescriptionItem term={t("client:supporter.confirm.cancel")}>
          {t("client:supporter.confirm.cancel.value")}
        </DescriptionItem>
      </DescriptionList>
      {commerceDisclosureUrl !== undefined && (
        <p>
          <DocumentLink href={commerceDisclosureUrl}>{t("client:supporter.commerce")}</DocumentLink>
        </p>
      )}
    </ConfirmDialog>
  );
}

const stateLabels = {
  pending: "client:supporter.state.pending",
  active: "client:supporter.state.active",
  canceling: "client:supporter.state.canceling",
  past_due: "client:supporter.state.past_due",
  lapsed: "client:supporter.state.lapsed",
  expired: "client:supporter.state.expired",
  revoked: "client:supporter.state.revoked",
  abandoned: "client:supporter.state.abandoned",
} as const satisfies Record<SupporterSubscription["state"], ClientMessageKey>;

/** A date and time in the viewer's locale and time zone. */
function When({ value }: { value: string }) {
  const intl = useIntl();
  return <time dateTime={value}>{intl.formatDate(value, { dateStyle: "long", timeStyle: "short" })}</time>;
}

/** The user's subscription, what its state means, and what they may do with it. */
function CurrentSubscription({
  subscription,
  plans,
  commerceDisclosureUrl,
}: {
  subscription: SupporterSubscription;
  plans: readonly SupporterPlan[];
  commerceDisclosureUrl: string | undefined;
}) {
  const t = useT();
  const format = useFormatMoney();
  const perInterval = usePerInterval();
  const intl = useIntl();
  const commands = useSubscriptionCommands();
  const scheduled = subscription.scheduled_change;
  return (
    <>
      <Section title={t("client:supporter.subscription.title")}>
        <DescriptionList columns={2}>
          <DescriptionItem term={t("client:supporter.subscription.plan")}>
            <span dir="auto" lang="">
              {subscription.plan_name}
            </span>
          </DescriptionItem>
          <DescriptionItem term={t("client:supporter.subscription.state")}>
            {t(stateLabels[subscription.state])}
          </DescriptionItem>
          <DescriptionItem term={t("client:supporter.subscription.price")}>
            {perInterval(format(subscription.price), subscription.interval)}
          </DescriptionItem>
          {subscription.period_ends_at !== undefined && (
            <DescriptionItem
              term={t(
                subscription.state === "canceling"
                  ? "client:supporter.subscription.ends_at"
                  : "client:supporter.subscription.renews_at",
              )}
            >
              <When value={subscription.period_ends_at} />
            </DescriptionItem>
          )}
          {subscription.grace_ends_at !== undefined && (
            <DescriptionItem term={t("client:supporter.subscription.grace_ends_at")}>
              <When value={subscription.grace_ends_at} />
            </DescriptionItem>
          )}
          {scheduled !== undefined && (
            <DescriptionItem term={t("client:supporter.subscription.scheduled")}>
              {t("client:supporter.subscription.scheduled.value", {
                plan: `⁨${scheduled.plan_name}⁩`,
                price: perInterval(format(scheduled.price), scheduled.interval),
                date: intl.formatDate(scheduled.effective_at, { dateStyle: "long", timeStyle: "short" }),
              })}
            </DescriptionItem>
          )}
        </DescriptionList>
        <StateNote subscription={subscription} />
        <StatusMessage tone="success">{commands.status}</StatusMessage>
        <SubscriptionActions
          subscription={subscription}
          commands={commands}
          commerceDisclosureUrl={commerceDisclosureUrl}
        />
      </Section>
      {subscription.state === "active" && <ChangePlan subscription={subscription} plans={plans} commands={commands} />}
    </>
  );
}

const stateNotes = {
  pending: "client:supporter.note.pending",
  active: undefined,
  canceling: "client:supporter.note.canceling",
  past_due: "client:supporter.note.past_due",
  lapsed: "client:supporter.note.lapsed",
  expired: undefined,
  revoked: undefined,
  abandoned: undefined,
} as const satisfies Record<SupporterSubscription["state"], ClientMessageKey | undefined>;

/** What the subscription's state means for the user, when it asks something of them. */
function StateNote({ subscription }: { subscription: SupporterSubscription }) {
  const t = useT();
  const note = stateNotes[subscription.state];
  if (note === undefined) return null;
  return <p>{t(note)}</p>;
}

type Commands = ReturnType<typeof useSubscriptionCommands>;

/** The buttons of the subscription's state: continue the payment, cancel, resume, withdraw the change, the portal. */
function SubscriptionActions({
  subscription,
  commands,
  commerceDisclosureUrl,
}: {
  subscription: SupporterSubscription;
  commands: Commands;
  commerceDisclosureUrl: string | undefined;
}) {
  const t = useT();
  const checkout = useCheckout();
  const portal = usePortal();
  const state = subscription.state;
  const failed = (kind: string) => (commands.failed === kind ? commands.error : undefined);
  return (
    <Stack gap="3">
      <Cluster gap="2">
        {state === "pending" && (
          <Button
            variant="primary"
            isPending={checkout.isPending}
            onPress={() => {
              checkout.start({ plan_id: subscription.plan_id, interval: subscription.interval });
            }}
          >
            {t("client:supporter.continue_checkout")}
          </Button>
        )}
        {state === "canceling" && (
          <Button
            variant="primary"
            isPending={commands.pending === "resume"}
            onPress={() => {
              void commands.run({ kind: "resume" });
            }}
          >
            {t("client:supporter.resume")}
          </Button>
        )}
        {subscription.scheduled_change !== undefined && (
          <Button
            isPending={commands.pending === "cancel_scheduled_change"}
            onPress={() => {
              void commands.run({ kind: "cancel_scheduled_change" });
            }}
          >
            {t("client:supporter.unschedule")}
          </Button>
        )}
        {state !== "pending" && (
          <Button isPending={portal.isPending} onPress={portal.open}>
            {t("client:supporter.portal")}
          </Button>
        )}
        {(state === "active" || state === "past_due") && (
          <CancelSubscription subscription={subscription} commands={commands} />
        )}
      </Cluster>
      {checkout.error !== undefined && (
        <InlineError alert>{t("client:supporter.confirm.failed", { reason: checkout.error })}</InlineError>
      )}
      {portal.error !== undefined && (
        <InlineError alert>{t("client:supporter.portal.failed", { reason: portal.error })}</InlineError>
      )}
      {(failed("resume") ?? failed("cancel_scheduled_change")) !== undefined && (
        <InlineError alert>
          {t("client:supporter.command.failed", {
            reason: failed("resume") ?? failed("cancel_scheduled_change") ?? "",
          })}
        </InlineError>
      )}
      {commerceDisclosureUrl !== undefined && (
        <p>
          <DocumentLink href={commerceDisclosureUrl}>{t("client:supporter.commerce")}</DocumentLink>
        </p>
      )}
    </Stack>
  );
}

/** Cancelling at the end of the period, after an alert dialog that says the benefits stay until then. */
function CancelSubscription({ subscription, commands }: { subscription: SupporterSubscription; commands: Commands }) {
  const t = useT();
  return (
    <DialogTrigger>
      <Button variant="danger">{t("client:supporter.cancel_subscription")}</Button>
      <ConfirmDialog
        title={t("client:supporter.cancel_subscription.title", { plan: `⁨${subscription.plan_name}⁩` })}
        description={t("client:supporter.cancel_subscription.description")}
        confirmLabel={t("client:supporter.cancel_subscription")}
        cancelLabel={t("client:supporter.keep")}
        tone="danger"
        isPending={commands.pending === "cancel"}
        {...(commands.failed === "cancel" &&
          commands.error !== undefined && {
            errorMessage: t("client:supporter.command.failed", { reason: commands.error }),
          })}
        onConfirm={(close) => {
          void commands.run({ kind: "cancel" }).then((done) => {
            if (done) close();
          });
        }}
      />
    </DialogTrigger>
  );
}

/** Changing the plan or the interval of an active subscription, after a dialog that says when the change applies. */
function ChangePlan({
  subscription,
  plans,
  commands,
}: {
  subscription: SupporterSubscription;
  plans: readonly SupporterPlan[];
  commands: Commands;
}) {
  const t = useT();
  const format = useFormatMoney();
  const current = plans.find((p) => p.id === subscription.plan_id);
  const [planId, setPlanId] = useState<string | undefined>(undefined);
  const [interval, setInterval] = useState<SupporterInterval>(subscription.interval);
  const plan = plans.find((p) => p.id === planId) ?? current ?? plans[0];
  if (plan === undefined) return null;
  const unchanged = plan.id === subscription.plan_id && interval === subscription.interval;
  const timing = changeTiming({ rank: current?.rank, interval: subscription.interval }, { rank: plan.rank, interval });
  const currentPrice =
    current === undefined ? undefined : subscription.interval === "year" ? current.yearly_price : current.monthly_price;
  const keepsLowerPrice = currentPrice !== undefined && currentPrice.amount !== subscription.price.amount;
  const timingNote: ClientMessageKey =
    timing === "now"
      ? "client:supporter.change.now"
      : timing === "period_end"
        ? "client:supporter.change.period_end"
        : "client:supporter.change.rules";
  return (
    <Section title={t("client:supporter.change.title")}>
      <Hint size="inherit">{t("client:supporter.change.description")}</Hint>
      {keepsLowerPrice && <p>{t("client:supporter.change.price_kept", { price: format(subscription.price) })}</p>}
      <PlanChooser
        plans={plans}
        plan={plan}
        onPlanChange={setPlanId}
        interval={interval}
        onIntervalChange={setInterval}
      />
      <div>
        <DialogTrigger>
          <Button isDisabled={unchanged}>{t("client:supporter.change")}</Button>
          <ConfirmDialog
            title={t("client:supporter.change.confirm", { plan: `⁨${plan.name}⁩` })}
            description={t(timingNote)}
            confirmLabel={t("client:supporter.change")}
            cancelLabel={t("client:supporter.cancel")}
            tone="primary"
            isPending={commands.pending === "change"}
            {...(commands.failed === "change" &&
              commands.error !== undefined && {
                errorMessage: t("client:supporter.command.failed", { reason: commands.error }),
              })}
            onConfirm={(close) => {
              void commands.run({ kind: "change", choice: { plan_id: plan.id, interval } }).then((done) => {
                if (done) close();
              });
            }}
          />
        </DialogTrigger>
      </div>
      {unchanged && <Hint>{t("client:supporter.change.unchanged")}</Hint>}
    </Section>
  );
}
