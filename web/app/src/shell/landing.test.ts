// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { instanceOf } from "../test/server";
import { accountOffer, landingFeatures, visitorPlaces } from "./landing";

test("a server that takes new accounts offers to ask for one, with what an account takes but in open mode", () => {
  expect(accountOffer("open")).toEqual({ signUp: "app.landing_sign_up" });
  expect(accountOffer("approval")).toEqual({ signUp: "app.landing_sign_up", note: "app.landing_approval" });
  expect(accountOffer("invite")).toEqual({ signUp: "app.landing_sign_up_invite", note: "app.landing_invite" });
});

test("a server closed to new accounts offers no account, and says it takes none", () => {
  expect(accountOffer("closed")).toEqual({ note: "app.landing_closed" });
});

test("the landing page names what a member can do of what is on here", async () => {
  const keys = async (server: Parameters<typeof instanceOf>[0]) =>
    landingFeatures(await instanceOf(server)).map((f) => f.key);
  expect(await keys({})).toEqual(["articles", "events", "voice"]);
  expect(await keys({ voiceOff: true, eventCreators: "permitted", search: { posts: true } })).toEqual([
    "articles",
    "search",
  ]);
});

test("a visitor goes on to the timelines shown to everyone, and to search", async () => {
  const hrefs = async (local: "off" | "signed_in" | "everyone", federated: "off" | "signed_in" | "everyone") =>
    visitorPlaces(await instanceOf({ timelines: { local, federated } })).map((p) => p.href);
  expect(await hrefs("everyone", "everyone")).toEqual(["/local", "/public", "/search"]);
  expect(await hrefs("everyone", "signed_in")).toEqual(["/local", "/search"]);
  expect(await hrefs("signed_in", "off")).toEqual(["/search"]);
});
