// SPDX-License-Identifier: AGPL-3.0-only
// The server's own icon and logos on the server settings screen (ADR-0116, manage_settings;
// docs/architecture/web-client/screens/admin-settings.md): the icon, the logo, and the logo for dark backgrounds, each
// a form of its own. Choosing an SVG checks it on the server and draws its PNG renditions here; the previews show it
// where it will be used, what the server removed from it is listed, and saving sets it for every visitor. Each form
// saves with its button, and the result shows beside it (FormActionBar); a file that cannot be used says why under the
// button that chose it.
import {
  BrandPreview,
  BrandPreviewList,
  Button,
  Cluster,
  FileTrigger,
  FormActionBar,
  Heading,
  Hint,
  InlineError,
  Stack,
  Text,
  TextField,
} from "@my/ui";
import { useId } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { Section } from "../../moderation";
import type { BrandKind, BrandRendition } from "../api/brand";
import { useBrandImage } from "../hooks/useBrandImage";

/** What the file picker offers: SVG files only. */
const acceptedSvgTypes = ["image/svg+xml", ".svg"];

/** The words of each kind's form. */
const kindMessages = {
  icon: {
    title: "client:admin.brand.icon.title",
    description: "client:admin.brand.icon.description",
    save: "client:admin.brand.icon.save",
    clear: "client:admin.brand.icon.clear",
  },
  logo: {
    title: "client:admin.brand.logo.title",
    description: "client:admin.brand.logo.description",
    save: "client:admin.brand.logo.save",
    clear: "client:admin.brand.logo.clear",
  },
  logo_dark: {
    title: "client:admin.brand.logo_dark.title",
    description: "client:admin.brand.logo_dark.description",
    save: "client:admin.brand.logo_dark.save",
    clear: "client:admin.brand.logo_dark.clear",
  },
} as const satisfies Record<BrandKind, Record<string, ClientMessageKey>>;

/** The previews of each rendition: where it is shown, and the caption that says so. */
const renditionPreviews: Partial<
  Record<
    BrandRendition,
    { place: "tab" | "homeScreen" | "maskable" | "mail" | "darkMail" | "linkPreview"; caption: ClientMessageKey }
  >
> = {
  "favicon-32": { place: "tab", caption: "client:admin.brand.preview.tab" },
  "apple-touch-180": { place: "homeScreen", caption: "client:admin.brand.preview.home_screen" },
  "maskable-512": { place: "maskable", caption: "client:admin.brand.preview.app_icon" },
  "logo-mail": { place: "mail", caption: "client:admin.brand.preview.mail" },
  "logo-dark-mail": { place: "darkMail", caption: "client:admin.brand.preview.mail_dark" },
  "preview-1200x630": { place: "linkPreview", caption: "client:admin.brand.preview.link" },
};

/** An element as it is written in SVG (`<script>`): code, not words to translate. */
function elementTag(name: string): string {
  return `<${name}>`;
}

export function BrandSettings() {
  const t = useT();
  return (
    <Section surface="card" title={t("client:admin.brand.title")}>
      <Hint size="inherit">{t("client:admin.brand.description")}</Hint>
      <Stack gap="8">
        <BrandForm kind="icon" />
        <BrandForm kind="logo" />
        <BrandForm kind="logo_dark" />
      </Stack>
    </Section>
  );
}

function BrandForm({ kind }: { kind: BrandKind }) {
  const t = useT();
  const form = useBrandImage(kind);
  const messages = kindMessages[kind];
  const headingId = useId();
  const descriptionId = useId();
  const fileErrorId = useId();
  const { draft, saved } = form;
  return (
    <Stack
      as="form"
      gap="4"
      aria-labelledby={headingId}
      aria-describedby={descriptionId}
      aria-busy={form.isChecking}
      onSubmit={form.submit}
    >
      <Heading level={3} size="medium" id={headingId}>
        {t(messages.title)}
      </Heading>
      <Hint id={descriptionId}>{t(messages.description)}</Hint>
      {draft === undefined ? (
        saved === undefined ? (
          <Text as="p" tone="muted">
            {t("client:admin.brand.built_in")}
          </Text>
        ) : (
          <BrandPreviewList>
            <BrandPreview place="image" src={saved.svg_url} caption={t("client:admin.brand.preview.current")} />
          </BrandPreviewList>
        )
      ) : (
        <BrandPreviewList>
          <BrandPreview place="image" src={draft.svgUrl} caption={t("client:admin.brand.preview.new")} />
          {draft.renditions.map((rendition) => {
            const preview = renditionPreviews[rendition.rendition];
            return (
              preview !== undefined && (
                <BrandPreview
                  key={rendition.rendition}
                  place={preview.place}
                  src={rendition.url}
                  caption={t(preview.caption)}
                />
              )
            );
          })}
        </BrandPreviewList>
      )}
      {form.dropped.length > 0 && (
        <Stack gap="2">
          <Text as="p" weight="semibold">
            {t("client:admin.brand.dropped", { count: form.dropped.length })}
          </Text>
          <Cluster as="ul" gap="2">
            {form.dropped.map((dropped) => (
              <Text as="li" key={`${dropped.kind}:${dropped.name}`} size="sm" tone="muted">
                <code dir="ltr">{dropped.kind === "element" ? elementTag(dropped.name) : dropped.name}</code>
              </Text>
            ))}
          </Cluster>
        </Stack>
      )}
      <Stack gap="2">
        <div>
          <FileTrigger
            acceptedFileTypes={acceptedSvgTypes}
            onSelect={(files) => {
              const file = files?.[0];
              if (file !== undefined) form.choose(file);
            }}
          >
            <Button
              size="sm"
              isPending={form.isChecking}
              aria-describedby={form.fileError === undefined ? headingId : `${headingId} ${fileErrorId}`}
            >
              {t("client:admin.brand.choose")}
            </Button>
          </FileTrigger>
        </div>
        {form.fileError !== undefined && (
          <InlineError alert id={fileErrorId}>
            {form.fileError}
          </InlineError>
        )}
      </Stack>
      {kind === "icon" && draft !== undefined && (
        <TextField
          label={t("client:admin.brand.background")}
          description={t("client:admin.brand.background.description")}
          value={form.background.value}
          onChange={form.background.onChange}
          autoComplete="off"
          {...(form.background.error === undefined ? {} : { isInvalid: true, errorMessage: form.background.error })}
        />
      )}
      <FormActionBar result={form.result}>
        <Button type="submit" variant="primary" isPending={form.isSaving}>
          {t(messages.save)}
        </Button>
        {saved !== undefined && (
          <Button variant="ghost" onPress={form.clear} isPending={form.isClearing}>
            {t(messages.clear)}
          </Button>
        )}
      </FormActionBar>
    </Stack>
  );
}
