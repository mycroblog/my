// SPDX-License-Identifier: AGPL-3.0-only

// Where the focus lands on the screens under the entry Account and data when it lands on what is no control (#3203):
// the status that says an archive was requested, that an import started, that an alias was added; the row of an
// alias removed; the title of a summary that takes the place of a form (the account moved, its move cancelled, a
// deactivation or a deletion refused once confirmed). jsdom gives the focus only to what can take it, which happy-dom
// does not ask; what each screen says and sends is the app's route tests (web/app, settings-account-data.dom.test.tsx
// and settings-account-leave.dom.test.tsx).

import { screen, waitFor, within } from "@testing-library/react";
import { describe, expect, test } from "bun:test";
import { File as RuntimeFile } from "node:buffer";
import type { ReactNode } from "react";
import { chooseFile } from "../test/profile-form";
import { focused, words } from "../test/security";
import { requestsOnJsdom, settingsOn, type addressProps, type Given } from "../test/settings";
import { AccountAliasesScreen } from "./account-aliases";
import { AccountDeactivateScreen, AccountDeleteScreen } from "./account-end";
import { AccountExportScreen } from "./account-export";
import { AccountImportScreen } from "./account-import";
import { AccountMoveScreen } from "./account-move";

type Props = ReturnType<typeof addressProps>;

const target = "sora@photos.example";

/** Draws `draw` alone on jsdom on a server of `given`, once it read what it shows. */
async function drawn(draw: (props: Props) => ReactNode, shown: () => Promise<unknown>, given: Given = {}) {
  requestsOnJsdom();
  const on = await settingsOn({}, draw, given);
  await shown();
  return on;
}

/** Waits until the focus is on what `says` names, as a reader's tools name it ("status: …", "heading: …"). */
const focusSays = (says: string | RegExp) =>
  waitFor(() => {
    const now = focused();
    if (typeof says === "string" ? now !== says : !says.test(now)) throw new Error(`test: the focus is on ${now}`);
  });

/** The section named `name`. */
const region = (name: string) => screen.getByRole("region", { name });

/** Confirms the dialog titled `title` with yuki's password, pressing its button `act`. */
async function confirm(user: Awaited<ReturnType<typeof settingsOn>>["user"], title: string, act: string) {
  const dialog = await screen.findByRole("dialog", { name: title });
  await user.type(within(dialog).getByLabelText("Current password"), "demo-password-yuki");
  await user.click(within(dialog).getByRole("button", { name: act }));
}

describe("a status that says what was done takes the focus", () => {
  test("Export: the archive was requested", async () => {
    const { user } = await drawn(
      (props) => <AccountExportScreen {...props} />,
      () => screen.findByRole("button", { name: "Request an archive" }),
    );

    await user.click(screen.getByRole("button", { name: "Request an archive" }));

    await focusSays("status: The archive was requested.");
  });

  test("Import: the import started", async () => {
    const { user } = await drawn(
      (props) => <AccountImportScreen {...props} />,
      () => screen.findByRole("list", { name: "Recent imports" }),
    );
    // The runtime's file, not jsdom's, which the runtime would not read (profile-form.tsx's `imageFile`).
    const file = new RuntimeFile(["marco@demo.test\nsam@demo.test"], "follows.csv", { type: "text/csv" });
    await chooseFile(user, screen.getByRole("button", { name: "Choose a file" }), file);
    await waitFor(() => within(region("Preview")).getByRole("list"));

    await user.click(screen.getAllByRole("button", { name: "Import" }).at(-1) ?? document.body);

    await focusSays("status: The import started. It goes on in the background, and you can leave this page.");
  });

  test("Aliases: the alias was added", async () => {
    const { user } = await drawn(
      (props) => <AccountAliasesScreen {...props} />,
      () => screen.findByRole("textbox", { name: "Account you are moving from" }),
    );
    await user.type(screen.getByRole("textbox", { name: "Account you are moving from" }), target);

    await user.click(screen.getByRole("button", { name: "Add" }));

    await focusSays("status: The alias was added.");
  });
});

