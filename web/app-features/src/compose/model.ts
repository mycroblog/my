// SPDX-License-Identifier: AGPL-3.0-only

// What the composer holds while the reader writes, and what it sends: a post's draft as the composer keeps it (the
// text, the content warning, the audience and who may interact with it, the language, the images or the poll, and the
// post replied to or quoted), the rules it is checked by before it is sent, the bodies of a post, of an edit and of a
// server draft made from it, and the draft made from a server draft, to go on writing it. No React and no server here: the hook (use-composer.ts) holds it, the view shows it.

import {
  choiceOf,
  ruleOf,
  type ComposeSettings,
  type CreateStatus,
  type DraftFailureReason,
  type DraftRequest,
  type EditStatus,
  type Interaction,
  type InteractionChoice,
  type MediaAttachment,
  type PostSource,
  type SavedDraft,
  type ServerDraft,
  type Status,
  type Visibility,
} from "@my/app-data";
import type { FieldMembers } from "../field-errors";
import { interactions } from "../interactions";

/**
 * What the composer was opened for: a new post, a reply to or a quote of a post, an edit of the reader's post, or a
 * new post written again from the reader's post that was deleted for it (redraft), each of the last two from what the
 * post held.
 */
export type Target =
  | { kind: "new" }
  | { kind: "reply"; id: string }
  | { kind: "quote"; id: string }
  | { kind: "edit" | "redraft"; id: string; source: PostSource };

/**
 * What the app opens the composer for: a target, or a draft of the reader's on the server (ADR-0156), continued from
 * any device; `schedule` opens it with its time's form open.
 */
export type Opening = Target | { kind: "draft"; id: string; schedule?: boolean | undefined };

/** A target as one string, by which a draft left for it is found again. */
export const targetKey = (target: Opening) => (target.kind === "new" ? "new" : `${target.kind}:${target.id}`);

/** An image of the post: its file while it uploads, the server's file once ready, and its description. */
export type Attachment = {
  /** Its own key in the list, apart from the server's ID, which it has once uploaded. */
  readonly key: string;
  /** The file chosen, to upload again after a failure. */
  readonly file?: File | undefined;
  /** What the composer shows of it: the file's own preview while it uploads, the server's once ready. */
  readonly preview?: string | undefined;
  readonly media?: MediaAttachment | undefined;
  /**
   * The server's ID of a draft's file that could not be read when the draft was opened: the server still holds it, so
   * a save keeps it, and a retry reads it again.
   */
  readonly id?: string | undefined;
  readonly state: "uploading" | "processing" | "failed" | "ready";
  readonly description: string;
};

/** Whether the server holds an image's file, to keep in a draft: one ready, or a draft's that could not be read. */
export const onServer = (a: Attachment) => a.state === "ready" || a.id !== undefined;

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
  /**
   * What goes before the text when it is sent, which the writer does not write: the mentions of a conversation's
   * people, whom its reply in place names apart (in-place.tsx).
   */
  readonly mentions?: string | undefined;
  /** The content warning; undefined while it is off. */
  readonly cw?: string | undefined;
  /** The images are marked sensitive, as the post edited or written again had them; the warning marks them too. */
  readonly sensitive?: boolean | undefined;
  readonly visibility: Visibility;
  /** Who may reply, boost and quote: an audience at once, and whether others may ask for approval. */
  readonly policy: Readonly<Record<Interaction, InteractionChoice>>;
  readonly language: string;
  readonly attachments: readonly Attachment[];
  readonly poll?: PollDraft | undefined;
  /** The reader took the quoted post out: it is a new post. */
  readonly quoteRemoved?: boolean | undefined;
  /** The draft on the server it was saved as, to save over or delete. */
  readonly saved?: SavedDraft | undefined;
  /**
   * It was opened from the reader's drafts on the server: closing it leaves that draft as it was saved, and a save
   * refused for a newer one saved elsewhere says so (ADR-0156 §9).
   */
  readonly stored?: boolean | undefined;
  /** When it is to be posted (ISO 8601); undefined to post now. */
  readonly scheduledAt?: string | undefined;
  /** Its last schedule failed: the time it was for and why it was not posted (ADR-0156 §8). */
  readonly failure?: { readonly publishAt: string; readonly reason: DraftFailureReason } | undefined;
  /** What its Idempotency-Key is made from (postKey); it names the post for the drafts left by accident too (kept.ts). */
  readonly key: string;
  /** How many of its writes the server answered without posting it: each one renews the key of its post (postKey). */
  readonly answers?: number | undefined;
};

