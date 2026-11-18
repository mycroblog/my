// SPDX-License-Identifier: AGPL-3.0-only

// Where a restriction says to appeal (#3115): the sentences the server answers an account restricted in part with,
// and the one the settings' Invites say in its place, name the place of the settings where the action stands and can
// be appealed, by the names the settings give that place. The server's catalog and the client's are two files a
// change may move apart, so the names are held together here, in each language. That each sentence says a next step, and that none of them stays on the copy lint's reviewed list
// of errors without one (tools/i18n/no-step.json), is the lint's own rule: it refuses a listed error that says its
// step, and an error that says none and is not listed (tools/i18n, copy.test.ts).

import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

type Catalog = Readonly<Record<string, { readonly defaultMessage: string } | undefined>>;

const root = join(import.meta.dir, "../../../..");
const catalog = (path: string): Catalog => JSON.parse(readFileSync(join(root, path), "utf8")) as Catalog;

/** The message of `key` in `messages`. */
function messageOf(messages: Catalog, key: string): string {
  const message = messages[key]?.defaultMessage;
  if (message === undefined) throw new Error(`test: the catalog has no message ${key}`);
  return message;
}

/** The text of the branch `name` of the select that the message of `key` is. */
function branchOf(messages: Catalog, key: string, name: string): string {
  const branch = new RegExp(`\\b${name} \\{([^{}]*)\\}`).exec(messageOf(messages, key))?.[1];
  if (branch === undefined) throw new Error(`test: the message ${key} has no branch ${name}`);
  return branch;
}

// How each language quotes the name of a place, and how it says that the reader can appeal.
const languages = [
  ["en", (name: string) => `“${name}”`, /\bappeal\b/],
  ["ja", (name: string) => `「${name}」`, /異議申し立て/],
] as const;

for (const [language, quoted, appeal] of languages) {
  const server = catalog(`locales/${language}/server/errors.json`);
  const settings = catalog(`web/app-features/locales/${language}/settings.json`);
  const sentences = [
    ["a post an account kept from posting sends", messageOf(server, "errors.status.posting_restricted")],
    ["what an account kept from interacting does", messageOf(server, "errors.interaction_restricted")],
    [
      "a supporter plan a restricted account asks for",
      branchOf(server, "errors.supporters.ineligible_detail", "restricted"),
    ],
    [
      "an invite an account kept from creating invites asks for",
      branchOf(server, "errors.invites.not_allowed_detail", "restricted"),
    ],
    // The settings say the same to such an account before it asks, in a sentence of the client's own catalog.
    [
      "an invite, as the settings' Invites say it beforehand,",
      branchOf(settings, "settings.invites.cannot.reason", "restricted"),
    ],
  ] as const;

  for (const [what, sentence] of sentences)
    test(`${language}: the refusal of ${what} says that the reader can appeal, and where: the screen and the entry by the settings' own names`, () => {
      expect(sentence).toMatch(appeal);
      expect(sentence).toContain(quoted(messageOf(settings, "settings.account.status")));
      expect(sentence).toContain(quoted(messageOf(settings, "settings.entry.account")));
    });
}
