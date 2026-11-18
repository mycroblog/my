// SPDX-License-Identifier: AGPL-3.0-only

// The shell's notices, from what the app's watch of the server knows: the toast of a newer build, which a call holds
// back, and the banners of a server that closes, of a reader offline and of a server that does not answer.

import { expect, test } from "bun:test";
import { AppDataProvider, instanceQuery, type AppData } from "@my/app-data";
import { I18nProvider } from "@my/design-system";
import { fakeStatusPage } from "@my/e2e/fake";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { fakeApp, instanceOf } from "../test/server";
import { ServerBanners, UpdateToast } from "./notices";

/** `node` on `data`, alone in the document: the toast it shows, if any, and what the document says. */
function shown(data: AppData, node: ReactNode) {
  cleanup();
  const { container } = render(
    <AppDataProvider data={data}>
      <I18nProvider locale="en">{node}</I18nProvider>
    </AppDataProvider>,
  );
  const [toast] = screen.queryAllByRole("status");
  return {
    said: container.textContent,
    toast:
      toast === undefined
        ? undefined
        : within(toast)
            .queryAllByRole("button")
            .map((button) => button.textContent || button.getAttribute("aria-label")),
  };
}

test("a newer build ready shows a toast that says so, with Update and a close; none during a call", () => {
  const { watch, data } = fakeApp();
  watch.offerUpdate();

  const ready = shown(data, <UpdateToast inCall={false} />);
  expect(ready.said).toContain("A newer version of the app is ready.");
  expect(ready.toast).toEqual(["Update", "Close"]);

  expect(shown(data, <UpdateToast inCall />)).toEqual({ said: "", toast: undefined });
});

test("the reader may close the toast of a newer build", async () => {
  const user = userEvent.setup();
  const { watch, data } = fakeApp();
  watch.offerUpdate();
  shown(data, <UpdateToast inCall={false} />);

  await user.click(screen.getByRole("button", { name: "Close" }));

  expect(screen.queryAllByRole("status")).toHaveLength(0);
});

test("an update the server requires shows a toast that cannot be closed; none during a call", async () => {
  const { watch, data } = fakeApp({ newerEpoch: true });
  await data.queryClient.query(instanceQuery(data));
  expect(watch.state().update).toBe("required");

  const required = shown(data, <UpdateToast inCall={false} />);
  expect(required.said).toContain("This version of the app no longer works with the server.");
  expect(required.toast).toEqual(["Update"]);

  expect(shown(data, <UpdateToast inCall />)).toEqual({ said: "", toast: undefined });
});

test("no update, no toast", () => {
  const { data } = fakeApp();

  expect(shown(data, <UpdateToast inCall={false} />)).toEqual({ said: "", toast: undefined });
});

/** The banners' region, and the links in it by their names. */
function banners() {
  const region = screen.getByRole("region", { name: "Server and connection status" });
  return {
    region,
    links: within(region)
      .getAllByRole<HTMLAnchorElement>("link")
      .map((link) => [link.textContent, link.getAttribute("href")]),
  };
}

test("the closure notice says when the server closes, leads to the export and the move, and cannot be dismissed", async () => {
  const closing = await instanceOf({ closure: "announced" });
  shown(fakeApp({ closure: "announced" }).data, <ServerBanners instance={closing} />);
  const { region, links } = banners();

  expect(region.textContent).toMatch(/^This server closes on .+\. Export your data by .+\./);
  expect(links).toEqual([
    ["Export your data", "/settings/account/export"],
    ["Move your account", "/settings/move"],
  ]);
  expect(within(region).queryAllByRole("button")).toHaveLength(0);
});

test("a server that answers and does not close shows no notice", async () => {
  expect(shown(fakeApp().data, <ServerBanners instance={await instanceOf()} />).said).toBe("");
  expect(screen.queryAllByRole("region")).toHaveLength(0);
});

/** The banners once the server stopped answering in `down`'s way, after the shell read /instance. */
async function bannersWhen(down: "offline" | "outage") {
  const { watch, data } = fakeApp({ down });
  const read = await data.queryClient.query(instanceQuery(data));
  await data.api.GET("/timelines/home").catch(() => undefined);
  await watch.check();
  expect(watch.state().connection).toBe(down);
  shown(data, <ServerBanners instance={read} />);
  return banners();
}

test("the reader offline and the server down each show their notice, with the status page of the last /instance", async () => {
  const offline = await bannersWhen("offline");
  expect(offline.region.textContent).toContain("You are offline.");
  expect(offline.links).toEqual([["Service status (opens in a new tab)", fakeStatusPage]]);

  const outage = await bannersWhen("outage");
  expect(outage.region.textContent).toContain("The server is not answering.");
  expect(outage.links).toEqual([["Service status (opens in a new tab)", fakeStatusPage]]);
});
