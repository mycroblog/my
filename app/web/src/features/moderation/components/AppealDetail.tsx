// SPDX-License-Identifier: AGPL-3.0-only
// An appeal's page below its heading (FR-707): the appeal (who, when, and what the user wrote) next to the
// moderation action it is of (what it does, why, its state, the moderators' reason and note), then the decision.
// While the appeal awaits one, "Uphold" and "Overturn" ask first in an alert dialog (ConfirmDialog, NFR-MOD-7)
// that takes the explanation the user is shown; an overturn lifts the action, and the action's state on the page
// says so. What the decision did stays in the decision region in a StatusMessage (role="status"), and focus
// moves to that region's heading, since its buttons give way to the decision.
import {
  Box,
  Button,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  DetailSection,
  Hint,
  Link,
  ReasonField,
  Stack,
  StatusMessage,
  Text,
} from "@my/ui";
import { useRef } from "react";
import { type Translate, useT } from "../../../lib/i18n";
import { type Appeal, type AppealDecision, appealReasonLimit } from "../api/appeals";
import { type AppealReview, appealStateLabels, useAppeal, useAppealReview } from "../hooks/useAppeals";
import { accountActionKindLabels, accountActionStateLabels, reportCategoryLabels } from "../hooks/useReports";
import { DateTime, RequirePermission, useFocusAfterChange } from "./parts";
import { AccountByIdLabel, AssigneeName } from "./reportParts";

export function AppealDetailView({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_accounts">
      <AppealPage id={id} />
    </RequirePermission>
  );
}

function AppealPage({ id }: { id: string }) {
  const t = useT();
  const appeal = useAppeal(id);
  const decisionHeadingRef = useRef<HTMLHeadingElement>(null);
  const focusDecision = useFocusAfterChange(decisionHeadingRef, appeal.state);
  const review = useAppealReview(id, focusDecision);
  return (
    <Stack gap="8">
      <DetailSection title={t("client:admin.appeal.appeal.title")}>
        <AppealSummary appeal={appeal} />
      </DetailSection>
      <DetailSection title={t("client:admin.appeal.action.title")}>
        <ActionSummary appeal={appeal} />
      </DetailSection>
      <DetailSection title={t("client:admin.appeal.decision.title")} headingRef={decisionHeadingRef}>
        <StatusMessage>{review.status}</StatusMessage>
        {appeal.state === "pending" ? <PendingDecision review={review} /> : <Decision appeal={appeal} />}
      </DetailSection>
      <DecisionDialog review={review} appeal={appeal} />
    </Stack>
  );
}

function AppealSummary({ appeal }: { appeal: Appeal }) {
  const t = useT();
  return (
    <Stack gap="3">
      <DescriptionList columns={3}>
        <DescriptionItem term={t("client:admin.appeal.state")}>
          <Text weight="semibold">{t(appealStateLabels[appeal.state])}</Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.appeal.account")}>
          <AccountByIdLabel id={appeal.account_id} />
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.appeal.submitted_at")}>
          <DateTime value={appeal.submitted_at} />
        </DescriptionItem>
      </DescriptionList>
      <Box>
        <Link href={`/moderation/accounts/${encodeURIComponent(appeal.account_id)}`} variant="standalone">
          {t("client:admin.appeal.account.admin")}
        </Link>
      </Box>
      <DescriptionList>
        <DescriptionItem term={t("client:admin.appeal.text")}>
          {/* Written by the user who appealed, in any language (Y10). */}
          <Text dir="auto" block align="start" wrap="anywhere" whitespace="preWrap">
            {appeal.text}
          </Text>
        </DescriptionItem>
      </DescriptionList>
    </Stack>
  );
}

/** A moderator's text, in any language, or the word for none. */
function ModeratorText({ text }: { text: string | undefined }) {
  const t = useT();
  if (text === undefined || text === "") return <Text tone="muted">{t("client:admin.appeal.none")}</Text>;
  return (
    <Text dir="auto" block align="start" wrap="anywhere" whitespace="preWrap">
      {text}
    </Text>
  );
}

function ActionSummary({ appeal }: { appeal: Appeal }) {
  const t = useT();
  const { action } = appeal;
  return (
    <Stack gap="3">
      <DescriptionList columns={3}>
        <DescriptionItem term={t("client:admin.appeal.action.kind")}>
          {t(accountActionKindLabels[action.kind])}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.account_actions.state")}>
          <Text weight="semibold">{t(accountActionStateLabels[action.state])}</Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.account_actions.category")}>
          {t(reportCategoryLabels[action.reason_category])}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.account_actions.issued_at")}>
          <DateTime value={action.issued_at} />
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.account_actions.expires_at")}>
          {action.expires_at === undefined ? (
            t("client:admin.account_actions.duration.indefinite")
          ) : (
            <DateTime value={action.expires_at} />
          )}
        </DescriptionItem>
        {action.lifted_at !== undefined && (
          <DescriptionItem term={t("client:admin.appeal.action.lifted_at")}>
            <DateTime value={action.lifted_at} />
          </DescriptionItem>
        )}
        {action.status_ids !== undefined && (
          <DescriptionItem term={t("client:admin.report.posts")}>
            <Text numeric>{t("client:admin.report.posts.count", { count: action.status_ids.length })}</Text>
          </DescriptionItem>
        )}
        {action.report_id !== undefined && (
          <DescriptionItem term={t("client:admin.account_actions.report")}>
            <Link href={`/moderation/reports/${encodeURIComponent(action.report_id)}`} variant="inline">
              {t("client:admin.appeal.action.report")}
            </Link>
          </DescriptionItem>
        )}
      </DescriptionList>
      <DescriptionList columns={2}>
        <DescriptionItem term={t("client:admin.account_actions.public_reason")}>
          <ModeratorText text={action.public_reason} />
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.account_actions.private_note")}>
          <ModeratorText text={action.private_note} />
        </DescriptionItem>
      </DescriptionList>
    </Stack>
  );
}

