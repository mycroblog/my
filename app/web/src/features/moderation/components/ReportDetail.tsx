// SPDX-License-Identifier: AGPL-3.0-only
// A report's page below its heading: everything a moderator needs to handle one report on one page (NFR-MOD-4,
// FR-710; screens/moderation-report.md), in @my/ui's TriageLayout: the summary with the report's state, category and
// wait as badges; the decision (who handles it, and resolving it, the action form's submit the one primary button);
// then the evidence. The report itself, the posts it points at (or the voice chat room, with its records and ending it,
// ADR-0100 §11), its target with the target's history (its other reports
// and its moderation actions), the moderator who handles it, the moderators' notes, and handling it: a
// moderation action on the target that resolves it, resolving it without one, or reopening it. What a change
// did stays on the page in a StatusMessage (role="status") next to what caused it; failures are toasts, and a
// failed note stays under its field (web-client/README.md E2).
import {
  AccountItem,
  Box,
  Button,
  CheckIcon,
  Cluster,
  DescriptionItem,
  DescriptionList,
  Section,
  EmptyState,
  Heading,
  Hint,
  Link,
  ReasonField,
  RegionError,
  ReportIcon,
  Spinner,
  Stack,
  StatusBadge,
  StatusMessage,
  Text,
  TriageLayout,
} from "@my/ui";
import { useRef, useState } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { StatusCard } from "../../status";
import { type AccountAction, type ReportDetail, reportNoteLimit } from "../api/reports";
import { useHasPermission } from "../hooks/usePermissions";
import { useRefetch } from "../hooks/useRefetch";
import {
  accountActionKindLabels,
  accountName,
  accountActionStateLabels,
  handleOf,
  isRemote,
  type ReportedStatus,
  reportCategoryLabels,
  reportedStatuses,
  reportOutcomeLabels,
  useCitedRules,
  useReport,
  useReportChanges,
  useReportNoteForm,
  useTargetActions,
} from "../hooks/useReports";
import { AccountActionForm } from "./AccountActionForm";
import { DateTime, RecordList, RequirePermission, useFocusAfterChange } from "./parts";
import { AccountLabel, AssigneeName, ReportCategoryBadge, ReportWaitBadge } from "./reportParts";
import { ReportVoiceRoom } from "./ReportVoiceRoom";
import { imageSources } from "../../../lib/images";

export function ReportDetailView({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_reports">
      <ReportPage id={id} />
    </RequirePermission>
  );
}

/** The region of the page that a message belongs to: the one whose button caused it. */
type Message = { region: "assignee" | "resolution"; text: string };

