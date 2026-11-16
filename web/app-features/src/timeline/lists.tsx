// SPDX-License-Identifier: AGPL-3.0-only

// The reader's lists (FR-211, ADR-0013 §11–§15): the index of the lists they made under its title with New list,
// oldest first, each a row that opens its timeline with how many accounts are in it; a list is made in place at the
// index's top, New list giving way while it is, and the server's refusal at 50 lists is said under the name, Create
// dimmed. A list's timeline, under the back button to the index, the list as the screen's title with how many and its
// menu at the end (rename it in place of the title, its members, delete it after a confirmation, which goes back to
// the index), then its posts in the column every timeline has, new ones coming by the stream as the home timeline's
// do; a list that failed to load says so with Try again, and one deleted, that it is gone. Its members are a screen of
// their own (list-members.tsx).

import { Column } from "@my/app-components";
import {
  listLimitOf,
  listQuery,
  listsQuery,
  listTimelineQuery,
  refusal,
  useList,
  useLists,
  useListTimeline,
  useListWrites,
  type AppData,
  type OneList,
} from "@my/app-data";
import {
  BackButton,
  Button,
  ButtonPair,
  ConfirmDialog,
  FeedNotice,
  MenuButton,
  MenuGroup,
  MenuItem,
  ModalPresence,
  OpenRow,
  ScreenStack,
  ScreenTitle,
  TextButton,
  TextField,
  useLocale,
} from "@my/design-system";
import { useId, useRef, useState } from "react";
import { stringsIn } from "../locale";
import { usePostActions, usePostNotice, usePostOwn, usePostPicker, usePostVoting } from "../post-actions";
import { TranslatablePost } from "../post-translation";
import { useNewPostsColumn } from "./new-posts";

type T = ReturnType<typeof stringsIn>["t"];

