// SPDX-License-Identifier: AGPL-3.0-only

// Edit profile, the form under the settings' Profile (#3200): it opens with the account's images, name, bio and
// fields; an image is picked, shown at once, replaced and removed; the name and the bio count against their limits
// and say when they are past them; the fields are pairs added up to four and removed; and Save sends the uploads, then
// one PATCH with only what changed, or nothing when nothing did. What the server refuses shows where it belongs: under
// the one input the problem names, inside the group of the image that could not be used, or in the bar beside Save.
// The form tells its owner whether it holds unsaved changes, and asks before they are left when its owner says the
// reader is leaving. Where the focus goes is profile-edit.jsdom.test.tsx; the way to the form, the way back and the
// held navigation are the app's (web/app/src/settings-profile.dom.test.tsx); the fields of an image and of a pair by
// themselves are the design system's.

import { describe, expect, test } from "bun:test";
import type { FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import {
  alerts,
  bioField,
  chooseFile,
  describedBy,
  image,
  imageButtons,
  imageFile,
  invalid,
  nameField,
  pairField,
  pairs,
  pictures,
  profileForm,
  rewrite,
  saveButton,
  type DrawnForm,
} from "../test/profile-form";

const header = "Header image";
const avatar = "Avatar";

/** What the status regions of the form say. */
const said = () =>
  screen
    .queryAllByRole("status")
    .map((status) => status.textContent)
    .filter((words) => words !== "");

/** The writes the form sent since `from`, each as `METHOD /path`. */
const writes = ({ requests }: DrawnForm, from = 0) => requests.slice(from).filter((r) => !r.startsWith("GET "));

/** Presses Save and returns once the form says that it saved. */
async function saved({ user }: DrawnForm): Promise<void> {
  await user.click(saveButton());
  await waitFor(() => {
    expect(said()).toEqual(["Saved"]);
  });
}

/** A problem of the server about the member `field` of the change, in its own words. */
const refusal = (field: string, detail: string) =>
  ({ type: "about:blank", title: "This could not be done.", status: 422, field, detail }) as const;

describe("the form opens with what the account has", () => {
  test("the scenario's account: both images with Replace and Remove, the name, the bio as written, and its one field", async () => {
    await profileForm();

    expect(imageButtons(header)).toEqual(["Replace", "Remove"]);
    expect(pictures(header)).toHaveLength(1);
    expect(imageButtons(avatar)).toEqual(["Replace", "Remove"]);
    expect(pictures(avatar)).toHaveLength(1);
    expect(nameField().value).toBe("Yuki 雪");
    expect(bioField().value).toBe("写真と散歩。週末は山にいます。");
    // Three lines tall, as the attribute says it: happy-dom's `rows` is that text and no number.
    expect(bioField().getAttribute("rows")).toBe("3");
    // The field as the account wrote it, not as a profile renders it.
    expect(pairs()).toEqual([["写真", "https://yuki.photos.test"]]);
    expect(within(screen.getByRole("list", { name: "Fields" })).getAllByRole("listitem")).toHaveLength(1);
    expect(
      within(screen.getByRole("group", { name: "Field 1" }))
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label")),
    ).toEqual(["Remove field 1"]);
    // Nothing is wrong, nothing was saved, and nothing is unsaved.
    expect(invalid()).toEqual([]);
    expect(alerts()).toEqual([]);
    expect(said()).toEqual([]);
    expect(saveButton().disabled).toBe(false);
  });

  test("an account with nothing yet: each image is chosen, the name and the bio are empty, and where the fields would be a line says so", async () => {
    await profileForm({ profile: { start: "empty" } });

    expect(imageButtons(header)).toEqual(["Choose an image"]);
    expect(pictures(header)).toEqual([]);
    expect(imageButtons(avatar)).toEqual(["Choose an image"]);
    expect(pictures(avatar)).toEqual([]);
    expect(nameField().value).toBe("");
    expect(bioField().value).toBe("");
    expect(pairs()).toEqual([]);
    expect(screen.queryAllByRole("list", { name: "Fields" })).toHaveLength(0);
    expect(
      screen.getByText("No fields yet. Add one to show something like a website or your pronouns on your profile."),
    ).not.toBeNull();
  });

  test("in Japanese the images, the fields and Save read in it", async () => {
    await profileForm({}, { locale: "ja" });

    expect(imageButtons("ヘッダー画像")).toEqual(["画像を変更", "外す"]);
    expect(imageButtons("アバター")).toEqual(["画像を変更", "外す"]);
    expect(
      within(screen.getByRole("group", { name: "補足情報 1" }))
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label")),
    ).toEqual(["補足情報 1 を外す"]);
    expect(screen.getAllByRole("button", { name: "補足情報を追加" })).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: "保存" })).toHaveLength(1);
  });

  test("in Japanese an image not there yet is chosen in it", async () => {
    await profileForm({ profile: { start: "empty" } }, { locale: "ja" });

    expect(imageButtons("ヘッダー画像")).toEqual(["画像を選ぶ"]);
    expect(imageButtons("アバター")).toEqual(["画像を選ぶ"]);
  });

  test("the way back, named after the entry, is the owner's to follow", async () => {
    const form = await profileForm();

    await form.user.click(screen.getByRole("button", { name: "Profile" }));

    expect(form.told).toContain("back");
    expect(screen.getByRole("heading", { name: "Edit profile" })).not.toBeNull();
  });
});