/**
 * The Idempotency-Key of the next post of a draft: the same for every attempt that got no answer, so that it is posted
 * once, and a new one after an answer, so that the post corrected is not taken for the one answered.
 */
export function postKey(d: { readonly key: string; readonly answers?: number | undefined }): string {
  return d.answers === undefined || d.answers === 0 ? d.key : `${d.key}.${String(d.answers)}`;
}

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

/** The duration the composer offers nearest to `seconds`; a day for a poll without an end. */
const nearestDuration = (seconds: number | undefined) =>
  seconds === undefined
    ? newPoll.duration
    : pollDurations.reduce((best, d) => (Math.abs(d - seconds) < Math.abs(best - seconds) ? d : best));

/**
 * The draft `target` opens with, from the reader's settings: empty, or for an edit or a redraft what the post held
 * (its poll only for a redraft, a new post; an edit never has one).
 */
export function emptyDraft(target: Target, settings: ComposeSettings, key: string): Draft {
  const language = settings.language === "und" ? "" : settings.language;
  const base = { target, visibility: settings.visibility, policy: settings.policy, language, key };
  if (target.kind !== "edit" && target.kind !== "redraft") return { ...base, text: "", attachments: [] };
  const { source } = target;
  const poll = target.kind === "redraft" ? source.poll : undefined;
  return {
    ...base,
    text: source.text,
    cw: source.spoiler_text === "" ? undefined : source.spoiler_text,
    sensitive: source.sensitive,
    visibility: source.visibility,
    language: source.language === "und" ? "" : source.language,
    attachments: source.media.map((media) => ({
      key: `source-${media.id}`,
      media,
      preview: media.preview_url ?? media.url,
      state: "ready",
      description: media.description ?? "",
    })),
    poll:
      poll === undefined
        ? undefined
        : {
            options: poll.options,
            duration: nearestDuration(poll.duration),
            multiple: poll.multiple,
            hideTotals: false,
          },
  };
}

/** Who may reply, boost and quote, as the server keeps it (`kept`, its kinds), the reader's defaults for the others. */
export function keptPolicy(kept: ServerDraft["interaction_policy"], settings: ComposeSettings): Draft["policy"] {
  const rule = (k: Interaction) => {
    const r = kept?.[k];
    return r === undefined ? settings.policy[k] : choiceOf(r);
  };
  return { reply: rule("reply"), announce: rule("announce"), quote: rule("quote") };
}

/**
 * The draft of a draft the server keeps, to go on writing it (ADR-0156): a reply or a quote of the post it answers, else
 * a new post; its images as the server holds them now (`media`, in the draft's order; undefined for one gone), its
 * schedule, and why its last schedule failed, while it has none.
 */
