// SPDX-License-Identifier: AGPL-3.0-only

// The samples of the landing page's What you can do here: for each thing a member can do that is on here, a screen of
// it in miniature, drawn with the parts the app draws it with and content of its own that stands for no one's (a
// room, an event, an article, a search, a translated post, a supporters' plan). The landing page shows them hidden
// from assistive technology and the pointer, with the thing's name and its sentence.

import {
  EventListLine,
  handleOf,
  nameOf,
  Post,
  PostContent,
  VoiceRoomLine,
  type EventShown,
  type PostAccount,
  type PostStatus,
  type RoomShown,
} from "@my/app-components";
import {
  Avatar,
  Byline,
  CaptionLine,
  instantAt,
  Money,
  PlanCard,
  SearchField,
  Stack,
  Stamp,
  timeZoneName,
  useLocale,
  useTimeZone,
  wallTime,
} from "@my/design-system";
import type { ReactNode } from "react";
import { useStrings } from "../locale";

const hour = 3_600_000;
const day = 24 * hour;

/** A person of the samples, on this server or on another. */
function person(username: string, name: string, domain: string): PostAccount {
  return { id: `sample-${username}`, username, acct: username, domain, display_name: name, emojis: [] };
}

/** A text as the server renders a post's: one paragraph, its characters escaped. */
const paragraph = (text: string) =>
  `<p>${text.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;")}</p>`;

/** A public post of the samples, written `ago` ms before `now`. */
function status(id: string, account: PostAccount, text: string, language: string, ago: number, now: number) {
  return {
    id: `sample-${id}`,
    account,
    content: paragraph(text),
    spoiler_text: "",
    visibility: "public",
    language,
    created_at: new Date(now - ago).toISOString(),
    media_attachments: [],
    mentions: [],
    emojis: [],
    replies_count: 0,
    boosts_count: 0,
    favourites_count: 0,
    reactions: [],
  } satisfies PostStatus;
}

/** The sample of each thing a member can do, by its key (landingFeatures), on the server at `domain`. */
export function useLandingSamples(domain: string): Readonly<Record<string, ReactNode>> {
  const t = useStrings();
  const { locale } = useLocale();
  const timeZone = useTimeZone();
  const now = Date.now();
  const mika = person("mika", "Mika", domain);
  const ren = person("ren", "Ren", domain);
  // Someone on another server, who writes in another language than the reader's.
  const sam = person("sam", "Sam", "another.example");
  const theirs = locale.startsWith("ja") ? "en" : "ja";
  const post = t("app.landing_sample_post");
  const room: RoomShown = {
    id: "sample-room",
    name: t("app.landing_sample_room"),
    host: mika,
    participants_count: 6,
    participants: [
      { account: mika, role: "speaker", hosting: true },
      { account: ren, role: "speaker", hosting: false },
      { account: sam, role: "speaker", hosting: false },
    ],
  };
  // In three days, at nine in the morning of the reader's zone.
  const starts = instantAt({ ...wallTime(new Date(now + 3 * day), timeZone), hour: 9, minute: 0 }, timeZone).getTime();
  const event: EventShown = {
    id: "sample-event",
    title: t("app.landing_sample_event"),
    organizer: ren,
    time_zone: timeZoneName(timeZone),
    starts_at: new Date(starts).toISOString(),
    ends_at: new Date(starts + 2 * hour).toISOString(),
    state: "scheduled",
    places: [{ kind: "physical" }],
    hidden_places: [],
    going_count: 12,
  };
  const article: PostStatus = {
    ...status("article", mika, "", locale, 5 * hour, now),
    content: "",
    article: { title: t("app.landing_sample_article_title"), summary: t("app.landing_sample_article_summary") },
  };
  // The server's currency is the operator's: a sample's is the reader's language's.
  const price = new Intl.NumberFormat(locale, {
    style: "currency",
    currency: theirs === "en" ? "JPY" : "USD",
    maximumFractionDigits: 0,
  }).format(theirs === "en" ? 500 : 5);
  return {
    voice: <VoiceRoomLine room={room} />,
    events: <EventListLine event={event} now={now} />,
    articles: <Post status={article} review />,
    search: (
      <Stack gap="entry-room">
        {/* Hidden from assistive technology, its field's names are never heard: search's own word stands in. Its words
            are shown alone, with no clear. */}
        <SearchField
          id="landing-sample-search"
          label={t("app.search")}
          labelHidden
          value={t("app.landing_sample_search")}
          onChange={() => undefined}
          clearLabel={t("app.search")}
          readOnly
        />
        <Post status={status("found", ren, post, locale, 2 * hour, now)} review />
      </Stack>
    ),
    // A post of someone on another server in their language, as the reader reads it translated: its byline, the
    // translation, and the line that says it was translated, without Show original, which a sample cannot do.
    translation: (
      <Stack gap="entry-line">
        <Byline
          avatar={<Avatar name={nameOf(sam)} size="post" decorative />}
          name={nameOf(sam)}
          handle={handleOf(sam)}
          end={<Stamp date={new Date(now - 40 * 60_000)} />}
        />
        <PostContent html={paragraph(post)} lang={locale} />
        <CaptionLine icon="translate">
          {t("app.landing_sample_translated", {
            language: new Intl.DisplayNames([locale], { type: "language" }).of(theirs) ?? theirs,
          })}
        </CaptionLine>
      </Stack>
    ),
    supporters: (
      <PlanCard
        name={t("app.landing_sample_plan")}
        price={<Money amount={price} per={t("app.landing_sample_month")} size="headline" />}
        perks={[]}
      />
    ),
  };
}