/** The texts of a decision's button and confirmation. */
type DecisionTexts = { action: string; title: string; description: string; tone: "primary" | "danger" };

function decisionTexts(t: Translate, decision: AppealDecision, appeal: Appeal): DecisionTexts {
  const kind = t(accountActionKindLabels[appeal.action.kind]);
  if (decision === "uphold") {
    return {
      action: t("client:admin.appeal.uphold"),
      title: t("client:admin.appeal.confirm_uphold.title"),
      description: t("client:admin.appeal.confirm_uphold.description", { kind }),
      tone: "primary",
    };
  }
  return {
    action: t("client:admin.appeal.overturn"),
    title: t("client:admin.appeal.confirm_overturn.title"),
    description:
      appeal.action.kind === "ban"
        ? t("client:admin.appeal.confirm_overturn.description_ban")
        : t("client:admin.appeal.confirm_overturn.description", { kind }),
    tone: "danger",
  };
}

const decisions = ["uphold", "overturn"] as const satisfies readonly AppealDecision[];

function PendingDecision({ review }: { review: AppealReview }) {
  const t = useT();
  return (
    <Stack align="start" gap="3">
      <Hint>{t("client:admin.appeal.decision.description")}</Hint>
      <Cluster gap="2">
        {decisions.map((decision) => (
          <Button
            key={decision}
            variant={decision === "uphold" ? "primary" : "secondary"}
            onPress={() => {
              review.open(decision);
            }}
          >
            {decision === "uphold" ? t("client:admin.appeal.uphold") : t("client:admin.appeal.overturn")}
          </Button>
        ))}
      </Cluster>
    </Stack>
  );
}

function Decision({ appeal }: { appeal: Appeal }) {
  const t = useT();
  return (
    <DescriptionList columns={3}>
      <DescriptionItem term={t("client:admin.appeal.decision")}>
        <Text weight="semibold">{t(appealStateLabels[appeal.state])}</Text>
      </DescriptionItem>
      {appeal.decided_at !== undefined && (
        <DescriptionItem term={t("client:admin.appeal.decided_at")}>
          <DateTime value={appeal.decided_at} />
        </DescriptionItem>
      )}
      <DescriptionItem term={t("client:admin.appeal.decided_by")}>
        {appeal.decided_by_account_id === undefined ? (
          t("client:admin.appeal.decided_by.cli")
        ) : (
          <AssigneeName id={appeal.decided_by_account_id} />
        )}
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.appeal.decision_reason")}>
        <ModeratorText text={appeal.decision_reason} />
      </DescriptionItem>
    </DescriptionList>
  );
}

function DecisionDialog({ review, appeal }: { review: AppealReview; appeal: Appeal }) {
  const t = useT();
  const { confirmation } = review;
  // The last confirmation stays while the dialog fades out; before the first one the dialog is closed and empty.
  const texts = decisionTexts(t, confirmation?.decision ?? "uphold", appeal);
  return (
    <ConfirmDialog
      isOpen={review.isOpen}
      onOpenChange={(isOpen) => {
        if (!isOpen) review.cancel();
      }}
      title={texts.title}
      description={texts.description}
      confirmLabel={texts.action}
      cancelLabel={t("client:admin.appeal.cancel")}
      tone={texts.tone}
      onConfirm={() => {
        review.confirm();
      }}
      isPending={review.isPending}
      {...(review.error === undefined
        ? {}
        : { errorMessage: t("client:admin.appeal.decide_failed", { reason: review.error }) })}
    >
      <ReasonField
        label={t("client:admin.appeal.reason")}
        description={t("client:admin.appeal.reason.description")}
        value={confirmation?.reason ?? ""}
        onChange={review.changeReason}
        characterLimit={appealReasonLimit}
        formatRemaining={(remaining) =>
          remaining < 0
            ? t("client:admin.account_actions.characters_over", { count: -remaining })
            : t("client:admin.account_actions.characters_left", { count: remaining })
        }
        errorMessage={t("client:admin.appeal.reason.too_long", { max: appealReasonLimit })}
      />
    </ConfirmDialog>
  );
}
