// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { rightsOf, type Rights } from "@my/app-data";
import { instanceOf } from "../test/server";
import { aroundPlacesFor, composesAt, isPlaceAt, navigationPaths, placesFor, type Place } from "./places";

const hrefs = (places: readonly Place[]) => places.map((p) => p.href);
const none: Rights = rightsOf([]);
const staff: Rights = rightsOf(["manage_reports", "manage_settings", "manage_finance"]);

test("a signed-in reader's tab bar is Home, Local, Federated, Notifications, Search, then Direct messages, Profile, Bookmarks, Settings", async () => {
  const places = placesFor(true, await instanceOf(), none);
  expect(hrefs(places.tabs)).toEqual(["/", "/local", "/public", "/notifications", "/search"]);
  expect(hrefs(places.own)).toEqual(["/conversations", "/profile", "/bookmarks", "/settings"]);
  expect(places.rights).toEqual([]);
});

test("the rail and the sidebar put Direct messages after Notifications (ADR-0110 §11)", async () => {
  expect(hrefs(placesFor(true, await instanceOf(), none).side)).toEqual([
    "/",
    "/local",
    "/public",
    "/notifications",
    "/conversations",
    "/search",
    "/profile",
    "/bookmarks",
    "/settings",
  ]);
});

test("Notifications and Direct messages carry the unread counts and Moderation the unhandled count", async () => {
  const places = placesFor(true, await instanceOf(), staff);
  expect(places.tabs.find((p) => p.href === "/notifications")?.count).toBe("notifications");
  expect(places.own.find((p) => p.href === "/conversations")?.count).toBe("direct");
  expect(places.rights.find((p) => p.href === "/moderation")?.count).toBe("moderation");
  expect(places.side.filter((p) => p.count !== undefined).map((p) => p.href)).toEqual([
    "/notifications",
    "/conversations",
  ]);
});

test("the timelines that GET /instance turns off are left out of the navigation", async () => {
  const off = await instanceOf({ timelines: { local: "off", federated: "everyone" } });
  expect(hrefs(placesFor(true, off, none).tabs)).toEqual(["/", "/public", "/notifications", "/search"]);
  const bothOff = await instanceOf({ timelines: { local: "off", federated: "off" } });
  expect(hrefs(placesFor(true, bothOff, none).tabs)).toEqual(["/", "/notifications", "/search"]);
});

test("a visitor has the timelines offered to everyone and Search, and no places of their own or of rights", async () => {
  const open = await instanceOf({ timelines: { local: "everyone", federated: "signed_in" } });
  const places = placesFor(false, open, staff);
  expect(hrefs(places.tabs)).toEqual(["/local", "/search"]);
  expect(places.own).toEqual([]);
  expect(places.rights).toEqual([]);
  // A timeline offered to signed-in people shows for one.
  expect(hrefs(placesFor(true, open, none).tabs)).toEqual(["/", "/local", "/public", "/notifications", "/search"]);
});

test("a one-person server has no Local or Federated, for its operator and for a visitor", async () => {
  // GET /instance turns both timelines off on a one-person server, whatever the settings store.
  const one = await instanceOf({ singleUser: true });
  expect(hrefs(placesFor(true, one, staff).tabs)).toEqual(["/", "/notifications", "/search"]);
  expect(hrefs(placesFor(false, one, none).tabs)).toEqual(["/search"]);
});

test("Moderation, Admin and Finance follow the reader's places, each for those with its rights", async () => {
  const i = await instanceOf();
  expect(hrefs(placesFor(true, i, staff).rights)).toEqual(["/moderation", "/admin", "/finance"]);
  expect(hrefs(placesFor(true, i, rightsOf(["manage_reports"])).rights)).toEqual(["/moderation"]);
  expect(hrefs(placesFor(true, i, rightsOf(["manage_settings"])).rights)).toEqual(["/admin"]);
  expect(hrefs(placesFor(true, i, rightsOf(["view_finance"])).rights)).toEqual(["/finance"]);
});