export function fromServer(
  server: ServerDraft,
  saved: SavedDraft,
  media: readonly (MediaAttachment | undefined)[],
  settings: ComposeSettings,
  key: string,
): Draft {
  const target: Target =
    server.in_reply_to_id !== undefined
      ? { kind: "reply", id: server.in_reply_to_id }
      : server.quoted_status_id !== undefined
        ? { kind: "quote", id: server.quoted_status_id }
        : { kind: "new" };
  const failed = server.scheduled_at === undefined ? server.last_schedule : undefined;
  return {
    target,
    text: server.text,
    cw: server.spoiler_text === "" ? undefined : server.spoiler_text,
    sensitive: server.sensitive,
    visibility: server.visibility,
    policy: keptPolicy(server.interaction_policy, settings),
    language: server.language === undefined || server.language === "und" ? "" : server.language,
    attachments: server.media.map((item, i) => {
      const file = media[i];
      return {
        key: `draft-${item.id}`,
        media: file,
        // A file that could not be read is still the server's: kept by its ID, and read again on a retry.
        ...(file === undefined ? { id: item.id } : {}),
        preview: file?.preview_url ?? file?.url,
        // A file not read, or one the server could not process, is no image the post may carry yet.
        state: file === undefined || file.state === "failed" ? "failed" : "ready",
        description: item.description ?? "",
      };
    }),
    poll:
      server.poll === undefined
        ? undefined
        : {
            options: server.poll.options,
            duration: nearestDuration(server.poll.expires_in),
            multiple: server.poll.multiple ?? false,
            hideTotals: server.poll.hide_totals ?? false,
          },
    saved,
    stored: true,
    scheduledAt: server.scheduled_at,
    failure:
      failed?.outcome === "failed" ? { publishAt: failed.publish_at, reason: failed.reason ?? "invalid" } : undefined,
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
  if (draft.target.kind === "reply" && draft.text === "" && draft.mentions === undefined) {
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

/** How many characters the text, its mentions and the content warning use together, as the server counts them. */
export const usedChars = (d: Draft) =>
  Array.from(d.mentions ?? "").length + Array.from(d.text).length + Array.from(d.cw ?? "").length;

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

/** Whether two choices make the same rule: asking for approval means nothing while anyone may. */
const sameRule = (a: InteractionChoice, b: InteractionChoice) => {
  const [x, y] = [ruleOf(a), ruleOf(b)];
  return x.automatic === y.automatic && x.manual === y.manual;
};

/**
 * Whom the reader changed from their defaults, of a post's or an article's `policy`: the rules of the kinds to send;
 * none when nothing changed.
 */
export function policyChange(policy: Draft["policy"], settings: ComposeSettings) {
  const kinds = interactions.filter((k) => !sameRule(policy[k], settings.policy[k]));
  if (kinds.length === 0) return undefined;
  return Object.fromEntries(kinds.map((k) => [k, ruleOf(policy[k])]));
}

/** The post it replies to: the target of a reply, or what the deleted post of a redraft replied to. */
const repliesTo = (d: Draft) =>
  d.target.kind === "reply" ? d.target.id : d.target.kind === "redraft" ? d.target.source.in_reply_to_id : undefined;
/** The post it quotes: the target of a quote, or what the deleted post of a redraft quoted. */
const quotes = (d: Draft) => {
  if (d.quoteRemoved === true) return undefined;
  if (d.target.kind === "quote") return d.target.id;
  return d.target.kind === "redraft" ? d.target.source.quoted_status_id : undefined;
};

/** The images of the post the server holds (finished uploading, or a draft's not read), with their descriptions. */
const mediaOf = (d: Draft) =>
  d.attachments.flatMap((a) => {
    const id = a.media?.id ?? a.id;
    return id === undefined
      ? []
      : [{ id, ...(a.description.trim() === "" ? {} : { description: a.description.trim() }) }];
  });

/** Whether the post's images are marked sensitive: as the draft keeps them, or under a content warning. */
const marksSensitive = (d: Draft, cw: string, media: readonly unknown[]) =>
  (d.sensitive === true || cw !== "") && media.length > 0;

/**
 * The body of an edit: the text, the content warning, the language and the images, all of them, since an edit
 * replaces each (none left is none).
 */
export function toEdit(d: Draft): EditStatus {
  const cw = d.cw?.trim() ?? "";
  const media = mediaOf(d);
  return {
    text: d.text,
    spoiler_text: cw,
    sensitive: marksSensitive(d, cw, media),
    ...(d.language === "" ? {} : { language: d.language }),
    media,
  };
}

/** The body of the post. */
export function toStatus(d: Draft, settings: ComposeSettings): CreateStatus {
  const cw = d.cw?.trim() ?? "";
  const media = mediaOf(d);
  const policy = policyChange(d.policy, settings);
  const reply = repliesTo(d);
  const quote = quotes(d);
  return {
    text: `${d.mentions ?? ""}${d.text}`,
    visibility: d.visibility,
    ...(cw === "" ? {} : { spoiler_text: cw }),
    ...(marksSensitive(d, cw, media) ? { sensitive: true } : {}),
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

/**
 * An input of the composer a refusal can stand beside: the text, the content warning, an option of the poll by its
 * index, or the description of an image by its key.
 */
export type ComposerInput = "text" | "cw" | `option:${string}` | `media:${string}`;

/**
 * Which input answers to which member of the body sent of `d` (a post, an edit or a draft, which name them alike): an
 * image by its place among those sent, the images the server holds. The server counts the content warning with the
 * text, under `text`. The members of no one input (`language`, `media`, `poll.options`, `poll.expires_in`) stay the
 * form's error.
 */
export function composerMembers(d: Draft): FieldMembers<ComposerInput> {
  const sent = d.attachments.filter((a) => (a.media?.id ?? a.id) !== undefined);
  return {
    text: "text",
    spoiler_text: "cw",
    "poll.options[]": (i) => `option:${String(i)}`,
    "media[].description": (i) => {
      const a = sent[i];
      return a === undefined ? undefined : `media:${a.key}`;
    },
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

/**
 * The sentence under an image's empty description once Post was pressed: that the server requires one, or, where
 * only the reader's own setting does, that their settings do.
 */
export const descriptionRequired = (
  settings: ComposeSettings,
): "compose.description.required" | "compose.description.required.own" =>
  settings.descriptionsRequiredBy === "own" ? "compose.description.required.own" : "compose.description.required";
