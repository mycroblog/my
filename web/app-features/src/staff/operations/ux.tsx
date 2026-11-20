// SPDX-License-Identifier: AGPL-3.0-only

// The UX measurements, at /admin/ux-measurements (ADR-0167 §3), as a holder of `manage_errors` reads them: over a
// period the reader chooses (the last 24 hours by default, the last 7 or 30 days), which the address keeps, a table of
// figures of each metric that has samples, a row a name (a route, a kind of navigation, an action, a call's transport)
// with its samples and the bound under which three quarters of them fall, and, for LCP and INP, the share that is good
// and the share that is poor by their thresholds, a 75th percentile past the good one marked. Under them, for a holder
// of `manage_settings` alone, whether the web client sends samples at all, the instance setting `ux_measurement` (off
// by default) with its own Save; while it is off and nothing was kept, the screen says so and offers no period.

import {
  loadAdminSettings,
  loadUXMeasurements,
  useAdminSettings,
  useAdminSettingsWrites,
  useForgetUXMeasurements,
  usePermissions,
  useInstance,
  useUXMeasurements,
  viewerQuery,
  uxMetrics,
  uxWindows,
  type AppData,
  type UXMeasurement,
  type UXMetric,
} from "@my/app-data";
import {
  Box,
  Button,
  FeedNotice,
  FinanceRow,
  FinanceTable,
  FormAlert,
  ModalPresence,
  Row,
  SaveState,
  ScreenStack,
  ScreenTitle,
  SegmentChoice,
  Stack,
  useLocale,
} from "@my/design-system";
import { useEffect, useId, useState, type ReactNode } from "react";
import type { StaffSearch } from "../entries";
import { ReadNotice, Titled, useMoment, useStaffStrings } from "../parts";
import { LeaveDialog, OnOffRow, ScreenNotice, type AdminSubProps } from "../settings/parts";

type Window = (typeof uxWindows)[number];

/** The window of an address's query, in hours: the last 24 hours unless it names another the screen offers. */
function uxWindowOf(search: StaffSearch): Window {
  return uxWindows.find((w) => String(w) === search["hours"]) ?? 24;
}

/** Reads what the screen shows at `search`, for the route's loader. */
export async function loadUXScreen(data: AppData, search: StaffSearch): Promise<void> {
  const viewer = await data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }).catch(() => undefined);
  // The setting's form is only for a holder of `manage_settings`, whose permission the settings' read needs.
  const keeps = viewer?.permissions.includes("manage_settings") === true;
  await Promise.all([keeps ? loadAdminSettings(data) : undefined, loadUXMeasurements(data, uxWindowOf(search))]);
}

/**
 * The bounds of the bands a metric's samples are shared in, good, needs improvement and poor: only where ADR-0167 §1
 * gives the thresholds, a screen shown within 2.5 s and past 4 s (LCP) and a press answered within 200 ms and past
 * 500 ms (INP), from web.dev's Core Web Vitals. No document gives thresholds of the other metrics (the long frames,
 * the navigations, the actions, a call's round trip, jitter and packet loss), whose tables say their samples and their
 * 75th percentile alone.
 */
const bandsOf = (metric: UXMetric): readonly number[] =>
  metric === "lcp" ? [2500, 4000] : metric === "inp" ? [200, 500] : [];

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

/** Which band `value` of `metric` falls in by its bounds: good, needs improvement, poor; none without bounds. */
function bandOf(metric: UXMetric, value: number | undefined): "good" | "improve" | "poor" | undefined {
  const [good, poor] = bandsOf(metric);
  if (good === undefined || poor === undefined) return undefined;
  if (value === undefined || value > poor) return "poor";
  return value > good ? "improve" : "good";
}

/**
 * The table of figures of one metric: a row a name, in the code's face, with its samples under it; for LCP and INP the
 * share of its samples that is good and that is poor; then its 75th percentile, which stands last so that it lines up
 * from one metric's table to the next, a percentile past the good threshold said so. Three columns at most leave a
 * route the room to break only at its slashes.
 */
function MetricTable({ metric, items }: { metric: UXMetric; items: readonly UXMeasurement[] }): ReactNode {
  const { t } = useStaffStrings();
  const { locale } = useLocale();
  const value = useValue();
  const share = new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 0 });
  const bounds = bandsOf(metric);
  const [good, poor] = bounds;
  const banded = good !== undefined && poor !== undefined;
  const columns = [
    ...(banded
      ? [
          { label: t("staff_operations.ux.band.upto", { value: value(metric, good) }), figures: true },
          { label: t("staff_operations.ux.over", { value: value(metric, poor) }), figures: true },
        ]
      : []),
    { label: t("staff_operations.ux.column.p75"), figures: true },
  ];
  return (
    <FinanceTable
      label={t("staff_operations.ux.metric", { metric })}
      head={t("staff_operations.ux.name", { metric })}
      columns={columns}
    >
      {items.map((m) => {
        const shares = sharesOf(m, bounds);
        // A 75th percentile in the last bucket has no bound: it is past the highest one of its own buckets.
        const top = m.buckets.findLast((b) => b.le !== undefined)?.le;
        const shown =
          m.p75 === undefined
            ? t("staff_operations.ux.over", { value: value(metric, top ?? 0) })
            : value(metric, m.p75);
        const band = bandOf(metric, m.p75);
        return (
          <FinanceRow
            key={m.name}
            head={m.name}
            caption={t("staff_operations.ux.samples", { count: m.samples })}
            code
            cells={[
              ...(banded
                ? [{ figures: [share.format(shares[0] ?? 0)] }, { figures: [share.format(shares.at(-1) ?? 0)] }]
                : []),
              {
                figures: [
                  band === undefined || band === "good"
                    ? shown
                    : t("staff_operations.ux.p75.band", { value: shown, band }),
                ],
              },
            ]}
          />
        );
      })}
    </FinanceTable>
  );
}

