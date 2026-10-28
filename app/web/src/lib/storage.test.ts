// SPDX-License-Identifier: AGPL-3.0-only
// The declared keys of lib/storage: the stored format and its checks, the storage being unusable, the
// notifications here and from another tab, and the removal of the user's data when the session ends.
import { act, renderHook } from "@testing-library/react";
import { describe, expect, test, vi } from "vitest";
import {
  autoplayKey,
  clearUserStorage,
  composeKey,
  localeKey,
  signInKey,
  themeKey,
  timelinesKey,
  useStorage,
} from "./storage";

describe("storage", () => {
  test("keeps a value with the version of its format, and removes the key for the default", () => {
    timelinesKey.write({ local: "everyone", federated: "off" });
    expect(localStorage.getItem("my:instance:timelines")).toBe(
      '{"v":2,"value":{"local":"everyone","federated":"off"}}',
    );
    expect(timelinesKey.read()).toEqual({ local: "everyone", federated: "off" });

    autoplayKey.write(false);
    expect(autoplayKey.read()).toBe(false);
    autoplayKey.write(true);
    expect(localStorage.getItem(autoplayKey.name)).toBeNull();
    expect(autoplayKey.read()).toBe(true);
  });

  test("returns the same value while the stored text is the same (a stable snapshot)", () => {
    timelinesKey.write({ local: "off", federated: "logged_in" });
    expect(timelinesKey.read()).toBe(timelinesKey.read());
  });

  test("reads a broken value, another version or a value that fails its check as the default", () => {
    for (const raw of [
      "{not json",
      '"dark"',
      '{"v":2,"value":"dark"}',
      '{"v":1,"value":"sepia"}',
      '{"value":"dark"}',
    ]) {
      localStorage.setItem(themeKey.name, raw);
      expect(themeKey.read()).toBe("system");
    }
    localStorage.setItem(localeKey.name, '{"v":1,"value":""}');
    expect(localeKey.read()).toBeNull();
  });

  test("keeps a write in memory for the page when the storage refuses it", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("full", "QuotaExceededError");
    });
    autoplayKey.write(false);
    expect(autoplayKey.read()).toBe(false);
    expect(localStorage.getItem(autoplayKey.name)).toBeNull();
  });

  test("reads the defaults and keeps writes in memory when the storage cannot be opened", () => {
    const blocked = vi.spyOn(window, "localStorage", "get").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });
    expect(themeKey.read()).toBe("system");
    themeKey.write("dark");
    expect(themeKey.read()).toBe("dark");
    // The test setup clears the storage after the test, before the spies are restored.
    blocked.mockRestore();
  });

  test("useStorage renders again on a write here and on one made in another tab", () => {
    const { result } = renderHook(() => useStorage(autoplayKey));
    expect(result.current).toBe(true);

    act(() => {
      autoplayKey.write(false);
    });
    expect(result.current).toBe(false);

    // Another tab of the origin turned it on: the key is removed there, and its storage event reaches this document.
    act(() => {
      localStorage.removeItem(autoplayKey.name);
      window.dispatchEvent(new StorageEvent("storage", { key: autoplayKey.name, newValue: null }));
    });
    expect(result.current).toBe(true);

    // A clear() in another tab has no key.
    act(() => {
      localStorage.setItem(autoplayKey.name, '{"v":1,"value":false}');
      window.dispatchEvent(new StorageEvent("storage", { key: null }));
    });
    expect(result.current).toBe(false);
  });

  test("the end of a session removes the user's data only", () => {
    const draft = { draft: { text: "Hi" }, outbox: [] };
    composeKey("account-1").write(draft);
    composeKey("account-2").write(draft);
    signInKey.write({ state: "s", codeVerifier: "v", returnTo: "/" });
    themeKey.write("dark");
    sessionStorage.setItem("not-declared", "kept");

    clearUserStorage();

    expect(composeKey("account-1").read()).toBeNull();
    expect(composeKey("account-2").read()).toBeNull();
    expect(Object.keys(sessionStorage).toSorted()).toEqual(["my:oauth-pending", "not-declared"]);
    expect(themeKey.read()).toBe("dark");
  });

  test("a family returns one key per ID", () => {
    expect(composeKey("account-1")).toBe(composeKey("account-1"));
    expect(composeKey("account-1").name).toBe("my:user:compose:account-1");
  });
});
