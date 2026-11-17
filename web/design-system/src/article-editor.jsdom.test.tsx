// SPDX-License-Identifier: AGPL-3.0-only

// The arrow keys of Write and Preview, which happy-dom cannot decide: there an arrow key moves the focus one way only.

import { expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ArticleMode } from "./article-editor";

test("the arrow keys go between Write and Preview both ways, stop at either end, and choose nothing", async () => {
  const user = userEvent.setup();
  const asked: string[] = [];
  render(
    <ArticleMode
      label="View"
      items={[
        { id: "write", name: "Write" },
        { id: "preview", name: "Preview" },
      ]}
      current="write"
      onChange={(id) => asked.push(id)}
    />,
  );
  const focused = () => document.activeElement?.textContent;

  await user.tab();
  expect(focused()).toBe("Write");
  await user.keyboard("{ArrowLeft}");
  expect(focused()).toBe("Write");
  await user.keyboard("{ArrowRight}");
  expect(focused()).toBe("Preview");
  await user.keyboard("{ArrowRight}");
  expect(focused()).toBe("Preview");
  await user.keyboard("{ArrowLeft}");
  expect(focused()).toBe("Write");

  expect(asked).toEqual([]);
  expect(screen.getByRole("radio", { name: "Write", checked: true })).not.toBeNull();
});