test("the rights come from the permissions' areas of ADR-0058 §4: moderation, server administration, finance", () => {
  const moderation = ["manage_reports", "manage_accounts", "manage_domains", "manage_invites", "view_audit"] as const;
  expect(Object.fromEntries(moderation.map((p) => [p, rightsOf([p])]))).toEqual(
    Object.fromEntries(moderation.map((p) => [p, { moderation: true, admin: false, finance: false }])),
  );
  const admin = ["manage_settings", "manage_rules", "manage_roles", "manage_jobs", "manage_announcements"] as const;
  expect(Object.fromEntries(admin.map((p) => [p, rightsOf([p])]))).toEqual(
    Object.fromEntries(admin.map((p) => [p, { moderation: false, admin: true, finance: false }])),
  );
  expect(rightsOf(["view_finance"])).toEqual({ moderation: false, admin: false, finance: true });
  expect(rightsOf(["manage_finance"]).finance).toBe(true);
  // Member permissions open no staff place.
  expect(rightsOf(["create_custom_emojis", "start_voice_rooms", "create_invites"])).toEqual(none);
  expect(none).toEqual({ moderation: false, admin: false, finance: false });
});

test("the menu holds Announcements, the voice rooms while they are on, the events, and the funding while it shows", async () => {
  expect(hrefs(aroundPlacesFor(true, await instanceOf(), "/login"))).toEqual([
    "/announcements",
    "/voice",
    "/events",
    "/about#funding",
  ]);
  expect(hrefs(aroundPlacesFor(true, await instanceOf({ voiceOff: true }), "/login"))).not.toContain("/voice");
  const noMeter = await instanceOf();
  Reflect.deleteProperty(noMeter, "funding_meter");
  expect(hrefs(aroundPlacesFor(true, noMeter, "/login"))).not.toContain("/about#funding");
});

test("a visitor's menu leads to the sign-in, and to nothing that needs a session", async () => {
  const around = hrefs(aroundPlacesFor(false, await instanceOf(), "/login?next=%2F"));
  expect(around).toContain("/login?next=%2F");
  expect(around).not.toContain("/announcements");
  expect(around).not.toContain("/voice");
});

test("moving between any two places of the navigation is a navigation between its items", async () => {
  const places = placesFor(true, await instanceOf(), staff);
  for (const href of hrefs([...places.tabs, ...places.own, ...places.rights])) expect(navigationPaths).toContain(href);
  expect(navigationPaths).not.toContain("/@yuki");
});

test("Settings is where the reader is at /settings and at every address under it, and Home at the root alone", async () => {
  const { side } = placesFor(true, await instanceOf(), none);
  const place = (href: string) => {
    const found = side.find((p) => p.href === href);
    if (found === undefined) throw new Error(`test: the navigation has no place at ${href}`);
    return found;
  };

  for (const at of ["/settings", "/settings/display", "/settings/nothing"])
    expect([at, isPlaceAt(place("/settings"), at)]).toEqual([at, true]);
  for (const at of ["/", "/settingsx", "/lists", "/bookmarks"])
    expect([at, isPlaceAt(place("/settings"), at)]).toEqual([at, false]);
  // Every address is under the root: Home is not marked on them.
  expect(isPlaceAt(place("/"), "/")).toBe(true);
  for (const at of ["/settings", "/settings/display", "/notifications"])
    expect([at, isPlaceAt(place("/"), at)]).toEqual([at, false]);
});

test("the new post floats on the timelines and profiles alone", () => {
  for (const at of ["/", "/local", "/public", "/profile", "/@yuki", "/@sam@remote.test"])
    expect(composesAt(at)).toBe(true);
  for (const at of ["/notifications", "/search", "/settings", "/bookmarks", "/announcements"])
    expect(composesAt(at)).toBe(false);
});
