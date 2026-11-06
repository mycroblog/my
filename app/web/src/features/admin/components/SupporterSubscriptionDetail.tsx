// SPDX-License-Identifier: AGPL-3.0-only
// One subscription's page (ADR-0072 §10, manage_supporters; web-client/screens/admin-supporters-subscription.md): who
// it is of, its plan (which leads to the plan's page), its state, price and dates, a change of interval scheduled for
// the next period, and ending it at once after an alert dialog: the role goes now, without a refund unless still
// pending, and it cannot be undone. What ending it did stays in the page's StatusMessage.
import {
  Button,
  Card,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  DialogTrigger,
  Link,
  Stack,
  StatusMessage,
} from "@my/ui";
import { useRef, useState } from "react";
import { useT } from "../../../lib/i18n";
import { useFormatMoney } from "../../costs";
import { DateTime, RequirePermission, Section, useFocusAfterChange } from "../../moderation";
import type { SupporterSubscription } from "../api/supporters";
import { useSubscriptionActions, useSupporterSubscription } from "../hooks/useSupporters";
import { isLive, subscriberName, subscriptionStateLabels } from "./supporterParts";

export function SupporterSubscriptionDetail({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_supporters">
      <SubscriptionPage id={id} />
    </RequirePermission>
  );
}

function SubscriptionPage({ id }: { id: string }) {
  const t = useT();
  const subscription = useSupporterSubscription(id);
  const actions = useSubscriptionActions();
  const factsRef = useRef<HTMLHeadingElement>(null);
  const live = isLive(subscription.state);
  // Ending the subscription takes the button away: focus goes to the heading of its facts.
  const expectEnded = useFocusAfterChange(factsRef, live);
  const who = subscriberName(t, subscription);
  return (
    <Stack gap="8">
      <Section surface="card" headingRef={factsRef} title={<span dir="ltr">{who}</span>}>
        <Card>
          <Stack gap="4">
            <StatusMessage>{actions.status}</StatusMessage>
            <SubscriptionFacts subscription={subscription} />
          </Stack>
        </Card>
      </Section>
      {live && (
        <EndSubscription
          who={who}
          error={actions.error}
          end={() => {
            expectEnded();
            return actions.cancel(subscription);
          }}
        />
      )}
    </Stack>
  );
}

function SubscriptionFacts({ subscription }: { subscription: SupporterSubscription }) {
  const t = useT();
  return (
    <DescriptionList columns={2}>
      <DescriptionItem term={t("client:admin.supporters.subscription.state")}>
        {t(subscriptionStateLabels[subscription.state])}
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.supporters.subscription.plan")}>
        <Link href={`/admin/supporters/plans/${encodeURIComponent(subscription.plan_id)}`} variant="standalone">
          <span dir="auto" lang="">
            {subscription.plan_name}
          </span>
        </Link>
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.supporters.subscription.price")}>
        <SubscriptionPrice subscription={subscription} />
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.supporters.subscription.started_at")}>
        <DateTime value={subscription.started_at} />
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.supporters.subscription.period_ends_at")}>
        {subscription.period_ends_at === undefined ? (
          t("client:admin.supporters.subscription.no_period")
        ) : (
          <DateTime value={subscription.period_ends_at} />
        )}
      </DescriptionItem>
      {subscription.grace_ends_at !== undefined && (
        <DescriptionItem term={t("client:admin.supporters.subscription.grace_ends_at")}>
          <DateTime value={subscription.grace_ends_at} />
        </DescriptionItem>
      )}
      {subscription.scheduled_interval !== undefined && (
        <DescriptionItem term={t("client:admin.supporters.subscription.scheduled")}>
          {t(
            subscription.scheduled_interval === "year"
              ? "client:admin.supporters.subscription.scheduled_year"
              : "client:admin.supporters.subscription.scheduled_month",
          )}
        </DescriptionItem>
      )}
    </DescriptionList>
  );
}

/** The price of a subscription a month or a year. */
function SubscriptionPrice({ subscription }: { subscription: SupporterSubscription }) {
  const t = useT();
  const format = useFormatMoney();
  return t(
    subscription.interval === "year"
      ? "client:admin.supporters.subscription.per_year"
      : "client:admin.supporters.subscription.per_month",
    { amount: format(subscription.price) },
  );
}

type EndSubscriptionProps = {
  /** Who the subscription is of, for the dialog's title. */
  who: string;
  /** Ends the subscription; resolves with whether it succeeded. */
  end: () => Promise<boolean>;
  /** Why the last attempt failed, in the server's words. */
  error: string | undefined;
};

/** Ending the subscription at once, after an alert dialog: the role goes now, without a refund unless still pending. */
function EndSubscription({ who, end, error }: EndSubscriptionProps) {
  const t = useT();
  const name = `⁦${who}⁩`;
  const [isEnding, setIsEnding] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <Section
      surface="card"
      title={t("client:admin.supporters.subscription.end.section")}
      description={t("client:admin.supporters.subscription.cancel.description")}
    >
      <Card>
        <DialogTrigger>
          <Button variant="danger">{t("client:admin.supporters.subscription.cancel")}</Button>
          <ConfirmDialog
            title={t("client:admin.supporters.subscription.cancel.title", { name })}
            description={t("client:admin.supporters.subscription.cancel.description")}
            confirmLabel={t("client:admin.supporters.subscription.cancel")}
            cancelLabel={t("client:admin.supporters.cancel")}
            tone="danger"
            isPending={isEnding}
            {...(failed &&
              error !== undefined && {
                errorMessage: t("client:admin.supporters.subscription.cancel_failed", { reason: error }),
              })}
            onConfirm={(close) => {
              setIsEnding(true);
              setFailed(false);
              void end().then((ended) => {
                setIsEnding(false);
                if (ended) close();
                else setFailed(true);
              });
            }}
          />
        </DialogTrigger>
      </Card>
    </Section>
  );
}
