// SPDX-License-Identifier: AGPL-3.0-only
import type { FormActionResult } from "@my/ui";
import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { shellQueryReader } from "../../../lib/query";
import { useFeaturedTagsDraft } from "../../hashtags";
import {
  type AIUsage,
  cacheSavedProfile,
  type CredentialAccount,
  credentialsQueryOptions,
  type FollowsVisibility,
  instanceQueryOptions,
  type ProfileChange,
  type SearchIndexing,
  updateProfileMutation,
} from "../api/me";
import { type ImagePurpose, type MediaAttachment, MediaProcessingError, uploadImage } from "../api/media";

export type { FollowsVisibility };

/** The limits of a profile (UpdateCredentialsRequest in api/openapi.yaml), counted in code points as the server does. */
export const profileLimits = { displayName: 30, note: 500, fields: 4, fieldName: 255, fieldValue: 255 } as const;

/** The number of characters of `text` as the server counts them: Unicode code points. */
export function characterCount(text: string): number {
  return Array.from(text).length;
}

/** One metadata field being edited; `key` identifies its row while rows are added and removed. */
export type FieldDraft = { key: number; name: string; value: string };

/** The avatar or the header image being edited. */
export type ImageControl = {
  /** The image to show now: the saved one, the new upload, or none. */
  url: string | undefined;
  /** Whether there is an image that can be removed. */
  hasImage: boolean;
  /** Uploads `file` to replace the image when the profile is saved. */
  choose: (file: File) => void;
  /** Removes the image when the profile is saved. */
  remove: () => void;
  isUploading: boolean;
  /** Why the upload failed, localized. */
  error: string | undefined;
};

type ImageChange = { kind: "keep" } | { kind: "new"; media: MediaAttachment } | { kind: "remove" };

/**
 * What the server states to search engines and AI crawlers for all its pages (FR-725, ADR-0057), which the user's
 * own choices can only narrow; undefined while GET /instance has not loaded.
 */
export type ServerCrawlerPolicy = { searchIndexing: SearchIndexing; aiUsage: AIUsage } | undefined;

/**
 * The profile form (FR-006): its fields and the hashtags it features (ADR-0006 §17), and saving them with the one save
 * button: the profile in one change, then the featured hashtags when they changed. The browser checks the limits
 * before submitting (React Aria's native validation), so `submit` runs only with valid values.
 * Mount it again (with a `key`) to start over from a newer saved profile.
 */
