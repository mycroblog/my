// SPDX-License-Identifier: AGPL-3.0-only
import { Button, EmptyState, PostSkeleton, RegionError, ReplyIcon, Section, Stack, VisuallyHidden } from "@my/ui";
import { useRef } from "react";
import { useT } from "../../../lib/i18n";
import { useReply } from "../../compose";
import type { Status } from "../api/status";
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
        <ThreadReplies thread={thread} status={status} />
      </Section>
    </Stack>
  );
}

/** Placeholder replies, for as long as the thread loads. */
const replyPlaceholders = ["first", "second"] as const;

function ThreadReplies({ thread, status }: { thread: ThreadState; status: Status }) {
  const t = useT();
  const reply = useReply();
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
      // The section's failure in its one form (G16); the post above stays.
      return (
        <RegionError
          headingLevel={3}
          heading={t("client:status.thread.failed.heading")}
          reason={thread.message}
          retryLabel={t("client:status.thread.retry")}
          onRetry={thread.retry}
          isRetrying={thread.isRetrying}
        />
      );
    case "loaded":
      // No replies yet: what comes next on this page is a reply (G14), offered while someone is signed in.
      return thread.descendants.length === 0 ? (
        <EmptyState
          headingLevel={3}
          heading={t("client:status.thread.no_replies")}
          {...(reply !== null && {
            description: t("client:status.thread.no_replies.description"),
            action: (
              <Button
                onPress={() => {
                  reply(status.boost ?? status);
                }}
              >
                <ReplyIcon />
                {t("client:status.thread.no_replies.action")}
              </Button>
            ),
          })}
        />
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
