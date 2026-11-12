// SPDX-License-Identifier: AGPL-3.0-only
// The commands of cli.ts, each run as `scripts/i18n.sh` runs it, on a temporary repository (I18N_TOOL_ROOT).
import { afterAll, beforeEach, expect, test } from "bun:test";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";

const tool = join(import.meta.dir, "..");
const roots: string[] = [];
let root = "";

type Entries = Record<string, { defaultMessage: string; description?: string }>;
const entry = (defaultMessage: string, description = "Where it appears.") => ({ defaultMessage, description });
const write = (path: string, content: string) => {
  mkdirSync(dirname(join(root, path)), { recursive: true });
  writeFileSync(join(root, path), content);
};
const writeCatalog = (path: string, entries: Entries) => write(path, `${JSON.stringify(entries, null, 2)}\n`);
const read = (path: string) => readFileSync(join(root, path), "utf8");
const catalog = (path: string): Entries => JSON.parse(read(path));
const keys = (path: string) => Object.keys(catalog(path));
const message = (path: string, key: string) => catalog(path)[key]?.defaultMessage;

function exec(command: string[], cwd: string) {
  const p = Bun.spawnSync(command, {
    cwd,
    env: { ...process.env, I18N_TOOL_ROOT: root },
    stdout: "pipe",
    stderr: "pipe",
  });
  return { code: p.exitCode, out: p.stdout.toString() + p.stderr.toString() };
}
const run = (...args: string[]) => exec(["bun", "src/cli.ts", ...args], tool);
const git = (...args: string[]) => exec(["git", "-c", "user.name=t", "-c", "user.email=t@example.com", ...args], root);

const homeGo = (calls: string) =>
  `package web\n\nfunc home(l *i18n.Localizer, name string) []string {\n\treturn []string{${calls}}\n}\n`;

beforeEach(() => {
  root = mkdtempSync(join(tmpdir(), "my-i18n-"));
  roots.push(root);
  writeCatalog("locales/en/web/home.json", {
    "home.title": entry("Home"),
    "home.empty.title": entry("Nothing here yet"),
    "home.empty.action": entry("Find {count, plural, one {# person} other {# people}} to follow"),
    "home.footer": entry("About {name}"),
  });
  writeCatalog("locales/ja/web/home.json", {
    "home.title": entry("ホーム"),
    "home.empty.title": entry("まだ何もありません"),
    "home.empty.action": entry("フォローする人を{count, plural, other {#人}}探す"),
    "home.footer": entry("{name}について"),
  });
  writeCatalog("locales/de/web/home.json", {
    "home.title": entry("Startseite"),
    "home.footer": entry("Über {name}"),
  });
  // The parity fixtures format the server messages they name in every golden locale.
  const server: Record<string, [string, string]> = {
    en: ["Not found", "{count, plural, one {# error} other {# errors}} in {name}"],
    ja: ["見つかりません", "{name}に{count, plural, other {#件}}のエラー"],
    de: ["Nicht gefunden", "{count, plural, one {# Fehler} other {# Fehler}} in {name}"],
    es: ["No encontrado", "{count, plural, one {# error} other {# errores}} en {name}"],
    fr: ["Introuvable", "{count, plural, one {# erreur} other {# erreurs}} dans {name}"],
    "pt-BR": ["Não encontrado", "{count, plural, one {# erro} other {# erros}} em {name}"],
    "zh-CN": ["未找到", "{name}中有{count, plural, other {#个}}错误"],
    "zh-TW": ["找不到", "{name}中有{count, plural, other {#個}}錯誤"],
  };
  for (const [locale, [notFound, count]] of Object.entries(server)) {
    writeCatalog(`locales/${locale}/server/errors.json`, {
      "errors.not_found": entry(notFound),
      "errors.count": entry(count),
    });
  }
  write(
    "internal/i18n/testdata/cases.json",
    `${JSON.stringify({ catalog: [{ key: "server:errors.not_found", args: {} }], syntax: [] })}\n`,
  );
  write(
    "internal/web/home.go",
    homeGo("l.WebHomeTitle(), l.WebHomeEmptyTitle(), l.WebHomeEmptyAction(1), l.WebHomeFooter(name)"),
  );
  write(
    "internal/web/handler.go",
    'package web\n\nfunc render(l *i18n.Localizer, cfg Config) string {\n\t_ = cfg.ServerErrorsNotFound\n\treturn l.ServerErrorsNotFound() + l.ServerErrorsCount(i18n.ServerErrorsCountArgs{Count: 1, Name: "x"})\n}\n',
  );
  write("internal/web/page.html", "<p>{{.L.ServerErrorsNotFound}}</p>\n");
  write("docs/note.md", "The empty state shows `web:home.empty.title`.\n");
  expect(exec(["bun", "src/gen.ts"], tool).code).toBe(0);
  git("init", "-q");
  git("add", "-A");
  expect(git("commit", "-q", "-m", "fixture").code).toBe(0);
});

