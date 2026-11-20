// SPDX-License-Identifier: AGPL-3.0-only

// The UX measurements of the web client on their way to the instance's own server (ADR-0167 §3, ADR-0103 §6):
// whether this browser session sends them (the instance's `ux_measurement`, and a draw at its `ux_measurement_share`
// kept for the tab's session), and the reports of `POST /client_measurements` that carry them, sent in batches of 50
// at most and whenever the page is hidden, by a beacon with no access token. A sample names a route by its ID, an
// interaction by its kind, an action by its ID, or a call by its transport, and nothing that identifies a person.
// Only the app imports this module (`@my/app-data/measure`).

import type { components } from "@mycroblog/mykit";
import { readTabValue, writeTabValue } from "@my/platform";
import { sendReport, watchPageHidden } from "@my/platform/measure";
import type { Instance } from "./instance";

/** One sample of how fast the web client responded. */
export type Measurement = components["schemas"]["ClientMeasurement"];

/** The most samples of one report (`ClientMeasurementReport`). */
export const maxReportSamples = 50;

/** The share of sessions measured when the instance names none, in percent. */
export const defaultMeasurementShare = 10;

/** The tab's key of the draw of its session: "1" measured, "0" not. */
const drawnKey = "my:ux-measured";

/** The path of the reports under the server's origin. */
export const measurementsPath = "/api/v1/client_measurements";

/**
 * Whether this browser session sends its samples: never while the instance's `ux_measurement` is not true; else as
 * the session was drawn, once, at the instance's `ux_measurement_share` (10 % when it names none), the draw kept in
 * the tab for the rest of its session. `draw` gives a number from 0 up to 1 (Math.random by default).
 */
export function measuredSession(instance: Instance | undefined, draw: () => number = Math.random): boolean {
  if (instance?.ux_measurement !== true) return false;
  const kept = readTabValue(drawnKey);
  if (kept === "1" || kept === "0") return kept === "1";
  const share = instance.ux_measurement_share ?? defaultMeasurementShare;
  const measured = draw() * 100 < share;
  writeTabValue(drawnKey, measured ? "1" : "0");
  return measured;
}

/** The reports of one page: `add` a sample, `flush` what is held now, `stop` hearing the page hide. */
export type MeasurementReports = {
  add: (sample: Measurement) => void;
  flush: () => void;
  stop: () => void;
};

export type MeasurementReportOptions = {
  /** The origin of the server's client API (the shell's `apiOrigin`). */
  server: string;
  /** Sends one report's body to the URL; a beacon by default (@my/platform's `sendReport`). */
  send?: (url: string, body: string) => void;
  /** Calls its listener each time the page is hidden; @my/platform's `watchPageHidden` by default. */
  watchHidden?: (listener: () => void) => () => void;
  /** What runs as the page is hidden, before the held samples are sent (the vitals that end there). */
  beforeHidden?: () => void;
};

/**
 * The reports of the samples to `server`: a report leaves as 50 samples are held, and with what is held each time the
 * page is hidden (after `beforeHidden`), the last moment it surely leaves.
 */
export function measurementReports({
  server,
  send = sendReport,
  watchHidden = watchPageHidden,
  beforeHidden,
}: MeasurementReportOptions): MeasurementReports {
  let held: Measurement[] = [];
  const url = server + measurementsPath;
  const flush = () => {
    if (held.length === 0) return;
    const measurements = held;
    held = [];
    send(url, JSON.stringify({ measurements }));
  };
  const stopHidden = watchHidden(() => {
    beforeHidden?.();
    flush();
  });
  let stopped = false;
  return {
    add: (sample) => {
      if (stopped || !Number.isFinite(sample.value) || sample.value < 0) return;
      held.push({ metric: sample.metric, name: sample.name, value: Math.round(sample.value * 10) / 10 });
      if (held.length >= maxReportSamples) flush();
    },
    flush,
    stop: () => {
      stopped = true;
      stopHidden();
      flush();
    },
  };
}