function ReportPage({ id }: { id: string }) {
  const t = useT();
  const report = useReport(id);
  const canAct = useHasPermission("manage_accounts");
  const canAudit = useHasPermission("view_audit");
  const posts = reportedStatuses(report);
  // Deleted posts stay to be reviewed, but no action can delete them or mark them sensitive.
  const actionable = posts.flatMap((post) =>
    post.status === "found" && post.deletedAt === undefined ? [post.value] : [],
  );
  const [message, setMessage] = useState<Message | undefined>(undefined);
  const resolutionHeadingRef = useRef<HTMLHeadingElement>(null);
  const focusResolution = useFocusAfterChange(resolutionHeadingRef, report.state);
  const changes = useReportChanges(id, (change, text) => {
    const isResolution = change.operation === "resolve" || change.operation === "reopen";
    setMessage({ region: isResolution ? "resolution" : "assignee", text });
    if (isResolution) focusResolution();
  });
  const target = report.target_account;
  return (
    <TriageLayout
      summary={
        <Section surface="card" title={t("client:admin.report.summary.title")}>
          <ReportSummary report={report} />
        </Section>
      }
      decision={
        <>
          <Section surface="card" title={t("client:admin.report.assignee.title")}>
            <Assignee
              report={report}
              pending={changes.pending !== undefined}
              onAssign={changes.assignToSelf}
              onUnassign={changes.unassign}
              isAssigning={changes.pending?.operation === "assign_to_self"}
              isUnassigning={changes.pending?.operation === "unassign"}
            />
            <StatusMessage>{message?.region === "assignee" ? message.text : undefined}</StatusMessage>
          </Section>
          <Section surface="card" title={t("client:admin.report.resolution.title")} headingRef={resolutionHeadingRef}>
            <StatusMessage>{message?.region === "resolution" ? message.text : undefined}</StatusMessage>
            {report.state === "resolved" ? (
              <Stack align="start" gap="3">
                <DescriptionList>
                  <DescriptionItem term={t("client:admin.report.outcome")}>
                    {report.outcome === undefined
                      ? t("client:admin.reports.filter.resolved")
                      : t(reportOutcomeLabels[report.outcome])}
                  </DescriptionItem>
                  <DescriptionItem term={t("client:admin.report.resolved_at")}>
                    <DateTime value={report.state_changed_at} />
                  </DescriptionItem>
                </DescriptionList>
                <Hint>{t("client:admin.report.reopen.description")}</Hint>
                <Button onPress={changes.reopen} isPending={changes.pending?.operation === "reopen"}>
                  {t("client:admin.report.reopen")}
                </Button>
              </Stack>
            ) : (
              <Stack gap="8">
                <Stack gap="3">
                  <Heading level={3}>{t("client:admin.report.action.title")}</Heading>
                  {target === undefined ? (
                    <p>{t("client:admin.report.action.deleted")}</p>
                  ) : canAct ? (
                    <AccountActionForm
                      target={{
                        accountId: target.id,
                        remote: isRemote(target),
                        reportId: report.id,
                        statusIds: actionable.map((post) => post.id),
                        category: report.category,
                      }}
                      handle={handleOf(target)}
                      posts={actionable}
                      onIssued={(action) => {
                        setMessage({
                          region: "resolution",
                          text: t("client:admin.report.action.issued", {
                            kind: t(accountActionKindLabels[action.kind]),
                          }),
                        });
                        focusResolution();
                      }}
                    />
                  ) : (
                    <p>{t("client:admin.report.action.missing_permission")}</p>
                  )}
                </Stack>
                {/* The other ways to resolve it, in the plain buttons: the action above is the primary decision (W1). */}
                <Stack align="start" gap="3">
                  <Heading level={3}>{t("client:admin.report.resolve.title")}</Heading>
                  <Hint>{t("client:admin.report.resolve.description")}</Hint>
                  <Cluster gap="2">
                    <Button
                      onPress={() => {
                        changes.resolve("dismissed");
                      }}
                      isPending={changes.pending?.operation === "resolve" && changes.pending.outcome === "dismissed"}
                    >
                      {t("client:admin.report.resolve.dismissed")}
                    </Button>
                    <Button
                      onPress={() => {
                        changes.resolve("actioned");
                      }}
                      isPending={changes.pending?.operation === "resolve" && changes.pending.outcome === "actioned"}
                    >
                      {t("client:admin.report.resolve.actioned")}
                    </Button>
                  </Cluster>
                </Stack>
              </Stack>
            )}
          </Section>
        </>
      }
      evidence={
        <>
          {report.voice_room !== undefined && <ReportVoiceRoom report={report} room={report.voice_room} />}
          {/* A room's report points at no posts. */}
          {report.voice_room === undefined && (
            <Section surface="card" title={t("client:admin.report.posts.title")}>
              <ReportedPosts posts={posts} />
            </Section>
          )}
          <Section surface="card" title={t("client:admin.report.target.title")}>
            {target === undefined ? (
              <p>{t("client:admin.report.target.deleted")}</p>
            ) : (
              <Stack gap="3">
                <AccountItem
                  name={accountName(target)}
                  handle={target.display_name === "" ? undefined : handleOf(target)}
                  avatar={imageSources(target.avatar)}
                  avatarSeed={target.id}
                />
                <Cluster as="ul" gap="4" rowGap="1">
                  <li>
                    <Link href={`/@${target.acct}`} variant="standalone">
                      {t("client:admin.report.target.profile")}
                    </Link>
                  </li>
                  {canAct && (
                    <li>
                      <Link href={`/moderation/accounts/${encodeURIComponent(target.id)}`} variant="standalone">
                        {t("client:admin.report.target.admin")}
                      </Link>
                    </li>
                  )}
                  {canAudit && (
                    <li>
                      <Link
                        href={`/moderation/audit-log?target_kind=account&target=${encodeURIComponent(target.id)}`}
                        variant="standalone"
                      >
                        {t("client:admin.report.target.audit")}
                      </Link>
                    </li>
                  )}
                </Cluster>
              </Stack>
            )}
            <DescriptionList columns={2}>
              <DescriptionItem term={t("client:admin.report.target.reports")}>
                <Text numeric>
                  {t("client:admin.report.target.reports.count", { count: report.target_history.reports_count })}
                </Text>
              </DescriptionItem>
              <DescriptionItem term={t("client:admin.report.target.actions")}>
                <Text numeric>
                  {t("client:admin.report.target.actions.count", { count: report.target_history.actions_count })}
                </Text>
              </DescriptionItem>
            </DescriptionList>
          </Section>
          {canAct && target !== undefined && (
            <Section surface="card" title={t("client:admin.report.history.title")}>
              <TargetActions accountId={target.id} reportId={report.id} />
            </Section>
          )}
          <Section surface="card" title={t("client:admin.report.notes.title")}>
            <Notes report={report} />
          </Section>
        </>
      }
    />
  );
}

