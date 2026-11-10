// SPDX-License-Identifier: AGPL-3.0-only
// The viewer's private note on the account of a profile (FR-310; web-client/screens/profile.md): a titled region under
// the profile's header that says only the viewer sees it, with the note as written (or that there is none) and a
// button that opens a dialog (WAI-ARIA APG, dialog pattern) to write it. The dialog saves on submit; an empty note
// removes it. A saved note closes the dialog, which gives focus back to the button, and is announced in a toast; a
// failure stays in the dialog.
import { Box, Button, Dialog, DialogActions, Hint, InlineError, Section, Stack, Text, TextArea } from "@my/ui";
import { type SubmitEvent, useState } from "react";
import { useT } from "../../../lib/i18n";
import { maxNoteLength, useAccountNote } from "../../relationship";
import { isolate } from "../hooks/useProfile";

export type AccountNoteProps = {
  accountId: string;
  /** The account's name in the UI. */
  name: string;
  /** The viewer's note on the account, "" without one. */
  note: string;
};

export function AccountNote({ accountId, name, note }: AccountNoteProps) {
  const t = useT();
  const [isOpen, setIsOpen] = useState(false);
  const values = { name: isolate(name) };
  return (
    // Inset like the profile's other notes under its header.
    <Box paddingX="4">
      <Section title={t("client:profile.note.title")} description={t("client:profile.note.description")}>
        <Stack gap="3" align="start">
          {note === "" ? (
            <Hint>{t("client:profile.note.empty", values)}</Hint>
          ) : (
            // The note is the viewer's own text, in any script (web-client/README.md Y10).
            <Text as="p" dir="auto" whitespace="preWrap" wrap="anywhere">
              {note}
            </Text>
          )}
          <Button
            onPress={() => {
              setIsOpen(true);
            }}
          >
            {note === "" ? t("client:profile.note.add") : t("client:profile.note.edit")}
          </Button>
        </Stack>
      </Section>
      <Dialog
        title={t("client:profile.note.dialog_title", values)}
        closeLabel={t("client:profile.note.close")}
        size="sm"
        isDismissable={false}
        isOpen={isOpen}
        onOpenChange={setIsOpen}
      >
        {({ close }) => <AccountNoteForm accountId={accountId} name={name} note={note} close={close} />}
      </Dialog>
    </Box>
  );
}

type AccountNoteFormProps = AccountNoteProps & { close: () => void };

function AccountNoteForm({ accountId, name, note, close }: AccountNoteFormProps) {
  const t = useT();
  const [draft, setDraft] = useState(note);
  const state = useAccountNote(accountId, name, close);
  const tooLong = Array.from(draft).length > maxNoteLength;
  const formatRemaining = (remaining: number) =>
    remaining < 0
      ? t("client:profile.note.characters_over", { count: -remaining })
      : t("client:profile.note.characters_left", { count: remaining });
  const submit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!tooLong) state.save(draft);
  };
  return (
    <Stack as="form" gap="4" onSubmit={submit} noValidate>
      <TextArea
        label={t("client:profile.note.field")}
        description={t("client:profile.note.field_description", { name: isolate(name) })}
        name="note"
        value={draft}
        onChange={setDraft}
        rows={4}
        characterLimit={maxNoteLength}
        formatRemaining={formatRemaining}
        autoFocus
      />
      {state.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:profile.note.save_failed", { reason: state.error })}
        </InlineError>
      )}
      <DialogActions>
        <Button onPress={close}>{t("client:profile.note.cancel")}</Button>
        <Button type="submit" variant="primary" isPending={state.isPending} isDisabled={tooLong}>
          {t("client:profile.note.save")}
        </Button>
      </DialogActions>
    </Stack>
  );
}
