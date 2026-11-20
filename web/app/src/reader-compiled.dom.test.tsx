// SPDX-License-Identifier: AGPL-3.0-only

// A language chosen in the settings, with the Reader as the build compiles it (#3094): React Compiler keeps what a
// call returned until its arguments change, so a Reader that asked hasStrings alone whether the new language's strings
// were read kept its answer of before the load, and stayed in the language it was in. Here reader.tsx goes through the
// build's compiler (vite-plugins/react-compiler.ts) and replaces the module the app imports. The tests read every
// locale with the package's preload, so the strings of ja are held for a test by the loading the Reader asks of
// strings.ts, as reader-load.dom.test.tsx holds them.

import { afterAll, expect, mock, test } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { waitFor } from "@testing-library/react";
import { compileModule } from "../vite-plugins/react-compiler";
import * as strings from "./strings";

const loadStrings = strings.loadStrings;
const hasStrings = strings.hasStrings;

/** The loading of ja's strings while the test holds it, and what ends it. */
let held: { promise: Promise<undefined>; resolve: () => void } | undefined;

/** Holds the loading of ja's strings until the test lets it end. */
function holdJa(): () => void {
  let resolve: (value: undefined) => void = () => undefined;
  const promise = new Promise<undefined>((ok) => {
    resolve = ok;
  });
  held = {
    promise,
    resolve: () => {
      resolve(undefined);
    },
  };
  return held.resolve;
}

const isJa = (locale: string) => locale.split("-")[0] === "ja";

await mock.module("./strings", () => ({
  ...strings,
  loadStrings: (locale: string) => (isJa(locale) && held !== undefined ? held.promise : loadStrings(locale)),
  hasStrings: (locale: string) => !(isJa(locale) && held !== undefined) && hasStrings(locale),
}));

/** reader.tsx as the build compiles it, its imports resolved from here, in a directory of its own. */
async function compiledReader(): Promise<string> {
  const source = join(import.meta.dir, "reader.tsx");
  const compiled = await compileModule(source, await Bun.file(source).text(), false);
  if (compiled.compiled === 0) throw new Error("test: React Compiler compiled nothing of reader.tsx");
  // The compiler leaves the JSX in place: it goes to the runtime of the React the app resolves.
  const react = dirname(Bun.resolveSync("react/package.json", import.meta.dir));
  const code = compiled.code.replace(
    /(\bfrom\s*)"([^"]+)"/g,
    (_, from: string, specifier: string) => `${from}"${Bun.resolveSync(specifier, import.meta.dir)}"`,
  );
  const file = join(directory, "reader.tsx");
  writeFileSync(file, `/** @jsxImportSource ${react} */\n${code}`);
  return file;
}

const directory = mkdtempSync(join(tmpdir(), "my-reader-compiled-"));
afterAll(() => {
  rmSync(directory, { recursive: true, force: true });
});
const reader = (await import(await compiledReader())) as Record<string, unknown>;
await mock.module("./reader", () => reader);

const { main, renderApp } = await import("./test/app");
const { ownDevice } = await import("./test/device");
const { fakeApp } = await import("./test/server");

/** The title of the screen in place, as its heading says it. */
const heading = () =>
  main()
    .queryAllByRole("heading", { level: 1 })
    .map((title) => title.textContent)
    .join();

test("a language chosen in the settings shows once its strings are loaded, with the Reader the build compiles", async () => {
  ownDevice();
  const zone = new Intl.DateTimeFormat().resolvedOptions().timeZone;
  const app = await renderApp("/settings/display", { on: fakeApp({ preferences: { locale: "en", time_zone: zone } }) });
  const language = await main().findByRole("combobox", { name: "Display language" });
  const loaded = holdJa();

  await app.user.selectOptions(language, "日本語");
  await new Promise((wait) => setTimeout(wait, 150));
  // Until ja's strings are loaded the screen reads en, as it did.
  expect(heading()).toBe("Display and posting");
  expect(document.documentElement.lang).toBe("en");

  held = undefined;
  loaded();
  await waitFor(() => {
    if (heading() !== "表示と投稿") throw new Error(`test: the screen is "${heading()}"`);
  });
  expect(document.documentElement.lang).toBe("ja");
  expect(app.path()).toBe("/settings/display");
});
