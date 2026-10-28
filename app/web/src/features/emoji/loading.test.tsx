// SPDX-License-Identifier: AGPL-3.0-only
// The emoji picker while the Unicode emoji data is still on its way (G15 of web-client/16-look-states-and-wording.md):
// the dialog answers the press at once, and the emoji fill its grid once the delayed import resolves.
import { screen, waitFor, within } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

const gate = vi.hoisted(() => {
  let resolve: () => void = () => undefined;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return {
    promise,
    resolve: () => {
      resolve();
    },
  };
});

// The base data waits for the test to release it, as a slow chunk would.
vi.mock("./unicode", async (importOriginal) => {
  const original = await importOriginal<typeof import("./unicode")>();
  return {
    ...original,
    loadUnicodeEmoji: async () => {
      await gate.promise;
      return original.loadUnicodeEmoji();
    },
  };
});

beforeEach(() => {
  sessionStorage.clear();
  localStorage.clear();
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

test("opens the picker at once and fills its grid when the delayed emoji data arrives", async () => {
  server.use(signedInHandler);
  const { user } = await renderApp();
  await user.click((await screen.findAllByRole("button", { name: "Write a post" }))[0] as HTMLElement);
  const sheet = await screen.findByRole("dialog", { name: "New post" });

  await user.click(within(sheet).getByRole("button", { name: "Add emoji" }));
  // The data is held back, yet the dialog is there right after the press, with its search field focused, its
  // section buttons and a busy grid.
  const picker = screen.getByRole("dialog", { name: "Emoji" });
  const search = within(picker).getByRole("searchbox", { name: "Search emoji" });
  await waitFor(() => {
    expect(search).toHaveFocus();
  });
  const toolbar = within(picker).getByRole("toolbar", { name: "Emoji categories" });
  const groups = within(toolbar).getAllByRole("button");
  expect(groups.map((button) => button.getAttribute("aria-label"))).toContain("Smileys & emotion");
  for (const button of groups) expect(button).toBeDisabled();
  const grid = within(picker).getByRole("grid", { name: "Emoji" });
  expect(grid).toHaveAttribute("aria-busy", "true");
  expect(within(picker).getByRole("status")).toHaveTextContent("Loading emoji");
  expect(within(picker).queryAllByRole("gridcell")).toHaveLength(0);

  gate.resolve();
  expect(await within(picker).findByRole("gridcell", { name: "thumbs up" })).toBeInTheDocument();
  expect(grid).not.toHaveAttribute("aria-busy");
  expect(within(toolbar).getByRole("button", { name: "Smileys & emotion" })).toBeEnabled();
}, 30_000);
