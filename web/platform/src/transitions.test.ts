// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, expect, test } from "bun:test";
import { settleSkippedTransitions, supportsViewTransitions } from "./transitions";

const globals = globalThis as Record<string, unknown>;

afterEach(() => {
  delete globals.document;
  delete globals.CSS;
});

test("a browser runs View Transitions with types only when it has both", () => {
  expect(supportsViewTransitions()).toBe(false);
  globals.document = { startViewTransition: () => undefined };
  globals.CSS = { supports: (q: string) => q === "selector(:active-view-transition-type(a))" };
  expect(supportsViewTransitions()).toBe(true);
  globals.CSS = { supports: () => false };
  expect(supportsViewTransitions()).toBe(false);
  globals.document = {};
  globals.CSS = { supports: () => true };
  expect(supportsViewTransitions()).toBe(false);
});

test("the ready of a transition the page starts is handled, once however often it is asked", () => {
  const handled: unknown[] = [];
  let started = 0;
  const document = {
    startViewTransition(update?: unknown) {
      started += 1;
      const ready = Promise.reject(new Error("skipped"));
      return {
        update,
        ready: {
          catch: (onRejected: (reason: unknown) => unknown) => {
            handled.push(onRejected);
            return ready.catch(onRejected);
          },
        },
      };
    },
  };
  globals.document = document;
  settleSkippedTransitions();
  settleSkippedTransitions();
  const update = () => undefined;
  const transition = (document as unknown as Document).startViewTransition(update) as unknown as { update: unknown };
  expect(started).toBe(1);
  expect(transition.update).toBe(update);
  expect(handled).toHaveLength(1);
});
