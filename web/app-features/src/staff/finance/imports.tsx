// SPDX-License-Identifier: AGPL-3.0-only

// The billing imports as a holder of `manage_finance` reads them, at /admin/finance/imports (ADR-0068 §9; FR-904): each
// cloud provider the configuration enables, in its order: how it reads the provider's data, the time zone of its
// billing months, its last import and what came of it (why it failed, and what to do), its last success, the requests
// billed today and this month against their caps, the bytes billed this month, and its billing months of the last
// year, each an estimate, final, or corrected, with its totals; and importing one now, besides the daily import.

import {
  loadBillingImports,
  useBillingImportRun,
  useBillingImports,
  type AppData,
  type BillingImport,
} from "@my/app-data";
import {
  Button,
  dateFormat,
  FactList,
  FeedNotice,
  FinanceRow,
  FinanceTable,
  Row,
  ScreenStack,
  ScreenTitle,
  Stack,
  type Fact,
} from "@my/design-system";
import { useRef, useState, type ReactNode } from "react";
import { moneyText } from "../../finance-totals";
import type { StaffUnderProps } from "../entries";
import { ReadNotice, StatusRegion, Titled, useMoment, useStaffStrings, useStatus, type T } from "../parts";
import { FinanceBack } from "./parts";

type Say = (words: string, alert?: boolean) => void;

/** Reads the state of the billing imports, for the route's loader. */
export const loadBillingImportsScreen = (data: AppData): Promise<void> => loadBillingImports(data);

/** A count of requests, against its cap where the source has one. */
const requestsOf = (t: T, locale: string, count: number, cap: number | undefined): string =>
  cap === undefined
    ? new Intl.NumberFormat(locale).format(count)
    : t("staff_finance.imports.requests_of", { count, cap });

/** The facts of a source's imports. */
function sourceFacts(t: T, locale: string, moment: (iso: string) => string, source: BillingImport): Fact[] {
  const run = source.last_run;
  const bytes = new Intl.NumberFormat(locale, {
    style: "unit",
    unit: "byte",
    notation: "compact",
    unitDisplay: "short",
  });
  return [
    {
      term: t("staff_finance.imports.method"),
      value: t("staff_finance.imports.method.value", { method: source.method }),
    },
    { term: t("staff_finance.imports.zone"), value: source.time_zone },
    {
      term: t("staff_finance.imports.last_run"),
      value:
        run === undefined
          ? t("staff_finance.imports.never")
          : run.outcome === "succeeded"
            ? t("staff_finance.imports.succeeded", { at: moment(run.occurred_at) })
            : t("staff_finance.imports.failed_at", { at: moment(run.occurred_at) }),
    },
    ...(run === undefined || run.outcome === "succeeded"
      ? []
      : [{ term: t("staff_finance.imports.why"), value: t("staff_finance.imports.error", { kind: run.error_kind }) }]),
    {
      term: t("staff_finance.imports.last_success"),
      value: source.last_success_at === undefined ? t("staff_finance.imports.never") : moment(source.last_success_at),
    },
    {
      term: t("staff_finance.imports.requests_today"),
      value: requestsOf(t, locale, source.requests_today, source.max_requests_per_day),
    },
    {
      term: t("staff_finance.imports.requests_month"),
      value: requestsOf(t, locale, source.requests_this_month, source.max_requests_per_month),
    },
    ...(source.method === "bigquery" || source.method === "plugin"
      ? [{ term: t("staff_finance.imports.bytes"), value: bytes.format(source.bytes_billed_this_month) }]
      : []),
  ];
}

/** A source of the imports: its facts, its billing months, and Import now. */
function Source({ source, say }: { source: BillingImport; say: Say }) {
  const { t, locale } = useStaffStrings();
  const moment = useMoment();
  const run = useBillingImportRun();
  const [busy, setBusy] = useState(false);
  const months = dateFormat(locale, "UTC", { year: "numeric", month: "long" });
  const importNow = () => {
    if (busy) return;
    setBusy(true);
    run(source.source).then(
      (answer) => {
        setBusy(false);
        say(
          t("staff_finance.imports.run", {
            answer: answer === "too-soon" ? "too_soon" : answer,
            provider: source.provider,
          }),
          answer !== "queued",
        );
      },
      () => {
        setBusy(false);
        say(t("staff_finance.imports.run_failed"), true);
      },
    );
  };
  return (
    <Titled title={source.provider}>
      <Stack gap="entry-room">
        <FactList
          label={t("staff_finance.imports.facts", { provider: source.provider })}
          items={sourceFacts(t, locale, moment, source)}
        />
        {source.months.length === 0 ? (
          <FeedNotice start>{t("staff_finance.imports.no_months")}</FeedNotice>
        ) : (
          <FinanceTable
            label={t("staff_finance.imports.months", { provider: source.provider })}
            head={t("staff_finance.imports.column.month")}
            columns={[
              { label: t("staff_finance.imports.column.kind") },
              { label: t("staff_finance.imports.column.total"), figures: true },
            ]}
          >
            {source.months.map((m) => (
              <FinanceRow
                key={m.month}
                head={months.format(new Date(`${m.month}-01T00:00:00Z`))}
                cells={[
                  t("staff_finance.imports.kind", { kind: m.kind }),
                  {
                    figures:
                      m.totals.length === 0 ? [t("finance.none")] : m.totals.map((total) => moneyText(locale, total)),
                    estimated: m.kind === "estimate",
                  },
                ]}
              />
            ))}
          </FinanceTable>
        )}
        <Row>
          <Button busy={busy} onPress={importNow}>
            {t("staff_finance.imports.now")}
          </Button>
        </Row>
      </Stack>
    </Titled>
  );
}

/** The billing imports of every source, or that none is set up. */
export function BillingImportsScreen({ go }: StaffUnderProps): ReactNode {
  const { t } = useStaffStrings();
  const read = useBillingImports();
  const region = useRef<HTMLDivElement>(null);
  const status = useStatus(region);
  const head = (
    <Stack gap="entry-line">
      <ScreenTitle back={<FinanceBack to="root" go={go} />}>{t("staff_finance.imports")}</ScreenTitle>
      <StatusRegion said={status.said} region={region} />
    </Stack>
  );
  if (read.forbidden)
    return (
      <ScreenStack>
        {head}
        <FeedNotice lead>{t("staff_finance.forbidden")}</FeedNotice>
      </ScreenStack>
    );
  if (read.value === undefined)
    return (
      <ScreenStack>
        {head}
        <ReadNotice
          status={read.status === "error" ? "error" : "pending"}
          loading={t("staff_finance.imports.loading")}
          failed={t("staff_finance.imports.failed")}
          retry={read.refetch}
        />
      </ScreenStack>
    );
  return (
    <ScreenStack>
      {head}
      {read.value.length === 0 ? (
        <FeedNotice lead>{t("staff_finance.imports.none")}</FeedNotice>
      ) : (
        read.value.map((source) => <Source key={source.source} source={source} say={status.say} />)
      )}
    </ScreenStack>
  );
}
