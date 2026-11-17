// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import {
  defaultComposeSettings,
  type ComposeSettings,
  type MediaAttachment,
  type PostSource,
  type ServerDraft,
  type Status,
} from "@my/app-data";
import { inputOf } from "../field-errors";
import {
  answering,
  composerMembers,
  emptyDraft,
  fromServer,
  onServer,
  postable,
  problemsOf,
  toDraft,
  toEdit,
  toStatus,
  written,
  type Attachment,
  type Draft,
} from "./model";

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
      policy: {
        reply: { audience: "everyone", approval: true },
        announce: { audience: "followers", approval: false },
        quote: { audience: "nobody", approval: false },
      },
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

test("the post sends each kind whose others may ask for approval with everyone else by approval", () => {
  const body = toStatus(
    draft({
      text: "hi",
      policy: {
        reply: { audience: "followers", approval: true },
        announce: { audience: "mentioned", approval: true },
        quote: { audience: "nobody", approval: true },
      },
    }),
    settings,
  );
  expect(body.interaction_policy).toEqual({
    reply: { automatic: "followers", manual: "everyone" },
    announce: { automatic: "mentioned", manual: "everyone" },
    quote: { automatic: "nobody", manual: "everyone" },
  });
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

test("an edit and a redraft keep a post's images marked sensitive without a warning", () => {
  const media = { id: "m1", type: "image", url: "/m1.png", description: "a hill" } as PostSource["media"][number];
  const source: PostSource = {
    id: "9",
    text: "hi",
    spoiler_text: "",
    language: "en",
    visibility: "public",
    sensitive: true,
    media: [media],
  };
  const edit = emptyDraft({ kind: "edit", id: "9", source }, settings, "key");
  expect(toEdit(edit).sensitive).toBe(true);
  expect(toEdit({ ...edit, attachments: [] }).sensitive).toBe(false);
  const redraft = emptyDraft({ kind: "redraft", id: "9", source }, settings, "key");
  expect(toStatus(redraft, settings).sensitive).toBe(true);
  const plain = emptyDraft({ kind: "edit", id: "9", source: { ...source, sensitive: false } }, settings, "key");
  expect(toEdit(plain).sensitive).toBe(false);
});

test("a server draft keeps a poll still being written as it is", () => {
  const body = toDraft(
    draft({ text: "when?", poll: { options: ["Sat", ""], duration: 3600, multiple: false, hideTotals: false } }),
    settings,
  );
  expect(body.poll?.options).toEqual(["Sat", ""]);
  expect(body.text).toBe("when?");
});

test("a draft's image whose file could not be read stays the server's: kept by its ID in the next save", () => {
  const server = {
    id: "d1",
    created_at: "2026-11-14T00:00:00Z",
    updated_at: "2026-11-14T01:00:00Z",
    text: "a walk",
    spoiler_text: "",
    sensitive: false,
    visibility: "public",
    media: [{ id: "m1", description: "a hill" }, { id: "m2" }],
  } as unknown as ServerDraft;
  const read = { id: "m2", type: "image", state: "ready", url: "/m2.png" } as unknown as MediaAttachment;
  // m1 could not be read (a request that failed), m2 was.
  const opened = fromServer(server, { id: "d1", etag: '"1"' }, [undefined, read], settings, "key");
  const [unread, ready] = opened.attachments;
  expect(unread).toMatchObject({ id: "m1", state: "failed", description: "a hill" });
  expect(ready).toMatchObject({ state: "ready", media: read });
  expect(opened.attachments.every(onServer)).toBe(true);
  expect(toDraft({ ...opened, attachments: opened.attachments.filter(onServer) }, settings).media).toEqual([
    { id: "m1", description: "a hill" },
    { id: "m2" },
  ]);
  // It is not posted until read again: the post waits for it.
  expect(problemsOf(opened, settings).waiting).toBe(true);
  // An image still uploading, or whose upload failed, is no file of the server's.
  expect(onServer({ key: "a", state: "uploading", description: "" })).toBe(false);
  expect(onServer({ key: "a", state: "failed", description: "" })).toBe(false);
});

test("a draft opens for what it was written for, and one whose scheduled post failed with when it was for and why", () => {
  const server = {
    id: "d1",
    created_at: "2026-11-14T00:00:00Z",
    updated_at: "2026-11-14T01:00:00Z",
    text: "Booked two seats.",
    spoiler_text: "",
    sensitive: false,
    visibility: "public",
    media: [],
    last_schedule: {
      publish_at: "2026-11-15T09:00:00Z",
      outcome: "failed",
      reason: "reply_unavailable",
      occurred_at: "2026-11-15T09:00:05Z",
    },
  } as unknown as ServerDraft;
  const open = (fields: Partial<ServerDraft>) =>
    fromServer({ ...server, ...fields }, { id: "d1", etag: '"1"' }, [], settings, "key");

  expect(open({}).failure).toEqual({ publishAt: "2026-11-15T09:00:00Z", reason: "reply_unavailable" });
  // What it was written for opens with it: a reply to its post, a quote of its post, else a new post.
  expect(open({}).target).toEqual({ kind: "new" });
  expect(open({ in_reply_to_id: "p1" }).target).toEqual({ kind: "reply", id: "p1" });
  expect(open({ quoted_status_id: "p2" }).target).toEqual({ kind: "quote", id: "p2" });
  // Scheduled again, the failure is behind it.
  expect(open({ scheduled_at: "2026-11-20T09:00:00Z" }).failure).toBeUndefined();
});

test("a refused member of a post names its input: the text, the warning, a poll's option, an image's description", () => {
  const ready = (key: string): Attachment => ({
    key,
    state: "ready",
    media: { id: `m-${key}`, type: "image", state: "ready" } as unknown as MediaAttachment,
    description: "",
  });
  const d = draft({
    text: "when?",
    cw: "food",
    poll: { options: ["Sat", "Sun", "Mon"], duration: 3600, multiple: false, hideTotals: false },
    // The first is still uploading, so the body sends the others: a draft's file the server holds, then a2, then a3.
    attachments: [
      { key: "a1", state: "uploading", description: "" },
      { key: "d1", id: "m1", state: "failed", description: "" },
      ready("a2"),
      ready("a3"),
    ],
  });
  const members = composerMembers(d);
  expect(inputOf("text", members)).toBe("text");
  expect(inputOf("spoiler_text", members)).toBe("cw");
  expect(inputOf("poll.options[0]", members)).toBe("option:0");
  expect(inputOf("poll.options[2]", members)).toBe("option:2");
  expect(inputOf("media[0].description", members)).toBe("media:d1");
  expect(inputOf("media[1].description", members)).toBe("media:a2");
  expect(inputOf("media[2].description", members)).toBe("media:a3");
  // No image is sent at that index.
  expect(inputOf("media[3].description", members)).toBeUndefined();
  // The members of the whole poll, the images, the language, the IDs and the key stay the form's.
  for (const member of [
    "poll.options",
    "poll.expires_in",
    "media",
    "media[0].id",
    "language",
    "in_reply_to_id",
    "quoted_status_id",
    "Idempotency-Key",
  ])
    expect(inputOf(member, members)).toBeUndefined();
});
