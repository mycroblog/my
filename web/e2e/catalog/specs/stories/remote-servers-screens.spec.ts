// SPDX-License-Identifier: AGPL-3.0-only

// The staff servers list of the stories (web/app/src/domains.stories.tsx, #3478) as a browser draws it at a phone's,
// a tablet's and a computer's width: each known server's row shows its domain, its state and its software (the words
// for a server that reports none too), and the page does not scroll sideways. What the list holds and what its filter
// asks is the app's route tests' (web/app/src/remote-servers.dom.test.tsx).

import type { Page } from "@playwright/test";
import { expectNoSidewaysScroll, shownTexts } from "../boxes";
import { expect, from, minimum, pc, test } from "../test";

const story = "screens-moderation-domains--remote-servers";

/** The fake's servers in domain order, each with its state and its software as the list says them. */
const servers = [
  ["a-rather-long-subdomain.of-a-community-run-server.social.example", "Available", "snac 2.63"],
  ["dead.example", "Unavailable", "mastodon 4.2.10"],
  ["flaky.example", "Available", "misskey 2024.11.0"],
  ["gone.example", "Unavailable", "Not reported"],
  ["mastodon.example", "Available", "mastodon 4.3.2"],
  ["misskey.example", "Available", "misskey 2025.1.0"],
  ["photos.example", "Available", "pixelfed 0.12.4"],
  ["quiet.example", "Available", "Not reported"],
  ["social.example", "Available", "gotosocial"],
] as const;

/** For each row of the table, what of its server's domain, state and software it does not show. */
async function unshown(page: Page): Promise<string[][]> {
  const rows = await page
    .getByRole("main")
    .getByRole("table", { name: "Remote servers", exact: true })
    .getByRole("row")
    .all();
  const shown = await Promise.all(rows.slice(1).map((row) => shownTexts(row)));
  if (shown.length !== servers.length) return [[`${String(shown.length)} rows`]];
  // A domain may be drawn in pieces, each where a line may break: read it from the row's texts joined.
  return servers.map(([domain, ...words], i) => [
    ...((shown[i] ?? []).join("").includes(domain) ? [] : [domain]),
    ...words.filter((text) => !(shown[i] ?? []).includes(text)),
  ]);
}

test("each server's row shows its domain, its state and its software at a phone's, a tablet's and a computer's width", async ({
  page,
  open,
}) => {
  for (const width of [minimum, from.tablet, pc])
    await test.step(`${String(width)} px`, async () => {
      await open(story, { width });
      await expect(page.getByRole("main").getByRole("table", { name: "Remote servers", exact: true })).toBeVisible();
      await expect.poll(() => unshown(page)).toEqual(servers.map(() => []));
      await expectNoSidewaysScroll(page);
    });
});
