// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { accountByIdQuery, accountStatusesQuery, useFeaturedHashtags, viewerQuery, type Account } from "./accounts";
import { notificationsQuery } from "./notifications";
import {
  badgeChoicesQuery,
  chooseBadge,
  followedHashtagsQuery,
  ownFeaturedHashtagsQuery,
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
