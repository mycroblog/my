// SPDX-License-Identifier: AGPL-3.0-only

// The custom emoji as a holder of `manage_custom_emojis` keeps them (FR-111, ADR-0051, ADR-0058 §1), at
// /admin/custom-emojis: under the tabs Emoji | Requests (the pending requests counted, ADR-0070 §9), every emoji, local
// and remote, newest first, kept to this server's or to others' and found by its shortcode, which the address keeps;
// each its image, :shortcode:, its category and aliases or its server, whether the picker leaves it out, and whether it
// is shown or disabled, a row that opens its page; a page at a time by the server's cursor; and Register an emoji.
// Where the list has nothing to show, it says why.

import {
  loadAdminEmojiRequests,
  loadAdminEmojis,
  useAdminEmojis,
  type AdminCustomEmoji,
  type AdminEmojiQuery,
  type AppData,
} from "@my/app-data";
import {
  Box,
  FeedNotice,
  FilterBar,
  FilterFacet,
  RecordPages,
  RecordTable,
  ScreenStack,
  type FilterValue,
  type RecordColumn,
  type RecordRow,
} from "@my/design-system";
import type { ReactNode } from "react";
import type { StaffEntryProps, StaffSearch } from "../entries";
import { ReadNotice, useStaffStrings, type T } from "../parts";
import { adminEmojiPath, coded, EmojiImage, EmojiState, EmojiTabs, EmojiTitle, originOf } from "./emoji-parts";

/** What the list shows at an address's query: every emoji unless it names an origin or a shortcode, and a page. */
function emojiQueryOf(search: StaffSearch): AdminEmojiQuery {
  const { origin, shortcode, cursor } = search;
  return {
    origin: origin === "local" || origin === "remote" ? origin : undefined,
    shortcode: shortcode === undefined || shortcode === "" ? undefined : shortcode,
    cursor: cursor === undefined || cursor === "" ? undefined : cursor,
  };
}

/** The query of the list's address for `q`, after the `trail` of pages before it. */
function searchOf(q: AdminEmojiQuery, trail: readonly string[] = []): StaffSearch {
  return {
    ...(q.origin === undefined ? {} : { origin: q.origin }),
    ...(q.shortcode === undefined ? {} : { shortcode: q.shortcode }),
    ...(q.cursor === undefined ? {} : { cursor: q.cursor }),
    ...(trail.length === 0 ? {} : { trail: trail.map((c) => encodeURIComponent(c)).join(",") }),
  };
}

/** The cursors of the pages before the one shown, the first page's as empty. */
const trailOf = (search: StaffSearch): string[] =>
  search["trail"] === undefined ? [] : search["trail"].split(",").map((c) => decodeURIComponent(c));

/** Reads what the list shows at `search`, and the pending requests its tab counts, for the route's loader. */
export async function loadEmojiScreen(data: AppData, search: StaffSearch): Promise<void> {
  await Promise.all([loadAdminEmojis(data, emojiQueryOf(search)), loadAdminEmojiRequests(data, "pending")]);
}

/** What a row says under a local emoji's shortcode: its category and aliases; nothing for another server's. */
function captionOf(t: T, emoji: AdminCustomEmoji): string | undefined {
  if (!emoji.local) return undefined;
  const parts = [
    ...(emoji.category === undefined ? [] : [t("staff_emoji.row.category", { category: emoji.category })]),
    ...(emoji.aliases.length === 0
      ? []
      : [t("staff_emoji.row.aliases", { aliases: emoji.aliases.map(coded).join(" ") })]),
  ];
  return parts.length === 0 ? undefined : parts.join(" · ");
}

type Column = "image" | "origin" | "state";

/** The custom emoji: their tabs, their filters, a page of them, and the pages. */
export function AdminEmojiScreen({ search, onSearch, go }: StaffEntryProps): ReactNode {
  const { t } = useStaffStrings();
  const q = emojiQueryOf(search);
  const trail = trailOf(search);
  const page = useAdminEmojis(q);
  const filtered = q.origin !== undefined || q.shortcode !== undefined;
  const change = (next: Partial<AdminEmojiQuery>) => {
    onSearch(searchOf({ ...q, ...next, cursor: undefined }));
  };
  const origins: FilterValue[] = [
    { id: "local", label: t("staff_emoji.origin.local"), icon: "home" },
    { id: "remote", label: t("staff_emoji.origin.remote"), icon: "federated" },
  ];
  const columns: RecordColumn<Column>[] = [
    { key: "image", label: t("staff_emoji.column.image"), kind: "fit" },
    { key: "origin", label: t("staff_emoji.origin"), bare: true },
    { key: "state", label: t("staff_emoji.column.state"), kind: "fit", beside: true },
  ];
  const rows = (page.items ?? []).map((e): RecordRow<Column> => ({
    id: e.id,
    head: coded(e.shortcode),
    caption: captionOf(t, e),
    href: adminEmojiPath(e.id),
    cells: {
      image: <EmojiImage shortcode={e.shortcode} url={e.url} staticUrl={e.static_url} />,
      // A server's domain is one word of its column, which keeps it whole.
      origin: (
        <Box as="span" nowrap>
          {originOf(t, e)}
        </Box>
      ),
      state: <EmojiState emoji={e} />,
    },
  }));
  const previous = trail.at(-1);
  return (
    <ScreenStack busy={page.stale}>
      <EmojiTitle />
      <EmojiTabs current="emoji" go={go}>
        <ScreenStack busy={page.stale}>
          <FilterBar
            label={t("staff_emoji.filters")}
            search={{
              label: t("staff_emoji.search"),
              value: q.shortcode ?? "",
              onChange: (shortcode) => {
                change({ shortcode: shortcode.replace(/:/g, "") || undefined });
              },
              clearLabel: t("staff_emoji.search.clear"),
            }}
            applied={filtered}
            onClear={() => {
              onSearch({});
            }}
          >
            <FilterFacet
              label={t("staff_emoji.origin")}
              any={t("staff_emoji.origin.any")}
              values={origins}
              chosen={q.origin}
              onChoose={(origin) => {
                change({ origin: origin === "local" || origin === "remote" ? origin : undefined });
              }}
            />
          </FilterBar>
          {page.items === undefined ? (
            <ReadNotice
              status={page.status === "failed" ? "error" : "pending"}
              loading={t("staff_emoji.loading")}
              failed={t("staff_emoji.failed")}
              retry={page.retry}
            />
          ) : (
            <>
              <RecordTable
                label={t("staff_emoji.table")}
                head={{ label: t("staff_emoji.column.shortcode") }}
                columns={columns}
                rows={rows}
                busy={page.stale}
                even
                empty={<FeedNotice>{t(filtered ? "staff_emoji.empty.filtered" : "staff_emoji.empty")}</FeedNotice>}
              />
              <RecordPages
                previous={
                  previous === undefined
                    ? undefined
                    : {
                        onPress: () => {
                          onSearch(
                            searchOf({ ...q, cursor: previous === "" ? undefined : previous }, trail.slice(0, -1)),
                          );
                        },
                      }
                }
                next={
                  page.next === undefined
                    ? undefined
                    : {
                        onPress: () => {
                          onSearch(searchOf({ ...q, cursor: page.next }, [...trail, q.cursor ?? ""]));
                        },
                      }
                }
              />
            </>
          )}
        </ScreenStack>
      </EmojiTabs>
    </ScreenStack>
  );
}
