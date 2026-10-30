// SPDX-License-Identifier: AGPL-3.0-only
// An area that takes files dropped on it (the compose form's attachments). It reacts only to a drag that carries
// files: text and links dragged over it are left to the fields inside, which insert them as they always do. While
// files are carried over it, a dashed outline and the label cover the area, so the drop target is shown by shape and
// words as well as color (WCAG 1.4.1). Dragging is never the only way: the area's owner keeps a button that picks the
// same files (WCAG 2.5.7), so the overlay is decorative and hidden from assistive technology.
// The browser's own drag and drop events are used rather than React Aria's useDrop, which takes every drop over its
// area and would keep text from being dropped into the fields.
import { type DragEvent, type ReactNode, useRef, useState } from "react";
import { tv } from "tailwind-variants";

const fileDropZone = tv({
  slots: {
    base: "relative",
    overlay: [
      "pointer-events-none absolute inset-0 z-10 flex items-center justify-center rounded-lg",
      "border-2 border-dashed border-accent bg-soft p-4 text-center text-base font-semibold text-accent",
    ],
  },
});

export type FileDropZoneProps = {
  children: ReactNode;
  /** Called with the files dropped on the area, in their order. */
  onDropFiles: (files: File[]) => void;
  /** Shown over the area while files are carried over it ("Drop to attach"). */
  label: string;
  /** Takes no files: a drag of files over the area shows nothing and the drop does nothing. */
  isDisabled?: boolean;
};

/** Whether a drag carries files (from the file manager, or an image dragged from a page). */
function carriesFiles(event: DragEvent): boolean {
  return Array.from(event.dataTransfer.types).includes("Files");
}

/** An area that takes dropped files and shows itself as the drop target while files are dragged over it. */
export function FileDropZone({ children, onDropFiles, label, isDisabled = false }: FileDropZoneProps) {
  // dragenter and dragleave fire for every element inside the area too: the area is the target while the count of
  // the elements entered and not yet left is above zero.
  const depthRef = useRef(0);
  const [isDropTarget, setIsDropTarget] = useState(false);
  const slots = fileDropZone();
  return (
    <div
      className={slots.base()}
      data-drop-target={isDropTarget && !isDisabled ? "" : undefined}
      onDragEnter={(event) => {
        if (!carriesFiles(event)) return;
        depthRef.current += 1;
        setIsDropTarget(true);
      }}
      onDragLeave={(event) => {
        if (!carriesFiles(event)) return;
        depthRef.current = Math.max(0, depthRef.current - 1);
        if (depthRef.current === 0) setIsDropTarget(false);
      }}
      onDragOver={(event) => {
        if (!carriesFiles(event)) return;
        // Taking the dragover is what lets the files drop here; "none" refuses them, and the browser then neither
        // drops them nor opens them in place of the page.
        event.preventDefault();
        event.dataTransfer.dropEffect = isDisabled ? "none" : "copy";
      }}
      onDrop={(event) => {
        if (!carriesFiles(event)) return;
        event.preventDefault();
        depthRef.current = 0;
        setIsDropTarget(false);
        if (!isDisabled) onDropFiles(Array.from(event.dataTransfer.files));
      }}
    >
      {children}
      {isDropTarget && !isDisabled && (
        <div className={slots.overlay()} aria-hidden="true">
          {label}
        </div>
      )}
    </div>
  );
}
