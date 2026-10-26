// SPDX-License-Identifier: AGPL-3.0-only
// The form of a room's settings (ADR-0100 §3, §9): its name, description, hashtags, who sees and joins it, and who
// speaks without asking. Opening a room also offers the start post for a room of the `local` or `followers`
// visibility, on by default (回答 13): the host writes its text, which starts as a draft with the room's name and
// follows the name until the host edits it (回答 39); the server adds the room's link at its end. The other
// visibilities have no post visibility to match, so the post is not offered for them.
import { useAnnouncer } from "@my/ui";
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
  type VoiceRoomJoin,
  voiceRoomLimits,
  type VoiceRoomSettings,
  type VoiceRoomSpeakers,
  type VoiceRoomUpdate,
  type VoiceRoomVisibility,
} from "../api/voice";

/** What the form holds. `postText` null is the draft that follows the name; a string is the host's own text. */
type VoiceRoomDraft = {
  name: string;
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

/** The hashtags typed in the field, without their `#`, each once. */
export function parseHashtags(text: string): string[] {
  const tags = text
    .split(/[\s,，、]+/u)
    .map((word) => word.replace(/^[#＃]+/u, ""))
    .filter((tag) => tag !== "");
  return [...new Set(tags)];
}

function draftOf(room: VoiceRoom | undefined): VoiceRoomDraft {
  return {
    name: room?.name ?? "",
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
  const announce = useAnnouncer();
  const [draft, setDraft] = useState<VoiceRoomDraft>(() => draftOf(room));
  const [fieldError, setFieldError] = useState<VoiceRoomFieldError | undefined>(undefined);
  const open = useMutation({
    ...openVoiceRoomMutation(api),
    onSuccess: async (joined: VoiceRoomJoin) => {
      await cacheVoiceRoom(api, queryClient, joined.room, { opened: true });
      onDone();
      await navigate({ to: "/voice/$id", params: { id: joined.room.id } });
    },
  });
  const update = useMutation({
    ...updateVoiceRoomMutation(api, room?.id ?? ""),
    onSuccess: async (saved: VoiceRoom) => {
      await cacheVoiceRoom(api, queryClient, saved);
      announce(t("client:voice.form.saved"));
      onDone();
    },
  });
  const mutation = room === undefined ? open : update;
  const postText = draft.postText ?? t("client:voice.form.post.draft", { name: draft.name.trim() });
  const offersPost = room === undefined && offersStartPost(draft.visibility);

  const check = (): VoiceRoomFieldError | undefined => {
    const name = draft.name.trim();
    if (name === "" || name.length > voiceRoomLimits.name || /[\r\n]/u.test(name)) {
      return { field: "name", message: t("client:voice.form.name.invalid", { max: String(voiceRoomLimits.name) }) };
    }
    if (draft.description.length > voiceRoomLimits.description) {
      return { field: "description", message: t("client:voice.form.characters_over", { count: draft.description.length - voiceRoomLimits.description }) };
    }
    if (parseHashtags(draft.hashtags).length > voiceRoomLimits.hashtags) {
      return { field: "hashtags", message: t("client:voice.form.hashtags.invalid", { max: String(voiceRoomLimits.hashtags) }) };
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
        return;
      }
      const settings = {
        name: draft.name.trim(),
        description: draft.description.trim(),
        hashtags: parseHashtags(draft.hashtags),
        visibility: draft.visibility,
        speakers: draft.speakers,
      };
      if (room === undefined) {
        const body: VoiceRoomSettings = { ...settings, ...(offersPost && draft.post && { post: { text: postText } }) };
        open.mutate(body);
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
  if (settings.description !== room.description) change.description = settings.description;
  if (settings.hashtags.join(" ") !== room.hashtags.join(" ")) change.hashtags = settings.hashtags;
  if (settings.visibility !== room.visibility) change.visibility = settings.visibility;
  if (settings.speakers !== room.speakers) change.speakers = settings.speakers;
  return change;
}
