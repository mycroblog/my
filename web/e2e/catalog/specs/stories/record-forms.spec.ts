// SPDX-License-Identifier: AGPL-3.0-only

// The stories of the record and form parts of the moderation, administration and operations screens (#3258) each
// show the state they are named by: the confirmation with a text to type, an alertdialog whose confirmation is
// disabled until the field holds the text (NFR-MOD-7); the facts of a record, side by side on a PC and stacked on a
// phone, a private value behind Show; the figures of a server's state; a log, its Details folded and open; a block of
// code that scrolls in place; and a color token's field, invalid where a pair is under its minimum. What each part
// says of its roles and names is its component tests' (web/design-system/src/{dialog,facts,log,color-field}.dom.test.tsx);
// the stories' accessibility checks are the check of every story.

import type { Page } from "@playwright/test";
import { boxOf } from "../boxes";
import { expect, minimum, pc, test } from "../test";

const story = (title: string, name: string) => `design-system-${title}--${name}`;

const banOf = (page: Page) =>
  page.getByRole("alertdialog", { name: "Ban @spam_bot_7731@social.example?", exact: true });
const banField = (page: Page) =>
  banOf(page).getByRole("textbox", { name: "To confirm, type spam_bot_7731@social.example", exact: true });
const banButton = (page: Page) => banOf(page).getByRole("button", { name: "Ban account", exact: true });

const states: readonly (readonly [string, (page: Page) => Promise<void>])[] = [
  [
    story("dialogs", "typed-confirm"),
    async (page) => {
      await expect(banField(page)).toHaveValue("");
      await expect(banButton(page)).toBeDisabled();
      await expect(banOf(page)).toContainText("This cannot be undone.");
    },
  ],
  [
    story("dialogs", "typed-confirm-not-yet"),
    async (page) => {
      await expect(banField(page)).toHaveValue("spam_bot_7731");
      await expect(banButton(page)).toBeDisabled();
    },
  ],
  [
    story("dialogs", "typed-confirm-typed"),
    async (page) => {
      await expect(banButton(page)).toBeEnabled();
    },
  ],
  [
    story("dialogs", "typed-confirm-others"),
    async (page) => {
      await expect(
        page.getByRole("textbox", { name: "To confirm, type mycroblog.example", exact: true }),
      ).toBeVisible();
      await expect(page.getByRole("textbox", { name: "To confirm, type Trusted reviewer", exact: true })).toBeVisible();
      await expect(page.getByRole("button", { name: /^(Close server|Delete role)$/ })).toHaveCount(2);
      await expect(page.getByRole("button", { name: /^(Close server|Delete role)$/, disabled: false })).toHaveCount(0);
    },
  ],
  [
    story("dialogs", "typed-confirm-over-the-page"),
    async (page) => {
      await expect(banField(page)).toBeFocused();
      await banField(page).fill("spam_bot_7731@social.example");
      await expect(banButton(page)).toBeEnabled();
    },
  ],
  [
    story("facts", "registration"),
    async (page) => {
      const facts = page.getByLabel("Registration", { exact: true });
      await expect(facts.getByRole("term")).toHaveText([
        "Account",
        "Registered",
        "Email",
        "Registered from",
        "Invited by",
        "Reason given",
      ]);
      await expect(facts.getByRole("button", { name: "Show", exact: true })).toHaveCount(2);
    },
  ],
  [
    story("facts", "reveal-on-press"),
    async (page) => {
      await page.getByRole("button", { name: "Show", exact: true, description: "Email" }).click();
      await expect(page.getByText("river.tanaka@mail.example", { exact: true })).toBeFocused();
    },
  ],
  [
    story("facts", "figures"),
    async (page) => {
      const figures = page.getByRole("list", { name: "This server", exact: true });
      await expect(figures.getByRole("link")).toHaveCount(5);
      await expect(figures.getByRole("link", { name: "12 Open reports", exact: true })).toBeVisible();
    },
  ],
  [
    story("facts", "figures-unlinked"),
    async (page) => {
      await expect(page.getByRole("list", { name: "This server", exact: true }).getByRole("listitem")).toHaveCount(5);
      await expect(page.getByRole("link")).toHaveCount(0);
    },
  ],
  [
    story("logs", "audit-log"),
    async (page) => {
      const log = page.getByRole("list", { name: "Audit log", exact: true });
      await expect(log.getByRole("listitem")).toHaveCount(5);
      await expect(log.getByRole("button", { name: "Details", exact: true, expanded: false })).toHaveCount(2);
    },
  ],
  [
    story("logs", "webhook-deliveries-open"),
    async (page) => {
      await expect(page.getByRole("button", { name: "Details", exact: true, expanded: true })).toHaveCount(3);
      const failed = page.getByRole("listitem").filter({ hasText: "report.created to hooks.example.com" });
      await expect(failed.getByRole("group", { name: "Payload", exact: true })).toContainText(
        '"event": "report.created"',
      );
      await expect(failed.getByRole("group", { name: "Response", exact: true })).toContainText(
        "500 Internal Server Error",
      );
    },
  ],
  [
    story("color-field", "below"),
    async (page) => {
      const token = page.getByRole("group", { name: "Accent text", exact: true });
      await expect(token.getByRole("textbox", { name: "Light", exact: true })).toHaveAttribute("aria-invalid", "true");
      await expect(token.getByRole("textbox", { name: "Dark", exact: true })).not.toHaveAttribute("aria-invalid");
      await expect(token.getByText(/^2\.77:1, needs 4\.5:1$/)).toBeVisible();
    },
  ],
  [
    story("color-field", "misformed"),
    async (page) => {
      await expect(
        page.getByText("Enter a color as # and 6 hexadecimal digits, such as #ff6b6b.", { exact: true }),
      ).toBeVisible();
    },
  ],
];

// One page walks the stories, a step each: a page of its own per story would cost the catalog's run its time limit.
test("each story of the record and form parts shows its state", async ({ page, open }) => {
  for (const [id, shows] of states)
    await test.step(id, async () => {
      await open(id, { width: pc });
      await shows(page);
    });
});

test("the facts of a record stand side by side on a PC and each term over its value on a phone", async ({
  page,
  open,
}) => {
  const term = () => page.getByText("Registered", { exact: true });
  const value = () => page.getByText("Nov 3, 2026, 9:41 PM", { exact: true });
  await open(story("facts", "registration"), { width: pc });
  const [wideTerm, wideValue] = [await boxOf(term()), await boxOf(value())];
  expect(wideValue.left).toBeGreaterThan(wideTerm.right);
  expect(wideValue.top).toBeLessThan(wideTerm.bottom);

  await open(story("facts", "registration"), { width: minimum });
  const [narrowTerm, narrowValue] = [await boxOf(term()), await boxOf(value())];
  expect(narrowValue.top).toBeGreaterThanOrEqual(narrowTerm.bottom);
  expect(Math.abs(narrowValue.left - narrowTerm.left)).toBeLessThan(1);
});

test("a block of code longer than its room scrolls in place, and never sideways", async ({ page, open }) => {
  await open(story("logs", "code-blocks"), { width: minimum });
  const block = page.getByRole("group", { name: "Payload", exact: true });
  const sizes = await block.evaluate((el) => ({
    scrollHeight: el.scrollHeight,
    clientHeight: el.clientHeight,
    scrollWidth: el.scrollWidth,
    clientWidth: el.clientWidth,
  }));
  expect(sizes.scrollHeight).toBeGreaterThan(sizes.clientHeight);
  expect(sizes.scrollWidth).toBeLessThanOrEqual(sizes.clientWidth);
});
