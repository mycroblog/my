// SPDX-License-Identifier: AGPL-3.0-only

// The UX measurements in the app (#2952, ADR-0167 §1, §3), on the fake API: a holder of `manage_errors` reads them
// whether or not their permissions keep the settings. Without `manage_settings` the screen asks the settings nothing and
// has no form, whether the web client is measured read from the instance; with it, the setting's form stands under
// them and saves the measuring alone. They are those of the last 24 hours until 7 or 30 days are chosen; each metric is
// a table of its routes with their samples and 75th percentile, with the shares under the good and over the poor
// threshold for LCP (2.5 s, 4 s) and INP (200 ms, 500 ms) alone, a percentile past the good one saying so, and one past
// the last bound saying its own row's bound; while measuring is off there is no period to choose. What each screen
// shows at rest is the stories' (operations.stories.tsx); the jobs, the errors and the link card rules are
// operations-actions.dom.test.tsx's.

import { expect, test } from "bun:test";
import type { FakeOptions } from "@my/e2e/fake";
import { waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

/** Waits until the app has no read on its way. */
const settled = (app: TestApp) =>
  waitFor(() => {
    if (app.data.queryClient.isFetching() > 0) throw new Error("test: the screen's reads are on their way");
  });

test("without manage_settings, the UX measurements show with no form, and the settings are not asked", async () => {
  const on = fakeApp({ viewer: "hana", uxMeasurement: true });
  on.fake.script("GET /accounts/verify_credentials", {
    change: (account) => ({ ...account, permissions: account.permissions.filter((p) => p !== "manage_settings") }),
  });
  const asked = on.fake.script("GET /admin/settings");
  const app = await renderApp("/admin/ux-measurements", { on });

  expect(await main().findByRole("table", { name: "Time for a screen to appear (LCP)" })).toBeDefined();
  await settled(app);
  expect(main().queryByRole("button", { name: "Save" })).toBeNull();
  expect(main().queryByText("Measuring is off: the web client sends no samples.")).toBeNull();
  expect(asked.seen).toHaveLength(0);
});

test("without manage_settings and with measuring off, the screen says it is off", async () => {
  const on = fakeApp({ viewer: "hana" });
  on.fake.script("GET /accounts/verify_credentials", {
    change: (account) => ({ ...account, permissions: account.permissions.filter((p) => p !== "manage_settings") }),
  });
  const app = await renderApp("/admin/ux-measurements", { on });

  expect(await main().findByText("Measuring is off: the web client sends no samples.")).toBeDefined();
  await settled(app);
  expect(main().queryByRole("button", { name: "Save" })).toBeNull();
});

/** The app at the UX measurements as hana, the scenario's administrator, once they are read; their reads counted. */
async function atMeasurements(at = "/admin/ux-measurements", options: FakeOptions = { uxMeasurement: true }) {
  const on = fakeApp({ viewer: "hana", ...options });
  const reads = on.fake.script("GET /admin/ux_measurements");
  const app = await renderApp(at, { on });
  await main().findByRole("heading", { level: 2, name: "Measuring" });
  await settled(app);
  return { app, on, reads };
}

/** The period the screen reads the measurements of, as the radio that is checked says it. */
const period = () =>
  within(main().getByRole("radiogroup", { name: "Period" }))
    .getAllByRole("radio")
    .filter((radio) => radio instanceof HTMLInputElement && radio.checked)
    .map((radio) => radio.closest("label")?.textContent ?? "");

/** The rows of the table `name`: its head's words, then each cell's. */
const table = (name: string) => {
  const [head, ...rows] = within(main().getByRole("table", { name })).getAllByRole("row");
  return [
    head === undefined ? [] : within(head).getAllByRole("columnheader").map(words),
    ...rows.map((row) => within(row).getAllByRole("cell").map(words)),
  ];
};

/** The heads of the rows of the table `name`. */
const heads = (name: string) => within(main().getByRole("table", { name })).getAllByRole("rowheader").map(words);

/** The head of a row of `m`: its name, then how many samples it holds. */
const headOf = (m: { name: string; samples: number }) => `${m.name}${m.samples.toLocaleString("en")} samples`;

const lcp = "Time for a screen to appear (LCP)";
const inp = "Time to answer a press (INP)";

test("the measurements are those of the last 24 hours, until 7 or 30 days are chosen", async () => {
  const { app, reads } = await atMeasurements();
  expect(period()).toEqual(["24 hours"]);
  expect(reads.seen.length).toBeGreaterThan(0);
  expect([...new Set(reads.seen.map((r) => r.url.searchParams.get("hours") ?? "24"))]).toEqual(["24"]);

  for (const [choice, hours] of [
    ["7 days", "168"],
    ["30 days", "720"],
  ] as const) {
    await app.user.click(main().getByText(choice, { exact: true }));
    await waitFor(() => {
      if (period()[0] !== choice) throw new Error(`test: the period is ${period().join()}`);
    });
    await settled(app);
    expect(reads.seen.at(-1)?.url.searchParams.get("hours")).toBe(hours);
    expect(app.path()).toBe(`/admin/ux-measurements?hours=${hours}`);
    const { data } = await app.data.api.GET("/admin/ux_measurements", {
      params: { query: { hours: Number(hours) } },
    });
    expect(heads(lcp)).toEqual((data?.items ?? []).filter((m) => m.metric === "lcp").map(headOf));
  }
});

test("each metric has a table of its routes with their samples and 75th percentile, bands for LCP and INP alone", async () => {
  const { app } = await atMeasurements();
  const { data } = await app.data.api.GET("/admin/ux_measurements", { params: { query: { hours: 24 } } });
  const of = (metric: string) => (data?.items ?? []).filter((m) => m.metric === metric);

  expect(
    main()
      .getAllByRole("table")
      .map((t) => t.getAttribute("aria-label")),
  ).toEqual([
    lcp,
    inp,
    "Long frames",
    "Time to navigate",
    "Time to answer an action",
    "Call round-trip time",
    "Call packet loss",
  ]);
  expect(table(lcp)[0]).toEqual(["Route", "Up to 2.5 sec", "Over 4 sec", "75th percentile"]);
  expect(table(inp)[0]).toEqual(["Route", "Up to 200 ms", "Over 500 ms", "75th percentile"]);
  for (const [name, first] of [
    ["Long frames", "Route"],
    ["Time to navigate", "Kind"],
    ["Time to answer an action", "Action"],
    ["Call round-trip time", "Transport"],
    ["Call packet loss", "Transport"],
  ] as const)
    expect([name, table(name)[0]]).toEqual([name, [first, "75th percentile"]]);

  expect(heads(lcp)).toEqual(of("lcp").map(headOf));
  expect(heads(inp)).toEqual(of("inp").map(headOf));
  expect(heads("Call packet loss")).toEqual(of("call_packet_loss").map(headOf));
  // The share of the samples up to the good threshold and over the poor one, of the buckets the server counted.
  expect(table(lcp).slice(1)).toEqual([
    ["98%", "1%", "1 sec"],
    ["95%", "2%", "1.5 sec"],
    ["99%", "0%", "1 sec"],
  ]);
  expect(
    table(inp)
      .slice(1)
      .map((cells) => cells.at(-1)),
  ).toEqual(["100 ms", "200 ms"]);
  expect(table("Call packet loss").slice(1)).toEqual([["1%"]]);
});

test("a 75th percentile past the good threshold says how far, and one past the last bound says its row's bound", async () => {
  const on = fakeApp({ viewer: "hana", uxMeasurement: true });
  on.fake.script("GET /admin/ux_measurements", {
    change: (page) => ({
      ...page,
      items: page.items.map((m, i) => {
        // LCP of /_app/ at the poor threshold, LCP of a thread past the last bound of its own buckets, INP of /_app/
        // between its thresholds, and INP of a thread past its poor threshold.
        if (i === 0) return { ...m, p75: 4000 };
        if (i === 1) {
          const { p75: _p75, ...rest } = m;
          return { ...rest, buckets: [{ le: 300, samples: 1 }, { le: 6000, samples: 2 }, { samples: 300 }] };
        }
        if (i === 2) return { ...m, p75: 2500 };
        if (i === 3) return { ...m, p75: 300 };
        if (i === 4) return { ...m, p75: 800 };
        return m;
      }),
    }),
  });
  const app = await renderApp("/admin/ux-measurements", { on });
  await main().findByRole("table", { name: lcp });
  await settled(app);

  expect(
    table(lcp)
      .slice(1)
      .map((cells) => cells.at(-1)),
  ).toEqual(["4 sec, needs improvement", "Over 6 sec, poor", "2.5 sec"]);
  expect(
    table(inp)
      .slice(1)
      .map((cells) => cells.at(-1)),
  ).toEqual(["300 ms, needs improvement", "800 ms, poor"]);
});

test("the setting's own Save sends the measuring alone, and once it is off the period leaves the screen", async () => {
  const { app, on } = await atMeasurements();
  const patch = on.fake.script("PATCH /admin/settings");
  expect(main().getAllByRole("radiogroup", { name: "Period" })).toHaveLength(1);

  await app.user.selectOptions(main().getByRole("combobox", { name: "Measure the web client" }), "Off");
  await app.user.click(main().getByRole("button", { name: "Save" }));

  expect((await patch.arrived()).body).toEqual({ values: { ux_measurement: "false" } });
  expect(await main().findByText("Measuring is off: the web client sends no samples.")).toBeDefined();
  await waitFor(() => {
    if (main().queryAllByRole("radiogroup", { name: "Period" }).length > 0)
      throw new Error("test: the period stays on the screen while measuring is off");
  });
  expect(main().queryAllByRole("table")).toHaveLength(0);
});

test("while measuring is on, the share of the sessions is saved with it, and an empty share is refused", async () => {
  const { app, on } = await atMeasurements();
  const patch = on.fake.script("PATCH /admin/settings");
  const share = main().getByRole("textbox", { name: "Sessions measured" });
  expect((share as HTMLInputElement).value).toBe("10");

  await app.user.clear(share);
  await app.user.click(main().getByRole("button", { name: "Save" }));
  expect(await main().findByText("Enter a whole number from 1 to 100.")).toBeDefined();

  await app.user.type(share, "25");
  await app.user.click(main().getByRole("button", { name: "Save" }));
  expect((await patch.arrived()).body).toEqual({ values: { ux_measurement: "true", ux_measurement_share: "25" } });
});

test("while measuring is off, the share of the sessions is not offered", async () => {
  const { app } = await atMeasurements();
  await app.user.selectOptions(main().getByRole("combobox", { name: "Measure the web client" }), "Off");
  expect(main().queryByRole("textbox", { name: "Sessions measured" })).toBeNull();
});

for (const settings of [true, false])
  test(`while measuring is off, ${settings ? "with" : "without"} manage_settings, the screen has no period to choose`, async () => {
    const on = fakeApp({ viewer: "hana" });
    on.fake.script("GET /accounts/verify_credentials", {
      change: (account) => ({
        ...account,
        permissions: account.permissions.filter((p) => settings || p !== "manage_settings"),
      }),
    });
    const app = await renderApp("/admin/ux-measurements", { on });

    expect(await main().findByText("Measuring is off: the web client sends no samples.")).toBeDefined();
    await settled(app);
    expect(main().queryAllByRole("radiogroup", { name: "Period" })).toHaveLength(0);
    expect(main().queryAllByRole("radio")).toHaveLength(0);
  });

test("with manage_settings, the setting's form stands under the measurements", async () => {
  const app = await renderApp("/admin/ux-measurements", { on: fakeApp({ viewer: "hana", uxMeasurement: true }) });

  expect(await main().findByRole("table", { name: "Time for a screen to appear (LCP)" })).toBeDefined();
  expect(await main().findByRole("button", { name: "Save" })).toBeDefined();
  await settled(app);
});
