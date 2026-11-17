// SPDX-License-Identifier: AGPL-3.0-only

// A list's screen, as the client treats the server's answer. That a member's new post arrives in a list by the stream
// is the E2E tests' (web/e2e/specs/lists.spec.ts).

import { expect, test } from "bun:test";
import { main, renderApp } from "./test/app";

test("a list that no longer exists says so", async () => {
  // An ID of the server's form that no list has.
  await renderApp("/lists/01890a5d-ac96-774b-bcce-b302099a8057");

  expect(await main().findByText("This list no longer exists.")).toBeDefined();
});
