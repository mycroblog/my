// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, onTestFinished, test } from "bun:test";
import { QueryObserver } from "@tanstack/react-query";
import { accountByIdQuery, accountStatusesQuery, useFeaturedHashtags, viewerQuery, type Account } from "./accounts";
import { notificationsQuery } from "./notifications";
import {
  badgeChoicesQuery,
  chooseBadge,
  failedPicture,
  followedHashtagsQuery,
  ownFeaturedHashtagsQuery,
  pictureRefusal,
  profileLimits,
  setFeaturedHashtags,
  updateProfile,
  useBadgeChoices,
  useFollowedHashtags,
  useOwnFeaturedHashtags,
} from "./profile";
import { followTag } from "./tags";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { refusedWith } from "./test/refused";
import { settled } from "./test/settings";
import { homeTimelineQuery } from "./timeline";

const picture = (name: string) => new File([new Uint8Array([137, 80, 78, 71])], name, { type: "image/png" });
const writes = (requests: readonly string[]) => requests.filter((r) => !r.startsWith("GET "));

describe("profileLimits", () => {
  test("are the limits of the specification, which the screens count against", () => {
    expect(profileLimits).toEqual({ displayName: 30, note: 500, fieldText: 255, fields: 4, featuredHashtags: 10 });
  });
});

describe("updateProfile", () => {
  test("sends only what changed, and the viewer and the account's copy hold the answer", async () => {
    const { fake, data, requests } = fakeData();
    const sent = fake.script("PATCH /accounts/update_credentials");
    const viewer = await data.queryClient.query(viewerQuery(data));
    await data.queryClient.query(accountByIdQuery(data, viewer.id));
    const answer = await updateProfile(data, { display_name: "Yuki 雪", note: "Baker.", locked: true });
    expect(sent.seen.map((s) => s.body)).toEqual([{ display_name: "Yuki 雪", note: "Baker.", locked: true }]);
    expect(writes(requests)).toEqual(["PATCH /accounts/update_credentials"]);
    expect(answer).toMatchObject({ id: viewer.id, display_name: "Yuki 雪", locked: true });
    const cachedViewer = data.queryClient.getQueryData<Account>(viewerQuery(data).queryKey);
    const cachedCopy = data.queryClient.getQueryData<Account>(accountByIdQuery(data, viewer.id).queryKey);
    expect(cachedViewer).toMatchObject({ display_name: "Yuki 雪", locked: true });
    expect(cachedCopy).toMatchObject({ display_name: "Yuki 雪", locked: true });
  });

  test("uploads a picked picture first, and the change names its media; null removes the picture", async () => {
    const { fake, data, requests } = fakeData();
    const sent = fake.script("PATCH /accounts/update_credentials");
    await updateProfile(data, { avatar: picture("me.png"), header: null });
    expect(writes(requests)).toEqual(["POST /media", "PATCH /accounts/update_credentials"]);
    const [body] = sent.seen.map((s) => s.body);
    expect(body?.avatar_media_id).toBeTruthy();
    // The specification: an empty ID removes the picture.
    expect(body?.header_media_id).toBe("");
    expect(body).not.toHaveProperty("avatar");
    expect(body).not.toHaveProperty("header");
  });

  test("leaves a picture out of the change when it is not given", async () => {
    const { fake, data } = fakeData();
    const sent = fake.script("PATCH /accounts/update_credentials");
    await updateProfile(data, { bot: true });
    const [body] = sent.seen.map((s) => s.body);
    expect(body).not.toHaveProperty("avatar_media_id");
    expect(body).not.toHaveProperty("header_media_id");
  });

  test("marks the timelines, the notifications and the statuses stale", async () => {
    const { data } = fakeData();
    const viewer = await data.queryClient.query(viewerQuery(data));
    const keys = [
      homeTimelineQuery(data).queryKey,
      notificationsQuery(data, "all").queryKey,
      accountStatusesQuery(data, viewer.id, "posts").queryKey,
    ];
    await data.queryClient.infiniteQuery(homeTimelineQuery(data));
    await data.queryClient.infiniteQuery(notificationsQuery(data, "all"));
    await data.queryClient.infiniteQuery(accountStatusesQuery(data, viewer.id, "posts"));
    await updateProfile(data, { display_name: "Yuki 雪" });
    for (const key of keys) expect([key, data.queryClient.getQueryState(key)?.isInvalidated]).toEqual([key, true]);
  });

  test("rejects with the problem the server gave, and the caches keep the profile", async () => {
    const { fake, data } = fakeData();
    const before = await data.queryClient.query(viewerQuery(data));
    fake.script("PATCH /accounts/update_credentials", {
      fail: { type: "/api/v1/problems/profile-rejected", title: "The profile is rejected", status: 422 },
    });
    const refused = await refusedWith(updateProfile(data, { display_name: "Yuki 雪" }));
    expect([refused.status, refused.type]).toEqual([422, "/api/v1/problems/profile-rejected"]);
    expect(data.queryClient.getQueryData<Account>(viewerQuery(data).queryKey)).toEqual(before);
  });

  test("does not change the profile when the upload of the picture is refused", async () => {
    const { fake, data, requests } = fakeData();
    fake.script("POST /media", { fail: 422 });
    const refused = await refusedWith(updateProfile(data, { display_name: "Yuki 雪", avatar: picture("me.png") }));
    expect(refused.status).toBe(422);
    expect(writes(requests)).toEqual(["POST /media"]);
  });
});

