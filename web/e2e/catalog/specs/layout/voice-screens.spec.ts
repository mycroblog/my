// SPDX-License-Identifier: AGPL-3.0-only

// The stories of the voice chat rooms' screens (web/app/src/voice.stories.tsx) each show the state they are named by,
// on the fake API: the list's own states, a room's loading and failure, the reader's place in a room (a hand raised,
// speaking, the sound held back, the microphone refused), why the call ended told on another screen, the visitor's
// page and the voice chat off. What the screens do from each state is the app's route tests'
// (web/app/src/voice.dom.test.tsx); this is that the catalog shows each state for a review and a shot.

import type { Page } from "@playwright/test";
import { expect, pc, test } from "../test";

/** The story of the screens of the voice chat rooms named `name`. */
const story = (name: string) => `screens-voice-chat-rooms--${name}`;

/** The rooms the fake lists, as the list shows them. */
const roomsOpen = (page: Page) =>
  page.getByRole("main").getByRole("list", { name: "Rooms open now", exact: true }).getByRole("link");

/** The room's controls of a reader in it. */
const controls = (page: Page) => page.getByRole("main").getByRole("toolbar", { name: "Room controls", exact: true });

// A line of a room is said twice in its main: shown where it belongs, and in the one live region it is spoken from.
const states: readonly (readonly [string, (page: Page) => Promise<void>])[] = [
  [
    "rooms",
    async (page) => {
      await expect(roomsOpen(page)).toHaveCount(4);
    },
  ],
  [
    "rooms-empty",
    async (page) => {
      await expect(
        page.getByRole("main").getByRole("heading", { level: 1, name: "Voice chat rooms", exact: true }),
      ).toBeVisible();
      await expect(roomsOpen(page)).toHaveCount(0);
    },
  ],
  [
    "rooms-loading",
    async (page) => {
      await expect(page.getByRole("main").getByRole("status")).toHaveCount(1);
      await expect(roomsOpen(page)).toHaveCount(0);
    },
  ],
  [
    "rooms-error",
    async (page) => {
      await expect(page.getByRole("main").getByRole("alert")).toHaveText(
        /^The rooms could not be loaded\.\s*Try again$/,
      );
      await expect(roomsOpen(page)).toHaveCount(0);
    },
  ],
  [
    "room-loading",
    async (page) => {
      await expect(page.getByRole("main").getByText(/^Loading the room…$/)).toBeVisible();
    },
  ],
  [
    "room-error",
    async (page) => {
      await expect(page.getByRole("main").getByRole("alert")).toHaveText(
        /^The room could not be loaded\.\s*Try again$/,
      );
    },
  ],
  [
    "hand-raised",
    async (page) => {
      await expect(controls(page).getByRole("button", { name: "Lower hand", exact: true })).toBeVisible();
    },
  ],
  [
    "speaking",
    async (page) => {
      await expect(controls(page).getByRole("button", { name: "Mute", exact: true })).toBeVisible();
      await expect(page.getByRole("main").getByRole("button", { name: "Stop speaking", exact: true })).toBeVisible();
    },
  ],
  [
    "sound-held",
    async (page) => {
      await expect(page.getByRole("main").getByText(/^The browser is holding the room's sound back\.$/)).toHaveCount(2);
      await expect(page.getByRole("main").getByRole("button", { name: "Hear the room", exact: true })).toBeVisible();
    },
  ],
  [
    "microphone-refused",
    async (page) => {
      await expect(
        page
          .getByRole("main")
          .getByText(
            /^The browser was not allowed to use your microphone\. Allow it in the site's settings, then try again\.$/,
          ),
      ).toHaveCount(2);
      await expect(controls(page).getByRole("button", { name: "Unmute", exact: true })).toBeVisible();
    },
  ],
  [
    "removed-elsewhere",
    async (page) => {
      await expect(
        page.getByRole("main").getByRole("heading", { level: 1, name: "Notifications", exact: true }),
      ).toBeVisible();
      await expect(
        page.getByRole("status").filter({ hasText: /^A host removed you from Morning coffee chat\./ }),
      ).toHaveCount(1);
      await expect(page.getByRole("button", { name: "Open the room", exact: true })).toBeVisible();
    },
  ],
  [
    "visitor",
    async (page) => {
      await expect(page.getByRole("main").getByRole("button", { name: "Sign in to join", exact: true })).toBeVisible();
      await expect(page.getByRole("main").getByRole("list")).toHaveCount(0);
    },
  ],
  [
    "voice-off",
    async (page) => {
      await expect(
        page.getByRole("main").getByText(/^Voice chat is (off|not available) on this server\.$/),
      ).toBeVisible();
    },
  ],
];

// One page walks the stories, a step each: a page of its own per story would cost the catalog's run its time limit.
test("each story of the voice chat rooms shows its state", async ({ page, open }) => {
  for (const [name, shows] of states)
    await test.step(name, async () => {
      await open(story(name), { width: pc });
      await shows(page);
    });
});
