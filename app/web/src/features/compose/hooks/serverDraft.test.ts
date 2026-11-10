// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test } from "vitest";
import { emptyDraft } from "./draftStore";
import { draftOfServer, draftRequest } from "./serverDraft";

describe("a draft saved on the server", () => {
  test("carries the draft as it is, with what a post would refuse", () => {
    const body = draftRequest(
      {
        ...emptyDraft,
        text: "",
        hasWarning: true,
        spoilerText: " Spoilers ",
        poll: {
          options: [
            { key: "a", title: "Yes" },
            { key: "b", title: "" },
          ],
          expiresIn: 3600,
          multiple: true,
          hideTotals: false,
        },
        attachments: [
          { key: "1", mediaId: "m1", description: "A cat", previewUrl: null, error: null },
          { key: "2", mediaId: null, description: "", previewUrl: null, error: null },
        ],
      },
      "unlisted",
      "ja",
    );
    expect(body).toEqual({
      text: "",
      visibility: "unlisted",
      language: "ja",
      spoiler_text: "Spoilers",
      media: [{ id: "m1", description: "A cat" }],
      sensitive: false,
      poll: { options: ["Yes", ""], expires_in: 3600, multiple: true },
    });
  });

  test("opens in the form as the continuation of the saved draft", () => {
    const draft = draftOfServer({
      etag: '"v3"',
      draft: {
        id: "d1",
        created_at: "2026-11-09T09:00:00Z",
        updated_at: "2026-11-09T09:00:00Z",
        text: "Hello",
        spoiler_text: "",
        sensitive: false,
        visibility: "private",
        language: "ja",
        in_reply_to_id: "s1",
        media: [{ id: "m1" }],
        poll: { options: ["Only"], expires_in: 4000 },
        scheduled_at: "2026-11-12T00:00:00Z",
      },
    });
    expect(draft).toMatchObject({
      text: "Hello",
      hasWarning: false,
      visibility: "private",
      language: "ja",
      replyToId: "s1",
      quoteId: null,
      attachments: [{ mediaId: "m1", description: "" }],
      // A poll keeps two fields, and its duration is one the form offers.
      poll: { options: [{ title: "Only" }, { title: "" }], expiresIn: 3600, multiple: false, hideTotals: false },
      saved: { id: "d1", etag: '"v3"', scheduledAt: "2026-11-12T00:00:00Z" },
    });
  });
});
