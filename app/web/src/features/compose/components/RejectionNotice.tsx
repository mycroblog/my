// SPDX-License-Identifier: AGPL-3.0-only
// The notice of a post that the server's automated review refused (ADR-0147): the fixed title and detail of the
// server's problem, which say nothing of why, and the appeal the author may send until its deadline, in an alert
// dialog (ConfirmDialog, as appealing a moderation action is: an appeal cannot be changed). The draft stays in the form.
// What came of the appeal stays in the notice in a StatusMessage (role="status"): sent, and once a moderator reviews it
// (read from the server, again when the window regains focus), upheld or overturned. The notice sits outside the post's
// <form>: the dialog's own form and its keys would otherwise reach the post's (React events bubble through portals).
import { Button, Callout, ConfirmDialog, ReasonField, StatusMessage, Text } from "@my/ui";
import { useQuery } from "@tanstack/react-query";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useNow } from "../../../lib/clock";
import { type Translate, useT } from "../../../lib/i18n";
import { hookAppealCommentLimit, hookAppealTextLimit, ownHookAppealQueryOptions } from "../api/compose";
import type { Rejection } from "../hooks/draftStore";
import { type RejectionAppeal, useRejectionAppeal } from "../hooks/useRejectionAppeal";

const dateTimeFormat = { dateStyle: "medium", timeStyle: "short" } as const;

export type RejectionNoticeProps = {
  rejection: Rejection;
  settle: (outcome: NonNullable<Rejection["outcome"]>) => void;
};

export function RejectionNotice({ rejection, settle }: RejectionNoticeProps) {
  const t = useT();
  const intl = useIntl();
  const now = useNow();
  const appeal = useRejectionAppeal(rejection, settle);
  const { appealDeadline: deadline, outcome } = rejection;
  const review = useAppealReview(rejection);
  const reviewMessage =
    review === null
      ? null
      : t(
          review.state === "overturned"
            ? "client:compose.rejected.appeal.overturned"
            : "client:compose.rejected.appeal.upheld",
          { date: intl.formatDate(review.decidedAt, dateTimeFormat) },
        );
  // An overturned appeal, or one sent, is good news; an upheld one or one that could not be sent is not.
  const good = review === null ? outcome?.sent !== false : review.state === "overturned";
  const tone = good ? "success" : "neutral";
  return (
    <Callout>
      <Text as="p" weight="semibold">
        {rejection.title}
      </Text>
      {rejection.detail !== null && <p>{rejection.detail}</p>}
      {outcome === null &&
        rejection.hookDecisionId !== null &&
        deadline !== null &&
        (Date.parse(deadline) > now ? (
          <>
            <p>{t("client:compose.rejected.appeal.until", { date: intl.formatDate(deadline, dateTimeFormat) })}</p>
            <Button size="sm" onPress={appeal.open}>
              {t("client:compose.rejected.appeal.open")}
            </Button>
          </>
        ) : (
          <p>{t("client:compose.rejected.appeal.closed", { date: intl.formatDate(deadline, dateTimeFormat) })}</p>
        ))}
      {/* In the page before the outcome, so that it is announced (web-client/README.md Y7). */}
      <StatusMessage tone={tone}>{reviewMessage ?? outcome?.message}</StatusMessage>
      <AppealDialog appeal={appeal} />
    </Callout>
  );
}

/** How a moderator reviewed the appeal of `rejection`: the outcome and when, or null while there is none to show. */
function useAppealReview(rejection: Rejection): { state: "upheld" | "overturned"; decidedAt: string } | null {
  const api = useApi();
  const id = rejection.hookDecisionId;
  // Only once the author appealed (or learned the decision was appealed already): before that there is no appeal.
  const appealed = rejection.outcome !== null && id !== null;
  const { data } = useQuery({ ...ownHookAppealQueryOptions(api, id ?? ""), enabled: appealed, retry: false });
  if (!appealed || data === undefined || data.state === "pending" || data.decided_at === undefined) return null;
  return { state: data.state, decidedAt: data.decided_at };
}

/** Why an appeal's text cannot be sent, or null when it can. */
function validateText(t: Translate, text: string): string | null {
  if (text.trim() === "") return t("client:compose.rejected.appeal.text.required");
  if (Array.from(text).length > hookAppealTextLimit)
    return t("client:compose.rejected.appeal.text.too_long", { max: hookAppealTextLimit });
  return null;
}

/** Why an appeal's comment cannot be sent, or null when it can. */
function validateComment(t: Translate, comment: string): string | null {
  if (Array.from(comment.trim()).length > hookAppealCommentLimit)
    return t("client:compose.rejected.appeal.comment.too_long", { max: hookAppealCommentLimit });
  return null;
}

function AppealDialog({ appeal }: { appeal: RejectionAppeal }) {
  const t = useT();
  const formatRemaining = (remaining: number) =>
    remaining < 0
      ? t("client:compose.rejected.appeal.characters_over", { count: -remaining })
      : t("client:compose.rejected.appeal.characters_left", { count: remaining });
  return (
    <ConfirmDialog
      isOpen={appeal.isOpen}
      onOpenChange={(isOpen) => {
        if (!isOpen) appeal.cancel();
      }}
      title={t("client:compose.rejected.appeal.confirm.title")}
      description={t("client:compose.rejected.appeal.confirm.description")}
      confirmLabel={t("client:compose.rejected.appeal.send")}
      cancelLabel={t("client:compose.rejected.appeal.cancel")}
      tone="primary"
      onConfirm={() => {
        appeal.submit();
      }}
      isPending={appeal.isPending}
      {...(appeal.error === undefined
        ? {}
        : { errorMessage: t("client:compose.rejected.appeal.failed", { reason: appeal.error }) })}
    >
      <ReasonField
        label={t("client:compose.rejected.appeal.text.label")}
        description={t("client:compose.rejected.appeal.text.description")}
        value={appeal.text}
        onChange={appeal.changeText}
        rows={5}
        isRequired
        // Text of white space only is empty to the server too, and too long a text is refused; either one stops the
        // dialog's form before it sends (the browser's constraint validation).
        validate={(value) => validateText(t, value)}
        characterLimit={hookAppealTextLimit}
        formatRemaining={formatRemaining}
        errorMessage={validateText(t, appeal.text) ?? t("client:compose.rejected.appeal.text.required")}
      />
      <ReasonField
        label={t("client:compose.rejected.appeal.comment.label")}
        description={t("client:compose.rejected.appeal.comment.description")}
        value={appeal.comment}
        onChange={appeal.changeComment}
        validate={(value) => validateComment(t, value)}
        characterLimit={hookAppealCommentLimit}
        formatRemaining={formatRemaining}
        errorMessage={
          validateComment(t, appeal.comment) ??
          t("client:compose.rejected.appeal.comment.too_long", { max: hookAppealCommentLimit })
        }
      />
    </ConfirmDialog>
  );
}
