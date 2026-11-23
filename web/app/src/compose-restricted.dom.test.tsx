// SPDX-License-Identifier: AGPL-3.0-only

// A post refused for a restriction of the reader's account (#3216), as the app leads from it: the composer's Account
// status leads to that screen of the settings in the same press that leaves the composer, and what was written is
// still there when New post opens the composer again. What the alert says is the feature's
// (web/app-features/src/compose/restricted.dom.test.tsx).

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { renderApp } from "./test/app";
import { composerHolding, newPost, paste, replace } from "./test/composer";

/** The server's sentence of a post refused for a restriction (server:errors.status.posting_restricted). */
const posting =
  "A moderation action keeps your account from posting for now. To see why or to appeal, open “Account status” under “Account and data” in the settings.";

test("Account status in a refused composer leads to Account status, leaves the composer, and keeps what was written", async () => {
  const text = "Market day: the bakery opens at nine.";
  const app = await renderApp("/lists");
  const composer = await newPost(app);
  await paste(app, composer.text, text);
  app.fake.script("POST /statuses", {
    fail: { type: "/api/v1/problems/posting-restricted", title: "Forbidden", status: 403, detail: posting },
  });

  await app.user.click(within(composer.dialog).getByRole("button", { name: "Post" }));
  const alert = await waitFor(() => {
    const found = within(composer.dialog)
      .queryAllByRole("alert")
      .find((each) => each.textContent.includes(posting));
    if (found === undefined) throw new Error("test: the composer does not say the server's sentence");
    return found;
  });
  await app.user.click(within(alert).getByRole("link", { name: "Account status" }));

  await app.arrived("/settings/account/status");
  expect(app.path()).toBe("/settings/account/status");
  await waitFor(() => {
    if (screen.queryAllByRole("dialog", { name: "New post" }).length > 0)
      throw new Error("test: the composer is still open");
  });

  await newPost(app);
  const kept = await composerHolding("New post", text);

  // What was written is kept on the page, for the process's next test too: emptied and closed, it is kept no more.
  await replace(app, kept.text, "");
  await app.user.click(within(kept.dialog).getByRole("button", { name: "Close" }));
  await waitFor(() => {
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
