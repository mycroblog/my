// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { AppDataProvider, instanceQuery, webClientEpoch, type AppData } from "@my/app-data";
import { I18nProvider } from "@my/design-system";
import type { ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { app, instance, statusPage, type Mode } from "../test/server";
import { ServerBanners, UpdateToast } from "./notices";

const render = (data: AppData, node: ReactNode) =>
  renderToStaticMarkup(
    <AppDataProvider data={data}>
      <I18nProvider locale="en">{node}</I18nProvider>
    </AppDataProvider>,
  );

test("a newer build ready shows its toast, and none during a call", () => {
  const { watch, data } = app();
  watch.offerUpdate();
  expect(render(data, <UpdateToast inCall={false} />)).toContain('role="status"');
  expect(render(data, <UpdateToast inCall />)).toBe("");
});

test("an update the server requires shows its toast, and none during a call", async () => {
  const { watch, data } = app(instance(), webClientEpoch + 1);
  await data.queryClient.query(instanceQuery(data));
  expect(watch.state().update).toBe("required");
  expect(render(data, <UpdateToast inCall={false} />)).toContain('role="status"');
  expect(render(data, <UpdateToast inCall />)).toBe("");
});

test("no update, no toast", () => {
  const { data } = app();
  expect(render(data, <UpdateToast inCall={false} />)).toBe("");
});

test("the closure notice shows and cannot be dismissed", () => {
  const closing = instance({
    closure: { state: "announced", closes_at: "2027-03-31T00:00:00Z", export_until: "2027-04-30T00:00:00Z" },
  });
  const banners = render(app(closing).data, <ServerBanners instance={closing} />);
  expect(banners).not.toBe("");
  expect(banners).not.toContain("<button");
});

test("a server that answers and does not close shows no notice", () => {
  expect(render(app().data, <ServerBanners instance={instance()} />)).toBe("");
});

/** The notices once the server stopped answering in `down`'s way, after the shell read /instance. */
async function noticesWhen(down: Exclude<Mode, "up">) {
  const { s, watch, data } = app();
  const read = await data.queryClient.query(instanceQuery(data));
  s.mode = down;
  await data.api.GET("/timelines/home").catch(() => undefined);
  await watch.check();
  expect(watch.state().connection).toBe(down);
  return render(data, <ServerBanners instance={read} />);
}

test("the reader offline and the server down each show their notice, with the status page of the last /instance", async () => {
  const offline = await noticesWhen("offline");
  const outage = await noticesWhen("outage");
  expect(offline).toContain(`href="${statusPage}"`);
  expect(outage).toContain(`href="${statusPage}"`);
  expect(offline).not.toBe(outage);
});
