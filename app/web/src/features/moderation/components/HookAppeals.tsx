// SPDX-License-Identifier: AGPL-3.0-only
// The appeals of the posts that the review of local posts refused (ADR-0147), on the automated reviews screen: the
// author, the decision (the review, its reason code, when it decided), the text the author tried to post and their
// comment, both as plain text in their own direction (Y10). An appeal that awaits a moderator has "Uphold" and
// "Overturn", which ask first in an alert dialog (ConfirmDialog, NFR-MOD-7), as the appeals of moderation actions do:
// a decision cannot be changed. What the decision did stays in the section in a StatusMessage (role="status"), and
// focus moves to the section's heading, since the row's buttons give way to the decision. The list is in the admin
// mode's table form, with the filter (pending by default) kept in the URL by the route (web-client/README.md S3).
import {
  Button,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  EmptyState,
  ListToolbar,
  Select,
  SelectItem,
  Stack,
  StatusMessage,
  TableCell,
  TableColumnHeader,
  TableRow,
  TableRowHeader,
  Text,
  VisuallyHidden,
} from "@my/ui";
import { useRef, useState } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, type Translate, useT } from "../../../lib/i18n";
import {
  type HookAppeal,
  type HookAppealDecision,
  type HookAppealFilter,
  hookAppealFilters,
} from "../api/hookDecisions";
import {
  type HookAppealReview,
  hookAppealStateLabels,
  hookNameLabels,
  hookReasonLabels,
  useHookAppealReview,
  useHookAppeals,
} from "../hooks/useHookDecisions";
import { DateTime, RecordTable, Section, useFocusAfterChange } from "./parts";
import { AccountByIdLabel } from "./reportParts";

const filterLabels: Readonly<Record<HookAppealFilter, ClientMessageKey>> = {
  pending: "client:admin.hook_appeals.filter.pending",
  all: "client:admin.hook_appeals.filter.all",
};

function isFilter(key: unknown): key is HookAppealFilter {
  return hookAppealFilters.some((item) => item === key);
}

const decisions = ["uphold", "overturn"] as const satisfies readonly HookAppealDecision[];

export type HookAppealsProps = {
  /** The selected filter, from the URL. */
  filter: HookAppealFilter;
  /** Selects another filter: the route puts it into the URL. */
  onFilterChange: (filter: HookAppealFilter) => void;
};

export function HookAppeals({ filter, onFilterChange }: HookAppealsProps) {
  const t = useT();
  const headingRef = useRef<HTMLHeadingElement>(null);
  // Counts the decisions, so that focus moves once the page shows each one.
  const [decided, setDecided] = useState(0);
  const expectFocus = useFocusAfterChange(headingRef, decided);
  const review = useHookAppealReview(() => {
    expectFocus();
    setDecided((count) => count + 1);
  });
  return (
    <Section title={t("client:admin.hook_appeals.title")} headingRef={headingRef}>
      <Stack gap="3">
        <ListToolbar label={t("client:admin.list.filters")}>
          <Select
            variant="toolbar"
            aria-label={t("client:admin.hook_appeals.filter.label")}
            value={filter}
            onChange={(key) => {
              if (isFilter(key)) onFilterChange(key);
            }}
          >
            {hookAppealFilters.map((item) => (
              <SelectItem key={item} id={item}>
                {t(filterLabels[item])}
              </SelectItem>
            ))}
          </Select>
        </ListToolbar>
        <StatusMessage>{review.status}</StatusMessage>
        {/* Another filter is another list: it starts at its first page. */}
        <HookAppealList key={filter} filter={filter} review={review} />
      </Stack>
      <DecisionDialog review={review} />
    </Section>
  );
}

function HookAppealList({ filter, review }: { filter: HookAppealFilter; review: HookAppealReview }) {
  const t = useT();
  const list = useHookAppeals(filter);
  return (
    <RecordTable
      list={list}
      caption={t("client:admin.hook_appeals.title")}
      getId={(appeal) => appeal.id}
      empty={
        <EmptyState
          headingLevel={3}
          heading={t(filter === "pending" ? "client:admin.hook_appeals.empty" : "client:admin.hook_appeals.empty_all")}
        />
      }
      head={
        <>
          <TableColumnHeader>{t("client:admin.hook_appeals.column.account")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.hook_appeals.column.decision")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.hook_appeals.column.appeal")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.hook_appeals.column.state")}</TableColumnHeader>
          <TableColumnHeader>
            <VisuallyHidden>{t("client:admin.list.column.actions")}</VisuallyHidden>
          </TableColumnHeader>
        </>
      }
    >
      {(appeal) => <HookAppealRow appeal={appeal} review={review} />}
    </RecordTable>
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

function HookAppealRow({ appeal, review }: { appeal: HookAppeal; review: HookAppealReview }) {
  const t = useT();
  const intl = useIntl();
  const { decision } = appeal;
  const date = intl.formatDate(appeal.submitted_at, { dateStyle: "medium", timeStyle: "short" });
  return (
    <TableRow data-row-id={appeal.id}>
      <TableRowHeader wrap="anywhere">
        <AccountByIdLabel id={appeal.account_id} />
      </TableRowHeader>
      <TableCell wrap="anywhere">
        <DescriptionList>
          <DescriptionItem term={t("client:admin.hook_decisions.column.hook")}>
            {t(hookNameLabels[decision.hook])}
          </DescriptionItem>
          {decision.reason !== undefined && (
            <DescriptionItem term={t("client:admin.hook_decisions.reason")}>
              {t(hookReasonLabels[decision.reason])}
            </DescriptionItem>
          )}
          {decision.note !== undefined && (
            <DescriptionItem term={t("client:admin.hook_decisions.note")}>
              {/* Written by the plugin, in any language. */}
              <Text dir="auto">{decision.note}</Text>
            </DescriptionItem>
          )}
          <DescriptionItem term={t("client:admin.hook_decisions.column.decided_at")}>
            <DateTime value={decision.decided_at} />
          </DescriptionItem>
        </DescriptionList>
      </TableCell>
      <TableCell wrap="anywhere">
        <DescriptionList>
          <DescriptionItem term={t("client:admin.hook_appeals.submitted_at")}>
            <DateTime value={appeal.submitted_at} />
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.hook_appeals.status_text")}>
            <AuthorText text={appeal.status_text} />
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.hook_appeals.comment")}>
            <AuthorText text={appeal.comment} />
          </DescriptionItem>
        </DescriptionList>
      </TableCell>
      <TableCell>
        <Stack gap="1">
          <Text weight="semibold">{t(hookAppealStateLabels[appeal.state])}</Text>
          {appeal.decided_at !== undefined && <DateTime value={appeal.decided_at} />}
        </Stack>
      </TableCell>
      <TableCell>
        {appeal.state === "pending" && (
          <Cluster gap="2">
            {decisions.map((item) => (
              <Button
                key={item}
                size="sm"
                variant={item === "uphold" ? "primary" : "secondary"}
                aria-label={t(
                  item === "uphold" ? "client:admin.hook_appeals.uphold_of" : "client:admin.hook_appeals.overturn_of",
                  { date },
                )}
                onPress={() => {
                  review.open(appeal, item);
                }}
              >
                {t(item === "uphold" ? "client:admin.hook_appeals.uphold" : "client:admin.hook_appeals.overturn")}
              </Button>
            ))}
          </Cluster>
        )}
      </TableCell>
    </TableRow>
  );
}

/** The texts of a decision's button and confirmation. */
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
