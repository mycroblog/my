// SPDX-License-Identifier: AGPL-3.0-only

// The board of an event (ADR-0161 §17), under the Board tab of its page, for a reader who reads it: at its top, for a
// reader who writes on it now, the bar that opens the app's composer for a post on the board (a modal of the app's
// state, which gives the focus back to the bar as it closes); else why nobody writes on it any more (the event was
// cancelled, or 7 days passed since its end), or that its writers leave the reader out. Under it, its posts, newest
// first, as any timeline's column draws them: they load, fail with Try again, ask for more near the end, and say when
// there are none. A post the reader writes comes in at the top at once (app-data), and its actions are every post's
// (post-actions.tsx), but boost and quote, which no post of a board has. The bar gives way to the line as the server
// says the reader writes no more, and as the 7 days after the end pass while the page is open (the page's moment);
// the focus it held, or the composer it opened, goes then to the Board tab.

import { Column, nameOf, type PostStatus } from "@my/app-components";
import { useEventBoard, useViewer, writesOnEventBoard, type ServerEvent } from "@my/app-data";
import { Avatar, Box, FeedNotice, PromptBar, Stack } from "@my/design-system";
import { useId, useLayoutEffect, useRef } from "react";
import { useCompose, usePostActions, usePostOwn, usePostPicker, usePostVoting } from "../post-actions";
import { TranslatablePost } from "../post-translation";
import { focusWhenFree, type T } from "./parts";

/** Whether the board's writers (`board_writers`) admit the reader, whatever the board's period says. */
function admitted(event: ServerEvent): boolean {
  const { organizer, coorganizer } = event.viewer;
  switch (event.board_writers) {
    case "organizer":
      return organizer;
    case "coorganizers":
      return organizer || coorganizer;
    case "readers":
      return true;
  }
}

/**
 * Why the reader does not write on the board, where the bar would be: the event was cancelled; 7 days passed since its
 * end (its writers admit the reader, so the period alone closes it); or its writers leave the reader out.
 */
function whyNot(event: ServerEvent, t: T): string {
  if (event.state === "canceled") return t("events.board.closed", { reason: "canceled" });
  if (admitted(event)) return t("events.board.closed", { reason: "ended" });
  return t("events.board.not_writer", { writers: event.board_writers });
}

/** The bar that opens the composer for a post on the board, the reader's avatar at its start. */
function WriteBar({
  event,
  onOpen,
  onFocusChange,
  t,
}: {
  event: ServerEvent;
  onOpen: () => void;
  onFocusChange: (f: boolean) => void;
  t: T;
}) {
  const compose = useCompose();
  const viewer = useViewer(compose !== undefined);
  return (
    <PromptBar
      avatar={
        <Avatar
          name={viewer === undefined ? "" : nameOf(viewer)}
          src={viewer?.avatar?.small.url}
          size="post"
          decorative
        />
      }
      onPress={() => {
        onOpen();
        compose?.({ kind: "board", event: event.id, title: event.title });
      }}
      onFocusChange={onFocusChange}
    >
      {t("events.board.write")}
    </PromptBar>
  );
}

/** The board of `event`, which the reader reads, at the page's moment `now`. */
export function Board({ event, now, t }: { event: ServerEvent; now: number; t: T }) {
  const writes = writesOnEventBoard(event, now);
  const onAction = usePostActions();
  const votingOf = usePostVoting();
  const ownOf = usePostOwn();
  const picker = usePostPicker();
  const board = useEventBoard(event.id);
  // Whether the focus is the bar's, kept from its own events: while it holds the focus, or while the composer it
  // opened does. When the bar gives way to the line, that focus goes to the Board tab, which labels the panel the line
  // is in: at once, or as the composer closes, which would give it back to the bar that is gone.
  const barFocused = useRef(false);
  const opened = useRef(false);
  const line = useId();
  useLayoutEffect(() => {
    if (writes || (!barFocused.current && !opened.current)) return;
    barFocused.current = false;
    opened.current = false;
    const tab = document.getElementById(line)?.closest("[role=tabpanel]")?.getAttribute("aria-labelledby");
    if (tab === null || tab === undefined) return;
    focusWhenFree(() => {
      document.getElementById(tab)?.focus();
    });
  }, [writes, line]);
  return (
    <Stack gap="entry-room">
      {writes ? (
        <WriteBar
          event={event}
          onOpen={() => {
            opened.current = true;
          }}
          onFocusChange={(focused) => {
            barFocused.current = focused;
            // The focus back on the bar: the composer it opened has closed.
            if (focused) opened.current = false;
          }}
          t={t}
        />
      ) : (
        <Box as="p" id={line} text="caption" tone="muted" dir="auto">
          {whyNot(event, t)}
        </Box>
      )}
      <Column
        label={t("events.board.feed")}
        {...board}
        onAction={onAction}
        postProps={(s: PostStatus) => ({ voting: votingOf(s), own: ownOf(s), picker })}
        postAs={TranslatablePost}
        empty={<FeedNotice>{t("events.board.empty")}</FeedNotice>}
      />
    </Stack>
  );
}
