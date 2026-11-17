// SPDX-License-Identifier: AGPL-3.0-only

// A modal that leaves through ModalPresence, with animations off (the preload's): what it leaves behind for
// assistive technology and the keyboard does not wait for its motion.

import { expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { PanelDialog } from "./dialog";
import { ModalPresence } from "./modal";

/** A button that opens a panel rendered in a ModalPresence, which the panel's close takes away. */
function History({ exits }: { exits: number[] }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button
        type="button"
        onClick={() => {
          setOpen(true);
        }}
      >
        Edit history
      </button>
      <ModalPresence onExited={() => exits.push(1)}>
        {open && (
          <PanelDialog
            title="Edit history"
            closeLabel="Close"
            onClose={() => {
              setOpen(false);
            }}
          >
            Edited yesterday.
          </PanelDialog>
        )}
      </ModalPresence>
    </>
  );
}

test("a modal that leaves is gone, the page is back in reach with the focus on what opened it, and it says so once", async () => {
  const user = userEvent.setup();
  const exits: number[] = [];
  render(<History exits={exits} />);
  const opener = screen.getByRole("button", { name: "Edit history" });

  await user.click(opener);
  await screen.findByRole("dialog", { name: "Edit history" });
  expect(exits).toEqual([]);
  await user.keyboard("{Escape}");

  expect(screen.queryByRole("dialog")).toBeNull();
  expect(opener.closest('[inert], [aria-hidden="true"]')).toBeNull();
  expect(document.activeElement).toBe(opener);
  expect(exits).toEqual([1]);
});

test("a modal opens again after it left, and says each time that it left", async () => {
  const user = userEvent.setup();
  const exits: number[] = [];
  render(<History exits={exits} />);

  for (const left of [1, 2]) {
    await user.click(screen.getByRole("button", { name: "Edit history" }));
    await screen.findByRole("dialog", { name: "Edit history" });
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(exits).toHaveLength(left);
  }
});