afterAll(() => {
  for (const dir of roots) rmSync(dir, { recursive: true, force: true });
});

test("add writes en and ja next to the key's siblings and regenerates", () => {
  const r = run(
    "add",
    "web:home.empty.hint",
    "--en",
    "Follow someone",
    "--ja",
    "誰かをフォローします",
    "--desc",
    "Hint.",
  );
  expect(r.out).toContain("added web:home.empty.hint; use (*i18n.Localizer).WebHomeEmptyHint");
  expect(r.code).toBe(0);
  const order = ["home.title", "home.empty.title", "home.empty.action", "home.empty.hint", "home.footer"];
  expect(keys("locales/en/web/home.json")).toEqual(order);
  expect(keys("locales/ja/web/home.json")).toEqual(order);
  expect(catalog("locales/ja/web/home.json")["home.empty.hint"]).toEqual(entry("誰かをフォローします", "Hint."));
  expect(keys("locales/de/web/home.json")).toEqual(["home.title", "home.footer"]);
  expect(read("internal/i18n/messages_gen.go")).toContain("func (l *Localizer) WebHomeEmptyHint() string");
  expect(keys("locales/en-XA/web/home.json")).toContain("home.empty.hint");
  expect(run("check").code).toBe(0);
});

test("add puts a key after --after and creates the file of a new area", () => {
  expect(
    run("add", "web:home.intro", "--en", "Welcome", "--ja", "ようこそ", "--desc", "d", "--after", "home.title").code,
  ).toBe(0);
  expect(keys("locales/ja/web/home.json").slice(0, 2)).toEqual(["home.title", "home.intro"]);
  expect(run("add", "web:admin.report.title", "--en", "Report", "--ja", "通報", "--desc", "d").code).toBe(0);
  expect(keys("locales/en/web/admin/report.json")).toEqual(["admin.report.title"]);
  expect(keys("locales/ja/web/admin/report.json")).toEqual(["admin.report.title"]);
  const elsewhere = run(
    "add",
    "web:lists.title",
    "--en",
    "Lists",
    "--ja",
    "リスト",
    "--desc",
    "d",
    "--after",
    "home.title",
  );
  expect(elsewhere.code).toBe(1);
  expect(elsewhere.out).toContain("not a key of locales/en/web/lists.json");
  expect(run("add", "server:errors.gone", "--en", "Gone", "--ja", "ありません", "--desc", "d").out).toContain(
    "(*i18n.Localizer).ServerErrorsGone",
  );
  expect(read("internal/i18n/messages_gen.go")).toContain("func (l *Localizer) ServerErrorsGone() string");
});