export function useProfileForm() {
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const { data: account } = useSuspenseQuery(credentialsQueryOptions(api));

  const [displayName, setDisplayName] = useState(account.display_name);
  const [note, setNote] = useState(account.source.note);
  const [fields, setFields] = useState<FieldDraft[]>(() =>
    account.source.fields.map((field, key) => ({ key, ...field })),
  );
  const [nextFieldKey, setNextFieldKey] = useState(account.source.fields.length);
  const [locked, setLocked] = useState(account.locked);
  const [bot, setBot] = useState(account.bot);
  const [discoverable, setDiscoverable] = useState(account.discoverable);
  const [indexable, setIndexable] = useState(account.indexable);
  const [aiOptOut, setAiOptOut] = useState(account.ai_opt_out);
  // The owner's own choice about the Atom feed (FR-213), undefined while there is none: the feed then follows
  // `indexable`, the one being edited included, and the change leaves the choice out.
  const [feedChoice, setFeedChoice] = useState<boolean | undefined>(
    account.feed_follows_indexable ? undefined : account.feed_enabled,
  );
  // Who may read the follows and followers lists (FR-311).
  const [followsVisibility, setFollowsVisibility] = useState<FollowsVisibility>(account.follows_visibility);
  // How many edits the form has had, and how many it had when it was last submitted: the result of a save shows
  // until the next edit.
  const [edits, setEdits] = useState(0);
  const [submittedAt, setSubmittedAt] = useState<number | undefined>(undefined);
  // The shell reads the instance on every page; the form shows its notes once it is there.
  const { data: instance } = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  const serverPolicy: ServerCrawlerPolicy =
    instance === undefined ? undefined : { searchIndexing: instance.search_indexing, aiUsage: instance.ai_usage };
  const avatar = useImageControl(account.avatar, "avatar");
  const header = useImageControl(account.header, "header");
  const featuredTags = useFeaturedTagsDraft(account.id);

  const save = useMutation({
    ...updateProfileMutation(api),
    onSuccess: (saved) => {
      // The profile page and the author of every post of the viewer show the new profile from their first
      // render, even those cached from before the change (web-client/README.md S1).
      cacheSavedProfile(api, queryClient, saved);
      // The saved profile has the new images from now on.
      avatar.saved();
      header.saved();
      // The form's action bar shows it beside the button (`result`).
    },
  });

  const change = (): ProfileChange => ({
    display_name: displayName,
    note,
    // A row left empty is not a field.
    fields: fields
      .filter((field) => field.name !== "" || field.value !== "")
      .map(({ name, value }) => ({ name, value })),
    locked,
    bot,
    discoverable,
    indexable,
    ai_opt_out: aiOptOut,
    ...(feedChoice === undefined ? {} : { feed_enabled: feedChoice }),
    follows_visibility: followsVisibility,
    ...avatar.change("avatar_media_id"),
    ...header.change("header_media_id"),
  });

  const draft = change();
  // Compared with the saved profile, so that an edit undone by hand leaves nothing to save.
  const profileChanged =
    draft.display_name !== account.display_name ||
    draft.note !== account.source.note ||
    draft.locked !== account.locked ||
    draft.bot !== account.bot ||
    draft.discoverable !== account.discoverable ||
    draft.indexable !== account.indexable ||
    draft.ai_opt_out !== account.ai_opt_out ||
    (draft.feed_enabled !== undefined &&
      (account.feed_follows_indexable || draft.feed_enabled !== account.feed_enabled)) ||
    draft.follows_visibility !== account.follows_visibility ||
    draft.avatar_media_id !== undefined ||
    draft.header_media_id !== undefined ||
    !sameFields(draft.fields ?? [], account.source.fields);
  const hasChanges = profileChanged || featuredTags.hasChanges;
  const isSaving = save.isPending || featuredTags.isSaving;

  let result: FormActionResult | undefined;
  if (save.isError)
    result = {
      tone: "danger",
      message: t("client:settings.profile.save_failed", { reason: errorMessage(save.error) }),
    };
  else if (featuredTags.error !== undefined) result = { tone: "danger", message: featuredTags.error };
  else if ((save.isSuccess || featuredTags.isSaved) && !isSaving && submittedAt === edits)
    result = { tone: "success", message: t("client:settings.profile.saved") };
  /** Runs an edit of the form, which takes away the result of the last save. */
  const edit =
    <A extends unknown[]>(apply: (...args: A) => void) =>
    (...args: A) => {
      setEdits((count) => count + 1);
      apply(...args);
    };
  return {
    account,
    /** Whether the form holds changes that are not saved: leaving the page asks first. */
    hasChanges,
    displayName: { value: displayName, onChange: edit(setDisplayName) },
    note: { value: note, onChange: edit(setNote) },
    fields: {
      rows: fields,
      canAdd: fields.length < profileLimits.fields,
      add: edit(() => {
        setFields([...fields, { key: nextFieldKey, name: "", value: "" }]);
        setNextFieldKey(nextFieldKey + 1);
      }),
      update: edit((key: number, change: Partial<Omit<FieldDraft, "key">>) => {
        setFields(fields.map((field) => (field.key === key ? { ...field, ...change } : field)));
      }),
      remove: edit((key: number) => {
        setFields(fields.filter((field) => field.key !== key));
      }),
    },
    locked: { isSelected: locked, onChange: edit(setLocked) },
    bot: { isSelected: bot, onChange: edit(setBot) },
    discoverable: { isSelected: discoverable, onChange: edit(setDiscoverable) },
    indexable: { isSelected: indexable, onChange: edit(setIndexable) },
    aiOptOut: { isSelected: aiOptOut, onChange: edit(setAiOptOut) },
    /** Whether the Atom feed is served: the owner's choice, or `indexable` until there is one. */
    feed: { isSelected: feedChoice ?? indexable, onChange: edit(setFeedChoice) },
    /** Whether the feed still follows `indexable`, which the form says. */
    feedFollowsIndexable: feedChoice === undefined,
    /** The address of the profile's feed: its page's, followed by `.atom`. */
    feedUrl: `${account.url}.atom`,
    /** Who may read the follows and followers lists. */
    followsVisibility: { value: followsVisibility, onChange: edit(setFollowsVisibility) },
    serverPolicy,
    featuredTags: {
      rows: featuredTags.rows,
      canAdd: featuredTags.canAdd,
      validate: featuredTags.validate,
      add: edit(featuredTags.add),
      update: edit(featuredTags.update),
      remove: edit(featuredTags.remove),
      move: edit(featuredTags.move),
    },
    avatar: { ...avatar.control, choose: edit(avatar.control.choose), remove: edit(avatar.control.remove) },
    header: { ...header.control, choose: edit(header.control.choose), remove: edit(header.control.remove) },
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (isSaving) return;
      setSubmittedAt(edits);
      save.reset();
      featuredTags.reset();
      // The profile first (also with nothing changed, as the button always saves it), unless only the hashtags
      // changed; the hashtags then, only when they changed. A failure stops there and shows in `result`.
      const saveProfile = profileChanged || !featuredTags.hasChanges;
      const saveTags = featuredTags.hasChanges;
      void (async () => {
        if (saveProfile) await save.mutateAsync(change());
        if (saveTags) await featuredTags.save();
      })().catch(() => {
        // The mutations hold the error, which `result` shows.
      });
    },
    isSaving,
    isBusy: isSaving || avatar.control.isUploading || header.control.isUploading,
    /**
     * The result of the last save beside the save button (web-client/16-look-states-and-wording.md G24): saved until
     * the form changes again, or why it failed (the server names the invalid property in it).
     */
    result,
  };
}