// What a form needs to say what is wrong under the image it is about, and not beside Save (#3200).
describe("failedPicture", () => {
  /** What `write` was rejected with, whatever it is. */
  const rejection = (write: Promise<unknown>) =>
    write.then(
      () => undefined,
      (error: unknown) => error,
    );

  for (const which of ["avatar", "header"] as const) {
    test(`names the ${which} when its upload is what the server refused`, async () => {
      const { fake, data } = fakeData();
      fake.script("POST /media", { fail: 422 });
      const refused = await refusedWith(updateProfile(data, { display_name: "Yuki 雪", [which]: picture("me.png") }));
      expect(failedPicture(refused)).toBe(which);
    });

    test(`names the ${which} when its upload got no answer`, async () => {
      const { fake, data } = fakeData();
      fake.script("POST /media", { fail: "network" });
      const failed = await rejection(updateProfile(data, { [which]: picture("me.png") }));
      expect(failed).toBeDefined();
      expect(failedPicture(failed)).toBe(which);
    });
  }

  test("names no picture when the pictures were uploaded and the change itself is refused", async () => {
    const { fake, data, requests } = fakeData();
    fake.script("PATCH /accounts/update_credentials", { fail: 422 });
    const refused = await refusedWith(updateProfile(data, { avatar: picture("me.png"), header: picture("sky.png") }));
    expect(writes(requests)).toEqual(["POST /media", "POST /media", "PATCH /accounts/update_credentials"]);
    expect(failedPicture(refused)).toBeUndefined();
  });

  test("names no picture for an error of something else", () => {
    expect(failedPicture(new Error("something else"))).toBeUndefined();
    expect(failedPicture(undefined)).toBeUndefined();
  });
});

// What a form says under the picture the server refused: the server's own words.
describe("pictureRefusal", () => {
  const refusedUpload = async (problem: { type: string; title: string; detail?: string }) => {
    const { fake, data } = fakeData();
    fake.script("POST /media", { fail: { ...problem, status: 422 } });
    return refusedWith(updateProfile(data, { avatar: picture("me.png") }));
  };

  test("is the detail of the server's refusal of an upload, where it gives one", async () => {
    const refused = await refusedUpload({
      type: "/api/v1/problems/media-too-large",
      title: "The file is too large.",
      detail: "Choose a file of at most 8 MB.",
    });
    expect(pictureRefusal(refused)).toBe("Choose a file of at most 8 MB.");
  });

  for (const kind of [
    "media-too-large",
    "unsupported-media-format",
    "media-dimensions",
    "media-corrupt",
    "media-too-long",
  ]) {
    test(`is the title of a refusal of the kind ${kind} that gives no detail`, async () => {
      const refused = await refusedUpload({ type: `/api/v1/problems/${kind}`, title: "The server's words." });
      expect(pictureRefusal(refused)).toBe("The server's words.");
    });
  }

  test("is none for another refusal without a detail, and for what is no refusal of the server", async () => {
    const refused = await refusedUpload({ type: "about:blank", title: "The request failed" });
    expect(pictureRefusal(refused)).toBeUndefined();
    expect(pictureRefusal(new TypeError("Failed to fetch"))).toBeUndefined();
    expect(pictureRefusal(undefined)).toBeUndefined();
  });
});

