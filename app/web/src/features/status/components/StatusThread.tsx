// SPDX-License-Identifier: AGPL-3.0-only
import { Box, Button, PostSkeleton, Section, Stack, Text, VisuallyHidden } from "@my/ui";
import { useRef } from "react";
import { useT } from "../../../lib/i18n";
import { useRevealOnce } from "../hooks/useRevealOnce";
import { type ThreadPost, type ThreadState, useStatusThread } from "../hooks/useStatusThread";
import { StatusCard } from "./StatusCard";

export type StatusThreadProps = {
  /** The ID of the post the page is about; the route loader has fetched it. */
  id: string;
  /** Called after the viewer deleted that post, so the page can leave it. */
  onDeleted: () => void;
};

/**
 * A post and the thread around it (FR-113): the posts it replies to, oldest first, under their own
 * heading; the post itself, highlighted, with its full date; and the replies, depth first, under theirs.
 * A line joins each post to the one it replies to when that is right above it. The thread loads after
 * the post, so the post is on screen at once; when posts appear above it, the page scrolls it back to the
 * top of the screen.
 */
export function StatusThread({ id, onDeleted }: StatusThreadProps) {
  const t = useT();
  const { status, thread } = useStatusThread(id);
  const focusedRef = useRef<HTMLElement>(null);
  const hasAncestors = thread.status === "loaded" && thread.ancestors.length > 0;
  useRevealOnce(focusedRef, hasAncestors);

  return (
    <Stack>
      {thread.status === "loaded" && thread.ancestors.length > 0 && (
        <Section surface="rows" title={t("client:status.thread.ancestors")}>
          <StatusList posts={thread.ancestors} />
        </Section>
      )}
      <StatusCard
        ref={focusedRef}
        status={status}
        variant="focused"
        thread={{ lineAbove: thread.status === "loaded" && thread.repliesToAncestor, lineBelow: false }}
        onDeleted={onDeleted}
      />
      <Section surface="rows" title={t("client:status.thread.replies")} busy={thread.status === "loading"}>
        <ThreadReplies thread={thread} />
      </Section>
    </Stack>
  );
}

/** Placeholder replies, for as long as the thread loads. */
const replyPlaceholders = ["first", "second"] as const;

function ThreadReplies({ thread }: { thread: ThreadState }) {
  const t = useT();
  switch (thread.status) {
    case "loading":
      // The section is busy meanwhile; the hidden text names what it waits for, once (web-client/16-look-states-and-wording.md G15).
      return (
        <>
          <VisuallyHidden>{t("client:status.thread.loading")}</VisuallyHidden>
          {replyPlaceholders.map((key) => (
            <PostSkeleton key={key} />
          ))}
        </>
      );
    case "error":
      return (
        <Stack align="center" gap="3" padding="4">
          <Text as="p" align="center">
            {t("client:status.thread.failed", { reason: thread.message })}
          </Text>
          <Button onPress={thread.retry}>{t("client:status.thread.retry")}</Button>
        </Stack>
      );
    case "loaded":
      return thread.descendants.length === 0 ? (
        <Box as="p" paddingX="4" paddingBottom="4" tone="muted">
          {t("client:status.thread.no_replies")}
        </Box>
      ) : (
        <StatusList posts={thread.descendants} />
      );
  }
}

function StatusList({ posts }: { posts: readonly ThreadPost[] }) {
  return (
    <ol>
      {posts.map(({ status, lineAbove, lineBelow }) => (
        <li key={status.id}>
          <StatusCard status={status} thread={{ lineAbove, lineBelow }} />
        </li>
      ))}
    </ol>
  );
}
