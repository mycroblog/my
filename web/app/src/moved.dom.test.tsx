// SPDX-License-Identifier: AGPL-3.0-only

// The profile of an account that moved (ADR-0089 §3, §4): its main action opens the new account, at its address. What
// the profile says of the move and offers is the feature's (web/app-features/src/profile/profile.dom.test.tsx). On the
// fake's server Sora's account on social.example moved to photos.example.

import { expect, test } from "bun:test";
import { main, renderApp } from "./test/app";
import { screen, within } from "@testing-library/react";

const sora = "Sora そら";
const old = "sora@social.example";
const now = "sora@photos.example";

/** The screen's heading of the profile of Sora's account `handle`: the display name and the full handle. */
const heading = (handle: string) => main().findByRole("heading", { level: 1, name: `${sora} (@${handle})` });

test("a person opens a moved account and follows its link to the new one, at its address", async () => {
  const app = await renderApp(`/@${old}`);
  expect(await heading(old)).toBeDefined();

  await app.user.click(
    within(screen.getByRole("region", { name: sora })).getByRole("link", { name: "Go to new account" }),
  );

  expect(await heading(now)).toBeDefined();
  expect(app.path()).toBe(`/@${now}`);
});
