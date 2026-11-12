// SPDX-License-Identifier: AGPL-3.0-only
// `scripts/i18n.sh <command>`: every edit of and question about the message catalogs of locales/. A command that
// writes a message checks it first with the rules of check.ts, writes it to the file of its area in each locale, and
// runs the generator; a refusal names the rule and the fix.
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { catalogFile, type Entry, isPseudo, rel, repoRoot, REQUIRED_LOCALES, SOURCE } from "./catalog";
import { KEY, messageFindings, printFindings, runCheck } from "./check";
import { DECISIONS } from "./copy";
import { writeGenerated } from "./gen";
import { goName } from "./generate";
import { type Ref, References } from "./refs";
import { splitId, Store } from "./store";

const USAGE = `scripts/i18n.sh <command>
  add <namespace>:<key> --en "<text>" --ja "<text>" --desc "<note for translators>" [--after <key>] [--drop-args]
  set <namespace>:<key> [--en "<text>"] [--ja "<text>"] [--<locale> "<text>"] [--desc "<note>"] [--keep-translations] [--drop-args]
  mv <namespace>:<key or prefix> <new key or prefix>
  rm <namespace>:<key>...
  show <key or prefix> [--refs] [--limit <n>]
  find "<text>" [--locale <locale>] [--limit <n>]
  check [--changed [--base <ref>]] [--locale <locale>] [--warnings]
  unused`;

class Refusal extends Error {}
/** Stops the command before it writes anything: each line is a rule and its fix. */
function refuse(...lines: string[]): never {
  throw new Refusal(lines.map((l) => `error: ${l}`).join("\n"));
}

const BOOLEAN_FLAGS = new Set(["drop-args", "keep-translations", "refs", "changed", "warnings"]);

function parseArgs(argv: string[]): { args: string[]; flags: Map<string, string> } {
  const args: string[] = [];
  const flags = new Map<string, string>();
  const rest = [...argv];
  for (let arg = rest.shift(); arg !== undefined; arg = rest.shift()) {
    if (!arg.startsWith("--") || arg === "--") {
      if (arg !== "--") args.push(arg);
      continue;
    }
    const [name = "", inline] = arg.slice(2).split(/=(.*)/s);
    const value = BOOLEAN_FLAGS.has(name) ? "true" : (inline ?? rest.shift());
    if (value === undefined) refuse(`--${name} needs a value`);
    flags.set(name, value);
  }
  return { args, flags };
}

function allow(flags: Map<string, string>, ...names: string[]) {
  for (const name of flags.keys()) {
    if (!names.includes(name)) refuse(`unknown option --${name}`, USAGE);
  }
}

const git = (...args: string[]): { ok: boolean; out: string } => {
  const p = Bun.spawnSync(["git", ...args], { cwd: repoRoot, stdout: "pipe", stderr: "pipe" });
  return { ok: p.exitCode === 0, out: p.stdout.toString() };
};

/** Parses "<namespace>:<key>" and refuses a namespace the catalog does not have. */
function parseId(store: Store, id: string | undefined, what = "<namespace>:<key>"): [ns: string, key: string] {
  if (!id) return refuse(`missing ${what}`, USAGE);
  const [ns, key] = splitId(id);
  const namespaces = store.namespaces();
  if (!key || !namespaces.includes(ns)) refuse(`${id}: write ${what}; the namespaces are ${namespaces.join(", ")}`);
  return [ns, key];
}

function checkNewKey(store: Store, ns: string, key: string, ignoring: string[] = []) {
  const id = `${ns}:${key}`;
  if (!KEY.test(key))
    refuse(`${id}: a key is dot-separated segments of a-z, 0-9 and _, each starting with a letter (${KEY})`);
  if (store.localesWith(ns, key).length) refuse(`${id} exists; change it with: scripts/i18n.sh set ${id} ...`);
  const clash = store.ids().find((other) => !ignoring.includes(other) && goName(other) === goName(id));
  if (clash) refuse(`${id} and ${clash} both map to the accessor ${goName(id)}; pick another key`);
}

