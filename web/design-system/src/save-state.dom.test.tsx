// SPDX-License-Identifier: AGPL-3.0-only

// The save state of a control that saves at once (#3198, ADR-0110 §5): its words, what of it a live region reads, and
// its Try again, which keeps the focus on the control it is about. The region that holds it is its holder's: a
// setting's row (settings.dom.test.tsx), a switch (switch.dom.test.tsx), the submit bar.

import { expect, test } from "bun:test";
import { Select, SettingRow } from "@my/design-system";
import { act, cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SaveState, type SaveStateProps } from "./save-state";

/** Draws one state alone, after taking the one before it away. */
function state(props: SaveStateProps) {
  cleanup();
  return render(<SaveState {...props} />).container;
}

test("each save state says the design system's words, and a failure with a way to try again offers it", () => {
  expect(state({ state: "saving" }).textContent).toBe("Saving…");
  expect(state({ state: "saved" }).textContent).toBe("Saved");
  expect(state({ state: "failed" }).textContent).toBe("This could not be saved.");
  expect(screen.queryAllByRole("button")).toHaveLength(0);

  expect(state({ state: "failed", onRetry: () => undefined }).textContent).toMatch(
    /^This could not be saved\.\s*Try again$/,
  );
  expect(screen.getByRole("button", { name: "Try again" })).not.toBeNull();
});

test("a save state says the words it is given in place of its own", () => {
  const drawn = state({
    state: "failed",
    onRetry: () => undefined,
    children: "The server did not take this time zone.",
  });

  expect(drawn.textContent).toMatch(/^The server did not take this time zone\.\s*Try again$/);
});

test("a save state is no live region of its own, and keeps Saving… from the region that holds it", () => {
  /** What the state says of reading its words as they come: `aria-live` on them or around them, inside the state. */
  const live = (props: SaveStateProps, words: string) => {
    const drawn = within(state(props));
    expect((["status", "alert", "log"] as const).flatMap((role) => drawn.queryAllByRole(role))).toHaveLength(0);
    return drawn.getByText(words).closest("[aria-live]")?.getAttribute("aria-live") ?? null;
  };

  expect(live({ state: "saving" }, "Saving…")).toBe("off");
  expect(live({ state: "saved" }, "Saved")).toBeNull();
  expect(live({ state: "failed", onRetry: () => undefined }, "This could not be saved.")).toBeNull();
});

for (const [how, press] of [
  [
    "a press",
    (user: ReturnType<typeof userEvent.setup>) => user.click(screen.getByRole("button", { name: "Try again" })),
  ],
  [
    "Enter",
    async (user: ReturnType<typeof userEvent.setup>) => {
      act(() => {
        screen.getByRole("button", { name: "Try again" }).focus();
      });
      await user.keyboard("{Enter}");
    },
  ],
] as const)
  test(`Try again of a row's failed save, by ${how}, puts the focus on the row's control, then asks once`, async () => {
    const user = userEvent.setup();
    // Whether the focus was on the control when the owner was asked: the owner then takes the failed state away,
    // and the button with it.
    const asked: boolean[] = [];
    render(
      <SettingRow
        id="zone"
        label="Time zone"
        save={
          <SaveState
            state="failed"
            onRetry={() => asked.push(document.activeElement === screen.getByRole("combobox", { name: "Time zone" }))}
          />
        }
      >
        <Select options={[{ value: "UTC", label: "UTC" }]} />
      </SettingRow>,
    );

    await press(user);

    expect(asked).toEqual([true]);
    expect(document.activeElement === screen.getByRole("combobox", { name: "Time zone" })).toBe(true);
  });

test("Try again of a save state that no control holds asks once", async () => {
  const user = userEvent.setup();
  let asked = 0;
  render(<SaveState state="failed" onRetry={() => (asked += 1)} />);

  await user.click(screen.getByRole("button", { name: "Try again" }));

  expect(asked).toBe(1);
});
