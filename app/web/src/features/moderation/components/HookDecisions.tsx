// SPDX-License-Identifier: AGPL-3.0-only
// The automated reviews screen below its heading (ADR-0147 §9): each hook with its mode and its counts per outcome and
// latency, then what the hooks stopped, would have stopped, or could not check, newest first. A decision holds no text of
// a post, so a row shows only what identifies its subject. Cancelling takes effect at once and asks nothing first: the
// result stays on the page in a StatusMessage (role="status"). The list is in the admin mode's table form: a toolbar with
// the filter (kept in the URL by the route, web-client/README.md S3), one page of the table at a time, previous / next.
import {
  Box,
  Button,
  Card,
  DescriptionItem,
  DescriptionList,
  EmptyState,
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
  VisuallyHidden,
} from "@my/ui";
import { useState } from "react";
import { useIntl } from "react-intl";
import { useNow } from "../../../lib/clock";
import { useT } from "../../../lib/i18n";
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
import { DateTime, RecordTable, RequirePermission, Section } from "./parts";

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
  return (
    <Box>
      <RecordTable
        list={list}
        caption={t("client:admin.hook_decisions.list.title")}
        getId={(decision) => decision.id}
        empty={<EmptyState headingLevel={3} heading={t("client:admin.hook_decisions.empty")} />}
        head={
          <>
            <TableColumnHeader>{t("client:admin.hook_decisions.column.subject")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.hook_decisions.column.hook")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.hook_decisions.column.outcome")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.hook_decisions.column.mode")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.hook_decisions.column.decided_at")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.hook_decisions.column.details")}</TableColumnHeader>
            <TableColumnHeader>
              <VisuallyHidden>{t("client:admin.list.column.actions")}</VisuallyHidden>
            </TableColumnHeader>
          </>
        }
      >
        {(decision) => <HookDecisionRow decision={decision} override={override} />}
      </RecordTable>
    </Box>
  );
}

function HookDecisionRow({ decision, override }: { decision: HookDecision; override: OverrideHookDecision }) {
  const t = useT();
  const intl = useIntl();
  const now = useNow();
  const hook = t(hookNameLabels[decision.hook]);
  const outcome = t(hookOutcomeLabels[decision.outcome]);
  return (
    <TableRow data-row-id={decision.id}>
      <TableRowHeader wrap="anywhere">
        {/* A URI or another key, which reads left to right in every locale. */}
        <span dir="ltr">{decision.subject_key}</span>
      </TableRowHeader>
      <TableCell>{hook}</TableCell>
      <TableCell>{outcome}</TableCell>
      <TableCell>{t(hookModeLabels[decision.mode])}</TableCell>
      <TableCell>
        <DateTime value={decision.decided_at} />
      </TableCell>
      <TableCell wrap="anywhere">
        <DescriptionList>
          <DescriptionItem term={t("client:admin.hook_decisions.latency")}>
            {t("client:admin.hook_decisions.latency.value", { value: intl.formatNumber(decision.latency_ms) })}
          </DescriptionItem>
          {decision.reason !== undefined && (
            <DescriptionItem term={t("client:admin.hook_decisions.reason")}>{decision.reason}</DescriptionItem>
          )}
          {decision.note !== undefined && (
            <DescriptionItem term={t("client:admin.hook_decisions.note")}>
              {/* Written by the plugin, in any language. */}
              <Text dir="auto">{decision.note}</Text>
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
      </TableCell>
      <TableCell>
        {decision.overridable && (
          <Button
            size="sm"
            isPending={override.pendingIds.includes(decision.id)}
            aria-label={t("client:admin.hook_decisions.override_action", {
              hook,
              outcome,
              date: intl.formatDate(decision.decided_at, { dateStyle: "medium", timeStyle: "short" }),
            })}
            onPress={() => {
              override.override(decision);
            }}
          >
            {t("client:admin.hook_decisions.override")}
          </Button>
        )}
      </TableCell>
    </TableRow>
  );
}
