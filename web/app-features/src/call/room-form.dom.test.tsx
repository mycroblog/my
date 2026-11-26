// SPDX-License-Identifier: AGPL-3.0-only

// How many people a voice chat room holds, in its form (ADR-0100 §3): what a send holds of the field, what the form
// refuses before sending, and what it says of the server's refusals, on the settings of the room the reader hosts.

import { describe, expect, test } from "bun:test";
import { screen, waitFor } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { maxParticipantsBroken, maxParticipantsSent, VoiceRoomSettingsScreen } from "./room-form";
import type { FakeOptions } from "@my/e2e/fake";

describe("what a send holds of the maximum", () => {
  test("a new room's holds only the number entered: an empty field leaves it to the server", () => {
    expect(maxParticipantsSent(null, null, true, 10)).toEqual({});
    expect(maxParticipantsSent(5, null, true, 10)).toEqual({ max_participants: 5 });
  });

  test("an open room's holds it only once changed, and an emptied one as the server's maximum", () => {
    expect(maxParticipantsSent(10, 10, false, 10)).toEqual({});
    expect(maxParticipantsSent(4, 10, false, 10)).toEqual({ max_participants: 4 });
    expect(maxParticipantsSent(null, 6, false, 10)).toEqual({ max_participants: 10 });
    expect(maxParticipantsSent(null, 6, false, undefined)).toEqual({});
  });
});

test("a maximum breaks the rule below 2, above the server's or not whole; an empty one breaks none", () => {
  expect(maxParticipantsBroken(null, 10)).toBe(false);
  expect(maxParticipantsBroken(1, 10)).toBe(true);
  expect(maxParticipantsBroken(2, 10)).toBe(false);
  expect(maxParticipantsBroken(10, 10)).toBe(false);
  expect(maxParticipantsBroken(11, 10)).toBe(true);
  expect(maxParticipantsBroken(2.5, 10)).toBe(true);
  expect(maxParticipantsBroken(11, undefined)).toBe(false);
});

const rule = "Enter a number from 2 to 10 for how many can be in the room, or leave it empty.";

/** The settings' words of the maximum in each locale the done-when names. */
const said = {
  en: {
    field: "How many can be in the room",
    hint: "2 to 10. Leave it empty for this server's maximum, 10.",
    rule,
    save: "Save",
    noRoom:
      "This server has no space for that many people right now, so the settings were not saved. Lower how many can be in the room, or try again later.",
  },
  ja: {
    field: "参加できる人数",
    hint: "2〜10 人。空欄のときは、このサーバーの上限の 10 人です。",
    rule: "参加できる人数には 2〜10 の数を入れるか、空欄にしてください。",
    save: "保存",
    noRoom:
      "いまはサーバーにその人数分の空きがないため、設定を保存しませんでした。参加できる人数を減らすか、しばらくしてからもう一度試してください。",
  },
} as const;

/** The settings of the room the reader hosts (the fake's third), on a fake of `options`, in `locale`. */
async function settings(options: FakeOptions = {}, locale: keyof typeof said = "en") {
  const on = fakeData(options);
  const id = on.fake.voice.roomId(2);
  const patch = on.fake.script("PATCH /voice/rooms/{id}");
  const user = renderFeature(
    on.data,
    <main>
      <VoiceRoomSettingsScreen data={on.data} id={id} sheet={false} onBack={() => undefined} />
    </main>,
    locale,
  );
  const field = await screen.findByRole("textbox", { name: said[locale].field });
  /** The room's maximum as the server holds it. */
  const held = async () => {
    const { data } = await on.data.api.GET("/voice/rooms/{id}", { params: { path: { id } } });
    return data?.max_participants;
  };
  return { ...on, id, patch, user, field, held };
}

/** What the field's error element says; null while it has none. */
const errorOf = (field: HTMLElement) => document.getElementById(`${field.id}-error`)?.textContent ?? null;

test("the settings hold the room's maximum, and refuse one out of range before sending, under the field", async () => {
  const { user, field, patch } = await settings();
  expect(field).toHaveProperty("value", "10");
  expect(screen.getByText("2 to 10. Leave it empty for this server's maximum, 10.")).not.toBeNull();
  await user.clear(field);
  await user.type(field, "60");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByText(rule);
  // The number stays as entered, the error in the hint's place, the field marked and focused; nothing sent.
  expect(field).toHaveProperty("value", "60");
  expect(screen.queryByText("2 to 10. Leave it empty for this server's maximum, 10.")).toBeNull();
  expect(field.getAttribute("aria-invalid")).toBe("true");
  expect(document.activeElement).toBe(field);
  expect(patch.seen).toHaveLength(0);
  // Put right, the mark goes, and Save sends the number.
  await user.clear(field);
  await user.type(field, "5");
  await user.tab();
  await waitFor(() => {
    expect(screen.queryByText(rule)).toBeNull();
  });
  await user.click(screen.getByRole("button", { name: "Save" }));
  const sent = await patch.served();
  expect(sent.body.max_participants).toBe(5);
});

