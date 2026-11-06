// SPDX-License-Identifier: AGPL-3.0-only
// The fields of an announcement that the form of a new one and the edit form of its page share: the title, one line
// of plain text whose description counts the characters left (grapheme clusters, as the server counts them), and the
// Markdown text with its counter.
import { TextArea, TextField } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { announcementTextLimit, announcementTitleLimit } from "../api/announcements";
import { titleLength } from "../hooks/useAnnouncements";

/** The counter of a text field, in the viewer's language. */
function useFormatRemaining() {
  const t = useT();
  return (remaining: number) =>
    remaining < 0
      ? t("client:admin.settings.characters_over", { count: -remaining })
      : t("client:admin.settings.characters_left", { count: remaining });
}

type FieldProps = {
  value: string;
  onChange: (value: string) => void;
  /** Why the value cannot be saved, localized; undefined when it can or it is not checked yet. */
  error: string | undefined;
};

/** The title of an announcement; past the limit, its error says so. */
export function AnnouncementTitleField({ value, onChange, error }: FieldProps) {
  const t = useT();
  const remaining = Math.max(0, announcementTitleLimit - titleLength(value.trim()));
  return (
    <TextField
      label={t("client:admin.announcements.announcement_title")}
      description={t("client:admin.announcements.announcement_title.description", { count: remaining })}
      value={value}
      onChange={onChange}
      {...(error === undefined ? {} : { isInvalid: true, errorMessage: error })}
    />
  );
}

/** The Markdown text of an announcement, at most 5,000 characters. */
export function AnnouncementTextField({ value, onChange, error, rows }: FieldProps & { rows: number }) {
  const t = useT();
  const formatRemaining = useFormatRemaining();
  return (
    <TextArea
      label={t("client:admin.announcements.text")}
      description={t("client:admin.announcements.text.description")}
      value={value}
      onChange={onChange}
      rows={rows}
      characterLimit={announcementTextLimit}
      formatRemaining={formatRemaining}
      {...(error === undefined ? {} : { isInvalid: true, errorMessage: error })}
    />
  );
}
