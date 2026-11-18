// SPDX-License-Identifier: AGPL-3.0-only

// How each screen under the entry Account and data says that it loads, and that what it reads could not be read, with
// the way to try again (#3203): Export, Import, Delete old posts automatically, Aliases, Move account, Deactivate
// account and Delete account, each drawn alone on the fake server with its read held or failed; and how Import holds
// the file picked last while the browser reads it, and says a file the server refuses as too large. A screen of the app
// is drawn once its route's loader answered, so a held read is seen here and not in a route test; what each screen
// does once read is the app's route tests (web/app, settings-account-data.dom.test.tsx and
// settings-account-leave.dom.test.tsx).

import type { Route } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { describe, expect, test } from "bun:test";
import type { ReactNode } from "react";
import { chooseFile } from "../test/profile-form";
import { plain } from "../test/relationships";
import { describedBy, focusOn, section, words } from "../test/security";
import { addressProps, settingsOn } from "../test/settings";
import { AccountAliasesScreen } from "./account-aliases";
import { AccountAutoDelete } from "./account-auto-delete";
import { AccountDeactivateScreen, AccountDeleteScreen } from "./account-end";
import { AccountExportScreen } from "./account-export";
import { AccountImportScreen } from "./account-import";
import { AccountMoveScreen } from "./account-move";

type Props = ReturnType<typeof addressProps>;

// Each screen, how it is drawn, the read it waits for, what it says meanwhile, what it says when the read failed, and
// what shows once it is read.
const screens: readonly [string, (props: Props) => ReactNode, Route, string, string, () => Promise<unknown>][] = [
  [
    "Export",
    (props) => <AccountExportScreen {...props} />,
    "GET /exports",
    "Loading your archives…",
    "Your archives could not be loaded.",
    () => screen.findByRole("button", { name: "Request an archive" }),
  ],
  [
    "Import",
    (props) => <AccountImportScreen {...props} />,
    "GET /imports",
    "Loading your imports…",
    "Your imports could not be loaded.",
    () => screen.findByRole("list", { name: "Recent imports" }),
  ],
  [
    "Delete old posts automatically",
    (props) => <AccountAutoDelete {...props} onUnsaved={() => undefined} />,
    "GET /preferences/auto_delete",
    "Loading your settings for old posts…",
    "Your settings for old posts could not be loaded.",
    () => screen.findByRole("textbox", { name: "Delete posts older than" }),
  ],
  [
    "Aliases",
    (props) => <AccountAliasesScreen {...props} />,
    "GET /accounts/aliases",
    "Loading your aliases…",
    "Your aliases could not be loaded.",
    () => screen.findByRole("list", { name: "1 of 5 aliases" }),
  ],
  [
    "Move account",
    (props) => <AccountMoveScreen {...props} />,
    "GET /accounts/verify_credentials",
    "Loading your account…",
    "Your account could not be loaded.",
    () => screen.findByRole("textbox", { name: "Account you are moving to" }),
  ],
  [
    "Deactivate account",
    (props) => <AccountDeactivateScreen {...props} />,
    "GET /accounts/deactivation",
    "Loading your account…",
    "Your account could not be loaded.",
    () => screen.findByRole("button", { name: "Deactivate account" }),
  ],
  [
    "Delete account",
    (props) => <AccountDeleteScreen {...props} />,
    "GET /accounts/verify_credentials",
    "Loading your account…",
    "Your account could not be loaded.",
    () => screen.findByRole("button", { name: "Delete account" }),
  ],
];

/** What the screen says, as one line. */
const page = () => words(screen.getByRole("main"));

describe("while its read is on its way, the screen says it loads", () => {
  for (const [name, draw, read, loading] of screens)
    test(name, async () => {
      let held: { arrived: () => Promise<unknown>; release: () => void } | undefined;
      await settingsOn({}, draw, {
        before: ({ fake }) => {
          held = fake.script(read, { hold: "request" });
        },
      });
      await held?.arrived();

      await waitFor(() => {
        if (!page().includes(loading)) throw new Error(`test: the screen says ${page().slice(0, 200)}`);
      });
      held?.release();
    });
});

