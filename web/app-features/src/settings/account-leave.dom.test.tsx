// SPDX-License-Identifier: AGPL-3.0-only

// What Deactivate account and Delete account tell their route once the server took the act (#3203; ADR-0089 §11):
// the server ended every session, this one too, which the screen says to the route once (`onSessionEnded`), while its
// confirmation stays open and busy until the app has left. What each asks and sends is the app's route test (web/app,
// settings-account-leave.dom.test.tsx).

import { screen, waitFor, within } from "@testing-library/react";
import { expect, test } from "bun:test";
import type { ReactNode } from "react";
import { atRest } from "../test/security";
import { settingsOn, type addressProps } from "../test/settings";
import { AccountDeactivateScreen, AccountDeleteScreen } from "./account-end";

type Props = ReturnType<typeof addressProps>;

for (const [name, draw, act, title, route] of [
  [
    "Deactivate account",
    (props: Props): ReactNode => <AccountDeactivateScreen {...props} />,
    "Deactivate account",
    "Deactivate this account?",
    "POST /accounts/deactivation",
  ],
  [
    "Delete account",
    (props: Props): ReactNode => <AccountDeleteScreen {...props} />,
    "Delete account",
    "Delete this account?",
    "POST /accounts/delete",
  ],
] as const)
  test(`${name} confirmed tells the route once that the session ended, and its confirmation stays busy`, async () => {
    const on = await settingsOn({}, draw);
    await screen.findByRole("button", { name: act });
    await atRest(on);
    const sent = on.fake.script(route);

    await on.user.click(screen.getByRole("button", { name: act }));
    const dialog = await screen.findByRole("dialog", { name: title });
    await on.user.type(within(dialog).getByLabelText("Current password"), "demo-password-yuki");
    await on.user.click(within(dialog).getByRole("button", { name: act }));

    await waitFor(() => {
      if (on.address.sessionsEnded === 0) throw new Error("test: the route was not told");
    });
    // A turn of the event loop for a second call that would come late.
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(on.address.sessionsEnded).toBe(1);
    expect(sent.seen).toHaveLength(1);
    expect(screen.queryAllByRole("dialog", { name: title })).toHaveLength(1);
    expect(within(dialog).getByRole("button", { name: act }).getAttribute("aria-busy")).toBe("true");
  });
