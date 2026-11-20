// SPDX-License-Identifier: AGPL-3.0-only

// The UX measurement as the app wires it (ADR-0167): a slow press of a control, a slow route and a call's quality
// reach the server's report by name, each timed by its own layer (the design system, @my/platform's vitals named by
// the route, @my/rtc), with nothing in a feature; and while the instance does not measure, or the session was not
// drawn, nothing is timed or sent.

import { afterEach, beforeEach, expect, test } from "bun:test";
import { Button } from "@my/design-system";
import type { Instance } from "@my/app-data";
import { reportQuality } from "@my/rtc/measure";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { startMeasuring } from "./measure";
import { fakeRouter, fakeTimeline, handMadeCall } from "./test/measure";
import { instanceOf } from "./test/server";

const server = "https://my.example";
const statusRoute = "/_app/statuses/$id";
const notificationsRoute = "/_app/notifications";

type Sample = { metric: string; name: string; value: number };

/** Keeps the thread busy for `ms`, as an action that is slow to respond does. */
function busyFor(ms: number): void {
  const until = performance.now() + ms;
  let turns = 0;
  while (performance.now() < until) turns += 1;
  if (turns < 0) throw new Error("unreachable");
}

/** Waits for the next frame to be painted, and the task after it. */
const painted = () =>
  new Promise<void>((resolve) => {
    requestAnimationFrame(() => setTimeout(resolve, 0));
  });

/** The instance of the fake server, measuring at `share` or not at all. */
async function measuring(share: number | null): Promise<Instance> {
  const instance = await instanceOf();
  return share === null
    ? { ...instance, ux_measurement: false }
    : { ...instance, ux_measurement: true, ux_measurement_share: share };
}

/** The app's measurement of a page on a fake router, timeline and beacon: `hide` hides the page. */
function measured(instance: Instance, draw = () => 0) {
  const timeline = fakeTimeline();
  const input = new EventTarget();
  const { router, go } = fakeRouter(statusRoute, "/@alice/statuses/1");
  const reports: { url: string; samples: Sample[] }[] = [];
  const hidden = new Set<() => void>();
  const stop = startMeasuring({
    router,
    instance,
    server,
    draw,
    send: (url, body) => {
      const { measurements } = JSON.parse(body) as { measurements: Sample[] };
      reports.push({ url, samples: measurements });
    },
    watchHidden: (listener) => {
      hidden.add(listener);
      return () => {
        hidden.delete(listener);
      };
    },
    vitals: { Observer: timeline.Observer, target: input, hidden: false },
  });
  return {
    stop,
    timeline,
    go,
    reports,
    firstInput: () => {
      input.dispatchEvent(new Event("pointerdown"));
    },
    hide: () => {
      for (const listener of hidden) listener();
    },
    sent: () => reports.flatMap((report) => report.samples),
  };
}

/** A slow reply button, the control a post's reply is, and a plain one; `press` presses one by its name. */
function shownButtons(slow: number) {
  render(
    <>
      <Button
        data-action="post.reply"
        onPress={() => {
          busyFor(slow);
        }}
      >
        Reply
      </Button>
      <Button onPress={() => undefined}>Plain</Button>
    </>,
  );
  const user = userEvent.setup();
  return async (name: string) => {
    await user.click(screen.getByRole("button", { name }));
    await painted();
    await painted();
  };
}

const ofMetric = (samples: readonly Sample[], metric: string) =>
  samples.filter((s) => s.metric === metric).map(({ name, value }) => ({ name, value }));

let stops: (() => void)[] = [];

beforeEach(() => {
  sessionStorage.clear();
});

afterEach(() => {
  for (const stop of stops) stop();
  stops = [];
  cleanup();
});

