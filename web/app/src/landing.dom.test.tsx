// SPDX-License-Identifier: AGPL-3.0-only

// A visitor without a session lands on the server's landing page: the server's name, the way to an account, and what
// they may read without one, which the instance decides.

import { expect, test } from "bun:test";
import { screen, within } from "@testing-library/react";
import { fakeHost } from "@my/e2e/fake";
import { renderApp } from "./test/app";

/** A server that shows its local timeline to everyone, and its federated one to the people signed in alone. */
const server = { visitor: true, timelines: { local: "everyone", federated: "signed_in" } } as const;

/** The places the landing page leads a visitor to without an account. */
const places = () => within(screen.getByRole("navigation", { name: "Without an account" }));

test("a visitor at the server's address sees its landing page, with the way to an account", async () => {
  const app = await renderApp("/", server);

  const name = within(screen.getByRole("banner")).getByRole("heading", { level: 1 });
  expect(name.textContent).not.toBe("");
  expect(name.textContent).not.toBe(fakeHost);
  expect(app.title()).toBe(name.textContent);
  const main = within(screen.getByRole("main"));
  expect(main.getByRole("link", { name: "Create an account" })).toBeDefined();
  expect(places().getByRole("link", { name: "Read the local timeline" })).toBeDefined();
  expect(places().queryByRole("link", { name: "Read the federated timeline" })).toBeNull();

  await app.user.click(main.getByRole("link", { name: "Sign in" }));
  expect(await within(screen.getByRole("main")).findByRole("heading", { level: 1, name: "Sign in" })).toBeDefined();
  expect(app.path()).toBe("/login");
});

test("a visitor goes on from the landing page to the local timeline", async () => {
  const app = await renderApp("/", server);

  await app.user.click(places().getByRole("link", { name: "Read the local timeline" }));

  expect(await within(screen.getByRole("main")).findByRole("heading", { level: 1, name: "Local" })).toBeDefined();
  expect(app.path()).toBe("/local");
});
