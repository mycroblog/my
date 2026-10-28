// SPDX-License-Identifier: AGPL-3.0-only
// The registrations screen below its heading (FR-721): the sign-ups that await approval, longest waiting
// first, in the admin mode's table form (one page at a time, previous / next), each row with the reason its
// applicant gave and buttons that approve or reject it. A decision cannot be
// changed afterwards, so both ask first in an alert dialog (ConfirmDialog, NFR-MOD-7) that takes an optional
// note for the moderators. A decided registration leaves the list, focus moves to the next one's buttons
// (lib/useFocusAfterRemoval), and the result stays above the list in a StatusMessage (role="status").
import {
  Box,
  Button,
  Cluster,
  ConfirmDialog,
  EmptyState,
  ReasonField,
  Stack,
  StatusMessage,
  TableCell,
  TableColumnHeader,
  TableRow,
  TableRowHeader,
  Text,
  VisuallyHidden,
} from "@my/ui";
import { useRef } from "react";
import { type Translate, useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { type Registration, type RegistrationDecision, registrationNoteLimit } from "../api/registrations";
import {
  applicantInText,
  type RegistrationReview,
  useRegistrationReview,
  useRegistrations,
} from "../hooks/useRegistrations";
import { DateTime, RecordTable, RequirePermission } from "./parts";

/** The texts of a decision's button and confirmation, about `applicant`. */
type DecisionTexts = {
  action: string;
  actionOf: string;
  title: string;
  description: string;
  tone: "primary" | "danger";
};

function decisionTexts(t: Translate, decision: RegistrationDecision, applicant: string): DecisionTexts {
  if (decision === "approve") {
    return {
      action: t("client:admin.registrations.approve"),
      actionOf: t("client:admin.registrations.approve_applicant", { applicant }),
      title: t("client:admin.registrations.confirm_approve.title", { applicant }),
      description: t("client:admin.registrations.confirm_approve.description", { applicant }),
      tone: "primary",
    };
  }
  return {
    action: t("client:admin.registrations.reject"),
    actionOf: t("client:admin.registrations.reject_applicant", { applicant }),
    title: t("client:admin.registrations.confirm_reject.title", { applicant }),
    description: t("client:admin.registrations.confirm_reject.description", { applicant }),
    tone: "danger",
  };
}

const decisions = ["approve", "reject"] as const satisfies readonly RegistrationDecision[];

export function RegistrationList() {
  return (
    <RequirePermission permission="manage_accounts">
      <Registrations />
    </RequirePermission>
  );
}

function Registrations() {
  const t = useT();
  const listRef = useRef<HTMLDivElement>(null);
  const list = useRegistrations();
  const removed = useFocusAfterRemoval(
    listRef,
    list.items.map((registration) => registration.user_id),
  );
  const review = useRegistrationReview(removed);
  return (
    <Stack gap="4">
      <StatusMessage>{review.status}</StatusMessage>
      <Box ref={listRef}>
        <RecordTable
          list={list}
          caption={t("client:admin.registrations.title")}
          getId={(registration) => registration.user_id}
          empty={
            <EmptyState
              headingLevel={2}
              heading={t("client:admin.registrations.empty")}
              description={t("client:admin.registrations.empty.description")}
            />
          }
          head={
            <>
              <TableColumnHeader>{t("client:admin.registrations.column.applicant")}</TableColumnHeader>
              <TableColumnHeader>{t("client:admin.registrations.email_verified")}</TableColumnHeader>
              <TableColumnHeader>{t("client:admin.registrations.invite")}</TableColumnHeader>
              <TableColumnHeader>{t("client:admin.registrations.submitted_at")}</TableColumnHeader>
              <TableColumnHeader>{t("client:admin.registrations.reason")}</TableColumnHeader>
              <TableColumnHeader>
                <VisuallyHidden>{t("client:admin.list.column.actions")}</VisuallyHidden>
              </TableColumnHeader>
            </>
          }
        >
          {(registration) => <RegistrationRow registration={registration} review={review} />}
        </RecordTable>
      </Box>
      <DecisionDialog review={review} />
    </Stack>
  );
}

function RegistrationRow({ registration, review }: { registration: Registration; review: RegistrationReview }) {
  const t = useT();
  const applicant = applicantInText(t, registration);
  const handle = registration.username === undefined ? undefined : `@${registration.username}`;
  return (
    <TableRow data-row-id={registration.user_id}>
      <TableRowHeader wrap="anywhere">
        <Stack gap="1">
          <Text weight="semibold" wrap="anywhere">
            {/* A username reads left to right in every locale. */}
            {handle === undefined ? t("client:admin.registrations.account_gone") : <span dir="ltr">{handle}</span>}
          </Text>
          {registration.email === "" ? (
            <Text size="sm" tone="muted">
              {t("client:admin.registrations.email.none")}
            </Text>
          ) : (
            <Text size="sm" tone="muted" dir="ltr" align="start" wrap="anywhere">
              {registration.email}
            </Text>
          )}
        </Stack>
      </TableRowHeader>
      <TableCell>
        {registration.email_verified
          ? t("client:admin.registrations.email_verified.yes")
          : t("client:admin.registrations.email_verified.no")}
      </TableCell>
      <TableCell>
        {registration.invite_id === undefined
          ? t("client:admin.registrations.invite.none")
          : t("client:admin.registrations.invite.used")}
      </TableCell>
      <TableCell>
        <DateTime value={registration.submitted_at} />
      </TableCell>
      <TableCell wrap="anywhere">
        {registration.reason === undefined ? (
          t("client:admin.registrations.reason.none")
        ) : (
          // Written by the applicant, in any language and direction (web-client/README.md Y10).
          <Text as="p" dir="auto" align="start" whitespace="preWrap" wrap="anywhere">
            {registration.reason}
          </Text>
        )}
      </TableCell>
      <TableCell>
        <Cluster gap="2">
          {decisions.map((decision) => {
            const texts = decisionTexts(t, decision, applicant);
            return (
              <Button
                key={decision}
                size="sm"
                variant={decision === "approve" ? "primary" : "secondary"}
                aria-label={texts.actionOf}
                onPress={() => {
                  review.open(registration, decision);
                }}
              >
                {texts.action}
              </Button>
            );
          })}
        </Cluster>
      </TableCell>
    </TableRow>
  );
}

function DecisionDialog({ review }: { review: RegistrationReview }) {
  const t = useT();
  const { confirmation } = review;
  // The last confirmation stays while the dialog fades out; before the first one the dialog is closed and empty.
  const texts = decisionTexts(
    t,
    confirmation?.decision ?? "approve",
    confirmation === null ? "" : applicantInText(t, confirmation.registration),
  );
  return (
    <ConfirmDialog
      isOpen={review.isOpen}
      onOpenChange={(isOpen) => {
        if (!isOpen) review.cancel();
      }}
      title={texts.title}
      description={texts.description}
      confirmLabel={texts.action}
      cancelLabel={t("client:admin.registrations.cancel")}
      tone={texts.tone}
      onConfirm={() => {
        review.confirm();
      }}
      isPending={review.isPending}
      {...(review.error === undefined
        ? {}
        : { errorMessage: t("client:admin.registrations.decide_failed", { reason: review.error }) })}
    >
      <ReasonField
        label={t("client:admin.registrations.note")}
        description={t("client:admin.registrations.note.description")}
        value={confirmation?.note ?? ""}
        onChange={review.changeNote}
        characterLimit={registrationNoteLimit}
        formatRemaining={(remaining) =>
          remaining < 0
            ? t("client:admin.registrations.characters_over", { count: -remaining })
            : t("client:admin.registrations.characters_left", { count: remaining })
        }
        errorMessage={t("client:admin.registrations.note.too_long", { max: registrationNoteLimit })}
      />
    </ConfirmDialog>
  );
}
