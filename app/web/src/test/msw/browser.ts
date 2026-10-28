// SPDX-License-Identifier: AGPL-3.0-only
import { setupWorker } from "msw/browser";
import { defaultHandlers } from "./defaultHandlers";

/**
 * The MSW worker of the screen stories (ADR-0112 §1), with the same default handlers as the Vitest server
 * (server.ts). src/test/stories.tsx starts it and gives each story its own overrides.
 */
export const worker = setupWorker(...defaultHandlers);