const stateLabels: Readonly<Record<ReportDetail["state"], ClientMessageKey>> = {
  open: "client:admin.reports.filter.open",
  resolved: "client:admin.reports.filter.resolved",
};

function ReportSummary({ report }: { report: ReportDetail }) {
  const t = useT();
  const rules = useCitedRules(report.rule_ids);
  return (
    <Stack gap="3">
      {/* The state, the category and the wait at a glance; the facts follow. */}
      <Cluster gap="2" align="center">
        <StatusBadge
          icon={report.state === "open" ? <ReportIcon /> : <CheckIcon />}
          label={t(stateLabels[report.state])}
          tone={report.state === "open" ? "accent" : "success"}
        />
        <ReportCategoryBadge category={report.category} />
        {report.state === "open" && <ReportWaitBadge filedAt={report.filed_at} />}
      </Cluster>
      <DescriptionList columns={3}>
        <DescriptionItem term={t("client:admin.report.filed_at")}>
          <DateTime value={report.filed_at} />
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.report.reporter")}>
          <AccountLabel account={report.reporter_account} />
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.report.origin")}>
          {report.uri === undefined ? t("client:admin.report.origin.local") : t("client:admin.report.origin.remote")}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.report.forwarded")}>
          {report.forwarded ? t("client:admin.report.forwarded.yes") : t("client:admin.report.forwarded.no")}
        </DescriptionItem>
      </DescriptionList>
      {report.rule_ids.length > 0 && (
        <DescriptionList>
          <DescriptionItem term={t("client:admin.report.rules")}>
            {rules === null ? (
              <Spinner size="sm" label={t("client:admin.report.rules.loading")} />
            ) : (
              <Stack as="ul" marker="discInside" gap="1">
                {rules.map((rule) =>
                  rule.text === null ? (
                    <Box as="li" key={rule.id} tone="muted">
                      {t("client:admin.report.rules.retired")}
                    </Box>
                  ) : (
                    <li key={rule.id} lang="" dir="auto">
                      {rule.text}
                    </li>
                  ),
                )}
              </Stack>
            )}
          </DescriptionItem>
        </DescriptionList>
      )}
      <DescriptionList>
        <DescriptionItem term={t("client:admin.report.comment")}>
          {report.comment === "" ? (
            <Text tone="muted">{t("client:admin.report.comment.none")}</Text>
          ) : (
            // Written by the reporter, in any language (Y10).
            <Text dir="auto" block align="start" wrap="anywhere" whitespace="preWrap">
              {report.comment}
            </Text>
          )}
        </DescriptionItem>
      </DescriptionList>
    </Stack>
  );
}

