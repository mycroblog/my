// SPDX-License-Identifier: AGPL-3.0-only

// The requests for custom emoji as a holder of `manage_custom_emojis` reviews them (FR-915, ADR-0070 §2), at
// /admin/custom-emojis/requests, under the tabs Emoji | Requests: the pending ones by default, newest first, or those
// of another state; each its :shortcode:, what it asks (a new emoji or a new image), its image, who asked and when, a
// row that opens it over the list. Opened, a pending request shows its image and its facts, then Approve, the
// primary action, and Reject, which asks an optional reason the requester reads. A request the server cannot approve
// (its requester may no longer create emoji, or a local emoji has its shortcode now) says why in words, and only
// Reject stays; one decided meanwhile says so. What came of a decision is said in the screen's status, which takes the
// focus once the dialog has left.

import {
  emojiLimits,
  emojiRefusal,
  emojiRequestStates,
  useAdminEmojiRequests,
  useAdminEmojiWrites,
  type AdminCustomEmojiRequest,
  type CustomEmojiRequestState,
} from "@my/app-data";
import { sentenceHandleOf } from "@my/app-components";
import {
  Button,
  FactList,
  FeedNotice,
  FilterBar,
  FilterFacet,
  FormAlert,
  ModalPresence,
  PanelDialog,
  RecordPages,
  RecordTable,
  RelativeTime,
  Row,
  ScreenStack,
  ScreenTitle,
  SettingsResult,
  StateMark,
  TextField,
  type FilterValue,
  type RecordColumn,
  type RecordRow,
  type RecordState,
} from "@my/design-system";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { ReadNotice, useMoment, useStaffStrings, type T } from "../parts";
import { coded, EmojiImage, EmojiTabs } from "./emoji-parts";
import { useStillThere, type AdminSubProps } from "./parts";

/** How a request's state looks: what waits, what was made, what is over. */
const stateLook: Record<CustomEmojiRequestState, RecordState> = {
  pending: "pending",
  approved: "resolved",
  rejected: "resolved",
  withdrawn: "lifted",
  lapsed: "expired",
};

/** A request's state in words. */
const requestStateWords = (t: T, state: CustomEmojiRequestState): string => t("staff_emoji.request.state", { state });

/** Who asked, as a sentence names them; the words for an account that is gone. */
const requesterOf = (t: T, r: AdminCustomEmojiRequest): string =>
  r.requester === undefined ? t("staff.account_gone") : sentenceHandleOf(r.requester);

/** Where a review of a request is: open, being sent, or refused with why and whether only Reject stays. */
type Review =
  | { readonly kind: "open" | "approving" | "rejecting" | "asking" }
  | { readonly kind: "refused"; readonly why: string; readonly onlyReject: boolean };

