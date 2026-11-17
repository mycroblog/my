// SPDX-License-Identifier: AGPL-3.0-only
// The tests of the sidebar that lists only the section of the current page (src/section-sidebar.ts), on trees shaped
// like the one src/sidebar.ts and the API reference's group give a page.
import assert from "node:assert/strict";
import { test } from "node:test";
import type { StarlightRouteData } from "@astrojs/starlight/route-data";
import { apiReferenceLabel } from "./api-sidebar";
import { currentSectionOnly } from "./section-sidebar";

type Entry = StarlightRouteData["sidebar"][number];

function link(label: string, href: string, current: boolean): Entry {
  return { type: "link", label, href, isCurrent: current, badge: undefined, attrs: {} };
}

function group(label: string, entries: Entry[]): Entry {
  return { type: "group", label, entries, collapsed: false, badge: undefined };
}

/** The sidebar of a locale: `labels` are the four section headings; `current` is the href of the current page. */
function sidebar(prefix: string, labels: [string, string, string, string], current: string | undefined): Entry[] {
  const at = (href: string, label: string) => link(label, `${prefix}${href}`, `${prefix}${href}` === current);
  const [using, operation, developers, plugins] = labels;
  return [
    group(using, [group("Posting", [at("/guide/a/", "A"), at("/guide/b/", "B")]), at("/guide/c/", "C")]),
    group(operation, [at("/operations/running/", "Running"), at("/operations/backup/", "Backup")]),
    group(developers, [at("/developers/client-api/", "Client API"), at("/developers/webhooks/", "Webhooks")]),
    group(apiReferenceLabel, [
      group("Accounts", [
        at("/api/accounts/", "Overview"),
        at("/api/accounts/get/", "Get"),
        at("/api/accounts/put/", "Put"),
      ]),
      group("Posts", [at("/api/posts/", "Overview"), at("/api/posts/get/", "Get")]),
    ]),
    group(plugins, [at("/plugins/intro/", "Intro")]),
  ];
}

const english: [string, string, string, string] = ["Using", "Operation", "Developers", "Plugins"];

/** Each entry as `label -> href` for a link, or as a list of its label and its entries for a group. */
function shape(entries: Entry[]): unknown[] {
  return entries.map((entry) =>
    entry.type === "group" ? [entry.label, ...shape(entry.entries)] : `${entry.label} -> ${entry.href}`,
  );
}

void test("a page of a section lists that section and one link per other section", () => {
  const out = currentSectionOnly(sidebar("/en", english, "/en/operations/backup/"));
  assert.deepEqual(shape(out), [
    "Using -> /en/guide/a/",
    ["Operation", "Running -> /en/operations/running/", "Backup -> /en/operations/backup/"],
    "Developers -> /en/developers/client-api/",
    "Plugins -> /en/plugins/intro/",
  ]);
  const section = out[1];
  assert.ok(section?.type === "group");
  const backup = section.entries[1];
  assert.ok(backup?.type === "link");
  assert.equal(backup.isCurrent, true);
});

void test("an operation page keeps Developers and the reference, and cuts the other sections to links", () => {
  const out = currentSectionOnly(sidebar("/en", english, "/en/api/accounts/get/"));
  assert.deepEqual(shape(out), [
    "Using -> /en/guide/a/",
    "Operation -> /en/operations/running/",
    ["Developers", "Client API -> /en/developers/client-api/", "Webhooks -> /en/developers/webhooks/"],
    [
      apiReferenceLabel,
      ["Accounts", "Overview -> /en/api/accounts/", "Get -> /en/api/accounts/get/", "Put -> /en/api/accounts/put/"],
      ["Posts", "Overview -> /en/api/posts/", "Get -> /en/api/posts/get/"],
    ],
    "Plugins -> /en/plugins/intro/",
  ]);
});

void test("a page of Developers keeps the reference's group", () => {
  const out = currentSectionOnly(sidebar("/en", english, "/en/developers/webhooks/"));
  assert.equal(out.length, 5);
  assert.equal(out[3]?.label, apiReferenceLabel);
});

void test("a page of another section drops the reference with Developers", () => {
  const out = currentSectionOnly(sidebar("/en", english, "/en/guide/b/"));
  assert.deepEqual(
    out.map((entry) => entry.label),
    ["Using", "Operation", "Developers", "Plugins"],
  );
  assert.equal(out[0]?.type, "group");
  assert.equal(out[2]?.type, "link");
});

void test("the landing page shows every section as one link", () => {
  const out = currentSectionOnly(sidebar("/en", english, undefined));
  assert.deepEqual(shape(out), [
    "Using -> /en/guide/a/",
    "Operation -> /en/operations/running/",
    "Developers -> /en/developers/client-api/",
    "Plugins -> /en/plugins/intro/",
  ]);
});

void test("a fallback page of a locale keeps the translated labels", () => {
  // A page the locale has not translated is the English document, so its link keeps the English title.
  const out = currentSectionOnly(sidebar("/ja", ["使う", "運用", "開発者", "プラグイン"], "/ja/operations/backup/"));
  assert.deepEqual(shape(out), [
    "使う -> /ja/guide/a/",
    ["運用", "Running -> /ja/operations/running/", "Backup -> /ja/operations/backup/"],
    "開発者 -> /ja/developers/client-api/",
    "プラグイン -> /ja/plugins/intro/",
  ]);
});