function ReportedPosts({ posts }: { posts: readonly ReportedStatus[] }) {
  const t = useT();
  const intl = useIntl();
  // Refetching a remote post from its server (FR-407) is for a viewer with manage_domains.
  const canRefetch = useHasPermission("manage_domains");
  const refetch = useRefetch();
  if (posts.length === 0) return <p>{t("client:admin.report.posts.none")}</p>;
  return (
    <Stack gap="3">
      {canRefetch && <StatusMessage tone="success">{refetch.status}</StatusMessage>}
      <Stack as="ul" divided border="lg">
        {posts.map((post) => (
          <li key={post.id}>
            {post.status === "found" ? (
              <>
                {post.deletedAt !== undefined && (
                  <Box paddingX="4" paddingTop="3">
                    <Hint weight="semibold">
                      {t("client:admin.report.posts.deleted", {
                        date: intl.formatDate(post.deletedAt, { dateStyle: "medium", timeStyle: "short" }),
                      })}
                    </Hint>
                  </Box>
                )}
                <StatusCard status={post.value} withActions={false} />
                {canRefetch && post.deletedAt === undefined && isRemote(post.value.account) && (
                  <Box paddingX="4" paddingBottom="3">
                    <Button
                      isPending={refetch.isPending({ kind: "status", id: post.value.id })}
                      onPress={() => {
                        refetch.request({ kind: "status", id: post.value.id });
                      }}
                    >
                      {t("client:admin.refetch.post.button", {
                        date: intl.formatDate(post.value.created_at, { dateStyle: "medium", timeStyle: "short" }),
                      })}
                    </Button>
                  </Box>
                )}
              </>
            ) : (
              <Box paddingX="4" paddingY="3">
                <Hint size="inherit">{t("client:admin.report.posts.missing")}</Hint>
              </Box>
            )}
          </li>
        ))}
      </Stack>
    </Stack>
  );
}

function TargetActions({ accountId, reportId }: { accountId: string; reportId: string }) {
  const t = useT();
  const actions = useTargetActions(accountId);
  if (actions.isLoading) return <Spinner size="sm" label={t("client:admin.report.history.loading")} />;
  if (actions.items.length === 0 && actions.error !== undefined)
    return <RegionError heading={t("client:admin.report.history.failed")} headingLevel={3} reason={actions.error} />;
  return (
    <RecordList
      list={actions}
      getId={(action) => action.id}
      empty={<EmptyState headingLevel={3} heading={t("client:admin.report.history.empty")} />}
      moreError={
        actions.error !== undefined && (
          <RegionError heading={t("client:admin.report.history.failed")} headingLevel={3} reason={actions.error} />
        )
      }
    >
      {(action) => <TargetActionRow action={action} fromThisReport={action.report_id === reportId} />}
    </RecordList>
  );
}

function TargetActionRow({ action, fromThisReport }: { action: AccountAction; fromThisReport: boolean }) {
  const t = useT();
  return (
    <Stack gap="2">
      <Heading level={3}>{t(accountActionKindLabels[action.kind])}</Heading>
      <DescriptionList columns={3}>
        <DescriptionItem term={t("client:admin.account_actions.state")}>
          {t(accountActionStateLabels[action.state])}
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
        <DescriptionItem term={t("client:admin.account_actions.category")}>
          {t(reportCategoryLabels[action.reason_category])}
        </DescriptionItem>
        {action.report_id !== undefined && (
          <DescriptionItem term={t("client:admin.account_actions.report")}>
            {fromThisReport ? (
              t("client:admin.account_actions.report.this")
            ) : (
              <Link href={`/moderation/reports/${encodeURIComponent(action.report_id)}`} variant="inline">
                {t("client:admin.account_actions.report.other")}
              </Link>
            )}
          </DescriptionItem>
        )}
      </DescriptionList>
      {action.public_reason !== undefined && (
        <DescriptionList>
          <DescriptionItem term={t("client:admin.account_actions.public_reason")}>
            {/* Written by a moderator, in any language. */}
            <Text dir="auto" block align="start" wrap="anywhere" whitespace="preWrap">
              {action.public_reason}
            </Text>
          </DescriptionItem>
        </DescriptionList>
      )}
    </Stack>
  );
}

