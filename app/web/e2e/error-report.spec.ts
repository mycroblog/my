// SPDX-License-Identifier: AGPL-3.0-only
// A page that fails while it renders reports the error to the server (POST /api/v1/client_errors) as JSON, with the
// route's pattern and the stack of the built scripts. `vite preview` has no client API, so the spec answers
// the local timeline itself with a post the page cannot display, and receives the report.
import type { Request } from "@playwright/test";
import { expect, test } from "./fixtures";

test("a page that fails while it renders reports the error", async ({ page }) => {
  await page.route("**/api/v1/timelines/local*", (route) => route.fulfill({ json: { items: [null] } }));
  const reports: Request[] = [];
  await page.route("**/api/v1/client_errors", async (route) => {
    reports.push(route.request());
    await route.fulfill({ status: 204 });
  });

  await page.goto("/local");

  await expect.poll(() => reports.length).toBe(1);
  const [report] = reports;
  if (report === undefined) throw new Error("no report");
  expect(report.method()).toBe("POST");
  expect(await report.headerValue("content-type")).toMatch(/^application\/json/);
  const body = report.postDataJSON() as Record<string, unknown>;
  expect(body).toMatchObject({ kind: "route_error", route: "/local" });
  expect(body["name"]).toEqual(expect.any(String));
  expect(body["message"]).toEqual(expect.any(String));
  // The stack names the app's own scripts, which the server keeps.
  expect(body["stack"]).toContain(new URL(page.url()).origin);
  expect(new TextEncoder().encode(report.postData() ?? "").length).toBeLessThanOrEqual(16 * 1024);
});