describe("badges", () => {
  test("a choice is kept in the cached list, and marks the viewer stale", async () => {
    const { data, requests } = fakeData();
    await data.queryClient.query(viewerQuery(data));
    await data.queryClient.query(badgeChoicesQuery(data));
    const [moderator, supporter] = probe(data, useBadgeChoices);
    expect([moderator?.displayed, supporter?.displayed]).toEqual([true, false]);
    await chooseBadge(data, supporter?.role_id ?? "", true);
    await chooseBadge(data, moderator?.role_id ?? "", false);
    expect(writes(requests)).toEqual([
      `PUT /preferences/badges/${supporter?.role_id}`,
      `PUT /preferences/badges/${moderator?.role_id}`,
    ]);
    expect(probe(data, useBadgeChoices).map((b) => [b.label, b.displayed])).toEqual([
      ["Moderator", false],
      ["Supporter", true],
    ]);
    expect(data.queryClient.getQueryState(viewerQuery(data).queryKey)?.isInvalidated).toBe(true);
  });

  test("a role the reader does not hold is refused as not found, and the list stays", async () => {
    const { data } = fakeData();
    await data.queryClient.query(badgeChoicesQuery(data));
    const refused = await refusedWith(chooseBadge(data, "no-such-role", true));
    expect(refused.status).toBe(404);
    expect(probe(data, useBadgeChoices).map((b) => b.displayed)).toEqual([true, false]);
  });
});

// Both write the reader's account: a change of the profile and a badge's choice go out one after the other.
describe("a profile change and a badge's choice", () => {
  const badgeRoute = "PUT /preferences/badges/{role_id}";
  const patchRoute = "PATCH /accounts/update_credentials";
  /** Some turns of the event loop: time for a request that is not queued to leave. */
  const turns = async (n = 5) => {
    for (let i = 0; i < n; i++) await new Promise((turn) => setTimeout(turn, 0));
  };

  async function withBadges() {
    const on = fakeData();
    await on.data.queryClient.query(viewerQuery(on.data));
    await on.data.queryClient.query(badgeChoicesQuery(on.data));
    const [, supporter] = probe(on.data, useBadgeChoices);
    return { ...on, roleId: supporter?.role_id ?? "" };
  }

  test("a change of the profile is sent only once the badge's choice before it was answered", async () => {
    const { fake, data, roleId } = await withBadges();
    const badge = fake.script(badgeRoute, { hold: "response" });
    const patch = fake.script(patchRoute);

    const chosen = chooseBadge(data, roleId, true);
    const changed = updateProfile(data, { display_name: "Yuki the baker" });
    await badge.served();
    await turns();
    expect(patch.seen).toHaveLength(0);

    badge.release();
    await Promise.all([chosen, changed]);
    expect(patch.seen).toHaveLength(1);
  });

  test("a badge's choice is sent only once the change of the profile before it was answered", async () => {
    const { fake, data, roleId } = await withBadges();
    const patch = fake.script(patchRoute, { hold: "response" });
    const badge = fake.script(badgeRoute);

    const changed = updateProfile(data, { display_name: "Yuki the baker" });
    const chosen = chooseBadge(data, roleId, true);
    await patch.served();
    await turns();
    expect(badge.seen).toHaveLength(0);

    patch.release();
    await Promise.all([changed, chosen]);
    expect(badge.seen).toHaveLength(1);
  });

  test("the account a badge's choice reads again never lands over the answer of a change sent after it", async () => {
    const { fake, data, roleId } = await withBadges();
    // A screen shows the reader's account, so what marks it is read again at once.
    onTestFinished(new QueryObserver(data.queryClient, viewerQuery(data)).subscribe(() => undefined));
    // The account read after the badge's choice answers late, with the name from before the change.
    const reread = fake.script("GET /accounts/verify_credentials", { hold: "response", times: 1 });

    const chosen = chooseBadge(data, roleId, true);
    const changed = updateProfile(data, { display_name: "Yuki the baker" });
    // Whichever waits for the other, the late read is let go once the change was answered or after some turns.
    await Promise.race([changed, turns(20)]);
    reread.release();
    await Promise.all([chosen, changed]);
    await settled(data);

    expect(data.queryClient.getQueryData<Account>(viewerQuery(data).queryKey)?.display_name).toBe("Yuki the baker");
  });
});