test("a maximum the server refuses (422 max_participants) says the rule under the field", async () => {
  const { user, field, fake } = await settings();
  fake.script("PATCH /voice/rooms/{id}", {
    fail: { type: "about:blank", title: "This value is not valid", status: 422, field: "max_participants" },
  });
  await user.clear(field);
  await user.type(field, "4");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByText(rule);
  // The field is marked, its error in its own element, and it takes the focus.
  expect(field.getAttribute("aria-invalid")).toBe("true");
  expect(errorOf(field)).toBe(rule);
  await waitFor(() => {
    expect(document.activeElement).toBe(field);
  });
});

describe.each(["en", "ja"] as const)("the settings' maximum in %s", (locale) => {
  const words = said[locale];

  test("the field holds the room's maximum, described by its hint, and refuses one out of range under it", async () => {
    const { user, field, patch } = await settings({}, locale);
    expect(field).toHaveProperty("value", "10");
    expect(
      (field.getAttribute("aria-describedby") ?? "").split(" ").map((id) => document.getElementById(id)?.textContent),
    ).toContain(words.hint);
    await user.clear(field);
    await user.type(field, "1");
    await user.click(screen.getByRole("button", { name: words.save }));
    await waitFor(() => {
      expect(errorOf(field)).toBe(words.rule);
    });
    expect(field.getAttribute("aria-invalid")).toBe("true");
    expect(document.activeElement).toBe(field);
    expect(patch.seen).toHaveLength(0);
  });

  test("a maximum raised past what the media node has room for now (503) is said under the field", async () => {
    const { user, field, patch, held } = await settings(
      { voiceMax: { rooms: [undefined, undefined, 6], noRoom: true } },
      locale,
    );
    expect(field).toHaveProperty("value", "6");
    await user.clear(field);
    await user.type(field, "8");
    await user.click(screen.getByRole("button", { name: words.save }));
    await screen.findByText(words.noRoom);
    expect(errorOf(field)).toBe(words.noRoom);
    expect(patch.seen[0]?.body.max_participants).toBe(8);
    expect(await held()).toBe(6);
  });
});

describe("what Save sends of the maximum, and what the server then holds", () => {
  test("raised within the server's maximum: the number, saved", async () => {
    const { user, field, patch, held } = await settings({ voiceMax: { rooms: [undefined, undefined, 6] } });
    await user.clear(field);
    await user.type(field, "9");
    await user.click(screen.getByRole("button", { name: "Save" }));
    const sent = await patch.served();
    expect(sent.body.max_participants).toBe(9);
    expect(await held()).toBe(9);
  });

  test("lowered below the people in the room: the number, saved, and nobody is refused for it", async () => {
    const { user, field, patch, held, data, id } = await settings();
    const { data: room } = await data.api.GET("/voice/rooms/{id}", { params: { path: { id } } });
    const inIt = room?.participants.length ?? 0;
    expect(inIt).toBeGreaterThan(2);
    await user.clear(field);
    await user.type(field, "2");
    await user.click(screen.getByRole("button", { name: "Save" }));
    const sent = await patch.served();
    expect(sent.body.max_participants).toBe(2);
    expect(await held()).toBe(2);
    expect(errorOf(field)).toBeNull();
  });

  test("emptied: the server's maximum", async () => {
    const { user, field, patch, held } = await settings({ voiceMax: { rooms: [undefined, undefined, 6] } });
    await user.clear(field);
    await user.click(screen.getByRole("button", { name: "Save" }));
    const sent = await patch.served();
    expect(sent.body.max_participants).toBe(10);
    expect(await held()).toBe(10);
  });

  test("left as it was, while another setting changes: nothing of the maximum", async () => {
    const { user, patch } = await settings({ voiceMax: { rooms: [undefined, undefined, 6] } });
    const name = screen.getByRole("textbox", { name: "Name" });
    await user.click(name);
    await user.paste(", again");
    await user.click(screen.getByRole("button", { name: "Save" }));
    const sent = await patch.served();
    expect(sent.body.name).toMatch(/, again$/);
    expect("max_participants" in sent.body).toBe(false);
  });
});
