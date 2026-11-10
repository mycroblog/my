// SPDX-License-Identifier: AGPL-3.0-only
// The screen of the notification requests (web-client/screens/notifications-requests.md, ADR-0158 §7): what the
// requests are and what answering does, with the link to the notification policy, then one row per source account
// with how many notifications wait, the answers, and the link to read them.
import { AccountItem, Box, Button, Cluster, EmptyState, Link, Stack, Text } from "@my/ui";
import { useRef } from "react";
import { handleOf } from "../../../lib/handle";
import { imageSources } from "../../../lib/images";
import { profilePath } from "../../../lib/paths";
import { useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { type NotificationRequest, notificationRequestPath, type RequestAnswer } from "../api/requests";
import { requestName, useNotificationRequests } from "../hooks/useNotificationRequests";

/** The notification settings, where the policy is chosen. */
const notificationSettingsPath = "/settings/notifications";

export function NotificationRequests() {
  const t = useT();
  const requests = useNotificationRequests();
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    requests.items.map((request) => request.id),
  );
  return (
    <Stack gap="6">
      <Stack gap="2">
        <Text as="p" tone="fg">
          {t("client:notification_requests.intro")}
        </Text>
        <div>
          <Link variant="standalone" href={notificationSettingsPath}>
            {t("client:notification_requests.settings_link")}
          </Link>
        </div>
      </Stack>
      {requests.items.length === 0 ? (
        <EmptyState
          heading={t("client:notification_requests.empty")}
          description={t("client:notification_requests.empty.description")}
        />
      ) : (
        <Stack gap="4">
          <Stack as="ul" ref={listRef} aria-label={t("client:notification_requests.title")} divided>
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
                {t("client:notification_requests.load_more")}
              </Button>
            </div>
          )}
        </Stack>
      )}
    </Stack>
  );
}

type RequestRowProps = {
  request: NotificationRequest;
  /** The answer being sent, or null. */
  pending: RequestAnswer | null;
  onAnswer: (answer: RequestAnswer) => void;
};

/**
 * One request: who sent the held notifications and how many, then the answers and the link to the notifications, each
 * named with the account (every row has the same ones, Y2).
 */
function RequestRow({ request, pending, onAnswer }: RequestRowProps) {
  const t = useT();
  const { account } = request;
  const name = requestName(request);
  const accept = t("client:notification_requests.accept");
  const dismiss = t("client:notification_requests.dismiss");
  const view = t("client:notification_requests.view");
  // The answers come first in the row, as on the approval requests: the focus goes to the next row's first button.
  const answers = (
    <Cluster gap="2" align="center">
      <Button
        size="sm"
        variant="primary"
        isPending={pending === "accept"}
        isDisabled={pending === "dismiss"}
        aria-label={t("client:notification_requests.action", { action: accept, name })}
        onPress={() => {
          onAnswer("accept");
        }}
      >
        {accept}
      </Button>
      <Button
        size="sm"
        isPending={pending === "dismiss"}
        isDisabled={pending === "accept"}
        aria-label={t("client:notification_requests.action", { action: dismiss, name })}
        onPress={() => {
          onAnswer("dismiss");
        }}
      >
        {dismiss}
      </Button>
      <Link
        variant="standalone"
        size="sm"
        href={notificationRequestPath(account.id)}
        aria-label={t("client:notification_requests.action", { action: view, name })}
      >
        {view}
      </Link>
    </Cluster>
  );
  return (
    <AccountItem
      name={name}
      handle={handleOf(account.acct)}
      avatar={imageSources(account.avatar)}
      avatarSeed={account.id}
      href={profilePath(account.acct)}
      details={<span>{t("client:notification_requests.count", { count: request.notifications_count })}</span>}
      trailing={answers}
      trailingPlacement="below"
    />
  );
}
