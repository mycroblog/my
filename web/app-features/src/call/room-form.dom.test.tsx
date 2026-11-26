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

/** The settings of the room the reader hosts (the fake's third), on a fake of `options`. */
async function settings(options: FakeOptions = {}) {
  const on = fakeData(options);
  const id = on.fake.voice.roomId(2);
  const patch = on.fake.script("PATCH /voice/rooms/{id}");
  const user = renderFeature(
    on.data,
    <main>
      <VoiceRoomSettingsScreen data={on.data} id={id} sheet={false} onBack={() => undefined} />
    </main>,
  );
  const field = await screen.findByRole("textbox", { name: "How many can be in the room" });
  return { ...on, patch, user, field };
}

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
});

test("a maximum raised past what the media node has room for now (503) is said under the field", async () => {
  const { user, field, patch } = await settings({ voiceMax: { rooms: [undefined, undefined, 6], noRoom: true } });
  expect(field).toHaveProperty("value", "6");
  await user.clear(field);
  await user.type(field, "8");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByText(
    "This server has no space for that many people right now, so the settings were not saved. Lower how many can be in the room, or try again later.",
  );
  expect(patch.seen[0]?.body.max_participants).toBe(8);
});
