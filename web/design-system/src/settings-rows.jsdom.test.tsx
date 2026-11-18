// SPDX-License-Identifier: AGPL-3.0-only

// On jsdom: happy-dom gives the focus to any element that is asked to take it, so that the result of a settings row
// (#3202) can take the focus its screen gives it once the row's control is gone, and that Tab passes it, are decided
// here (ADR-0171 §4). What the result says is settings-rows.dom.test.tsx.

import { expect, test } from "bun:test";
import { Button } from "@my/design-system";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SettingsAccountItem, SettingsList, SettingsListRow } from "./settings";

for (const [kind, row] of [
  [
    "a row",
    (result: { id: string; shown: string; spoken: string }) => <SettingsListRow title="spam.example" result={result} />,
  ],
  [
    "an account's row",
    (result: { id: string; shown: string; spoken: string }) => (
      <SettingsAccountItem account={<span>Kai</span>} label="@kai" result={result} />
    ),
  ],
] as const)
  test(`the result of ${kind} takes the focus a script gives it by its id, and is no stop of Tab`, async () => {
    const user = userEvent.setup();
    render(
      <>
        <Button>Before</Button>
        <SettingsList label="Muted">
          {row({ id: "done", shown: "Unmuted", spoken: "You unmuted it." })}
          <SettingsListRow title="ads.example" end={<Button>Unmute</Button>} />
        </SettingsList>
      </>,
    );
    const result = document.getElementById("done");
    if (result === null) throw new Error("test: no element bears the result's id");

    act(() => {
      result.focus();
    });

    expect(document.activeElement === result).toBe(true);
    // From the result, Tab goes on to the next row's control.
    await user.tab();
    expect(document.activeElement === screen.getByRole("button", { name: "Unmute" })).toBe(true);
    // And from before the list, Tab passes the result.
    act(() => {
      screen.getByRole("button", { name: "Before" }).focus();
    });
    await user.tab();
    expect(document.activeElement === screen.getByRole("button", { name: "Unmute" })).toBe(true);
  });
