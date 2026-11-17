// SPDX-License-Identifier: AGPL-3.0-only

// The announcements as a feature: the list marks the unread ones, and an announcement opened is read.

import { expect, test } from "bun:test";
import { fakeData, renderFeature } from "../test/feature";
import { screen, waitFor, within } from "@testing-library/react";
import { AnnouncementList, AnnouncementPage, loadAnnouncement, loadAnnouncements } from "./announcements";

test("an announcement is unread in the list until its page is opened, which reads it", async () => {
  const { fake, data } = fakeData({ viewer: "sam" });
  await loadAnnouncements(data);
  const { data: all } = await data.api.GET("/announcements");
  const first = all?.items[0];
  if (first === undefined) throw new Error("test: the server has no announcement");
  const unread = () =>
    within(screen.getByRole("article", { name: first.title })).queryAllByRole("img", { name: "Unread" });
  const read = fake.script("POST /announcements/{id}/read", { hold: "request" });
  await loadAnnouncement(data, first.id);
  renderFeature(
    data,
    <>
      <AnnouncementList />
      <AnnouncementPage id={first.id} onBack={() => undefined} />
    </>,
  );

  await read.arrived();
  expect(unread()).toHaveLength(1);
  read.release();
  await waitFor(() => {
    if (unread().length !== 0) throw new Error("the list still says the announcement is unread");
  });
});
