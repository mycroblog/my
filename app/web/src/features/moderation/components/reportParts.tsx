// SPDX-License-Identifier: AGPL-3.0-only
// Pieces of the report queue and the report's page: an account as a row shows it (its display name and handle,
// or that it is gone) and an account known by ID only (the moderator who handles a report or wrote a note, or the
// user who appealed an action), the category of a report and how long it has waited, as badges (ADR-0110 P3).
import {
  AgeBadge,
  AssignedIcon,
  Cluster,
  LegalIcon,
  ReportIcon,
  RulesIcon,
  SpamIcon,
  Stack,
  StatusBadge,
  type StatusBadgeProps,
  Text,
  UnassignedIcon,
} from "@my/ui";
import type { ReactNode } from "react";
import { useT } from "../../../lib/i18n";
import type { Account } from "../../profile";
import { useViewer } from "../../session";
import type { ReportCategory } from "../api/reports";
import { accountName, handleOf, reportCategoryLabels, useAccountById } from "../hooks/useReports";
import { reportWaitThresholds, useWaitView } from "../hooks/useWait";

/** The look of each category: legal reports in the danger tone, rule violations in the warning tone. */
const categoryLooks: Readonly<Record<ReportCategory, { icon: ReactNode; tone: StatusBadgeProps["tone"] }>> = {
  legal: { icon: <LegalIcon />, tone: "danger" },
  violation: { icon: <RulesIcon />, tone: "warning" },
  spam: { icon: <SpamIcon />, tone: "neutral" },
  other: { icon: <ReportIcon />, tone: "neutral" },
};

/** A report's category as a badge: its icon, its name, and a tone by how grave it is. */
export function ReportCategoryBadge({ category }: { category: ReportCategory }) {
  const t = useT();
  const look = categoryLooks[category];
  return <StatusBadge icon={look.icon} label={t(reportCategoryLabels[category])} tone={look.tone} />;
}

/** How long an open report has waited since it was filed. */
export function ReportWaitBadge({ filedAt }: { filedAt: string }) {
  const wait = useWaitView(filedAt, reportWaitThresholds);
  return <AgeBadge {...wait} />;
}

/** Who handles an open report, as a badge: "you", another moderator's name, or nobody yet. */
export function ReportAssigneeBadge({ id }: { id: string | undefined }) {
  const t = useT();
  const viewer = useViewer();
  if (id === undefined) {
    return <StatusBadge icon={<UnassignedIcon />} label={t("client:admin.report.assignee.unassigned")} />;
  }
  if (viewer.status === "signed-in" && viewer.account.id === id) {
    return <StatusBadge icon={<AssignedIcon />} label={t("client:admin.report.assignee.you")} tone="accent" />;
  }
  return (
    <Cluster gap="2" align="center">
      <StatusBadge icon={<AssignedIcon />} label={t("client:admin.report.assignee.other")} />
      <AccountByIdLabel id={id} />
    </Cluster>
  );
}

/** An account as its display name over its handle; `undefined` is an account that is gone. */
export function AccountLabel({ account }: { account: Account | undefined }) {
  const t = useT();
  if (account === undefined) return t("client:admin.report.account.deleted");
  // Without a display name, the handle is the name: it is shown once.
  if (account.display_name === "") {
    return (
      <Text dir="ltr" align="start" wrap="anywhere">
        {handleOf(account)}
      </Text>
    );
  }
  return (
    <Stack as="span" shrink>
      {/* A name the user chose, in any language (web-client/16-look-states-and-wording.md G9). */}
      <Text dir="auto" align="start" wrap="anywhere">
        {accountName(account)}
      </Text>
      <Text dir="ltr" align="start" size="sm" wrap="anywhere" tone="muted">
        {handleOf(account)}
      </Text>
    </Stack>
  );
}

/** A moderator by account ID: "you" for the viewer, else the account's name once loaded. */
export function AssigneeName({ id }: { id: string }) {
  const t = useT();
  const viewer = useViewer();
  if (viewer.status === "signed-in" && viewer.account.id === id) return t("client:admin.report.assignee.you");
  return <AccountByIdLabel id={id} />;
}

/** An account known by ID only (a moderator, or the user who appealed): its name once loaded, or that it is gone. */
export function AccountByIdLabel({ id }: { id: string }) {
  const t = useT();
  const account = useAccountById(id);
  switch (account.status) {
    case "loading":
      return <Text tone="muted">{t("client:admin.report.account.loading")}</Text>;
    case "missing":
      return t("client:admin.report.account.deleted");
    case "found":
      return <AccountLabel account={account.account} />;
  }
}
