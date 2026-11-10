// SPDX-License-Identifier: AGPL-3.0-only
// The form's draft and the draft saved on the server (ADR-0156), each made from the other. A saved draft keeps what a
// post would refuse (empty text, media still being processed, a poll still being written), so the request carries the
// draft as it is; publishing checks it as a new post.
import type { DraftRequest, DraftVersion } from "../api/drafts";
import type { Visibility } from "../api/compose";
import { type Draft, emptyDraft } from "./draftStore";
import { maxPollOptions, pollDurations, type PollDraft } from "./poll";

/** The content of a draft, to save on the server: the visibility and language the form shows, the media uploaded. */
export function draftRequest(draft: Draft, visibility: Visibility, language: string): DraftRequest {
  const media = draft.attachments.flatMap((a) =>
    a.mediaId === null
      ? []
      : [{ id: a.mediaId, ...(a.description.trim() === "" ? {} : { description: a.description.trim() }) }],
  );
  return {
    text: draft.text,
    visibility,
    language,
    ...(draft.hasWarning && draft.spoilerText.trim() !== "" ? { spoiler_text: draft.spoilerText.trim() } : {}),
    ...(media.length > 0 ? { media, sensitive: draft.sensitive } : {}),
    ...(draft.poll === null ? {} : { poll: draftPoll(draft.poll) }),
    ...(draft.replyToId === null ? {} : { in_reply_to_id: draft.replyToId }),
    ...(draft.quoteId === null ? {} : { quoted_status_id: draft.quoteId }),
    ...(Object.keys(draft.interactionPolicy).length === 0 ? {} : { interaction_policy: draft.interactionPolicy }),
  };
}

/** A poll being written, as a draft keeps it: its options as typed. */
function draftPoll(poll: PollDraft): NonNullable<DraftRequest["poll"]> {
  return {
    options: poll.options.map((o) => o.title),
    expires_in: poll.expiresIn,
    ...(poll.multiple ? { multiple: true } : {}),
    ...(poll.hideTotals ? { hide_totals: true } : {}),
  };
}

/** The duration the form offers that is closest to `seconds`: another app may have saved one the form does not offer. */
function nearestDuration(seconds: number): number {
  return pollDurations.reduce((best, d) => (Math.abs(d - seconds) < Math.abs(best - seconds) ? d : best));
}

/** The form's draft of a draft read from the server, which the next save replaces (`saved`). */
export function draftOfServer({ draft, etag }: DraftVersion): Draft {
  const options = draft.poll?.options.slice(0, maxPollOptions) ?? [];
  // A poll keeps at least two fields to type in, as a new one has.
  const fields = options.length >= 2 ? options : [...options, ...Array<string>(2 - options.length).fill("")];
  return {
    ...emptyDraft,
    text: draft.text,
    hasWarning: draft.spoiler_text !== "",
    spoilerText: draft.spoiler_text,
    sensitive: draft.sensitive,
    visibility: draft.visibility,
    language: draft.language ?? null,
    replyToId: draft.in_reply_to_id ?? null,
    quoteId: draft.quoted_status_id ?? null,
    interactionPolicy: draft.interaction_policy ?? {},
    attachments: draft.media.map((item) => ({
      key: crypto.randomUUID(),
      mediaId: item.id,
      description: item.description ?? "",
      previewUrl: null,
      error: null,
    })),
    poll:
      draft.poll === undefined
        ? null
        : {
            options: fields.map((title) => ({ key: crypto.randomUUID(), title })),
            expiresIn: nearestDuration(draft.poll.expires_in),
            multiple: draft.poll.multiple ?? false,
            hideTotals: draft.poll.hide_totals ?? false,
          },
    saved: { id: draft.id, etag, scheduledAt: draft.scheduled_at ?? null },
  };
}
