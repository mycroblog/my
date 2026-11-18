// SPDX-License-Identifier: AGPL-3.0-only

// How each screen under the entry Account and data says that it loads, and that what it reads could not be read, with
// the way to try again (#3203): Export, Import, Delete old posts automatically, Aliases, Move account, Deactivate
// account and Delete account, each drawn alone on the fake server with its read held or failed. A screen of the app
// is drawn once its route's loader answered, so a held read is seen here and not in a route test; what each screen
// does once read is the app's route tests (web/app, settings-account-data.dom.test.tsx and
// settings-account-leave.dom.test.tsx).

import type { Route } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { describe, expect, test } from "bun:test";
import type { ReactNode } from "react";
import { words } from "../test/security";
import { addressProps, settingsOn } from "../test/settings";
import { AccountAliasesScreen } from "./account-aliases";
import { AccountAutoDelete } from "./account-auto-delete";
import { AccountDeactivateScreen, AccountDeleteScreen } from "./account-end";
import { AccountExportScreen } from "./account-export";
import { AccountImportScreen } from "./account-import";
import { AccountMoveScreen } from "./account-move";

type Props = ReturnType<typeof addressProps>;

// Each screen, how it is drawn, the read it waits for, what it says meanwhile, what it says when the read failed, and
// what shows once it is read.
const screens: readonly [string, (props: Props) => ReactNode, Route, string, string, () => Promise<unknown>][] = [
  [
    "Export",
    (props) => <AccountExportScreen {...props} />,
    "GET /exports",
    "Loading your archives…",
    "Your archives could not be loaded.",
    () => screen.findByRole("button", { name: "Request an archive" }),
  ],
  [
    "Import",
    (props) => <AccountImportScreen {...props} />,
    "GET /imports",
    "Loading your imports…",
    "Your imports could not be loaded.",
    () => screen.findByRole("list", { name: "Recent imports" }),
  ],
  [
    "Delete old posts automatically",
    (props) => <AccountAutoDelete {...props} onUnsaved={() => undefined} />,
    "GET /preferences/auto_delete",
    "Loading your settings for old posts…",
    "Your settings for old posts could not be loaded.",
    () => screen.findByRole("textbox", { name: "Delete posts older than" }),
  ],
  [
    "Aliases",
    (props) => <AccountAliasesScreen {...props} />,
    "GET /accounts/aliases",
    "Loading your aliases…",
    "Your aliases could not be loaded.",
    () => screen.findByRole("list", { name: "1 of 5 aliases" }),
  ],
  [
    "Move account",
    (props) => <AccountMoveScreen {...props} />,
    "GET /accounts/verify_credentials",
    "Loading your account…",
    "Your account could not be loaded.",
    () => screen.findByRole("textbox", { name: "Account you are moving to" }),
  ],
  [
    "Deactivate account",
    (props) => <AccountDeactivateScreen {...props} />,
    "GET /accounts/deactivation",
    "Loading your account…",
    "Your account could not be loaded.",
    () => screen.findByRole("button", { name: "Deactivate account" }),
  ],
  [
    "Delete account",
    (props) => <AccountDeleteScreen {...props} />,
    "GET /accounts/verify_credentials",
    "Loading your account…",
    "Your account could not be loaded.",
    () => screen.findByRole("button", { name: "Delete account" }),
  ],
];

/** What the screen says, as one line. */
const page = () => words(screen.getByRole("main"));

describe("while its read is on its way, the screen says it loads", () => {
  for (const [name, draw, read, loading] of screens)
    test(name, async () => {
      let held: { arrived: () => Promise<unknown>; release: () => void } | undefined;
      await settingsOn({}, draw, {
        before: ({ fake }) => {
          held = fake.script(read, { hold: "request" });
        },
      });
      await held?.arrived();

      await waitFor(() => {
        if (!page().includes(loading)) throw new Error(`test: the screen says ${page().slice(0, 200)}`);
      });
      held?.release();
    });
});

describe("when its read failed, the screen says so, and Try again reads it again", () => {
  for (const [name, draw, read, , failed, shown] of screens)
    test(name, async () => {
      let failing: { end: () => void } | undefined;
      const { user } = await settingsOn({}, draw, {
        before: ({ fake }) => {
          failing = fake.script(read, { fail: 500 });
        },
      });

      await waitFor(() => {
        if (!page().includes(failed)) throw new Error(`test: the screen says ${page().slice(0, 200)}`);
      });
      failing?.end();
      const [again, ...others] = within(screen.getByRole("main")).getAllByRole("button", { name: "Try again" });
      expect(others).toEqual([]);
      if (again === undefined) throw new Error("test: no Try again");
      await user.click(again);

      await shown();
      expect(page()).not.toContain(failed);
    });
});
