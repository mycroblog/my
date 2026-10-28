// SPDX-License-Identifier: AGPL-3.0-only
// The requests tab of the custom emoji screen (ADR-0070 §2, manage_custom_emojis): the requests of the holders of
// create_custom_emojis in one state (the pending ones by default, the state kept in the URL by the route,
// web-client/README.md S3), newest first. Each shows the requested image, its shortcode, what it asks for and who asked; a
// pending one is approved at once, or rejected after an alert dialog with an optional reason that the requester
// sees. What a decision did stays above the list in a StatusMessage, and a failure (a request that cannot be decided
// says why) stays there too (role="alert").
import {
  Button,
  Cluster,
  ConfirmDialog,
  DialogTrigger,
  EmojiTile,
  EmptyState,
  Hint,
  InlineError,
  Link,
  ListToolbar,
  ReasonField,
  Select,
  SelectItem,
  Stack,
  StatusMessage,
  TableCell,
  TableColumnHeader,
  TableRow,
  TableRowHeader,
  Text,
} from "@my/ui";
import { useRef, useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { profilePath } from "../../../lib/paths";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { DateTime, RecordTable } from "../../moderation";
import {
  type AdminCustomEmojiRequest,
  type CustomEmojiRequestState,
  customEmojiRequestStates,
  rejectReasonLimit,
} from "../api/customEmojiRequests";
import {
  type CustomEmojiRequestActions,
  requestInText,
  useAdminCustomEmojiRequests,
  useCustomEmojiRequestActions,
} from "../hooks/useCustomEmojiRequests";

/** The name of each state of a request, shared with the user's page of their requests. */
export const requestStateLabels: Readonly<Record<CustomEmojiRequestState, ClientMessageKey>> = {
  pending: "client:custom_emoji_requests.state.pending",
  approved: "client:custom_emoji_requests.state.approved",
  rejected: "client:custom_emoji_requests.state.rejected",
  withdrawn: "client:custom_emoji_requests.state.withdrawn",
  lapsed: "client:custom_emoji_requests.state.lapsed",
};

/** What each kind of request asks for. */
const kindLabels: Readonly<Record<AdminCustomEmojiRequest["kind"], ClientMessageKey>> = {
  create: "client:custom_emoji_requests.kind.create",
  image: "client:custom_emoji_requests.kind.image",
};

function isState(key: unknown): key is CustomEmojiRequestState {
  return customEmojiRequestStates.some((state) => state === key);
}

export type CustomEmojiRequestsProps = {
  /** The state of the listed requests, from the URL. */
  state: CustomEmojiRequestState;
  /** Shows another state: the route puts it into the URL. */
  onStateChange: (state: CustomEmojiRequestState) => void;
};

export function CustomEmojiRequests({ state, onStateChange }: CustomEmojiRequestsProps) {
  const t = useT();
  return (
    <Stack gap="4">
      <Hint>{t("client:admin.custom_emoji_requests.description")}</Hint>
      <ListToolbar label={t("client:admin.list.filters")}>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.custom_emoji_requests.state")}
          value={state}
          onChange={(key) => {
            if (isState(key)) onStateChange(key);
          }}
          items={customEmojiRequestStates.map((id) => ({ id, name: t(requestStateLabels[id]) }))}
        >
          {(choice) => (
            <SelectItem id={choice.id} textValue={choice.name}>
              {choice.name}
            </SelectItem>
          )}
        </Select>
      </ListToolbar>
      <RequestList key={state} state={state} />
    </Stack>
  );
}

function RequestList({ state }: { state: CustomEmojiRequestState }) {
  const t = useT();
  const tableRef = useRef<HTMLTableElement>(null);
  const list = useAdminCustomEmojiRequests(state);
  const removed = useFocusAfterRemoval(
    tableRef,
    list.items.map((request) => request.id),
  );
  const actions = useCustomEmojiRequestActions(removed);
  const empty =
    state === "pending" ? (
      <EmptyState headingLevel={3} heading={t("client:admin.custom_emoji_requests.empty.pending")} />
    ) : (
      <EmptyState headingLevel={3} heading={t("client:admin.custom_emoji_requests.empty")} />
    );
  return (
    <Stack gap="3">
      <StatusMessage>{actions.status}</StatusMessage>
      {actions.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.custom_emoji_requests.decide_failed", { reason: actions.error })}
        </InlineError>
      )}
      <RecordTable
        list={list}
        caption={t("client:admin.custom_emoji_requests.title")}
        getId={(request) => request.id}
        tableRef={tableRef}
        empty={empty}
        head={
          <>
            <TableColumnHeader>{t("client:admin.custom_emojis.shortcode")}</TableColumnHeader>
            <TableColumnHeader>{t("client:custom_emoji_requests.kind")}</TableColumnHeader>
            <TableColumnHeader>{t("client:custom_emoji_requests.state")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.custom_emoji_requests.requester")}</TableColumnHeader>
            <TableColumnHeader>{t("client:custom_emoji_requests.created_at")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.list.column.actions")}</TableColumnHeader>
          </>
        }
      >
        {(request) => <RequestRow request={request} actions={actions} />}
      </RecordTable>
    </Stack>
  );
}

function RequestRow({ request, actions }: { request: AdminCustomEmojiRequest; actions: CustomEmojiRequestActions }) {
  const t = useT();
  const name = requestInText(request);
  const heading = `:${request.shortcode}:`;
  const isPending = actions.pendingId === request.id;
  const { requester } = request;
  return (
    <TableRow data-row-id={request.id}>
      <TableRowHeader wrap="anywhere">
        <Cluster gap="3" align="center" wrap={false}>
          {/* The name of the emoji; the image beside it shows what was requested. */}
          <EmojiTile
            emoji={
              request.url !== undefined && request.static_url !== undefined
                ? { shortcode: request.shortcode, url: request.url, static_url: request.static_url }
                : undefined
            }
          />
          <Stack gap="1">
            <Text weight="semibold" mono wrap="anywhere" dir="ltr">
              {heading}
            </Text>
            {request.url === undefined && (
              <Text size="sm" tone="muted">
                {t("client:custom_emoji_requests.no_image")}
              </Text>
            )}
          </Stack>
        </Cluster>
      </TableRowHeader>
      <TableCell>{t(kindLabels[request.kind])}</TableCell>
      <TableCell>
        <Stack gap="1">
          <span>{t(requestStateLabels[request.state])}</span>
          {request.decided_at !== undefined && (
            <Text size="sm" tone="muted">
              <DateTime value={request.decided_at} />
            </Text>
          )}
          {request.reason !== undefined && (
            <Text size="sm" whitespace="preLine" dir="auto">
              {request.reason}
            </Text>
          )}
        </Stack>
      </TableCell>
      <TableCell wrap="anywhere">
        {requester === undefined ? (
          t("client:admin.custom_emoji_requests.requester.deleted")
        ) : (
          <AccountLink acct={requester.acct} />
        )}
      </TableCell>
      <TableCell>
        <DateTime value={request.created_at} />
      </TableCell>
      <TableCell>
        {request.state === "pending" && (
          <Cluster gap="2">
            <Button
              size="sm"
              variant="primary"
              isPending={isPending}
              aria-label={t("client:admin.custom_emoji_requests.approve_request", { emoji: name })}
              onPress={() => {
                void actions.approve(request);
              }}
            >
              {t("client:admin.custom_emoji_requests.approve")}
            </Button>
            <RejectRequest request={request} actions={actions} isPending={isPending} />
          </Cluster>
        )}
      </TableCell>
    </TableRow>
  );
}

/** A link to an account's profile, named by its handle, which reads left to right. */
export function AccountLink({ acct }: { acct: string }) {
  const handle = `@${acct}`;
  return (
    <Link href={profilePath(acct)}>
      <span dir="ltr">{handle}</span>
    </Link>
  );
}

/** Rejecting a request, after an alert dialog with an optional reason that the requester sees. */
function RejectRequest({
  request,
  actions,
  isPending,
}: {
  request: AdminCustomEmojiRequest;
  actions: CustomEmojiRequestActions;
  isPending: boolean;
}) {
  const t = useT();
  const name = requestInText(request);
  const [reason, setReason] = useState("");
  const [isRejecting, setIsRejecting] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <DialogTrigger>
      <Button
        size="sm"
        variant="danger"
        isDisabled={isPending}
        aria-label={t("client:admin.custom_emoji_requests.reject_request", { emoji: name })}
      >
        {t("client:admin.custom_emoji_requests.reject")}
      </Button>
      <ConfirmDialog
        title={t("client:admin.custom_emoji_requests.reject.title", { emoji: name })}
        description={t("client:admin.custom_emoji_requests.reject.description")}
        confirmLabel={t("client:admin.custom_emoji_requests.reject")}
        cancelLabel={t("client:admin.custom_emoji_requests.cancel")}
        isPending={isRejecting}
        {...(failed &&
          actions.error !== undefined && {
            errorMessage: t("client:admin.custom_emoji_requests.decide_failed", { reason: actions.error }),
          })}
        onConfirm={(close) => {
          setIsRejecting(true);
          setFailed(false);
          void actions.reject(request, reason).then((rejected) => {
            setIsRejecting(false);
            if (rejected) close();
            else setFailed(true);
          });
        }}
      >
        <ReasonField
          label={t("client:admin.custom_emoji_requests.reason")}
          description={t("client:admin.custom_emoji_requests.reason.description")}
          value={reason}
          onChange={setReason}
          characterLimit={rejectReasonLimit}
          formatRemaining={(remaining) =>
            remaining < 0
              ? t("client:admin.custom_emoji_requests.characters_over", { count: -remaining })
              : t("client:admin.custom_emoji_requests.characters_left", { count: remaining })
          }
          errorMessage={t("client:admin.custom_emoji_requests.reason.too_long", { max: String(rejectReasonLimit) })}
        />
      </ConfirmDialog>
    </DialogTrigger>
  );
}