/** The errors of `text` as the `locale` message of the key, against the source text when it is a translation. */
function textErrors(
  locale: string,
  ns: string,
  key: string,
  text: string,
  source: string,
  dropArgs: boolean,
): string[] {
  const { errors, dropped } = messageFindings(locale, ns, text, locale === SOURCE ? undefined : source, key);
  if (!dropArgs) {
    for (const name of dropped)
      errors.push(
        `drops argument {${name}} of the ${SOURCE} text; keep it, or pass --drop-args when the sentence does not need it`,
      );
  }
  return errors.map((e) => `--${locale}: ${e}`);
}

/** Saves the catalogs and regenerates; prints what changed. */
async function commit(store: Store) {
  for (const line of store.save()) console.log(line);
  for (const path of await writeGenerated()) console.log(`generated ${path}`);
}

async function add(argv: string[]) {
  const { args, flags } = parseArgs(argv);
  allow(flags, SOURCE, ...REQUIRED_LOCALES, "desc", "after", "drop-args");
  const store = new Store();
  const [ns, key] = parseId(store, args[0]);
  const id = `${ns}:${key}`;
  checkNewKey(store, ns, key);
  const locales = [SOURCE, ...REQUIRED_LOCALES];
  const missing = [...locales, "desc"].filter((f) => !flags.get(f)?.trim());
  if (missing.length)
    refuse(
      `${missing.map((f) => `--${f}`).join(", ")} missing: a message is added in ${locales.join(" and ")} together, with a note that tells translators where the text appears`,
      `usage: scripts/i18n.sh add ${id} --en "<text>" --ja "<text>" --desc "<note for translators>"`,
    );
  const text = (locale: string) => flags.get(locale) as string;
  const errors = locales.flatMap((l) => textErrors(l, ns, key, text(l), text(SOURCE), flags.has("drop-args")));
  if (errors.length) refuse(...errors);
  const description = (flags.get("desc") as string).trim();
  try {
    for (const l of locales) store.set(l, ns, key, { defaultMessage: text(l), description }, flags.get("after"));
  } catch (e) {
    refuse((e as Error).message);
  }
  await commit(store);
  console.log(`added ${id}; use (*i18n.Localizer).${goName(id)}`);
}

async function set(argv: string[]) {
  const { args, flags } = parseArgs(argv);
  const store = new Store();
  allow(flags, ...store.locales, "desc", "keep-translations", "drop-args");
  const [ns, key] = parseId(store, args[0]);
  const id = `${ns}:${key}`;
  const source = store.get(SOURCE, ns, key);
  if (!source)
    return refuse(`${id} is not in ${SOURCE}; add it: scripts/i18n.sh add ${id} --en ... --ja ... --desc ...`);
  const given = store.locales.filter((l) => flags.has(l));
  if (!given.length && !flags.has("desc")) refuse("nothing to set", USAGE);
  const next = flags.get(SOURCE) ?? source.defaultMessage;
  const sourceChanged = next !== source.defaultMessage;
  const required = REQUIRED_LOCALES.filter((l) => !flags.has(l));
  if (sourceChanged && required.length)
    refuse(
      ...required.map(
        (l) =>
          `--${l} missing: ${l} follows ${SOURCE} in the same change; pass the new text, or the present one if it still says the same: ${JSON.stringify(store.get(l, ns, key)?.defaultMessage ?? "")}`,
      ),
    );
  const followers = store.locales.filter((l) => l !== SOURCE && !REQUIRED_LOCALES.includes(l) && !flags.has(l));
  const kept = sourceChanged && flags.has("keep-translations") ? followers.filter((l) => store.get(l, ns, key)) : [];
  const text = (l: string) => flags.get(l) ?? (store.get(l, ns, key) as Entry).defaultMessage;
  const errors = [...given, ...kept].flatMap((l) => textErrors(l, ns, key, text(l), next, flags.has("drop-args")));
  if (errors.length) refuse(...errors);

  const description = flags.get("desc")?.trim() || source.description;
  for (const l of given) store.set(l, ns, key, { defaultMessage: text(l), ...(description ? { description } : {}) });
  if (flags.has("desc")) {
    for (const l of store.localesWith(ns, key))
      store.set(l, ns, key, {
        defaultMessage: (store.get(l, ns, key) as Entry).defaultMessage,
        ...(description ? { description } : {}),
      });
  }
  const dropped =
    sourceChanged && !flags.has("keep-translations") ? followers.filter((l) => store.delete(l, ns, key)) : [];
  await commit(store);
  if (dropped.length)
    console.log(
      `dropped the ${dropped.join(", ")} translations of ${id}: they were written for the old ${SOURCE} text, so these locales show ${SOURCE} until the next translation run (--keep-translations keeps them when the meaning did not change)`,
    );
  console.log(`set ${id}`);
}