describe("when its read failed, the screen says so, and Try again reads it again", () => {
  for (const [name, draw, read, , failed, shown] of screens)
    test(name, async () => {
      let failing: { end: () => void } | undefined;
      const { user } = await settingsOn({}, draw, {
        before: ({ fake }) => {
          failing = fake.script(read, { fail: 500 });
        },
      });

      await waitFor(() => {
        if (!page().includes(failed)) throw new Error(`test: the screen says ${page().slice(0, 200)}`);
      });
      failing?.end();
      const [again, ...others] = within(screen.getByRole("main")).getAllByRole("button", { name: "Try again" });
      expect(others).toEqual([]);
      if (again === undefined) throw new Error("test: no Try again");
      await user.click(again);

      await shown();
      expect(page()).not.toContain(failed);
    });
});

// ---- Import: the file picked, as it is read ------------------------------------------------------------------------

// The screen reads a file picked before it previews or sends it, and the reader may pick again meanwhile: what the
// field holds is the file picked last, whatever order the reads end in. A file whose read the test gives, ends or
// fails stands in for a large or slow one (the browser's `File.text()`).

/** A CSV file named `name` whose text the browser gives once the test calls `give`, or refuses on `fail`. */
function heldFile(name: string, text: string) {
  let give: () => void = () => undefined;
  let fail: () => void = () => undefined;
  const read = new Promise<string>((resolve, reject) => {
    give = () => {
      resolve(text);
    };
    fail = () => {
      reject(new DOMException("The file could not be read.", "NotReadableError"));
    };
  });
  // A rejection the screen has not asked for yet is not one of the test's.
  read.catch(() => undefined);
  class Held extends File {
    override text(): Promise<string> {
      return read;
    }
  }
  return { file: new Held([text], name, { type: "text/csv" }), give, fail };
}

/** A CSV file of follows named `name`, read at once, listing `handles`. */
const csvFile = (name: string, ...handles: string[]) => new File([handles.join("\n")], name, { type: "text/csv" });

/** The button that opens the file chooser, whatever a file chosen made of its name. */
const fileButton = () => within(section("File")).getByRole("button", { name: /^Choose (a|another) file$/ });

/** The field of the file: the group of its button and of what names the file chosen. */
const fileField = () => within(section("File")).getByRole("group", { name: "CSV file" });

/** The submit bar's Import. */
const importButton = () => screen.getAllByRole("button", { name: "Import" }).at(-1) ?? document.body;

/** What describes the file's field and its button, part by part, the marks that isolate a name taken out. */
const fileSays = () => [...describedBy(fileField()), ...describedBy(fileButton())].map(plain);

/** Whether `button` takes no press: disabled, or marked so. */
const disabled = (button: HTMLElement) =>
  button.hasAttribute("disabled") || button.getAttribute("aria-disabled") === "true";

/** The sentence of a file longer than the server takes, named `name`. */
const tooLarge = (name: string) =>
  `“${name}” is too large: a file holds up to 1,000,000 characters. Choose a smaller one.`;

/** Import, drawn alone with its recent imports read, and the scripts that count its previews and its starts. */
async function importing() {
  const on = await settingsOn({}, (props) => <AccountImportScreen {...props} />);
  await screen.findByRole("list", { name: "Recent imports" });
  const previews = on.fake.script("POST /imports/preview");
  const started = on.fake.script("POST /imports");
  return { ...on, previews, started };
}

/** Waits one turn of the event loop and a little more, for a request or a change that would come late. */
const aWhile = () => new Promise((resolve) => setTimeout(resolve, 50));

/** Waits until the preview lists what a file holds. */
const previewShown = () => waitFor(() => within(section("Preview")).getByRole("list", { name: /./ }));