describe("an image", () => {
  for (const name of [header, avatar])
    test(`picked for the ${name.toLowerCase()} of an account without one shows at once, and can be replaced or removed`, async () => {
      const form = await profileForm({ profile: { start: "empty" } });
      const before = form.requests.length;

      await chooseFile(form.user, image(name).getByRole("button", { name: "Choose an image" }), imageFile("dusk.png"));

      // The file itself, from the reader's device: nothing was sent.
      await waitFor(() => {
        expect(pictures(name)).toHaveLength(1);
      });
      expect(pictures(name)[0]).toMatch(/^blob:/);
      expect(imageButtons(name)).toEqual(["Replace", "Remove"]);
      expect(writes(form, before)).toEqual([]);
      // The other image is as it was.
      const other = name === header ? avatar : header;
      expect(imageButtons(other)).toEqual(["Choose an image"]);
      expect(pictures(other)).toEqual([]);
    });

  test("picked in place of the one the account has shows in its place at once", async () => {
    const form = await profileForm();
    const [had] = pictures(avatar);

    await chooseFile(form.user, image(avatar).getByRole("button", { name: "Replace" }), imageFile("snow.png"));

    await waitFor(() => {
      expect(pictures(avatar)[0]).not.toBe(had);
    });
    expect(pictures(avatar)).toHaveLength(1);
    expect(pictures(avatar)[0]).toMatch(/^blob:/);
    expect(imageButtons(avatar)).toEqual(["Replace", "Remove"]);
  });

  test("removed is gone from the form at once, and its group offers to choose one", async () => {
    const form = await profileForm();

    await form.user.click(image(header).getByRole("button", { name: "Remove" }));

    expect(pictures(header)).toEqual([]);
    expect(imageButtons(header)).toEqual(["Choose an image"]);
    expect(imageButtons(avatar)).toEqual(["Replace", "Remove"]);
  });

  test("picked and then removed leaves the account without one, and one chosen after that shows again", async () => {
    const form = await profileForm({ profile: { start: "empty" } });
    await chooseFile(form.user, image(avatar).getByRole("button", { name: "Choose an image" }), imageFile("a.png"));
    await waitFor(() => {
      expect(pictures(avatar)).toHaveLength(1);
    });

    await form.user.click(image(avatar).getByRole("button", { name: "Remove" }));
    expect(pictures(avatar)).toEqual([]);
    expect(imageButtons(avatar)).toEqual(["Choose an image"]);

    await chooseFile(form.user, image(avatar).getByRole("button", { name: "Choose an image" }), imageFile("a.png"));
    await waitFor(() => {
      expect(pictures(avatar)).toHaveLength(1);
    });
  });
});

