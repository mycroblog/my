// SPDX-License-Identifier: AGPL-3.0-only
// One image of a draft: its preview, the progress of the upload over it, and the fields and actions beside it.
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";
import { Spinner } from "./Spinner";
import { Stack } from "./Stack";

const attachmentItem = tv({
  slots: {
    root: "flex flex-col gap-2 rounded-lg border border-border p-2 @md:flex-row",
    preview:
      "relative grid aspect-video w-full shrink-0 place-items-center overflow-hidden rounded-md bg-surface-sunken @md:w-40",
    image: "size-full object-cover",
    progress: "absolute inset-x-0 bottom-0 flex items-center gap-2 bg-surface-raised px-2 py-1 text-sm",
  },
});

export type AttachmentItemProps = {
  /** The preview's address (an object URL, or the server's preview); without it the well stays empty. */
  src: string | null;
  /** What is in progress ("Uploading"): a band over the preview with a spinner. Nothing when unset. */
  progressLabel?: string | undefined;
  /** The description field, the error and the actions, one above another beside the preview. */
  children: ReactNode;
};

/**
 * A `li` for an `AttachmentList`'s `<Stack as="ul">`: a framed row of a 16:9 preview and the fields, a column below
 * `@md` of the container and a row from there. The preview is decorative (`alt=""`): the description field next to it
 * says what the image shows.
 */
export function AttachmentItem({ src, progressLabel, children }: AttachmentItemProps) {
  const styles = attachmentItem();
  return (
    <li className={styles.root()}>
      <div className={styles.preview()}>
        {src !== null && <img src={src} alt="" className={styles.image()} />}
        {progressLabel !== undefined && (
          <span className={styles.progress()}>
            <Spinner size="sm" label={progressLabel} />
            <span aria-hidden="true">{progressLabel}</span>
          </span>
        )}
      </div>
      <Stack gap="2" shrink grow>
        {children}
      </Stack>
    </li>
  );
}
