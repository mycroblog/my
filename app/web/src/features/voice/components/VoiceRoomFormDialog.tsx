// SPDX-License-Identifier: AGPL-3.0-only
// The dialog of a room's settings (ADR-0100 §3, §9): starting a room from the list page, or the host changing its
// settings from the room's page. Closing it drops the draft, so the scrim does not close it (web-client/README.md
// Y11). The fields mount with the dialog's content, so every opening starts from the room as it is.
import {
  Button,
  Dialog,
  DialogActions,
  InlineError,
  Radio,
  RadioGroup,
  Stack,
  Switch,
  TextArea,
  TextField,
} from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  type VoiceRoom,
  voiceRoomLimits,
  type VoiceRoomSpeakers,
  voiceRoomSpeakers,
  type VoiceRoomVisibility,
  voiceRoomVisibilities,
} from "../api/voice";
import { type VoiceRoomFieldError, useVoiceRoomForm } from "../hooks/useVoiceRoomForm";

/** The label of each visibility. */
export const visibilityLabels: Readonly<Record<VoiceRoomVisibility, ClientMessageKey>> = {
  local: "client:voice.visibility.local",
  followers: "client:voice.visibility.followers",
  mutuals: "client:voice.visibility.mutuals",
  invited: "client:voice.visibility.invited",
};

/** The label of each choice of who speaks without asking. */
export const speakerLabels: Readonly<Record<VoiceRoomSpeakers, ClientMessageKey>> = {
  everyone: "client:voice.speakers.everyone",
  followers: "client:voice.speakers.followers",
  mutuals: "client:voice.speakers.mutuals",
  manual: "client:voice.speakers.manual",
};

function fieldError(error: VoiceRoomFieldError | undefined, field: VoiceRoomFieldError["field"]) {
  return error?.field === field ? { isInvalid: true, errorMessage: error.message } : {};
}

type VoiceRoomFormDialogProps = {
  /** The room whose settings change; without it the dialog starts a room. */
  room?: VoiceRoom;
  isOpen: boolean;
  onOpenChange: (isOpen: boolean) => void;
};

export function VoiceRoomFormDialog({ room, isOpen, onOpenChange }: VoiceRoomFormDialogProps) {
  const t = useT();
  return (
    <Dialog
      title={t(room === undefined ? "client:voice.form.new_title" : "client:voice.form.edit_title")}
      closeLabel={t("client:voice.form.close")}
      isDismissable={false}
      isOpen={isOpen}
      onOpenChange={onOpenChange}
    >
      {({ close }) => <VoiceRoomFields {...(room !== undefined && { room })} close={close} />}
    </Dialog>
  );
}

function VoiceRoomFields({ room, close }: { room?: VoiceRoom; close: () => void }) {
  const t = useT();
  const form = useVoiceRoomForm({ ...(room !== undefined && { room }), onDone: close });
  const { draft, change } = form;
  const formatRemaining = (remaining: number) =>
    remaining < 0
      ? t("client:voice.form.characters_over", { count: -remaining })
      : t("client:voice.form.characters_left", { count: remaining });
  return (
    <Stack as="form" gap="4" onSubmit={form.submit} noValidate>
      <TextField
        label={t("client:voice.form.name")}
        name="name"
        value={draft.name}
        onChange={(name) => {
          change({ name });
        }}
        dir="auto"
        isRequired
        autoComplete="off"
        autoFocus
        {...fieldError(form.fieldError, "name")}
      />
      <TextArea
        label={t("client:voice.form.description")}
        name="description"
        value={draft.description}
        onChange={(description) => {
          change({ description });
        }}
        rows={3}
        characterLimit={voiceRoomLimits.description}
        formatRemaining={formatRemaining}
        {...fieldError(form.fieldError, "description")}
      />
      <TextField
        label={t("client:voice.form.hashtags")}
        name="hashtags"
        description={t("client:voice.form.hashtags.description", { max: String(voiceRoomLimits.hashtags) })}
        value={draft.hashtags}
        onChange={(hashtags) => {
          change({ hashtags });
        }}
        dir="auto"
        autoComplete="off"
        {...fieldError(form.fieldError, "hashtags")}
      />
      <RadioGroup
        label={t("client:voice.form.visibility")}
        value={draft.visibility}
        onChange={(value) => {
          const visibility = voiceRoomVisibilities.find((v) => v === value);
          if (visibility !== undefined) change({ visibility });
        }}
      >
        {voiceRoomVisibilities.map((visibility) => (
          <Radio key={visibility} value={visibility}>
            {t(visibilityLabels[visibility])}
          </Radio>
        ))}
      </RadioGroup>
      <RadioGroup
        label={t("client:voice.form.speakers")}
        value={draft.speakers}
        onChange={(value) => {
          const speakers = voiceRoomSpeakers.find((s) => s === value);
          if (speakers !== undefined) change({ speakers });
        }}
      >
        {voiceRoomSpeakers.map((speakers) => (
          <Radio key={speakers} value={speakers}>
            {t(speakerLabels[speakers])}
          </Radio>
        ))}
      </RadioGroup>
      {form.offersPost && (
        <>
          <Switch
            isSelected={draft.post}
            onChange={(post) => {
              change({ post });
            }}
            description={t(
              draft.visibility === "local" ? "client:voice.form.post.local" : "client:voice.form.post.followers",
            )}
          >
            {t("client:voice.form.post")}
          </Switch>
          {draft.post && (
            <TextArea
              label={t("client:voice.form.post.text")}
              description={t("client:voice.form.post.text.description")}
              value={form.postText}
              onChange={(postText) => {
                change({ postText });
              }}
              rows={3}
            />
          )}
        </>
      )}
      {form.error !== undefined && (
        <InlineError size="inherit" alert>
          {t(form.mode === "new" ? "client:voice.form.start_failed" : "client:voice.form.save_failed", {
            reason: form.error,
          })}
        </InlineError>
      )}
      <DialogActions>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t(form.mode === "new" ? "client:voice.form.submit_new" : "client:voice.form.submit_edit")}
        </Button>
      </DialogActions>
    </Stack>
  );
}
