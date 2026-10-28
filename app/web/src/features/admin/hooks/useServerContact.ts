// SPDX-License-Identifier: AGPL-3.0-only
// How people reach the server's operators and the image that represents the server (#663): the contact account (a
// user of this server), the contact email address, and the thumbnail with its alt text, one value that every
// language shows. The about page, the landing page, GET /instance and NodeInfo show them. A new image is uploaded as
// soon as it is chosen and becomes the thumbnail when the form is saved; saving sends the settings that changed
// together, then the alt text when it changed, each an audited change.
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { MediaProcessingError, type MediaAttachment, uploadImage } from "../../settings";
import { instanceQueryOptions, instanceTextsQueryOptions } from "../api/documents";
import {
  cacheSavedSettings,
  cacheSavedTexts,
  type ContactChange,
  type InstanceSettingKey,
  type InstanceTextChange,
  instanceSettingsQueryOptions,
  saveContactMutation,
  thumbnailAltLimit,
} from "../api/settings";
import { characterCount } from "./languages";

/** The settings of the form. */
type ContactKey = Extract<InstanceSettingKey, "contact_account" | "contact_email" | "thumbnail">;

/** What saving does to the thumbnail: keep the saved one, use a new upload, or remove it. */
type ThumbnailChange = { kind: "keep" } | { kind: "new"; media: MediaAttachment } | { kind: "remove" };

/** A username as the server takes it: without the `@` that people may type before it, and without spaces. */
function username(value: string): string {
  return value.trim().replace(/^@/u, "");
}

export function useServerContact() {
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const { data: settings } = useSuspenseQuery(instanceSettingsQueryOptions(api));
  const { data: texts } = useSuspenseQuery(instanceTextsQueryOptions(api));
  const { data: instance } = useSuspenseQuery(instanceQueryOptions(api));
  const savedSetting = (key: ContactKey) => settings.items.find((setting) => setting.key === key)?.value ?? "";
  const savedAlt = texts.items.find((text) => text.key === "thumbnail_alt")?.body ?? "";

  const [account, setAccount] = useState<string | undefined>(undefined);
  const [email, setEmail] = useState<string | undefined>(undefined);
  const [thumbnail, setThumbnail] = useState<ThumbnailChange>({ kind: "keep" });
  const [altDraft, setAltDraft] = useState<string | undefined>(undefined);
  const [status, setStatus] = useState<string | undefined>(undefined);

  const upload = useMutation({
    mutationFn: (file: File) => uploadImage(api, file, "attachment"),
    onSuccess: (media) => {
      setThumbnail({ kind: "new", media });
    },
  });

  const accountValue = account ?? savedSetting("contact_account");
  const emailValue = email ?? savedSetting("contact_email");
  const altValue = altDraft ?? savedAlt;

  const values: Partial<Record<ContactKey, string>> = {};
  if (username(accountValue) !== savedSetting("contact_account")) values.contact_account = username(accountValue);
  if (emailValue.trim() !== savedSetting("contact_email")) values.contact_email = emailValue.trim();
  if (thumbnail.kind === "new") values.thumbnail = thumbnail.media.id;
  if (thumbnail.kind === "remove" && savedSetting("thumbnail") !== "") values.thumbnail = "";
  const altChanges: InstanceTextChange[] = altValue === savedAlt ? [] : [{ key: "thumbnail_alt", body: altValue }];
  const altTooLong = characterCount(altValue) > thumbnailAltLimit;

  const save = useMutation({
    ...saveContactMutation(api),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: async (saved) => {
      // The about page and the landing page show the new contact and thumbnail once GET /instance is fetched again
      // (the button stays pending until then).
      await Promise.all([
        saved === undefined ? undefined : cacheSavedSettings(queryClient, api, saved),
        cacheSavedTexts(queryClient),
      ]);
      setAccount(undefined);
      setEmail(undefined);
      setThumbnail({ kind: "keep" });
      upload.reset();
      setAltDraft(undefined);
      setStatus(t("client:admin.settings.contact.saved"));
    },
    // The settings may have been saved before the alt text failed: the form compares with what is saved now, and keeps
    // the draft to send again (web-client/README.md E2).
    onError: () => cacheSavedTexts(queryClient),
  });

  // The medium rendition of a new upload fills the preview; the saved thumbnail comes from GET /instance.
  const previewUrl =
    thumbnail.kind === "new"
      ? thumbnail.media.variants?.medium.url
      : thumbnail.kind === "keep"
        ? instance.thumbnail?.url
        : undefined;
  const uploadError =
    upload.error instanceof MediaProcessingError
      ? t("client:admin.settings.contact.thumbnail.failed")
      : errorMessage(upload.error);

  return {
    account: { value: accountValue, onChange: setAccount },
    email: { value: emailValue, onChange: setEmail },
    thumbnail: {
      /** The image the form shows: the new upload, the saved thumbnail, or none. */
      url: previewUrl,
      choose: (file: File) => {
        upload.mutate(file);
      },
      remove: () => {
        upload.reset();
        setThumbnail({ kind: "remove" });
      },
      isUploading: upload.isPending,
      /** Why the upload failed, localized; undefined when it did not. */
      error: upload.isError ? uploadError : undefined,
    },
    /** The alt text of the thumbnail. */
    alt: { value: altValue, onChange: setAltDraft },
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      // An alt text over its limit shows why under its field (its count); the server would refuse it.
      if (altTooLong || save.isPending || upload.isPending) return;
      const change: ContactChange = { values, texts: altChanges };
      if (Object.keys(values).length === 0 && altChanges.length === 0) {
        setStatus(t("client:admin.settings.contact.unchanged"));
        return;
      }
      save.mutate(change);
    },
    isSaving: save.isPending,
    /** Why the last save failed, localized; the server names the invalid setting in it. */
    saveError: save.isError ? errorMessage(save.error) : undefined,
    /** What the last save did, for the region that reports it. */
    status,
  };
}
