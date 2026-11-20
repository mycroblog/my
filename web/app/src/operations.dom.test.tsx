// SPDX-License-Identifier: AGPL-3.0-only

// The UX measurements in the app (#2952, ADR-0167 §3), on the fake API: a holder of `manage_errors` reads them whether
// or not their permissions keep the settings. Without `manage_settings` the screen asks the settings nothing and has no
// form, whether the web client is measured read from the instance; with it, the setting's form stands under them.
// What each screen shows at rest is the stories' (operations.stories.tsx).

import { expect, test } from "bun:test";
import { waitFor } from "@testing-library/react";
import { main, renderApp, type TestApp } from "./test/app";
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

test("with manage_settings, the setting's form stands under the measurements", async () => {
  const app = await renderApp("/admin/ux-measurements", { on: fakeApp({ viewer: "hana", uxMeasurement: true }) });

  expect(await main().findByRole("table", { name: "Time for a screen to appear (LCP)" })).toBeDefined();
  expect(await main().findByRole("button", { name: "Save" })).toBeDefined();
  await settled(app);
});
