// SPDX-License-Identifier: AGPL-3.0-only

// The preload of the component tests happy-dom cannot decide (`*.jsdom.test.tsx`, ADR-0171 §4): jsdom is the global
// DOM before a test module loads (`bun test --preload`, tasks/web.yml).

import globalJsdom from "global-jsdom";

globalJsdom(undefined, { url: "http://localhost/", pretendToBeVisual: true });

// global-jsdom keeps every global Bun already has. jsdom takes only its own of these: it refuses to dispatch an event
// Bun's constructor made and to listen with Bun's signal, and Bun's FormData reads nothing from a form. A part reaches
// them as globals, so they are jsdom's here.
const jsdom: object = window;
for (const name of [
  "EventTarget",
  "Event",
  "CustomEvent",
  "CloseEvent",
  "ErrorEvent",
  "MessageEvent",
  "AbortController",
  "AbortSignal",
  "DOMException",
  "Blob",
  "File",
  "FormData",
])
  Reflect.set(globalThis, name, Reflect.get(jsdom, name));

await import("./component-tests");
