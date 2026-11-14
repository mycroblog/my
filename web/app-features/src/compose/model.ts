// SPDX-License-Identifier: AGPL-3.0-only

// What the composer holds while the reader writes, and what it sends: a post's draft as the composer keeps it (the
// text, the content warning, the audience and who may interact with it, the language, the images or the poll, and the
// post replied to or quoted), the rules it is checked by before it is sent, and the bodies of a post and of a server
// draft made from it. No React and no server here: the hook (use-composer.ts) holds it, the view shows it.

import type {
  Audience,
  ComposeSettings,
  CreateStatus,
  DraftRequest,
  Interaction,
  MediaAttachment,
  SavedDraft,
  Status,
  Visibility,
} from "@my/app-data";

/** What the composer was opened for: a new post, or a reply to or a quote of a post. */
export type Target = { kind: "new" } | { kind: "reply"; id: string } | { kind: "quote"; id: string };

/** A target as one string, by which a draft left for it is found again. */
export const targetKey = (target: Target) => (target.kind === "new" ? "new" : `${target.kind}:${target.id}`);

/** An image of the post: its file while it uploads, the server's file once ready, and its description. */
export type Attachment = {
  /** Its own key in the list, apart from the server's ID, which it has once uploaded. */
  readonly key: string;
  /** The file chosen, to upload again after a failure. */
  readonly file?: File | undefined;
  /** What the composer shows of it: the file's own preview while it uploads, the server's once ready. */
  readonly preview?: string | undefined;
  readonly media?: MediaAttachment | undefined;
  readonly state: "uploading" | "processing" | "failed" | "ready";
  readonly description: string;
};

/** A poll as it is written: its options, how long it runs (seconds), and how it counts. */
export type PollDraft = {
  readonly options: readonly string[];
  readonly duration: number;
  readonly multiple: boolean;
  readonly hideTotals: boolean;
};

/** A post as the composer holds it. */
export type Draft = {
  readonly target: Target;
  readonly text: string;
  /** The content warning; undefined while it is off. */
  readonly cw?: string | undefined;
  readonly visibility: Visibility;
  readonly policy: Readonly<Record<Interaction, Audience>>;
  readonly language: string;
  readonly attachments: readonly Attachment[];
  readonly poll?: PollDraft | undefined;
  /** The reader took the quoted post out: it is a new post. */
  readonly quoteRemoved?: boolean | undefined;
  /** The draft on the server it was saved as, to save over or delete. */
  readonly saved?: SavedDraft | undefined;
  /** The Idempotency-Key of its post, the same for every attempt. */
  readonly key: string;
};

/** How long a poll may run, the choices the composer offers, in seconds: 5 minutes, an hour, 1, 3 and 7 days. */
export const pollDurations = [300, 3600, 86400, 259200, 604800] as const;
/** How many options a poll has at most and at least, and the most characters of one (PollInput). */
export const pollLimits = { min: 2, max: 4, length: 50 } as const;
/** How many images a post attaches at most. */
export const maxAttachments = 4;
/** How many characters left before the count stops being muted. */
export const nearChars = 20;

/** The new poll: two empty options, a day. */
export const newPoll: PollDraft = { options: ["", ""], duration: 86400, multiple: false, hideTotals: false };

/** The visibilities from the widest, as the audience menu lists them. */
export const visibilities: readonly Visibility[] = ["public", "local", "unlisted", "private", "direct"];
/** A post of these may not quote a post from another server (ADR-0069 §4). */
const closed: readonly Visibility[] = ["local", "private", "direct"];
/** Whether a post of `visibility` may quote a remote post. */
export const quotesRemote = (visibility: Visibility) => !closed.includes(visibility);

/** The empty draft for `target`, from the reader's settings. */
export function emptyDraft(target: Target, settings: ComposeSettings, key: string): Draft {
  return {
    target,
    text: "",
    visibility: settings.visibility,
    policy: settings.policy,
    language: settings.language === "und" ? "" : settings.language,
    attachments: [],
    key,
  };
}

/** The handle a status's author is mentioned by: `@user` here, `@user@domain` elsewhere. */
const mention = (acct: string) => `@${acct}`;

/**
 * What a draft takes from the post it answers once that is read: a reply mentions its author and the accounts it
 * mentions (but the reader), and keeps a followers-only or direct post's audience; a quote of a remote post leaves
 * an audience that may not quote it for one that may. Nothing changes when the reader has written already.
 */
