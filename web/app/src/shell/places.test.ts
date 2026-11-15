// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { rightsOf, type Rights } from "@my/app-data";
import { instance } from "../test/server";
import { aroundPlacesFor, composesAt, navigationPaths, placesFor, type Place } from "./places";

const hrefs = (places: readonly Place[]) => places.map((p) => p.href);
const none: Rights = rightsOf([]);
const staff: Rights = rightsOf(["manage_reports", "manage_settings", "manage_finance"]);

test("a signed-in reader's tab bar is Home, Local, Federated, Notifications, Search, then Profile, Bookmarks, Settings", () => {
  const places = placesFor(true, instance(), none);
  expect(hrefs(places.tabs)).toEqual(["/", "/local", "/public", "/notifications", "/search"]);
  expect(hrefs(places.own)).toEqual(["/profile", "/bookmarks", "/settings"]);
  expect(places.rights).toEqual([]);
});

test("Notifications carries the unread count and Moderation the unhandled count", () => {
  const places = placesFor(true, instance(), staff);
  expect(places.tabs.find((p) => p.href === "/notifications")?.count).toBe("notifications");
  expect(places.rights.find((p) => p.href === "/moderation")?.count).toBe("moderation");
  expect([...places.tabs, ...places.own].filter((p) => p.count !== undefined).map((p) => p.href)).toEqual([
    "/notifications",
  ]);
});

test("the timelines that GET /instance turns off are left out of the navigation", () => {
  const off = instance({ timelines: { local: "off", federated: "everyone" } });
  expect(hrefs(placesFor(true, off, none).tabs)).toEqual(["/", "/public", "/notifications", "/search"]);
  const bothOff = instance({ timelines: { local: "off", federated: "off" } });
  expect(hrefs(placesFor(true, bothOff, none).tabs)).toEqual(["/", "/notifications", "/search"]);
});

test("a visitor has the timelines offered to everyone and Search, and no places of their own or of rights", () => {
  const open = instance({ timelines: { local: "everyone", federated: "signed_in" } });
  const places = placesFor(false, open, staff);
  expect(hrefs(places.tabs)).toEqual(["/local", "/search"]);
  expect(places.own).toEqual([]);
  expect(places.rights).toEqual([]);
  // A timeline offered to signed-in people shows for one.
  expect(hrefs(placesFor(true, open, none).tabs)).toEqual(["/", "/local", "/public", "/notifications", "/search"]);
});

test("a one-person server has no Local or Federated, for its operator and for a visitor", () => {
  // GET /instance turns both timelines off on a one-person server, whatever the settings store.
  const one = instance({ single_user: true, timelines: { local: "off", federated: "off" } });
  expect(hrefs(placesFor(true, one, staff).tabs)).toEqual(["/", "/notifications", "/search"]);
  expect(hrefs(placesFor(false, one, none).tabs)).toEqual(["/search"]);
});

test("Moderation, Admin and Finance follow the reader's places, each for those with its rights", () => {
  const i = instance();
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

test("the menu holds Announcements, the voice rooms while they are on, the events, and the funding while it shows", () => {
  expect(hrefs(aroundPlacesFor(true, instance(), "/login"))).toEqual([
    "/announcements",
    "/voice",
    "/events",
    "/about#funding",
  ]);
  expect(hrefs(aroundPlacesFor(true, instance({ voice_enabled: false }), "/login"))).not.toContain("/voice");
  const noMeter = instance();
  Reflect.deleteProperty(noMeter, "funding_meter");
  expect(hrefs(aroundPlacesFor(true, noMeter, "/login"))).not.toContain("/about#funding");
});

test("a visitor's menu leads to the sign-in, and to nothing that needs a session", () => {
  const around = hrefs(aroundPlacesFor(false, instance(), "/login?next=%2F"));
  expect(around).toContain("/login?next=%2F");
  expect(around).not.toContain("/announcements");
  expect(around).not.toContain("/voice");
});

test("moving between any two places of the navigation is a navigation between its items", () => {
  const places = placesFor(true, instance(), staff);
  for (const href of hrefs([...places.tabs, ...places.own, ...places.rights])) expect(navigationPaths).toContain(href);
  expect(navigationPaths).not.toContain("/@yuki");
});

test("the new post floats on the timelines and profiles alone", () => {
  for (const at of ["/", "/local", "/public", "/profile", "/@yuki", "/@sam@remote.test"])
    expect(composesAt(at)).toBe(true);
  for (const at of ["/notifications", "/search", "/settings", "/bookmarks", "/announcements"])
    expect(composesAt(at)).toBe(false);
});