test("add refuses a message that breaks a rule, and names the rule", () => {
  const before = read("locales/en/web/home.json");
  const refused = (expected: string, ...args: string[]) => {
    const r = run("add", ...args);
    expect(r.out).toContain(expected);
    expect(r.code).toBe(1);
  };
  refused("--ja missing", "web:home.x", "--en", "X", "--desc", "d");
  refused("--desc missing", "web:home.x", "--en", "X", "--ja", "X");
  refused("web:home.title exists", "web:home.title", "--en", "X", "--ja", "X", "--desc", "d");
  refused("the namespaces are", "app:home.x", "--en", "X", "--ja", "X", "--desc", "d");
  refused("a key is dot-separated segments", "web:Home.X", "--en", "X", "--ja", "X", "--desc", "d");
  refused("--ja: unknown argument {nam}", "web:home.x", "--en", "Hi {name}", "--ja", "{nam}さん", "--desc", "d");
  refused("--ja: drops argument {name}", "web:home.x", "--en", "Hi {name}", "--ja", "こんにちは", "--desc", "d");
  refused(
    "--ja: {count} is string, the source has number",
    "web:home.x",
    "--en",
    "{count, plural, one {# post} other {# posts}}",
    "--ja",
    "{count}件",
    "--desc",
    "d",
    "--drop-args",
  );
  refused("--en: ICU syntax", "web:home.x", "--en", "Hi {name", "--ja", "{name}さん", "--desc", "d");
  refused(
    '--ja: cardinal category "one" does not exist in ja',
    "web:home.x",
    "--en",
    "{n, plural, one {#} other {#}}",
    "--ja",
    "{n, plural, one {#} other {#}}",
    "--desc",
    "d",
  );
  refused("--en: no exclamation marks", "web:home.x", "--en", "Done!", "--ja", "完了", "--desc", "d");
  refused("--ja: use the ASCII colon", "web:home.x", "--en", "Note", "--ja", "注意：これ", "--desc", "d");
  refused('--ja: use "、" instead of an ASCII comma', "web:home.x", "--en", "Note", "--ja", "これ,あれ", "--desc", "d");
  refused('use "ログイン"', "web:home.x", "--en", "Sign in", "--ja", "サインイン", "--desc", "d");
  for (const ns of ["server", "web"]) {
    refused(
      "--en: tag <b> is outside the Go/TS shared subset",
      `${ns}:home.x`,
      "--en",
      "<b>No</b>",
      "--ja",
      "<b>いいえ</b>",
      "--desc",
      "d",
    );
  }
  refused(
    "both map to the accessor WebHomeEmptyTitle",
    "web:home.empty_title",
    "--en",
    "X",
    "--ja",
    "X",
    "--desc",
    "d",
  );
  expect(read("locales/en/web/home.json")).toBe(before);
  expect(run("add", "web:home.y", "--en", "Hi {name}", "--ja", "こんにちは", "--desc", "d", "--drop-args").code).toBe(
    0,
  );
});

test("set refuses a new en text without ja, and drops the machine translations of the old text", () => {
  const alone = run("set", "web:home.footer", "--en", "More about {name}");
  expect(alone.code).toBe(1);
  expect(alone.out).toContain("--ja missing: ja follows en in the same change");
  expect(alone.out).toContain('"{name}について"');
  const both = run("set", "web:home.footer", "--en", "More about {name}", "--ja", "{name}の詳細");
  expect(both.code).toBe(0);
  expect(both.out).toContain("dropped the de translations of web:home.footer");
  expect(message("locales/en/web/home.json", "home.footer")).toBe("More about {name}");
  expect(message("locales/ja/web/home.json", "home.footer")).toBe("{name}の詳細");
  expect(keys("locales/de/web/home.json")).toEqual(["home.title"]);
  expect(run("check").code).toBe(0);
});

test("set keeps the translations on request, and changes ja, another locale, or the description alone", () => {
  expect(
    run("set", "web:home.footer", "--en", "About {name}.", "--ja", "{name}について", "--keep-translations").code,
  ).toBe(0);
  expect(message("locales/de/web/home.json", "home.footer")).toBe("Über {name}");
  const broken = run("set", "web:home.footer", "--en", "About {site}", "--ja", "{site}について", "--keep-translations");
  expect(broken.code).toBe(1);
  expect(broken.out).toContain("--de: unknown argument {name}");

  expect(run("set", "web:home.title", "--ja", "ホーム画面").code).toBe(0);
  expect(message("locales/ja/web/home.json", "home.title")).toBe("ホーム画面");
  expect(message("locales/de/web/home.json", "home.title")).toBe("Startseite");
  expect(run("set", "web:home.title", "--ja", "ホーム！").out).toContain("--ja: no exclamation marks");

  expect(run("set", "web:home.empty.title", "--de", "Noch nichts da").code).toBe(0);
  expect(keys("locales/de/web/home.json")).toEqual(["home.title", "home.empty.title", "home.footer"]);
  expect(run("set", "web:home.empty.title", "--de", 'Noch "nichts" da').out).toContain("--de: use");

  expect(run("set", "web:home.title", "--desc", "Title of the home timeline.").code).toBe(0);
  for (const locale of ["en", "ja", "de"]) {
    expect(catalog(`locales/${locale}/web/home.json`)["home.title"]?.description).toBe("Title of the home timeline.");
  }
  expect(run("set", "web:home.nope", "--ja", "x").out).toContain("web:home.nope is not in en");
});