describe("the limits of the name and the bio", () => {
  // The field, its limit in characters, and what it says past it.
  const limited = [
    ["Display name", nameField, 30, "The display name must be at most 30 characters."],
    ["Bio", bioField, 500, "The bio must be at most 500 characters."],
  ] as const;

  for (const [name, field, limit, sentence] of limited) {
    test(`${name}: far from its ${String(limit)} characters it counts nothing, near them it says how many are left`, async () => {
      const form = await profileForm();
      expect(screen.queryAllByText(/characters? left$/)).toHaveLength(0);

      await rewrite(form.user, field(), "a".repeat(limit - 2));

      expect(screen.getAllByText("2 characters left").length).toBeGreaterThan(0);
      expect(invalid()).toEqual([]);

      await form.user.click(field());
      await form.user.paste("aa");

      // At the limit itself nothing is wrong.
      expect(screen.getAllByText("0 characters left").length).toBeGreaterThan(0);
      expect(invalid()).toEqual([]);
      expect(screen.queryAllByText(sentence)).toHaveLength(0);
    });

    test(`${name}: past its ${String(limit)} characters it says by how many and what the limit is, and Save sends nothing`, async () => {
      const form = await profileForm();
      const before = form.requests.length;

      await rewrite(form.user, field(), "a".repeat(limit + 2));

      expect(screen.getAllByText("2 characters over").length).toBeGreaterThan(0);
      expect(invalid()).toEqual([name]);
      expect(describedBy(field())).toContain(sentence);

      await form.user.click(saveButton());

      expect(writes(form, before)).toEqual([]);
      expect(said()).toEqual([]);
      expect(invalid()).toEqual([name]);
      // What the reader wrote is kept for them to shorten.
      expect(field().value).toBe("a".repeat(limit + 2));
    });

    test(`${name}: shortened to its limit it is no longer wrong, and Save sends it`, async () => {
      const form = await profileForm();
      const sent = form.fake.script("PATCH /accounts/update_credentials");
      await rewrite(form.user, field(), "a".repeat(limit + 1));
      expect(invalid()).toEqual([name]);

      await form.user.click(field());
      await form.user.keyboard("{Backspace}");

      expect(invalid()).toEqual([]);
      expect(screen.queryAllByText(sentence)).toHaveLength(0);
      await saved(form);
      expect(sent.seen.map((request) => request.body)).toEqual([
        name === "Bio" ? { note: "a".repeat(limit) } : { display_name: "a".repeat(limit) },
      ]);
    });
  }

  test("a character counts once, whatever it takes to encode it: thirty emoji are a name within the limit", async () => {
    const form = await profileForm();
    const sent = form.fake.script("PATCH /accounts/update_credentials");

    await rewrite(form.user, nameField(), "😀".repeat(30));

    expect(screen.getAllByText("0 characters left").length).toBeGreaterThan(0);
    expect(invalid()).toEqual([]);
    await saved(form);
    expect(sent.seen.map((request) => request.body)).toEqual([{ display_name: "😀".repeat(30) }]);
  });
});

describe("the fields", () => {
  /** How many Add a field the form offers. */
  const adds = () => screen.queryAllByRole("button", { name: "Add a field" }).length;

  test("Add a field stands once: before the pairs while there is one, and after the line that says there is none", async () => {
    const form = await profileForm();
    const none = "No fields yet. Add one to show something like a website or your pronouns on your profile.";
    /** Whether Add a field comes before `part` as the page is read. */
    const addBefore = (part: HTMLElement) =>
      (part.compareDocumentPosition(screen.getByRole("button", { name: "Add a field" })) &
        Node.DOCUMENT_POSITION_PRECEDING) !==
      0;
    expect(adds()).toBe(1);
    expect(addBefore(screen.getByRole("list", { name: "Fields" }))).toBe(true);
    expect(screen.queryAllByText(none)).toHaveLength(0);

    await form.user.click(screen.getByRole("button", { name: "Remove field 1" }));

    expect(pairs()).toEqual([]);
    expect(adds()).toBe(1);
    expect(addBefore(screen.getByText(none))).toBe(false);

    await form.user.click(screen.getByRole("button", { name: "Add a field" }));

    expect(pairs()).toEqual([["", ""]]);
    expect(adds()).toBe(1);
    expect(addBefore(screen.getByRole("list", { name: "Fields" }))).toBe(true);
    expect(screen.queryAllByText(none)).toHaveLength(0);
  });

  test("Add a field adds an empty pair after the last, up to four, and then is not offered", async () => {
    const form = await profileForm();

    for (const n of [2, 3, 4]) {
      expect(adds()).toBe(1);
      await form.user.click(screen.getByRole("button", { name: "Add a field" }));
      expect(pairs()).toHaveLength(n);
      expect(pairs().at(-1)).toEqual(["", ""]);
      expect(screen.getAllByRole("button", { name: `Remove field ${String(n)}` })).toHaveLength(1);
    }

    expect(adds()).toBe(0);
    expect(pairs()[0]).toEqual(["写真", "https://yuki.photos.test"]);
  });

  test("Remove of a pair takes that pair away, the pairs after it move up, and Add a field is offered again", async () => {
    const form = await profileForm();
    for (const [n, label] of [
      [2, "second"],
      [3, "third"],
      [4, "fourth"],
    ] as const) {
      await form.user.click(screen.getByRole("button", { name: "Add a field" }));
      await rewrite(form.user, pairField(n, "Label"), label);
    }
    expect(adds()).toBe(0);

    await form.user.click(screen.getByRole("button", { name: "Remove field 2" }));

    expect(pairs().map(([label]) => label)).toEqual(["写真", "third", "fourth"]);
    expect(screen.queryAllByRole("group", { name: "Field 4" })).toHaveLength(0);
    expect(adds()).toBe(1);
  });
});

