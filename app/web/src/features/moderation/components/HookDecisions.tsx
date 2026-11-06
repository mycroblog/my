// SPDX-License-Identifier: AGPL-3.0-only
// The first page of the automated reviews screen below its heading and local navigation (ADR-0147 §9): each hook with
// its mode and its counts per outcome and latency, then what the hooks stopped, would have stopped, or could not check,
// newest first. A decision holds no text of a post, so a row names its subject, which links to the decision's page
// with every other fact. The list is in the admin mode's table form: a toolbar with the filter (kept in the URL by the
// route, web-client/README.md S3), one page of the table at a time, previous / next. Cancelling a row takes effect at
// once and asks nothing first: the result stays on the page in a StatusMessage (role="status"). The authors' appeals
// of refused posts are the screen's other page (HookAppeals.tsx).
import {
  Button,
  Card,
  DescriptionItem,
  DescriptionList,
  EmptyState,
  Link,
  ListToolbar,
  Select,
  SelectItem,
  Stack,
  StatusMessage,
  Table,
  TableBody,
  TableCell,
  TableColumnHeader,
  TableHead,
  TableRow,
  TableRowHeader,
  Text,
} from "@my/ui";
import { useState } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { moderationHookDecisionPath } from "../../../lib/paths";
import {
  type HookDecision,
  type HookDecisionFilter,
  hookDecisionFilters,
  type HookSummary,
} from "../api/hookDecisions";
import {
  hookModeLabels,
  hookNameLabels,
  hookOutcomeLabels,
  type OverrideHookDecision,
  useHookDecisions,
  useHookSummary,
  useOverrideHookDecision,
} from "../hooks/useHookDecisions";
import { DateTime, type RecordColumns, RecordTable, RequirePermission, Section } from "./parts";

type HookRow = HookSummary["hooks"][number];

function isFilter(key: unknown): key is HookDecisionFilter {
  return hookDecisionFilters.some((item) => item === key);
}

export function HookDecisions({
  filter,
  onFilterChange,
}: {
  filter: HookDecisionFilter;
  onFilterChange: (next: HookDecisionFilter) => void;
}) {
  const t = useT();
  const [status, setStatus] = useState("");
  return (
    <RequirePermission permission="manage_reports">
      <Stack gap="6">
        <Section title={t("client:admin.hook_decisions.summary.title")}>
          <HookSummaryTable />
        </Section>
        <Section title={t("client:admin.hook_decisions.list.title")}>
          <Stack gap="3">
            <ListToolbar label={t("client:admin.list.filters")}>
              <Select
                variant="toolbar"
                aria-label={t("client:admin.hook_decisions.filter.label")}
                value={filter}
                onChange={(key) => {
                  if (isFilter(key)) onFilterChange(key);
                }}
              >
                {hookDecisionFilters.map((item) => (
                  <SelectItem key={item} id={item}>
                    {item === "all" ? t("client:admin.hook_decisions.filter.all") : t(hookNameLabels[item])}
                  </SelectItem>
                ))}
              </Select>
            </ListToolbar>
            <StatusMessage>{status}</StatusMessage>
            {/* Another filter is another list: it starts at its first page. */}
            <HookDecisionList
              key={filter}
              filter={filter}
              onOverridden={(decision) => {
                setStatus(
                  t("client:admin.hook_decisions.overridden", {
                    hook: t(hookNameLabels[decision.hook]),
                    outcome: t(hookOutcomeLabels[decision.outcome]),
                  }),
                );
              }}
            />
          </Stack>
        </Section>
      </Stack>
    </RequirePermission>
  );
}

/** Every hook, whether it has a plugin or not: the screen says why one that has none records nothing. */
function HookSummaryTable() {
  const t = useT();
  const summary = useHookSummary();
  return (
    <Stack gap="3">
      <Text tone="muted">{t("client:admin.hook_decisions.summary.period")}</Text>
      <Card padding="0">
        <Table caption={t("client:admin.hook_decisions.summary.title")}>
          <TableHead>
            <TableColumnHeader>{t("client:admin.hook_decisions.column.hook")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.hook_decisions.column.mode")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.hook_decisions.column.outcomes")}</TableColumnHeader>
          </TableHead>
          <TableBody>
            {summary.hooks.map((hook) => (
              <HookSummaryRow key={hook.hook} hook={hook} />
            ))}
          </TableBody>
        </Table>
      </Card>
    </Stack>
  );
}

