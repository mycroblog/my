// SPDX-License-Identifier: AGPL-3.0-only
// The server's announcements (FR-726, ADR-0125, manage_announcements): every announcement, newest first, with its
// state; writing one with a plain-text title and a Markdown text, shown for a chosen time or until it is withdrawn, as
// a draft or published at once; revising its title and text; publishing a draft; and withdrawing one for good. Every
// change goes to the audit log.
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  type AdminAnnouncement,
  adminAnnouncementsQueryOptions,
  announcementTextLimit,
  announcementTitleLimit,
  createAnnouncementMutation,
  publishAnnouncementMutation,
  refreshAnnouncements,
  updateAnnouncementMutation,
  withdrawAnnouncementMutation,
} from "../api/announcements";
import { characterCount } from "./languages";

const day = 86_400;

/** How long a new announcement may show, in seconds; the form also offers "until withdrawn". */
export const announcementDurations = [day, 3 * day, 7 * day, 14 * day, 30 * day] as const;

/** Every announcement, newest first. */
export function useAdminAnnouncements(): AdminAnnouncement[] {
  const api = useApi();
  return useSuspenseQuery(adminAnnouncementsQueryOptions(api)).data.items;
}

/** The first line of an announcement's text, without its Markdown marks: its name in the list and in the buttons. */
function announcementName(text: string): string {
  const line = text
    .split("\n")
    .map((item) =>
      item
        .trim()
        .replace(/^[#>*+\- ]+/u, "")
        .trim(),
    )
    .find((item) => item !== "");
  return line ?? "";
}

/**
 * An announcement's name in the list, the buttons and the results: its title, or for one written before titles, the
 * first line of its text.
 */
export function announcementLabel(announcement: Pick<AdminAnnouncement, "title" | "text">): string {
  return announcement.title === "" ? announcementName(announcement.text) : announcement.title;
}

/** The length of a title as the server counts it: grapheme clusters (ADR-0125 §1). */
export function titleLength(title: string): number {
  return Array.from(new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(title)).length;
}

/** Why a title cannot be saved, localized; undefined when it can. */
function useTitleError() {
  const t = useT();
  return (title: string): string | undefined => {
    if (title.trim() === "") return t("client:admin.announcements.title_required");
    if (titleLength(title.trim()) > announcementTitleLimit)
      return t("client:admin.announcements.too_long", { max: String(announcementTitleLimit) });
    return undefined;
  };
}

/** Why a text cannot be saved, localized; undefined when it can. */
function useTextError() {
  const t = useT();
  return (text: string): string | undefined => {
    if (text.trim() === "") return t("client:admin.announcements.text_required");
    if (characterCount(text) > announcementTextLimit)
      return t("client:admin.announcements.too_long", { max: String(announcementTextLimit) });
    return undefined;
  };
}

/** The form of a new announcement: it empties once the list shows the announcement. */
export function useNewAnnouncementForm() {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const textError = useTextError();
  const titleError = useTitleError();
  const [title, setTitle] = useState("");
  const [text, setText] = useState("");
  const [duration, setDuration] = useState<number | null>(7 * day);
  const [publish, setPublish] = useState(true);
  const [submitted, setSubmitted] = useState(false);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const create = useMutation({
    ...createAnnouncementMutation(api),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: async (announcement) => {
      await refreshAnnouncements(queryClient);
      setTitle("");
      setText("");
      setSubmitted(false);
      setStatus(
        announcement.state === "published"
          ? t("client:admin.announcements.published_one", { announcement: announcementLabel(announcement) })
          : t("client:admin.announcements.drafted"),
      );
    },
  });
  const error = submitted ? textError(text) : undefined;
  // Over the limit, the title says so at once; empty, once the form is sent.
  const titleProblem = titleError(title);
  return {
    title,
    changeTitle: (value: string) => {
      setTitle(value);
    },
    titleError: submitted || title.trim() !== "" ? titleProblem : undefined,
    text,
    changeText: (value: string) => {
      setText(value);
    },
    duration,
    changeDuration: (value: number | null) => {
      setDuration(value);
    },
    publish,
    changePublish: (value: boolean) => {
      setPublish(value);
    },
    textError: error,
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      setSubmitted(true);
      if (create.isPending || titleError(title) !== undefined || textError(text) !== undefined) return;
      // The end counts from the moment the form is sent.
      const endsAt = duration === null ? {} : { ends_at: new Date(Date.now() + duration * 1000).toISOString() };
      create.mutate({ title: title.trim(), text, publish, ...endsAt });
    },
    isPending: create.isPending,
    /** Why writing failed, localized; the text stays. */
    failure: create.isError ? errorMessage(create.error) : undefined,
    status,
  };
}

/** What the list of announcements does to one; each resolves whether it worked. */
export type AnnouncementActions = {
  /** Replaces the title and the text of the announcement and keeps its window. */
  update: (announcement: AdminAnnouncement, title: string, text: string) => Promise<boolean>;
  publish: (announcement: AdminAnnouncement) => Promise<boolean>;
  withdraw: (announcement: AdminAnnouncement) => Promise<boolean>;
  /** Why a text cannot be saved, localized; undefined when it can. */
  textError: (text: string) => string | undefined;
  /** Why a title cannot be saved, localized; undefined when it can. */
  titleError: (title: string) => string | undefined;
  /** Why the last change failed, localized. */
  error: string | undefined;
};

/**
 * The changes of the announcements in the list. Each says what it did through `onDone(announcement, status)` once
 * the list shows it (the announcements are fetched again first), so that the screen can move focus and keep the
 * status next to the list.
 */
export function useAnnouncementActions(
  onDone: (announcement: AdminAnnouncement, status: string) => void,
): AnnouncementActions {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const textError = useTextError();
  const titleError = useTitleError();
  const [error, setError] = useState<string | undefined>(undefined);
  const update = useMutation({ ...updateAnnouncementMutation(api) });
  const publish = useMutation({ ...publishAnnouncementMutation(api) });
  const withdraw = useMutation({ ...withdrawAnnouncementMutation(api) });
  const run = async (
    work: () => Promise<AdminAnnouncement>,
    status: (announcement: AdminAnnouncement) => string,
  ): Promise<boolean> => {
    setError(undefined);
    try {
      const changed = await work();
      await refreshAnnouncements(queryClient);
      onDone(changed, status(changed));
      return true;
    } catch (failure) {
      setError(errorMessage(failure));
      return false;
    }
  };
  return {
    update: (announcement, title, text) =>
      run(
        () =>
          update.mutateAsync({
            id: announcement.id,
            body: {
              title: title.trim(),
              text,
              ...(announcement.starts_at === undefined ? {} : { starts_at: announcement.starts_at }),
              ...(announcement.ends_at === undefined ? {} : { ends_at: announcement.ends_at }),
            },
          }),
        (changed) => t("client:admin.announcements.updated", { announcement: announcementLabel(changed) }),
      ),
    publish: (announcement) =>
      run(
        () => publish.mutateAsync(announcement.id),
        (changed) => t("client:admin.announcements.published_one", { announcement: announcementLabel(changed) }),
      ),
    withdraw: (announcement) =>
      run(
        () => withdraw.mutateAsync(announcement.id),
        (changed) => t("client:admin.announcements.withdrawn", { announcement: announcementLabel(changed) }),
      ),
    textError,
    titleError,
    error,
  };
}
