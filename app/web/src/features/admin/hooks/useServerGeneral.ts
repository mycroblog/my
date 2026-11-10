// SPDX-License-Identifier: AGPL-3.0-only
// The general page of the server settings (FR-705, #663): the instance's display name, which names the instance to
// people in place of its domain (the navigation, the tab title, the sign-in pages, emails, link previews); its
// descriptions, the short one that the about page and other servers' directories show and the full one; how people
// reach its operators (the contact account, a user of this server, and the contact email address); the thumbnail
// with its alt text; and the image of link previews with its alt text (preview_image, ADR-0028 §2), which comes before
// the thumbnail in og:image. The administrator writes each text once, in any language, and every language shows it as
// it is; with no name the domain shows, and handles and links keep the domain. A new image is uploaded as soon as it
// is chosen and becomes the setting when the form is saved. Saving sends only what changed: the settings together
// (PATCH /admin/settings), then each text that changed, each an audited change.
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
  descriptionLimit,
  type GeneralChange,
  type InstanceSettingKey,
  type InstanceTextChange,
  instanceNameLimit,
  instanceSettingsQueryOptions,
  imageAltLimit,
  saveGeneralMutation,
  shortDescriptionLimit,
} from "../api/settings";
import { characterCount } from "./languages";

/** The texts of the form. */
type TextKey = Extract<
  InstanceTextChange["key"],
  "name" | "short_description" | "description" | "thumbnail_alt" | "preview_image_alt"
>;

type DescriptionKey = Extract<TextKey, "short_description" | "description">;

/** The settings of the form. */
type ContactKey = Extract<InstanceSettingKey, "contact_account" | "contact_email" | ImageKey>;

/** The image settings of the form. */
type ImageKey = Extract<InstanceSettingKey, "thumbnail" | "preview_image">;

/** The most characters of each description. */
export const descriptionLimits: Readonly<Record<DescriptionKey, number>> = {
  short_description: shortDescriptionLimit,
  description: descriptionLimit,
};

/** One description: what the field holds and how to change it. */
export type DescriptionField = {
  key: DescriptionKey;
  value: string;
  onChange: (value: string) => void;
};

/** What saving does to an image setting: keep the saved one, use a new upload, or remove it. */
type ImageChange = { kind: "keep" } | { kind: "new"; media: MediaAttachment } | { kind: "remove" };

const textKeys = [
  "name",
  "short_description",
  "description",
  "thumbnail_alt",
  "preview_image_alt",
] as const satisfies readonly TextKey[];

/** One image setting of the form: the image it shows and how to change it. */
export type ImageField = {
  /** The image the form shows: the new upload, the saved image, or none. */
  url: string | undefined;
  choose: (file: File) => void;
  remove: () => void;
  isUploading: boolean;
  /** Why the upload failed, localized; undefined when it did not. */
  error: string | undefined;
};

/**
 * The state of one image setting: a new image is uploaded as soon as it is chosen, and the form sends it, or its
 * removal, when it is saved. savedUrl is the saved image, which the form shows until the image changes.
 */
function useImageSetting(savedUrl: string | undefined) {
  const api = useApi();
  const t = useT();
  const [change, setChange] = useState<ImageChange>({ kind: "keep" });
  const upload = useMutation({
    mutationFn: (file: File) => uploadImage(api, file, "attachment"),
    onSuccess: (media) => {
      setChange({ kind: "new", media });
    },
  });
  // The medium rendition of a new upload fills the preview.
  const url = change.kind === "new" ? change.media.variants?.medium.url : change.kind === "keep" ? savedUrl : undefined;
  const uploadError =
    upload.error instanceof MediaProcessingError
      ? t("client:admin.settings.contact.thumbnail.failed")
      : errorMessage(upload.error);
  const field: ImageField = {
    url,
    choose: (file: File) => {
      upload.mutate(file);
    },
    remove: () => {
      upload.reset();
      setChange({ kind: "remove" });
    },
    isUploading: upload.isPending,
    error: upload.isError ? uploadError : undefined,
  };
  return {
    field,
    change,
    reset: () => {
      setChange({ kind: "keep" });
      upload.reset();
    },
  };
}

/** A username as the server takes it: without the `@` that people may type before it, and without spaces. */
function username(value: string): string {
  return value.trim().replace(/^@/u, "");
}

