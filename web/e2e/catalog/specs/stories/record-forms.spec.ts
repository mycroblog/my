// SPDX-License-Identifier: AGPL-3.0-only

// The stories of the record and form parts of the moderation, administration and operations screens (#3258) each
// show the state they are named by: the confirmation with a text to type, an alertdialog whose confirmation is
// disabled until the field holds the text, in any case, and again once it does not, and nothing in it answering while
// in flight (NFR-MOD-7); the facts of a record, side by side from an 18rem column and stacked in a narrower one, a
// private value behind Show; the figures of a server's state; a log, its Details folded and open; a block of code
// shown whole; and a color token's field, invalid where a pair is under its minimum. What each part
// says of its roles and names is its component tests' (web/design-system/src/{dialog,facts,log,color-field}.dom.test.tsx);
// the stories' accessibility checks are the check of every story.

import type { Page } from "@playwright/test";
import { boxOf, reachingOut } from "../boxes";
import { expect, minimum, pc, test } from "../test";

const story = (title: string, name: string) => `design-system-${title}--${name}`;

const banOf = (page: Page) =>
  page.getByRole("alertdialog", { name: "Ban @spam_bot_7731@social.example?", exact: true });
const banField = (page: Page) =>
  banOf(page).getByRole("textbox", { name: "To confirm, enter spam_bot_7731@social.example", exact: true });
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
    story("dialogs", "typed-confirm-busy"),
    async (page) => {
      // The ban in flight: still the alertdialog, and nothing in it answers until it is over.
      await expect(banField(page)).toHaveValue("spam_bot_7731@social.example");
      await expect(banButton(page)).toBeDisabled();
      await expect(banOf(page).getByRole("button", { name: "Cancel", exact: true })).toBeDisabled();
      await expect(banOf(page).getByRole("button", { name: "Close", exact: true })).toBeDisabled();
    },
  ],
  [
    story("dialogs", "typed-confirm-others"),
    async (page) => {
      await expect(
        page.getByRole("textbox", { name: "To confirm, enter mycroblog.example", exact: true }),
      ).toBeVisible();
      await expect(
        page.getByRole("textbox", { name: "To confirm, enter Trusted reviewer", exact: true }),
      ).toBeVisible();
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
    story("facts", "reveal-asked"),
    async (page) => {
      // The value on its way: its Show waits, read with its term still.
      await expect(page.getByRole("button", { name: "Show", exact: true, description: "Email" })).toBeDisabled();
    },
  ],
  [
    story("facts", "revealed"),
    async (page) => {
      await expect(page.getByRole("button", { name: "Show", exact: true })).toHaveCount(0);
      await expect(page.getByRole("definition").filter({ hasText: "river.tanaka@mail.example" })).toHaveCount(1);
    },
  ],
  [
    story("facts", "endpoint"),
    async (page) => {
      await expect(page.getByRole("term")).toHaveText(["URL", "Events", "Signing secret", "Created", "Last delivery"]);
      await expect(page.getByRole("definition")).toHaveCount(5);
    },
  ],
  [
    story("facts", "in-a-section"),
    async (page) => {
      const section = page.getByRole("region", { name: "Registration", exact: true });
      await expect(section.getByRole("heading", { name: "Registration", exact: true })).toBeVisible();
      await expect(section.getByRole("term")).toHaveCount(6);
      await expect(section.getByRole("button", { name: "Show", exact: true, description: "Email" })).toBeEnabled();
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
    story("logs", "audit-log-open"),
    async (page) => {
      const log = page.getByRole("list", { name: "Audit log", exact: true });
      await expect(log.getByRole("button", { name: "Details", exact: true, expanded: true })).toHaveCount(2);
      await expect(log.getByRole("group", { name: "Details", exact: true })).toHaveCount(2);
      await expect(log.getByRole("term")).toHaveText(["Action", "Account", "Reports", "Notified", "Before", "After"]);
    },
  ],
  [
    story("logs", "webhook-deliveries"),
    async (page) => {
      const log = page.getByRole("list", { name: "Deliveries", exact: true });
      await expect(log.getByRole("listitem")).toHaveCount(3);
      await expect(log.getByRole("button", { name: "Details", exact: true, expanded: false })).toHaveCount(3);
      // Folded, what Details holds is out of the page.
      await expect(page.getByRole("group", { name: "Payload", exact: true })).toHaveCount(0);
    },
  ],
  [
    story("logs", "job-attempts"),
    async (page) => {
      // The newest first.
      await expect(page.getByRole("list", { name: "Attempts", exact: true }).getByRole("listitem")).toHaveText([
        /Attempt 3 delivered Create to social\.example/,
        /Attempt 2 failed/,
        /Attempt 1 failed/,
      ]);
      await expect(page.getByRole("button", { name: "Details", exact: true })).toHaveCount(1);
    },
  ],
  [
    story("logs", "code-blocks"),
    async (page) => {
      // A block of code is a group named by its label, its copy beside it; a list of values the same, a list.
      await expect(page.getByRole("group", { name: "Payload", exact: true })).toContainText(
        '"event": "account.suspended"',
      );
      await expect(page.getByRole("group", { name: "Errors", exact: true })).toContainText(
        "line 31: bad-actors.example is already suspended",
      );
      await expect(page.getByRole("list", { name: "Signing secret", exact: true }).getByRole("listitem")).toHaveText([
        "whsec_4f9b2c7e1a8d6305b9e2f71c4a0d8e36",
      ]);
      for (const copy of ["Copy payload", "Copy secret", "Copy errors"])
        await expect(page.getByRole("button", { name: copy, exact: true })).toBeVisible();
      await expect(page.getByRole("status").filter({ hasText: "Copied" })).toHaveCount(1);
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
    story("color-field", "meets"),
    async (page) => {
      const token = page.getByRole("group", { name: "Accent text", exact: true });
      for (const scheme of ["Light", "Dark"])
        await expect(token.getByRole("textbox", { name: `Accent text ${scheme}`, exact: true })).not.toHaveAttribute(
          "aria-invalid",
        );
      // The minimum once for the field; each pair its ratio, which a screen reader hears after its verdict.
      await expect(token.getByText("Each pair needs 4.5:1 or more (WCAG AA).", { exact: true })).toBeVisible();
      await expect(token.getByText(/^Meets AA \d+\.\d+:1$/)).toHaveCount(2);
    },
  ],
  [
    story("color-field", "below"),
    async (page) => {
      const token = page.getByRole("group", { name: "Accent text", exact: true });
      await expect(token.getByRole("textbox", { name: "Accent text Light", exact: true })).toHaveAttribute(
        "aria-invalid",
        "true",
      );
      await expect(token.getByRole("textbox", { name: "Accent text Dark", exact: true })).not.toHaveAttribute(
        "aria-invalid",
      );
      await expect(token.getByText(/^Below AA 2\.77:1, needs 4\.5:1$/)).toBeVisible();
    },
  ],
  [
    story("color-field", "misformed"),
    async (page) => {
      await expect(
        page.getByText("Enter a color as # and 6 characters from 0 to 9 and a to f, such as #ff6b6b.", {
          exact: true,
        }),
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

test("a kept value, once Show brings it a moment later, takes the focus", async ({ page, open }) => {
  await open(story("facts", "reveal-on-press"), { width: pc });
  await page.getByRole("button", { name: "Show", exact: true, description: "Email" }).click();
  await expect(page.getByText("river.tanaka@mail.example", { exact: true })).toBeFocused();
});

test("the irreversible confirmation unlocks only while its field holds the whole text typed, and locks again once it does not", async ({
  page,
  open,
}) => {
  await open(story("dialogs", "typed-confirm"), { width: pc });
  await banField(page).click();
  // Its Cancel answers while the confirmation waits.
  await expect(banOf(page).getByRole("button", { name: "Cancel", exact: true })).toBeEnabled();
  await page.keyboard.type("spam_bot_7731@social.exampl");
  await expect(banButton(page)).toBeDisabled();
  await page.keyboard.type("e");
  await expect(banButton(page)).toBeEnabled();
  await page.keyboard.press("Backspace");
  await expect(banButton(page)).toBeDisabled();
  // Whatever its case: a handle is the same in capitals.
  await banField(page).fill("SPAM_BOT_7731@Social.Example");
  await expect(banButton(page)).toBeEnabled();
});

test("the facts of a record stand side by side from an 18rem column, at a phone's width too, and each term over its value in a narrower one", async ({
  page,
  open,
}) => {
  const term = () => page.getByText("Registered", { exact: true });
  const value = () => page.getByText("Nov 3, 2026, 9:41 PM", { exact: true });
  for (const width of [pc, minimum]) {
    await open(story("facts", "registration"), { width });
    const [wideTerm, wideValue] = [await boxOf(term()), await boxOf(value())];
    expect(wideValue.left).toBeGreaterThan(wideTerm.right);
    expect(wideValue.top).toBeLessThan(wideTerm.bottom);
  }

  await open(story("facts", "narrow-column"), { width: pc });
  const [narrowTerm, narrowValue] = [await boxOf(term()), await boxOf(value())];
  expect(narrowValue.top).toBeGreaterThanOrEqual(narrowTerm.bottom);
  expect(Math.abs(narrowValue.left - narrowTerm.left)).toBeLessThan(1);
});

test("a block of code is shown whole: it never scrolls, and stays inside its width", async ({ page, open }) => {
  await open(story("logs", "code-blocks"), { width: minimum });
  const block = page.getByRole("group", { name: "Payload", exact: true });
  const sizes = await block.evaluate((el) => ({
    scrollHeight: el.scrollHeight,
    clientHeight: el.clientHeight,
    scrollWidth: el.scrollWidth,
    clientWidth: el.clientWidth,
  }));
  expect(sizes.scrollHeight).toBeLessThanOrEqual(sizes.clientHeight);
  expect(sizes.scrollWidth).toBeLessThanOrEqual(sizes.clientWidth);
  await expect.poll(() => reachingOut(page)).toEqual([]);
});
