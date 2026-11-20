// SPDX-License-Identifier: AGPL-3.0-only

// The UX measurements, at /admin/ux-measurements (ADR-0167 §3), as a holder of `manage_errors` reads them: over a
// window the reader chooses (the last 24 hours by default, the last 7 or 30 days), which the address keeps, a table of
// each metric that has samples, a row a name (a route, a kind of navigation, an action, a call's transport) with its
// samples, the bound under which three quarters of them fall, and the share of them that is good, that needs
// improvement and that is poor, by the thresholds of the metric. Under them, whether the web client
// sends samples at all, a form saved as a whole (form.tsx) of the instance setting `ux_measurement`, off by default;
// while it is off, the window says so, and shows what was kept from before.

import {
  loadAdminSettings,
  loadUXMeasurements,
  useAdminSettings,
  useUXMeasurements,
  uxMetrics,
  uxWindows,
  type AppData,
  type UXMeasurement,
  type UXMetric,
} from "@my/app-data";
import {
  FeedNotice,
  RecordTable,
  SegmentChoice,
  SettingsScreen,
  SettingsSection,
  useLocale,
  type RecordColumn,
  type RecordRow,
} from "@my/design-system";
import type { ReactNode } from "react";
import type { StaffSearch } from "../entries";
import { ReadNotice, useMoment, useStaffStrings } from "../parts";
import { TopicForm } from "../settings/form";
import { ScreenNotice, type AdminSubProps } from "../settings/parts";
import { uxMeasurementTopic } from "../settings/topics";
import { useCount } from "./parts";

type Window = (typeof uxWindows)[number];

/** The window of an address's query, in hours: the last 24 hours unless it names another the screen offers. */
function uxWindowOf(search: StaffSearch): Window {
  return uxWindows.find((w) => String(w) === search["hours"]) ?? 24;
}

/** Reads what the screen shows at `search`, for the route's loader. */
export async function loadUXScreen(data: AppData, search: StaffSearch): Promise<void> {
  await Promise.all([loadAdminSettings(data), loadUXMeasurements(data, uxWindowOf(search))]);
}

/**
 * The bounds of the bands a metric's samples are shared in, the thresholds of a good and of a poor value: a screen
 * shown within 2.5 s and past 4 s (LCP), and a response within 200 ms and past 500 ms (INP, and every other duration);
 * a call's packet loss within 1% and past 5%.
 */
const bandsOf = (metric: UXMetric): readonly number[] =>
  metric === "call_packet_loss" ? [1, 5] : metric === "lcp" ? [2500, 4000] : [200, 500];

/** Writes a value of `metric`: a duration in ms or s, a packet loss in percent. */
function useValue(): (metric: UXMetric, value: number) => string {
  const { locale } = useLocale();
  const ms = new Intl.NumberFormat(locale, { style: "unit", unit: "millisecond", maximumFractionDigits: 0 });
  const s = new Intl.NumberFormat(locale, { style: "unit", unit: "second", maximumFractionDigits: 1 });
  const percent = new Intl.NumberFormat(locale, { style: "unit", unit: "percent", maximumFractionDigits: 1 });
  return (metric, value) =>
    metric === "call_packet_loss" ? percent.format(value) : value < 1000 ? ms.format(value) : s.format(value / 1000);
}

/** The share of `m`'s samples in each band of `bounds`: those up to each bound, then those over the last. */
function sharesOf(m: UXMeasurement, bounds: readonly number[]): number[] {
  const shares = [...bounds.map(() => 0), 0];
  for (const bucket of m.buckets) {
    const le = bucket.le;
    const at = le === undefined ? -1 : bounds.findIndex((b) => le <= b);
    const band = at === -1 ? bounds.length : at;
    shares[band] = (shares[band] ?? 0) + bucket.samples;
  }
  return shares.map((n) => (m.samples === 0 ? 0 : n / m.samples));
}