test("mv renames a key in place in every locale and rewrites the references", () => {
  const r = run("mv", "web:home.footer", "home.about");
  expect(r.out).toContain("renamed web:home.footer -> web:home.about");
  expect(r.out).toContain("rewrote internal/web/home.go");
  expect(r.code).toBe(0);
  const order = ["home.title", "home.empty.title", "home.empty.action", "home.about"];
  expect(keys("locales/en/web/home.json")).toEqual(order);
  expect(keys("locales/ja/web/home.json")).toEqual(order);
  expect(keys("locales/de/web/home.json")).toEqual(["home.title", "home.about"]);
  expect(read("internal/web/home.go")).toContain("l.WebHomeAbout(name)");
  expect(run("check").code).toBe(0);
});

test("mv moves a key to the file of its new area and removes an emptied file", () => {
  expect(run("mv", "web:home.footer", "shell.footer").code).toBe(0);
  for (const locale of ["en", "ja", "de"]) {
    expect(keys(`locales/${locale}/web/shell.json`)).toEqual(["shell.footer"]);
    expect(keys(`locales/${locale}/web/home.json`)).not.toContain("home.footer");
  }
  expect(run("mv", "web:home.title", "admin.home.title").code).toBe(0);
  expect(keys("locales/ja/web/admin/home.json")).toEqual(["admin.home.title"]);
  expect(existsSync(join(root, "locales/de/web/home.json"))).toBe(false);
  expect(read("internal/web/home.go")).toContain("l.WebAdminHomeTitle(), l.WebHomeEmptyTitle()");
  expect(run("check").code).toBe(0);
});

test("mv rewrites the Go accessors, the argument struct, the templates and the fixtures, not a field of the same name", () => {
  expect(run("mv", "server:errors.not_found", "errors.missing").code).toBe(0);
  expect(run("mv", "server:errors.count", "problems.total").code).toBe(0);
  const go = read("internal/web/handler.go");
  expect(go).toContain("l.ServerErrorsMissing() + l.ServerProblemsTotal(i18n.ServerProblemsTotalArgs{Count: 1");
  expect(go).toContain("cfg.ServerErrorsNotFound\n");
  expect(read("internal/web/page.html")).toContain("{{.L.ServerErrorsMissing}}");
  expect(read("internal/i18n/testdata/cases.json")).toContain('"server:errors.missing"');
  expect(keys("locales/zh-TW/server/problems.json")).toEqual(["problems.total"]);
  expect(read("internal/i18n/messages_gen.go")).toContain(
    "func (l *Localizer) ServerProblemsTotal(args ServerProblemsTotalArgs)",
  );
  expect(run("check").code).toBe(0);
});

test("mv renames every key under a prefix, and lists what it could not rewrite", () => {
  const r = run("mv", "web:home.empty", "home.blank");
  expect(r.code).toBe(1);
  expect(r.out).toContain("renamed web:home.empty.title -> web:home.blank.title");
  expect(r.out).toContain("docs/note.md:1:The empty state shows `web:home.empty.title`.");
  expect(keys("locales/en/web/home.json")).toEqual([
    "home.title",
    "home.blank.title",
    "home.blank.action",
    "home.footer",
  ]);
  expect(read("internal/web/home.go")).toContain("l.WebHomeBlankAction(1)");

  expect(run("mv", "web:home.title", "home.footer").out).toContain("web:home.footer exists");
  expect(run("mv", "web:home.nope", "home.x").out).toContain("neither a key nor a prefix");
  expect(run("mv", "web:home.title", "server:home.title").out).toContain("renamed inside its namespace");
});

test("rm refuses a referenced key, and removes an unreferenced one from every locale", () => {
  const refused = run("rm", "web:home.footer", "server:errors.not_found");
  expect(refused.code).toBe(1);
  expect(refused.out).toContain("web:home.footer is still referenced");
  expect(refused.out).toContain("internal/web/home.go:4: return []string{l.WebHomeTitle()");
  expect(refused.out).toContain("internal/web/handler.go:5: return l.ServerErrorsNotFound()");
  expect(refused.out).toContain("internal/web/page.html:1:");
  expect(refused.out).toContain("internal/i18n/testdata/cases.json:1:");
  expect(keys("locales/de/web/home.json")).toContain("home.footer");

  write("internal/web/home.go", homeGo("l.WebHomeTitle()"));
  const removed = run("rm", "web:home.footer", "web:home.empty.title", "web:home.empty.action");
  expect(removed.out).toContain("removed web:home.footer from en, de, ja");
  expect(removed.code).toBe(0);
  for (const locale of ["en", "ja", "de"]) expect(keys(`locales/${locale}/web/home.json`)).toEqual(["home.title"]);
  expect(run("rm", "web:home.nope").out).toContain("web:home.nope is in no locale");
  expect(run("check").code).toBe(0);
});