/** What the index's loader waits for: the reader's lists, from the cache unless a write marked them stale. */
export async function loadLists(data: AppData): Promise<void> {
  await data.queryClient.query({ ...listsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** What a list's loader waits for: the list and its first page. */
export async function loadListTimeline(data: AppData, id: string): Promise<void> {
  await Promise.all([
    data.queryClient.query({ ...listQuery(data, id), staleTime: Infinity }),
    data.queryClient.infiniteQuery({ ...listTimelineQuery(data, id), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/** What a refused or failed write of a list says under its field: its limit, the server's words, or that it failed. */
export function listWriteError(error: unknown, t: T): string {
  const limit = listLimitOf(error);
  if (limit === "lists") return t("timeline.list_limit");
  if (limit === "members") return t("timeline.list_members_limit");
  return refusal(error) ?? t("timeline.list_write_failed");
}

/**
 * A list's name in place, a field over Cancel and the action (Create, Save): the action sends what is written, and is
 * dimmed while the server refuses at its limit (`dimmed`), what is wrong said under the field.
 */
function NameForm({
  initial,
  action,
  error,
  dimmed,
  busy,
  onCancel,
  onSend,
  t,
}: {
  initial?: string | undefined;
  action: string;
  error: string | undefined;
  dimmed: boolean;
  busy: boolean;
  onCancel: () => void;
  onSend: (name: string) => void;
  t: T;
}) {
  const id = useId();
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        const name = new FormData(e.currentTarget).get("name");
        if (!busy && !dimmed) onSend(typeof name === "string" ? name : "");
      }}
    >
      <ScreenStack>
        <TextField
          id={id}
          name="name"
          label={t("timeline.list_name")}
          defaultValue={initial}
          error={error}
          required
          autoFocus
        />
        <ButtonPair>
          <Button kind="quiet" size="timeline" onPress={onCancel} isDisabled={busy}>
            {t("timeline.list_cancel")}
          </Button>
          {/* Dimmed, never busy, while it is sent: React Aria announces a pending button by its id, which the form
              takes away as it closes, and the announcement would be left naming nothing. */}
          <Button kind="primary" size="timeline" type="submit" isDisabled={dimmed || busy}>
            {action}
          </Button>
        </ButtonPair>
      </ScreenStack>
    </form>
  );
}

/** A refused or failed write as its form shows it: what it says, and whether it was the server's limit. */
type Refusal = { readonly text: string; readonly limit: boolean };

/** A list made in place at the index's top; `onClose` closes it, made or not. */
function CreateList({ onClose, t }: { onClose: () => void; t: T }) {
  const writes = useListWrites();
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState<Refusal | undefined>(undefined);
  return (
    <NameForm
      action={t("timeline.list_create")}
      error={refused?.text}
      dimmed={refused?.limit ?? false}
      busy={busy}
      onCancel={onClose}
      onSend={(name) => {
        setBusy(true);
        writes.create(name).then(onClose, (e: unknown) => {
          setBusy(false);
          setRefused({ text: listWriteError(e, t), limit: listLimitOf(e) === "lists" });
        });
      }}
      t={t}
    />
  );
}

/**
 * The reader's lists under the screen's title with New list, each a row that opens its timeline; `create` opens the
 * name of a new list at once (New list of a profile's Add to list).
 */
export function ListIndex({ create = false }: { create?: boolean }) {
  const { t } = stringsIn(useLocale().locale);
  const [creating, setCreating] = useState(create);
  const { status } = useLists();
  return (
    <ScreenStack>
      <ScreenTitle
        glyph="list"
        actions={
          !creating &&
          status === "success" && (
            <Button
              size="timeline"
              kind="primary"
              onPress={() => {
                setCreating(true);
              }}
            >
              {t("timeline.list_new")}
            </Button>
          )
        }
      >
        {t("timeline.lists_title")}
      </ScreenTitle>
      {creating && (
        <CreateList
          onClose={() => {
            setCreating(false);
          }}
          t={t}
        />
      )}
      <ListRows />
    </ScreenStack>
  );
}

/** The rows of the reader's lists, or what stands in their place: that they load, failed with Try again, or are none. */
function ListRows() {
  const { t } = stringsIn(useLocale().locale);
  const { lists, status, refetch } = useLists();
  if (status === "pending") return <FeedNotice>{t("timeline.lists_loading")}</FeedNotice>;
  if (status === "error")
    return (
      <FeedNotice alert action={<TextButton onPress={refetch}>{t("timeline.retry")}</TextButton>}>
        {t("timeline.lists_failed")}
      </FeedNotice>
    );
  if (lists.length === 0) return <FeedNotice>{t("timeline.lists_empty")}</FeedNotice>;
  return (
    <nav aria-label={t("timeline.lists")}>
      <ul className="m-0 grid list-none p-0">
        {lists.map((list) => (
          <li key={list.id}>
            <OpenRow
              href={`/lists/${encodeURIComponent(list.id)}`}
              icon="list"
              title={list.name}
              hint={t("timeline.list_members", { count: list.members_count })}
            />
          </li>
        ))}
      </ul>
    </nav>
  );
}

/** What a list's screen says until the list is read: that it loads, that it failed with Try again, or that it is gone. */
function ListNotice({ status, refetch }: { status: OneList["status"]; refetch: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  if (status === "gone") return <FeedNotice>{t("timeline.list_gone")}</FeedNotice>;
  if (status === "error")
    return (
      <FeedNotice alert action={<TextButton onPress={refetch}>{t("timeline.retry")}</TextButton>}>
        {t("timeline.list_failed")}
      </FeedNotice>
    );
  return <FeedNotice>{t("timeline.list_loading")}</FeedNotice>;
}

/** The path of the members of the list of `id`. */
const listMembersPath = (id: string) => `/lists/${encodeURIComponent(id)}/members`;

/** What may be done with a list from its own screen: rename it, its members, delete it (after a confirmation). */
function ListMenu({ id, onRename, onDelete, t }: { id: string; onRename: () => void; onDelete: () => void; t: T }) {
  return (
    <MenuButton icon="more" label={t("timeline.list_more")}>
      <MenuGroup>
        <MenuItem id="rename" icon="edit" onAction={onRename}>
          {t("timeline.list_rename")}
        </MenuItem>
        <MenuItem id="members" icon="members" href={listMembersPath(id)}>
          {t("timeline.list_members_open")}
        </MenuItem>
      </MenuGroup>
      <MenuGroup>
        <MenuItem id="delete" icon="delete" onAction={onDelete}>
          {t("timeline.list_delete")}
        </MenuItem>
      </MenuGroup>
    </MenuButton>
  );
}

/** Renaming the list of `id` in place of its title; `onClose` closes it, renamed or not. */
function RenameList({ id, name, onClose, t }: { id: string; name: string; onClose: () => void; t: T }) {
  const writes = useListWrites();
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState<string | undefined>(undefined);
  return (
    <NameForm
      initial={name}
      action={t("timeline.list_save")}
      error={refused}
      dimmed={false}
      busy={busy}
      onCancel={onClose}
      onSend={(next) => {
        setBusy(true);
        writes.rename(id, next).then(onClose, (e: unknown) => {
          setBusy(false);
          setRefused(listWriteError(e, t));
        });
      }}
      t={t}
    />
  );
}

/**
 * The timeline of the reader's list of `id`, whose new posts come by the stream as the home timeline's do; `onBack`
 * goes back to the index, as deleting the list does.
 */
export function ListTimeline({ id, onBack }: { id: string; onBack: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  const { list, status, refetch } = useList(id);
  const onAction = usePostActions();
  const votingOf = usePostVoting();
  const ownOf = usePostOwn();
  const picker = usePostPicker();
  const timeline = useListTimeline(id);
  const newPosts = useNewPostsColumn({ list: id });
  const writes = useListWrites();
  const failed = usePostNotice();
  const [renaming, setRenaming] = useState(false);
  const [asking, setAsking] = useState(false);
  const deleting = useRef(false);
  const back = <BackButton onPress={onBack}>{t("timeline.list_back")}</BackButton>;
  if (list === undefined)
    return (
      <ScreenStack>
        {back}
        <ListNotice status={status} refetch={refetch} />
      </ScreenStack>
    );
  // A press while the server answers does nothing; the dialog shows no busy button, which would be announced by an id
  // the dialog takes away as it closes.
  const remove = () => {
    if (deleting.current) return;
    deleting.current = true;
    writes.remove(id).then(
      () => {
        deleting.current = false;
        onBack();
      },
      (e: unknown) => {
        deleting.current = false;
        setAsking(false);
        failed(e, t("timeline.list_write_failed"), remove);
      },
    );
  };
  return (
    <ScreenStack>
      {renaming ? (
        <>
          {back}
          <RenameList
            id={id}
            name={list.name}
            onClose={() => {
              setRenaming(false);
            }}
            t={t}
          />
        </>
      ) : (
        <ScreenTitle
          back={back}
          glyph="list"
          count={t("timeline.list_members", { count: list.members_count })}
          actions={
            <ListMenu
              id={id}
              onRename={() => {
                setRenaming(true);
              }}
              onDelete={() => {
                setAsking(true);
              }}
              t={t}
            />
          }
        >
          {list.name}
        </ScreenTitle>
      )}
      <Column
        label={t("timeline.list", { name: list.name })}
        {...timeline}
        {...newPosts}
        onAction={onAction}
        postProps={(s) => ({ voting: votingOf(s), own: ownOf(s), picker })}
        postAs={TranslatablePost}
        empty={<FeedNotice>{t("timeline.list_empty")}</FeedNotice>}
      />
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            key="delete"
            title={t("timeline.list_delete_title", { name: list.name })}
            cancel={t("timeline.list_cancel")}
            confirm={t("timeline.list_delete_confirm")}
            onCancel={() => {
              if (!deleting.current) setAsking(false);
            }}
            onConfirm={remove}
          >
            {t("timeline.list_delete_text")}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </ScreenStack>
  );
}
