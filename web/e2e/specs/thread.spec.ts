// SPDX-License-Identifier: AGPL-3.0-only

// A person opens a post from the home timeline and reads its thread, and replies in it.

import { homeTimeline, Thread } from "./screens";
import { expect, person, test } from "./test";

test("a post opens with its replies in the order they were written", async ({ page, signIn }) => {
  const yuki = person("yuki");
  await signIn(person("sam"));
  await homeTimeline(page).post(yuki.displayName, "今朝の尾根").open();
  const thread = new Thread(page);
  await expect(thread.heading).toBeVisible();
  await expect(thread.posts).toContainText([
    "今朝の尾根。三時に起きた甲斐がありました",
    "That light! Which trail is this?",
    "北側の尾根道です。駐車場から二時間くらい。",
    "Adding it to my list for the spring.",
    "きれい！次の集まりのポスターに使わせてほしいです",
  ]);
});

test("a person replies in a thread and the reply stays there after a reload", async ({ page, signIn }) => {
  const marco = person("marco");
  const sam = person("sam");
  const text = "Thread check: that crumb looks right to me.";
  await signIn(marco);
  await homeTimeline(page).post(sam.displayName, "Day 3 of the sourdough").open();
  const thread = new Thread(page);
  const composer = await thread.post(sam.displayName, "Day 3 of the sourdough").reply();
  await expect(composer.text).toHaveValue(/^@sam\b/);
  await composer.write(text);
  await composer.post();
  await expect(thread.post(marco.displayName, text).article).toBeVisible();
  await page.reload();
  await expect(thread.post(marco.displayName, text).paragraph(text)).toContainText("@sam");
});