describe("Save", () => {
  test("with nothing changed sends nothing, and says Saved", async () => {
    const form = await profileForm();
    const before = form.requests.length;

    await saved(form);

    expect(writes(form, before)).toEqual([]);
    expect(alerts()).toEqual([]);
  });

  // What the reader changes, and the one PATCH that carries it and nothing else.
  const changes: readonly {
    what: string;
    from?: FakeOptions;
    change: (form: DrawnForm) => Promise<void>;
    sends: object;
  }[] = [
    {
      what: "the display name",
      change: (form) => rewrite(form.user, nameField(), "Yuki Snow"),
      sends: { display_name: "Yuki Snow" },
    },
    {
      what: "a display name emptied",
      change: (form) => rewrite(form.user, nameField(), ""),
      sends: { display_name: "" },
    },
    {
      what: "the bio, with its line break",
      change: (form) => rewrite(form.user, bioField(), "Photos and walks.\nIn the mountains on weekends."),
      sends: { note: "Photos and walks.\nIn the mountains on weekends." },
    },
    {
      what: "the content of a field: the whole list",
      change: (form) => rewrite(form.user, pairField(1, "Content"), "https://yuki.example"),
      sends: { fields: [{ name: "写真", value: "https://yuki.example" }] },
    },
    {
      what: "a field added: the whole list, in its order",
      change: async (form) => {
        await form.user.click(screen.getByRole("button", { name: "Add a field" }));
        await rewrite(form.user, pairField(2, "Label"), "Pronouns");
        await rewrite(form.user, pairField(2, "Content"), "she/her");
      },
      sends: {
        fields: [
          { name: "写真", value: "https://yuki.photos.test" },
          { name: "Pronouns", value: "she/her" },
        ],
      },
    },
    {
      what: "a field added after a pair left blank: the list without the blank pair",
      change: async (form) => {
        await form.user.click(screen.getByRole("button", { name: "Add a field" }));
        await form.user.click(screen.getByRole("button", { name: "Add a field" }));
        await rewrite(form.user, pairField(3, "Label"), "Pronouns");
        await rewrite(form.user, pairField(3, "Content"), "she/her");
      },
      sends: {
        fields: [
          { name: "写真", value: "https://yuki.photos.test" },
          { name: "Pronouns", value: "she/her" },
        ],
      },
    },
    {
      what: "the only field removed: an empty list",
      change: (form) => form.user.click(screen.getByRole("button", { name: "Remove field 1" })),
      sends: { fields: [] },
    },
    {
      what: "the header removed: an empty ID in its place",
      change: (form) => form.user.click(image(header).getByRole("button", { name: "Remove" })),
      sends: { header_media_id: "" },
    },
    {
      what: "the avatar removed: an empty ID in its place",
      change: (form) => form.user.click(image(avatar).getByRole("button", { name: "Remove" })),
      sends: { avatar_media_id: "" },
    },
  ];

  for (const { what, from, change, sends } of changes)
    test(`sends one PATCH with only what changed: ${what}`, async () => {
      const form = await profileForm(from);
      const sent = form.fake.script("PATCH /accounts/update_credentials");
      const before = form.requests.length;
      await change(form);

      await saved(form);

      expect(sent.seen.map((request) => request.body)).toEqual([sends]);
      expect(writes(form, before)).toEqual(["PATCH /accounts/update_credentials"]);
    });

  test("with a field added and left blank sends nothing: a blank pair is no change", async () => {
    const form = await profileForm();
    const before = form.requests.length;
    await form.user.click(screen.getByRole("button", { name: "Add a field" }));

    await saved(form);

    expect(writes(form, before)).toEqual([]);
  });

  for (const [name, member] of [
    [header, "header_media_id"],
    [avatar, "avatar_media_id"],
  ] as const)
    test(`uploads the ${name.toLowerCase()} picked, then sends one PATCH that names the upload`, async () => {
      const form = await profileForm();
      const sent = form.fake.script("PATCH /accounts/update_credentials");
      const before = form.requests.length;
      await chooseFile(form.user, image(name).getByRole("button", { name: "Replace" }), imageFile("dusk.png"));
      // Picking sends nothing: the upload is Save's.
      expect(writes(form, before)).toEqual([]);

      // The server takes the change only for an upload of the reader's it knows, so Saved says the ID is the upload's.
      await saved(form);

      expect(writes(form, before)).toEqual(["POST /media", "PATCH /accounts/update_credentials"]);
      const [body, ...others] = sent.seen.map((request) => request.body);
      expect(others).toEqual([]);
      expect(Object.keys(body ?? {})).toEqual([member]);
      expect(body?.[member]).toBeTruthy();
    });

  test("with everything changed uploads both images, then sends the one PATCH with all of it", async () => {
    const form = await profileForm({ profile: { start: "empty" } });
    const sent = form.fake.script("PATCH /accounts/update_credentials");
    const before = form.requests.length;
    await chooseFile(form.user, image(header).getByRole("button", { name: "Choose an image" }), imageFile("h.png"));
    await chooseFile(form.user, image(avatar).getByRole("button", { name: "Choose an image" }), imageFile("a.png"));
    await rewrite(form.user, nameField(), "Yuki");
    await rewrite(form.user, bioField(), "Photos and walks.");
    await form.user.click(screen.getByRole("button", { name: "Add a field" }));
    await rewrite(form.user, pairField(1, "Label"), "Pronouns");
    await rewrite(form.user, pairField(1, "Content"), "she/her");

    await saved(form);

    expect(writes(form, before)).toEqual(["POST /media", "POST /media", "PATCH /accounts/update_credentials"]);
    const [body] = sent.seen.map((request) => request.body);
    expect(body).toMatchObject({
      display_name: "Yuki",
      note: "Photos and walks.",
      fields: [{ name: "Pronouns", value: "she/her" }],
    });
    expect(Object.keys(body ?? {}).toSorted()).toEqual([
      "avatar_media_id",
      "display_name",
      "fields",
      "header_media_id",
      "note",
    ]);
    expect(body?.avatar_media_id).toBeTruthy();
    expect(body?.header_media_id).toBeTruthy();
    expect(body?.avatar_media_id).not.toBe(body?.header_media_id);
  });

  test("once saved, the form holds the account as it is now: Save again sends nothing", async () => {
    const form = await profileForm();
    await rewrite(form.user, nameField(), "Yuki Snow");
    await chooseFile(form.user, image(avatar).getByRole("button", { name: "Replace" }), imageFile("snow.png"));
    await saved(form);
    const before = form.requests.length;

    await form.user.click(saveButton());
    await waitFor(() => {
      expect(said()).toEqual(["Saved"]);
    });

    expect(writes(form, before)).toEqual([]);
    expect(nameField().value).toBe("Yuki Snow");
    expect(imageButtons(avatar)).toEqual(["Replace", "Remove"]);
  });

  test("while the server has not answered, Save is busy and nothing of the form can be changed", async () => {
    const form = await profileForm({ profile: { save: "waits" } });
    await rewrite(form.user, nameField(), "Yuki Snow");

    await form.user.click(saveButton());

    await waitFor(() => {
      expect(saveButton().getAttribute("aria-busy")).toBe("true");
    });
    const buttons = [header, avatar].flatMap((name) => image(name).getAllByRole<HTMLButtonElement>("button"));
    expect(buttons.map((button) => button.disabled)).toEqual([true, true, true, true]);
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Add a field" }).disabled).toBe(true);
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Remove field 1" }).disabled).toBe(true);
    // A text field takes no change: disabled, or read-only so that it keeps the focus it has.
    const fixed = (field: HTMLInputElement | HTMLTextAreaElement) => field.disabled || field.readOnly;
    expect([nameField(), bioField(), pairField(1, "Label"), pairField(1, "Content")].map(fixed)).toEqual([
      true,
      true,
      true,
      true,
    ]);
    // A second press sends no second change.
    const sent = form.fake.script("PATCH /accounts/update_credentials");
    await form.user.click(saveButton());
    expect(sent.seen).toHaveLength(0);
  });
});

