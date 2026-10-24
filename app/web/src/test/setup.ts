// SPDX-License-Identifier: AGPL-3.0-only
// Runs before every Vitest file (vitest.config.ts): jest-dom matchers, DOM cleanup and the MSW server.
// The network is mocked only through MSW (web-client/README.md T3); a request that no handler matches fails the test.
import "@testing-library/jest-dom/vitest";
import { cleanup, configure } from "@testing-library/react";
import { destroyAnnouncer } from "react-aria/private/live-announcer/LiveAnnouncer";
import { afterAll, afterEach, beforeAll, beforeEach, vi } from "vitest";
import { forgetUnsavedStorage } from "../lib/storage";
import { server } from "./msw/server";

// findBy and waitFor give up after this long (web-client/README.md T7). The wait is wall-clock time, which a busy
// machine stretches: Testing Library's 1 s default timed out in `scripts/task.sh check` while Go and PostgreSQL ran
// alongside. A test that passes never waits this long; the limit only bounds how long a broken one hangs.
// testTimeout in vitest.config.ts leaves room for several such waits.
configure({ asyncUtilTimeout: 10_000 });

beforeAll(() => {
  server.listen({ onUnhandledFrame: "error" });
  // jsdom has no Element.scrollIntoView; a page that scrolls an element into view (a post's page) calls it.
  Object.defineProperty(Element.prototype, "scrollIntoView", { configurable: true, writable: true, value: () => undefined });
  // Nor does jsdom draw: without the canvas package, getContext logs "Not implemented". The blur of a picture
  // (@my/ui's Blurhash) then draws nothing, as in a browser without a 2D canvas.
  Object.defineProperty(HTMLCanvasElement.prototype, "getContext", { configurable: true, writable: true, value: () => null });
  // The completion of a text field (@my/ui TextAreaAutocomplete) moves virtual focus to an option, and React Aria tells
  // the field with a blur event of its own (its relatedTarget is the option), sent from a React effect. user-event
  // answers each blur of an edited field with a change event wrapped in act, which React refuses while it commits
  // ("Should not already be working"); no browser has such a listener, so these blurs stop on the window, before
  // user-event's listener on the document sees them (as in @my/ui's TextAreaAutocomplete.test.tsx).
  window.addEventListener(
    "blur",
    (event) => {
      if (event.relatedTarget instanceof Element && event.relatedTarget.getAttribute("role") === "option") event.stopImmediatePropagation();
    },
    { capture: true },
  );
});

beforeEach(() => {
  // jsdom has no layout, so it does not implement scrolling; the router restores the scroll position
  // on every navigation. restoreMocks (vitest.config.ts) removes the stub after each test.
  vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
});

afterEach(() => {
  cleanup();
  // React Aria's components announce through a live announcer of its own, kept in a module variable. An
  // announcement made outside act (while waitFor polls) waits for a 100 ms timer, which can fire after the
  // last test of the file, once jsdom is gone ("document is not defined"). Destroying the announcer makes
  // that timer do nothing and gives the next test a fresh one.
  destroyAnnouncer();
  server.resetHandlers();
  // What the app keeps in the browser (lib/storage) must not leak into the next test, nor what it kept in
  // memory when a test blocked the storage.
  localStorage.clear();
  sessionStorage.clear();
  forgetUnsavedStorage();
  // The tests share one document; a theme chosen in one (lib/theme) must not color the next.
  delete document.documentElement.dataset["theme"];
});

afterAll(() => {
  server.close();
});