describe("Import holds the file picked last", () => {
  test("while a pick is read, the field names it, its button reads Choose another file, Import takes no press, and no preview is asked; once read, the file is previewed", async () => {
    const on = await importing();
    const picked = heldFile("slow.csv", "marco@demo.test\nsam@demo.test");

    await chooseFile(on.user, fileButton(), picked.file);

    await waitFor(() => {
      if (!fileSays().includes("slow.csv")) throw new Error(`test: the field says ${JSON.stringify(fileSays())}`);
    });
    expect(fileButton().textContent).toBe("Choose another file");
    expect(disabled(importButton())).toBe(true);
    await on.user.click(importButton());
    await aWhile();
    expect(on.started.seen).toHaveLength(0);
    expect(on.previews.seen).toHaveLength(0);

    picked.give();

    await previewShown();
    expect(on.previews.seen.map((s) => s.body.text)).toEqual(["marco@demo.test\nsam@demo.test"]);
    expect(fileSays()).toContain("slow.csv");
    expect(disabled(importButton())).toBe(false);
  });

  test("a slow pick, then a fast one: the fast one stays, and the slow one's late text is ignored", async () => {
    const on = await importing();
    const slow = heldFile("b.csv", "marco@demo.test");
    await chooseFile(on.user, fileButton(), slow.file);

    await chooseFile(on.user, fileButton(), csvFile("c.csv", "sam@demo.test", "hana@demo.test"));
    await previewShown();
    slow.give();
    await aWhile();

    expect(fileSays()).toContain("c.csv");
    expect(fileSays()).not.toContain("b.csv");
    expect(on.previews.seen.map((s) => s.body.text)).toEqual(["sam@demo.test\nhana@demo.test"]);
  });

  test("a file read, then another whose read has not ended: Import sends nothing", async () => {
    const on = await importing();
    await chooseFile(on.user, fileButton(), csvFile("a.csv", "marco@demo.test"));
    await previewShown();
    const later = heldFile("b.csv", "sam@demo.test");
    await chooseFile(on.user, fileButton(), later.file);

    await on.user.click(importButton());

    await aWhile();
    expect(on.started.seen).toHaveLength(0);
    later.give();
    await aWhile();
    expect(on.started.seen).toHaveLength(0);
  });

  test("a file over 1,000,000 characters is refused under the field, which holds no file, and is not previewed", async () => {
    const on = await importing();

    await chooseFile(on.user, fileButton(), new File(["a".repeat(1_000_001)], "huge.csv", { type: "text/csv" }));

    await waitFor(() => {
      if (!fileSays().includes(tooLarge("huge.csv")))
        throw new Error(`test: the field says ${JSON.stringify(fileSays())}`);
    });
    expect(fileButton().textContent).toBe("Choose a file");
    expect(fileSays()).toContain("No file chosen");
    await aWhile();
    expect(on.previews.seen).toHaveLength(0);
  });

  test("a file the browser could not read is refused under the field, which holds no file", async () => {
    const on = await importing();
    const broken = heldFile("broken.csv", "marco@demo.test");
    await chooseFile(on.user, fileButton(), broken.file);

    broken.fail();

    await waitFor(() => {
      if (!fileSays().includes("This file could not be read as a CSV file. Choose another one."))
        throw new Error(`test: the field says ${JSON.stringify(fileSays())}`);
    });
    expect(fileButton().textContent).toBe("Choose a file");
    expect(on.previews.seen).toHaveLength(0);
  });
});

describe("Import: a request the server refuses as too large (413)", () => {
  const problem = { type: "about:blank", title: "Request Entity Too Large", status: 413 } as const;

  test("on the preview, the field says the file is too large, by its name, and keeps it", async () => {
    const on = await importing();
    on.fake.script("POST /imports/preview", { fail: problem });

    await chooseFile(on.user, fileButton(), csvFile("follows.csv", "marco@demo.test"));

    await waitFor(() => {
      if (!fileSays().includes(tooLarge("follows.csv")))
        throw new Error(`test: the field says ${JSON.stringify(fileSays())}`);
    });
    expect(fileButton().textContent).toBe("Choose another file");
    expect(fileSays()).toContain("follows.csv");
  });

  test("on the start, the same sentence stands under the field, which takes the focus, and the bar refuses", async () => {
    const on = await importing();
    await chooseFile(on.user, fileButton(), csvFile("follows.csv", "marco@demo.test"));
    await previewShown();
    on.fake.script("POST /imports", { fail: problem });

    await on.user.click(importButton());

    await waitFor(() => {
      if (!fileSays().includes(tooLarge("follows.csv")))
        throw new Error(`test: the field says ${JSON.stringify(fileSays())}`);
    });
    await focusOn(fileButton);
    expect(screen.getAllByRole("alert").map(words)).toEqual(["Nothing was imported. Fix what is marked."]);
    expect(fileButton().textContent).toBe("Choose another file");
  });
});
