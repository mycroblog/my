// SPDX-License-Identifier: AGPL-3.0-only
// The form of a room's settings (ADR-0100 §3, §9): its name, its emoji icon (FR-923), description, hashtags, who sees and joins it, and who
// speaks without asking. Opening a room also offers the start post for a room of the `local` or `followers`
// visibility, on by default (回答 13): the host writes its text, which starts as a draft with the room's name and
// follows the name until the host edits it (回答 39); the server adds the room's link at its end. The other
// visibilities have no post visibility to match, so the post is not offered for them. Opening a room joins the host
// (ADR-0100 §5), so the host's call starts from the submit, without a join button.
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  cacheVoiceRoom,
  openVoiceRoomMutation,
  updateVoiceRoomMutation,
  type VoiceRoom,
  type VoiceRoomEmoji,
  type VoiceRoomJoin,
  voiceRoomLimits,
  type VoiceRoomSettings,
  type VoiceRoomSpeakers,
  type VoiceRoomUpdate,
  type VoiceRoomVisibility,
} from "../api/voice";
import type { ReactionChoice } from "../../emoji";
import { voiceSupported } from "../rtc/connect";
import { useViewer } from "../../session";
import { type VoiceCallGesture, useVoiceCallState } from "./useVoiceCallProvider";

/** What the form holds. `postText` null is the draft that follows the name; a string is the host's own text. */
type VoiceRoomDraft = {
  name: string;
  /** The emoji before the name, with what shows it; undefined for none. */
  icon: VoiceRoomEmoji | undefined;
  description: string;
  hashtags: string;
  visibility: VoiceRoomVisibility;
  speakers: VoiceRoomSpeakers;
  post: boolean;
  postText: string | null;
};

/** The field that a check of the draft failed on, with the message to show next to it. */
export type VoiceRoomFieldError = { field: "name" | "description" | "hashtags"; message: string };

/** Whether a room of `visibility` has a start post: only `local` (a `local` post) and `followers` (a `private` one). */
export function offersStartPost(visibility: VoiceRoomVisibility): boolean {
  return visibility === "local" || visibility === "followers";
}

/** How many characters (code points) `text` has, as the server and the character counter of @my/ui count them. */
export function characterCount(text: string): number {
  return Array.from(text).length;
}

/** How many grapheme clusters (what a reader sees as one character) `text` has. */
function graphemeCount(text: string): number {
  return Array.from(new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(text)).length;
}

/** Whether `name` (trimmed) is a name the server takes: 1 to 100 grapheme clusters on one line, no control characters. */
export function isValidRoomName(name: string): boolean {
  return (
    name !== "" &&
    !/\p{Cc}/u.test(name) &&
    characterCount(name) <= voiceRoomLimits.nameCodePoints &&
    graphemeCount(name) <= voiceRoomLimits.name
  );
}

/** How the server tells hashtags apart: Unicode NFKC and lower case, as the hashtags of posts. */
function normalizedTag(tag: string): string {
  return tag.normalize("NFKC").toLowerCase();
}