export function useServerGeneral() {
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const { data: settings } = useSuspenseQuery(instanceSettingsQueryOptions(api));
  const { data: texts } = useSuspenseQuery(instanceTextsQueryOptions(api));
  const { data: instance } = useSuspenseQuery(instanceQueryOptions(api));
  const savedSetting = (key: ContactKey) => settings.items.find((setting) => setting.key === key)?.value ?? "";
  const savedText = (key: TextKey) => texts.items.find((text) => text.key === key)?.body ?? "";

  const [textDrafts, setTextDrafts] = useState<Readonly<Partial<Record<TextKey, string>>>>({});
  const [account, setAccount] = useState<string | undefined>(undefined);
  const [email, setEmail] = useState<string | undefined>(undefined);
  const [status, setStatus] = useState<string | undefined>(undefined);
  // The saved thumbnail comes from GET /instance; the saved preview image from its setting, which carries its image.
  const thumbnail = useImageSetting(instance.thumbnail?.url);
  const previewImage = useImageSetting(settings.items.find((setting) => setting.key === "preview_image")?.image?.url);
  const images = { thumbnail, preview_image: previewImage } as const satisfies Record<
    ImageKey,
    ReturnType<typeof useImageSetting>
  >;

  const textValue = (key: TextKey) => textDrafts[key] ?? savedText(key);
  const changeText = (key: TextKey) => (value: string) => {
    setTextDrafts({ ...textDrafts, [key]: value });
  };
  const name = textValue("name");
  const nameError = (() => {
    if (characterCount(name) > instanceNameLimit)
      return t("client:admin.settings.name.too_long", { max: String(instanceNameLimit) });
    if (name !== "" && name.trim() === "") return t("client:admin.settings.name.blank");
    return undefined;
  })();
  const tooLong =
    characterCount(textValue("short_description")) > shortDescriptionLimit ||
    characterCount(textValue("description")) > descriptionLimit ||
    characterCount(textValue("thumbnail_alt")) > imageAltLimit ||
    characterCount(textValue("preview_image_alt")) > imageAltLimit;

  const accountValue = account ?? savedSetting("contact_account");
  const emailValue = email ?? savedSetting("contact_email");
  const values: Partial<Record<ContactKey, string>> = {};
  if (username(accountValue) !== savedSetting("contact_account")) values.contact_account = username(accountValue);
  if (emailValue.trim() !== savedSetting("contact_email")) values.contact_email = emailValue.trim();
  for (const key of ["thumbnail", "preview_image"] as const) {
    const { change } = images[key];
    if (change.kind === "new") values[key] = change.media.id;
    if (change.kind === "remove" && savedSetting(key) !== "") values[key] = "";
  }
  const textChanges: InstanceTextChange[] = textKeys
    .filter((key) => textValue(key) !== savedText(key))
    .map((key) => ({ key, body: textValue(key) }));

  const save = useMutation({
    ...saveGeneralMutation(api),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: async (saved) => {
      // The navigation, the tab title, the about page and the landing page show the new name, descriptions, contact
      // and thumbnail once GET /instance is fetched again (the button stays pending until then); the audit log has
      // the change when it is opened next.
      await Promise.all([
        saved === undefined ? undefined : cacheSavedSettings(queryClient, api, saved),
        cacheSavedTexts(queryClient),
      ]);
      setTextDrafts({});
      setAccount(undefined);
      setEmail(undefined);
      thumbnail.reset();
      previewImage.reset();
      setStatus(t("client:admin.settings.general.saved"));
    },
    // The settings and some texts may have been saved before a text failed: the form compares with what is saved now,
    // and keeps the other drafts to send again (web-client/README.md E2).
    onError: () => cacheSavedTexts(queryClient),
  });

  const description = (key: DescriptionKey): DescriptionField => ({
    key,
    value: textValue(key),
    onChange: changeText(key),
  });

  return {
    /** The domain that names the instance without a name, and that handles and links use. */
    domain: instance.domain,
    /** The name: what the field holds, how to change it, and why it cannot be saved (localized; undefined when it can). */
    name: { value: name, onChange: changeText("name"), error: nameError },
    short: description("short_description"),
    full: description("description"),
    account: { value: accountValue, onChange: setAccount },
    email: { value: emailValue, onChange: setEmail },
    thumbnail: thumbnail.field,
    /** The alt text of the thumbnail. */
    alt: { value: textValue("thumbnail_alt"), onChange: changeText("thumbnail_alt") },
    /** The image of link previews (preview_image). */
    previewImage: previewImage.field,
    /** The alt text of the image of link previews. */
    previewAlt: { value: textValue("preview_image_alt"), onChange: changeText("preview_image_alt") },
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      // A field that cannot be saved shows why under it (its error or its count); the server would refuse it.
      if (
        nameError !== undefined ||
        tooLong ||
        save.isPending ||
        thumbnail.field.isUploading ||
        previewImage.field.isUploading
      )
        return;
      if (Object.keys(values).length === 0 && textChanges.length === 0) {
        setStatus(t("client:admin.settings.general.unchanged"));
        return;
      }
      const change: GeneralChange = { values, texts: textChanges };
      save.mutate(change);
    },
    isSaving: save.isPending,
    /** Why the last save failed, localized; the server names the invalid setting or text in it. */
    saveError: save.isError ? errorMessage(save.error) : undefined,
    /** What the last save did, for the region that reports it. */
    status,
  };
}
