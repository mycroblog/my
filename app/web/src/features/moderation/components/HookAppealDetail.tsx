// SPDX-License-Identifier: AGPL-3.0-only
// An appeal's page below its heading (ADR-0147 §9): the appeal (its state, the author, when they sent it, the text they
// tried to post and their comment, both as plain text in their own direction, Y10), the decision it is of, which links
// to the decision's page, then the review. While the appeal awaits a moderator, "Uphold" and "Overturn" ask first in
// an alert dialog (ConfirmDialog, NFR-MOD-7), as the appeals of moderation actions do: a review cannot be changed. What
// the review did stays in the review region in a StatusMessage (role="status"), and focus moves to that region's
// heading, since its buttons give way to the review.
import {
  Box,
  Button,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  Hint,
  Link,
  Section,
  Stack,
  StatusMessage,
  Text,
} from "@my/ui";
import { useRef } from "react";
import { type Translate, useT } from "../../../lib/i18n";
import { moderationAccountPath, moderationHookDecisionPath } from "../../../lib/paths";
import type { HookAppeal, HookAppealDecision } from "../api/hookDecisions";
import {
  type HookAppealReview,
  hookAppealStateLabels,
  useHookAppeal,
  useHookAppealReview,
} from "../hooks/useHookDecisions";
import { HookDecisionFacts } from "./hookDecisionParts";
import { DateTime, RequirePermission, useFocusAfterChange } from "./parts";
import { AccountByIdLabel } from "./reportParts";

const decisions = ["uphold", "overturn"] as const satisfies readonly HookAppealDecision[];

export function HookAppealDetailView({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_reports">
      <HookAppealPage id={id} />
    </RequirePermission>
  );
}

function HookAppealPage({ id }: { id: string }) {
  const t = useT();
  const appeal = useHookAppeal(id);
  const reviewHeadingRef = useRef<HTMLHeadingElement>(null);
  const focusReview = useFocusAfterChange(reviewHeadingRef, appeal.state);
  const review = useHookAppealReview(focusReview);
  return (
    <Stack gap="8">
      <Section surface="card" title={t("client:admin.hook_appeal.appeal.title")}>
        <AppealSummary appeal={appeal} />
      </Section>
      <Section surface="card" title={t("client:admin.hook_appeal.decision.title")}>
        <Stack gap="3">
          <HookDecisionFacts decision={appeal.decision} account={false} />
          <Box>
            <Link href={moderationHookDecisionPath(appeal.decision.id)} variant="standalone">
              {t("client:admin.hook_appeal.decision.open")}
            </Link>
          </Box>
        </Stack>
      </Section>
      <Section surface="card" title={t("client:admin.hook_appeal.review.title")} headingRef={reviewHeadingRef}>
        <StatusMessage>{review.status}</StatusMessage>
        {appeal.state === "pending" ? <PendingReview appeal={appeal} review={review} /> : <Review appeal={appeal} />}
      </Section>
      <DecisionDialog review={review} />
    </Stack>
  );
}

function AppealSummary({ appeal }: { appeal: HookAppeal }) {
  const t = useT();
  return (
    <Stack gap="3">
      <DescriptionList columns={3}>
        <DescriptionItem term={t("client:admin.hook_appeals.column.state")}>
          <Text weight="semibold">{t(hookAppealStateLabels[appeal.state])}</Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.hook_appeals.column.account")}>
          <AccountByIdLabel id={appeal.account_id} />
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.hook_appeals.submitted_at")}>
          <DateTime value={appeal.submitted_at} />
        </DescriptionItem>
      </DescriptionList>
      <Box>
        <Link href={moderationAccountPath(appeal.account_id)} variant="standalone">
          {t("client:admin.hook_appeal.account.admin")}
        </Link>
      </Box>
      <DescriptionList>
        <DescriptionItem term={t("client:admin.hook_appeals.status_text")}>
          <AuthorText text={appeal.status_text} />
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.hook_appeals.comment")}>
          <AuthorText text={appeal.comment} />
        </DescriptionItem>
      </DescriptionList>
    </Stack>
  );
}

/** A text the author wrote, in any language, or the word for none. */
function AuthorText({ text }: { text: string | undefined }) {
  const t = useT();
  if (text === undefined || text === "") return <Text tone="muted">{t("client:admin.hook_appeals.none")}</Text>;
  return (
    <Text dir="auto" block align="start" wrap="anywhere" whitespace="preWrap">
      {text}
    </Text>
  );
}

/** Upholds or overturns a pending appeal, after a confirmation. */
function PendingReview({ appeal, review }: { appeal: HookAppeal; review: HookAppealReview }) {
  const t = useT();
  return (
    <Stack align="start" gap="3">
      <Hint>{t("client:admin.hook_appeal.review.description")}</Hint>
      <Cluster gap="2">
        {decisions.map((item) => (
          <Button
            key={item}
            variant={item === "uphold" ? "primary" : "secondary"}
            onPress={() => {
              review.open(appeal, item);
            }}
          >
            {t(item === "uphold" ? "client:admin.hook_appeals.uphold" : "client:admin.hook_appeals.overturn")}
          </Button>
        ))}
      </Cluster>
    </Stack>
  );
}

/** How a moderator reviewed a decided appeal, and when. */
function Review({ appeal }: { appeal: HookAppeal }) {
  const t = useT();
  return (
    <DescriptionList columns={2}>
      <DescriptionItem term={t("client:admin.hook_appeal.review.result")}>
        <Text weight="semibold">{t(hookAppealStateLabels[appeal.state])}</Text>
      </DescriptionItem>
      {appeal.decided_at !== undefined && (
        <DescriptionItem term={t("client:admin.hook_appeal.review.decided_at")}>
          <DateTime value={appeal.decided_at} />
        </DescriptionItem>
      )}
    </DescriptionList>
  );
}

/** The texts of a review's button and confirmation. */
type DecisionTexts = { action: string; title: string; description: string; tone: "primary" | "danger" };

function decisionTexts(t: Translate, decision: HookAppealDecision): DecisionTexts {
  if (decision === "uphold") {
    return {
      action: t("client:admin.hook_appeals.uphold"),
      title: t("client:admin.hook_appeals.confirm_uphold.title"),
      description: t("client:admin.hook_appeals.confirm_uphold.description"),
      tone: "primary",
    };
  }
  return {
    action: t("client:admin.hook_appeals.overturn"),
    title: t("client:admin.hook_appeals.confirm_overturn.title"),
    description: t("client:admin.hook_appeals.confirm_overturn.description"),
    tone: "danger",
  };
}

function DecisionDialog({ review }: { review: HookAppealReview }) {
  const t = useT();
  // The last confirmation stays while the dialog fades out; before the first one the dialog is closed and empty.
  const texts = decisionTexts(t, review.confirmation?.decision ?? "uphold");
  return (
    <ConfirmDialog
      isOpen={review.isOpen}
      onOpenChange={(isOpen) => {
        if (!isOpen) review.cancel();
      }}
      title={texts.title}
      description={texts.description}
      confirmLabel={texts.action}
      cancelLabel={t("client:admin.hook_appeals.cancel")}
      tone={texts.tone}
      onConfirm={() => {
        review.confirm();
      }}
      isPending={review.isPending}
      {...(review.error === undefined
        ? {}
        : { errorMessage: t("client:admin.hook_appeals.decide_failed", { reason: review.error }) })}
    />
  );
}