describe("featured hashtags", () => {
  test("setting them keeps the own list and the profile's copy as the server answered", async () => {
    const { fake, data } = fakeData();
    const viewer = await data.queryClient.query(viewerQuery(data));
    await data.queryClient.query(ownFeaturedHashtagsQuery(data));
    expect(probe(data, useOwnFeaturedHashtags).map((t) => t.name)).toEqual(["baking", "photography"]);
    const sent = fake.script("PUT /featured_tags");
    await setFeaturedHashtags(data, ["#sourdough", "baking"]);
    expect(sent.seen.map((s) => s.body)).toEqual([{ names: ["#sourdough", "baking"] }]);
    expect(probe(data, useOwnFeaturedHashtags).map((t) => t.name)).toEqual(["sourdough", "baking"]);
    expect(probe(data, () => useFeaturedHashtags(viewer.id)).map((t) => t.name)).toEqual(["sourdough", "baking"]);
  });

  test("an empty list removes them all", async () => {
    const { data } = fakeData();
    await data.queryClient.query(ownFeaturedHashtagsQuery(data));
    await setFeaturedHashtags(data, []);
    expect(probe(data, useOwnFeaturedHashtags)).toEqual([]);
  });

  test("more than the limit is refused with a problem of its own, and the list stays", async () => {
    const { data } = fakeData();
    await data.queryClient.query(ownFeaturedHashtagsQuery(data));
    const names = Array.from({ length: profileLimits.featuredHashtags + 1 }, (_, i) => `tag${String(i)}`);
    const refused = await refusedWith(setFeaturedHashtags(data, names));
    expect([refused.status, refused.type]).toEqual([422, "/api/v1/problems/too-many-featured-tags"]);
    expect(probe(data, useOwnFeaturedHashtags).map((t) => t.name)).toEqual(["baking", "photography"]);
  });

  test("a name that is no hashtag is refused", async () => {
    const { data } = fakeData();
    expect((await refusedWith(setFeaturedHashtags(data, ["not a tag"]))).status).toBe(422);
  });
});

describe("followed hashtags", () => {
  test("following a hashtag marks the list of followed ones stale, and reading it again holds the hashtag", async () => {
    const { data } = fakeData();
    await data.queryClient.query(followedHashtagsQuery(data));
    const before = probe(data, useFollowedHashtags).map((t) => t.name);
    expect(before).not.toContain("sourdough");
    await followTag(data, "sourdough", true);
    expect(data.queryClient.getQueryState(followedHashtagsQuery(data).queryKey)?.isInvalidated).toBe(true);
    await data.queryClient.query(followedHashtagsQuery(data));
    expect(probe(data, useFollowedHashtags).map((t) => t.name)).toContain("sourdough");
    await followTag(data, "sourdough", false);
    expect(data.queryClient.getQueryState(followedHashtagsQuery(data).queryKey)?.isInvalidated).toBe(true);
    await data.queryClient.query(followedHashtagsQuery(data));
    expect(probe(data, useFollowedHashtags).map((t) => t.name)).not.toContain("sourdough");
  });
});
