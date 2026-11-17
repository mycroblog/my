import { afterEach, expect, test } from "bun:test";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Button, Menu, MenuItem, MenuTrigger, Popover } from "react-aria-components";

afterEach(cleanup);

test("plain react-aria-components menu: arrows move the focus", async () => {
  const user = userEvent.setup();
  render(
    <MenuTrigger>
      <Button aria-label="More">…</Button>
      <Popover>
        <Menu aria-label="More">
          <MenuItem id="a">Copy link</MenuItem>
          <MenuItem id="b">Block…</MenuItem>
        </Menu>
      </Popover>
    </MenuTrigger>,
  );
  await user.click(screen.getByRole("button", { name: "More" }));
  await screen.findByRole("menu");
  await user.keyboard("{ArrowDown}");
  await waitFor(() => expect(document.activeElement?.textContent).toBe("Copy link"));
  await user.keyboard("{ArrowDown}");
  await waitFor(() => expect(document.activeElement?.textContent).toBe("Block…"));
});