export function answering(draft: Draft, status: Status, viewer: ComposeSettings["viewer"]): Draft {
  const shown = status.boost ?? status;
  if (draft.target.kind === "reply" && draft.text === "") {
    const accts = [shown.account.acct, ...shown.mentions.map((m) => m.acct)].filter(
      (acct, i, all) => acct !== viewer?.acct && all.indexOf(acct) === i,
    );
    const visibility =
      shown.visibility === "private" || shown.visibility === "direct" ? shown.visibility : draft.visibility;
    return { ...draft, text: accts.map((a) => `${mention(a)} `).join(""), visibility };
  }
  if (draft.target.kind === "quote" && shown.account.acct.includes("@") && !quotesRemote(draft.visibility))
    return { ...draft, visibility: "unlisted" };
  return draft;
}

/** Whether a draft holds anything worth keeping. */
export const written = (d: Draft) =>
  d.text.trim() !== "" || d.attachments.length > 0 || d.poll !== undefined || (d.cw ?? "").trim() !== "";

/** How many characters the text and the content warning use together, as the server counts them. */
export const usedChars = (d: Draft) => Array.from(d.text).length + Array.from(d.cw ?? "").length;

/** What keeps a draft from being posted. */
export type Problems = {
  /** Characters over the limit; 0 within it. */
  readonly over: number;
  /** Images without the description the server asks for. */
  readonly missingDescriptions: boolean;
  /** Images still uploading or failed. */
  readonly waiting: boolean;
  /** A poll with too few options, a blank one, or two the same. */
  readonly pollInvalid: boolean;
  /** Nothing to post. */
  readonly empty: boolean;
};

export function problemsOf(d: Draft, settings: ComposeSettings): Problems {
  const options = d.poll?.options.map((o) => o.trim()) ?? [];
  return {
    over: Math.max(0, usedChars(d) - settings.maxChars),
    missingDescriptions:
      settings.requireDescriptions && d.attachments.some((a) => a.state === "ready" && a.description.trim() === ""),
    waiting: d.attachments.some((a) => a.state !== "ready"),
    pollInvalid:
      d.poll !== undefined &&
      (options.length < pollLimits.min || options.includes("") || new Set(options).size !== options.length),
    empty: d.text.trim() === "" && d.attachments.length === 0,
  };
}

/** Whether nothing keeps the draft from being posted. */
export const postable = (p: Problems) =>
  p.over === 0 && !p.missingDescriptions && !p.waiting && !p.pollInvalid && !p.empty;

/** Whom the reader changed from their defaults: the kinds to send; none when nothing changed. */
function policyChange(d: Draft, settings: ComposeSettings) {
  const kinds = (["reply", "announce", "quote"] as const).filter((k) => d.policy[k] !== settings.policy[k]);
  if (kinds.length === 0) return undefined;
  return Object.fromEntries(kinds.map((k) => [k, { automatic: d.policy[k] }]));
}

const repliesTo = (d: Draft) => (d.target.kind === "reply" ? d.target.id : undefined);
const quotes = (d: Draft) => (d.target.kind === "quote" && !d.quoteRemoved ? d.target.id : undefined);

/** The body of the post. */
export function toStatus(d: Draft, settings: ComposeSettings): CreateStatus {
  const cw = d.cw?.trim() ?? "";
  const media = d.attachments.flatMap((a) =>
    a.media === undefined
      ? []
      : [{ id: a.media.id, ...(a.description.trim() === "" ? {} : { description: a.description.trim() }) }],
  );
  const policy = policyChange(d, settings);
  const reply = repliesTo(d);
  const quote = quotes(d);
  return {
    text: d.text,
    visibility: d.visibility,
    ...(cw === "" ? {} : { spoiler_text: cw, sensitive: media.length > 0 }),
    ...(d.language === "" ? {} : { language: d.language }),
    ...(reply === undefined ? {} : { in_reply_to_id: reply }),
    ...(quote === undefined ? {} : { quoted_status_id: quote }),
    ...(media.length === 0 ? {} : { media }),
    ...(d.poll === undefined
      ? {}
      : {
          poll: {
            options: d.poll.options.map((o) => o.trim()),
            expires_in: d.poll.duration,
            multiple: d.poll.multiple,
            hide_totals: d.poll.hideTotals,
          },
        }),
    ...(policy === undefined ? {} : { interaction_policy: policy }),
  };
}

/** The body of the server's draft (ADR-0156): what was written, with the images that finished uploading. */
export function toDraft(d: Draft, settings: ComposeSettings): DraftRequest {
  const status = toStatus(d, settings);
  return {
    ...status,
    ...(d.cw !== undefined && d.cw.trim() !== "" ? { spoiler_text: d.cw } : {}),
    ...(d.poll === undefined
      ? {}
      : { poll: { ...status.poll, options: [...d.poll.options], expires_in: d.poll.duration } }),
  };
}
