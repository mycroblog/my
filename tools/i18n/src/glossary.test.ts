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
  expect(glossaryFindings(glossary, "zh-CN", "请先登陆")).toHaveLength(1);
  expect(glossaryFindings(glossary, "zh-CN", "请先登录")).toHaveLength(0);
  expect(glossaryFindings(glossary, "es", "Haz login para continuar")).toHaveLength(1);
  expect(glossaryFindings(glossary, "es", "Inicia sesión para continuar")).toHaveLength(0);
  expect(glossaryFindings(glossary, "es", "Login providers")).toHaveLength(0);
});

test("a locale only matches its own entries, and en and ja have none", () => {
  expect(glossaryFindings(glossary, "zh-TW", "请先登陆")).toHaveLength(0);
  expect(glossaryFindings(glossary, "it", "Log in")).toHaveLength(0);
  expect(glossary.en).toBeUndefined();
  expect(glossary.ja).toBeUndefined();
});
