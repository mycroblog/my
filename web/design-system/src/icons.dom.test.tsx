// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { isInaccessible, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Button } from "./button";
import { Icon, icons, type IconName } from "./icons";

const names = Object.keys(icons) as IconName[];

test("every icon, off and on, is hidden from assistive technology", () => {
  for (const name of names)
    for (const on of [false, true]) {
      const { container, unmount } = render(<Icon name={name} place="action" on={on} />);
      const hidden = container.childElementCount === 1 && isInaccessible(container.firstElementChild as Element);
      expect([name, on, hidden, container.textContent]).toEqual([name, on, true, ""]);
      unmount();
    }
});

test("an icon in any place is hidden from assistive technology", () => {
  render(
    <>
      <Icon name="favourite" place="action" />
      <Icon name="favourite" place="caption" on />
      <Icon name="home" place="navigation" on />
    </>,
  );

  expect(screen.queryAllByRole("img")).toEqual([]);
  expect(screen.queryAllByRole("img", { hidden: true })).toEqual([]);
});

test("an icon is no stop of Tab and adds nothing to the name of the button that holds it", async () => {
  const user = userEvent.setup();
  render(
    <>
      <Icon name="favourite" place="action" />
      <Icon name="boost" place="action" on />
      <Button>
        <Icon name="compose" place="caption" /> New post
      </Button>
    </>,
  );

  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "New post" }));
});
