// SPDX-License-Identifier: AGPL-3.0-only

// A person follows and unfollows another from their profile, and follows one of that person's followers from its row
// in the list the profile's counts open; the server keeps each. And a person edits their own profile from it (#3200):
// the display name and the avatar saved in the form show on the profile at once and after a reload, the browser's Back
// does not lose what the form holds unsaved without asking, and a choice of the settings' Profile, which saves as it
// is made, is the account's after a reload. Each reader is a fresh person.
// What each control of the settings saves and says is a component test on the fake (app-features,
// settings/profile.dom.test.tsx and profile-edit.dom.test.tsx).

import { drawPNG } from "../src/seed/png";
import { countLink, Follows, Frame, Profile, ProfileForm, ProfileSettings, type Upload } from "./screens";
import { expect, test } from "./test";

/** A small PNG to pick in the file chooser. */
const png = (name: string): Upload => ({
  name,
  mimeType: "image/png",
  buffer: Buffer.from(
    drawPNG({
      width: 96,
      height: 96,
      top: [140, 196, 236],
      bottom: [52, 84, 120],
      shapes: [{ kind: "circle", x: 48, y: 48, r: 24, color: [250, 240, 220] }],
    }),
  ),
});

test("a person follows another from their profile and one of their followers from its row, then unfollows, and each stays after a reload", async ({
  page,
  signIn,
  fresh,
}) => {
  const follower = await fresh.person();
  const followee = await fresh.person({ followedBy: [follower] });
  await signIn(await fresh.person());
  const profile = new Profile(page, followee);
  await profile.open();
  await profile.follow();
  await expect(profile.followingButton).toBeVisible();
  await page.reload();
  await expect(profile.followingButton).toBeVisible();

  // The followers: the one who followed before, whose row offers a follow, and the reader, whose row offers none.
  await countLink(profile.card, "Followers").click();
  const follows = new Follows(page, followee);
  await expect(page).toHaveURL(follows.address("Followers"));
  await expect(follows.tab("Followers")).toHaveAttribute("aria-selected", "true");
  await expect(follows.row(follower)).toBeVisible();
  await expect(follows.followButtons).toHaveCount(1);
  await follows.followTheOne();
  await expect(follows.followButtons).toHaveCount(0);
  await page.reload();
  await expect(follows.row(follower)).toBeVisible();
  await expect(follows.followButtons).toHaveCount(0);
  await follows.row(follower).click();
  await expect(new Profile(page, follower).followingButton).toBeVisible();

  await profile.open();
  await profile.unfollow();
  await expect(profile.followButton).toBeVisible();
  await page.reload();
  await expect(profile.followButton).toBeVisible();
});

test("a person changes their display name and their avatar from their own profile, and the profile shows both at once and after a reload", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  // A name of the test's own, within the 30 characters a display name takes.
  const renamed = { ...reader, displayName: `Snow ${reader.displayName}` };
  await signIn(reader);
  await new Frame(page).goTo("Profile");
  const before = new Profile(page, reader);
  await expect(before.heading).toBeVisible();
  // No picture yet: the person's initial stands in for the avatar.
  await expect(before.pictures).toHaveCount(0);

  await before.editProfile.click();
  const form = new ProfileForm(page);
  await expect(form.heading).toBeVisible();
  await expect(page).toHaveURL("/settings/profile/edit");
  await expect(form.displayName).toHaveValue(reader.displayName);
  await form.displayName.fill(renamed.displayName);
  // The browser's own Back is a way out of the form too: the app asks before the name written is lost.
  await page.goBack();
  await form.keepEditing();
  await expect(page).toHaveURL("/settings/profile/edit");
  await expect(form.displayName).toHaveValue(renamed.displayName);
  await form.chooseAvatar(png("snow.png"));
  await form.save();

  // The profile shows the change with no reload: the navigation's link stays in the app.
  await new Frame(page).goTo("Profile");
  const after = new Profile(page, renamed);
  /** Whether the profile's one picture, the avatar, is drawn from what the server serves. */
  const drawn = () =>
    after.pictures.evaluate((picture: HTMLImageElement) => picture.complete && picture.naturalWidth > 0);
  await expect(after.heading).toBeVisible();
  await expect(after.pictures).toHaveCount(1);
  await expect.poll(drawn).toBe(true);

  await page.reload();
  await expect(after.heading).toBeVisible();
  await expect(after.pictures).toHaveCount(1);
  await expect.poll(drawn).toBe(true);
});

test("a choice of the settings' Profile and a featured hashtag, each saved as it is made, are the account's after a reload", async ({
  page,
  signIn,
  fresh,
}) => {
  const locked = "Require approval to follow you";
  await signIn(await fresh.person());
  const settings = new ProfileSettings(page);
  await settings.goTo();
  await expect(settings.toggle(locked)).not.toBeChecked();
  await expect(settings.featured("sourdough")).toHaveCount(0);

  await settings.flip(locked);
  await expect(settings.toggle(locked)).toBeChecked();
  await settings.feature("sourdough");
  await expect(settings.featured("sourdough")).toBeVisible();

  await page.reload();
  await expect(settings.heading).toBeVisible();
  await expect(settings.toggle(locked)).toBeChecked();
  await expect(settings.featured("sourdough")).toBeVisible();
});