type AssigneeProps = {
  report: ReportDetail;
  pending: boolean;
  onAssign: () => void;
  onUnassign: () => void;
  isAssigning: boolean;
  isUnassigning: boolean;
};

function Assignee({ report, pending, onAssign, onUnassign, isAssigning, isUnassigning }: AssigneeProps) {
  const t = useT();
  const assignee = report.assignee_account_id;
  return (
    <Stack align="start" gap="3">
      <DescriptionList>
        <DescriptionItem term={t("client:admin.report.assignee")}>
          {assignee === undefined ? t("client:admin.report.assignee.none") : <AssigneeName id={assignee} />}
        </DescriptionItem>
      </DescriptionList>
      {/*
        The buttons keep their places as the assignee changes, so that focus stays on the one pressed: taking it
        from nobody or from another moderator, and leaving it to nobody.
      */}
      <Cluster gap="2">
        <Button onPress={onAssign} isPending={isAssigning} isDisabled={pending && !isAssigning}>
          {assignee === undefined ? t("client:admin.report.assign") : t("client:admin.report.assign.take_over")}
        </Button>
        {assignee !== undefined && (
          <Button onPress={onUnassign} isPending={isUnassigning} isDisabled={pending && !isUnassigning}>
            {t("client:admin.report.unassign")}
          </Button>
        )}
      </Cluster>
    </Stack>
  );
}

function Notes({ report }: { report: ReportDetail }) {
  const t = useT();
  const form = useReportNoteForm(report.id);
  return (
    <Stack gap="4">
      {report.notes.length === 0 ? (
        <Hint size="inherit">{t("client:admin.report.notes.empty")}</Hint>
      ) : (
        <Stack as="ul" divided>
          {report.notes.map((note) => (
            <Stack as="li" key={note.id} gap="1" paddingY="3">
              <Cluster as="span" align="baseline" gap="3" rowGap="0" tone="muted" textSize="sm">
                <span>
                  {note.author_account_id === undefined ? (
                    t("client:admin.report.notes.cli")
                  ) : (
                    <AssigneeName id={note.author_account_id} />
                  )}
                </span>
                <DateTime value={note.created_at} />
              </Cluster>
              {/* Written by a moderator, in any language. */}
              <Text as="p" dir="auto" align="start" wrap="anywhere" whitespace="preWrap">
                {note.body}
              </Text>
            </Stack>
          ))}
        </Stack>
      )}
      <Stack as="form" onSubmit={form.submit} gap="3">
        <ReasonField
          label={t("client:admin.report.notes.new")}
          description={t("client:admin.report.notes.new.description")}
          value={form.body}
          onChange={form.change}
          isRequired
          characterLimit={reportNoteLimit}
          formatRemaining={(remaining) =>
            remaining < 0
              ? t("client:admin.account_actions.characters_over", { count: -remaining })
              : t("client:admin.account_actions.characters_left", { count: remaining })
          }
          isInvalid={form.error !== undefined}
          errorMessage={(validation) =>
            form.error ??
            (validation.validationDetails.valueMissing
              ? t("client:admin.report.notes.blank")
              : t("client:admin.report.notes.too_long", { max: reportNoteLimit }))
          }
        />
        <Box>
          <Button type="submit" isPending={form.isPending}>
            {t("client:admin.report.notes.submit")}
          </Button>
        </Box>
        <StatusMessage>{form.status}</StatusMessage>
      </Stack>
    </Stack>
  );
}
