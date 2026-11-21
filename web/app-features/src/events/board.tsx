// SPDX-License-Identifier: AGPL-3.0-only

// The board of an event (ADR-0161 §17), under the Board tab of its page, for a reader who reads it: at its top, for a
// reader who writes on it now, the bar that opens the app's composer for a post on the board (a modal of the app's
// state, which gives the focus back to the bar as it closes); else why nobody writes on it any more (the event was
// cancelled, or 7 days passed since its end), or that its writers leave the reader out. Under it, its posts, newest
// first, as any timeline's column draws them: they load, fail with Try again, ask for more near the end, and say when
// there are none. A post the reader writes comes in at the top at once (app-data), and its actions are every post's
// (post-actions.tsx), but boost and quote, which no post of a board has. When the bar gives way to the read-only line
// while it holds the focus (the event was cancelled meanwhile), the focus goes to the Board tab.

import { Column, nameOf, type PostStatus } from "@my/app-components";
import { useEventBoard, useViewer, type ServerEvent } from "@my/app-data";
import { Avatar, Box, FeedNotice, PromptBar, Stack } from "@my/design-system";
import { useId, useLayoutEffect, useRef } from "react";
import { useCompose, usePostActions, usePostOwn, usePostPicker, usePostVoting } from "../post-actions";
import { TranslatablePost } from "../post-translation";
import type { T } from "./parts";

/** Whether the board's writers (`board_writers`) admit the reader, whatever the board's period says. */
function admitted(event: ServerEvent): boolean {
  const { organizer, coorganizer } = event.viewer;
  switch (event.board_writers) {
    case "organizer":
      return organizer;
    case "organizers":
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
  if (event.state === "cancelled") return t("events.board.closed", { reason: "cancelled" });
  if (admitted(event)) return t("events.board.closed", { reason: "ended" });
  return t("events.board.not_writer", { writers: event.board_writers });
}

/** The bar that opens the composer for a post on the board, the reader's avatar at its start. */
function WriteBar({ event, onFocusChange, t }: { event: ServerEvent; onFocusChange: (f: boolean) => void; t: T }) {
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
        compose?.({ kind: "board", event: event.id, title: event.title });
      }}
      onFocusChange={onFocusChange}
    >
      {t("events.board.write")}
    </PromptBar>
  );
}

/** The board of `event`, which the reader reads. */
export function Board({ event, t }: { event: ServerEvent; t: T }) {
  const writes = event.viewer.board.writable;
  const onAction = usePostActions();
  const votingOf = usePostVoting();
  const ownOf = usePostOwn();
  const picker = usePostPicker();
  const board = useEventBoard(event.id);
  // Whether the bar holds the focus, kept from its own events: when it gives way to the line, the focus it had goes to
  // the Board tab, which labels the panel the line is in.
  const barFocused = useRef(false);
  const line = useId();
  useLayoutEffect(() => {
    if (writes || !barFocused.current) return;
    barFocused.current = false;
    const tab = document.getElementById(line)?.closest("[role=tabpanel]")?.getAttribute("aria-labelledby");
    if (tab !== null && tab !== undefined) document.getElementById(tab)?.focus();
  }, [writes, line]);
  return (
    <Stack gap="entry-room">
      {writes ? (
        <WriteBar
          event={event}
          onFocusChange={(focused) => {
            barFocused.current = focused;
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