test("show prints a key or the keys under a prefix, find looks a text up", () => {
  const one = run("show", "web:home.footer", "--refs");
  expect(one.code).toBe(0);
  expect(one.out).toContain("web:home.footer  (locales/en/web/home.json)");
  expect(one.out).toContain("en    About {name}\n");
  expect(one.out).toContain("ja    {name}について\n");
  expect(one.out).toContain("desc  Where it appears.");
  expect(one.out).toContain("also  de  (untranslated: es fr pt-BR zh-CN zh-TW)");
  expect(one.out).toContain("internal/web/home.go:4:");
  const prefix = run("show", "home.empty", "--limit", "1");
  expect(prefix.out).toContain("web:home.empty.title");
  expect(prefix.out).toContain("... 1 more");
  expect(run("show", "nope").code).toBe(1);

  expect(run("find", "NOTHING here").out).toContain(
    "web:home.empty.title\n  en  Nothing here yet\n  ja  まだ何もありません",
  );
  expect(run("find", "見つかり").out).toContain("server:errors.not_found");
  expect(run("find", "Startseite", "--locale", "de").out).toContain("web:home.title");
  expect(run("find", "no such text").code).toBe(1);
});

test("check --changed checks the keys that differ from the merge base, and only those", () => {
  const clean = run("check", "--changed", "--base", "HEAD");
  expect(clean.out).toContain("checking 0 changed key(s)");
  expect(clean.code).toBe(0);

  const en = catalog("locales/en/web/home.json");
  writeCatalog("locales/en/web/home.json", { ...en, "home.title": entry("Home!"), "home.new": entry("New") });
  const ja = catalog("locales/ja/web/home.json");
  writeCatalog("locales/ja/web/home.json", { ...ja, "home.empty.title": entry("まだ,何もありません") });
  writeCatalog("locales/fr/web/lists.json", { "lists.title": entry("Listes") });
  const r = run("check", "--changed", "--base", "HEAD");
  expect(r.code).toBe(1);
  expect(r.out).toContain("checking 4 changed key(s)");
  expect(r.out).toContain("locales/en/web/home.json home.title: no exclamation marks");
  expect(r.out).toContain(
    'locales/ja/web/home.json home.new: missing; ja is written in the same change as en: scripts/i18n.sh set web:home.new --ja "<text>"',
  );
  expect(r.out).toContain('locales/ja/web/home.json home.empty.title: use "、"');
  expect(r.out).toContain(
    "locales/fr/web/lists.json lists.title: not in the source locale (obsolete); remove it: scripts/i18n.sh rm web:lists.title",
  );
  expect(r.out).not.toContain("untranslated");
  expect(r.out).toMatch(/i18n: 4 error\(s\), \d+ to answer, \d+ warning\(s\) in \d+\.\ds/);
  expect(run("check", "--changed", "--base", "HEAD", "--locale", "de").out).not.toContain("locales/ja/");
  expect(run("check", "--changed", "--base", "nope").out).toContain("no merge base with nope");
});

test("check fails on a key missing in ja and only warns for a machine-translated locale", () => {
  const all = run("check", "--warnings");
  expect(all.code).toBe(0);
  expect(all.out).toContain("warn:  de: untranslated web:home.empty.title");
  expect(run("check").out).not.toContain("untranslated");
  const { "home.footer": _, ...rest } = catalog("locales/ja/web/home.json");
  writeCatalog("locales/ja/web/home.json", rest);
  const r = run("check");
  expect(r.code).toBe(1);
  expect(r.out).toContain("locales/ja/web/home.json home.footer: missing; ja is written in the same change as en");
});

test("unused lists the keys nothing refers to and the literals no catalog defines", () => {
  write("internal/web/home.go", homeGo('l.WebHomeTitle(), l.Text("web:home.gone")'));
  write("internal/web/handler.go", "package web\n\nvar f = (*i18n.Localizer).ServerErrorsCount\n");
  write("internal/web/page.html", "");
  const r = run("unused");
  expect(r.code).toBe(0);
  expect(r.out).toContain("unused web:home.footer");
  expect(r.out).toContain("unused server:errors.not_found");
  expect(r.out).not.toContain("unused web:home.title");
  expect(r.out).not.toContain("unused server:errors.count");
  expect(r.out).toContain("undefined web:home.gone at internal/web/home.go:4");
  expect(r.out).toContain("i18n: 4 unused key(s), 1 undefined key literal(s)");
});
