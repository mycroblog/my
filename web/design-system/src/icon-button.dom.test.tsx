// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "react-aria-components";
import { IconButton } from "./icon-button";

test("an icon button is a button named by its label alone, and a click, Enter and Space each press it once", async () => {
  const user = userEvent.setup();
  let presses = 0;
  render(<IconButton icon="more" label="More" onPress={() => presses++} />);
  const button = screen.getByRole("button", { name: "More" });
  // An action that is no toggle says nothing of being on or off.
  expect(button.hasAttribute("aria-pressed")).toBe(false);

  await user.tab();
  expect(document.activeElement).toBe(button);
  await user.keyboard("{Enter}");
  expect(presses).toBe(1);
  await user.keyboard(" ");
  expect(presses).toBe(2);
  await user.click(button);
  expect(presses).toBe(3);
});

test("the name carries the count as shown and the description the exact one", () => {
  render(<IconButton icon="favourite" label="Favourite" count={12_840} locale="en" />);

  const button = screen.getByRole("button", { name: "Favourite, 12.8K" });
  expect(button.getAttribute("aria-description")).toBe("12,840");
  expect(button.textContent).toBe("12.8K");
});

test("a count shown whole has no description", () => {
  render(<IconButton icon="reply" label="Reply" count={999} locale="en" />);

  const button = screen.getByRole("button", { name: "Reply, 999" });
  expect(button.hasAttribute("aria-description")).toBe(false);
  expect(button.textContent).toBe("999");
});

test("zero shows no count and adds none to the name", () => {
  render(<IconButton icon="reply" label="Reply" count={0} locale="en" />);

  expect(screen.getByRole("button", { name: "Reply" }).textContent).toBe("");
});

test("a toggle says whether it is on", () => {
  render(
    <>
      <IconButton icon="boost" label="Boost" pressed />
      <IconButton icon="favourite" label="Favourite" pressed={false} />
    </>,
  );

  expect(screen.getByRole("button", { name: "Boost", pressed: true })).not.toBeNull();
  expect(screen.getByRole("button", { name: "Favourite", pressed: false })).not.toBeNull();
});

test("the count is in the locale of the I18nProvider, or in the locale it is given", () => {
  render(
    <>
      <I18nProvider locale="ja">
        <IconButton icon="favourite" label="お気に入り" count={12_840} />
      </I18nProvider>
      <I18nProvider locale="ar-XB">
        <IconButton icon="favourite" label="Favourite" count={12_840} locale="en" />
      </I18nProvider>
    </>,
  );

  const provided = screen.getByRole("button", { name: "お気に入り, 1.3万" });
  expect(screen.getByText("1.3万").lang).toBe("ja");
  expect(provided.dir).toBe("ltr");
  // A locale of its own names the count's language; the direction stays the reader's.
  const given = screen.getByRole("button", { name: "Favourite, 12.8K" });
  expect(screen.getByText("12.8K").lang).toBe("en");
  expect(given.dir).toBe("rtl");
});

test("a busy icon button says it is busy under its name and ignores presses", async () => {
  const user = userEvent.setup();
  let presses = 0;
  render(<IconButton icon="bookmark" label="Bookmark" pressed={false} busy onPress={() => presses++} />);
  const button = screen.getByRole("button", { name: "Bookmark", busy: true, pressed: false });

  await user.tab();
  expect(document.activeElement).toBe(button);
  await user.keyboard("{Enter}");
  await user.click(button);
  expect(presses).toBe(0);
});

test("a disabled icon button is disabled and a click does not press it", async () => {
  const user = userEvent.setup();
  let presses = 0;
  render(<IconButton icon="boost" label="Boost" count={3} locale="en" isDisabled onPress={() => presses++} />);
  const button = screen.getByRole<HTMLButtonElement>("button", { name: "Boost, 3" });
  expect(button.disabled).toBe(true);

  await user.click(button);
  expect(presses).toBe(0);
});
