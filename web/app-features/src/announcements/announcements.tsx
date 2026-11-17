// SPDX-License-Identifier: AGPL-3.0-only

// The server's announcements (ADR-0125 §3, §5): their list, every one that showed, newest first, a page at a time,
// each with its title (a link to its page), when it started to show and its first lines; those showing now marked while
// unread, with an action that marks them read, and those whose window ended said to be over. And the page of one: its
// title, its text as the server renders its Markdown, and when it shows; opening it marks it read while it shows and is
// unread. One that never showed, or was withdrawn, is not found, with a link to the list.

import { PostContent, ListColumn } from "@my/app-components";
import {
  announcementHistoryQuery,
  announcementQuery,
  useAnnouncement,
  useAnnouncementHistory,
  useMarkAnnouncementRead,
  type Announcement,
  type AppData,
} from "@my/app-data";
import {
  BackButton,
  CaptionLine,
  Entry,
  Excerpt,
  FeedNotice,
  NoticeLine,
  StateText,
  TextButton,
  TextLink,
  useLocale,
} from "@my/design-system";
import { useEffect, useId, useRef } from "react";
import { stringsIn } from "../locale";

type T = ReturnType<typeof stringsIn>["t"];

/** The address of an announcement's page. */
const pathOf = (a: Announcement) => `/announcements/${a.id}`;

/** What the list's loader waits for: its first page, from the cache unless a write marked it stale. */
export async function loadAnnouncements(data: AppData): Promise<void> {
  await data.queryClient
    .infiniteQuery({ ...announcementHistoryQuery(data), staleTime: Infinity })
    .catch(() => undefined);
}

/** What an announcement's loader waits for: the announcement, from the cache unless a write marked it stale. */
export async function loadAnnouncement(data: AppData, id: string): Promise<void> {
  await data.queryClient.query({ ...announcementQuery(data, id), staleTime: Infinity }).catch(() => undefined);
}

/** An announcement's title, or the word for one without. */
const titleOf = (a: Announcement, t: T) => (a.title === "" ? t("announcements.untitled") : a.title);

/** When an announcement shows: since it started, until its end, or over. */
function period(a: Announcement, t: T, lang: string): string {
  const date = (iso: string) => new Intl.DateTimeFormat(lang, { dateStyle: "long" }).format(new Date(iso));
  if (a.ends_at === undefined) return t("announcements.period_open", { from: date(a.published_at) });
  const span = { from: date(a.published_at), until: date(a.ends_at) };
  return a.ended ? t("announcements.period_ended", span) : t("announcements.period_until", span);
}

/** An announcement of the list. */
function Row({ a, t }: { a: Announcement; t: T }) {
  const id = useId();
  const markRead = useMarkAnnouncementRead();
  const unread = !a.read && !a.ended;
  return (
    <Entry labelledBy={id}>
      <NoticeLine
        icon="announcements"
        date={a.published_at}
        id={id}
        unread={unread ? t("announcements.unread") : undefined}
      >
        <TextLink href={pathOf(a)}>{titleOf(a, t)}</TextLink>
      </NoticeLine>
      <Excerpt>
        <PostContent html={a.content} lang={undefined} />
      </Excerpt>
      {a.ended ? (
        <StateText>{t("announcements.ended")}</StateText>
      ) : (
        unread && (
          <TextButton
            onPress={() => {
              void markRead(a.id).catch(() => undefined);
            }}
          >
            {t("announcements.mark_read")}
          </TextButton>
        )
      )}
    </Entry>
  );
}

/** The list of the server's announcements. */
export function AnnouncementList() {
  const { t } = stringsIn(useLocale().locale);
  const { items, ...list } = useAnnouncementHistory();
  return (
    <ListColumn
      {...list}
      label={t("announcements.label")}
      items={items}
      keyOf={(a) => a.id}
      words={{
        loading: t("announcements.loading"),
        loadingMore: t("announcements.loading_more"),
        failed: t("announcements.failed"),
        failedMore: t("announcements.failed_more"),
        empty: t("announcements.empty"),
      }}
    >
      {(a) => <Row a={a} t={t} />}
    </ListColumn>
  );
}

/** The title of the announcement of `id` once it is known, for the screen's heading and the document's title. */
export function useAnnouncementTitle(id: string): string | undefined {
  const { t } = stringsIn(useLocale().locale);
  const { announcement } = useAnnouncement(id);
  return announcement === undefined ? undefined : titleOf(announcement, t);
}

/**
 * The page of the announcement of `id`, which marks it read while it shows and is unread; `onBack` goes back to the
 * list.
 */
export function AnnouncementPage({ id, onBack }: { id: string; onBack: () => void }) {
  const { t, lang } = stringsIn(useLocale().locale);
  const { announcement: a, error } = useAnnouncement(id);
  const markRead = useMarkAnnouncementRead();
  const opens = a !== undefined && !a.read && !a.ended;
  // Once for each announcement opened, however often the page renders before the server's answer.
  const marked = useRef<string | null>(null);
  useEffect(() => {
    if (!opens || marked.current === id) return;
    marked.current = id;
    void markRead(id).catch(() => undefined);
  }, [opens, id, markRead]);
  const back = <BackButton onPress={onBack}>{t("announcements.back")}</BackButton>;
  if (a === undefined)
    return (
      <>
        {back}
        {error === null ? (
          <FeedNotice>{t("announcements.loading_one")}</FeedNotice>
        ) : (
          <FeedNotice alert action={<TextLink href="/announcements">{t("announcements.all")}</TextLink>}>
            {t("announcements.not_found")}
          </FeedNotice>
        )}
      </>
    );
  return (
    <>
      {back}
      <Entry>
        <NoticeLine icon="announcements" date={a.published_at}>
          <strong dir="auto">{titleOf(a, t)}</strong>
        </NoticeLine>
        <PostContent html={a.content} lang={undefined} />
        <CaptionLine icon="events">{period(a, t, lang)}</CaptionLine>
      </Entry>
    </>
  );
}
