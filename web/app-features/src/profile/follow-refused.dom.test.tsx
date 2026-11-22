// SPDX-License-Identifier: AGPL-3.0-only

// A follow that did not happen (#3216), from a profile's head (`Follow`) and from a row of an account list
// (`FollowOf`): refused for a restriction of the reader's account, it says the server's sentence and links to Account
// status; with no answer of the server, it says that the follow, or the unfollowing, did not finish, with Try again.

import { expect, test } from "bun:test";
import { accountStatusPath } from "@my/app-components";
import { scenario } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { NavigationProvider } from "@my/design-system";
import type { ReactNode } from "react";
import { PostActions } from "../post-actions";
import { fakeData, renderFeature, type FakeData } from "../test/feature";
import { Follows, loadFollows } from "./follows";
import { loadProfile, Profile } from "./profile";

/** The server's sentence of a follow refused for a restriction of interacting. */
const interacting = "A moderation action keeps your account from interacting with others for now.";

const restricted = {
  type: "/api/v1/problems/interaction-restricted",
  title: "Forbidden",
  status: 403,
  detail: interacting,
} as const;

/** A server where the reader follows sam alone. */
const followingSam = () =>
  fakeData({ seed: { people: scenario.people, follows: [{ follower: "yuki", followee: "sam" }] } });

/** `children` drawn on `made`'s data under the app's navigation; `went` holds where a link asked to go. */
function drawnOn(made: FakeData, children: ReactNode) {
  const went: string[] = [];
  const user = renderFeature(
    made.data,
    <NavigationProvider navigate={(to) => went.push(to)}>
      <PostActions onCompose={() => undefined}>{children}</PostActions>
    </NavigationProvider>,
  );
  return { ...made, user, went };
}

/** The profile of `acct` on `made`'s server, once its head offers the button `name`. */
async function profileOf(acct: string, made: FakeData, name: string) {
  await loadProfile(made.data, acct);
  const drawn = drawnOn(made, <Profile acct={acct} />);
  const button = await within(await screen.findByRole("region")).findByRole("button", { name });
  return { ...drawn, button };
}

/** What says `words` on the page, a status or an alert, once it does. */
const sayingOf = (words: string) =>
  waitFor(() => {
    const found = [...screen.queryAllByRole("status"), ...screen.queryAllByRole("alert")].find((region) =>
      region.textContent.includes(words),
    );
    if (found === undefined) throw new Error(`test: nothing on the page says "${words}"`);
    return found;
  });

/** Confirms an unfollow when the screen asks first. */
async function confirmed(user: ReturnType<typeof drawnOn>["user"]) {
  const asking = screen.queryAllByRole("alertdialog");
  const [dialog] = asking;
  if (dialog !== undefined) await user.click(within(dialog).getByRole("button", { name: /^Unfollow/ }));
}

test("a follow from a profile refused for a restriction says the server's sentence, with the link to Account status and no Try again", async () => {
  // The reader does not follow marco.
  const made = fakeData();
  made.fake.script("POST /accounts/{id}/follow", { fail: restricted });
  const { user, button, went } = await profileOf("marco", made, "Follow");

  await user.click(button);

  const said = await sayingOf(interacting);
  const link = within(said).getByRole("link", { name: "Account status" });
  expect(link.getAttribute("href")).toBe(accountStatusPath);
  expect(screen.queryAllByRole("button", { name: "Try again" })).toHaveLength(0);
  await user.click(link);
  expect(went).toEqual([accountStatusPath]);
});

test("a follow from a profile that gets no answer says it did not finish, with Try again, which follows", async () => {
  const made = fakeData();
  const every = made.fake.script("POST /accounts/{id}/follow");
  made.fake.script("POST /accounts/{id}/follow", { times: 1, fail: "network" });
  const { user, button } = await profileOf("marco", made, "Follow");

  await user.click(button);

  const said = await sayingOf("The follow did not finish.");
  expect(screen.queryAllByRole("link", { name: "Account status" })).toHaveLength(0);
  await user.click(within(said).getByRole("button", { name: "Try again" }));
  await waitFor(() => {
    if (every.seen.length !== 2) throw new Error("test: the follow was not sent again");
  });
  expect(await within(screen.getByRole("region")).findByRole("button", { name: "Following" })).not.toBeNull();
});

test("an unfollow from a profile that gets no answer says unfollowing did not finish, with Try again", async () => {
  const made = followingSam();
  made.fake.script("DELETE /accounts/{id}/follow", { times: 1, fail: "network" });
  const { user, button } = await profileOf("sam", made, "Following");

  await user.click(button);
  await confirmed(user);

  const said = await sayingOf("Unfollowing did not finish.");
  expect(within(said).getAllByRole("button", { name: "Try again" })).toHaveLength(1);
});

/** sam's followers on `made`'s server, drawn as a list, once marco's row offers Follow. */
async function followersOfSam(made: FakeData) {
  await loadFollows(made.data, "sam", "followers");
  const drawn = drawnOn(
    made,
    <Follows acct="sam" list="followers" onList={() => undefined} onBack={() => undefined} />,
  );
  const panel = within(screen.getByRole("tabpanel"));
  const button = await panel.findByRole("button", { name: "Follow" });
  return { ...drawn, button };
}

test("a follow from a row of an account list refused for a restriction says the server's sentence, with the link to Account status", async () => {
  // marco, layla, yuki and hana follow sam; the reader follows all but marco.
  const made = fakeData();
  made.fake.script("POST /accounts/{id}/follow", { fail: restricted });
  const { user, button } = await followersOfSam(made);

  await user.click(button);

  const said = await sayingOf(interacting);
  expect(within(said).getByRole("link", { name: "Account status" }).getAttribute("href")).toBe(accountStatusPath);
  expect(screen.queryAllByRole("button", { name: "Try again" })).toHaveLength(0);
});

test("a follow from a row of an account list that gets no answer says it did not finish, with Try again", async () => {
  const made = fakeData();
  made.fake.script("POST /accounts/{id}/follow", { times: 1, fail: "network" });
  const { user, button } = await followersOfSam(made);

  await user.click(button);

  const said = await sayingOf("The follow did not finish.");
  expect(within(said).getAllByRole("button", { name: "Try again" })).toHaveLength(1);
  expect(screen.queryAllByRole("link", { name: "Account status" })).toHaveLength(0);
});
