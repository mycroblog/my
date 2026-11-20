// SPDX-License-Identifier: AGPL-3.0-only

// What a component test needs once a DOM is registered (ADR-0171 §4): the preloads happy-dom.ts and jsdom.ts load it
// after they registered theirs, since React, Testing Library and Motion look for `document` when they are loaded.

import { afterEach } from "bun:test";
import { cleanup } from "@testing-library/react";
import { MotionGlobalConfig } from "motion/react";

// React warns about an update outside act() only where this says the tests wrap theirs, as Testing Library does.
(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// Neither simulation has an animation engine: Motion finishes every animation at once, so what leaves is gone.
MotionGlobalConfig.skipAnimations = true;

// Each test starts from an empty document. React Aria's announcer lives on the body, outside what Testing Library
// renders, and keeps what a test announced; its logs are emptied as its own clearAnnouncer does, the node left in place
// for the next test's announcements.
afterEach(() => {
  cleanup();
  for (const log of document.querySelectorAll("[data-live-announcer] [role='log']")) log.replaceChildren();
});
