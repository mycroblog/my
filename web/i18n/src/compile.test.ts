// SPDX-License-Identifier: AGPL-3.0-only
// compile.ts run as `scripts/task.sh i18n:compile` runs it, on a temporary repository (I18N_TOOL_ROOT), and its module
// read with Lingui.

import { setupI18n, type Messages } from "@lingui/core";
import { afterAll, beforeEach, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";

const roots: string[] = [];
let root = "";

const writeCatalog = (path: string, messages: Record<string, string>) => {
  mkdirSync(dirname(join(root, path)), { recursive: true });
  const entries = Object.fromEntries(Object.entries(messages).map(([k, m]) => [k, { defaultMessage: m }]));
  writeFileSync(join(root, path), JSON.stringify(entries));
};

const compile = () => {
  const p = Bun.spawnSync(["bun", "src/compile.ts"], {
    cwd: join(import.meta.dir, ".."),
    env: { ...process.env, I18N_TOOL_ROOT: root },
    stdout: "pipe",
    stderr: "pipe",
  });
  return { code: p.exitCode, out: p.stdout.toString() + p.stderr.toString() };
};

beforeEach(() => {
  root = mkdtempSync(join(tmpdir(), "my-i18n-compile-"));
  roots.push(root);
  mkdirSync(join(root, "locales"));
  writeCatalog("web/parts/locales/en/post.json", {
    "post.greeting": "Hello, {name}",
    "post.replies": "{count, plural, one {# reply} other {# replies}}",
    "post.more": "More",
  });
  writeCatalog("web/parts/locales/ja/post.json", {
    "post.greeting": "{name}さん、こんにちは",
    "post.replies": "{count, plural, other {#件の返信}}",
  });
});

afterAll(() => {
  for (const dir of roots) rmSync(dir, { recursive: true, force: true });
});

test("each locale's messages compile to a module Lingui formats, with en for a key a locale lacks", async () => {
  const r = compile();
  expect(r.out).toContain("wrote web/parts/src/messages.gen.ts");
  expect(r.code).toBe(0);
  const { messages } = (await import(join(root, "web/parts/src/messages.gen.ts"))) as {
    messages: Record<string, Messages>;
  };
  expect(Object.keys(messages)).toEqual(["ar-XB", "en", "en-XA", "ja"]);
  const t = (locale: string, id: string, values?: Record<string, unknown>) =>
    setupI18n({ locale, messages: { [locale]: messages[locale] ?? {} } })._(id, values);
  expect(t("en", "post.greeting", { name: "Ada" })).toBe("Hello, Ada");
  expect(t("en", "post.replies", { count: 1 })).toBe("1 reply");
  expect(t("en", "post.replies", { count: 2 })).toBe("2 replies");
  expect(t("ja", "post.greeting", { name: "Ada" })).toBe("Adaさん、こんにちは");
  expect(t("ja", "post.replies", { count: 3 })).toBe("3件の返信");
  expect(t("ja", "post.more")).toBe("More");
  const pseudo = t("en-XA", "post.greeting", { name: "Ada" });
  expect(pseudo).toContain("Ada");
  expect(pseudo).not.toBe("Hello, Ada");
  expect(t("ar-XB", "post.more")).toStartWith("‮");
});

test("it names the message Lingui cannot compile", () => {
  writeCatalog("web/parts/locales/ja/post.json", { "post.more": "{count, plural, other {#件}" });
  const r = compile();
  expect(r.code).not.toBe(0);
  expect(r.out).toContain("web/parts/locales/ja post.more");
});
