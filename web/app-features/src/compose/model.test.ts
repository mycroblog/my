// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { defaultComposeSettings, type ComposeSettings, type Status } from "@my/app-data";
import { answering, emptyDraft, postable, problemsOf, toDraft, toStatus, written, type Draft } from "./model";

const settings: ComposeSettings = { ...defaultComposeSettings, maxChars: 10, viewer: { id: "me", acct: "kumi" } };
const draft = (fields: Partial<Draft> = {}): Draft => ({ ...emptyDraft({ kind: "new" }, settings, "key"), ...fields });

const status = (fields: Partial<Status>): Status =>
  ({
    id: "9",
    account: { id: "a", acct: "alice@example.social" },
    mentions: [],
    visibility: "public",
    ...fields,
  }) as unknown as Status;

test("a reply mentions its author and the accounts it mentions, but the reader, and keeps a narrow audience", () => {
  const reply = draft({ target: { kind: "reply", id: "9" } });
  const answered = answering(
    reply,
    status({
      visibility: "private",
      mentions: [
        { id: "me", acct: "kumi" },
        { id: "b", acct: "bob" },
      ] as Status["mentions"],
    }),
    settings.viewer,
  );
  expect(answered.text).toBe("@alice@example.social @bob ");
  expect(answered.visibility).toBe("private");
  // What the reader wrote already stays.
  expect(answering({ ...reply, text: "hi" }, status({}), settings.viewer).text).toBe("hi");
});

test("a quote of a remote post leaves an audience that may not quote it", () => {
  const quote = draft({ target: { kind: "quote", id: "9" }, visibility: "private" });
  expect(answering(quote, status({}), settings.viewer).visibility).toBe("unlisted");
  const local = status({ account: { id: "a", acct: "alice" } as Status["account"] });
  expect(answering(quote, local, settings.viewer).visibility).toBe("private");
});

test("a draft is worth keeping once it holds text, a warning, an image or a poll", () => {
  expect(written(draft())).toBe(false);
  expect(written(draft({ text: "  " }))).toBe(false);
  expect(written(draft({ text: "hi" }))).toBe(true);
  expect(written(draft({ cw: "spoiler" }))).toBe(true);
  expect(written(draft({ poll: { options: ["", ""], duration: 300, multiple: false, hideTotals: false } }))).toBe(true);
});

test("what keeps a post from being sent: the limit with the warning, uploads, a poll's options, nothing written", () => {
  expect(problemsOf(draft({ text: "12345", cw: "1234567" }), settings).over).toBe(2);
  expect(postable(problemsOf(draft(), settings))).toBe(false);
  expect(postable(problemsOf(draft({ text: "hi" }), settings))).toBe(true);
  const uploading = draft({ text: "hi", attachments: [{ key: "a", state: "uploading", description: "" }] });
  expect(problemsOf(uploading, settings).waiting).toBe(true);
  const poll = (options: string[]) =>
    problemsOf(draft({ text: "hi", poll: { options, duration: 300, multiple: false, hideTotals: false } }), settings);
  expect(poll(["a", ""]).pollInvalid).toBe(true);
  expect(poll(["a", "a"]).pollInvalid).toBe(true);
  expect(poll(["a", "b"]).pollInvalid).toBe(false);
  const described = { key: "a", state: "ready" as const, description: "" };
  expect(problemsOf(draft({ text: "hi", attachments: [described] }), settings).missingDescriptions).toBe(false);
  const required = { ...settings, requireDescriptions: true };
  expect(problemsOf(draft({ text: "hi", attachments: [described] }), required).missingDescriptions).toBe(true);
});

test("the post sends what was chosen, the permissions only where they differ from the reader's defaults", () => {
  const media = { id: "m1", type: "image", state: "ready", meta: { focus: { x: 0, y: 0 } } } as const;
  const body = toStatus(
    draft({
      target: { kind: "reply", id: "9" },
      text: "hi",
      cw: " spoiler ",
      language: "ja",
      visibility: "unlisted",
      policy: { reply: "everyone", announce: "followers", quote: "nobody" },
      attachments: [{ key: "a", state: "ready", media, description: " a hill " }],
    }),
    settings,
  );
  expect(body).toEqual({
    text: "hi",
    visibility: "unlisted",
    spoiler_text: "spoiler",
    sensitive: true,
    language: "ja",
    in_reply_to_id: "9",
    media: [{ id: "m1", description: "a hill" }],
    interaction_policy: { announce: { automatic: "followers" }, quote: { automatic: "nobody" } },
  });
  expect(toStatus(draft({ text: "hi" }), settings)).toEqual({ text: "hi", visibility: "public" });
});

test("a quote sends the quoted post until the reader removes it; a poll sends its options and duration", () => {
  const quote = draft({ target: { kind: "quote", id: "9" }, text: "look" });
  expect(toStatus(quote, settings).quoted_status_id).toBe("9");
  expect(toStatus({ ...quote, quoteRemoved: true }, settings).quoted_status_id).toBeUndefined();
  const poll = toStatus(
    draft({ text: "when?", poll: { options: [" Sat ", "Sun"], duration: 3600, multiple: true, hideTotals: false } }),
    settings,
  );
  expect(poll.poll).toEqual({ options: ["Sat", "Sun"], expires_in: 3600, multiple: true, hide_totals: false });
});

test("a server draft keeps a poll still being written as it is", () => {
  const body = toDraft(
    draft({ text: "when?", poll: { options: ["Sat", ""], duration: 3600, multiple: false, hideTotals: false } }),
    settings,
  );
  expect(body.poll?.options).toEqual(["Sat", ""]);
  expect(body.text).toBe("when?");
});