function HookSummaryRow({ hook }: { hook: HookRow }) {
  const t = useT();
  const intl = useIntl();
  return (
    <TableRow data-row-id={hook.hook}>
      <TableRowHeader>{t(hookNameLabels[hook.hook])}</TableRowHeader>
      <TableCell>
        {hook.configured ? t(hookModeLabels[hook.mode]) : t("client:admin.hook_decisions.not_configured")}
      </TableCell>
      <TableCell>
        {hook.outcomes.length === 0 ? (
          t("client:admin.hook_decisions.summary.none")
        ) : (
          <DescriptionList>
            {hook.outcomes.map((row) => (
              <DescriptionItem
                key={`${row.mode}-${row.outcome}`}
                term={t("client:admin.hook_decisions.summary.outcome", {
                  outcome: t(hookOutcomeLabels[row.outcome]),
                  mode: t(hookModeLabels[row.mode]),
                })}
              >
                {t("client:admin.hook_decisions.summary.count", {
                  count: row.count,
                  average: intl.formatNumber(row.average_latency_ms),
                  max: intl.formatNumber(row.max_latency_ms),
                })}
              </DescriptionItem>
            ))}
          </DescriptionList>
        )}
      </TableCell>
    </TableRow>
  );
}

function HookDecisionList({
  filter,
  onOverridden,
}: {
  filter: HookDecisionFilter;
  onOverridden: (decision: HookDecision) => void;
}) {
  const t = useT();
  const list = useHookDecisions(filter);
  const override = useOverrideHookDecision(onOverridden);
  const columns: RecordColumns<HookDecision> = [
    {
      id: "subject",
      header: t("client:admin.hook_decisions.column.subject"),
      wrap: "anywhere",
      cell: (decision) => <DecisionName decision={decision} />,
    },
    {
      id: "outcome",
      header: t("client:admin.hook_decisions.column.outcome"),
      cell: (decision) => <DecisionState decision={decision} />,
    },
    {
      id: "decided_at",
      header: t("client:admin.hook_decisions.column.decided_at"),
      cell: (decision) => <DateTime value={decision.decided_at} />,
    },
    {
      id: "actions",
      kind: "actions",
      header: t("client:admin.list.column.actions"),
      cell: (decision) => <OverrideButton decision={decision} override={override} />,
    },
  ];
  return (
    <RecordTable
      list={list}
      caption={t("client:admin.hook_decisions.list.title")}
      columns={columns}
      getId={(decision) => decision.id}
      headingLevel={3}
      empty={<EmptyState headingLevel={3} heading={t("client:admin.hook_decisions.empty")} />}
    />
  );
}

/** The name of a decision: its subject, a link to its page, above the review that decided. */
function DecisionName({ decision }: { decision: HookDecision }) {
  const t = useT();
  return (
    <Stack as="span" gap="1">
      {/* A URI or another key, which reads left to right in every locale. */}
      <Link href={moderationHookDecisionPath(decision.id)} variant="standalone" wrap="anywhere">
        <span dir="ltr">{decision.subject_key}</span>
      </Link>
      <Text size="sm" tone="muted">
        {t(hookNameLabels[decision.hook])}
      </Text>
    </Stack>
  );
}

/** Where a decision stands: its outcome in its mode, and whether a moderator cancelled it. */
function DecisionState({ decision }: { decision: HookDecision }) {
  const t = useT();
  return (
    <Stack as="span" gap="1">
      <Text weight="semibold">
        {t("client:admin.hook_decisions.summary.outcome", {
          outcome: t(hookOutcomeLabels[decision.outcome]),
          mode: t(hookModeLabels[decision.mode]),
        })}
      </Text>
      {decision.overridden_at !== undefined && <Text size="sm">{t("client:admin.hook_decisions.cancelled")}</Text>}
    </Stack>
  );
}

function OverrideButton({ decision, override }: { decision: HookDecision; override: OverrideHookDecision }) {
  const t = useT();
  const intl = useIntl();
  if (!decision.overridable) return null;
  return (
    <Button
      size="sm"
      isPending={override.pendingIds.includes(decision.id)}
      aria-label={t("client:admin.hook_decisions.override_action", {
        hook: t(hookNameLabels[decision.hook]),
        outcome: t(hookOutcomeLabels[decision.outcome]),
        date: intl.formatDate(decision.decided_at, { dateStyle: "medium", timeStyle: "short" }),
      })}
      onPress={() => {
        override.override(decision);
      }}
    >
      {t("client:admin.hook_decisions.override")}
    </Button>
  );
}
