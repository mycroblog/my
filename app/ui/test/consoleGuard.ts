// SPDX-License-Identifier: AGPL-3.0-only
// Fails a test of @my/ui that logs a warning: every console.error and console.warn during a test (from React, React
// Aria, the browser or jsdom) fails it, with the messages in the failure. Both projects of ../vitest.config.ts list
// this file first among their setup files, so its afterEach runs after every other one (Vitest runs afterEach hooks
// in the reverse order of registration), and the screenshot step of ../.storybook/vitest.setup.ts is covered too.
// The messages are not printed: the failure carries them. A message logged between tests fails the next one.
// A test that expects a warning spies on the console itself (vi.spyOn(console, "error").mockImplementation(...));
// restoreMocks puts this guard back after it.
import { afterEach } from "vitest";

/**
 * Messages known to be harmless, each with its reason. A message is allowed when it contains the text; it is then
 * dropped, neither failing the test nor printed.
 */
const allowed: readonly { text: string; reason: string }[] = [
  {
    text: "ResizeObserver loop completed with undelivered notifications",
    // React Aria's overlay positioning (useOverlayPosition) moves an open popover or tooltip in its ResizeObserver
    // callback, which resizes the overlay, during the screenshot step's resizes; TanStack Virtual's ResizeObserver
    // flushSyncs the Feed "Virtualized" story's items by design, which grows the observed <body>. The browser delivers
    // the notification one frame later and the layout settles: nothing loops, and the screenshots are stable.
    reason: "an overlay or the virtualized feed resizes in a library's ResizeObserver callback and settles one frame later",
  },
];

const levels = ["error", "warn"] as const;
const logged: string[] = [];

/** Formats a console call as the console prints it: printf-style substitutions, then the remaining arguments. */
function format(args: readonly unknown[]): string {
  const [first, ...rest] = args;
  let message: string;
  if (typeof first === "string") {
    message = first.replace(/%[sdifoOjc%]/g, (directive) => {
      if (directive === "%%") return "%";
      if (rest.length === 0) return directive;
      const value = rest.shift();
      return directive === "%c" ? "" : show(value);
    });
  } else {
    message = show(first);
  }
  return [message, ...rest.map(show)].join(" ");
}

function show(value: unknown): string {
  if (typeof value === "string") return value;
  if (value instanceof Error) return value.stack ?? `${value.name}: ${value.message}`;
  if (typeof value === "object" && value !== null) {
    try {
      return JSON.stringify(value);
    } catch {
      return Object.prototype.toString.call(value);
    }
  }
  return String(value);
}

for (const level of levels) {
  console[level] = (...args: unknown[]) => {
    const message = format(args);
    if (allowed.some(({ text }) => message.includes(text))) return;
    logged.push(`console.${level}: ${message}`);
  };
}

afterEach(() => {
  if (logged.length === 0) return;
  const count = logged.length;
  const messages = logged.splice(0).join("\n\n");
  throw new Error(`the test logged ${count === 1 ? "a warning" : `${count} warnings`} (test/consoleGuard.ts allows none but the known harmless ones):\n\n${messages}`);
});