test("a slow press, a slow route and a call's quality reach the server's report by name", async () => {
  const page = measured(await measuring(100));
  expect(page.stop).not.toBeNull();
  if (page.stop !== null) stops.push(page.stop);
  const press = shownButtons(80);

  page.timeline.give("largest-contentful-paint", [{ startTime: 2600, duration: 0 }]);
  page.firstInput();
  page.timeline.give("event", [
    { startTime: 3000, duration: 120, interactionId: 1 },
    { startTime: 4000, duration: 320, interactionId: 2 },
    { startTime: 5000, duration: 16, interactionId: 3 },
  ]);
  page.timeline.give("long-animation-frame", [{ startTime: 4000, duration: 150 }]);
  await press("Reply");
  const call = handMadeCall("livekit");
  reportQuality(call.session);
  call.say({ rtt: 80, jitter: 5, packetLoss: 0.02 });

  page.hide();
  expect(page.reports.map((report) => report.url)).toEqual([`${server}/api/v1/client_measurements`]);
  const sent = page.sent();
  expect(ofMetric(sent, "lcp")).toEqual([{ name: statusRoute, value: 2600 }]);
  expect(ofMetric(sent, "inp")).toEqual([{ name: statusRoute, value: 320 }]);
  expect(ofMetric(sent, "long_frame")).toEqual([{ name: statusRoute, value: 150 }]);
  expect(ofMetric(sent, "press").map(({ name }) => name)).toEqual(["post.reply"]);
  expect(ofMetric(sent, "press")[0]?.value).toBeGreaterThanOrEqual(80);
  expect(ofMetric(sent, "call_rtt")).toEqual([{ name: "livekit", value: 80 }]);
  expect(ofMetric(sent, "call_jitter")).toEqual([{ name: "livekit", value: 5 }]);
  expect(ofMetric(sent, "call_packet_loss")).toEqual([{ name: "livekit", value: 2 }]);
});

test("each visit of a route gives its own worst interaction, and the change of screen is named by its kind", async () => {
  const page = measured(await measuring(100));
  if (page.stop !== null) stops.push(page.stop);

  page.timeline.give("largest-contentful-paint", [{ startTime: 1900, duration: 0 }]);
  page.firstInput();
  page.timeline.give("event", [
    { startTime: 3000, duration: 200, interactionId: 1 },
    { startTime: 3500, duration: 90, interactionId: 2 },
  ]);
  page.go({ routeId: notificationsRoute, pathname: "/notifications" });
  await painted();
  await painted();
  page.timeline.give("event", [{ startTime: 6000, duration: 150, interactionId: 3 }]);
  page.timeline.give("long-animation-frame", [{ startTime: 6000, duration: 70 }]);

  page.hide();
  const sent = page.sent();
  expect(ofMetric(sent, "lcp")).toEqual([{ name: statusRoute, value: 1900 }]);
  expect(ofMetric(sent, "inp")).toEqual([
    { name: statusRoute, value: 200 },
    { name: notificationsRoute, value: 150 },
  ]);
  expect(ofMetric(sent, "long_frame")).toEqual([{ name: notificationsRoute, value: 70 }]);
  expect(ofMetric(sent, "transition").map(({ name }) => name)).toEqual(["push"]);
  for (const sample of sent) expect([sample.name, sample.name.includes("/@alice")]).toEqual([sample.name, false]);
});

test("a load's largest paint keeps the route that loaded, when the screen changed before the reader acted", async () => {
  // The reader waits on a post they opened, then goes back with the browser's own button: no input reaches the page.
  const page = measured(await measuring(100));
  if (page.stop !== null) stops.push(page.stop);
  page.timeline.give("largest-contentful-paint", [{ startTime: 1900, duration: 0 }]);
  page.go({ routeId: notificationsRoute, pathname: "/notifications" });
  await painted();
  page.hide();
  expect(ofMetric(page.sent(), "lcp")).toEqual([{ name: statusRoute, value: 1900 }]);
});

test("the stop sends what is held, and nothing is timed after it", async () => {
  const page = measured(await measuring(100));
  const press = shownButtons(0);
  await press("Reply");
  page.stop?.();
  expect(ofMetric(page.sent(), "press").map(({ name }) => name)).toEqual(["post.reply"]);

  await press("Reply");
  handMadeCall("p2p").say({ rtt: 10, jitter: 1, packetLoss: 0 });
  page.hide();
  expect(ofMetric(page.sent(), "press")).toHaveLength(1);
});

test.each([
  ["the instance does not measure", null, 0],
  ["the session was not drawn", 10, 0.5],
] as const)("while %s nothing is timed, held or sent", async (_, share, drawn) => {
  const page = measured(await measuring(share), () => drawn);
  expect(page.stop).toBeNull();
  const press = shownButtons(40);

  page.timeline.give("largest-contentful-paint", [{ startTime: 2600, duration: 0 }]);
  page.firstInput();
  page.timeline.give("event", [{ startTime: 3000, duration: 320, interactionId: 1 }]);
  await press("Reply");
  const call = handMadeCall("sora");
  reportQuality(call.session);
  call.say({ rtt: 80, jitter: 5, packetLoss: 0.02 });
  page.hide();

  expect(page.reports).toEqual([]);
});
