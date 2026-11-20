// SPDX-License-Identifier: AGPL-3.0-only

// Which browser sessions send their UX samples, and how the samples leave (ADR-0167 §3): none while the instance's
// `ux_measurement` is off; else a session drawn once at the instance's share and kept in the tab, its samples sent to
// the client API's report in bodies of at most 50, and with what is held each time the page hides.

import { afterEach, beforeEach, expect, test } from "bun:test";
import { instanceQuery, type Instance } from "./instance";
import { defaultMeasurementShare, measuredSession, measurementReports, type Measurement } from "./measure";
import { fakeData } from "./test/data";

/** The tab's storage, which Bun has none of: a Map behind the Storage interface. */
function tabStorage(): Storage {
  const items = new Map<string, string>();
  return {
    get length() {
      return items.size;
    },
    clear: () => {
      items.clear();
    },
    getItem: (key) => items.get(key) ?? null,
    key: (index) => [...items.keys()][index] ?? null,
    removeItem: (key) => {
      items.delete(key);
    },
    setItem: (key, value) => {
      items.set(key, value);
    },
  };
}

let storage = tabStorage();

beforeEach(() => {
  storage = tabStorage();
  Object.defineProperty(globalThis, "sessionStorage", { configurable: true, value: storage });
});

afterEach(() => {
  Reflect.deleteProperty(globalThis, "sessionStorage");
});

/** The fake server's instance, with the measurement settings `settings` names. */
async function instanceWith(settings: Partial<Pick<Instance, "ux_measurement" | "ux_measurement_share">>) {
  const { data } = fakeData();
  const {
    ux_measurement: _on,
    ux_measurement_share: _share,
    ...rest
  } = await data.queryClient.query(instanceQuery(data));
  return { ...rest, ...settings };
}

/** A draw that gives `value` and counts its calls. */
function drawOf(value: number) {
  const draw = () => {
    draw.calls += 1;
    return value;
  };
  draw.calls = 0;
  return draw;
}

test("no session is measured while the instance does not measure, and none is drawn", async () => {
  const draw = drawOf(0);
  expect(measuredSession(undefined, draw)).toBe(false);
  expect(measuredSession(await instanceWith({}), draw)).toBe(false);
  expect(measuredSession(await instanceWith({ ux_measurement: false, ux_measurement_share: 100 }), draw)).toBe(false);
  expect(draw.calls).toBe(0);
});

test("a session is measured as it is drawn within the instance's share", async () => {
  expect(measuredSession(await instanceWith({ ux_measurement: true, ux_measurement_share: 100 }), drawOf(0.99))).toBe(
    true,
  );
  storage.clear();
  expect(measuredSession(await instanceWith({ ux_measurement: true, ux_measurement_share: 30 }), drawOf(0.2))).toBe(
    true,
  );
  storage.clear();
  expect(measuredSession(await instanceWith({ ux_measurement: true, ux_measurement_share: 30 }), drawOf(0.5))).toBe(
    false,
  );
  storage.clear();
  expect(measuredSession(await instanceWith({ ux_measurement: true, ux_measurement_share: 1 }), drawOf(0.5))).toBe(
    false,
  );
});

test("an instance that names no share measures one session in ten", async () => {
  expect(defaultMeasurementShare).toBe(10);
  const on = await instanceWith({ ux_measurement: true });
  expect(measuredSession(on, drawOf(0.05))).toBe(true);
  storage.clear();
  expect(measuredSession(on, drawOf(0.2))).toBe(false);
});

test("a tab's session is drawn once, measured or not, and kept in the tab", async () => {
  const on = await instanceWith({ ux_measurement: true, ux_measurement_share: 50 });
  expect(measuredSession(on, drawOf(0.1))).toBe(true);
  expect(storage.getItem("my:ux-measured")).not.toBeNull();
  const again = drawOf(0.9);
  expect(measuredSession(on, again)).toBe(true);
  expect(again.calls).toBe(0);

  storage.clear();
  expect(measuredSession(on, drawOf(0.9))).toBe(false);
  expect(measuredSession(on, drawOf(0))).toBe(false);
});

test("a session drawn in turns measures nothing once the instance stops measuring", async () => {
  expect(measuredSession(await instanceWith({ ux_measurement: true, ux_measurement_share: 100 }), drawOf(0))).toBe(
    true,
  );
  expect(measuredSession(await instanceWith({ ux_measurement: false, ux_measurement_share: 100 }), drawOf(0))).toBe(
    false,
  );
});

const server = "https://my.example";
const sample = (value: number, name = "/_app/"): Measurement => ({ metric: "lcp", name, value });

/** The reports of a page whose hiding the test calls with `hide`, and what they sent. */
function reportsOf(beforeHidden?: () => void) {
  const sent: { url: string; body: unknown }[] = [];
  let hide: () => void = () => undefined;
  let watching = false;
  const reports = measurementReports({
    server,
    send: (url, body) => {
      sent.push({ url, body: JSON.parse(body) });
    },
    watchHidden: (listener) => {
      hide = listener;
      watching = true;
      return () => {
        watching = false;
      };
    },
    ...(beforeHidden === undefined ? {} : { beforeHidden }),
  });
  return {
    reports,
    sent,
    hide: () => {
      hide();
    },
    watching: () => watching,
  };
}

test("a report leaves to the client API as 50 samples are held, with those 50", () => {
  const { reports, sent } = reportsOf();
  for (let i = 1; i < 50; i++) reports.add(sample(i));
  expect(sent).toEqual([]);
  reports.add(sample(50));
  expect(sent).toEqual([
    {
      url: "https://my.example/api/v1/client_measurements",
      body: { measurements: Array.from({ length: 50 }, (_, i) => sample(i + 1)) },
    },
  ]);
  reports.add(sample(51));
  expect(sent).toHaveLength(1);
  reports.stop();
});

test("what is held leaves as the page hides, after what runs before the hiding", () => {
  const order: string[] = [];
  const { reports, sent, hide } = reportsOf(() => {
    order.push("before");
    if (order.length === 1) reports.add(sample(2500, "/_app/notifications"));
  });
  reports.add({ metric: "press", name: "post.reply", value: 120 });
  hide();
  expect(order).toEqual(["before"]);
  expect(sent.map(({ body }) => body)).toEqual([
    {
      measurements: [
        { metric: "press", name: "post.reply", value: 120 },
        { metric: "lcp", name: "/_app/notifications", value: 2500 },
      ],
    },
  ]);
  hide();
  expect(order).toEqual(["before", "before"]);
  expect(sent).toHaveLength(1);
  reports.stop();
});

test("a value leaves rounded to a tenth", () => {
  const { reports, sent } = reportsOf();
  reports.add(sample(12.345));
  reports.add(sample(2.06));
  reports.add({ metric: "call_packet_loss", name: "p2p", value: 0.04 });
  reports.flush();
  expect(sent.map(({ body }) => body)).toEqual([
    {
      measurements: [sample(12.3), sample(2.1), { metric: "call_packet_loss", name: "p2p", value: 0 }],
    },
  ]);
  reports.stop();
});

test("flush sends what is held now, and nothing when nothing is", () => {
  const { reports, sent } = reportsOf();
  reports.flush();
  expect(sent).toEqual([]);
  reports.add(sample(10));
  reports.flush();
  reports.flush();
  expect(sent.map(({ body }) => body)).toEqual([{ measurements: [sample(10)] }]);
  reports.stop();
});

test("stop no longer hears the page hide", () => {
  const { reports, watching } = reportsOf();
  expect(watching()).toBe(true);
  reports.stop();
  expect(watching()).toBe(false);
});
