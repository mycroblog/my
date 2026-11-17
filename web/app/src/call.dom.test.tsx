// SPDX-License-Identifier: AGPL-3.0-only

// A call the reader is in, as the shell shows it on every screen: its bar, whose microphone mutes and unmutes, stays
// while the reader moves between screens, and a newer build of the app that waits is not offered meanwhile. What the
// bar's parts are is the design system's (shell.dom.test.tsx), what the store tells the voice feature the call's
// (web/app-features/src/call/store.test.ts), and when the toast shows the notices' (shell/notices.dom.test.tsx).

import { expect, onTestFinished, test } from "bun:test";
import { enterCall, leaveCall } from "@my/app-features";
import { screen, waitFor, within } from "@testing-library/react";
import { renderApp } from "./test/app";
import { fakeApp } from "./test/server";

test("the bar of a call stays across the screens, its microphone mutes and unmutes, and no toast offers a newer build meanwhile", async () => {
  enterCall({
    roomId: "morning-coffee",
    room: "Morning coffee chat",
    host: "Sam Rivera",
    participants: 12,
    muted: false,
  });
  onTestFinished(leaveCall);
  const on = fakeApp();
  on.watch.offerUpdate();
  const app = await renderApp("/lists", { on });
  /**
   * The call's bar, a group named by its room, and whether its microphone is muted. The shell keeps a bar per width
   * (beside the rail, and in a phone's dock), and no CSS chooses one on a simulation: the first of them.
   */
  const bar = () => {
    const [first] = screen.getAllByRole("group", { name: "Morning coffee chat" });
    if (first === undefined) throw new Error("test: the shell shows no bar of the call");
    return within(first);
  };
  const muted = () => bar().getByRole("button", { name: "Mute" }).getAttribute("aria-pressed");
  const becomes = (pressed: string) =>
    waitFor(() => {
      if (muted() !== pressed) throw new Error(`the microphone's button is pressed: ${String(muted())}`);
    });

  expect(muted()).toBe("false");
  expect(screen.queryAllByRole("button", { name: "Update" })).toHaveLength(0);

  await app.user.click(bar().getByRole("button", { name: "Mute" }));
  await becomes("true");
  await app.navigate("/notifications/follow_requests");
  expect(muted()).toBe("true");

  await app.user.click(bar().getByRole("button", { name: "Mute" }));
  await becomes("false");
  await app.navigate("/lists");
  expect(muted()).toBe("false");
  expect(screen.queryAllByRole("button", { name: "Update" })).toHaveLength(0);

  // Once the call is left, the bar goes and the newer build is offered.
  await app.user.click(bar().getByRole("button", { name: "Leave" }));
  expect(await screen.findByRole("button", { name: "Update" })).toBeDefined();
  expect(screen.queryAllByRole("group", { name: "Morning coffee chat" })).toHaveLength(0);
});
