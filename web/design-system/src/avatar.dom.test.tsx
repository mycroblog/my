// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { fireEvent, render, screen } from "@testing-library/react";
import { Avatar } from "./avatar";

test("an avatar is one image named by the display name, and shows the picture it is given", () => {
  render(<Avatar name="Alice Chen" src="/alice.png" size="post" />);

  // The picture inside says nothing of its own: the name is read once.
  expect(screen.getAllByRole("img")).toHaveLength(1);
  expect(screen.getByRole("img", { name: "Alice Chen" })).not.toBeNull();
  expect(screen.getByRole("presentation").getAttribute("src")).toBe("/alice.png");
});

test("an avatar beside its name is hidden from assistive technology, and still shows its picture", () => {
  render(<Avatar name="Alice Chen" src="/alice.png" size="post" decorative />);

  expect(screen.queryByRole("img")).toBeNull();
  expect(screen.queryByRole("presentation")).toBeNull();
  expect(screen.getByRole("presentation", { hidden: true }).getAttribute("src")).toBe("/alice.png");
});

test("without an image, the first letter of the name stands in, one grapheme in any script", () => {
  const initial = (name: string) => {
    const { unmount } = render(<Avatar name={name} size="inline" />);
    const shown = screen.getByRole("img", { name }).textContent;
    expect(screen.queryByRole("presentation")).toBeNull();
    unmount();
    return shown;
  };

  expect(initial("ravi")).toBe("R");
  expect(initial("たくや")).toBe("た");
  expect(initial("佐藤 美香")).toBe("佐");
  expect(initial("@ Kenji")).toBe("K");
});

test("when its image fails to load, the initial stands in under the same name", () => {
  render(<Avatar name="Alice Chen" src="/gone.png" size="row" />);

  fireEvent.error(screen.getByRole("presentation"));

  expect(screen.getByRole("img", { name: "Alice Chen" }).textContent).toBe("A");
  expect(screen.queryByRole("presentation")).toBeNull();
});
