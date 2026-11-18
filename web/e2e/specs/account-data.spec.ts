// SPDX-License-Identifier: AGPL-3.0-only

// A person's data in the settings, on the real server (#3203, FR-008): a fresh person imports a CSV file of follows
// as an export writes it, sees what the server read in it, imports it, and then follows the account it lists. The
// import runs in the background; its account is found once the job has followed it. What the screens of the export,
// the import, the deletion of old posts and the aliases show and send is a component test on the fake (web/app,
// settings-account-data.dom.test.tsx).

import { AccountData, Profile } from "./screens";
import { expect, test } from "./test";

test("a person imports a CSV file of follows, sees its preview, and follows the account it lists once it is imported", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  const followed = await fresh.person();
  await signIn(reader);
  // The domain of the server's accounts, which a handle in an export carries.
  const instance = (await (await page.request.get("/api/v1/instance")).json()) as { domain: string };

  const data = new AccountData(page);
  await data.openImport();
  await data.chooseFile(
    "follows.csv",
    `Account address,Show boosts,Notify on new posts,Languages\n${followed.username}@${instance.domain},true,false,\n`,
  );
  await expect(data.preview).toHaveText(["1 new account to follow"]);

  await data.startImport();
  await expect(data.recentImports).toHaveCount(1);
  await expect(data.recentImports).toHaveText([/^Follows\s*Merge\b/]);

  // The server follows the account in a job of its own: its profile, read again until it is done, says so.
  const profile = new Profile(page, followed);
  await expect(async () => {
    await profile.open();
    await expect(profile.followingButton).toBeVisible();
  }).toPass();
});
