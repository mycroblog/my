// SPDX-License-Identifier: AGPL-3.0-only

// The UX measurements of the app (ADR-0167 §2, §3): the app alone wires them, and no feature writes code for them.
// While the instance measures this browser session (app-data's `measuredSession`), it gives the reports of app-data
// what each layer times: the design system's changes of screen by kind and presses by action ID, the call quality of
// every RoomSession by transport, and the platform's vitals, which the app names by the route's ID (a pattern such as
// `/_app/statuses/$id`, never the URL): the largest contentful paint of the first screen (settled at the reader's
// first input, before any navigation), the worst interaction of each visit of a route (its INP), and each long
// animation frame. The router's changes of screen are timed from the navigation's start to the render of the new
// screen, by the kind the design system moves it by (transitions.ts).

import { measuredSession, measurementReports, type Measurement, type MeasurementReports } from "@my/app-data/measure";
import type { Instance } from "@my/app-data";
import { measureWith, timeTransition } from "@my/design-system/measure";
import { watchVitals, type VitalsEnvironment } from "@my/platform/measure";
import { measureCallsWith } from "@my/rtc/measure";
import { navigationKind, type NavigationChange } from "./transitions";

/** What the measurements read of the router: the route of the screen shown, and the navigations. */
type MeasuredRouter = {
  readonly state: { readonly matches: readonly { readonly routeId: string }[] };
  subscribe(
    event: "onBeforeNavigate" | "onRendered",
    fn: (change: NavigationChange & { pathChanged: boolean }) => void,
  ): () => void;
};

export type MeasuringOptions = {
  readonly router: MeasuredRouter;
  /** The instance, whose `ux_measurement` and `ux_measurement_share` say whether this session is measured. */
  readonly instance: Instance | undefined;
  /** The origin of the server's client API. */
  readonly server: string;
  /** Draws the session (from 0 up to 1; Math.random by default). */
  readonly draw?: () => number;
  /** Sends a report; a beacon by default. */
  readonly send?: (url: string, body: string) => void;
  /** Calls its listener each time the page is hidden; the document's by default. */
  readonly watchHidden?: (listener: () => void) => () => void;
  /** Where the vitals are read; the browser's by default. */
  readonly vitals?: VitalsEnvironment;
};

/** The route that has no other: the root's. */
const rootRoute = "__root__";

/**
 * Measures this browser session if the instance measures it, until the function it returns is called (which sends
 * what is held); null when it does not, and then nothing is timed, held or sent.
 */
export function startMeasuring({
  router,
  instance,
  server,
  draw,
  send,
  watchHidden,
  vitals,
}: MeasuringOptions): (() => void) | null {
  if (!measuredSession(instance, draw)) return null;
  const route = () => router.state.matches.at(-1)?.routeId ?? rootRoute;
  // The worst interaction of the visit of a route on screen, which goes as the visit ends or the page is hidden.
  let visit = route();
  let worst = 0;
  let reports: MeasurementReports | undefined;
  const add = (sample: Measurement) => reports?.add(sample);
  const endVisit = () => {
    if (worst > 0) add({ metric: "inp", name: visit, value: worst });
    worst = 0;
    visit = route();
  };
  const watched = watchVitals(
    {
      largestPaint: (ms) => {
        add({ metric: "lcp", name: route(), value: ms });
      },
      interaction: (ms) => {
        worst = Math.max(worst, ms);
      },
      longFrame: (ms) => {
        add({ metric: "long_frame", name: route(), value: ms });
      },
    },
    vitals,
  );
  reports = measurementReports({
    server,
    ...(send === undefined ? {} : { send }),
    ...(watchHidden === undefined ? {} : { watchHidden }),
    beforeHidden: () => {
      watched.settle();
      endVisit();
    },
  });
  const stopDesign = measureWith(add);
  const stopCalls = measureCallsWith(add);
  let endTransition: (() => void) | undefined;
  const stopStart = router.subscribe("onBeforeNavigate", (change) => {
    endTransition?.();
    endTransition = change.fromLocation === undefined ? undefined : timeTransition(navigationKind(change));
  });
  const stopRendered = router.subscribe("onRendered", (change) => {
    endTransition?.();
    endTransition = undefined;
    if (change.pathChanged) endVisit();
  });
  return () => {
    stopStart();
    stopRendered();
    stopDesign();
    stopCalls();
    watched.settle();
    endVisit();
    watched.stop();
    reports?.stop();
    reports = undefined;
  };
}