/** The request opened over the list: its image and facts, Approve and Reject, and Reject's reason once asked. */
function RequestDialog({
  request,
  onClose,
  onDecided,
}: {
  request: AdminCustomEmojiRequest;
  onClose: () => void;
  onDecided: (said: string) => void;
}) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const writes = useAdminEmojiWrites();
  const there = useStillThere();
  const [review, setReview] = useState<Review>({ kind: "open" });
  const [reason, setReason] = useState("");
  const pending = request.state === "pending";
  const busy = review.kind === "approving" || review.kind === "rejecting";
  const asking = review.kind === "asking" || review.kind === "rejecting";
  const refusedOf = (error: unknown): Review => {
    const why = emojiRefusal(error);
    if (why === "requester_cannot_create")
      return { kind: "refused", why: t("staff_emoji.request.cannot_create"), onlyReject: true };
    if (why === "shortcode_taken")
      return {
        kind: "refused",
        why: t("staff_emoji.request.taken", { shortcode: request.shortcode }),
        onlyReject: true,
      };
    if (why === "not_pending") return { kind: "refused", why: t("staff_emoji.request.decided"), onlyReject: false };
    if (why === "invalid_reason")
      return {
        kind: "refused",
        why: t("staff_emoji.request.reason.rule", { max: emojiLimits.reason }),
        onlyReject: false,
      };
    return { kind: "refused", why: t("staff_emoji.request.failed.write"), onlyReject: false };
  };
  const approve = () => {
    setReview({ kind: "approving" });
    writes.approve(request.id).then(
      () => {
        if (there.current) onDecided(t("staff_emoji.request.approved", { shortcode: coded(request.shortcode) }));
      },
      (error: unknown) => {
        if (there.current) setReview(refusedOf(error));
      },
    );
  };
  const reject = () => {
    if (Array.from(reason).length > emojiLimits.reason) {
      setReview({
        kind: "refused",
        why: t("staff_emoji.request.reason.rule", { max: emojiLimits.reason }),
        onlyReject: false,
      });
      return;
    }
    setReview({ kind: "rejecting" });
    writes.reject(request.id, reason).then(
      () => {
        if (there.current) onDecided(t("staff_emoji.request.rejected", { shortcode: coded(request.shortcode) }));
      },
      (error: unknown) => {
        if (there.current) setReview(refusedOf(error));
      },
    );
  };
  const onlyReject = review.kind === "refused" && review.onlyReject;
  const decided = review.kind === "refused" && review.why === t("staff_emoji.request.decided");
  const foot =
    !pending || decided ? undefined : (
      <Row wrap align="center" justify="end" gap="screen-row-gap">
        {review.kind === "refused" && <FormAlert>{review.why}</FormAlert>}
        {asking || onlyReject ? (
          <Button
            size="timeline"
            touch
            busy={review.kind === "rejecting"}
            isDisabled={busy}
            onPress={
              asking
                ? reject
                : () => {
                    setReview({ kind: "asking" });
                  }
            }
          >
            {t(asking ? "staff_emoji.request.reject.confirm" : "staff_emoji.request.reject")}
          </Button>
        ) : (
          <>
            <Button
              size="timeline"
              touch
              isDisabled={busy}
              onPress={() => {
                setReview({ kind: "asking" });
              }}
            >
              {t("staff_emoji.request.reject")}
            </Button>
            <Button kind="primary" busy={review.kind === "approving"} isDisabled={busy} onPress={approve}>
              {t("staff_emoji.request.approve")}
            </Button>
          </>
        )}
      </Row>
    );
  return (
    <PanelDialog
      title={coded(request.shortcode)}
      closeLabel={t("staff_emoji.request.close")}
      onClose={onClose}
      foot={foot}
    >
      <ScreenStack>
        <FactList
          label={t("staff_emoji.request.facts")}
          items={[
            {
              term: t("staff_emoji.request.image"),
              value: <EmojiImage shortcode={request.shortcode} url={request.url} staticUrl={request.static_url} />,
            },
            { term: t("staff_emoji.request.kind"), value: t("staff_emoji.request.kind.name", { kind: request.kind }) },
            { term: t("staff_emoji.request.requester"), value: requesterOf(t, request) },
            { term: t("staff_emoji.request.created"), value: moment(request.created_at) },
            { term: t("staff_emoji.request.state.term"), value: requestStateWords(t, request.state) },
            ...(request.decided_at === undefined
              ? []
              : [{ term: t("staff_emoji.request.decided_at"), value: moment(request.decided_at) }]),
            ...(request.reason === undefined
              ? []
              : [{ term: t("staff_emoji.request.reason.term"), value: <bdi dir="auto">{request.reason}</bdi> }]),
          ]}
        />
        {asking && (
          <TextField
            id="emoji-request-reason"
            name="reason"
            label={t("staff_emoji.request.reason")}
            hint={t("staff_emoji.request.reason.hint", { max: emojiLimits.reason })}
            multiline
            rows={3}
            maxLength={emojiLimits.reason}
            autoFocus
            readOnly={busy}
            defaultValue={reason}
            onChange={setReason}
          />
        )}
      </ScreenStack>
    </PanelDialog>
  );
}

type Column = "image" | "requester" | "created" | "state";