describe("the form tells its owner whether it holds unsaved changes", () => {
  test("none when it opens, some once something is written, and none again once it is saved", async () => {
    const form = await profileForm();
    expect(form.told.filter((word) => word.startsWith("unsaved")).at(-1)).not.toBe("unsaved:true");

    await form.user.type(nameField(), "!");
    await waitFor(() => {
      expect(form.told.at(-1)).toBe("unsaved:true");
    });

    await saved(form);
    await waitFor(() => {
      expect(form.told.at(-1)).toBe("unsaved:false");
    });
  });

  test("some once an image is removed", async () => {
    const form = await profileForm();

    await form.user.click(image(header).getByRole("button", { name: "Remove" }));

    await waitFor(() => {
      expect(form.told.at(-1)).toBe("unsaved:true");
    });
  });

  test("some once an image is picked", async () => {
    const form = await profileForm();

    await chooseFile(form.user, image(avatar).getByRole("button", { name: "Replace" }), imageFile("snow.png"));

    await waitFor(() => {
      expect(form.told.at(-1)).toBe("unsaved:true");
    });
  });

  test("none once what was written is put back as it was", async () => {
    const form = await profileForm();
    await form.user.type(nameField(), "!");
    await waitFor(() => {
      expect(form.told.at(-1)).toBe("unsaved:true");
    });

    await form.user.type(nameField(), "{Backspace}");

    await waitFor(() => {
      expect(form.told.at(-1)).toBe("unsaved:false");
    });
  });

  test("a save the server refuses leaves them unsaved", async () => {
    const form = await profileForm({ profile: { save: "refused" } });
    await form.user.type(nameField(), "!");

    await form.user.click(saveButton());
    await waitFor(() => {
      expect(alerts()).not.toEqual([]);
    });

    expect(form.told.filter((word) => word.startsWith("unsaved")).at(-1)).toBe("unsaved:true");
    expect(nameField().value).toBe("Yuki 雪!");
  });
});

