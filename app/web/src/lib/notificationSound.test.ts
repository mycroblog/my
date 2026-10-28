// SPDX-License-Identifier: AGPL-3.0-only
// The notification sound (ADR-0109 §5): which notifications play it, the burst rule, and a browser that blocks autoplay.
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import {
  notificationSoundQuietMs,
  playNotificationSound,
  setNotificationSoundEnabled,
  setNotificationSoundTypes,
  tryNotificationSound,
} from "./notificationSound";
import { notificationSoundKey } from "./storage";

/** Stands in for the browser's playback, which jsdom does not implement; `blocked` plays as a browser that blocks autoplay. */
function stubPlayback({ blocked = false } = {}) {
  return vi
    .spyOn(HTMLMediaElement.prototype, "play")
    .mockImplementation(() =>
      blocked ? Promise.reject(new DOMException("not allowed", "NotAllowedError")) : Promise.resolve(),
    );
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"], now: new Date("2026-10-28T10:00:00Z") });
});

afterEach(() => {
  vi.useRealTimers();
});

describe("the settings of the notification sound", () => {
  test("are off by default, with no type", () => {
    expect(notificationSoundKey.read()).toEqual({ enabled: false, types: [] });
  });

  test("keep the types of a row when the sound is turned off and on", () => {
    setNotificationSoundTypes(["voice_room_started", "voice_room_invited"], true);
    setNotificationSoundTypes(["mention"], true);
    setNotificationSoundEnabled(true);
    setNotificationSoundTypes(["voice_room_started", "voice_room_invited"], false);
    setNotificationSoundEnabled(false);

    expect(notificationSoundKey.read()).toEqual({ enabled: false, types: ["mention"] });
  });
});

describe("a new notification", () => {
  test("plays nothing while the sound is off, even for a type that is on", () => {
    const play = stubPlayback();
    setNotificationSoundTypes(["mention"], true);

    playNotificationSound("mention");

    expect(play).not.toHaveBeenCalled();
  });

  test("plays the sound for a type that is on, and nothing for the others", () => {
    const play = stubPlayback();
    setNotificationSoundTypes(["mention"], true);
    setNotificationSoundEnabled(true);

    playNotificationSound("boost");
    expect(play).not.toHaveBeenCalled();
    playNotificationSound("mention");
    expect(play).toHaveBeenCalledOnce();
  });

  test("plays once for a burst, and again after a quiet moment", () => {
    const play = stubPlayback();
    setNotificationSoundTypes(["mention", "follow"], true);
    setNotificationSoundEnabled(true);

    playNotificationSound("mention");
    vi.advanceTimersByTime(1000);
    playNotificationSound("follow");
    vi.advanceTimersByTime(notificationSoundQuietMs - 1);
    // Still within the quiet time of the last one, so the burst goes on.
    playNotificationSound("mention");
    expect(play).toHaveBeenCalledOnce();

    vi.advanceTimersByTime(notificationSoundQuietMs);
    playNotificationSound("mention");
    expect(play).toHaveBeenCalledTimes(2);
  });

  test("plays nothing, and fails nothing, while the browser blocks autoplay", async () => {
    const play = stubPlayback({ blocked: true });
    setNotificationSoundTypes(["mention"], true);
    setNotificationSoundEnabled(true);

    playNotificationSound("mention");

    expect(play).toHaveBeenCalledOnce();
    await expect(play.mock.results[0]?.value).rejects.toThrow("not allowed");
  });
});

describe("the button that plays the sound", () => {
  test("plays it and says so", async () => {
    const play = stubPlayback();

    await expect(tryNotificationSound()).resolves.toBe(true);
    expect(play).toHaveBeenCalledOnce();
  });

  test("says when the browser did not play it", async () => {
    stubPlayback({ blocked: true });

    await expect(tryNotificationSound()).resolves.toBe(false);
  });
});