/** The requests: their tabs, the state shown, a page of them, the request opened, and what came of a decision. */
export function AdminEmojiRequestsScreen({ go }: AdminSubProps): ReactNode {
  const { t } = useStaffStrings();
  const [state, setState] = useState<CustomEmojiRequestState>("pending");
  const [trail, setTrail] = useState<readonly (string | undefined)[]>([]);
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const page = useAdminEmojiRequests(state, cursor);
  const [open, setOpen] = useState<AdminCustomEmojiRequest | undefined>(undefined);
  const [said, setSaid] = useState<{ text: string; n: number } | undefined>(undefined);
  const saidRef = useRef<HTMLParagraphElement>(null);
  const focusSaid = useRef(false);
  useEffect(() => {
    if (!focusSaid.current || said === undefined || open !== undefined) return;
    focusSaid.current = false;
    saidRef.current?.focus();
  }, [said, open]);
  const states: FilterValue[] = emojiRequestStates.map((s) => ({
    id: s,
    label: requestStateWords(t, s),
    icon:
      s === "pending" ? "statePending" : s === "approved" ? "accepted" : s === "rejected" ? "rejected" : "stateLifted",
  }));
  const columns: RecordColumn<Column>[] = [
    { key: "image", label: t("staff_emoji.column.image"), kind: "fit" },
    { key: "requester", label: t("staff_emoji.request.requester"), bare: true },
    { key: "created", label: t("staff_emoji.request.created"), kind: "fit", bare: true },
    { key: "state", label: t("staff_emoji.column.state"), kind: "fit", beside: true },
  ];
  const rows = (page.items ?? []).map((r): RecordRow<Column> => ({
    id: r.id,
    head: coded(r.shortcode),
    caption: t("staff_emoji.request.kind.name", { kind: r.kind }),
    cells: {
      image: <EmojiImage shortcode={r.shortcode} url={r.url} staticUrl={r.static_url} />,
      requester: requesterOf(t, r),
      created: <RelativeTime date={r.created_at} />,
      state: <StateMark state={stateLook[r.state]}>{requestStateWords(t, r.state)}</StateMark>,
    },
  }));
  const previous = trail.at(-1);
  return (
    <>
      <ScreenStack busy={page.stale}>
        <ScreenTitle>{t("staff_emoji.entry")}</ScreenTitle>
        <EmojiTabs current="requests" go={go}>
          <ScreenStack busy={page.stale}>
            <FilterBar
              label={t("staff_emoji.request.filters")}
              applied={state !== "pending"}
              onClear={() => {
                setState("pending");
                setCursor(undefined);
                setTrail([]);
              }}
            >
              <FilterFacet
                label={t("staff_emoji.request.state.term")}
                any={t("staff_emoji.request.state.any")}
                values={states}
                chosen={state}
                onChoose={(id) => {
                  setState(emojiRequestStates.find((s) => s === id) ?? "pending");
                  setCursor(undefined);
                  setTrail([]);
                }}
              />
            </FilterBar>
            <SettingsResult ref={saidRef}>
              {said === undefined ? undefined : <span key={said.n}>{said.text}</span>}
            </SettingsResult>
            {page.items === undefined ? (
              <ReadNoticeOf status={page.status} retry={page.retry} />
            ) : (
              <>
                <RecordTable
                  label={t("staff_emoji.request.table")}
                  head={{ label: t("staff_emoji.column.shortcode") }}
                  columns={columns}
                  rows={rows}
                  busy={page.stale}
                  onOpen={(id) => {
                    setOpen(page.items?.find((r) => r.id === id));
                  }}
                  empty={
                    <FeedNotice>
                      {state === "pending" ? t("staff_emoji.request.empty") : t("staff_emoji.request.empty.state")}
                    </FeedNotice>
                  }
                />
                <RecordPages
                  previous={
                    previous === undefined && trail.length === 0
                      ? undefined
                      : {
                          onPress: () => {
                            setCursor(previous);
                            setTrail(trail.slice(0, -1));
                          },
                        }
                  }
                  next={
                    page.next === undefined
                      ? undefined
                      : {
                          onPress: () => {
                            setTrail([...trail, cursor]);
                            setCursor(page.next);
                          },
                        }
                  }
                />
              </>
            )}
          </ScreenStack>
        </EmojiTabs>
      </ScreenStack>
      <ModalPresence>
        {open !== undefined && (
          <RequestDialog
            key={open.id}
            request={open}
            onClose={() => {
              setOpen(undefined);
            }}
            onDecided={(text) => {
              focusSaid.current = true;
              setSaid((was) => ({ text, n: (was?.n ?? 0) + 1 }));
              setOpen(undefined);
            }}
          />
        )}
      </ModalPresence>
    </>
  );
}

/** That the requests load, or could not be read, with Try again. */
function ReadNoticeOf({ status, retry }: { status: "loading" | "failed" | "read"; retry: () => void }) {
  const { t } = useStaffStrings();
  return (
    <ReadNotice
      status={status === "failed" ? "error" : "pending"}
      loading={t("staff_emoji.request.loading")}
      failed={t("staff_emoji.request.failed")}
      retry={retry}
    />
  );
}