describe("what the server refuses shows where it belongs", () => {
  /** The form with one more pair written, and a script that refuses the next save with `problem`. */
  async function refused(problem: ReturnType<typeof refusal>) {
    const form = await profileForm();
    await form.user.click(screen.getByRole("button", { name: "Add a field" }));
    await rewrite(form.user, pairField(2, "Label"), "Pronouns");
    await rewrite(form.user, pairField(2, "Content"), "she/her");
    await rewrite(form.user, nameField(), "Yuki Snow");
    await rewrite(form.user, bioField(), "Photos and walks.");
    form.fake.script("PATCH /accounts/update_credentials", { times: 1, fail: problem });
    await form.user.click(saveButton());
    await waitFor(() => {
      expect(alerts()).not.toEqual([]);
    });
    return form;
  }

  // The member the problem names, and the one input that is marked and described by the server's words.
  const named = [
    ["display_name", "Display name", nameField],
    ["note", "Bio", bioField],
    ["fields[0].name", "Field 1: Label", () => pairField(1, "Label")],
    ["fields[1].name", "Field 2: Label", () => pairField(2, "Label")],
    ["fields[1].value", "Field 2: Content", () => pairField(2, "Content")],
  ] as const;

  for (const [member, name, field] of named)
    test(`${member}: the server's words under ${name}, which alone is marked, and the problem's title beside Save`, async () => {
      await refused(refusal(member, "The server does not take this here."));

      expect(invalid()).toEqual([name]);
      expect(describedBy(field())).toEqual(["The server does not take this here."]);
      expect(alerts()).toEqual(["This could not be done."]);
      // Nothing was saved, and what the reader wrote stays.
      expect(said().includes("Saved")).toBe(false);
      expect(nameField().value).toBe("Yuki Snow");
      expect(pairs()).toEqual([
        ["写真", "https://yuki.photos.test"],
        ["Pronouns", "she/her"],
      ]);
    });

  test("fields, as a whole: the server's words describe the list, and no input is marked", async () => {
    await refused(refusal("fields", "You can have at most 4."));

    expect(invalid()).toEqual([]);
    expect(describedBy(screen.getByRole("list", { name: "Fields" }))).toEqual(["You can have at most 4."]);
    expect(alerts()).toEqual(["This could not be done."]);
  });

  test("the display name the server refuses (the story's server): its words under Display name", async () => {
    const form = await profileForm({ profile: { save: "name" } });
    await form.user.type(nameField(), "!");

    await form.user.click(saveButton());

    await waitFor(() => {
      expect(invalid()).toEqual(["Display name"]);
    });
    expect(describedBy(nameField())).toEqual(["This value is not valid. Check what you entered."]);
    expect(alerts()).toEqual(["This could not be done."]);
  });

  // The server names a part of a field without saying which field: the form finds the first field it sent that the
  // server's rule refuses (a control character; in a label a tab is one).
  for (const [part, text] of [
    ["Label", "Pro\tnouns"],
    ["Content", "she\u0007her"],
  ] as const)
    test(`a field's ${part.toLowerCase()} the server refuses without saying which field: the first field sent that breaks the rule is marked, a blank pair before it not counted`, async () => {
      const form = await profileForm();
      const sent = form.fake.script("PATCH /accounts/update_credentials");
      // Field 2 stays blank and is not sent; fields 3 and 4 both break the rule.
      for (const n of [2, 3, 4]) {
        await form.user.click(screen.getByRole("button", { name: "Add a field" }));
        expect(pairs()).toHaveLength(n);
      }
      for (const n of [3, 4]) {
        await rewrite(form.user, pairField(n, "Label"), part === "Label" ? text : "Pronouns");
        await rewrite(form.user, pairField(n, "Content"), part === "Content" ? text : "she/her");
      }

      await form.user.click(saveButton());

      await waitFor(() => {
        expect(invalid()).toEqual([`Field 3: ${part}`]);
      });
      expect(describedBy(pairField(3, part))).toEqual(["This value is not valid. Check what you entered."]);
      expect(alerts()).toEqual(["This could not be done."]);
      expect(sent.seen).toHaveLength(1);
    });

  test("a field the server names by its place among those sent is marked in its own pair, a blank pair before it not counted", async () => {
    const form = await profileForm();
    await form.user.click(screen.getByRole("button", { name: "Add a field" }));
    await form.user.click(screen.getByRole("button", { name: "Add a field" }));
    await rewrite(form.user, pairField(3, "Label"), "Pronouns");
    await rewrite(form.user, pairField(3, "Content"), "she/her");
    // The second of the two fields sent is the form's third pair.
    form.fake.script("PATCH /accounts/update_credentials", {
      times: 1,
      fail: refusal("fields[1].value", "The server does not take this here."),
    });

    await form.user.click(saveButton());

    await waitFor(() => {
      expect(invalid()).toEqual(["Field 3: Content"]);
    });
    expect(describedBy(pairField(3, "Content"))).toEqual(["The server does not take this here."]);
  });

  for (const [name, other] of [
    [header, avatar],
    [avatar, header],
  ] as const) {
    test(`an upload the server refuses: its words inside the group of the ${name.toLowerCase()}, the bar says an image could not be used, and the profile is not changed`, async () => {
      const form = await profileForm({ uploads: "refused" });
      const before = form.requests.length;
      await rewrite(form.user, nameField(), "Yuki Snow");
      await chooseFile(form.user, image(name).getByRole("button", { name: "Replace" }), imageFile("dusk.png"));

      await form.user.click(saveButton());

      await waitFor(() => {
        expect(alerts()).toEqual([
          "Your profile was not saved, because an image could not be used. See the note under the image.",
        ]);
      });
      expect(describedBy(screen.getByRole("group", { name }))).toContain(
        "This file is not an image this server can use.",
      );
      expect(describedBy(screen.getByRole("group", { name: other }))).not.toContain(
        "This file is not an image this server can use.",
      );
      expect(invalid()).toEqual([]);
      expect(writes(form, before)).toEqual(["POST /media"]);
      // The picture picked is still shown, for the reader to replace.
      expect(pictures(name)[0]).toMatch(/^blob:/);
      expect(nameField().value).toBe("Yuki Snow");
    });

    for (const fail of [500, "network"] as const)
      test(`an upload of the ${name.toLowerCase()} that fails (${String(fail)}): the form's own sentence inside its group, and the profile is not changed`, async () => {
        const form = await profileForm();
        form.fake.script("POST /media", { fail });
        const before = form.requests.length;
        await chooseFile(form.user, image(name).getByRole("button", { name: "Replace" }), imageFile("dusk.png"));

        await form.user.click(saveButton());

        await waitFor(() => {
          expect(alerts()).toEqual([
            "Your profile was not saved, because an image could not be used. See the note under the image.",
          ]);
        });
        expect(describedBy(screen.getByRole("group", { name }))).toContain(
          "This image could not be uploaded. Save again, or choose another one.",
        );
        expect(describedBy(screen.getByRole("group", { name: other }))).toEqual([]);
        expect(writes(form, before)).toEqual(["POST /media"]);
      });
  }

  test("an upload that failed is sent again by the next Save, and the profile is saved with it", async () => {
    const form = await profileForm();
    const failing = form.fake.script("POST /media", { times: 1, fail: 500 });
    const sent = form.fake.script("PATCH /accounts/update_credentials");
    await chooseFile(form.user, image(avatar).getByRole("button", { name: "Replace" }), imageFile("dusk.png"));
    await form.user.click(saveButton());
    await waitFor(() => {
      expect(alerts()).not.toEqual([]);
    });
    failing.end();

    await saved(form);

    expect(alerts()).toEqual([]);
    expect(describedBy(screen.getByRole("group", { name: avatar }))).toEqual([]);
    expect(sent.seen).toHaveLength(1);
    expect(sent.seen[0]?.body.avatar_media_id).toBeTruthy();
  });

  test("the review's refusal: the server's words beside Save, and no input is marked", async () => {
    const form = await profileForm({ profile: { save: "refused" } });
    await form.user.type(nameField(), "!");

    await form.user.click(saveButton());

    await waitFor(() => {
      expect(alerts()).toEqual([
        "This server's automated review did not accept this change. Your profile stays as it was.",
      ]);
    });
    expect(invalid()).toEqual([]);
    expect(said().includes("Saved")).toBe(false);
  });

  test("a save the server fails says so beside Save, and the next Save sends the change again", async () => {
    const form = await profileForm();
    const sent = form.fake.script("PATCH /accounts/update_credentials");
    const failing = form.fake.script("PATCH /accounts/update_credentials", { times: 1, fail: 500 });
    await form.user.type(nameField(), "!");

    await form.user.click(saveButton());
    await waitFor(() => {
      expect(alerts()).toHaveLength(1);
    });
    expect(invalid()).toEqual([]);
    expect(said().includes("Saved")).toBe(false);
    failing.end();

    await saved(form);

    expect(alerts()).toEqual([]);
    expect(sent.seen.map((request) => request.body)).toEqual([
      { display_name: "Yuki 雪!" },
      { display_name: "Yuki 雪!" },
    ]);
  });

  test("what was refused is no longer said once the next Save is taken", async () => {
    const form = await refused(refusal("fields[1].value", "The server does not take this here."));

    await saved(form);

    expect(invalid()).toEqual([]);
    expect(alerts()).toEqual([]);
    expect(describedBy(pairField(2, "Content"))).toEqual([]);
  });
});

describe("leaving with unsaved changes", () => {
  /** The confirmation the form shows while its owner says the reader is leaving. */
  const asking = () => screen.getByRole("alertdialog", { name: "Leave without saving?" });

  test("the form asks first, in an alert dialog that says what is lost, with Keep editing and Leave", async () => {
    await profileForm({}, { leaving: true });

    expect(describedBy(asking())).toEqual(["The changes you have not saved will be lost. This cannot be undone."]);
    expect(
      within(asking())
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label") ?? button.textContent),
    ).toEqual(["Close", "Keep editing", "Leave"]);
  });

  for (const [button, tells] of [
    ["Keep editing", "stay"],
    ["Close", "stay"],
    ["Leave", "leave"],
  ] as const)
    test(`${button} tells the owner to ${tells}`, async () => {
      const form = await profileForm({}, { leaving: true });

      await form.user.click(within(asking()).getByRole("button", { name: button }));

      expect(form.told.filter((word) => word === "stay" || word === "leave")).toEqual([tells]);
    });

  test("no confirmation shows while the reader is not leaving", async () => {
    const form = await profileForm();
    await form.user.type(nameField(), "!");

    expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
  });
});
