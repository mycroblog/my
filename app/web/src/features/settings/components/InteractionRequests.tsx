// SPDX-License-Identifier: AGPL-3.0-only
// The screen of the approval requests (web-client/screens/settings-interaction-requests.md, ADR-0155 §10): the replies,
// boosts, and quotes of the viewer's posts that wait for their approval, each with the account, what it asks, the
// reply or the quoting post when the server has it, the viewer's post, and the answers.
import { AccountItem, Box, Button, Cluster, EmptyState, Stack } from "@my/ui";
import { useRef } from "react";
import { handleOf } from "../../../lib/handle";
import { imageSources } from "../../../lib/images";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { CompactStatus, type InteractionRequest } from "../../status";
import type { RequestAnswer } from "../api/interactionRequests";
import { useInteractionRequests } from "../hooks/useInteractionRequests";

/** What each kind of request asks, under the account's handle. */
const asks: Readonly<Record<InteractionRequest["kind"], ClientMessageKey>> = {
  reply: "client:interaction_requests.kind.reply",
  announce: "client:interaction_requests.kind.announce",
  quote: "client:interaction_requests.kind.quote",
};

/** The label of the interacting post, by kind; a boost has none. */
const interactingLabel: Readonly<Record<"reply" | "quote", ClientMessageKey>> = {
  reply: "client:interaction_requests.interacting.reply",
  quote: "client:interaction_requests.interacting.quote",
};

export function InteractionRequests() {
  const t = useT();
  const requests = useInteractionRequests();
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    requests.items.map((request) => request.id),
  );
  if (requests.items.length === 0)
    return (
      <EmptyState
        heading={t("client:interaction_requests.empty")}
        description={t("client:interaction_requests.empty.description")}
      />
    );
  return (
    <Stack gap="4">
      <Stack as="ul" ref={listRef} aria-label={t("client:interaction_requests.title")} divided>
        {requests.items.map((request) => (
          <Box as="li" key={request.id} data-row-id={request.id} container paddingY="3">
            <RequestRow
              request={request}
              pending={requests.pending?.id === request.id ? requests.pending.answer : null}
              onAnswer={(answer) => {
                requests.answer(request, answer, () => {
                  removed(request.id);
                });
              }}
            />
          </Box>
        ))}
      </Stack>
      {requests.hasMore && (
        <div>
          <Button onPress={requests.loadMore} isPending={requests.isLoadingMore}>
            {t("client:settings.list.more")}
          </Button>
        </div>
      )}
    </Stack>
  );
}

type RequestRowProps = {
  request: InteractionRequest;
  /** The answer being sent, or null. */
  pending: RequestAnswer | null;
  onAnswer: (answer: RequestAnswer) => void;
};

/** One request: who asks and what, the posts, and the answers, which name the account (every row has the same ones). */
function RequestRow({ request, pending, onAnswer }: RequestRowProps) {
  const t = useT();
  const { account, kind } = request;
  const name = account.display_name || account.username;
  const interacting = kind === "announce" ? undefined : request.interacting_status;
  const approve = t("client:interaction_requests.accept");
  const decline = t("client:interaction_requests.reject");
  // The answers come first in the row, as on the follow requests: the focus goes to the next row's first button.
  const answers = (
    <Cluster gap="2">
      <Button
        size="sm"
        variant="primary"
        isPending={pending === "accept"}
        isDisabled={pending === "reject"}
        aria-label={t("client:settings.list.action", { action: approve, name })}
        onPress={() => {
          onAnswer("accept");
        }}
      >
        {approve}
      </Button>
      <Button
        size="sm"
        isPending={pending === "reject"}
        isDisabled={pending === "accept"}
        aria-label={t("client:settings.list.action", { action: decline, name })}
        onPress={() => {
          onAnswer("reject");
        }}
      >
        {decline}
      </Button>
    </Cluster>
  );
  return (
    <Stack gap="3">
      <AccountItem
        name={name}
        handle={handleOf(account.acct)}
        avatar={imageSources(account.avatar)}
        avatarSeed={account.id}
        details={<span>{t(asks[kind])}</span>}
        trailing={answers}
        trailingPlacement="below"
        paragraph
      />
      {interacting !== undefined && kind !== "announce" && (
        <CompactStatus status={interacting} label={t(interactingLabel[kind])} />
      )}
      <CompactStatus status={request.status} label={t("client:interaction_requests.own_post")} />
    </Stack>
  );
}
