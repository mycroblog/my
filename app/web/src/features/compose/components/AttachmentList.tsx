// SPDX-License-Identifier: AGPL-3.0-only
import { AttachmentItem, Box, Button, InlineError, Stack, TextArea } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { maxDescriptionLength } from "../hooks/policy";
import { type AttachmentView, descriptionFieldId } from "../hooks/useAttachments";
import { useFormatRemaining } from "../hooks/useFormatRemaining";

export type AttachmentListProps = {
  items: readonly AttachmentView[];
  onDescribe: (key: string, description: string) => void;
  /** Removes an image; without it, the images stay (editing a post changes only their descriptions). */
  onRemove?: (key: string) => void;
  /** Whether the user's preferences ask for a description of every image. */
  requireDescriptions: boolean;
  /**
   * Whether the server requires a description of every image (`limits.require_media_description`): the fields are
   * required, and the browser keeps the form from being sent while one is empty or only spaces.
   */
  isDescriptionRequired?: boolean;
  isDisabled?: boolean;
};

/**
 * The images of the draft. Every image has a description field, always shown, whatever its state
 * (web-client/README.md Y9): the description can be written while the file uploads.
 */
export function AttachmentList({
  items,
  onDescribe,
  onRemove,
  requireDescriptions,
  isDescriptionRequired = false,
  isDisabled = false,
}: AttachmentListProps) {
  const t = useT();
  const hint = isDescriptionRequired
    ? "client:compose.media.description.server_required"
    : requireDescriptions
      ? "client:compose.media.description.required"
      : "client:compose.media.description.hint";
  const formatRemaining = useFormatRemaining();
  if (items.length === 0) return null;
  return (
    <Stack as="ul" aria-label={t("client:compose.media.list")} gap="3">
      {items.map((item) => {
        const progressKey =
          item.status === "uploading" ? "client:compose.media.uploading" : "client:compose.media.processing";
        return (
          <AttachmentItem
            key={item.key}
            src={item.src}
            progressLabel={item.status === "uploading" || item.status === "processing" ? t(progressKey) : undefined}
          >
            {item.status === "failed" && (
              <InlineError>{t("client:compose.media.failed", { reason: item.message })}</InlineError>
            )}
            <TextArea
              id={descriptionFieldId(item.key)}
              label={t("client:compose.media.description.label", { position: String(item.position) })}
              description={t(hint)}
              value={item.description}
              isRequired={isDescriptionRequired}
              validate={(value) =>
                isDescriptionRequired && value.trim() === "" ? t("client:compose.media.description.missing") : null
              }
              onChange={(description) => {
                onDescribe(item.key, description);
              }}
              rows={2}
              characterLimit={maxDescriptionLength}
              formatRemaining={formatRemaining}
              isReadOnly={isDisabled}
            />
            {onRemove !== undefined && (
              <Box self="start">
                <Button
                  size="sm"
                  variant="ghost"
                  isDisabled={isDisabled}
                  onPress={() => {
                    onRemove(item.key);
                  }}
                >
                  {t("client:compose.media.remove", { position: String(item.position) })}
                </Button>
              </Box>
            )}
          </AttachmentItem>
        );
      })}
    </Stack>
  );
}