// Finding: the row stays and reads Removed, but the focus, on its Remove as it was pressed, falls to the page as the
// button goes: a reader of the keyboard starts again from the top (on happy-dom too, which focuses whatever is asked).
test.failing("Aliases: Remove leaves the row in place, reading Removed, with the focus on it", async () => {
  const { user } = await drawn(
    (props) => <AccountAliasesScreen {...props} />,
    () => screen.findByRole("list", { name: "1 of 5 aliases" }),
  );

  await user.click(
    within(
      within(screen.getByRole("list", { name: "1 of 5 aliases" })).getByRole("group", { name: "@yuki@old.example" }),
    ).getByRole("button", { name: "Remove" }),
  );

  const row = await waitFor(() => {
    const [found] = screen.queryAllByRole("listitem").filter((item) => words(item).includes("Removed"));
    if (found === undefined) throw new Error("test: no row reads Removed");
    return found;
  });
  // The focus has had its turn to move.
  await new Promise((resolve) => setTimeout(resolve, 50));
  expect(row.contains(document.activeElement)).toBe(true);
});

describe("a summary that takes the place of the form takes the focus on its title", () => {
  test("Move account: the account moved", async () => {
    const { user } = await drawn(
      (props) => <AccountMoveScreen {...props} />,
      () => screen.findByRole("textbox", { name: "Account you are moving to" }),
    );
    await user.type(screen.getByRole("textbox", { name: "Account you are moving to" }), target);
    await user.click(screen.getAllByRole("button", { name: "Move account" }).at(-1) ?? document.body);

    await confirm(user, `Move this account to @${target}?`, "Move account");

    await focusSays(`heading: This account moved to @${target}`);
  });

  test("Move account: the move is cancelled", async () => {
    const { user } = await drawn(
      (props) => <AccountMoveScreen {...props} />,
      () => screen.findByRole("button", { name: "Cancel the move" }),
      { server: { settings: { movedTo: target } } },
    );
    await user.click(screen.getByRole("button", { name: "Cancel the move" }));
    const asking = await screen.findByRole("alertdialog");

    await user.click(within(asking).getAllByRole("button", { name: "Cancel the move" }).at(-1) ?? document.body);

    await focusSays("heading: The move is cancelled");
  });

  test("Deactivate account: refused once confirmed, after the server learned why", async () => {
    const on = await drawn(
      (props) => <AccountDeactivateScreen {...props} />,
      () => screen.findByRole("button", { name: "Deactivate account" }),
    );
    on.fake.script("GET /accounts/deactivation", {
      change: (body) => ({ ...body, refusal: "paid_subscription" as const }),
    });
    on.fake.script("POST /accounts/deactivation", {
      fail: {
        type: "/api/v1/problems/deactivation-refused",
        title: "This account cannot be deactivated now.",
        status: 409,
        detail: "Cancel your supporter plan first.",
      },
    });
    await on.user.click(screen.getByRole("button", { name: "Deactivate account" }));

    await confirm(on.user, "Deactivate this account?", "Deactivate account");

    await focusSays("heading: This account cannot be deactivated now");
  });

  test("Delete account: the only administrator, refused once confirmed", async () => {
    const { user } = await drawn(
      (props) => <AccountDeleteScreen {...props} />,
      () => screen.findByRole("button", { name: "Delete account" }),
      { server: { settings: { deletionRefusal: "last_administrator" } } },
    );
    await user.click(screen.getByRole("button", { name: "Delete account" }));

    await confirm(user, "Delete this account?", "Delete account");

    await focusSays("heading: This account cannot be deleted now");
  });
});

test("the status that says an archive was requested is passed by Tab, as no control", async () => {
  const { user } = await drawn(
    (props) => <AccountExportScreen {...props} />,
    () => screen.findByRole("button", { name: "Request an archive" }),
  );
  await user.click(screen.getByRole("button", { name: "Request an archive" }));
  await focusSays("status: The archive was requested.");

  await user.tab();

  expect(focused()).not.toMatch(/^status:/);
  await user.tab({ shift: true });
  expect(focused()).not.toMatch(/^status:/);
});