/** The lines of the repository that still name a renamed key or its accessor, outside the catalogs and generated files. */
function leftovers(renames: Map<string, string>, references: References): string[] {
  const needles: string[] = [];
  const patterns: RegExp[] = [];
  const names: RegExp[] = [];
  const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  for (const from of renames.keys()) {
    needles.push(from);
    patterns.push(new RegExp(`${escape(from)}(?![A-Za-z0-9_]|\\.[a-z])`));
    needles.push(goName(from));
    names.push(new RegExp(`(?<![A-Za-z0-9_])${goName(from)}(Args)?(?![A-Za-z0-9_])`));
  }
  const found = git(
    "grep",
    "-n",
    "-I",
    "--untracked",
    "-F",
    ...needles.flatMap((n) => ["-e", n]),
    "--",
    ".",
    ":!locales",
    ":!internal/i18n/messages_gen.go",
    ":!internal/i18n/testdata/golden.json",
  );
  // In the files whose accessors were rewritten, what still carries the name is something else (a field of that name).
  return found.out
    .split("\n")
    .filter(
      (line) =>
        line &&
        (patterns.some((p) => p.test(line)) ||
          (!references.rewritesAccessors(line.split(":")[0] ?? "") && names.some((p) => p.test(line)))),
    );
}

async function mv(argv: string[]) {
  const { args, flags } = parseArgs(argv);
  allow(flags);
  const store = new Store();
  const [ns, from] = parseId(store, args[0], "<namespace>:<key or prefix>");
  if (!args[1]) refuse("missing the new key", USAGE);
  const [toNs, toKey] = args[1].includes(":") ? splitId(args[1]) : [ns, args[1]];
  if (toNs !== ns) refuse(`a key is renamed inside its namespace (${ns})`);
  const keys = store.get(SOURCE, ns, from)
    ? [from]
    : store
        .ids()
        .map((id) => splitId(id))
        .filter(([n, k]) => n === ns && k.startsWith(`${from}.`))
        .map(([, k]) => k);
  if (!keys.length)
    refuse(
      `${ns}:${from} is neither a key nor a prefix of keys of ${SOURCE}; look it up: scripts/i18n.sh show ${from}`,
    );
  const renames = new Map(keys.map((k) => [`${ns}:${k}`, `${ns}:${toKey}${k.slice(from.length)}`]));
  for (const id of renames.values()) checkNewKey(store, ns, splitId(id)[1], [...renames.keys()]);

  const references = new References(store.namespaces());
  for (const [old, id] of renames) store.rename(ns, splitId(old)[1], splitId(id)[1]);
  for (const path of references.rewrite(renames)) console.log(`rewrote ${path}`);
  await commit(store);
  for (const [old, id] of renames) console.log(`renamed ${old} -> ${id}`);
  const left = leftovers(renames, references);
  if (left.length)
    refuse(
      "the catalogs and the references are renamed, but these lines still name the old key and were not rewritten (a key built at run time, prose, another file type); fix them by hand:",
      ...left,
    );
}

const printRefs = (refs: Ref[]) => refs.map((r) => `  ${r.file}:${r.line}: ${r.text}`);

async function rm(argv: string[]) {
  const { args, flags } = parseArgs(argv);
  allow(flags);
  const store = new Store();
  if (!args.length) refuse("missing <namespace>:<key>", USAGE);
  const ids = args.map((arg) => parseId(store, arg));
  const references = new References(store.namespaces());
  const errors: string[] = [];
  for (const [ns, key] of ids) {
    const id = `${ns}:${key}`;
    if (!store.localesWith(ns, key).length)
      errors.push(`${id} is in no locale; look it up: scripts/i18n.sh show ${key}`);
    const refs = references.of(id);
    if (refs.length)
      errors.push(
        `${id} is still referenced; remove the references first (or rename: scripts/i18n.sh mv):\n${printRefs(refs).join("\n")}`,
      );
  }
  if (errors.length) refuse(...errors);
  for (const [ns, key] of ids) {
    const from = store.localesWith(ns, key).filter((l) => store.delete(l, ns, key));
    console.log(`removed ${ns}:${key} from ${from.join(", ")}`);
  }
  await commit(store);
}

