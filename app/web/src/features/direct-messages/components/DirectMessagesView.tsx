// SPDX-License-Identifier: AGPL-3.0-only
// The direct messages page below its heading (ADR-0154 §6; screens/direct-messages.md): the user's conversations, the
// one with the newest direct post first, as a feed (web-client/README.md Y6, the WAI-ARIA APG feed pattern), loaded a
// page at a time. The stream keeps the rows and their unread marks up to date (../api/live.ts).
import { EmptyState, Feed } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { SignInPrompt } from "../../session";
import { useDirectMessages } from "../hooks/useDirectMessages";
import { ConversationItem } from "./ConversationItem";
import { DirectMessagesError } from "./DirectMessagesError";
import { DirectMessagesSkeleton } from "./DirectMessagesSkeleton";

export type DirectMessagesViewProps = {
  /** The ID of the page heading, which names the feed. */
  headingId: string;
};

export function DirectMessagesView({ headingId }: DirectMessagesViewProps) {
  const t = useT();
  const list = useDirectMessages();
  switch (list.status) {
    case "signed-out":
      return (
        <SignInPrompt
          heading={t("client:direct_messages.signed_out.heading")}
          description={t("client:direct_messages.signed_out")}
          returnTo="/direct-messages"
        />
      );
    case "loading":
      return <DirectMessagesSkeleton />;
    case "error":
      return (
        <DirectMessagesError headingLevel={2} message={list.message} retry={list.retry} isRetrying={list.isRetrying} />
      );
    case "loaded":
      // A page may hold no conversation the user may see while more follow: the feed then offers the next one.
      if (list.items.length === 0 && !list.hasMore) {
        return (
          <EmptyState
            heading={t("client:direct_messages.empty")}
            description={t("client:direct_messages.empty.description")}
          />
        );
      }
      return (
        <Feed
          aria-labelledby={headingId}
          items={list.items}
          getKey={(conversation) => conversation.id}
          renderItem={(conversation) => (
            <ConversationItem
              conversation={conversation}
              onOpen={() => {
                list.open(conversation);
              }}
            />
          )}
          isLoading={list.isLoadingMore}
          hasMore={list.hasMore}
          onLoadMore={list.loadMore}
          loadMoreLabel={t("client:direct_messages.load_more")}
          loadingLabel={t("client:direct_messages.loading")}
          estimateSize={140}
        />
      );
  }
}
