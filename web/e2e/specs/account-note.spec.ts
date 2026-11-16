// SPDX-License-Identifier: AGPL-3.0-only

// A person writes a private note on another person's profile, reads it after a reload, and deletes it.

import { AccountNote, Profile } from "./screens";
import { expect, test } from "./test";

test("a person writes a private note on another's profile and reads it after a reload, then deletes it", async ({
  page,
  signIn,
  fresh,
}) => {
  const other = await fresh.person();
  await signIn(await fresh.person());
  const profile = new Profile(page, other);
  await profile.open();
  const note = new AccountNote(page, profile.card);
  const text = "Met at the bakery on Saturday. 土曜のパン屋で会った。";
  await note.add.click();
  await note.box.fill(text);
  await note.save();
  await expect(note.written(text)).toBeVisible();
  await page.reload();
  await expect(note.written(text)).toBeVisible();
  await note.written(text).click();
  await expect(note.box).toBeFocused();
  // An empty note deletes the note.
  await page.keyboard.press("ControlOrMeta+a");
  await page.keyboard.press("Delete");
  await note.save();
  await expect(note.add).toBeVisible();
  await page.reload();
  await expect(note.add).toBeVisible();
  await expect(note.written(text)).toHaveCount(0);
});
