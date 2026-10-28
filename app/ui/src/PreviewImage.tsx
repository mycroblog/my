// SPDX-License-Identifier: AGPL-3.0-only
import type { ComponentPropsWithRef } from "react";
import { tv } from "tailwind-variants";

const previewImage = tv({
  base: "aspect-video w-full max-w-md rounded-lg bg-surface-sunken object-cover",
});

export type PreviewImageProps = Omit<ComponentPropsWithRef<"img">, "className" | "style">;

/** A 16:9 preview of an image the user chose, before it is saved. */
export function PreviewImage(props: PreviewImageProps) {
  return <img {...props} className={previewImage()} />;
}
