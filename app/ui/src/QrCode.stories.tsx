// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { QrCode } from "./QrCode";

/** A finder pattern (7 × 7 modules) whose top-left module is at (x, y), as runs of dark modules per row. */
function finder(x: number, y: number): string {
  const rows = [
    [[0, 7]],
    [
      [0, 1],
      [6, 1],
    ],
    [
      [0, 1],
      [2, 3],
      [6, 1],
    ],
    [
      [0, 1],
      [2, 3],
      [6, 1],
    ],
    [
      [0, 1],
      [2, 3],
      [6, 1],
    ],
    [
      [0, 1],
      [6, 1],
    ],
    [[0, 7]],
  ];
  return rows
    .flatMap((runs, row) =>
      runs.map(
        ([start = 0, length = 0]) => `M${String(x + start)} ${String(y + row)}h${String(length)}v1h-${String(length)}z`,
      ),
    )
    .join("");
}

/** The three finder patterns of a 21-module code (version 1) inside a quiet zone of 4 modules, and a timing row. */
const path = `${finder(4, 4)}${finder(18, 4)}${finder(4, 18)}M12 10h1v1h-1zM14 10h1v1h-1zM16 10h1v1h-1z`;

const meta = {
  title: "Display/QrCode",
  component: QrCode,
  args: { size: 29, path, label: "QR code of the secret for the authenticator app" },
} satisfies Meta<typeof QrCode>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A picture named by its label, dark modules on a light ground in both themes. */
export const Default: Story = {
  play: async ({ canvas }) => {
    const code = canvas.getByRole("img", { name: "QR code of the secret for the authenticator app" });
    await expect(code).toHaveAttribute("viewBox", "0 0 29 29");
    await expect(getComputedStyle(code).colorScheme).toBe("light");
  },
};