const clip = (s: string, max = 400) => (s.length > max ? `${s.slice(0, max)}…` : s);

function show(argv: string[]) {
  const { args, flags } = parseArgs(argv);
  allow(flags, "refs", "limit");
  if (!args[0]) refuse("missing <key or prefix>", USAGE);
  const store = new Store();
  const [wantNs, query] = args[0].includes(":") ? splitId(args[0]) : [undefined, args[0]];
  const ids = store
    .ids()
    .map((id) => splitId(id))
    .filter(([ns]) => wantNs === undefined || ns === wantNs);
  const exact = ids.filter(([, key]) => key === query);
  const matches = exact.length ? exact : ids.filter(([, key]) => key.startsWith(query));
  if (!matches.length) refuse(`no key starts with ${args[0]}; search the text instead: scripts/i18n.sh find "<text>"`);
  const limit = Number(flags.get("limit") ?? 20);
  const references = flags.has("refs") ? new References(store.namespaces()) : undefined;
  for (const [ns, key] of matches.slice(0, limit)) {
    const id = `${ns}:${key}`;
    console.log(`${id}  (${rel(catalogFile(SOURCE, ns, key))})`);
    for (const locale of [SOURCE, ...REQUIRED_LOCALES]) {
      console.log(`  ${locale}    ${clip(store.get(locale, ns, key)?.defaultMessage ?? "MISSING")}`);
    }
    console.log(`  desc  ${clip(store.get(SOURCE, ns, key)?.description ?? "(none)")}`);
    const others = store.locales.filter((l) => l !== SOURCE && !REQUIRED_LOCALES.includes(l));
    const without = others.filter((l) => !store.get(l, ns, key));
    console.log(
      `  also  ${others.filter((l) => !without.includes(l)).join(" ") || "-"}${without.length ? `  (untranslated: ${without.join(" ")})` : ""}`,
    );
    if (references) {
      const refs = references.of(id);
      console.log(
        refs.length
          ? `  refs\n${printRefs(refs.slice(0, 10))
              .map((l) => `  ${l}`)
              .join("\n")}`
          : "  refs  none",
      );
      if (refs.length > 10) console.log(`    ... ${refs.length - 10} more`);
    }
  }
  if (matches.length > limit)
    console.log(`... ${matches.length - limit} more; narrow the prefix or pass --limit ${matches.length}`);
}

function find(argv: string[]) {
  const { args, flags } = parseArgs(argv);
  allow(flags, "locale", "limit");
  const needle = args[0]?.toLowerCase();
  if (!needle) refuse('missing "<text>"', USAGE);
  const store = new Store();
  const locales = flags.has("locale") ? [flags.get("locale") as string] : [SOURCE, ...REQUIRED_LOCALES];
  const unknown = locales.filter((l) => !store.locales.includes(l));
  if (unknown.length) refuse(`--locale ${unknown[0]}: the locales are ${store.locales.join(", ")}`);
  const limit = Number(flags.get("limit") ?? 20);
  const shown = [...new Set([SOURCE, ...REQUIRED_LOCALES, ...locales])];
  let found = 0;
  for (const id of store.ids()) {
    const [ns, key] = splitId(id);
    if (!locales.some((l) => store.get(l, ns, key)?.defaultMessage.toLowerCase().includes(needle))) continue;
    if (++found > limit) continue;
    console.log(id);
    for (const l of shown) console.log(`  ${l}  ${clip(store.get(l, ns, key)?.defaultMessage ?? "MISSING", 200)}`);
  }
  if (found > limit) console.log(`... ${found - limit} more; add words to the text or pass --limit ${found}`);
  if (!found)
    refuse(
      `no ${locales.join(" or ")} message contains ${JSON.stringify(args[0])} (the search ignores case, not ICU syntax)`,
    );
}