/** The period's choice, and a table of each metric that has samples; while nothing is measured, only that it is not. */
function Measurements({ hours, onHours, on }: { hours: Window; onHours: (hours: Window) => void; on: boolean }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const read = useUXMeasurements(hours);
  const choice = (about?: string) => (
    <SegmentChoice
      label={t("staff_operations.ux.window")}
      shown
      about={about}
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
      <>
        {choice()}
        <ReadNotice
          status={read.status === "error" ? "error" : "pending"}
          loading={t("staff_operations.ux.loading")}
          failed={t("staff_operations.ux.failed")}
          retry={read.refetch}
        />
      </>
    );
  const list = read.value;
  const metrics = uxMetrics.filter((m) => list.items.some((i) => i.metric === m));
  // Off with nothing kept from before, or off while what shows was read before it was turned off: there is no period to
  // choose among.
  if (!on && (metrics.length === 0 || read.stale)) return <FeedNotice start>{t("staff_operations.ux.off")}</FeedNotice>;
  return (
    <>
      {choice(t("staff_operations.ux.since", { date: moment(list.since) }))}
      {!on && <FeedNotice start>{t("staff_operations.ux.off")}</FeedNotice>}
      {on && metrics.length === 0 && <FeedNotice start>{t("staff_operations.ux.empty")}</FeedNotice>}
      {metrics.map((metric) => (
        <Titled key={metric} title={t("staff_operations.ux.metric", { metric })}>
          <MetricTable metric={metric} items={list.items.filter((i) => i.metric === metric)} />
        </Titled>
      ))}
    </>
  );
}

type UXScreenProps = Partial<AdminSubProps> & {
  search?: StaffSearch | undefined;
  onSearch?: ((search: StaffSearch) => void) | undefined;
};

/** Where the setting's last save is. */
type Sent = "idle" | "sending" | "saved" | "failed";

/**
 * Whether the web client is measured, the setting `ux_measurement`, with its own Save beside it, for a holder of
 * `manage_settings`: once the settings are read, and holding the reader back while it has a change not saved.
 */
function MeasuringSetting({ onUnsaved, leaving }: Partial<AdminSubProps>): ReactNode {
  const { t } = useStaffStrings();
  const id = useId();
  const read = useAdminSettings();
  const writes = useAdminSettingsWrites();
  const forget = useForgetUXMeasurements();
  const saved = read.settings?.get("ux_measurement")?.value === "true";
  const [on, setOn] = useState<boolean | undefined>(undefined);
  const [sent, setSent] = useState<Sent>("idle");
  const chosen = on ?? saved;
  const unsaved = chosen !== saved;
  useEffect(() => {
    onUnsaved?.(unsaved && sent !== "sending");
  }, [onUnsaved, unsaved, sent]);
  useEffect(
    () => () => {
      onUnsaved?.(false);
    },
    [onUnsaved],
  );
  const heading = t("staff_settings.section.ux_measurement");
  if (read.settings === undefined)
    return (
      <Titled title={heading}>
        <ScreenNotice
          status={read.status === "failed" ? "failed" : "loading"}
          retry={read.retry}
          words={{ loading: t("staff_settings.loading"), failed: t("staff_settings.failed") }}
        />
      </Titled>
    );
  const save = () => {
    if (sent === "sending") return;
    setSent("sending");
    writes.save({ ux_measurement: chosen ? "true" : "false" }).then(
      () => {
        setOn(undefined);
        setSent("saved");
        // What was measured before is read again as the server holds it now, the period with it.
        void forget();
      },
      () => {
        setSent("failed");
      },
    );
  };
  return (
    <>
      <Titled title={heading}>
        <Stack gap="entry-room">
          <OnOffRow
            id={`${id}-ux`}
            label={t("staff_settings.setting.ux_measurement")}
            on={chosen}
            onChange={(next) => {
              setOn(next);
              setSent("idle");
            }}
            disabled={sent === "sending"}
          />
          <Row wrap align="center" gap="screen-tab-gap">
            <Button kind="primary" size="timeline" busy={sent === "sending"} onPress={save}>
              {t("staff_settings.save")}
            </Button>
            <Box as="span" role="status">
              {sent === "saved" ? (
                <SaveState state="saved" />
              ) : sent === "failed" ? (
                <FormAlert>{t("staff_settings.save.failed")}</FormAlert>
              ) : null}
            </Box>
          </Row>
        </Stack>
      </Titled>
      <ModalPresence>{leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />}</ModalPresence>
    </>
  );
}

/**
 * The UX measurements over the period of `search`, and, for a holder of `manage_settings`, whether they are taken, a
 * setting saved by itself; any other reader sees the measurements alone, whether they are taken read from the instance.
 */
export function UXMeasurementsScreen({ search = {}, onSearch, onUnsaved, leaving }: UXScreenProps): ReactNode {
  const { t } = useStaffStrings();
  const keeps = usePermissions().includes("manage_settings");
  const settings = useAdminSettings(keeps).settings;
  const instance = useInstance();
  const on = keeps ? settings?.get("ux_measurement")?.value === "true" : instance?.ux_measurement === true;
  return (
    <ScreenStack>
      <ScreenTitle glyph="measurements">{t("staff_operations.entry.ux_measurements")}</ScreenTitle>
      <Measurements
        hours={uxWindowOf(search)}
        on={on}
        onHours={(next) => {
          onSearch?.(next === 24 ? {} : { hours: String(next) });
        }}
      />
      {keeps && <MeasuringSetting onUnsaved={onUnsaved} leaving={leaving} />}
    </ScreenStack>
  );
}
