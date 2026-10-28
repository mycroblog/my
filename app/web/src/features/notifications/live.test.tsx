// SPDX-License-Identifier: AGPL-3.0-only
// Notifications from the event stream through the whole app (web-client/README.md S10, T4; FR-207): they update
// the unread badge of the navigation.
import { screen, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import { apiBase, resumableSession } from "../../test/msw/http";
import { notificationsFixture } from "./test/handlers";
import { otherAccountFixture } from "../../test/fixtures";
import { streamingServer } from "../../test/msw/streaming";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import type { Notification, NotificationMarker } from "./api/notifications";

const newNotification: Notification = {
  id: "0192f0c4-7a8e-7000-8000-0000000000c1",
  type: "follow",
  created_at: "2026-10-16T10:00:00Z",
  account: otherAccountFixture,
};

describe("a notification from the stream", () => {
  test("adds one to the unread badge without reading the marker again", async () => {
    const streams = streamingServer();
    let markerReads = 0;
    const marker: NotificationMarker = { last_read_id: notificationsFixture[1]?.id ?? "", unread_count: 1 };
    server.use(
      resumableSession(),
      streams.handler,
      http.get(`${apiBase}/notifications/marker`, () => {
        markerReads += 1;
        return HttpResponse.json(marker);
      }),
    );
    await renderApp({ path: "/home" });
    expect(await screen.findByRole("link", { name: "Notifications, 1 unread" })).toBeInTheDocument();
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });

    streams.current().send("notification", newNotification);

    expect(await screen.findByRole("link", { name: "Notifications, 2 unread" })).toBeInTheDocument();
    expect(markerReads).toBe(1);
  });

  test("reads the unread count again after the stream reconnected", async () => {
    const streams = streamingServer();
    let marker: NotificationMarker = { last_read_id: notificationsFixture[1]?.id ?? "", unread_count: 1 };
    server.use(
      resumableSession(),
      streams.handler,
      http.get(`${apiBase}/notifications/marker`, () => HttpResponse.json(marker)),
    );
    await renderApp({ path: "/home" });
    expect(await screen.findByRole("link", { name: "Notifications, 1 unread" })).toBeInTheDocument();
    await waitFor(() => {
      expect(streams.connections()).toBe(1);
    });

    // Two notifications arrive while the stream is down.
    marker = { ...marker, unread_count: 3 };
    streams.current().close();

    expect(
      await screen.findByRole("link", { name: "Notifications, 3 unread" }, { timeout: 3_000 }),
    ).toBeInTheDocument();
  });
});
