// SPDX-License-Identifier: AGPL-3.0-only
// The parts that the pages of the automated reviews screen share (ADR-0147 §9, web-client §14): the local navigation
// between the decisions and the appeals of refused posts, and the facts of one decision, which its page and the page of
// an appeal of it show.
import { DescriptionItem, DescriptionList, LocalNav, Text } from "@my/ui";
import { useIntl } from "react-intl";
import { useNow } from "../../../lib/clock";
import { useT } from "../../../lib/i18n";
import type { HookDecision } from "../api/hookDecisions";
import { hookModeLabels, hookNameLabels, hookOutcomeLabels, hookReasonLabels } from "../hooks/useHookDecisions";
import { DateTime } from "./parts";
import { AccountByIdLabel } from "./reportParts";

/** The pages of the automated reviews screen. */
export type HookReviewPage = "decisions" | "appeals";

/** The links between the pages of the automated reviews screen, under its `h1`; `current` is the page on screen. */
export function HookReviewPages({ current }: { current: HookReviewPage }) {
  const t = useT();
  return (
    <LocalNav
      label={t("client:admin.hook_decisions.pages")}
      items={[
        {
          href: "/moderation/hook-decisions",
          label: t("client:admin.hook_decisions.pages.decisions"),
          current: current === "decisions",
        },
        {
          href: "/moderation/hook-decisions/appeals",
          label: t("client:admin.hook_decisions.pages.appeals"),
          current: current === "appeals",
        },
      ]}
    />
  );
}

/**
 * What a decision is: the review, its subject, outcome and mode, when it decided, the account of its subject (unless
 * `account` is false, as on an appeal's page, which names its author), the wait, the reason code, the plugin's note,
 * and its cancellation. A dropped activity says until when cancelling brings it back.
 */
export function HookDecisionFacts({ decision, account = true }: { decision: HookDecision; account?: boolean }) {
  const t = useT();
  const intl = useIntl();
  const now = useNow();
  return (
    <DescriptionList columns={2}>
      <DescriptionItem term={t("client:admin.hook_decisions.column.hook")}>
        {t(hookNameLabels[decision.hook])}
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.hook_decisions.column.subject")}>
        {/* A URI or another key, which reads left to right in every locale. */}
        <Text dir="ltr" align="start" wrap="anywhere">
          {decision.subject_key}
        </Text>
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.hook_decisions.column.outcome")}>
        <Text weight="semibold">{t(hookOutcomeLabels[decision.outcome])}</Text>
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.hook_decisions.column.mode")}>
        {t(hookModeLabels[decision.mode])}
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.hook_decisions.column.decided_at")}>
        <DateTime value={decision.decided_at} />
      </DescriptionItem>
      {account && decision.account_id !== undefined && (
        <DescriptionItem term={t("client:admin.hook_decisions.account")}>
          <AccountByIdLabel id={decision.account_id} />
        </DescriptionItem>
      )}
      <DescriptionItem term={t("client:admin.hook_decisions.latency")}>
        {t("client:admin.hook_decisions.latency.value", { value: intl.formatNumber(decision.latency_ms) })}
      </DescriptionItem>
      {decision.reason !== undefined && (
        <DescriptionItem term={t("client:admin.hook_decisions.reason")}>
          {t(hookReasonLabels[decision.reason])}
        </DescriptionItem>
      )}
      {decision.note !== undefined && (
        <DescriptionItem term={t("client:admin.hook_decisions.note")}>
          {/* Written by the plugin, in any language. */}
          <Text dir="auto" align="start" wrap="anywhere">
            {decision.note}
          </Text>
        </DescriptionItem>
      )}
      {decision.overridden_at !== undefined && (
        <DescriptionItem term={t("client:admin.hook_decisions.overridden_at")}>
          <DateTime value={decision.overridden_at} />
        </DescriptionItem>
      )}
      {decision.overridable && decision.restorable_until !== undefined && (
        <DescriptionItem term={t("client:admin.hook_decisions.restorable_until")}>
          {Date.parse(decision.restorable_until) > now ? (
            <DateTime value={decision.restorable_until} />
          ) : (
            t("client:admin.hook_decisions.restorable_gone")
          )}
        </DescriptionItem>
      )}
    </DescriptionList>
  );
}
