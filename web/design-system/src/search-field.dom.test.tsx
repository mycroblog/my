// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SearchField, type SearchFieldProps } from "./search-field";

/** A search field named "Search" whose clear is named "Clear", and the words it reported, in order. */
function field(props: Partial<SearchFieldProps> = {}) {
  const words: string[] = [];
  render(
    <SearchField id="search" label="Search" clearLabel="Clear" onChange={(value) => words.push(value)} {...props} />,
  );
  return words;
}

test("a search field is a searchbox named by its label and described by its hint", () => {
  field({ hint: "People by name or handle, hashtags, and posts by their words" });

  const box = screen.getByRole<HTMLInputElement>("searchbox", {
    name: "Search",
    description: "People by name or handle, hashtags, and posts by their words",
    busy: false,
  });
  expect(box.value).toBe("");
  expect(box.getAttribute("aria-invalid")).not.toBe("true");
});

test("a search field reports its words as the reader writes them", async () => {
  const user = userEvent.setup();
  const words = field();

  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("searchbox", { name: "Search" }));
  await user.keyboard("sam");

  expect(words).toEqual(["s", "sa", "sam"]);
});

test("an empty search field has no clear", () => {
  field();

  expect(screen.queryByRole("button", { name: "Clear" })).toBeNull();
});

test("clear empties a search field, reports it, and gives the focus back to the field", async () => {
  const user = userEvent.setup();
  const words = field({ defaultValue: "sam" });
  const box = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search" });
  expect(box.value).toBe("sam");

  await user.click(screen.getByRole("button", { name: "Clear" }));

  expect(box.value).toBe("");
  expect(words).toEqual([""]);
  expect(document.activeElement).toBe(box);
  expect(screen.queryByRole("button", { name: "Clear" })).toBeNull();
});

test("Escape in a search field empties it and reports it", async () => {
  const user = userEvent.setup();
  const words = field({ defaultValue: "sam" });
  const box = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search" });

  await user.click(box);
  await user.keyboard("{Escape}");

  expect(box.value).toBe("");
  expect(words).toEqual([""]);
  expect(document.activeElement).toBe(box);
});

test("a search field with an error is invalid and described by the error", () => {
  field({ label: "Add an account you follow", defaultValue: "sam", error: "A list can have at most 1,000 members." });

  const box = screen.getByRole("searchbox", {
    name: "Add an account you follow",
    description: "A list can have at most 1,000 members.",
  });
  expect(box.getAttribute("aria-invalid")).toBe("true");
});

test("a busy search field says it is busy, keeps its words open to the reader, and has no clear", async () => {
  const user = userEvent.setup();
  const words = field({ defaultValue: "@kiri@far.example", busy: true });
  const box = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search", busy: true });
  expect(screen.queryByRole("button", { name: "Clear" })).toBeNull();

  await user.type(box, "x");

  expect(box.value).toBe("@kiri@far.examplex");
  expect(words).toEqual(["@kiri@far.examplex"]);
});

test("a search field that asks for the focus has it as it mounts", () => {
  field({ autoFocus: true });

  expect(document.activeElement).toBe(screen.getByRole("searchbox", { name: "Search" }));
});
