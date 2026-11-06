// SPDX-License-Identifier: AGPL-3.0-only
// The billing imports of the costs screen (`/admin/finance/imports`, ADR-0068 §9, manage_finance;
// web-client/screens/admin-finance-imports.md): for each billing source, in the order of billing_sources, its time
// zone, its last success and last run with the kind of its failure, the requests of today and of the month against
// their caps, the bytes billed of BigQuery, and the months it imported with their kind and totals; and importing its
// charges now, at most once an hour (429), whose result shows above the sources (web-client/README.md E2). The list is
// short (one row per configured source) and is not paged.
import {
  Box,
  Button,
  Cluster,
  DescriptionItem,
  DescriptionList,
  EmptyState,
  Heading,
  Hint,
  InlineError,
  Stack,
  StatusMessage,
  Text,
} from "@my/ui";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useFormatTotals, utcDate } from "../../costs";
import { DateTime, Section } from "../../moderation";
import type { BillingImport } from "../api/finance";
import { useBillingImports, useRunBillingImport } from "../hooks/useFinance";
import { FinanceScreen } from "./FinanceParts";

const monthKindLabels: Readonly<Record<BillingImport["months"][number]["kind"], ClientMessageKey>> = {
  estimate: "client:admin.finance.imports.kind.estimate",
  final: "client:admin.finance.imports.kind.final",
  correction: "client:admin.finance.imports.kind.correction",
};

const errorKindLabels: Readonly<Record<NonNullable<BillingImport["last_run"]>["error_kind"], ClientMessageKey>> = {
  none: "client:admin.finance.imports.error.none",
  permission: "client:admin.finance.imports.error.permission",
  not_enabled: "client:admin.finance.imports.error.not_enabled",
  not_found: "client:admin.finance.imports.error.not_found",
  throttled: "client:admin.finance.imports.error.throttled",
  request_cap: "client:admin.finance.imports.error.request_cap",
  bytes_cap: "client:admin.finance.imports.error.bytes_cap",
  invalid_response: "client:admin.finance.imports.error.invalid_response",
  unavailable: "client:admin.finance.imports.error.unavailable",
};

export function BillingImports() {
  const t = useT();
  return (
    <FinanceScreen page="imports">
      <Section surface="card" title={t("client:admin.finance.imports.title")}>
        <BillingImportList />
      </Section>
    </FinanceScreen>
  );
}

function BillingImportList() {
  const t = useT();
  const sources = useBillingImports();
  const run = useRunBillingImport();
  if (sources.length === 0) {
    return (
      <EmptyState
        headingLevel={3}
        heading={t("client:admin.finance.imports.none")}
        description={t("client:admin.finance.imports.none.description")}
      />
    );
  }
  return (
    <Stack gap="3">
      <StatusMessage>{run.status}</StatusMessage>
      {run.error !== undefined && (
        <InlineError size="inherit" alert>
          {run.error}
        </InlineError>
      )}
      <Stack as="ul" divided>
        {sources.map((source) => (
          <Box as="li" container paddingY="3" key={source.source}>
            <BillingImportRow source={source} isPending={run.pendingSource === source.source} onRun={run.run} />
          </Box>
        ))}
      </Stack>
    </Stack>
  );
}

function BillingImportRow({
  source,
  isPending,
  onRun,
}: {
  source: BillingImport;
  isPending: boolean;
  onRun: (source: BillingImport) => void;
}) {
  const t = useT();
  const intl = useIntl();
  const formatTotals = useFormatTotals();
  const count = (used: number, max: number | undefined) =>
    max === undefined
      ? intl.formatNumber(used)
      : t("client:admin.finance.imports.used_of", { used: intl.formatNumber(used), max: intl.formatNumber(max) });
  return (
    <Stack gap="3">
      <Heading level={3}>{source.provider}</Heading>
      {source.method === "budget" && <Hint size="inherit">{t("client:admin.finance.imports.method.budget")}</Hint>}
      <DescriptionList>
        <DescriptionItem term={t("client:admin.finance.imports.time_zone")}>{source.time_zone}</DescriptionItem>
        <DescriptionItem term={t("client:admin.finance.imports.last_success")}>
          {source.last_success_at === undefined ? (
            t("client:admin.finance.imports.never")
          ) : (
            <DateTime value={source.last_success_at} />
          )}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.finance.imports.last_run")}>
          {source.last_run === undefined ? (
            t("client:admin.finance.imports.never")
          ) : (
            <Stack as="span">
              <DateTime value={source.last_run.occurred_at} />
              <span>{t(errorKindLabels[source.last_run.error_kind])}</span>
            </Stack>
          )}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.finance.imports.requests_today")}>
          {count(source.requests_today, source.max_requests_per_day)}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.finance.imports.requests_month")}>
          {count(source.requests_this_month, source.max_requests_per_month)}
        </DescriptionItem>
        {source.method === "bigquery" && (
          <DescriptionItem term={t("client:admin.finance.imports.bytes")}>
            {intl.formatNumber(source.bytes_billed_this_month, {
              style: "unit",
              unit: "byte",
              notation: "compact",
              unitDisplay: "narrow",
            })}
          </DescriptionItem>
        )}
      </DescriptionList>
      {source.months.length > 0 && (
        <Stack as="ul" gap="1" aria-label={t("client:admin.finance.imports.months", { provider: source.provider })}>
          {source.months.map((month) => (
            <Cluster as="li" gap="2" rowGap="0" key={month.month}>
              <span>
                {intl.formatDate(utcDate(`${month.month}-01`), { year: "numeric", month: "long", timeZone: "UTC" })}
              </span>
              <Text tone="muted">{t(monthKindLabels[month.kind])}</Text>
              <Text numeric>{formatTotals(month.totals)}</Text>
            </Cluster>
          ))}
        </Stack>
      )}
      <div>
        <Button
          size="sm"
          isPending={isPending}
          aria-label={t("client:admin.finance.imports.run_one", { provider: source.provider })}
          onPress={() => {
            onRun(source);
          }}
        >
          {t("client:admin.finance.imports.run")}
        </Button>
      </div>
    </Stack>
  );
}
