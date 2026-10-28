// SPDX-License-Identifier: AGPL-3.0-only
import { useToast } from "@my/ui";
import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { shellQueryReader } from "../../../lib/query";
import {
  type AIUsage,
  cacheSavedProfile,
  type CredentialAccount,
  credentialsQueryOptions,
  instanceQueryOptions,
  type ProfileChange,
  type SearchIndexing,
  updateProfileMutation,
} from "../api/me";
import { type ImagePurpose, type MediaAttachment, MediaProcessingError, uploadImage } from "../api/media";

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
 * The profile form (FR-006): its fields, and saving them in one change of the profile. The browser checks the
 * limits before submitting (React Aria's native validation), so `submit` runs only with valid values.
 * Mount it again (with a `key`) to start over from a newer saved profile.
 */
export function useProfileForm() {
  const api = useApi();
  const queryClient = useQueryClient();
  const toast = useToast();
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
  const [indexable, setIndexable] = useState(account.indexable);
  const [aiOptOut, setAiOptOut] = useState(account.ai_opt_out);
  // The shell reads the instance on every page; the form shows its notes once it is there.
  const { data: instance } = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  const serverPolicy: ServerCrawlerPolicy =
    instance === undefined ? undefined : { searchIndexing: instance.search_indexing, aiUsage: instance.ai_usage };
  const avatar = useImageControl(account.avatar, "avatar");
  const header = useImageControl(account.header, "header");

  const save = useMutation({
    ...updateProfileMutation(api),
    onSuccess: (saved) => {
      // The profile page and the author of every post of the viewer show the new profile from their first
      // render, even those cached from before the change (web-client/README.md S1).
      cacheSavedProfile(api, queryClient, saved);
      // The saved profile has the new images from now on.
      avatar.saved();
      header.saved();
      toast.show({ message: t("client:settings.profile.saved"), tone: "success" });
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
    indexable,
    ai_opt_out: aiOptOut,
    ...avatar.change("avatar_media_id"),
    ...header.change("header_media_id"),
  });

  const draft = change();
  // Compared with the saved profile, so that an edit undone by hand leaves nothing to save.
  const hasChanges =
    draft.display_name !== account.display_name ||
    draft.note !== account.source.note ||
    draft.locked !== account.locked ||
    draft.bot !== account.bot ||
    draft.indexable !== account.indexable ||
    draft.ai_opt_out !== account.ai_opt_out ||
    draft.avatar_media_id !== undefined ||
    draft.header_media_id !== undefined ||
    !sameFields(draft.fields ?? [], account.source.fields);

  return {
    account,
    /** Whether the form holds changes that are not saved: leaving the page asks first. */
    hasChanges,
    displayName: { value: displayName, onChange: setDisplayName },
    note: { value: note, onChange: setNote },
    fields: {
      rows: fields,
      canAdd: fields.length < profileLimits.fields,
      add: () => {
        setFields([...fields, { key: nextFieldKey, name: "", value: "" }]);
        setNextFieldKey(nextFieldKey + 1);
      },
      update: (key: number, change: Partial<Omit<FieldDraft, "key">>) => {
        setFields(fields.map((field) => (field.key === key ? { ...field, ...change } : field)));
      },
      remove: (key: number) => {
        setFields(fields.filter((field) => field.key !== key));
      },
    },
    locked: { isSelected: locked, onChange: setLocked },
    bot: { isSelected: bot, onChange: setBot },
    indexable: { isSelected: indexable, onChange: setIndexable },
    aiOptOut: { isSelected: aiOptOut, onChange: setAiOptOut },
    serverPolicy,
    avatar: avatar.control,
    header: header.control,
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      save.mutate(change());
    },
    isSaving: save.isPending,
    isBusy: save.isPending || avatar.control.isUploading || header.control.isUploading,
    /** Why the last save failed, localized; the server names the invalid property in it. */
    saveError: save.isError ? errorMessage(save.error) : undefined,
  };
}

type Field = { name: string; value: string };

function sameFields(a: readonly Field[], b: readonly Field[]): boolean {
  return a.length === b.length && a.every((field, i) => field.name === b[i]?.name && field.value === b[i].value);
}

/** The state of one image of the profile, and its upload. */
function useImageControl(saved: CredentialAccount["avatar"], purpose: ImagePurpose) {
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