/** The ids whose entry differs from the merge base with `base` in any locale; undefined when every key must be checked. */
function changedIds(base: string): Set<string> | undefined {
  const mergeBase = git("merge-base", base, "HEAD");
  if (!mergeBase.ok)
    refuse(
      `no merge base with ${base}; fetch it (git fetch origin main) or name another ref: scripts/i18n.sh check --changed --base <ref>`,
    );
  const ref = mergeBase.out.trim();
  const names = (...args: string[]) =>
    git(...args)
      .out.split("\0")
      .filter(Boolean);
  const paths = (...spec: string[]) => [
    ...names("diff", "--name-only", "-z", ref, "--", ...spec),
    ...names("ls-files", "--others", "--exclude-standard", "-z", "--", ...spec),
  ];
  // A changed rule, glossary, or word decision of the copy role can fail a message the change did not touch.
  if (paths("tools/i18n", DECISIONS).length) return undefined;
  const ids = new Set<string>();
  for (const path of paths("locales")) {
    const [, locale = "", ns = "", ...area] = path.split("/");
    if (!path.endsWith(".json") || !area.length || isPseudo(locale)) continue;
    const before = git("show", `${ref}:${path}`);
    const old: Record<string, Entry> = before.ok ? JSON.parse(before.out) : {};
    const file = join(repoRoot, path);
    const now: Record<string, Entry> = existsSync(file) ? JSON.parse(readFileSync(file, "utf8")) : {};
    for (const key of new Set([...Object.keys(old), ...Object.keys(now)])) {
      if (JSON.stringify(old[key]) !== JSON.stringify(now[key])) ids.add(`${ns}:${key}`);
    }
  }
  return ids;
}

async function check(argv: string[]) {
  const { args, flags } = parseArgs(argv);
  allow(flags, "changed", "base", "locale", "warnings");
  if (args.length) refuse(`unexpected ${args[0]}`, USAGE);
  const locales = new Store().locales;
  if (flags.has("locale") && !locales.includes(flags.get("locale") as string))
    refuse(`--locale ${flags.get("locale")}: the locales are ${locales.join(", ")}`);
  const only = flags.has("changed") ? changedIds(flags.get("base") ?? "origin/redesign-web") : undefined;
  const findings = await runCheck({ only, locales: flags.has("locale") ? [flags.get("locale") as string] : undefined });
  if (flags.has("changed"))
    console.log(only ? `checking ${only.size} changed key(s)` : "the i18n tool changed: checking every key");
  // An untranslated key of a machine-translated locale is expected in every change of en: counted, not listed.
  const warnings = (message: string) => flags.has("warnings") || (only !== undefined && message.startsWith("unused "));
  const errors = printFindings(findings, { warnings, seconds: Bun.nanoseconds() / 1e9 });
  if (errors > 0) process.exit(1);
}

function unused(argv: string[]) {
  allow(parseArgs(argv).flags);
  const store = new Store();
  const namespaces = store.namespaces();
  const references = new References(namespaces);
  const ids = new Set(store.ids());
  const unreferenced = [...ids].filter((id) => !references.used(id)).sort();
  for (const id of unreferenced) console.log(`unused ${id}; refer to it, or remove it: scripts/i18n.sh rm ${id}`);
  const undefinedIds = references.undefinedLiterals(namespaces, (id) => ids.has(id));
  for (const [id, refs] of [...undefinedIds].sort(([a], [b]) => (a < b ? -1 : 1))) {
    console.log(
      `undefined ${id} at ${refs.map((r) => `${r.file}:${r.line}`).join(", ")}; add it: scripts/i18n.sh add ${id} ...`,
    );
  }
  console.log(`i18n: ${unreferenced.length} unused key(s), ${undefinedIds.size} undefined key literal(s)`);
}

const commands: Record<string, (argv: string[]) => void | Promise<void>> = {
  add,
  set,
  mv,
  rm,
  show,
  find,
  check,
  unused,
};

const [name = "help", ...rest] = process.argv.slice(2);
try {
  const command = commands[name];
  if (!command) {
    console.log(USAGE);
    process.exit(name === "help" || name === "--help" ? 0 : 2);
  }
  await command(rest);
} catch (e) {
  if (!(e instanceof Refusal)) throw e;
  console.error(e.message);
  process.exit(1);
}
