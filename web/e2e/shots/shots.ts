// SPDX-License-Identifier: AGPL-3.0-only

// What each screenshot of list.ts shows: who looks, what is made for it first (stage.ts), the address it opens, and
// what shows once the screen has loaded. A record by the names of list.ts, so that a name without its shot does not
// type-check.

import { expect, type Browser, type Page } from "@playwright/test";
import type { ShotName } from "./list";
import type { Stage } from "./stage";

export type Shot = {
  /** The person of the scenario who looks at the screen. */
  as: string;
  /** Makes what the screen shows beyond the scenario, and returns the address to open. */
  open: (stage: Stage, browser: Browser) => Promise<string>;
  /** Waits until the screen shows what the shot is of. */
  ready: (page: Page) => Promise<void>;
};

const heading = (page: Page, name: string) =>
  expect(page.getByRole("main").getByRole("heading", { level: 1, name, exact: true })).toBeVisible();

export const shots: Record<ShotName, Shot> = {
  home: {
    as: "yuki",
    open: () => Promise.resolve("/"),
    ready: async (page) => {
      await heading(page, "Home");
      const loaf = "A round loaf of sourdough on a wooden board, seen from above";
      // Sam's post and Hana's boost of it.
      await expect(page.getByRole("feed").getByRole("img", { name: loaf, exact: true })).not.toHaveCount(0);
    },
  },
  post: {
    as: "hana",
    open: async (stage) => `/@yuki/status/${(await stage.post("yuki", "今朝の尾根")).id}`,
    ready: async (page) => {
      await expect(page.getByRole("main").getByText(/^@yuki That light! Which trail is this\?$/)).toBeVisible();
    },
  },
  reports: {
    as: "marco",
    open: async (stage) => {
      await stage.reports();
      return "/moderation/reports";
    },
    ready: async (page) => {
      await heading(page, "Reports");
      await expect(page.getByRole("main").getByRole("table")).toBeVisible();
    },
  },
  supporters: {
    as: "ren",
    open: async (stage, browser) => {
      await stage.supporters(browser);
      return "/about/supporters";
    },
    ready: async (page) => {
      await heading(page, "Supporters");
    },
  },
  "voice-room": {
    as: "kai",
    open: async (stage) => `/voice/${await stage.voiceRoom()}`,
    ready: async (page) => {
      await expect(page.getByRole("main").getByRole("button", { name: "Join", exact: true })).toBeVisible();
    },
  },
};