/** The hashtags typed in the field, without their `#`, each once by its normalized name (the first spelling stays). */
export function parseHashtags(text: string): string[] {
  const seen = new Set<string>();
  const tags: string[] = [];
  for (const word of text.split(/[\s,，、]+/u)) {
    const tag = word.replace(/^[#＃]+/u, "");
    const key = normalizedTag(tag);
    if (tag === "" || seen.has(key)) continue;
    seen.add(key);
    tags.push(tag);
  }
  return tags;
}

function draftOf(room: VoiceRoom | undefined): VoiceRoomDraft {
  return {
    name: room?.name ?? "",
    icon: room?.icon,
    description: room?.description ?? "",
    hashtags: room === undefined ? "" : room.hashtags.map((tag) => `#${tag}`).join(" "),
    visibility: room?.visibility ?? "local",
    speakers: room?.speakers ?? "manual",
    post: true,
    postText: null,
  };
}

export type VoiceRoomForm = {
  /** `new` opens a room; `edit` changes the settings of the user's room. */
  mode: "new" | "edit";
  draft: VoiceRoomDraft;
  change: (change: Partial<VoiceRoomDraft>) => void;
  /** The text of the start post as the field shows it: the host's text, or the draft with the name. */
  postText: string;
  /** Whether the form offers the start post now. */
  offersPost: boolean;
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  isPending: boolean;
  /** The field that a check failed on; the draft stays. */
  fieldError: VoiceRoomFieldError | undefined;
  /** Why the server refused the form, localized; the draft stays. */
  error: string | undefined;
};

type Options = {
  /** The room whose settings the form changes; without it the form opens a room. */
  room?: VoiceRoom;
  /** Called once the server took the form, to close the dialog; a new room's page opens after it. */
  onDone: () => void;
};

/** The form of a new room, or of the settings of `room`, which the user hosts. */
export function useVoiceRoomForm({ room, onDone }: Options): VoiceRoomForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const notice = useNotice();
  const [draft, setDraft] = useState<VoiceRoomDraft>(() => draftOf(room));
  const [fieldError, setFieldError] = useState<VoiceRoomFieldError | undefined>(undefined);
  const call = useVoiceCallState();
  const viewer = useViewer();
  const open = useMutation({
    mutationKey: openVoiceRoomMutation(api).mutationKey,
    mutationFn: ({ settings }: { settings: VoiceRoomSettings; gesture: VoiceCallGesture | undefined }) =>
      openVoiceRoomMutation(api).mutationFn(settings),
    onSuccess: async (joined: VoiceRoomJoin, { gesture }) => {
      await cacheVoiceRoom(api, queryClient, joined.room, { opened: true });
      // Opening the room joined the host (ADR-0100 §5): the call starts here, with the audio started by the submit.
      if (gesture !== undefined) call.adopt(joined, gesture);
      onDone();
      await navigate({ to: "/voice/$id", params: { id: joined.room.id } });
    },
  });
  const update = useMutation({
    ...updateVoiceRoomMutation(api, room?.id ?? ""),
    onSuccess: async (saved: VoiceRoom) => {
      await cacheVoiceRoom(api, queryClient, saved);
      notice.inPlace(t("client:voice.form.saved"));
      onDone();
    },
  });
  const mutation = room === undefined ? open : update;
  const postText = draft.postText ?? t("client:voice.form.post.draft", { name: draft.name.trim() });
  const offersPost = room === undefined && offersStartPost(draft.visibility);

  const check = (): VoiceRoomFieldError | undefined => {
    if (!isValidRoomName(draft.name.trim())) {
      return { field: "name", message: t("client:voice.form.name.invalid", { max: String(voiceRoomLimits.name) }) };
    }
    const description = characterCount(draft.description.trim());
    if (description > voiceRoomLimits.description) {
      return {
        field: "description",
        message: t("client:voice.form.characters_over", { count: description - voiceRoomLimits.description }),
      };
    }
    const tags = parseHashtags(draft.hashtags);
    if (tags.length > voiceRoomLimits.hashtags) {
      return {
        field: "hashtags",
        message: t("client:voice.form.hashtags.invalid", { max: String(voiceRoomLimits.hashtags) }),
      };
    }
    if (
      tags.some(
        (tag) =>
          characterCount(normalizedTag(tag)) > voiceRoomLimits.hashtagLength ||
          characterCount(tag) > voiceRoomLimits.hashtagLength,
      )
    ) {
      return {
        field: "hashtags",
        message: t("client:voice.form.hashtags.too_long", { max: String(voiceRoomLimits.hashtagLength) }),
      };
    }
    return undefined;
  };

  return {
    mode: room === undefined ? "new" : "edit",
    draft,
    change: (change) => {
      setDraft((previous) => ({ ...previous, ...change }));
    },
    postText,
    offersPost,
    submit: (event) => {
      event.preventDefault();
      if (mutation.isPending) return;
      const checked = check();
      setFieldError(checked);
      if (checked !== undefined) {
        mutation.reset();
        // The first field that the check failed on takes focus, so that its message is read and it can be fixed.
        const field = event.currentTarget.elements.namedItem(checked.field);
        if (field instanceof HTMLElement) field.focus();
        return;
      }
      const settings = {
        name: draft.name.trim(),
        icon: draft.icon?.name ?? "",
        description: draft.description.trim(),
        hashtags: parseHashtags(draft.hashtags),
        visibility: draft.visibility,
        speakers: draft.speakers,
      };
      if (room === undefined) {
        const { icon, ...rest } = settings;
        const body: VoiceRoomSettings = {
          ...rest,
          ...(icon !== "" && { icon }),
          ...(offersPost && draft.post && { post: { text: postText } }),
        };
        // From the submit's gesture, as the join button does: the audio of the host's call starts here (ADR-0101 §7).
        const gesture = viewer.status === "signed-in" && voiceSupported() ? call.prepare(viewer.account.id) : undefined;
        open.mutate({ settings: body, gesture });
        return;
      }
      update.mutate(changeOf(room, settings));
    },
    isPending: mutation.isPending,
    fieldError,
    error: mutation.isError ? errorMessage(mutation.error) : undefined,
  };
}

/** What the form changes of `room`; only what differs is sent. */
function changeOf(room: VoiceRoom, settings: Required<Omit<VoiceRoomSettings, "post">>): VoiceRoomUpdate {
  const change: VoiceRoomUpdate = {};
  if (settings.name !== room.name) change.name = settings.name;
  // An icon that the room has but cannot show (its custom emoji was disabled) reads as none and stays as it is.
  if (settings.icon !== (room.icon?.name ?? "")) change.icon = settings.icon;
  if (settings.description !== room.description) change.description = settings.description;
  if (settings.hashtags.join(" ") !== room.hashtags.join(" ")) change.hashtags = settings.hashtags;
  if (settings.visibility !== room.visibility) change.visibility = settings.visibility;
  if (settings.speakers !== room.speakers) change.speakers = settings.speakers;
  return change;
}

/** The icon of a room from the emoji picked (a Unicode emoji, or a custom emoji of this server with its image). */
export function iconOf(choice: ReactionChoice): VoiceRoomEmoji {
  if ("unicode" in choice) return { name: choice.unicode };
  const { shortcode, url, static_url } = choice.custom;
  return { name: shortcode, emoji: { shortcode, url, static_url } };
}