/** The table of one metric: a row a name, its samples, its 75th percentile and its share in each band. */
function MetricTable({ metric, items }: { metric: UXMetric; items: readonly UXMeasurement[] }): ReactNode {
  const { t } = useStaffStrings();
  const { locale } = useLocale();
  const count = useCount();
  const value = useValue();
  const share = new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 0 });
  const bounds = bandsOf(metric);
  const last = items[0]?.buckets.findLast((b) => b.le !== undefined)?.le;
  const bands = [
    ...bounds.map((b, i) => ({
      key: `band${String(i)}`,
      label: t("staff_operations.ux.band.upto", { value: value(metric, b) }),
    })),
    {
      key: `band${String(bounds.length)}`,
      label: t("staff_operations.ux.over", { value: value(metric, bounds.at(-1) ?? 0) }),
    },
  ];
  const columns: RecordColumn[] = [
    { key: "samples", label: t("staff_operations.ux.column.samples"), kind: "figures" },
    { key: "p75", label: t("staff_operations.ux.column.p75"), kind: "figures" },
    ...bands.map((b) => ({ key: b.key, label: b.label, kind: "figures" as const })),
  ];
  const rows = items.map((m): RecordRow => {
    const shares = sharesOf(m, bounds);
    return {
      id: m.name,
      head: m.name,
      cells: {
        samples: count(m.samples),
        p75:
          m.p75 === undefined
            ? t("staff_operations.ux.over", { value: value(metric, last ?? 0) })
            : value(metric, m.p75),
        ...Object.fromEntries(bands.map((b, i) => [b.key, share.format(shares[i] ?? 0)])),
      },
    };
  });
  return (
    <RecordTable
      label={t("staff_operations.ux.metric", { metric })}
      head={{ label: t("staff_operations.ux.name", { metric }) }}
      columns={columns}
      rows={rows}
    />
  );
}

/** The window's choice, whether samples are taken, and a section of each metric that has samples. */
function Measurements({ hours, onHours, on }: { hours: Window; onHours: (hours: Window) => void; on: boolean }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const read = useUXMeasurements(hours);
  const heading = t("staff_operations.ux.window");
  const choice = (
    <SegmentChoice
      label={heading}
      fit
      options={uxWindows.map((w) => ({
        value: String(w),
        label: t("staff_operations.ux.window.choice", { hours: w }),
      }))}
      value={String(hours)}
      onChange={(v) => {
        onHours(uxWindows.find((w) => String(w) === v) ?? 24);
      }}
    />
  );
  if (read.value === undefined)
    return (
      <SettingsSection heading={heading}>
        {choice}
        <ReadNotice
          status={read.status === "error" ? "error" : "pending"}
          loading={t("staff_operations.ux.loading")}
          failed={t("staff_operations.ux.failed")}
          retry={read.refetch}
        />
      </SettingsSection>
    );
  const list = read.value;
  const metrics = uxMetrics.filter((m) => list.items.some((i) => i.metric === m));
  return (
    <>
      <SettingsSection heading={heading} about={t("staff_operations.ux.since", { date: moment(list.since) })}>
        {choice}
        {!on && <FeedNotice>{t("staff_operations.ux.off")}</FeedNotice>}
        {on && metrics.length === 0 && <FeedNotice>{t("staff_operations.ux.empty")}</FeedNotice>}
      </SettingsSection>
      {metrics.map((metric) => (
        <SettingsSection key={metric} heading={t("staff_operations.ux.metric", { metric })}>
          <MetricTable metric={metric} items={list.items.filter((i) => i.metric === metric)} />
        </SettingsSection>
      ))}
    </>
  );
}

/**
 * The UX measurements over the window of `search`, and whether they are taken, once the settings are read. Its route
 * holds the reader back while the form has changes not saved (`onUnsaved`, `leaving`).
 */
export function UXMeasurementsScreen({
  search = {},
  onSearch,
  onUnsaved,
  leaving,
}: Partial<AdminSubProps> & { search?: StaffSearch; onSearch?: (search: StaffSearch) => void }): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminSettings();
  const title = t("staff_operations.entry.ux_measurements");
  if (read.settings === undefined || read.texts === undefined)
    return (
      <SettingsScreen
        sub
        icon={uxMeasurementTopic.icon}
        title={title}
        notice={
          <ScreenNotice
            status={read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{ loading: t("staff_settings.loading"), failed: t("staff_settings.failed") }}
          />
        }
      />
    );
  const hours = uxWindowOf(search);
  return (
    <TopicForm
      topic={uxMeasurementTopic}
      title={title}
      settings={read.settings}
      texts={read.texts}
      before={
        <Measurements
          hours={hours}
          on={read.settings.get("ux_measurement")?.value === "true"}
          onHours={(next) => {
            onSearch?.(next === 24 ? {} : { hours: String(next) });
          }}
        />
      }
      onUnsaved={onUnsaved}
      leaving={leaving}
    />
  );
}
