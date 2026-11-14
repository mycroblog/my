// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { instance } from "../test/server";
import { accountOffer, visitorPlaces } from "./landing";

test("a server that takes new accounts offers Create an account, with what an account takes but in open mode", () => {
  expect(accountOffer("open")).toEqual({ signUp: true });
  expect(accountOffer("approval")).toEqual({ signUp: true, note: "app.landing_approval" });
  expect(accountOffer("invite")).toEqual({ signUp: true, note: "app.landing_invite" });
});

test("a server closed to new accounts offers no Create an account, and says it takes none", () => {
  expect(accountOffer("closed")).toEqual({ signUp: false, note: "app.landing_closed" });
});

test("a visitor goes on to the timelines shown to everyone, and to search", () => {
  const hrefs = (local: "off" | "signed_in" | "everyone", federated: "off" | "signed_in" | "everyone") =>
    visitorPlaces(instance({ timelines: { local, federated } })).map((p) => p.href);
  expect(hrefs("everyone", "everyone")).toEqual(["/local", "/public", "/search"]);
  expect(hrefs("everyone", "signed_in")).toEqual(["/local", "/search"]);
  expect(hrefs("signed_in", "off")).toEqual(["/search"]);
});
