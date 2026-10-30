// SPDX-License-Identifier: AGPL-3.0-only
import { createEvent, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { FileDropZone } from "./FileDropZone";

const photo = new File(["png"], "photo.png", { type: "image/png" });

/** A drag that carries `files`, or only text. */
function carrying(files: File[] | null) {
  return {
    dataTransfer: {
      types: files === null ? ["text/plain"] : ["Files"],
      files: files ?? [],
      dropEffect: "none",
    },
  };
}

function mountZone(props: { onDropFiles?: (files: File[]) => void; isDisabled?: boolean } = {}) {
  render(
    <FileDropZone
      label="Drop to attach"
      onDropFiles={props.onDropFiles ?? vi.fn()}
      isDisabled={props.isDisabled ?? false}
    >
      <textarea aria-label="Your post" />
    </FileDropZone>,
  );
  return screen.getByRole("textbox", { name: "Your post" });
}

describe("FileDropZone", () => {
  it("shows the drop target while files are dragged over it, also over the fields inside, and takes the drop", () => {
    const onDropFiles = vi.fn();
    const field = mountZone({ onDropFiles });
    // eslint-disable-next-line testing-library/no-node-access -- the area is the field's container, which has no role.
    const zone = field.parentElement as HTMLElement;

    fireEvent.dragEnter(zone, carrying([photo]));
    fireEvent.dragEnter(field, carrying([photo]));
    fireEvent.dragLeave(zone, carrying([photo]));
    // Still over the field inside the area.
    expect(screen.getByText("Drop to attach")).toBeInTheDocument();
    expect(zone).toHaveAttribute("data-drop-target");
    const over = createEvent.dragOver(field, carrying([photo]));
    fireEvent(field, over);
    expect(over.defaultPrevented).toBe(true);

    const drop = createEvent.drop(field, carrying([photo]));
    fireEvent(field, drop);
    expect(drop.defaultPrevented).toBe(true);
    expect(onDropFiles).toHaveBeenCalledWith([photo]);
    expect(screen.queryByText("Drop to attach")).not.toBeInTheDocument();
  });

  it("hides the drop target once the files leave the area", () => {
    const field = mountZone();
    fireEvent.dragEnter(field, carrying([photo]));
    expect(screen.getByText("Drop to attach")).toBeInTheDocument();
    fireEvent.dragLeave(field, carrying([photo]));
    expect(screen.queryByText("Drop to attach")).not.toBeInTheDocument();
  });

  it("leaves text dragged over it to the fields", () => {
    const onDropFiles = vi.fn();
    const field = mountZone({ onDropFiles });
    fireEvent.dragEnter(field, carrying(null));
    expect(screen.queryByText("Drop to attach")).not.toBeInTheDocument();
    const over = createEvent.dragOver(field, carrying(null));
    fireEvent(field, over);
    expect(over.defaultPrevented).toBe(false);
    const drop = createEvent.drop(field, carrying(null));
    fireEvent(field, drop);
    expect(drop.defaultPrevented).toBe(false);
    expect(onDropFiles).not.toHaveBeenCalled();
  });

  it("refuses files while disabled, without showing a drop target", () => {
    const onDropFiles = vi.fn();
    const field = mountZone({ onDropFiles, isDisabled: true });
    fireEvent.dragEnter(field, carrying([photo]));
    expect(screen.queryByText("Drop to attach")).not.toBeInTheDocument();
    const drag = carrying([photo]);
    fireEvent.dragOver(field, drag);
    expect(drag.dataTransfer.dropEffect).toBe("none");
    fireEvent.drop(field, carrying([photo]));
    expect(onDropFiles).not.toHaveBeenCalled();
  });
});
