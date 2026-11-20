// SPDX-License-Identifier: AGPL-3.0-only

// The server's about page and a document of it on the real server (#3301, FR-705): a visitor, signed out, goes from the
// landing page to /about, reads there the server's name and the facts the API answers of it, and the terms of service
// a fresh administrator published, from the about page's row to the document's page and back. What the pages show in
// each state is the stories' and the route tests' on the fake (web/app, about.stories.tsx, about.dom.test.tsx).

import { randomUUID } from "node:crypto";
import { signIn as apiSignIn } from "../src/seed/sign-in";
import { expect, test } from "./test";

/** Puts back what the test changed of the server's texts, whatever became of the test. */
const restores: (() => unknown)[] = [];
test.afterEach(async () => {
  for (const restore of restores.splice(0).toReversed()) await restore();
});

test("a visitor reads the about page and the terms of service, and goes back", async ({ page, fresh, baseURL }) => {
  if (baseURL === undefined) throw new Error("e2e: no demo server");
  const words = `Terms of the e2e run ${randomUUID().slice(0, 8)}`;

  // A fresh administrator publishes terms of service of the run's own while the test runs, and puts back the ones the
  // server had after it, so that no other spec depends on the order.
  const admin = await fresh.person({ admin: true });
  const api = await apiSignIn(baseURL, admin);
  const texts = await api.GET("/admin/instance_texts");
  const before = texts.data?.items.find((t) => t.key === "terms")?.body ?? "";
  const set = await api.PUT("/admin/instance_texts/{key}", {
    params: { path: { key: "terms" } },
    body: { body: `## Who may use this server\n\n${words}.` },
  });
  expect(set.error).toBeUndefined();
  restores.push(() =>
    api.PUT("/admin/instance_texts/{key}", { params: { path: { key: "terms" } }, body: { body: before } }),
  );

  const instance = (await (await page.request.get("/api/v1/instance")).json()) as {
    name: string;
    version: string;
    source_url: string;
  };
  // From the landing page, which a visitor at the server's address gets.
  await page.goto("/");
  await page.getByRole("link", { name: "More about this server", exact: true }).click();
  await expect(page).toHaveURL(/\/about$/);
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { level: 1 })).toHaveText(`About ${instance.name}`);
  await expect(page).toHaveTitle(`About ${instance.name}`);
  const server = main.getByRole("region", { name: "Server", exact: true });
  await expect(server).toContainText(instance.version);
  await expect(
    server.getByRole("link", { name: instance.source_url.replace(/^https?:\/\//, "").replace(/\/$/, ""), exact: true }),
  ).toBeVisible();

  await main.getByRole("link", { name: "Terms of service", exact: true }).click();
  await expect(page).toHaveURL(/\/about\/terms$/);
  await expect(main.getByRole("heading", { level: 2, name: "Terms of service", exact: true })).toBeVisible();
  await expect(main.getByRole("heading", { name: "Who may use this server", exact: true })).toBeVisible();
  await expect(main).toContainText(words);
  await expect(main).toContainText("Last revised");

  await main.getByRole("button", { name: "About this server", exact: true }).click();
  await expect(page).toHaveURL(/\/about$/);
  await expect(server).toBeVisible();
});
