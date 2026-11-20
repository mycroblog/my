// SPDX-License-Identifier: AGPL-3.0-only
// The UX measurements of the web client as a holder of `manage_errors` reads them (ADR-0167 §3): the samples of each
// metric and name over the last hours of a window, the current one included, with their buckets and the bound of the
// bucket that holds the 75th percentile. Whether the server takes samples at all is the instance setting
// `ux_measurement`, read and saved with the other settings (admin-settings.ts).
import type { components } from "@mycroblog/mykit";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import { oneOf, type StaffOne } from "./staff-reads";

/** The measurements of a window. */
export type UXMeasurementList = components["schemas"]["UXMeasurementList"];
/** The samples of one metric and name over the window. */
export type UXMeasurement = components["schemas"]["UXMeasurementSummary"];
/** What a measurement measures. */
export type UXMetric = components["schemas"]["UXMetric"];

/** The metrics, in the server's order. */
export const uxMetrics = [
  "lcp",
  "inp",
  "long_frame",
  "transition",
  "press",
  "call_rtt",
  "call_jitter",
  "call_packet_loss",
] as const satisfies readonly UXMetric[];

/** The windows a screen offers, in hours: the last day (the server's default), week and 30 days (all it keeps). */
export const uxWindows = [24, 168, 720] as const;

/** The measurements of the last `hours` hours. */
function measurementsQuery({ queries }: AppData, hours: number) {
  return queries.queryOptions("get", "/admin/ux_measurements", { params: { query: { hours } } });
}

/** Reads the measurements of the last `hours` hours, for a route's loader. */
export async function loadUXMeasurements(data: AppData, hours: number): Promise<void> {
  await data.queryClient.query(measurementsQuery(data, hours)).catch(() => undefined);
}

/**
 * The measurements of the last `hours` hours, and `stale` while those of another window stay shown until these are
 * read.
 */
export function useUXMeasurements(hours: number): StaffOne<UXMeasurementList> & { readonly stale: boolean } {
  const query = useQuery({ ...measurementsQuery(useAppData(), hours), placeholderData: keepPreviousData });
  return { ...oneOf(query), stale: query.isPlaceholderData };
}
