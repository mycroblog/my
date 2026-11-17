// SPDX-License-Identifier: AGPL-3.0-only

// What a profile's head and the lines under it are to a reader and to assistive technology: a region named by the
// account's name, its handle read left to right, its fields, its counts, the reader's note and its editor, and the
// hashtags it features. That the head lays out in the reader's direction is a layout test's.

import { expect, test } from "bun:test";
import { Badge, Button, I18nProvider } from "@my/design-system";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { FieldList, MetaCount, MetaLine, NoteEditor, NoteLine, ProfileHead, TagList } from "./profile";

test("a profile's head is a region named by the account's name, its heading; the avatar's row comes before the names", () => {
  render(
    <ProfileHead
      nameId="profile-name"
      banner="https://m.example/banner.webp"
      avatar={<span>Avatar</span>}
      actions={<Button>Follow</Button>}
      name="Alice Chen"
      marks={<span>Locked</span>}
      handle="@alice@example.social"
      badges={<Badge>Follows you</Badge>}
    >
      <p>Bread and bicycles.</p>
    </ProfileHead>,
  );
  const head = screen.getByRole("region", { name: "Alice Chen" });
  const part = within(head);
  const heading = part.getByRole("heading", { level: 2, name: /^Alice Chen\s*Locked$/ });

  expect(part.getByText("Alice Chen").id).toBe("profile-name");
  const follow = part.getByRole("button", { name: "Follow" });
  expect(follow.compareDocumentPosition(heading) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  expect(part.getByText("Avatar")).not.toBeNull();
  expect(part.getByText("Bread and bicycles.")).not.toBeNull();
  // The banner is decoration.
  expect(part.queryByRole("img")).toBeNull();
});

test("the handle reads left to right and a badge in its own direction, whatever the reader's", () => {
  render(
    <I18nProvider locale="ar">
      <ProfileHead
        nameId="profile-name"
        avatar={<span>Avatar</span>}
        name="Alice Chen"
        handle="@alice@example.social"
        badges={<Badge>يتابعك</Badge>}
      />
    </I18nProvider>,
  );

  const handle = screen.getByText("@alice@example.social");
  expect(handle.tagName).toBe("BDI");
  expect(handle.dir).toBe("ltr");
  const badge = screen.getByText("يتابعك");
  expect(badge.tagName).toBe("BDI");
  expect(badge.dir).toBe("auto");
});

test("a profile's fields are terms with their values", () => {
  render(
    <FieldList
      items={[
        { name: "Site", value: <a href="https://alice.example">alice.example</a> },
        { name: "Pronouns", value: "she/her" },
      ]}
    />,
  );

  expect(screen.getAllByRole("term").map((term) => term.textContent)).toEqual(["Site", "Pronouns"]);
  const [site, pronouns] = screen.getAllByRole("definition");
  if (site === undefined || pronouns === undefined) throw new Error("the list has no two values");
  expect(within(site).getByRole("link", { name: "alice.example" }).getAttribute("href")).toBe("https://alice.example");
  expect(pronouns.textContent).toBe("she/her");
});

test("a count reads as its label then its value, and is a link where it opens a list", () => {
  render(
    <MetaLine>
      Joined April 2025
      <MetaCount label="Posts" value="1.2K" />
      <MetaCount label="Following" value="12" href="/@alice/following" />
    </MetaLine>,
  );

  expect(screen.getByText(/Joined April 2025/)).not.toBeNull();
  expect(screen.getByText("1.2K").parentElement?.textContent).toMatch(/^Posts\s*1\.2K$/);
  const following = screen.getByRole("link", { name: /^Following\s*12$/ });
  expect(following.getAttribute("href")).toBe("/@alice/following");
  expect(screen.getAllByRole("link")).toEqual([following]);
});

test("the reader's note is a button named by its label and the note, which opens its editor", async () => {
  const user = userEvent.setup();
  let opened = 0;
  render(
    <NoteLine note="Met at the fair" placeholder="Add a private note" label="Your note:" onPress={() => opened++} />,
  );

  await user.click(screen.getByRole("button", { name: /^Your note:\s*Met at the fair$/ }));

  expect(opened).toBe(1);
});

test("with no note, the line says what adding one does", async () => {
  const user = userEvent.setup();
  let opened = 0;
  render(<NoteLine note="" placeholder="Add a private note" label="Your note:" onPress={() => opened++} />);

  await user.tab();
  expect(document.activeElement === screen.getByRole("button", { name: "Add a private note" })).toBe(true);
  await user.keyboard("{Enter}");

  expect(opened).toBe(1);
});

/** The note's editor as a caller holds it, and what it asked of the caller, in order. */
function Editor({ asked, busy }: { asked: string[]; busy?: boolean }) {
  const [value, setValue] = useState("Met at the fai");
  return (
    <NoteEditor
      label="Your note"
      value={value}
      onChange={(next) => {
        asked.push(`change ${next}`);
        setValue(next);
      }}
      maxLength={16}
      cancel="Cancel"
      save="Save"
      onCancel={() => asked.push("cancel")}
      onSave={() => asked.push("save")}
      busy={busy}
    />
  );
}

test("the note's editor is a box named by its label that takes no more than its limit, then Cancel and Save", async () => {
  const user = userEvent.setup();
  const asked: string[] = [];
  render(<Editor asked={asked} />);
  const box = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Your note" });

  expect(box.value).toBe("Met at the fai");
  await user.type(box, "r!!!");
  expect(box.value).toBe("Met at the fair!");
  await user.tab();
  expect(document.activeElement === screen.getByRole("button", { name: "Cancel" })).toBe(true);
  await user.keyboard("{Enter}");
  await user.tab();
  expect(document.activeElement === screen.getByRole("button", { name: "Save" })).toBe(true);
  await user.keyboard("{Enter}");

  expect(asked).toEqual(["change Met at the fair", "change Met at the fair!", "cancel", "save"]);
});

test("while the note is saved, Save is busy, Cancel is disabled, and neither answers", async () => {
  const user = userEvent.setup();
  const asked: string[] = [];
  render(<Editor asked={asked} busy />);

  const save = screen.getByRole("button", { name: "Save", busy: true });
  const cancel = screen.getByRole<HTMLButtonElement>("button", { name: "Cancel" });
  expect(cancel.disabled).toBe(true);
  await user.click(save);
  await user.click(cancel);

  expect(asked).toEqual([]);
});

test("the featured hashtags are a list of links under their heading, each with its # and its count", () => {
  render(
    <TagList
      heading="Featured hashtags"
      items={[
        { name: "bread", href: "/@alice/tagged/bread", count: "12 posts" },
        { name: "山", href: "/@alice/tagged/%E5%B1%B1", count: "3 posts" },
      ]}
    />,
  );

  expect(screen.getByRole("heading", { level: 3, name: "Featured hashtags" })).not.toBeNull();
  const list = within(screen.getByRole("list"));
  expect(list.getAllByRole("listitem")).toHaveLength(2);
  expect(list.getByRole("link", { name: /^#bread\s*12 posts$/ }).getAttribute("href")).toBe("/@alice/tagged/bread");
  expect(list.getByRole("link", { name: /^#山\s*3 posts$/ }).getAttribute("href")).toBe("/@alice/tagged/%E5%B1%B1");
});
