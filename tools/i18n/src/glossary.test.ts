// SPDX-License-Identifier: AGPL-3.0-only
import { expect, test } from "bun:test";
import { glossaryFindings, loadGlossary } from "./glossary";

const glossary = loadGlossary();

test("glossary entries compile as regular expressions", () => {
  for (const terms of Object.values(glossary)) {
    for (const term of terms) for (const avoid of term.avoid) expect(() => new RegExp(avoid, "mu")).not.toThrow();
  }
});

test("a banned variant is a finding, the preferred term is not", () => {
  expect(glossaryFindings(glossary, "ja", "サインインしてください")).toHaveLength(1);
  expect(glossaryFindings(glossary, "ja", "ログインしてください")).toHaveLength(0);
  expect(glossaryFindings(glossary, "ja", "もう一度お試しください")).toHaveLength(1);
  expect(glossaryFindings(glossary, "ja", "アカウントを作る")).toHaveLength(1);
  expect(glossaryFindings(glossary, "en", "Log in to continue")).toHaveLength(1);
  expect(glossaryFindings(glossary, "en", "Stops it logging in.")).toHaveLength(1);
  expect(glossaryFindings(glossary, "en", "Sign in to continue")).toHaveLength(0);
  expect(glossaryFindings(glossary, "en", "Login providers")).toHaveLength(0);
  expect(glossaryFindings(glossary, "en", "Read the audit catalog index")).toHaveLength(0);
});

test("a locale only matches its own entries", () => {
  expect(glossaryFindings(glossary, "en", "サインイン")).toHaveLength(0);
  expect(glossaryFindings(glossary, "fr", "Log in")).toHaveLength(0);
});
