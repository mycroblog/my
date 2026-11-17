// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { fakeHost } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { loadNotifications, Notifications } from "./notifications";

test("the Mentions tab asks the server for the mentions alone, and shows them in place of every notification", async () => {
  const { fake, data } = fakeData({ viewer: "sam" });
  await loadNotifications(data);
  const user = renderFeature(data, <Notifications instance={fakeHost} />);
  const feed = () => screen.getByRole("feed", { name: "Notifications" });
  expect(within(feed()).getByRole("article", { name: "Yuki 雪 followed you" })).toBeDefined();

  const read = fake.script("GET /notifications");
  await user.click(screen.getByRole("tab", { name: "Mentions" }));

  const { url } = await read.arrived();
  expect(url.searchParams.getAll("types")).toContain("mention");
  await waitFor(() => {
    expect(within(feed()).queryByRole("article", { name: "Yuki 雪 followed you" })).toBeNull();
  });
  expect(within(feed()).getAllByRole("article", { name: "Yuki 雪" }).length).toBeGreaterThan(0);
});