type Field = { name: string; value: string };

function sameFields(a: readonly Field[], b: readonly Field[]): boolean {
  return a.length === b.length && a.every((field, i) => field.name === b[i]?.name && field.value === b[i].value);
}

/** The state of one image of the profile, and its upload (the profile form and the welcome step). */
export function useImageControl(saved: CredentialAccount["avatar"], purpose: ImagePurpose) {
  const api = useApi();
  const t = useT();
  const [image, setImage] = useState<ImageChange>({ kind: "keep" });
  const upload = useMutation({
    mutationFn: (file: File) => uploadImage(api, file, purpose),
    onSuccess: (media) => {
      setImage({ kind: "new", media });
    },
  });
  // The medium rendition fills the form's preview at twice its CSS size.
  const url =
    image.kind === "new" ? image.media.variants?.medium.url : image.kind === "keep" ? saved?.medium.url : undefined;
  const uploadError =
    upload.error instanceof MediaProcessingError
      ? t("client:settings.profile.image.failed")
      : errorMessage(upload.error);
  const control: ImageControl = {
    url,
    hasImage: url !== undefined,
    choose: (file) => {
      upload.mutate(file);
    },
    remove: () => {
      upload.reset();
      setImage({ kind: "remove" });
    },
    isUploading: upload.isPending,
    error: upload.isError ? uploadError : undefined,
  };
  return {
    control,
    /** The profile was saved with this image's change, so the saved profile has it from now on. */
    saved: () => {
      setImage({ kind: "keep" });
    },
    /** The property of the profile change for this image: a new media ID, "" to remove it, or nothing. */
    change: <K extends "avatar_media_id" | "header_media_id">(property: K): Partial<Record<K, string>> => {
      if (image.kind === "new") return { [property]: image.media.id } as Partial<Record<K, string>>;
      if (image.kind === "remove") return { [property]: "" } as Partial<Record<K, string>>;
      return {};
    },
  };
}
