// SPDX-License-Identifier: AGPL-3.0-only
// The lists page below its heading (web-client/screens/lists.md, ADR-0122): the list-and-add part (`ListSection`) with
// the user's lists, oldest first, its count over the most lists, and the button that opens the form that creates one (a
// note in its place once the user has the most). Each row has the list's name (a link to its timeline) and its number of
// members, "Manage members" (a disclosure that opens the members' region under the row), "Rename" (the form in the
// row's place) and "Delete" (after an alert dialog: the members' choice is lost, the follows stay). What creating did
// shows above the rows; renaming and deleting are announced, and focus goes back to the row's button, or to the next
// row (web-client/16-look-states-and-wording.md G24).
import {
  Box,
  Button,
  Cluster,
  ConfirmDialog,
  FormActionBar,
  Heading,
  Hint,
  Link,
  ListSection,
  Stack,
  TextField,
} from "@my/ui";
import { useEffect, useId, useRef, useState } from "react";
import { useT } from "../../../lib/i18n";
import { listPath } from "../../../lib/paths";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { type List, listNameInputLimit, maxLists } from "../api/lists";
import { isolate, useLimits } from "../hooks/names";
import {
  type DeleteList,
  type ListNameForm,
  useCreateListForm,
  useDeleteList,
  useListCollection,
  useRenameListForm,
} from "../hooks/useLists";
import { ListMembersPanel } from "./ListMembersPanel";

export function ListsView() {
  const t = useT();
  const limits = useLimits();
  const lists = useListCollection();
  const form = useCreateListForm();
  const remove = useDeleteList();
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    lists.map((list) => list.id),
  );
  const [confirming, setConfirming] = useState<List | null>(null);
  return (
    <>
      <ListSection
        title={t("client:lists.list.title")}
        items={lists}
        itemKey={(list) => list.id}
        renderItem={(list) => (
          <ListRow
            list={list}
            remove={remove}
            onDelete={() => {
              setConfirming(list);
            }}
          />
        )}
        listRef={listRef}
        countLabel={t("client:lists.count", { count: lists.length, max: limits.lists })}
        limit={maxLists}
        empty={{ heading: t("client:lists.empty"), description: t("client:lists.empty.description") }}
        add={
          form.isFull
            ? { unavailable: t("client:lists.limit", { max: limits.lists }) }
            : {
                label: t("client:lists.create"),
                panel: (close) => (
                  <ListNameFields form={form} submitLabel={t("client:lists.create.submit")} close={close} />
                ),
              }
        }
        result={form.status === undefined ? undefined : { tone: "success", message: form.status }}
      />
      <ConfirmDialog
        isOpen={confirming !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) setConfirming(null);
        }}
        title={t("client:lists.delete.title", { name: isolate(confirming?.name ?? "") })}
        description={t("client:lists.delete.description")}
        confirmLabel={t("client:lists.delete")}
        cancelLabel={t("client:lists.delete.cancel")}
        tone="danger"
        isPending={confirming !== null && remove.pendingId === confirming.id}
        onConfirm={(close) => {
          if (confirming === null) return;
          remove.run(confirming, (deleted) => {
            if (deleted) removed(confirming.id);
            close();
          });
        }}
      />
    </>
  );
}

type ListNameFieldsProps = {
  form: ListNameForm;
  submitLabel: string;
  /** Closes the form; called once the name is saved too. */
  close: () => void;
};

/** The name field of a list and the form's action bar: the panel that creates a list, and the row that renames one. */
function ListNameFields({ form, submitLabel, close }: ListNameFieldsProps) {
  const t = useT();
  return (
    <Stack
      as="form"
      onSubmit={(event) => {
        form.submit(event, close);
      }}
      gap="4"
    >
      <Box width="sm">
        <TextField
          label={t("client:lists.name")}
          description={t("client:lists.name.description")}
          value={form.name}
          onChange={form.changeName}
          isRequired
          maxLength={listNameInputLimit}
          autoComplete="off"
          errorMessage={t("client:lists.name.required")}
        />
      </Box>
      <FormActionBar result={form.error === undefined ? undefined : { tone: "danger", message: form.error }}>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {submitLabel}
        </Button>
        <Button onPress={close}>{t("client:lists.cancel")}</Button>
      </FormActionBar>
    </Stack>
  );
}

type ListRowProps = { list: List; remove: DeleteList; onDelete: () => void };

/**
 * A list: its name as the link to its timeline and its number of members, with its buttons; the rename form takes the
 * row's place while it is open, and the members' region opens under it.
 */
function ListRow({ list, remove, onDelete }: ListRowProps) {
  const t = useT();
  // `returned` once the rename form closed, which gives the Rename button focus back.
  const [renaming, setRenaming] = useState<"closed" | "open" | "returned">("closed");
  const [managing, setManaging] = useState(false);
  const renameButtonId = useId();
  const membersId = useId();
  useEffect(() => {
    // Focus is moved in the DOM, outside React, once the row shows its buttons again.
    if (renaming === "returned") document.getElementById(renameButtonId)?.focus();
  }, [renaming, renameButtonId]);
  const name = isolate(list.name);
  return (
    <Stack gap="4">
      {renaming === "open" ? (
        <RenameList
          list={list}
          close={() => {
            setRenaming("returned");
          }}
        />
      ) : (
        <Stack gap="3" rowAt="md">
          <Stack as="p" shrink grow>
            <Link href={listPath(list.id)} variant="standalone">
              <span dir="auto">{list.name}</span>
            </Link>
            <Hint as="span">{t("client:lists.member_count", { count: list.members_count })}</Hint>
          </Stack>
          <Cluster gap="2">
            <Button
              size="sm"
              aria-expanded={managing}
              aria-controls={membersId}
              aria-label={t("client:lists.list_action", { action: t("client:lists.members.manage"), name })}
              onPress={() => {
                setManaging(!managing);
              }}
            >
              {t("client:lists.members.manage")}
            </Button>
            <Button
              id={renameButtonId}
              size="sm"
              aria-label={t("client:lists.list_action", { action: t("client:lists.rename"), name })}
              onPress={() => {
                setRenaming("open");
              }}
            >
              {t("client:lists.rename")}
            </Button>
            <Button
              size="sm"
              variant="danger"
              isPending={remove.pendingId === list.id}
              aria-label={t("client:lists.list_action", { action: t("client:lists.delete"), name })}
              onPress={onDelete}
            >
              {t("client:lists.delete")}
            </Button>
          </Cluster>
        </Stack>
      )}
      <div id={membersId} hidden={!managing}>
        {managing && <ListMembersPanel list={list} />}
      </div>
    </Stack>
  );
}

/** The form that renames a list, in the row's place: its title (`h3`), the name field with the name, and its bar. */
function RenameList({ list, close }: { list: List; close: () => void }) {
  const t = useT();
  const form = useRenameListForm(list);
  const headingId = useId();
  const ref = useRef<HTMLElement>(null);
  useEffect(() => {
    // Focus goes into the form's field once it has rendered (a DOM node outside React's state).
    ref.current?.querySelector<HTMLElement>("input")?.focus();
  }, []);
  return (
    <Box as="section" ref={ref} aria-labelledby={headingId} border="lg" background="sunken" padding="4">
      <Stack gap="4">
        <Heading level={3} id={headingId}>
          {t("client:lists.rename.title", { name: isolate(list.name) })}
        </Heading>
        <ListNameFields form={form} submitLabel={t("client:lists.rename.submit")} close={close} />
      </Stack>
    </Box>
  );
}
